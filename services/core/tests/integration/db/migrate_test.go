// Package db_test holds the M3 database tests that need a real PostgreSQL 18
// cluster: the migrations, the schema against the data dictionary, the socket
// and role provisioning (D-67), the grants (DB-04) and the append-only rules.
package db_test

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/persistence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/tests/integration/pgtest"
)

func TestMain(m *testing.M) { os.Exit(pgtest.Run(m)) }

func TestMigrationsApplyInOrder(t *testing.T) {
	c := pgtest.Get(t)
	c.Fresh(t)
	conn := c.Conn(t, persistence.RoleMigrator)
	rows, err := conn.Query(t.Context(), "SELECT version_id FROM public.goose_db_version WHERE is_applied ORDER BY version_id")
	if err != nil {
		t.Fatal(err)
	}
	got, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		t.Fatal(err)
	}
	want := []int64{0, 1, 2, 3, 4, 5, 6, 7, 9, 13, 14, 15, 16, 17, 18, 19}
	if len(got) != len(want) {
		t.Fatalf("applied versions = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("applied versions = %v, want %v", got, want)
		}
	}
}

// DB-02 and the data dictionary are the contract. A column that is missing,
// extra, of another type or with another nullability is a defect.
func TestSchemaMatchesTheDataDictionary(t *testing.T) {
	c := pgtest.Get(t)
	c.Fresh(t)
	conn := c.Conn(t, persistence.RoleMigrator)

	type col struct{ typ, null string }
	want := map[string]map[string]col{}
	text, err := os.ReadFile(filepath.Join(pgtest.RepoRoot(), "docs", "design", "database", "data-dictionary.md"))
	if err != nil {
		t.Fatal(err)
	}
	domain := regexp.MustCompile(`Domain: (\w+)\.`)
	m3 := map[string]bool{"identity": true, "policy": true, "scope": true, "assessment": true, "authorization": true, "execution": true, "audit": true, "evidence": true}
	heading := regexp.MustCompile("(?m)^### `(\\w+)`$")
	locs := heading.FindAllStringSubmatchIndex(string(text), -1)
	var sections [][]string
	for i, loc := range locs {
		end := len(text)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		sections = append(sections, []string{"", string(text[loc[2]:loc[3]]), string(text[loc[1]:end])})
	}
	for _, s := range sections {
		d := domain.FindStringSubmatch(s[2])
		if d == nil || !m3[d[1]] {
			continue
		}
		cols := map[string]col{}
		for _, line := range strings.Split(s[2], "\n") {
			if !strings.HasPrefix(line, "| `") {
				continue
			}
			cells := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
			if len(cells) < 8 {
				t.Fatalf("table %s: malformed row %q", s[1], line)
			}
			cols[strings.Trim(strings.TrimSpace(cells[0]), "`")] = col{
				typ:  strings.Trim(strings.TrimSpace(cells[1]), "`"),
				null: strings.TrimSpace(cells[2]),
			}
		}
		want[s[1]] = cols
	}
	if len(want) != 36 {
		t.Fatalf("the data dictionary has %d tables in the domains of M3 and M6, want 36", len(want))
	}

	rows, err := conn.Query(t.Context(), `
		SELECT c.relname, a.attname,
		       replace(replace(format_type(a.atttypid, a.atttypmod), 'timestamp with time zone', 'timestamptz'), 'character varying', 'varchar'),
		       CASE WHEN a.attnotnull THEN 'NOT NULL' ELSE 'NULL' END
		  FROM pg_class c
		  JOIN pg_namespace n ON n.oid = c.relnamespace AND n.nspname = 'morningstar'
		  JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped
		 WHERE c.relkind = 'r'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	have := map[string]map[string]col{}
	for rows.Next() {
		var tbl, name, typ, null string
		if err := rows.Scan(&tbl, &name, &typ, &null); err != nil {
			t.Fatal(err)
		}
		if have[tbl] == nil {
			have[tbl] = map[string]col{}
		}
		have[tbl][name] = col{typ, null}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	var problems []string
	for tbl, cols := range want {
		got, ok := have[tbl]
		if !ok {
			problems = append(problems, "missing table "+tbl)
			continue
		}
		for name, w := range cols {
			g, ok := got[name]
			switch {
			case !ok:
				problems = append(problems, "missing column "+tbl+"."+name)
			case g != w:
				problems = append(problems, "column "+tbl+"."+name+": have "+g.typ+" "+g.null+", dictionary "+w.typ+" "+w.null)
			}
		}
		for name := range got {
			if _, ok := cols[name]; !ok {
				problems = append(problems, "extra column "+tbl+"."+name)
			}
		}
	}
	for tbl := range have {
		if _, ok := want[tbl]; !ok {
			problems = append(problems, "table "+tbl+" is not in the M3 and M6 domains of the dictionary")
		}
	}
	sort.Strings(problems)
	for _, p := range problems {
		t.Error(p)
	}
}

// D-67: the cluster is reachable by a unix socket in an owner-only directory,
// accepts peer authentication only, and holds no password.
func TestSocketAndAuthenticationAreOwnerOnly(t *testing.T) {
	c := pgtest.Get(t)
	c.Fresh(t)

	for _, p := range []string{c.Base, c.SocketDir, filepath.Join(c.Base, "pg")} {
		st, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if mode := st.Mode().Perm(); mode != 0o700 {
			t.Errorf("%s mode = %o, want 700", p, mode)
		}
	}
	sock, err := os.Stat(filepath.Join(c.SocketDir, ".s.PGSQL.5432"))
	if err != nil {
		t.Fatal(err)
	}
	if mode := sock.Mode().Perm(); mode != 0o700 {
		t.Errorf("socket mode = %o, want 700", mode)
	}
	if sock.Mode()&os.ModeSocket == 0 {
		t.Error("the socket path is not a socket")
	}

	admin := c.Admin(t)
	for setting, want := range map[string]string{"listen_addresses": "", "unix_socket_permissions": "0700", "password_encryption": "scram-sha-256"} {
		var got string
		if err := admin.QueryRow(t.Context(), "SELECT current_setting($1)", setting).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%s = %q, want %q", setting, got, want)
		}
	}

	hba, err := os.ReadFile(filepath.Join(c.Base, "pg", "pg_hba.conf"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(hba), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !regexp.MustCompile(`^local\s+all\s+all\s+peer\b`).MatchString(line) {
			t.Errorf("pg_hba.conf has a rule that is not local peer: %q", line)
		}
	}

	// The four roles exist, can log in, hold no password and no extra privilege.
	rows, err := admin.Query(t.Context(), `
		SELECT rolname, rolcanlogin, rolsuper, rolcreatedb, rolcreaterole, rolreplication, rolbypassrls, rolpassword IS NOT NULL
		  FROM pg_authid WHERE rolname LIKE 'ms\_%' ORDER BY rolname`)
	if err != nil {
		t.Fatal(err)
	}
	type role struct {
		name                                                  string
		login, super, createdb, createrole, repl, bypass, pwd bool
	}
	roles, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (role, error) {
		var x role
		err := r.Scan(&x.name, &x.login, &x.super, &x.createdb, &x.createrole, &x.repl, &x.bypass, &x.pwd)
		return x, err
	})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, r := range roles {
		names = append(names, r.name)
		if !r.login || r.super || r.createdb || r.createrole || r.repl || r.bypass || r.pwd {
			t.Errorf("role %+v has the wrong attributes", r)
		}
	}
	if got := strings.Join(names, ","); got != "ms_api,ms_migrator,ms_retention,ms_worker" {
		t.Errorf("roles = %s", got)
	}

	// An identity that the peer map does not allow cannot connect.
	cfg, err := pgx.ParseConfig("host=" + c.SocketDir + " user=postgres dbname=morningstar sslmode=disable passfile=/dev/null")
	if err != nil {
		t.Fatal(err)
	}
	if conn, err := pgx.ConnectConfig(context.Background(), cfg); err == nil {
		_ = conn.Close(context.Background())
		t.Error("a connection as an unmapped role succeeded")
	}
}
