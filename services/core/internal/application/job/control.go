package job

import (
	"context"
	"errors"
	"log/slog"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/audit"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/job"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

// ErrAlreadyFinished: the job is in a final state, so there is nothing to stop.
var ErrAlreadyFinished = errors.New("job: the job is already finished")

// Control is the use case that stops a job. The routes of M21 (cancel) and the
// use cases of M19 (revoke) and of the scope and lab changes call it. A stop
// takes effect whether or not its audit record can be written (SEC 13).
type Control struct {
	Store  outbound.JobStore
	Tx     outbound.Transactor
	Audit  outbound.AuditSink
	NewID  func() string
	Logger *slog.Logger
}

// StopCause says who stops the job and why. Reason is a stable code such as
// user_cancel, approval_revoked, scope_changed or lab_changed.
type StopCause struct {
	Audit      audit.Actor
	Transition outbound.TransitionActor
	Reason     string
	RequestID  string
}

// RequestStop stops a job that has not started at once (it is cancelled where it
// waits), and asks the worker to stop a running one (control_state becomes
// stop_requested, and the worker kills the process tree and confirms it, within
// the stop objective). Asking twice changes nothing.
func (c *Control) RequestStop(ctx context.Context, jobID string, cause StopCause) error {
	if cause.Reason == "" {
		return errors.New("job: a stop needs a reason")
	}
	var j outbound.JobRecord
	var changed bool
	err := c.Tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		if j, err = c.Store.Lock(ctx, jobID); err != nil {
			return err
		}
		switch {
		case j.State.Terminal():
			return ErrAlreadyFinished
		case j.State == job.Running:
			changed, err = c.Store.RequestStop(ctx, j.ID, cause.Reason)
			return err
		}
		changed = true
		return c.Store.Cancel(ctx, outbound.CancelJob{
			JobID: j.ID, From: j.State, Control: job.ControlStopConfirmed, Reason: cause.Reason, Actor: cause.Transition,
		})
	})
	if err != nil {
		return err
	}
	if changed {
		e, err := audit.New(c.NewID(), audit.Input{
			Actor: cause.Audit, Action: "job.cancel_requested", Outcome: audit.Success, Reason: cause.Reason, RequestID: cause.RequestID,
			Subject: subject(j, ""), Detail: map[string]any{"state": string(j.State)},
		})
		if err == nil {
			err = c.Audit.Record(ctx, e)
		}
		if err != nil && c.Logger != nil {
			c.Logger.Error("job: a stop request was not recorded", "job", j.ID)
		}
	}
	return nil
}
