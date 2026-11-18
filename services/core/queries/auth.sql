-- name: CountUsers :one
SELECT count(*)::bigint AS n FROM morningstar.users;

-- name: SingleUser :one
SELECT id::text AS id, username, coalesce(display_name, '')::text AS display_name, (status = 'disabled')::boolean AS disabled
FROM morningstar.users ORDER BY created_at, id LIMIT 1;

-- name: UserByUsername :one
SELECT id::text AS id, username, coalesce(display_name, '')::text AS display_name, (status = 'disabled')::boolean AS disabled
FROM morningstar.users WHERE username = $1;

-- name: UserByID :one
SELECT id::text AS id, username, coalesce(display_name, '')::text AS display_name, (status = 'disabled')::boolean AS disabled
FROM morningstar.users WHERE id = $1::uuid;

-- name: InsertUser :exec
INSERT INTO morningstar.users (id, username) VALUES ($1::uuid, $2);

-- name: TouchPasswordChanged :exec
UPDATE morningstar.users SET password_changed_at = now(), updated_at = now(), version = version + 1 WHERE id = $1::uuid;

-- name: CurrentPasswordHash :one
SELECT phc_hash FROM morningstar.user_credentials WHERE user_id = $1::uuid AND is_current;

-- name: SupersedeCredentials :exec
UPDATE morningstar.user_credentials SET is_current = false, superseded_at = now() WHERE user_id = $1::uuid AND is_current;

-- name: InsertCredential :exec
INSERT INTO morningstar.user_credentials (id, user_id, phc_hash) VALUES ($1::uuid, $2::uuid, $3);

-- name: LockLiveBootstrapSecret :one
SELECT id::text AS id, secret_hash FROM morningstar.bootstrap_secrets
WHERE consumed_at IS NULL AND superseded_at IS NULL ORDER BY created_at DESC, id LIMIT 1 FOR UPDATE;

-- name: SupersedeBootstrapSecrets :execrows
UPDATE morningstar.bootstrap_secrets SET superseded_at = now() WHERE consumed_at IS NULL AND superseded_at IS NULL;

-- name: InsertBootstrapSecret :exec
INSERT INTO morningstar.bootstrap_secrets (id, secret_hash) VALUES ($1::uuid, $2);

-- name: ConsumeBootstrapSecret :execrows
UPDATE morningstar.bootstrap_secrets SET consumed_at = now(), consumed_user_id = $2::uuid
WHERE id = $1::uuid AND consumed_at IS NULL AND superseded_at IS NULL;

-- name: LockLiveRecoveryCode :one
SELECT id::text AS id, code_hash FROM morningstar.recovery_codes
WHERE user_id = $1::uuid AND used_at IS NULL AND revoked_at IS NULL ORDER BY created_at DESC, id LIMIT 1 FOR UPDATE;

-- name: InsertRecoveryCode :exec
INSERT INTO morningstar.recovery_codes (id, user_id, code_hash) VALUES ($1::uuid, $2::uuid, $3);

-- name: MarkRecoveryCodeUsed :execrows
UPDATE morningstar.recovery_codes SET used_at = now() WHERE id = $1::uuid AND used_at IS NULL AND revoked_at IS NULL;

-- name: RevokeLiveRecoveryCodes :exec
UPDATE morningstar.recovery_codes SET revoked_at = now() WHERE user_id = $1::uuid AND used_at IS NULL AND revoked_at IS NULL;

-- name: InsertSession :exec
INSERT INTO morningstar.sessions (id, user_id, token_hash, csrf_token_hash, idle_expires_at, absolute_expires_at, user_agent)
VALUES ($1::uuid, $2::uuid, $3, $4, now() + make_interval(secs => sqlc.arg(idle_secs)::float8),
        now() + make_interval(secs => sqlc.arg(absolute_secs)::float8), sqlc.narg(user_agent)::text);

-- name: LookupSession :one
SELECT s.id::text AS id, s.user_id::text AS user_id, s.csrf_token_hash, s.created_at, s.last_seen_at, s.idle_expires_at,
       s.absolute_expires_at, (s.revoked_at IS NOT NULL)::boolean AS revoked, coalesce(s.user_agent, '')::text AS user_agent,
       (u.status = 'disabled')::boolean AS user_disabled, now()::timestamptz AS db_now
FROM morningstar.sessions s JOIN morningstar.users u ON u.id = s.user_id
WHERE s.token_hash = $1;

-- name: TouchSession :exec
UPDATE morningstar.sessions
SET last_seen_at = now(), idle_expires_at = least(now() + make_interval(secs => sqlc.arg(idle_secs)::float8), absolute_expires_at)
WHERE id = $1::uuid AND revoked_at IS NULL AND idle_expires_at > now() AND absolute_expires_at > now()
  AND last_seen_at <= now() - make_interval(secs => sqlc.arg(every_secs)::float8);

-- name: RevokeSession :execrows
UPDATE morningstar.sessions SET revoked_at = now(), revoked_reason = $3
WHERE id = $1::uuid AND user_id = $2::uuid AND revoked_at IS NULL;

-- name: RevokeSessionsExcept :execrows
UPDATE morningstar.sessions SET revoked_at = now(), revoked_reason = sqlc.arg(reason)::text
WHERE user_id = sqlc.arg(user_id)::uuid AND (sqlc.narg(except_id)::uuid IS NULL OR id <> sqlc.narg(except_id)::uuid) AND revoked_at IS NULL;

-- name: FindSession :one
SELECT s.id::text AS id, s.user_id::text AS user_id, s.csrf_token_hash, s.created_at, s.last_seen_at, s.idle_expires_at,
       s.absolute_expires_at, (s.revoked_at IS NOT NULL)::boolean AS revoked, coalesce(s.user_agent, '')::text AS user_agent,
       false::boolean AS user_disabled, now()::timestamptz AS db_now
FROM morningstar.sessions s WHERE s.id = $1::uuid AND s.user_id = $2::uuid;

-- name: ListLiveSessions :many
SELECT s.id::text AS id, s.user_id::text AS user_id, s.csrf_token_hash, s.created_at, s.last_seen_at, s.idle_expires_at,
       s.absolute_expires_at, (s.revoked_at IS NOT NULL)::boolean AS revoked, coalesce(s.user_agent, '')::text AS user_agent,
       false::boolean AS user_disabled, now()::timestamptz AS db_now
FROM morningstar.sessions s
WHERE s.user_id = $1::uuid AND s.revoked_at IS NULL AND s.idle_expires_at > now() AND s.absolute_expires_at > now()
ORDER BY s.created_at DESC, s.id LIMIT $2;

-- name: InsertReauthGrant :one
INSERT INTO morningstar.reauth_grants (id, user_id, session_id, purpose, subject_hash, expires_at)
VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, now() + make_interval(secs => sqlc.arg(ttl_secs)::float8))
RETURNING id::text AS id, purpose, expires_at;

-- name: ConsumeReauthGrant :execrows
UPDATE morningstar.reauth_grants g SET consumed_at = clock_timestamp()
FROM morningstar.sessions s
WHERE g.id = $1::uuid AND g.session_id = $2::uuid AND s.id = g.session_id AND g.user_id = s.user_id
  AND s.revoked_at IS NULL AND s.idle_expires_at > clock_timestamp() AND s.absolute_expires_at > clock_timestamp()
  AND g.purpose = $3 AND g.subject_hash = $4 AND g.consumed_at IS NULL AND g.expires_at > clock_timestamp();

-- name: LockLoginAttempts :exec
SELECT pg_advisory_xact_lock(hashtext('morningstar.login_attempts'));

-- name: CountEffectiveFailures :one
SELECT greatest(count(*) FILTER (WHERE NOT succeeded) - count(*) FILTER (WHERE succeeded), 0)::bigint AS n
FROM morningstar.login_attempts
WHERE attempted_at > now() - make_interval(secs => sqlc.arg(window_secs)::float8)
  AND kind = ANY(sqlc.arg(kinds)::text[])
  AND (source_key = sqlc.arg(source_key)::text OR (sqlc.narg(user_id)::uuid IS NOT NULL AND user_id = sqlc.narg(user_id)::uuid));

-- name: InsertLoginAttempt :exec
INSERT INTO morningstar.login_attempts (id, kind, user_id, source_key, succeeded)
VALUES ($1::uuid, $2, sqlc.narg(user_id)::uuid, $3, $4);

-- name: InsertAuditEvent :exec
INSERT INTO morningstar.audit_events (id, actor_kind, actor_user_id, actor_ref, action, outcome, request_id, detail)
VALUES ($1::uuid, $2, sqlc.narg(actor_user_id)::uuid, sqlc.narg(actor_ref)::text, $3, $4, sqlc.narg(request_id)::uuid, $5::jsonb);
