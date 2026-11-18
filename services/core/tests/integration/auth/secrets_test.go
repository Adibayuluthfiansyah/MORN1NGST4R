package auth_test

import (
	"strings"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/tests/integration/pgtest"
)

// security.md 6 and 13: a password, a token, a code or a secret never reaches a
// log line, an audit event or an error response.
func TestNoSecretReachesLogsAuditOrErrorResponses(t *testing.T) {
	c := pgtest.Get(t)
	c.Fresh(t)
	a := start(t, c, options{})
	secret := a.secret()
	b := newBrowser(a)

	var errorBodies []string
	note := func(r *reply) *reply {
		if r.status >= 400 {
			errorBodies = append(errorBodies, string(r.body))
		}
		return r
	}
	note(b.setup("wrong-"+secret, username, goodPass))
	r := note(b.setup(secret, username, goodPass))
	code := r.str("recoveryCode")
	session := b.cookies["ms_session"]
	csrf := b.csrf
	note(newBrowser(a).login(username, wrongPass))
	note(newBrowser(a).login("unknown-"+username, wrongPass))
	note(b.reauth(wrongPass, "verification", map[string]any{"x": "y"}))
	note(newBrowser(a).reset("AAAA-AAAA-AAAA-AAAA-AAAA-AAAA-AAAA-AAAA", otherPass))
	note(b.post("/api/v1/account/password", map[string]any{"currentPassword": wrongPass, "newPassword": otherPass, "reauthGrantId": "1"}))
	note(b.post("/api/v1/session/login", `{"username":"`+username+`","password":"`+goodPass+`","bad":1}`))

	sensitive := map[string]string{
		"password": goodPass, "wrong password": wrongPass, "new password": otherPass, "bootstrap secret": secret,
		"recovery code": code, "canonical recovery code": canonical(code), "session token": session, "csrf token": csrf,
	}
	logs := a.logs.String()
	audit := a.scalar(`SELECT coalesce(string_agg(detail::text || coalesce(actor_ref, '') || action, ' '), '') FROM morningstar.audit_events`)
	for name, v := range sensitive {
		if strings.Contains(logs, v) {
			t.Errorf("the %s is in the log", name)
		}
		if strings.Contains(audit, v) {
			t.Errorf("the %s is in an audit event", name)
		}
		for _, body := range errorBodies {
			if strings.Contains(body, v) {
				t.Errorf("the %s is in an error response: %s", name, body)
			}
		}
	}
	for _, leak := range []string{"argon2id", "phc", "token_hash", "SELECT ", "pgx", "/dev/shm", a.runtime, c.SocketDir} {
		if strings.Contains(logs, leak) {
			t.Errorf("the log contains %q", leak)
		}
		for _, body := range errorBodies {
			if strings.Contains(body, leak) {
				t.Errorf("an error response contains %q: %s", leak, body)
			}
		}
	}
	if !strings.Contains(a.scalar(`SELECT count(*) FROM morningstar.audit_events`), "") || a.scalar(`SELECT count(*) FROM morningstar.audit_events WHERE actor_kind = 'user' AND actor_user_id IS NULL`) != "0" {
		t.Error("an audit event of a user has no user")
	}
	// Failed logins are audited without the name that was typed.
	if a.scalar(`SELECT count(*) FROM morningstar.audit_events WHERE outcome = 'denied' AND (detail::text LIKE '%unknown-%' OR detail::text LIKE '%' || $1 || '%')`, username) != "0" {
		t.Error("a denied event records the attempted user name")
	}
}

// The recovery code that a reconciliation issues, and the two passwords of the
// request, never reach a log line, an audit event or an error response, whether
// the request is refused or accepted.
func TestNoSecretReachesLogsAuditOrErrorsOnReconcile(t *testing.T) {
	_, a, _, oldCode := fresh(t)
	logs := []string{a.logs.String()}
	a.stop()
	removeMarker(t, a)
	a = start(t, a.c, options{runtime: a.runtime, state: a.state})

	nb := newBrowser(a)
	nb.state()
	var bodies []string
	call := func(body map[string]any) *reply {
		r := nb.do("POST", "/api/v1/system/recovery/reconcile", body)
		if r.status >= 400 {
			bodies = append(bodies, string(r.body))
		}
		return r
	}
	call(map[string]any{"password": wrongPass, "newPassword": otherPass, "confirmRevocation": true})
	call(map[string]any{"password": goodPass, "newPassword": "short", "confirmRevocation": true})
	call(map[string]any{"password": goodPass, "newPassword": goodPass, "confirmRevocation": true})
	r := call(map[string]any{"password": goodPass, "newPassword": otherPass, "confirmRevocation": true})
	newCode := r.str("recoveryCode")
	if r.status != 200 || newCode == "" {
		t.Fatalf("reconcile = %d %s", r.status, r.body)
	}
	call(map[string]any{"password": goodPass, "newPassword": otherPass, "confirmRevocation": true})
	logs = append(logs, a.logs.String())

	sensitive := map[string]string{
		"restored password": goodPass, "new password": otherPass, "wrong password": wrongPass,
		"new recovery code": newCode, "canonical new recovery code": canonical(newCode),
		"old recovery code": oldCode, "canonical old recovery code": canonical(oldCode),
	}
	audit := a.scalar(`SELECT coalesce(string_agg(detail::text || coalesce(actor_ref, '') || action, ' '), '') FROM morningstar.audit_events`)
	for name, v := range sensitive {
		for i, l := range logs {
			if strings.Contains(l, v) {
				t.Errorf("the %s is in log %d", name, i)
			}
		}
		if strings.Contains(audit, v) {
			t.Errorf("the %s is in an audit event", name)
		}
		for _, body := range bodies {
			if strings.Contains(body, v) {
				t.Errorf("the %s is in an error response: %s", name, body)
			}
		}
	}
	if len(bodies) != 4 {
		t.Errorf("%d refused requests were recorded, want 4", len(bodies))
	}
}

// An anonymous cookie is signed with a key that lives only in the process. A
// forged one, one from before a restart and a swapped one are refused.
func TestPreSessionTokenIsBoundToTheProcess(t *testing.T) {
	_, a, _, _ := fresh(t)
	b := newBrowser(a)
	b.state()
	anon := b.cookies["ms_anon"]
	login := func(cookie, csrf string) *reply {
		nb := newBrowser(a)
		nb.cookies["ms_anon"], nb.csrf = cookie, csrf
		return nb.do("POST", "/api/v1/session/login", map[string]any{"username": username, "password": goodPass})
	}
	if r := login(anon, b.csrf); r.status != 200 {
		t.Fatalf("a good pre-session pair = %d %s", r.status, r.body)
	}
	flip := func(s string) string {
		i := len(s) - 2
		c := byte('A')
		if s[i] == 'A' {
			c = 'B'
		}
		return s[:i] + string(c) + s[i+1:]
	}
	for name, c := range map[string]struct{ cookie, csrf string }{
		"tampered cookie": {flip(anon), b.csrf},
		"tampered csrf":   {anon, flip(b.csrf)},
		"empty csrf":      {anon, ""},
		"cookie as csrf":  {anon, anon},
		"no cookie":       {"", b.csrf},
	} {
		if r := login(c.cookie, c.csrf); r.status != 403 || r.code() != "csrf_invalid" {
			t.Errorf("%s = %d %s", name, r.status, r.code())
		}
	}
	// The session CSRF token does not stand in for the pre-session one.
	s := newBrowser(a)
	s.login(username, goodPass)
	if r := login(anon, s.csrf); r.status != 403 {
		t.Errorf("a session CSRF token on a public route = %d", r.status)
	}

	rt, state := a.runtime, a.state
	a.stop()
	a2 := start(t, a.c, options{runtime: rt, state: state})
	nb := newBrowser(a2)
	nb.cookies["ms_anon"], nb.csrf = anon, b.csrf
	if r := nb.do("POST", "/api/v1/session/login", map[string]any{"username": username, "password": goodPass}); r.status != 403 || r.code() != "csrf_invalid" {
		t.Errorf("a pre-session cookie from before the restart = %d %s", r.status, r.code())
	}
	if nb.state(); nb.cookies["ms_anon"] == anon {
		t.Error("GET /session kept a cookie that no longer verifies")
	}
	if r := nb.do("POST", "/api/v1/session/login", map[string]any{"username": username, "password": goodPass}); r.status != 200 {
		t.Errorf("login after asking for a new pre-session token = %d %s", r.status, r.body)
	}
}

// The session CSRF token of one session does not work for another, and the
// session cookie of one browser does not work with the token of another.
func TestSessionCSRFTokenIsPerSession(t *testing.T) {
	_, a, b, _ := fresh(t)
	other := newBrowser(a)
	other.login(username, goodPass)
	if b.csrf == other.csrf || b.csrf == "" {
		t.Fatal("two sessions share a CSRF token")
	}
	if r := b.post("/api/v1/session/logout", nil, header("X-CSRF-Token", other.csrf)); r.status != 403 || r.code() != "csrf_invalid" {
		t.Errorf("another session's token = %d %s", r.status, r.code())
	}
	if r := b.post("/api/v1/session/logout", nil, header("X-CSRF-Token", b.cookies["ms_session"])); r.status != 403 {
		t.Errorf("the session token as CSRF token = %d", r.status)
	}
	if r := b.post("/api/v1/session/logout", nil); r.status != 204 {
		t.Errorf("the right token = %d %s", r.status, r.body)
	}
}
