package job

import (
	"context"
	"errors"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/approval"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/audit"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/job"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

func bindingOf(in outbound.ScopeInput) approval.Binding {
	return approval.Binding{ValidationID: in.Validation.ID, ScopeVersionID: in.Policy.ScopeVersionID, PolicySnapshot: in.Policy.PolicySnapshot}
}

func approvalReason(err error) (string, bool) {
	r, ok := approval.ReasonOf(err)
	return string(r), ok
}

// reapBatch bounds the jobs that one pass handles.
const reapBatch = 20

// Reap handles the jobs that no worker looks after: a job whose lease ended, and
// a job that waits (queued or awaiting approval) with a stop request. Any worker
// may run it, and the row locks keep two workers off one job. It returns the
// first error after it has tried every job.
func (w *Worker) Reap(ctx context.Context) error {
	var first error
	note := func(err error) {
		if err != nil && first == nil && !errors.Is(err, ErrFenced) {
			first = err
		}
	}
	var expired, waiting []outbound.JobRecord
	err := w.Tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		if expired, err = w.Store.ExpiredLeases(ctx, w.Now(), reapBatch); err != nil {
			return err
		}
		if waiting, err = w.Store.StopRequestedWaiting(ctx, reapBatch); err != nil {
			return err
		}
		// A job that waits with a stop request is cancelled where it stands.
		for _, j := range waiting {
			if err := w.Store.Cancel(ctx, outbound.CancelJob{
				JobID: j.ID, From: j.State, Control: job.ControlStopConfirmed, Reason: job.FailureStopped, Actor: outbound.TransitionActor{Kind: "system"},
			}); err != nil {
				return err
			}
		}
		// The attempts of expired leases are marked lost in the same pass.
		for _, j := range expired {
			if err := w.markLost(ctx, j); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, j := range waiting {
		w.record(ctx, "job.stop_confirmed", audit.Success, "", j, "", map[string]any{"state": string(j.State)})
	}
	for _, j := range expired {
		w.record(ctx, "job.lease_lost", audit.Failure, "lease_expired", j, "", map[string]any{"epoch": j.ClaimEpoch, "worker": j.LeaseOwner})
		note(w.settleLost(ctx, j))
	}
	return first
}

// markLost marks the live attempt of the expired claim as lost. A job that was
// claimed and never launched has no attempt to mark.
func (w *Worker) markLost(ctx context.Context, j outbound.JobRecord) error {
	attempts, err := w.Store.Attempts(ctx, j.ID)
	if err != nil {
		return err
	}
	for _, a := range attempts {
		if a.ClaimEpoch == j.ClaimEpoch && a.State.Live() {
			if err := w.Store.UpdateAttempt(ctx, outbound.AttemptUpdate{ID: a.ID, State: job.AttemptLost, End: true}); err != nil {
				return err
			}
		}
	}
	return nil
}

// settleLost decides the job of an expired lease (D-49, D-80). A claim without
// an attempt is queued again at once, and its approval use stays consumed. A
// claim with an attempt waits for the confirmed end of the process tree, and then
// the retry rules decide: an intrusive job fails, a retryable one is requeued.
// The decision is made on the locked row, so a stop request that arrives while
// the reaper works is never overwritten: it ends in a cancellation.
func (w *Worker) settleLost(ctx context.Context, j outbound.JobRecord) error {
	attempts, err := w.Store.Attempts(ctx, j.ID)
	if err != nil {
		return err
	}
	var last *outbound.Attempt
	for i := range attempts {
		if attempts[i].ClaimEpoch == j.ClaimEpoch {
			last = &attempts[i]
		}
	}
	if last != nil && !last.TerminationConfirmed {
		// The runner is asked even when no reference was recorded: the worker may
		// have ended between the start of the process and that record.
		tctx, cancel := context.WithTimeout(ctx, w.Timing.StopTimeout)
		ok, terr := w.Runner.Terminate(tctx, last.ID, last.ProcessRef)
		cancel()
		if terr != nil || !ok {
			return w.terminationFailed(ctx, j, w.fence(j), *last)
		}
		if err := w.Store.UpdateAttempt(ctx, outbound.AttemptUpdate{ID: last.ID, ConfirmTermination: true}); err != nil {
			return err
		}
	}

	var after func()
	octx := ctx
	err = w.Tx.InTx(ctx, func(ctx context.Context) error {
		after = nil
		cur, err := w.Store.Lock(ctx, j.ID)
		if err != nil {
			return err
		}
		if cur.State != job.Running || cur.ClaimEpoch != j.ClaimEpoch || cur.LeaseOwner != j.LeaseOwner {
			return nil // another worker settled it
		}
		fenced := func(m outbound.MoveJob) error {
			m.JobID, m.From, m.Epoch, m.Worker, m.Actor = cur.ID, job.Running, cur.ClaimEpoch, cur.LeaseOwner, outbound.TransitionActor{Kind: "system"}
			return w.Store.Move(ctx, m)
		}
		// stop_failed without a reason was written by this reaper when it could not
		// confirm a lost tree. It is not a stop that a person asked for, so it does
		// not stop the retry (D-80 forbids a retry after a cancellation or
		// revocation only).
		control := cur.Control
		if control == job.ControlStopFailed && cur.ControlReason == "" {
			control = job.ControlNone
		}
		if control != job.ControlNone {
			// The stop that was asked for is done now that the tree is confirmed gone.
			if err := fenced(outbound.MoveJob{To: job.Cancelled, Control: job.ControlStopConfirmed, FailureClass: job.FailureStopped, Reason: job.FailureStopped}); err != nil {
				return err
			}
			after = func() {
				w.record(octx, "job.stop_confirmed", audit.Success, "", cur, "", map[string]any{"attempts": cur.Attempts})
			}
			return nil
		}
		if last == nil {
			return fenced(outbound.MoveJob{To: job.Queued, Control: job.ControlNone, Reason: "claim_not_launched", AvailableAt: w.Now()})
		}
		out := job.DecideRetry(job.RetryInput{
			Policy: cur.RetryPolicy, Risk: cur.Risk, Cause: job.CauseLeaseLost, Attempts: cur.Attempts, MaxAttempts: cur.MaxAttempts,
			Control: control, TerminationConfirmed: true,
		})
		switch out.Action {
		case job.ActionRequeue:
			return fenced(outbound.MoveJob{To: job.Queued, Control: job.ControlNone, Reason: string(job.CauseLeaseLost), AvailableAt: w.Now()})
		case job.ActionFail:
			if err := fenced(outbound.MoveJob{To: job.Failed, FailureClass: out.FailureClass, Reason: out.FailureClass}); err != nil {
				return err
			}
			after = func() {
				w.record(octx, "job.failed", audit.Failure, out.FailureClass, cur, "", map[string]any{"cause": string(job.CauseLeaseLost)})
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if after != nil {
		after()
	}
	return nil
}

// terminationFailed says, once, that a lost process tree could not be confirmed
// gone. The job stays where it is: nothing is requeued while a process may run.
func (w *Worker) terminationFailed(ctx context.Context, j outbound.JobRecord, f outbound.Fence, a outbound.Attempt) error {
	if j.Control == job.ControlStopFailed {
		return nil
	}
	err := w.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := w.Store.SetControl(ctx, f, job.ControlStopFailed); err != nil {
			return err
		}
		return w.Alerts.Raise(ctx, "sandbox_unavailable", map[string]any{"jobId": j.ID, "attemptId": a.ID, "reason": "termination_unconfirmed"})
	})
	w.record(ctx, "job.stop_failed", audit.Failure, "termination_unconfirmed", j, "", map[string]any{"attempt": a.No})
	return err
}
