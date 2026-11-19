package job

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// Run is the loop of the worker. It claims jobs while it has capacity, runs each
// in its own goroutine, reaps expired leases and stop requests, and advances the
// recovery marker on a timer. It returns when ctx is done and every running job
// has been stopped and settled. A claim that fails because audit is degraded or
// recovery is required is not an error of the loop: the worker waits and tries
// again, so it resumes by itself when the cause is gone.
func (w *Worker) Run(ctx context.Context) error {
	var running sync.WaitGroup
	var reaping atomic.Bool
	slots := make(chan struct{}, w.Capacity)
	reap := time.NewTicker(w.Timing.Reap)
	mark := time.NewTicker(w.Timing.Watermark)
	defer reap.Stop()
	defer mark.Stop()

	for {
		select {
		case <-ctx.Done():
			running.Wait()
			return nil
		case <-reap.C:
			// One reaper at a time, outside the claim loop: a runner that is slow to
			// answer must not hold the claims.
			if reaping.CompareAndSwap(false, true) {
				running.Add(1)
				go func() {
					defer running.Done()
					defer reaping.Store(false)
					if err := w.Reap(ctx); err != nil {
						w.Logger.Warn("job: the reaper met an error")
					}
				}()
			}
		case <-mark.C:
			if err := w.Recovery.AdvanceWatermark(ctx); err != nil {
				w.Logger.Warn("job: the recovery marker was not advanced")
			}
		case slots <- struct{}{}:
			c, err := w.ClaimNext(ctx)
			switch {
			case err == nil && c != nil:
				running.Add(1)
				go func() {
					defer running.Done()
					defer func() { <-slots }()
					w.runClaim(ctx, c)
				}()
				continue
			case err == nil:
				<-slots // a job was refused: look at the next one at once
				continue
			case !errors.Is(err, ErrNothingToClaim):
				w.Logger.Warn("job: no claim was made")
			}
			<-slots
			select {
			case <-ctx.Done():
			case <-time.After(w.ClaimPoll):
			}
		}
	}
}

func (w *Worker) runClaim(ctx context.Context, c *Claim) {
	a, err := w.Launch(ctx, c)
	if err != nil || a == nil {
		if err != nil && !errors.Is(err, ErrFenced) {
			w.Logger.Warn("job: a launch did not complete", "job", c.Job.ID)
		}
		return
	}
	if err := w.Supervise(ctx, a); err != nil && !errors.Is(err, ErrFenced) {
		w.Logger.Warn("job: an attempt did not end cleanly", "job", c.Job.ID)
	}
}
