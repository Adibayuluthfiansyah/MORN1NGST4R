package persistence

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// rollbackTimeout bounds a rollback that runs after the caller gave up.
const rollbackTimeout = 5 * time.Second

// DBTX is what a repository needs to run a statement. Both a pool and a
// transaction satisfy it.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type txKey struct{}

// Querier returns the transaction that ctx carries, or the pool when there is
// none. Every repository of this package runs its statements through it, so
// that a call made inside Transactor.InTx joins the transaction (P-07).
func Querier(ctx context.Context, pool *pgxpool.Pool) DBTX {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return pool
}

// Transactor implements outbound.Transactor over a pool.
type Transactor struct {
	pool *pgxpool.Pool
}

// NewTransactor returns a Transactor that opens READ COMMITTED transactions.
func NewTransactor(pool *pgxpool.Pool) *Transactor { return &Transactor{pool: pool} }

// InTx runs fn in a transaction. A context that already carries a transaction
// joins it: fn runs in it, and its commit or rollback belongs to the outer call.
func (t *Transactor) InTx(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	if _, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return fn(ctx)
	}
	tx, err := t.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted, AccessMode: pgx.ReadWrite})
	if err != nil {
		return fmt.Errorf("persistence: begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		// A rollback uses a fresh context: ctx may be the reason for the failure.
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
		defer cancel()
		_ = tx.Rollback(rctx)
		// A panic continues to unwind after the rollback.
	}()
	if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("persistence: commit: %w", err)
	}
	committed = true
	return nil
}
