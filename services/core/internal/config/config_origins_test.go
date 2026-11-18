package config_test

import (
	"reflect"
	"strings"
	"testing"
)

func TestLoadAllowedOriginsDefaultToTheWebUI(t *testing.T) {
	cfg, err := load(nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"http://127.0.0.1:3000", "http://localhost:3000"}
	if !reflect.DeepEqual(cfg.HTTP.AllowedOrigins, want) {
		t.Errorf("AllowedOrigins = %v, want %v", cfg.HTTP.AllowedOrigins, want)
	}
}

func TestLoadAllowedOriginsAreNormalized(t *testing.T) {
	cfg, err := load([]string{"MORNINGSTAR_HTTP_ALLOWED_ORIGINS= HTTP://LOCALHOST:4000 ,http://127.0.0.1:4000,http://localhost:4000"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"http://localhost:4000", "http://127.0.0.1:4000"}
	if !reflect.DeepEqual(cfg.HTTP.AllowedOrigins, want) {
		t.Errorf("AllowedOrigins = %v, want %v", cfg.HTTP.AllowedOrigins, want)
	}
}

func TestLoadRefusesBadOrigins(t *testing.T) {
	for _, raw := range []string{
		"", "*", "null", "localhost:3000", "http://localhost", "http://localhost:3000/", "http://localhost:3000/path",
		"http://example.com:3000", "http://10.0.0.5:3000", "ftp://localhost:3000", "http://user@localhost:3000",
		"http://localhost:3000?x=1", "http://localhost:3000,,http://localhost:3001", "http://[::1%lo]:3000", "http://localhost:0",
	} {
		t.Run(raw, func(t *testing.T) {
			_, err := load([]string{"MORNINGSTAR_HTTP_ALLOWED_ORIGINS=" + raw})
			if err == nil || !strings.Contains(err.Error(), "MORNINGSTAR_HTTP_ALLOWED_ORIGINS") {
				t.Errorf("error = %v, want one naming MORNINGSTAR_HTTP_ALLOWED_ORIGINS", err)
			}
		})
	}
}

func TestLoadRuntimeDirComesFromXDGOnly(t *testing.T) {
	cfg, err := load([]string{"XDG_RUNTIME_DIR=/run/user/1000"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth.RuntimeDir != "/run/user/1000" {
		t.Errorf("RuntimeDir = %q", cfg.Auth.RuntimeDir)
	}
	for _, bad := range []string{"", "relative/dir", "/run/user/1000/", "/run/../x"} {
		cfg, err := load([]string{"XDG_RUNTIME_DIR=" + bad})
		if err != nil {
			t.Fatalf("a bad XDG_RUNTIME_DIR must not stop the server (setup is refused instead): %v", err)
		}
		if cfg.Auth.RuntimeDir != "" {
			t.Errorf("RuntimeDir = %q for %q, want empty", cfg.Auth.RuntimeDir, bad)
		}
	}
	if _, err := load([]string{"MORNINGSTAR_RUNTIME_DIR=/tmp"}); err == nil {
		t.Error("an override of the runtime directory was accepted")
	}
}
