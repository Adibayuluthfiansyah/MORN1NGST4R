# Design question register (DQ-01 to DQ-18)

Status: each question has exactly one status. **Decided** has an authoritative source. **Proposed** is a design recommendation that is not approved. **Deferred** names its trigger and the milestone where it must be revisited. **Owner Decision Required** states the exact question and the safest temporary behavior. No Proposed item is an approved decision, and nothing here was converted into an owner decision by the review that created it.

## Summary

| Status | Count | IDs |
| --- | --- | --- |
| Decided | 2 | DQ-02, DQ-03 |
| Proposed | 10 | DQ-01, DQ-04, DQ-05, DQ-06, DQ-07, DQ-12, DQ-13, DQ-15, DQ-16, DQ-17 |
| Deferred | 5 | DQ-08, DQ-10, DQ-11, DQ-14, DQ-18 |
| Owner Decision Required | 1 | DQ-09 |

| ID | Title | Status | Milestone |
| --- | --- | --- | --- |
| DQ-01 | Clearing a compromised evidence state | Proposed | M6 |
| DQ-02 | Lab registration reuse | Decided | M17 |
| DQ-03 | Re-authentication for human verification | Decided | M22 |
| DQ-04 | Lifetime of a single-use re-authentication grant | Proposed | M4 |
| DQ-05 | Session expiry | Proposed | M4 |
| DQ-06 | Request limits and rate-limit numbers | Proposed | M4 and M16 |
| DQ-07 | Operational retention numbers | Proposed | M7 (sweeps) |
| DQ-08 | Program restore and permanent deletion | Deferred | M16 and G10 |
| DQ-09 | AI tasks and approval | Owner Decision Required | M12 (before the first remote adapter) |
| DQ-10 | Finding severity | Deferred | M22 |
| DQ-11 | Multi-user mode | Deferred | post-MVP |
| DQ-12 | Who may request an approval | Proposed | M19 |
| DQ-13 | Auditing reads of the audit trail | Proposed | M5 |
| DQ-14 | Manual asset entry | Deferred | M20 |
| DQ-15 | Downloading evidence that failed an integrity check | Proposed | M6 |
| DQ-16 | Who supplies a tool digest | Proposed | M10 |
| DQ-17 | Behavior after a database restore | Proposed | M3 and M4 |
| DQ-18 | Reusable re-authentication window | Deferred | M19 and M24 |

## DQ-01 Clearing a compromised evidence state

| Field | Value |
| --- | --- |
| Decision statement | An evidence item marked `compromised` stays compromised. Only a new human-triggered integrity check that matches, with a recorded reason, can move it, and the MVP has no endpoint for that. |
| Status | Proposed |
| Basis | Design proposal. Needs owner confirmation before M6. Not an approved decision. |
| Rationale | A silent return to `ok` would let altered evidence support a verification. |
| Security impact | Keeps tampering visible. Failure mode is an unusable item, which fails closed. |
| Database impact | `evidence_status.integrity_state`, `evidence_integrity_checks`, CHECK on `compromised_at`. |
| API impact | EVD-03 records checks. No clearing route. |
| Related PRD requirements | FR-EVIDENCE-002 |
| Implementation milestone | M6 |
| Required verification | Test that no code path writes `ok` after `compromised` without a matching check row and a reason. |
| Residual risk | A genuinely restored file stays unusable until a clearing procedure exists. |

## DQ-02 Lab registration reuse

| Field | Value |
| --- | --- |
| Decision statement | A lab registration is bound to one program and one assessment. A new assessment needs a new registration. |
| Status | Decided |
| Source | Source: D-47 (bound to a program and an assessment), `security.md` 2. |
| Rationale | Prevents a lab exemption from silently covering unrelated work. |
| Security impact | Limits blast radius of a lab exemption. |
| Database impact | `lab_registrations.assessment_id` NOT NULL with a composite foreign key. |
| API impact | LAB-01 takes the assessment in its path. |
| Related PRD requirements | FR-SCOPE-004 |
| Implementation milestone | M17 |
| Required verification | Test that a registration of assessment A cannot authorize a target in assessment B. |
| Residual risk | Operator effort for repeated labs. |

## DQ-03 Re-authentication for human verification

| Field | Value |
| --- | --- |
| Decision statement | Finding verification does not require password re-entry by default. A policy field `verification_requires_reauth` can require it. |
| Status | Decided |
| Source | Source: D-75 in `0003-foundation-closure-decisions.md` ("Verified may require password entry, decided at M22 as a policy field, default off"). |
| Rationale | Keeps verification fast while the human is already authenticated. The human, not the password, is the control (SEC 5). |
| Security impact | Verification stays human-only whatever the field says. |
| Database impact | `policy_settings` key `verification_requires_reauth`; `finding_verifications.reauth_grant_id` nullable. |
| API impact | FND-06 is class RA and consumes a grant only when the field is on. |
| Related PRD requirements | FR-FINDING-003 |
| Implementation milestone | M22 |
| Required verification | Test both field values, and that an AI or worker actor can never reach the verified transition. |
| Residual risk | A hijacked session can verify a finding while the field is off. |

## DQ-04 Lifetime of a single-use re-authentication grant

| Field | Value |
| --- | --- |
| Decision statement | A grant expires 120 seconds after issue and is consumed by the first matching action. |
| Status | Proposed |
| Basis | Design proposal. The number is not owner-confirmed. |
| Rationale | Long enough to read a screen and confirm, short enough to limit a stolen grant. |
| Security impact | Shorter lifetime narrows the replay window. |
| Database impact | `reauth_grants.expires_at`, CHECK that the lifetime is at most 120 seconds. |
| API impact | AUTH-07 returns `expiresAt`. |
| Related PRD requirements | FR-APPROVAL-005 |
| Implementation milestone | M4 |
| Required verification | Test expiry boundary and single use. |
| Residual risk | A slow human must re-enter the password. |

## DQ-05 Session expiry

| Field | Value |
| --- | --- |
| Decision statement | Idle expiry 30 minutes, absolute expiry 12 hours. |
| Status | Proposed |
| Basis | Design proposal. D-60 requires expiry but sets no number. |
| Rationale | Typical local-tool balance. |
| Security impact | Limits the life of a stolen cookie. |
| Database impact | `sessions.idle_expires_at`, `absolute_expires_at`. |
| API impact | `GET /session` returns both times. |
| Related PRD requirements | FR-AUTH-001 |
| Implementation milestone | M4 |
| Required verification | Test idle and absolute expiry and sliding behavior. |
| Residual risk | Annoyance for long sessions. |

## DQ-06 Request limits and rate-limit numbers

| Field | Value |
| --- | --- |
| Decision statement | Body sizes, timeouts, rate-limit numbers, page sizes and the idempotency lifetime follow `api/conventions.md` and `endpoint-specification.md`. Only the authentication limit (5 failures per 15 minutes) is decided (D-68). |
| Status | Proposed |
| Basis | Design proposal for every number except the D-68 authentication limit. |
| Rationale | SEC 11 requires limits but sets none. |
| Security impact | A wrong limit weakens flood protection or blocks the user. |
| Database impact | `idempotency_keys.expires_at`, `login_attempts`. |
| API impact | API-11, API-12, API-14. |
| Related PRD requirements | NFR-SEC-001 |
| Implementation milestone | M4 and M16 |
| Required verification | Load and flood tests against each class. |
| Residual risk | Numbers may need tuning. |

## DQ-07 Operational retention numbers

| Field | Value |
| --- | --- |
| Decision statement | Operational rows (sessions, grants, attempts, idempotency, checks, alerts) are purged 30 days after expiry or clearance. Pending approvals older than 7 days are marked stale. |
| Status | Proposed |
| Basis | Design proposal. No owner decision sets retention numbers. |
| Rationale | Operational data has no audit value after expiry. |
| Security impact | Removing expired authority reduces stale-grant risk. Audit rows are never purged by this rule. |
| Database impact | RC-B rows in `data-lifecycle.md`. |
| API impact | None. |
| Related PRD requirements | NFR-PRIV-002 |
| Implementation milestone | M7 (sweeps) |
| Required verification | Test that the sweep never touches RC-A rows. |
| Residual risk | Short retention can hide a late investigation. |

## DQ-08 Program restore and permanent deletion

| Field | Value |
| --- | --- |
| Decision statement | The MVP can archive a program. It cannot restore an archived program or permanently delete data. |
| Status | Deferred |
| Trigger and revisit milestone | Trigger: the owner asks for restore or purge, or retention rules for target data are decided. Revisit at the start of M16 (program use cases) and again before the first release (gate G10). |
| Rationale | Permanent deletion needs a retention procedure that deletes leaf rows and audits the purge. |
| Security impact | No hard deletion means no accidental loss of authorization history. |
| Database impact | No delete grants. RESTRICT foreign keys (DB-05). |
| API impact | No DELETE routes. DEF-03. |
| Related PRD requirements | FR-PROG-001 |
| Implementation milestone | M16 and G10 |
| Required verification | Review before any deletion route is added. |
| Residual risk | Data of an abandoned program stays on disk. |

## DQ-09 AI tasks and approval

| Field | Value |
| --- | --- |
| Decision statement | Does an AI analysis task need an approval record in addition to the per-program provider enablement and the disclosure that D-46 and D-50 require? |
| Status | Owner Decision Required |
| Exact question and temporary behavior | Exact question: should a remote-provider AI task require an explicit approval (like an operation) for each transmission, or is the per-program enablement with disclosure and an audit record enough? Temporary behavior: enablement, disclosure, audit and the digest only. No tool runs in an AI job. |
| Rationale | A transmission leaves the machine, so the question is about consent granularity. |
| Security impact | Under-approval could send target data more often than the human expects. |
| Database impact | `jobs.risk_class = ai`, `program_provider_enablements`, `ai_transmissions`. |
| API impact | AI-07 enables, AI-10 requests a task. |
| Related PRD requirements | FR-AI-002, FR-AI-003 |
| Implementation milestone | M12 (before the first remote adapter) |
| Required verification | Test that no AI job reaches a tool and that a transmission without enablement fails. |
| Residual risk | Data egress without per-task consent if the owner chooses the lighter model. |

## DQ-10 Finding severity

| Field | Value |
| --- | --- |
| Decision statement | Severity, CVSS and bounty status are not modeled. |
| Status | Deferred |
| Trigger and revisit milestone | Trigger: the owner wants severity or a bounty status in reports. Revisit at M22 and M14. The PRD has no such requirement. |
| Rationale | Not required by the PRD. |
| Security impact | None. |
| Database impact | No columns. |
| API impact | No fields. |
| Related PRD requirements | FR-FINDING-001 |
| Implementation milestone | M22 |
| Required verification | None until requested. |
| Residual risk | Reports carry no severity. |

## DQ-11 Multi-user mode

| Field | Value |
| --- | --- |
| Decision statement | The database has `owner_user_id` but enforces one active account. |
| Status | Deferred |
| Trigger and revisit milestone | Trigger: owner approves a multi-user or remote mode (D-20). Revisit before any such mode. A migration drops the single-account index and adds roles. |
| Rationale | D-60 limits the MVP to one account. |
| Security impact | The constraint prevents a second account by bug. |
| Database impact | `users_single_active`. |
| API impact | No user routes beyond the account. |
| Related PRD requirements | FR-AUTH-001 |
| Implementation milestone | post-MVP |
| Required verification | Test that a second active user cannot be created. |
| Residual risk | A future mode needs a real authorization model. |

## DQ-12 Who may request an approval

| Field | Value |
| --- | --- |
| Decision statement | Only a human requests an approval. The schema has no system or AI requester. |
| Status | Proposed |
| Basis | Design change made in the hardening round (the `requested_by_kind` column was removed). Needs owner confirmation. |
| Rationale | Removes a path through which automation could create an approval request. |
| Security impact | An approval request is not an approval, but the narrower rule is easier to verify. |
| Database impact | `approvals.requested_by_user_id` NOT NULL. |
| API impact | APR-01 is a human action. |
| Related PRD requirements | FR-APPROVAL-001 |
| Implementation milestone | M19 |
| Required verification | Test that no worker or AI path inserts an approval. |
| Residual risk | Automation of approval requests would need a new design. |

## DQ-13 Auditing reads of the audit trail

| Field | Value |
| --- | --- |
| Decision statement | Reading the audit trail is not itself audited. Evidence content access and report export are audited. |
| Status | Proposed |
| Basis | Design proposal. `NFR-OBS-002` says read-only polling is not audited per request, but does not address the audit list. |
| Rationale | Avoids audit growth from polling. |
| Security impact | Reading audit data cannot change state. |
| Database impact | None. |
| API impact | AUD-01 has no audit event. |
| Related PRD requirements | FR-AUDIT-001 |
| Implementation milestone | M5 |
| Required verification | Test that EVD-05 and RPT-07 write their events first. |
| Residual risk | A reader of the trail leaves no record. |

## DQ-14 Manual asset entry

| Field | Value |
| --- | --- |
| Decision statement | There is no route to add an asset by hand. |
| Status | Deferred |
| Trigger and revisit milestone | Trigger: the owner needs seed assets without running a tool. Revisit at M20. |
| Rationale | Not a PRD requirement. Discovery and promotion cover the workflow. |
| Security impact | A manual entry would need scope evaluation like any asset (E4). |
| Database impact | `assets.first_origin` already allows `human_entered`. |
| API impact | DEF-02. |
| Related PRD requirements | FR-ASSET-001 |
| Implementation milestone | M20 |
| Required verification | None. |
| Residual risk | A researcher cannot seed assets from a notebook. |

## DQ-15 Downloading evidence that failed an integrity check

| Field | Value |
| --- | --- |
| Decision statement | Evidence in state `compromised` or `missing` is withheld unless the human passes `acknowledgeIntegrity=true`. The download is audited and the item stays excluded from verification. |
| Status | Proposed |
| Basis | Design proposal. Needs owner confirmation before M6. |
| Rationale | A human may need the bytes to investigate corruption. |
| Security impact | A bypass flag on an integrity control. The flag does not make the evidence usable as proof. |
| Database impact | `evidence_status`, `audit_events`. |
| API impact | EVD-05 query parameter. |
| Related PRD requirements | FR-EVIDENCE-002, FR-EVIDENCE-003 |
| Implementation milestone | M6 |
| Required verification | Test that an acknowledged download never changes integrity state or enables verification. |
| Residual risk | Altered content leaves the store. |

## DQ-16 Who supplies a tool digest

| Field | Value |
| --- | --- |
| Decision statement | The human types the expected SHA-256 of a tool binary. Verification computes the digest and must match. The server never trusts a computed digest alone. |
| Status | Proposed |
| Basis | Design proposal. Consistent with D-43 (pinned binaries) but the entry flow is not decided. |
| Rationale | A digest computed by the server at registration would pin whatever is on disk. |
| Security impact | Pin comes from a human review of the release checksum. |
| Database impact | `tool_configs.binary_sha256`, `verification_state`. |
| API impact | TLS-02, TLS-03. |
| Related PRD requirements | FR-TOOL-001 |
| Implementation milestone | M10 |
| Required verification | Test mismatch, swap and path changes. |
| Residual risk | Human typing errors. |

## DQ-17 Behavior after a database restore

| Field | Value |
| --- | --- |
| Decision statement | After a detected restore, authority recorded in the restored database is revoked or cancelled and a new recovery generation starts (`database/recovery-generation.md`, DB-28). |
| Status | Proposed |
| Basis | Design proposal. The mechanism is new in the hardening round and needs owner confirmation before M3. |
| Rationale | A restored backup can bring back consumed approval uses, revoked approvals and live sessions. |
| Security impact | Fail closed against replay of old authority. |
| Database impact | `recovery_state`, the external marker, reconciliation transactions. |
| API impact | AUTH-10, `recovery_required` in `GET /session`. |
| Related PRD requirements | FR-APPROVAL-002, FR-APPROVAL-003, FR-SCOPE-006 |
| Implementation milestone | M3 and M4 |
| Required verification | Tests listed in `recovery-generation.md` section 9. |
| Residual risk | Not protected against a same-user attacker or a full home-directory snapshot restore. |

## DQ-18 Reusable re-authentication window

| Field | Value |
| --- | --- |
| Decision statement | No reusable grant exists in the MVP design. Every grant is single use and bound to one action. |
| Status | Deferred |
| Trigger and revisit milestone | Trigger: the owner decides to enable the optional window of D-69. Revisit at M19 and M24. If adopted later, only the purpose `verification` may use a window grant, for at most 300 seconds, bound to one session, revoked by logout, session revocation and password change, and never usable for any target-affecting purpose. |
| Rationale | D-69 makes the window optional and off by default. Nothing in the MVP needs it. |
| Security impact | No reusable grant can bypass exact-operation approval or add an approval use. |
| Database impact | `reauth_grants` has no reusable column. The key `reauth_window_seconds` is not in the policy key list. |
| API impact | No window grant in AUTH-07. |
| Related PRD requirements | FR-APPROVAL-005 |
| Implementation milestone | M19 and M24 |
| Required verification | If ever adopted: tests that the window cannot cover approval, scope, policy, lab, tool, provider or recovery purposes. |
| Residual risk | More password prompts for the user. |

