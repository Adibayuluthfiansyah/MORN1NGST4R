package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/persistence/sqlcgen"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/audit"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/infrastructure/ids"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

// insertAuditEvent is the one statement that writes an audit event. The sink
// uses it, and so does the recovery repository for the events of its own
// transactions, so there is no second path (TD-M4-03). It runs on q, which is
// the transaction of ctx when there is one.
func insertAuditEvent(ctx context.Context, q DBTX, e audit.Event) error {
	id, err := uuidOf(e.ID())
	if err != nil {
		return err
	}
	a, s := e.Actor(), e.Subject()
	p := sqlcgen.InsertAuditEventParams{Column1: id, ActorKind: string(a.Kind()), Action: string(e.Action()), Outcome: string(e.Outcome()), Column5: e.DetailJSON()}
	if a.Ref() != "" {
		r := a.Ref()
		p.ActorRef = &r
	}
	if s.EntityType != "" {
		t := s.EntityType
		p.EntityType = &t
	}
	for _, f := range []struct {
		in  string
		out *pgtype.UUID
	}{
		{a.UserID(), &p.ActorUserID}, {e.RequestID(), &p.RequestID}, {s.ProgramID, &p.ProgramID}, {s.AssessmentID, &p.AssessmentID},
		{s.JobID, &p.JobID}, {s.EntityID, &p.EntityID}, {s.ScopeVersionID, &p.ScopeVersionID}, {s.DecisionID, &p.DecisionID},
		{s.ApprovalID, &p.ApprovalID},
	} {
		if *f.out, err = uuidOrNull(f.in); err != nil {
			return err
		}
	}
	return wrap("write the audit event", sqlcgen.New(q).InsertAuditEvent(ctx, p))
}

// appendEvent builds and writes an event of the transaction that q belongs to.
// The recovery repository uses it: its events are part of its own transactions
// and roll back with them.
func appendEvent(ctx context.Context, q DBTX, in audit.Input) error {
	e, err := audit.New(ids.New(), in)
	if err != nil {
		return err
	}
	return insertAuditEvent(ctx, q, e)
}

// insertSystemAlert raises an alert of a kind other than audit_retry_exhausted
// inside a transaction (restore_recovered). The audit alert has its own
// statement because it is raised at most once while open.
func insertSystemAlert(ctx context.Context, q DBTX, kind, detail string) error {
	id, err := uuidOf(ids.New())
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `INSERT INTO morningstar.system_alerts (id, kind, detail) VALUES ($1, $2, $3::jsonb)`, id, kind, detail)
	return wrap("raise a system alert", err)
}

// Alerts implements outbound.SystemAlerts. Like every write of system_alerts, it
// lives in this file (the architecture test keeps the path single).
type Alerts struct{ base }

var _ outbound.SystemAlerts = Alerts{}

func (d *DB) SystemAlerts() outbound.SystemAlerts { return Alerts{base{d.pool}} }

// NewAlerts returns the adapter over pool.
func NewAlerts(pool *pgxpool.Pool) outbound.SystemAlerts { return Alerts{base{pool}} }

// Raise raises an alert of a kind that a person has to clear. The detail is
// built by the caller from codes and ids, never from the text of an error.
func (a Alerts) Raise(ctx context.Context, kind string, detail map[string]any) error {
	b, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	return insertSystemAlert(ctx, Querier(ctx, a.pool), kind, string(b))
}

// countsMap turns the counts of phase 1 into a detail object.
func countsMap(c outbound.ReconcileCounts) map[string]any {
	var m map[string]any
	_ = json.Unmarshal([]byte(mustJSON(c)), &m)
	return m
}

// alertDetail is what a persistent alert says. It names the lost event by its
// action, outcome and request id, and the cause by a class. It never carries
// the detail of the event or the text of the error.
type alertDetail struct {
	Action     string `json:"action"`
	Outcome    string `json:"outcome"`
	RequestID  string `json:"requestId,omitempty"`
	Attempts   int    `json:"attempts"`
	ErrorClass string `json:"errorClass"`
}

func errorClass(err error) string {
	var pe *pgconn.PgError
	switch {
	case errors.Is(err, outbound.ErrStoreUnavailable) || unavailable(err):
		return "unavailable"
	case errors.As(err, &pe) && pe.Code == "42501":
		return "permission"
	case errors.As(err, &pe) && len(pe.Code) >= 2 && pe.Code[:2] == "23":
		return "constraint"
	}
	return "other"
}

// AuditSink implements outbound.AuditSink on PostgreSQL.
type AuditSink struct {
	pool   *pgxpool.Pool
	policy audit.RetryPolicy
	logger *slog.Logger

	// The seams below are replaced only by the tests of this package.
	sleep     func(time.Duration)
	insertOwn func(ctx context.Context, e audit.Event) error
	raise     func(ctx context.Context, d alertDetail) error
	alertOpen func(ctx context.Context) (bool, error)

	mu      sync.Mutex
	pending *alertDetail
}

var _ outbound.AuditSink = (*AuditSink)(nil)

// NewAuditSink returns the sink. The policy is required: a zero or unbounded
// policy is refused, so the retry bound is always an explicit value.
func NewAuditSink(pool *pgxpool.Pool, policy audit.RetryPolicy, logger *slog.Logger) (*AuditSink, error) {
	if err := policy.Validate(); err != nil {
		return nil, fmt.Errorf("persistence: %w", err)
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	policy.Delays = append([]time.Duration(nil), policy.Delays...)
	s := &AuditSink{pool: pool, policy: policy, logger: logger, sleep: time.Sleep}
	s.insertOwn = func(ctx context.Context, e audit.Event) error { return insertAuditEvent(ctx, pool, e) }
	s.raise = func(ctx context.Context, d alertDetail) error {
		id, err := uuidOf(ids.New())
		if err != nil {
			return err
		}
		raw, err := json.Marshal(d)
		if err != nil {
			return err
		}
		_, err = sqlcgen.New(pool).RaiseAuditRetryAlert(ctx, sqlcgen.RaiseAuditRetryAlertParams{Column1: id, Column2: raw})
		return wrap("raise the audit alert", err)
	}
	s.alertOpen = func(ctx context.Context) (bool, error) {
		open, err := sqlcgen.New(pool).AuditAlertIsOpen(ctx)
		return open, wrap("read the audit alert", err)
	}
	return s, nil
}

// AuditSink returns the sink of this database.
func (d *DB) AuditSink(policy audit.RetryPolicy, logger *slog.Logger) (*AuditSink, error) {
	return NewAuditSink(d.pool, policy, logger)
}

func notRecorded(err error) error { return fmt.Errorf("%w: %w", outbound.ErrAuditNotRecorded, err) }

// Require joins the transaction of ctx. Without one it writes on its own.
func (s *AuditSink) Require(ctx context.Context, e audit.Event) error {
	if inTx(ctx) {
		if err := insertAuditEvent(ctx, Querier(ctx, s.pool), e); err != nil {
			return notRecorded(err)
		}
		return nil
	}
	return s.write(ctx, e)
}

// Record never joins a transaction, so a rollback cannot erase it: every write
// of this path goes to the pool, whatever ctx carries.
func (s *AuditSink) Record(ctx context.Context, e audit.Event) error {
	return s.write(ctx, e)
}

func (s *AuditSink) write(ctx context.Context, e audit.Event) error {
	attempts, err := s.attempts(ctx, e)
	if err == nil {
		return nil
	}
	s.exhausted(ctx, e, attempts, err)
	return notRecorded(err)
}

// attempts writes e within the policy. Only a failure to reach the database is
// retried: a refused privilege or a broken constraint gives the same answer
// again. Each attempt has its own timeout and does not depend on the request
// context, so a client that left does not cancel the record of its denial.
func (s *AuditSink) attempts(ctx context.Context, e audit.Event) (int, error) {
	for n := 1; ; n++ {
		actx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.policy.AttemptTimeout)
		err := s.insertOwn(actx, e)
		cancel()
		if err == nil {
			return n, nil
		}
		// A timeout can strike after the commit. On a retry, the id that the
		// first attempt wrote is the proof that the record exists.
		if n > 1 && errors.Is(err, outbound.ErrUniqueViolation) {
			return n, nil
		}
		if n >= s.policy.MaxAttempts || !errors.Is(err, outbound.ErrStoreUnavailable) {
			return n, err
		}
		s.sleep(s.policy.Delay(n))
	}
}

// exhausted raises the persistent alert. When that fails too, the alert is kept
// in memory and Gate keeps denying until it is stored.
func (s *AuditSink) exhausted(ctx context.Context, e audit.Event, attempts int, cause error) {
	d := alertDetail{Action: string(e.Action()), Outcome: string(e.Outcome()), RequestID: e.RequestID(), Attempts: attempts, ErrorClass: errorClass(cause)}
	// The alert keeps only the first lost event of an outage, so the log names
	// every one. The request id is made by the server and holds no secret.
	s.logger.Error("audit: a record was not written", "action", d.Action, "requestId", d.RequestID, "attempts", attempts, "class", d.ErrorClass)
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.policy.AttemptTimeout)
	defer cancel()
	if err := s.raise(rctx, d); err != nil {
		s.mu.Lock()
		if s.pending == nil {
			s.pending = &d
		}
		s.mu.Unlock()
		s.logger.Error("audit: the persistent alert could not be stored, new operations stay denied", "class", errorClass(err))
	}
}

// Gate denies while an alert is open, while a raised alert is still waiting to
// be stored, and whenever the state cannot be read.
func (s *AuditSink) Gate(ctx context.Context) error {
	gctx, cancel := context.WithTimeout(ctx, s.policy.AttemptTimeout)
	defer cancel()
	s.mu.Lock()
	p := s.pending
	s.mu.Unlock()
	if p != nil {
		if err := s.raise(gctx, *p); err != nil {
			return fmt.Errorf("%w: %w", outbound.ErrAuditDegraded, err)
		}
		s.mu.Lock()
		if s.pending == p {
			s.pending = nil
		}
		s.mu.Unlock()
	}
	open, err := s.alertOpen(gctx)
	if err != nil {
		return fmt.Errorf("%w: %w", outbound.ErrAuditDegraded, err)
	}
	if open {
		return outbound.ErrAuditDegraded
	}
	return nil
}
