package pgtest

import (
	"crypto/rand"
	"fmt"
	"testing"
	"time"
)

// Base holds the rows every authorization test needs: one account, one
// program, one assessment, one scope version, one current target validation and
// the recovery state of a normal installation.
type Base struct {
	InstanceID   string
	UserID       string
	ProgramID    string
	AssessmentID string
	ScopeID      string
	ValidationID string
	PolicyHash   []byte
}

// Random returns n random bytes.
func Random(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

// SeedBase inserts the base rows as the schema owner and returns their ids. It
// also inserts a recovery_state row in the normal state, generation 1.
func (c *Cluster) SeedBase(t testing.TB) Base { return c.seedBase(t, true) }

// SeedBaseWithoutRecovery is SeedBase without the recovery_state row, for the
// tests that run the first-run initialization themselves.
func (c *Cluster) SeedBaseWithoutRecovery(t testing.TB) Base { return c.seedBase(t, false) }

func (c *Cluster) seedBase(t testing.TB, withRecovery bool) Base {
	t.Helper()
	ctx := t.Context()
	conn := c.Owner(t)
	b := Base{PolicyHash: Random(32)}
	if withRecovery {
		if err := conn.QueryRow(ctx, `INSERT INTO morningstar.recovery_state (instance_id) VALUES (uuidv7()) RETURNING instance_id`).Scan(&b.InstanceID); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	if err := conn.QueryRow(ctx, `INSERT INTO morningstar.users (id, username) VALUES (uuidv7(), 'researcher') RETURNING id`).Scan(&b.UserID); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var rulesID string
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	must := func(dst any, sql string, args ...any) {
		t.Helper()
		if err := tx.QueryRow(ctx, sql, args...).Scan(dst); err != nil {
			t.Fatalf("seed: %v\n%s", err, sql)
		}
	}
	must(&b.ProgramID, `INSERT INTO morningstar.programs (id, owner_user_id, name) VALUES (uuidv7(), $1, 'lab program') RETURNING id`, b.UserID)
	must(&rulesID, `INSERT INTO morningstar.program_rules_versions (id, program_id, version_no, rules_text, content_sha256, recorded_by) VALUES (uuidv7(), $1, 1, 'lab rules', $2, $3) RETURNING id`, b.ProgramID, Random(32), b.UserID)
	must(&b.AssessmentID, `INSERT INTO morningstar.assessments (id, program_id, name, created_by) VALUES (uuidv7(), $1, 'first', $2) RETURNING id`, b.ProgramID, b.UserID)
	must(&b.ScopeID, `INSERT INTO morningstar.scope_versions (id, program_id, version_no, rules_version_id, content_hash, widening, created_by, reason) VALUES (uuidv7(), $1, 1, $2, $3, false, $4, 'initial') RETURNING id`, b.ProgramID, rulesID, Random(32), b.UserID)
	must(&b.ValidationID, `INSERT INTO morningstar.target_validations (id, program_id, assessment_id, scope_version_id, rules_version_id, target_set_hash, attestation_kind, attestation_text_version, attested_by, policy_snapshot_hash, expires_at)
		VALUES (uuidv7(), $1, $2, $3, $4, $5, 'standard', 'v1', $6, $7, now() + interval '1 hour') RETURNING id`,
		b.ProgramID, b.AssessmentID, b.ScopeID, rulesID, Random(32), b.UserID, b.PolicyHash)
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("seed commit: %v", err)
	}
	return b
}

// ApprovalSpec describes an approval for a test. Zero values give an approved,
// valid low-impact approval with one use and one attempt.
type ApprovalSpec struct {
	Risk        string        // "low_impact" (default) or "intrusive"
	Status      string        // "approved" (default), "pending", "revoked" or "rejected"
	MaxUses     int           // default 1
	MaxAttempts int           // default 1
	TTL         time.Duration // default 1 hour. A negative value gives an approval that has expired.
	// OperationHash binds the approval to a given operation hash. Default: random.
	OperationHash []byte
}

// Approval is an approval with the operation it is bound to.
type Approval struct {
	ID            string
	OperationID   string
	OperationHash []byte
}

// SeedApproval inserts an operation and an approval for it as the schema owner.
func (c *Cluster) SeedApproval(t testing.TB, b Base, spec ApprovalSpec) Approval {
	t.Helper()
	if spec.Risk == "" {
		spec.Risk = "low_impact"
	}
	if spec.Status == "" {
		spec.Status = "approved"
	}
	if spec.MaxUses == 0 {
		spec.MaxUses = 1
	}
	if spec.MaxAttempts == 0 {
		spec.MaxAttempts = 1
	}
	if spec.TTL == 0 {
		spec.TTL = time.Hour
	}
	ctx := t.Context()
	conn := c.Owner(t)
	a := Approval{OperationHash: Random(32)}
	if spec.OperationHash != nil {
		a.OperationHash = spec.OperationHash
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := tx.QueryRow(ctx, `INSERT INTO morningstar.operations (id, program_id, assessment_id, operation_hash, tool_key, operation_kind, risk_class, canonical_target, manifest_digest)
		VALUES (uuidv7(), $1, $2, $3, 'httpx', 'http_probe', $4, 'lab.example.test', $5) RETURNING id`,
		b.ProgramID, b.AssessmentID, a.OperationHash, spec.Risk, Random(32)).Scan(&a.OperationID); err != nil {
		t.Fatalf("seed operation: %v", err)
	}
	// The approval is inserted as the API would insert it: pending. The decision
	// is a later UPDATE, because the decision columns follow the lifecycle. The
	// expiry of an expired approval is set directly, in the past.
	var reauth *string
	if spec.Risk == "intrusive" && spec.Status == "approved" {
		var id string
		if err := tx.QueryRow(ctx, `WITH s AS (INSERT INTO morningstar.sessions (id, user_id, token_hash, csrf_token_hash, idle_expires_at, absolute_expires_at)
				VALUES (uuidv7(), $1, $2, $3, now() + interval '1 hour', now() + interval '8 hours') RETURNING id)
			INSERT INTO morningstar.reauth_grants (id, user_id, session_id, purpose, subject_hash, expires_at)
			SELECT uuidv7(), $1, s.id, 'approval_intrusive', $4, now() + interval '1 minute' FROM s RETURNING id`,
			b.UserID, Random(32), Random(32), Random(32)).Scan(&id); err != nil {
			t.Fatalf("seed reauth grant: %v", err)
		}
		reauth = &id
	}
	if err := tx.QueryRow(ctx, `INSERT INTO morningstar.approvals (id, program_id, assessment_id, operation_id, risk_class, status, requested_by_user_id, max_uses, max_attempts, validation_id, scope_version_id, policy_snapshot_hash)
		VALUES (uuidv7(), $1, $2, $3, $4, 'pending', $5, $6, $7, $8, $9, $10) RETURNING id`,
		b.ProgramID, b.AssessmentID, a.OperationID, spec.Risk, b.UserID, spec.MaxUses, spec.MaxAttempts, b.ValidationID, b.ScopeID, b.PolicyHash).Scan(&a.ID); err != nil {
		t.Fatalf("seed approval: %v", err)
	}
	// The decision is a later UPDATE, because the decision columns follow the
	// lifecycle: pending, then approved or rejected, then revoked.
	if spec.Status != "pending" {
		decided := spec.Status
		if decided == "revoked" {
			decided = "approved"
		}
		if _, err := tx.Exec(ctx, `UPDATE morningstar.approvals SET status = $2, decided_by = $3, decided_at = now(), expires_at = now() + $4::interval, reauth_grant_id = $5 WHERE id = $1`,
			a.ID, decided, b.UserID, fmt.Sprintf("%d seconds", int(spec.TTL.Seconds())), reauth); err != nil {
			t.Fatalf("seed decision: %v", err)
		}
		if spec.Status == "revoked" {
			if _, err := tx.Exec(ctx, `UPDATE morningstar.approvals SET status = 'revoked', revoked_at = now(), revoked_by = $2, revoke_reason = 'test' WHERE id = $1`, a.ID, b.UserID); err != nil {
				t.Fatalf("seed revocation: %v", err)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("seed approval commit: %v", err)
	}
	return a
}

// Expire moves the expiry of a decided approval into the past.
func (c *Cluster) Expire(t testing.TB, approvalID string) {
	c.setExpiry(t, approvalID, "now() - interval '1 second'")
}

// ExpireNow sets the expiry to the current clock reading. It is after the start
// of any transaction that began earlier, which is what the long-transaction
// test needs.
func (c *Cluster) ExpireNow(t testing.TB, approvalID string) {
	c.setExpiry(t, approvalID, "clock_timestamp()")
}

// setExpiry changes the expiry of a decided approval. The schema freezes the
// terms of a decided approval, so the helper lifts that trigger for its own
// transaction, as the owner, the way a test may move a clock.
func (c *Cluster) setExpiry(t testing.TB, approvalID, expression string) {
	t.Helper()
	ctx := t.Context()
	tx, err := c.Owner(t).Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for _, q := range []string{
		`ALTER TABLE morningstar.approvals DISABLE TRIGGER approvals_guard_update`,
		`UPDATE morningstar.approvals SET expires_at = ` + expression + ` WHERE id = '` + approvalID + `'`,
		`ALTER TABLE morningstar.approvals ENABLE TRIGGER approvals_guard_update`,
	} {
		if _, err := tx.Exec(ctx, q); err != nil {
			t.Fatalf("set the expiry: %v", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// SeedJob inserts a queued job bound to the approval, as the API would.
func (c *Cluster) SeedJob(t testing.TB, b Base, a Approval, risk string, maxAttempts int) string {
	t.Helper()
	if maxAttempts == 0 {
		maxAttempts = 1
	}
	var id string
	conn := c.Owner(t)
	if err := conn.QueryRow(t.Context(), `INSERT INTO morningstar.jobs (id, program_id, assessment_id, job_kind, operation_id, risk_class, approval_id, scope_version_id, validation_id, max_execution_attempts, created_by)
		VALUES (uuidv7(), $1, $2, 'tool_operation', $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
		b.ProgramID, b.AssessmentID, a.OperationID, risk, a.ID, b.ScopeID, b.ValidationID, maxAttempts, b.UserID).Scan(&id); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	return id
}

// SeedSession inserts an active session and returns its id.
func (c *Cluster) SeedSession(t testing.TB, b Base) string {
	t.Helper()
	var id string
	if err := c.Owner(t).QueryRow(t.Context(), `INSERT INTO morningstar.sessions (id, user_id, token_hash, csrf_token_hash, idle_expires_at, absolute_expires_at)
		VALUES (uuidv7(), $1, $2, $3, now() + interval '1 hour', now() + interval '8 hours') RETURNING id`, b.UserID, Random(32), Random(32)).Scan(&id); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	return id
}

// SeedLab inserts an active lab registration and returns its id.
func (c *Cluster) SeedLab(t testing.TB, b Base) string {
	t.Helper()
	ctx := t.Context()
	tx, err := c.Owner(t).Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var target, session, grant, lab string
	if err := tx.QueryRow(ctx, `INSERT INTO morningstar.targets (id, program_id, kind, canonical_value, display_value, created_by) VALUES (uuidv7(), $1, 'ip', '127.0.0.1', '127.0.0.1', $2) RETURNING id`, b.ProgramID, b.UserID).Scan(&target); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO morningstar.sessions (id, user_id, token_hash, csrf_token_hash, idle_expires_at, absolute_expires_at)
		VALUES (uuidv7(), $1, $2, $3, now() + interval '1 hour', now() + interval '8 hours') RETURNING id`, b.UserID, Random(32), Random(32)).Scan(&session); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO morningstar.reauth_grants (id, user_id, session_id, purpose, subject_hash, expires_at) VALUES (uuidv7(), $1, $2, 'lab_registration', $3, now() + interval '1 minute') RETURNING id`,
		b.UserID, session, Random(32)).Scan(&grant); err != nil {
		t.Fatalf("seed grant: %v", err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO morningstar.lab_registrations (id, program_id, assessment_id, target_id, kind, ports_required, scope_version_id, attestation_text_version, registered_by, reauth_grant_id, expires_at)
		VALUES (uuidv7(), $1, $2, $3, 'address', false, $4, 'v1', $5, $6, now() + interval '1 day') RETURNING id`,
		b.ProgramID, b.AssessmentID, target, b.ScopeID, b.UserID, grant).Scan(&lab); err != nil {
		t.Fatalf("seed lab: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return lab
}

// SetAssessmentStatus moves the base assessment to a status, as the schema
// owner. The status needs the current validation, which the base has.
func (c *Cluster) SetAssessmentStatus(t testing.TB, b Base, status string) {
	t.Helper()
	if _, err := c.Owner(t).Exec(t.Context(), `UPDATE morningstar.assessments SET status = $2, current_validation_id = $3 WHERE id = $1`, b.AssessmentID, status, b.ValidationID); err != nil {
		t.Fatalf("set assessment status: %v", err)
	}
}

// Snapshot copies the database under a name, to restore later.
func (c *Cluster) Snapshot(t testing.TB, name string) {
	t.Helper()
	ctx := t.Context()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.owner != nil {
		c.owner.Close()
		c.owner = nil
	}
	admin := c.Admin(t)
	if _, err := admin.Exec(ctx, "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = 'morningstar' AND pid <> pg_backend_pid()"); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name+" TEMPLATE morningstar OWNER ms_migrator"); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	c.reopenOwner(t)
}

// Restore replaces the database with a snapshot, like a restore of an older dump.
func (c *Cluster) Restore(t testing.TB, name string) {
	t.Helper()
	ctx := t.Context()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.owner != nil {
		c.owner.Close()
		c.owner = nil
	}
	admin := c.Admin(t)
	for _, stmt := range []string{
		"DROP DATABASE morningstar WITH (FORCE)",
		"CREATE DATABASE morningstar TEMPLATE " + name + " OWNER ms_migrator",
		"REVOKE ALL ON DATABASE morningstar FROM PUBLIC",
		"GRANT CONNECT ON DATABASE morningstar TO ms_api, ms_worker, ms_retention",
	} {
		if _, err := admin.Exec(ctx, stmt); err != nil {
			t.Fatalf("restore: %s: %v", stmt, err)
		}
	}
	c.reopenOwner(t)
}
