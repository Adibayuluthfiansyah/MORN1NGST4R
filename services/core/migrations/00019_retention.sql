-- +goose Up
-- Slice 00019 retention (milestone M7): the audited purge of security-operational rows (RC-B, TD-M3-03 and TD-M4-02, by the owner decision of 2026-11-18) and
-- the right of the retention role to write the audit event of its purge. Audit rows are not purged here: no number has been approved for them, so the
-- always-refuse guard of audit_events stays (TD-M3-03, audit part).

-- login_attempts: one path may delete, the purge function. Everything else is still refused, whoever asks.
-- +goose StatementBegin
CREATE FUNCTION morningstar.login_attempts_guard() RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
BEGIN
    IF TG_OP = 'DELETE' AND pg_catalog.current_setting('morningstar.retention_path', true) = 'login_attempts' THEN
        RETURN OLD;
    END IF;
    RAISE EXCEPTION 'table %.% is append-only: % is refused', TG_TABLE_SCHEMA, TG_TABLE_NAME, TG_OP USING ERRCODE = 'restrict_violation';
END
$$;
-- +goose StatementEnd

DROP TRIGGER login_attempts_no_update_delete ON morningstar.login_attempts;
CREATE TRIGGER login_attempts_guard BEFORE UPDATE OR DELETE ON morningstar.login_attempts FOR EACH ROW EXECUTE FUNCTION morningstar.login_attempts_guard();

-- Sessions and re-authentication grants that ended more than the floor ago, and that nothing refers to. A grant that an approval, a policy setting, a
-- scope version, a lab registration or a refused exclusion names is part of the history of that authority and stays, and so does its session.
-- +goose StatementBegin
CREATE FUNCTION morningstar.retention_purge_sessions(p_cutoff timestamptz, OUT grants_deleted bigint, OUT sessions_deleted bigint)
LANGUAGE plpgsql SECURITY DEFINER
SET search_path = pg_catalog, morningstar
AS $$
BEGIN
    IF p_cutoff IS NULL OR p_cutoff > pg_catalog.now() - interval '30 days' THEN
        RAISE EXCEPTION 'a purge of sessions needs a cutoff at least 30 days in the past' USING ERRCODE = 'restrict_violation';
    END IF;
    WITH gone AS (
        DELETE FROM morningstar.reauth_grants g
         WHERE g.expires_at < p_cutoff
           AND NOT EXISTS (SELECT 1 FROM morningstar.approvals x WHERE x.reauth_grant_id = g.id)
           AND NOT EXISTS (SELECT 1 FROM morningstar.policy_settings x WHERE x.reauth_grant_id = g.id)
           AND NOT EXISTS (SELECT 1 FROM morningstar.scope_versions x WHERE x.reauth_grant_id = g.id)
           AND NOT EXISTS (SELECT 1 FROM morningstar.lab_registrations x WHERE x.reauth_grant_id = g.id)
           AND NOT EXISTS (SELECT 1 FROM morningstar.refused_exclusions x WHERE x.reauth_grant_id = g.id)
        RETURNING 1)
    SELECT count(*) INTO grants_deleted FROM gone;
    WITH gone AS (
        DELETE FROM morningstar.sessions s
         WHERE least(coalesce(s.revoked_at, 'infinity'::timestamptz), s.idle_expires_at, s.absolute_expires_at) < p_cutoff
           AND NOT EXISTS (SELECT 1 FROM morningstar.reauth_grants g WHERE g.session_id = s.id)
        RETURNING 1)
    SELECT count(*) INTO sessions_deleted FROM gone;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION morningstar.retention_purge_login_attempts(p_cutoff timestamptz) RETURNS bigint
LANGUAGE plpgsql SECURITY DEFINER
SET search_path = pg_catalog, morningstar
AS $$
DECLARE
    n bigint;
BEGIN
    IF p_cutoff IS NULL OR p_cutoff > pg_catalog.now() - interval '30 days' THEN
        RAISE EXCEPTION 'a purge of login attempts needs a cutoff at least 30 days in the past' USING ERRCODE = 'restrict_violation';
    END IF;
    PERFORM pg_catalog.set_config('morningstar.retention_path', 'login_attempts', true);
    WITH gone AS (DELETE FROM morningstar.login_attempts WHERE attempted_at < p_cutoff RETURNING 1)
    SELECT count(*) INTO n FROM gone;
    PERFORM pg_catalog.set_config('morningstar.retention_path', '', true);
    RETURN n;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION morningstar.login_attempts_guard() FROM PUBLIC;
REVOKE ALL ON FUNCTION morningstar.retention_purge_sessions(timestamptz) FROM PUBLIC;
REVOKE ALL ON FUNCTION morningstar.retention_purge_login_attempts(timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION morningstar.retention_purge_sessions(timestamptz) TO ms_retention;
GRANT EXECUTE ON FUNCTION morningstar.retention_purge_login_attempts(timestamptz) TO ms_retention;

-- The retention role writes the event of its own purge, in the transaction of the purge (SEC 13).
GRANT INSERT ON morningstar.audit_events TO ms_retention;

-- The worker runs the same startup recovery check as the server (CC-15). The check asks only whether an account exists, so the worker may read the id column of users and no other column.
GRANT SELECT (id) ON morningstar.users TO ms_worker;
