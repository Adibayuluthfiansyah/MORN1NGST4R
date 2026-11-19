package worker_test

import (
	"context"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/job"
)

// G-revoke (D-48, O-35): work stops within 30 seconds of a revocation, a scope
// change or a lab change. These tests measure the control path of the worker
// with the real timing (job.DefaultTiming) and a fake process whose kill takes
// time. They measure the worker, the database and the polling. They do not
// measure how fast the sandbox of M8 kills a real process tree: that is
// measured with the launcher.

func TestGRevokeAQueuedJobStopsAtOnce(t *testing.T) {
	r := newRig(t)
	f := r.seedJob(jobSpec{Risk: job.LowImpact})
	start := time.Now()
	if err := r.stop(f.JobID, "approval_revoked"); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if got := r.jobState(f.JobID); got != "cancelled/stop_confirmed/0/0" {
		t.Fatalf("job = %s", got)
	}
	t.Logf("G-revoke: a queued job was cancelled in %v", elapsed)
	if elapsed > job.StopObjective {
		t.Errorf("took %v", elapsed)
	}
}

func TestGRevokeARunningJobOfALiveWorkerStopsWithinThirtySeconds(t *testing.T) {
	r := newRig(t)
	r.timing = job.DefaultTiming()
	r.runner.set(func(f *fakeRunner) { f.stopDelay = time.Second }) // the kill of a process tree takes time
	w := r.worker("worker-a")
	f := r.seedJob(jobSpec{Risk: job.LowImpact})

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	until(t, 10*time.Second, "the job to run", func() bool { return r.runner.count() == 1 })

	start := time.Now()
	if err := r.stop(f.JobID, "scope_changed"); err != nil {
		t.Fatal(err)
	}
	until(t, job.StopObjective, "the job to be cancelled", func() bool { return r.jobState(f.JobID) == "cancelled/stop_confirmed/1/1" })
	elapsed := time.Since(start)
	cancel()
	<-done

	t.Logf("G-revoke: a running job of a live worker was stopped and confirmed in %v (poll %v, kill %v, bound %v)", elapsed, r.timing.ControlPoll, time.Second, r.timing.ControlPoll+r.timing.StopTimeout)
	if elapsed > r.timing.ControlPoll+r.timing.StopTimeout+time.Second {
		t.Errorf("took %v, more than the poll plus the kill timeout", elapsed)
	}
	if elapsed > job.StopObjective {
		t.Errorf("took %v, more than the objective", elapsed)
	}
}

// The attempt of a worker that died is stopped when its lease expires and the
// reaper confirms the tree: the lease, the reaper interval and the termination.
func TestGRevokeTheAttemptOfADeadWorkerEndsWithinThirtySeconds(t *testing.T) {
	if testing.Short() {
		t.Skip("takes about twenty seconds of real time")
	}
	r := newRig(t)
	r.timing = job.DefaultTiming()
	f := r.seedJob(jobSpec{Risk: job.Passive, Retry: job.RetryBounded, MaxAttempts: 3})
	a := r.worker("worker-a")
	mustLaunch(t, a, mustClaim(t, a)) // the worker dies here: no heartbeat follows

	b := r.worker("worker-b")
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- b.Run(ctx) }()
	start := time.Now()
	until(t, job.StopObjective, "the lost attempt to be settled", func() bool { return r.jobState(f.JobID) != "running/none/1/1" })
	elapsed := time.Since(start)
	cancel()
	<-done

	t.Logf("G-revoke: the attempt of a dead worker was settled in %v (lease %v, reaper %v, termination bound %v)", elapsed, r.timing.LeaseTTL, r.timing.Reap, r.timing.StopTimeout)
	if elapsed > job.StopObjective {
		t.Errorf("took %v", elapsed)
	}
	if got := r.scalar(`SELECT state || '/' || (termination_confirmed_at IS NOT NULL) FROM morningstar.job_attempts WHERE job_id = $1 AND attempt_no = 1`, f.JobID); got != "lost/true" {
		t.Errorf("attempt = %s", got)
	}
}

// The loop runs a job from the queue to its end, and stops cleanly.
func TestTheLoopRunsAJobAndStopsCleanly(t *testing.T) {
	r := newRig(t)
	f := r.seedJob(jobSpec{Risk: job.Passive})
	w := r.worker("worker-a")
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	until(t, 5*time.Second, "the process to start", func() bool { return r.runner.count() == 1 })
	r.runner.last().exit(0)
	until(t, 5*time.Second, "the job to succeed", func() bool { return r.jobState(f.JobID) == "succeeded/none/1/1" })
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// A worker that is told to stop ends the attempts that it holds: the process is
// killed and confirmed, and the retry rules decide the job.
func TestAShutdownStopsTheRunningProcessAndSettlesTheJob(t *testing.T) {
	r := newRig(t)
	f := r.seedJob(jobSpec{Risk: job.Passive, Retry: job.RetryBounded, MaxAttempts: 3})
	w := r.worker("worker-a")
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	until(t, 5*time.Second, "the process to start", func() bool { return r.runner.count() == 1 })
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !r.runner.last().stopped.Load() {
		t.Error("the process was left running")
	}
	if got := r.jobState(f.JobID); got != "queued/none/1/1" {
		t.Errorf("job = %s", got)
	}
	if got := r.scalar(`SELECT state || '/' || (termination_confirmed_at IS NOT NULL) FROM morningstar.job_attempts WHERE job_id = $1`, f.JobID); got != "killed/true" {
		t.Errorf("attempt = %s", got)
	}
}
