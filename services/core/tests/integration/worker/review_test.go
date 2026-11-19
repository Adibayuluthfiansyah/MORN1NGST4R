package worker_test

// Regression tests for the findings of the M7 review: a stop request is never
// lost, a retry never starts while a process may still run, and a claim under an
// old recovery generation is refused for every kind of job.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/persistence"
	appjob "github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/application/job"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/application/recovery"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/job"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/infrastructure/ids"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

// A stop that arrives between the claim and the launch ends the job at the
// launch. No process was started, so the stop is confirmed at once.
func TestAStopRequestedBeforeTheLaunchCancelsTheJob(t *testing.T) {
	r := newRig(t)
	f := r.seedJob(jobSpec{Risk: job.LowImpact})
	a := r.worker("worker-a")
	c := mustClaim(t, a)
	if err := r.stop(f.JobID, "user_cancel"); err != nil {
		t.Fatal(err)
	}
	act, err := a.Launch(t.Context(), c)
	if act != nil || err != nil {
		t.Fatalf("Launch = %v, %v", act, err)
	}
	if got := r.jobState(f.JobID); got != "cancelled/stop_confirmed/1/0" {
		t.Errorf("job = %s", got)
	}
	if r.runner.count() != 0 {
		t.Error("a process was started for a job that was stopped")
	}
}

// The same for a job that needs no approval, where the refusal comes from the
// attempt counter and not from the launch check of the approval.
func TestAStopRequestedBeforeThePassiveLaunchCancelsTheJob(t *testing.T) {
	r := newRig(t)
	f := r.seedJob(jobSpec{Risk: job.Passive})
	a := r.worker("worker-a")
	c := mustClaim(t, a)
	if err := r.stop(f.JobID, "user_cancel"); err != nil {
		t.Fatal(err)
	}
	act, err := a.Launch(t.Context(), c)
	if act != nil || err != nil {
		t.Fatalf("Launch = %v, %v", act, err)
	}
	if got := r.jobState(f.JobID); got != "cancelled/stop_confirmed/1/0" {
		t.Errorf("job = %s", got)
	}
	if r.runner.count() != 0 {
		t.Error("a process was started for a job that was stopped")
	}
}

// The same stop, when the worker never comes back: the reaper must not turn the
// stop into a requeue (R1).
func TestAStopOnAClaimThatNeverLaunchedIsNotErasedByTheReaper(t *testing.T) {
	r := newRig(t)
	f := r.seedJob(jobSpec{Risk: job.Passive, Retry: job.RetryBounded, MaxAttempts: 3})
	mustClaim(t, r.worker("worker-a"))
	if err := r.stop(f.JobID, "user_cancel"); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(time.Minute)
	if err := r.worker("worker-b").Reap(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := r.jobState(f.JobID); got != "cancelled/stop_confirmed/1/0" {
		t.Errorf("job = %s", got)
	}
	if _, err := r.worker("worker-b").ClaimNext(t.Context()); !errors.Is(err, appjob.ErrNothingToClaim) {
		t.Errorf("a stopped job was claimed again: %v", err)
	}
}

// unconfirmed brings a job to the state where its attempt is lost and the
// runner cannot confirm that the tree is gone: control stop_failed, no reason,
// written by the reaper.
func (r *rig) unconfirmed(f fixture, attemptsBefore int) {
	r.t.Helper()
	a := r.worker("worker-a")
	for i := 0; i < attemptsBefore; i++ {
		act := mustLaunch(r.t, a, mustClaim(r.t, a))
		r.runner.last().exit(1)
		if err := a.Supervise(r.t.Context(), act); err != nil {
			r.t.Fatal(err)
		}
	}
	mustLaunch(r.t, a, mustClaim(r.t, a))
	r.clock.Advance(time.Minute)
	r.runner.set(func(f *fakeRunner) { f.terminateOK = false })
	if err := r.worker("worker-b").Reap(r.t.Context()); err != nil {
		r.t.Fatal(err)
	}
}

// R4: the reason of an earlier requeue is not the reason of a stop. A retryable
// job whose second attempt is lost is requeued once its tree is confirmed gone.
func TestTheReasonOfAnEarlierRequeueIsNotAStopRequest(t *testing.T) {
	r := newRig(t)
	f := r.seedJob(jobSpec{Risk: job.Passive, Retry: job.RetryBounded, MaxAttempts: 3})
	r.unconfirmed(f, 1)
	if got := r.jobState(f.JobID); got != "running/stop_failed/2/2" {
		t.Fatalf("after the unconfirmed tree: %s", got)
	}
	if got := r.scalar(`SELECT coalesce(control_reason, '') FROM morningstar.jobs WHERE id = $1`, f.JobID); got != "" {
		t.Errorf("control_reason = %q, want none", got)
	}
	r.runner.set(func(f *fakeRunner) { f.terminateOK = true })
	if err := r.worker("worker-b").Reap(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := r.jobState(f.JobID); got != "queued/none/2/2" {
		t.Errorf("after the confirmation: %s, want a requeue", got)
	}
	if got := r.events("job.stop_confirmed", "success"); got != "0" {
		t.Errorf("job.stop_confirmed events = %s, but nobody asked for a stop", got)
	}
}

// R3: a person's stop request is taken even while the reaper has marked the tree
// as unconfirmed, and it ends in a cancellation.
func TestAHumanStopIsNotLostWhileTheTreeIsUnconfirmed(t *testing.T) {
	r := newRig(t)
	f := r.seedJob(jobSpec{Risk: job.Passive, Retry: job.RetryBounded, MaxAttempts: 3})
	r.unconfirmed(f, 0)
	if got := r.jobState(f.JobID); got != "running/stop_failed/1/1" {
		t.Fatalf("after the unconfirmed tree: %s", got)
	}
	if err := r.stop(f.JobID, "user_cancel"); err != nil {
		t.Fatal(err)
	}
	if got := r.jobState(f.JobID); got != "running/stop_requested/1/1" {
		t.Fatalf("after the request: %s", got)
	}
	r.runner.set(func(f *fakeRunner) { f.terminateOK = true })
	if err := r.worker("worker-b").Reap(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := r.jobState(f.JobID); got != "cancelled/stop_confirmed/1/1" {
		t.Errorf("job = %s", got)
	}
	if got := r.events("job.cancel_requested", "success"); got != "1" {
		t.Errorf("job.cancel_requested events = %s", got)
	}
}

// R5: the claim of a job that needs no approval is also fenced by the recovery
// generation of the database.
func TestGRecoveryAWorkerCannotClaimAPassiveJobAfterAReconciliation(t *testing.T) {
	r := newRig(t)
	old := r.worker("worker-a")
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
	f := r.seedJob(jobSpec{Risk: job.Passive})
	c, err := old.ClaimNext(t.Context())
	if c != nil || !errors.Is(err, outbound.ErrRecoveryGeneration) {
		t.Fatalf("a stale worker: ClaimNext = %v, %v", c, err)
	}
	if got := r.jobState(f.JobID); got != "queued/none/0/0" {
		t.Errorf("job = %s", got)
	}
}

// R2: the worker may end between the start of a process and the record of its
// reference. The reaper still asks the runner, by the attempt id, and a retry
// waits for the answer.
func TestTheReaperAsksTheRunnerAboutAnAttemptWithoutAReference(t *testing.T) {
	r := newRig(t)
	f := r.seedJob(jobSpec{Risk: job.Passive, Retry: job.RetryBounded, MaxAttempts: 3})
	a := r.worker("worker-a")
	act := mustLaunch(t, a, mustClaim(t, a))
	if _, err := r.c.Owner(t).Exec(t.Context(), `UPDATE morningstar.job_attempts SET process_group_ref = NULL WHERE job_id = $1`, f.JobID); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(time.Minute)
	r.runner.set(func(f *fakeRunner) { f.terminateOK = false })
	if err := r.worker("worker-b").Reap(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := r.jobState(f.JobID); got != "running/stop_failed/1/1" {
		t.Fatalf("an unconfirmed tree must keep the job where it is: %s", got)
	}
	if len(r.runner.terminatedBy) != 1 || r.runner.terminatedBy[0] != act.Attempt.ID || r.runner.terminated[0] != "" {
		t.Errorf("Terminate asked about %v by %v", r.runner.terminated, r.runner.terminatedBy)
	}
}

// R8: a runner that hangs does not hold the reaper past the stop timeout.
func TestAHangingTerminateDoesNotHoldTheReaper(t *testing.T) {
	r := newRig(t)
	r.seedJob(jobSpec{Risk: job.Passive, Retry: job.RetryBounded, MaxAttempts: 3})
	a := r.worker("worker-a")
	mustLaunch(t, a, mustClaim(t, a))
	r.clock.Advance(time.Minute)
	r.runner.set(func(f *fakeRunner) { f.terminateHang = true })
	done := make(chan error, 1)
	go func() { done <- r.worker("worker-b").Reap(t.Context()) }()
	select {
	case <-done:
	case <-time.After(5 * fastTiming.StopTimeout):
		t.Fatal("Reap did not return while the runner hung")
	}
}
