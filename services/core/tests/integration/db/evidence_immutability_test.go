package db_test

import (
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/persistence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/evidence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/tests/integration/pgtest"
)

// The API and worker roles can add evidence and move its integrity state. They
// cannot rewrite an item, a check, the retention columns, or a compromise.
func TestTheDatabaseRefusesToRewriteEvidence(t *testing.T) {
	e := newEvEnv(t)
	it := e.item(t, "x", 0)
	e.insert(t, it)
	c, _, err := e.check(t, it, bad(it), evidence.TriggerManual)
	if err != nil {
		t.Fatal(err)
	}
	type attempt struct {
		name string
		sql  string
		args []any
	}
	attempts := []attempt{
		{"change the hash of an item", `UPDATE morningstar.evidence_items SET sha256 = $1 WHERE id = $2`, []any{pgtest.Random(32), it.ID()}},
		{"delete an item", `DELETE FROM morningstar.evidence_items WHERE id = $1`, []any{it.ID()}},
		{"rewrite a check", `UPDATE morningstar.evidence_integrity_checks SET result = 'match' WHERE id = $1`, []any{c.ID}},
		{"delete a check", `DELETE FROM morningstar.evidence_integrity_checks WHERE id = $1`, []any{c.ID}},
		{"purge", `UPDATE morningstar.evidence_status SET retention_state = 'purged', purged_at = now(), purged_by = $2 WHERE evidence_id = $1`, []any{it.ID(), e.b.UserID}},
		{"clear a compromise", `UPDATE morningstar.evidence_status SET integrity_state = 'ok' WHERE evidence_id = $1`, []any{it.ID()}},
		{"delete a status", `DELETE FROM morningstar.evidence_status WHERE evidence_id = $1`, []any{it.ID()}},
		{"truncate the items", `TRUNCATE morningstar.evidence_items`, nil},
	}
	for _, role := range []persistence.Role{persistence.RoleAPI, persistence.RoleWorker} {
		conn := e.c.Conn(t, role)
		for _, a := range attempts {
			if _, err := conn.Exec(t.Context(), a.sql, a.args...); err == nil {
				t.Errorf("the %s role was allowed to %s", role, a.name)
			}
		}
	}

	// The schema owner is refused too, by the triggers, so a mistake in a migration or a script cannot rewrite the record.
	// The first check is no longer the last one, so no foreign key protects it.
	if _, _, err := e.check(t, it, bad(it), evidence.TriggerManual); err != nil {
		t.Fatal(err)
	}
	spare := newID()
	owner := e.c.Owner(t)
	if _, err := owner.Exec(t.Context(), `INSERT INTO morningstar.evidence_items (id, program_id, assessment_id, kind, storage_key, sha256, size_bytes, content_type, captured_at, origin)
		VALUES ($1::uuid, $2, $3, 'note', $5::text, $4, 1, 'text/plain', now(), 'observed')`, spare, e.b.ProgramID, e.b.AssessmentID, pgtest.Random(32), spare); err != nil {
		t.Fatal(err)
	}
	for name, stmt := range map[string]struct {
		sql  string
		args []any
	}{
		"update an item":                   {`UPDATE morningstar.evidence_items SET size_bytes = 99`, nil},
		"update a check":                   {`UPDATE morningstar.evidence_integrity_checks SET result = 'match' WHERE id = $1`, []any{c.ID}},
		"delete a check no row protects":   {`DELETE FROM morningstar.evidence_integrity_checks WHERE id = $1`, []any{c.ID}},
		"clear the flag":                   {`UPDATE morningstar.evidence_status SET integrity_state = 'ok' WHERE evidence_id = $1`, []any{it.ID()}},
		"move the time":                    {`UPDATE morningstar.evidence_status SET compromised_at = now() + interval '1 day' WHERE evidence_id = $1`, []any{it.ID()}},
		"point the status at another item": {`UPDATE morningstar.evidence_status SET evidence_id = $2 WHERE evidence_id = $1`, []any{it.ID(), spare}},
		"truncate the checks":              {`TRUNCATE morningstar.evidence_integrity_checks`, nil},
	} {
		if _, err := owner.Exec(t.Context(), stmt.sql, stmt.args...); err == nil {
			t.Errorf("the schema owner was allowed to %s", name)
		}
	}
	rec, err := e.repo.Get(t.Context(), e.b.UserID, it.ID())
	if err != nil || rec.Status.Integrity != evidence.Compromised || rec.Item.Size() != 1 {
		t.Fatalf("after the refused rewrites: %+v, %v", rec, err)
	}
}

// Only the retention role changes retention, and a purge is final.
func TestOnlyTheRetentionRolePurgesAndAPurgeIsFinal(t *testing.T) {
	e := newEvEnv(t)
	it := e.item(t, "x", 0)
	e.insert(t, it)
	conn := e.c.Conn(t, persistence.RoleRetention)
	if _, err := conn.Exec(t.Context(), `UPDATE morningstar.evidence_status SET retention_state = 'purged', purged_at = now(), purged_by = $2, updated_at = now() WHERE evidence_id = $1`, it.ID(), e.b.UserID); err != nil {
		t.Fatalf("the retention role could not purge: %v", err)
	}
	if _, err := conn.Exec(t.Context(), `UPDATE morningstar.evidence_status SET retention_state = 'active', purged_at = NULL, purged_by = NULL, updated_at = now() WHERE evidence_id = $1`, it.ID()); err == nil {
		t.Error("a purge was undone")
	}
	if _, err := conn.Exec(t.Context(), `UPDATE morningstar.evidence_status SET integrity_state = 'ok' WHERE evidence_id = $1`, it.ID()); err == nil {
		t.Error("the retention role changed the integrity state")
	}
}
