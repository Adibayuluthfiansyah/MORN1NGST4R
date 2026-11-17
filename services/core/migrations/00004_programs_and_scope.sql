-- +goose Up
-- Slice 00004 programs_and_scope (milestone M3). Generated from docs/design/database/data-dictionary.md and constraints-and-indexes.md, then reviewed.
-- Foreign keys are created in the migration of the later of their two tables (DB-21). Indexes, triggers and grants are in 00013 to 00015.

CREATE TABLE morningstar.programs (
    id uuid NOT NULL,
    owner_user_id uuid NOT NULL,
    name text NOT NULL,
    platform_label text,
    description text,
    status text NOT NULL DEFAULT 'active',
    archived_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version integer NOT NULL DEFAULT 1,
    CONSTRAINT programs_pk PRIMARY KEY (id),
    CONSTRAINT programs_ck_c1 CHECK (status IN ('active', 'archived')),
    CONSTRAINT programs_ck_c2 CHECK ((status = 'archived') = (archived_at IS NOT NULL)),
    CONSTRAINT programs_uq_c3 UNIQUE (id)
);

CREATE TABLE morningstar.program_rules_versions (
    id uuid NOT NULL,
    program_id uuid NOT NULL,
    version_no integer NOT NULL,
    reference_url text,
    rules_text text,
    content_sha256 bytea NOT NULL,
    recorded_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT program_rules_versions_pk PRIMARY KEY (id),
    CONSTRAINT program_rules_versions_uq_c1 UNIQUE (program_id, version_no),
    CONSTRAINT program_rules_versions_uq_c2 UNIQUE (program_id, id),
    CONSTRAINT program_rules_versions_ck_c3 CHECK (reference_url IS NOT NULL OR rules_text IS NOT NULL),
    CONSTRAINT program_rules_versions_ck_c4 CHECK (length(content_sha256) = 32)
);

CREATE TABLE morningstar.scope_versions (
    id uuid NOT NULL,
    program_id uuid NOT NULL,
    version_no integer NOT NULL,
    rules_version_id uuid,
    content_hash bytea NOT NULL,
    widening boolean NOT NULL,
    supersedes_version_id uuid,
    created_by uuid NOT NULL,
    reason text NOT NULL,
    reauth_grant_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    superseded_at timestamptz,
    CONSTRAINT scope_versions_pk PRIMARY KEY (id),
    CONSTRAINT scope_versions_uq_c1 UNIQUE (program_id, version_no),
    CONSTRAINT scope_versions_uq_c2 UNIQUE (program_id, id),
    CONSTRAINT scope_versions_ck_c3 CHECK (length(content_hash) = 32),
    CONSTRAINT scope_versions_ck_c4 CHECK (NOT widening OR reauth_grant_id IS NOT NULL),
    CONSTRAINT scope_versions_ck_c5 CHECK ((version_no = 1) = (supersedes_version_id IS NULL))
);

CREATE TABLE morningstar.scope_rules (
    id uuid NOT NULL,
    program_id uuid NOT NULL,
    scope_version_id uuid NOT NULL,
    kind text NOT NULL,
    exclude boolean NOT NULL,
    canonical_value text NOT NULL,
    ports_canonical text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT scope_rules_pk PRIMARY KEY (id),
    CONSTRAINT scope_rules_ck_c1 CHECK (kind IN ('domain', 'wildcard', 'ip', 'cidr')),
    CONSTRAINT scope_rules_ck_c2 CHECK (NOT (kind = 'wildcard' AND NOT exclude)),
    CONSTRAINT scope_rules_uq_c3 UNIQUE (scope_version_id, kind, exclude, canonical_value, ports_canonical),
    CONSTRAINT scope_rules_ck_c5 CHECK (length(canonical_value) BETWEEN 1 AND 253 OR kind IN ('ip', 'cidr'))
);

CREATE TABLE morningstar.refused_exclusions (
    id uuid NOT NULL,
    program_id uuid NOT NULL,
    scope_version_id uuid NOT NULL,
    original_text text NOT NULL,
    refusal_reason text NOT NULL,
    status text NOT NULL DEFAULT 'open',
    acknowledged_by uuid,
    acknowledged_at timestamptz,
    acknowledged_target_set_hash bytea,
    replaced_by_rule_id uuid,
    reauth_grant_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT refused_exclusions_pk PRIMARY KEY (id),
    CONSTRAINT refused_exclusions_ck_c1 CHECK (status IN ('open', 'acknowledged', 'replaced')),
    CONSTRAINT refused_exclusions_ck_c2 CHECK (length(original_text) <= 1000),
    CONSTRAINT refused_exclusions_ck_c3 CHECK ((status <> 'acknowledged') OR (acknowledged_by IS NOT NULL AND acknowledged_at IS NOT NULL AND acknowledged_target_set_hash IS NOT NULL AND reauth_grant_id IS NOT NULL)),
    CONSTRAINT refused_exclusions_ck_c4 CHECK ((status <> 'replaced') OR replaced_by_rule_id IS NOT NULL)
);

CREATE TABLE morningstar.targets (
    id uuid NOT NULL,
    program_id uuid NOT NULL,
    kind text NOT NULL,
    canonical_value text NOT NULL,
    display_value text NOT NULL,
    origin text NOT NULL DEFAULT 'human',
    promoted_from_asset_id uuid,
    label text,
    status text NOT NULL DEFAULT 'registered',
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    CONSTRAINT targets_pk PRIMARY KEY (id),
    CONSTRAINT targets_ck_c1 CHECK (kind IN ('hostname', 'ip', 'cidr')),
    CONSTRAINT targets_ck_c2 CHECK (origin IN ('human', 'promoted_asset')),
    CONSTRAINT targets_ck_c3 CHECK ((origin = 'promoted_asset') = (promoted_from_asset_id IS NOT NULL)),
    CONSTRAINT targets_ck_c4 CHECK (status IN ('registered', 'archived')),
    CONSTRAINT targets_uq_c5 UNIQUE (program_id, kind, canonical_value),
    CONSTRAINT targets_uq_c6 UNIQUE (program_id, id)
);

CREATE TABLE morningstar.lab_registrations (
    id uuid NOT NULL,
    program_id uuid NOT NULL,
    assessment_id uuid NOT NULL,
    target_id uuid NOT NULL,
    kind text NOT NULL,
    ports_canonical text,
    ports_required boolean NOT NULL,
    scope_version_id uuid NOT NULL,
    attestation_text_version text NOT NULL,
    registered_by uuid NOT NULL,
    reauth_grant_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    status text NOT NULL DEFAULT 'active',
    revoked_at timestamptz,
    revoked_by uuid,
    revoke_reason text,
    supersedes_lab_id uuid,
    CONSTRAINT lab_registrations_pk PRIMARY KEY (id),
    CONSTRAINT lab_registrations_ck_c1 CHECK (kind IN ('address', 'cidr', 'hostname')),
    CONSTRAINT lab_registrations_ck_c2 CHECK (status IN ('active', 'expired', 'revoked')),
    CONSTRAINT lab_registrations_ck_c3 CHECK (expires_at > created_at),
    CONSTRAINT lab_registrations_ck_c4 CHECK (NOT ports_required OR ports_canonical IS NOT NULL),
    CONSTRAINT lab_registrations_ck_c5 CHECK ((status = 'revoked') = (revoked_at IS NOT NULL AND revoked_by IS NOT NULL AND revoke_reason IS NOT NULL)),
    CONSTRAINT lab_registrations_uq_c9 UNIQUE (program_id, id)
);

CREATE TABLE morningstar.lab_registration_ranges (
    id uuid NOT NULL,
    lab_registration_id uuid NOT NULL,
    range_cidr cidr NOT NULL,
    ports_canonical text,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT lab_registration_ranges_pk PRIMARY KEY (id),
    CONSTRAINT lab_registration_ranges_uq_c1 UNIQUE (lab_registration_id, range_cidr),
    CONSTRAINT lab_registration_ranges_ck_c2 CHECK (NOT (range_cidr && ANY (ARRAY['169.254.0.0/16','fe80::/10','224.0.0.0/4','ff00::/8','0.0.0.0/8','::/128','255.255.255.255/32','240.0.0.0/4','168.63.129.16/32','100.100.100.200/32','fd00:ec2::254/128','fd20:ce::254/128']::cidr[]))),
    CONSTRAINT lab_registration_ranges_ck_c3 CHECK ((family(range_cidr) = 4 AND masklen(range_cidr) >= 24) OR (family(range_cidr) = 6 AND masklen(range_cidr) >= 48) OR masklen(range_cidr) = CASE family(range_cidr) WHEN 4 THEN 32 ELSE 128 END)
);

CREATE TABLE morningstar.target_validations (
    id uuid NOT NULL,
    program_id uuid NOT NULL,
    assessment_id uuid NOT NULL,
    scope_version_id uuid NOT NULL,
    rules_version_id uuid NOT NULL,
    target_set_hash bytea NOT NULL,
    attestation_kind text NOT NULL,
    attestation_text_version text NOT NULL,
    attested_by uuid NOT NULL,
    attested_at timestamptz NOT NULL DEFAULT now(),
    status text NOT NULL DEFAULT 'current',
    invalidated_at timestamptz,
    invalidation_reason text,
    policy_snapshot_hash bytea NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT target_validations_pk PRIMARY KEY (id),
    CONSTRAINT target_validations_ck_c1 CHECK (attestation_kind IN ('standard', 'lab')),
    CONSTRAINT target_validations_ck_c2 CHECK (status IN ('current', 'stale', 'expired', 'invalidated')),
    CONSTRAINT target_validations_ck_c3 CHECK ((status = 'current') = (invalidated_at IS NULL)),
    CONSTRAINT target_validations_ck_c4 CHECK ((status = 'current') OR invalidation_reason IS NOT NULL),
    CONSTRAINT target_validations_ck_c5 CHECK (invalidation_reason IS NULL OR invalidation_reason IN ('scope_change', 'rules_change', 'lab_change', 'target_set_change', 'expiry', 'manual', 'restore_recovery')),
    CONSTRAINT target_validations_ck_c6 CHECK (expires_at > attested_at),
    CONSTRAINT target_validations_uq_c10 UNIQUE (program_id, id)
);

CREATE TABLE morningstar.target_validation_targets (
    id uuid NOT NULL,
    validation_id uuid NOT NULL,
    target_id uuid NOT NULL,
    canonical_value text NOT NULL,
    decision text NOT NULL,
    reasons jsonb NOT NULL DEFAULT '[]',
    matched_scope_rule_id uuid,
    lab_registration_id uuid,
    CONSTRAINT target_validation_targets_pk PRIMARY KEY (id),
    CONSTRAINT target_validation_targets_ck_c1 CHECK (decision IN ('allow', 'deny')),
    CONSTRAINT target_validation_targets_uq_c2 UNIQUE (validation_id, target_id)
);

ALTER TABLE morningstar.programs ADD CONSTRAINT programs_fk_owner_user_id FOREIGN KEY (owner_user_id) REFERENCES morningstar.users (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.program_rules_versions ADD CONSTRAINT program_rules_versions_fk_program_id FOREIGN KEY (program_id) REFERENCES morningstar.programs (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.program_rules_versions ADD CONSTRAINT program_rules_versions_fk_recorded_by FOREIGN KEY (recorded_by) REFERENCES morningstar.users (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.scope_versions ADD CONSTRAINT scope_versions_fk_program_id FOREIGN KEY (program_id) REFERENCES morningstar.programs (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.scope_versions ADD CONSTRAINT scope_versions_fk_rules_version_id FOREIGN KEY (rules_version_id) REFERENCES morningstar.program_rules_versions (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.scope_versions ADD CONSTRAINT scope_versions_fk_supersedes_version_id FOREIGN KEY (supersedes_version_id) REFERENCES morningstar.scope_versions (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.scope_versions ADD CONSTRAINT scope_versions_fk_created_by FOREIGN KEY (created_by) REFERENCES morningstar.users (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.scope_versions ADD CONSTRAINT scope_versions_fk_reauth_grant_id FOREIGN KEY (reauth_grant_id) REFERENCES morningstar.reauth_grants (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.scope_versions ADD CONSTRAINT scope_versions_fk_program_id_rules_version_id FOREIGN KEY (program_id, rules_version_id) REFERENCES morningstar.program_rules_versions (program_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.scope_versions ADD CONSTRAINT scope_versions_fk_program_id_supersedes_version_id FOREIGN KEY (program_id, supersedes_version_id) REFERENCES morningstar.scope_versions (program_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.scope_rules ADD CONSTRAINT scope_rules_fk_program_id FOREIGN KEY (program_id) REFERENCES morningstar.programs (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.scope_rules ADD CONSTRAINT scope_rules_fk_scope_version_id FOREIGN KEY (scope_version_id) REFERENCES morningstar.scope_versions (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.scope_rules ADD CONSTRAINT scope_rules_fk_program_id_scope_version_id FOREIGN KEY (program_id, scope_version_id) REFERENCES morningstar.scope_versions (program_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.refused_exclusions ADD CONSTRAINT refused_exclusions_fk_program_id FOREIGN KEY (program_id) REFERENCES morningstar.programs (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.refused_exclusions ADD CONSTRAINT refused_exclusions_fk_scope_version_id FOREIGN KEY (scope_version_id) REFERENCES morningstar.scope_versions (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.refused_exclusions ADD CONSTRAINT refused_exclusions_fk_acknowledged_by FOREIGN KEY (acknowledged_by) REFERENCES morningstar.users (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.refused_exclusions ADD CONSTRAINT refused_exclusions_fk_replaced_by_rule_id FOREIGN KEY (replaced_by_rule_id) REFERENCES morningstar.scope_rules (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.refused_exclusions ADD CONSTRAINT refused_exclusions_fk_reauth_grant_id FOREIGN KEY (reauth_grant_id) REFERENCES morningstar.reauth_grants (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.refused_exclusions ADD CONSTRAINT refused_exclusions_fk_program_id_scope_version_id FOREIGN KEY (program_id, scope_version_id) REFERENCES morningstar.scope_versions (program_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.targets ADD CONSTRAINT targets_fk_program_id FOREIGN KEY (program_id) REFERENCES morningstar.programs (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.targets ADD CONSTRAINT targets_fk_created_by FOREIGN KEY (created_by) REFERENCES morningstar.users (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.lab_registrations ADD CONSTRAINT lab_registrations_fk_program_id FOREIGN KEY (program_id) REFERENCES morningstar.programs (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.lab_registrations ADD CONSTRAINT lab_registrations_fk_target_id FOREIGN KEY (target_id) REFERENCES morningstar.targets (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.lab_registrations ADD CONSTRAINT lab_registrations_fk_scope_version_id FOREIGN KEY (scope_version_id) REFERENCES morningstar.scope_versions (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.lab_registrations ADD CONSTRAINT lab_registrations_fk_registered_by FOREIGN KEY (registered_by) REFERENCES morningstar.users (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.lab_registrations ADD CONSTRAINT lab_registrations_fk_reauth_grant_id FOREIGN KEY (reauth_grant_id) REFERENCES morningstar.reauth_grants (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.lab_registrations ADD CONSTRAINT lab_registrations_fk_revoked_by FOREIGN KEY (revoked_by) REFERENCES morningstar.users (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.lab_registrations ADD CONSTRAINT lab_registrations_fk_supersedes_lab_id FOREIGN KEY (supersedes_lab_id) REFERENCES morningstar.lab_registrations (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.lab_registrations ADD CONSTRAINT lab_registrations_fk_program_id_target_id FOREIGN KEY (program_id, target_id) REFERENCES morningstar.targets (program_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.lab_registrations ADD CONSTRAINT lab_registrations_fk_program_id_scope_version_id FOREIGN KEY (program_id, scope_version_id) REFERENCES morningstar.scope_versions (program_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.lab_registration_ranges ADD CONSTRAINT lab_registration_ranges_fk_lab_registration_id FOREIGN KEY (lab_registration_id) REFERENCES morningstar.lab_registrations (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.target_validations ADD CONSTRAINT target_validations_fk_program_id FOREIGN KEY (program_id) REFERENCES morningstar.programs (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.target_validations ADD CONSTRAINT target_validations_fk_scope_version_id FOREIGN KEY (scope_version_id) REFERENCES morningstar.scope_versions (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.target_validations ADD CONSTRAINT target_validations_fk_rules_version_id FOREIGN KEY (rules_version_id) REFERENCES morningstar.program_rules_versions (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.target_validations ADD CONSTRAINT target_validations_fk_attested_by FOREIGN KEY (attested_by) REFERENCES morningstar.users (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.target_validations ADD CONSTRAINT target_validations_fk_program_id_scope_version_id FOREIGN KEY (program_id, scope_version_id) REFERENCES morningstar.scope_versions (program_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.target_validations ADD CONSTRAINT target_validations_fk_program_id_rules_version_id FOREIGN KEY (program_id, rules_version_id) REFERENCES morningstar.program_rules_versions (program_id, id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.target_validation_targets ADD CONSTRAINT target_validation_targets_fk_validation_id FOREIGN KEY (validation_id) REFERENCES morningstar.target_validations (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.target_validation_targets ADD CONSTRAINT target_validation_targets_fk_target_id FOREIGN KEY (target_id) REFERENCES morningstar.targets (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.target_validation_targets ADD CONSTRAINT target_validation_targets_fk_matched_scope_rule_id FOREIGN KEY (matched_scope_rule_id) REFERENCES morningstar.scope_rules (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.target_validation_targets ADD CONSTRAINT target_validation_targets_fk_lab_registration_id FOREIGN KEY (lab_registration_id) REFERENCES morningstar.lab_registrations (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
