package worker_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	appjob "github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/application/job"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/audit"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/job"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/scope"
)

func mustClaim(t *testing.T, w *appjob.Worker) *appjob.Claim {
	t.Helper()
	c, err := w.ClaimNext(t.Context())
	if err != nil || c == nil {
		t.Fatalf("ClaimNext = %v, %v", c, err)
	}
	return c
}

func mustLaunch(t *testing.T, w *appjob.Worker, c *appjob.Claim) *appjob.Active {
	t.Helper()
	a, err := w.Launch(t.Context(), c)
	if err != nil || a == nil {
		t.Fatalf("Launch = %v, %v", a, err)
	}
	return a
}

func TestAClaimConsumesOneUseAndStartsTheLease(t *testing.T) {
	r := newRig(t)
	f := r.seedJob(jobSpec{Risk: job.LowImpact})
	c := mustClaim(t, r.worker("worker-a"))
	if c.Job.ClaimEpoch != 1 || c.UseID == "" {
		t.Fatalf("claim = %+v", c)
	}
	if got := r.jobState(f.JobID); got != "running/none/1/0" {
		t.Errorf("job = %s", got)
	}
	if got := r.uses(f.ApprovalID); got != "1/1" {
		t.Errorf("uses = %s", got)
	}
	if got := r.scalar(`SELECT lease_owner || '/' || (lease_expires_at > now()) FROM morningstar.jobs WHERE id = $1`, f.JobID); got != "worker-a/true" {
		t.Errorf("lease = %s", got)
	}
	if got := r.scalar(`SELECT count(*) FROM morningstar.policy_decisions WHERE job_id = $1 AND point = 'E2' AND outcome = 'allow'`, f.JobID); got != "1" {
		t.Errorf("E2 decisions = %s", got)
	}
	if got := r.scalar(`SELECT count(*) FROM morningstar.job_transitions WHERE job_id = $1 AND from_state = 'queued' AND to_state = 'running' AND actor_kind = 'worker'`, f.JobID); got != "1" {
		t.Errorf("history rows = %s", got)
	}
	if r.events("job.claimed", audit.Success) != "1" || r.events("approval.use_consumed", audit.Success) != "1" {
		t.Errorf("events: claimed %s, use_consumed %s", r.events("job.claimed", audit.Success), r.events("approval.use_consumed", audit.Success))
	}
}

func TestNothingToClaimIsNotAnError(t *testing.T) {
	r := newRig(t)
	if _, err := r.worker("worker-a").ClaimNext(t.Context()); !errors.Is(err, appjob.ErrNothingToClaim) {
		t.Fatalf("err = %v", err)
	}
}

// G-consume at the level of the worker: two workers, one job, one consumption.
func TestTwoWorkersAndOneJobGiveOneConsumption(t *testing.T) {
	for round := range 10 {
		r := newRig(t)
		f := r.seedJob(jobSpec{Risk: job.LowImpact, MaxUses: 5})
		var wg sync.WaitGroup
		claims := make([]*appjob.Claim, 4)
		errs := make([]error, 4)
		for i := range claims {
			wg.Add(1)
			go func() {
				defer wg.Done()
				claims[i], errs[i] = r.worker("worker-" + string(rune('a'+i))).ClaimNext(t.Context())
			}()
		}
		wg.Wait()
		won := 0
		for i := range claims {
			switch {
			case claims[i] != nil:
				won++
			case !errors.Is(errs[i], appjob.ErrNothingToClaim):
				t.Errorf("round %d: worker %d: %v", round, i, errs[i])
			}
		}
		if won != 1 {
			t.Fatalf("round %d: %d workers claimed the job", round, won)
		}
		if got := r.uses(f.ApprovalID); got != "1/1" {
			t.Fatalf("round %d: uses = %s", round, got)
		}
	}
}

func TestALaunchCountsTheAttemptAndASuccessFinishesTheJob(t *testing.T) {
	r := newRig(t)
	f := r.seedJob(jobSpec{Risk: job.LowImpact, MaxAttempts: 3})
	w := r.worker("worker-a")
	a := mustLaunch(t, w, mustClaim(t, w))
	if got := r.jobState(f.JobID); got != "running/none/1/1" {
		t.Errorf("after the launch: %s", got)
	}
	if got := r.scalar(`SELECT state || '/' || attempt_no || '/' || process_group_ref FROM morningstar.job_attempts WHERE job_id = $1`, f.JobID); got != "running/1/pgrp-1" {
		t.Errorf("attempt = %s", got)
	}
	if r.events("job.launched", audit.Success) != "1" {
		t.Error("no job.launched event")
	}
	spec := r.runner.last().spec
	if spec.Authorized.IsZero() || spec.Authorized.Point() != scope.PointE3 || spec.Operation.Hash() != f.Op.Hash() || spec.MaxDuration != 10*time.Minute {
		t.Errorf("launch spec = %+v", spec)
	}

	done := make(chan error, 1)
	go func() { done <- w.Supervise(t.Context(), a) }()
	r.runner.last().exit(0)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := r.jobState(f.JobID); got != "succeeded/none/1/1" {
		t.Errorf("after success: %s", got)
	}
	if got := r.scalar(`SELECT state || '/' || (termination_confirmed_at IS NOT NULL) || '/' || exit_code FROM morningstar.job_attempts WHERE job_id = $1`, f.JobID); got != "succeeded/true/0" {
		t.Errorf("attempt = %s", got)
	}
	if r.events("job.succeeded", audit.Success) != "1" {
		t.Error("no job.succeeded event")
	}
	if got := r.scalar(`SELECT count(*) FROM morningstar.policy_decisions WHERE job_id = $1 AND point = 'E3' AND outcome = 'allow'`, f.JobID); got != "1" {
		t.Errorf("E3 decisions = %s", got)
	}
}

// A job that needs no approval consumes none.
func TestAPassiveJobNeedsNoApproval(t *testing.T) {
	r := newRig(t)
	f := r.seedJob(jobSpec{Risk: job.Passive})
	w := r.worker("worker-a")
	c := mustClaim(t, w)
	if c.UseID != "" {
		t.Errorf("a passive claim holds the use %s", c.UseID)
	}
	a := mustLaunch(t, w, c)
	go func() { r.runner.last().exit(0) }()
	if err := w.Supervise(t.Context(), a); err != nil {
		t.Fatal(err)
	}
	if got := r.jobState(f.JobID); got != "succeeded/none/1/1" {
		t.Errorf("job = %s", got)
	}
	if got := r.scalar(`SELECT count(*) FROM morningstar.approval_uses`); got != "0" {
		t.Errorf("approval uses = %s", got)
	}
}

// A crash between the claim and the launch has one defined result: the job is
// queued again, nothing is counted, and the use that was consumed is reused.
func TestACrashBetweenTheClaimAndTheLaunchRequeuesWithoutAnAttempt(t *testing.T) {
	r := newRig(t)
	f := r.seedJob(jobSpec{Risk: job.LowImpact})
	a := r.worker("worker-a")
	mustClaim(t, a) // the process dies here
	r.clock.Advance(time.Minute)

	b := r.worker("worker-b")
	if err := b.Reap(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := r.jobState(f.JobID); got != "queued/none/1/0" {
		t.Fatalf("after the reaper: %s", got)
	}
	if got := r.scalar(`SELECT count(*) FROM morningstar.job_attempts`); got != "0" {
		t.Errorf("attempts = %s", got)
	}
	if r.events("job.lease_lost", audit.Failure) != "1" {
		t.Error("no job.lease_lost event")
	}
	c := mustClaim(t, b)
	if c.Job.ClaimEpoch != 2 {
		t.Errorf("epoch = %d", c.Job.ClaimEpoch)
	}
	if got := r.uses(f.ApprovalID); got != "1/1" {
		t.Errorf("uses = %s: a retry claim consumed another use", got)
	}
	if r.events("approval.use_consumed", audit.Success) != "1" {
		t.Error("the reused use was recorded as consumed again")
	}
}

// Redelivery of a passive job after the loss of a worker: the attempt is lost,
// the process tree is confirmed gone, and the job is claimed again.
func TestARetryWaitsForTheConfirmedEndOfThePreviousProcessTree(t *testing.T) {
	r := newRig(t)
	f := r.seedJob(jobSpec{Risk: job.Passive, Retry: job.RetryBounded, MaxAttempts: 3})
	a := r.worker("worker-a")
	mustLaunch(t, a, mustClaim(t, a)) // the worker dies while the process runs
	r.clock.Advance(time.Minute)
	r.runner.set(func(f *fakeRunner) { f.terminateOK = false })

	b := r.worker("worker-b")
	if err := b.Reap(t.Context()); err != nil {
		t.Fatal(err)
	}
	// The tree is not confirmed gone: nothing is requeued and no new attempt can start.
	if got := r.jobState(f.JobID); got != "running/stop_failed/1/1" {
		t.Fatalf("while the tree may run: %s", got)
	}
	if got := r.scalar(`SELECT state || '/' || (termination_confirmed_at IS NOT NULL) FROM morningstar.job_attempts WHERE job_id = $1`, f.JobID); got != "lost/false" {
		t.Errorf("attempt = %s", got)
	}
	if got := r.scalar(`SELECT count(*) FROM morningstar.system_alerts WHERE kind = 'sandbox_unavailable'`); got != "1" {
		t.Errorf("alerts = %s", got)
	}
	// The reaper tries again and the alert is not repeated.
	if err := b.Reap(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := r.scalar(`SELECT count(*) FROM morningstar.system_alerts WHERE kind = 'sandbox_unavailable'`); got != "1" {
		t.Errorf("alerts after a second pass = %s", got)
	}
	if _, err := b.ClaimNext(t.Context()); !errors.Is(err, appjob.ErrNothingToClaim) {
		t.Errorf("a job with a live tree was claimed: %v", err)
	}

	// Once the tree is confirmed gone, the job is requeued: the marker that the
	// reaper set was not a stop that a person asked for.
	r.runner.set(func(f *fakeRunner) { f.terminateOK = true })
	if err := b.Reap(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := r.jobState(f.JobID); got != "queued/none/1/1" {
		t.Fatalf("after the confirmation of the lost tree: %s", got)
	}
}

func TestALostAttemptIsRequeuedOnceItsTreeIsConfirmedGone(t *testing.T) {
	r := newRig(t)
	f := r.seedJob(jobSpec{Risk: job.Passive, Retry: job.RetryBounded, MaxAttempts: 3})
	a := r.worker("worker-a")
	mustLaunch(t, a, mustClaim(t, a))
	r.clock.Advance(time.Minute)

	b := r.worker("worker-b")
	if err := b.Reap(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := r.jobState(f.JobID); got != "queued/none/1/1" {
		t.Fatalf("after the reaper: %s", got)
	}
	if got := r.scalar(`SELECT state || '/' || (termination_confirmed_at IS NOT NULL) FROM morningstar.job_attempts WHERE job_id = $1`, f.JobID); got != "lost/true" {
		t.Errorf("attempt = %s", got)
	}
	if len(r.runner.terminated) != 1 || r.runner.terminated[0] != "pgrp-1" {
		t.Errorf("terminated = %v", r.runner.terminated)
	}
	// The second claim and launch are attempt 2, with no new use.
	c := mustClaim(t, b)
	a2 := mustLaunch(t, b, c)
	if a2.Attempt.No != 2 || c.Job.ClaimEpoch != 2 {
		t.Errorf("attempt %d under epoch %d", a2.Attempt.No, c.Job.ClaimEpoch)
	}
}

// D-49: an intrusive job is failed when its lease expires. It is never requeued.
func TestAnIntrusiveJobIsFailedOnLeaseExpiry(t *testing.T) {
	r := newRig(t)
	f := r.seedJob(jobSpec{Risk: job.Intrusive, Retry: job.RetryBounded})
	a := r.worker("worker-a")
	mustLaunch(t, a, mustClaim(t, a))
	r.clock.Advance(time.Minute)
	if err := r.worker("worker-b").Reap(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := r.jobState(f.JobID); got != "failed/none/1/1" {
		t.Fatalf("job = %s", got)
	}
	if got := r.scalar(`SELECT failure_class FROM morningstar.jobs WHERE id = $1`, f.JobID); got != job.FailureLeaseExpired {
		t.Errorf("failure class = %s", got)
	}
	if _, err := r.worker("worker-b").ClaimNext(t.Context()); !errors.Is(err, appjob.ErrNothingToClaim) {
		t.Errorf("a failed intrusive job was claimed: %v", err)
	}
}

// P-36: a worker that lost its lease cannot write anything.
func TestAStaleWorkerIsFencedOut(t *testing.T) {
	r := newRig(t)
	f := r.seedJob(jobSpec{Risk: job.Passive, Retry: job.RetryBounded, MaxAttempts: 3})
	a := r.worker("worker-a")
	active := mustLaunch(t, a, mustClaim(t, a))

	r.clock.Advance(time.Minute)
	b := r.worker("worker-b")
	if err := b.Reap(t.Context()); err != nil {
		t.Fatal(err)
	}
	cb := mustClaim(t, b) // epoch 2
	before := r.jobState(f.JobID)

	// The stale worker wakes up. Its heartbeat is refused, its process is stopped, and it writes nothing.
	done := make(chan error, 1)
	go func() { done <- a.Supervise(t.Context(), active) }()
	if err := <-done; !errors.Is(err, appjob.ErrFenced) {
		t.Fatalf("Supervise = %v, want ErrFenced", err)
	}
	if !r.runner.started[0].stopped.Load() {
		t.Error("the stale worker left its process running")
	}
	if got := r.jobState(f.JobID); got != before || cb.Job.ClaimEpoch != 2 {
		t.Errorf("job = %s, was %s", got, before)
	}
	if got := r.scalar(`SELECT count(*) FROM morningstar.job_attempts WHERE state = 'succeeded'`); got != "0" {
		t.Error("the stale worker finished an attempt")
	}
}

func TestEveryFencedWriteIsRefusedForAStaleEpoch(t *testing.T) {
	r := newRig(t)
	f := r.seedJob(jobSpec{Risk: job.Passive, Retry: job.RetryBounded, MaxAttempts: 3})
	a := r.worker("worker-a")
	active := mustLaunch(t, a, mustClaim(t, a))
	r.clock.Advance(time.Minute)
	b := r.worker("worker-b")
	if err := b.Reap(t.Context()); err != nil {
		t.Fatal(err)
	}
	mustClaim(t, b)

	// The attempt of epoch 1 is lost and terminal: no worker can finish it.
	r.runner.last().exit(0)
	if err := a.Supervise(t.Context(), active); !errors.Is(err, appjob.ErrFenced) && err != nil {
		t.Logf("Supervise = %v", err)
	}
	if got := r.scalar(`SELECT count(*) FROM morningstar.job_attempts WHERE job_id = $1 AND state = 'lost'`, f.JobID); got != "1" {
		t.Errorf("the lost attempt changed: %s", got)
	}
	if got := r.jobState(f.JobID); got != "running/none/2/1" {
		t.Errorf("job = %s", got)
	}
}
