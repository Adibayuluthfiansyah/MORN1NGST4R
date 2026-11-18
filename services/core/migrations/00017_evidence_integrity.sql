-- +goose Up
-- Indexes, triggers and grants of the evidence slice (milestone M6). Slices 00013 to 00015 were applied before 00009 existed and are never edited, so the
-- later slice adds its own. D-77, P-37, DB-11, DB-17.

CREATE INDEX evidence_assessment_idx ON morningstar.evidence_items (assessment_id, captured_at DESC); -- List evidence
CREATE INDEX evidence_job_idx ON morningstar.evidence_items (job_id) WHERE job_id IS NOT NULL; -- Evidence of a job
CREATE INDEX evidence_sha_idx ON morningstar.evidence_items (sha256); -- Find duplicates and orphan checks
CREATE INDEX evidence_checks_idx ON morningstar.evidence_integrity_checks (evidence_id, checked_at DESC); -- Latest check and history
CREATE INDEX evidence_status_state_idx ON morningstar.evidence_status (integrity_state) WHERE integrity_state IN ('compromised', 'missing'); -- Find affected evidence

-- +goose StatementBegin
CREATE FUNCTION morningstar.evidence_status_guard() RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
BEGIN
    -- A compromised item stays compromised: no role clears it, and the time of the
    -- first mark is kept (P-37). A purge is final.
    IF OLD.integrity_state = 'compromised' AND (NEW.integrity_state <> 'compromised' OR NEW.compromised_at IS DISTINCT FROM OLD.compromised_at) THEN
        RAISE EXCEPTION 'an item marked compromised stays compromised' USING ERRCODE = 'restrict_violation';
    END IF;
    IF OLD.retention_state = 'purged' AND (NEW.retention_state <> 'purged' OR NEW.purged_at IS DISTINCT FROM OLD.purged_at OR NEW.purged_by IS DISTINCT FROM OLD.purged_by) THEN
        RAISE EXCEPTION 'a purge is final' USING ERRCODE = 'restrict_violation';
    END IF;
    IF OLD.retention_state = 'purge_pending' AND NEW.retention_state = 'active' THEN
        RAISE EXCEPTION 'a pending purge does not go back to active' USING ERRCODE = 'restrict_violation';
    END IF;
    RETURN NEW;
END
$$;
-- +goose StatementEnd

CREATE TRIGGER evidence_items_no_update_delete BEFORE UPDATE OR DELETE ON morningstar.evidence_items FOR EACH ROW EXECUTE FUNCTION morningstar.forbid_update_delete();
CREATE TRIGGER evidence_items_no_truncate BEFORE TRUNCATE ON morningstar.evidence_items FOR EACH STATEMENT EXECUTE FUNCTION morningstar.forbid_truncate();
CREATE TRIGGER evidence_integrity_checks_no_update_delete BEFORE UPDATE OR DELETE ON morningstar.evidence_integrity_checks FOR EACH ROW EXECUTE FUNCTION morningstar.forbid_update_delete();
CREATE TRIGGER evidence_integrity_checks_no_truncate BEFORE TRUNCATE ON morningstar.evidence_integrity_checks FOR EACH STATEMENT EXECUTE FUNCTION morningstar.forbid_truncate();
CREATE TRIGGER evidence_status_update_columns BEFORE UPDATE ON morningstar.evidence_status FOR EACH ROW EXECUTE FUNCTION morningstar.forbid_update_except('integrity_state', 'last_check_id', 'retention_state', 'purged_at', 'purged_by', 'compromised_at', 'updated_at');
CREATE TRIGGER evidence_status_guard BEFORE UPDATE ON morningstar.evidence_status FOR EACH ROW EXECUTE FUNCTION morningstar.evidence_status_guard();
CREATE TRIGGER evidence_status_no_delete BEFORE DELETE ON morningstar.evidence_status FOR EACH ROW EXECUTE FUNCTION morningstar.forbid_update_delete();
CREATE TRIGGER evidence_status_no_truncate BEFORE TRUNCATE ON morningstar.evidence_status FOR EACH STATEMENT EXECUTE FUNCTION morningstar.forbid_truncate();

-- ms_api and ms_worker record and read evidence and mark its integrity. Only ms_retention changes the retention columns.
GRANT SELECT, INSERT ON morningstar.evidence_items TO ms_api, ms_worker;
GRANT SELECT, INSERT ON morningstar.evidence_integrity_checks TO ms_api, ms_worker;
GRANT SELECT, INSERT ON morningstar.evidence_status TO ms_api, ms_worker;
GRANT UPDATE (integrity_state, last_check_id, compromised_at, updated_at) ON morningstar.evidence_status TO ms_api, ms_worker;
GRANT SELECT ON morningstar.evidence_items, morningstar.evidence_integrity_checks, morningstar.evidence_status TO ms_retention;
GRANT UPDATE (retention_state, purged_at, purged_by, updated_at) ON morningstar.evidence_status TO ms_retention;

-- The guard is a trigger function: it needs no privilege on it, and 00015 revoked the default of every function that existed then.
REVOKE ALL ON FUNCTION morningstar.evidence_status_guard() FROM PUBLIC;
