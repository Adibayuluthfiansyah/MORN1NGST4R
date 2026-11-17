-- +goose Up
-- Slice 00005 assessments_and_plans (milestone M3). Generated from docs/design/database/data-dictionary.md and constraints-and-indexes.md, then reviewed.
-- Foreign keys are created in the migration of the later of their two tables (DB-21). Indexes, triggers and grants are in 00013 to 00015.

CREATE TABLE morningstar.assessments (
    id uuid NOT NULL,
    program_id uuid NOT NULL,
    name text NOT NULL,
    description text,
    status text NOT NULL DEFAULT 'draft',
    control_state text NOT NULL DEFAULT 'none',
    raw_evidence_capture boolean NOT NULL DEFAULT false,
    current_validation_id uuid,
    created_by uuid NOT NULL,
    started_at timestamptz,
    completed_at timestamptz,
    cancelled_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version integer NOT NULL DEFAULT 1,
    CONSTRAINT assessments_pk PRIMARY KEY (id),
    CONSTRAINT assessments_ck_c1 CHECK (status IN ('draft', 'ready', 'running', 'paused', 'completed', 'cancelled')),
    CONSTRAINT assessments_ck_c2 CHECK (control_state IN ('none', 'stop_requested')),
    CONSTRAINT assessments_uq_c3 UNIQUE (program_id, id),
    CONSTRAINT assessments_ck_c4 CHECK (status IN ('draft', 'cancelled') OR current_validation_id IS NOT NULL)
);

CREATE TABLE morningstar.assessment_targets (
    assessment_id uuid NOT NULL,
    target_id uuid NOT NULL,
    program_id uuid NOT NULL,
    added_by uuid NOT NULL,
    added_at timestamptz NOT NULL DEFAULT now(),
    removed_at timestamptz,
    CONSTRAINT assessment_targets_pk PRIMARY KEY (assessment_id, target_id)
);

CREATE TABLE morningstar.assessment_transitions (
    id uuid NOT NULL,
    assessment_id uuid NOT NULL,
    from_status text,
    to_status text NOT NULL,
    actor_user_id uuid NOT NULL,
    reason text,
    request_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT assessment_transitions_pk PRIMARY KEY (id),
    CONSTRAINT assessment_transitions_ck_c1 CHECK ((from_status IS NULL AND to_status = 'draft') OR (from_status = 'draft' AND to_status = 'ready') OR (from_status = 'ready' AND to_status = 'draft') OR (from_status = 'ready' AND to_status = 'running') OR (from_status = 'running' AND to_status = 'paused') OR (from_status = 'paused' AND to_status = 'running') OR (from_status = 'running' AND to_status = 'completed') OR (from_status = 'paused' AND to_status = 'completed') OR (from_status = 'draft' AND to_status = 'cancelled') OR (from_status = 'ready' AND to_status = 'cancelled') OR (from_status = 'running' AND to_status = 'cancelled') OR (from_status = 'paused' AND to_status = 'cancelled'))
);

CREATE TABLE morningstar.plan_items (
    id uuid NOT NULL,
    program_id uuid NOT NULL,
    assessment_id uuid NOT NULL,
    target_id uuid NOT NULL,
    operation_id uuid NOT NULL,
    origin text NOT NULL,
    ai_job_id uuid,
    status text NOT NULL DEFAULT 'proposed',
    rationale text,
    decided_by uuid,
    decided_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT plan_items_pk PRIMARY KEY (id),
    CONSTRAINT plan_items_ck_c1 CHECK (origin IN ('human', 'ai')),
    CONSTRAINT plan_items_ck_c2 CHECK (status IN ('proposed', 'accepted', 'rejected', 'converted', 'superseded')),
    CONSTRAINT plan_items_ck_c3 CHECK ((origin = 'ai') = (ai_job_id IS NOT NULL)),
    CONSTRAINT plan_items_ck_c4 CHECK ((status IN ('accepted', 'rejected')) = (decided_by IS NOT NULL)),
    CONSTRAINT plan_items_uq_c8 UNIQUE (assessment_id, id)
);

ALTER TABLE morningstar.lab_registrations ADD CONSTRAINT lab_registrations_fk_assessment_id FOREIGN KEY (assessment_id) REFERENCES morningstar.assessments (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.lab_registrations ADD CONSTRAINT lab_registrations_fk_program_id_assessment_id FOREIGN KEY (program_id, assessment_id) REFERENCES morningstar.assessments (program_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.target_validations ADD CONSTRAINT target_validations_fk_assessment_id FOREIGN KEY (assessment_id) REFERENCES morningstar.assessments (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.target_validations ADD CONSTRAINT target_validations_fk_program_id_assessment_id FOREIGN KEY (program_id, assessment_id) REFERENCES morningstar.assessments (program_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.assessments ADD CONSTRAINT assessments_fk_program_id FOREIGN KEY (program_id) REFERENCES morningstar.programs (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.assessments ADD CONSTRAINT assessments_fk_current_validation_id FOREIGN KEY (current_validation_id) REFERENCES morningstar.target_validations (id) ON DELETE RESTRICT ON UPDATE RESTRICT DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE morningstar.assessments ADD CONSTRAINT assessments_fk_created_by FOREIGN KEY (created_by) REFERENCES morningstar.users (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.assessments ADD CONSTRAINT assessments_fk_program_id_current_validation_id FOREIGN KEY (program_id, current_validation_id) REFERENCES morningstar.target_validations (program_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE morningstar.assessment_targets ADD CONSTRAINT assessment_targets_fk_assessment_id FOREIGN KEY (assessment_id) REFERENCES morningstar.assessments (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.assessment_targets ADD CONSTRAINT assessment_targets_fk_target_id FOREIGN KEY (target_id) REFERENCES morningstar.targets (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.assessment_targets ADD CONSTRAINT assessment_targets_fk_program_id FOREIGN KEY (program_id) REFERENCES morningstar.programs (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.assessment_targets ADD CONSTRAINT assessment_targets_fk_added_by FOREIGN KEY (added_by) REFERENCES morningstar.users (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.assessment_targets ADD CONSTRAINT assessment_targets_fk_program_id_assessment_id FOREIGN KEY (program_id, assessment_id) REFERENCES morningstar.assessments (program_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.assessment_targets ADD CONSTRAINT assessment_targets_fk_program_id_target_id FOREIGN KEY (program_id, target_id) REFERENCES morningstar.targets (program_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.assessment_transitions ADD CONSTRAINT assessment_transitions_fk_assessment_id FOREIGN KEY (assessment_id) REFERENCES morningstar.assessments (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.assessment_transitions ADD CONSTRAINT assessment_transitions_fk_actor_user_id FOREIGN KEY (actor_user_id) REFERENCES morningstar.users (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.plan_items ADD CONSTRAINT plan_items_fk_program_id FOREIGN KEY (program_id) REFERENCES morningstar.programs (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.plan_items ADD CONSTRAINT plan_items_fk_assessment_id FOREIGN KEY (assessment_id) REFERENCES morningstar.assessments (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.plan_items ADD CONSTRAINT plan_items_fk_target_id FOREIGN KEY (target_id) REFERENCES morningstar.targets (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.plan_items ADD CONSTRAINT plan_items_fk_decided_by FOREIGN KEY (decided_by) REFERENCES morningstar.users (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.plan_items ADD CONSTRAINT plan_items_fk_program_id_assessment_id FOREIGN KEY (program_id, assessment_id) REFERENCES morningstar.assessments (program_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.plan_items ADD CONSTRAINT plan_items_fk_program_id_target_id FOREIGN KEY (program_id, target_id) REFERENCES morningstar.targets (program_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT;
