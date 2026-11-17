package persistence_test

import (
	"strings"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/persistence"
)

func TestConnConfigUsesTheSocketAndTheRoleOnly(t *testing.T) {
	cfg, err := persistence.ConnConfig("/run/ms", persistence.RoleAPI)
	if err != nil {
		t.Fatalf("ConnConfig error: %v", err)
	}
	if cfg.Host != "/run/ms" || cfg.Port != 5432 {
		t.Errorf("host/port = %q/%d, want /run/ms/5432", cfg.Host, cfg.Port)
	}
	if cfg.User != "ms_api" || cfg.Database != "morningstar" {
		t.Errorf("user/database = %q/%q", cfg.User, cfg.Database)
	}
	if cfg.Password != "" {
		t.Error("a password is set")
	}
	if cfg.TLSConfig != nil || len(cfg.Fallbacks) != 0 {
		t.Error("TLS or a fallback is configured on a unix socket connection")
	}
	if cfg.RequireAuth != "none" {
		t.Errorf("RequireAuth = %q, want none", cfg.RequireAuth)
	}
}

// The environment must not redirect the connection, supply a password or add a
// fallback host. Anything that would is refused, never ignored silently.
func TestConnConfigIgnoresConnectionEnvironment(t *testing.T) {
	t.Setenv("PGHOST", "db.example.com")
	t.Setenv("PGUSER", "postgres")
	t.Setenv("PGDATABASE", "other")
	t.Setenv("PGPORT", "6543")
	t.Setenv("PGSSLMODE", "require")
	cfg, err := persistence.ConnConfig("/run/ms", persistence.RoleWorker)
	if err != nil {
		t.Fatalf("ConnConfig error: %v", err)
	}
	if cfg.Host != "/run/ms" || cfg.User != "ms_worker" || cfg.Database != "morningstar" || cfg.Port != 5432 || cfg.TLSConfig != nil {
		t.Errorf("environment changed the target: %s %s %s %d", cfg.Host, cfg.User, cfg.Database, cfg.Port)
	}
}

func TestConnConfigRefusesAPasswordFromTheEnvironment(t *testing.T) {
	t.Setenv("PGPASSWORD", "secret")
	_, err := persistence.ConnConfig("/run/ms", persistence.RoleAPI)
	if err == nil {
		t.Fatal("a password from PGPASSWORD was accepted")
	}
	if strings.Contains(err.Error(), "secret") {
		t.Errorf("the error leaks the password: %v", err)
	}
}

// Options from the environment can change the search path or other session
// settings of a connection. The SQL functions of the schema pin their own
// search path, but the adapter still refuses a setting it did not choose.
func TestConnConfigRefusesSessionOptionsFromTheEnvironment(t *testing.T) {
	for _, env := range []struct{ key, value string }{
		{"PGOPTIONS", "-c search_path=public"},
		{"PGOPTIONS", "-c role=ms_migrator"},
	} {
		t.Run(env.key+" "+env.value, func(t *testing.T) {
			t.Setenv(env.key, env.value)
			if _, err := persistence.ConnConfig("/run/ms", persistence.RoleAPI); err == nil {
				t.Error("a session option from the environment was accepted")
			}
			if _, err := persistence.NewPool(t.Context(), "/run/ms", persistence.RoleAPI, 2); err == nil {
				t.Error("a pool accepted a session option from the environment")
			}
		})
	}
}

func TestConnConfigRefusesBadInput(t *testing.T) {
	tests := []struct {
		name string
		dir  string
		role persistence.Role
	}{
		{"relative directory", "run/ms", persistence.RoleAPI},
		{"empty directory", "", persistence.RoleAPI},
		{"control character", "/run/m\ns", persistence.RoleAPI},
		{"unknown role", "/run/ms", persistence.Role("postgres")},
		{"empty role", "/run/ms", persistence.Role("")},
		{"role with an injected option", "/run/ms", persistence.Role("ms_api password=x")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := persistence.ConnConfig(tc.dir, tc.role); err == nil {
				t.Fatal("ConnConfig succeeded, want an error")
			}
		})
	}
}

// A directory with a quote, a backslash or a space must reach the driver as one
// value and not as extra connection options.
func TestConnConfigEscapesTheDirectory(t *testing.T) {
	dir := `/run/m s'x\y host=evil`
	cfg, err := persistence.ConnConfig(dir, persistence.RoleAPI)
	if err != nil {
		t.Fatalf("ConnConfig error: %v", err)
	}
	if cfg.Host != dir {
		t.Errorf("Host = %q, want %q", cfg.Host, dir)
	}
}

func TestNewPoolIsLazyAndHasABoundedSize(t *testing.T) {
	// No server listens here. A lazy pool is created without a connection.
	pool, err := persistence.NewPool(t.Context(), "/nonexistent-ms-dir", persistence.RoleAPI, 4)
	if err != nil {
		t.Fatalf("NewPool error: %v", err)
	}
	defer pool.Close()
	if got := pool.Config().MaxConns; got != 4 {
		t.Errorf("MaxConns = %d, want 4", got)
	}
	if err := pool.Ping(t.Context()); err == nil {
		t.Error("Ping succeeded without a server")
	}
}

// The migration tool keeps the default search path: goose creates its version
// table in the first schema of the path, and the application schema does not
// exist before the first migration.
func TestConnConfigSearchPath(t *testing.T) {
	m, err := persistence.ConnConfig("/run/ms", persistence.RoleMigrator)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := m.RuntimeParams["search_path"]; ok {
		t.Errorf("migrator search_path = %q, want unset", v)
	}
	a, err := persistence.ConnConfig("/run/ms", persistence.RoleAPI)
	if err != nil {
		t.Fatal(err)
	}
	if got := a.RuntimeParams["search_path"]; got != "morningstar,pg_catalog" {
		t.Errorf("api search_path = %q", got)
	}
}

func TestNewPoolRefusesANonPositiveSize(t *testing.T) {
	if _, err := persistence.NewPool(t.Context(), "/run/ms", persistence.RoleAPI, 0); err == nil {
		t.Error("a pool of size 0 was accepted")
	}
}
