package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/persistence/sqlcgen"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/audit"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

// Recovery implements outbound.RecoveryRepository for the server role
// (recovery-generation.md). Phase 1 changes many tables in one transaction, so
// that either all authority recorded in the database is invalidated or none is.
type Recovery struct {
	pool *pgxpool.Pool
	tx   *Transactor
}

// NewRecovery returns the repository. pool is the server pool.
func NewRecovery(pool *pgxpool.Pool) *Recovery {
	return &Recovery{pool: pool, tx: NewTransactor(pool)}
}

var _ outbound.RecoveryRepository = (*Recovery)(nil)

// allowedReasons are the values that recovery_state.last_reason accepts for a
// reconciliation (CHECK c4 of recovery_state).
var allowedReasons = map[string]bool{
	"marker_missing": true, "marker_invalid": true, "marker_older": true, "database_older": true, "instance_mismatch": true,
}

// Facts reads the database inputs of the startup decision.
func (r *Recovery) Facts(ctx context.Context) (outbound.RecoveryFacts, error) {
	var f outbound.RecoveryFacts
	q := sqlcgen.New(Querier(ctx, r.pool))
	row, err := q.GetRecoveryState(ctx)
	switch {
	case err == nil:
		f.Row = &outbound.RecoveryRow{InstanceID: row.InstanceID, Generation: uint64(row.Generation), Reconciling: row.Reconciling, LastReason: row.LastReason}
	case !errors.Is(err, pgx.ErrNoRows):
		return outbound.RecoveryFacts{}, fmt.Errorf("persistence: read recovery_state: %w", err)
	}
	if f.HasUsers, err = q.HasUsers(ctx); err != nil {
		return outbound.RecoveryFacts{}, fmt.Errorf("persistence: read users: %w", err)
	}
	if f.MaxAuditSeq, err = r.MaxAuditSeq(ctx); err != nil {
		return outbound.RecoveryFacts{}, err
	}
	return f, nil
}

// MaxAuditSeq returns the highest audit sequence, or 0.
func (r *Recovery) MaxAuditSeq(ctx context.Context) (uint64, error) {
	seq, err := sqlcgen.New(Querier(ctx, r.pool)).MaxAuditSeq(ctx)
	if err != nil {
		return 0, fmt.Errorf("persistence: read the audit sequence: %w", err)
	}
	return uint64(seq), nil
}

// InitializeFirstRun inserts the first recovery_state row and an audit event,
// and calls beforeCommit inside the transaction.
func (r *Recovery) InitializeFirstRun(ctx context.Context, instanceID string, beforeCommit func(ctx context.Context, maxSeq uint64) error) error {
	if !validUUID(instanceID) {
		return errors.New("persistence: the instance id is not a UUID")
	}
	return r.tx.InTx(ctx, func(ctx context.Context) error {
		q := Querier(ctx, r.pool)
		if _, err := q.Exec(ctx, `INSERT INTO morningstar.recovery_state (instance_id, generation, state, last_reason) VALUES ($1, 1, 'normal', 'first_run')`, instanceID); err != nil {
			return fmt.Errorf("persistence: insert recovery_state: %w", err)
		}
		if err := appendEvent(ctx, q, audit.Input{Actor: audit.System(), Action: "system.first_run", Outcome: audit.Success, Detail: map[string]any{"generation": 1}}); err != nil {
			return fmt.Errorf("persistence: audit the first run: %w", err)
		}
		seq, err := r.MaxAuditSeq(ctx)
		if err != nil {
			return err
		}
		return beforeCommit(ctx, seq)
	})
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err) // the values above are maps of numbers and strings
	}
	return string(b)
}

// countQuery runs a statement of the form "WITH ... SELECT count(*)" and
// returns the count.
func countQuery(ctx context.Context, q DBTX, sql string, args ...any) (int, error) {
	var n int
	if err := q.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// BeginReconcile is phase 1 of the reconciliation (section 6).
func (r *Recovery) BeginReconcile(ctx context.Context, in outbound.ReconcileInput) (outbound.ReconcileCounts, error) {
	if !validUUID(in.ActorUserID) || !validUUID(in.RequestID) || !validUUID(in.InstanceID) ||
		in.NewGeneration < 1 || in.NewGeneration > maxGeneration || !allowedReasons[in.Reason] ||
		(in.RecoveryCodeHash != "") != validUUID(in.RecoveryCodeID) {
		return outbound.ReconcileCounts{}, errors.New("persistence: the reconciliation input is invalid")
	}
	var counts outbound.ReconcileCounts
	err := r.tx.InTx(ctx, func(ctx context.Context) error {
		q := Querier(ctx, r.pool)

		// The exclusive row lock waits for every claim that holds the shared lock
		// (CC-15) and keeps a second reconciliation out.
		var current int64
		err := q.QueryRow(ctx, `SELECT generation FROM morningstar.recovery_state WHERE singleton FOR UPDATE`).Scan(&current)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			if in.ExpectedGeneration != 0 {
				return outbound.ErrRecoveryChanged
			}
			if _, err := q.Exec(ctx, `INSERT INTO morningstar.recovery_state (instance_id, generation, state, last_reason) VALUES ($1, $2, 'reconciling', $3)`,
				in.InstanceID, int64(in.NewGeneration), in.Reason); err != nil {
				return fmt.Errorf("insert recovery_state: %w", err)
			}
		case err != nil:
			return fmt.Errorf("lock recovery_state: %w", err)
		default:
			if uint64(current) != in.ExpectedGeneration {
				return outbound.ErrRecoveryChanged
			}
			if _, err := q.Exec(ctx, `UPDATE morningstar.recovery_state SET state = 'reconciling', generation = $1, last_reason = $2, updated_at = now() WHERE singleton`,
				int64(in.NewGeneration), in.Reason); err != nil {
				return fmt.Errorf("update recovery_state: %w", err)
			}
		}

		steps := []struct {
			dst  *int
			name string
			sql  string
			args []any
		}{
			{&counts.SessionsRevoked, "sessions", `WITH x AS (UPDATE morningstar.sessions SET revoked_at = now(), revoked_reason = 'recovery_reset' WHERE revoked_at IS NULL RETURNING 1) SELECT count(*) FROM x`, nil},
			{&counts.ReauthGrantsConsumed, "reauth grants", `WITH x AS (UPDATE morningstar.reauth_grants SET consumed_at = now() WHERE consumed_at IS NULL RETURNING 1) SELECT count(*) FROM x`, nil},
			{&counts.ApprovalsRevoked, "approvals", `
				WITH r AS (
					UPDATE morningstar.approvals SET status = 'revoked', revoked_at = now(), revoked_by = $1, revoke_reason = 'recovery_reset', updated_at = now(), version = version + 1
					 WHERE status = 'approved' RETURNING id),
				e AS (
					INSERT INTO morningstar.approval_events (id, approval_id, event_type, actor_kind, actor_user_id, reason_code, request_id)
					SELECT uuidv7(), id, 'revoked', 'user', $1, 'recovery_reset', $2 FROM r)
				SELECT count(*) FROM r`, []any{in.ActorUserID, in.RequestID}},
			{&counts.ApprovalsRejected, "pending approvals", `
				WITH r AS (
					UPDATE morningstar.approvals SET status = 'rejected', decided_by = $1, decided_at = now(), updated_at = now(), version = version + 1
					 WHERE status = 'pending' RETURNING id),
				e AS (
					INSERT INTO morningstar.approval_events (id, approval_id, event_type, actor_kind, actor_user_id, reason_code, request_id)
					SELECT uuidv7(), id, 'rejected', 'user', $1, 'recovery_reset', $2 FROM r)
				SELECT count(*) FROM r`, []any{in.ActorUserID, in.RequestID}},
			{&counts.ValidationsInvalid, "target validations", `WITH x AS (UPDATE morningstar.target_validations SET status = 'invalidated', invalidated_at = now(), invalidation_reason = 'restore_recovery' WHERE status = 'current' RETURNING 1) SELECT count(*) FROM x`, nil},
			{&counts.LabsRevoked, "lab registrations", `WITH x AS (UPDATE morningstar.lab_registrations SET status = 'revoked', revoked_at = now(), revoked_by = $1, revoke_reason = 'recovery_reset' WHERE status = 'active' RETURNING 1) SELECT count(*) FROM x`, []any{in.ActorUserID}},
			{&counts.AssessmentsToDraft, "assessments to draft", `
				WITH a AS (
					UPDATE morningstar.assessments SET status = 'draft', updated_at = now(), version = version + 1 WHERE status = 'ready' RETURNING id),
				t AS (
					INSERT INTO morningstar.assessment_transitions (id, assessment_id, from_status, to_status, actor_user_id, reason, request_id)
					SELECT uuidv7(), id, 'ready', 'draft', $1, 'recovery_reset', $2 FROM a)
				SELECT count(*) FROM a`, []any{in.ActorUserID, in.RequestID}},
			{&counts.AssessmentsPaused, "assessments to paused", `
				WITH a AS (
					UPDATE morningstar.assessments SET status = 'paused', updated_at = now(), version = version + 1 WHERE status = 'running' RETURNING id),
				t AS (
					INSERT INTO morningstar.assessment_transitions (id, assessment_id, from_status, to_status, actor_user_id, reason, request_id)
					SELECT uuidv7(), id, 'running', 'paused', $1, 'recovery_reset', $2 FROM a)
				SELECT count(*) FROM a`, []any{in.ActorUserID, in.RequestID}},
			{&counts.RecoveryCodesRevoked, "recovery codes", `WITH x AS (UPDATE morningstar.recovery_codes SET revoked_at = now() WHERE used_at IS NULL AND revoked_at IS NULL RETURNING 1) SELECT count(*) FROM x`, nil},
			{&counts.BootstrapExpired, "bootstrap secrets", `WITH x AS (UPDATE morningstar.bootstrap_secrets SET superseded_at = now() WHERE consumed_at IS NULL AND superseded_at IS NULL RETURNING 1) SELECT count(*) FROM x`, nil},
			{&counts.RawCaptureCleared, "raw evidence capture", `WITH x AS (UPDATE morningstar.assessments SET raw_evidence_capture = false, updated_at = now(), version = version + 1 WHERE raw_evidence_capture RETURNING 1) SELECT count(*) FROM x`, nil},
			{&counts.JobsCancelled, "queued jobs", `
				WITH j AS (
					UPDATE morningstar.jobs x SET state = 'cancelled', finished_at = now(), updated_at = now(), version = x.version + 1
					  FROM (SELECT id, state AS old_state FROM morningstar.jobs WHERE state IN ('queued', 'awaiting_approval') FOR UPDATE) o
					 WHERE x.id = o.id RETURNING x.id, o.old_state, x.control_state, x.claim_epoch),
				t AS (
					INSERT INTO morningstar.job_transitions (id, job_id, from_state, to_state, control_state, claim_epoch, actor_kind, actor_user_id, reason)
					SELECT uuidv7(), id, old_state, 'cancelled', control_state, claim_epoch, 'user', $1, 'recovery_reset' FROM j)
				SELECT count(*) FROM j`, []any{in.ActorUserID}},
			{&counts.JobsStopRequested, "running jobs", `WITH x AS (UPDATE morningstar.jobs SET control_state = 'stop_requested', control_reason = 'recovery_reset', control_requested_at = now(), updated_at = now(), version = version + 1 WHERE state = 'running' AND control_state = 'none' RETURNING 1) SELECT count(*) FROM x`, nil},
			{&counts.AttemptsLost, "live attempts", `WITH x AS (UPDATE morningstar.job_attempts SET state = 'lost', ended_at = now(), error_class = 'recovery_reset' WHERE state IN ('launching', 'running') RETURNING 1) SELECT count(*) FROM x`, nil},
		}
		for _, s := range steps {
			n, err := countQuery(ctx, q, s.sql, s.args...)
			if err != nil {
				return fmt.Errorf("invalidate %s: %w", s.name, err)
			}
			*s.dst = n
		}

		if in.NewPasswordHash != "" {
			if _, err := q.Exec(ctx, `UPDATE morningstar.user_credentials SET is_current = false, superseded_at = now() WHERE user_id = $1 AND is_current`, in.ActorUserID); err != nil {
				return fmt.Errorf("supersede the restored password: %w", err)
			}
			if _, err := q.Exec(ctx, `INSERT INTO morningstar.user_credentials (id, user_id, phc_hash) VALUES ($1, $2, $3)`, in.NewCredentialID, in.ActorUserID, in.NewPasswordHash); err != nil {
				return fmt.Errorf("insert the new password: %w", err)
			}
			if _, err := q.Exec(ctx, `UPDATE morningstar.users SET password_changed_at = now(), updated_at = now(), version = version + 1 WHERE id = $1`, in.ActorUserID); err != nil {
				return fmt.Errorf("record the password change: %w", err)
			}
		}

		if in.RecoveryCodeHash != "" {
			if _, err := q.Exec(ctx, `INSERT INTO morningstar.recovery_codes (id, user_id, code_hash) VALUES ($1, $2, $3)`, in.RecoveryCodeID, in.ActorUserID, in.RecoveryCodeHash); err != nil {
				return fmt.Errorf("insert the new recovery code: %w", err)
			}
		}

		// The audit event and the alert are part of phase 1. When either fails the
		// whole phase rolls back and the installation stays recovery_required.
		d := map[string]any{
			"old_marker_generation": in.OldMarkerGeneration, "old_database_generation": current, "new_generation": in.NewGeneration, "counts": countsMap(counts),
		}
		actor, err := audit.User(in.ActorUserID)
		if err != nil {
			return fmt.Errorf("audit the reconciliation: %w", err)
		}
		if err := appendEvent(ctx, q, audit.Input{Actor: actor, Action: "system.restore_recovered", Outcome: audit.Success, Reason: in.Reason, RequestID: in.RequestID, Detail: d}); err != nil {
			return fmt.Errorf("audit the reconciliation: %w", err)
		}
		d["reason"] = in.Reason
		if err := insertSystemAlert(ctx, q, "restore_recovered", mustJSON(d)); err != nil {
			return fmt.Errorf("raise the alert: %w", err)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, outbound.ErrRecoveryChanged) {
			return outbound.ReconcileCounts{}, err
		}
		return outbound.ReconcileCounts{}, fmt.Errorf("persistence: reconciliation phase 1: %w", err)
	}
	return counts, nil
}

// CompleteReconcile is phase 3.
func (r *Recovery) CompleteReconcile(ctx context.Context, generation uint64, reason string) error {
	if generation < 1 || generation > maxGeneration {
		return errors.New("persistence: the generation is out of range")
	}
	return r.tx.InTx(ctx, func(ctx context.Context) error {
		q := Querier(ctx, r.pool)
		tag, err := q.Exec(ctx, `UPDATE morningstar.recovery_state SET state = 'normal', reconciled_at = now(), updated_at = now() WHERE singleton AND generation = $1 AND state = 'reconciling'`, int64(generation))
		if err != nil {
			return fmt.Errorf("persistence: complete the reconciliation: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return outbound.ErrRecoveryChanged
		}
		if err := appendEvent(ctx, q, audit.Input{Actor: audit.System(), Action: "system.recovery_reconciled", Outcome: audit.Success, Reason: reason, Detail: map[string]any{"generation": generation}}); err != nil {
			return fmt.Errorf("persistence: audit the completion: %w", err)
		}
		return nil
	})
}
