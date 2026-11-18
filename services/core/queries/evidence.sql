-- Queries of the evidence repository
-- (internal/adapters/outbound/persistence/evidence.go). Every read that serves a
-- user joins the program and filters on its owner, so a guessed id returns no
-- row (BOLA). The ownership join stays here until the program module offers a
-- contract for it (TD-M6-03).

-- name: EvidenceAssessment :one
SELECT a.program_id::text AS program_id, a.raw_evidence_capture AS raw_capture, (p.status = 'archived')::boolean AS program_archived
FROM morningstar.assessments a JOIN morningstar.programs p ON p.id = a.program_id
WHERE a.id = $1::uuid;

-- name: EvidenceAssessmentOwned :one
SELECT EXISTS (
    SELECT 1 FROM morningstar.assessments a JOIN morningstar.programs p ON p.id = a.program_id
    WHERE a.id = $1::uuid AND p.owner_user_id = $2::uuid
)::boolean AS owned;

-- name: InsertEvidenceItem :exec
INSERT INTO morningstar.evidence_items (
    id, program_id, assessment_id, job_id, attempt_id, target_id, kind, store_backend, storage_key, sha256, size_bytes,
    content_type, redaction_state, sensitivity, captured_at, origin, tool_key, tool_version, tool_binary_sha256,
    operation_hash, decision_id, redacted_args)
VALUES (
    $1::uuid, $2::uuid, $3::uuid, sqlc.narg(job_id)::uuid, sqlc.narg(attempt_id)::uuid, sqlc.narg(target_id)::uuid, $4, $5, $6,
    $7, $8, $9, $10, $11, $12, $13, sqlc.narg(tool_key)::text, sqlc.narg(tool_version)::text,
    sqlc.narg(tool_binary_sha256)::bytea, sqlc.narg(operation_hash)::bytea, sqlc.narg(decision_id)::uuid, sqlc.narg(redacted_args)::jsonb);

-- name: InsertEvidenceStatus :exec
INSERT INTO morningstar.evidence_status (evidence_id) VALUES ($1::uuid);

-- name: GetEvidence :one
SELECT sqlc.embed(e), sqlc.embed(s), (p.status = 'archived')::boolean AS program_archived, c.checked_at AS last_checked_at
FROM morningstar.evidence_items e
JOIN morningstar.evidence_status s ON s.evidence_id = e.id
JOIN morningstar.programs p ON p.id = e.program_id
LEFT JOIN morningstar.evidence_integrity_checks c ON c.id = s.last_check_id
WHERE e.id = $1::uuid AND p.owner_user_id = $2::uuid;

-- name: ListEvidenceAsc :many
SELECT sqlc.embed(e), sqlc.embed(s), (p.status = 'archived')::boolean AS program_archived, c.checked_at AS last_checked_at
FROM morningstar.evidence_items e
JOIN morningstar.evidence_status s ON s.evidence_id = e.id
JOIN morningstar.programs p ON p.id = e.program_id
LEFT JOIN morningstar.evidence_integrity_checks c ON c.id = s.last_check_id
WHERE e.assessment_id = $1::uuid AND p.owner_user_id = $2::uuid
  AND (sqlc.narg(kind)::text IS NULL OR e.kind = sqlc.narg(kind)::text)
  AND (sqlc.narg(integrity_state)::text IS NULL OR s.integrity_state = sqlc.narg(integrity_state)::text)
  AND (sqlc.narg(job_id)::uuid IS NULL OR e.job_id = sqlc.narg(job_id)::uuid)
  AND (sqlc.narg(after_at)::timestamptz IS NULL OR (e.captured_at, e.id) > (sqlc.narg(after_at)::timestamptz, sqlc.narg(after_id)::uuid))
ORDER BY e.captured_at ASC, e.id ASC
LIMIT sqlc.arg(max_rows)::integer;

-- name: ListEvidenceDesc :many
SELECT sqlc.embed(e), sqlc.embed(s), (p.status = 'archived')::boolean AS program_archived, c.checked_at AS last_checked_at
FROM morningstar.evidence_items e
JOIN morningstar.evidence_status s ON s.evidence_id = e.id
JOIN morningstar.programs p ON p.id = e.program_id
LEFT JOIN morningstar.evidence_integrity_checks c ON c.id = s.last_check_id
WHERE e.assessment_id = $1::uuid AND p.owner_user_id = $2::uuid
  AND (sqlc.narg(kind)::text IS NULL OR e.kind = sqlc.narg(kind)::text)
  AND (sqlc.narg(integrity_state)::text IS NULL OR s.integrity_state = sqlc.narg(integrity_state)::text)
  AND (sqlc.narg(job_id)::uuid IS NULL OR e.job_id = sqlc.narg(job_id)::uuid)
  AND (sqlc.narg(before_at)::timestamptz IS NULL OR (e.captured_at, e.id) < (sqlc.narg(before_at)::timestamptz, sqlc.narg(before_id)::uuid))
ORDER BY e.captured_at DESC, e.id DESC
LIMIT sqlc.arg(max_rows)::integer;

-- name: LockEvidenceStatus :one
SELECT integrity_state, retention_state FROM morningstar.evidence_status WHERE evidence_id = $1::uuid FOR UPDATE;

-- name: InsertEvidenceCheck :one
INSERT INTO morningstar.evidence_integrity_checks (id, evidence_id, trigger, result, observed_sha256, observed_size, actor_kind, actor_user_id, request_id)
VALUES ($1::uuid, $2::uuid, $3, $4, sqlc.narg(observed_sha256)::bytea, sqlc.narg(observed_size)::bigint, $5, sqlc.narg(actor_user_id)::uuid, sqlc.narg(request_id)::uuid)
RETURNING checked_at;

-- name: UpdateEvidenceIntegrity :one
UPDATE morningstar.evidence_status
SET integrity_state = sqlc.arg(state)::text, last_check_id = sqlc.arg(check_id)::uuid, updated_at = now(),
    compromised_at = CASE WHEN sqlc.arg(state)::text = 'compromised' THEN coalesce(compromised_at, now()) ELSE compromised_at END
WHERE evidence_id = sqlc.arg(evidence_id)::uuid
RETURNING integrity_state, retention_state, compromised_at, purged_at;

-- name: ListEvidenceChecksAsc :many
SELECT c.id::text AS id, c.evidence_id::text AS evidence_id, c.checked_at, c.trigger, c.result, c.observed_sha256, c.observed_size,
       c.actor_kind, coalesce(c.actor_user_id::text, '')::text AS actor_user_id, coalesce(c.request_id::text, '')::text AS request_id
FROM morningstar.evidence_integrity_checks c
JOIN morningstar.evidence_items e ON e.id = c.evidence_id
JOIN morningstar.programs p ON p.id = e.program_id
WHERE c.evidence_id = $1::uuid AND p.owner_user_id = $2::uuid
  AND (sqlc.narg(after_at)::timestamptz IS NULL OR (c.checked_at, c.id) > (sqlc.narg(after_at)::timestamptz, sqlc.narg(after_id)::uuid))
ORDER BY c.checked_at ASC, c.id ASC
LIMIT sqlc.arg(max_rows)::integer;

-- name: ListEvidenceChecksDesc :many
SELECT c.id::text AS id, c.evidence_id::text AS evidence_id, c.checked_at, c.trigger, c.result, c.observed_sha256, c.observed_size,
       c.actor_kind, coalesce(c.actor_user_id::text, '')::text AS actor_user_id, coalesce(c.request_id::text, '')::text AS request_id
FROM morningstar.evidence_integrity_checks c
JOIN morningstar.evidence_items e ON e.id = c.evidence_id
JOIN morningstar.programs p ON p.id = e.program_id
WHERE c.evidence_id = $1::uuid AND p.owner_user_id = $2::uuid
  AND (sqlc.narg(before_at)::timestamptz IS NULL OR (c.checked_at, c.id) < (sqlc.narg(before_at)::timestamptz, sqlc.narg(before_id)::uuid))
ORDER BY c.checked_at DESC, c.id DESC
LIMIT sqlc.arg(max_rows)::integer;

-- name: EvidenceOwned :one
SELECT EXISTS (
    SELECT 1 FROM morningstar.evidence_items e JOIN morningstar.programs p ON p.id = e.program_id
    WHERE e.id = $1::uuid AND p.owner_user_id = $2::uuid
)::boolean AS owned;
