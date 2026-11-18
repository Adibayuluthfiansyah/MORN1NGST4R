package db_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/persistence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/audit"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/infrastructure/ids"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/tests/integration/pgtest"
)

// M5: the audit sink against a real database. The security semantics of the
// operations that use it (start, denial, cancellation) are tested through the
// server in tests/integration/auth.

var fastPolicy = audit.RetryPolicy{MaxAttempts: 3, Delays: []time.Duration{5 * time.Millisecond, 10 * time.Millisecond}, AttemptTimeout: 2 * time.Second}

type auditEnv struct {
	c     *pgtest.Cluster
	base  pgtest.Base
	owner *pgxpool.Pool
	api   *pgxpool.Pool
	sink  *persistence.AuditSink
}

func newAuditEnv(t *testing.T) *auditEnv {
	t.Helper()
	c := pgtest.Get(t)
	c.Fresh(t)
	e := &auditEnv{c: c, base: c.SeedBase(t), owner: c.Owner(t), api: c.Pool(t, persistence.RoleAPI, 6)}
	e.sink = e.newSink(t, e.api)
	return e
}

func (e *auditEnv) newSink(t *testing.T, pool *pgxpool.Pool) *persistence.AuditSink {
	t.Helper()
	s, err := persistence.NewAuditSink(pool, fastPolicy, nil)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func (e *auditEnv) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := e.owner.Exec(t.Context(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func (e *auditEnv) scalar(t *testing.T, sql string, args ...any) string {
	t.Helper()
	var v string
	if err := e.owner.QueryRow(t.Context(), "SELECT ("+sql+")::text", args...).Scan(&v); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return v
}

func event(t *testing.T, who audit.Actor, action string, outcome audit.Outcome, in ...func(*audit.Input)) audit.Event {
	t.Helper()
	i := audit.Input{Actor: who, Action: action, Outcome: outcome, RequestID: ids.New()}
	for _, f := range in {
		f(&i)
	}
	ev, err := audit.New(ids.New(), i)
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

// failAuditInserts makes every insert into audit_events fail with the SQLSTATE
// of a database that cannot take connections (57P03), which the sink treats as
// a failure to reach the database. A sequence counts the attempts, because a
// sequence is not rolled back with the failed statement.
func (e *auditEnv) failAuditInserts(t *testing.T) {
	t.Helper()
	e.exec(t, `CREATE SEQUENCE morningstar.test_audit_attempts`)
	e.exec(t, `CREATE FUNCTION morningstar.test_fail() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER AS $$
		BEGIN PERFORM nextval('morningstar.test_audit_attempts'); RAISE EXCEPTION 'injected' USING ERRCODE = '57P03'; END $$`)
	e.exec(t, `CREATE TRIGGER test_fail BEFORE INSERT ON morningstar.audit_events FOR EACH ROW EXECUTE FUNCTION morningstar.test_fail()`)
}

func (e *auditEnv) attempts(t *testing.T) string {
	t.Helper()
	return e.scalar(t, `SELECT CASE WHEN is_called THEN last_value ELSE 0 END FROM morningstar.test_audit_attempts`)
}

func TestAuditSinkStoresEveryFieldOfAnEvent(t *testing.T) {
	e := newAuditEnv(t)
	user, _ := audit.User(e.base.UserID)
	ev := event(t, user, "assessment.created", audit.Success, func(in *audit.Input) {
		in.Reason = "created"
		in.Subject = audit.Subject{ProgramID: e.base.ProgramID, AssessmentID: e.base.AssessmentID, EntityType: "assessment", EntityID: e.base.AssessmentID}
		in.Detail = map[string]any{"name": "first"}
	})
	if err := e.sink.Require(t.Context(), ev); err != nil {
		t.Fatal(err)
	}
	anon := event(t, audit.Anonymous(), "auth.login", audit.Denied, func(in *audit.Input) { in.Reason = "invalid_credentials" })
	if err := e.sink.Record(t.Context(), anon); err != nil {
		t.Fatal(err)
	}
	got := e.scalar(t, `SELECT actor_kind || '|' || actor_user_id || '|' || action || '|' || outcome || '|' || program_id || '|' || assessment_id || '|' || entity_type || '|' || (detail->>'reason') || '|' || (detail->>'name') FROM morningstar.audit_events WHERE id = $1`, ev.ID())
	want := strings.Join([]string{"user", e.base.UserID, "assessment.created", "success", e.base.ProgramID, e.base.AssessmentID, "assessment", "created", "first"}, "|")
	if got != want {
		t.Fatalf("stored row = %s, want %s", got, want)
	}
	if got := e.scalar(t, `SELECT actor_kind || '|' || coalesce(actor_user_id::text, '-') || '|' || request_id FROM morningstar.audit_events WHERE id = $1`, anon.ID()); got != "anonymous|-|"+anon.RequestID() {
		t.Fatalf("anonymous row = %s", got)
	}
	if got := e.scalar(t, `SELECT count(*) FROM morningstar.audit_events WHERE actor_kind = 'system'`); got != "0" {
		t.Fatalf("%s system events: an anonymous denial must not be one", got)
	}
}

// The runtime role that the sink uses cannot change what it wrote (FR-AUDIT-001
// AC3). M3 holds the full grant matrix. This is the check on the sink's own
// connection and on the row it just wrote.
func TestTheSinksRoleCannotChangeOrRemoveAnEvent(t *testing.T) {
	e := newAuditEnv(t)
	ev := event(t, audit.System(), "system.first_run", audit.Success)
	if err := e.sink.Record(t.Context(), ev); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		`UPDATE morningstar.audit_events SET outcome = 'denied'`,
		`UPDATE morningstar.audit_events SET detail = '{}'`,
		`DELETE FROM morningstar.audit_events`,
		`TRUNCATE morningstar.audit_events`,
	} {
		if _, err := e.api.Exec(t.Context(), sql); err == nil {
			t.Errorf("the runtime role ran %q", sql)
		}
	}
	if got := e.scalar(t, `SELECT outcome FROM morningstar.audit_events WHERE id = $1`, ev.ID()); got != "success" {
		t.Fatalf("outcome = %s", got)
	}
}

func TestRequireJoinsTheTransactionOfTheCaller(t *testing.T) {
	e := newAuditEnv(t)
	tx := persistence.NewTransactor(e.api)
	user, _ := audit.User(e.base.UserID)

	rolledBack := event(t, user, "program.updated", audit.Success)
	boom := errors.New("the operation failed")
	err := tx.InTx(t.Context(), func(ctx context.Context) error {
		if err := e.sink.Require(ctx, rolledBack); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if got := e.scalar(t, `SELECT count(*) FROM morningstar.audit_events WHERE id = $1`, rolledBack.ID()); got != "0" {
		t.Fatal("a required record outlived the rollback of its operation")
	}

	committed := event(t, user, "program.updated", audit.Success)
	if err := tx.InTx(t.Context(), func(ctx context.Context) error { return e.sink.Require(ctx, committed) }); err != nil {
		t.Fatal(err)
	}
	if got := e.scalar(t, `SELECT count(*) FROM morningstar.audit_events WHERE id = $1`, committed.ID()); got != "1" {
		t.Fatal("the record of a committed operation is missing")
	}
}

// An operation must not start when its record cannot be written: the write that
// fails inside the transaction takes the change with it.
func TestAFailedRequiredWriteRollsTheOperationBack(t *testing.T) {
	e := newAuditEnv(t)
	e.failAuditInserts(t)
	tx := persistence.NewTransactor(e.api)
	user, _ := audit.User(e.base.UserID)
	err := tx.InTx(t.Context(), func(ctx context.Context) error {
		if _, err := persistence.Querier(ctx, e.api).Exec(ctx, `INSERT INTO morningstar.login_attempts (id, kind, source_key, succeeded) VALUES (uuidv7(), 'login', 'operation', true)`); err != nil {
			return err
		}
		return e.sink.Require(ctx, event(t, user, "job.created", audit.Success))
	})
	if !errors.Is(err, outbound.ErrAuditNotRecorded) {
		t.Fatalf("err = %v, want ErrAuditNotRecorded", err)
	}
	if got := e.scalar(t, `SELECT count(*) FROM morningstar.login_attempts WHERE source_key = 'operation'`); got != "0" {
		t.Fatal("the operation survived the failure of its audit record")
	}
	if got := e.attempts(t); got != "1" {
		t.Fatalf("%s attempts inside a transaction, want one (there are no retries inside a transaction)", got)
	}
	if got := e.scalar(t, `SELECT count(*) FROM morningstar.system_alerts`); got != "0" {
		t.Fatal("a rolled back operation raised an alert: nothing is missing, the operation did not happen")
	}
}

// A denial or a cancellation is recorded on its own and survives the rollback
// of the transaction that carried the request.
func TestRecordSurvivesTheRollbackOfTheCallersTransaction(t *testing.T) {
	e := newAuditEnv(t)
	tx := persistence.NewTransactor(e.api)
	denial := event(t, audit.Anonymous(), "policy.decision_denied", audit.Denied)
	_ = tx.InTx(t.Context(), func(ctx context.Context) error {
		if err := e.sink.Record(ctx, denial); err != nil {
			t.Fatal(err)
		}
		return errors.New("the request failed")
	})
	if got := e.scalar(t, `SELECT count(*) FROM morningstar.audit_events WHERE id = $1`, denial.ID()); got != "1" {
		t.Fatal("the denial was erased with the transaction")
	}
}

func TestConcurrentAppendsAreAllStored(t *testing.T) {
	e := newAuditEnv(t)
	user, _ := audit.User(e.base.UserID)
	var wg sync.WaitGroup
	errs := make(chan error, 80)
	for i := 0; i < 40; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			errs <- e.sink.Record(t.Context(), event(t, audit.Anonymous(), "auth.login", audit.Denied))
		}()
		go func() {
			defer wg.Done()
			errs <- e.sink.Require(t.Context(), event(t, user, "auth.logout", audit.Success))
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := e.scalar(t, `SELECT count(*) || '/' || count(DISTINCT seq) || '/' || count(DISTINCT id) FROM morningstar.audit_events`); got != "80/80/80" {
		t.Fatalf("rows/seq/id = %s", got)
	}
}

// Exhausting the retries raises a persistent alert, once, and the gate denies.
func TestRetryExhaustionRaisesOnePersistentAlertAndTheGateDenies(t *testing.T) {
	e := newAuditEnv(t)
	if err := e.sink.Gate(t.Context()); err != nil {
		t.Fatalf("gate of a healthy system: %v", err)
	}
	e.failAuditInserts(t)
	denial := event(t, audit.Anonymous(), "auth.login", audit.Denied, func(in *audit.Input) {
		in.Reason = "invalid_credentials"
		in.Detail = map[string]any{"note": "SECRET-NOTE"}
	})
	start := time.Now()
	err := e.sink.Record(t.Context(), denial)
	if !errors.Is(err, outbound.ErrAuditNotRecorded) || !errors.Is(err, outbound.ErrStoreUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if got := e.attempts(t); got != "3" {
		t.Fatalf("%s attempts, the policy allows 3", got)
	}
	if time.Since(start) < 15*time.Millisecond {
		t.Fatal("the retries did not wait for their delays")
	}
	alert := e.scalar(t, `SELECT detail::text FROM morningstar.system_alerts WHERE kind = 'audit_retry_exhausted' AND cleared_at IS NULL`)
	for _, want := range []string{`"action": "auth.login"`, `"attempts": 3`, `"errorClass": "unavailable"`, denial.RequestID()} {
		if !strings.Contains(alert, want) {
			t.Errorf("alert %s lacks %s", alert, want)
		}
	}
	if strings.Contains(alert, "SECRET-NOTE") || strings.Contains(alert, "injected") {
		t.Errorf("alert %s carries event detail or error text", alert)
	}
	if err := e.sink.Gate(t.Context()); !errors.Is(err, outbound.ErrAuditDegraded) {
		t.Fatalf("gate with an open alert = %v", err)
	}
	// A second exhaustion does not add a second open alert (T-52).
	_ = e.sink.Record(t.Context(), event(t, audit.Anonymous(), "auth.login", audit.Denied))
	if got := e.scalar(t, `SELECT count(*) FROM morningstar.system_alerts WHERE kind = 'audit_retry_exhausted' AND cleared_at IS NULL`); got != "1" {
		t.Fatalf("%s open alerts", got)
	}
	// Once the cause is gone and a human cleared the alert, nothing of the
	// second failure is left behind to open it again.
	e.exec(t, `DROP TRIGGER test_fail ON morningstar.audit_events`)
	e.exec(t, `UPDATE morningstar.system_alerts SET cleared_at = now(), cleared_by = $1 WHERE kind = 'audit_retry_exhausted'`, e.base.UserID)
	if err := e.sink.Gate(t.Context()); err != nil {
		t.Fatalf("gate after the alert was cleared: %v", err)
	}
	if got := e.scalar(t, `SELECT count(*) FROM morningstar.system_alerts WHERE kind = 'audit_retry_exhausted' AND cleared_at IS NULL`); got != "0" {
		t.Fatalf("%s open alerts after the clearing", got)
	}
}

// The alert is a row, so it survives a restart, and only a human clears it.
func TestTheAlertSurvivesARestartUntilAHumanClearsIt(t *testing.T) {
	e := newAuditEnv(t)
	e.failAuditInserts(t)
	_ = e.sink.Record(t.Context(), event(t, audit.Anonymous(), "auth.login", audit.Denied))
	e.api.Close()

	restarted := e.newSink(t, e.c.Pool(t, persistence.RoleAPI, 4))
	if err := restarted.Gate(t.Context()); !errors.Is(err, outbound.ErrAuditDegraded) {
		t.Fatalf("a restarted sink forgot the alert: %v", err)
	}
	e.exec(t, `DROP TRIGGER test_fail ON morningstar.audit_events`)
	if err := restarted.Gate(t.Context()); !errors.Is(err, outbound.ErrAuditDegraded) {
		t.Fatalf("a working database cleared the alert by itself: %v", err)
	}
	e.exec(t, `UPDATE morningstar.system_alerts SET cleared_at = now(), cleared_by = $1 WHERE kind = 'audit_retry_exhausted'`, e.base.UserID)
	if err := restarted.Gate(t.Context()); err != nil {
		t.Fatalf("gate after the alert was cleared: %v", err)
	}
}

// When the alert cannot be written either, audit is not called healthy.
func TestAnAlertThatCannotBeWrittenKeepsTheGateClosed(t *testing.T) {
	e := newAuditEnv(t)
	e.failAuditInserts(t)
	e.exec(t, `CREATE FUNCTION morningstar.test_fail_alert() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER AS $$
		BEGIN RAISE EXCEPTION 'injected' USING ERRCODE = '57P03'; END $$`)
	e.exec(t, `CREATE TRIGGER test_fail_alert BEFORE INSERT ON morningstar.system_alerts FOR EACH ROW EXECUTE FUNCTION morningstar.test_fail_alert()`)

	err := e.sink.Record(t.Context(), event(t, audit.Anonymous(), "auth.login", audit.Denied))
	if !errors.Is(err, outbound.ErrAuditNotRecorded) {
		t.Fatalf("err = %v", err)
	}
	if got := e.scalar(t, `SELECT count(*) FROM morningstar.system_alerts`); got != "0" {
		t.Fatal("the alert was written although its insert is refused")
	}
	if err := e.sink.Gate(t.Context()); !errors.Is(err, outbound.ErrAuditDegraded) {
		t.Fatalf("gate with an alert that is not stored = %v", err)
	}
	e.exec(t, `DROP TRIGGER test_fail_alert ON morningstar.system_alerts`)
	if err := e.sink.Gate(t.Context()); !errors.Is(err, outbound.ErrAuditDegraded) {
		t.Fatalf("gate after the alert could be stored = %v", err)
	}
	if got := e.scalar(t, `SELECT count(*) FROM morningstar.system_alerts WHERE kind = 'audit_retry_exhausted' AND cleared_at IS NULL`); got != "1" {
		t.Fatal("the pending alert was not stored once the database took it")
	}
}

// A refused privilege gives the same answer every time, so it is not retried,
// but it still raises the alert.
func TestAPermanentFailureIsNotRetriedAndStillAlerts(t *testing.T) {
	e := newAuditEnv(t)
	e.exec(t, `REVOKE INSERT ON morningstar.audit_events FROM ms_api`)
	start := time.Now()
	err := e.sink.Record(t.Context(), event(t, audit.Anonymous(), "auth.login", audit.Denied))
	if !errors.Is(err, outbound.ErrAuditNotRecorded) || errors.Is(err, outbound.ErrStoreUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("a refused privilege was retried")
	}
	if got := e.scalar(t, `SELECT detail->>'errorClass' || '/' || (detail->>'attempts') FROM morningstar.system_alerts WHERE kind = 'audit_retry_exhausted'`); got != "permission/1" {
		t.Fatalf("alert = %s", got)
	}
}

// A detail that audit.New accepts must be accepted by the database, whatever its
// shape: a refusal by the check on jsonb::text would raise the alert that closes
// the gate (the review of M5 found the limit too weak). Each shape grows until
// the domain refuses it, and the last accepted one is written.
func TestTheLargestDetailThatTheDomainAcceptsIsAcceptedByTheDatabase(t *testing.T) {
	e := newAuditEnv(t)
	shapes := map[string]func(n int) map[string]any{
		"single digit numbers in arrays": func(n int) map[string]any {
			d := map[string]any{}
			for i := 0; i < n; i++ {
				outer, mid := "a"+string(rune('a'+i/576)), "b"+string(rune('a'+(i/24)%24))
				m, _ := d[outer].(map[string]any)
				if m == nil {
					m = map[string]any{}
					d[outer] = m
				}
				arr, _ := m[mid].([]any)
				m[mid] = append(arr, 1)
			}
			return d
		},
		"short keys with booleans": func(n int) map[string]any {
			d := map[string]any{}
			for i := 0; i < n; i++ {
				outer, mid := "a"+string(rune('a'+i/24)), "b"+string(rune('a'+i%24))
				m, _ := d[outer].(map[string]any)
				if m == nil {
					m = map[string]any{}
					d[outer] = m
				}
				m[mid] = true
			}
			return d
		},
		"long strings": func(n int) map[string]any {
			d := map[string]any{}
			for i := 0; i < n; i++ {
				d["k"+string(rune('a'+i%24))+string(rune('a'+i/24))] = strings.Repeat("y", 200)
			}
			return d
		},
	}
	for name, build := range shapes {
		last := -1
		for n := 1; n < 20000; n++ {
			if _, err := audit.New(ids.New(), audit.Input{Actor: audit.System(), Action: "system.first_run", Outcome: audit.Success, Detail: build(n)}); err != nil {
				break
			}
			last = n
			if name == "long strings" && n == 24 {
				break
			}
		}
		if last < 1 {
			t.Fatalf("%s: nothing was accepted", name)
		}
		ev, err := audit.New(ids.New(), audit.Input{Actor: audit.System(), Action: "system.first_run", Outcome: audit.Success, Detail: build(last)})
		if err != nil {
			t.Fatal(err)
		}
		if err := e.sink.Record(t.Context(), ev); err != nil {
			t.Errorf("%s (%d entries, %d bytes): the database refused what the domain accepted: %v", name, last, len(ev.DetailJSON()), err)
		}
	}
	if got := e.scalar(t, `SELECT count(*) FROM morningstar.system_alerts`); got != "0" {
		t.Fatalf("%s alerts", got)
	}
}
