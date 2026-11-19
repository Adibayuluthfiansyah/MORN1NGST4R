-- Queries of the job store (internal/adapters/outbound/persistence/jobs.go).
-- Every write of a running job is conditioned on the claim epoch and the lease
-- owner, so a stale worker changes nothing (P-36).

-- name: LockNextQueuedJob :one
SELECT sqlc.embed(j) FROM morningstar.jobs j
WHERE j.state = 'queued' AND j.control_state = 'none' AND j.available_at <= sqlc.arg(now)::timestamptz
ORDER BY j.priority DESC, j.available_at, j.id
FOR UPDATE SKIP LOCKED LIMIT 1;

-- name: LockJob :one
SELECT sqlc.embed(j) FROM morningstar.jobs j WHERE j.id = $1::uuid FOR UPDATE;

-- name: ListExpiredLeases :many
SELECT sqlc.embed(j) FROM morningstar.jobs j
WHERE j.state = 'running' AND j.lease_expires_at < sqlc.arg(now)::timestamptz
ORDER BY j.lease_expires_at, j.id
FOR UPDATE SKIP LOCKED LIMIT sqlc.arg(max_rows)::integer;

-- name: ListStopRequestedWaiting :many
SELECT sqlc.embed(j) FROM morningstar.jobs j
WHERE j.state IN ('queued', 'awaiting_approval') AND j.control_state = 'stop_requested'
ORDER BY j.control_requested_at NULLS FIRST, j.id
FOR UPDATE SKIP LOCKED LIMIT sqlc.arg(max_rows)::integer;

-- name: CountRunningUnderApproval :one
SELECT count(*)::integer AS n FROM morningstar.jobs WHERE approval_id = $1::uuid AND state = 'running';

-- name: ClaimJobRow :execrows
UPDATE morningstar.jobs
SET state = 'running', claim_epoch = sqlc.arg(epoch)::integer, lease_owner = sqlc.arg(worker)::text,
    lease_expires_at = sqlc.arg(lease_until)::timestamptz, heartbeat_at = now(),
    approval_use_id = coalesce(sqlc.narg(use_id)::uuid, approval_use_id), updated_at = now(), version = version + 1
WHERE id = sqlc.arg(job_id)::uuid AND state = 'queued' AND control_state = 'none' AND claim_epoch = sqlc.arg(epoch)::integer - 1
  AND EXISTS (SELECT 1 FROM morningstar.recovery_state WHERE singleton AND state = 'normal' AND generation = sqlc.arg(generation)::bigint);

-- name: MoveJobRow :one
UPDATE morningstar.jobs
SET state = sqlc.arg(to_state)::text,
    control_state = coalesce(sqlc.narg(control)::text, control_state),
    control_reason = CASE WHEN sqlc.narg(control)::text IS NULL THEN control_reason
                          WHEN sqlc.narg(control)::text = 'none' THEN NULL
                          ELSE coalesce(control_reason, sqlc.narg(reason)::text) END,
    failure_class = coalesce(sqlc.narg(failure_class)::text, failure_class),
    finished_at = CASE WHEN sqlc.arg(to_state)::text IN ('succeeded', 'failed', 'cancelled') THEN now() ELSE finished_at END,
    lease_owner = CASE WHEN sqlc.arg(to_state)::text = 'running' THEN lease_owner END,
    lease_expires_at = CASE WHEN sqlc.arg(to_state)::text = 'running' THEN lease_expires_at END,
    available_at = coalesce(sqlc.narg(available_at)::timestamptz, available_at),
    updated_at = now(), version = version + 1
WHERE id = sqlc.arg(job_id)::uuid AND state = sqlc.arg(from_state)::text
  AND (sqlc.arg(epoch)::integer = 0 OR claim_epoch = sqlc.arg(epoch)::integer)
  AND (sqlc.arg(worker)::text = '' OR lease_owner = sqlc.arg(worker)::text)
RETURNING claim_epoch, control_state;

-- name: CancelJobRow :one
UPDATE morningstar.jobs
SET state = 'cancelled', control_state = sqlc.arg(control)::text, control_reason = coalesce(sqlc.narg(reason)::text, control_reason),
    finished_at = now(), updated_at = now(), version = version + 1
WHERE id = sqlc.arg(job_id)::uuid AND state = sqlc.arg(from_state)::text
RETURNING claim_epoch, control_state;

-- name: InsertJobTransition :exec
INSERT INTO morningstar.job_transitions (id, job_id, from_state, to_state, control_state, claim_epoch, actor_kind, actor_user_id, reason)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(job_id)::uuid, sqlc.narg(from_state)::text, sqlc.arg(to_state)::text, sqlc.arg(control_state)::text,
        sqlc.arg(claim_epoch)::integer, sqlc.arg(actor_kind)::text, sqlc.narg(actor_user_id)::uuid, sqlc.narg(reason)::text);

-- name: HeartbeatJobRow :one
UPDATE morningstar.jobs SET heartbeat_at = now(), lease_expires_at = sqlc.arg(until)::timestamptz
WHERE id = sqlc.arg(job_id)::uuid AND state = 'running' AND claim_epoch = sqlc.arg(epoch)::integer AND lease_owner = sqlc.arg(worker)::text
RETURNING control_state;

-- name: JobControlRow :one
SELECT control_state FROM morningstar.jobs
WHERE id = sqlc.arg(job_id)::uuid AND state = 'running' AND claim_epoch = sqlc.arg(epoch)::integer AND lease_owner = sqlc.arg(worker)::text;

-- name: RequestJobStop :execrows
UPDATE morningstar.jobs
SET control_state = 'stop_requested', control_reason = sqlc.arg(reason)::text, control_requested_at = now(), updated_at = now(), version = version + 1
WHERE id = sqlc.arg(job_id)::uuid AND state IN ('queued', 'awaiting_approval', 'running')
  AND (control_state = 'none' OR (control_state = 'stop_failed' AND control_reason IS NULL));

-- name: RaiseAttemptCounter :one
UPDATE morningstar.jobs SET execution_attempts = execution_attempts + 1, updated_at = now(), version = version + 1
WHERE id = sqlc.arg(job_id)::uuid AND state = 'running' AND claim_epoch = sqlc.arg(epoch)::integer AND lease_owner = sqlc.arg(worker)::text
  AND control_state = 'none' AND execution_attempts < max_execution_attempts
RETURNING execution_attempts;

-- name: InsertJobAttempt :exec
INSERT INTO morningstar.job_attempts (id, job_id, attempt_no, claim_epoch, approval_use_id, decision_id, worker_instance)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(job_id)::uuid, sqlc.arg(attempt_no)::smallint, sqlc.arg(claim_epoch)::integer,
        sqlc.narg(use_id)::uuid, sqlc.arg(decision_id)::uuid, sqlc.arg(worker)::text);

-- name: UpdateLiveAttempt :execrows
UPDATE morningstar.job_attempts
SET state = coalesce(sqlc.narg(state)::text, state), process_group_ref = coalesce(sqlc.narg(process_ref)::text, process_group_ref),
    error_class = coalesce(sqlc.narg(error_class)::text, error_class), exit_code = coalesce(sqlc.narg(exit_code)::integer, exit_code),
    ended_at = CASE WHEN sqlc.arg(ended)::boolean THEN now() ELSE ended_at END,
    termination_confirmed_at = CASE WHEN sqlc.arg(confirmed)::boolean THEN now() ELSE termination_confirmed_at END
WHERE id = sqlc.arg(attempt_id)::uuid AND state IN ('launching', 'running');

-- name: ConfirmAttemptTermination :execrows
UPDATE morningstar.job_attempts SET termination_confirmed_at = now()
WHERE id = sqlc.arg(attempt_id)::uuid AND termination_confirmed_at IS NULL;

-- name: ListJobAttempts :many
SELECT id::text AS id, job_id::text AS job_id, attempt_no::integer AS attempt_no, claim_epoch, state, worker_instance,
       coalesce(process_group_ref, '')::text AS process_ref, (termination_confirmed_at IS NOT NULL)::boolean AS confirmed, started_at
FROM morningstar.job_attempts WHERE job_id = $1::uuid ORDER BY attempt_no;

-- name: SetJobControl :execrows
UPDATE morningstar.jobs SET control_state = sqlc.arg(control)::text, updated_at = now(), version = version + 1
WHERE id = sqlc.arg(job_id)::uuid AND state = 'running' AND claim_epoch = sqlc.arg(epoch)::integer AND lease_owner = sqlc.arg(worker)::text;

-- name: InsertPolicyDecision :exec
INSERT INTO morningstar.policy_decisions (id, program_id, assessment_id, point, operation_id, job_id, outcome, reasons, scope_version_id, validation_id, approval_id, policy_snapshot_hash)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(program_id)::uuid, sqlc.arg(assessment_id)::uuid, sqlc.arg(point)::text, sqlc.narg(operation_id)::uuid, sqlc.narg(job_id)::uuid,
        sqlc.arg(outcome)::text, sqlc.arg(reasons)::jsonb, sqlc.arg(scope_version_id)::uuid, sqlc.narg(validation_id)::uuid, sqlc.narg(approval_id)::uuid,
        sqlc.arg(policy_snapshot_hash)::bytea);

