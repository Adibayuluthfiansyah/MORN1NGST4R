// Package maintenance is the periodic work of the worker that no job asks for:
// the purge of operational rows past their floor, the sweep of the evidence
// directory and the integrity re-check of evidence (TD-M3-03, TD-M4-02,
// TD-M6-01). One pass is Once, and Run repeats it. A failure of one part does
// not stop the others, and none of it holds a claim on a job.
package maintenance

import (
	"context"
	"log/slog"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
)

// Policy holds the numbers of the maintenance. They are provisional until the
// owner sets them (DQ-07, O-08).
type Policy struct {
	// Every is how often a pass runs.
	Every time.Duration
	Sweep inbound.SweepPolicy
}

// DefaultPolicy: a pass every hour, an orphan after one hour, an integrity
// re-check of every item at least once a day, 50 items per pass.
func DefaultPolicy() Policy {
	return Policy{Every: time.Hour, Sweep: inbound.SweepPolicy{OrphanAge: time.Hour, RecheckAfter: 24 * time.Hour, Batch: 50}}
}

type Service struct {
	Evidence inbound.EvidenceSweeper
	Purger   inbound.OperationalPurger
	Policy   Policy
	Logger   *slog.Logger
}

// Once runs one pass.
func (s *Service) Once(ctx context.Context) error {
	var first error
	if s.Purger != nil {
		if c, err := s.Purger.PurgeOperational(ctx); err != nil {
			first = err
			s.logger().Warn("maintenance: the purge of operational rows failed")
		} else if !c.None() {
			s.logger().Info("maintenance: operational rows were purged", "sessions", c.Sessions, "grants", c.Grants, "loginAttempts", c.LoginAttempts)
		}
	}
	if s.Evidence != nil {
		if r, err := s.Evidence.Sweep(ctx, s.Policy.Sweep); err != nil {
			if first == nil {
				first = err
			}
			s.logger().Warn("maintenance: the evidence sweep failed")
		} else if r != (inbound.SweepResult{}) {
			s.logger().Info("maintenance: the evidence sweep ran", "orphans", r.OrphansRemoved, "temporary", r.TemporaryRemoved, "unrecognized", r.Unrecognized, "checked", r.Checked, "failed", r.Failed)
		}
	}
	return first
}

// Run runs a pass at once and then every Policy.Every until ctx is done.
func (s *Service) Run(ctx context.Context) {
	every := s.Policy.Every
	if every <= 0 {
		every = DefaultPolicy().Every
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		_ = s.Once(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *Service) logger() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.New(slog.DiscardHandler)
}
