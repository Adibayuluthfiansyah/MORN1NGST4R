package router_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/inbound/http/router"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

type fakeHealth struct {
	live, ready bool
	panicOnLive bool
}

func (f fakeHealth) Live() bool {
	if f.panicOnLive {
		panic("boom-secret-value")
	}
	return f.live
}
func (f fakeHealth) Ready(context.Context) bool { return f.ready }

type errorBody struct {
	Error struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"requestId"`
		Retryable bool   `json:"retryable"`
	} `json:"error"`
}

type harness struct {
	handler http.Handler
	logs    *bytes.Buffer
}

func newHarness(t *testing.T, h fakeHealth) harness {
	t.Helper()
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	handler := router.New(router.Deps{
		Health:       h,
		AllowedHosts: []string{"127.0.0.1:8080", "localhost:8080"},
		Logger:       logger,
	})
	return harness{handler: handler, logs: &logs}
}

func (h harness) do(method, target, host string, header map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	req.Host = host
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)
	return rec
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) errorBody {
	t.Helper()
	dec := json.NewDecoder(rec.Body)
	dec.DisallowUnknownFields() // the envelope has no other field
	var body errorBody
	if err := dec.Decode(&body); err != nil {
		t.Fatalf("body is not the error envelope: %v (%q)", err, rec.Body.String())
	}
	return body
}

func assertCommonHeaders(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
	// API-19 (docs/design/api/README.md): every response denies framing and
	// carries the same policy set.
	for name, want := range map[string]string{
		"X-Frame-Options":              "DENY",
		"Content-Security-Policy":      "default-src 'none'; frame-ancestors 'none'",
		"Referrer-Policy":              "no-referrer",
		"Cross-Origin-Resource-Policy": "same-origin",
	} {
		if got := rec.Header().Values(name); len(got) != 1 || got[0] != want {
			t.Errorf("%s = %q, want exactly one value %q", name, got, want)
		}
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", got)
	}
	if rec.Header().Get("X-Request-Id") == "" {
		t.Error("X-Request-Id is missing")
	}
	for _, leak := range []string{"Server", "X-Powered-By"} {
		if v := rec.Header().Get(leak); v != "" {
			t.Errorf("%s header = %q, want it absent", leak, v)
		}
	}
}

func TestLive(t *testing.T) {
	h := newHarness(t, fakeHealth{live: true})
	rec := h.do(http.MethodGet, "/api/v1/health/live", "127.0.0.1:8080", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"status":"ok"}` {
		t.Errorf("body = %s, want {\"status\":\"ok\"}", got)
	}
	assertCommonHeaders(t, rec)
}

func TestLiveDoesNotDependOnReadiness(t *testing.T) {
	h := newHarness(t, fakeHealth{live: true, ready: false})
	if rec := h.do(http.MethodGet, "/api/v1/health/live", "localhost:8080", nil); rec.Code != http.StatusOK {
		t.Errorf("live status = %d, want 200 while not ready", rec.Code)
	}
}

func TestLiveReportsAFailedProcess(t *testing.T) {
	h := newHarness(t, fakeHealth{live: false})
	rec := h.do(http.MethodGet, "/api/v1/health/live", "127.0.0.1:8080", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"status":"not_live"}` {
		t.Errorf("body = %s, want {\"status\":\"not_live\"}", got)
	}
}

func TestReady(t *testing.T) {
	h := newHarness(t, fakeHealth{live: true, ready: true})
	rec := h.do(http.MethodGet, "/api/v1/health/ready", "127.0.0.1:8080", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"status":"ready"}` {
		t.Errorf("body = %s, want {\"status\":\"ready\"}", got)
	}
	assertCommonHeaders(t, rec)
}

func TestNotReadyRevealsNothingBeyondTheStatus(t *testing.T) {
	h := newHarness(t, fakeHealth{live: true, ready: false})
	rec := h.do(http.MethodGet, "/api/v1/health/ready", "127.0.0.1:8080", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"status":"not_ready"}` {
		t.Errorf("body = %s, want {\"status\":\"not_ready\"}", got)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("Retry-After is missing on 503")
	}
	assertCommonHeaders(t, rec)
}

func TestHostAllowList(t *testing.T) {
	h := newHarness(t, fakeHealth{live: true, ready: true})

	allowed := []string{"127.0.0.1:8080", "localhost:8080", "LOCALHOST:8080"}
	for _, host := range allowed {
		if rec := h.do(http.MethodGet, "/api/v1/health/live", host, nil); rec.Code != http.StatusOK {
			t.Errorf("Host %q: status = %d, want 200", host, rec.Code)
		}
	}

	denied := []string{"evil.example:8080", "127.0.0.1:9999", "127.0.0.1", "localhost", "", "localhost.:8080", "localhost:8080.evil.example", "[::1]:8080", "127.0.0.1:8080@evil.example"}
	paths := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/health/live"},
		{http.MethodGet, "/api/v1/health/ready"},
		{http.MethodGet, "/no/such/route"},       // the allow-list runs before routing
		{http.MethodPost, "/api/v1/health/live"}, // and before method matching
	}
	for _, host := range denied {
		for _, p := range paths {
			rec := h.do(p.method, p.path, host, nil)
			if rec.Code != http.StatusForbidden {
				t.Errorf("%s %s Host %q: status = %d, want 403", p.method, p.path, host, rec.Code)
				continue
			}
			body := decodeError(t, rec)
			if body.Error.Code != "host_denied" || body.Error.Retryable {
				t.Errorf("Host %q: error = %+v, want host_denied, not retryable", host, body.Error)
			}
			if host != "" && strings.Contains(rec.Body.String(), host) {
				t.Errorf("Host %q is echoed in the body: %s", host, rec.Body.String())
			}
			assertCommonHeaders(t, rec)
		}
	}
}

func TestEmptyAllowListDeniesEverything(t *testing.T) {
	handler := router.New(router.Deps{Health: fakeHealth{live: true}, AllowedHosts: nil})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil)
	req.Host = "127.0.0.1:8080"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (fail closed)", rec.Code)
	}
}

func TestUnknownRouteAndMethod(t *testing.T) {
	h := newHarness(t, fakeHealth{live: true, ready: true})

	for _, path := range []string{"/", "/api/v1/nothing", "/api/v1/health", "/api/v1/health/live/", "/healthz"} {
		rec := h.do(http.MethodGet, path, "127.0.0.1:8080", nil)
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404 without redirect", path, rec.Code)
			continue
		}
		if body := decodeError(t, rec); body.Error.Code != "not_found" {
			t.Errorf("GET %s: code = %q, want not_found", path, body.Error.Code)
		}
		assertCommonHeaders(t, rec)
	}

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead} {
		rec := h.do(method, "/api/v1/health/live", "127.0.0.1:8080", nil)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: status = %d, want 405", method, rec.Code)
			continue
		}
		if got := rec.Header().Get("Allow"); got != "GET" {
			t.Errorf("%s: Allow = %q, want GET", method, got)
		}
		if method != http.MethodHead {
			if body := decodeError(t, rec); body.Error.Code != "method_not_allowed" {
				t.Errorf("%s: code = %q, want method_not_allowed", method, body.Error.Code)
			}
		}
	}
}

func TestRequestID(t *testing.T) {
	h := newHarness(t, fakeHealth{live: true})
	const valid = "018f0000-0000-7000-8000-000000000002"

	generated := h.do(http.MethodGet, "/api/v1/health/live", "127.0.0.1:8080", nil).Header().Get("X-Request-Id")
	if !uuidPattern.MatchString(generated) {
		t.Errorf("generated id %q is not a UUID", generated)
	}
	other := h.do(http.MethodGet, "/api/v1/health/live", "127.0.0.1:8080", nil).Header().Get("X-Request-Id")
	if generated == other {
		t.Errorf("two requests got the same id %q", generated)
	}

	// The id is the correlation id of the audit trail (TD-M4-03), so the server
	// makes it. A caller supplied id is never kept, valid or not.
	for _, supplied := range []string{valid, strings.ToUpper(valid), "abc", "not-a-uuid-at-all-but-36-characters-xx", strings.Repeat("a", 500), "018f0000-0000-7000-8000-00000000000g", " " + valid} {
		got := h.do(http.MethodGet, "/api/v1/health/live", "127.0.0.1:8080", map[string]string{"X-Request-Id": supplied}).Header().Get("X-Request-Id")
		if strings.EqualFold(got, supplied) || !uuidPattern.MatchString(got) {
			t.Errorf("caller id %q: got %q, want a freshly generated UUID", supplied, got)
		}
	}
}

func TestErrorBodyCarriesTheRequestID(t *testing.T) {
	h := newHarness(t, fakeHealth{live: true})
	rec := h.do(http.MethodGet, "/nope", "127.0.0.1:8080", nil)
	if body := decodeError(t, rec); body.Error.RequestID == "" || body.Error.RequestID != rec.Header().Get("X-Request-Id") {
		t.Errorf("requestId %q does not match header %q", body.Error.RequestID, rec.Header().Get("X-Request-Id"))
	}
}

func TestPanicIsContainedAndNotLeaked(t *testing.T) {
	h := newHarness(t, fakeHealth{panicOnLive: true})
	rec := h.do(http.MethodGet, "/api/v1/health/live", "127.0.0.1:8080", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if body := decodeError(t, rec); body.Error.Code != "internal_error" {
		t.Errorf("code = %q, want internal_error", body.Error.Code)
	}
	for name, text := range map[string]string{"response": rec.Body.String(), "log": h.logs.String()} {
		if strings.Contains(text, "boom-secret-value") {
			t.Errorf("panic value leaked into the %s: %s", name, text)
		}
		if strings.Contains(text, "goroutine ") || strings.Contains(text, ".go:") {
			t.Errorf("stack trace leaked into the %s: %s", name, text)
		}
	}
	if !strings.Contains(h.logs.String(), "panic") {
		t.Errorf("panic was not logged: %s", h.logs.String())
	}
	// The server keeps serving afterwards.
	if rec := h.do(http.MethodGet, "/api/v1/health/ready", "127.0.0.1:8080", nil); rec.Code != http.StatusServiceUnavailable && rec.Code != http.StatusOK {
		t.Errorf("follow-up status = %d", rec.Code)
	}
}

func TestAccessLogHasNoQueryOrHeaderSecrets(t *testing.T) {
	h := newHarness(t, fakeHealth{live: true})
	h.do(http.MethodGet, "/api/v1/health/live?token=query-secret", "127.0.0.1:8080", map[string]string{
		"Authorization": "Bearer header-secret",
		"Cookie":        "session=cookie-secret",
	})
	out := h.logs.String()
	for _, leaked := range []string{"query-secret", "header-secret", "cookie-secret"} {
		if strings.Contains(out, leaked) {
			t.Errorf("%q leaked into the access log: %s", leaked, out)
		}
	}
	for _, want := range []string{`"method":"GET"`, `"status":200`, `"request_id"`, `"route":"/api/v1/health/live"`} {
		if !strings.Contains(out, want) {
			t.Errorf("access log lacks %s: %s", want, out)
		}
	}
}

func TestAccessLogDoesNotRecordAnUnmatchedPath(t *testing.T) {
	h := newHarness(t, fakeHealth{live: true})
	h.do(http.MethodGet, "/forged%0Alevel=ERROR-secret-path", "127.0.0.1:8080", nil)
	if strings.Contains(h.logs.String(), "secret-path") {
		t.Errorf("unmatched path was logged: %s", h.logs.String())
	}
}
