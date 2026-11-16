package router_test

import (
	"net/http"
	"strings"
	"testing"
)

// A request that panics in the handler is still access-logged, once, as 500,
// and the panic is logged separately. Neither record carries the panic value.
func TestAccessLogRecordsAPanickedRequestAs500(t *testing.T) {
	h := newHarness(t, fakeHealth{panicOnLive: true})
	rec := h.do(http.MethodGet, "/api/v1/health/live", "127.0.0.1:8080", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	out := h.logs.String()
	if n := strings.Count(out, `"msg":"request"`); n != 1 {
		t.Errorf("%d access records, want exactly 1: %s", n, out)
	}
	if !strings.Contains(out, `"status":500`) {
		t.Errorf("the access record does not say 500: %s", out)
	}
	if !strings.Contains(out, `"msg":"panic recovered"`) {
		t.Errorf("no panic record: %s", out)
	}
	id := rec.Header().Get("X-Request-Id")
	if strings.Count(out, id) < 2 {
		t.Errorf("request id %q is not in both records: %s", id, out)
	}
	if strings.Contains(out, "boom-secret-value") {
		t.Errorf("panic value leaked: %s", out)
	}
}
