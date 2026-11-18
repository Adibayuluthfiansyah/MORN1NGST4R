package outbound

import (
	"context"
	"errors"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/audit"
)

// ErrAuditNotRecorded marks an event that was not written. It is not a denial of
// the operation: a caller that sees it knows only that the record is missing.
// When the cause was an unreachable database the error also wraps
// ErrStoreUnavailable.
var ErrAuditNotRecorded = errors.New("the audit record was not written")

// ErrAuditDegraded is returned by AuditSink.Gate while audit cannot be trusted:
// retries were exhausted and the persistent alert is open, or the state could
// not be read. New operations must not start (SEC 13, D-48).
var ErrAuditDegraded = errors.New("audit is degraded: new operations are denied")

// AuditSink is the only way to write an audit event (D-23). It has two write
// semantics, and the caller chooses by what the event records.
//
// Require is for the record that an operation needs before it may start
// (job creation, a claim, an approval, a change of authority). Inside
// Transactor.InTx the event joins the transaction, so a failed write rolls the
// operation back and nothing starts. There are no retries inside a transaction:
// the transaction is aborted by the failure. Outside a transaction the write
// runs on its own with the bounded retries, and exhaustion raises the alert.
//
// Record is for an event whose operation has already taken effect or been
// denied: a denial, a cancellation, a revocation. It always commits on its own
// and is retried within the bound. Whatever it returns, the effect of the
// operation stands. Exhaustion raises the persistent alert. The returned error
// is for the log and the tests, and a caller must not turn it into success or
// into a change of the decision.
//
// Gate says whether new operations may start. Use cases that start work call it
// before they begin.
type AuditSink interface {
	Require(ctx context.Context, e audit.Event) error
	Record(ctx context.Context, e audit.Event) error
	Gate(ctx context.Context) error
}
