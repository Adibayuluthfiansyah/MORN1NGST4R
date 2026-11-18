package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/audit"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

var testPolicy = audit.RetryPolicy{MaxAttempts: 3, Delays: []time.Duration{100 * time.Millisecond, 400 * time.Millisecond}, AttemptTimeout: time.Second}

type sinkProbe struct {
	mu      sync.Mutex
	inserts int
	sleeps  []time.Duration
	alerts  []alertDetail
	ctxErrs []error
	budgets []time.Duration
}

func newSink(t *testing.T, p *sinkProbe, insert func(n int) error) *AuditSink {
	t.Helper()
	s, err := NewAuditSink(nil, testPolicy, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.sleep = func(d time.Duration) { p.mu.Lock(); p.sleeps = append(p.sleeps, d); p.mu.Unlock() }
	s.insertOwn = func(ctx context.Context, _ audit.Event) error {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.inserts++
		p.ctxErrs = append(p.ctxErrs, ctx.Err())
		if dl, ok := ctx.Deadline(); ok {
			p.budgets = append(p.budgets, time.Until(dl))
		}
		return insert(p.inserts)
	}
	s.raise = func(_ context.Context, d alertDetail) error {
		p.mu.Lock()
		p.alerts = append(p.alerts, d)
		p.mu.Unlock()
		return nil
	}
	s.alertOpen = func(context.Context) (bool, error) { p.mu.Lock(); defer p.mu.Unlock(); return len(p.alerts) > 0, nil }
	return s
}

func event(t *testing.T, detail map[string]any) audit.Event {
	t.Helper()
	e, err := audit.New("0198a2b4-1111-7222-8333-444455556666", audit.Input{
		Actor: audit.Anonymous(), Action: "auth.login", Outcome: audit.Denied, Reason: "invalid_credentials",
		RequestID: "0198a2b4-3333-4222-8333-444455556666", Detail: detail,
	})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

var errDown = wrap("test", io.ErrUnexpectedEOF)

func TestTheSinkRefusesAnUnboundedPolicy(t *testing.T) {
	for _, p := range []audit.RetryPolicy{{}, {MaxAttempts: 99, AttemptTimeout: time.Second}} {
		if _, err := NewAuditSink(nil, p, nil); !errors.Is(err, audit.ErrInvalidPolicy) {
			t.Errorf("policy %+v: %v", p, err)
		}
	}
}

func TestRetriesStopAtTheBoundAndRaiseTheAlert(t *testing.T) {
	var p sinkProbe
	s := newSink(t, &p, func(int) error { return errDown })
	err := s.Record(context.Background(), event(t, nil))
	if !errors.Is(err, outbound.ErrAuditNotRecorded) || !errors.Is(err, outbound.ErrStoreUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if p.inserts != testPolicy.MaxAttempts {
		t.Fatalf("%d attempts, the bound is %d", p.inserts, testPolicy.MaxAttempts)
	}
	if len(p.sleeps) != 2 || p.sleeps[0] != 100*time.Millisecond || p.sleeps[1] != 400*time.Millisecond {
		t.Fatalf("sleeps = %v, want the delays of the policy and none after the last attempt", p.sleeps)
	}
	if len(p.alerts) != 1 || p.alerts[0].Attempts != 3 || p.alerts[0].ErrorClass != "unavailable" || p.alerts[0].Action != "auth.login" {
		t.Fatalf("alerts = %+v", p.alerts)
	}
}

func TestAPermanentFailureIsNotRetried(t *testing.T) {
	var p sinkProbe
	s := newSink(t, &p, func(int) error { return wrap("test", &pgconn.PgError{Code: "42501"}) })
	if err := s.Record(context.Background(), event(t, nil)); !errors.Is(err, outbound.ErrAuditNotRecorded) || errors.Is(err, outbound.ErrStoreUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if p.inserts != 1 || len(p.sleeps) != 0 {
		t.Fatalf("%d attempts and %d sleeps for a refused privilege", p.inserts, len(p.sleeps))
	}
	if len(p.alerts) != 1 || p.alerts[0].ErrorClass != "permission" {
		t.Fatalf("alerts = %+v", p.alerts)
	}
}

// A timeout can hit after the database committed. The retry then meets the id
// that it wrote, which is the proof that the record exists, not a failure.
func TestARetryThatMeetsItsOwnRecordIsASuccess(t *testing.T) {
	var p sinkProbe
	s := newSink(t, &p, func(n int) error {
		if n == 1 {
			return errDown
		}
		return wrap("test", &pgconn.PgError{Code: "23505"})
	})
	if err := s.Record(context.Background(), event(t, nil)); err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(p.alerts) != 0 {
		t.Fatalf("a record that exists raised an alert: %+v", p.alerts)
	}
}

// The same answer on the first attempt is not a retry: the id was used before.
func TestADuplicateIdOnTheFirstAttemptIsAFailure(t *testing.T) {
	var p sinkProbe
	s := newSink(t, &p, func(int) error { return wrap("test", &pgconn.PgError{Code: "23505"}) })
	if err := s.Record(context.Background(), event(t, nil)); !errors.Is(err, outbound.ErrAuditNotRecorded) {
		t.Fatalf("err = %v", err)
	}
	if p.inserts != 1 || len(p.alerts) != 1 {
		t.Fatalf("%d attempts, %d alerts", p.inserts, len(p.alerts))
	}
}

func TestARecoveredWriteRaisesNoAlert(t *testing.T) {
	var p sinkProbe
	s := newSink(t, &p, func(n int) error {
		if n < 3 {
			return errDown
		}
		return nil
	})
	if err := s.Record(context.Background(), event(t, nil)); err != nil {
		t.Fatal(err)
	}
	if p.inserts != 3 || len(p.alerts) != 0 {
		t.Fatalf("%d attempts, %d alerts", p.inserts, len(p.alerts))
	}
}

func TestTheAlertCarriesNeitherTheDetailNorTheErrorText(t *testing.T) {
	var p sinkProbe
	s := newSink(t, &p, func(int) error { return wrap("test", errors.New("password=hunter2 in the message")) })
	_ = s.Record(context.Background(), event(t, map[string]any{"note": "SECRET-DETAIL-VALUE"}))
	raw, _ := json.Marshal(p.alerts)
	for _, leak := range []string{"SECRET-DETAIL-VALUE", "hunter2", "password"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("the alert %s contains %q", raw, leak)
		}
	}
	var m map[string]any
	_ = json.Unmarshal(raw[1:len(raw)-1], &m)
	if len(m) != 5 {
		t.Errorf("alert fields = %v", m)
	}
}

func TestAWriteDoesNotDependOnTheRequestContext(t *testing.T) {
	var p sinkProbe
	s := newSink(t, &p, func(int) error { return nil })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Record(ctx, event(t, nil)); err != nil {
		t.Fatal(err)
	}
	if len(p.ctxErrs) != 1 || p.ctxErrs[0] != nil {
		t.Fatalf("the attempt ran with a cancelled context: %v", p.ctxErrs)
	}
	if len(p.budgets) != 1 || p.budgets[0] <= 0 || p.budgets[0] > testPolicy.AttemptTimeout {
		t.Fatalf("the attempt has no bounded deadline: %v", p.budgets)
	}
}

func TestGateDeniesWhileAnAlertIsOpenOrUnreadable(t *testing.T) {
	var p sinkProbe
	s := newSink(t, &p, func(int) error { return nil })
	if err := s.Gate(context.Background()); err != nil {
		t.Fatalf("healthy gate: %v", err)
	}
	s.alertOpen = func(context.Context) (bool, error) { return true, nil }
	if err := s.Gate(context.Background()); !errors.Is(err, outbound.ErrAuditDegraded) {
		t.Fatalf("open alert: %v", err)
	}
	s.alertOpen = func(context.Context) (bool, error) { return false, errDown }
	if err := s.Gate(context.Background()); !errors.Is(err, outbound.ErrAuditDegraded) || !errors.Is(err, outbound.ErrStoreUnavailable) {
		t.Fatalf("unreadable state: %v", err)
	}
}

// When the alert itself cannot be stored, the sink must not call audit healthy.
func TestAnAlertThatCannotBeStoredKeepsTheGateClosedUntilItIs(t *testing.T) {
	var p sinkProbe
	s := newSink(t, &p, func(int) error { return errDown })
	raiseFails := true
	var raised []alertDetail
	s.raise = func(_ context.Context, d alertDetail) error {
		if raiseFails {
			return errDown
		}
		raised = append(raised, d)
		return nil
	}
	open := false
	s.alertOpen = func(context.Context) (bool, error) { return open, nil }

	_ = s.Record(context.Background(), event(t, nil))
	if err := s.Gate(context.Background()); !errors.Is(err, outbound.ErrAuditDegraded) {
		t.Fatalf("gate with a pending alert and a database that refuses it: %v", err)
	}
	raiseFails, open = false, true
	if err := s.Gate(context.Background()); !errors.Is(err, outbound.ErrAuditDegraded) {
		t.Fatalf("gate after the alert was stored: %v", err)
	}
	if len(raised) != 1 || raised[0].Action != "auth.login" {
		t.Fatalf("stored alerts = %+v", raised)
	}
	open = false // a human cleared it
	if err := s.Gate(context.Background()); err != nil {
		t.Fatalf("gate after the alert was cleared: %v", err)
	}
	if len(raised) != 1 {
		t.Fatalf("the pending alert was raised again: %d", len(raised))
	}
}

func TestConcurrentFailuresAreSafe(t *testing.T) {
	var p sinkProbe
	s := newSink(t, &p, func(int) error { return errDown })
	s.raise = func(context.Context, alertDetail) error { return errDown }
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.Record(context.Background(), event(t, nil))
			_ = s.Gate(context.Background())
		}()
	}
	wg.Wait()
}

// The counts of phase 1 are part of the event of a restore. A key that looks
// like a secret would hide a count (the review of M5 found two).
func TestTheCountsOfARestoreAreNotRedacted(t *testing.T) {
	e, err := audit.New("0198a2b4-1111-7222-8333-444455556666", audit.Input{
		Actor: audit.System(), Action: "system.restore_recovered", Outcome: audit.Success,
		Detail: map[string]any{"counts": countsMap(outbound.ReconcileCounts{SessionsRevoked: 3, RecoveryCodesRevoked: 2, BootstrapExpired: 1})},
	})
	if err != nil {
		t.Fatal(err)
	}
	var d struct{ Counts map[string]any }
	_ = json.Unmarshal(e.DetailJSON(), &d)
	if len(d.Counts) == 0 {
		t.Fatalf("detail = %s", e.DetailJSON())
	}
	for k, v := range d.Counts {
		if _, ok := v.(float64); !ok {
			t.Errorf("count %s = %v: a count must be stored as a number", k, v)
		}
	}
}

func TestThePolicyOfTheSinkIsItsOwnCopy(t *testing.T) {
	delays := []time.Duration{100 * time.Millisecond, 400 * time.Millisecond}
	s, err := NewAuditSink(nil, audit.RetryPolicy{MaxAttempts: 3, Delays: delays, AttemptTimeout: time.Second}, nil)
	if err != nil {
		t.Fatal(err)
	}
	delays[0] = 24 * time.Hour
	if s.policy.Delays[0] != 100*time.Millisecond || s.policy.WorstCase() > 10*time.Second {
		t.Fatalf("the policy of the sink changed after validation: %v", s.policy.Delays)
	}
}

// A newer alert that arrives while the pending one is being stored must not be
// cleared with it.
func TestAPendingAlertThatArrivesDuringTheFlushIsKept(t *testing.T) {
	var p sinkProbe
	s := newSink(t, &p, func(int) error { return errDown })
	s.raise = func(context.Context, alertDetail) error { return errDown }
	_ = s.Record(context.Background(), event(t, nil))
	s.raise = func(context.Context, alertDetail) error {
		s.mu.Lock()
		s.pending = &alertDetail{Action: "auth.logout"}
		s.mu.Unlock()
		return nil
	}
	s.alertOpen = func(context.Context) (bool, error) { return false, nil }
	_ = s.Gate(context.Background())
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending == nil || s.pending.Action != "auth.logout" {
		t.Fatalf("pending = %+v", s.pending)
	}
}

func TestTheLogLineNamesTheLostEventSoItCanBeRecovered(t *testing.T) {
	var buf strings.Builder
	var p sinkProbe
	s := newSink(t, &p, func(int) error { return errDown })
	s.logger = slog.New(slog.NewTextHandler(&buf, nil))
	_ = s.Record(context.Background(), event(t, map[string]any{"note": "SECRET-DETAIL"}))
	for _, want := range []string{"auth.login", "0198a2b4-3333-4222-8333-444455556666", "attempts=3", "class=unavailable"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("log %q lacks %q", buf.String(), want)
		}
	}
	if strings.Contains(buf.String(), "SECRET-DETAIL") {
		t.Errorf("the log carries event detail: %q", buf.String())
	}
}
