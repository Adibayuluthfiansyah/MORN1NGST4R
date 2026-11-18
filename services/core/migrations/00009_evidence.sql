-- +goose Up
-- Slice 00009 evidence (milestone M6). Generated from docs/design/database/data-dictionary.md and constraints-and-indexes.md, then reviewed.
-- It depends on the tables of 00001 to 00007 only: evidence has no reference to the asset tables of 00008, which belong to M20.
-- Indexes, triggers and grants of this slice are in 00017, because 00013 to 00015 were applied before the slice existed.

CREATE TABLE morningstar.evidence_items (
    id uuid NOT NULL,
    program_id uuid NOT NULL,
    assessment_id uuid NOT NULL,
    job_id uuid,
    attempt_id uuid,
    target_id uuid,
    kind text NOT NULL,
    store_backend text NOT NULL DEFAULT 'local_fs',
    storage_key text NOT NULL,
    sha256 bytea NOT NULL,
    size_bytes bigint NOT NULL,
    content_type text NOT NULL,
    redaction_state text NOT NULL DEFAULT 'redacted',
    sensitivity text NOT NULL DEFAULT 'sensitive',
    captured_at timestamptz NOT NULL,
    origin text NOT NULL,
    tool_key text,
    tool_version text,
    tool_binary_sha256 bytea,
    operation_hash bytea,
    decision_id uuid,
    redacted_args jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT evidence_items_pk PRIMARY KEY (id),
    CONSTRAINT evidence_items_uq_c1 UNIQUE (store_backend, storage_key),
    CONSTRAINT evidence_items_uq_c10 UNIQUE (program_id, id),
    CONSTRAINT evidence_items_ck_c2 CHECK (length(sha256) = 32),
    CONSTRAINT evidence_items_ck_c3 CHECK (size_bytes >= 0),
    CONSTRAINT evidence_items_ck_c4 CHECK (kind IN ('request', 'response', 'request_response', 'tool_output', 'note', 'report')),
    CONSTRAINT evidence_items_ck_c5 CHECK (redaction_state IN ('redacted', 'raw_retained')),
    CONSTRAINT evidence_items_ck_c6 CHECK (sensitivity IN ('normal', 'sensitive')),
    CONSTRAINT evidence_items_ck_c7 CHECK (origin IN ('observed', 'ai_inferred', 'human_entered'))
);

CREATE TABLE morningstar.evidence_integrity_checks (
    id uuid NOT NULL,
    evidence_id uuid NOT NULL,
    checked_at timestamptz NOT NULL DEFAULT now(),
    trigger text NOT NULL,
    result text NOT NULL,
    observed_sha256 bytea,
    observed_size bigint,
    actor_kind text NOT NULL,
    actor_user_id uuid,
    request_id uuid,
    CONSTRAINT evidence_integrity_checks_pk PRIMARY KEY (id),
    CONSTRAINT evidence_integrity_checks_ck_c1 CHECK (trigger IN ('read', 'manual', 'sweep', 'pre_verification')),
    CONSTRAINT evidence_integrity_checks_ck_c2 CHECK (result IN ('match', 'hash_mismatch', 'size_mismatch', 'missing', 'metadata_inconsistent')),
    CONSTRAINT evidence_integrity_checks_ck_c3 CHECK (actor_kind IN ('user', 'worker', 'system')),
    CONSTRAINT evidence_integrity_checks_ck_c4 CHECK ((result IN ('match', 'hash_mismatch')) = (observed_sha256 IS NOT NULL))
);

CREATE TABLE morningstar.evidence_status (
    evidence_id uuid NOT NULL,
    integrity_state text NOT NULL DEFAULT 'unchecked',
    last_check_id uuid,
    retention_state text NOT NULL DEFAULT 'active',
    purged_at timestamptz,
    purged_by uuid,
    compromised_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT evidence_status_pk PRIMARY KEY (evidence_id),
    CONSTRAINT evidence_status_ck_c1 CHECK (integrity_state IN ('unchecked', 'ok', 'compromised', 'missing')),
    CONSTRAINT evidence_status_ck_c2 CHECK (retention_state IN ('active', 'purge_pending', 'purged')),
    CONSTRAINT evidence_status_ck_c3 CHECK ((retention_state = 'purged') = (purged_at IS NOT NULL AND purged_by IS NOT NULL)),
    CONSTRAINT evidence_status_ck_c4 CHECK ((integrity_state <> 'compromised') OR compromised_at IS NOT NULL)
);

ALTER TABLE morningstar.evidence_items ADD CONSTRAINT evidence_items_fk_program_id FOREIGN KEY (program_id) REFERENCES morningstar.programs (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.evidence_items ADD CONSTRAINT evidence_items_fk_assessment_id FOREIGN KEY (assessment_id) REFERENCES morningstar.assessments (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.evidence_items ADD CONSTRAINT evidence_items_fk_job_id FOREIGN KEY (job_id) REFERENCES morningstar.jobs (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.evidence_items ADD CONSTRAINT evidence_items_fk_attempt_id FOREIGN KEY (attempt_id) REFERENCES morningstar.job_attempts (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.evidence_items ADD CONSTRAINT evidence_items_fk_target_id FOREIGN KEY (target_id) REFERENCES morningstar.targets (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.evidence_items ADD CONSTRAINT evidence_items_fk_decision_id FOREIGN KEY (decision_id) REFERENCES morningstar.policy_decisions (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.evidence_items ADD CONSTRAINT evidence_items_fk_program_id_assessment_id FOREIGN KEY (program_id, assessment_id) REFERENCES morningstar.assessments (program_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.evidence_items ADD CONSTRAINT evidence_items_fk_program_id_target_id FOREIGN KEY (program_id, target_id) REFERENCES morningstar.targets (program_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.evidence_integrity_checks ADD CONSTRAINT evidence_integrity_checks_fk_evidence_id FOREIGN KEY (evidence_id) REFERENCES morningstar.evidence_items (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.evidence_integrity_checks ADD CONSTRAINT evidence_integrity_checks_fk_actor_user_id FOREIGN KEY (actor_user_id) REFERENCES morningstar.users (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.evidence_status ADD CONSTRAINT evidence_status_fk_evidence_id FOREIGN KEY (evidence_id) REFERENCES morningstar.evidence_items (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.evidence_status ADD CONSTRAINT evidence_status_fk_last_check_id FOREIGN KEY (last_check_id) REFERENCES morningstar.evidence_integrity_checks (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.evidence_status ADD CONSTRAINT evidence_status_fk_purged_by FOREIGN KEY (purged_by) REFERENCES morningstar.users (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
