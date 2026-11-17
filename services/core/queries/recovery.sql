-- Queries of the recovery adapter (internal/adapters/outbound/persistence/recovery.go).
-- Phase 1 of the reconciliation is a sequence of data-modifying CTEs that sqlc
-- cannot type, so it stays in Go. These are the reads and the small writes.

-- name: GetRecoveryState :one
SELECT instance_id::text AS instance_id, generation, (state = 'reconciling')::boolean AS reconciling, coalesce(last_reason, '')::text AS last_reason
FROM morningstar.recovery_state
WHERE singleton;

-- name: HasUsers :one
SELECT EXISTS (SELECT 1 FROM morningstar.users)::boolean AS has_users;

-- name: MaxAuditSeq :one
SELECT coalesce(max(seq), 0)::bigint AS max_seq FROM morningstar.audit_events;
