# MORN1NGST4R Product Requirements Document

## 3.1 Document Control

| Field | Value |
| --- | --- |
| Product | MORN1NGST4R (canonical technical name `morningstar`) |
| Document | Product Requirements Document |
| Version | 0.2 |
| Status | Draft. Foundation decisions closed in Round 6 (D-63 to D-85). D-86 (record 0004) keeps the health endpoints in the MVP. Nothing is implemented except the Phase 1 scaffold. |
| Last updated | 2026-11-16 |
| Implementation state | The Phase 1 scaffold exists: a Go server skeleton with the two health endpoints (NFR-OBS-003), configuration loading and a Next.js shell. No other requirement in this document is implemented. Two throwaway feasibility spikes exist (`scripts/spikes/`). |
| Owner | The project owner (single decision authority) |

**Source documents.** Listed in precedence order (the order this document follows when sources differ).

1. Owner decisions: `docs/decisions/0001-owner-security-decisions-round-2.md`, `docs/decisions/0002-foundation-completion-decisions.md`, `docs/decisions/0003-foundation-closure-decisions.md`, `docs/decisions/0004-health-endpoints-mvp-decision.md` (D-86), and the decision register in `.kiro/steering/architecture.md` section 19 (D-01 to D-85).
2. Product steering: `.kiro/steering/product.md`.
3. Steering: `.kiro/steering/architecture.md`, `security.md`, `structure.md`.
4. Technology: `.kiro/steering/tech.md` and `docs/development/tech-verification/`.
5. Plan: `.kiro/specs/mvp/plan.md`.
6. Evidence: `docs/security/threat-model.md`, `docs/security/feasibility/sandbox-spike.md`, `docs/security/feasibility/egress-spike.md`.
7. Proposals (not decisions): `docs/decisions/pending/` (`S1-security-semantics.md`, `C-product-decisions.md`, `F-password-recovery-proposal.md`), and every P- and O- entry in the register.

**Identifier conventions.**

| Prefix | Meaning | Defined in |
| --- | --- | --- |
| `FR-<AREA>-nnn` | Functional requirement | this document |
| `NFR-<AREA>-nnn` | Non-functional requirement | this document |
| `SR-nn` | Security requirement summary row (points to FR and NFR) | this document |
| `UX-nnn` | Product-level experience requirement | this document |
| `SM-nn` | Success metric | this document |
| `CF-nn`, `PM-nn` | Source conflict, plan mismatch found while writing this document | this document |
| `D-nn` | Decided (owner statement or approved decision) | architecture.md section 19 |
| `P-nn` | Proposed (a recommendation, not a decision) | architecture.md section 19 |
| `O-nn` | Open (unresolved) | architecture.md section 19 |
| `DR-nn` | Round 2 owner decision item | record 0001 |
| `T-nn` | Threat | `docs/security/threat-model.md` |
| `M0` to `M25`, `M1b`, `M22b` | Implementation milestone | `.kiro/specs/mvp/plan.md` |
| `SEC n`, `S n`, `P n` | Section n of `security.md`, `structure.md`, `product.md` | steering files |

**Requirement basis tags** used in the Source field of every requirement.

| Tag | Meaning |
| --- | --- |
| Decided | An owner decision or an approved steering statement exists |
| Proposed | Only a P- entry or a pending proposal exists. It is not approved. |
| Conditional | The direction is decided, the mechanism depends on a validation result |
| Open | An O- entry exists and no choice was made. The text is marked `OPEN — OWNER DECISION REQUIRED`. After Round 6 no requirement carries this marker. Eight decisions stay open as gates, listed in 3.17. |
| Blocked | Cannot proceed until the named open decision or validation is settled |

**MVP status values.** Planned (Mx) means a milestone of the plan covers it. Not in plan (PM-nn) means the plan names no milestone for it. After Round 6 every requirement has a milestone or is Out of MVP. Out of MVP means excluded. Every status is "not implemented", except NFR-OBS-003, which is partly implemented by the Phase 1 scaffold.

## 3.2 Executive Summary

MORN1NGST4R is a local-first, AI-assisted security assessment platform for independent bug bounty hunters and security researchers. Its core philosophy is evidence-driven security assessment with human-verified findings.

It serves one researcher who works on authorized programs and wants one place to keep scope, assets, controlled tests, evidence and reports. It addresses fragmented tooling, weak evidence handling and the gap between a scanner result and a verified vulnerability. Its differentiator is evidence validation: every finding is traceable to preserved evidence, and only a human can mark it Verified (D-09, `P 3`).

The first MVP delivers one complete, authorized assessment on a Linux localhost web application: register a program and scope, validate authorization, run passive reconnaissance and HTTP probing through approved tools, keep an asset inventory, collect integrity-checked evidence, review candidate findings, verify them as a human, and export a Markdown report. The MVP is limited to what `product.md` section 8 and the plan accept. Several capabilities remain proposals or open decisions and are marked as such below.

## 3.3 Problem Statement

Researchers work with disconnected tools and manual steps. The product aims to reduce these specific problems (`P 4`):

- **Fragmented workflow.** Reconnaissance, probing, testing, evidence and reporting live in separate tools and notes.
- **Scope and asset management.** Scope rules differ per program and are easy to misread. Discovered assets are mixed with authorized targets.
- **Weak evidence.** Requests, responses and tool output are scattered, with no integrity record and no provenance.
- **Inconsistent authorization and execution control.** Tools run with the researcher's full rights, and nothing records who allowed what.
- **Candidates versus verified findings.** Scanner and AI output is easily mistaken for a confirmed vulnerability.
- **No single path from reconnaissance to report.**

**Limits of the claim.** The MVP does not replace the researcher's judgment, does not prove ownership of a target, and does not provide autonomous exploitation. The platform does not make a tool safe. It restricts and records what runs.

## 3.4 Product Vision and Goals

`P 5` defines four pillars. Each is a capability area. The MVP delivers a subset.

| Pillar | Purpose | MVP depth |
| --- | --- | --- |
| 1. Evidence Validation (primary differentiator) | Collect, preserve and verify evidence, track validation and keep human verification explicit | Core. Required for the MVP. |
| 2. Asset Intelligence and Attack Surface Mapping | Domain, IP, service, HTTP service, technology, endpoint inventory with observation history | Inventory and observations. User-triggered refresh only (D-76). |
| 3. Advanced Security Assessment | Authentication, authorization, API, JWT, input validation, business logic, common web vulnerabilities | Incremental. The MVP offers passive reconnaissance, HTTP probing and one optional template check class. Advanced business logic is excluded (`P 9`, D-13). |
| 4. AI Agent Orchestration | Planning, analysis, recommendations and drafting under backend policy | Provider abstraction and basic assisted analysis. No autonomy. |

**How they fit the workflow.** Pillar 2 produces observations about assets. Pillar 3 turns approved operations into results. Pillar 1 preserves and verifies what those operations produced. Pillar 4 proposes plans and drafts, and never executes, approves or verifies.

## 3.5 Target Users and Personas

No market research or demographic data was collected. The personas below describe roles and goals taken from `product.md`.

| Persona | Goals | Pain points | Expected workflow in the MVP |
| --- | --- | --- | --- |
| Independent bug bounty hunter (primary) | Find and report valid issues on programs they are allowed to test, with defensible evidence | Scope mistakes, tool sprawl, lost evidence, duplicate manual write-ups | Register a program, enter scope, validate authorization, recon, probe, review candidates, verify, export a report |
| Security researcher | Reproduce and document findings across programs or labs | Reproducibility, evidence integrity, consistent write-ups | Same workflow, with emphasis on evidence and validation |
| Individual application security practitioner | Assess systems they are authorized to test, including self-owned labs | No approval and audit trail for tool runs | Local lab registration, controlled probing, evidence and report |

All three use the product alone on one Linux machine. Multi-user operation, teams and hosted use are out of the MVP (`P 9`, D-20).

## 3.6 Product Principles

| Principle | Statement | Source |
| --- | --- | --- |
| Local-first | Data, tools and the worker run on the researcher's machine. No hosted service is required. | D-15, D-16 |
| Evidence-driven | Findings rest on preserved, integrity-checked evidence | `P 3`, D-10 |
| Human verification | Only a human assigns Verified | D-09, `SEC 5` |
| Default deny | An unknown, ambiguous or unmatched target is denied | `SEC 2`, D-38 |
| Fail closed | A missing control, mechanism or record blocks the operation | `SEC 1`, D-31, D-42 |
| Explicit approval | Risky operations need a bound, expiring, revocable approval | D-06, D-45 |
| AI cannot authorize | AI cannot approve, change policy, grant itself access or run tools | D-07, `SEC 5` |
| Candidates stay candidates | Scanner and AI results are candidates until verified | `P 3`, `P 7` |
| Two separate statuses | Finding validation status and report status are separate | `P 7`, D-09 |

## 3.7 Product Workflow

Workflow of `P 6`, with the stages named in the owner's request:
Program registration → Scope validation → Reconnaissance → Asset inventory → Controlled testing → Evidence collection → Finding validation → Human verification → Reporting.

The MVP status of each stage is the status of the requirements it cites (section 3.8 and the milestone table in 3.16). Nothing is implemented.

### Stage 1: Program registration
- **User goal:** record a program, its rules and its rules reference.
- **System responsibility:** store the program, version the rules text, require every assessment to belong to a program (FR-PROG-001, FR-PROG-002).
- **Inputs:** name, rules text, rules reference.
- **Outputs:** a program record with a rules version.
- **Security controls:** authenticated session, audit of changes, rules text treated as untrusted content for display and AI (T-42).
- **Failure conditions:** unauthenticated request, invalid input. The assessment cannot be created.

### Stage 2: Scope validation
- **User goal:** define authorized targets and exclusions, then confirm authorization.
- **System responsibility:** canonicalize rules, refuse unsupported rules, evaluate deterministically, record the three-part authorization validation (FR-SCOPE-001 to FR-SCOPE-007).
- **Inputs:** scope rules, program rules reference, researcher attestation.
- **Outputs:** a scope rule set and a validation record bound to the target-set hash and rules version.
- **Security controls:** default deny, exclusions first, wildcard inclusion refused through the MVP (D-73), AI cannot attest, change makes validation stale.
- **Failure conditions:** an ambiguous or refused rule, an unacknowledged refused exclusion, a stale validation. The assessment is not ready.

### Stage 3: Reconnaissance
- **User goal:** discover names and hosts for in-scope targets.
- **System responsibility:** run passive recon through an approved tool from sources a human enabled for the program (FR-RECON-001).
- **Inputs:** in-scope names, enabled sources.
- **Outputs:** observations. A discovered asset is not a testable target.
- **Security controls:** policy evaluation, source enablement per program, disclosure of what is sent, sandbox, policy proxy.
- **Failure conditions:** source not enabled, sandbox or egress gate not passed, out-of-scope result. The run is refused or the result stays an observation.

### Stage 4: Asset inventory
- **User goal:** see and filter what was found.
- **System responsibility:** store assets and endpoints with an append-only observation history, flag assets that have not passed scope evaluation (FR-ASSET-001 to FR-ASSET-003).
- **Inputs:** tool results.
- **Outputs:** inventory entries with origin and time.
- **Security controls:** tool output is untrusted, strict parsing, promotion to a target needs scope evaluation.
- **Failure conditions:** parse failure. The job fails and no partial result is trusted.

### Stage 5: Controlled testing
- **User goal:** run limited, approved probing and checks.
- **System responsibility:** classify the operation, evaluate policy, bind and consume an approval, run it in the sandbox through the egress path (FR-APPROVAL-001 to FR-APPROVAL-006, FR-JOB-001, FR-TOOL-001 to FR-TOOL-004, FR-PROBE-001).
- **Inputs:** a plan item or operation, an approval where required.
- **Outputs:** a job result and raw artifacts.
- **Security controls:** two policy checks (before scheduling, before launch), expiry, revocation, 30-second stop objective, resource limits, one launch for intrusive operations.
- **Failure conditions:** denied policy decision, missing or stale approval, sandbox self-test failure, unavailable required limit, missing numeric limits (O-38). The operation does not run.

### Stage 6: Evidence collection
- **User goal:** keep what a test produced.
- **System responsibility:** write evidence once, hash it, record provenance (FR-EVIDENCE-001, FR-EVIDENCE-002).
- **Inputs:** tool output, request and response data.
- **Outputs:** evidence items with SHA-256 and provenance.
- **Security controls:** redaction rules (O-08 open), owner-only storage, no replacement of an existing item.
- **Failure conditions:** write failure, hash mismatch on read. The item is flagged and cannot support Verified.

### Stage 7: Finding validation
- **User goal:** decide whether a candidate is real.
- **System responsibility:** store candidates, link evidence, record reproduction attempts (FR-FINDING-001, FR-FINDING-002).
- **Inputs:** candidates, evidence, AI recommendations.
- **Outputs:** a validation result and recommendation.
- **Security controls:** automation can create Candidates and attach recommendations only.
- **Failure conditions:** inconclusive or non-reproducible results stay recorded as such.

### Stage 8: Human verification
- **User goal:** confirm a finding.
- **System responsibility:** accept only a human actor's Verified transition (FR-FINDING-003).
- **Inputs:** evidence and the validation record.
- **Outputs:** a Verified finding with actor and time.
- **Security controls:** AI, workers and tools cannot set Verified. Proposed rule: linked evidence must pass its integrity check (C item 6).
- **Failure conditions:** failed evidence integrity, missing validation record. The transition is refused.

### Stage 9: Reporting
- **User goal:** produce a usable report.
- **System responsibility:** generate a Markdown report from verified findings with evidence references, with report status separate from finding status (FR-REPORT-001 to FR-REPORT-003).
- **Inputs:** verified findings, evidence references.
- **Outputs:** an exported report with a content hash (proposed).
- **Security controls:** untrusted values escaped, redaction by default, export audited.
- **Failure conditions:** unverified findings are excluded by default. Export failure leaves the status unchanged.

## 3.8 Functional Requirements

Priorities: Must, Should, Could, Won't (for the MVP). "Planned (M1)" means the plan's milestone M1 covers it. Source tags follow section 3.1.

### Programs

#### FR-PROG-001 Program registration
- **Requirement:** The system shall let the authenticated researcher register a security program and shall require every assessment to belong to a registered program.
- **User value:** One place for the authorization context of an assessment.
- **Priority:** Must
- **Acceptance criteria:** AC1 Creating an assessment without a program is rejected. AC2 A program stores a name and rules text. AC3 Creating and editing a program produces an audit event with the actor.
- **Dependencies:** FR-AUTH-001, FR-AUDIT-001.
- **Security:** Program text is untrusted when displayed or sent to a provider (T-42).
- **MVP status:** Planned (M3 schema, M16 use cases, M24 screens, M15).
- **Source:** Decided. `P 6` step 1, `P 8`, `SEC 2`, structure.md 5.1.

#### FR-PROG-002 Program rules reference and version
- **Requirement:** The system shall store a reference to the program rules (URL or identifier) and shall create a new rules version whenever the rules text changes.
- **User value:** The attestation points to the rules it was made against.
- **Priority:** Must
- **Acceptance criteria:** AC1 A rules edit creates a new version identifier. AC2 The validation record stores the rules version it was bound to. AC3 A rules edit marks the validation record stale (FR-SCOPE-006).
- **Dependencies:** FR-PROG-001, FR-SCOPE-005.
- **Security:** The reference is a statement, not proof of authorization (T-13).
- **MVP status:** Planned (M2 domain, M3 schema, M16 use cases, M18 validation).
- **Source:** Decided. D-61, C item 5.

### Scope

#### FR-SCOPE-001 Scope rule registration
- **Requirement:** The system shall accept scope rules of four kinds only (domain, wildcard, IP, CIDR), each as in-scope or exclusion, store the canonical form with the original, and refuse every other construct.
- **User value:** Scope is unambiguous and cannot be widened by odd syntax.
- **Priority:** Must
- **Acceptance criteria:** AC1 A wildcard inclusion such as `*.example.com` is refused (D-73), while a wildcard exclusion is accepted. AC2 Numeric hosts `127.1`, `2130706433`, `0x7f.0.0.1`, `0177.0.0.1`, `127.000.000.001`, `1.2.3.256`, `1.2.3.4.5` and `foo.0x7f` are refused as ambiguous. AC3 `Example.COM.` is stored as `example.com`, while `example.com..` and `.` are refused. AC4 Ports `0`, `65536`, `08080` and `80,80` are refused. AC5 An in-scope CIDR such as `198.51.0.0/16`, `0.0.0.0/0`, `10/8` or `203.0.113.5/24` is refused, while `203.0.113.0/24` is accepted and a `/16` exclusion is accepted. AC6 A path, regular expression or scheme in a rule value is refused. AC7 A refused exclusion is never dropped silently: the assessment is not ready until a human acknowledges it or replaces it with a supported exclusion that denies at least as much.
- **Dependencies:** none for names and literals. Host conversion is FR-SCOPE-007.
- **Security:** Parsing differences are a scope bypass (T-01 to T-04).
- **MVP status:** Planned (M1 core, M16 registration use case).
- **Source:** Decided. D-34 to D-38, D-57, D-64 (choices 1, 6 and 10), S1.1, `SEC 2`.

#### FR-SCOPE-002 Deterministic scope evaluation
- **Requirement:** The system shall evaluate a target against the rule set deterministically: exclusions first, then in-scope rules, with default deny, and shall deny any ambiguous or zero-value target.
- **User value:** The same inputs always give the same decision.
- **Priority:** Must
- **Acceptance criteria:** AC1 An exclusion with no port denies every port, including a port an in-scope rule lists. AC2 An in-scope rule with no port allows only 80 and 443. AC3 An exclusion `*.x.example.com` does not cover `x.example.com`, and a wildcard exclusion covers `a.b.x.example.com`. AC4 For any rule set, adding an exclusion never turns a denial into an allow. AC5 Normalizing a host or prefix twice gives the same result. AC6 A zero-value target is denied.
- **Dependencies:** FR-SCOPE-001.
- **Security:** Hostname exclusions do not cover access by IP address. This residual risk is accepted and stated (T-05).
- **MVP status:** Planned (M1).
- **Source:** Decided. D-36, D-38, D-57, S1.1; T-01, T-05, T-06.

#### FR-SCOPE-003 Resolved-address and redirect evaluation
- **Requirement:** The system shall evaluate every address a target name resolves to (A, AAAA and CNAME chains), deny the target if any address is denied, connect only to an address that passed, and treat every redirect hop as a new target.
- **User value:** An in-scope name cannot be used to reach internal or metadata addresses.
- **Priority:** Must
- **Acceptance criteria:** AC1 A name with one allowed and one denied address is denied. AC2 A name whose AAAA answer is denied is denied. AC3 A redirect to `169.254.169.254` or `[fd00:ec2::254]` in absolute, scheme-relative (`//host`) and backslash forms is not followed. AC4 A redirect that cannot be evaluated is not followed. AC5 A second DNS answer that differs from the checked one is never used for the connection. AC6 The IANA globally reachable flag never authorizes an address by itself. AC7 An address assigned to a local interface is denied unless a lab authorization allows it.
- **Dependencies:** FR-SCOPE-002, FR-TOOL-003.
- **Security:** T-07 to T-10. The deny tables are generated from the IANA registries with a pinned source version.
- **MVP status:** Conditional. Spike PASS on the reference host only. Planned (M9). Gate G-egress.
- **Source:** Decided requirement, Conditional mechanism. D-39, D-40, D-41, D-54, D-57, D-72, S1.2. The metadata list is decided (D-72). The table generation procedure and the review of other provider addresses are M9 work.

#### FR-SCOPE-004 Lab authorization
- **Requirement:** The system shall allow a denied address class only for the exactly registered lab target and its bound permitted range, under an authenticated human registration with program and assessment binding, expiry and revocation.
- **User value:** Local labs and self-owned systems can be tested without weakening scope for real targets.
- **Priority:** Must
- **Acceptance criteria:** AC1 A hostname lab that resolves outside its permitted range is denied. AC2 A public hostname that resolves into a registered lab range does not inherit the registration. AC3 A loopback registration, a hostname resolving to loopback and a registration whose address is on a local interface are refused without explicit ports. AC4 Registering the application's own API, web server or database endpoint is refused. AC5 A lab registration cannot authorize link-local, metadata, unspecified, multicast, broadcast, reserved or application-owned addresses. AC6 Lab expiry, removal or revocation stops affected jobs within 30 seconds. AC7 A redirect into a lab target is denied. AC8 A lab registration never replaces the in-scope rule. AC9 Every registration change is audited.
- **Dependencies:** FR-AUTH-001, FR-SCOPE-003, FR-AUDIT-001.
- **Security:** T-11, T-12.
- **MVP status:** Planned (M2 domain, M9 deny tables, M17 lifecycle).
- **Source:** Decided. D-32, D-47, D-54, D-56, D-64 (choice 11), `SEC 2`.

#### FR-SCOPE-005 Target authorization validation
- **Requirement:** The system shall require, for each assessment, a validation record made of the mechanical scope validation, a stored program-rule reference and a human attestation, bound to the canonical target-set hash and the rules version, before any active operation.
- **User value:** A recorded, accountable statement of authorization before testing.
- **Priority:** Must
- **Acceptance criteria:** AC1 An active operation without a current validation record is denied. AC2 An attestation by an AI actor is refused. AC3 A lab target requires the lab-specific attestation wording. AC4 The record stores actor, time, target-set hash and rules version. AC5 The user interface does not describe the record as proof of ownership.
- **Dependencies:** FR-PROG-002, FR-SCOPE-001, FR-AUTH-001.
- **Security:** The attestation is a statement that supports accountability and does not prevent misuse (T-13). Proof of control is deferred.
- **MVP status:** Planned (M2 domain, M18 workflow, M24 screen).
- **Source:** Decided. D-61, D-69 (24-hour expiry, stops new launches only), C item 5, `P 6` step 4.

#### FR-SCOPE-006 Invalidation on change
- **Requirement:** The system shall treat any change to scope, program rules or a lab registration, widening or narrowing, as invalidating the validation record and the authorizations derived from it, and shall re-evaluate names whose resolved addresses change.
- **User value:** A running assessment cannot continue on authority that no longer holds.
- **Priority:** Must
- **Acceptance criteria:** AC1 Widening a rule makes the validation record stale. AC2 Narrowing a rule makes it stale and cancels queued jobs. AC3 Running jobs affected by the change stop within 30 seconds. AC4 A changed DNS answer forces a new evaluation before the next connection.
- **Dependencies:** FR-SCOPE-005, FR-JOB-001.
- **Security:** T-14, T-15.
- **MVP status:** Planned (M2, M7, M18). The 30-second figure is an objective until measured (O-35, gate G-revoke).
- **Source:** Decided. D-48, D-56, `SEC 2`, `SEC 3`.

#### FR-SCOPE-007 Host name conversion
- **Requirement:** The system shall convert internationalized host names to canonical ASCII in the application layer with a strict, pinned profile, refuse mixed-script labels by default, and pass only canonical ASCII to the domain.
- **User value:** Look-alike names are not treated as in-scope names.
- **Priority:** Should
- **Acceptance criteria:** AC1 `fuß.de` becomes `xn--fu-hia.de`. AC2 An ideographic full stop and a soft hyphen are handled as the profile defines. AC3 An invalid `xn--` label is ambiguous and denied. AC4 A mixed-script label is refused at registration unless an explicit, audited, narrow override exists. AC5 Names of 253 octets pass and of 254 octets are denied.
- **Dependencies:** FR-SCOPE-001.
- **Security:** T-03, T-04.
- **MVP status:** Planned (M1b). Gate O-33 corpus (D-73). Until M1b every non-ASCII name is denied as ambiguous.
- **Source:** Decided requirement. D-37, D-73 (single-script rule instead of UTS #39 levels), P-57 modified, S1.1.

### Assessments, policy and approvals

#### FR-ASSESS-001 Assessment management
- **Requirement:** The system shall let the researcher create an assessment under a program, move it through defined lifecycle states, and start it only when its validation record is current.
- **User value:** A structured container for one assessment.
- **Priority:** Must
- **Acceptance criteria:** AC1 An assessment cannot start without a current validation record. AC2 An assessment with an unacknowledged refused exclusion is not ready. AC3 State changes are audited.
- **Dependencies:** FR-PROG-001, FR-SCOPE-005.
- **Security:** Starting an assessment is a policy-relevant action and needs an authenticated human. States are Draft, Ready (current validation record and no unacknowledged refused exclusion), Running, Paused (no new launches, queued jobs held), Completed and Cancelled (D-83).
- **MVP status:** Planned (M3 schema, M16 records and target set, M21 lifecycle transitions, M24 screens, M15).
- **Source:** Decided requirement. `P 8`, structure.md 5.1 (create, start, pause, complete), D-61, D-83 (states: Draft, Ready, Running, Paused, Completed, Cancelled).

#### FR-POLICY-001 Policy evaluation at enforcement points
- **Requirement:** The system shall evaluate every security operation with one policy engine before scheduling and again immediately before launch, and shall deny when data is missing or evaluation errors.
- **User value:** No path executes work that policy did not allow.
- **Priority:** Must
- **Acceptance criteria:** AC1 Creating a job for an out-of-scope target is denied and audited. AC2 Narrowing scope after scheduling makes the launch check deny. AC3 An evaluation error returns deny. AC4 A tool launcher given no authorization value refuses to launch.
- **Dependencies:** FR-SCOPE-002, FR-APPROVAL-001.
- **Security:** T-14, T-24. The `AuthorizedOperation` mechanism (P-04) is Proposed.
- **MVP status:** Planned (M2, M7).
- **Source:** Decided. D-05, D-28, `SEC 3`.

#### FR-POLICY-002 Policy change control
- **Requirement:** The system shall allow policy changes only by an authenticated human through one canonical policy store, shall audit every change with actor, old value, new value, time and reason, and shall require password re-entry for changes that widen what can be tested or sent.
- **User value:** Policy cannot be altered silently or by automation.
- **Priority:** Must
- **Acceptance criteria:** AC1 A change request from an AI, worker or API-token actor is refused and audited. AC2 The audit event contains the old and new value. AC3 A widening change without recent password entry is refused.
- **Dependencies:** FR-AUTH-001, FR-AUDIT-001.
- **Security:** T-20, T-21. Same-user processes remain a residual risk (T-61).
- **MVP status:** Planned (M2, M4, M5, M16). The store is versioned PostgreSQL tables behind one `PolicyStore` port (D-64 choice 4).
- **Source:** Decided. D-53, `SEC 3`.

#### FR-APPROVAL-001 Approval record and binding
- **Requirement:** The system shall create approvals bound to the exact target, operation, parameters, risk class, execution limits, canonical operation hash, declared destinations, approver, timestamp, expiry, use count, maximum duration and maximum attempts.
- **User value:** An approval cannot be reused for something different.
- **Priority:** Must
- **Acceptance criteria:** AC1 An approval for target A does not authorize target B. AC2 A changed parameter changes the operation hash and invalidates the approval. AC3 An approval created by an AI, worker or API token is refused.
- **Dependencies:** FR-APPROVAL-004.
- **Security:** T-16, T-18.
- **MVP status:** Planned (M2, M3, M19).
- **Source:** Decided. D-06, D-30, D-51, `SEC 4`.

#### FR-APPROVAL-002 Approval consumption and attempts
- **Requirement:** The system shall consume one approval use per (approval, job) atomically when the worker claims the job, allow at most three execution attempts in total for approval-bound active jobs, and give intrusive operations exactly one process launch.
- **User value:** Redelivery and retries cannot multiply what was approved.
- **Priority:** Must
- **Acceptance criteria:** AC1 With two workers claiming one job, exactly one consumption exists for the pair. AC2 A second consumption for the same pair is impossible by database constraint. AC3 A retry claim reuses the original record. AC4 A denial before launch is not counted as an attempt and does not restore the use. AC5 An intrusive job whose lease expires is failed, not retried. AC6 A rerun of an intrusive operation needs a new job and a new approval. AC7 A retry launch does not start before the previous attempt's process tree is confirmed terminated.
- **Dependencies:** FR-APPROVAL-001, FR-JOB-001.
- **Security:** T-16, T-17.
- **MVP status:** Planned (M2, M3, M7, M19). Gate G-consume.
- **Source:** Decided. D-30, D-49, D-55, D-64 (choice 12), `SEC 4`.

#### FR-APPROVAL-003 Expiry, revocation and running work
- **Requirement:** The system shall treat expiry as preventing new launches only, shall stop a launched operation at its approved maximum duration, and shall stop queued and running work within 30 seconds of revocation or a scope or lab change.
- **User value:** Withdrawing authority takes effect quickly.
- **Priority:** Must
- **Acceptance criteria:** AC1 An expired approval refuses launch. AC2 A launched operation is killed at its maximum duration. AC3 After revocation, queued jobs are cancelled and running process trees are gone within 30 seconds, measured on the reference host (gate G-revoke).
- **Dependencies:** FR-JOB-001, FR-TOOL-002.
- **Security:** T-15.
- **MVP status:** Conditional. Heartbeat and kill timing are unmeasured (O-35, gate G-revoke). Planned (M7, M19).
- **Source:** Decided requirement. D-48, `SEC 3`, `SEC 4`.

#### FR-APPROVAL-004 Approval classes
- **Requirement:** The system shall classify every operation as Passive, Low-impact or Intrusive from the reviewed tool manifest and policy engine, shall deny an operation with no reviewed classification, and shall ignore any class supplied by an AI.
- **User value:** The level of control matches the risk.
- **Priority:** Must
- **Acceptance criteria:** AC1 Passive reconnaissance classifies as Passive. AC2 HTTP probing and safe-method replay classify as Low-impact. AC3 State-changing replay and known-vulnerability template checks classify as Intrusive. AC4 An unknown operation is denied. AC5 A class value in an AI proposal has no effect.
- **Dependencies:** FR-TOOL-001.
- **Security:** T-22.
- **MVP status:** Planned (M2, M10).
- **Source:** Decided. D-59, C item 9.

#### FR-APPROVAL-005 Approval interface and re-authentication
- **Requirement:** The system shall render the approval screen only from the canonical operation, shall never show AI-written text as the basis for consent, and shall require password re-entry for intrusive approval.
- **User value:** The researcher approves exactly what will run.
- **Priority:** Must
- **Acceptance criteria:** AC1 A test with injected AI text shows that none appears in the approval basis. AC2 An approve request from an AI actor, a worker or without a session is refused. AC3 Intrusive approval without password entry is refused, unless a window of at most five minutes was enabled by an explicit policy change (D-69). AC4 With no window configured, which is the default, password entry is required every time. AC5 Target strings are escaped and length-capped.
- **Dependencies:** FR-AUTH-001, FR-APPROVAL-001.
- **Security:** T-18, T-19, T-20. Framing denial is required on every response (D-79).
- **MVP status:** Planned (M4 backend, M19 use case, M24 screen). Default is password entry every time (D-69).
- **Source:** Decided. D-45, D-58, D-60, D-69, D-79, S1.6.

#### FR-APPROVAL-006 Fan-out limits
- **Requirement:** The system shall bind concurrency, total duration and request rate in each approval and shall not let jobs under one approval multiply the approved rate.
- **User value:** One approval cannot become unbounded traffic.
- **Priority:** Must
- **Acceptance criteria:** AC1 Parallel jobs under one approval never exceed the approved request rate in aggregate. AC2 An active operation with no established numeric limits is blocked.
- **Dependencies:** FR-APPROVAL-001.
- **Security:** T-23.
- **MVP status:** Blocked by the numeric-limit gate (D-71, O-38). Planned (M2, M11).
- **Source:** Decided requirement. D-55, D-71.

### Authentication

#### FR-AUTH-001 Local account and session
- **Requirement:** The system shall provide one local researcher account protected by a memory-hard password hash, server-side sessions stored hashed, an HttpOnly cookie, CSRF protection and rate-limited, audited login, with no API tokens.
- **User value:** Every approval and audit record has an authenticated human actor.
- **Priority:** Must
- **Acceptance criteria:** AC1 More than 5 failed login attempts within a rolling 15 minutes are refused until the window clears, and each is audited (policy default, D-68). AC2 No endpoint creates an API token. AC3 A stored session token is not recoverable from the database. AC4 Revoking a session invalidates it immediately. AC5 No credential appears in a URL. AC6 Account creation without the single-use bootstrap secret is refused, and the secret is never printed, logged or placed in a URL. AC7 A recovery code works once, a reset revokes every session, and a lost code has no bypass.
- **Dependencies:** FR-AUDIT-001, M3.
- **Security:** T-58, T-59, T-71. Recovery is a one-time recovery code through the web UI (D-68). First-run account creation uses a single-use bootstrap secret on a memory-backed file (D-68). Neither is implemented or tested.
- **MVP status:** Planned (M4).
- **Source:** Decided. D-60, D-68, D-69, `SEC 8`. The Argon2id values are starting values until the M4 benchmark (D-68).

#### FR-AUTH-002 Localhost request protection
- **Requirement:** The system shall bind its listeners to loopback explicitly, reject requests whose Host is not allow-listed, reject state-changing requests whose Origin is missing or not allow-listed, restrict CORS, and protect state-changing requests against CSRF.
- **User value:** A web page or another local user cannot act as the researcher.
- **Priority:** Must
- **Acceptance criteria:** AC1 The API and web server accept no connection on a non-loopback interface. AC2 A request with a foreign Host is rejected. AC3 A state-changing request without Origin is rejected. AC4 A cross-origin request without a valid CSRF token is rejected.
- **Dependencies:** FR-AUTH-001.
- **Security:** T-54 to T-57, T-68. Whether Next.js forwards a usable Host header through rewrites is not verified.
- **MVP status:** Conditional on the M13 check of Host and Origin through the rewrite (D-79). Planned (M4 server side, M13 browser side).
- **Source:** Decided. D-33, D-79, `SEC 8`, `SEC 15`.

### Jobs

#### FR-JOB-001 Persistent job lifecycle
- **Requirement:** The system shall run jobs from a persistent queue in a separate worker process, with leases, heartbeat, fencing of stale writers, timeouts and cancellation.
- **User value:** Work survives restarts and can be stopped.
- **Priority:** Must
- **Acceptance criteria:** AC1 A job whose worker is killed is redelivered or failed according to its class. AC2 A stale worker's late writes are rejected. AC3 Cancelling a job ends its tool process tree. AC4 The worker exposes no network listener.
- **Dependencies:** FR-AUDIT-001.
- **Security:** T-14, T-64.
- **MVP status:** Planned (M7).
- **Source:** Decided. D-03, D-62, D-80 (custom queue, P-36 fencing, bounded passive retries); `P 8`.

### Tools, reconnaissance and probing

#### FR-TOOL-001 Tool adapter framework
- **Requirement:** The system shall run tools only through adapters that declare a manifest (risk class, destinations, flags), validate every parameter against a schema, reject option and file injection, run a pinned binary by its verified bytes with an allow-listed environment, and store redacted arguments.
- **User value:** Tool use is controlled, replaceable and reviewable.
- **Priority:** Must
- **Acceptance criteria:** AC1 A host value such as `-config /etc/passwd`, `--output=/x` or `@/etc/passwd` is rejected. AC2 A secret is absent from arguments, environment and logs after a run. AC3 A binary whose hash differs is not launched. AC4 A binary replaced between hash and launch is not executed. AC5 A tool that cannot disable updates, telemetry or callbacks is unsupported.
- **Dependencies:** FR-TOOL-002, FR-TOOL-003.
- **Security:** T-33 to T-35, T-39, T-65.
- **MVP status:** Planned (M10). Exact-bytes launch is Conditional.
- **Source:** Decided. D-43, D-44, D-70, S1.4, `SEC 10`. Tool selection is P-52, adopted as a direction (D-76).

#### FR-TOOL-002 Sandbox isolation
- **Requirement:** The system shall run every tool process in a sandbox that isolates filesystem, network, process, IPC, capabilities and system calls, applies the required resource limits, and shall refuse to run when any required layer or limit is unavailable or the start-up self-test fails.
- **User value:** A hostile tool or target cannot reach the researcher's secrets.
- **Priority:** Must
- **Acceptance criteria:** AC1 The self-test uses the production arguments and refuses, naming the missing layer, in each simulated failure. AC2 A sandboxed process cannot read the worker's environment, memory or inherited descriptors. AC3 No flag, environment variable or lab registration disables isolation. AC4 A required limit that cannot be applied blocks the run.
- **Dependencies:** FR-JOB-001.
- **Security:** T-25 to T-32.
- **MVP status:** Conditional. Spike PASS on one reference host, other hosts NOT TESTED. Planned (M8, gate G-sandbox on a named baseline, O-13, O-22).
- **Source:** Decided requirement, Conditional mechanism. D-31, D-42, D-70, `SEC 10`, S1.3.

#### FR-TOOL-003 Egress enforcement
- **Requirement:** The system shall let a tool reach the network only through an approved path (first-party client or an isolated per-job policy proxy) that applies the address, port and redirect rules and refuses Host overrides.
- **User value:** Tools cannot reach targets or internal services outside scope.
- **Priority:** Must
- **Acceptance criteria:** AC1 A direct connection from the sandbox fails. AC2 The proxy refuses a denied address, a denied port and a mixed-answer name. AC3 A Host override is refused and audited, not neutralized. AC4 Each job has its own proxy socket. AC5 A running tunnel stops on revocation within 30 seconds.
- **Dependencies:** FR-TOOL-002, FR-SCOPE-003.
- **Security:** T-36 to T-40. The spike found the proxy neutralizes but does not refuse Host overrides, has no tunnel revocation hook and is reachable by any process with socket access.
- **MVP status:** Conditional. Planned (M9, gate G-egress, O-07).
- **Source:** Decided direction, Conditional mechanism. D-41, S1.2, `SEC 2`.

#### FR-TOOL-004 Template-based vulnerability checks
- **Requirement:** The system may offer template checks from a pinned, reviewed template store as an Intrusive operation after authentication and sandbox gates pass.
- **User value:** Known-issue checks inside the same controlled workflow.
- **Priority:** Could
- **Acceptance criteria:** AC1 The operation cannot be approved before FR-AUTH-001 re-authentication is implemented and tested. AC2 Templates come only from the pinned local store. AC3 Interactsh and update features are disabled.
- **Dependencies:** FR-APPROVAL-005, FR-TOOL-001.
- **Security:** T-65.
- **MVP status:** Blocked by the D-58 gate and the numeric-limit gate (D-71). Planned (M11, M15). Optional.
- **Source:** Decided class, optional tool. D-58, D-59, D-76.

#### FR-RECON-001 Passive reconnaissance
- **Requirement:** The system shall run passive reconnaissance only from sources that a human enabled for the program, shall show what target information goes to each source, and shall keep results as observations.
- **User value:** Discover names without touching the target.
- **Priority:** Must
- **Acceptance criteria:** AC1 A source that was not enabled for the program is blocked. AC2 Enabling a source is audited. AC3 The enablement screen lists the target information sent to the source. AC4 A discovered asset is not a testable target until it passes scope evaluation.
- **Dependencies:** FR-TOOL-001, FR-TOOL-003, FR-ASSET-001.
- **Security:** T-39, T-69. Passive volume and retry limits are undefined (O-12).
- **MVP status:** Planned (M11).
- **Source:** Decided. D-44, D-76, S1.5, `P 8`.

#### FR-PROBE-001 HTTP probing
- **Requirement:** The system shall probe in-scope HTTP services as a Low-impact operation within approved limits and evaluate every redirect.
- **User value:** Identify live services and technologies.
- **Priority:** Must
- **Acceptance criteria:** AC1 Probing an out-of-scope target is denied. AC2 Each redirect hop is evaluated before it is followed. AC3 The approved request rate is enforced. The numeric values are a measured gate (D-71, O-38).
- **Dependencies:** FR-APPROVAL-006, FR-TOOL-003.
- **Security:** T-08, T-23.
- **MVP status:** Blocked by the numeric-limit gate (D-71, O-38). Planned (M11).
- **Source:** Decided class, gated limits. D-59, D-71, `P 8`, D-76.

### Assets

#### FR-ASSET-001 Asset inventory
- **Requirement:** The system shall store domains, IP addresses, services, HTTP services and technologies with origin and an append-only observation history, and shall mark an asset that has not passed scope evaluation.
- **User value:** A reviewable picture of the attack surface.
- **Priority:** Must
- **Acceptance criteria:** AC1 A repeated run adds an observation and does not overwrite earlier ones. AC2 Each asset records its origin. AC3 A discovered asset is flagged as an observation until scope evaluation passes.
- **Dependencies:** FR-JOB-001, FR-SCOPE-002.
- **Security:** Tool output is untrusted (T-46, T-69).
- **MVP status:** Planned (M20, tool ingestion in M11).
- **Source:** Decided. `P 5.2`, `P 8`, architecture E4, D-76 (append-only observation history).

#### FR-ASSET-002 Endpoint and parameter inventory
- **Requirement:** The system shall store endpoints and parameters discovered for in-scope HTTP services with the same observation history.
- **User value:** A basis for planning tests.
- **Priority:** Must
- **Acceptance criteria:** AC1 Endpoints link to their service and observation time. AC2 An endpoint from an out-of-scope host is not stored as a target.
- **Dependencies:** FR-ASSET-001.
- **Security:** As FR-ASSET-001.
- **MVP status:** Planned (M20).
- **Source:** Decided. `P 5.2`, `P 8`, D-76.

#### FR-ASSET-003 Scheduled or continuous refresh
- **Requirement:** Scheduled or unattended inventory refresh is not part of the MVP. Refresh is started by the researcher.
- **User value:** Avoids unattended traffic.
- **Priority:** Won't (MVP)
- **Acceptance criteria:** AC1 No scheduler exists in the MVP. AC2 Every refresh is started by a human action and audited.
- **Dependencies:** none.
- **Security:** Unattended active traffic would break the approval model.
- **MVP status:** Out of MVP. `product.md` section 13 reads "continuous" as a persistent inventory (CF-02 resolved).
- **Source:** Decided. D-76, O-30.

### Evidence

#### FR-EVIDENCE-001 Evidence collection
- **Requirement:** The system shall store evidence items (request and response data, tool output) with SHA-256, provenance (tool, pinned version, redacted arguments, time, job) and owner-only permissions.
- **User value:** Every finding can be traced to what produced it.
- **Priority:** Must
- **Acceptance criteria:** AC1 Each item has a SHA-256 and provenance fields. AC2 Target cookies and authorization headers are not stored in clear text. AC3 Evidence files have owner-only permissions.
- **Dependencies:** FR-JOB-001.
- **Security:** T-49. Redacted evidence is the evidence of record, raw capture is off by default and needs a policy change, there is no encryption at rest in the MVP, and retention is by audited purge (D-77).
- **MVP status:** Planned (M6, capture from jobs in M11).
- **Source:** Decided. D-10, D-77, `P 5.1`, `SEC 9`. Write protocol P-37 adopted.

#### FR-EVIDENCE-002 Integrity verification
- **Requirement:** The system shall publish evidence write-once, never replace an existing item, and re-hash an item on read.
- **User value:** Tampering and corruption are detected.
- **Priority:** Must
- **Acceptance criteria:** AC1 A modified file is detected on read. AC2 Writing to an existing evidence name fails. AC3 An item that fails its check cannot support a Verified transition.
- **Dependencies:** FR-EVIDENCE-001.
- **Security:** The hash and file share a trust domain, so the check detects corruption and one-sided edits, not a determined local attacker (T-50).
- **MVP status:** Planned (M6).
- **Source:** Decided. D-10, D-77, `SEC 9`; P-37 adopted.

#### FR-EVIDENCE-003 Safe display and download
- **Requirement:** The system shall show and export evidence so that it cannot execute: CSP without inline scripts, nosniff, attachment downloads, escaping per format, and no tool output rendered as HTML in the application origin.
- **User value:** Hostile target content cannot attack the researcher.
- **Priority:** Must
- **Acceptance criteria:** AC1 Evidence containing a script tag renders inert. AC2 Responses carry the required headers. AC3 Evidence files download as attachments.
- **Dependencies:** FR-EVIDENCE-001.
- **Security:** T-47.
- **MVP status:** Planned (M13 headers, M24 viewer).
- **Source:** Decided. `SEC 9` (approved with B1), D-79 (framing denial).

### Findings

#### FR-FINDING-001 Finding lifecycle
- **Requirement:** The system shall keep each finding in one of the seven states of `P 7` and shall keep validation status separate from report status.
- **User value:** A finding's status says how far it has been checked.
- **Priority:** Must
- **Acceptance criteria:** AC1 An automated actor can create a Candidate, move it to Needs Review and attach a recommendation, and nothing else. A duplicate observation adds evidence and never changes or reopens a finding. AC2 An attempt by an AI, worker or tool to set Verified is refused and audited. AC3 Changing report status does not change finding status.
- **Dependencies:** FR-AUDIT-001.
- **Security:** T-44.
- **MVP status:** Planned (M22).
- **Source:** Decided. D-09, D-75 (transition table, deduplication), `P 7`.

#### FR-FINDING-002 Validation workflow
- **Requirement:** The system shall record validation attempts, link evidence, store a validation result and a recommendation, and keep a failed reproduction distinct from an inconclusive one.
- **User value:** Reproducible validation instead of memory and notes.
- **Priority:** Must
- **Acceptance criteria:** AC1 A validation result links to evidence identifiers. AC2 A Not Reproducible outcome references the recorded attempts (D-75). AC3 A recommendation from automation is labeled with its origin.
- **Dependencies:** FR-FINDING-001, FR-EVIDENCE-001.
- **Security:** Replays pass the same scope and approval checks through the first-party client (D-76).
- **MVP status:** Planned (M22 records, M22b replay).
- **Source:** Decided. `P 5.1`, `P 8`, architecture section 12, D-75, D-76.

#### FR-FINDING-003 Human verification
- **Requirement:** The system shall accept the Verified transition only from a human actor.
- **User value:** Accountability for what is called a vulnerability.
- **Priority:** Must
- **Acceptance criteria:** AC1 The transition records actor and time. AC2 The transition is refused when the linked evidence fails its integrity check or no validation record exists (D-75).
- **Dependencies:** FR-FINDING-002, FR-EVIDENCE-002, FR-AUTH-001.
- **Security:** T-44.
- **MVP status:** Planned (M22).
- **Source:** Decided. D-09, D-75, `SEC 5`, `P 7`.

#### FR-FINDING-004 Test planning
- **Requirement:** The system should persist plan items (target, operation, parameters, class, limits, origin) that a human or an AI proposal creates, and run them only after policy and approval.
- **User value:** Review AI suggestions before acting.
- **Priority:** Should
- **Acceptance criteria:** AC1 A plan item cannot become a job without a policy decision and, where required, an approval. AC2 An AI-originated item is labeled with its origin. AC3 An approved plan item is immutable.
- **Dependencies:** FR-APPROVAL-001, FR-AI-003.
- **Security:** T-41.
- **MVP status:** Planned (M19 plan items, M23 AI proposals).
- **Source:** Decided. D-80 (P-38 adopted), D-83, `P 6` step 8.

### AI

#### FR-AI-001 Provider abstraction and configuration
- **Requirement:** The system shall access AI providers through one provider-neutral port, keep AI off until a provider is configured, and send a provider's API key only to its configured endpoint.
- **User value:** No dependency on one vendor.
- **Priority:** Must
- **Acceptance criteria:** AC1 A fake provider satisfies the port in tests. AC2 Changing the provider changes no scope, approval or authorization decision. AC3 A stored key is never sent to another endpoint.
- **Dependencies:** FR-CRED-001, FR-AUDIT-001.
- **Security:** T-41, T-43.
- **MVP status:** Planned (M12, M25 for key storage).
- **Source:** Decided. D-07, D-50, D-81, `P 11`. Two adapters: loopback and generic OpenAI-compatible.

#### FR-AI-002 Remote provider disclosure
- **Requirement:** The system shall treat any provider endpoint that is not on loopback as remote, shall send target data, evidence or findings to a remote provider only after human configuration for that program and disclosure of what is sent, and shall never send target credentials, cookies, tokens or session secrets to any provider.
- **User value:** Control over what leaves the machine.
- **Priority:** Must
- **Acceptance criteria:** AC1 A provider reached through an SSH tunnel or forwarding proxy is classified remote. AC2 A provider whose hostname later resolves elsewhere is refused, not reclassified. AC3 No transmission happens when its audit record cannot be written. AC4 A secret search over prompts finds no target credential.
- **Dependencies:** FR-AI-001, FR-AUDIT-001.
- **Security:** T-43, T-63. A loopback model server can forward to a cloud service (the start-up check is decided, D-81). A trusted-LAN category does not exist (O-36, deferred).
- **MVP status:** Planned (M12).
- **Source:** Decided. D-46, D-50, D-81, `SEC 5`. Prompts are not stored (D-81).

#### FR-AI-003 Bounded AI proposals
- **Requirement:** The system shall treat AI output as untrusted typed proposals labeled with their origin, and shall give an AI no ability to approve, change policy or scope, run tools or set Verified.
- **User value:** AI helps without gaining authority.
- **Priority:** Must
- **Acceptance criteria:** AC1 No AI-reachable interface creates an approval or changes policy. AC2 An AI-originated finding is a Candidate. AC3 AI output is not shown as observed fact.
- **Dependencies:** FR-AI-001.
- **Security:** T-41, T-42, T-44.
- **MVP status:** Planned (M12, M23).
- **Source:** Decided. D-07, D-80 (P-11, P-32 adopted), `SEC 5`.

#### FR-AI-004 AI auditability
- **Requirement:** The system shall audit every AI transmission with provider, program, size and content digest, and make AI-assisted actions traceable to their origin.
- **User value:** A record of what the model saw and proposed.
- **Priority:** Must
- **Acceptance criteria:** AC1 Each call has an audit event without the content. AC2 A proposal accepted by a human records the proposing provider.
- **Dependencies:** FR-AUDIT-001.
- **Security:** T-53.
- **MVP status:** Planned (M5, M12).
- **Source:** Decided. `SEC 5`, `SEC 13`.

#### FR-AI-005 Basic AI-assisted analysis
- **Requirement:** The system should offer three assisted functions: summarize and group discovered assets, propose plan items, and draft finding or report text from evidence.
- **User value:** Saves routine analysis time.
- **Priority:** Should
- **Acceptance criteria:** AC1 Each function returns a typed proposal that executes nothing. AC2 Output is labeled with its origin.
- **Dependencies:** FR-AI-003.
- **Security:** Prompt injection from target content (T-42).
- **MVP status:** Planned (M23).
- **Source:** Decided scope. `P 8` ("Basic AI-assisted analysis"), D-81, the three functions of the requirement text.

### Reporting

#### FR-REPORT-001 Report generation
- **Requirement:** The system shall generate a Markdown report from Verified findings with evidence referenced by identifier and hash.
- **User value:** A usable report.
- **Priority:** Must
- **Acceptance criteria:** AC1 The report lists Verified findings only by default. AC2 Evidence appears by identifier and hash. AC3 A report can be produced without leaving the machine.
- **Dependencies:** FR-FINDING-003, FR-EVIDENCE-002.
- **Security:** T-48.
- **MVP status:** Planned (M14).
- **Source:** Decided. `P 8`, `P 12`, D-75 (Markdown, verified-only confirmed section).

#### FR-REPORT-002 Report status
- **Requirement:** The system shall keep a report status separate from finding status. A finding's report status is Not reported, In draft or In final. A report is Draft or Final, set by a human (D-75).
- **User value:** Reports and findings progress independently.
- **Priority:** Should
- **Acceptance criteria:** AC1 Changing report status changes no finding status. AC2 Only the human can mark a report Final, and an unverified candidate never appears in the confirmed section of a report.
- **Dependencies:** FR-REPORT-001.
- **Security:** none specific.
- **MVP status:** Planned (M14).
- **Source:** Decided. `P 7`, D-09, D-75 (report status values Not reported, In draft, In final; report states Draft and Final).

#### FR-REPORT-003 Export safety
- **Requirement:** The system shall escape untrusted values in reports, redact by default, and audit every export.
- **User value:** A report cannot carry an attack or leak a secret.
- **Priority:** Must
- **Acceptance criteria:** AC1 Untrusted values appear only in code spans or fences. AC2 A secret search over an exported report finds none. AC3 An export produces an audit event.
- **Dependencies:** FR-REPORT-001, FR-AUDIT-001.
- **Security:** T-48, T-63.
- **MVP status:** Planned (M14).
- **Source:** Decided. `SEC 9`, `SEC 13`, D-75.

#### FR-REPORT-004 Submission tracking
- **Requirement:** Tracking of report submission or acceptance by a program is not part of the MVP.
- **User value:** none claimed.
- **Priority:** Won't (MVP)
- **Acceptance criteria:** AC1 The MVP has no submitted or accepted report state.
- **Dependencies:** none.
- **Security:** none.
- **MVP status:** Out of MVP. Excluded by D-83.
- **Source:** None. Not in `P 8`. Excluded.

### Audit and credentials

#### FR-AUDIT-001 Audit trail
- **Requirement:** The system shall record structured audit events for the categories of `SEC 13`, and shall not start an operation when its audit record cannot be written, while cancellation, revocation and denials still take effect.
- **User value:** A record of who allowed and did what.
- **Priority:** Must
- **Acceptance criteria:** AC1 An operation does not start when the audit write fails. AC2 Revocation still takes effect when audit is down. AC3 The runtime database role cannot update or delete audit rows. AC4 Audit retries are bounded and exhaustion raises a persistent alert.
- **Dependencies:** M3.
- **Security:** T-51, T-52. Same-user processes can connect as another database role under peer authentication (T-61).
- **MVP status:** Planned (M5).
- **Source:** Decided requirement. `SEC 13`, D-12, D-23, D-48. The hash chain is deferred and accepted (D-78).

#### FR-CRED-001 Credential protection
- **Requirement:** The system shall store credentials only in the Linux Secret Service, fail closed when it is unavailable, and never deliver or store a secret in plaintext, environment variables, standard input, logs or URLs.
- **User value:** Keys and passwords are not left lying around.
- **Priority:** Must
- **Acceptance criteria:** AC1 With the Secret Service unavailable, credential features refuse to work and say what alternatives exist. AC2 A secret search over argv, environment, logs, evidence and reports finds none. AC3 No secret is written to PostgreSQL in plaintext.
- **Dependencies:** none.
- **Security:** T-62, T-63. Whether a worker outside a desktop session can use the Secret Service is not verified. Until it is, credential features are disabled (D-85).
- **MVP status:** Planned (M25). Gate: use outside a desktop session verified (D-85).
- **Source:** Decided. D-08, D-52, D-85, `SEC 6`, `SEC 7`.

## 3.9 Non-Functional Requirements

Targets appear only where a decision or evidence supports them.

| ID | Area | Requirement | Target | Source | Milestone |
| --- | --- | --- | --- | --- | --- |
| NFR-SEC-001 | Security | Every control fails closed. A missing mechanism, record or limit blocks the operation. | Binary | `SEC 1`, D-31, D-42 | M1 to M15 |
| NFR-SEC-002 | Security | Every security-critical area has regression tests that pass before release. | Gates G-arch, G-scope, G-consume, G-auth, G-sandbox, G-egress, G-revoke, G-redact | `SEC 16`, plan | M1 to M15 |
| NFR-SEC-003 | Security | Dependencies are pinned stable releases with a recorded licence review, source and verification date. | Binary | `SEC 14`, tech.md 14 | all |
| NFR-PRIV-001 | Privacy | Target data goes to a remote provider only after configuration and disclosure. | Binary | D-46, D-50 | M12 |
| NFR-PRIV-002 | Privacy | Evidence is access-restricted and sensitive data is minimized. Redacted evidence of record, raw capture off by default, no encryption at rest in the MVP, retention by audited purge (D-77). | Binary | `SEC 9`, D-77 | M6 |
| NFR-REL-001 | Reliability | Jobs are persistent and delivered at least once, with idempotent or non-retryable handlers. | Binary | arch section 14, D-03 | M7 |
| NFR-REL-002 | Reliability | An audit failure stops new operations but not cancellation, revocation or denials. | Binary | `SEC 13` | M5 |
| NFR-REL-003 | Reliability | Revocation, scope change and lab change stop affected work within 30 seconds, measured on the baseline. | 30 s (objective until measured) | D-48, O-35 | M7 |
| NFR-REL-004 | Reliability | After a detected database restore, authority recorded in the restored database (sessions, approvals, validations, labs, enablements, queued and running jobs) is revoked or cancelled before any operation runs. The detector is an external marker, not a database flag. Limits: same-user attackers and snapshots that restore the marker are not covered. | Binary (Proposed, DB-28, DQ-17) | `database/recovery-generation.md` | M3, M4 |
| NFR-PERF-001 | Performance | No numeric response-time, throughput or job-latency target is set for the MVP. Performance is measured and recorded, not promised. The only timing requirement is the 30-second stop objective (NFR-REL-003). | No target (decided: none) | D-83 | none |
| NFR-PERF-002 | Performance | Sandbox start-up overhead is recorded as evidence: about 12 to 22 ms in process on the reference host. This is an observation, not a target. | Not a target | sandbox-spike S14 | M8 |
| NFR-MAINT-001 | Maintainability | Package dependency directions are enforced by an architecture test, and the repository passes `scripts/validate-foundation.py` before each foundation commit. | Binary | structure.md 5.6, D-23 to D-27, D-84 | M0, M1 |
| NFR-MAINT-002 | Maintainability | Domain packages import only the standard library and permitted domain packages. | Binary | structure.md 5.6, tech.md | M1 |
| NFR-MAINT-003 | Maintainability | Decision identifiers are never reused or renumbered. | Binary | architecture.md how to read | all |
| NFR-USE-001 | Usability | No numeric usability target is set for the MVP. Every control is keyboard operable and every form field has a label. No accessibility conformance level is claimed. | Binary for the two stated rules | D-83 | M13, M24 |
| NFR-USE-002 | Usability | The approval and registration screens show a target in both its ASCII and Unicode forms. | Binary | S1.1 | M24 |
| NFR-RES-001 | Resource efficiency | The application runs on a Linux laptop with 8 GB RAM. Idle PostgreSQL memory, the Next.js server and test-browser sizes are not measured. M3 and M13 record them. No number is invented. | Stated target, measured gate | tech.md 15 (owner statement), D-67 | M3, M13 |
| NFR-OBS-001 | Observability | Structured, redacted logs with request and job identifiers. Library is `log/slog` (D-80). | Binary | arch section 17, `SEC 13` | M5 |
| NFR-OBS-002 | Observability | Metrics endpoints are post-MVP. Read-only polling is not audited per request. Evidence access and exports are audited. Health endpoints are in NFR-OBS-003. | Binary | D-80, `SEC 13` | M5 |
| NFR-OBS-003 | Observability | `GET /api/v1/health/live` and `GET /api/v1/health/ready` are part of the MVP. They return fixed, minimal status words, expose no sensitive information, and stay behind the Host allow-list on the loopback-only listener. Readiness confirms process initialization only until readiness probes exist. | Binary | D-86, `SEC 8` | Phase 1 scaffold, M3 |
| NFR-INT-001 | Data integrity | Evidence integrity uses SHA-256 with re-hash on read. | Binary | D-10 | M6 |
| NFR-INT-002 | Data integrity | Approval consumption and job claim are atomic in one transaction, with a unique (approval, job) constraint. | Binary | D-30, D-55 | M3 |
| NFR-INT-003 | Data integrity | Asset observation history is append-only. | Binary | D-76 | M20 |
| NFR-PORT-001 | Portability | The MVP targets Linux x86_64 on localhost. The reference host is the only platform tested. Other distributions are NOT TESTED. The supported baseline is defined by capability (P-51, O-22). | Reference host only | D-17, D-19, D-20, tech.md 1 | M8 |
| NFR-PORT-002 | Portability | Business logic is platform-independent. OS-specific code lives under `platform/`. | Binary | structure.md 5.5, P-21 (adopted, D-80) | M1 |

**NFR-OBS-003 acceptance criteria.** The contract is in `docs/decisions/0004-health-endpoints-mvp-decision.md`. Metrics endpoints are not part of it.

1. Liveness returns 200 with `{"status":"ok"}` on an allowed Host and does not depend on the database, the worker or any other dependency.
2. Readiness returns 200 with `{"status":"ready"}` when initialization is complete and every registered probe passes, and 503 with `{"status":"not_ready"}` and `Retry-After` otherwise.
3. Phase 1 readiness has no probe, so it does not claim database readiness. M3 adds the database probe, with a test that a database failure gives 503 `not_ready` while liveness stays 200.
4. A Host header outside the allow-list gets 403 `host_denied` on every path. The server refuses to bind a non-loopback address.
5. Another method on a health path gets 405 `method_not_allowed` with an `Allow` header, and an unknown path gets 404 `not_found`, both in the standard error envelope.
6. No response contains a stack trace, secret, environment value, internal path, version string or dependency detail. A failing probe is logged by its fixed name only.
7. The health routes are the only routes reachable without a session. Authentication for every other route is FR-AUTH-002 (M4).
8. Evidence: the automated tests of the Phase 1 scaffold (`services/core` router, bootstrap and architecture tests) for criteria 1, 2 and 4 to 6. Criterion 3 is verified by the M3 test.

## 3.10 Security and Privacy Requirements

This section maps the existing decisions to product requirements. It does not redesign them. Detailed semantics are in `S1-security-semantics.md` (approved, D-64). The requirement text is in `security.md`. The architecture that carries these requirements is in `architecture.md` (sections 8, 10, 14 and 19). Threats are in `docs/security/threat-model.md`, where no threat has the status "mitigated and tested" because no code exists.

| ID | Requirement area | Status | Product requirements | References |
| --- | --- | --- | --- | --- |
| SR-01 | Scope enforcement and default deny | Decided | FR-SCOPE-001, FR-SCOPE-002, FR-POLICY-001 | D-34 to D-38, SEC 2; T-01 to T-06 |
| SR-02 | Exclusion precedence | Decided | FR-SCOPE-001, FR-SCOPE-002 | D-57; T-05 |
| SR-03 | Lab authorization | Decided, mechanism Conditional | FR-SCOPE-004 | D-32, D-47; T-11, T-12 |
| SR-04 | DNS and IP validation | Decided. Metadata list decided (D-72). Table generation is M9 work. | FR-SCOPE-003 | D-39, D-72; T-09, T-10, T-67 |
| SR-05 | Redirect revalidation | Decided, mechanism Conditional | FR-SCOPE-003, FR-PROBE-001 | D-40; T-08 |
| SR-06 | DNS rebinding protection (outbound) | Decided, Conditional (spike PASS on reference host) | FR-SCOPE-003, FR-TOOL-003 | D-40, D-41; T-07 |
| SR-07 | Sandbox isolation | Conditional. Required limits decided (D-70). Gate G-sandbox on a named baseline (O-13, O-22). | FR-TOOL-002 | D-31, D-42, D-70; T-25 to T-32 |
| SR-08 | Egress enforcement | Conditional. Gaps recorded in the spike. | FR-TOOL-003 | D-41; T-36 to T-40 |
| SR-09 | Approval binding and expiry | Decided | FR-APPROVAL-001, FR-APPROVAL-002, FR-APPROVAL-003 | D-06, D-30, D-48, D-49, D-51, D-55; T-16, T-17 |
| SR-10 | Revocation | Decided objective, timing is the G-revoke gate (O-35) | FR-APPROVAL-003, FR-SCOPE-006 | D-48, D-71; T-15 |
| SR-11 | Authentication and session | Decided (D-60, D-68, D-69). Not implemented. | FR-AUTH-001, FR-APPROVAL-005 | D-60, D-58, D-68, D-69; T-58, T-59, T-71 |
| SR-12 | Localhost inbound protection | Decided requirement, Conditional mechanism | FR-AUTH-002 | D-33; T-54 to T-57, T-68 |
| SR-13 | Credential protection | Decided (D-52, D-85). Worker access to the Secret Service not verified, gate in M25. | FR-CRED-001 | D-08, D-52, D-85; T-62, T-63 |
| SR-14 | AI provider data disclosure | Decided (D-46, D-50, D-81). Trusted-LAN category deferred (O-36). | FR-AI-002, NFR-PRIV-001 | D-46, D-50, D-81; T-43 |
| SR-15 | Evidence integrity | Decided (D-10, D-77). Protocol adopted (P-37), not implemented. | FR-EVIDENCE-001, FR-EVIDENCE-002 | D-10, D-77; T-49, T-50 |
| SR-16 | Audit logging | Decided (approved with B1). Hash chain deferred and accepted (D-78). | FR-AUDIT-001 | SEC 13, D-12, D-78; T-51 to T-53 |
| SR-17 | Resource limits | Decided list (D-70). Mechanism Conditional. | FR-TOOL-002, FR-APPROVAL-006 | D-42, D-55, D-70; T-30, T-31 |
| SR-18 | Policy change control | Decided (D-53, D-64 choice 4). | FR-POLICY-002 | D-53, D-64; T-20, T-21 |
| SR-19 | Tool invocation hygiene and destinations | Decided. Exact-bytes launch Conditional. | FR-TOOL-001 | D-43, D-44; T-33 to T-35, T-39 |
| SR-20 | Output hardening | Decided (approved with B1). Framing denial decided (D-79). | FR-EVIDENCE-003, FR-REPORT-003 | SEC 9, D-79; T-19, T-47, T-48 |
| SR-21 | Intrusive operations gated on authentication | Decided | FR-APPROVAL-005, FR-TOOL-004 | D-58 |
| SR-22 | Application-owned endpoint protection | Decided (D-54, D-64 choice 11) | FR-SCOPE-004 | D-54, D-64; T-11 |

**Privacy.** Target data stays on the machine unless a human configures a remote provider and a passive source for a program. Passive sources receive names that belong to the program, so enabling a source is a disclosure decision (D-44). No telemetry is sent by the product. Third-party tools that send update checks or telemetry are unsupported unless those features are disabled (D-44).

**Blocked by feasibility validation.** SR-07 and SR-08 need the production sandbox and egress path to pass their gates. The tool flag and proxy behavior of each candidate tool (hostname versus IP sent to a proxy) is unverified.

## 3.11 MVP Scope

### In scope

| Area | Included | Source |
| --- | --- | --- |
| Platform | Linux localhost web application, one researcher, one machine | D-16, D-17 |
| Stack | Next.js App Router, React, TypeScript frontend. Go and Gin backend. Modular monolith. PostgreSQL. Separate server and worker processes. Local evidence storage. Provider-agnostic AI architecture. Local credential protection. | D-01 to D-03, D-62 |
| Workflow | Program, target and scope management, scope enforcement, assessments, persistent jobs, passive reconnaissance, HTTP probing, asset and endpoint inventory, 2 to 3 tool integrations, evidence, findings, validation, human verification, basic AI analysis, reports | `P 8` |
| Controls | Approvals, authentication, audit, evidence integrity (required by security decisions, listed in `product.md` section 13, CF-03 resolved) | D-45, D-60, `SEC 13`, D-83 |

Technology choices and versions are in `tech.md`. The module path, Go floor, Node 24, npm and PostgreSQL provisioning are decided (D-65 to D-67). Other versions and every item marked Proposed there are not finalized.

### Out of scope

| Excluded | Source |
| --- | --- |
| Electron, Tauri or any native desktop application | D-18 |
| Windows, macOS and Linux headless delivery (future goals) | D-19, D-20 |
| Multi-tenant SaaS, teams, microservices, Kubernetes | `P 9`, D-13 |
| Redis, Kafka, Elasticsearch, Neo4j and other brokers or graph stores | `P 9`, architecture "Not adopted". Elasticsearch is not named in any decision. It is excluded by this requirement and needs owner confirmation if different. |
| Port scanning and any non-HTTP network tool access | D-41, C item 9 |
| Autonomous exploitation, fully autonomous verification, automatic AI approval | `P 9`, `SEC 5` |
| Unrestricted shell execution, running a command built from an AI string | `SEC 10` |
| Scheduled or unattended inventory refresh | D-76, FR-ASSET-003 |
| Production security-tool execution before the sandbox and egress gates pass | D-41, D-42, plan |
| External target testing during foundation implementation | plan, owner instruction |
| Advanced business-logic engine, every possible scanner integration | `P 9` |
| Report submission or acceptance tracking | FR-REPORT-004 |
| API tokens and a second authentication factor | D-60 |

## 3.12 UX/UI Requirements

These are product-level experiences. Layout, navigation, visual design and interaction details are design work for M13 and M24 under `design-skills.md`. They do not change any security requirement. The application is an operator application (design-skills.md). No usability or accessibility targets exist (NFR-USE-001).

| ID | Experience | Requirement | Related | UX decision |
| --- | --- | --- | --- | --- |
| UX-001 | Dashboard | Show programs and assessments with their readiness and current activity | FR-PROG-001, FR-ASSESS-001 | Deferred to M24 |
| UX-002 | Program management | Create and edit programs and rules, show rules versions | FR-PROG-001, FR-PROG-002 | Deferred to M24 |
| UX-003 | Scope management | Enter rules, show canonical forms and refusal reasons, show validation status and staleness, show refused exclusions that need acknowledgement | FR-SCOPE-001 to FR-SCOPE-006 | Deferred to M24 |
| UX-004 | Asset inventory | Browse assets and endpoints with origin and observation history, flag unevaluated assets | FR-ASSET-001, FR-ASSET-002 | Deferred to M24 |
| UX-005 | Assessment activity | Show jobs, states, denials with reasons, and a stop control | FR-JOB-001, FR-APPROVAL-003 | Deferred to M24 |
| UX-006 | Approval interface | Render only the canonical operation: target (ASCII and Unicode), resolved addresses (informational), declared destinations, limits, expiry. Require password entry for intrusive approval. | FR-APPROVAL-005 | Layout deferred to M24. Framing denial required (D-79). |
| UX-007 | Evidence viewer | Show evidence as inert content with hash and provenance, download as attachment | FR-EVIDENCE-001 to FR-EVIDENCE-003 | Deferred to M24 |
| UX-008 | Finding management | Show candidates, status, linked evidence, AI recommendations labeled by origin, and human-only verification | FR-FINDING-001 to FR-FINDING-003 | Deferred to M24 |
| UX-009 | AI configuration | Configure providers, mark remote providers, show what will be sent, enable per program | FR-AI-001, FR-AI-002 | Deferred to M24 |
| UX-010 | Reports | Create, preview and export a Markdown report | FR-REPORT-001 to FR-REPORT-003 | Deferred to M24 |
| UX-011 | Security and application settings | Policy, lab registrations, passive source enablement, sessions, credential-store status | FR-POLICY-002, FR-SCOPE-004, FR-RECON-001, FR-AUTH-001, FR-CRED-001 | Deferred to M24 |

All UX requirements are Planned (M13 foundation, M24 screens). The data behind them is delivered by M16 to M23.

## 3.13 Finding Lifecycle

**Authoritative states** (`P 7`, D-09): Candidate, Needs Review, Inconclusive, False Positive, Not Reproducible, Awaiting Human Verification, Verified. Validation status and report status stay separate. Automation may recommend a result but cannot assign final human verification.

**Mapping of the stages named in the owner's request (CF-01 resolved by D-75).** The seven states are the only finding states. The other terms describe properties or other objects.

| Stage in the request | Relationship to the seven states | Status |
| --- | --- | --- |
| Candidate | State: Candidate | Decided |
| Evidence collected | Not a state. A property: the finding has linked evidence. | Decided (D-75) |
| Validation in progress | Needs Review. Awaiting Human Verification when validation is complete and a human decision is pending. | Decided (D-75) |
| Human verified | State: Verified | Decided |
| Rejected | False Positive, Inconclusive or Not Reproducible | Decided (D-75) |
| Report draft, submitted, accepted or resolved | Report lifecycle, not finding states. A finding's report status is Not reported, In draft or In final. A report is Draft or Final. Submitted, accepted and resolved states do not exist in the MVP (FR-REPORT-004). | Decided (D-75) |

**Transitions (D-75).** An automated actor (AI, worker, tool) may create a Candidate, move it to Needs Review and attach a recommendation, and nothing else. A human actor makes every other transition and records a reason: Needs Review to Inconclusive, False Positive, Not Reproducible or Awaiting Human Verification, Awaiting Human Verification to Verified or back to Needs Review, and reopening any outcome to Needs Review. Verified requires a validation record and linked evidence that passes its integrity check. A finding key (assessment, canonical asset or endpoint, vulnerability class, parameter) deduplicates: a new observation adds evidence and never changes or reopens a finding.

## 3.14 AI Agent Requirements

| Requirement | Statement | Product requirement |
| --- | --- | --- |
| Provider abstraction | One provider-neutral port. A provider change changes no authorization or policy. | FR-AI-001 |
| Local and remote | A provider not on loopback is remote, including a LAN host. Locality is classified by the connected address, and tunnels and proxies are remote. | FR-AI-002 |
| User-configured access | AI is off until a provider is configured. Keys are stored in the credential store and sent only to their endpoint. | FR-AI-001, FR-CRED-001 |
| Remote disclosure | Explicit per-program configuration and a view of what is sent. | FR-AI-002 |
| Permission boundaries | AI cannot approve, change policy or scope, run tools, access a shell or set Verified. It receives no tool handle. | FR-AI-003 |
| Policy outside the agent | The backend policy engine decides every operation. AI proposals are data. | FR-POLICY-001 |
| Human approval | A human approves risky operations on a canonical screen. | FR-APPROVAL-005 |
| Evidence-based output | AI output is labeled and distinguishable from observed facts. | FR-AI-003 |
| Auditability | Each transmission and each accepted proposal is audited. | FR-AI-004 |

The product does not promise autonomous exploitation, an unrestricted agent or full prompt-injection immunity. Prompt injection from target content cannot be eliminated. The platform limits what a steered model can cause (T-42). The MVP builds one loopback adapter and one generic OpenAI-compatible remote adapter, and no provider is enabled by default (D-81).

## 3.15 Acceptance and Success Metrics

No numerical performance or business target is invented. Each metric is a test.

| ID | Metric | How it is tested | Requirements |
| --- | --- | --- | --- |
| SM-01 | Scope decisions are deterministic | The M1 table and property tests give identical decisions for identical inputs, and adding an exclusion never changes a denial into an allow | FR-SCOPE-001, FR-SCOPE-002 |
| SM-02 | Out-of-scope targets are denied at every enforcement point | Tests at job creation and at launch deny a target removed from scope | FR-POLICY-001 |
| SM-03 | AI cannot bypass approval or policy | No AI-reachable interface creates an approval or changes policy | FR-AI-003, FR-POLICY-002 |
| SM-04 | An approval cannot be reused beyond its binding | Concurrency and redelivery tests (gate G-consume) | FR-APPROVAL-001, FR-APPROVAL-002 |
| SM-05 | Evidence integrity is verifiable | A modified evidence file is detected | FR-EVIDENCE-002 |
| SM-06 | Status transitions follow the rules | An automated actor cannot set Verified | FR-FINDING-001, FR-FINDING-003 |
| SM-07 | Revocation stops work within 30 seconds on the baseline | Measured test (gate G-revoke) | FR-APPROVAL-003 |
| SM-08 | No secret reaches argv, environment, logs, evidence or reports | Secret search (gate G-redact) | FR-CRED-001, FR-TOOL-001 |
| SM-09 | A basic assessment completes on the MVP environment | The plan's acceptance scenario: register a program and a local lab, validate authorization, run enabled passive recon, probe an in-scope lab host, collect and verify evidence, verify a candidate as a human, export a Markdown report, with every denial and approval audited, using no external target | FR-PROG-001 to FR-REPORT-003 |

The plan gaps PM-01 to PM-06 are closed by milestones M16 to M25 (D-83). SM-09 can pass only when those milestones are built and their gates pass.

## 3.16 Release Roadmap

The roadmap is `.kiro/specs/mvp/plan.md`. This section only maps requirements to its milestones. Dependencies, gates and readiness conditions are those of the plan. M1 is the only milestone expanded to task level (Tasks 1.1 to 1.5). The Phase 1 scaffold is existing preparatory work and is not a milestone. M1 has not started.

| Milestone | Plan deliverable | Requirements |
| --- | --- | --- |
| M0 | Foundation and repository | NFR-MAINT-001 |
| M1 | Scope canonicalization core (pure domain) | FR-SCOPE-001, FR-SCOPE-002, NFR-MAINT-001, NFR-MAINT-002, NFR-PORT-002 |
| M1b | Host conversion (IDNA) | FR-SCOPE-007 |
| M2 | Policy and approval domain | FR-POLICY-001, FR-APPROVAL-001 to FR-APPROVAL-004, FR-APPROVAL-006 (domain parts), FR-SCOPE-004 to FR-SCOPE-006 (domain parts) |
| M3 | Persistence | NFR-INT-002, NFR-OBS-003 (database readiness probe), schema for FR-PROG-001, FR-ASSESS-001, FR-AUDIT-001 |
| M4 | Authentication and session | FR-AUTH-001, FR-AUTH-002 (server side), FR-APPROVAL-005 (backend) |
| M5 | Audit model | FR-AUDIT-001, NFR-OBS-001, NFR-OBS-002, NFR-REL-002 |
| M6 | Evidence metadata and integrity | FR-EVIDENCE-001, FR-EVIDENCE-002, NFR-INT-001, NFR-PRIV-002 |
| M7 | Worker and job lifecycle | FR-JOB-001, FR-APPROVAL-002, FR-APPROVAL-003, FR-SCOPE-006, NFR-REL-001, NFR-REL-003 |
| M8 | Sandbox launcher and self-test | FR-TOOL-002, NFR-PORT-001, NFR-PERF-002 |
| M9 | Egress enforcement | FR-TOOL-003, FR-SCOPE-003, FR-SCOPE-004 (deny tables) |
| M10 | Tool adapter framework | FR-TOOL-001, FR-APPROVAL-004 |
| M11 | First adapters (Subfinder, httpx, optional Nuclei) | FR-RECON-001, FR-PROBE-001, FR-APPROVAL-006, FR-TOOL-004 (after the D-58 gate) |
| M12 | AI provider abstraction | FR-AI-001 to FR-AI-004, NFR-PRIV-001 |
| M13 | Frontend foundation | FR-AUTH-002 (browser side), FR-EVIDENCE-003, NFR-USE-001 |
| M14 | Reporting | FR-REPORT-001 to FR-REPORT-003 |
| M15 | End-to-end local lab assessment | SM-09 and every MVP requirement |
| M16 | Program, target, scope and assessment records | FR-PROG-001, FR-PROG-002, FR-POLICY-002, FR-SCOPE-001, FR-ASSESS-001 (records) |
| M17 | Lab registration and lifecycle | FR-SCOPE-004 |
| M18 | Target authorization validation | FR-SCOPE-005, FR-SCOPE-006 |
| M19 | Operations, plan items and the approval application lifecycle | FR-APPROVAL-001 to FR-APPROVAL-003, FR-APPROVAL-005, FR-FINDING-004 (plan items) |
| M20 | Asset and endpoint inventory | FR-ASSET-001, FR-ASSET-002, NFR-INT-003 |
| M21 | Assessment lifecycle and job creation | FR-ASSESS-001 (lifecycle), FR-JOB-001 (creation) |
| M22 | Finding lifecycle, validation and human verification | FR-FINDING-001 to FR-FINDING-003 |
| M22b | Validation replay | FR-FINDING-002 (replay) |
| M23 | AI-assisted analysis and orchestration | FR-AI-005, FR-FINDING-004 (AI proposals) |
| M24 | UI integration of the workflow screens | UX-001 to UX-011, NFR-USE-002 |
| M25 | Credential store | FR-CRED-001 |

**Plan mismatches (resolved in Round 6, D-83).** The plan was changed to close them.

| ID | Mismatch | Resolution |
| --- | --- | --- |
| PM-01 | No milestone implemented program, target, scope and assessment use cases or the validation workflow | M16, M18, M21 |
| PM-02 | The lab registration lifecycle was not named | M17 |
| PM-03 | Asset and endpoint inventory had no milestone | M20 |
| PM-04 | Finding lifecycle, validation and human verification had no milestone, and M14 depended on M6 only | M22, M22b. M14 now depends on M22. |
| PM-05 | Test planning had no milestone | M21, M23 |
| PM-06 | The credential store had no milestone | M25 |

## 3.17 Risks, Dependencies, and Open Decisions

### Security and feasibility blockers

| Blocker | Affects | IDs |
| --- | --- | --- |
| Sandbox verified on one host only. Required limits decided (D-70). The bounded job working directory is unproven. No per-job I/O limit (accepted). | Any tool execution | O-13, O-22, D-70, T-31, T-32 |
| Egress path gaps: Host override not refused, no tunnel revocation hook, per-job socket needed | Any network-touching tool | D-41, T-36 to T-38 |
| Numeric request, concurrency and duration limits not established. A measured gate (D-71). | Probing and any active operation | O-38, D-71 |
| Intrusive operations need authentication and re-authentication implemented and tested | Template checks, state-changing replay | D-58 |
| Wildcard inclusion refused through the MVP (D-73) | Wildcard scope rules | O-32, P-56 |
| IDNA library and single-script rule unvalidated. M1 is ASCII only. | Non-ASCII host names | O-33, D-73 |
| Table generation and review of other provider addresses (list decided, D-72) | Address policy | D-72 |
| Heartbeat and kill timing unmeasured | The 30-second objective | O-35 |
| Same-user processes can reach PostgreSQL peer authentication, the keyring, evidence and the proxy socket. Accepted for the MVP (D-78). | Role separation and audit guarantees | T-61, D-78 |
| Secret Service from a worker outside a desktop session not verified | Credential features | D-85 |

### Owner and product decisions

Decisions are closed in `docs/decisions/0003-foundation-closure-decisions.md` (D-63 to D-85) and `docs/decisions/0004-health-endpoints-mvp-decision.md` (D-86). The register is in `architecture.md` section 19. Eight items stay open as gates, and none blocks M1.

| Item | Gate | Blocks |
| --- | --- | --- |
| O-07 | G-egress against the product client and proxy | M10, network-touching tools |
| O-13, O-22 | G-sandbox on a named baseline | M10, any tool launch |
| O-32 | Post-MVP public suffix decision | Wildcard inclusion |
| O-33 | M1b corpus | M1b |
| O-35 | G-revoke measurement | Approval-bound jobs |
| O-36 | A separate owner decision | Nothing in the MVP |
| O-38 | Measured numbers in the manifest and an approved policy default | Low-impact and Intrusive operations |

### Platform limitations and external dependencies

- Only one Linux host was tested. Other distributions, the Ubuntu 24.04 user-namespace default and other architectures are NOT TESTED.
- Candidate tools (Subfinder, httpx, Nuclei) are external projects. Their flags, proxy behavior and update checks need per-tool validation (tech.md 9).
- Bubblewrap is an external LGPL helper executed as a separate program that the user installs (D-74).
- Public suffix data is MPL-2.0 and is not in the repository (D-74).

### Source conflicts

| ID | Conflict | Resolution |
| --- | --- | --- |
| CF-01 | The finding stages in the owner's request differ from the seven states of `product.md` section 7 | Resolved (D-75). The seven states are the only states. The other terms are properties or report values. |
| CF-02 | `product.md` sections 4 and 8 say "continuous asset and endpoint inventory". C item 8 and O-30 say user-triggered only. | Resolved (D-76). Persistent inventory with an append-only observation history, user-driven, no scheduler. Recorded in `product.md` section 13. |
| CF-03 | `product.md` section 8 does not list approvals, authentication or audit, which D-45, D-60 and `SEC 13` require | Resolved (D-83). Recorded in `product.md` section 13. |
| CF-04 | Enforcement points E1 to E4 and `AuthorizedOperation` were Proposed (P-04, P-05) while D-05 and D-28 are Decided | Resolved (D-80). P-02 to P-05 adopted. P-04 is not a compile-time guarantee in Go. |
| CF-05 | S1 was "awaiting final owner review" while `security.md` already carried its requirements | Resolved (D-64). S1 is approved. |
| CF-06 | `architecture.md` listed O-24 among open items although its grammar was decided | Resolved (D-73). O-24 is in the Resolved table. The libraries are gated (O-33). |

## 3.18 Glossary

| Term | Definition |
| --- | --- |
| Program | A bug bounty or authorized assessment program with rules. Every assessment belongs to one. |
| Target | A registered host, address or range that an assessment may test. |
| Scope rule | A domain, wildcard, IP or CIDR rule, either in-scope or an exclusion. |
| Exclusion | A rule that denies a target. It always wins over an in-scope rule. |
| Canonical form | The single normalized spelling of a rule or target used for comparison. A target with more than one possible form is ambiguous and denied. |
| Lab target | A target a human registered for local or self-owned testing, with a bound permitted address range, expiry and revocation. |
| Assessment | One authorized testing effort under a program. |
| Target authorization validation | The record made of a mechanical scope check, a stored program-rule reference and a human attestation. It is not proof of ownership. |
| Operation | A single tool action with a target, parameters, class and limits. |
| Approval | A human decision bound to one canonical operation, with expiry, uses, duration and attempts. |
| Approval class | Passive, Low-impact or Intrusive, taken from the reviewed tool manifest. |
| Job | A persistent unit of work run by the worker. |
| Execution attempt | One launched tool process. A denial before launch is not an attempt. |
| Candidate | A finding that no human has verified. Scanner and AI output starts here. |
| Finding | A potential vulnerability record with evidence links and a status. |
| Evidence | A preserved item with SHA-256 and provenance, such as tool output or a request and response pair. |
| Validation status | Where a finding stands in validation. Separate from report status. |
| Report status | Where a finding stands in reporting (Not reported, In draft, In final). A report itself is Draft or Final. Separate from validation status (D-75). |
| Human verification | The transition to Verified made by a human actor. |
| Provider | An AI model service. Remote means any endpoint not on loopback. |
| Policy engine | The backend decision function that allows, denies or requires approval for every operation. |
| Sandbox | The isolated environment in which a tool process runs. |
| Egress path | The only route by which a tool may reach the network, enforced by the first-party client or a per-job policy proxy. |
| Fail closed | When a control or mechanism is unavailable, the operation does not run. |
| Reference host | The single machine on which the feasibility spikes ran. It is not a support baseline. |
| Spike | A throwaway local experiment. A pass is not production readiness. |
