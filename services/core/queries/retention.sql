-- Queries of the retention adapter (internal/adapters/outbound/persistence/retention.go).
-- Both functions refuse a cutoff that is less than 30 days in the past, and only
-- the retention role may execute them (migration 00019).

-- name: PurgeSessions :one
SELECT grants_deleted::bigint AS grants_deleted, sessions_deleted::bigint AS sessions_deleted
FROM morningstar.retention_purge_sessions(sqlc.arg(cutoff)::timestamptz);

-- name: PurgeLoginAttempts :one
SELECT morningstar.retention_purge_login_attempts(sqlc.arg(cutoff)::timestamptz)::bigint AS deleted;
