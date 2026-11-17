package persistence

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

// DB is the connection pool of one process role together with the adapters
// that use it. The composition root creates one per process (bootstrap), so no
// other package needs the driver.
type DB struct {
	pool *pgxpool.Pool
	role Role
}

// Open creates the pool for role over the unix socket in socketDir. The pool is
// lazy: it connects on first use, so a database that is not running yet does not
// stop the process from starting and the readiness probe reports it.
func Open(ctx context.Context, socketDir string, role Role, maxConns int32) (*DB, error) {
	pool, err := NewPool(ctx, socketDir, role, maxConns)
	if err != nil {
		return nil, err
	}
	return &DB{pool: pool, role: role}, nil
}

// Close releases every connection.
func (d *DB) Close() { d.pool.Close() }

// Probe is the readiness probe of the database (D-86).
func (d *DB) Probe() outbound.ReadinessProbe { return probe{pool: d.pool} }

// Transactor returns the transaction foundation (P-07).
func (d *DB) Transactor() outbound.Transactor { return NewTransactor(d.pool) }

// ApprovalUses returns the approval consumption adapter. Only the worker role
// has the right to run it.
func (d *DB) ApprovalUses() outbound.ApprovalUses { return NewApprovalUses(d.pool) }

// Recovery returns the recovery repository. Only the server role runs it.
func (d *DB) Recovery() outbound.RecoveryRepository { return NewRecovery(d.pool) }

// errUnavailable is the only error that the probe returns. The driver error can
// quote the socket path and the role, and the health service logs only the name.
var errUnavailable = errors.New("database unavailable")

type probe struct{ pool *pgxpool.Pool }

func (probe) Name() string { return "database" }

// Check opens a connection if the pool has none and runs an empty query. It
// returns when ctx is done.
func (p probe) Check(ctx context.Context) error {
	if err := p.pool.Ping(ctx); err != nil {
		return errUnavailable
	}
	return nil
}
