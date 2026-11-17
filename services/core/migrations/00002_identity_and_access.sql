-- +goose Up
-- Slice 00002 identity_and_access (milestone M3). Generated from docs/design/database/data-dictionary.md and constraints-and-indexes.md, then reviewed.
-- Foreign keys are created in the migration of the later of their two tables (DB-21). Indexes, triggers and grants are in 00013 to 00015.

CREATE TABLE morningstar.users (
    id uuid NOT NULL,
    username text NOT NULL,
    display_name text,
    status text NOT NULL DEFAULT 'active',
    password_changed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version integer NOT NULL DEFAULT 1,
    CONSTRAINT users_pk PRIMARY KEY (id),
    CONSTRAINT users_ck_c1 CHECK (status IN ('active', 'disabled')),
    CONSTRAINT users_uq_c2 UNIQUE (username),
    CONSTRAINT users_ck_c3 CHECK (username = lower(username) AND length(username) BETWEEN 3 AND 64)
);

CREATE TABLE morningstar.user_credentials (
    id uuid NOT NULL,
    user_id uuid NOT NULL,
    algorithm text NOT NULL DEFAULT 'argon2id',
    phc_hash text NOT NULL,
    is_current boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    superseded_at timestamptz,
    CONSTRAINT user_credentials_pk PRIMARY KEY (id),
    CONSTRAINT user_credentials_ck_c1 CHECK (algorithm IN ('argon2id')),
    CONSTRAINT user_credentials_ck_c2 CHECK ((is_current AND superseded_at IS NULL) OR (NOT is_current AND superseded_at IS NOT NULL))
);

CREATE TABLE morningstar.recovery_codes (
    id uuid NOT NULL,
    user_id uuid NOT NULL,
    code_hash text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    used_at timestamptz,
    revoked_at timestamptz,
    CONSTRAINT recovery_codes_pk PRIMARY KEY (id),
    CONSTRAINT recovery_codes_ck_c1 CHECK (NOT (used_at IS NOT NULL AND revoked_at IS NOT NULL))
);

CREATE TABLE morningstar.bootstrap_secrets (
    id uuid NOT NULL,
    secret_hash text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    superseded_at timestamptz,
    consumed_at timestamptz,
    consumed_user_id uuid,
    CONSTRAINT bootstrap_secrets_pk PRIMARY KEY (id),
    CONSTRAINT bootstrap_secrets_ck_c1 CHECK ((consumed_at IS NULL) = (consumed_user_id IS NULL)),
    CONSTRAINT bootstrap_secrets_ck_c2 CHECK (NOT (consumed_at IS NOT NULL AND superseded_at IS NOT NULL))
);

CREATE TABLE morningstar.sessions (
    id uuid NOT NULL,
    user_id uuid NOT NULL,
    token_hash bytea NOT NULL,
    csrf_token_hash bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    idle_expires_at timestamptz NOT NULL,
    absolute_expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    revoked_reason text,
    user_agent text,
    CONSTRAINT sessions_pk PRIMARY KEY (id),
    CONSTRAINT sessions_uq_c1 UNIQUE (token_hash),
    CONSTRAINT sessions_ck_c2 CHECK (length(token_hash) = 32 AND length(csrf_token_hash) = 32),
    CONSTRAINT sessions_ck_c3 CHECK (absolute_expires_at > created_at),
    CONSTRAINT sessions_ck_c4 CHECK ((revoked_at IS NULL) = (revoked_reason IS NULL))
);

CREATE TABLE morningstar.reauth_grants (
    id uuid NOT NULL,
    user_id uuid NOT NULL,
    session_id uuid NOT NULL,
    purpose text NOT NULL,
    subject_hash bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    CONSTRAINT reauth_grants_pk PRIMARY KEY (id),
    CONSTRAINT reauth_grants_ck_c1 CHECK (purpose IN ('approval_intrusive', 'policy_widening', 'scope_widening', 'lab_registration', 'exclusion_acknowledgement', 'raw_evidence_capture', 'provider_enablement', 'tool_registration', 'passive_source_enablement', 'verification', 'password_change')),
    CONSTRAINT reauth_grants_ck_c2 CHECK (expires_at - created_at <= interval '120 seconds'),
    CONSTRAINT reauth_grants_ck_c3 CHECK (length(subject_hash) = 32),
    CONSTRAINT reauth_grants_ck_c4 CHECK (expires_at > created_at)
);

CREATE TABLE morningstar.login_attempts (
    id uuid NOT NULL,
    kind text NOT NULL,
    user_id uuid,
    source_key text NOT NULL,
    succeeded boolean NOT NULL,
    attempted_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT login_attempts_pk PRIMARY KEY (id),
    CONSTRAINT login_attempts_ck_c1 CHECK (kind IN ('login', 'reauthenticate', 'recovery', 'bootstrap'))
);

CREATE TABLE morningstar.recovery_state (
    singleton boolean NOT NULL DEFAULT true,
    instance_id uuid NOT NULL,
    generation bigint NOT NULL DEFAULT 1,
    state text NOT NULL DEFAULT 'normal',
    last_reason text,
    reconciled_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT recovery_state_pk PRIMARY KEY (singleton),
    CONSTRAINT recovery_state_ck_c1 CHECK (singleton),
    CONSTRAINT recovery_state_ck_c2 CHECK (state IN ('normal', 'reconciling')),
    CONSTRAINT recovery_state_ck_c3 CHECK (generation >= 1),
    CONSTRAINT recovery_state_ck_c4 CHECK (last_reason IS NULL OR last_reason IN ('marker_missing', 'marker_invalid', 'marker_older', 'database_older', 'instance_mismatch', 'reconcile_interrupted', 'first_run'))
);

ALTER TABLE morningstar.user_credentials ADD CONSTRAINT user_credentials_fk_user_id FOREIGN KEY (user_id) REFERENCES morningstar.users (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.recovery_codes ADD CONSTRAINT recovery_codes_fk_user_id FOREIGN KEY (user_id) REFERENCES morningstar.users (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.bootstrap_secrets ADD CONSTRAINT bootstrap_secrets_fk_consumed_user_id FOREIGN KEY (consumed_user_id) REFERENCES morningstar.users (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.sessions ADD CONSTRAINT sessions_fk_user_id FOREIGN KEY (user_id) REFERENCES morningstar.users (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.reauth_grants ADD CONSTRAINT reauth_grants_fk_user_id FOREIGN KEY (user_id) REFERENCES morningstar.users (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.reauth_grants ADD CONSTRAINT reauth_grants_fk_session_id FOREIGN KEY (session_id) REFERENCES morningstar.sessions (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
ALTER TABLE morningstar.login_attempts ADD CONSTRAINT login_attempts_fk_user_id FOREIGN KEY (user_id) REFERENCES morningstar.users (id) ON DELETE RESTRICT ON UPDATE RESTRICT;
