package db_test

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/persistence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/approval"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/operation"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/scope"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/tests/integration/pgtest"
)

const (
	sqlstateRestrict  = "23001" // restrict_violation, raised by the append-only triggers
	sqlstateIntegrity = "23000" // integrity_constraint_violation, raised by the deferred parity triggers
	sqlstateNoRight   = "42501" // insufficient_privilege
)

func pgCode(err error) string {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

func wantCode(t *testing.T, what string, err error, code string) {
	t.Helper()
	if err == nil {
		t.Errorf("%s: succeeded, want SQLSTATE %s", what, code)
		return
	}
	if got := pgCode(err); got != code {
		t.Errorf("%s: SQLSTATE %q (%v), want %s", what, got, err, code)
	}
}

// DB-02: a CHECK list in the schema must equal the constants of the Go domain
// that the same values come from. A value added on one side only is a defect.
func TestCheckConstraintsMatchTheGoConstants(t *testing.T) {
	c := pgtest.Get(t)
	c.Fresh(t)
	conn := c.Owner(t)

	checkList := func(table, column string) []string {
		t.Helper()
		var def string
		err := conn.QueryRow(t.Context(), `
			SELECT pg_get_constraintdef(k.oid) FROM pg_constraint k
			 WHERE k.conrelid = $1::regclass AND k.contype = 'c' AND pg_get_constraintdef(k.oid) ~ ('^CHECK \(\(' || $2 || ' = ANY')`,
			"morningstar."+table, column).Scan(&def)
		if err != nil {
			t.Fatalf("%s.%s: %v", table, column, err)
		}
		var out []string
		for _, m := range regexp.MustCompile(`'([A-Za-z0-9_]+)'::text`).FindAllStringSubmatch(def, -1) {
			out = append(out, m[1])
		}
		sort.Strings(out)
		return out
	}
	same := func(name string, have, want []string) {
		t.Helper()
		sort.Strings(want)
		if strings.Join(have, ",") != strings.Join(want, ",") {
			t.Errorf("%s: schema %v, Go %v", name, have, want)
		}
	}

	same("approvals.status", checkList("approvals", "status"),
		[]string{approval.StatusPending.String(), approval.StatusApproved.String(), approval.StatusRejected.String(), approval.StatusRevoked.String()})
	same("approvals.risk_class", checkList("approvals", "risk_class"),
		[]string{operation.ClassLowImpact.String(), operation.ClassIntrusive.String()})
	same("operations.risk_class", checkList("operations", "risk_class"),
		[]string{operation.ClassPassive.String(), operation.ClassLowImpact.String(), operation.ClassIntrusive.String()})
	same("policy_decisions.outcome", checkList("policy_decisions", "outcome"),
		[]string{scope.OutcomeAllow.String(), scope.OutcomeRequireApproval.String(), scope.OutcomeDeny.String()})

	// The schema also holds E4 (promotion of a discovered asset). Its Go
	// constant arrives with the asset domain. Until then E4 is the only extra.
	points := checkList("policy_decisions", "point")
	goPoints := map[string]bool{scope.PointE1.String(): true, scope.PointE2.String(): true, scope.PointE3.String(): true}
	for _, p := range points {
		if !goPoints[p] && p != "E4" {
			t.Errorf("policy_decisions.point has %q, which Go does not know", p)
		}
		delete(goPoints, p)
	}
	for p := range goPoints {
		t.Errorf("Go has decision point %q and the schema does not", p)
	}
}

// DB-17: these tables are append-only. The trigger set is checked in the
// catalog, and the behavior is checked on tables that hold rows.
func TestAppendOnlyTriggersExist(t *testing.T) {
	c := pgtest.Get(t)
	c.Fresh(t)
	conn := c.Owner(t)

	appendOnly := []string{"login_attempts", "program_rules_versions", "scope_rules", "lab_registration_ranges", "target_validation_targets",
		"assessment_transitions", "operations", "policy_decisions", "approval_events", "approval_uses", "job_transitions"}
	updatable := map[string]string{ // tables that allow UPDATE of a few columns and never DELETE
		"user_credentials": "forbid_update_except", "policy_settings": "forbid_update_except", "scope_versions": "forbid_update_except",
		"lab_registrations": "forbid_update_except", "target_validations": "forbid_update_except", "job_attempts": "forbid_update_except",
	}
	rows, err := conn.Query(t.Context(), `
		SELECT c.relname, t.tgname, p.proname, t.tgtype, t.tgenabled::text
		  FROM pg_trigger t JOIN pg_class c ON c.oid = t.tgrelid JOIN pg_namespace n ON n.oid = c.relnamespace AND n.nspname = 'morningstar'
		  JOIN pg_proc p ON p.oid = t.tgfoid WHERE NOT t.tgisinternal`)
	if err != nil {
		t.Fatal(err)
	}
	type trig struct {
		table, name, fn string
		kind            int16
		enabled         string
	}
	all, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (trig, error) {
		var x trig
		return x, r.Scan(&x.table, &x.name, &x.fn, &x.kind, &x.enabled)
	})
	if err != nil {
		t.Fatal(err)
	}
	const (
		row      = 1 << 0
		before   = 1 << 1
		del      = 1 << 3
		upd      = 1 << 4
		truncate = 1 << 5
	)
	has := func(table, fn string, mask int16) bool {
		for _, x := range all {
			if x.table == table && x.fn == fn && x.kind&mask == mask && x.enabled == "O" {
				return true
			}
		}
		return false
	}
	for _, tbl := range appendOnly {
		if !has(tbl, "forbid_update_delete", row|before|upd|del) {
			t.Errorf("%s has no enabled BEFORE UPDATE OR DELETE trigger", tbl)
		}
		if !has(tbl, "forbid_truncate", before|truncate) {
			t.Errorf("%s has no enabled BEFORE TRUNCATE trigger", tbl)
		}
	}
	for tbl, fn := range updatable {
		if !has(tbl, fn, row|before|upd) {
			t.Errorf("%s has no enabled column-limiting UPDATE trigger", tbl)
		}
		if !has(tbl, "forbid_update_delete", row|before|del) {
			t.Errorf("%s has no enabled BEFORE DELETE trigger", tbl)
		}
		if !has(tbl, "forbid_truncate", before|truncate) {
			t.Errorf("%s has no enabled BEFORE TRUNCATE trigger", tbl)
		}
	}
	if !has("audit_events", "audit_events_guard", row|before|upd|del) || !has("audit_events", "audit_events_guard", before|truncate) {
		t.Error("audit_events has no guard trigger for UPDATE, DELETE and TRUNCATE")
	}
}

func TestAppendOnlyTablesRefuseChangesEvenFromTheOwner(t *testing.T) {
	e := newEnv(t)
	a := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{})
	job := e.c.SeedJob(t, e.b, a, "low_impact", 1)
	if _, err := e.uses.Claim(t.Context(), e.claim(a, job)); err != nil {
		t.Fatal(err)
	}
	owner := e.c.Owner(t)
	ctx := t.Context()

	// Rows exist in operations and approval_uses.
	_, err := owner.Exec(ctx, `UPDATE morningstar.operations SET tool_key = 'x'`)
	wantCode(t, "UPDATE operations", err, sqlstateRestrict)
	_, err = owner.Exec(ctx, `DELETE FROM morningstar.operations`)
	wantCode(t, "DELETE operations", err, sqlstateRestrict)
	_, err = owner.Exec(ctx, `UPDATE morningstar.approval_uses SET claim_epoch = 9`)
	wantCode(t, "UPDATE approval_uses", err, sqlstateRestrict)
	_, err = owner.Exec(ctx, `DELETE FROM morningstar.approval_uses`)
	wantCode(t, "DELETE approval_uses", err, sqlstateRestrict)
	_, err = owner.Exec(ctx, `UPDATE morningstar.scope_versions SET reason = 'changed'`)
	wantCode(t, "UPDATE scope_versions.reason", err, sqlstateRestrict)
	_, err = owner.Exec(ctx, `UPDATE morningstar.scope_versions SET superseded_at = now()`)
	if err != nil {
		t.Errorf("UPDATE scope_versions.superseded_at is allowed: %v", err)
	}
	_, err = owner.Exec(ctx, `DELETE FROM morningstar.scope_versions`)
	wantCode(t, "DELETE scope_versions", err, sqlstateRestrict)
	_, err = owner.Exec(ctx, `UPDATE morningstar.target_validations SET attested_by = attested_by, status = 'stale', invalidated_at = now(), invalidation_reason = 'manual'`)
	if err != nil {
		t.Errorf("UPDATE target_validations status columns is allowed: %v", err)
	}
	_, err = owner.Exec(ctx, `UPDATE morningstar.target_validations SET target_set_hash = $1`, pgtest.Random(32))
	wantCode(t, "UPDATE target_validations.target_set_hash", err, sqlstateRestrict)

	for _, tbl := range []string{"login_attempts", "program_rules_versions", "scope_rules", "lab_registration_ranges", "target_validation_targets",
		"assessment_transitions", "operations", "policy_decisions", "approval_events", "approval_uses", "job_transitions",
		"user_credentials", "policy_settings", "scope_versions", "lab_registrations", "target_validations", "job_attempts", "audit_events"} {
		_, err := owner.Exec(ctx, "TRUNCATE morningstar."+tbl+" CASCADE")
		wantCode(t, "TRUNCATE "+tbl, err, sqlstateRestrict)
	}
}

// DB-17: audit rows are appended and never changed or deleted. M3 builds no
// purge path: the retention role reads the table and nothing else. The purge
// procedure (an audited function with an age floor) is a later milestone
// (TD-M3-03), so no role, not even the owner, can remove an audit row today.
func TestAuditEventsAreAppendOnly(t *testing.T) {
	e := newEnv(t)
	owner := e.c.Owner(t)
	ctx := t.Context()
	for _, occurred := range []string{"now() - interval '10 days'", "now()"} {
		if _, err := e.api.Exec(ctx, `INSERT INTO morningstar.audit_events (id, occurred_at, actor_kind, action, outcome) VALUES (uuidv7(), `+occurred+`, 'system', 'test.event', 'success')`); err != nil {
			t.Fatalf("insert audit event: %v", err)
		}
	}
	if _, err := e.worker.Exec(ctx, `INSERT INTO morningstar.audit_events (id, actor_kind, action, outcome) VALUES (uuidv7(), 'worker', 'test.event', 'success')`); err != nil {
		t.Fatalf("the worker cannot append: %v", err)
	}
	count := func() (n int) {
		if err := owner.QueryRow(ctx, `SELECT count(*) FROM morningstar.audit_events`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(); n != 3 {
		t.Fatalf("%d audit rows, want 3", n)
	}

	ret := e.c.Pool(t, persistence.RoleRetention, 2)
	var n int
	if err := ret.QueryRow(ctx, `SELECT count(*) FROM morningstar.audit_events`).Scan(&n); err != nil || n != 3 {
		t.Errorf("the retention role cannot read the audit log (%d, %v)", n, err)
	}

	// The owner is stopped by the trigger. Every other role is stopped by the grants.
	for _, stmt := range []string{
		`UPDATE morningstar.audit_events SET outcome = 'failure'`,
		`DELETE FROM morningstar.audit_events`,
		`DELETE FROM morningstar.audit_events WHERE occurred_at < now() - interval '5 days'`,
	} {
		_, err := owner.Exec(ctx, stmt)
		wantCode(t, "owner: "+stmt, err, sqlstateRestrict)
		for name, pool := range map[string]persistence.DBTX{"api": e.api, "worker": e.worker, "retention": ret} {
			_, err := pool.Exec(ctx, stmt)
			wantCode(t, name+": "+stmt, err, sqlstateNoRight)
		}
	}
	_, err := owner.Exec(ctx, `TRUNCATE morningstar.audit_events`)
	wantCode(t, "owner TRUNCATE", err, sqlstateRestrict)

	// A cutoff that a role sets for itself does not open a purge path.
	for name, begin := range map[string]func(context.Context) (pgx.Tx, error){"owner": owner.Begin, "retention": ret.Begin} {
		tx, err := begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `SELECT set_config('morningstar.audit_purge_before', $1, true)`, time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(ctx, `DELETE FROM morningstar.audit_events WHERE occurred_at < now() - interval '5 days'`)
		_ = tx.Rollback(context.Background())
		if err == nil {
			t.Errorf("%s: a self-set cutoff allowed a purge", name)
		}
	}
	if n := count(); n != 3 {
		t.Errorf("%d audit rows at the end, want 3", n)
	}
}

// The decision of an approval cannot be changed afterwards, and the status
// moves only forward (approvals_guard). The API holds UPDATE on these columns
// to record the decision. This guard keeps a defect from widening an approval
// that a human already decided.
func TestApprovalTermsAndStatusAreFrozenAfterTheDecision(t *testing.T) {
	e := newEnv(t)
	ctx := t.Context()
	exec := func(id, set string) error {
		_, err := e.api.Exec(ctx, `UPDATE morningstar.approvals SET `+set+`, updated_at = now(), version = version + 1 WHERE id = $1`, id)
		return err
	}
	approved := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{MaxUses: 2})
	for name, set := range map[string]string{
		"raise max_uses":       "max_uses = 50",
		"raise max_attempts":   "max_attempts = 3",
		"extend the expiry":    "expires_at = expires_at + interval '1 day'",
		"back to pending":      "status = 'pending', decided_by = NULL, decided_at = NULL",
		"approved to rejected": "status = 'rejected'",
	} {
		err := exec(approved.ID, set)
		if err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if err := exec(approved.ID, fmt.Sprintf("status = 'revoked', revoked_at = now(), revoked_by = '%s', revoke_reason = 'x'", e.b.UserID)); err != nil {
		t.Errorf("approved to revoked: %v", err)
	}
	if err := exec(approved.ID, "status = 'approved', revoked_at = NULL, revoked_by = NULL, revoke_reason = NULL"); err == nil {
		t.Error("revoked back to approved: accepted")
	}

	pending := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{Status: "pending"})
	if err := exec(pending.ID, fmt.Sprintf("status = 'approved', decided_by = '%s', decided_at = now(), expires_at = now() + interval '1 hour', max_uses = 2", e.b.UserID)); err != nil {
		t.Errorf("pending to approved: %v", err)
	}
	rejected := e.c.SeedApproval(t, e.b, pgtest.ApprovalSpec{Status: "rejected"})
	if err := exec(rejected.ID, "status = 'approved'"); err == nil {
		t.Error("rejected to approved: accepted")
	}
}
