package outbound

import "context"

// Transactor runs a function in one database transaction (P-07). Every
// repository call made with the context that fn receives joins that
// transaction. fn's error rolls it back, and so does a panic. A nil error
// commits.
//
// Transactions are READ COMMITTED. Atomicity of a read-then-write sequence
// comes from a conditional statement or a row lock inside the transaction, not
// from a stronger isolation level.
//
// InTx does not nest: calling it with a context that already carries a
// transaction joins that transaction and does not start a savepoint.
type Transactor interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
}
