package db_test

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/tests/integration/pgtest"
)

// grantMatrix is the reviewed grant matrix of migration 00015 (DB-04, P-13,
// migration-plan.md). Each entry is the table-level privileges as letters (S
// select, I insert, U update, D delete) followed by the columns on which UPDATE
// is granted separately. A table that is missing from a role has no privilege.
// The test compares it with the catalog for every table and role, so a grant
// that is added, widened or dropped without a change here fails.
var grantMatrix = map[string]map[string][]string{
	"ms_api": {
		"approval_events":           {"SI"},
		"approval_uses":             {"S"},
		"approvals":                 {"SI", "status", "decided_by", "decided_at", "expires_at", "max_uses", "max_attempts", "reauth_grant_id", "revoked_at", "revoked_by", "revoke_reason", "updated_at", "version"},
		"assessment_targets":        {"SIU"},
		"assessment_transitions":    {"SI"},
		"assessments":               {"SIU"},
		"audit_events":              {"SI"},
		"bootstrap_secrets":         {"SIU"},
		"job_attempts":              {"S", "state", "ended_at", "error_class"},
		"job_transitions":           {"SI"},
		"jobs":                      {"SI", "state", "control_state", "control_reason", "control_requested_at", "finished_at", "updated_at", "version"},
		"lab_registration_ranges":   {"SI"},
		"lab_registrations":         {"SI", "status", "revoked_at", "revoked_by", "revoke_reason"},
		"login_attempts":            {"SI"},
		"operations":                {"SI"},
		"plan_items":                {"SIU"},
		"policy_decisions":          {"SI"},
		"policy_settings":           {"SI", "superseded_at"},
		"program_rules_versions":    {"SI"},
		"programs":                  {"SIU"},
		"reauth_grants":             {"SIU"},
		"recovery_codes":            {"SIU"},
		"recovery_state":            {"SI", "state", "generation", "last_reason", "reconciled_at", "updated_at"},
		"refused_exclusions":        {"SIU"},
		"scope_rules":               {"SI"},
		"scope_versions":            {"SI", "superseded_at"},
		"sessions":                  {"SIU"},
		"system_alerts":             {"SI", "cleared_at", "cleared_by"},
		"target_validation_targets": {"SI"},
		"target_validations":        {"SI", "status", "invalidated_at", "invalidation_reason"},
		"targets":                   {"SIU"},
		"user_credentials":          {"SI", "is_current", "superseded_at"},
		"users":                     {"SIU"},
	},
	"ms_worker": {
		"approval_events":           {"SI"},
		"approval_uses":             {"S"},
		"approvals":                 {"S"},
		"assessment_targets":        {"S"},
		"assessment_transitions":    {"S"},
		"assessments":               {"S"},
		"audit_events":              {"SI"},
		"job_attempts":              {"SI", "state", "process_group_ref", "ended_at", "termination_confirmed_at", "exit_code", "error_class", "tool_version", "tool_binary_sha256", "sandbox_digest", "argv_redacted"},
		"job_transitions":           {"SI"},
		"jobs":                      {"S", "state", "control_state", "control_reason", "approval_use_id", "lease_owner", "lease_expires_at", "heartbeat_at", "claim_epoch", "execution_attempts", "available_at", "failure_class", "finished_at", "updated_at", "version"},
		"lab_registration_ranges":   {"S"},
		"lab_registrations":         {"S"},
		"operations":                {"S"},
		"plan_items":                {"S"},
		"policy_decisions":          {"SI"},
		"policy_settings":           {"S"},
		"program_rules_versions":    {"S"},
		"programs":                  {"S"},
		"recovery_state":            {"S"},
		"refused_exclusions":        {"S"},
		"scope_rules":               {"S"},
		"scope_versions":            {"S"},
		"system_alerts":             {"SI"},
		"target_validation_targets": {"S"},
		"target_validations":        {"S"},
		"targets":                   {"S"},
	},
	"ms_retention": {
		"audit_events": {"S"},
	},
}

var privileges = []struct {
	letter string
	name   string
}{{"S", "SELECT"}, {"I", "INSERT"}, {"U", "UPDATE"}, {"D", "DELETE"}, {"T", "TRUNCATE"}, {"R", "REFERENCES"}, {"G", "TRIGGER"}}

func TestGrantsMatchTheReviewedMatrix(t *testing.T) {
	c := pgtest.Get(t)
	c.Fresh(t)
	conn := c.Owner(t)

	rows, err := conn.Query(t.Context(), `SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace AND n.nspname = 'morningstar' WHERE c.relkind = 'r' ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	tables, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 33 {
		t.Fatalf("%d tables, want 33", len(tables))
	}
	for _, role := range []string{"ms_api", "ms_worker", "ms_retention"} {
		for tbl := range grantMatrix[role] {
			found := false
			for _, x := range tables {
				found = found || x == tbl
			}
			if !found {
				t.Errorf("matrix of %s names a table that does not exist: %s", role, tbl)
			}
		}
		for _, tbl := range tables {
			want := grantMatrix[role][tbl]
			wantTable, wantCols := "", map[string]bool{}
			if len(want) > 0 {
				wantTable = want[0]
				for _, col := range want[1:] {
					wantCols[col] = true
				}
			}
			gotTable := ""
			for _, p := range privileges {
				var ok bool
				if err := conn.QueryRow(t.Context(), `SELECT has_table_privilege($1, $2::regclass, $3)`, role, "morningstar."+tbl, p.name).Scan(&ok); err != nil {
					t.Fatal(err)
				}
				if ok {
					gotTable += p.letter
				}
			}
			if gotTable != wantTable {
				t.Errorf("%s on %s: table privileges %q, want %q", role, tbl, gotTable, wantTable)
			}
			crows, err := conn.Query(t.Context(), `
				SELECT a.attname FROM pg_attribute a
				 WHERE a.attrelid = $2::regclass AND a.attnum > 0 AND NOT a.attisdropped
				   AND has_column_privilege($1, a.attrelid, a.attnum, 'UPDATE') ORDER BY a.attnum`, role, "morningstar."+tbl)
			if err != nil {
				t.Fatal(err)
			}
			updatable, err := pgx.CollectRows(crows, pgx.RowTo[string])
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(gotTable, "U") {
				continue // UPDATE on the whole table covers every column
			}
			got := map[string]bool{}
			for _, col := range updatable {
				got[col] = true
			}
			if fmt.Sprint(sortedKeys(got)) != fmt.Sprint(sortedKeys(wantCols)) {
				t.Errorf("%s on %s: UPDATE columns %v, want %v", role, tbl, sortedKeys(got), sortedKeys(wantCols))
			}
		}
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Properties of the matrix that must hold whatever the table list is (DB-04,
// P-32, DB-17). The retention role holds no DELETE: M3 has no purge path.
func TestNoRoleCanRewriteHistoryOrBypassTheClaimFunction(t *testing.T) {
	c := pgtest.Get(t)
	c.Fresh(t)
	conn := c.Owner(t)

	for _, role := range []string{"ms_api", "ms_worker", "ms_retention", "public"} {
		rows, err := conn.Query(t.Context(), `
			SELECT c.relname, p.priv FROM pg_class c
			  JOIN pg_namespace n ON n.oid = c.relnamespace AND n.nspname = 'morningstar'
			  CROSS JOIN (VALUES ('DELETE'), ('TRUNCATE'), ('REFERENCES'), ('TRIGGER')) p(priv)
			 WHERE c.relkind = 'r' AND has_table_privilege($1, c.oid, p.priv)
			 ORDER BY 1, 2`, role)
		if err != nil {
			t.Fatal(err)
		}
		type hit struct{ tbl, priv string }
		hits, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (hit, error) {
			var h hit
			return h, r.Scan(&h.tbl, &h.priv)
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, h := range hits {
			t.Errorf("%s holds %s on %s", role, h.priv, h.tbl)
		}
	}

	// The worker never updates an approval, a scope version, a policy setting or an account row.
	for _, tbl := range []string{"approvals", "scope_versions", "scope_rules", "policy_settings", "users", "user_credentials", "sessions", "programs", "targets", "target_validations", "lab_registrations"} {
		var any bool
		if err := conn.QueryRow(t.Context(), `SELECT has_any_column_privilege('ms_worker', $1::regclass, 'UPDATE') OR has_any_column_privilege('ms_worker', $1::regclass, 'INSERT')`, "morningstar."+tbl).Scan(&any); err != nil {
			t.Fatal(err)
		}
		if any {
			t.Errorf("ms_worker can write %s (P-32)", tbl)
		}
	}
	// Nobody but the owner can change the use counter.
	for _, role := range []string{"ms_api", "ms_worker", "ms_retention"} {
		var ok bool
		if err := conn.QueryRow(t.Context(), `SELECT has_column_privilege($1, 'morningstar.approvals', 'uses_consumed', 'UPDATE')`, role).Scan(&ok); err != nil {
			t.Fatal(err)
		}
		if ok {
			t.Errorf("%s can update approvals.uses_consumed", role)
		}
	}
	// Only the worker executes the claim functions. No role executes anything else.
	rows, err := conn.Query(t.Context(), `
		SELECT p.proname, r.rolname
		  FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace AND n.nspname = 'morningstar'
		  CROSS JOIN (VALUES ('ms_api'), ('ms_worker'), ('ms_retention'), ('public')) r(rolname)
		 WHERE p.prokind = 'f' AND has_function_privilege(r.rolname, p.oid, 'EXECUTE') ORDER BY 1, 2`)
	if err != nil {
		t.Fatal(err)
	}
	type fn struct{ name, role string }
	fns, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (fn, error) {
		var f fn
		return f, r.Scan(&f.name, &f.role)
	})
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[fn]bool{{"consume_approval_use", "ms_worker"}: true, {"check_approval_launch", "ms_worker"}: true}
	for _, f := range fns {
		if !allowed[f] {
			t.Errorf("%s may execute morningstar.%s", f.role, f.name)
		}
	}
	for f := range allowed {
		found := false
		for _, g := range fns {
			found = found || g == f
		}
		if !found {
			t.Errorf("%s may not execute morningstar.%s", f.role, f.name)
		}
	}
	// The schema is closed to PUBLIC.
	var open bool
	if err := conn.QueryRow(t.Context(), `SELECT has_schema_privilege('public', 'morningstar', 'USAGE') OR has_schema_privilege('public', 'morningstar', 'CREATE')`).Scan(&open); err != nil || open {
		t.Errorf("PUBLIC has access to the schema (%v)", err)
	}
	for _, role := range []string{"ms_api", "ms_worker", "ms_retention"} {
		var create bool
		if err := conn.QueryRow(t.Context(), `SELECT has_schema_privilege($1, 'morningstar', 'CREATE') OR has_schema_privilege($1, 'public', 'CREATE')`, role).Scan(&create); err != nil || create {
			t.Errorf("%s can create objects (%v)", role, err)
		}
	}
}
