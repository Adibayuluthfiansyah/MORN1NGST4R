-- +goose Up
-- Slice 00014 integrity_triggers (milestone M3 part). DB-11, DB-13, DB-17.

-- +goose StatementBegin
CREATE FUNCTION morningstar.forbid_update_except() RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
BEGIN
    -- TG_ARGV lists the only columns that an UPDATE may change.
    IF (to_jsonb(NEW) - TG_ARGV) IS DISTINCT FROM (to_jsonb(OLD) - TG_ARGV) THEN
        RAISE EXCEPTION 'table %.% allows UPDATE of the columns % only', TG_TABLE_SCHEMA, TG_TABLE_NAME, TG_ARGV
            USING ERRCODE = 'restrict_violation';
    END IF;
    RETURN NEW;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION morningstar.audit_events_guard() RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
BEGIN
    -- Audit rows are never updated, deleted or truncated, by any role, the owner
    -- included. M3 builds no purge path. The retention procedure (an audited
    -- function with an age floor) is a later milestone (TD-M3-03) and will change
    -- this by a reviewed migration.
    RAISE EXCEPTION 'table %.% is append-only: % is refused', TG_TABLE_SCHEMA, TG_TABLE_NAME, TG_OP
        USING ERRCODE = 'restrict_violation';
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION morningstar.recovery_state_guard() RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
BEGIN
    -- The recovery generation only goes up, and only inside a reconciliation
    -- (recovery-generation.md section 3): it rises together with state
    -- 'reconciling', and phase 3 returns to 'normal' at the same generation.
    IF NEW.instance_id <> OLD.instance_id THEN
        RAISE EXCEPTION 'the instance id of the recovery state is fixed' USING ERRCODE = 'restrict_violation';
    END IF;
    IF NEW.generation < OLD.generation THEN
        RAISE EXCEPTION 'the recovery generation cannot go down' USING ERRCODE = 'restrict_violation';
    END IF;
    IF NEW.generation > OLD.generation AND NEW.state <> 'reconciling' THEN
        RAISE EXCEPTION 'the recovery generation rises only while reconciling' USING ERRCODE = 'restrict_violation';
    END IF;
    IF NEW.generation = OLD.generation AND OLD.state = 'normal' AND NEW.state = 'reconciling' THEN
        RAISE EXCEPTION 'a reconciliation starts with a higher generation' USING ERRCODE = 'restrict_violation';
    END IF;
    RETURN NEW;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION morningstar.approvals_guard() RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
BEGIN
    -- The status moves forward only: pending to approved or rejected, approved to
    -- revoked. Rejected and revoked are final (D-06, FR-APPROVAL).
    IF OLD.status <> NEW.status AND NOT (
        (OLD.status = 'pending' AND NEW.status IN ('approved', 'rejected'))
        OR (OLD.status = 'approved' AND NEW.status = 'revoked')
    ) THEN
        RAISE EXCEPTION 'approval % cannot change from % to %', OLD.id, OLD.status, NEW.status
            USING ERRCODE = 'restrict_violation';
    END IF;
    -- The terms of a decided approval are fixed. The use counter, the revocation
    -- columns and the bookkeeping columns are the only ones that change later.
    IF OLD.status <> 'pending' AND (NEW.decided_by, NEW.decided_at, NEW.expires_at, NEW.max_uses, NEW.max_attempts, NEW.reauth_grant_id)
        IS DISTINCT FROM (OLD.decided_by, OLD.decided_at, OLD.expires_at, OLD.max_uses, OLD.max_attempts, OLD.reauth_grant_id) THEN
        RAISE EXCEPTION 'the decision and the terms of approval % are fixed', OLD.id
            USING ERRCODE = 'restrict_violation';
    END IF;
    RETURN NEW;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION morningstar.check_approval_use_parity() RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
DECLARE
    v_id uuid;
    v_consumed integer;
    v_count bigint;
BEGIN
    -- DB-13: the counter of an approval equals the number of its approval_uses rows at commit.
    IF TG_TABLE_NAME = 'approvals' THEN
        v_id := NEW.id;
    ELSE
        v_id := NEW.approval_id;
    END IF;
    SELECT uses_consumed INTO v_consumed FROM morningstar.approvals WHERE id = v_id;
    SELECT count(*) INTO v_count FROM morningstar.approval_uses WHERE approval_id = v_id;
    IF v_consumed IS DISTINCT FROM v_count THEN
        RAISE EXCEPTION 'approval % has uses_consumed % and % approval_uses rows', v_id, v_consumed, v_count
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN NULL;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION morningstar.check_job_approval_use() RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
BEGIN
    -- jobs.approval_use_id equals the approval_uses row of (approval_id, id) (CC-01, DB-13).
    IF NEW.approval_use_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM morningstar.approval_uses u
         WHERE u.id = NEW.approval_use_id AND u.approval_id = NEW.approval_id AND u.job_id = NEW.id
    ) THEN
        RAISE EXCEPTION 'job % names approval use % that does not belong to it', NEW.id, NEW.approval_use_id
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN NULL;
END
$$;
-- +goose StatementEnd

CREATE TRIGGER login_attempts_no_update_delete BEFORE UPDATE OR DELETE ON morningstar.login_attempts FOR EACH ROW EXECUTE FUNCTION morningstar.forbid_update_delete();
CREATE TRIGGER login_attempts_no_truncate BEFORE TRUNCATE ON morningstar.login_attempts FOR EACH STATEMENT EXECUTE FUNCTION morningstar.forbid_truncate();
CREATE TRIGGER program_rules_versions_no_update_delete BEFORE UPDATE OR DELETE ON morningstar.program_rules_versions FOR EACH ROW EXECUTE FUNCTION morningstar.forbid_update_delete();
CREATE TRIGGER program_rules_versions_no_truncate BEFORE TRUNCATE ON morningstar.program_rules_versions FOR EACH STATEMENT EXECUTE FUNCTION morningstar.forbid_truncate();
CREATE TRIGGER scope_rules_no_update_delete BEFORE UPDATE OR DELETE ON morningstar.scope_rules FOR EACH ROW EXECUTE FUNCTION morningstar.forbid_update_delete();
CREATE TRIGGER scope_rules_no_truncate BEFORE TRUNCATE ON morningstar.scope_rules FOR EACH STATEMENT EXECUTE FUNCTION morningstar.forbid_truncate();
CREATE TRIGGER lab_registration_ranges_no_update_delete BEFORE UPDATE OR DELETE ON morningstar.lab_registration_ranges FOR EACH ROW EXECUTE FUNCTION morningstar.forbid_update_delete();
CREATE TRIGGER lab_registration_ranges_no_truncate BEFORE TRUNCATE ON morningstar.lab_registration_ranges FOR EACH STATEMENT EXECUTE FUNCTION morningstar.forbid_truncate();
CREATE TRIGGER target_validation_targets_no_update_delete BEFORE UPDATE OR DELETE ON morningstar.target_validation_targets FOR EACH ROW EXECUTE FUNCTION morningstar.forbid_update_delete();
CREATE TRIGGER target_validation_targets_no_truncate BEFORE TRUNCATE ON morningstar.target_validation_targets FOR EACH STATEMENT EXECUTE FUNCTION morningstar.forbid_truncate();
CREATE TRIGGER assessment_transitions_no_update_delete BEFORE UPDATE OR DELETE ON morningstar.assessment_transitions FOR EACH ROW EXECUTE FUNCTION morningstar.forbid_update_delete();
CREATE TRIGGER assessment_transitions_no_truncate BEFORE TRUNCATE ON morningstar.assessment_transitions FOR EACH STATEMENT EXECUTE FUNCTION morningstar.forbid_truncate();
CREATE TRIGGER operations_no_update_delete BEFORE UPDATE OR DELETE ON morningstar.operations FOR EACH ROW EXECUTE FUNCTION morningstar.forbid_update_delete();
CREATE TRIGGER operations_no_truncate BEFORE TRUNCATE ON morningstar.operations FOR EACH STATEMENT EXECUTE FUNCTION morningstar.forbid_truncate();
CREATE TRIGGER policy_decisions_no_update_delete BEFORE UPDATE OR DELETE ON morningstar.policy_decisions FOR EACH ROW EXECUTE FUNCTION morningstar.forbid_update_delete();
CREATE TRIGGER policy_decisions_no_truncate BEFORE TRUNCATE ON morningstar.policy_decisions FOR EACH STATEMENT EXECUTE FUNCTION morningstar.forbid_truncate();
CREATE TRIGGER approval_events_no_update_delete BEFORE UPDATE OR DELETE ON morningstar.approval_events FOR EACH ROW EXECUTE FUNCTION morningstar.forbid_update_delete();
CREATE TRIGGER approval_events_no_truncate BEFORE TRUNCATE ON morningstar.approval_events FOR EACH STATEMENT EXECUTE FUNCTION morningstar.forbid_truncate();
CREATE TRIGGER approval_uses_no_update_delete BEFORE UPDATE OR DELETE ON morningstar.approval_uses FOR EACH ROW EXECUTE FUNCTION morningstar.forbid_update_delete();
CREATE TRIGGER approval_uses_no_truncate BEFORE TRUNCATE ON morningstar.approval_uses FOR EACH STATEMENT EXECUTE FUNCTION morningstar.forbid_truncate();
CREATE TRIGGER job_transitions_no_update_delete BEFORE UPDATE OR DELETE ON morningstar.job_transitions FOR EACH ROW EXECUTE FUNCTION morningstar.forbid_update_delete();
CREATE TRIGGER job_transitions_no_truncate BEFORE TRUNCATE ON morningstar.job_transitions FOR EACH STATEMENT EXECUTE FUNCTION morningstar.forbid_truncate();

-- Tables that allow an UPDATE of a few named columns and no DELETE.
CREATE TRIGGER user_credentials_update_columns BEFORE UPDATE ON morningstar.user_credentials FOR EACH ROW EXECUTE FUNCTION morningstar.forbid_update_except('is_current', 'superseded_at');
CREATE TRIGGER user_credentials_no_delete BEFORE DELETE ON morningstar.user_credentials FOR EACH ROW EXECUTE FUNCTION morningstar.forbid_update_delete();
CREATE TRIGGER user_credentials_no_truncate BEFORE TRUNCATE ON morningstar.user_credentials FOR EACH STATEMENT EXECUTE FUNCTION morningstar.forbid_truncate();
CREATE TRIGGER policy_settings_update_columns BEFORE UPDATE ON morningstar.policy_settings FOR EACH ROW EXECUTE FUNCTION morningstar.forbid_update_except('superseded_at');
CREATE TRIGGER policy_settings_no_delete BEFORE DELETE ON morningstar.policy_settings FOR EACH ROW EXECUTE FUNCTION morningstar.forbid_update_delete();
CREATE TRIGGER policy_settings_no_truncate BEFORE TRUNCATE ON morningstar.policy_settings FOR EACH STATEMENT EXECUTE FUNCTION morningstar.forbid_truncate();
CREATE TRIGGER scope_versions_update_columns BEFORE UPDATE ON morningstar.scope_versions FOR EACH ROW EXECUTE FUNCTION morningstar.forbid_update_except('superseded_at');
CREATE TRIGGER scope_versions_no_delete BEFORE DELETE ON morningstar.scope_versions FOR EACH ROW EXECUTE FUNCTION morningstar.forbid_update_delete();
CREATE TRIGGER scope_versions_no_truncate BEFORE TRUNCATE ON morningstar.scope_versions FOR EACH STATEMENT EXECUTE FUNCTION morningstar.forbid_truncate();
CREATE TRIGGER lab_registrations_update_columns BEFORE UPDATE ON morningstar.lab_registrations FOR EACH ROW EXECUTE FUNCTION morningstar.forbid_update_except('status', 'revoked_at', 'revoked_by', 'revoke_reason');
CREATE TRIGGER lab_registrations_no_delete BEFORE DELETE ON morningstar.lab_registrations FOR EACH ROW EXECUTE FUNCTION morningstar.forbid_update_delete();
CREATE TRIGGER lab_registrations_no_truncate BEFORE TRUNCATE ON morningstar.lab_registrations FOR EACH STATEMENT EXECUTE FUNCTION morningstar.forbid_truncate();
CREATE TRIGGER target_validations_update_columns BEFORE UPDATE ON morningstar.target_validations FOR EACH ROW EXECUTE FUNCTION morningstar.forbid_update_except('status', 'invalidated_at', 'invalidation_reason');
CREATE TRIGGER target_validations_no_delete BEFORE DELETE ON morningstar.target_validations FOR EACH ROW EXECUTE FUNCTION morningstar.forbid_update_delete();
CREATE TRIGGER target_validations_no_truncate BEFORE TRUNCATE ON morningstar.target_validations FOR EACH STATEMENT EXECUTE FUNCTION morningstar.forbid_truncate();
CREATE TRIGGER job_attempts_update_columns BEFORE UPDATE ON morningstar.job_attempts FOR EACH ROW EXECUTE FUNCTION morningstar.forbid_update_except('state', 'process_group_ref', 'ended_at', 'termination_confirmed_at', 'exit_code', 'error_class', 'tool_version', 'tool_binary_sha256', 'sandbox_digest', 'argv_redacted');
CREATE TRIGGER job_attempts_no_delete BEFORE DELETE ON morningstar.job_attempts FOR EACH ROW EXECUTE FUNCTION morningstar.forbid_update_delete();
CREATE TRIGGER job_attempts_no_truncate BEFORE TRUNCATE ON morningstar.job_attempts FOR EACH STATEMENT EXECUTE FUNCTION morningstar.forbid_truncate();

-- Audit events: append-only, with a narrow retention path (DB-17).
CREATE TRIGGER audit_events_guard_rows BEFORE UPDATE OR DELETE ON morningstar.audit_events FOR EACH ROW EXECUTE FUNCTION morningstar.audit_events_guard();
CREATE TRIGGER audit_events_guard_truncate BEFORE TRUNCATE ON morningstar.audit_events FOR EACH STATEMENT EXECUTE FUNCTION morningstar.audit_events_guard();

-- The recovery state is one row. Its generation never goes down.
CREATE TRIGGER recovery_state_guard_update BEFORE UPDATE ON morningstar.recovery_state FOR EACH ROW EXECUTE FUNCTION morningstar.recovery_state_guard();
CREATE TRIGGER recovery_state_no_delete BEFORE DELETE ON morningstar.recovery_state FOR EACH ROW EXECUTE FUNCTION morningstar.forbid_update_delete();
CREATE TRIGGER recovery_state_no_truncate BEFORE TRUNCATE ON morningstar.recovery_state FOR EACH STATEMENT EXECUTE FUNCTION morningstar.forbid_truncate();

-- Approval decision and terms are frozen once decided; the status moves forward only.
CREATE TRIGGER approvals_guard_update BEFORE UPDATE ON morningstar.approvals FOR EACH ROW EXECUTE FUNCTION morningstar.approvals_guard();

-- Deferred parity checks (DB-13). They run at commit.
CREATE CONSTRAINT TRIGGER approvals_uses_parity AFTER INSERT OR UPDATE OF uses_consumed ON morningstar.approvals
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION morningstar.check_approval_use_parity();
CREATE CONSTRAINT TRIGGER approval_uses_parity AFTER INSERT ON morningstar.approval_uses
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION morningstar.check_approval_use_parity();
CREATE CONSTRAINT TRIGGER jobs_approval_use_match AFTER INSERT OR UPDATE OF approval_use_id, approval_id ON morningstar.jobs
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION morningstar.check_job_approval_use();
