package config_test

import (
	"strings"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/config"
)

func TestLoadPathDefaultsFollowXDG(t *testing.T) {
	tests := []struct {
		name       string
		environ    []string
		wantSocket string
		wantState  string
	}{
		{"home only", []string{"HOME=/srv/ana"}, "/srv/ana/.local/share/morningstar/pg-run", "/srv/ana/.local/state/morningstar"},
		{"xdg dirs", []string{"HOME=/srv/ana", "XDG_DATA_HOME=/data", "XDG_STATE_HOME=/state"}, "/data/morningstar/pg-run", "/state/morningstar"},
		{"relative xdg is ignored", []string{"HOME=/srv/ana", "XDG_DATA_HOME=rel", "XDG_STATE_HOME=rel"}, "/srv/ana/.local/share/morningstar/pg-run", "/srv/ana/.local/state/morningstar"},
		{"xdg without home", []string{"XDG_DATA_HOME=/data", "XDG_STATE_HOME=/state"}, "/data/morningstar/pg-run", "/state/morningstar"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := config.Load(tc.environ)
			if err != nil {
				t.Fatalf("Load error: %v", err)
			}
			if cfg.Database.SocketDir != tc.wantSocket {
				t.Errorf("SocketDir = %q, want %q", cfg.Database.SocketDir, tc.wantSocket)
			}
			if cfg.State.Dir != tc.wantState {
				t.Errorf("State.Dir = %q, want %q", cfg.State.Dir, tc.wantState)
			}
		})
	}
}

func TestLoadPathOverrides(t *testing.T) {
	cfg, err := config.Load([]string{"MORNINGSTAR_DB_SOCKET_DIR=/run/ms", "MORNINGSTAR_STATE_DIR=/var/lib/ms", "MORNINGSTAR_EVIDENCE_DIR=/var/lib/ms/evidence"})
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.Database.SocketDir != "/run/ms" || cfg.State.Dir != "/var/lib/ms" {
		t.Errorf("got %q and %q", cfg.Database.SocketDir, cfg.State.Dir)
	}
}

// A directory setting is never repaired. The socket directory must stay short
// enough for a unix socket address.
func TestLoadRefusesBadPaths(t *testing.T) {
	long := "/" + strings.Repeat("a", 90)
	tests := []struct {
		name    string
		environ []string
		wantKey string
	}{
		{"no home and no xdg", nil, "MORNINGSTAR_DB_SOCKET_DIR"},
		{"relative home", []string{"HOME=home"}, "MORNINGSTAR_DB_SOCKET_DIR"},
		{"relative socket dir", []string{"MORNINGSTAR_DB_SOCKET_DIR=run/ms", "HOME=/h"}, "MORNINGSTAR_DB_SOCKET_DIR"},
		{"empty socket dir", []string{"MORNINGSTAR_DB_SOCKET_DIR=", "HOME=/h"}, "MORNINGSTAR_DB_SOCKET_DIR"},
		{"trailing slash", []string{"MORNINGSTAR_DB_SOCKET_DIR=/run/ms/", "HOME=/h"}, "MORNINGSTAR_DB_SOCKET_DIR"},
		{"dot elements", []string{"MORNINGSTAR_DB_SOCKET_DIR=/run/../ms", "HOME=/h"}, "MORNINGSTAR_DB_SOCKET_DIR"},
		{"socket dir over 90 bytes", []string{"MORNINGSTAR_DB_SOCKET_DIR=" + long, "HOME=/h"}, "MORNINGSTAR_DB_SOCKET_DIR"},
		{"control character", []string{"MORNINGSTAR_DB_SOCKET_DIR=/run/m\ts", "HOME=/h"}, "MORNINGSTAR_DB_SOCKET_DIR"},
		{"relative state dir", []string{"MORNINGSTAR_STATE_DIR=state", "HOME=/h"}, "MORNINGSTAR_STATE_DIR"},
		{"state dir control character", []string{"MORNINGSTAR_STATE_DIR=/s\x7f", "HOME=/h"}, "MORNINGSTAR_STATE_DIR"},
		{"default socket dir too long", []string{"HOME=" + long}, "MORNINGSTAR_DB_SOCKET_DIR"},
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

// The database is reached by socket and peer identity. These settings must not
// exist, so a connection string or password can never be supplied.
func TestLoadHasNoDatabaseCredentialSettings(t *testing.T) {
	for _, key := range []string{"MORNINGSTAR_DATABASE_URL", "MORNINGSTAR_DB_PASSWORD", "MORNINGSTAR_DB_HOST", "MORNINGSTAR_DB_DSN"} {
		if _, err := config.Load([]string{key + "=x", "HOME=/h"}); err == nil {
			t.Errorf("%s was accepted", key)
		}
	}
}
