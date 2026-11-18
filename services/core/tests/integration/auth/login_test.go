package auth_test

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func cookieOf(r *reply, name string) *http.Cookie {
	for _, c := range (&http.Response{Header: r.header}).Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func TestLoginIssuesAHardenedSessionCookie(t *testing.T) {
	_, a, _, _ := fresh(t)
	b := newBrowser(a)
	r := b.login(username, goodPass)
	if r.status != 200 || r.str("state") != "authenticated" || r.str("user", "username") != username || r.str("csrfToken") == "" {
		t.Fatalf("login = %d %s", r.status, r.body)
	}
	c := cookieOf(r, "ms_session")
	if c == nil {
		t.Fatal("no ms_session cookie")
	}
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/" || c.Domain != "" || c.MaxAge <= 0 || c.MaxAge > 12*3600 {
		t.Errorf("cookie attributes: %+v", c)
	}
	if c.Secure {
		t.Error("Secure is set on a plain http listener, where the browser would drop the cookie")
	}
	if len(c.Value) != 43 {
		t.Errorf("token length %d, want 43 characters (256 bits)", len(c.Value))
	}
	if strings.Contains(string(r.body), c.Value) {
		t.Error("the session token is in the body")
	}
	anon := cookieOf(b.do("GET", "/api/v1/session", nil), "ms_anon")
	_ = anon
	// No fixation: logging in twice gives two different tokens.
	first := b.cookies["ms_session"]
	if second := newBrowser(a); second.login(username, goodPass).status != 200 || second.cookies["ms_session"] == first {
		t.Error("a second login reused a token")
	}
	// The state endpoint returns the same CSRF token again, and never the session token.
	s := b.do("GET", "/api/v1/session", nil)
	if s.str("state") != "authenticated" || s.str("csrfToken") != b.csrf || strings.Contains(string(s.body), first) {
		t.Errorf("session state = %s", s.body)
	}
}

func TestLoginFailureDoesNotDiscloseTheAccount(t *testing.T) {
	_, a, _, _ := fresh(t)
	b := newBrowser(a)
	unknown := b.login("nobody-here", goodPass)
	wrong := b.login(username, wrongPass)
	weird := b.login("ünïcode\x00user", goodPass)
	for name, r := range map[string]*reply{"unknown user": unknown, "wrong password": wrong, "malformed name": weird} {
		if r.status != 401 || r.code() != "invalid_credentials" {
			t.Errorf("%s = %d %s", name, r.status, r.body)
		}
	}
	strip := func(r *reply) string {
		m := r.json()
		e := m["error"].(map[string]any)
		delete(e, "requestId")
		return fmt.Sprint(m)
	}
	if strip(unknown) != strip(wrong) || strip(unknown) != strip(weird) {
		t.Errorf("the answers differ:\n%s\n%s\n%s", unknown.body, wrong.body, weird.body)
	}
	for _, r := range []*reply{unknown, wrong} {
		if strings.Contains(string(r.body), username) || strings.Contains(string(r.body), "nobody-here") {
			t.Error("the response echoes a username")
		}
	}
	if cookieOf(wrong, "ms_session") != nil {
		t.Error("a failed login set a session cookie")
	}
}

// D-68: more than 5 failures in a rolling 15 minutes are refused, and a trivial
// variation of the request does not pick another bucket.
func TestLoginRateLimitCannotBeBypassedByVariations(t *testing.T) {
	_, a, _, _ := fresh(t)
	b := newBrowser(a)
	for i := 0; i < 5; i++ {
		if r := b.login(username, wrongPass); r.status != 401 {
			t.Fatalf("attempt %d = %d %s", i, r.status, r.body)
		}
	}
	variations := map[string][]mod{
		"correct password":     nil,
		"another Host":         {func(r *http.Request) { r.Host = "localhost:" + strings.Split(a.addr, ":")[1] }},
		"forwarded for":        {header("X-Forwarded-For", "203.0.113.9"), header("X-Real-Ip", "203.0.113.9")},
		"forwarded host":       {header("X-Forwarded-Host", "other.example")},
		"other user agent":     {header("User-Agent", "something else")},
		"another origin entry": {header("Origin", "http://localhost:3000")},
	}
	for name, mods := range variations {
		nb := newBrowser(a)
		nb.state()
		body := map[string]any{"username": username, "password": goodPass}
		if name == "another Host" {
			nb.origin = "http://localhost:3000"
		}
		r := nb.do("POST", "/api/v1/session/login", body, mods...)
		if r.status != 429 || r.code() != "rate_limited" || r.header.Get("Retry-After") == "" {
			t.Errorf("%s = %d %s (Retry-After %q)", name, r.status, r.body, r.header.Get("Retry-After"))
		}
	}
	// Another spelling of the name is the same account.
	nb := newBrowser(a)
	if r := nb.login("RESEARCHER", goodPass); r.status != 429 {
		t.Errorf("upper-case user name = %d", r.status)
	}
	if got := a.scalar(`SELECT count(*) FROM morningstar.login_attempts WHERE kind = 'login' AND NOT succeeded`); got != "5" {
		t.Errorf("%s failed attempts recorded, want exactly 5 (the refused ones must not be hashed or recorded as admitted)", got)
	}
	if got := a.scalar(`SELECT count(*) FROM morningstar.audit_events WHERE action = 'auth.login' AND outcome = 'denied' AND detail->>'reason' = 'rate_limited'`); got != "7" {
		t.Errorf("%s rate-limit refusals are audited, want 7", got)
	}
}

// The window rolls: failures older than 15 minutes do not count.
func TestLoginAttemptsAgeOutOfTheWindow(t *testing.T) {
	_, a, _, _ := fresh(t)
	a.exec(`INSERT INTO morningstar.login_attempts (id, kind, source_key, succeeded, attempted_at)
		SELECT uuidv7(), 'login', 'loopback', false, now() - interval '16 minutes' FROM generate_series(1, 20)`)
	if r := newBrowser(a).login(username, goodPass); r.status != 200 {
		t.Errorf("login with only old failures = %d %s", r.status, r.body)
	}
	a.exec(`INSERT INTO morningstar.login_attempts (id, kind, source_key, succeeded, attempted_at)
		SELECT uuidv7(), 'login', 'loopback', false, now() - interval '14 minutes' FROM generate_series(1, 5)`)
	if r := newBrowser(a).login(username, goodPass); r.status != 429 {
		t.Errorf("login with 5 recent failures = %d", r.status)
	}
}

// Parallel guesses are counted before any hashing, so a burst cannot get more
// than 5 checks through.
func TestParallelGuessesAreBoundedByTheLimit(t *testing.T) {
	_, a, _, _ := fresh(t)
	var wg sync.WaitGroup
	var mu sync.Mutex
	counts := map[int]int{}
	seed := newBrowser(a)
	seed.state()
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			nb := newBrowser(a)
			nb.cookies["ms_anon"], nb.csrf = seed.cookies["ms_anon"], seed.csrf
			r := nb.do("POST", "/api/v1/session/login", map[string]any{"username": username, "password": fmt.Sprintf("%s %d", wrongPass, i)})
			mu.Lock()
			counts[r.status]++
			mu.Unlock()
		}()
	}
	wg.Wait()
	if counts[401] != 5 || counts[429] != 20 {
		t.Errorf("answers = %v, want five 401 and twenty 429", counts)
	}
}

func TestSessionExpiryAndRevocationAreEnforcedOnTheServer(t *testing.T) {
	_, a, b, _ := fresh(t)
	if r := b.do("GET", "/api/v1/sessions", nil); r.status != 200 {
		t.Fatalf("sessions = %d %s", r.status, r.body)
	}
	token := b.cookies["ms_session"]

	cases := map[string]string{
		"idle expiry":     `UPDATE morningstar.sessions SET idle_expires_at = now() - interval '1 second'`,
		"absolute expiry": `UPDATE morningstar.sessions SET created_at = now() - interval '13 hours', idle_expires_at = now() + interval '1 hour', absolute_expires_at = now() - interval '1 second'`,
	}
	for name, sql := range cases {
		a.exec(`UPDATE morningstar.sessions SET created_at = now(), idle_expires_at = now() + interval '1 hour', absolute_expires_at = now() + interval '8 hours'`)
		a.exec(sql)
		if r := b.do("GET", "/api/v1/sessions", nil); r.status != 401 || r.code() != "session_expired" {
			t.Errorf("%s = %d %s", name, r.status, r.body)
		}
	}
	a.exec(`UPDATE morningstar.sessions SET created_at = now(), idle_expires_at = now() + interval '1 hour', absolute_expires_at = now() + interval '8 hours'`)
	a.exec(`UPDATE morningstar.sessions SET revoked_at = now(), revoked_reason = 'user_revoked'`)
	if r := b.do("GET", "/api/v1/sessions", nil); r.status != 401 || r.code() != "session_expired" {
		t.Errorf("revoked = %d %s", r.status, r.body)
	}

	for name, tok := range map[string]string{
		"no cookie":        "",
		"garbage":          "not a token",
		"wrong length":     token[:20],
		"unknown token":    strings.Repeat("A", 43),
		"padding":          token + "=",
		"injection":        token + "' OR '1'='1",
		"token with space": token[:10] + " " + token[11:],
	} {
		nb := newBrowser(a)
		if tok != "" {
			nb.cookies["ms_session"] = tok
		}
		if r := nb.do("GET", "/api/v1/sessions", nil); r.status != 401 {
			t.Errorf("%s = %d %s", name, r.status, r.body)
		}
	}
	// No bearer, no query token: the cookie is the only credential (D-60).
	nb := newBrowser(a)
	r := nb.do("GET", "/api/v1/sessions?token="+token, nil, header("Authorization", "Bearer "+token))
	if r.status != 401 {
		t.Errorf("a bearer token or a token in the URL was accepted: %d", r.status)
	}
}

func TestIdleExpiryExtendsOncePerMinute(t *testing.T) {
	_, a, b, _ := fresh(t)
	a.exec(`UPDATE morningstar.sessions SET last_seen_at = now() - interval '5 minutes', idle_expires_at = now() + interval '10 minutes'`)
	before := a.scalar(`SELECT extract(epoch FROM idle_expires_at) FROM morningstar.sessions`)
	if r := b.do("GET", "/api/v1/sessions", nil); r.status != 200 {
		t.Fatal(r.status)
	}
	after := a.scalar(`SELECT extract(epoch FROM idle_expires_at) FROM morningstar.sessions`)
	if after <= before {
		t.Errorf("the idle expiry did not move: %s -> %s", before, after)
	}
	b.do("GET", "/api/v1/sessions", nil)
	if again := a.scalar(`SELECT extract(epoch FROM idle_expires_at) FROM morningstar.sessions`); again != after {
		t.Errorf("a second request within a minute wrote again: %s -> %s", after, again)
	}
	if left := a.scalar(`SELECT extract(epoch FROM idle_expires_at - now()) > 29 * 60 AND extract(epoch FROM idle_expires_at - now()) <= 30 * 60 FROM morningstar.sessions`); left != "true" {
		t.Errorf("the idle expiry is not 30 minutes ahead")
	}
}

func TestLogoutRevokesTheSessionAndClearsTheCookie(t *testing.T) {
	_, a, b, _ := fresh(t)
	old := b.cookies["ms_session"]
	r := b.post("/api/v1/session/logout", nil)
	if r.status != 204 {
		t.Fatalf("logout = %d %s", r.status, r.body)
	}
	if c := cookieOf(r, "ms_session"); c == nil || c.MaxAge >= 0 || c.Value != "" {
		t.Errorf("the cookie was not cleared: %+v", c)
	}
	nb := newBrowser(a)
	nb.cookies["ms_session"] = old
	if r := nb.do("GET", "/api/v1/sessions", nil); r.status != 401 || r.code() != "session_expired" {
		t.Errorf("the old cookie = %d %s", r.status, r.code())
	}
	if a.scalar(`SELECT revoked_reason FROM morningstar.sessions`) != "logout" {
		t.Error("the revocation reason is not logout")
	}
	if a.scalar(`SELECT count(*) FROM morningstar.audit_events WHERE action = 'auth.logout'`) != "1" {
		t.Error("logout is not audited")
	}
}

func TestSessionListAndRevoke(t *testing.T) {
	_, a, b, _ := fresh(t)
	other := newBrowser(a)
	if other.login(username, goodPass).status != 200 {
		t.Fatal("second login failed")
	}
	r := b.do("GET", "/api/v1/sessions", nil)
	items, _ := r.json()["items"].([]any)
	if r.status != 200 || len(items) != 2 {
		t.Fatalf("list = %d %s", r.status, r.body)
	}
	var otherID string
	current := 0
	for _, it := range items {
		m := it.(map[string]any)
		if m["current"] == true {
			current++
		} else {
			otherID, _ = m["id"].(string)
		}
		if _, has := m["tokenHash"]; has || strings.Contains(fmt.Sprint(m), "csrf") {
			t.Errorf("the list leaks credential data: %v", m)
		}
	}
	if current != 1 || otherID == "" {
		t.Fatalf("list = %s", r.body)
	}
	if r := b.post("/api/v1/sessions/"+otherID+"/revoke", nil); r.status != 200 || r.str("id") != otherID {
		t.Fatalf("revoke = %d %s", r.status, r.body)
	}
	if r := other.do("GET", "/api/v1/sessions", nil); r.status != 401 {
		t.Errorf("the revoked session still works: %d", r.status)
	}
	if r := b.post("/api/v1/sessions/"+otherID+"/revoke", nil); r.status != 409 || r.code() != "conflict_state" {
		t.Errorf("a second revoke = %d %s", r.status, r.code())
	}
	for _, id := range []string{"00000000-0000-7000-8000-000000000000", "not-a-uuid", "1' OR '1'='1"} {
		if r := b.post("/api/v1/sessions/"+strings.ReplaceAll(id, " ", "%20")+"/revoke", nil); r.status != 404 {
			t.Errorf("revoke %q = %d %s", id, r.status, r.body)
		}
	}
	if r := b.do("GET", "/api/v1/sessions?limit=0", nil); r.status != 422 {
		t.Errorf("limit 0 = %d", r.status)
	}
	if r := b.do("GET", "/api/v1/sessions?limit=201", nil); r.status != 422 {
		t.Errorf("limit 201 = %d", r.status)
	}
	// Revoking the current session behaves like a logout.
	cur := b.do("GET", "/api/v1/sessions", nil).json()["items"].([]any)[0].(map[string]any)["id"].(string)
	if r := b.post("/api/v1/sessions/"+cur+"/revoke", nil); r.status != 200 {
		t.Errorf("revoking the current session = %d %s", r.status, r.body)
	}
	if r := b.do("GET", "/api/v1/sessions", nil); r.status != 401 {
		t.Errorf("after revoking itself = %d", r.status)
	}
	_ = time.Second
}
