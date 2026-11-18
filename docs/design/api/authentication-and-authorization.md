# Authentication and authorization

Status: design proposal. It applies the approved localhost web security profile (`security.md` 8, D-33, D-60, D-68, D-69, D-79). M4 (2026-11-18) implements and tests the controls of this file for the routes AUTH-01 to AUTH-10. The re-authentication check is available to the approval and policy use cases as `Reauth.Consume` (M19 and later call it). The approval and policy routes themselves, and the browser side, are later milestones. Every other statement of this file is still a design. The threat rows are in `docs/security/threat-model.md` (T-19, T-20, T-54 to T-59, T-71).

## 1. Principals

There is one principal: the local researcher account (D-60). There are no API tokens, no service accounts and no bearer authentication. The worker has no network listener (P-17). An AI provider, a tool and the worker therefore have no way to call the API, and no request body or header can name an actor (API-17). The audit actor of every API call is the session user.

## 2. Browser protection pipeline (applies before any route logic)

| Order | Check | Rule | Failure |
| --- | --- | --- | --- |
| 1 | Host | The `Host` header must equal one of the configured allow-list entries (exact host and port, for example `127.0.0.1:3000` and `localhost:3000`). `X-Forwarded-Host` and other forwarding headers are ignored. | 403 `host_denied` |
| 2 | Size and media type | Body within the route limit, `application/json` | 413, 415 |
| 3 | Rate limit | Per class (API-14) | 429 |
| 4 | Session | `ms_session` cookie resolves a non-revoked, unexpired session by SHA-256 of the token | 401 `unauthenticated`, `session_expired` |
| 5 | Origin | On every non-GET request the `Origin` header is required and must equal an allow-list entry. `Origin: null` and a missing Origin are refused. When `Sec-Fetch-Site` is present and says `cross-site`, the request is refused. | 403 `origin_denied` |
| 6 | CSRF | `X-CSRF-Token` must equal the session token (constant-time compare against the stored hash). Public state-changing routes use the pre-session token bound to `ms_anon`. | 403 `csrf_invalid` |
| 7 | Resource and ownership | Load by ID, derive the program, check `programs.owner_user_id` | 404 `not_found` |
| 8 | State precondition | The state check of the route, evaluated in the transaction that changes state | 409 `conflict_state` |
| 9 | Re-authentication | Consume the grant if the route needs one | 403 `reauth_required`, `reauth_invalid` |
| 10 | Policy engine | For operations only (E1, E4) | 403 `policy_denied` |

A cheap check always runs before an expensive one. A public route is throttled before any password hashing.

## 3. Session

- **Token.** 256 bits from the operating system random source. Stored only as SHA-256 in `sessions.token_hash`. Never in a JSON body, a URL or a log. A new token is issued on login, setup, recovery reset and password change (no fixation).
- **Cookie.** Name `ms_session`. Attributes: `HttpOnly`, `SameSite=Strict`, `Path=/`, no `Domain` attribute (host-only), `Secure` when the site is served over HTTPS. The `__Host-` prefix needs `Secure` and is not used while the MVP is served over plain `http://localhost`. Whether browsers accept `Secure` cookies on `http://localhost` and how they treat sibling ports is not verified (T-57) and is an M4 and M13 test.
- **Expiry (proposed, DQ-05).** Idle 30 minutes, absolute 12 hours. `last_seen_at` is updated at most once a minute. Expiry is checked on every request. A cleanup sweep removes dead rows after 30 days.
- **Revocation.** Logout, the session list, recovery reset (all sessions), password change (all other sessions). A revoked session fails the next request.
- **State endpoint.** `GET /session` is public. It returns `setup_required` when no account exists, `anonymous` with a pre-session CSRF token and `ms_anon` cookie, or `authenticated` with the user summary, expiry times and the session CSRF token. It never returns the session token.

## 4. CSRF, Origin, CORS (API-04, API-21)

- The CSRF token is 256 bits, created per session, stored as a hash (`sessions.csrf_token_hash`), delivered in the body of `GET /session`, login, setup and recovery responses, and sent in the `X-CSRF-Token` header. It is never in a cookie readable by script, and never in a URL.
- `SameSite=Strict` is defence in depth. It is not a CSRF control here, because cookies are not isolated by port and two local web apps share a site (T-57). The token plus the Origin check are the controls.
- `ms_anon` is an HMAC-signed 15-minute pre-session token, keyed by a random per-process key held in memory. It exists so that login, setup and recovery are also protected against login CSRF. A server restart invalidates it and the client calls `GET /session` again.
- CORS: no CORS headers by default because browsers call same-origin (D-79). The fallback allows exactly one configured origin with credentials, the methods in this contract, the headers `Content-Type`, `X-CSRF-Token`, `Idempotency-Key`, `If-Match`, `X-Request-Id`, a 600-second preflight cache, and never `*`.
- `http.CrossOriginProtection` alone is not used because it accepts a request with no Origin (P-60).

## 5. Authentication flows

**Bootstrap (AUTH-02, D-68).** Available only while no account exists. The server holds a live bootstrap secret whose hash is in `bootstrap_secrets` and whose value is on a memory-backed file. Until an account exists no session can exist, so every authenticated route returns 401 `unauthenticated`, and the public routes login and recovery return 409 `setup_unavailable`. The request carries the secret, the username and the password. One transaction locks the secret row, checks `users` is empty, creates the account, the Argon2id hash, the recovery code hash and the session. The recovery code is returned once. A wrong secret counts toward the rate limit and is audited.

**Login (AUTH-03).** Same response for an unknown user and a wrong password. A dummy hash check keeps the cost uniform. Failures are recorded in `login_attempts` before the hash check and audited in their own transaction. After 5 failures in a rolling 15 minutes further attempts are refused with 429 until the window clears (D-68). Argon2id concurrency is bounded (P-48).

**Recovery (AUTH-08, D-68).** The recovery code (at least 128 bits) and the new password. One transaction consumes the code, replaces the hash, revokes every session, issues a new recovery code and needs no extra flag for "intrusive operations stay disabled until the password has been entered once": by default every intrusive approval already needs a fresh password entry, and a window grant cannot exist before a password entry in the new session (D-68, D-69). No information is given about whether a code exists.

**Password change (AUTH-09).** Needs the current password and a re-authentication grant with purpose `password_change`. Revokes other sessions.

## 6. Re-authentication (API-24, D-58, D-69)

1. The client calls `POST /session/reauthenticate` with the password, a `purpose` and the `subject` of the action. The server canonicalizes the subject, hashes it (SHA-256) and stores a grant bound to the session, the purpose and the hash. Wrong passwords count toward the rate limit.
2. The action request carries `reauthGrantId`. In the same transaction as the guarded change the server runs the conditional update of CC-14: the grant must be unconsumed, unexpired, of this session, of this purpose and of this subject hash.
3. Every grant is single use and lasts at most 120 seconds (proposed, DQ-04). The optional re-authentication window of D-69 is **not implemented in this design**: there is no reusable grant, no window column and no window policy key (DQ-18 is deferred). If it is ever adopted, it may cover only the purpose `verification`, for at most 300 seconds, bound to one session, revoked by logout, session revocation and password change, and it may never cover approval, scope, policy, lab, tool, provider, passive-source, exclusion, raw-capture, password or recovery actions.
4. A grant is never accepted from another session, for another purpose or another subject, and never in a URL.
5. Re-authentication is not approval. A grant proves the human is present for one action. It cannot approve an operation, cannot add or restore an approval use, cannot extend an approval and cannot be requested by an AI actor or a worker, because the route needs the password and a human session.

| Endpoint | Purpose | Required |
| --- | --- | --- |
| AI-02 `/ai/providers` | `provider_enablement` | always |
| AI-07 `/programs/{programId}/ai-providers/{providerId}/enable` | `provider_enablement` | always |
| APR-04 `/approvals/{approvalId}/approve` | `approval_intrusive` | only for the intrusive class |
| ASM-06 `/assessments/{assessmentId}/raw-evidence-capture` | `raw_evidence_capture` | only when enabling |
| AUTH-09 `/account/password` | `password_change` | always |
| FND-06 `/findings/{findingId}/verify` | `verification` | only when policy verification_requires_reauth is on (default off, D-75) |
| LAB-01 `/assessments/{assessmentId}/lab-registrations` | `lab_registration` | always |
| POL-02 `/policy/settings/{settingKey}` | `policy_widening` | only when the change widens what can be tested or sent |
| SCP-02 `/programs/{programId}/scope-versions` | `scope_widening` | only when the new version widens scope |
| SCP-07 `/refused-exclusions/{refusedExclusionId}/acknowledge` | `exclusion_acknowledgement` | always |
| TLS-02 `/tools/{toolKey}` | `tool_registration` | always |
| TLS-04 `/tools/{toolKey}/enable` | `tool_registration` | always |
| TLS-06 `/programs/{programId}/passive-sources/{sourceKey}/enable` | `passive_source_enablement` | always |

What each purpose hashes as the subject: `approval_intrusive` the operation hash and the declared-destinations hash. `scope_widening` the content hash of the new scope version. `policy_widening` the policy key and the canonical new value. `lab_registration` the target, ranges, ports and expiry. `exclusion_acknowledgement` the refused-exclusion id and the target-set hash. `raw_evidence_capture` the assessment id and the new value. `verification` the finding id and the evidence hashes. `provider_enablement`, `tool_registration` and `passive_source_enablement` the subject id and the disclosure hash or the digest. `password_change` a fixed value.

## 7. Authorization model

Authentication says who is calling. It does not say what they may do. Every route performs the checks of the pipeline in section 2 and the route-specific checks below.

| Rule | How it is enforced |
| --- | --- |
| Ownership | The row is loaded by ID, its program is resolved (directly or through the assessment), and `programs.owner_user_id` must equal the session user. A mismatch or a missing row is the same 404 (no existence leak). |
| Program boundary | No item route accepts a program id. Composite foreign keys prevent a cross-program link at the database (DB-09). A list under `/programs/{programId}/...` filters on that program. |
| Archived program | Read-only. Every state change returns 409 `program_archived`. |
| Target access | A target is used only through its program's scope version. Scope evaluation is done by the authoritative scope engine and the policy engine, not by the HTTP layer. |
| Scope evaluation | `SCP-05` previews names and literals only. `VAL-01`, `PLN-01`, `JOB-01`, `AST-06` call the policy engine (E1, E4). The worker repeats it at claim (E2) and before launch (E3). The HTTP result of a preview is never an authorization. |
| Approval binding | The approve request echoes the operation hash and the destinations hash. The server compares them with the stored operation, and the worker compares them again at claim and launch. |
| Job execution | A job row is a request. Authorization is re-evaluated by the worker (D-05). |
| Human-only actions | Approve, revoke, verify, reopen, scope and policy changes, lab registration, tool and provider enablement, exclusion acknowledgement, validation attestation. Only a session can call them, and the database also refuses an AI, worker or tool identity (CHECKs on `approvals.decided_by`, `finding_verifications.verified_by`, `target_validations.attested_by`). |
| AI | An AI request is a worker job created by a human (`AI-10`). Its output is data: plan items with origin `ai`, candidate findings with origin `ai`, notes and drafts. It cannot approve, verify, change policy, call a tool or reach the API. |
| Internal routes | None exist. Worker use cases are not routes (API-16). |

## 8. Recovery state

When startup reconciliation finds a restore or a marker problem (`database/recovery-generation.md`), `GET /session` returns `state: recovery_required` with the reason, and every route except `GET /session` and `POST /system/recovery/reconcile` returns 503 `recovery_required`, including login. Reconciliation takes the account password (checked like a login and counted by the rate limit), revokes every session and every other piece of restored authority, and starts a new recovery generation. The worker refuses claims in the same state.

## 9. Denial semantics

Out-of-scope and ambiguous targets are denied and audited. A denial is never retried into success by the API. A denial that touches the audit trail (policy denial, failed login) is written in its own transaction so a rollback cannot erase it (P-33a). If the audit write fails, the operation does not start, but cancellation, revocation and denials still take effect (D-48).

## 10. Verification status

None of the above is verified by a test. The browser-level behaviors (cookie scope across ports, Secure cookies on localhost, Host seen through the Next.js rewrite, DNS rebinding) are listed as unverified in the threat model (T-55 to T-57) and are acceptance tests of M4 and M13.

