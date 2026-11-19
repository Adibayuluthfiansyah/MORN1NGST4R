package worker_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/persistence"
	appjob "github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/application/job"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/application/recovery"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/audit"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/job"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/scope"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/infrastructure/ids"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/tests/integration/pgtest"
)

// running starts a job and supervises it in the background. The returned
// channel delivers the result of Supervise.
func running(t *testing.T, r *rig, w *appjob.Worker, spec jobSpec) (fixture, chan error) {
	t.Helper()
	f := r.seedJob(spec)
	a := mustLaunch(t, w, mustClaim(t, w))
	done := make(chan error, 1)
	go func() { done <- w.Supervise(context.Background(), a) }()
	return f, done
}

func TestAStopRequestKillsTheTreeAndCancelsTheJob(t *testing.T) {
	r := newRig(t)
	w := r.worker("worker-a")
	f, done := running(t, r, w, jobSpec{Risk: job.LowImpact})

	if err := r.stop(f.JobID, "approval_revoked"); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("Supervise = %v", err)
	}
	if !r.runner.last().stopped.Load() {
		t.Error("the process was not stopped")
	}
	if got := r.jobState(f.JobID); got != "cancelled/stop_confirmed/1/1" {
		t.Errorf("job = %s", got)
	}
	if got := r.scalar(`SELECT state || '/' || (termination_confirmed_at IS NOT NULL) FROM morningstar.job_attempts WHERE job_id = $1`, f.JobID); got != "cancelled/true" {
		t.Errorf("attempt = %s", got)
	}
	if r.events("job.cancel_requested", audit.Success) != "1" || r.events("job.stop_confirmed", audit.Success) != "1" {
		t.Errorf("events: requested %s, confirmed %s", r.events("job.cancel_requested", audit.Success), r.events("job.stop_confirmed", audit.Success))
	}
	// A cancelled job is not requeued, and asking again says that it is finished.
	if err := r.stop(f.JobID, "again"); !errors.Is(err, appjob.ErrAlreadyFinished) {
		t.Errorf("a second stop: %v", err)
	}
}

func TestAStopThatCannotBeConfirmedRaisesAnAlertAndKeepsTheJobRunning(t *testing.T) {
	r := newRig(t)
	r.runner.set(func(f *fakeRunner) { f.stopErr = errors.New("the tree survived") })
	w := r.worker("worker-a")
	f, done := running(t, r, w, jobSpec{Risk: job.LowImpact})
	if err := r.stop(f.JobID, "scope_changed"); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err == nil {
		t.Fatal("Supervise reported a clean end")
	}
	if got := r.jobState(f.JobID); got != "running/stop_failed/1/1" {
		t.Errorf("job = %s", got)
	}
	if got := r.scalar(`SELECT count(*) FROM morningstar.system_alerts WHERE kind = 'sandbox_unavailable' AND cleared_at IS NULL`); got != "1" {
		t.Errorf("alerts = %s", got)
	}
	if r.events("job.stop_failed", audit.Failure) != "1" {
		t.Error("no job.stop_failed event")
	}
	// The job does not come back until a person has settled it: the stop was asked for.
	r.clock.Advance(time.Minute)
	if err := r.worker("worker-b").Reap(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := r.jobState(f.JobID); got != "cancelled/stop_confirmed/1/1" {
		t.Errorf("after the tree was confirmed gone by the reaper: %s", got)
	}
}

func TestAJobThatWaitsIsCancelledAtOnce(t *testing.T) {
	r := newRig(t)
	f := r.seedJob(jobSpec{Risk: job.LowImpact})
	if err := r.stop(f.JobID, "user_cancel"); err != nil {
		t.Fatal(err)
	}
	if got := r.jobState(f.JobID); got != "cancelled/stop_confirmed/0/0" {
		t.Errorf("job = %s", got)
	}
	if _, err := r.worker("worker-a").ClaimNext(t.Context()); !errors.Is(err, appjob.ErrNothingToClaim) {
		t.Errorf("a cancelled job was claimed: %v", err)
	}
	if got := r.uses(f.ApprovalID); got != "0/0" {
		t.Errorf("uses = %s", got)
	}
}

// A revocation sets the stop request on the dependent jobs in its own
// transaction (M19). A job that waits with that request is cancelled by the reaper.
func TestAWaitingJobWithAStopRequestIsCancelledByTheReaper(t *testing.T) {
	r := newRig(t)
	f := r.seedJob(jobSpec{Risk: job.LowImpact})
	r.c.Owner(t).Exec(t.Context(), `UPDATE morningstar.jobs SET control_state = 'stop_requested', control_reason = 'approval_revoked', control_requested_at = now() WHERE id = $1`, f.JobID)
	if _, err := r.worker("worker-a").ClaimNext(t.Context()); !errors.Is(err, appjob.ErrNothingToClaim) {
		t.Fatalf("a job with a stop request was claimed: %v", err)
	}
	if err := r.worker("worker-a").Reap(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := r.jobState(f.JobID); got != "cancelled/stop_confirmed/0/0" {
		t.Errorf("job = %s", got)
	}
}

// TD-M5-01: a cancellation takes effect when its record cannot be written.
func TestACancellationTakesEffectWhenTheRecordCannotBeWritten(t *testing.T) {
	r := newRig(t)
	r.sink.failRecord.Store(true)
	r.sink.failRequire.Store(true)
	f := r.seedJob(jobSpec{Risk: job.LowImpact})
	if err := r.stop(f.JobID, "user_cancel"); err != nil {
		t.Fatalf("a cancellation was refused because audit is down: %v", err)
	}
	if got := r.jobState(f.JobID); got != "cancelled/stop_confirmed/0/0" {
		t.Errorf("job = %s", got)
	}
	// A running job is stopped too, and its stop is confirmed without a record.
	r2 := newRig(t)
	w := r2.worker("worker-a")
	f2, done := running(t, r2, w, jobSpec{Risk: job.LowImpact})
	r2.sink.failRecord.Store(true)
	r2.sink.failRequire.Store(true)
	if err := r2.stop(f2.JobID, "approval_revoked"); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("Supervise = %v", err)
	}
	if got := r2.jobState(f2.JobID); got != "cancelled/stop_confirmed/1/1" {
		t.Errorf("running job = %s", got)
	}
}

// TD-M5-01: a claim does not start when its required record cannot be written.
func TestAClaimDoesNotStartWhenItsRecordCannotBeWritten(t *testing.T) {
	r := newRig(t)
	f := r.seedJob(jobSpec{Risk: job.LowImpact})
	r.sink.failRequire.Store(true)
	if c, err := r.worker("worker-a").ClaimNext(t.Context()); err == nil || c != nil {
		t.Fatalf("ClaimNext = %v, %v", c, err)
	}
	if got := r.jobState(f.JobID); got != "queued/none/0/0" {
		t.Errorf("job = %s", got)
	}
	if got := r.uses(f.ApprovalID); got != "0/0" {
		t.Errorf("uses = %s: a use was consumed for a claim that did not start", got)
	}
	if got := r.scalar(`SELECT count(*) FROM morningstar.policy_decisions`); got != "0" {
		t.Errorf("a decision of a rolled-back claim stayed: %s", got)
	}
	r.sink.failRequire.Store(false)
	mustClaim(t, r.worker("worker-a"))
}

// TD-M5-01: a degraded gate refuses the claim and the launch.
func TestADegradedAuditGateRefusesTheClaimAndTheLaunch(t *testing.T) {
	r := newRig(t)
	f := r.seedJob(jobSpec{Risk: job.LowImpact})
	w := r.worker("worker-a")
	r.sink.degraded.Store(true)
	if _, err := w.ClaimNext(t.Context()); !errors.Is(err, outbound.ErrAuditDegraded) {
		t.Fatalf("ClaimNext = %v", err)
	}
	if got := r.jobState(f.JobID); got != "queued/none/0/0" {
		t.Errorf("job = %s", got)
	}
	r.sink.degraded.Store(false)
	c := mustClaim(t, w)
	r.sink.degraded.Store(true)
	if a, err := w.Launch(t.Context(), c); err == nil || a != nil {
		t.Fatalf("Launch = %v, %v", a, err)
	}
	if got := r.jobState(f.JobID); got != "queued/none/1/0" {
		t.Errorf("a refused launch left the job at %s: it must be given back with no attempt", got)
	}
	if r.runner.count() != 0 {
		t.Error("a process started under a degraded audit")
	}
}

func TestALaunchDoesNotStartWhenItsRecordCannotBeWritten(t *testing.T) {
	r := newRig(t)
	f := r.seedJob(jobSpec{Risk: job.LowImpact, MaxAttempts: 3})
	w := r.worker("worker-a")
	c := mustClaim(t, w)
	r.sink.failRequire.Store(true)
	if a, err := w.Launch(t.Context(), c); err == nil || a != nil {
		t.Fatalf("Launch = %v, %v", a, err)
	}
	if got := r.scalar(`SELECT count(*) FROM morningstar.job_attempts WHERE job_id = $1`, f.JobID); got != "0" {
		t.Errorf("an attempt was counted for a launch that did not start: %s", got)
	}
	if r.runner.count() != 0 {
		t.Error("a process started")
	}
}

// A revocation that races a claim: the claim wins and holds one use, or it is
// refused and holds none. The counter is true either way (G-consume, G-revoke).
func TestARevocationRacingAClaimLeavesTheCountersTrue(t *testing.T) {
	for round := range 12 {
		r := newRig(t)
		f := r.seedJob(jobSpec{Risk: job.LowImpact})
		w := r.worker("worker-a")
		var wg sync.WaitGroup
		var claim *appjob.Claim
		var cerr error
		wg.Add(2)
		go func() { defer wg.Done(); claim, cerr = w.ClaimNext(t.Context()) }()
		go func() {
			defer wg.Done()
			_, _ = r.c.Owner(t).Exec(t.Context(), `UPDATE morningstar.approvals SET status = 'revoked', revoked_at = now(), revoked_by = $2, revoke_reason = 'test' WHERE id = $1`, f.ApprovalID, r.b.UserID)
		}()
		wg.Wait()
		uses := r.uses(f.ApprovalID)
		state := r.jobState(f.JobID)
		switch {
		case claim != nil:
			if uses != "1/1" || state != "running/none/1/0" {
				t.Fatalf("round %d: claimed with uses %s and job %s", round, uses, state)
			}
		case cerr == nil:
			if uses != "0/0" || state != "failed/none/0/0" {
				t.Fatalf("round %d: refused with uses %s and job %s", round, uses, state)
			}
		default:
			t.Fatalf("round %d: %v", round, cerr)
		}
	}
}

// A revoked approval is refused at the claim, and the denial is recorded.
func TestARevokedApprovalIsDeniedAtTheClaim(t *testing.T) {
	r := newRig(t)
	f := r.seedJob(jobSpec{Risk: job.LowImpact})
	if _, err := r.c.Owner(t).Exec(t.Context(), `UPDATE morningstar.approvals SET status = 'revoked', revoked_at = now(), revoked_by = $2, revoke_reason = 'test' WHERE id = $1`, f.ApprovalID, r.b.UserID); err != nil {
		t.Fatal(err)
	}
	c, err := r.worker("worker-a").ClaimNext(t.Context())
	if err != nil || c != nil {
		t.Fatalf("ClaimNext = %v, %v", c, err)
	}
	if got := r.jobState(f.JobID); got != "failed/none/0/0" {
		t.Errorf("job = %s", got)
	}
	if got := r.scalar(`SELECT failure_class FROM morningstar.jobs WHERE id = $1`, f.JobID); got != job.FailurePolicyDenial {
		t.Errorf("failure class = %s", got)
	}
	if got := r.scalar(`SELECT count(*) FROM morningstar.policy_decisions WHERE job_id = $1 AND point = 'E2' AND outcome = 'deny'`, f.JobID); got != "1" {
		t.Errorf("denials = %s", got)
	}
	if r.events("policy.decision_denied", audit.Denied) != "1" || r.events("job.failed", audit.Failure) != "1" {
		t.Errorf("events: denied %s, failed %s", r.events("policy.decision_denied", audit.Denied), r.events("job.failed", audit.Failure))
	}
	if got := r.uses(f.ApprovalID); got != "0/0" {
		t.Errorf("uses = %s", got)
	}
}

// A changed scope version or policy snapshot between the claim and the launch
// denies at E3 (S1.6 item 1), and a denied launch is not an attempt (D-55).
func TestAChangeBetweenTheClaimAndTheLaunchDenies(t *testing.T) {
	for name, change := range map[string]func(*rig){
		"another scope version": func(r *rig) {
			if _, err := r.c.Owner(r.t).Exec(r.t.Context(), `UPDATE morningstar.scope_versions SET superseded_at = now() WHERE id = $1`, r.b.ScopeID); err != nil {
				r.t.Fatal(err)
			}
			var id string
			if err := r.c.Owner(r.t).QueryRow(r.t.Context(), `INSERT INTO morningstar.scope_versions (id, program_id, version_no, rules_version_id, content_hash, widening, created_by, reason, supersedes_version_id)
				SELECT uuidv7(), program_id, 2, rules_version_id, $2, false, created_by, 'edit', id FROM morningstar.scope_versions WHERE id = $1 RETURNING id`, r.b.ScopeID, pgtest.Random(32)).Scan(&id); err != nil {
				r.t.Fatal(err)
			}
			r.inputs.setPolicy(func(p *scope.PolicyState) { p.ScopeVersionID = id })
		},
		"another policy snapshot": func(r *rig) { r.inputs.setPolicy(func(p *scope.PolicyState) { p.PolicySnapshot[0] ^= 0xff }) },
		"the validation is no longer current": func(r *rig) {
			r.inputs.mu.Lock()
			r.inputs.valid.Current = false
			r.inputs.mu.Unlock()
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			f := r.seedJob(jobSpec{Risk: job.LowImpact, MaxAttempts: 3})
			w := r.worker("worker-a")
			c := mustClaim(t, w)
			change(r)
			a, err := w.Launch(t.Context(), c)
			if err != nil || a != nil {
				t.Fatalf("Launch = %v, %v", a, err)
			}
			if got := r.jobState(f.JobID); got != "failed/none/1/0" {
				t.Errorf("job = %s", got)
			}
			if got := r.scalar(`SELECT count(*) FROM morningstar.job_attempts`); got != "0" {
				t.Errorf("attempts = %s", got)
			}
			if r.runner.count() != 0 {
				t.Error("a process started")
			}
			if r.events("policy.decision_denied", audit.Denied) != "1" {
				t.Error("the denial was not recorded")
			}
			if got := r.scalar(`SELECT count(*) FROM morningstar.policy_decisions WHERE job_id = $1 AND point = 'E3' AND outcome = 'deny'`, f.JobID); got != "1" {
				t.Errorf("E3 denials = %s", got)
			}
		})
	}
}

func TestARefusedSandboxFailsTheJobWithoutAnAttempt(t *testing.T) {
	r := newRig(t)
	f := r.seedJob(jobSpec{Risk: job.LowImpact, MaxAttempts: 3})
	r.runner.set(func(f *fakeRunner) { f.preflightErr = errors.New("no sandbox") })
	w := r.worker("worker-a")
	a, err := w.Launch(t.Context(), mustClaim(t, w))
	if err != nil || a != nil {
		t.Fatalf("Launch = %v, %v", a, err)
	}
	if got := r.jobState(f.JobID); got != "failed/none/1/0" {
		t.Errorf("job = %s", got)
	}
	if got := r.scalar(`SELECT failure_class FROM morningstar.jobs WHERE id = $1`, f.JobID); got != job.FailureSandboxRefused {
		t.Errorf("failure class = %s", got)
	}
}

// D-55: a launch whose process cannot start still counts, and the retry rules decide.
func TestAFailedSpawnCountsAsAnAttempt(t *testing.T) {
	r := newRig(t)
	f := r.seedJob(jobSpec{Risk: job.Passive, Retry: job.RetryBounded, MaxAttempts: 2})
	r.runner.set(func(f *fakeRunner) { f.startErr = errors.New("spawn failed") })
	w := r.worker("worker-a")
	if a, err := w.Launch(t.Context(), mustClaim(t, w)); err != nil || a != nil {
		t.Fatalf("Launch = %v, %v", a, err)
	}
	if got := r.jobState(f.JobID); got != "queued/none/1/1" {
		t.Fatalf("after the first failed spawn: %s", got)
	}
	if got := r.scalar(`SELECT state || '/' || error_class || '/' || (termination_confirmed_at IS NOT NULL) FROM morningstar.job_attempts WHERE job_id = $1`, f.JobID); got != "failed/sandbox_refused/true" {
		t.Errorf("attempt = %s", got)
	}
	if a, err := w.Launch(t.Context(), mustClaim(t, w)); err != nil || a != nil {
		t.Fatalf("Launch = %v, %v", a, err)
	}
	if got := r.jobState(f.JobID); got != "failed/none/2/2" {
		t.Errorf("after the second: %s", got)
	}
	if got := r.scalar(`SELECT failure_class FROM morningstar.jobs WHERE id = $1`, f.JobID); got != job.FailureAttemptsExhausted {
		t.Errorf("failure class = %s", got)
	}
}

// D-80: at most three attempts for a passive job, a retry never reuses a use or
// consumes another, and the rules of the stop beat them.
func TestThePassiveRetryPolicyOfD80(t *testing.T) {
	r := newRig(t)
	f := r.seedJob(jobSpec{Risk: job.LowImpact, Retry: job.RetryBounded, MaxAttempts: 3})
	w := r.worker("worker-a")
	for n := 1; n <= 3; n++ {
		c := mustClaim(t, w)
		a := mustLaunch(t, w, c)
		if a.Attempt.No != n || c.Job.ClaimEpoch != n {
			t.Fatalf("round %d: attempt %d, epoch %d", n, a.Attempt.No, c.Job.ClaimEpoch)
		}
		done := make(chan error, 1)
		go func() { done <- w.Supervise(t.Context(), a) }()
		r.runner.last().exit(1)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		want := "queued/none/" + string(rune('0'+n)) + "/" + string(rune('0'+n))
		if n == 3 {
			want = "failed/none/3/3"
		}
		if got := r.jobState(f.JobID); got != want {
			t.Fatalf("after attempt %d: %s, want %s", n, got, want)
		}
		if got := r.uses(f.ApprovalID); got != "1/1" {
			t.Fatalf("after attempt %d: uses %s", n, got)
		}
	}
	if got := r.scalar(`SELECT failure_class FROM morningstar.jobs WHERE id = $1`, f.JobID); got != job.FailureAttemptsExhausted {
		t.Errorf("failure class = %s", got)
	}
}

func TestAJobWithoutARetryPolicyFailsAfterOneAttempt(t *testing.T) {
	r := newRig(t)
	f := r.seedJob(jobSpec{Risk: job.Passive, MaxAttempts: 3})
	w := r.worker("worker-a")
	a := mustLaunch(t, w, mustClaim(t, w))
	done := make(chan error, 1)
	go func() { done <- w.Supervise(t.Context(), a) }()
	r.runner.last().exit(2)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := r.jobState(f.JobID); got != "failed/none/1/1" {
		t.Errorf("job = %s", got)
	}
	if got := r.scalar(`SELECT failure_class FROM morningstar.jobs WHERE id = $1`, f.JobID); got != job.FailureRetryDisabled {
		t.Errorf("failure class = %s", got)
	}
}

func TestATimeoutIsFinal(t *testing.T) {
	r := newRig(t)
	f := r.seedJob(jobSpec{Risk: job.Passive, Retry: job.RetryBounded, MaxAttempts: 3})
	w := r.worker("worker-a")
	a := mustLaunch(t, w, mustClaim(t, w))
	r.runner.last().done <- outbound.ProcessExit{TimedOut: true}
	if err := w.Supervise(t.Context(), a); err != nil {
		t.Fatal(err)
	}
	if got := r.jobState(f.JobID); got != "failed/none/1/1" {
		t.Errorf("job = %s", got)
	}
	if got := r.scalar(`SELECT state FROM morningstar.job_attempts WHERE job_id = $1`, f.JobID); got != "timed_out" {
		t.Errorf("attempt = %s", got)
	}
}

// G-recovery 7: a worker that kept its claim loop through a reconciliation
// cannot claim afterwards (CC-15). Its service still believes in generation 1.
func TestGRecoveryAWorkerCannotClaimAfterAReconciliation(t *testing.T) {
	r := newRig(t)
	old := r.worker("worker-a") // its recovery service is at generation 1

	// The server side detects a missing marker and reconciles to generation 2.
	server := recovery.NewService(recovery.Deps{Markers: r.markers, Repo: persistence.NewRecovery(r.api), NewID: ids.New, Now: time.Now})
	if err := os.Remove(filepath.Join(r.markerDir, "recovery-generation.json")); err != nil {
		t.Fatal(err)
	}
	if st, err := server.Check(t.Context()); err != nil || !st.Required {
		t.Fatalf("Check = %+v, %v", st, err)
	}
	if _, err := server.Reconcile(t.Context(), inbound.ReconcileRequest{ActorUserID: r.b.UserID, RequestID: ids.New()}); err != nil {
		t.Fatal(err)
	}

	f := r.seedJob(jobSpec{Risk: job.LowImpact})
	c, err := old.ClaimNext(t.Context())
	if c != nil || !errors.Is(err, outbound.ErrRecoveryGeneration) {
		t.Fatalf("a stale worker: ClaimNext = %v, %v", c, err)
	}
	if got := r.jobState(f.JobID); got != "queued/none/0/0" {
		t.Errorf("job = %s", got)
	}
	if got := r.uses(f.ApprovalID); got != "0/0" {
		t.Errorf("uses = %s", got)
	}

	// A worker that starts after the reconciliation reads generation 2 and claims.
	fresh := recovery.NewService(recovery.Deps{Markers: r.markers, Repo: persistence.NewRecovery(r.api), NewID: ids.New, Now: time.Now})
	if st, err := fresh.Check(t.Context()); err != nil || st.Required {
		t.Fatalf("Check = %+v, %v", st, err)
	}
	r.rec = fresh
	mustClaim(t, r.worker("worker-b"))
}

// A worker does not claim while the installation requires recovery.
func TestGRecoveryNoClaimWhileRecoveryIsRequired(t *testing.T) {
	r := newRig(t)
	r.seedJob(jobSpec{Risk: job.LowImpact})
	if err := os.Remove(filepath.Join(r.markerDir, "recovery-generation.json")); err != nil {
		t.Fatal(err)
	}
	other := recovery.NewService(recovery.Deps{Markers: r.markers, Repo: persistence.NewRecovery(r.api), NewID: ids.New, Now: time.Now})
	if st, err := other.Check(t.Context()); err != nil || !st.Required {
		t.Fatalf("Check = %+v, %v", st, err)
	}
	r.rec = other
	if _, err := r.worker("worker-a").ClaimNext(t.Context()); !errors.Is(err, recovery.ErrNotNormal) {
		t.Fatalf("ClaimNext = %v", err)
	}
	if got := r.scalar(`SELECT count(*) FROM morningstar.approval_uses`); got != "0" {
		t.Errorf("uses = %s", got)
	}
}

// G-recovery 11: the marker moves before a launch, and a failed write stops claims.
func TestGRecoveryTheMarkerMovesBeforeALaunchAndAFailedWriteStopsClaims(t *testing.T) {
	r := newRig(t)
	f := r.seedJob(jobSpec{Risk: job.LowImpact})
	w := r.worker("worker-a")
	before, err := r.markers.Read(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	mustClaim(t, w)
	after, err := r.markers.Read(t.Context())
	if err != nil || after.AuditWatermark <= before.AuditWatermark {
		t.Fatalf("the watermark did not move at the claim: %d -> %d (%v)", before.AuditWatermark, after.AuditWatermark, err)
	}

	// A failed write: the claim is given back, no launch, and no claim follows.
	g := r.seedJob(jobSpec{Risk: job.LowImpact})
	r.markers.fail.Store(true)
	c, err := w.ClaimNext(t.Context())
	if err == nil || c != nil {
		t.Fatalf("ClaimNext = %v, %v", c, err)
	}
	if got := r.jobState(g.JobID); got != "queued/none/1/0" {
		t.Errorf("the job whose marker write failed: %s", got)
	}
	if _, err := w.ClaimNext(t.Context()); err == nil {
		t.Error("a claim was made while the marker cannot be written")
	}
	if r.runner.count() != 0 {
		t.Error("a process started")
	}
	r.markers.fail.Store(false)
	if err := r.rec.AdvanceWatermark(t.Context()); err != nil {
		t.Fatal(err)
	}
	if c, err := w.ClaimNext(t.Context()); err != nil || c == nil || c.Job.ID != g.JobID {
		t.Errorf("after the marker is writable again: %v, %v", c, err)
	}
	_ = f
}
