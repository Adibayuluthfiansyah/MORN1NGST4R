// Package logging builds the structured logger of the process.
//
// The logger replaces the value of every attribute whose key (or whose
// enclosing group name) looks like a secret. The match is by key name only. It
// does not inspect values, so callers must still never pass secrets to a log
// call (security.md sections 6 and 13).
package logging

import (
	"io"
	"log/slog"
	"strings"
)

// Redacted replaces the value of a sensitive attribute.
const Redacted = "[REDACTED]"

// sensitive holds lower-case fragments. A key that contains one of them is
// redacted. Over-redaction is the safe failure.
var sensitive = []string{
	"password", "passwd", "secret", "token", "authorization", "cookie",
	"api_key", "apikey", "api-key", "credential", "private_key", "session",
	"recovery_code", "bearer",
}

func isSensitive(name string) bool {
	name = strings.ToLower(name)
	for _, fragment := range sensitive {
		if strings.Contains(name, fragment) {
			return true
		}
	}
	return false
}

// New returns a logger that writes to w in the given format ("json" or
// "text", anything else is treated as "json") at the given level.
func New(w io.Writer, level slog.Level, format string) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			redact := isSensitive(a.Key)
			for _, g := range groups {
				redact = redact || isSensitive(g)
			}
			if redact {
				return slog.String(a.Key, Redacted)
			}
			return a
		},
	}
	if format == "text" {
		return slog.New(slog.NewTextHandler(w, opts))
	}
	return slog.New(slog.NewJSONHandler(w, opts))
}
