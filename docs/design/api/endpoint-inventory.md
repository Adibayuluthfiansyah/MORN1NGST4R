# Endpoint inventory

Status: design proposal. No route exists. Every endpoint has a stable ID (`AREA-nn`). The OpenAPI file `openapi.yaml` contains exactly the 118 endpoints of the first table and no others. Paths below are relative to the base path `/api/v1`.

Route classes: **PUB** public before a session (still protected by Host, Origin and rate limits), **R** authenticated read or side-effect-free evaluation, **W** authenticated state change, **RA** state change that consumes a re-authentication grant (some only conditionally, see the specification). Every non-GET method needs the CSRF token and a valid Origin.

## Counts

| Domain | Endpoints | PUB | R | W | RA |
| --- | --- | --- | --- | --- | --- |
| Authentication and account | 10 | 5 | 1 | 3 | 1 |
| Security policy | 2 | 0 | 1 | 0 | 1 |
| Programs | 7 | 0 | 3 | 4 | 0 |
| Targets, scope, labs and target authorization | 19 | 0 | 11 | 5 | 3 |
| Assessments and plans | 17 | 0 | 5 | 11 | 1 |
| Approvals | 7 | 0 | 3 | 3 | 1 |
| Jobs | 6 | 0 | 4 | 2 | 0 |
| Assets and endpoints | 6 | 0 | 5 | 1 | 0 |
| Evidence | 5 | 0 | 4 | 1 | 0 |
| Findings | 12 | 0 | 5 | 6 | 1 |
| Reports | 7 | 0 | 3 | 4 | 0 |
| AI providers and tasks | 10 | 0 | 4 | 4 | 2 |
| Tools, passive sources and integration status | 9 | 0 | 3 | 3 | 3 |
| Audit | 1 | 0 | 1 | 0 | 0 |
| **Total** | **118** | 5 | 53 | 47 | 13 |

## Endpoints in OpenAPI

### Authentication and account

| ID | Method | Path | Class | Purpose | PRD |
| --- | --- | --- | --- | --- | --- |
| AUTH-01 | GET | `/session` | PUB | Report setup, anonymous or authenticated state and issue the CSRF token. | FR-AUTH-001, FR-AUTH-002 |
| AUTH-02 | POST | `/setup/account` | PUB | Create the single account with the bootstrap secret. | FR-AUTH-001 |
| AUTH-03 | POST | `/session/login` | PUB | Log in with username and password. | FR-AUTH-001 |
| AUTH-04 | POST | `/session/logout` | W | End the current session. | FR-AUTH-001 |
| AUTH-05 | GET | `/sessions` | R | List active sessions of the account. | FR-AUTH-001 |
| AUTH-06 | POST | `/sessions/{sessionId}/revoke` | W | Revoke one session. | FR-AUTH-001 |
| AUTH-07 | POST | `/session/reauthenticate` | W | Enter the password again and receive a single-use grant for one exact action. | FR-APPROVAL-005 |
| AUTH-08 | POST | `/recovery/reset` | PUB | Reset the password with the one-time recovery code. | FR-AUTH-001 |
| AUTH-10 | POST | `/system/recovery/reconcile` | PUB | After a detected restore, run the reconciliation that invalidates old authority and starts a new recovery generation. Needs the account password. | FR-AUTH-001, FR-APPROVAL-003, FR-SCOPE-006 |
| AUTH-09 | POST | `/account/password` | RA | Change the password. | FR-AUTH-001 |

### Security policy

| ID | Method | Path | Class | Purpose | PRD |
| --- | --- | --- | --- | --- | --- |
| POL-01 | GET | `/policy/settings` | R | List the current security policy values. | FR-POLICY-002 |
| POL-02 | PUT | `/policy/settings/{settingKey}` | RA | Change one policy value as a new version. | FR-POLICY-002 |

### Programs

| ID | Method | Path | Class | Purpose | PRD |
| --- | --- | --- | --- | --- | --- |
| PRG-01 | POST | `/programs` | W | Register a program. | FR-PROG-001 |
| PRG-02 | GET | `/programs` | R | List programs. | FR-PROG-001 |
| PRG-03 | GET | `/programs/{programId}` | R | Get a program. | FR-PROG-001 |
| PRG-04 | PATCH | `/programs/{programId}` | W | Update name, label or description. | FR-PROG-001 |
| PRG-05 | POST | `/programs/{programId}/archive` | W | Archive a program (read-only afterwards). | FR-PROG-001 |
| PRG-06 | POST | `/programs/{programId}/rules-versions` | W | Record a new version of the program rules reference. | FR-PROG-002 |
| PRG-07 | GET | `/programs/{programId}/rules-versions` | R | List rules reference versions. | FR-PROG-002 |

### Targets, scope, labs and target authorization

| ID | Method | Path | Class | Purpose | PRD |
| --- | --- | --- | --- | --- | --- |
| TGT-01 | POST | `/programs/{programId}/targets` | W | Register a target (registration is not authorization). | FR-SCOPE-001 |
| TGT-02 | GET | `/programs/{programId}/targets` | R | List targets. | FR-SCOPE-001 |
| TGT-03 | GET | `/targets/{targetId}` | R | Get a target. | FR-SCOPE-001 |
| TGT-04 | PATCH | `/targets/{targetId}` | W | Change the label. The canonical value is immutable. | FR-SCOPE-001 |
| TGT-05 | POST | `/targets/{targetId}/archive` | W | Archive a target and invalidate dependent validations. | FR-SCOPE-006 |
| SCP-01 | GET | `/programs/{programId}/scope` | R | Get the current scope version with its rules. | FR-SCOPE-001 |
| SCP-02 | POST | `/programs/{programId}/scope-versions` | RA | Publish a new scope version from a full rule list. | FR-SCOPE-001, FR-SCOPE-006 |
| SCP-03 | GET | `/programs/{programId}/scope-versions` | R | List scope versions. | FR-SCOPE-001 |
| SCP-04 | GET | `/scope-versions/{scopeVersionId}` | R | Get one scope version with its rules. | FR-SCOPE-001 |
| SCP-05 | POST | `/programs/{programId}/scope-evaluations` | R | Preview the scope decision for names or literals. Nothing is resolved, stored or authorized. | FR-SCOPE-002 |
| SCP-06 | GET | `/programs/{programId}/refused-exclusions` | R | List refused exclusions. | FR-SCOPE-001 |
| SCP-07 | POST | `/refused-exclusions/{refusedExclusionId}/acknowledge` | RA | Acknowledge a refused exclusion, bound to the current target-set hash. | FR-SCOPE-001 |
| LAB-01 | POST | `/assessments/{assessmentId}/lab-registrations` | RA | Register a lab target for one assessment. | FR-SCOPE-004 |
| LAB-02 | GET | `/assessments/{assessmentId}/lab-registrations` | R | List lab registrations. | FR-SCOPE-004 |
| LAB-03 | GET | `/lab-registrations/{labRegistrationId}` | R | Get a lab registration. | FR-SCOPE-004 |
| LAB-04 | POST | `/lab-registrations/{labRegistrationId}/revoke` | W | Revoke a lab registration. | FR-SCOPE-004 |
| VAL-01 | POST | `/assessments/{assessmentId}/target-validations` | W | Record a target authorization validation with a human attestation. | FR-SCOPE-005 |
| VAL-02 | GET | `/assessments/{assessmentId}/target-validations` | R | List validation records. | FR-SCOPE-005 |
| VAL-03 | GET | `/target-validations/{validationId}` | R | Get a validation record with per-target results. | FR-SCOPE-005 |

### Assessments and plans

| ID | Method | Path | Class | Purpose | PRD |
| --- | --- | --- | --- | --- | --- |
| ASM-01 | POST | `/programs/{programId}/assessments` | W | Create an assessment in draft. | FR-ASSESS-001 |
| ASM-02 | GET | `/programs/{programId}/assessments` | R | List assessments. | FR-ASSESS-001 |
| ASM-03 | GET | `/assessments/{assessmentId}` | R | Get an assessment. | FR-ASSESS-001 |
| ASM-04 | PATCH | `/assessments/{assessmentId}` | W | Update name or description while draft. | FR-ASSESS-001 |
| ASM-05 | PUT | `/assessments/{assessmentId}/targets` | W | Replace the target set. Invalidates the validation record. | FR-ASSESS-001, FR-SCOPE-006 |
| ASM-06 | PUT | `/assessments/{assessmentId}/raw-evidence-capture` | RA | Turn raw evidence capture on or off (D-77). Turning it on widens. | NFR-PRIV-002 |
| ASM-07 | POST | `/assessments/{assessmentId}/ready` | W | Mark ready. Needs a current validation record. | FR-ASSESS-001 |
| ASM-08 | POST | `/assessments/{assessmentId}/start` | W | Start. | FR-ASSESS-001 |
| ASM-09 | POST | `/assessments/{assessmentId}/pause` | W | Pause: no new launches, queued jobs held. | FR-ASSESS-001 |
| ASM-10 | POST | `/assessments/{assessmentId}/resume` | W | Resume from paused. | FR-ASSESS-001 |
| ASM-11 | POST | `/assessments/{assessmentId}/complete` | W | Complete. | FR-ASSESS-001 |
| ASM-12 | POST | `/assessments/{assessmentId}/cancel` | W | Cancel: stop queued and running work (30 s objective). | FR-ASSESS-001, FR-APPROVAL-003 |
| ASM-13 | GET | `/assessments/{assessmentId}/status` | R | Summary: state, validation currency, blockers, job counts. | FR-ASSESS-001 |
| PLN-01 | POST | `/assessments/{assessmentId}/plan-items` | W | Create a human plan item. | FR-FINDING-004 |
| PLN-02 | GET | `/assessments/{assessmentId}/plan-items` | R | List plan items. | FR-FINDING-004 |
| PLN-03 | GET | `/plan-items/{planItemId}` | R | Get a plan item with its canonical operation. | FR-FINDING-004, FR-APPROVAL-005 |
| PLN-04 | POST | `/plan-items/{planItemId}/reject` | W | Reject a proposed plan item. | FR-FINDING-004 |

### Approvals

| ID | Method | Path | Class | Purpose | PRD |
| --- | --- | --- | --- | --- | --- |
| APR-01 | POST | `/plan-items/{planItemId}/approvals` | W | Request an approval for the exact operation of a plan item. | FR-APPROVAL-001, FR-APPROVAL-006 |
| APR-02 | GET | `/assessments/{assessmentId}/approvals` | R | List approvals. | FR-APPROVAL-001 |
| APR-03 | GET | `/approvals/{approvalId}` | R | Get an approval with its canonical operation. | FR-APPROVAL-001, FR-APPROVAL-005 |
| APR-04 | POST | `/approvals/{approvalId}/approve` | RA | Approve exactly the shown operation. The body echoes the operation hash and destinations hash. | FR-APPROVAL-001, FR-APPROVAL-005 |
| APR-05 | POST | `/approvals/{approvalId}/reject` | W | Reject a pending approval. | FR-APPROVAL-001 |
| APR-06 | POST | `/approvals/{approvalId}/revoke` | W | Revoke an approval; dependent jobs get stop_requested. | FR-APPROVAL-003 |
| APR-07 | GET | `/approvals/{approvalId}/events` | R | Approval history. | FR-APPROVAL-001 |

### Jobs

| ID | Method | Path | Class | Purpose | PRD |
| --- | --- | --- | --- | --- | --- |
| JOB-01 | POST | `/plan-items/{planItemId}/jobs` | W | Create a job from an accepted plan item (E1). | FR-JOB-001, FR-POLICY-001 |
| JOB-02 | GET | `/assessments/{assessmentId}/jobs` | R | List jobs. | FR-JOB-001 |
| JOB-03 | GET | `/jobs/{jobId}` | R | Get a job. | FR-JOB-001 |
| JOB-04 | POST | `/jobs/{jobId}/cancel` | W | Request cancellation. The database records intent, the worker stops the process. | FR-JOB-001, FR-APPROVAL-003 |
| JOB-05 | GET | `/jobs/{jobId}/attempts` | R | Execution attempts with provenance. | FR-JOB-001 |
| JOB-06 | GET | `/jobs/{jobId}/transitions` | R | State and control history. | FR-JOB-001 |

### Assets and endpoints

| ID | Method | Path | Class | Purpose | PRD |
| --- | --- | --- | --- | --- | --- |
| AST-01 | GET | `/programs/{programId}/assets` | R | List assets with scope decision and staleness. | FR-ASSET-001 |
| AST-02 | GET | `/assets/{assetId}` | R | Get an asset. | FR-ASSET-001 |
| AST-03 | GET | `/programs/{programId}/endpoints` | R | List endpoints with parameters. | FR-ASSET-002 |
| AST-04 | GET | `/endpoints/{endpointId}` | R | Get an endpoint. | FR-ASSET-002 |
| AST-05 | GET | `/assets/{assetId}/observations` | R | Discovery provenance of an asset. | FR-ASSET-001 |
| AST-06 | POST | `/assets/{assetId}/promote` | W | Promote an in-scope asset to a registered target (E4). | FR-ASSET-001, FR-SCOPE-002 |

### Evidence

| ID | Method | Path | Class | Purpose | PRD |
| --- | --- | --- | --- | --- | --- |
| EVD-01 | GET | `/assessments/{assessmentId}/evidence` | R | List evidence metadata. | FR-EVIDENCE-001 |
| EVD-02 | GET | `/evidence/{evidenceId}` | R | Get evidence metadata and integrity state. | FR-EVIDENCE-001, FR-EVIDENCE-002 |
| EVD-03 | POST | `/evidence/{evidenceId}/integrity-checks` | W | Re-hash the stored content and record the result. | FR-EVIDENCE-002 |
| EVD-04 | GET | `/evidence/{evidenceId}/integrity-checks` | R | Integrity check history. | FR-EVIDENCE-002 |
| EVD-05 | GET | `/evidence/{evidenceId}/content` | R | Download the stored bytes as an attachment. | FR-EVIDENCE-003 |

### Findings

| ID | Method | Path | Class | Purpose | PRD |
| --- | --- | --- | --- | --- | --- |
| FND-01 | POST | `/assessments/{assessmentId}/findings` | W | Record a human-created candidate finding. | FR-FINDING-001 |
| FND-02 | GET | `/assessments/{assessmentId}/findings` | R | List findings. | FR-FINDING-001 |
| FND-03 | GET | `/findings/{findingId}` | R | Get a finding with its verification record. | FR-FINDING-001 |
| FND-04 | POST | `/findings/{findingId}/submit-for-review` | W | Move a candidate to Needs Review (a human may do what automation may). | FR-FINDING-001 |
| FND-05 | POST | `/findings/{findingId}/review-outcome` | W | Record the human review outcome. | FR-FINDING-001, FR-FINDING-002 |
| FND-06 | POST | `/findings/{findingId}/verify` | RA | Human verification. Never available to automation. | FR-FINDING-003 |
| FND-07 | POST | `/findings/{findingId}/reopen` | W | Reopen any outcome or a verification to Needs Review. | FR-FINDING-001 |
| FND-08 | GET | `/findings/{findingId}/transitions` | R | Finding state history. | FR-FINDING-001 |
| FND-09 | POST | `/findings/{findingId}/evidence` | W | Link evidence to a finding. | FR-FINDING-002 |
| FND-10 | GET | `/findings/{findingId}/evidence` | R | List linked evidence. | FR-FINDING-002 |
| FND-11 | POST | `/findings/{findingId}/validations` | W | Record a human validation result. | FR-FINDING-002 |
| FND-12 | GET | `/findings/{findingId}/validations` | R | List validation records and recommendations. | FR-FINDING-002 |

### Reports

| ID | Method | Path | Class | Purpose | PRD |
| --- | --- | --- | --- | --- | --- |
| RPT-01 | POST | `/assessments/{assessmentId}/reports` | W | Create a draft report. | FR-REPORT-001 |
| RPT-02 | GET | `/assessments/{assessmentId}/reports` | R | List reports. | FR-REPORT-001 |
| RPT-03 | GET | `/reports/{reportId}` | R | Get a report with its findings. | FR-REPORT-001 |
| RPT-04 | PUT | `/reports/{reportId}/findings` | W | Set the included findings and their sections. | FR-REPORT-001, FR-REPORT-002 |
| RPT-05 | POST | `/reports/{reportId}/generate` | W | Generate the Markdown file from the included findings. | FR-REPORT-001, FR-REPORT-003 |
| RPT-06 | POST | `/reports/{reportId}/finalize` | W | Finalize a generated report. Immutable afterwards. | FR-REPORT-002 |
| RPT-07 | GET | `/reports/{reportId}/content` | R | Download the report as an attachment (export). | FR-REPORT-003 |

### AI providers and tasks

| ID | Method | Path | Class | Purpose | PRD |
| --- | --- | --- | --- | --- | --- |
| AI-02 | POST | `/ai/providers` | RA | Configure a provider. The secret is accepted once and goes to the Secret Service. | FR-AI-001, FR-CRED-001 |
| AI-03 | GET | `/ai/providers` | R | List providers. | FR-AI-001 |
| AI-04 | GET | `/ai/providers/{providerId}` | R | Get a provider. No secret is returned. | FR-AI-001 |
| AI-05 | PATCH | `/ai/providers/{providerId}` | W | Update name or model. Endpoint changes need a new provider. | FR-AI-001 |
| AI-06 | POST | `/ai/providers/{providerId}/tests` | W | Test connectivity with a fixed synthetic prompt. No program or target data is sent. | FR-AI-001 |
| AI-07 | POST | `/programs/{programId}/ai-providers/{providerId}/enable` | RA | Enable a provider for a program after showing what will be sent. | FR-AI-002 |
| AI-08 | POST | `/programs/{programId}/ai-providers/{providerId}/disable` | W | Disable a provider for a program. | FR-AI-002 |
| AI-09 | GET | `/programs/{programId}/ai-transmissions` | R | List transmissions (digest and size only). | FR-AI-004 |
| AI-10 | POST | `/assessments/{assessmentId}/ai-tasks` | W | Request an AI analysis task. It runs as an audited worker job and returns proposals only. | FR-AI-003, FR-AI-005 |
| AI-11 | GET | `/assessments/{assessmentId}/ai-outputs` | R | List AI notes and drafts, labeled by origin. | FR-AI-005 |

### Tools, passive sources and integration status

| ID | Method | Path | Class | Purpose | PRD |
| --- | --- | --- | --- | --- | --- |
| TLS-01 | GET | `/tools` | R | List registered tools and verification state. | FR-TOOL-001 |
| TLS-02 | PUT | `/tools/{toolKey}` | RA | Register or update the pinned version and digest of a tool. Disabled until verified. | FR-TOOL-001 |
| TLS-03 | POST | `/tools/{toolKey}/verify` | W | Verify binary digest and manifest, and run the sandbox preflight. | FR-TOOL-001, FR-TOOL-002 |
| TLS-04 | POST | `/tools/{toolKey}/enable` | RA | Enable a verified tool. | FR-TOOL-001 |
| TLS-05 | GET | `/programs/{programId}/passive-sources` | R | List declared passive sources and their enablement. | FR-RECON-001 |
| TLS-06 | POST | `/programs/{programId}/passive-sources/{sourceKey}/enable` | RA | Enable a passive source after showing what target information it receives. | FR-RECON-001 |
| TLS-07 | POST | `/programs/{programId}/passive-sources/{sourceKey}/disable` | W | Disable a passive source. | FR-RECON-001 |
| INT-01 | GET | `/integrations/status` | R | Status of database, Secret Service, sandbox, egress path, tools and providers, and open system alerts. | FR-TOOL-002, FR-CRED-001, FR-AUDIT-001 |
| INT-02 | POST | `/system-alerts/{alertId}/clear` | W | Clear a system alert after its cause is resolved (an audit-retry alert needs a successful audit write first). | FR-AUDIT-001 |

### Audit

| ID | Method | Path | Class | Purpose | PRD |
| --- | --- | --- | --- | --- | --- |
| AUD-01 | GET | `/audit-events` | R | List audit events (keyset pagination, filters). | FR-AUDIT-001 |

## Deferred endpoints (not in OpenAPI)

| ID | Method | Path | Reason |
| --- | --- | --- | --- |
| POL-03 | GET | `/policy/settings/{settingKey}/versions` | Redundant with `AUD-01` filtered by `policy.changed`, because every policy change records the old and the new value (D-53). Revisit when a policy history screen is designed (M24). |
| FND-13 | POST | `/findings/{findingId}/validations/replay` | Needs milestone M22b (first-party replay client behind the egress gate G-egress). |
| AI-01 | GET | `/ai/provider-kinds` | Static information. The adapter kinds are the enum of `AiProvider.adapterKind` in the OpenAPI schema (D-81). Revisit if adapters become pluggable. |
| DEF-01 | - | - | Password-recovery code rotation without a reset. Not required by D-68. |
| DEF-02 | - | - | Manual asset entry (`human_entered` origin exists in the schema). Not a PRD requirement, see DQ-14. |
| DEF-03 | - | - | Program restore from archive and permanent deletion. Not in the MVP (DB retention, DQ-08). |
| DEF-04 | - | - | Scope version diff view. Derivable client-side from two versions. |
| DEF-05 | - | - | Metrics endpoints. Post-MVP (D-80). The health endpoints are in the MVP (D-86, `docs/decisions/0004-health-endpoints-mvp-decision.md`) and are specified there until the OpenAPI document is validated. |
| DEF-06 | - | - | API tokens and any non-session authentication. Not allowed in the MVP (D-60). |
| DEF-07 | - | - | Evidence upload by the researcher. Not in the PRD. |
| DEF-08 | - | - | Server-sent events or WebSockets for job progress. Polling is the MVP choice (P-12). |

## Internal worker use cases (never routes)

The worker process has no network listener (P-17). It reaches the database through application use cases. These operations are listed so each one has an owner and a data mapping, and so nobody exposes them as HTTP routes.

| ID | Operation | Transaction boundary | Tables | PRD |
| --- | --- | --- | --- | --- |
| WRK-01 | Claim a job | One transaction: SELECT FOR UPDATE SKIP LOCKED, E2 decision, conditional approval use update, approval_uses insert, lease and claim_epoch update, job_transitions insert, audit | jobs; approvals; approval_uses; policy_decisions; job_transitions; audit_events | FR-JOB-001, FR-APPROVAL-002 |
| WRK-02 | Heartbeat and renew lease | One short fenced update conditioned on job id, claim_epoch and state running | jobs | FR-JOB-001 |
| WRK-03 | Record attempt launch (E3) | One transaction: E3 decision, conditional execution_attempts increment, job_attempts insert (unique unconfirmed attempt), audit | jobs; job_attempts; policy_decisions; audit_events | FR-APPROVAL-002, FR-TOOL-002 |
| WRK-04 | Commit results | One short fenced transaction: observations, evidence metadata and status, attempt end, job transition, audit | observations; evidence_items; evidence_status; job_attempts; jobs; job_transitions; audit_events | FR-EVIDENCE-001, FR-ASSET-001 |
| WRK-05 | Confirm termination | Update of termination_confirmed_at after the process tree is verified gone | job_attempts; jobs; job_transitions | FR-APPROVAL-003 |
| WRK-06 | Observe stop request and acknowledge | Reads control_state; records stop_confirmed or stop_failed | jobs; job_transitions; audit_events | FR-APPROVAL-003 |
| WRK-07 | Lease reaper | Fails intrusive jobs on lease expiry, requeues retryable jobs below the attempt maximum once termination is confirmed | jobs; job_attempts; job_transitions; audit_events | FR-JOB-001, FR-APPROVAL-002 |
| WRK-08 | Expiry and retention sweeps | Marks expired approvals observed, validations expired, labs expired, sessions cleaned, orphan evidence files reported | approvals; approval_events; target_validations; lab_registrations; sessions; idempotency_keys; evidence_status | FR-APPROVAL-003, FR-SCOPE-005 |
| WRK-09 | Write AI transmission record | Audit event first, then ai_transmissions, then send | audit_events; ai_transmissions | FR-AI-004 |
