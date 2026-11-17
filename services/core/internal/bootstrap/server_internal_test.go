package bootstrap

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/config"
)

// The logger that NewServer builds must be the redacting one, and its output
// must go to the writer the caller gave. White-box on purpose: nothing a
// request can do makes the server log a secret-named attribute, so the only
// way to prove the wiring is to use the server's own logger.
func TestServerLoggerRedactsSecretNamedAttributes(t *testing.T) {
	cfg, err := config.Load([]string{"HOME=/home/test"})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	srv, err := NewServer(cfg, &out, WithoutDatabase())
	if err != nil {
		t.Fatal(err)
	}

	srv.logger.Info("probe event",
		"password", "pw-secret-value",
		"Authorization", "Bearer token-secret-value",
		"session_id", "sid-secret-value",
		"route", "/api/v1/health/live",
	)

	text := out.String()
	for _, secret := range []string{"pw-secret-value", "token-secret-value", "sid-secret-value"} {
		if strings.Contains(text, secret) {
			t.Errorf("%q reached the log: %s", secret, text)
		}
	}
	var rec map[string]any
	if err := json.Unmarshal(out.Bytes(), &rec); err != nil {
		t.Fatalf("log line is not JSON: %v: %s", err, text)
	}
	for _, key := range []string{"password", "Authorization", "session_id"} {
		if rec[key] != "[REDACTED]" {
			t.Errorf("%s = %v, want [REDACTED]", key, rec[key])
		}
	}
	if rec["route"] != "/api/v1/health/live" {
		t.Errorf("an ordinary attribute was altered: %v", rec["route"])
	}
}

// The http.Server error log shares the same redacting handler.
func TestServerErrorLogUsesTheRedactingHandler(t *testing.T) {
	cfg, err := config.Load([]string{"HOME=/home/test"})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	srv, err := NewServer(cfg, &out, WithoutDatabase())
	if err != nil {
		t.Fatal(err)
	}
	if srv.http.ErrorLog == nil {
		t.Fatal("http.Server.ErrorLog is nil, so net/http would log to stderr unredacted")
	}
	srv.http.ErrorLog.Print("http: example server error")
	if !strings.Contains(out.String(), "example server error") || !strings.Contains(out.String(), `"level":"WARN"`) {
		t.Errorf("the error log did not reach the structured writer: %s", out.String())
	}
}
