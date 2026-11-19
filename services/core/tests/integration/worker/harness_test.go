// Package worker_test runs the job lifecycle of the worker against a real
// PostgreSQL cluster, the real audit sink and the real recovery service, with a
// fake process runner (the sandbox is M8) and a fake source for the rules, the
// policy and the approval (they belong to M16 to M19).
package worker_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/persistence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/recoverymarker"
	appjob "github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/application/job"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/application/recovery"
	appscope "github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/application/scope"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/actor"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/approval"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/audit"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/job"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/operation"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/scope"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/infrastructure/ids"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/tests/integration/pgtest"
)

func TestMain(m *testing.M) { os.Exit(pgtest.Run(m)) }

var fastPolicy = audit.RetryPolicy{MaxAttempts: 3, Delays: []time.Duration{5 * time.Millisecond, 10 * time.Millisecond}, AttemptTimeout: 2 * time.Second}

// fastTiming keeps the ratios of the real timing and shrinks the numbers.
var fastTiming = job.Timing{
	LeaseTTL: 300 * time.Millisecond, Heartbeat: 40 * time.Millisecond, ControlPoll: 10 * time.Millisecond,
	Reap: 20 * time.Millisecond, StopTimeout: 400 * time.Millisecond, Watermark: time.Second,
}

// clock is the clock of a worker. A test moves it forward to expire a lease.
type clock struct {
	mu     sync.Mutex
	offset time.Duration
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return time.Now().Add(c.offset)
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	c.offset += d
	c.mu.Unlock()
}

// flaky wraps the audit sink so that a test can make a kind of write fail.
type flaky struct {
	outbound.AuditSink
	failRequire, failRecord, degraded atomic.Bool
}

func (f *flaky) Require(ctx context.Context, e audit.Event) error {
	if f.failRequire.Load() {
		return outbound.ErrAuditNotRecorded
	}
	return f.AuditSink.Require(ctx, e)
}

func (f *flaky) Record(ctx context.Context, e audit.Event) error {
	if f.failRecord.Load() {
		return outbound.ErrAuditNotRecorded
	}
	return f.AuditSink.Record(ctx, e)
}

func (f *flaky) Gate(ctx context.Context) error {
	if f.degraded.Load() {
		return outbound.ErrAuditDegraded
	}
	return f.AuditSink.Gate(ctx)
}

type failingMarkers struct {
	outbound.RecoveryMarkerStore
	fail atomic.Bool
}

func (f *failingMarkers) Advance(ctx context.Context, m outbound.RecoveryMarker) error {
	if f.fail.Load() {
		return errors.New("injected marker failure")
	}
	return f.RecoveryMarkerStore.Advance(ctx, m)
}

// rig is an installation with a worker pool, a recovery service and a fake runner.
type rig struct {
	t       *testing.T
	c       *pgtest.Cluster
	b       pgtest.Base
	pool    *pgxpool.Pool
	api     *pgxpool.Pool
	rec     *recovery.Service
	markers *failingMarkers
	sink    *flaky
	runner  *fakeRunner
	inputs  *scopeInputs
	clock   *clock
	timing  job.Timing
	seq     int
	// markerDir holds the recovery marker file.
	markerDir string
}

func newRig(t *testing.T) *rig {
	t.Helper()
	c := pgtest.Get(t)
	c.Fresh(t)
	api := c.Pool(t, persistence.RoleAPI, 6)
	markerDir := filepath.Join(t.TempDir(), "state")
	store, err := recoverymarker.New(markerDir)
	if err != nil {
		t.Fatal(err)
	}
	markers := &failingMarkers{RecoveryMarkerStore: store}
	rec := recovery.NewService(recovery.Deps{Markers: markers, Repo: persistence.NewRecovery(api), NewID: ids.New, Now: time.Now})
	if st, err := rec.Check(t.Context()); err != nil || st.Required {
		t.Fatalf("first run: %+v, %v", st, err)
	}
	b := c.SeedBaseWithoutRecovery(t)
	pool := c.Pool(t, persistence.RoleWorker, 16)
	sink, err := persistence.NewAuditSink(pool, fastPolicy, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := &rig{t: t, c: c, b: b, pool: pool, api: api, rec: rec, markers: markers, sink: &flaky{AuditSink: sink}, runner: newRunner(), clock: &clock{}, timing: fastTiming, markerDir: markerDir}
	r.inputs = newInputs(r)
	return r
}

// worker builds a worker with the identity id.
func (r *rig) worker(id string) *appjob.Worker {
	r.t.Helper()
	jobs := persistence.NewJobs(r.pool)
	ev := appscope.New(appscope.Deps{Inputs: r.inputs, Decisions: jobs, Audit: r.sink, NewID: ids.New, Now: r.clock.Now})
	w, err := appjob.New(appjob.Deps{
		Store: jobs, Uses: persistence.NewApprovalUses(r.pool), Tx: persistence.NewTransactor(r.pool), Evaluator: ev, Runner: r.runner,
		Alerts: persistence.NewAlerts(r.pool), Audit: r.sink, Recovery: r.rec, Timing: r.timing, WorkerID: id, ClaimPoll: 10 * time.Millisecond,
		NewID: ids.New, Now: r.clock.Now,
	})
	if err != nil {
		r.t.Fatal(err)
	}
	return w
}

// control is the stop use case, as the routes of later milestones build it.
func (r *rig) control() *appjob.Control {
	return &appjob.Control{Store: persistence.NewJobs(r.api), Tx: persistence.NewTransactor(r.api), Audit: r.sink, NewID: ids.New}
}

func (r *rig) human() audit.Actor {
	who, err := audit.User(r.b.UserID)
	if err != nil {
		r.t.Fatal(err)
	}
	return who
}

func (r *rig) stop(jobID, reason string) error {
	return r.control().RequestStop(r.t.Context(), jobID, appjob.StopCause{
		Audit: r.human(), Transition: outbound.TransitionActor{Kind: "user", UserID: r.b.UserID}, Reason: reason,
	})
}

// fixture is a seeded job.
type fixture struct {
	JobID, OperationID, ApprovalID string
	Op                             operation.Operation
	Approval                       pgtest.Approval
}

type jobSpec struct {
	Risk        job.Risk
	Retry       job.RetryPolicy
	MaxAttempts int
	MaxUses     int
}

func categoryOf(r job.Risk) operation.Category {
	switch r {
	case job.LowImpact:
		return operation.CategoryHTTPProbe
	case job.Intrusive:
		return operation.CategoryStateChangingReplay
	}
	return operation.CategoryPassiveRecon
}

// seedJob inserts an operation, an approval for an active one, and a queued job.
func (r *rig) seedJob(spec jobSpec) fixture {
	r.t.Helper()
	if spec.Risk == "" {
		spec.Risk = job.LowImpact
	}
	if spec.Retry == "" {
		spec.Retry = job.RetryNone
	}
	if spec.MaxAttempts == 0 {
		spec.MaxAttempts = 1
	}
	r.seq++
	op, err := operation.New(operation.Spec{
		AssessmentID: r.b.AssessmentID, ToolKey: "httpx", Kind: "probe" + strconv.Itoa(r.seq), Category: categoryOf(spec.Risk), Target: "app.example.com", Port: 443,
		Params: map[string]string{"path_depth": "1"}, Destinations: []string{"app.example.com:443"},
		Limits: operation.Limits{MaxConcurrency: 2, MaxDuration: 10 * time.Minute, RequestsPerMinute: 60}, ManifestDigest: [32]byte{1},
	})
	if err != nil {
		r.t.Fatal(err)
	}
	hash := op.Hash()
	f := fixture{Op: op}
	conn := r.c.Owner(r.t)
	var approvalID *string
	if spec.Risk.NeedsApproval() {
		f.Approval = r.c.SeedApproval(r.t, r.b, pgtest.ApprovalSpec{
			Risk: string(spec.Risk), MaxUses: max(spec.MaxUses, 1), MaxAttempts: spec.MaxAttempts, OperationHash: hash[:],
		})
		f.OperationID, f.ApprovalID = f.Approval.OperationID, f.Approval.ID
		approvalID = &f.ApprovalID
	} else {
		if err := conn.QueryRow(r.t.Context(), `INSERT INTO morningstar.operations (id, program_id, assessment_id, operation_hash, tool_key, operation_kind, risk_class, canonical_target, manifest_digest)
			VALUES (uuidv7(), $1, $2, $3, 'subfinder', 'passive', 'passive', 'example.com', $4) RETURNING id`, r.b.ProgramID, r.b.AssessmentID, hash[:], pgtest.Random(32)).Scan(&f.OperationID); err != nil {
			r.t.Fatal(err)
		}
	}
	if err := conn.QueryRow(r.t.Context(), `INSERT INTO morningstar.jobs (id, program_id, assessment_id, job_kind, operation_id, risk_class, approval_id, scope_version_id, validation_id, max_execution_attempts, retry_policy, created_by)
		VALUES (uuidv7(), $1, $2, 'tool_operation', $3, $4, $5, $6, $7, $8, $9, $10) RETURNING id`,
		r.b.ProgramID, r.b.AssessmentID, f.OperationID, string(spec.Risk), approvalID, r.b.ScopeID, r.b.ValidationID, spec.MaxAttempts, string(spec.Retry), r.b.UserID).Scan(&f.JobID); err != nil {
		r.t.Fatal(err)
	}
	r.inputs.register(f.OperationID, op)
	return f
}

func (r *rig) scalar(sql string, args ...any) string {
	r.t.Helper()
	var v string
	if err := r.c.Owner(r.t).QueryRow(r.t.Context(), "SELECT ("+sql+")::text", args...).Scan(&v); err != nil {
		r.t.Fatalf("%s: %v", sql, err)
	}
	return v
}

func (r *rig) jobState(id string) string {
	return r.scalar(`SELECT state || '/' || control_state || '/' || claim_epoch || '/' || execution_attempts FROM morningstar.jobs WHERE id = $1`, id)
}

func (r *rig) uses(approvalID string) string {
	return r.scalar(`SELECT (SELECT uses_consumed FROM morningstar.approvals WHERE id = $1) || '/' || (SELECT count(*) FROM morningstar.approval_uses WHERE approval_id = $1)`, approvalID)
}

func (r *rig) events(action string, outcome audit.Outcome) string {
	return r.scalar(`SELECT count(*) FROM morningstar.audit_events WHERE action = $1 AND outcome = $2`, action, string(outcome))
}

// scopeInputs is the fake of the source that M16 to M19 provide. It answers from
// what the test seeded, and a test changes the policy to simulate a change.
type scopeInputs struct {
	r      *rig
	mu     sync.Mutex
	ops    map[string]operation.Operation
	policy scope.PolicyState
	valid  scope.ValidationState
	err    error
}

func newInputs(r *rig) *scopeInputs {
	var snap [32]byte
	copy(snap[:], r.b.PolicyHash)
	return &scopeInputs{
		r: r, ops: map[string]operation.Operation{},
		policy: scope.PolicyState{ScopeVersionID: r.b.ScopeID, PolicySnapshot: snap, IntrusiveEnabled: true},
		valid:  scope.ValidationState{ID: r.b.ValidationID, Current: true},
	}
}

func (i *scopeInputs) register(opID string, op operation.Operation) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.ops[opID] = op
}

func (i *scopeInputs) setPolicy(mod func(*scope.PolicyState)) {
	i.mu.Lock()
	defer i.mu.Unlock()
	mod(&i.policy)
}

func (i *scopeInputs) Load(ctx context.Context, j outbound.JobRecord) (outbound.ScopeInput, error) {
	i.mu.Lock()
	op, ok := i.ops[j.OperationID]
	policy, valid, err := i.policy, i.valid, i.err
	i.mu.Unlock()
	if err != nil {
		return outbound.ScopeInput{}, err
	}
	if !ok {
		return outbound.ScopeInput{}, errors.New("unknown operation")
	}
	rule, rerr := scope.NewRule(scope.KindDomain, false, "app.example.com", "", scope.DefaultPrefixMin())
	if rerr != nil {
		return outbound.ScopeInput{}, rerr
	}
	in := outbound.ScopeInput{Operation: op, Rules: scope.RuleSet{rule}, Policy: policy, Validation: valid}
	if j.ApprovalID != "" {
		a, err := i.approval(ctx, j.ApprovalID, op)
		if err != nil {
			return outbound.ScopeInput{}, err
		}
		in.Approval = &a
	}
	return in, nil
}

// approval rebuilds the approval of the database from the domain API, the way
// the adapter of M19 will: request, approve with the stored terms, replay the
// uses and the launches, and revoke.
func (i *scopeInputs) approval(ctx context.Context, id string, op operation.Operation) (approval.Approval, error) {
	var (
		status                string
		maxUses, maxAttempts  int
		expires               time.Time
		reauth                *string
		binding               approval.Binding
		policyHash            []byte
		scopeID, validationID string
	)
	err := i.r.c.Owner(i.r.t).QueryRow(ctx, `SELECT status, max_uses, max_attempts, expires_at, reauth_grant_id::text, scope_version_id::text, validation_id::text, policy_snapshot_hash FROM morningstar.approvals WHERE id = $1`, id).
		Scan(&status, &maxUses, &maxAttempts, &expires, &reauth, &scopeID, &validationID, &policyHash)
	if err != nil {
		return approval.Approval{}, err
	}
	copy(binding.PolicySnapshot[:], policyHash)
	binding.ScopeVersionID, binding.ValidationID = scopeID, validationID
	human, err := actor.New(actor.KindHuman, i.r.b.UserID)
	if err != nil {
		return approval.Approval{}, err
	}
	now := time.Now()
	a, err := approval.Request(id, op, human, binding, now.Add(-time.Hour))
	if err != nil {
		return approval.Approval{}, err
	}
	terms := approval.Terms{ExpiresAt: expires, MaxUses: maxUses, MaxAttempts: maxAttempts}
	if reauth != nil {
		terms.ReauthGrantID = *reauth
	}
	if a, err = a.Approve(human, terms, now.Add(-time.Minute)); err != nil {
		return approval.Approval{}, err
	}
	rows, err := i.r.c.Owner(i.r.t).Query(ctx, `SELECT job_id::text, claim_epoch, (SELECT count(*) FROM morningstar.job_attempts t WHERE t.job_id = u.job_id) FROM morningstar.approval_uses u WHERE approval_id = $1 ORDER BY consumed_at, id`, id)
	if err != nil {
		return approval.Approval{}, err
	}
	type use struct {
		job             string
		epoch, attempts int
	}
	var uses []use
	for rows.Next() {
		var u use
		if err := rows.Scan(&u.job, &u.epoch, &u.attempts); err != nil {
			rows.Close()
			return approval.Approval{}, err
		}
		uses = append(uses, u)
	}
	rows.Close()
	for _, u := range uses {
		if a, _, err = a.Claim(approval.JobID(u.job), u.epoch, op, binding, now); err != nil {
			return approval.Approval{}, err
		}
		for range u.attempts {
			if a, err = a.AuthorizeLaunch(approval.JobID(u.job), op, binding, 0, true, now); err != nil {
				return approval.Approval{}, err
			}
		}
	}
	if status == "revoked" {
		return a.Revoke(human, "test", now)
	}
	return a, nil
}

// fakeRunner stands for the sandbox launcher of M8.
type fakeRunner struct {
	mu           sync.Mutex
	preflightErr error
	startErr     error
	terminateOK  bool
	stopErr      error
	stopDelay    time.Duration
	started      []*fakeProc
	terminated   []string
	// terminatedBy holds the attempt ids that Terminate was asked about.
	terminatedBy []string
	// terminateHang makes Terminate block until its context ends.
	terminateHang bool
	n             int
}

func newRunner() *fakeRunner { return &fakeRunner{terminateOK: true} }

func (f *fakeRunner) Preflight(context.Context, outbound.LaunchSpec) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.preflightErr
}

func (f *fakeRunner) Start(_ context.Context, s outbound.LaunchSpec) (outbound.Process, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.startErr != nil {
		return nil, f.startErr
	}
	f.n++
	p := &fakeProc{ref: "pgrp-" + strconv.Itoa(f.n), spec: s, done: make(chan outbound.ProcessExit, 1), runner: f}
	f.started = append(f.started, p)
	return p, nil
}

func (f *fakeRunner) Terminate(ctx context.Context, attemptID, ref string) (bool, error) {
	f.mu.Lock()
	hang := f.terminateHang
	f.terminated = append(f.terminated, ref)
	f.terminatedBy = append(f.terminatedBy, attemptID)
	ok := f.terminateOK
	f.mu.Unlock()
	if hang {
		<-ctx.Done()
		return false, ctx.Err()
	}
	return ok, nil
}

func (f *fakeRunner) set(mod func(*fakeRunner)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	mod(f)
}

func (f *fakeRunner) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.started)
}

func (f *fakeRunner) last() *fakeProc {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.started) == 0 {
		return nil
	}
	return f.started[len(f.started)-1]
}

type fakeProc struct {
	ref     string
	spec    outbound.LaunchSpec
	done    chan outbound.ProcessExit
	runner  *fakeRunner
	stopped atomic.Bool
}

func (p *fakeProc) Ref() string                       { return p.ref }
func (p *fakeProc) Done() <-chan outbound.ProcessExit { return p.done }

func (p *fakeProc) Stop(ctx context.Context) error {
	p.runner.mu.Lock()
	delay, err := p.runner.stopDelay, p.runner.stopErr
	p.runner.mu.Unlock()
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err != nil {
		return err
	}
	p.stopped.Store(true)
	return nil
}

func (p *fakeProc) exit(code int) { p.done <- outbound.ProcessExit{Code: code} }

// until polls cond for up to d.
func until(t *testing.T, d time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}
