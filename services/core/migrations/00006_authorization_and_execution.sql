-- +goose Up
-- Slice 00006 authorization_and_execution (milestone M3). Generated from docs/design/database/data-dictionary.md and constraints-and-indexes.md, then reviewed.
-- Foreign keys are created in the migration of the later of their two tables (DB-21). Indexes, triggers and grants are in 00013 to 00015.

CREATE TABLE morningstar.operations (
    id uuid NOT NULL,
    program_id uuid NOT NULL,
    assessment_id uuid NOT NULL,
    operation_hash bytea NOT NULL,
    tool_key text NOT NULL,
    operation_kind text NOT NULL,
    risk_class text NOT NULL,
    canonical_target text NOT NULL,
    parameters jsonb NOT NULL DEFAULT '{}',
    declared_destinations jsonb NOT NULL DEFAULT '[]',
    limits jsonb NOT NULL DEFAULT '{}',
    manifest_digest bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT operations_pk PRIMARY KEY (id),
    CONSTRAINT operations_ck_c1 CHECK (risk_class IN ('passive', 'low_impact', 'intrusive')),
    CONSTRAINT operations_ck_c2 CHECK (length(operation_hash) = 32 AND length(manifest_digest) = 32),
    CONSTRAINT operations_uq_c3 UNIQUE (assessment_id, operation_hash),
    CONSTRAINT operations_uq_c4 UNIQUE (assessment_id, id)
);

CREATE TABLE morningstar.policy_decisions (
    id uuid NOT NULL,
    program_id uuid NOT NULL,
    assessment_id uuid NOT NULL,
    point text NOT NULL,
    operation_id uuid,
    job_id uuid,
    outcome text NOT NULL,
    reasons jsonb NOT NULL DEFAULT '[]',
    scope_version_id uuid NOT NULL,
    validation_id uuid,
    approval_id uuid,
    lab_registration_id uuid,
    policy_snapshot_hash bytea NOT NULL,
    decided_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT policy_decisions_pk PRIMARY KEY (id),
    CONSTRAINT policy_decisions_ck_c1 CHECK (point IN ('E1', 'E2', 'E3', 'E4')),
    CONSTRAINT policy_decisions_ck_c2 CHECK (outcome IN ('allow', 'deny', 'require_approval')),
    CONSTRAINT policy_decisions_uq_c5 UNIQUE (program_id, id)
);

CREATE TABLE morningstar.approvals (
    id uuid NOT NULL,
    program_id uuid NOT NULL,
    assessment_id uuid NOT NULL,
    operation_id uuid NOT NULL,
    risk_class text NOT NULL,
    status text NOT NULL DEFAULT 'pending',
    requested_by_user_id uuid NOT NULL,
    decided_by uuid,
    decided_at timestamptz,
    expires_at timestamptz,
    max_uses integer NOT NULL DEFAULT 1,
    uses_consumed integer NOT NULL DEFAULT 0,
    max_attempts integer NOT NULL DEFAULT 1,
    validation_id uuid NOT NULL,
    scope_version_id uuid NOT NULL,
    policy_snapshot_hash bytea NOT NULL,
    reauth_grant_id uuid,
    revoked_at timestamptz,
    revoked_by uuid,
    revoke_reason text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version integer NOT NULL DEFAULT 1,
    CONSTRAINT approvals_pk PRIMARY KEY (id),
    CONSTRAINT approvals_ck_c1 CHECK (status IN ('pending', 'approved', 'rejected', 'revoked')),
    CONSTRAINT approvals_ck_c2 CHECK (risk_class IN ('low_impact', 'intrusive')),
    CONSTRAINT approvals_ck_c3 CHECK (uses_consumed BETWEEN 0 AND max_uses),
    CONSTRAINT approvals_ck_c4 CHECK (max_uses >= 1 AND max_attempts BETWEEN 1 AND 3),
    CONSTRAINT approvals_ck_c5 CHECK ((risk_class <> 'intrusive') OR (max_uses = 1 AND max_attempts = 1)),
    CONSTRAINT approvals_ck_c6 CHECK ((status = 'pending') = (decided_by IS NULL AND decided_at IS NULL)),
    CONSTRAINT approvals_ck_c7 CHECK ((status <> 'approved') OR expires_at IS NOT NULL),
    CONSTRAINT approvals_ck_c8 CHECK ((risk_class <> 'intrusive') OR status <> 'approved' OR reauth_grant_id IS NOT NULL),
    CONSTRAINT approvals_ck_c9 CHECK ((status = 'revoked') = (revoked_at IS NOT NULL)),
    CONSTRAINT approvals_uq_c14 UNIQUE (assessment_id, id),
    CONSTRAINT approvals_uq_c15 UNIQUE (reauth_grant_id)
);

CREATE TABLE morningstar.approval_events (
    id uuid NOT NULL,
    approval_id uuid NOT NULL,
    event_type text NOT NULL,
    actor_kind text NOT NULL,
    actor_user_id uuid,
    job_id uuid,
    reason_code text,
    request_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT approval_events_pk PRIMARY KEY (id),
    CONSTRAINT approval_events_ck_c1 CHECK (event_type IN ('requested', 'approved', 'rejected', 'revoked', 'use_consumed', 'use_denied', 'expiry_observed')),
    CONSTRAINT approval_events_ck_c2 CHECK (actor_kind IN ('user', 'worker', 'system')),
    CONSTRAINT approval_events_ck_c3 CHECK (event_type NOT IN ('approved', 'rejected', 'revoked') OR (actor_kind = 'user' AND actor_user_id IS NOT NULL))
);

CREATE TABLE morningstar.approval_uses (
    id uuid NOT NULL,
    approval_id uuid NOT NULL,
    job_id uuid NOT NULL,
    claim_epoch integer NOT NULL,
    consumed_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT approval_uses_pk PRIMARY KEY (id),
    CONSTRAINT approval_uses_uq_c1 UNIQUE (approval_id, job_id)
);

CREATE TABLE morningstar.jobs (
    id uuid NOT NULL,
    program_id uuid NOT NULL,
    assessment_id uuid NOT NULL,
    job_kind text NOT NULL,
    operation_id uuid,
    plan_item_id uuid,
    risk_class text NOT NULL,
    state text NOT NULL DEFAULT 'queued',
    control_state text NOT NULL DEFAULT 'none',
    control_reason text,
    control_requested_at timestamptz,
    approval_id uuid,
    approval_use_id uuid,
    scope_version_id uuid NOT NULL,
    validation_id uuid,
    max_execution_attempts integer NOT NULL DEFAULT 1,
    execution_attempts integer NOT NULL DEFAULT 0,
    available_at timestamptz NOT NULL DEFAULT now(),
    priority smallint NOT NULL DEFAULT 0,
    lease_owner text,
    lease_expires_at timestamptz,
    heartbeat_at timestamptz,
    claim_epoch integer NOT NULL DEFAULT 0,
    retry_policy text NOT NULL DEFAULT 'none',
    failure_class text,
    created_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version integer NOT NULL DEFAULT 1,
    finished_at timestamptz,
    CONSTRAINT jobs_pk PRIMARY KEY (id),
    CONSTRAINT jobs_ck_c1 CHECK (job_kind IN ('tool_operation', 'ai_task')),
    CONSTRAINT jobs_ck_c2 CHECK (risk_class IN ('passive', 'low_impact', 'intrusive', 'ai')),
    CONSTRAINT jobs_ck_c3 CHECK (state IN ('queued', 'awaiting_approval', 'running', 'succeeded', 'failed', 'cancelled')),
    CONSTRAINT jobs_ck_c4 CHECK (control_state IN ('none', 'stop_requested', 'stop_confirmed', 'stop_failed')),
    CONSTRAINT jobs_ck_c5 CHECK ((job_kind = 'tool_operation') = (operation_id IS NOT NULL)),
    CONSTRAINT jobs_ck_c6 CHECK (risk_class IN ('passive', 'ai') OR approval_id IS NOT NULL),
    CONSTRAINT jobs_ck_c7 CHECK (state NOT IN ('running', 'succeeded', 'failed') OR risk_class IN ('passive', 'ai') OR approval_use_id IS NOT NULL OR failure_class = 'policy_denial'),
    CONSTRAINT jobs_ck_c8 CHECK (execution_attempts BETWEEN 0 AND max_execution_attempts),
    CONSTRAINT jobs_ck_c9 CHECK (max_execution_attempts BETWEEN 1 AND 3),
    CONSTRAINT jobs_ck_c10 CHECK (risk_class <> 'intrusive' OR max_execution_attempts = 1),
    CONSTRAINT jobs_ck_c11 CHECK (state <> 'running' OR (lease_owner IS NOT NULL AND lease_expires_at IS NOT NULL)),
    CONSTRAINT jobs_ck_c12 CHECK ((state IN ('succeeded', 'failed', 'cancelled')) = (finished_at IS NOT NULL)),
    CONSTRAINT jobs_uq_c18 UNIQUE (assessment_id, id)
);

CREATE TABLE morningstar.job_attempts (
    id uuid NOT NULL,
    job_id uuid NOT NULL,
    attempt_no smallint NOT NULL,
    claim_epoch integer NOT NULL,
    approval_use_id uuid,
    decision_id uuid NOT NULL,
    state text NOT NULL DEFAULT 'launching',
    worker_instance text NOT NULL,
    process_group_ref text,
    started_at timestamptz NOT NULL DEFAULT now(),
    ended_at timestamptz,
    termination_confirmed_at timestamptz,
    exit_code integer,
    error_class text,
    tool_version text,
    tool_binary_sha256 bytea,
    sandbox_digest bytea,
    argv_redacted jsonb,
    limits_applied jsonb,
    output_bytes bigint,
    CONSTRAINT job_attempts_pk PRIMARY KEY (id),
    CONSTRAINT job_attempts_uq_c1 UNIQUE (job_id, attempt_no),
    CONSTRAINT job_attempts_ck_c2 CHECK (attempt_no BETWEEN 1 AND 3),
    CONSTRAINT job_attempts_ck_c3 CHECK (state IN ('launching', 'running', 'succeeded', 'failed', 'timed_out', 'killed', 'cancelled', 'lost')),
    CONSTRAINT job_attempts_ck_c4 CHECK (state IN ('launching', 'running', 'lost') OR termination_confirmed_at IS NOT NULL),
    CONSTRAINT job_attempts_ck_c5 CHECK ((state IN ('launching', 'running')) = (ended_at IS NULL)),
    CONSTRAINT job_attempts_ck_c6 CHECK (octet_length(tool_binary_sha256) = 32 OR tool_binary_sha256 IS NULL)
);

CREATE TABLE morningstar.job_transitions (
    id uuid NOT NULL,
    job_id uuid NOT NULL,
    from_state text,
    to_state text NOT NULL,
    control_state text NOT NULL,
    claim_epoch integer NOT NULL,
    actor_kind text NOT NULL,
    actor_user_id uuid,
    reason text,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT job_transitions_pk PRIMARY KEY (id),
    CONSTRAINT job_transitions_ck_c1 CHECK (actor_kind IN ('user', 'worker', 'system')),
    CONSTRAINT job_transitions_ck_c2 CHECK ((from_state IS NULL AND to_state = 'queued') OR (from_state IS NULL AND to_state = 'awaiting_approval') OR (from_state = 'awaiting_approval' AND to_state = 'queued') OR (from_state = 'queued' AND to_state = 'running') OR (from_state = 'running' AND to_state = 'succeeded') OR (from_state = 'running' AND to_state = 'failed') OR (from_state = 'running' AND to_state = 'queued') OR (from_state = 'queued' AND to_state = 'failed') OR (from_state = 'queued' AND to_state = 'cancelled') OR (from_state = 'awaiting_approval' AND to_state = 'cancelled') OR (from_state = 'running' AND to_state = 'cancelled'))
);

ALTER TABLE morningstar.plan_items ADD CONSTRAINT plan_items_fk_operation_id FOREIGN KEY (operation_id) REFERENCES morningstar.operations (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.plan_items ADD CONSTRAINT plan_items_fk_ai_job_id FOREIGN KEY (ai_job_id) REFERENCES morningstar.jobs (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.plan_items ADD CONSTRAINT plan_items_fk_assessment_id_operation_id FOREIGN KEY (assessment_id, operation_id) REFERENCES morningstar.operations (assessment_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.operations ADD CONSTRAINT operations_fk_program_id FOREIGN KEY (program_id) REFERENCES morningstar.programs (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.operations ADD CONSTRAINT operations_fk_assessment_id FOREIGN KEY (assessment_id) REFERENCES morningstar.assessments (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.operations ADD CONSTRAINT operations_fk_program_id_assessment_id FOREIGN KEY (program_id, assessment_id) REFERENCES morningstar.assessments (program_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.policy_decisions ADD CONSTRAINT policy_decisions_fk_program_id FOREIGN KEY (program_id) REFERENCES morningstar.programs (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.policy_decisions ADD CONSTRAINT policy_decisions_fk_assessment_id FOREIGN KEY (assessment_id) REFERENCES morningstar.assessments (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.policy_decisions ADD CONSTRAINT policy_decisions_fk_operation_id FOREIGN KEY (operation_id) REFERENCES morningstar.operations (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.policy_decisions ADD CONSTRAINT policy_decisions_fk_job_id FOREIGN KEY (job_id) REFERENCES morningstar.jobs (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.policy_decisions ADD CONSTRAINT policy_decisions_fk_scope_version_id FOREIGN KEY (scope_version_id) REFERENCES morningstar.scope_versions (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.policy_decisions ADD CONSTRAINT policy_decisions_fk_validation_id FOREIGN KEY (validation_id) REFERENCES morningstar.target_validations (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.policy_decisions ADD CONSTRAINT policy_decisions_fk_approval_id FOREIGN KEY (approval_id) REFERENCES morningstar.approvals (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.policy_decisions ADD CONSTRAINT policy_decisions_fk_lab_registration_id FOREIGN KEY (lab_registration_id) REFERENCES morningstar.lab_registrations (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.policy_decisions ADD CONSTRAINT policy_decisions_fk_program_id_assessment_id FOREIGN KEY (program_id, assessment_id) REFERENCES morningstar.assessments (program_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.policy_decisions ADD CONSTRAINT policy_decisions_fk_program_id_scope_version_id FOREIGN KEY (program_id, scope_version_id) REFERENCES morningstar.scope_versions (program_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.approvals ADD CONSTRAINT approvals_fk_program_id FOREIGN KEY (program_id) REFERENCES morningstar.programs (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.approvals ADD CONSTRAINT approvals_fk_assessment_id FOREIGN KEY (assessment_id) REFERENCES morningstar.assessments (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.approvals ADD CONSTRAINT approvals_fk_operation_id FOREIGN KEY (operation_id) REFERENCES morningstar.operations (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.approvals ADD CONSTRAINT approvals_fk_requested_by_user_id FOREIGN KEY (requested_by_user_id) REFERENCES morningstar.users (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.approvals ADD CONSTRAINT approvals_fk_decided_by FOREIGN KEY (decided_by) REFERENCES morningstar.users (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.approvals ADD CONSTRAINT approvals_fk_validation_id FOREIGN KEY (validation_id) REFERENCES morningstar.target_validations (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.approvals ADD CONSTRAINT approvals_fk_scope_version_id FOREIGN KEY (scope_version_id) REFERENCES morningstar.scope_versions (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.approvals ADD CONSTRAINT approvals_fk_reauth_grant_id FOREIGN KEY (reauth_grant_id) REFERENCES morningstar.reauth_grants (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.approvals ADD CONSTRAINT approvals_fk_revoked_by FOREIGN KEY (revoked_by) REFERENCES morningstar.users (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.approvals ADD CONSTRAINT approvals_fk_program_id_assessment_id FOREIGN KEY (program_id, assessment_id) REFERENCES morningstar.assessments (program_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.approvals ADD CONSTRAINT approvals_fk_assessment_id_operation_id FOREIGN KEY (assessment_id, operation_id) REFERENCES morningstar.operations (assessment_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.approvals ADD CONSTRAINT approvals_fk_program_id_validation_id FOREIGN KEY (program_id, validation_id) REFERENCES morningstar.target_validations (program_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.approvals ADD CONSTRAINT approvals_fk_program_id_scope_version_id FOREIGN KEY (program_id, scope_version_id) REFERENCES morningstar.scope_versions (program_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.approval_events ADD CONSTRAINT approval_events_fk_approval_id FOREIGN KEY (approval_id) REFERENCES morningstar.approvals (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.approval_events ADD CONSTRAINT approval_events_fk_actor_user_id FOREIGN KEY (actor_user_id) REFERENCES morningstar.users (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.approval_events ADD CONSTRAINT approval_events_fk_job_id FOREIGN KEY (job_id) REFERENCES morningstar.jobs (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.approval_uses ADD CONSTRAINT approval_uses_fk_approval_id FOREIGN KEY (approval_id) REFERENCES morningstar.approvals (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.approval_uses ADD CONSTRAINT approval_uses_fk_job_id FOREIGN KEY (job_id) REFERENCES morningstar.jobs (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.jobs ADD CONSTRAINT jobs_fk_program_id FOREIGN KEY (program_id) REFERENCES morningstar.programs (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.jobs ADD CONSTRAINT jobs_fk_assessment_id FOREIGN KEY (assessment_id) REFERENCES morningstar.assessments (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.jobs ADD CONSTRAINT jobs_fk_operation_id FOREIGN KEY (operation_id) REFERENCES morningstar.operations (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.jobs ADD CONSTRAINT jobs_fk_plan_item_id FOREIGN KEY (plan_item_id) REFERENCES morningstar.plan_items (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.jobs ADD CONSTRAINT jobs_fk_approval_id FOREIGN KEY (approval_id) REFERENCES morningstar.approvals (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.jobs ADD CONSTRAINT jobs_fk_approval_use_id FOREIGN KEY (approval_use_id) REFERENCES morningstar.approval_uses (id) ON DELETE RESTRICT ON UPDATE RESTRICT DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE morningstar.jobs ADD CONSTRAINT jobs_fk_scope_version_id FOREIGN KEY (scope_version_id) REFERENCES morningstar.scope_versions (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.jobs ADD CONSTRAINT jobs_fk_validation_id FOREIGN KEY (validation_id) REFERENCES morningstar.target_validations (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.jobs ADD CONSTRAINT jobs_fk_created_by FOREIGN KEY (created_by) REFERENCES morningstar.users (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.jobs ADD CONSTRAINT jobs_fk_program_id_assessment_id FOREIGN KEY (program_id, assessment_id) REFERENCES morningstar.assessments (program_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.jobs ADD CONSTRAINT jobs_fk_assessment_id_operation_id FOREIGN KEY (assessment_id, operation_id) REFERENCES morningstar.operations (assessment_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.jobs ADD CONSTRAINT jobs_fk_assessment_id_approval_id FOREIGN KEY (assessment_id, approval_id) REFERENCES morningstar.approvals (assessment_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.jobs ADD CONSTRAINT jobs_fk_program_id_scope_version_id FOREIGN KEY (program_id, scope_version_id) REFERENCES morningstar.scope_versions (program_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.job_attempts ADD CONSTRAINT job_attempts_fk_job_id FOREIGN KEY (job_id) REFERENCES morningstar.jobs (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.job_attempts ADD CONSTRAINT job_attempts_fk_approval_use_id FOREIGN KEY (approval_use_id) REFERENCES morningstar.approval_uses (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.job_attempts ADD CONSTRAINT job_attempts_fk_decision_id FOREIGN KEY (decision_id) REFERENCES morningstar.policy_decisions (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.job_transitions ADD CONSTRAINT job_transitions_fk_job_id FOREIGN KEY (job_id) REFERENCES morningstar.jobs (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.job_transitions ADD CONSTRAINT job_transitions_fk_actor_user_id FOREIGN KEY (actor_user_id) REFERENCES morningstar.users (id) ON DELETE RESTRICT ON UPDATE RESTRICT;

-- ---------------------------------------------------------------------------
-- Approval consumption (M3 task 3.5, DB-13, CC-01, CC-02, CC-15, TD-M2-05 M3 part).
--
-- The worker role has no UPDATE on approvals. It consumes a use only through
-- consume_approval_use, which runs with the rights of ms_migrator. The function
-- is the SQL form of the claim step of the M2 model (internal/domain/approval,
-- CheckClaim and Claim): the same checks in the same order and the same stable
-- reason codes. All time comparisons use the database clock (DB-08).
-- ---------------------------------------------------------------------------

-- +goose StatementBegin
CREATE FUNCTION morningstar.approval_claim_denial(
    p_approval_id uuid,
    p_operation_hash bytea,
    p_scope_version_id uuid,
    p_policy_snapshot_hash bytea,
    p_validation_id uuid
) RETURNS text
LANGUAGE plpgsql SECURITY DEFINER
SET search_path = pg_catalog, morningstar
AS $$
DECLARE
    a record;
BEGIN
    -- A comparison with NULL is NULL, and an IF treats NULL as false. A missing
    -- argument must refuse, never pass (the worker may call this directly).
    IF p_approval_id IS NULL OR p_operation_hash IS NULL OR p_scope_version_id IS NULL
       OR p_policy_snapshot_hash IS NULL OR p_validation_id IS NULL THEN
        RETURN 'invalid_input';
    END IF;
    SELECT ap.status, ap.expires_at, ap.scope_version_id, ap.policy_snapshot_hash, ap.validation_id, o.operation_hash
      INTO a
      FROM morningstar.approvals ap
      JOIN morningstar.operations o ON o.id = ap.operation_id
     WHERE ap.id = p_approval_id;
    IF NOT FOUND THEN
        RETURN 'approval_not_approved';
    END IF;
    IF a.status = 'revoked' THEN
        RETURN 'approval_revoked';
    END IF;
    IF a.status <> 'approved' THEN
        RETURN 'approval_not_approved';
    END IF;
    IF a.expires_at <= clock_timestamp() THEN
        RETURN 'approval_expired';
    END IF;
    IF a.operation_hash <> p_operation_hash THEN
        RETURN 'operation_mismatch';
    END IF;
    IF a.scope_version_id <> p_scope_version_id THEN
        RETURN 'approval_scope_changed';
    END IF;
    IF a.policy_snapshot_hash <> p_policy_snapshot_hash THEN
        RETURN 'approval_policy_changed';
    END IF;
    IF a.validation_id <> p_validation_id THEN
        RETURN 'approval_validation_changed';
    END IF;
    RETURN NULL;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION morningstar.approval_retry_claim(
    p_approval_id uuid,
    p_job_id uuid,
    p_use_id uuid,
    p_operation_hash bytea,
    p_scope_version_id uuid,
    p_policy_snapshot_hash bytea,
    p_validation_id uuid,
    OUT outcome text,
    OUT reason text,
    OUT use_id uuid
)
LANGUAGE plpgsql SECURITY DEFINER
SET search_path = pg_catalog, morningstar
AS $$
DECLARE
    v_reason text;
    v_class text;
    v_max_attempts integer;
    v_attempts integer;
BEGIN
    -- A job that holds a use claims again: the record is reused and nothing is
    -- consumed. The rules are CheckClaim of the M2 model for a job with a use.
    v_reason := morningstar.approval_claim_denial(p_approval_id, p_operation_hash, p_scope_version_id, p_policy_snapshot_hash, p_validation_id);
    IF v_reason IS NULL THEN
        SELECT ap.risk_class, ap.max_attempts INTO v_class, v_max_attempts
          FROM morningstar.approvals ap WHERE ap.id = p_approval_id;
        IF v_class = 'intrusive' THEN
            v_reason := 'intrusive_no_retry';
        ELSE
            SELECT j.execution_attempts INTO v_attempts FROM morningstar.jobs j WHERE j.id = p_job_id;
            IF coalesce(v_attempts, 0) >= v_max_attempts THEN
                v_reason := 'attempts_exhausted';
            END IF;
        END IF;
    END IF;
    IF v_reason IS NOT NULL THEN
        outcome := 'refused'; reason := v_reason; use_id := NULL;
    ELSE
        outcome := 'reused'; reason := NULL; use_id := p_use_id;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION morningstar.consume_approval_use(
    p_use_id uuid,
    p_approval_id uuid,
    p_job_id uuid,
    p_claim_epoch integer,
    p_generation bigint,
    p_operation_hash bytea,
    p_scope_version_id uuid,
    p_policy_snapshot_hash bytea,
    p_validation_id uuid,
    OUT outcome text,
    OUT reason text,
    OUT use_id uuid
)
LANGUAGE plpgsql SECURITY DEFINER
SET search_path = pg_catalog, morningstar
AS $$
DECLARE
    v_reason text;
    v_use uuid;
    v_constraint text;
BEGIN
    use_id := NULL;
    IF p_use_id IS NULL OR p_approval_id IS NULL OR p_job_id IS NULL OR p_claim_epoch IS NULL OR p_generation IS NULL
       OR p_operation_hash IS NULL OR p_scope_version_id IS NULL OR p_policy_snapshot_hash IS NULL OR p_validation_id IS NULL
       OR p_claim_epoch < 1 THEN
        outcome := 'refused'; reason := 'invalid_input';
        RETURN;
    END IF;

    -- CC-15: claims happen only under the current recovery generation. The
    -- shared row lock keeps a reconciliation from starting in the middle of a claim.
    PERFORM 1 FROM morningstar.recovery_state
     WHERE singleton AND state = 'normal' AND generation = p_generation
       FOR SHARE;
    IF NOT FOUND THEN
        outcome := 'refused'; reason := 'recovery_generation';
        RETURN;
    END IF;

    IF NOT EXISTS (SELECT 1 FROM morningstar.approvals WHERE id = p_approval_id) THEN
        outcome := 'refused'; reason := 'approval_not_approved';
        RETURN;
    END IF;
    -- The caller names the job. It must be the job that this approval was made for.
    IF NOT EXISTS (
        SELECT 1 FROM morningstar.jobs j JOIN morningstar.approvals ap ON ap.id = j.approval_id
         WHERE j.id = p_job_id AND ap.id = p_approval_id AND j.operation_id = ap.operation_id
           AND j.program_id = ap.program_id AND j.assessment_id = ap.assessment_id
    ) THEN
        outcome := 'refused'; reason := 'invalid_input';
        RETURN;
    END IF;

    -- A job that holds a use is a retry claim. It reuses the record and consumes nothing.
    SELECT u.id INTO v_use
      FROM morningstar.approval_uses u
     WHERE u.approval_id = p_approval_id AND u.job_id = p_job_id;
    IF FOUND THEN
        SELECT r.outcome, r.reason, r.use_id INTO outcome, reason, use_id
          FROM morningstar.approval_retry_claim(p_approval_id, p_job_id, v_use, p_operation_hash, p_scope_version_id, p_policy_snapshot_hash, p_validation_id) r;
        RETURN;
    END IF;

    BEGIN
        -- One conditional statement (S1.6 item 2). Every validity field is a
        -- column of the approval row, so a concurrent change is seen again after
        -- the row lock is acquired.
        UPDATE morningstar.approvals ap
           SET uses_consumed = ap.uses_consumed + 1, updated_at = now(), version = ap.version + 1
          FROM morningstar.operations o
         WHERE ap.id = p_approval_id
           AND o.id = ap.operation_id
           AND ap.status = 'approved'
           AND ap.revoked_at IS NULL
           AND ap.expires_at > clock_timestamp()
           AND ap.uses_consumed < ap.max_uses
           AND o.operation_hash = p_operation_hash
           AND ap.scope_version_id = p_scope_version_id
           AND ap.policy_snapshot_hash = p_policy_snapshot_hash
           AND ap.validation_id = p_validation_id;
        IF NOT FOUND THEN
            -- Zero rows means refusal. Nothing is consumed and no attempt is counted.
            -- A concurrent claim for this very job may have taken the last use
            -- first. Its record is then this job's use (a retry claim), and the
            -- approval is not exhausted for this job.
            SELECT u.id INTO v_use
              FROM morningstar.approval_uses u
             WHERE u.approval_id = p_approval_id AND u.job_id = p_job_id;
            IF FOUND THEN
                SELECT r.outcome, r.reason, r.use_id INTO outcome, reason, use_id
                  FROM morningstar.approval_retry_claim(p_approval_id, p_job_id, v_use, p_operation_hash, p_scope_version_id, p_policy_snapshot_hash, p_validation_id) r;
                RETURN;
            END IF;
            v_reason := coalesce(
                morningstar.approval_claim_denial(p_approval_id, p_operation_hash, p_scope_version_id, p_policy_snapshot_hash, p_validation_id),
                'approval_exhausted');
            outcome := 'refused'; reason := v_reason;
            RETURN;
        END IF;
        INSERT INTO morningstar.approval_uses (id, approval_id, job_id, claim_epoch)
        VALUES (p_use_id, p_approval_id, p_job_id, p_claim_epoch);
        outcome := 'consumed'; reason := NULL; use_id := p_use_id;
        RETURN;
    EXCEPTION WHEN unique_violation THEN
        GET STACKED DIAGNOSTICS v_constraint = CONSTRAINT_NAME;
        IF v_constraint <> 'approval_uses_uq_c1' THEN
            RAISE;
        END IF;
        -- Another transaction consumed the use of this (approval, job) first.
        -- The block above is rolled back, so the counter is untouched, and this
        -- claim is a retry claim of that record.
        SELECT u.id INTO v_use
          FROM morningstar.approval_uses u
         WHERE u.approval_id = p_approval_id AND u.job_id = p_job_id;
        SELECT r.outcome, r.reason, r.use_id INTO outcome, reason, use_id
          FROM morningstar.approval_retry_claim(p_approval_id, p_job_id, v_use, p_operation_hash, p_scope_version_id, p_policy_snapshot_hash, p_validation_id) r;
        RETURN;
    END;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION morningstar.check_approval_launch(
    p_approval_id uuid,
    p_job_id uuid,
    p_generation bigint,
    p_operation_hash bytea,
    p_scope_version_id uuid,
    p_policy_snapshot_hash bytea,
    p_validation_id uuid
) RETURNS text
LANGUAGE plpgsql SECURITY DEFINER
SET search_path = pg_catalog, morningstar
AS $$
DECLARE
    v_reason text;
BEGIN
    -- E3: expiry, revocation, bindings and the operation hash are checked again
    -- immediately before the launch. This check consumes nothing (S1.6 item 1).
    IF p_approval_id IS NULL OR p_job_id IS NULL OR p_generation IS NULL THEN
        RETURN 'invalid_input';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM morningstar.recovery_state WHERE singleton AND state = 'normal' AND generation = p_generation) THEN
        RETURN 'recovery_generation';
    END IF;
    v_reason := morningstar.approval_claim_denial(p_approval_id, p_operation_hash, p_scope_version_id, p_policy_snapshot_hash, p_validation_id);
    IF v_reason IS NOT NULL THEN
        RETURN v_reason;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM morningstar.jobs j JOIN morningstar.approvals ap ON ap.id = j.approval_id
         WHERE j.id = p_job_id AND ap.id = p_approval_id AND j.operation_id = ap.operation_id
           AND j.program_id = ap.program_id AND j.assessment_id = ap.assessment_id
    ) THEN
        RETURN 'invalid_input';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM morningstar.approval_uses WHERE approval_id = p_approval_id AND job_id = p_job_id) THEN
        RETURN 'use_not_claimed';
    END IF;
    RETURN NULL;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION morningstar.approval_claim_denial(uuid, bytea, uuid, bytea, uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION morningstar.approval_retry_claim(uuid, uuid, uuid, bytea, uuid, bytea, uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION morningstar.consume_approval_use(uuid, uuid, uuid, integer, bigint, bytea, uuid, bytea, uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION morningstar.check_approval_launch(uuid, uuid, bigint, bytea, uuid, bytea, uuid) FROM PUBLIC;
