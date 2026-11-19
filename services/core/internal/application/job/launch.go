package job

import (
	"context"
	"errors"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/audit"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/job"
	domscope "github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/scope"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

// Active is a launched attempt that this worker supervises.
type Active struct {
	Claim   *Claim
	Attempt outbound.Attempt
	Process outbound.Process
}

// Launch decides at E3, refuses before an attempt exists when the sandbox
// refuses, and then counts the attempt and starts the process (D-55, section 4.3
// of domain-model.md). It returns a nil Active with a nil error when the job
// ended during the launch (a denial, a refused preflight or a failed spawn), and
// ErrFenced when the claim is no longer current.
func (w *Worker) Launch(ctx context.Context, c *Claim) (*Active, error) {
	if err := w.Audit.Gate(ctx); err != nil {
		w.release(ctx, c, "audit_degraded")
		return nil, err
	}

	// E3 and its decision commit before the attempt exists. A denial fails the job
	// in the same transaction, so a refused launch is never an attempt.
	req, err := w.launchRequest(ctx, c)
	if err != nil {
		return nil, err
	}
	var (
		res    inbound.ScopeResult
		denied bool
	)
	err = w.Tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		if res, err = w.Evaluator.Evaluate(ctx, req); err != nil {
			return err
		}
		if res.Allowed() {
			return nil
		}
		denied = true
		return w.Store.Move(ctx, outbound.MoveJob{
			JobID: c.Job.ID, From: job.Running, To: job.Failed, Epoch: c.Fence.Epoch, Worker: c.Fence.Worker,
			FailureClass: job.FailurePolicyDenial, Reason: job.FailurePolicyDenial, Actor: workerActor,
		})
	})
	if err != nil {
		return nil, err
	}
	if denied {
		w.Evaluator.AuditDenial(ctx, w.actor, req, res, "")
		w.record(ctx, "job.failed", audit.Failure, job.FailurePolicyDenial, c.Job, res.DecisionID, map[string]any{"point": "E3"})
		return nil, nil
	}

	spec := outbound.LaunchSpec{
		AttemptID: w.NewID(), JobID: c.Job.ID, OperationID: c.Job.OperationID, Epoch: c.Fence.Epoch,
		MaxDuration: c.Input.Operation.Limits().MaxDuration, Operation: c.Input.Operation, Authorized: res.Verdict.Authorized,
	}
	if err := w.Runner.Preflight(ctx, spec); err != nil {
		return nil, w.failClaimed(ctx, c, res.DecisionID, job.FailureSandboxRefused)
	}

	var attempt outbound.Attempt
	err = w.Tx.InTx(ctx, func(ctx context.Context) error {
		if c.Job.Risk.NeedsApproval() {
			if err := w.Uses.CheckLaunch(ctx, outbound.LaunchCheck{
				ApprovalID: c.Job.ApprovalID, JobID: c.Job.ID, Generation: w.generation(), ClaimEpoch: c.Fence.Epoch,
				Binding: bindingOf(c.Input), Operation: c.Input.Operation.Hash(),
			}); err != nil {
				return err
			}
		}
		var err error
		attempt, err = w.Store.AddAttempt(ctx, outbound.NewAttempt{ID: spec.AttemptID, Fence: c.Fence, DecisionID: res.DecisionID, UseID: c.UseID})
		if err != nil {
			return err
		}
		launched := c.Job
		launched.Attempts = attempt.No
		return w.require(ctx, "job.launched", launched, res.DecisionID, map[string]any{"attempt": attempt.No, "epoch": c.Fence.Epoch})
	})
	switch {
	case errors.Is(err, ErrFenced):
		// A stop request refuses the attempt too (the counter is raised only while
		// the control is none). No process exists, so the stop is confirmed here.
		stopped, serr := w.cancelStopped(ctx, c)
		if serr != nil {
			return nil, serr
		}
		if stopped {
			return nil, nil
		}
		return nil, ErrFenced
	case errors.Is(err, outbound.ErrAttemptsExhausted):
		return nil, w.failClaimed(ctx, c, res.DecisionID, job.FailureAttemptsExhausted)
	case err != nil:
		// A stop request refuses the launch check as well: the claim is cancelled,
		// not failed by a denial.
		if stopped, _ := w.cancelStopped(ctx, c); stopped {
			return nil, nil
		}
		// The launch was refused after E3 allowed it (a revocation, a changed scope
		// or an audit failure). Nothing was counted: the claim is given back or the
		// job is failed by the denial.
		if reason, isDenial := approvalReason(err); isDenial {
			return nil, w.failClaimed(ctx, c, res.DecisionID, job.FailurePolicyDenial, reason)
		}
		w.release(ctx, c, "launch_refused")
		return nil, err
	}

	proc, err := w.Runner.Start(ctx, spec)
	if err != nil {
		// The launch is consumed and counts (D-55). The attempt failed before any
		// process existed, so its termination is confirmed.
		a := &Active{Claim: c, Attempt: attempt}
		return nil, w.endAttempt(ctx, a, ended{state: job.AttemptFailed, errorClass: job.FailureSandboxRefused, confirmed: true, cause: job.CauseExecution})
	}
	if err := w.Store.UpdateAttempt(ctx, outbound.AttemptUpdate{ID: attempt.ID, State: job.AttemptRunning, ProcessRef: proc.Ref()}); err != nil {
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), w.Timing.StopTimeout)
		defer cancel()
		_ = proc.Stop(stopCtx)
		return nil, err
	}
	attempt.State, attempt.ProcessRef = job.AttemptRunning, proc.Ref()
	return &Active{Claim: c, Attempt: attempt, Process: proc}, nil
}

func (w *Worker) generation() uint64 {
	g, _ := w.Recovery.Generation()
	return g
}

func (w *Worker) launchRequest(ctx context.Context, c *Claim) (inbound.ScopeRequest, error) {
	req := inbound.ScopeRequest{Point: domscope.PointE3, Job: c.Job, PreviousTreeConfirmed: true}
	if c.Job.ApprovalID != "" {
		n, err := w.Store.RunningCount(ctx, c.Job.ApprovalID)
		if err != nil {
			return req, err
		}
		// The count includes this job, which the approval counts as it launches.
		req.RunningJobs = max(n-1, 0)
	}
	attempts, err := w.Store.Attempts(ctx, c.Job.ID)
	if err != nil {
		return req, err
	}
	if len(attempts) > 0 {
		req.PreviousTreeConfirmed = attempts[len(attempts)-1].TerminationConfirmed
	}
	return req, nil
}

// cancelStopped cancels the job of a claim that has no attempt when a person has
// asked for a stop. It reports false when the claim is gone or no stop was asked.
func (w *Worker) cancelStopped(ctx context.Context, c *Claim) (bool, error) {
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), w.Timing.StopTimeout)
	defer cancel()
	var cancelled bool
	err := w.Tx.InTx(rctx, func(ctx context.Context) error {
		cancelled = false
		cur, err := w.Store.Lock(ctx, c.Job.ID)
		if err != nil {
			return err
		}
		asked := cur.Control == job.ControlStopRequested || (cur.Control == job.ControlStopFailed && cur.ControlReason != "")
		if cur.State != job.Running || cur.ClaimEpoch != c.Fence.Epoch || cur.LeaseOwner != c.Fence.Worker || !asked {
			return nil
		}
		cancelled = true
		return w.Store.Move(ctx, outbound.MoveJob{
			JobID: cur.ID, From: job.Running, To: job.Cancelled, Epoch: c.Fence.Epoch, Worker: c.Fence.Worker,
			Control: job.ControlStopConfirmed, FailureClass: job.FailureStopped, Reason: job.FailureStopped, Actor: workerActor,
		})
	})
	if err != nil {
		return false, err
	}
	if cancelled {
		w.record(ctx, "job.stop_confirmed", audit.Success, "", c.Job, "", map[string]any{"attempt": 0})
	}
	return cancelled, nil
}

// failClaimed fails a job that holds a claim and has no live attempt.
func (w *Worker) failClaimed(ctx context.Context, c *Claim, decision, class string, why ...string) error {
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), w.Timing.StopTimeout)
	defer cancel()
	err := w.Tx.InTx(rctx, func(ctx context.Context) error {
		return w.Store.Move(ctx, outbound.MoveJob{
			JobID: c.Job.ID, From: job.Running, To: job.Failed, Epoch: c.Fence.Epoch, Worker: c.Fence.Worker,
			FailureClass: class, Reason: class, Actor: workerActor,
		})
	})
	if err != nil {
		return err
	}
	reason := class
	if len(why) > 0 {
		reason = why[0]
	}
	w.record(ctx, "job.failed", audit.Failure, reason, c.Job, decision, map[string]any{"failureClass": class})
	return nil
}

type ended struct {
	state      job.AttemptState
	errorClass string
	exit       *int
	confirmed  bool
	cause      job.Cause
	success    bool
	stopped    bool
}

// endAttempt closes the attempt and moves the job in one transaction: success,
// the confirmed stop of a cancel or revocation, a requeue for a retry, or the
// failure of the job (D-80). The records follow the commit.
func (w *Worker) endAttempt(ctx context.Context, a *Active, e ended) error {
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), w.Timing.StopTimeout)
	defer cancel()
	var outcome job.Outcome
	var to job.State
	err := w.Tx.InTx(rctx, func(ctx context.Context) error {
		if err := w.Store.UpdateAttempt(ctx, outbound.AttemptUpdate{
			ID: a.Attempt.ID, State: e.state, ErrorClass: e.errorClass, ExitCode: e.exit, End: true, ConfirmTermination: e.confirmed,
		}); err != nil {
			return err
		}
		cur, err := w.Store.Lock(ctx, a.Claim.Job.ID)
		if err != nil {
			return err
		}
		mv := outbound.MoveJob{JobID: cur.ID, From: job.Running, Epoch: a.Claim.Fence.Epoch, Worker: a.Claim.Fence.Worker, Actor: workerActor}
		switch {
		case e.success:
			mv.To = job.Succeeded
		case e.stopped:
			mv.To, mv.Control, mv.FailureClass, mv.Reason = job.Cancelled, job.ControlStopConfirmed, job.FailureStopped, job.FailureStopped
		default:
			outcome = job.DecideRetry(job.RetryInput{
				Policy: cur.RetryPolicy, Risk: cur.Risk, Cause: e.cause, Attempts: cur.Attempts, MaxAttempts: cur.MaxAttempts,
				Control: cur.Control, TerminationConfirmed: e.confirmed,
			})
			switch outcome.Action {
			case job.ActionRequeue:
				mv.To, mv.AvailableAt, mv.Reason = job.Queued, w.Now(), string(e.cause)
			case job.ActionFail:
				mv.To, mv.FailureClass, mv.Reason = job.Failed, outcome.FailureClass, outcome.FailureClass
			default:
				// Nothing is decided while the process tree may still run.
				return nil
			}
		}
		to = mv.To
		return w.Store.Move(ctx, mv)
	})
	if err != nil {
		return err
	}
	j := a.Claim.Job
	switch {
	case to == job.Succeeded:
		w.record(ctx, "job.succeeded", audit.Success, "", j, "", map[string]any{"attempt": a.Attempt.No})
	case to == job.Cancelled:
		w.record(ctx, "job.stop_confirmed", audit.Success, "", j, "", map[string]any{"attempt": a.Attempt.No})
	case to == job.Failed:
		w.record(ctx, "job.failed", audit.Failure, outcome.FailureClass, j, "", map[string]any{"attempt": a.Attempt.No})
	}
	return nil
}

// Supervise keeps the lease of a running attempt, watches for a stop request and
// ends the attempt when the process ends. It returns ErrFenced when the claim is
// lost: the process is then stopped and nothing is written.
func (w *Worker) Supervise(ctx context.Context, a *Active) error {
	beat := time.NewTicker(w.Timing.Heartbeat)
	poll := time.NewTicker(w.Timing.ControlPoll)
	defer beat.Stop()
	defer poll.Stop()
	for {
		select {
		case exit := <-a.Process.Done():
			return w.onExit(ctx, a, exit)
		case <-beat.C:
			bctx, cancel := context.WithTimeout(ctx, w.Timing.LeaseTTL)
			ctl, err := w.Store.Heartbeat(bctx, a.Claim.Fence, w.Now().Add(w.Timing.LeaseTTL))
			cancel()
			if errors.Is(err, ErrFenced) {
				return w.abandon(ctx, a)
			}
			if err == nil && ctl != job.ControlNone {
				return w.stop(ctx, a)
			}
			// A database that is down is not a reason to kill work: the lease holds
			// until it ends, and the next beat tries again.
		case <-poll.C:
			pctx, cancel := context.WithTimeout(ctx, w.Timing.LeaseTTL)
			ctl, err := w.Store.Control(pctx, a.Claim.Fence)
			cancel()
			if errors.Is(err, ErrFenced) {
				return w.abandon(ctx, a)
			}
			if err == nil && ctl != job.ControlNone {
				return w.stop(ctx, a)
			}
		case <-ctx.Done():
			return w.shutdown(ctx, a)
		}
	}
}

func (w *Worker) onExit(ctx context.Context, a *Active, exit outbound.ProcessExit) error {
	code := exit.Code
	switch {
	case exit.Err != nil:
		// The worker cannot tell whether the tree is gone: it makes sure.
		if err := w.stopProcess(ctx, a); err != nil {
			return w.stopFailed(ctx, a)
		}
		return w.endAttempt(ctx, a, ended{state: job.AttemptFailed, errorClass: "wait_failed", confirmed: true, cause: job.CauseExecution})
	case exit.TimedOut:
		return w.endAttempt(ctx, a, ended{state: job.AttemptTimedOut, errorClass: job.FailureTimedOut, exit: &code, confirmed: true, cause: job.CauseTimeout})
	case exit.Code == 0:
		return w.endAttempt(ctx, a, ended{state: job.AttemptSucceeded, exit: &code, confirmed: true, success: true})
	}
	return w.endAttempt(ctx, a, ended{state: job.AttemptFailed, errorClass: "exit_code", exit: &code, confirmed: true, cause: job.CauseExecution})
}

func (w *Worker) stopProcess(ctx context.Context, a *Active) error {
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), w.Timing.StopTimeout)
	defer cancel()
	return a.Process.Stop(sctx)
}

// stop ends the attempt on a stop request: the process tree is killed and
// confirmed gone, then the job is cancelled (FR-APPROVAL-003). If the worker
// cannot confirm it, it says so and raises an alert, and the job stays running.
func (w *Worker) stop(ctx context.Context, a *Active) error {
	if err := w.stopProcess(ctx, a); err != nil {
		return w.stopFailed(ctx, a)
	}
	return w.endAttempt(ctx, a, ended{state: job.AttemptCancelled, errorClass: job.FailureStopped, confirmed: true, stopped: true})
}

func (w *Worker) stopFailed(ctx context.Context, a *Active) error {
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), w.Timing.StopTimeout)
	defer cancel()
	err := w.Tx.InTx(rctx, func(ctx context.Context) error {
		if err := w.Store.SetControl(ctx, a.Claim.Fence, job.ControlStopFailed); err != nil {
			return err
		}
		return w.Alerts.Raise(ctx, "sandbox_unavailable", map[string]any{"jobId": a.Claim.Job.ID, "attemptId": a.Attempt.ID, "reason": "stop_failed"})
	})
	w.record(ctx, "job.stop_failed", audit.Failure, "stop_failed", a.Claim.Job, "", map[string]any{"attempt": a.Attempt.No})
	if err != nil {
		return err
	}
	return errors.New("job: the process of the attempt could not be confirmed gone")
}

// abandon is what a stale worker does: it stops its process and writes nothing,
// because another worker or the reaper owns the job now.
func (w *Worker) abandon(ctx context.Context, a *Active) error {
	_ = w.stopProcess(ctx, a)
	return ErrFenced
}

// shutdown ends an attempt when the worker is told to stop. The process is
// killed and confirmed, and the job is retried or failed by the retry rules.
func (w *Worker) shutdown(ctx context.Context, a *Active) error {
	if err := w.stopProcess(ctx, a); err != nil {
		return w.stopFailed(ctx, a)
	}
	return w.endAttempt(ctx, a, ended{state: job.AttemptKilled, errorClass: "worker_shutdown", confirmed: true, cause: job.CauseExecution})
}
