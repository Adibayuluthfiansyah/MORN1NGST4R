package worker_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/persistence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/job"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/infrastructure/ids"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

// The epoch fences alone. A worker that comes back under the same identity after
// its lease was lost holds the owner name of the new claim, so only the epoch
// tells its old claim from the current one.
func TestTheEpochFencesAWorkerThatReturnsUnderTheSameName(t *testing.T) {
	r := newRig(t)
	f := r.seedJob(jobSpec{Risk: job.Passive, Retry: job.RetryBounded, MaxAttempts: 3})
	old := mustClaim(t, r.worker("worker-a"))
	r.clock.Advance(time.Minute)
	if err := r.worker("worker-a").Reap(t.Context()); err != nil {
		t.Fatal(err)
	}
	cur := mustClaim(t, r.worker("worker-a"))
	if old.Fence.Epoch != 1 || cur.Fence.Epoch != 2 || old.Fence.Worker != cur.Fence.Worker {
		t.Fatalf("fences = %+v and %+v", old.Fence, cur.Fence)
	}

	store := persistence.NewJobs(r.pool)
	tx := persistence.NewTransactor(r.pool)
	stale := old.Fence
	until := r.clock.Now().Add(time.Minute)
	if _, err := store.Heartbeat(t.Context(), stale, until); !errors.Is(err, outbound.ErrStaleClaim) {
		t.Errorf("Heartbeat: %v", err)
	}
	if _, err := store.Control(t.Context(), stale); !errors.Is(err, outbound.ErrStaleClaim) {
		t.Errorf("Control: %v", err)
	}
	if err := store.SetControl(t.Context(), stale, job.ControlStopRequested); !errors.Is(err, outbound.ErrStaleClaim) {
		t.Errorf("SetControl: %v", err)
	}
	err := tx.InTx(t.Context(), func(ctx context.Context) error {
		return store.Move(ctx, outbound.MoveJob{
			JobID: f.JobID, From: job.Running, To: job.Succeeded, Epoch: stale.Epoch, Worker: stale.Worker,
			Actor: outbound.TransitionActor{Kind: "worker"},
		})
	})
	if !errors.Is(err, outbound.ErrStaleClaim) {
		t.Errorf("Move: %v", err)
	}
	err = tx.InTx(t.Context(), func(ctx context.Context) error {
		_, err := store.AddAttempt(ctx, outbound.NewAttempt{ID: ids.New(), Fence: stale, DecisionID: ids.New(), UseID: old.UseID})
		return err
	})
	if err == nil {
		t.Error("AddAttempt: a stale epoch started an attempt")
	}

	// The current claim is not affected, and nothing changed the job.
	if _, err := store.Heartbeat(t.Context(), cur.Fence, until); err != nil {
		t.Errorf("Heartbeat of the current claim: %v", err)
	}
	if got := r.jobState(f.JobID); got != "running/none/2/0" {
		t.Errorf("job = %s", got)
	}
}
