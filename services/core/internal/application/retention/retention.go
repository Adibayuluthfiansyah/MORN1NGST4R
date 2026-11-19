// Package retention purges the security-operational rows of class RC-B
// (data-lifecycle.md, TD-M3-03 and TD-M4-02): sessions and re-authentication
// grants 30 days after they ended, and login attempts 30 days after they were
// made. The database enforces the age floor and the role, and the purge writes
// its audit event in the same transaction, so there is no purge without an
// event (SEC 13).
//
// Audit rows are not purged. No number for them was approved, and the guard of
// audit_events still refuses every update and delete (TD-M3-03, audit part).
package retention

import (
	"context"
	"errors"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/audit"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

// Floor is the age floor of RC-B. The database refuses a younger cutoff.
const Floor = 30 * 24 * time.Hour

// margin keeps the cutoff behind the clock of the database when the clock of
// this process runs a little ahead of it.
const margin = time.Minute

type Deps struct {
	Repo  outbound.OperationalRetention
	Tx    outbound.Transactor
	Audit outbound.AuditSink
	NewID func() string
	Now   func() time.Time
}

type Service struct{ Deps }

func New(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Service{d}
}

// Counts is what a purge removed.
type Counts = inbound.PurgeCounts

// PurgeOperational removes what is past the floor. It writes `retention.purged`
// in the same transaction when it removed anything, and nothing when there was
// nothing to remove.
func (s *Service) PurgeOperational(ctx context.Context) (Counts, error) {
	if s.Repo == nil || s.Tx == nil || s.Audit == nil || s.NewID == nil {
		return Counts{}, errors.New("retention: a dependency is missing")
	}
	cutoff := s.Now().UTC().Add(-Floor - margin)
	var c Counts
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		sess, err := s.Repo.PurgeSessions(ctx, cutoff)
		if err != nil {
			return err
		}
		attempts, err := s.Repo.PurgeLoginAttempts(ctx, cutoff)
		if err != nil {
			return err
		}
		c = Counts{Grants: sess.Grants, Sessions: sess.Sessions, LoginAttempts: attempts}
		if c.None() {
			return nil
		}
		e, err := audit.New(s.NewID(), audit.Input{
			Actor: audit.System(), Action: "retention.purged", Outcome: audit.Success,
			Detail: map[string]any{"sessions": c.Sessions, "grants": c.Grants, "loginAttempts": c.LoginAttempts, "cutoff": cutoff.Format(time.RFC3339)},
		})
		if err != nil {
			return err
		}
		return s.Audit.Require(ctx, e)
	})
	if err != nil {
		return Counts{}, err
	}
	return c, nil
}
