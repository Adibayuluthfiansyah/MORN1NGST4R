package logging_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/infrastructure/logging"
)

func TestRedactsSensitiveKeys(t *testing.T) {
	keys := []string{
		"password", "Password", "new_password", "passwd", "secret", "client_secret", "token", "access_token",
		"refresh_token", "session_id", "Authorization", "cookie", "Set-Cookie", "api_key", "apiKey", "api-key",
		"credential", "credentials", "private_key", "bootstrap_secret", "recovery_code",
	}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			var buf bytes.Buffer
			logger := logging.New(&buf, slog.LevelInfo, "json")
			logger.Info("event", key, "value-that-must-not-appear")
			if strings.Contains(buf.String(), "value-that-must-not-appear") {
				t.Errorf("value of %q was logged: %s", key, buf.String())
			}
			if !strings.Contains(buf.String(), logging.Redacted) {
				t.Errorf("%q was not replaced by %q: %s", key, logging.Redacted, buf.String())
			}
		})
	}
}

func TestRedactsInGroupsAndPreboundAttributes(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.New(&buf, slog.LevelInfo, "json").With("token", "bound-secret").WithGroup("req")
	logger.Info("event", slog.Group("hdr", slog.String("authorization", "group-secret")))
	out := buf.String()
	for _, leaked := range []string{"bound-secret", "group-secret"} {
		if strings.Contains(out, leaked) {
			t.Errorf("%q leaked: %s", leaked, out)
		}
	}
}

func TestKeepsOrdinaryKeys(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.New(&buf, slog.LevelInfo, "json")
	logger.Info("request", "method", "GET", "status", 200, "request_id", "abc")
	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("output is not JSON: %v: %s", err, buf.String())
	}
	if rec["method"] != "GET" || rec["status"] != float64(200) || rec["request_id"] != "abc" || rec["msg"] != "request" {
		t.Errorf("ordinary attributes were altered: %v", rec)
	}
}

func TestLevelFiltersAndTextFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.New(&buf, slog.LevelWarn, "text")
	logger.Info("hidden")
	logger.Warn("shown", "password", "x")
	out := buf.String()
	if strings.Contains(out, "hidden") {
		t.Errorf("info record was written at warn level: %s", out)
	}
	if !strings.Contains(out, "shown") || strings.Contains(out, "password=x") {
		t.Errorf("text output wrong: %s", out)
	}
}
