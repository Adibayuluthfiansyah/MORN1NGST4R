package inbound

import (
	"context"
	"time"
)

// SweepPolicy holds the numbers of one evidence sweep.
type SweepPolicy struct {
	// OrphanAge is how old a file must be before the sweep treats a file without a
	// row as left behind. A write that is in progress is younger than this.
	OrphanAge time.Duration
	// RecheckAfter is how long an item may go without a check.
	RecheckAfter time.Duration
	// Batch bounds the items that one sweep checks.
	Batch int
}

// SweepResult counts what a sweep did.
type SweepResult struct {
	OrphansRemoved, TemporaryRemoved, Unrecognized int
	Checked, Failed                                int
}

// EvidenceSweeper looks after the evidence directory and the evidence rows.
type EvidenceSweeper interface {
	Sweep(ctx context.Context, p SweepPolicy) (SweepResult, error)
}

// PurgeCounts is what one purge of operational rows removed.
type PurgeCounts struct{ Grants, Sessions, LoginAttempts int64 }

func (c PurgeCounts) None() bool { return c.Grants == 0 && c.Sessions == 0 && c.LoginAttempts == 0 }

// OperationalPurger removes operational rows past their floor.
type OperationalPurger interface {
	PurgeOperational(ctx context.Context) (PurgeCounts, error)
}
