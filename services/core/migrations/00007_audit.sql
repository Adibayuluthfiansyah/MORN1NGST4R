-- +goose Up
-- Slice 00007 audit (milestone M3). Generated from docs/design/database/data-dictionary.md and constraints-and-indexes.md, then reviewed.
-- Foreign keys are created in the migration of the later of their two tables (DB-21). Indexes, triggers and grants are in 00013 to 00015.

CREATE TABLE morningstar.audit_events (
    id uuid NOT NULL,
    seq bigint GENERATED ALWAYS AS IDENTITY NOT NULL,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    actor_kind text NOT NULL,
    actor_user_id uuid,
    actor_ref text,
    action text NOT NULL,
    outcome text NOT NULL,
    request_id uuid,
    program_id uuid,
    assessment_id uuid,
    job_id uuid,
    entity_type text,
    entity_id uuid,
    scope_version_id uuid,
    decision_id uuid,
    approval_id uuid,
    detail jsonb NOT NULL DEFAULT '{}',
    CONSTRAINT audit_events_pk PRIMARY KEY (id),
    CONSTRAINT audit_events_uq_c1 UNIQUE (seq),
    CONSTRAINT audit_events_ck_c2 CHECK (actor_kind IN ('user', 'worker', 'system', 'ai')),
    CONSTRAINT audit_events_ck_c3 CHECK (outcome IN ('success', 'denied', 'failure')),
    CONSTRAINT audit_events_ck_c4 CHECK ((actor_kind = 'user') = (actor_user_id IS NOT NULL)),
    CONSTRAINT audit_events_ck_c5 CHECK (octet_length(detail::text) <= 8192)
);

CREATE TABLE morningstar.system_alerts (
    id uuid NOT NULL,
    kind text NOT NULL,
    raised_at timestamptz NOT NULL DEFAULT now(),
    cleared_at timestamptz,
    cleared_by uuid,
    detail jsonb NOT NULL DEFAULT '{}',
    CONSTRAINT system_alerts_pk PRIMARY KEY (id),
    CONSTRAINT system_alerts_ck_c1 CHECK (kind IN ('audit_retry_exhausted', 'evidence_integrity', 'sandbox_unavailable', 'credential_store_unavailable', 'restore_recovered')),
    CONSTRAINT system_alerts_ck_c2 CHECK ((cleared_at IS NULL) = (cleared_by IS NULL))
);

ALTER TABLE morningstar.audit_events ADD CONSTRAINT audit_events_fk_actor_user_id FOREIGN KEY (actor_user_id) REFERENCES morningstar.users (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.audit_events ADD CONSTRAINT audit_events_fk_program_id FOREIGN KEY (program_id) REFERENCES morningstar.programs (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.audit_events ADD CONSTRAINT audit_events_fk_assessment_id FOREIGN KEY (assessment_id) REFERENCES morningstar.assessments (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.audit_events ADD CONSTRAINT audit_events_fk_job_id FOREIGN KEY (job_id) REFERENCES morningstar.jobs (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.audit_events ADD CONSTRAINT audit_events_fk_scope_version_id FOREIGN KEY (scope_version_id) REFERENCES morningstar.scope_versions (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.audit_events ADD CONSTRAINT audit_events_fk_decision_id FOREIGN KEY (decision_id) REFERENCES morningstar.policy_decisions (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.audit_events ADD CONSTRAINT audit_events_fk_approval_id FOREIGN KEY (approval_id) REFERENCES morningstar.approvals (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.system_alerts ADD CONSTRAINT system_alerts_fk_cleared_by FOREIGN KEY (cleared_by) REFERENCES morningstar.users (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
