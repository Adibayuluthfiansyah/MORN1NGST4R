package config_test

import (
	"log/slog"
	"net/netip"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/config"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := config.Load(nil)
	if err != nil {
		t.Fatalf("Load(nil) error: %v", err)
	}
	if got, want := cfg.HTTP.Listen, netip.MustParseAddrPort("127.0.0.1:8080"); got != want {
		t.Errorf("Listen = %v, want %v", got, want)
	}
	if got, want := cfg.HTTP.AllowedHosts, []string{"127.0.0.1:8080", "localhost:8080"}; !reflect.DeepEqual(got, want) {
		t.Errorf("AllowedHosts = %v, want %v", got, want)
	}
	if cfg.Log.Level != slog.LevelInfo || cfg.Log.Format != "json" {
		t.Errorf("Log = %+v, want info/json", cfg.Log)
	}
	// docs/design/api/conventions.md: read-header, read, write, idle.
	if cfg.HTTP.ReadHeaderTimeout != 5*time.Second || cfg.HTTP.ReadTimeout != 15*time.Second ||
		cfg.HTTP.WriteTimeout != 30*time.Second || cfg.HTTP.IdleTimeout != 60*time.Second {
		t.Errorf("timeouts = %+v, want 5s/15s/30s/60s", cfg.HTTP)
	}
	if cfg.HTTP.ShutdownTimeout <= 0 {
		t.Errorf("ShutdownTimeout = %v, want > 0", cfg.HTTP.ShutdownTimeout)
	}
}

func TestLoadOverrides(t *testing.T) {
	cfg, err := config.Load([]string{
		"PATH=/usr/bin", // variables outside the MORNINGSTAR_ prefix are ignored
		"MORNINGSTAR_HTTP_LISTEN=[::1]:9090",
		"MORNINGSTAR_LOG_LEVEL=debug",
		"MORNINGSTAR_LOG_FORMAT=text",
	})
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if got, want := cfg.HTTP.Listen, netip.MustParseAddrPort("[::1]:9090"); got != want {
		t.Errorf("Listen = %v, want %v", got, want)
	}
	// The default allow-list follows the listen address.
	if got, want := cfg.HTTP.AllowedHosts, []string{"[::1]:9090", "localhost:9090"}; !reflect.DeepEqual(got, want) {
		t.Errorf("AllowedHosts = %v, want %v", got, want)
	}
	if cfg.Log.Level != slog.LevelDebug || cfg.Log.Format != "text" {
		t.Errorf("Log = %+v, want debug/text", cfg.Log)
	}
}

func TestLoadAllowedHostsAreNormalized(t *testing.T) {
	cfg, err := config.Load([]string{"MORNINGSTAR_HTTP_ALLOWED_HOSTS= LOCALHOST:8080 ,127.0.0.1:8080,localhost:8080"})
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if got, want := cfg.HTTP.AllowedHosts, []string{"localhost:8080", "127.0.0.1:8080"}; !reflect.DeepEqual(got, want) {
		t.Errorf("AllowedHosts = %v, want %v (trimmed, lower-cased, de-duplicated, order kept)", got, want)
	}
}

// Fail closed: every one of these must be refused, never repaired.
func TestLoadRefuses(t *testing.T) {
	tests := []struct {
		name    string
		environ []string
		wantKey string
	}{
		{"wildcard IPv4 listen", []string{"MORNINGSTAR_HTTP_LISTEN=0.0.0.0:8080"}, "MORNINGSTAR_HTTP_LISTEN"},
		{"wildcard IPv6 listen", []string{"MORNINGSTAR_HTTP_LISTEN=[::]:8080"}, "MORNINGSTAR_HTTP_LISTEN"},
		{"LAN address", []string{"MORNINGSTAR_HTTP_LISTEN=192.168.1.5:8080"}, "MORNINGSTAR_HTTP_LISTEN"},
		{"hostname instead of IP literal", []string{"MORNINGSTAR_HTTP_LISTEN=localhost:8080"}, "MORNINGSTAR_HTTP_LISTEN"},
		{"missing host", []string{"MORNINGSTAR_HTTP_LISTEN=:8080"}, "MORNINGSTAR_HTTP_LISTEN"},
		{"missing port", []string{"MORNINGSTAR_HTTP_LISTEN=127.0.0.1"}, "MORNINGSTAR_HTTP_LISTEN"},
		{"port zero", []string{"MORNINGSTAR_HTTP_LISTEN=127.0.0.1:0"}, "MORNINGSTAR_HTTP_LISTEN"},
		{"port out of range", []string{"MORNINGSTAR_HTTP_LISTEN=127.0.0.1:65536"}, "MORNINGSTAR_HTTP_LISTEN"},
		{"IPv4-mapped IPv6 loopback", []string{"MORNINGSTAR_HTTP_LISTEN=[::ffff:127.0.0.1]:8080"}, "MORNINGSTAR_HTTP_LISTEN"},
		{"empty listen", []string{"MORNINGSTAR_HTTP_LISTEN="}, "MORNINGSTAR_HTTP_LISTEN"},
		{"IPv6 loopback with a zone", []string{"MORNINGSTAR_HTTP_LISTEN=[::1%lo]:8080"}, "MORNINGSTAR_HTTP_LISTEN"},
		{"IPv6 loopback with a numeric zone", []string{"MORNINGSTAR_HTTP_LISTEN=[::1%1]:8080"}, "MORNINGSTAR_HTTP_LISTEN"},
		{"IPv6 loopback with an empty zone", []string{"MORNINGSTAR_HTTP_LISTEN=[::1%]:8080"}, "MORNINGSTAR_HTTP_LISTEN"},
		{"IPv6 loopback with an escaped zone", []string{"MORNINGSTAR_HTTP_LISTEN=[::1%25lo]:8080"}, "MORNINGSTAR_HTTP_LISTEN"},
		{"allowed host IPv6 with a zone", []string{"MORNINGSTAR_HTTP_ALLOWED_HOSTS=[::1%lo]:8080"}, "MORNINGSTAR_HTTP_ALLOWED_HOSTS"},
		{"empty allowed hosts", []string{"MORNINGSTAR_HTTP_ALLOWED_HOSTS="}, "MORNINGSTAR_HTTP_ALLOWED_HOSTS"},
		{"empty allowed host entry", []string{"MORNINGSTAR_HTTP_ALLOWED_HOSTS=localhost:8080,,127.0.0.1:8080"}, "MORNINGSTAR_HTTP_ALLOWED_HOSTS"},
		{"wildcard allowed host", []string{"MORNINGSTAR_HTTP_ALLOWED_HOSTS=*"}, "MORNINGSTAR_HTTP_ALLOWED_HOSTS"},
		{"allowed host without port", []string{"MORNINGSTAR_HTTP_ALLOWED_HOSTS=localhost"}, "MORNINGSTAR_HTTP_ALLOWED_HOSTS"},
		{"allowed host with scheme", []string{"MORNINGSTAR_HTTP_ALLOWED_HOSTS=http://localhost:8080"}, "MORNINGSTAR_HTTP_ALLOWED_HOSTS"},
		{"allowed host that is not loopback", []string{"MORNINGSTAR_HTTP_ALLOWED_HOSTS=example.com:8080"}, "MORNINGSTAR_HTTP_ALLOWED_HOSTS"},
		{"allowed host that is a LAN address", []string{"MORNINGSTAR_HTTP_ALLOWED_HOSTS=10.0.0.5:8080"}, "MORNINGSTAR_HTTP_ALLOWED_HOSTS"},
		{"allowed host port out of range", []string{"MORNINGSTAR_HTTP_ALLOWED_HOSTS=localhost:99999"}, "MORNINGSTAR_HTTP_ALLOWED_HOSTS"},
		{"unknown level", []string{"MORNINGSTAR_LOG_LEVEL=trace"}, "MORNINGSTAR_LOG_LEVEL"},
		{"upper-case level", []string{"MORNINGSTAR_LOG_LEVEL=INFO"}, "MORNINGSTAR_LOG_LEVEL"},
		{"unknown format", []string{"MORNINGSTAR_LOG_FORMAT=xml"}, "MORNINGSTAR_LOG_FORMAT"},
		{"misspelled variable is not ignored", []string{"MORNINGSTAR_HTTP_ALLOWD_HOSTS=localhost:8080"}, "MORNINGSTAR_HTTP_ALLOWD_HOSTS"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := config.Load(tc.environ)
			if err == nil {
				t.Fatal("Load succeeded, want an error")
			}
			if !strings.Contains(err.Error(), tc.wantKey) {
				t.Errorf("error %q does not name %s", err, tc.wantKey)
			}
		})
	}
}

// MORNINGSTAR_API_ORIGIN belongs to apps/web (next.config.ts). The Go server
// knows it only so that one shared environment can serve both processes. It
// must not change any Go setting.
func TestLoadIgnoresTheWebOnlyAPIOrigin(t *testing.T) {
	want, err := config.Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := config.Load([]string{"MORNINGSTAR_API_ORIGIN=http://127.0.0.1:9999"})
	if err != nil {
		t.Fatalf("Load refused the web-only MORNINGSTAR_API_ORIGIN: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MORNINGSTAR_API_ORIGIN changed the Go configuration:\n got  %+v\n want %+v", got, want)
	}
}

// The Go server does not validate the web value. apps/web owns that rule, so
// even a value the web side would refuse must not break the Go server.
func TestLoadDoesNotValidateTheWebOnlyAPIOrigin(t *testing.T) {
	if _, err := config.Load([]string{"MORNINGSTAR_API_ORIGIN=http://example.com:1"}); err != nil {
		t.Errorf("Load validated a web-only value: %v", err)
	}
}

// Recognising one web-only name must not weaken the strict prefix check.
func TestLoadStillRefusesNearMissesOfTheWebOnlyName(t *testing.T) {
	for _, key := range []string{
		"MORNINGSTAR_API_ORIGINS", "MORNINGSTAR_API_ORIGIN_", "MORNINGSTAR_API", "MORNINGSTAR_API_ORIGIN2",
		"MORNINGSTAR_WEB_API_ORIGIN", "MORNINGSTAR_NEXT_ANYTHING", "MORNINGSTAR_",
	} {
		t.Run(key, func(t *testing.T) {
			_, err := config.Load([]string{key + "=x"})
			if err == nil || !strings.Contains(err.Error(), key) {
				t.Errorf("Load(%s) error = %v, want an unknown-setting error naming it", key, err)
			}
		})
	}
	// Case matters: an environment variable name is case sensitive.
	if _, err := config.Load([]string{"MORNINGSTAR_api_origin=x"}); err == nil {
		t.Error("lower-case variant was accepted, want an unknown-setting error")
	}
}

func TestLoadReportsEveryProblem(t *testing.T) {
	_, err := config.Load([]string{"MORNINGSTAR_LOG_LEVEL=trace", "MORNINGSTAR_LOG_FORMAT=xml"})
	if err == nil {
		t.Fatal("Load succeeded, want an error")
	}
	for _, key := range []string{"MORNINGSTAR_LOG_LEVEL", "MORNINGSTAR_LOG_FORMAT"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error %q does not name %s", err, key)
		}
	}
}

// A rejected value is quoted so a control character cannot forge a log line.
func TestLoadQuotesRejectedValues(t *testing.T) {
	_, err := config.Load([]string{"MORNINGSTAR_LOG_LEVEL=info\nFAKE LOG LINE"})
	if err == nil {
		t.Fatal("Load succeeded, want an error")
	}
	if strings.Contains(err.Error(), "\n") {
		t.Errorf("error contains a raw newline: %q", err)
	}
}
