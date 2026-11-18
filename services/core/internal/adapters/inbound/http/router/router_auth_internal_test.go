package router

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/actor"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
)

var errReached = errors.New("handler reached")

type stubPrincipal struct{}

func (stubPrincipal) UserID() string          { return "u1" }
func (stubPrincipal) SessionID() string       { return "s1" }
func (stubPrincipal) Actor() actor.Actor      { return actor.Actor{} }
func (stubPrincipal) ValidCSRF(p string) bool { return p == "csrf-ok" }

// stubAuth answers the checks of the pipeline and records every use case call,
// so a test can tell whether a request got through the guards.
type stubAuth struct {
	gate    atomic.Int32
	reached atomic.Int32
}

func (s *stubAuth) hit() error { s.reached.Add(1); return errReached }

func (s *stubAuth) Gate() inbound.Gate { return inbound.Gate(s.gate.Load()) }
func (s *stubAuth) Authenticate(_ context.Context, token string) (inbound.Principal, error) {
	switch token {
	case "good":
		return stubPrincipal{}, nil
	case "expired":
		return nil, inbound.ErrSessionExpired
	}
	return nil, inbound.ErrUnauthenticated
}
func (s *stubAuth) ValidAnonCSRF(anon, presented string) bool {
	return anon == "anon" && presented == "anon-csrf"
}
func (s *stubAuth) SessionState(context.Context, inbound.Cookies) (inbound.SessionState, error) {
	return inbound.SessionState{}, s.hit()
}
func (s *stubAuth) Setup(context.Context, inbound.SetupInput) (inbound.SetupResult, error) {
	return inbound.SetupResult{}, s.hit()
}
func (s *stubAuth) Login(context.Context, inbound.LoginInput) (inbound.LoginResult, error) {
	return inbound.LoginResult{}, s.hit()
}
func (s *stubAuth) Logout(context.Context, inbound.Principal, string) error { return s.hit() }
func (s *stubAuth) ListSessions(context.Context, inbound.Principal, int) ([]inbound.SessionView, error) {
	return nil, s.hit()
}
func (s *stubAuth) RevokeSession(context.Context, inbound.Principal, string, string) (inbound.SessionView, error) {
	return inbound.SessionView{}, s.hit()
}
func (s *stubAuth) Reauthenticate(context.Context, inbound.Principal, inbound.ReauthInput) (inbound.ReauthGrant, error) {
	return inbound.ReauthGrant{}, s.hit()
}
func (s *stubAuth) ResetPassword(context.Context, inbound.RecoveryResetInput) (inbound.RecoveryResetResult, error) {
	return inbound.RecoveryResetResult{}, s.hit()
}
func (s *stubAuth) ChangePassword(context.Context, inbound.Principal, inbound.ChangePasswordInput) (inbound.IssuedSession, error) {
	return inbound.IssuedSession{}, s.hit()
}
func (s *stubAuth) Reconcile(context.Context, inbound.ReconcileInput) (inbound.ReconcileResult, error) {
	return inbound.ReconcileResult{}, s.hit()
}

const (
	goodHost   = "127.0.0.1:8080"
	goodOrigin = "http://127.0.0.1:3000"
)

func authEngine(t *testing.T) (*stubAuth, http.Handler) {
	t.Helper()
	st := &stubAuth{}
	engine := build(Deps{Health: fakeHealthStub{}, Auth: st, Evidence: &stubEvidence{st}, AllowedHosts: []string{goodHost}, AllowedOrigins: []string{goodOrigin}})
	return st, engine
}

type fakeHealthStub struct{}

func (fakeHealthStub) Live() bool                 { return true }
func (fakeHealthStub) Ready(context.Context) bool { return true }

type route struct {
	method, path string
	kind         string // public, session
	body         bool
	needsGate    bool
	payload      string
}

// The route table is part of the test: a new route has to be added here, with
// its kind, or the inventory test fails (T-54).
var table = []route{
	{"GET", "/api/v1/session", "public", false, false, ""},
	{"POST", "/api/v1/system/recovery/reconcile", "public", true, false, ""},
	{"POST", "/api/v1/setup/account", "public", true, true, ""},
	{"POST", "/api/v1/session/login", "public", true, true, ""},
	{"POST", "/api/v1/recovery/reset", "public", true, true, ""},
	{"GET", "/api/v1/sessions", "session", false, true, ""},
	{"POST", "/api/v1/session/logout", "session", false, true, ""},
	{"POST", "/api/v1/sessions/7a1c2b3d-0000-4000-8000-000000000001/revoke", "session", false, true, ""},
	{"POST", "/api/v1/session/reauthenticate", "session", true, true, `{"subject":{}}`},
	{"POST", "/api/v1/account/password", "session", true, true, ""},
	{"GET", "/api/v1/assessments/7a1c2b3d-0000-4000-8000-0000000000e1/evidence", "session", false, true, ""},
	{"GET", "/api/v1/evidence/7a1c2b3d-0000-4000-8000-0000000000e2", "session", false, true, ""},
	{"POST", "/api/v1/evidence/7a1c2b3d-0000-4000-8000-0000000000e2/integrity-checks", "session", false, true, ""},
	{"GET", "/api/v1/evidence/7a1c2b3d-0000-4000-8000-0000000000e2/integrity-checks", "session", false, true, ""},
	{"GET", "/api/v1/evidence/7a1c2b3d-0000-4000-8000-0000000000e2/content", "session", false, true, ""},
}

func (r route) request(mods ...func(*http.Request)) *http.Request {
	body := ""
	if r.body {
		body = "{}"
		if r.payload != "" {
			body = r.payload
		}
	}
	req := httptest.NewRequest(r.method, r.path, strings.NewReader(body))
	req.Host = goodHost
	if r.body {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, m := range mods {
		m(req)
	}
	return req
}

func withCookies(kind string) func(*http.Request) {
	return func(r *http.Request) {
		if kind == "session" {
			r.AddCookie(&http.Cookie{Name: "ms_session", Value: "good"})
		} else {
			r.AddCookie(&http.Cookie{Name: "ms_anon", Value: "anon"})
		}
	}
}

func withOrigin(o string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set("Origin", o) }
}
func withCSRF(c string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set("X-CSRF-Token", c) }
}

func csrfFor(kind string) string {
	if kind == "session" {
		return "csrf-ok"
	}
	return "anon-csrf"
}

func do(h http.Handler, req *http.Request) (int, string, http.Header) {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String(), rec.Header()
}

func codeOf(body string) string {
	i := strings.Index(body, `"code":"`)
	if i < 0 {
		return ""
	}
	rest := body[i+len(`"code":"`):]
	return rest[:strings.Index(rest, `"`)]
}

// The inventory of registered routes is exactly the table.
func TestRouteInventoryMatchesTheTable(t *testing.T) {
	st := &stubAuth{}
	engine := build(Deps{Health: fakeHealthStub{}, Auth: st, Evidence: &stubEvidence{st}, AllowedHosts: []string{goodHost}, AllowedOrigins: []string{goodOrigin}})
	have := map[string]bool{}
	for _, r := range engine.Routes() {
		have[r.Method+" "+r.Path] = true
	}
	want := map[string]bool{"GET /api/v1/health/live": true, "GET /api/v1/health/ready": true}
	for _, r := range table {
		p := strings.NewReplacer("7a1c2b3d-0000-4000-8000-000000000001", ":sessionId", "7a1c2b3d-0000-4000-8000-0000000000e1", ":assessmentId", "7a1c2b3d-0000-4000-8000-0000000000e2", ":evidenceId").Replace(r.path)
		want[r.method+" "+p] = true
	}
	var extra, missing []string
	for k := range have {
		if !want[k] {
			extra = append(extra, k)
		}
	}
	for k := range want {
		if !have[k] {
			missing = append(missing, k)
		}
	}
	sort.Strings(extra)
	sort.Strings(missing)
	if len(extra) > 0 || len(missing) > 0 {
		t.Errorf("routes not in the test table: %v, table entries without a route: %v", extra, missing)
	}
}

func TestEveryRouteRefusesAForeignHostBeforeAnythingElse(t *testing.T) {
	st, h := authEngine(t)
	for _, r := range table {
		for _, host := range []string{"evil.example", "127.0.0.1:9999", "localhost:8080", ""} {
			status, body, _ := do(h, r.request(withCookies(r.kind), withOrigin("http://evil.example"), func(q *http.Request) { q.Host = host }))
			if status != 403 || codeOf(body) != "host_denied" {
				t.Errorf("%s %s with Host %q = %d %s", r.method, r.path, host, status, codeOf(body))
			}
		}
		// A forwarded host header does not replace Host.
		status, body, _ := do(h, r.request(withCookies(r.kind), func(q *http.Request) { q.Header.Set("X-Forwarded-Host", goodHost); q.Host = "evil.example" }))
		if status != 403 || codeOf(body) != "host_denied" {
			t.Errorf("%s %s trusted X-Forwarded-Host: %d %s", r.method, r.path, status, codeOf(body))
		}
	}
	if st.reached.Load() != 0 {
		t.Error("a handler ran for a foreign Host")
	}
}

func TestEveryStateChangingRouteRequiresOriginAndCSRF(t *testing.T) {
	for _, r := range table {
		if r.method == "GET" {
			continue
		}
		t.Run(r.path, func(t *testing.T) {
			st, h := authEngine(t)
			deny := func(name string, req *http.Request, wantCode string) {
				t.Helper()
				before := st.reached.Load()
				status, body, _ := do(h, req)
				if status != 403 || codeOf(body) != wantCode {
					t.Errorf("%s = %d %s, want 403 %s", name, status, codeOf(body), wantCode)
				}
				if st.reached.Load() != before {
					t.Errorf("%s reached the handler", name)
				}
			}
			cookies := withCookies(r.kind)
			deny("no Origin", r.request(cookies, withCSRF(csrfFor(r.kind))), "origin_denied")
			for _, o := range []string{"null", "http://evil.example", "http://127.0.0.1:3001", "http://localhost:3000", "https://127.0.0.1:3000", goodOrigin + "/", " " + goodOrigin} {
				deny("Origin "+o, r.request(cookies, withOrigin(o), withCSRF(csrfFor(r.kind))), "origin_denied")
			}
			deny("cross-site fetch metadata", r.request(cookies, withOrigin(goodOrigin), withCSRF(csrfFor(r.kind)), func(q *http.Request) { q.Header.Set("Sec-Fetch-Site", "cross-site") }), "origin_denied")
			deny("no CSRF token", r.request(cookies, withOrigin(goodOrigin)), "csrf_invalid")
			deny("wrong CSRF token", r.request(cookies, withOrigin(goodOrigin), withCSRF("nope")), "csrf_invalid")
			deny("the other kind of token", r.request(cookies, withOrigin(goodOrigin), withCSRF(csrfFor(map[string]string{"session": "public", "public": "session"}[r.kind]))), "csrf_invalid")
			if r.kind == "public" {
				deny("no pre-session cookie", r.request(withOrigin(goodOrigin), withCSRF("anon-csrf")), "csrf_invalid")
				deny("another pre-session cookie", r.request(func(q *http.Request) { q.AddCookie(&http.Cookie{Name: "ms_anon", Value: "forged"}) }, withOrigin(goodOrigin), withCSRF("anon-csrf")), "csrf_invalid")
			}
			// All checks pass: the handler is reached, with same-origin metadata too.
			for _, site := range []string{"", "same-origin", "same-site", "none"} {
				before := st.reached.Load()
				status, _, _ := do(h, r.request(cookies, withOrigin(goodOrigin), withCSRF(csrfFor(r.kind)), func(q *http.Request) {
					if site != "" {
						q.Header.Set("Sec-Fetch-Site", site)
					}
				}))
				if status == 403 || st.reached.Load() != before+1 {
					t.Errorf("a valid request (Sec-Fetch-Site %q) did not reach the handler: %d", site, status)
				}
			}
		})
	}
}

func TestAuthenticatedRoutesNeedALiveSessionBeforeAnythingElse(t *testing.T) {
	for _, r := range table {
		if r.kind != "session" {
			continue
		}
		st, h := authEngine(t)
		cases := map[string]struct {
			mods []func(*http.Request)
			want string
		}{
			"no cookie":     {nil, "unauthenticated"},
			"unknown token": {[]func(*http.Request){func(q *http.Request) { q.AddCookie(&http.Cookie{Name: "ms_session", Value: "bad"}) }}, "unauthenticated"},
			"expired":       {[]func(*http.Request){func(q *http.Request) { q.AddCookie(&http.Cookie{Name: "ms_session", Value: "expired"}) }}, "session_expired"},
			"bearer only":   {[]func(*http.Request){func(q *http.Request) { q.Header.Set("Authorization", "Bearer good") }}, "unauthenticated"},
			"token in URL":  {[]func(*http.Request){func(q *http.Request) { q.URL.RawQuery = "token=good&ms_session=good" }}, "unauthenticated"},
		}
		for name, c := range cases {
			mods := append([]func(*http.Request){withOrigin(goodOrigin), withCSRF("csrf-ok")}, c.mods...)
			status, body, _ := do(h, r.request(mods...))
			if status != 401 || codeOf(body) != c.want {
				t.Errorf("%s %s (%s) = %d %s", r.method, r.path, name, status, codeOf(body))
			}
		}
		if st.reached.Load() != 0 {
			t.Errorf("%s reached a handler without a session", r.path)
		}
	}
}

func TestRecoveryGateClosesEveryRouteButTheTwoThatMustWork(t *testing.T) {
	for _, r := range table {
		for gate, wantCode := range map[inbound.Gate]string{inbound.GateRecoveryRequired: "recovery_required", inbound.GateNotReady: "database_unavailable"} {
			st, h := authEngine(t)
			st.gate.Store(int32(gate))
			mods := []func(*http.Request){withCookies(r.kind), withOrigin(goodOrigin), withCSRF(csrfFor(r.kind))}
			status, body, hdr := do(h, r.request(mods...))
			if r.needsGate {
				if status != 503 || codeOf(body) != wantCode || hdr.Get("Retry-After") == "" || st.reached.Load() != 0 {
					t.Errorf("%s %s under gate %d = %d %s", r.method, r.path, gate, status, codeOf(body))
				}
			} else if gate == inbound.GateRecoveryRequired && st.reached.Load() != 1 {
				t.Errorf("%s %s must work while recovery is required", r.method, r.path)
			}
		}
	}
	// Health stays available.
	st, h := authEngine(t)
	st.gate.Store(int32(inbound.GateRecoveryRequired))
	status, _, _ := do(h, httptest.NewRequest("GET", "/api/v1/health/live", nil))
	_ = status
	req := httptest.NewRequest("GET", "/api/v1/health/live", nil)
	req.Host = goodHost
	if status, _, _ := do(h, req); status != 200 {
		t.Errorf("liveness under the gate = %d", status)
	}
}

func TestBodyRules(t *testing.T) {
	r := route{"POST", "/api/v1/session/login", "public", true, true, ""}
	post := func(body, contentType string, extra ...func(*http.Request)) (int, string) {
		st, h := authEngine(t)
		_ = st
		mods := append([]func(*http.Request){withCookies("public"), withOrigin(goodOrigin), withCSRF("anon-csrf"), func(q *http.Request) {
			q.Body = http.NoBody
			q.Body = nopCloser(strings.NewReader(body))
			q.ContentLength = int64(len(body))
			if contentType == "" {
				q.Header.Del("Content-Type")
			} else {
				q.Header.Set("Content-Type", contentType)
			}
		}}, extra...)
		status, b, _ := do(h, r.request(mods...))
		return status, codeOf(b)
	}
	for name, c := range map[string]struct {
		body, ct string
		status   int
		code     string
	}{
		"text/plain":          {"{}", "text/plain", 415, "unsupported_media_type"},
		"form":                {"a=b", "application/x-www-form-urlencoded", 415, "unsupported_media_type"},
		"no content type":     {"{}", "", 415, "unsupported_media_type"},
		"json with latin1":    {"{}", "application/json; charset=iso-8859-1", 415, "unsupported_media_type"},
		"too large":           {`{"username":"` + strings.Repeat("a", 17<<10) + `"}`, "application/json", 413, "payload_too_large"},
		"trailing value":      {`{} {}`, "application/json", 400, "invalid_request"},
		"trailing garbage":    {`{}x`, "application/json", 400, "invalid_request"},
		"array":               {`[]`, "application/json", 400, "invalid_request"},
		"unknown field":       {`{"username":"a","password":"b","admin":true}`, "application/json", 400, "invalid_request"},
		"truncated":           {`{"username":`, "application/json", 400, "invalid_request"},
		"empty":               {``, "application/json", 400, "invalid_request"},
		"charset utf-8 is ok": {`{"username":"a","password":"b"}`, "application/json; charset=utf-8", 500, "internal_error"},
	} {
		status, code := post(c.body, c.ct)
		if status != c.status || code != c.code {
			t.Errorf("%s = %d %s, want %d %s", name, status, code, c.status, c.code)
		}
	}
}

func TestRequestRateLimitUsesOneBucketForTheClient(t *testing.T) {
	_, h := authEngine(t)
	var limited int
	for i := 0; i < 40; i++ {
		req := httptest.NewRequest("POST", "/api/v1/session/login", strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.Host = goodHost
		req.RemoteAddr = "127.0.0.1:" + strings.Repeat("1", 1) + string(rune('0'+i%10)) + "000"
		req.Header.Set("X-Forwarded-For", "203.0.113."+string(rune('1'+i%9)))
		status, body, hdr := do(h, req)
		if status == 429 {
			limited++
			if codeOf(body) != "rate_limited" || hdr.Get("Retry-After") == "" {
				t.Errorf("429 without the envelope or Retry-After: %s", body)
			}
		}
	}
	if limited != 10 {
		t.Errorf("%d of 40 requests were limited, want 10 (limit 30 per minute whatever the source port or forwarded address)", limited)
	}
}

// Reading the session state is not a credential attempt. A flood of it must not
// use up the budget of the routes that take credentials.
func TestReadingTheSessionStateDoesNotSpendTheLoginBudget(t *testing.T) {
	_, h := authEngine(t)
	for i := 0; i < 100; i++ {
		req := httptest.NewRequest("GET", "/api/v1/session", nil)
		req.Host = goodHost
		if status, body, _ := do(h, req); status == 429 {
			t.Fatalf("GET /session was limited at request %d: %s", i+1, body)
		}
	}
	req := httptest.NewRequest("POST", "/api/v1/session/login", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Host = goodHost
	if status, body, _ := do(h, req); status == 429 {
		t.Fatalf("login was limited after reads: %s", body)
	}
}

func TestAllowHeaderForAParameterisedRoute(t *testing.T) {
	_, h := authEngine(t)
	req := httptest.NewRequest("GET", "/api/v1/sessions/7a1c2b3d-0000-4000-8000-000000000001/revoke", nil)
	req.Host = goodHost
	status, body, hdr := do(h, req)
	if status != 405 || codeOf(body) != "method_not_allowed" || hdr.Get("Allow") != "POST" {
		t.Errorf("= %d %s Allow %q", status, codeOf(body), hdr.Get("Allow"))
	}
	for _, p := range []string{"/api/v1/sessions/", "/api/v1/sessions/x/revoke/extra"} {
		req := httptest.NewRequest("POST", p, nil)
		req.Host = goodHost
		if status, _, _ := do(h, req); status != 404 {
			t.Errorf("POST %s = %d, want 404", p, status)
		}
	}
}
