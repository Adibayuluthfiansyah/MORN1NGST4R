-- name: InsertAuditEvent :exec
INSERT INTO morningstar.audit_events (
    id, actor_kind, actor_user_id, actor_ref, action, outcome, request_id,
    program_id, assessment_id, job_id, entity_type, entity_id, scope_version_id, decision_id, approval_id, detail)
VALUES (
    $1::uuid, $2, sqlc.narg(actor_user_id)::uuid, sqlc.narg(actor_ref)::text, $3, $4, sqlc.narg(request_id)::uuid,
    sqlc.narg(program_id)::uuid, sqlc.narg(assessment_id)::uuid, sqlc.narg(job_id)::uuid, sqlc.narg(entity_type)::text,
    sqlc.narg(entity_id)::uuid, sqlc.narg(scope_version_id)::uuid, sqlc.narg(decision_id)::uuid, sqlc.narg(approval_id)::uuid, $5::jsonb);

-- name: RaiseAuditRetryAlert :execrows
INSERT INTO morningstar.system_alerts (id, kind, detail)
VALUES ($1::uuid, 'audit_retry_exhausted', $2::jsonb)
ON CONFLICT (kind) WHERE kind = 'audit_retry_exhausted' AND cleared_at IS NULL DO NOTHING;

-- name: AuditAlertIsOpen :one
SELECT EXISTS (SELECT 1 FROM morningstar.system_alerts WHERE kind = 'audit_retry_exhausted' AND cleared_at IS NULL) AS open;
