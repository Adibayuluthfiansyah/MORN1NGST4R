// Package persistence is the PostgreSQL adapter (pgx). It owns every SQL
// statement and every database connection of the application.
//
// The database is a per-user cluster that listens on a unix socket only and
// accepts peer authentication only (D-67). A connection therefore carries no
// password, no host name and no connection string. This package builds the
// connection from a socket directory and a role name, and refuses to run when
// the environment would add a password or change the target.
package persistence

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Role is a PostgreSQL login role of the application (DB-04). Each runtime
// connects as the role that has only the grants it needs.
type Role string

const (
	// RoleMigrator owns the schema. Only the migration tool uses it.
	RoleMigrator Role = "ms_migrator"
	// RoleAPI is the HTTP server process.
	RoleAPI Role = "ms_api"
	// RoleWorker is the background worker process.
	RoleWorker Role = "ms_worker"
	// RoleRetention is the explicit retention procedure.
	RoleRetention Role = "ms_retention"
)

// Database is the fixed database name.
const Database = "morningstar"

// socketPort is the port number that names the socket file (.s.PGSQL.5432).
const socketPort = 5432

var applicationName = map[Role]string{
	RoleMigrator:  "morningstar-migrator",
	RoleAPI:       "morningstar-api",
	RoleWorker:    "morningstar-worker",
	RoleRetention: "morningstar-retention",
}

// ConnConfig returns the connection settings for role over the unix socket in
// socketDir. It fails when the directory is not an absolute path without
// control characters, when the role is unknown, or when the environment
// supplies a password (PGPASSWORD, a password file).
func ConnConfig(socketDir string, role Role) (*pgx.ConnConfig, error) {
	connString, err := connString(socketDir, role)
	if err != nil {
		return nil, err
	}
	cfg, err := pgx.ParseConfig(connString)
	if err != nil {
		// The parser error can quote the connection string. Report no detail.
		return nil, errors.New("persistence: the connection settings are invalid")
	}
	if err := checkConn(cfg, socketDir, role); err != nil {
		return nil, err
	}
	return cfg, nil
}

// NewPool returns a pool of at most maxConns connections. The pool is lazy: no
// connection opens until the first use, so a database that is not running yet
// does not stop the process from starting.
func NewPool(ctx context.Context, socketDir string, role Role, maxConns int32) (*pgxpool.Pool, error) {
	if maxConns < 1 {
		return nil, errors.New("persistence: the pool size must be at least 1")
	}
	connString, err := connString(socketDir, role)
	if err != nil {
		return nil, err
	}
	cfg, err := pgxpool.ParseConfig(connString + " pool_max_conns=" + strconv.Itoa(int(maxConns)))
	if err != nil {
		return nil, errors.New("persistence: the connection settings are invalid")
	}
	if err := checkConn(cfg.ConnConfig, socketDir, role); err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.New("persistence: the pool could not be created")
	}
	return pool, nil
}

func connString(socketDir string, role Role) (string, error) {
	if _, ok := applicationName[role]; !ok {
		return "", errors.New("persistence: unknown role")
	}
	if socketDir == "" || socketDir[0] != '/' {
		return "", errors.New("persistence: the socket directory must be an absolute path")
	}
	for i := 0; i < len(socketDir); i++ {
		if socketDir[i] < 0x20 || socketDir[i] == 0x7f {
			return "", errors.New("persistence: the socket directory has a control character")
		}
	}
	// Every value is fixed except the directory, which is quoted. sslmode and
	// require_auth keep a unix socket connection plain and password-free.
	// passfile is an empty device so that ~/.pgpass is never read.
	parts := []string{
		"host=" + quote(socketDir),
		"port=" + strconv.Itoa(socketPort),
		"user=" + string(role),
		"dbname=" + Database,
		"sslmode=disable",
		"require_auth=none",
		"passfile=/dev/null",
		"application_name=" + applicationName[role],
	}
	if role != RoleMigrator {
		// The migration tool keeps the default path (see ConnConfig tests).
		parts = append(parts, "search_path=morningstar,pg_catalog")
	}
	return strings.Join(parts, " "), nil
}

// quote wraps v for the keyword/value connection syntax.
func quote(v string) string {
	r := strings.NewReplacer(`\`, `\\`, `'`, `\'`)
	return "'" + r.Replace(v) + "'"
}

// checkConn proves that nothing outside the arguments changed the target.
func checkConn(cfg *pgx.ConnConfig, socketDir string, role Role) error {
	switch {
	case cfg.Password != "":
		return errors.New("persistence: a password is configured, but the database accepts peer authentication only (check PGPASSWORD)")
	case cfg.Host != socketDir, cfg.User != string(role), cfg.Database != Database, int(cfg.Port) != socketPort:
		return fmt.Errorf("persistence: the environment changed the connection target")
	case cfg.TLSConfig != nil, len(cfg.Fallbacks) != 0:
		return errors.New("persistence: TLS or a fallback host is configured on a unix socket connection")
	}
	// Only the session settings of connString may be present. PGOPTIONS and a
	// service file can add others, such as a different role or search path.
	for name := range cfg.RuntimeParams {
		if name != "application_name" && name != "search_path" {
			return errors.New("persistence: the environment added a session option (check PGOPTIONS)")
		}
	}
	return nil
}
