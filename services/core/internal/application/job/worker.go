// Package job is the job lifecycle use case (FR-JOB-001, FR-APPROVAL-002,
// FR-APPROVAL-003): the claim at E2, the launch at E3, the lease and its
// heartbeat, the stop of running work, the reaper of expired leases and the
// retry rules of D-80. The worker process runs it. It starts no process
// itself: the sandbox launcher of M8 provides the AttemptRunner, and until then
// the worker has one that refuses.
//
// Every write of a running job names its claim (job, epoch, lease owner), so a
// stale worker changes nothing (P-36). The claim and the launch call the audit
// gate before they start and write their required record inside their own
// transaction, so an audit outage stops them and a failed write rolls them back
// (D-48, TD-M5-01). A stop, a denial and a lease loss take effect whether or not
// their record can be written.
package job

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/approval"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/audit"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/job"
	domscope "github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/scope"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

var (
	// ErrNothingToClaim: no job is ready.
	ErrNothingToClaim = errors.New("job: nothing to claim")
	// ErrFenced: the claim of the worker is no longer current. The worker has
	// stopped its process and written nothing.
	ErrFenced = outbound.ErrStaleClaim
)

// Recovery is what the worker needs of the recovery service (CC-15, section 4 of
// recovery-generation.md). *recovery.Service implements it.
type Recovery interface {
	// ClaimsAllowed refuses while the installation is not normal or the last
	// marker write failed.
	ClaimsAllowed() error
	// AdvanceWatermark writes the marker after a commit that must not be replayed.
	AdvanceWatermark(ctx context.Context) error
	// Generation is the generation of the installation that the worker claims under.
	Generation() (uint64, bool)
}

type Deps struct {
	Store     outbound.JobStore
	Uses      outbound.ApprovalUses
	Tx        outbound.Transactor
	Evaluator inbound.ScopeEvaluator
	Runner    outbound.AttemptRunner
	Alerts    outbound.SystemAlerts
	Audit     outbound.AuditSink
	Recovery  Recovery

	Timing   job.Timing
	WorkerID string
	// Capacity is how many jobs the worker runs at the same time. Default 1.
	Capacity int
	// ClaimPoll is how long the loop waits when no job is ready. Default 1 second.
	ClaimPoll time.Duration

	NewID  func() string
	Now    func() time.Time
	Logger *slog.Logger
}

// Worker runs jobs.
type Worker struct {
	Deps
	actor audit.Actor
}

// New validates the dependencies and the timing. A timing that cannot keep the
// stop objective is refused (G-revoke).
func New(d Deps) (*Worker, error) {
	if err := d.Timing.Validate(); err != nil {
		return nil, err
	}
	if d.Store == nil || d.Tx == nil || d.Evaluator == nil || d.Runner == nil || d.Audit == nil || d.Recovery == nil || d.Alerts == nil || d.NewID == nil {
		return nil, errors.New("job: a dependency of the worker is missing")
	}
	who, err := audit.Worker(d.WorkerID)
	if err != nil {
		return nil, fmt.Errorf("job: the worker id is not valid: %w", err)
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Capacity < 1 {
		d.Capacity = 1
	}
	if d.ClaimPoll <= 0 {
		d.ClaimPoll = time.Second
	}
	if d.Logger == nil {
		d.Logger = slog.New(slog.DiscardHandler)
	}
	return &Worker{Deps: d, actor: who}, nil
}

func subject(j outbound.JobRecord, decision string) audit.Subject {
	return audit.Subject{
		ProgramID: j.ProgramID, AssessmentID: j.AssessmentID, JobID: j.ID, EntityType: "job", EntityID: j.ID,
		ApprovalID: j.ApprovalID, DecisionID: decision,
	}
}

func (w *Worker) event(action string, outcome audit.Outcome, reason string, j outbound.JobRecord, decision string, detail map[string]any) (audit.Event, error) {
	return audit.New(w.NewID(), audit.Input{Actor: w.actor, Action: action, Outcome: outcome, Reason: reason, Subject: subject(j, decision), Detail: detail})
}

// require writes the record that an operation needs. Inside the transaction of
// the operation it joins it, so a failed write rolls the operation back.
func (w *Worker) require(ctx context.Context, action string, j outbound.JobRecord, decision string, detail map[string]any) error {
	e, err := w.event(action, audit.Success, "", j, decision, detail)
	if err != nil {
		return err
	}
	return w.Audit.Require(ctx, e)
}

// record writes the record of something that has already taken effect. The
// effect stands whatever happens to the record.
func (w *Worker) record(ctx context.Context, action string, outcome audit.Outcome, reason string, j outbound.JobRecord, decision string, detail map[string]any) {
	e, err := w.event(action, outcome, reason, j, decision, detail)
	if err == nil {
		err = w.Audit.Record(ctx, e)
	}
	if err != nil {
		w.Logger.Error("job: an audit event was not recorded", "action", action, "job", j.ID)
	}
}

func (w *Worker) fence(j outbound.JobRecord) outbound.Fence {
	return outbound.Fence{JobID: j.ID, Epoch: j.ClaimEpoch, Worker: j.LeaseOwner}
}

var workerActor = outbound.TransitionActor{Kind: "worker"}

// Claim is a job that this worker holds.
type Claim struct {
	Job   outbound.JobRecord
	Fence outbound.Fence
	// UseID is the approval use of the job, empty for a job that needs none.
	UseID string
	Input outbound.ScopeInput
}

// ClaimNext claims the next ready job (E2). It returns ErrNothingToClaim when
// there is none, and a nil Claim with a nil error when it took a job and refused
// it (a denial fails the job, and the denial is audited).
func (w *Worker) ClaimNext(ctx context.Context) (*Claim, error) {
	if err := w.Audit.Gate(ctx); err != nil {
		return nil, err
	}
	if err := w.Recovery.ClaimsAllowed(); err != nil {
		return nil, err
	}
	gen, ok := w.Recovery.Generation()
	if !ok {
		return nil, errors.New("job: the installation has no generation to claim under")
	}

	var (
		claim  *Claim
		denied *denial
	)
	err := w.Tx.InTx(ctx, func(ctx context.Context) error {
		claim, denied = nil, nil
		j, err := w.Store.LockNextQueued(ctx, w.Now())
		if errors.Is(err, outbound.ErrNoJob) {
			return ErrNothingToClaim
		}
		if err != nil {
			return err
		}
		epoch := j.ClaimEpoch + 1
		req := inbound.ScopeRequest{Point: domscope.PointE2, Job: j}
		res, err := w.Evaluator.Evaluate(ctx, req)
		if err != nil {
			return err
		}
		if !res.Allowed() {
			denied = &denial{job: j, req: req, res: res}
			return w.failQueued(ctx, j, job.FailurePolicyDenial)
		}
		useID, consumed := "", false
		if j.Risk.NeedsApproval() {
			in := res.Input
			r, err := w.Uses.Claim(ctx, outbound.ApprovalClaim{
				UseID: w.NewID(), ApprovalID: j.ApprovalID, JobID: j.ID, ClaimEpoch: epoch, Generation: gen,
				Binding:   approval.Binding{ValidationID: in.Validation.ID, ScopeVersionID: in.Policy.ScopeVersionID, PolicySnapshot: in.Policy.PolicySnapshot},
				Operation: in.Operation.Hash(),
			})
			if reason, isDenial := approval.ReasonOf(err); isDenial {
				denied = &denial{job: j, req: req, res: res, approvalReason: string(reason)}
				return w.failQueued(ctx, j, job.FailurePolicyDenial)
			}
			if err != nil {
				return err
			}
			useID, consumed = r.UseID, r.Outcome == outbound.ClaimConsumed
		}
		held := j
		held.State, held.ClaimEpoch, held.LeaseOwner, held.ApprovalUseID = job.Running, epoch, w.WorkerID, useID
		held.LeaseExpiresAt = w.Now().Add(w.Timing.LeaseTTL)
		if err := w.Store.Claim(ctx, outbound.ClaimJob{Fence: w.fence(held), LeaseUntil: held.LeaseExpiresAt, UseID: useID, Generation: gen}); err != nil {
			return err
		}
		if err := w.require(ctx, "job.claimed", held, res.DecisionID, map[string]any{"epoch": epoch, "worker": w.WorkerID}); err != nil {
			return err
		}
		if consumed {
			if err := w.require(ctx, "approval.use_consumed", held, res.DecisionID, map[string]any{"useId": useID, "epoch": epoch}); err != nil {
				return err
			}
		}
		claim = &Claim{Job: held, Fence: w.fence(held), UseID: useID, Input: res.Input}
		return nil
	})
	switch {
	case errors.Is(err, ErrNothingToClaim):
		return nil, ErrNothingToClaim
	case err != nil:
		return nil, err
	case denied != nil:
		w.auditRefusal(ctx, denied)
		return nil, nil
	}
	// The use is consumed and committed. The marker moves before the launch, so a
	// restore to a state before this claim is detected (section 4). If the write
	// fails the claim is given back and claims stop until a write succeeds.
	if err := w.Recovery.AdvanceWatermark(ctx); err != nil {
		w.release(ctx, claim, "watermark")
		return nil, fmt.Errorf("job: the recovery marker could not be advanced: %w", err)
	}
	return claim, nil
}

// failQueued fails a queued job inside the transaction of the claim (D-55): a
// denial writes no attempt, and a denied job is not retried (D-80).
func (w *Worker) failQueued(ctx context.Context, j outbound.JobRecord, class string) error {
	return w.Store.Move(ctx, outbound.MoveJob{
		JobID: j.ID, From: job.Queued, To: job.Failed, FailureClass: class, Reason: class, Actor: workerActor,
	})
}

type denial struct {
	job            outbound.JobRecord
	req            inbound.ScopeRequest
	res            inbound.ScopeResult
	approvalReason string
}

// auditRefusal records a refusal after the commit that wrote its decision.
func (w *Worker) auditRefusal(ctx context.Context, d *denial) {
	w.Evaluator.AuditDenial(ctx, w.actor, d.req, d.res, "")
	if d.approvalReason != "" {
		w.record(ctx, "approval.use_denied", audit.Denied, d.approvalReason, d.job, d.res.DecisionID, nil)
	}
	w.record(ctx, "job.failed", audit.Failure, job.FailurePolicyDenial, d.job, d.res.DecisionID, map[string]any{"point": d.req.Point.String()})
}

// release gives a claim back before anything launched: the job is queued again
// with its use consumed, and no attempt is counted (the result that the design
// defines for a crash between the claim and the launch).
func (w *Worker) release(ctx context.Context, c *Claim, why string) {
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), w.Timing.StopTimeout)
	defer cancel()
	err := w.Tx.InTx(rctx, func(ctx context.Context) error {
		return w.Store.Move(ctx, outbound.MoveJob{
			JobID: c.Job.ID, From: job.Running, To: job.Queued, Epoch: c.Fence.Epoch, Worker: c.Fence.Worker, Reason: why, Actor: workerActor, AvailableAt: w.Now(),
		})
	})
	if err != nil && !errors.Is(err, ErrFenced) {
		w.Logger.Error("job: a claim was not given back", "job", c.Job.ID, "reason", why)
	}
}
