---
inclusion: auto
name: architecture
description: MORN1NGST4R system architecture. Use when designing or implementing backend modules, domain boundaries, ports and adapters, policy and approval enforcement, jobs and workers, evidence, findings and validation, AI orchestration, security tool adapters, persistence, credentials, or the localhost web MVP and future deployment modes.
---

# MORN1NGST4R architecture (steering entry point)

**Round 6 (2026-11-16): STALE draft.** Written against `architecture.md` revision 7. Regenerate before use. Not active.

DRAFT. This is the proposed concise replacement for `.kiro/steering/architecture.md`. It is not active. The detail lives in `docs/architecture/` and every decision lives in `docs/decisions/`. See `docs/decisions/pending/E1-architecture-split-mapping.md`.

This file refines `product.md`, `structure.md` and `security.md`. It does not replace them. Package layout and allowed dependency directions are in `structure.md` sections 3 and 5.6 and are not repeated here. No application code exists yet.

## How decisions are tracked

Every statement in the detail documents is tagged Decided (`D-`), Proposed (`P-`) or Open (`O-`). IDs are never reused. The legend and the registries are in `docs/decisions/README.md`, `decided.md`, `proposed.md` and `open.md`. Treat a Proposed item as a recommendation and an Open item as unsettled. Do not implement against either one without approval.

## MVP delivery model

The MVP is a local-first web application used in a browser on Linux localhost (D-16, D-17). The backend is a Go and Gin modular monolith with a separate Go worker, backed by PostgreSQL, local evidence storage and local security tools (D-01, D-02, D-03). Native desktop packaging, Windows and macOS, and Linux headless are future modes (D-18, D-19, D-20, D-29). A native desktop framework needs explicit owner approval (D-18). No PostgreSQL provisioning strategy is assumed (D-22, O-01).

## Invariants every change must respect

| Invariant | IDs | Detail |
| --- | --- | --- |
| One policy evaluator is the only authorization boundary. It fails closed. Scope and approval are checked before scheduling and again immediately before execution. A worker never trusts an earlier check. | D-05, D-28, P-05 | `docs/architecture/05-policy-and-scope.md` |
| An approval is bound to the exact operation and is never general. One use is consumed per (approval, job) at claim, retries share it, intrusive operations are not retried, and revocation stops work within 30 seconds. | D-06, D-30, D-45 | same |
| Scope rules use only domain, wildcard, IP and CIDR kinds. Ambiguous means deny and exclusions win. Wildcards are refused until a public suffix source is chosen. | D-34, D-35, D-36, D-37, D-38 | same |
| Every resolved address is evaluated and one denied address denies the target. Redirects are new targets. Lab exemptions are narrow and the application's own listeners are never targets. | D-32, D-39, D-40 | same |
| AI output is untrusted. AI proposes typed suggestions, and cannot approve, change policy, run tools or mark a finding Verified. | D-07, P-11 | `06-ai-orchestration.md` |
| A finding stays a Candidate until a human verifies it. Validation status and report status are separate. | D-09, P-10 | `08-evidence-and-findings.md` |
| Evidence has SHA-256 integrity, provenance and access control. All tool output is untrusted. | D-10, P-08, P-37 | same |
| Tools run only through isolated adapters, without a shell and with validated arguments. No flag, environment variable or lab registration bypasses isolation, and missing isolation means no run. Non-target destinations are declared and disabled by default. | D-11, D-31, D-42, D-43, D-44 | `07-tool-adapters.md` |
| A tool reaches the network only through an approved enforcement path (a first-party HTTP client or an isolated policy proxy). Port scanners are outside the MVP. The mechanism is conditional on a feasibility spike. | D-41 | `07-tool-adapters.md` |
| Secrets are never stored in PostgreSQL, logged or sent to the frontend. There is no silent plaintext fallback. | D-08 | `10-credentials-and-deployment.md` |
| Security-relevant events are audited. | D-12, P-33 | `11-errors-audit-testing.md` |
| Domain, application, ports, adapters and infrastructure follow the dependency directions of `structure.md` 5.6. | D-23, D-24, D-25, D-26 | `02-modular-monolith.md` |
| The localhost web application is its own security mode and the loopback interface is not trusted. | D-33, P-19 | `01-vision-and-context.md` (TB1) |
| An AI provider endpoint that is not on loopback is remote, classified by the connected address. Target data, evidence and findings go to a remote provider only after explicit configuration and disclosure. Target credentials never go to any provider. | D-46, D-50, P-34 | `06-ai-orchestration.md` |
| Lab exemptions are evaluated first, bound to a registered target and permitted range, expire, can be revoked, apply to the initial target only, and never cover the application's own endpoints. Exclusions always win and are accepted when inclusion syntax is refused. | D-47, D-54, D-56, D-57 | `05-policy-and-scope.md` |
| Expiry stops new launches only. Revocation and scope or lab changes stop work within 30 seconds. An intrusive operation has one launch and is never rerun on the same approval. Intrusive operations stay disabled until authentication and re-authentication are tested. | D-48, D-49, D-58 | same |
| Approval class comes from the reviewed manifest, never from an AI. Passive, Low-impact and Intrusive membership is fixed. Unknown operations are denied. | D-59 | `07-tool-adapters.md` |
| One local researcher account, memory-hard password hash, server-side sessions, no API tokens. Target authorization validation is a mechanical check, a rule reference and a human attestation. Policy changes need an authenticated human. | D-53, D-60, D-61 | `03-backend-and-frontend.md` |
| Linux credentials use the Secret Service only and fail closed. No secret in argv, environment or standard input. | D-52 | `10-credentials-and-deployment.md` |
| Frontend and backend meet only at the versioned REST API. | D-21, P-20 | `03-backend-and-frontend.md` |
| Tests that need `internal/` packages live inside the Go module. | D-27 | `11-errors-audit-testing.md` |

## Open decisions that block implementation

| Work that is blocked | Blocking decisions |
| --- | --- |
| Scope evaluator and target registration | O-32 (wildcards), O-33 (non-ASCII names) |
| HTTP probing and any network-touching tool | O-07, O-34, O-22 |
| First tool adapter | O-05, O-13, O-22, O-29 |
| Approvals and risky jobs | O-35, O-37, O-38 |
| Persistence adapter | O-01, O-20 |
| Authenticated endpoints and UI serving | O-37, validation of P-49 |
| Findings, reports and validation | O-11, O-27, O-31 |

The detailed semantics for the security items are in `docs/decisions/pending/S1-security-semantics.md` until the owner approves that revision. The implementation plan is `.kiro/specs/mvp/plan.md`. Do not implement the scope evaluator or tool adapters before then, and do not treat a CONDITIONAL item there as an approved mechanism.

## Where things are

| Topic | File |
| --- | --- |
| Vision, principles, system context, trust boundaries | `docs/architecture/01-vision-and-context.md` |
| Modular monolith and components | `docs/architecture/02-modular-monolith.md` |
| Backend request flow, frontend | `docs/architecture/03-backend-and-frontend.md` |
| Assessment lifecycle, jobs, queue and worker | `docs/architecture/04-assessment-and-jobs.md` |
| Scope and policy enforcement | `docs/architecture/05-policy-and-scope.md` |
| AI orchestration | `docs/architecture/06-ai-orchestration.md` |
| Tool adapters | `docs/architecture/07-tool-adapters.md` |
| Evidence, findings, validation | `docs/architecture/08-evidence-and-findings.md` |
| Persistence | `docs/architecture/09-persistence.md` |
| Credentials, deployment | `docs/architecture/10-credentials-and-deployment.md` |
| Errors, audit, testing | `docs/architecture/11-errors-audit-testing.md` |
| Diagrams | `docs/architecture/12-diagrams.md` |
| Technology versions, licences and status | `.kiro/steering/tech.md` |
| Threats | `docs/security/threat-model.md` |
| Decision registries, revision logs, steering alignment | `docs/decisions/` |
