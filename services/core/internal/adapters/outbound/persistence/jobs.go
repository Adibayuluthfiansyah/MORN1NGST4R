package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/persistence/sqlcgen"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/job"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/infrastructure/ids"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

// Jobs implements outbound.JobStore and outbound.PolicyDecisions. The database
// Jobs implements outbound.JobStore and outbound.PolicyDecisions. The database
// rules of the lifecycle (migration 00018)
// back every statement, so a defect here cannot break the state machine.
type Jobs struct{ base }

var (
	_ outbound.JobStore        = Jobs{}
	_ outbound.PolicyDecisions = Jobs{}
)

func NewJobs(pool *pgxpool.Pool) Jobs { return Jobs{base{pool}} }

func (d *DB) Jobs() Jobs { return NewJobs(d.pool) }

func tsParam(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: !t.IsZero()} }

func textPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func recordOf(j sqlcgen.MorningstarJob) outbound.JobRecord {
	r := outbound.JobRecord{
		ID: j.ID.String(), ProgramID: j.ProgramID.String(), AssessmentID: j.AssessmentID.String(), Kind: j.JobKind,
		OperationID: uuidString(j.OperationID), ApprovalID: uuidString(j.ApprovalID), ApprovalUseID: uuidString(j.ApprovalUseID),
		ScopeVersionID: uuidString(j.ScopeVersionID), ValidationID: uuidString(j.ValidationID),
		Risk: job.Risk(j.RiskClass), State: job.State(j.State), Control: job.Control(j.ControlState), RetryPolicy: job.RetryPolicy(j.RetryPolicy),
		MaxAttempts: int(j.MaxExecutionAttempts), Attempts: int(j.ExecutionAttempts), ClaimEpoch: int(j.ClaimEpoch),
		AvailableAt: pgTime(j.AvailableAt),
	}
	if j.ControlReason != nil {
		r.ControlReason = *j.ControlReason
	}
	if j.LeaseOwner != nil {
		r.LeaseOwner = *j.LeaseOwner
	}
	if j.LeaseExpiresAt.Valid {
		r.LeaseExpiresAt = pgTime(j.LeaseExpiresAt)
	}
	return r
}

func (s Jobs) LockNextQueued(ctx context.Context, now time.Time) (outbound.JobRecord, error) {
	row, err := s.q(ctx).LockNextQueuedJob(ctx, tsParam(now))
	if noRows(err) {
		return outbound.JobRecord{}, outbound.ErrNoJob
	}
	if err != nil {
		return outbound.JobRecord{}, wrap("lock the next queued job", err)
	}
	return recordOf(row.MorningstarJob), nil
}

func (s Jobs) Lock(ctx context.Context, id string) (outbound.JobRecord, error) {
	u, err := uuidOf(id)
	if err != nil {
		return outbound.JobRecord{}, outbound.ErrJobNotFound
	}
	row, err := s.q(ctx).LockJob(ctx, u)
	if noRows(err) {
		return outbound.JobRecord{}, outbound.ErrJobNotFound
	}
	if err != nil {
		return outbound.JobRecord{}, wrap("lock a job", err)
	}
	return recordOf(row.MorningstarJob), nil
}

func (s Jobs) Claim(ctx context.Context, c outbound.ClaimJob) error {
	id, err := uuidOf(c.JobID)
	if err != nil {
		return err
	}
	use, err := uuidOrNull(c.UseID)
	if err != nil {
		return err
	}
	if c.Epoch < 1 || c.Worker == "" || c.LeaseUntil.IsZero() {
		return errors.New("persistence: a claim needs an epoch, a worker and a lease")
	}
	n, err := s.q(ctx).ClaimJobRow(ctx, sqlcgen.ClaimJobRowParams{
		Epoch: int32(c.Epoch), Worker: c.Worker, LeaseUntil: tsParam(c.LeaseUntil), UseID: use, JobID: id, Generation: int64(c.Generation),
	})
	if err != nil {
		return wrap("claim a job", err)
	}
	if n != 1 {
		if row, gerr := s.q(ctx).GetRecoveryState(ctx); gerr == nil && (uint64(row.Generation) != c.Generation || row.Reconciling) {
			return outbound.ErrRecoveryGeneration
		}
		return outbound.ErrStaleClaim
	}
	return s.history(ctx, c.JobID, "queued", "running", string(job.ControlNone), c.Epoch, outbound.TransitionActor{Kind: "worker"}, "")
}

func (s Jobs) history(ctx context.Context, jobID, from, to, control string, epoch int, who outbound.TransitionActor, reason string) error {
	id, err := uuidOf(ids.New())
	if err != nil {
		return err
	}
	jid, err := uuidOf(jobID)
	if err != nil {
		return err
	}
	user, err := uuidOrNull(who.UserID)
	if err != nil {
		return err
	}
	return wrap("write the history of a job", s.q(ctx).InsertJobTransition(ctx, sqlcgen.InsertJobTransitionParams{
		ID: id, JobID: jid, FromState: textPtr(from), ToState: to, ControlState: control, ClaimEpoch: int32(epoch),
		ActorKind: who.Kind, ActorUserID: user, Reason: textPtr(reason),
	}))
}

func (s Jobs) Move(ctx context.Context, m outbound.MoveJob) error {
	id, err := uuidOf(m.JobID)
	if err != nil {
		return err
	}
	if !job.CanMove(m.From, m.To) {
		return fmt.Errorf("persistence: a job does not move from %s to %s", m.From, m.To)
	}
	row, err := s.q(ctx).MoveJobRow(ctx, sqlcgen.MoveJobRowParams{
		ToState: string(m.To), Control: textPtr(string(m.Control)), Reason: textPtr(m.Reason), FailureClass: textPtr(m.FailureClass),
		AvailableAt: tsParam(m.AvailableAt), JobID: id, FromState: string(m.From), Epoch: int32(m.Epoch), Worker: m.Worker,
	})
	if noRows(err) {
		return outbound.ErrStaleClaim
	}
	if err != nil {
		return wrap("move a job", err)
	}
	return s.history(ctx, m.JobID, string(m.From), string(m.To), row.ControlState, int(row.ClaimEpoch), m.Actor, m.Reason)
}

func (s Jobs) Cancel(ctx context.Context, c outbound.CancelJob) error {
	id, err := uuidOf(c.JobID)
	if err != nil {
		return err
	}
	if !job.CanMove(c.From, job.Cancelled) || c.From == job.Running {
		return fmt.Errorf("persistence: a job that is %s is not cancelled here", c.From)
	}
	row, err := s.q(ctx).CancelJobRow(ctx, sqlcgen.CancelJobRowParams{Control: string(c.Control), Reason: textPtr(c.Reason), JobID: id, FromState: string(c.From)})
	if noRows(err) {
		return outbound.ErrStaleClaim
	}
	if err != nil {
		return wrap("cancel a job", err)
	}
	return s.history(ctx, c.JobID, string(c.From), string(job.Cancelled), row.ControlState, int(row.ClaimEpoch), c.Actor, c.Reason)
}

func (s Jobs) Heartbeat(ctx context.Context, f outbound.Fence, until time.Time) (job.Control, error) {
	id, err := uuidOf(f.JobID)
	if err != nil {
		return "", err
	}
	c, err := s.q(ctx).HeartbeatJobRow(ctx, sqlcgen.HeartbeatJobRowParams{Until: tsParam(until), JobID: id, Epoch: int32(f.Epoch), Worker: f.Worker})
	if noRows(err) {
		return "", outbound.ErrStaleClaim
	}
	return job.Control(c), wrap("renew a lease", err)
}

func (s Jobs) Control(ctx context.Context, f outbound.Fence) (job.Control, error) {
	id, err := uuidOf(f.JobID)
	if err != nil {
		return "", err
	}
	c, err := s.q(ctx).JobControlRow(ctx, sqlcgen.JobControlRowParams{JobID: id, Epoch: int32(f.Epoch), Worker: f.Worker})
	if noRows(err) {
		return "", outbound.ErrStaleClaim
	}
	return job.Control(c), wrap("read the control state of a job", err)
}

func (s Jobs) SetControl(ctx context.Context, f outbound.Fence, c job.Control) error {
	id, err := uuidOf(f.JobID)
	if err != nil {
		return err
	}
	n, err := s.q(ctx).SetJobControl(ctx, sqlcgen.SetJobControlParams{Control: string(c), JobID: id, Epoch: int32(f.Epoch), Worker: f.Worker})
	if err != nil {
		return wrap("set the control state of a job", err)
	}
	if n != 1 {
		return outbound.ErrStaleClaim
	}
	return nil
}

func (s Jobs) RequestStop(ctx context.Context, jobID, reason string) (bool, error) {
	id, err := uuidOf(jobID)
	if err != nil {
		return false, outbound.ErrJobNotFound
	}
	n, err := s.q(ctx).RequestJobStop(ctx, sqlcgen.RequestJobStopParams{Reason: reason, JobID: id})
	return n == 1, wrap("request the stop of a job", err)
}

func (s Jobs) ExpiredLeases(ctx context.Context, now time.Time, limit int) ([]outbound.JobRecord, error) {
	rows, err := s.q(ctx).ListExpiredLeases(ctx, sqlcgen.ListExpiredLeasesParams{Now: tsParam(now), MaxRows: int32(limit)})
	if err != nil {
		return nil, wrap("list expired leases", err)
	}
	out := make([]outbound.JobRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, recordOf(r.MorningstarJob))
	}
	return out, nil
}

func (s Jobs) StopRequestedWaiting(ctx context.Context, limit int) ([]outbound.JobRecord, error) {
	rows, err := s.q(ctx).ListStopRequestedWaiting(ctx, int32(limit))
	if err != nil {
		return nil, wrap("list jobs with a stop request", err)
	}
	out := make([]outbound.JobRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, recordOf(r.MorningstarJob))
	}
	return out, nil
}

func (s Jobs) RunningCount(ctx context.Context, approvalID string) (int, error) {
	id, err := uuidOf(approvalID)
	if err != nil {
		return 0, err
	}
	n, err := s.q(ctx).CountRunningUnderApproval(ctx, id)
	return int(n), wrap("count running jobs", err)
}

func (s Jobs) AddAttempt(ctx context.Context, a outbound.NewAttempt) (outbound.Attempt, error) {
	jid, err := uuidOf(a.Fence.JobID)
	if err != nil {
		return outbound.Attempt{}, err
	}
	aid, err := uuidOf(a.ID)
	if err != nil {
		return outbound.Attempt{}, err
	}
	decision, err := uuidOf(a.DecisionID)
	if err != nil {
		return outbound.Attempt{}, err
	}
	use, err := uuidOrNull(a.UseID)
	if err != nil {
		return outbound.Attempt{}, err
	}
	q := s.q(ctx)
	n, err := q.RaiseAttemptCounter(ctx, sqlcgen.RaiseAttemptCounterParams{JobID: jid, Epoch: int32(a.Fence.Epoch), Worker: a.Fence.Worker})
	if noRows(err) {
		// Nothing was raised: the claim is gone, or the job has no attempt left.
		cur, lerr := q.LockJob(ctx, jid)
		if lerr == nil {
			j := cur.MorningstarJob
			if j.State == string(job.Running) && int(j.ClaimEpoch) == a.Fence.Epoch && j.LeaseOwner != nil && *j.LeaseOwner == a.Fence.Worker &&
				j.ControlState == string(job.ControlNone) && j.ExecutionAttempts >= j.MaxExecutionAttempts {
				return outbound.Attempt{}, outbound.ErrAttemptsExhausted
			}
		}
		return outbound.Attempt{}, outbound.ErrStaleClaim
	}
	if err != nil {
		return outbound.Attempt{}, wrap("raise the attempt counter", err)
	}
	if err := q.InsertJobAttempt(ctx, sqlcgen.InsertJobAttemptParams{
		ID: aid, JobID: jid, AttemptNo: int16(n), ClaimEpoch: int32(a.Fence.Epoch), UseID: use, DecisionID: decision, Worker: a.Fence.Worker,
	}); err != nil {
		return outbound.Attempt{}, wrap("insert an attempt", err)
	}
	return outbound.Attempt{ID: a.ID, JobID: a.Fence.JobID, No: int(n), ClaimEpoch: a.Fence.Epoch, State: job.AttemptLaunching, Worker: a.Fence.Worker}, nil
}

func (s Jobs) UpdateAttempt(ctx context.Context, u outbound.AttemptUpdate) error {
	id, err := uuidOf(u.ID)
	if err != nil {
		return err
	}
	q := s.q(ctx)
	var n int64
	if u.State == "" && u.ProcessRef == "" && u.ErrorClass == "" && u.ExitCode == nil && !u.End && u.ConfirmTermination {
		// The termination of a lost attempt is confirmed after the state changed.
		n, err = q.ConfirmAttemptTermination(ctx, id)
	} else {
		var code *int32
		if u.ExitCode != nil {
			c := int32(*u.ExitCode)
			code = &c
		}
		n, err = q.UpdateLiveAttempt(ctx, sqlcgen.UpdateLiveAttemptParams{
			State: textPtr(string(u.State)), ProcessRef: textPtr(u.ProcessRef), ErrorClass: textPtr(u.ErrorClass), ExitCode: code,
			Ended: u.End, Confirmed: u.ConfirmTermination, AttemptID: id,
		})
	}
	if err != nil {
		return wrap("update an attempt", err)
	}
	if n != 1 {
		return outbound.ErrStaleClaim
	}
	return nil
}

func (s Jobs) Attempts(ctx context.Context, jobID string) ([]outbound.Attempt, error) {
	id, err := uuidOf(jobID)
	if err != nil {
		return nil, outbound.ErrJobNotFound
	}
	rows, err := s.q(ctx).ListJobAttempts(ctx, id)
	if err != nil {
		return nil, wrap("list attempts", err)
	}
	out := make([]outbound.Attempt, 0, len(rows))
	for _, r := range rows {
		out = append(out, outbound.Attempt{
			ID: r.ID, JobID: r.JobID, No: int(r.AttemptNo), ClaimEpoch: int(r.ClaimEpoch), State: job.AttemptState(r.State),
			Worker: r.WorkerInstance, ProcessRef: r.ProcessRef, TerminationConfirmed: r.Confirmed, StartedAt: pgTime(r.StartedAt),
		})
	}
	return out, nil
}

func (s Jobs) Insert(ctx context.Context, d outbound.PolicyDecision) error {
	id, err := uuidOf(d.ID)
	if err != nil {
		return err
	}
	program, err := uuidOf(d.ProgramID)
	if err != nil {
		return err
	}
	assessment, err := uuidOf(d.AssessmentID)
	if err != nil {
		return err
	}
	op, err := uuidOrNull(d.OperationID)
	if err != nil {
		return err
	}
	jid, err := uuidOrNull(d.JobID)
	if err != nil {
		return err
	}
	scopeVersion, err := uuidOf(d.ScopeVersionID)
	if err != nil {
		return err
	}
	validation, err := uuidOrNull(d.ValidationID)
	if err != nil {
		return err
	}
	ap, err := uuidOrNull(d.ApprovalID)
	if err != nil {
		return err
	}
	reasons, err := json.Marshal(append([]string{}, d.Reasons...))
	if err != nil {
		return err
	}
	return wrap("record a policy decision", s.q(ctx).InsertPolicyDecision(ctx, sqlcgen.InsertPolicyDecisionParams{
		ID: id, ProgramID: program, AssessmentID: assessment, Point: d.Point.String(), OperationID: op, JobID: jid, Outcome: d.Outcome.String(),
		Reasons: reasons, ScopeVersionID: scopeVersion, ValidationID: validation, ApprovalID: ap, PolicySnapshotHash: d.PolicySnapshot[:],
	}))
}
