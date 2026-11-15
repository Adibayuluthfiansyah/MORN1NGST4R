# 0002: Foundation completion decisions (Round 5)

Status: owner decisions recorded, with the items below marked PROPOSED kept proposed. The owner gave these as one coordinated message on 2026-11-15, after the Round 4 review. This record continues `0001-owner-security-decisions-round-2.md`.

## What is decided and what is not

- **Decided by the owner (D-47 to D-62):** the conservative resolutions for the Round 4 findings A1 to A9, B1 to B10, C1 to C6 and the seven fail-open interactions, the three C2 decisions (items 9, 2 and 5), and the confirmation of the technology baseline.
- **Proposed, not owner-confirmed:** every number and every technology choice the owner did not name. These are `P-39` to `P-62` in the register. The owner marked two numbers as proposed in the instruction: the five-minute recency window (P-39) and the 24-hour validation expiry (P-40).
- **Open:** O-37 (password recovery), O-38 (numeric active-operation limits), O-39 (PostgreSQL binaries), O-40 (licence acceptance), plus the older open items listed in the register.

No proposal was promoted to a decision. Technical versions were taken from primary sources and recorded in `.kiro/steering/tech.md` and `docs/development/tech-verification/`. Each carries its own status.

## Decision register

| ID | Decision | Owner source |
| --- | --- | --- |
| D-47 | Lab authorization: evaluated first, bound to the registered target and permitted range, full lifecycle, initial target only, never for forbidden classes | A1, B1, C (lab CIDRs), D (lab hostnames, redirects, forbidden classes) |
| D-48 | Expiry prevents new launches only. Revocation and scope or lab changes terminate running work within 30 seconds. Heartbeat and kill timing stay conditional until measured. Audit retries are bounded. | A2, B6 |
| D-49 | Intrusive operations: exactly one process launch, no retry of any kind, a rerun needs a new job and approval | A3, B5 |
| D-50 | AI credentials and provider locality | A4, B9 |
| D-51 | Approval record binds the operation hash and declared destinations. Three attempts for approval-bound active jobs. Traceability through existing records. | A5, A6, A7 |
| D-52 | Linux MVP credentials: Secret Service only, fail closed, no environment or standard-input delivery, files or descriptors conditional | A9 |
| D-53 | Policy changes: authenticated human only, one canonical policy store, audited, re-authentication for widening changes | B2, B10 |
| D-54 | Application-owned endpoints protected, IPv4 and IPv6 symmetric | B3, B7 |
| D-55 | Approval accounting separate from process attempts. Fan-out limits. Blocked when no safe default exists. | B4, B8, C (retry claims) |
| D-56 | Scope, rules or lab change in either direction invalidates validation. DNS changes revalidate. Redirects into labs are denied. | C (scope edits), D |
| D-57 | Exclusions all ports, accepted when inclusion is disabled, exempt from the CIDR minimum. Loopback needs ports. IANA reachable flag never authorizes. Deny wins. | C, D |
| D-58 | Intrusive operations disabled until authentication and re-authentication are implemented and tested | D |
| D-59 | Approval class membership, strict mapping | C2 item 9 |
| D-60 | Authentication, option A | C2 item 2 |
| D-61 | Target authorization validation, option A | C2 item 5 |
| D-62 | Technology baseline confirmed | Phase 4 list |

## How each finding was resolved

`sec` is `.kiro/steering/security.md`. `S1` is `docs/decisions/pending/S1-security-semantics.md`. `arch` is `.kiro/steering/architecture.md`.

| Finding | Resolution | Written in |
| --- | --- | --- |
| A1 CNAME versus lab | Lab authorization is evaluated first and the CNAME rule has an explicit exception for it. All other non-public resolutions stay denied. | sec 2, S1.2 step 9, D-47 |
| A2 expiry | Expiry stops new launches only. | sec 4, S1.6 item 1, D-48 |
| A3 intrusive retries | One launch, no retry of any kind. The idempotent-retry text in the architecture no longer applies to approval-bound jobs. | sec 4, arch 7, 14, 16, D-49 |
| A4 AI credentials | Target credentials never go to a provider. The provider's own key goes only to its endpoint. | sec 5, D-50 |
| A5 references | The grammar now points to S1.1 and this record series. The lab reference no longer points at section 17 for rules. Traceability table extended. | sec 2, 19, D-51 |
| A6 approval binding | The operation hash and declared destinations are added to the binding list. | sec 4, D-51 |
| A7 attempts | Three total attempts apply to approval-bound active jobs. Passive jobs use the queue policy. | sec 4, D-51, D-30 |
| A8 architecture text | Section 4 tables restored (D-23). P-22 marked superseded. P-24 body updated. P-33, P-34 and P-35 state exactly which parts `security.md` adopted. Platform text corrected. | arch 4, 7, 8, 14, 16, 18, 19 |
| A9 credentials | Secret Service only on the Linux MVP. The dependency tables, product text and architecture text now agree. | sec 7, 10, `structure.md` 10, `product.md` 10, arch 10, 15, D-52 |
| B1 lab lifecycle | Authenticated human, bound to program and assessment, exact kind and permitted range, expiry, revocation, jobs invalidated, audit, never replaces scope validation | sec 2, D-47 |
| B2 policy changes | Authenticated human only. Audit with old and new value. | sec 3, D-53 |
| B3 own endpoints | Derived from configuration and bootstrap registrations, rechecked at connection time | sec 2, D-54 |
| B4 accounting | Consumption at claim. A pre-launch denial is not a launch. No silent restoration. Audited. | sec 4, D-55 |
| B5 intrusive rerun | New job and new approval | sec 4, D-49 |
| B6 timing | 30 seconds for revocation, scope and lab changes. Measurement required. Audit retries bounded. | sec 3, 13, D-48 |
| B7 IPv6 | The local-interface rule is family-neutral | sec 2, D-54 |
| B8 fan-out | Concurrency, total duration and request rate bound in the approval. Blocked without safe defaults. | sec 4, D-55, O-38 |
| B9 remote AI | Resolved and connected address, tunnels and proxies remote, re-resolution cannot change it | sec 5, D-50 |
| B10 policy ownership | One canonical policy store. The authenticated human is the single local researcher account (D-60). | sec 3, D-53 |
| C1 lab CIDRs | /24 and /48 minimum applies | sec 2, D-47 |
| C2 out-of-scope ports | All ports unless narrowed | sec 2, D-57 |
| C3 loopback ports | Any hostname or address resolving to loopback needs explicit ports | sec 2, D-57 |
| C4 retry claims | Reuse the original consumption record | sec 4, D-55 |
| C5 reachable flag | Never authorizes by itself | sec 2, D-57 |
| C6 scope edits | Widening and narrowing both invalidate | sec 2, 3, D-56 |
| D bullets 1 to 7 | Hostname labs bound to ranges, DNS change revalidation, redirects into labs denied, exclusions accepted, deny wins, intrusive disabled until authentication is tested, labs cannot authorize forbidden classes | sec 2, 4, D-47, D-56, D-57, D-58 |

## Interpretive choices to review

These are my readings of the owner's wording. They were applied conservatively and need the owner's confirmation.

1. The CIDR minimum is applied to in-scope rules and lab ranges but not to exclusions. The owner said exclusions stay accepted and deny always wins, and a broad exclusion only denies.
2. "Authenticated human owner/approver" is the single local researcher account of D-60.
3. "High-impact policy changes" that need re-authentication are all changes that widen what can be tested or sent. The list in `security.md` 3 names scope, lab registrations, the CIDR minimum, approval defaults and attempt maximums, risk classification overrides, passive source enablement and remote provider enablement.
4. The canonical security policy store is a single store owned by the policy domain. Its storage technology is not decided here.
5. The 30-second objective is applied to scope edits and lab changes as well as revocation, because the owner said scope changes trigger termination and kept the 30-second objective.
6. Wildcard exclusions are accepted even though wildcard inclusion is refused (D-35, D-57).
7. A lab registration still needs a matching in-scope rule. The owner said registration never replaces program scope validation.
8. Numeric request-rate, concurrency and duration limits could not be established because no tool has been measured, so active operations are blocked (O-38). This is the owner's own fail-closed instruction.
9. The earlier DR-17 wording said credentials are "not sent without explicit user configuration". D-50 supersedes it: target credentials are never sent, and a provider's own key goes only to its endpoint.

10. A refused exclusion is never dropped silently. The assessment is not ready until a human acknowledges it or replaces it with a supported exclusion that denies at least as much. This follows "deny always wins" and "never silently widen scope". It was found by the independent review.
11. A lab registration whose address is assigned to any local interface of the host must list ports, not only a loopback one, and the application must register every listener it starts or depends on (including the web server and PostgreSQL), or no tool operation starts.
12. Active operations need a current target authorization validation, and a retry launch waits until the previous attempt's process tree is confirmed terminated. No policy setting allows a second launch of an intrusive operation under one approval.
13. If no re-authentication window is configured, password entry is required on every intrusive approval and on every policy change that widens what can be tested or sent. Session tokens are stored only as hashes. A secret delivered through a file must live on a memory-backed filesystem inside the job sandbox.
14. P-58 was changed so that metadata addresses are named individually instead of removing whole ranges, to keep IPv4 and IPv6 treatment symmetric.

## Findings of the independent Round 5 review that still need an owner decision

A clean-context reviewer read the Round 5 documents. Findings that were pure inconsistencies or direct consequences of owner decisions were fixed (the stale P-31, P-24 and credential text, the wildcard exclusion wording, the D-61 requirement text, the resource-limit gap statement, the unverified claims in `tech.md`). These were **not** applied because they change requirements or promote proposals:

| Finding | Why it needs the owner | Register |
| --- | --- | --- |
| Which sandbox resource limits are required, and whether a per-job disk quota and an I/O limit are | Requiring them blocks every tool run on the reference host. Not requiring them contradicts "no execution without required limits" only if they are on the list. | O-41, P-63 |
| Per-launch sandbox preflight and a pinned helper digest | New requirement beyond D-42 | P-64 |
| Framing denial for the approval and policy pages | Part of P-35, still Proposed | P-35 |
| Loopback model servers that forward to a cloud service | Locality by connected address (D-50) does not detect it | P-53, threat model T-43 |
| First-run bootstrap code handling, hashed session tokens, DSN password delivery | Mechanism not decided | O-42, O-43 |
| Metadata addresses beyond the two decided | Changes the owner's list | P-58, O-34 |
| Peer authentication cannot separate database roles for same-user processes | Audit append-only and role separation are a design aim until separate credentials or users exist | threat model T-61 |
| Passive job volume and retry limits | No approval binds them, and the queue policy is undefined | O-12, O-38 |
| The validation expiry applied to a running job | Proposed expiry would stop new launches only | P-40 |

## Feasibility

The local spikes ran on 2026-11-15 on the reference host only. Results and limits are in `docs/security/feasibility/sandbox-spike.md` and `docs/security/feasibility/egress-spike.md`. Production tool execution is not authorized by them. The conditional status of D-41, D-42 and D-43 is unchanged except where the register records the observed result.

## Documents changed by this round

`.kiro/steering/security.md`, `product.md`, `structure.md`, `architecture.md`, `tech.md`, this record, `docs/decisions/pending/` (S1, C, README, E1 files, the new password recovery proposal), `docs/security/threat-model.md`, `docs/security/feasibility/`, `docs/development/tech-verification/`, `scripts/spikes/`, and the roadmap in `.kiro/specs/mvp/`.
