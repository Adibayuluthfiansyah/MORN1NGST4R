// Package pgtest starts a throwaway per-user PostgreSQL 18 cluster for the
// integration tests. It runs the same scripts/db/pg-cluster.sh that a developer
// runs (D-67) and the same goose migrations that make db-migrate runs, so the
// tests exercise the real provisioning path: unix socket only, peer
// authentication, four login roles, no password.
//
// One cluster serves one test binary. Every test that needs a clean database
// calls Fresh, which recreates the database from a migrated template in a few
// hundred milliseconds.
//
// Without PostgreSQL 18 on the PATH the tests are skipped, unless
// MORNINGSTAR_REQUIRE_DB_TESTS=1, which makes the absence a failure. make
// test-db sets it.
package pgtest

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/persistence"
)

const templateDB = "morningstar_tpl"

// Cluster is a running throwaway cluster.
type Cluster struct {
	// Base is the directory that holds the data directory and the socket.
	Base string
	// SocketDir is the directory of the unix socket.
	SocketDir string

	mu    sync.Mutex
	owner *pgxpool.Pool
}

var (
	once    sync.Once
	cluster *Cluster
	setup   error
	skipMsg string
)

// Run is for TestMain: it runs the tests and then stops the cluster.
func Run(m *testing.M) int {
	code := m.Run()
	shutdown()
	return code
}

// Get returns the cluster of this test binary and starts it on first use.
func Get(t testing.TB) *Cluster {
	t.Helper()
	once.Do(start)
	if skipMsg != "" {
		if os.Getenv("MORNINGSTAR_REQUIRE_DB_TESTS") == "1" {
			t.Fatalf("PostgreSQL 18 is required: %s", skipMsg)
		}
		t.Skip(skipMsg)
	}
	if setup != nil {
		t.Fatalf("cluster setup failed: %v", setup)
	}
	return cluster
}

func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	// .../services/core/tests/integration/pgtest/pgtest.go
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "..")
}

// RepoRoot is the root of the repository.
func RepoRoot() string { return repoRoot() }

// MigrationsDir is the directory of the goose migrations.
func MigrationsDir() string {
	return filepath.Join(repoRoot(), "services", "core", "migrations")
}

func script() string { return filepath.Join(repoRoot(), "scripts", "db", "pg-cluster.sh") }

func start() {
	for _, tool := range []string{"initdb", "pg_ctl", "postgres", "psql", "bash"} {
		if _, err := exec.LookPath(tool); err != nil {
			skipMsg = tool + " is not on the PATH"
			return
		}
	}
	base, err := os.MkdirTemp("", "ms-pg-")
	if err != nil {
		setup = err
		return
	}
	c := &Cluster{Base: base, SocketDir: filepath.Join(base, "pg-run")}
	cluster = c
	for _, step := range []string{"init", "start", "bootstrap"} {
		if err := c.script(step); err != nil {
			setup = err
			return
		}
	}
	if err := c.migrateTemplate(); err != nil {
		setup = err
	}
}

func shutdown() {
	if cluster == nil {
		return
	}
	_ = cluster.script("stop")
	_ = os.RemoveAll(cluster.Base)
}

func (c *Cluster) script(args ...string) error {
	cmd := exec.Command("bash", append([]string{script()}, args...)...)
	cmd.Env = append(os.Environ(), "MORNINGSTAR_PG_BASE="+c.Base)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pg-cluster.sh %s: %w: %s", strings.Join(args, " "), err, out.String())
	}
	return nil
}

// adminConfig connects as the OS user, the cluster superuser, over the socket.
func (c *Cluster) adminConfig(database string) (*pgx.ConnConfig, error) {
	u, err := user.Current()
	if err != nil {
		return nil, err
	}
	return pgx.ParseConfig(fmt.Sprintf("host=%s user=%s dbname=%s sslmode=disable require_auth=none passfile=/dev/null", c.SocketDir, u.Username, database))
}

// Admin opens a superuser connection to the maintenance database.
func (c *Cluster) Admin(t testing.TB) *pgx.Conn {
	t.Helper()
	cfg, err := c.adminConfig("postgres")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.ConnectConfig(t.Context(), cfg)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	return conn
}

// migrateTemplate migrates the database that bootstrap created, with the goose
// library and as ms_migrator, and then keeps it as the template.
func (c *Cluster) migrateTemplate() error {
	ctx := context.Background()
	cfg, err := persistence.ConnConfig(c.SocketDir, persistence.RoleMigrator)
	if err != nil {
		return err
	}
	db := stdlib.OpenDB(*cfg)
	provider, err := goose.NewProvider(goose.DialectPostgres, db, os.DirFS(MigrationsDir()))
	if err != nil {
		_ = db.Close()
		return err
	}
	if _, err := provider.Up(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("migrate: %w", err)
	}
	if err := closeDB(db); err != nil {
		return err
	}
	admin, err := c.adminConfig("postgres")
	if err != nil {
		return err
	}
	conn, err := pgx.ConnectConfig(ctx, admin)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	_, err = conn.Exec(ctx, "CREATE DATABASE "+templateDB+" TEMPLATE morningstar OWNER ms_migrator")
	return err
}

func closeDB(db *sql.DB) error { return db.Close() }

// Fresh replaces the database with a newly migrated one and returns when it is
// ready. Pools opened through the cluster before are closed first by the test
// cleanups of the previous test.
func (c *Cluster) Fresh(t testing.TB) {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	ctx := t.Context()
	if c.owner != nil {
		c.owner.Close()
		c.owner = nil
	}
	admin := c.Admin(t)
	for _, stmt := range []string{
		"DROP DATABASE IF EXISTS morningstar WITH (FORCE)",
		"CREATE DATABASE morningstar TEMPLATE " + templateDB + " OWNER ms_migrator",
		"REVOKE ALL ON DATABASE morningstar FROM PUBLIC",
		"GRANT CONNECT ON DATABASE morningstar TO ms_api, ms_worker, ms_retention",
	} {
		if _, err := admin.Exec(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	c.reopenOwner(t)
}

// reopenOwner opens one small pool of the schema owner. It serves the seed
// helpers and the checks of a test, so that a test with many jobs does not open
// a connection each. The caller holds c.mu.
func (c *Cluster) reopenOwner(t testing.TB) {
	t.Helper()
	pool, err := persistence.NewPool(t.Context(), c.SocketDir, persistence.RoleMigrator, 4)
	if err != nil {
		t.Fatal(err)
	}
	c.owner = pool
	t.Cleanup(func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.owner == pool {
			pool.Close()
			c.owner = nil
		}
	})
}

// Owner returns the pool of the schema owner for the database of this test. It
// is available after Fresh and closed when the test ends.
func (c *Cluster) Owner(t testing.TB) *pgxpool.Pool {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.owner == nil {
		t.Fatal("Owner is available after Fresh")
	}
	return c.owner
}

// Conn opens a connection as role, through the same code the application uses.
func (c *Cluster) Conn(t testing.TB, role persistence.Role) *pgx.Conn {
	t.Helper()
	cfg, err := persistence.ConnConfig(c.SocketDir, role)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.ConnectConfig(t.Context(), cfg)
	if err != nil {
		t.Fatalf("connect as %s: %v", role, err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	return conn
}

// Pool opens a pool as role.
func (c *Cluster) Pool(t testing.TB, role persistence.Role, size int32) *pgxpool.Pool {
	t.Helper()
	pool, err := persistence.NewPool(t.Context(), c.SocketDir, role, size)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// Stop stops the cluster, for the tests of a database that is down.
func (c *Cluster) Stop(t testing.TB) {
	t.Helper()
	if err := c.script("stop"); err != nil {
		t.Fatal(err)
	}
}

// Start starts a stopped cluster and waits until it accepts connections.
func (c *Cluster) Start(t testing.TB) {
	t.Helper()
	if err := c.script("start"); err != nil {
		t.Fatal(err)
	}
}

// Eventually polls f until it returns true or the timeout passes.
func Eventually(t testing.TB, timeout time.Duration, f func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if f() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("condition not met in time")
}
