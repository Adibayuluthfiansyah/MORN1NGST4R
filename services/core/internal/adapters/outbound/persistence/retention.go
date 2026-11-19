package persistence

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/persistence/sqlcgen"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

// Retention implements outbound.OperationalRetention and
// outbound.EvidenceRetention. It runs with the retention role: the functions of
// migration 00019 and the retention columns of the evidence status accept no
// other role (TestOnlyTheRetentionRolePurgesAndAPurgeIsFinal).
type Retention struct{ base }

var (
	_ outbound.OperationalRetention = Retention{}
	_ outbound.EvidenceRetention    = Retention{}
)

// NewRetention returns the adapter over a pool of the retention role.
func NewRetention(pool *pgxpool.Pool) Retention { return Retention{base{pool}} }

func (d *DB) Retention() Retention { return NewRetention(d.pool) }

func (r Retention) PurgeSessions(ctx context.Context, cutoff time.Time) (outbound.PurgedSessions, error) {
	row, err := r.q(ctx).PurgeSessions(ctx, tsParam(cutoff))
	if err != nil {
		return outbound.PurgedSessions{}, wrap("purge sessions", err)
	}
	return outbound.PurgedSessions{Grants: row.GrantsDeleted, Sessions: row.SessionsDeleted}, nil
}

func (r Retention) PurgeLoginAttempts(ctx context.Context, cutoff time.Time) (int64, error) {
	n, err := r.q(ctx).PurgeLoginAttempts(ctx, tsParam(cutoff))
	return n, wrap("purge login attempts", err)
}

func (r Retention) BeginPurge(ctx context.Context, id string) (bool, error) {
	u, err := uuidOf(id)
	if err != nil {
		return false, outbound.ErrEvidenceNotFound
	}
	n, err := r.q(ctx).BeginEvidencePurge(ctx, u)
	return n == 1, wrap("mark evidence for purge", err)
}

func (r Retention) CompletePurge(ctx context.Context, id, userID string) (bool, error) {
	u, err := uuidOf(id)
	if err != nil {
		return false, outbound.ErrEvidenceNotFound
	}
	user, err := uuidOf(userID)
	if err != nil {
		return false, err
	}
	n, err := r.q(ctx).CompleteEvidencePurge(ctx, sqlcgen.CompleteEvidencePurgeParams{UserID: user, EvidenceID: u})
	return n == 1, wrap("purge evidence", err)
}
