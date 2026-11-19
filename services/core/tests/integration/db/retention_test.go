package db_test

import (
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/persistence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/application/retention"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/audit"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/infrastructure/ids"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/tests/integration/pgtest"
)

// The retention of security-operational rows (migration 00019, TD-M3-03 and
// TD-M4-02): the database enforces the floor and the role, and the purge event
// is written in the transaction of the purge.

func (e *env) oldSession(t *testing.T, age time.Duration) string {
	t.Helper()
	var id string
	if err := e.c.Owner(t).QueryRow(t.Context(), `INSERT INTO morningstar.sessions (id, user_id, token_hash, csrf_token_hash, created_at, last_seen_at, idle_expires_at, absolute_expires_at)
		VALUES (uuidv7(), $1, $2, $3, now() - $4::interval, now() - $4::interval, now() - $4::interval + interval '1 hour', now() - $4::interval + interval '8 hours') RETURNING id`,
		e.b.UserID, pgtest.Random(32), pgtest.Random(32), age.String()).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (e *env) oldGrant(t *testing.T, session string, age time.Duration) string {
	t.Helper()
	var id string
	if err := e.c.Owner(t).QueryRow(t.Context(), `INSERT INTO morningstar.reauth_grants (id, user_id, session_id, purpose, subject_hash, created_at, expires_at)
		VALUES (uuidv7(), $1, $2, 'verification', $3, now() - $4::interval, now() - $4::interval + interval '1 minute') RETURNING id`,
		e.b.UserID, session, pgtest.Random(32), age.String()).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (e *env) attempt(t *testing.T, age time.Duration) {
	t.Helper()
	if _, err := e.c.Owner(t).Exec(t.Context(), `INSERT INTO morningstar.login_attempts (id, kind, source_key, succeeded, attempted_at) VALUES (uuidv7(), 'login', 'k', false, now() - $1::interval)`, age.String()); err != nil {
		t.Fatal(err)
	}
}

func (e *env) retentionService(t *testing.T) *retention.Service {
	t.Helper()
	pool := e.c.Pool(t, persistence.RoleRetention, 4)
	sink, err := persistence.NewAuditSink(pool, audit.RetryPolicy{MaxAttempts: 2, Delays: []time.Duration{time.Millisecond}, AttemptTimeout: 2 * time.Second}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return retention.New(retention.Deps{Repo: persistence.NewRetention(pool), Tx: persistence.NewTransactor(pool), Audit: sink, NewID: ids.New})
}

const day = 24 * time.Hour

func (e *env) scalar(t *testing.T, sql string, args ...any) string {
	t.Helper()
	var s string
	if err := e.c.Owner(t).QueryRow(t.Context(), `SELECT (`+sql+`)::text`, args...).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestTheRetentionFunctionsRefuseACutoffYoungerThanThirtyDays(t *testing.T) {
	e := newEnv(t)
	r := persistence.NewRetention(e.c.Pool(t, persistence.RoleRetention, 2))
	for _, age := range []time.Duration{0, 29 * day, 30*day - time.Hour} {
		if _, err := r.PurgeSessions(t.Context(), time.Now().Add(-age)); err == nil {
			t.Errorf("sessions: a cutoff %v old was accepted", age)
		}
		if _, err := r.PurgeLoginAttempts(t.Context(), time.Now().Add(-age)); err == nil {
			t.Errorf("login attempts: a cutoff %v old was accepted", age)
		}
	}
	if _, err := r.PurgeSessions(t.Context(), time.Now().Add(-31*day)); err != nil {
		t.Errorf("a cutoff 31 days old: %v", err)
	}
}

func TestOnlyTheRetentionRoleMayRunThePurgeFunctions(t *testing.T) {
	e := newEnv(t)
	for _, role := range []persistence.Role{persistence.RoleAPI, persistence.RoleWorker} {
		conn := e.c.Conn(t, role)
		if _, err := conn.Exec(t.Context(), `SELECT * FROM morningstar.retention_purge_sessions(now() - interval '40 days')`); err == nil {
			t.Errorf("%s ran the session purge", role)
		}
		if _, err := conn.Exec(t.Context(), `SELECT morningstar.retention_purge_login_attempts(now() - interval '40 days')`); err == nil {
			t.Errorf("%s ran the login attempt purge", role)
		}
		if _, err := conn.Exec(t.Context(), `DELETE FROM morningstar.login_attempts`); err == nil {
			t.Errorf("%s deleted login attempts", role)
		}
	}
	if _, err := e.c.Conn(t, persistence.RoleRetention).Exec(t.Context(), `DELETE FROM morningstar.login_attempts`); err == nil {
		t.Error("the retention role deleted login attempts outside the function")
	}
	if _, err := e.c.Owner(t).Exec(t.Context(), `UPDATE morningstar.login_attempts SET succeeded = true`); err != nil {
		t.Logf("owner update: %v", err)
	}
}

func TestThePurgeRemovesOldRowsAndKeepsTheRest(t *testing.T) {
	e := newEnv(t)
	oldS := e.oldSession(t, 45*day)
	youngS := e.oldSession(t, 5*day)
	oldG := e.oldGrant(t, oldS, 45*day)
	e.attempt(t, 45*day)
	e.attempt(t, 5*day)

	// A grant that an approval names, and its session, stay although they are old.
	a := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{Risk: "intrusive"})
	var keptSession, keptGrant string
	if err := e.c.Owner(t).QueryRow(t.Context(), `SELECT g.session_id, g.id FROM morningstar.approvals x JOIN morningstar.reauth_grants g ON g.id = x.reauth_grant_id WHERE x.id = $1`, a.ID).Scan(&keptSession, &keptGrant); err != nil {
		t.Fatal(err)
	}
	if _, err := e.c.Owner(t).Exec(t.Context(), `UPDATE morningstar.reauth_grants SET created_at = now() - interval '45 days', expires_at = now() - interval '45 days' + interval '1 minute' WHERE id = $1`, keptGrant); err != nil {
		t.Fatal(err)
	}
	if _, err := e.c.Owner(t).Exec(t.Context(), `UPDATE morningstar.sessions SET created_at = now() - interval '46 days', idle_expires_at = now() - interval '45 days', absolute_expires_at = now() - interval '44 days' WHERE id = $1`, keptSession); err != nil {
		t.Fatal(err)
	}

	got, err := e.retentionService(t).PurgeOperational(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got.Grants != 1 || got.Sessions != 1 || got.LoginAttempts != 1 {
		t.Errorf("counts = %+v, want one grant, one session and one attempt", got)
	}
	exists := func(sql, id string) bool { return e.scalar(t, sql, id) == "1" }
	for name, c := range map[string]struct {
		sql, id string
		want    bool
	}{
		"old session":      {`SELECT count(*) FROM morningstar.sessions WHERE id = $1`, oldS, false},
		"old grant":        {`SELECT count(*) FROM morningstar.reauth_grants WHERE id = $1`, oldG, false},
		"young session":    {`SELECT count(*) FROM morningstar.sessions WHERE id = $1`, youngS, true},
		"referenced grant": {`SELECT count(*) FROM morningstar.reauth_grants WHERE id = $1`, keptGrant, true},
		"its session":      {`SELECT count(*) FROM morningstar.sessions WHERE id = $1`, keptSession, true},
	} {
		if exists(c.sql, c.id) != c.want {
			t.Errorf("%s: present = %v, want %v", name, !c.want, c.want)
		}
	}
	if n := e.scalar(t, `SELECT count(*) FROM morningstar.login_attempts`); n != "1" {
		t.Errorf("login attempts left = %s, want the young one", n)
	}
	if n := e.scalar(t, `SELECT count(*) FROM morningstar.audit_events WHERE action = 'retention.purged' AND outcome = 'success'`); n != "1" {
		t.Errorf("retention.purged events = %s, want 1", n)
	}
}

// Even the owner of the schema cannot delete a login attempt by hand: the only
// path is the purge function, which names itself in a transaction-local setting.
func TestLoginAttemptsCannotBeDeletedOutsideThePurgeFunction(t *testing.T) {
	e := newEnv(t)
	e.attempt(t, 45*day)
	owner := e.c.Owner(t)
	if _, err := owner.Exec(t.Context(), `DELETE FROM morningstar.login_attempts`); err == nil {
		t.Error("the owner deleted login attempts directly")
	}
	tx, err := owner.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context()) //nolint:errcheck
	if _, err := tx.Exec(t.Context(), `SELECT set_config('morningstar.retention_path', 'something_else', true)`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `DELETE FROM morningstar.login_attempts`); err == nil {
		t.Error("a delete passed with another path name")
	}
	if n := e.scalar(t, `SELECT count(*) FROM morningstar.login_attempts`); n != "1" {
		t.Errorf("login attempts = %s, want 1", n)
	}
}

func TestAPurgeWithNothingToRemoveWritesNoEventAndAuditRowsAreUntouched(t *testing.T) {
	e := newEnv(t)
	svc := e.retentionService(t)
	if got, err := svc.PurgeOperational(t.Context()); err != nil || !got.None() {
		t.Fatalf("PurgeOperational = %+v, %v", got, err)
	}
	if n := e.scalar(t, `SELECT count(*) FROM morningstar.audit_events WHERE action = 'retention.purged'`); n != "0" {
		t.Errorf("events = %s, want none", n)
	}
	// An audit row stays refused for every role, including the one that may now insert.
	e.attempt(t, 45*day)
	if _, err := svc.PurgeOperational(t.Context()); err != nil {
		t.Fatal(err)
	}
	conn := e.c.Conn(t, persistence.RoleRetention)
	if _, err := conn.Exec(t.Context(), `DELETE FROM morningstar.audit_events`); err == nil {
		t.Error("the retention role deleted audit rows")
	}
	if _, err := conn.Exec(t.Context(), `UPDATE morningstar.audit_events SET reason = 'x'`); err == nil {
		t.Error("the retention role updated audit rows")
	}
}
