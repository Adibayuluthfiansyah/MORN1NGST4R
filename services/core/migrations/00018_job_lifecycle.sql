-- +goose Up
-- Slice 00018 job_lifecycle (milestone M7): the database rules of the job state machine and of its attempts (TD-M3-01, TD-M3-02, DB-11, DB-12).
-- The claim and launch functions of 00006 carry the job state and the claim epoch from here on. The old launch check is replaced by one that takes the epoch.

-- +goose StatementBegin
CREATE FUNCTION morningstar.jobs_state_guard() RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.state NOT IN ('queued', 'awaiting_approval') OR NEW.claim_epoch <> 0 OR NEW.execution_attempts <> 0 THEN
            RAISE EXCEPTION 'a job starts queued or awaiting approval, with no claim and no attempt' USING ERRCODE = 'restrict_violation';
        END IF;
        RETURN NEW;
    END IF;
    IF NEW.state <> OLD.state AND NOT (
        (OLD.state = 'awaiting_approval' AND NEW.state IN ('queued', 'cancelled'))
        OR (OLD.state = 'queued' AND NEW.state IN ('running', 'failed', 'cancelled'))
        OR (OLD.state = 'running' AND NEW.state IN ('succeeded', 'failed', 'queued', 'cancelled'))
    ) THEN
        RAISE EXCEPTION 'a job does not move from % to %', OLD.state, NEW.state USING ERRCODE = 'restrict_violation';
    END IF;
    IF NEW.claim_epoch < OLD.claim_epoch THEN
        RAISE EXCEPTION 'the claim epoch of a job only grows' USING ERRCODE = 'restrict_violation';
    END IF;
    IF NEW.execution_attempts < OLD.execution_attempts THEN
        RAISE EXCEPTION 'the attempt counter of a job only grows' USING ERRCODE = 'restrict_violation';
    END IF;
    RETURN NEW;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION morningstar.jobs_transition_parity() RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
BEGIN
    -- A change of state and its history row commit together (DB-11): the row that
    -- names this very change was written by this transaction.
    IF NEW.state <> OLD.state AND NOT EXISTS (
        SELECT 1 FROM morningstar.job_transitions t
         WHERE t.job_id = NEW.id AND t.from_state = OLD.state AND t.to_state = NEW.state AND t.created_at = now()
    ) THEN
        RAISE EXCEPTION 'the change of state of job % has no transition row from this transaction', NEW.id USING ERRCODE = 'restrict_violation';
    END IF;
    RETURN NULL;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION morningstar.job_attempts_guard() RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
DECLARE
    j record;
    n integer;
BEGIN
    -- An attempt is the launch of a running job under the claim that holds it. The
    -- counter of the job was raised for this attempt first (D-55), the number of
    -- attempts never exceeds the approved number, and an intrusive job has one.
    SELECT state, claim_epoch, max_execution_attempts, execution_attempts, risk_class, approval_use_id, control_state INTO j
      FROM morningstar.jobs WHERE id = NEW.job_id FOR SHARE;
    IF NOT FOUND OR j.state <> 'running' OR j.control_state <> 'none' OR NEW.claim_epoch <> j.claim_epoch THEN
        RAISE EXCEPTION 'an attempt is made by the claim that holds a running job' USING ERRCODE = 'restrict_violation';
    END IF;
    SELECT count(*) INTO n FROM morningstar.job_attempts WHERE job_id = NEW.job_id;
    IF n >= j.max_execution_attempts OR (j.risk_class = 'intrusive' AND n >= 1) THEN
        RAISE EXCEPTION 'the attempts of job % are used up', NEW.job_id USING ERRCODE = 'restrict_violation';
    END IF;
    IF NEW.attempt_no <> n + 1 OR j.execution_attempts <> NEW.attempt_no THEN
        RAISE EXCEPTION 'the attempt number and the attempt counter of job % disagree', NEW.job_id USING ERRCODE = 'restrict_violation';
    END IF;
    IF NEW.approval_use_id IS DISTINCT FROM j.approval_use_id THEN
        RAISE EXCEPTION 'the attempt names another approval use than its job' USING ERRCODE = 'restrict_violation';
    END IF;
    RETURN NEW;
END
$$;
-- +goose StatementEnd

CREATE TRIGGER jobs_state_guard BEFORE INSERT OR UPDATE ON morningstar.jobs FOR EACH ROW EXECUTE FUNCTION morningstar.jobs_state_guard();
CREATE CONSTRAINT TRIGGER jobs_transition_parity AFTER UPDATE OF state ON morningstar.jobs
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION morningstar.jobs_transition_parity();
CREATE TRIGGER job_attempts_guard BEFORE INSERT ON morningstar.job_attempts FOR EACH ROW EXECUTE FUNCTION morningstar.job_attempts_guard();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION morningstar.consume_approval_use(
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

    -- The claim is made by the transaction that locks the job (TD-M3-01). The job
    -- must be queued, and the epoch of the claim must be the next one, so a claim
    -- of a running or finished job, a replayed epoch and a skipped epoch all refuse.
    PERFORM 1 FROM morningstar.jobs j
     WHERE j.id = p_job_id AND j.state = 'queued' AND j.control_state = 'none' AND j.claim_epoch + 1 = p_claim_epoch
       FOR UPDATE;
    IF NOT FOUND THEN
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

DROP FUNCTION morningstar.check_approval_launch(uuid, uuid, bigint, bytea, uuid, bytea, uuid);

-- +goose StatementBegin
CREATE FUNCTION morningstar.check_approval_launch(
    p_approval_id uuid,
    p_job_id uuid,
    p_generation bigint,
    p_operation_hash bytea,
    p_scope_version_id uuid,
    p_policy_snapshot_hash bytea,
    p_validation_id uuid,
    p_claim_epoch integer
) RETURNS text
LANGUAGE plpgsql SECURITY DEFINER
SET search_path = pg_catalog, morningstar
AS $$
DECLARE
    v_reason text;
BEGIN
    -- E3: expiry, revocation, bindings and the operation hash are checked again
    -- immediately before the launch. This check consumes nothing (S1.6 item 1).
    IF p_approval_id IS NULL OR p_job_id IS NULL OR p_generation IS NULL OR p_claim_epoch IS NULL THEN
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
    -- The launch belongs to the claim that is running (TD-M3-01): the job is
    -- running under the epoch that the caller holds, and no stop is requested.
    IF NOT EXISTS (SELECT 1 FROM morningstar.jobs j WHERE j.id = p_job_id AND j.state = 'running' AND j.claim_epoch = p_claim_epoch AND j.control_state = 'none') THEN
        RETURN 'invalid_input';
    END IF;
    RETURN NULL;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION morningstar.jobs_state_guard() FROM PUBLIC;
REVOKE ALL ON FUNCTION morningstar.jobs_transition_parity() FROM PUBLIC;
REVOKE ALL ON FUNCTION morningstar.job_attempts_guard() FROM PUBLIC;
REVOKE ALL ON FUNCTION morningstar.consume_approval_use(uuid, uuid, uuid, integer, bigint, bytea, uuid, bytea, uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION morningstar.check_approval_launch(uuid, uuid, bigint, bytea, uuid, bytea, uuid, integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION morningstar.check_approval_launch(uuid, uuid, bigint, bytea, uuid, bytea, uuid, integer) TO ms_worker;
