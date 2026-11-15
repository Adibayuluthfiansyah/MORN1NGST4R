# C1 and C2: Owner decision proposal for the MVP product definition

**Round 6 (2026-11-16): the remaining items are decided in `docs/decisions/0003-foundation-closure-decisions.md`.** Item 3 (audit hash chain) is deferred and accepted as a residual risk (D-78). Item 4 (test planning) is adopted as P-38 (D-80, D-83). Items 6 and 7 (finding transitions, report status) are D-75. Item 8 (asset inventory) is D-76. Item 10 (AI scope) is D-81. `product.md` gained section 13 (interpretation notes) for the points it raised. Wording below that says "proposal" is history.

Status: items 1, 2, 5 and 9 are DECIDED by the owner (Round 5, D-59 to D-61, with D-45). Items 3, 4, 6, 7, 8 and 10 remain PROPOSAL. The document was reconciled with the owner security decisions D-30 to D-46 in Round 3. `product.md` has not been changed for these items. Each item has a recommendation, alternatives, implications and a blocking status. The owner decides. Where a security decision is already made, this document points to it and does not restate or redefine it. An item that still waits on another decision is marked DEPENDENT, and no decision is invented for it.

This covers audit findings C1 (the MVP list omits capabilities the workflow needs) and C2 (undefined product terms). Related architecture IDs are listed per item. Security wording that depends on these decisions is in `S1-security-semantics.md` and `B1-security.md.diff`.

## Summary

| # | Decision | Recommendation (short) | Blocks MVP implementation |
| --- | --- | --- | --- |
| 1 | Approval workflow | DECIDED. Class names, membership, uses, validity, attempts, revocation and passive source enablement are fixed by D-30, D-44, D-45 and D-59. Numeric operation limits stay open and fail closed (O-38). | Yes |
| 2 | Authentication | DECIDED (option A, D-60). One local researcher account, memory-hard password hash, PostgreSQL sessions, HttpOnly cookie, CSRF protection, rate-limited login, no API tokens, password re-entry for intrusive approval. Recency window P-39 and password recovery O-37 are proposed. | Yes |
| 3 | Audit trail | Append-only PostgreSQL table written with the change, denials in their own transaction. Hash chain after the MVP. | Partly |
| 4 | Test planning | Persisted plan items owned by the assessment, human or AI-proposed, executed only after policy and approval. | Yes |
| 5 | Target authorization validation | DECIDED (option A, D-61). Mechanical scope check, stored program-rule reference and human attestation, bound per assessment to the target-set hash and rules version. The 24-hour expiry P-40 is proposed. | Yes |
| 6 | Finding lifecycle | Explicit transition table. Automation may only create, set Needs Review and attach recommendations. | Yes, for findings |
| 7 | Report lifecycle | Draft, Reviewed, Exported (immutable snapshot). Verified findings by default. One human-readable format. | Yes, for reporting |
| 8 | Continuous inventory | Deferred. User-triggered refresh with append-only history. | No |
| 9 | Controlled tests | DECIDED (strict M1 mapping, D-59). Four-class catalog, non-destructive. No exploitation, brute force, DoS or port scanning. Numeric limits stay open and fail closed (O-38). | Yes |
| 10 | AI provider strategy | Provider-neutral port, one local and one generic remote-compatible adapter, AI off by default. | Only AI features |

Suggested decision order, because later items depend on earlier ones: 9, 2, 5, 1, 4, 3, 6, 7, 10, 8.

## Canonical definitions

Each topic has one canonical definition. This table says where it lives and what this document may still decide. Anything not listed here is defined only in this document.

| Topic | Canonical definition | State | What this document still decides |
| --- | --- | --- | --- |
| Approval uses, validity, retries, attempts, duration, revocation, re-authentication requirement, who may approve | D-30, D-45, S1.6, `security.md` 3 and 4 | Decided | Nothing. Items 1 and 4 refer to it. |
| Approval class names and membership | D-45, D-59 | Decided | Nothing. Numeric rate, concurrency and duration limits are O-38 and fail closed. |
| Passive reconnaissance gating | D-44, S1.5 | Decided | Nothing. Items 1 and 9 refer to it. |
| Authentication and re-authentication | D-60, S1.7, `security.md` 8 | Decided (option A). Recency window P-39 and password recovery O-37 are proposed. | Nothing |
| Target authorization validation | D-61 | Decided (option A). Expiry P-40 is proposed. | Nothing. The mechanical scope check is defined by D-34 to D-40 and S1.1, S1.2. |
| Scope grammar, address policy, redirects | D-32 to D-40, S1.1, S1.2 | Decided | Nothing |
| Tool hygiene and non-target destinations | D-43, D-44, S1.4, S1.5 | Decided | Nothing |
| Remote AI provider definition | D-46, `security.md` 5 | Decided | Nothing. Item 10 refers to it. |
| Which operations exist in the MVP | Item 9, D-59 | Decided (strict M1 mapping of the four-class catalog). Tool selection is P-52. | Nothing |

## 1. MVP approval workflow

Related: O-06, P-30, P-31, P-14, S1.6.

**Recommended decision.** Each tool operation declares a risk class in its tool manifest (P-14). The MVP has three classes:

| Class | Meaning | Approval |
| --- | --- | --- |
| Passive | No traffic to the target. Queries third-party data sources about in-scope names. | Policy evaluation and per-program source enablement (D-44). No per-run approval. Listed in the plan and audited. |
| Low-impact active | Capped-rate HTTP probing and fingerprinting of in-scope targets. Membership is fixed by item 9 and D-59. | Approval as defined in D-45 and S1.6. |
| Intrusive | Vulnerability checks beyond probing, and replay of state-changing requests. Membership is fixed by item 9 and D-59. | Approval as defined in D-45 and S1.6, on a screen rendered from the canonical operation. |

The approver is the authenticated researcher (a human actor). The AI actor, API tokens and the worker cannot approve. Approval is bound to the exact operation hash (`security.md` 4). Uses, validity, attempts, duration and revocation are defined once, in D-45, D-30 and S1.6, and are not restated here. Re-authentication for intrusive approvals is the password re-entry decided in D-60.

**Status of this item:** DECIDED by D-30, D-44, D-45, D-59 and D-60 (Round 5). The numeric request-rate, concurrency and duration limits of each operation are not established and stay open (O-38). An active operation without them is blocked.

**Alternatives.**
- Approve every operation individually: safest, but approval fatigue pushes users to approve without reading.
- One approval per plan: fewer prompts, but binds less precisely than `security.md` 4 asks.
- No approval below intrusive, with policy evaluation only: simplest, but low-impact active traffic reaches third parties without a human decision.

**Implications.** Security: replay, reuse and AI-written consent text are blocked by S1.6. Product: approval screens and a risk field in every tool manifest are required. Too many classes confuse users, and too few leave gaps.

**Blocks MVP implementation:** yes. Approval implementation, job creation for active operations, and test planning cannot proceed without it.

## 2. MVP authentication

Related: O-16, O-25, O-03, P-19, `security.md` 8.

**Recommended decision.** One local researcher account, created at first start. A password verified with a memory-hard password hash (the algorithm is chosen in the security review and recorded in `tech.md`). A server-side session stored in PostgreSQL with an idle and an absolute expiry, delivered in a cookie that is HttpOnly and sent only to the application origin, with cross-site request forgery protection on state-changing requests. Login attempts are rate limited and audited. The first-run bootstrap uses a one-time code printed in the terminal, never placed in a URL (`security.md` 8). Session revocation is supported. Password recovery is a separate proposal (O-37). No reset path exists until it is approved.

**Alternatives.**
- No password, a random per-start token exchanged once for a session: most convenient, but anyone who can read the start output or the terminal gets in, and there is no durable identity for audit.
- Operating-system user or socket peer credentials: not usable from a browser.
- Multiple user accounts with roles: out of MVP scope (`product.md` 9), and a future headless mode needs it.

**Implications.** Security: every approval and audit record gets a real actor. A localhost web app that skips authentication would let any local process or malicious web page act as the researcher. Product: a login screen, password rules and a recovery path. Architecture: fills the `identity` and `auth` packages and closes O-16 and the single-user part of O-25.

**Owner decision (Round 5, D-60): option A.** One local researcher account. Memory-hard password hashing (the algorithm and parameters are PROPOSED in P-48, not decided). Server-side sessions stored in PostgreSQL. HttpOnly cookie. CSRF protection. Login rate limiting with audit. No API tokens in the MVP. Approval of an intrusive operation requires password re-entry. A second factor is deferred. Intrusive operations stay disabled until authentication and re-authentication are implemented and tested (D-58).

**Proposed, not decided:** a five-minute recency window for the password re-entry (P-39). **Separate proposal:** password recovery (O-37, `docs/decisions/pending/F-password-recovery-proposal.md`). No reset path exists until that proposal is approved.

**Blocks MVP implementation:** yes. HTTP and authenticated endpoints, approvals ("approving user") and audit actors all depend on it.

## 3. MVP audit trail

Related: P-33, `security.md` 13, B1.

**Recommended decision.** An append-only table in PostgreSQL, written by each use case in the same transaction as the change it records. Denials and failures have no change to roll back with, so they are written in their own transaction. An operation does not start if its audit record cannot be written. Cancellation, revocation, scope narrowing and denials still take effect when audit is down, and the missing record is retried and reported. Events: the `security.md` 13 list plus the additions in B1. The server and worker database role can insert and read but not update or delete. A read-only audit list is available in the UI. The keyed hash chain of P-33 is deferred to after the MVP.

**Alternatives.**
- Include the hash chain in the MVP: stronger tamper detection against the migration role and database owner, at the cost of key management in the credential store and more complexity.
- Log files only: easy, but not queryable and easy to alter.
- Database table plus log file: more places to protect and keep consistent.

**Implications.** Security: without the chain, a database administrator can alter history undetected. That is acceptable for a single-user local MVP only if the owner accepts it. Product: a minimal audit view is needed.

**Blocks MVP implementation:** partly. The audit event model and the `AuditSink` port must exist before the first state-changing use case. The hash chain does not block.

## 4. MVP test planning

Related: O-29, P-38, P-11, workflow steps 7 to 10.

**Recommended decision.** A plan is a set of plan items owned by the assessment. Each item records: target or targets (which must be in scope), operation type from a tool manifest, validated parameters, risk class, rate and duration limits, the origin (human or AI), and a rationale labeled with its origin. Items are created by the researcher or accepted from AI proposals. A plan is data. Each item must pass policy and approval before it becomes a job. An approved plan item is immutable, and the approval binds its hash. The approval fields, including the maximum attempt count, are defined by D-45 and D-30 and are not restated here. There is no autonomous execution and no scheduler.

**Alternatives.**
- No persisted plan, only "run this operation now": the simplest slice, but it drops workflow step 8 and the ability to review AI suggestions before acting.
- Plan as free-text notes: no enforcement and no approval binding.
- A workflow engine with dependencies and branching: excluded from the MVP (`product.md` 9).

**Implications.** Security: AI suggestions enter as typed proposals and are never executed directly. Product: a plan screen and an AI-suggestion review step.

**Blocks MVP implementation:** yes for steps 8 to 10. The thinner alternative would unblock a smaller first slice.

## 5. Target authorization validation

Related: O-28, O-24, P-24, workflow step 4.

**Recommended decision.** "Validate target authorization" produces a validation record on the assessment, with three parts:

1. A mechanical check: every target parses into canonical form, is covered by an in-scope rule, is not excluded, and resolves to acceptable addresses. The rules for each of those steps are the decided ones in D-34 to D-40, S1.1 and S1.2. This item adds no scope rule of its own.
2. A recorded reference to the program rules: the rules text stored on the program plus a reference such as a URL or document identifier.
3. A human attestation: the researcher confirms authorization for this assessment, recorded with actor and time.

The assessment becomes Ready only when all three pass. The result is re-evaluated at execution time and becomes stale when scope or rules change. There is no automated proof of ownership, because bug bounty targets belong to someone else.

**Alternatives.**
- Attestation only, without the mechanical check: weaker and easy to get wrong.
- Proof of control (a DNS record or a file on the host): works only for self-owned targets, which suits local labs.
- No validation step, treating registration as validation: leaves workflow step 4 empty.

**Implications.** Security: the attestation is a recorded statement, not proof. It supports accountability, not prevention. Product: clear wording so users do not read "validated" as "verified by the platform".

**Owner decision (Round 5, D-61): option A.** The validation record has three parts: the mechanical scope validation (decided in D-34 to D-40), a stored program-rule reference, and a human attestation. It is bound per assessment to the canonical target-set hash and the rules version. Any change to scope, rules or a lab registration makes it stale. A lab target needs lab-specific attestation wording. An AI cannot attest. Proof of control is deferred.

**Proposed, not decided:** a 24-hour validation expiry in addition to the change-based staleness (P-40).

**Blocks MVP implementation:** yes. Assessment readiness and the scope evaluator depend on it.

## 6. Finding lifecycle and transitions

Related: O-11, P-10, D-09, `product.md` 7.

**Recommended decision.** Definitions: Candidate is a newly created finding. Needs Review is accepted for triage. Inconclusive means validation ran and the evidence does not settle it either way. Not Reproducible means reproduction was attempted, the attempts are recorded, and they failed. False Positive means a human judged it not real. Awaiting Human Verification means validation is complete, with evidence and a recommendation, and a human decision is pending. Verified means a human confirmed it.

Transitions:

| From | To | Who | Condition |
| --- | --- | --- | --- |
| (new) | Candidate | Automation or human | AI-origin findings are labeled |
| Candidate | Needs Review | Automation or human | None |
| Candidate | False Positive | Human | Reason recorded |
| Needs Review | Inconclusive, Not Reproducible, False Positive, Awaiting Human Verification | Human | Automation may attach a recommendation but not move the state |
| Awaiting Human Verification | Verified | Human | At least one linked evidence item with a passing integrity check, and a validation record |
| Awaiting Human Verification | Needs Review | Human | Reason recorded |
| Inconclusive, Not Reproducible, False Positive | Needs Review | Human | New evidence or a retest |
| Verified | Needs Review | Human | Evidence later flagged as compromised, or contrary evidence. The platform raises a flag and the human decides. |

Automation may only create Candidates, move Candidate to Needs Review, and attach recommendations.

**Alternatives.**
- Merge Needs Review and Awaiting Human Verification: simpler, but loses the distinction between triage and ready-to-confirm.
- Let automation set Not Reproducible after N failed retries: less manual work, but a transient failure would close a real finding.
- Free transitions with an audit log only: no state-machine protection.

**Implications.** Security: Verified cannot be reached by automation. The domain state machine enforces it, not the UI. Product: the states stay as `product.md` 7 lists them. Only their meaning and the allowed moves are added.

**Blocks MVP implementation:** yes, for the findings module. Persistence and scaffolding can start without it.

## 7. Report lifecycle and status

Related: O-27, `product.md` 7, B1 section 9.

**Recommended decision.** One report entity per assessment with its own status, separate from finding validation status: Draft, Reviewed (a human marked it ready), Exported (an immutable snapshot with a content hash and a timestamp). Default content is Verified findings only. Unverified findings can be included in a clearly labeled appendix when the researcher chooses. Evidence is referenced by ID and hash. Export is redacted by default and audited. The MVP supports one human-readable format. Markdown is the proposal because its output is easy to escape correctly. The format is a product choice and the tooling choice belongs in `tech.md`.

**Alternatives.**
- Add HTML, PDF or a machine-readable format in the MVP: more reach, with more escaping and rendering risk (`security.md` 9).
- A single "report status" on each finding: conflates two statuses `product.md` 7 says must stay separate.
- Editable exported reports: breaks the integrity of what was delivered.

**Implications.** Security: each format needs its own escaping rules and tests. Product: a report screen and an export action.

**Blocks MVP implementation:** yes for "generate a usable report" (`product.md` 12), but not for the early slices.

## 8. Continuous inventory: MVP or deferred

Related: O-30, `product.md` 4 and 8.

**Recommended decision.** Deferred. The MVP refreshes the inventory when the researcher triggers it. Asset observations are append-only, so history accumulates from repeated runs and the inventory still builds up over time. Scheduled or unattended runs are a later feature. `product.md` 4 should say "continuous" means repeated user-triggered refresh in the MVP.

**Alternatives.**
- Scheduled passive refresh in the MVP: useful for monitoring, but needs a scheduler, unattended execution and a policy for acting without a person present.
- Scheduled refresh of everything: not recommended. It breaks the approval model for active operations.

**Implications.** Security: unattended active traffic is avoided. Product: a clear statement so the word "continuous" is not read as a promise.

**Blocks MVP implementation:** no.

## 9. Meaning and limits of controlled tests

Related: O-29, O-05, S1.2, S1.4, S1.5.

**Recommended decision.** The MVP catalog has four classes, each within scope, rate-limited and non-destructive:

1. Passive reconnaissance from declared data sources, each enabled once per program by a human (D-44).
2. HTTP probing and fingerprinting: status, headers, titles, TLS metadata.
3. Known-vulnerability checks from a reviewed allow-list of non-destructive templates. No denial of service, brute force, credential guessing, state change or data extraction beyond the minimum needed as proof.
4. Human-initiated replay of captured requests. Safe methods by default. A state-changing method needs its own intrusive approval.

Out of the MVP: exploitation, brute force, denial of service, authenticated testing with stored target credentials, business-logic testing, port scanning, and anything needing non-HTTP network access (under the S1.2 direction). Limits for every operation: a request rate cap, a concurrency cap, a maximum duration, no destructive payloads, and minimal evidence capture. Each limit is part of the approved operation.

**Alternatives.**
- Probing only: the smallest and safest MVP, but "execute controlled tests" in `product.md` 12 becomes thin.
- A broader catalog including port scanning: more capability, but needs network controls beyond HTTP and a larger isolation story.

**Implications.** Security: this sets what the approval classes (item 1) and the tool manifests must cover. Product: it defines what a user can actually run in the first release.

**Owner decision (Round 5, D-59): strict mapping M1.** Passive reconnaissance is Passive. HTTP probing and safe-method replay are Low-impact. State-changing replay is Intrusive. Known-vulnerability template checks are Intrusive in the MVP. An unknown operation is denied. The risk class comes from the reviewed tool manifest and the policy engine and never from an AI. The numeric request-rate, concurrency and duration limits stay fail-closed until they are established (O-38).

This resolves the earlier inconsistency in which item 9 class 3 had no approval class: it is Intrusive.

**Blocks MVP implementation:** yes. Tool selection, risk classes and approval design all depend on it.

## 10. Initial AI provider strategy

Related: O-26, O-10, P-34, D-07, `product.md` 11.

**Recommended decision.** Keep the provider-neutral port that D-07 already requires. The MVP implements two adapters: one for a local model server on the loopback interface, and one generic adapter for remote services that speak a common compatible HTTP interface. The second covers several of the providers `product.md` 11 lists through configuration, without a vendor-specific adapter. AI is off until a provider is configured. A local provider is the default. A remote provider is any endpoint that is not on the loopback interface, including a host on the local network (D-46). It needs an explicit enable per provider and per program (P-34), with allow-listed endpoints and audited transmissions, and disclosure of what is sent. "Basic AI-assisted analysis" is three functions: summarize and group discovered assets and flag the interesting ones, propose plan items, and draft finding or report text from evidence. All output is a typed proposal labeled with its AI origin, and none of it executes.

**Alternatives.**
- One provider only: smaller, but contradicts the spirit of provider independence.
- Vendor-native adapters in the MVP: richer features, more code and more dependencies.
- No AI in the first slice, with a fake provider only: the fastest route to a working core, and AI added afterwards.

**Implications.** Security: the main risks are data egress, prompt injection from target content, and cost. They are handled by P-34, P-11 and the proposal-only rule. Product: a provider settings screen and clear AI labels. SDK and library choices are not made here.

**Blocks MVP implementation:** only the AI features. The core slice can use a fake provider.

## What I need from you

For each item, reply Approve, Edit (with changes) or Defer. Items 9, 2, 5 and 1 unlock the most work. Items 3 and 6 to 10 can follow.

After approval, these will be reflected in `product.md` (C1: the MVP list and the definitions) and in the architecture decision registry. Until then they stay proposals.
