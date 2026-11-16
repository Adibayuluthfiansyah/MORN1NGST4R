package router

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// These white-box tests use the unexported build seam, which inserts handlers
// directly after the Recovery middleware. They prove that Recovery is the
// outermost application middleware.

type okHealth struct{}

func (okHealth) Live() bool                 { return true }
func (okHealth) Ready(context.Context) bool { return true }

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func serve(t *testing.T, h http.Handler, host string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil)
	req.Host = host
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func jsonLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func deps(logger *slog.Logger) Deps {
	return Deps{Health: okHealth{}, AllowedHosts: []string{"127.0.0.1:8080"}, Logger: logger}
}

func TestRecoveryWrapsEveryApplicationMiddleware(t *testing.T) {
	var logs bytes.Buffer
	boom := func(*gin.Context) { panic("middleware-secret-value") }
	h := build(deps(jsonLogger(&logs)), boom)

	rec := serve(t, h, "127.0.0.1:8080")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"code":"internal_error"`) {
		t.Errorf("body is not the internal_error envelope: %s", rec.Body.String())
	}
	// The panic happened before the request-id and header middleware ran.
	// Recovery must still produce a complete, well-formed response.
	id := rec.Header().Get("X-Request-Id")
	if !uuidRe.MatchString(id) {
		t.Errorf("X-Request-Id = %q, want a UUID even when a middleware panicked first", id)
	}
	if !strings.Contains(rec.Body.String(), id) {
		t.Errorf("body does not carry the request id %q: %s", id, rec.Body.String())
	}
	for name, want := range map[string]string{
		"Cache-Control": "no-store", "X-Content-Type-Options": "nosniff", "X-Frame-Options": "DENY",
		"Content-Security-Policy": "default-src 'none'; frame-ancestors 'none'",
	} {
		if got := rec.Header().Values(name); len(got) != 1 || got[0] != want {
			t.Errorf("%s = %q, want exactly %q", name, got, want)
		}
	}
	for what, text := range map[string]string{"response": rec.Body.String(), "log": logs.String()} {
		if strings.Contains(text, "middleware-secret-value") {
			t.Errorf("panic value leaked into the %s: %s", what, text)
		}
	}
	if !strings.Contains(logs.String(), "panic recovered") {
		t.Errorf("the panic was not logged: %s", logs.String())
	}
}

func TestPanicAfterTheResponseStartedKeepsTheResponseIntact(t *testing.T) {
	var logs bytes.Buffer
	half := func(c *gin.Context) {
		c.Status(http.StatusAccepted)
		_, _ = c.Writer.WriteString("partial")
		panic("late")
	}
	rec := serve(t, build(deps(jsonLogger(&logs)), half), "127.0.0.1:8080")
	if rec.Code != http.StatusAccepted || rec.Body.String() != "partial" {
		t.Errorf("response = %d %q, want the already written 202 \"partial\" without an appended envelope", rec.Code, rec.Body.String())
	}
	if !strings.Contains(logs.String(), "panic recovered") {
		t.Errorf("the panic was not logged: %s", logs.String())
	}
}

type panicHandler struct{}

func (panicHandler) Enabled(context.Context, slog.Level) bool  { return true }
func (panicHandler) Handle(context.Context, slog.Record) error { panic("logger-failure") }
func (h panicHandler) WithAttrs([]slog.Attr) slog.Handler      { return h }
func (h panicHandler) WithGroup(string) slog.Handler           { return h }

// A logging failure must neither crash the request nor corrupt the response.
func TestAFailingLoggerDoesNotBreakTheResponse(t *testing.T) {
	h := build(deps(slog.New(panicHandler{})))
	rec := serve(t, h, "127.0.0.1:8080")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"status":"ok"}` {
		t.Errorf("response = %d %q, want the normal 200 answer", rec.Code, rec.Body.String())
	}

	// Same with a handler panic, where Recovery itself has to log.
	h = build(deps(slog.New(panicHandler{})), func(*gin.Context) { panic("x") })
	rec = serve(t, h, "127.0.0.1:8080")
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), `"code":"internal_error"`) {
		t.Errorf("response = %d %q, want the internal_error envelope", rec.Code, rec.Body.String())
	}
}

func TestBuildWithoutHooksEqualsNew(t *testing.T) {
	var logs bytes.Buffer
	rec := serve(t, New(deps(jsonLogger(&logs))), "127.0.0.1:8080")
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}
