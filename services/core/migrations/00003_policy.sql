-- +goose Up
-- Slice 00003 policy (milestone M3). Generated from docs/design/database/data-dictionary.md and constraints-and-indexes.md, then reviewed.
-- Foreign keys are created in the migration of the later of their two tables (DB-21). Indexes, triggers and grants are in 00013 to 00015.

CREATE TABLE morningstar.policy_settings (
    id uuid NOT NULL,
    setting_key text NOT NULL,
    version_no integer NOT NULL,
    value jsonb NOT NULL,
    widening boolean NOT NULL,
    changed_by uuid NOT NULL,
    reason text NOT NULL,
    reauth_grant_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    superseded_at timestamptz,
    CONSTRAINT policy_settings_pk PRIMARY KEY (id),
    CONSTRAINT policy_settings_ck_c1 CHECK (setting_key IN ('cidr_minimum_v4', 'cidr_minimum_v6', 'approval_defaults', 'attempt_maximum', 'risk_overrides', 'audit_retry_bound', 'login_rate_limit', 'evidence_retention', 'verification_requires_reauth')),
    CONSTRAINT policy_settings_uq_c2 UNIQUE (setting_key, version_no),
    CONSTRAINT policy_settings_ck_c3 CHECK (NOT widening OR reauth_grant_id IS NOT NULL),
    CONSTRAINT policy_settings_ck_c4 CHECK (CASE setting_key WHEN 'cidr_minimum_v4' THEN jsonb_typeof(value) = 'number' AND (value #>> '{}')::numeric BETWEEN 24 AND 32 WHEN 'cidr_minimum_v6' THEN jsonb_typeof(value) = 'number' AND (value #>> '{}')::numeric BETWEEN 48 AND 128 ELSE true END)
);

ALTER TABLE morningstar.policy_settings ADD CONSTRAINT policy_settings_fk_changed_by FOREIGN KEY (changed_by) REFERENCES morningstar.users (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.policy_settings ADD CONSTRAINT policy_settings_fk_reauth_grant_id FOREIGN KEY (reauth_grant_id) REFERENCES morningstar.reauth_grants (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
