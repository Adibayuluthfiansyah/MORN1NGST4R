package auth_test

import (
	"strings"
	"testing"
	"time"
)

// M5 (FR-AUDIT-001, SEC 13, D-48). Three behaviors, each tested on its own:
// an operation does not start when its record cannot be written, a denial still
// applies, and a revocation still takes effect.

// breakAuditWrites makes every insert into audit_events fail like a database
// that cannot take connections (SQLSTATE 57P03), which the sink retries.
func breakAuditWrites(a *app) {
	a.t.Helper()
	a.exec(`CREATE FUNCTION morningstar.test_fail() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER AS $$
		BEGIN RAISE EXCEPTION 'injected' USING ERRCODE = '57P03'; END $$`)
	a.exec(`CREATE TRIGGER test_fail BEFORE INSERT ON morningstar.audit_events FOR EACH ROW EXECUTE FUNCTION morningstar.test_fail()`)
}

func openAlerts(a *app) string {
	a.t.Helper()
	return a.scalar(`SELECT count(*) FROM morningstar.system_alerts WHERE kind = 'audit_retry_exhausted' AND cleared_at IS NULL`)
}

func TestAnOperationDoesNotStartWhenItsRecordCannotBeWritten(t *testing.T) {
	for name, brk := range map[string]struct {
		break_ func(*app)
		status int
		code   string
	}{
		"the database cannot be reached": {breakAuditWrites, 503, "database_unavailable"},
		"the privilege was removed":      {func(a *app) { a.exec(`REVOKE INSERT ON morningstar.audit_events FROM ms_api`) }, 500, "internal_error"},
	} {
		t.Run(name, func(t *testing.T) {
			_, a, b, _ := fresh(t)
			sessions := a.scalar(`SELECT count(*) FROM morningstar.sessions`)
			grants := a.scalar(`SELECT count(*) FROM morningstar.reauth_grants`)
			brk.break_(a)

			nb := newBrowser(a)
			r := nb.login(username, goodPass)
			if r.status != brk.status || r.code() != brk.code {
				t.Fatalf("login = %d %s", r.status, r.body)
			}
			if nb.cookies["ms_session"] != "" {
				t.Error("a session cookie was issued for an operation that did not start")
			}
			if g := b.reauth(goodPass, "verification", map[string]any{"x": "y"}); g.status != brk.status {
				t.Errorf("reauthentication = %d %s", g.status, g.body)
			}
			if got := a.scalar(`SELECT count(*) FROM morningstar.sessions`); got != sessions {
				t.Errorf("%s sessions, was %s: the operation left a trace", got, sessions)
			}
			if got := a.scalar(`SELECT count(*) FROM morningstar.reauth_grants`); got != grants {
				t.Errorf("%s grants, was %s: the operation left a trace", got, grants)
			}
			if got := a.scalar(`SELECT count(*) FROM morningstar.audit_events WHERE action = 'auth.login' AND outcome = 'success'`); got != "0" {
				t.Errorf("%s successful login records, want none", got)
			}
		})
	}
}

func TestADenialStillAppliesWhenItsRecordCannotBeWritten(t *testing.T) {
	_, a, _, _ := fresh(t)
	breakAuditWrites(a)

	nb := newBrowser(a)
	start := time.Now()
	r := nb.login(username, wrongPass)
	if r.status != 401 || r.code() != "invalid_credentials" {
		t.Fatalf("a wrong password = %d %s, the denial must stand whatever audit does", r.status, r.body)
	}
	if nb.cookies["ms_session"] != "" {
		t.Error("a session was issued for a denied login")
	}
	if took := time.Since(start); took > 8*time.Second {
		t.Errorf("the denial waited %v for its record: the retries are not bounded", took)
	}
	if openAlerts(a) != "1" {
		t.Fatal("a denial that could not be recorded raised no persistent alert")
	}
	if got := a.scalar(`SELECT detail->>'action' || '/' || (detail->>'attempts') FROM morningstar.system_alerts WHERE kind = 'audit_retry_exhausted'`); got != "auth.login/3" {
		t.Errorf("alert = %s", got)
	}
	// Five denials more: still denials, and still one open alert.
	for i := 0; i < 5; i++ {
		if r := newBrowser(a).login(username, wrongPass); r.status != 401 && r.status != 429 {
			t.Fatalf("denial %d = %d %s", i+2, r.status, r.body)
		}
	}
	if openAlerts(a) != "1" {
		t.Errorf("%s open alerts, want one", openAlerts(a))
	}
}

func TestARevocationStillTakesEffectWhenItsRecordCannotBeWritten(t *testing.T) {
	_, a, first, _ := fresh(t)
	second := newBrowser(a)
	if r := second.login(username, goodPass); r.status != 200 {
		t.Fatalf("login = %d %s", r.status, r.body)
	}
	firstID := a.scalar(`SELECT id FROM morningstar.sessions ORDER BY created_at LIMIT 1`)
	secondID := a.scalar(`SELECT id FROM morningstar.sessions ORDER BY created_at DESC LIMIT 1`)
	breakAuditWrites(a)

	if r := second.post("/api/v1/sessions/"+firstID+"/revoke", nil); r.status != 200 {
		t.Fatalf("revoking another session = %d %s", r.status, r.body)
	}
	if got := a.scalar(`SELECT revoked_at IS NOT NULL FROM morningstar.sessions WHERE id = $1`, firstID); got != "true" {
		t.Fatal("the revocation did not take effect")
	}
	if r := first.do("GET", "/api/v1/sessions", nil); r.status != 401 {
		t.Errorf("the revoked session still works: %d", r.status)
	}
	if r := second.post("/api/v1/session/logout", nil); r.status != 204 {
		t.Fatalf("logout = %d %s", r.status, r.body)
	}
	if got := a.scalar(`SELECT revoked_at IS NOT NULL FROM morningstar.sessions WHERE id = $1`, secondID); got != "true" {
		t.Fatal("the logout did not revoke the session")
	}
	if second.cookies["ms_session"] != "" {
		t.Error("the cookie of the logged out session was not cleared")
	}
	if openAlerts(a) != "1" {
		t.Error("revocations that could not be recorded raised no persistent alert")
	}
}

// TD-M4-03: an unauthenticated caller is not a system actor.
func TestAnonymousDenialsAreNotSystemEvents(t *testing.T) {
	_, a, b, _ := fresh(t)
	newBrowser(a).login(username, wrongPass)
	newBrowser(a).reset("AAAA-AAAA-AAAA-AAAA-AAAA-AAAA-AAAA-AAAA", otherPass)
	if r := b.reauth(wrongPass, "verification", map[string]any{"x": "y"}); r.status != 401 {
		t.Fatalf("reauthentication = %d", r.status)
	}
	if got := a.scalar(`SELECT string_agg(actor_kind || ':' || action, ',' ORDER BY seq) FROM morningstar.audit_events WHERE outcome = 'denied'`); got != "anonymous:auth.login,anonymous:auth.recovery_reset,user:auth.reauthenticated" {
		t.Errorf("denied events = %s", got)
	}
	if got := a.scalar(`SELECT count(*) FROM morningstar.audit_events WHERE actor_kind = 'system' AND actor_ref = 'anonymous'`); got != "0" {
		t.Errorf("%s anonymous events recorded as system events", got)
	}
	if got := a.scalar(`SELECT count(*) FROM morningstar.audit_events WHERE actor_kind = 'user' AND actor_user_id IS NULL`); got != "0" {
		t.Error("a user event without a user")
	}
}

// TD-M4-03: the correlation id of the audit trail is made by the server.
func TestACallerCannotChooseTheCorrelationId(t *testing.T) {
	_, a, _, _ := fresh(t)
	const chosen = "11111111-2222-4333-8444-555555555555"
	nb := newBrowser(a)
	nb.state()
	r := nb.do("POST", "/api/v1/session/login", map[string]any{"username": username, "password": wrongPass}, header("X-Request-Id", chosen))
	if r.status != 401 {
		t.Fatalf("login = %d", r.status)
	}
	id := r.header.Get("X-Request-Id")
	if id == "" || strings.EqualFold(id, chosen) {
		t.Fatalf("the response carries %q after the caller sent %q", id, chosen)
	}
	if r.str("error", "requestId") != id {
		t.Errorf("the error body names %q, the header %q", r.str("error", "requestId"), id)
	}
	if got := a.scalar(`SELECT count(*) FROM morningstar.audit_events WHERE request_id = $1`, chosen); got != "0" {
		t.Error("the caller's id reached the audit trail")
	}
	if got := a.scalar(`SELECT count(*) FROM morningstar.audit_events WHERE request_id = $1 AND action = 'auth.login' AND outcome = 'denied'`, id); got != "1" {
		t.Error("the denial is not recorded under the id that the response carries")
	}
}
