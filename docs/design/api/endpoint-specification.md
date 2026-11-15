# Endpoint specification

Status: design proposal. Common rules for every endpoint are in `conventions.md` and `authentication-and-authorization.md`. This file states what is specific. Rate limit classes: RL-AUTH = 5 failed attempts per rolling 15 minutes per client key and per account (D-68), plus 30 requests per minute; RL-READ = 300 per minute per session; RL-WRITE = 60 per minute per session; RL-PREVIEW = 30 per minute per session; RL-DOWNLOAD = 30 per minute per session; RL-AITEST = 6 per minute per session (proposed numbers, DQ-06).

Success status codes: 200 for reads and actions, 201 for creation, 204 for logout and password change. Every response carries `X-Request-Id`. Every error uses the schema in `error-model.md`.

## Authentication and account

#### AUTH-01 GET `/api/v1/session`

| Property | Value |
| --- | --- |
| Purpose | Report setup, anonymous or authenticated state and issue the CSRF token. |
| Authentication | None (public route) |
| Authorization | Public route. Host allow-list, Origin allow-list for state changes, rate limit. State precondition: none |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, cookies optional |
| Path parameters | none |
| Query parameters | none |
| Request schema | none |
| Response schema | `SessionState` |
| Success status | 200 |
| Error responses | 403 `host_denied`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-AUTH (5 failed attempts per rolling 15 minutes per client key and per account (D-68), plus 30 requests per minute) |
| Security considerations | Returns no account data to an anonymous caller beyond the state. The CSRF token is returned in the body and never in a URL or cookie readable by script. |
| PRD requirements | FR-AUTH-001, FR-AUTH-002 |

#### AUTH-02 POST `/api/v1/setup/account`

| Property | Value |
| --- | --- |
| Purpose | Create the single account with the bootstrap secret. |
| Authentication | None (public route) |
| Authorization | Public route. Host allow-list, Origin allow-list for state changes, rate limit. State precondition: No account exists; live bootstrap secret |
| Re-authentication | No |
| CSRF | Yes (anonymous pre-session token) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_anon` (from GET /session), `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | none |
| Query parameters | none |
| Request schema | `SetupAccountRequest` |
| Response schema | `SetupAccountResult` |
| Success status | 201 |
| Error responses | 400 `invalid_request`, 401 `bootstrap_secret_invalid`, 403 `csrf_invalid`, 403 `host_denied`, 403 `origin_denied`, 409 `account_exists`, 409 `conflict_state`, 409 `setup_unavailable`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | State-guarded. A repeat after the transition returns `conflict_state` and changes nothing. |
| Audit event | `auth.account_created` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-AUTH (5 failed attempts per rolling 15 minutes per client key and per account (D-68), plus 30 requests per minute) |
| Security considerations | Single use. The bootstrap secret is compared in constant time against its stored hash, never logged, never in a URL. Refused when the memory-backed secret file location is unavailable (D-68). The recovery code is returned once and never again. |
| PRD requirements | FR-AUTH-001 |

#### AUTH-03 POST `/api/v1/session/login`

| Property | Value |
| --- | --- |
| Purpose | Log in with username and password. |
| Authentication | None (public route) |
| Authorization | Public route. Host allow-list, Origin allow-list for state changes, rate limit. State precondition: Account exists |
| Re-authentication | No |
| CSRF | Yes (anonymous pre-session token) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_anon` (from GET /session), `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | none |
| Query parameters | none |
| Request schema | `LoginRequest` |
| Response schema | `SessionState` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `invalid_credentials`, 403 `csrf_invalid`, 403 `host_denied`, 403 `origin_denied`, 409 `setup_unavailable`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | `auth.login` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-AUTH (5 failed attempts per rolling 15 minutes per client key and per account (D-68), plus 30 requests per minute) |
| Security considerations | The same error for an unknown user and a wrong password. A constant-cost hash check runs for unknown users. Session fixation is prevented by always issuing a new token. Failed attempts are counted before the hash check. |
| PRD requirements | FR-AUTH-001 |

#### AUTH-04 POST `/api/v1/session/logout`

| Property | Value |
| --- | --- |
| Purpose | End the current session. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; account-level resource |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token` |
| Path parameters | none |
| Query parameters | none |
| Request schema | none |
| Response schema | empty |
| Success status | 204 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | `auth.logout` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-AUTH-001 |

#### AUTH-05 GET `/api/v1/sessions`

| Property | Value |
| --- | --- |
| Purpose | List active sessions of the account. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; account-level resource |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | none |
| Query parameters | `limit`, `cursor`, `sort` |
| Request schema | none |
| Response schema | `SessionList` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-AUTH-001 |

#### AUTH-06 POST `/api/v1/sessions/{sessionId}/revoke`

| Property | Value |
| --- | --- |
| Purpose | Revoke one session. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; account-level resource; precondition: Session belongs to the user |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token` |
| Path parameters | `sessionId` |
| Query parameters | none |
| Request schema | none |
| Response schema | `Session` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | State-guarded. A repeat after the transition returns `conflict_state` and changes nothing. |
| Audit event | `auth.session_revoked` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-AUTH-001 |

#### AUTH-07 POST `/api/v1/session/reauthenticate`

| Property | Value |
| --- | --- |
| Purpose | Enter the password again and receive a single-use grant for one exact action. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; account-level resource |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | none |
| Query parameters | none |
| Request schema | `ReauthRequest` |
| Response schema | `ReauthGrant` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `invalid_credentials`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `reauth_subject_invalid`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | `auth.reauthenticated` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-AUTH (5 failed attempts per rolling 15 minutes per client key and per account (D-68), plus 30 requests per minute) |
| Security considerations | The grant is bound to the session, the purpose and the SHA-256 of the canonical subject. It is single use and short lived. It cannot be used from another session or for another action (D-69). |
| PRD requirements | FR-APPROVAL-005 |

#### AUTH-08 POST `/api/v1/recovery/reset`

| Property | Value |
| --- | --- |
| Purpose | Reset the password with the one-time recovery code. |
| Authentication | None (public route) |
| Authorization | Public route. Host allow-list, Origin allow-list for state changes, rate limit. State precondition: A live recovery code exists |
| Re-authentication | No |
| CSRF | Yes (anonymous pre-session token) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_anon` (from GET /session), `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | none |
| Query parameters | none |
| Request schema | `RecoveryResetRequest` |
| Response schema | `RecoveryResetResult` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `recovery_invalid`, 403 `csrf_invalid`, 403 `host_denied`, 403 `origin_denied`, 409 `conflict_state`, 409 `setup_unavailable`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | State-guarded. A repeat after the transition returns `conflict_state` and changes nothing. |
| Audit event | `auth.recovery_reset` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-AUTH (5 failed attempts per rolling 15 minutes per client key and per account (D-68), plus 30 requests per minute) |
| Security considerations | No information about whether a code exists. Revokes every session. The new recovery code is returned once. |
| PRD requirements | FR-AUTH-001 |

#### AUTH-10 POST `/api/v1/system/recovery/reconcile`

| Property | Value |
| --- | --- |
| Purpose | After a detected restore, run the reconciliation that invalidates old authority and starts a new recovery generation. Needs the account password. |
| Authentication | None (public route) |
| Authorization | Public route. Host allow-list, Origin allow-list for state changes, rate limit. State precondition: recovery_required state |
| Re-authentication | No |
| CSRF | Yes (anonymous pre-session token) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_anon` (from GET /session), `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | none |
| Query parameters | none |
| Request schema | `RecoveryReconcileRequest` |
| Response schema | `RecoveryReconcileResult` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `invalid_credentials`, 403 `csrf_invalid`, 403 `host_denied`, 403 `origin_denied`, 409 `conflict_state`, 409 `recovery_not_required`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable` |
| Idempotency | State-guarded. A repeat after the transition returns `conflict_state` and changes nothing. |
| Audit event | `system.recovery_reconciled` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-AUTH (5 failed attempts per rolling 15 minutes per client key and per account (D-68), plus 30 requests per minute) |
| Security considerations | Public route. Throttled before any hashing work. |
| PRD requirements | FR-AUTH-001, FR-APPROVAL-003, FR-SCOPE-006 |

#### AUTH-09 POST `/api/v1/account/password`

| Property | Value |
| --- | --- |
| Purpose | Change the password. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; account-level resource; precondition: Grant purpose password_change |
| Re-authentication | Yes, purpose `password_change`, always |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | none |
| Query parameters | none |
| Request schema | `ChangePasswordRequest` |
| Response schema | empty |
| Success status | 204 |
| Error responses | 400 `invalid_request`, 401 `invalid_credentials`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 403 `reauth_invalid`, 403 `reauth_required`, 409 `conflict_state`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | State-guarded. A repeat after the transition returns `conflict_state` and changes nothing. |
| Audit event | `auth.password_changed` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The re-authentication grant is consumed in the same transaction as the guarded change. |
| PRD requirements | FR-AUTH-001 |

## Security policy

#### POL-01 GET `/api/v1/policy/settings`

| Property | Value |
| --- | --- |
| Purpose | List the current security policy values. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; account-level resource |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | none |
| Query parameters | `limit`, `cursor`, `sort` |
| Request schema | none |
| Response schema | `PolicySettingList` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-POLICY-002 |

#### POL-02 PUT `/api/v1/policy/settings/{settingKey}`

| Property | Value |
| --- | --- |
| Purpose | Change one policy value as a new version. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; account-level resource; precondition: Widening changes need a grant; narrowing does not |
| Re-authentication | Yes, purpose `policy_widening`, only when the change widens what can be tested or sent |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `settingKey` |
| Query parameters | none |
| Request schema | `PolicySettingChange` |
| Response schema | `PolicySetting` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 403 `reauth_invalid`, 403 `reauth_required`, 404 `not_found`, 409 `conflict_state`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `policy_value_invalid`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | `policy.changed` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | Every change creates a new version with the old value retained. A change that widens what can be tested or sent consumes a re-authentication grant (D-53). |
| PRD requirements | FR-POLICY-002 |

## Programs

#### PRG-01 POST `/api/v1/programs`

| Property | Value |
| --- | --- |
| Purpose | Register a program. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; account-level resource |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json`, `Idempotency-Key` (required) |
| Path parameters | none |
| Query parameters | none |
| Request schema | `ProgramCreate` |
| Response schema | `Program` |
| Success status | 201 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 409 `idempotency_conflict`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Idempotency-Key required. A repeat with the same key and body returns the original result with `Idempotency-Replayed: true`. The same key with another body returns `idempotency_conflict`. |
| Audit event | `program.created` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-PROG-001 |

#### PRG-02 GET `/api/v1/programs`

| Property | Value |
| --- | --- |
| Purpose | List programs. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; account-level resource |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | none |
| Query parameters | `limit`, `cursor`, `sort`, `status` |
| Request schema | none |
| Response schema | `ProgramList` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-PROG-001 |

#### PRG-03 GET `/api/v1/programs/{programId}`

| Property | Value |
| --- | --- |
| Purpose | Get a program. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; resource resolved by ID, its program owned by the session user (programs.owner_user_id) |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `programId` |
| Query parameters | none |
| Request schema | none |
| Response schema | `Program` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-PROG-001 |

#### PRG-04 PATCH `/api/v1/programs/{programId}`

| Property | Value |
| --- | --- |
| Purpose | Update name, label or description. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; resource resolved by ID, its program owned by the session user (programs.owner_user_id); program active (else program_archived); precondition: Program active; If-Match version |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json`, `If-Match: "<version>"` (required) |
| Path parameters | `programId` |
| Query parameters | none |
| Request schema | `ProgramUpdate` |
| Response schema | `Program` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `program_archived`, 409 `version_conflict`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | `program.updated` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-PROG-001 |

#### PRG-05 POST `/api/v1/programs/{programId}/archive`

| Property | Value |
| --- | --- |
| Purpose | Archive a program (read-only afterwards). |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; resource resolved by ID, its program owned by the session user (programs.owner_user_id); program active (else program_archived); precondition: No assessment running or paused |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `programId` |
| Query parameters | none |
| Request schema | `ArchiveRequest` |
| Response schema | `Program` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `program_archived`, 409 `program_has_active_assessments`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | State-guarded. A repeat after the transition returns `conflict_state` and changes nothing. |
| Audit event | `program.archived` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-PROG-001 |

#### PRG-06 POST `/api/v1/programs/{programId}/rules-versions`

| Property | Value |
| --- | --- |
| Purpose | Record a new version of the program rules reference. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; resource resolved by ID, its program owned by the session user (programs.owner_user_id); program active (else program_archived); precondition: Program active |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json`, `Idempotency-Key` (required) |
| Path parameters | `programId` |
| Query parameters | none |
| Request schema | `RulesVersionCreate` |
| Response schema | `RulesVersion` |
| Success status | 201 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `idempotency_conflict`, 409 `program_archived`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Idempotency-Key required. A repeat with the same key and body returns the original result with `Idempotency-Replayed: true`. The same key with another body returns `idempotency_conflict`. |
| Audit event | `program.rules_recorded` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-PROG-002 |

#### PRG-07 GET `/api/v1/programs/{programId}/rules-versions`

| Property | Value |
| --- | --- |
| Purpose | List rules reference versions. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; resource resolved by ID, its program owned by the session user (programs.owner_user_id) |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `programId` |
| Query parameters | `limit`, `cursor`, `sort` |
| Request schema | none |
| Response schema | `RulesVersionList` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-PROG-002 |

## Targets, scope, labs and target authorization

#### TGT-01 POST `/api/v1/programs/{programId}/targets`

| Property | Value |
| --- | --- |
| Purpose | Register a target (registration is not authorization). |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; resource resolved by ID, its program owned by the session user (programs.owner_user_id); program active (else program_archived); precondition: Program active |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json`, `Idempotency-Key` (required) |
| Path parameters | `programId` |
| Query parameters | none |
| Request schema | `TargetCreate` |
| Response schema | `Target` |
| Success status | 201 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `idempotency_conflict`, 409 `program_archived`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `target_ambiguous`, 422 `target_invalid`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Idempotency-Key required. A repeat with the same key and body returns the original result with `Idempotency-Replayed: true`. The same key with another body returns `idempotency_conflict`. |
| Audit event | `target.registered` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-SCOPE-001 |

#### TGT-02 GET `/api/v1/programs/{programId}/targets`

| Property | Value |
| --- | --- |
| Purpose | List targets. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; resource resolved by ID, its program owned by the session user (programs.owner_user_id) |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `programId` |
| Query parameters | `limit`, `cursor`, `sort`, `status`, `kind` |
| Request schema | none |
| Response schema | `TargetList` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-SCOPE-001 |

#### TGT-03 GET `/api/v1/targets/{targetId}`

| Property | Value |
| --- | --- |
| Purpose | Get a target. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; resource resolved by ID, its program owned by the session user (programs.owner_user_id) |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `targetId` |
| Query parameters | none |
| Request schema | none |
| Response schema | `Target` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-SCOPE-001 |

#### TGT-04 PATCH `/api/v1/targets/{targetId}`

| Property | Value |
| --- | --- |
| Purpose | Change the label. The canonical value is immutable. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; resource resolved by ID, its program owned by the session user (programs.owner_user_id); program active (else program_archived); precondition: Program active; If-Match version |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json`, `If-Match: "<version>"` (required) |
| Path parameters | `targetId` |
| Query parameters | none |
| Request schema | `TargetUpdate` |
| Response schema | `Target` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `program_archived`, 409 `version_conflict`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | `target.updated` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-SCOPE-001 |

#### TGT-05 POST `/api/v1/targets/{targetId}/archive`

| Property | Value |
| --- | --- |
| Purpose | Archive a target and invalidate dependent validations. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; resource resolved by ID, its program owned by the session user (programs.owner_user_id); program active (else program_archived); precondition: Not in a running assessment |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `targetId` |
| Query parameters | none |
| Request schema | `ArchiveRequest` |
| Response schema | `Target` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `program_archived`, 409 `target_in_use`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | State-guarded. A repeat after the transition returns `conflict_state` and changes nothing. |
| Audit event | `target.archived` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-SCOPE-006 |

#### SCP-01 GET `/api/v1/programs/{programId}/scope`

| Property | Value |
| --- | --- |
| Purpose | Get the current scope version with its rules. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; resource resolved by ID, its program owned by the session user (programs.owner_user_id) |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `programId` |
| Query parameters | none |
| Request schema | none |
| Response schema | `ScopeVersionDetail` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-SCOPE-001 |

#### SCP-02 POST `/api/v1/programs/{programId}/scope-versions`

| Property | Value |
| --- | --- |
| Purpose | Publish a new scope version from a full rule list. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; resource resolved by ID, its program owned by the session user (programs.owner_user_id); program active (else program_archived); precondition: Program active; grant required only when the change widens |
| Re-authentication | Yes, purpose `scope_widening`, only when the new version widens scope |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json`, `Idempotency-Key` (required) |
| Path parameters | `programId` |
| Query parameters | none |
| Request schema | `ScopeVersionCreate` |
| Response schema | `ScopePublishResult` |
| Success status | 201 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 403 `reauth_invalid`, 403 `reauth_required`, 404 `not_found`, 409 `conflict_state`, 409 `idempotency_conflict`, 409 `program_archived`, 409 `version_conflict`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `scope_rule_refused`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Idempotency-Key required. A repeat with the same key and body returns the original result with `Idempotency-Replayed: true`. The same key with another body returns `idempotency_conflict`. |
| Audit event | `scope.version_published` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The submitted rules are parsed by the authoritative scope engine only. A rule outside the grammar is refused and an unsupported exclusion becomes a refused-exclusion row, never dropped (D-64 choice 10). Wildcard inclusion is refused (D-73). |
| PRD requirements | FR-SCOPE-001, FR-SCOPE-006 |

#### SCP-03 GET `/api/v1/programs/{programId}/scope-versions`

| Property | Value |
| --- | --- |
| Purpose | List scope versions. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; resource resolved by ID, its program owned by the session user (programs.owner_user_id) |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `programId` |
| Query parameters | `limit`, `cursor`, `sort` |
| Request schema | none |
| Response schema | `ScopeVersionList` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-SCOPE-001 |

#### SCP-04 GET `/api/v1/scope-versions/{scopeVersionId}`

| Property | Value |
| --- | --- |
| Purpose | Get one scope version with its rules. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; resource resolved by ID, its program owned by the session user (programs.owner_user_id) |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `scopeVersionId` |
| Query parameters | none |
| Request schema | none |
| Response schema | `ScopeVersionDetail` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-SCOPE-001 |

#### SCP-05 POST `/api/v1/programs/{programId}/scope-evaluations`

| Property | Value |
| --- | --- |
| Purpose | Preview the scope decision for names or literals. Nothing is resolved, stored or authorized. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; resource resolved by ID, its program owned by the session user (programs.owner_user_id) |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `programId` |
| Query parameters | none |
| Request schema | `ScopePreviewRequest` |
| Response schema | `ScopePreviewResult` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-PREVIEW (30 per minute per session) |
| Security considerations | Evaluates names and literals only. No DNS lookup, no network traffic, nothing stored. The result is a preview and never an authorization. |
| PRD requirements | FR-SCOPE-002 |

#### SCP-06 GET `/api/v1/programs/{programId}/refused-exclusions`

| Property | Value |
| --- | --- |
| Purpose | List refused exclusions. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; resource resolved by ID, its program owned by the session user (programs.owner_user_id) |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `programId` |
| Query parameters | `limit`, `cursor`, `sort`, `status` |
| Request schema | none |
| Response schema | `RefusedExclusionList` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-SCOPE-001 |

#### SCP-07 POST `/api/v1/refused-exclusions/{refusedExclusionId}/acknowledge`

| Property | Value |
| --- | --- |
| Purpose | Acknowledge a refused exclusion, bound to the current target-set hash. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; resource resolved by ID, its program owned by the session user (programs.owner_user_id); program active (else program_archived); precondition: Status open |
| Re-authentication | Yes, purpose `exclusion_acknowledgement`, always |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `refusedExclusionId` |
| Query parameters | none |
| Request schema | `ExclusionAcknowledge` |
| Response schema | `RefusedExclusion` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 403 `reauth_invalid`, 403 `reauth_required`, 404 `not_found`, 409 `conflict_state`, 409 `program_archived`, 409 `target_set_hash_mismatch`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | State-guarded. A repeat after the transition returns `conflict_state` and changes nothing. |
| Audit event | `scope.exclusion_acknowledged` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The re-authentication grant is consumed in the same transaction as the guarded change. |
| PRD requirements | FR-SCOPE-001 |

#### LAB-01 POST `/api/v1/assessments/{assessmentId}/lab-registrations`

| Property | Value |
| --- | --- |
| Purpose | Register a lab target for one assessment. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; program active (else program_archived); precondition: Assessment draft or ready; matching in-scope rule; no exclusion; forbidden classes refused |
| Re-authentication | Yes, purpose `lab_registration`, always |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json`, `Idempotency-Key` (required) |
| Path parameters | `assessmentId` |
| Query parameters | none |
| Request schema | `LabRegistrationCreate` |
| Response schema | `LabRegistration` |
| Success status | 201 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 403 `reauth_invalid`, 403 `reauth_required`, 404 `not_found`, 409 `conflict_state`, 409 `idempotency_conflict`, 409 `program_archived`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `lab_ports_required`, 422 `lab_range_forbidden`, 422 `target_not_in_scope`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Idempotency-Key required. A repeat with the same key and body returns the original result with `Idempotency-Replayed: true`. The same key with another body returns `idempotency_conflict`. |
| Audit event | `lab.registered` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | Forbidden address classes and the metadata list are refused (D-47, D-72). The registration binds one assessment, one target and an expiry. |
| PRD requirements | FR-SCOPE-004 |

#### LAB-02 GET `/api/v1/assessments/{assessmentId}/lab-registrations`

| Property | Value |
| --- | --- |
| Purpose | List lab registrations. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `assessmentId` |
| Query parameters | `limit`, `cursor`, `sort`, `status` |
| Request schema | none |
| Response schema | `LabRegistrationList` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-SCOPE-004 |

#### LAB-03 GET `/api/v1/lab-registrations/{labRegistrationId}`

| Property | Value |
| --- | --- |
| Purpose | Get a lab registration. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `labRegistrationId` |
| Query parameters | none |
| Request schema | none |
| Response schema | `LabRegistration` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-SCOPE-004 |

#### LAB-04 POST `/api/v1/lab-registrations/{labRegistrationId}/revoke`

| Property | Value |
| --- | --- |
| Purpose | Revoke a lab registration. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; program active (else program_archived); precondition: Status active |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `labRegistrationId` |
| Query parameters | none |
| Request schema | `RevokeRequest` |
| Response schema | `LabRegistration` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `program_archived`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | State-guarded. A repeat after the transition returns `conflict_state` and changes nothing. |
| Audit event | `lab.revoked` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-SCOPE-004 |

#### VAL-01 POST `/api/v1/assessments/{assessmentId}/target-validations`

| Property | Value |
| --- | --- |
| Purpose | Record a target authorization validation with a human attestation. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; program active (else program_archived); precondition: Assessment draft, ready or paused; no open refused exclusion; every target allowed |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json`, `Idempotency-Key` (required) |
| Path parameters | `assessmentId` |
| Query parameters | none |
| Request schema | `TargetValidationCreate` |
| Response schema | `TargetValidation` |
| Success status | 201 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `attestation_text_outdated`, 409 `conflict_state`, 409 `exclusion_unacknowledged`, 409 `idempotency_conflict`, 409 `program_archived`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `target_not_in_scope`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Idempotency-Key required. A repeat with the same key and body returns the original result with `Idempotency-Replayed: true`. The same key with another body returns `idempotency_conflict`. |
| Audit event | `validation.recorded` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | An AI or worker cannot call it. The attestation text version must match the current wording. A lab target needs the lab wording (D-61). |
| PRD requirements | FR-SCOPE-005 |

#### VAL-02 GET `/api/v1/assessments/{assessmentId}/target-validations`

| Property | Value |
| --- | --- |
| Purpose | List validation records. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `assessmentId` |
| Query parameters | `limit`, `cursor`, `sort`, `status` |
| Request schema | none |
| Response schema | `TargetValidationList` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-SCOPE-005 |

#### VAL-03 GET `/api/v1/target-validations/{validationId}`

| Property | Value |
| --- | --- |
| Purpose | Get a validation record with per-target results. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `validationId` |
| Query parameters | none |
| Request schema | none |
| Response schema | `TargetValidation` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-SCOPE-005 |

## Assessments and plans

#### ASM-01 POST `/api/v1/programs/{programId}/assessments`

| Property | Value |
| --- | --- |
| Purpose | Create an assessment in draft. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; resource resolved by ID, its program owned by the session user (programs.owner_user_id); program active (else program_archived); precondition: Program active |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json`, `Idempotency-Key` (required) |
| Path parameters | `programId` |
| Query parameters | none |
| Request schema | `AssessmentCreate` |
| Response schema | `Assessment` |
| Success status | 201 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `idempotency_conflict`, 409 `program_archived`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Idempotency-Key required. A repeat with the same key and body returns the original result with `Idempotency-Replayed: true`. The same key with another body returns `idempotency_conflict`. |
| Audit event | `assessment.created` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-ASSESS-001 |

#### ASM-02 GET `/api/v1/programs/{programId}/assessments`

| Property | Value |
| --- | --- |
| Purpose | List assessments. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; resource resolved by ID, its program owned by the session user (programs.owner_user_id) |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `programId` |
| Query parameters | `limit`, `cursor`, `sort`, `status` |
| Request schema | none |
| Response schema | `AssessmentList` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-ASSESS-001 |

#### ASM-03 GET `/api/v1/assessments/{assessmentId}`

| Property | Value |
| --- | --- |
| Purpose | Get an assessment. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `assessmentId` |
| Query parameters | none |
| Request schema | none |
| Response schema | `Assessment` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-ASSESS-001 |

#### ASM-04 PATCH `/api/v1/assessments/{assessmentId}`

| Property | Value |
| --- | --- |
| Purpose | Update name or description while draft. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; program active (else program_archived); precondition: Status draft; If-Match version |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json`, `If-Match: "<version>"` (required) |
| Path parameters | `assessmentId` |
| Query parameters | none |
| Request schema | `AssessmentUpdate` |
| Response schema | `Assessment` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `program_archived`, 409 `version_conflict`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | `assessment.updated` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-ASSESS-001 |

#### ASM-05 PUT `/api/v1/assessments/{assessmentId}/targets`

| Property | Value |
| --- | --- |
| Purpose | Replace the target set. Invalidates the validation record. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; program active (else program_archived); precondition: Status draft, ready or paused |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `assessmentId` |
| Query parameters | none |
| Request schema | `AssessmentTargetsSet` |
| Response schema | `AssessmentTargets` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `program_archived`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `target_not_registered`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | `assessment.targets_set` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-ASSESS-001, FR-SCOPE-006 |

#### ASM-06 PUT `/api/v1/assessments/{assessmentId}/raw-evidence-capture`

| Property | Value |
| --- | --- |
| Purpose | Turn raw evidence capture on or off (D-77). Turning it on widens. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; program active (else program_archived); precondition: Status draft, ready or paused for on |
| Re-authentication | Yes, purpose `raw_evidence_capture`, only when enabling |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `assessmentId` |
| Query parameters | none |
| Request schema | `RawCaptureChange` |
| Response schema | `Assessment` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 403 `reauth_invalid`, 403 `reauth_required`, 404 `not_found`, 409 `conflict_state`, 409 `program_archived`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | `assessment.raw_capture_changed` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The re-authentication grant is consumed in the same transaction as the guarded change. |
| PRD requirements | NFR-PRIV-002 |

#### ASM-07 POST `/api/v1/assessments/{assessmentId}/ready`

| Property | Value |
| --- | --- |
| Purpose | Mark ready. Needs a current validation record. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; program active (else program_archived); precondition: Status draft; validation current |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `assessmentId` |
| Query parameters | none |
| Request schema | `TransitionRequest` |
| Response schema | `Assessment` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `exclusion_unacknowledged`, 409 `program_archived`, 409 `validation_stale`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | State-guarded. A repeat after the transition returns `conflict_state` and changes nothing. |
| Audit event | `assessment.ready` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-ASSESS-001 |

#### ASM-08 POST `/api/v1/assessments/{assessmentId}/start`

| Property | Value |
| --- | --- |
| Purpose | Start. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; program active (else program_archived); precondition: Status ready; validation current |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `assessmentId` |
| Query parameters | none |
| Request schema | `TransitionRequest` |
| Response schema | `Assessment` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `program_archived`, 409 `validation_stale`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | State-guarded. A repeat after the transition returns `conflict_state` and changes nothing. |
| Audit event | `assessment.started` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-ASSESS-001 |

#### ASM-09 POST `/api/v1/assessments/{assessmentId}/pause`

| Property | Value |
| --- | --- |
| Purpose | Pause: no new launches, queued jobs held. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; program active (else program_archived); precondition: Status running |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `assessmentId` |
| Query parameters | none |
| Request schema | `TransitionRequest` |
| Response schema | `Assessment` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `program_archived`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | State-guarded. A repeat after the transition returns `conflict_state` and changes nothing. |
| Audit event | `assessment.paused` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-ASSESS-001 |

#### ASM-10 POST `/api/v1/assessments/{assessmentId}/resume`

| Property | Value |
| --- | --- |
| Purpose | Resume from paused. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; program active (else program_archived); precondition: Status paused; validation current |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `assessmentId` |
| Query parameters | none |
| Request schema | `TransitionRequest` |
| Response schema | `Assessment` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `program_archived`, 409 `validation_stale`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | State-guarded. A repeat after the transition returns `conflict_state` and changes nothing. |
| Audit event | `assessment.resumed` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-ASSESS-001 |

#### ASM-11 POST `/api/v1/assessments/{assessmentId}/complete`

| Property | Value |
| --- | --- |
| Purpose | Complete. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; program active (else program_archived); precondition: Status running or paused; no running job |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `assessmentId` |
| Query parameters | none |
| Request schema | `TransitionRequest` |
| Response schema | `Assessment` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `jobs_still_running`, 409 `program_archived`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | State-guarded. A repeat after the transition returns `conflict_state` and changes nothing. |
| Audit event | `assessment.completed` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-ASSESS-001 |

#### ASM-12 POST `/api/v1/assessments/{assessmentId}/cancel`

| Property | Value |
| --- | --- |
| Purpose | Cancel: stop queued and running work (30 s objective). |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; program active (else program_archived); precondition: Status not terminal |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `assessmentId` |
| Query parameters | none |
| Request schema | `TransitionRequest` |
| Response schema | `Assessment` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `program_archived`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | State-guarded. A repeat after the transition returns `conflict_state` and changes nothing. |
| Audit event | `assessment.cancelled` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-ASSESS-001, FR-APPROVAL-003 |

#### ASM-13 GET `/api/v1/assessments/{assessmentId}/status`

| Property | Value |
| --- | --- |
| Purpose | Summary: state, validation currency, blockers, job counts. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `assessmentId` |
| Query parameters | none |
| Request schema | none |
| Response schema | `AssessmentStatus` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-ASSESS-001 |

#### PLN-01 POST `/api/v1/assessments/{assessmentId}/plan-items`

| Property | Value |
| --- | --- |
| Purpose | Create a human plan item. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; program active (else program_archived); precondition: Assessment not terminal |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json`, `Idempotency-Key` (required) |
| Path parameters | `assessmentId` |
| Query parameters | none |
| Request schema | `PlanItemCreate` |
| Response schema | `PlanItem` |
| Success status | 201 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `idempotency_conflict`, 409 `program_archived`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `operation_invalid`, 422 `target_not_in_assessment`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Idempotency-Key required. A repeat with the same key and body returns the original result with `Idempotency-Replayed: true`. The same key with another body returns `idempotency_conflict`. |
| Audit event | `plan_item.created` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-FINDING-004 |

#### PLN-02 GET `/api/v1/assessments/{assessmentId}/plan-items`

| Property | Value |
| --- | --- |
| Purpose | List plan items. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `assessmentId` |
| Query parameters | `limit`, `cursor`, `sort`, `status` |
| Request schema | none |
| Response schema | `PlanItemList` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-FINDING-004 |

#### PLN-03 GET `/api/v1/plan-items/{planItemId}`

| Property | Value |
| --- | --- |
| Purpose | Get a plan item with its canonical operation. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `planItemId` |
| Query parameters | none |
| Request schema | none |
| Response schema | `PlanItem` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-FINDING-004, FR-APPROVAL-005 |

#### PLN-04 POST `/api/v1/plan-items/{planItemId}/reject`

| Property | Value |
| --- | --- |
| Purpose | Reject a proposed plan item. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; program active (else program_archived); precondition: Status proposed |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `planItemId` |
| Query parameters | none |
| Request schema | `RejectRequest` |
| Response schema | `PlanItem` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `program_archived`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | State-guarded. A repeat after the transition returns `conflict_state` and changes nothing. |
| Audit event | `plan_item.rejected` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-FINDING-004 |

## Approvals

#### APR-01 POST `/api/v1/plan-items/{planItemId}/approvals`

| Property | Value |
| --- | --- |
| Purpose | Request an approval for the exact operation of a plan item. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; program active (else program_archived); precondition: Operation class low_impact or intrusive; E1 result require_approval |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json`, `Idempotency-Key` (required) |
| Path parameters | `planItemId` |
| Query parameters | none |
| Request schema | `ApprovalRequestCreate` |
| Response schema | `Approval` |
| Success status | 201 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `idempotency_conflict`, 409 `numeric_limits_missing`, 409 `operation_not_approvable`, 409 `program_archived`, 409 `validation_stale`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Idempotency-Key required. A repeat with the same key and body returns the original result with `Idempotency-Replayed: true`. The same key with another body returns `idempotency_conflict`. |
| Audit event | `approval.requested` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-APPROVAL-001, FR-APPROVAL-006 |

#### APR-02 GET `/api/v1/assessments/{assessmentId}/approvals`

| Property | Value |
| --- | --- |
| Purpose | List approvals. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `assessmentId` |
| Query parameters | `limit`, `cursor`, `sort`, `status` |
| Request schema | none |
| Response schema | `ApprovalList` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-APPROVAL-001 |

#### APR-03 GET `/api/v1/approvals/{approvalId}`

| Property | Value |
| --- | --- |
| Purpose | Get an approval with its canonical operation. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `approvalId` |
| Query parameters | none |
| Request schema | none |
| Response schema | `Approval` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-APPROVAL-001, FR-APPROVAL-005 |

#### APR-04 POST `/api/v1/approvals/{approvalId}/approve`

| Property | Value |
| --- | --- |
| Purpose | Approve exactly the shown operation. The body echoes the operation hash and destinations hash. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; program active (else program_archived); precondition: Status pending; validation current; hash echo equals stored hash |
| Re-authentication | Yes, purpose `approval_intrusive`, only for the intrusive class |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `approvalId` |
| Query parameters | none |
| Request schema | `ApprovalApprove` |
| Response schema | `Approval` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 403 `reauth_invalid`, 403 `reauth_required`, 404 `not_found`, 409 `conflict_state`, 409 `operation_hash_mismatch`, 409 `program_archived`, 409 `validation_stale`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | State-guarded. A repeat after the transition returns `conflict_state` and changes nothing. |
| Audit event | `approval.approved` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The body echoes the operation hash and the declared-destinations hash that the screen showed. The server compares them with the stored canonical operation. Only a human session can call it. No API token or AI path exists (D-60). |
| PRD requirements | FR-APPROVAL-001, FR-APPROVAL-005 |

#### APR-05 POST `/api/v1/approvals/{approvalId}/reject`

| Property | Value |
| --- | --- |
| Purpose | Reject a pending approval. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; program active (else program_archived); precondition: Status pending |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `approvalId` |
| Query parameters | none |
| Request schema | `RejectRequest` |
| Response schema | `Approval` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `program_archived`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | State-guarded. A repeat after the transition returns `conflict_state` and changes nothing. |
| Audit event | `approval.rejected` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-APPROVAL-001 |

#### APR-06 POST `/api/v1/approvals/{approvalId}/revoke`

| Property | Value |
| --- | --- |
| Purpose | Revoke an approval; dependent jobs get stop_requested. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; program active (else program_archived); precondition: Status approved |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `approvalId` |
| Query parameters | none |
| Request schema | `RevokeRequest` |
| Response schema | `Approval` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `program_archived`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | State-guarded. A repeat after the transition returns `conflict_state` and changes nothing. |
| Audit event | `approval.revoked` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-APPROVAL-003 |

#### APR-07 GET `/api/v1/approvals/{approvalId}/events`

| Property | Value |
| --- | --- |
| Purpose | Approval history. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `approvalId` |
| Query parameters | `limit`, `cursor`, `sort` |
| Request schema | none |
| Response schema | `ApprovalEventList` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-APPROVAL-001 |

## Jobs

#### JOB-01 POST `/api/v1/plan-items/{planItemId}/jobs`

| Property | Value |
| --- | --- |
| Purpose | Create a job from an accepted plan item (E1). |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; program active (else program_archived); precondition: Plan item proposed or accepted; for risky classes an approved, unexpired approval for the same operation hash |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json`, `Idempotency-Key` (required) |
| Path parameters | `planItemId` |
| Query parameters | none |
| Request schema | `JobCreate` |
| Response schema | `Job` |
| Success status | 201 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 403 `policy_denied`, 404 `not_found`, 409 `approval_exhausted`, 409 `approval_expired`, 409 `approval_required`, 409 `approval_revoked`, 409 `conflict_state`, 409 `idempotency_conflict`, 409 `program_archived`, 409 `validation_stale`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Idempotency-Key required. A repeat with the same key and body returns the original result with `Idempotency-Replayed: true`. The same key with another body returns `idempotency_conflict`. |
| Audit event | `job.created` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | Creating a job is not authorization to run it. The worker re-evaluates at claim (E2) and before launch (E3). The approval use is consumed at claim, not here (D-30). |
| PRD requirements | FR-JOB-001, FR-POLICY-001 |

#### JOB-02 GET `/api/v1/assessments/{assessmentId}/jobs`

| Property | Value |
| --- | --- |
| Purpose | List jobs. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `assessmentId` |
| Query parameters | `limit`, `cursor`, `sort`, `state`, `jobKind` |
| Request schema | none |
| Response schema | `JobList` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-JOB-001 |

#### JOB-03 GET `/api/v1/jobs/{jobId}`

| Property | Value |
| --- | --- |
| Purpose | Get a job. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `jobId` |
| Query parameters | none |
| Request schema | none |
| Response schema | `Job` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-JOB-001 |

#### JOB-04 POST `/api/v1/jobs/{jobId}/cancel`

| Property | Value |
| --- | --- |
| Purpose | Request cancellation. The database records intent, the worker stops the process. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; program active (else program_archived); precondition: State queued, awaiting_approval or running |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `jobId` |
| Query parameters | none |
| Request schema | `RejectRequest` |
| Response schema | `Job` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `program_archived`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | State-guarded. A repeat after the transition returns `conflict_state` and changes nothing. |
| Audit event | `job.cancel_requested` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-JOB-001, FR-APPROVAL-003 |

#### JOB-05 GET `/api/v1/jobs/{jobId}/attempts`

| Property | Value |
| --- | --- |
| Purpose | Execution attempts with provenance. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `jobId` |
| Query parameters | `limit`, `cursor`, `sort` |
| Request schema | none |
| Response schema | `JobAttemptList` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-JOB-001 |

#### JOB-06 GET `/api/v1/jobs/{jobId}/transitions`

| Property | Value |
| --- | --- |
| Purpose | State and control history. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `jobId` |
| Query parameters | `limit`, `cursor`, `sort` |
| Request schema | none |
| Response schema | `JobTransitionList` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-JOB-001 |

## Assets and endpoints

#### AST-01 GET `/api/v1/programs/{programId}/assets`

| Property | Value |
| --- | --- |
| Purpose | List assets with scope decision and staleness. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; resource resolved by ID, its program owned by the session user (programs.owner_user_id) |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `programId` |
| Query parameters | `limit`, `cursor`, `sort`, `scopeDecision`, `kind`, `q` |
| Request schema | none |
| Response schema | `AssetList` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-ASSET-001 |

#### AST-02 GET `/api/v1/assets/{assetId}`

| Property | Value |
| --- | --- |
| Purpose | Get an asset. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; resource resolved by ID, its program owned by the session user (programs.owner_user_id) |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `assetId` |
| Query parameters | none |
| Request schema | none |
| Response schema | `Asset` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-ASSET-001 |

#### AST-03 GET `/api/v1/programs/{programId}/endpoints`

| Property | Value |
| --- | --- |
| Purpose | List endpoints with parameters. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; resource resolved by ID, its program owned by the session user (programs.owner_user_id) |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `programId` |
| Query parameters | `limit`, `cursor`, `sort`, `q` |
| Request schema | none |
| Response schema | `EndpointList` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-ASSET-002 |

#### AST-04 GET `/api/v1/endpoints/{endpointId}`

| Property | Value |
| --- | --- |
| Purpose | Get an endpoint. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; resource resolved by ID, its program owned by the session user (programs.owner_user_id) |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `endpointId` |
| Query parameters | none |
| Request schema | none |
| Response schema | `Endpoint` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-ASSET-002 |

#### AST-05 GET `/api/v1/assets/{assetId}/observations`

| Property | Value |
| --- | --- |
| Purpose | Discovery provenance of an asset. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; resource resolved by ID, its program owned by the session user (programs.owner_user_id) |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `assetId` |
| Query parameters | `limit`, `cursor`, `sort`, `observationKind` |
| Request schema | none |
| Response schema | `ObservationList` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-ASSET-001 |

#### AST-06 POST `/api/v1/assets/{assetId}/promote`

| Property | Value |
| --- | --- |
| Purpose | Promote an in-scope asset to a registered target (E4). |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; resource resolved by ID, its program owned by the session user (programs.owner_user_id); program active (else program_archived); precondition: Program active; scope decision in_scope under the current version |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `assetId` |
| Query parameters | none |
| Request schema | `PromoteRequest` |
| Response schema | `Target` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `asset_not_in_scope`, 409 `conflict_state`, 409 `program_archived`, 409 `scope_decision_stale`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | State-guarded. A repeat after the transition returns `conflict_state` and changes nothing. |
| Audit event | `asset.promoted` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | Promotion re-evaluates the asset under the current scope version. A discovery is never authorization (E4). |
| PRD requirements | FR-ASSET-001, FR-SCOPE-002 |

## Evidence

#### EVD-01 GET `/api/v1/assessments/{assessmentId}/evidence`

| Property | Value |
| --- | --- |
| Purpose | List evidence metadata. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `assessmentId` |
| Query parameters | `limit`, `cursor`, `sort`, `kind`, `integrityState`, `jobId` |
| Request schema | none |
| Response schema | `EvidenceList` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-EVIDENCE-001 |

#### EVD-02 GET `/api/v1/evidence/{evidenceId}`

| Property | Value |
| --- | --- |
| Purpose | Get evidence metadata and integrity state. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `evidenceId` |
| Query parameters | none |
| Request schema | none |
| Response schema | `Evidence` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-EVIDENCE-001, FR-EVIDENCE-002 |

#### EVD-03 POST `/api/v1/evidence/{evidenceId}/integrity-checks`

| Property | Value |
| --- | --- |
| Purpose | Re-hash the stored content and record the result. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; program active (else program_archived); precondition: Content not purged |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token` |
| Path parameters | `evidenceId` |
| Query parameters | none |
| Request schema | none |
| Response schema | `EvidenceIntegrityCheck` |
| Success status | 201 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `program_archived`, 410 `evidence_purged`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | `evidence.integrity_checked` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-EVIDENCE-002 |

#### EVD-04 GET `/api/v1/evidence/{evidenceId}/integrity-checks`

| Property | Value |
| --- | --- |
| Purpose | Integrity check history. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `evidenceId` |
| Query parameters | `limit`, `cursor`, `sort` |
| Request schema | none |
| Response schema | `EvidenceIntegrityCheckList` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-EVIDENCE-002 |

#### EVD-05 GET `/api/v1/evidence/{evidenceId}/content`

| Property | Value |
| --- | --- |
| Purpose | Download the stored bytes as an attachment. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; precondition: Integrity state not compromised or missing, unless the human adds acknowledgeIntegrity=true; content not purged |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `evidenceId` |
| Query parameters | `acknowledgeIntegrity` (boolean, default false) |
| Request schema | none |
| Response schema | binary attachment |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `evidence_integrity_failed`, 410 `evidence_purged`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | `evidence.accessed` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-DOWNLOAD (30 per minute per session) |
| Security considerations | Streams stored bytes with attachment, nosniff and no-store headers. Re-hashes while streaming. The audit event is written before the first byte. Content is untrusted and is never rendered as HTML. |
| PRD requirements | FR-EVIDENCE-003 |

## Findings

#### FND-01 POST `/api/v1/assessments/{assessmentId}/findings`

| Property | Value |
| --- | --- |
| Purpose | Record a human-created candidate finding. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; program active (else program_archived); precondition: Assessment not terminal; asset or endpoint in the same program |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json`, `Idempotency-Key` (required) |
| Path parameters | `assessmentId` |
| Query parameters | none |
| Request schema | `FindingCreate` |
| Response schema | `Finding` |
| Success status | 201 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `duplicate_finding`, 409 `idempotency_conflict`, 409 `program_archived`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Idempotency-Key required. A repeat with the same key and body returns the original result with `Idempotency-Replayed: true`. The same key with another body returns `idempotency_conflict`. |
| Audit event | `finding.created` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-FINDING-001 |

#### FND-02 GET `/api/v1/assessments/{assessmentId}/findings`

| Property | Value |
| --- | --- |
| Purpose | List findings. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `assessmentId` |
| Query parameters | `limit`, `cursor`, `sort`, `status`, `origin` |
| Request schema | none |
| Response schema | `FindingList` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-FINDING-001 |

#### FND-03 GET `/api/v1/findings/{findingId}`

| Property | Value |
| --- | --- |
| Purpose | Get a finding with its verification record. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `findingId` |
| Query parameters | none |
| Request schema | none |
| Response schema | `Finding` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-FINDING-001 |

#### FND-04 POST `/api/v1/findings/{findingId}/submit-for-review`

| Property | Value |
| --- | --- |
| Purpose | Move a candidate to Needs Review (a human may do what automation may). |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; program active (else program_archived); precondition: Status candidate |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `findingId` |
| Query parameters | none |
| Request schema | `TransitionRequest` |
| Response schema | `Finding` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `program_archived`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | State-guarded. A repeat after the transition returns `conflict_state` and changes nothing. |
| Audit event | `finding.submitted` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-FINDING-001 |

#### FND-05 POST `/api/v1/findings/{findingId}/review-outcome`

| Property | Value |
| --- | --- |
| Purpose | Record the human review outcome. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; program active (else program_archived); precondition: Status needs_review |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `findingId` |
| Query parameters | none |
| Request schema | `ReviewOutcome` |
| Response schema | `Finding` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `program_archived`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | State-guarded. A repeat after the transition returns `conflict_state` and changes nothing. |
| Audit event | `finding.review_outcome` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-FINDING-001, FR-FINDING-002 |

#### FND-06 POST `/api/v1/findings/{findingId}/verify`

| Property | Value |
| --- | --- |
| Purpose | Human verification. Never available to automation. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; program active (else program_archived); precondition: Status awaiting_human_verification; validation record of this finding; evidence passes integrity check |
| Re-authentication | Yes, purpose `verification`, only when policy verification_requires_reauth is on (default off, D-75) |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `findingId` |
| Query parameters | none |
| Request schema | `FindingVerify` |
| Response schema | `Finding` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 403 `reauth_invalid`, 403 `reauth_required`, 404 `not_found`, 409 `conflict_state`, 409 `evidence_integrity_failed`, 409 `evidence_required`, 409 `program_archived`, 409 `validation_required`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | State-guarded. A repeat after the transition returns `conflict_state` and changes nothing. |
| Audit event | `finding.verified` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | Human sessions only. The verification stores the user, the validation record and the evidence hashes. A recommendation from automation is never a verification (SEC 5). |
| PRD requirements | FR-FINDING-003 |

#### FND-07 POST `/api/v1/findings/{findingId}/reopen`

| Property | Value |
| --- | --- |
| Purpose | Reopen any outcome or a verification to Needs Review. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; program active (else program_archived); precondition: Status not candidate or needs_review |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `findingId` |
| Query parameters | none |
| Request schema | `ReopenRequest` |
| Response schema | `Finding` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `program_archived`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | State-guarded. A repeat after the transition returns `conflict_state` and changes nothing. |
| Audit event | `finding.reopened` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-FINDING-001 |

#### FND-08 GET `/api/v1/findings/{findingId}/transitions`

| Property | Value |
| --- | --- |
| Purpose | Finding state history. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `findingId` |
| Query parameters | `limit`, `cursor`, `sort` |
| Request schema | none |
| Response schema | `FindingTransitionList` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-FINDING-001 |

#### FND-09 POST `/api/v1/findings/{findingId}/evidence`

| Property | Value |
| --- | --- |
| Purpose | Link evidence to a finding. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; program active (else program_archived); precondition: Evidence in the same program |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `findingId` |
| Query parameters | none |
| Request schema | `FindingEvidenceLink` |
| Response schema | `FindingEvidence` |
| Success status | 201 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `program_archived`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | `finding.evidence_linked` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-FINDING-002 |

#### FND-10 GET `/api/v1/findings/{findingId}/evidence`

| Property | Value |
| --- | --- |
| Purpose | List linked evidence. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `findingId` |
| Query parameters | `limit`, `cursor`, `sort` |
| Request schema | none |
| Response schema | `FindingEvidenceList` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-FINDING-002 |

#### FND-11 POST `/api/v1/findings/{findingId}/validations`

| Property | Value |
| --- | --- |
| Purpose | Record a human validation result. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; program active (else program_archived); precondition: Status needs_review or awaiting_human_verification |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json`, `Idempotency-Key` (required) |
| Path parameters | `findingId` |
| Query parameters | none |
| Request schema | `ValidationCreate` |
| Response schema | `ValidationRecord` |
| Success status | 201 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `idempotency_conflict`, 409 `program_archived`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Idempotency-Key required. A repeat with the same key and body returns the original result with `Idempotency-Replayed: true`. The same key with another body returns `idempotency_conflict`. |
| Audit event | `finding.validation_recorded` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-FINDING-002 |

#### FND-12 GET `/api/v1/findings/{findingId}/validations`

| Property | Value |
| --- | --- |
| Purpose | List validation records and recommendations. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `findingId` |
| Query parameters | `limit`, `cursor`, `sort`, `result` |
| Request schema | none |
| Response schema | `ValidationRecordList` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-FINDING-002 |

## Reports

#### RPT-01 POST `/api/v1/assessments/{assessmentId}/reports`

| Property | Value |
| --- | --- |
| Purpose | Create a draft report. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; program active (else program_archived); precondition: Assessment not cancelled |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json`, `Idempotency-Key` (required) |
| Path parameters | `assessmentId` |
| Query parameters | none |
| Request schema | `ReportCreate` |
| Response schema | `Report` |
| Success status | 201 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `idempotency_conflict`, 409 `program_archived`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Idempotency-Key required. A repeat with the same key and body returns the original result with `Idempotency-Replayed: true`. The same key with another body returns `idempotency_conflict`. |
| Audit event | `report.created` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-REPORT-001 |

#### RPT-02 GET `/api/v1/assessments/{assessmentId}/reports`

| Property | Value |
| --- | --- |
| Purpose | List reports. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `assessmentId` |
| Query parameters | `limit`, `cursor`, `sort`, `state` |
| Request schema | none |
| Response schema | `ReportList` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-REPORT-001 |

#### RPT-03 GET `/api/v1/reports/{reportId}`

| Property | Value |
| --- | --- |
| Purpose | Get a report with its findings. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `reportId` |
| Query parameters | none |
| Request schema | none |
| Response schema | `Report` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-REPORT-001 |

#### RPT-04 PUT `/api/v1/reports/{reportId}/findings`

| Property | Value |
| --- | --- |
| Purpose | Set the included findings and their sections. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; program active (else program_archived); precondition: Report draft; confirmed section only for verified findings |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `reportId` |
| Query parameters | none |
| Request schema | `ReportFindingsSet` |
| Response schema | `Report` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `finding_not_verified`, 409 `program_archived`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | `report.findings_set` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-REPORT-001, FR-REPORT-002 |

#### RPT-05 POST `/api/v1/reports/{reportId}/generate`

| Property | Value |
| --- | --- |
| Purpose | Generate the Markdown file from the included findings. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; program active (else program_archived); precondition: Report draft |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `reportId` |
| Query parameters | none |
| Request schema | `TransitionRequest` |
| Response schema | `Report` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `program_archived`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | State-guarded. A repeat after the transition returns `conflict_state` and changes nothing. |
| Audit event | `report.generated` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-REPORT-001, FR-REPORT-003 |

#### RPT-06 POST `/api/v1/reports/{reportId}/finalize`

| Property | Value |
| --- | --- |
| Purpose | Finalize a generated report. Immutable afterwards. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; program active (else program_archived); precondition: Report draft with a generated file |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `reportId` |
| Query parameters | none |
| Request schema | `TransitionRequest` |
| Response schema | `Report` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `program_archived`, 409 `report_not_generated`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | State-guarded. A repeat after the transition returns `conflict_state` and changes nothing. |
| Audit event | `report.finalized` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-REPORT-002 |

#### RPT-07 GET `/api/v1/reports/{reportId}/content`

| Property | Value |
| --- | --- |
| Purpose | Download the report as an attachment (export). |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; precondition: Report generated |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `reportId` |
| Query parameters | none |
| Request schema | none |
| Response schema | binary attachment |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `report_not_generated`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | `report.exported` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-DOWNLOAD (30 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-REPORT-003 |

## AI providers and tasks

#### AI-02 POST `/api/v1/ai/providers`

| Property | Value |
| --- | --- |
| Purpose | Configure a provider. The secret is accepted once and goes to the Secret Service. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; account-level resource; precondition: Secret Service available |
| Re-authentication | Yes, purpose `provider_enablement`, always |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json`, `Idempotency-Key` (required) |
| Path parameters | none |
| Query parameters | none |
| Request schema | `AiProviderCreate` |
| Response schema | `AiProvider` |
| Success status | 201 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 403 `reauth_invalid`, 403 `reauth_required`, 409 `conflict_state`, 409 `idempotency_conflict`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `endpoint_invalid`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `credential_store_unavailable`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Idempotency-Key required. A repeat with the same key and body returns the original result with `Idempotency-Replayed: true`. The same key with another body returns `idempotency_conflict`. |
| Audit event | `ai.provider_configured` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The secret is accepted once, sent to the Secret Service, and never stored in PostgreSQL or returned. If the Secret Service is unavailable the request fails and nothing is stored (D-52). The endpoint URL may not contain userinfo. |
| PRD requirements | FR-AI-001, FR-CRED-001 |

#### AI-03 GET `/api/v1/ai/providers`

| Property | Value |
| --- | --- |
| Purpose | List providers. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; account-level resource |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | none |
| Query parameters | `limit`, `cursor`, `sort`, `enabled` |
| Request schema | none |
| Response schema | `AiProviderList` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-AI-001 |

#### AI-04 GET `/api/v1/ai/providers/{providerId}`

| Property | Value |
| --- | --- |
| Purpose | Get a provider. No secret is returned. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; account-level resource |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `providerId` |
| Query parameters | none |
| Request schema | none |
| Response schema | `AiProvider` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-AI-001 |

#### AI-05 PATCH `/api/v1/ai/providers/{providerId}`

| Property | Value |
| --- | --- |
| Purpose | Update name or model. Endpoint changes need a new provider. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; account-level resource; precondition: If-Match version |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json`, `If-Match: "<version>"` (required) |
| Path parameters | `providerId` |
| Query parameters | none |
| Request schema | `AiProviderUpdate` |
| Response schema | `AiProvider` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `version_conflict`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | `ai.provider_updated` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-AI-001 |

#### AI-06 POST `/api/v1/ai/providers/{providerId}/tests`

| Property | Value |
| --- | --- |
| Purpose | Test connectivity with a fixed synthetic prompt. No program or target data is sent. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; account-level resource |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token` |
| Path parameters | `providerId` |
| Query parameters | none |
| Request schema | none |
| Response schema | `IntegrationCheck` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 502 `provider_unreachable`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | `ai.provider_tested` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-AITEST (6 per minute per session) |
| Security considerations | Sends only a fixed synthetic prompt. No program, target or evidence data leaves the machine. The result is classified by the connected address. |
| PRD requirements | FR-AI-001 |

#### AI-07 POST `/api/v1/programs/{programId}/ai-providers/{providerId}/enable`

| Property | Value |
| --- | --- |
| Purpose | Enable a provider for a program after showing what will be sent. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; resource resolved by ID, its program owned by the session user (programs.owner_user_id); program active (else program_archived); precondition: Program active; disclosure hash equals the server-computed text |
| Re-authentication | Yes, purpose `provider_enablement`, always |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `programId`, `providerId` |
| Query parameters | none |
| Request schema | `ProviderEnable` |
| Response schema | `ProgramProviderEnablement` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 403 `reauth_invalid`, 403 `reauth_required`, 404 `not_found`, 409 `conflict_state`, 409 `disclosure_mismatch`, 409 `program_archived`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | State-guarded. A repeat after the transition returns `conflict_state` and changes nothing. |
| Audit event | `ai.provider_enabled` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The re-authentication grant is consumed in the same transaction as the guarded change. |
| PRD requirements | FR-AI-002 |

#### AI-08 POST `/api/v1/programs/{programId}/ai-providers/{providerId}/disable`

| Property | Value |
| --- | --- |
| Purpose | Disable a provider for a program. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; resource resolved by ID, its program owned by the session user (programs.owner_user_id); program active (else program_archived); precondition: Enabled |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `programId`, `providerId` |
| Query parameters | none |
| Request schema | `RevokeRequest` |
| Response schema | `ProgramProviderEnablement` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `program_archived`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | State-guarded. A repeat after the transition returns `conflict_state` and changes nothing. |
| Audit event | `ai.provider_disabled` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-AI-002 |

#### AI-09 GET `/api/v1/programs/{programId}/ai-transmissions`

| Property | Value |
| --- | --- |
| Purpose | List transmissions (digest and size only). |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; resource resolved by ID, its program owned by the session user (programs.owner_user_id) |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `programId` |
| Query parameters | `limit`, `cursor`, `sort`, `providerId` |
| Request schema | none |
| Response schema | `AiTransmissionList` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-AI-004 |

#### AI-10 POST `/api/v1/assessments/{assessmentId}/ai-tasks`

| Property | Value |
| --- | --- |
| Purpose | Request an AI analysis task. It runs as an audited worker job and returns proposals only. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user; program active (else program_archived); precondition: An enabled provider for the program; Secret Service available for remote providers |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json`, `Idempotency-Key` (required) |
| Path parameters | `assessmentId` |
| Query parameters | none |
| Request schema | `AiTaskCreate` |
| Response schema | `Job` |
| Success status | 201 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `idempotency_conflict`, 409 `program_archived`, 409 `provider_not_enabled`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `credential_store_unavailable`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Idempotency-Key required. A repeat with the same key and body returns the original result with `Idempotency-Replayed: true`. The same key with another body returns `idempotency_conflict`. |
| Audit event | `ai.task_requested` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-AI-003, FR-AI-005 |

#### AI-11 GET `/api/v1/assessments/{assessmentId}/ai-outputs`

| Property | Value |
| --- | --- |
| Purpose | List AI notes and drafts, labeled by origin. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; assessment resolved by ID, its program owned by the session user |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `assessmentId` |
| Query parameters | `limit`, `cursor`, `sort`, `kind` |
| Request schema | none |
| Response schema | `AiOutputList` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-AI-005 |

## Tools, passive sources and integration status

#### TLS-01 GET `/api/v1/tools`

| Property | Value |
| --- | --- |
| Purpose | List registered tools and verification state. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; account-level resource |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | none |
| Query parameters | `limit`, `cursor`, `sort` |
| Request schema | none |
| Response schema | `ToolConfigList` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-TOOL-001 |

#### TLS-02 PUT `/api/v1/tools/{toolKey}`

| Property | Value |
| --- | --- |
| Purpose | Register or update the pinned version and digest of a tool. Disabled until verified. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; account-level resource; precondition: Absolute path; digest supplied by the human |
| Re-authentication | Yes, purpose `tool_registration`, always |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `toolKey` |
| Query parameters | none |
| Request schema | `ToolConfigSet` |
| Response schema | `ToolConfig` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 403 `reauth_invalid`, 403 `reauth_required`, 404 `not_found`, 404 `tool_unknown`, 409 `conflict_state`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | `tool.registered` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The path must be absolute. The digest is supplied by the human and must equal the digest computed at verification. A tool stays disabled until verified. |
| PRD requirements | FR-TOOL-001 |

#### TLS-03 POST `/api/v1/tools/{toolKey}/verify`

| Property | Value |
| --- | --- |
| Purpose | Verify binary digest and manifest, and run the sandbox preflight. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; account-level resource |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token` |
| Path parameters | `toolKey` |
| Query parameters | none |
| Request schema | none |
| Response schema | `IntegrationCheck` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `isolation_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | `tool.verified` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-TOOL-001, FR-TOOL-002 |

#### TLS-04 POST `/api/v1/tools/{toolKey}/enable`

| Property | Value |
| --- | --- |
| Purpose | Enable a verified tool. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; account-level resource; precondition: verification_state verified |
| Re-authentication | Yes, purpose `tool_registration`, always |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `toolKey` |
| Query parameters | none |
| Request schema | `RevokeRequest` |
| Response schema | `ToolConfig` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 403 `reauth_invalid`, 403 `reauth_required`, 404 `not_found`, 409 `conflict_state`, 409 `tool_not_verified`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | State-guarded. A repeat after the transition returns `conflict_state` and changes nothing. |
| Audit event | `tool.enabled` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The re-authentication grant is consumed in the same transaction as the guarded change. |
| PRD requirements | FR-TOOL-001 |

#### TLS-05 GET `/api/v1/programs/{programId}/passive-sources`

| Property | Value |
| --- | --- |
| Purpose | List declared passive sources and their enablement. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; resource resolved by ID, its program owned by the session user (programs.owner_user_id) |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | `programId` |
| Query parameters | `limit`, `cursor`, `sort`, `enabled` |
| Request schema | none |
| Response schema | `PassiveSourceList` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-RECON-001 |

#### TLS-06 POST `/api/v1/programs/{programId}/passive-sources/{sourceKey}/enable`

| Property | Value |
| --- | --- |
| Purpose | Enable a passive source after showing what target information it receives. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; resource resolved by ID, its program owned by the session user (programs.owner_user_id); program active (else program_archived); precondition: Program active; disclosure hash equals the server-computed text |
| Re-authentication | Yes, purpose `passive_source_enablement`, always |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `programId`, `sourceKey` |
| Query parameters | none |
| Request schema | `ProviderEnable` |
| Response schema | `PassiveSource` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 403 `reauth_invalid`, 403 `reauth_required`, 404 `not_found`, 404 `source_unknown`, 409 `conflict_state`, 409 `disclosure_mismatch`, 409 `program_archived`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | State-guarded. A repeat after the transition returns `conflict_state` and changes nothing. |
| Audit event | `passive_source.enabled` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The re-authentication grant is consumed in the same transaction as the guarded change. |
| PRD requirements | FR-RECON-001 |

#### TLS-07 POST `/api/v1/programs/{programId}/passive-sources/{sourceKey}/disable`

| Property | Value |
| --- | --- |
| Purpose | Disable a passive source. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; resource resolved by ID, its program owned by the session user (programs.owner_user_id); program active (else program_archived); precondition: Enabled |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `programId`, `sourceKey` |
| Query parameters | none |
| Request schema | `RevokeRequest` |
| Response schema | `PassiveSource` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `conflict_state`, 409 `program_archived`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | State-guarded. A repeat after the transition returns `conflict_state` and changes nothing. |
| Audit event | `passive_source.disabled` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-RECON-001 |

#### INT-01 GET `/api/v1/integrations/status`

| Property | Value |
| --- | --- |
| Purpose | Status of database, Secret Service, sandbox, egress path, tools and providers, and open system alerts. |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; account-level resource |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | none |
| Query parameters | none |
| Request schema | none |
| Response schema | `IntegrationStatus` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The resource is loaded by ID and its program ownership is checked before any field is returned, so another program's data cannot be reached by guessing an ID (BOLA). |
| PRD requirements | FR-TOOL-002, FR-CRED-001, FR-AUDIT-001 |

#### INT-02 POST `/api/v1/system-alerts/{alertId}/clear`

| Property | Value |
| --- | --- |
| Purpose | Clear a system alert after its cause is resolved (an audit-retry alert needs a successful audit write first). |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; account-level resource; precondition: Alert open |
| Re-authentication | No |
| CSRF | Yes |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session`, `Origin` (required, allow-listed), `X-CSRF-Token`, `Content-Type: application/json` |
| Path parameters | `alertId` |
| Query parameters | none |
| Request schema | `RevokeRequest` |
| Response schema | `SystemAlert` |
| Success status | 200 |
| Error responses | 400 `invalid_request`, 401 `session_expired`, 401 `unauthenticated`, 403 `csrf_invalid`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 404 `not_found`, 409 `alert_cause_unresolved`, 409 `conflict_state`, 413 `payload_too_large`, 415 `unsupported_media_type`, 422 `validation_failed`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | State-guarded. A repeat after the transition returns `conflict_state` and changes nothing. |
| Audit event | `system_alert.cleared` (same transaction; a denial is audited in its own transaction) |
| Rate limit | RL-WRITE (60 per minute per session) |
| Security considerations | The actor is always the session user. The state precondition is checked in the same transaction that changes state, so two requests cannot both succeed. |
| PRD requirements | FR-AUDIT-001 |

## Audit

#### AUD-01 GET `/api/v1/audit-events`

| Property | Value |
| --- | --- |
| Purpose | List audit events (keyset pagination, filters). |
| Authentication | Session cookie `ms_session` |
| Authorization | Valid server-side session; account-level resource |
| Re-authentication | No |
| CSRF | No (safe method) |
| Request headers | `Host` (allow-listed), `Accept: application/json`, `Cookie: ms_session` |
| Path parameters | none |
| Query parameters | `limit`, `cursor`, `sort`, `action`, `outcome`, `entityType`, `entityId`, `programId`, `assessmentId`, `from`, `to` |
| Request schema | none |
| Response schema | `AuditEventList` |
| Success status | 200 |
| Error responses | 401 `session_expired`, 401 `unauthenticated`, 403 `forbidden`, 403 `host_denied`, 403 `origin_denied`, 429 `rate_limited`, 500 `internal_error`, 503 `database_unavailable`, 503 `recovery_required` |
| Idempotency | Naturally idempotent (reads, or a PUT/PATCH that sets state with optimistic locking). |
| Audit event | none (read only) |
| Rate limit | RL-READ (300 per minute per session) |
| Security considerations | Read only. The detail field is already redacted at write time. Reading audit events is not itself audited (DQ-13). |
| PRD requirements | FR-AUDIT-001 |

