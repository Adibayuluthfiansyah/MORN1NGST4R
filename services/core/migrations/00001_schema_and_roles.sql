-- +goose Up
-- Slice 00001 schema_and_roles (milestone M3).
--
-- The login roles and the database are created by scripts/db/pg-cluster.sh
-- bootstrap, because creating a role needs a privilege that ms_migrator must
-- not hold. This migration fails closed when a role is missing. It creates the
-- schema (DB-01) and the two helper functions of the append-only rules (DB-17).

-- +goose StatementBegin
DO $$
DECLARE
    r text;
BEGIN
    FOREACH r IN ARRAY ARRAY['ms_migrator', 'ms_api', 'ms_worker', 'ms_retention'] LOOP
        IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = r) THEN
            RAISE EXCEPTION 'role % does not exist. Run scripts/db/pg-cluster.sh bootstrap first', r;
        END IF;
    END LOOP;
    IF current_user <> 'ms_migrator' THEN
        RAISE EXCEPTION 'migrations run as ms_migrator, not as %', current_user;
    END IF;
END
$$;
-- +goose StatementEnd

CREATE SCHEMA morningstar AUTHORIZATION ms_migrator;

-- Nothing in the schema is visible to a role until a grant says so (migration 00015).
REVOKE ALL ON SCHEMA morningstar FROM PUBLIC;

-- +goose StatementBegin
CREATE FUNCTION morningstar.forbid_update_delete() RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
BEGIN
    RAISE EXCEPTION 'table %.% is append-only or immutable: % is refused', TG_TABLE_SCHEMA, TG_TABLE_NAME, TG_OP
        USING ERRCODE = 'restrict_violation';
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION morningstar.forbid_truncate() RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
BEGIN
    RAISE EXCEPTION 'table %.% is append-only or immutable: TRUNCATE is refused', TG_TABLE_SCHEMA, TG_TABLE_NAME
        USING ERRCODE = 'restrict_violation';
END
$$;
-- +goose StatementEnd
