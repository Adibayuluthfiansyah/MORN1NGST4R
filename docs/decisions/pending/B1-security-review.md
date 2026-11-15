# B1: Threat check and review record for the proposed `security.md` additions

**Round 6 (2026-11-16): history.** The requirements this review checks are applied in `security.md`. The threat model review it asked for is `docs/security/threat-model.md`, revised in Round 6.

Status: APPLIED in Round 3. Owner decisions of Rounds 2 and 3 are incorporated into the patch (record `docs/decisions/0001-owner-security-decisions-round-2.md`), and the owner authorized applying it. The patch `B1-security.md.diff` was applied to `.kiro/steering/security.md` after checking that the file matched the patch base (SHA-256 `a6c70466f5667ce2e08b056a3024948c8babfd6ef135dbf7fd453429296c8fca`). The diff stays in this folder as the record of what was applied. It cannot be applied a second time.

Round 2 changes to the patch, in short: retry and consumption rule (DR-01), no isolation exception and no bypass switches (DR-02), narrow lab rule with explicit loopback ports (DR-03), localhost as a separate mode with the Desktop protections restated (DR-04), scope grammar details (DR-05 to DR-09), generated address list and metadata addresses (DR-10), redirect and CNAME rules (DR-11), tool hygiene and passive source enablement (DR-14, DR-15), approval lifecycle and 30-second revocation (DR-16), and the remote provider definition (DR-17). The first-round review table below keeps its original findings. Its "open" statements are marked with the Round 2 outcome.

## Basis

There is no standalone threat model document yet (`security.md` 18 expects one under `docs/security/`). This check therefore uses:

1. The trust boundaries TB1 to TB8 in `.kiro/steering/architecture.md` section 2.
2. Three independent audit reports (security coverage, product readiness, technical correctness) from the foundation audit.
3. The seven points of `security.md` 18, applied to every added requirement: threat, trust boundary, permission, abuse case, failure mode, logging, test.
4. A second independent review of the diff itself, described below.

A standalone `docs/security/threat-model.md` is still needed before the scope evaluator, tool adapters and approval features are built. This document does not replace it.

## What the patch does and does not do

- It is additive. The applied diff has 0 removed lines and 210 non-blank added lines (117 in the first draft, 178 after Round 2), and it applied cleanly. `security.md` itself is byte-identical to its baseline.
- It adds requirements. It does not remove or soften an existing requirement. Where the reviewer found a possible narrowing (the cross-platform test line), the original line is kept and a separate line is added.
- Section numbers do not change.

## Requirement-by-requirement check

Columns follow `security.md` 18. "Fail mode" says what happens when the control cannot work.

| # | Section | Added requirement (short) | Threat and boundary | Abuse case | Fail mode | Logging | Test area (`security.md` 16) | Contradiction or weakening check |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 1 | 2 | Documented scope grammar. Canonical comparison. Ambiguous means deny. | Scope bypass through parsing differences (TB4) | `*.example.com` versus apex, trailing dot, IDN lookalike, numeric IP forms | Deny | Scope evaluation events (already required) | Scope parsing and normalization (added) | Strengthens "ambiguous targets must not be tested". No conflict. |
| 2 | 2 | Exclusions and out-of-scope rules win. | Over-broad inclusion (TB4) | A wildcard in-scope rule swallowing an excluded host | Deny | Same | Scope enforcement (existing) | Consistent with "out-of-scope targets must be denied". |
| 3 | 2 | Every resolved address is evaluated. Non-global addresses denied. Human registration required to allow. Never allow link-local, metadata or unspecified. | In-scope name pointing at internal or metadata addresses (TB4) | DNS answer of `127.0.0.1` or `169.254.169.254` | Deny | Scope evaluation with the address | Resolved-address enforcement (added) | Section 17 local labs stay possible through explicit registration. Not a weakening. Round 2: the exemption is narrowed to operations whose own target is the registered lab target (DR-03), so this row is now stricter than the first draft. |
| 4 | 2 | Connect only to a checked address. Each redirect is a new target. | DNS rebinding and redirect escape (TB4) | Second lookup returns a private address | Deny | Scope evaluation per hop | Resolved-address and redirect enforcement (added) | No conflict. The mechanism is open (S1.2). |
| 5 | 3 | Re-evaluate in the step that starts execution. Cancel on scope change or revocation within a limit. | Stale authorization (TB2) | Scope narrowed while a scan runs | Stop operation | Cancellation and policy change events | Cancellation on scope change (added) | Complements "workers must not trust previously generated plans". |
| 6 | 4 | Approval expiry, use limit, revocation, atomic use, canonical-form screen, worker verifies match. | Replay and consent obtained through AI text (TB1, TB2) | Reused approval; AI-written rationale shown as the basis | Deny | Approval decisions (existing) | Approval expiry, bounded use, revocation (added) | Strengthens `security.md` 4. Round 2: the retry rule is decided (DR-01), see Open items. |
| 7 | 5 | Limit data sent to providers. Remote providers need human enablement per program. Credentials never sent. Audit digest of each transmission. | Data exfiltration to third parties (TB5) | Target data in a cloud prompt; API key in a prompt | Block transmission | AI transmissions (added) | AI data egress (added) | Local-first and provider independence stay possible. A LAN model server counts as remote, which is conservative. |
| 8 | 7 | macOS Keychain entry. | Credential storage on a future platform | None new | Fail safely (existing rule) | Credential configuration events (existing) | Credential storage behavior (existing) | Aligns with `structure.md` 10. Future platform only. |
| 9 | 8 | Localhost web requirements: loopback only, authenticate, Host and Origin allow-lists, CORS, CSRF, not SameSite alone, TLS and review for remote. | Malicious web page or other local user reaching the loopback API (TB1) | Cross-site request to a state-changing endpoint; DNS rebinding to `localhost` | Reject request | Authentication events (existing) | Localhost request validation (added) | Strengthens "CSRF where applicable". Adds a rule for remote access. |
| 10 | 9 | Output cannot execute: CSP without inline scripts, `nosniff`, attachment downloads, per-format escaping, CSV formula neutralization, no tool output as HTML. | Stored script injection through tool output (TB7) | Hostile response body rendered in the UI or export | Neutralize | Evidence access (existing) | Output escaping and download headers (added) | No conflict. |
| 11 | 10 | Isolation required. Unavailable means no run. Unprivileged, no privilege gain, no credentials, own directory only. Argument schema, no option injection, no secrets in args or environment. Pinned and verified binary. Declared destinations governed. Updates, telemetry and callbacks off. | Hostile tool output, option injection, secret leakage, ungoverned traffic (TB3, TB6) | A parser flaw reading credentials; `-config` passed as a host; a secret in `/proc` | Deny and audit | Tool execution (existing), denial audit (added) | Worker isolation (existing), tool argument and secret handling and denial on missing isolation (added) | The "must not run" rule has no development exception. Round 2: the owner confirmed there is none and that no flag or lab registration bypasses isolation (DR-02). |
| 12 | 12 | Backups keep permissions no wider than the source. | Evidence exposure through backups | World-readable backup | Not applicable | Not applicable | Evidence integrity (existing) | Consistent with "restricted database access". |
| 13 | 13 | Extra audit events. Denials and failed authentication audited. No start without audit. Cancellation and denials still work when audit is down. INSERT and SELECT only for runtime role, separate purge role. Extra log-ban items. | Undetected scope edits, export and egress; audit tampering (TB8) | Editing scope without a trace; filling the disk to disable denials | Operation does not start. Safety actions still proceed. | The audit trail itself | Audit write failure and append-only (added) | The review found a contradiction in the first draft. It is fixed (V-01 below). No conflict with retention, because purges run under a separate audited role. |
| 14 | 15 | Local web application deployment section. | MVP deployment gap | Treating loopback as trusted | Not applicable | Not applicable | Localhost request validation (added) | Round 2: the owner chose a separate deployment mode (DR-04). The section no longer inherits the Desktop requirements, and the Desktop protections that apply are restated inside it so none is lost. |
| 15 | 16 | New required test areas. Original "Cross-platform behavior" line kept. | Missing regression coverage | Not applicable | Not applicable | Not applicable | Itself | The reviewer found a narrowing in the first draft. It is fixed (V-02 below). |

## Independent review of the diff (18 findings)

A reviewer with no prior context read the current file, the patch and the product requirements and tried to find weakening, contradiction and untestable wording. Disposition of each finding:

| ID | Finding | Disposition |
| --- | --- | --- |
| V-01 | "Operation must fail if audit cannot be written" would also block revocation and cancellation, and a full disk could disable denials | Accepted. Section 13 rewritten. The same flaw in P-33 of `architecture.md` was fixed too. |
| V-02 | "Cross-platform behavior for each supported platform" narrows an existing line | Accepted. The original line is kept and a separate line added. |
| V-03 | Address rule did not say one bad address denies the target, or that the connection must use the checked address | Accepted. Section 2 rewritten. |
| V-04 | "Private" undefined, no limit on registered ranges, no human requirement, own listeners not excluded | Accepted. Denied classes, human registration, never-authorized classes and the application's own listeners are stated. |
| V-05 | "Cloud provider" and "target data" undefined, credentials not forbidden, audit might hold content | Accepted with changes. A remote provider is any endpoint not on loopback. Credentials are never sent. The audit record holds a digest. |
| V-06 | "Required isolation" undefined | Accepted. Every tool operation requires isolation, and the user is told what is missing. |
| V-07 | "Bounded" approval use is vague, and retries undefined | Partly accepted in round 1. Round 2: the owner chose one use per (approval, job) with shared retries up to a maximum attempt count and no automatic retry of intrusive operations (DR-01). The finding's reading (each retry consumes a use) is kept here as the alternative that was not chosen. |
| V-08 | The approver could be shown AI text next to the canonical form | Accepted. |
| V-09 | "Defined grammar" has no location | Accepted. The grammar is documented in an architecture decision record. |
| V-10 | "Immediately" and "stop" untestable | Accepted with changes. "In the step that starts execution" and "within a configured time limit". |
| V-11 | The localhost sentence was too long and untestable | Accepted. Split into testable bullets. |
| V-12 | "Application accounts" undefined and the retention conflict | Accepted. Roles and the separate purge role are stated. |
| V-13 | "Restrictive" headers untestable | Accepted. |
| V-14 | "No more privilege than required" untestable and the environment left open | Accepted. Unprivileged, no privilege gain, no secrets in arguments or environment, a human-only setting for updates and callbacks. |
| V-15 | "Linux credential storage" ambiguous between Desktop and Headless | Accepted. |
| V-16 | "Keep access restrictions" untestable | Accepted with changes. "No wider than the source". |
| V-17 | Test list missed several new requirements | Accepted. |
| V-18 | Style: long bullets | Accepted. Bullets were split. Some remain one requirement plus its action. |

The reviewer's verdict on the first draft was that weakening was found in V-02 only. After the changes, the diff has 0 removed lines.

## Open items (resolved by owner decisions of Round 2)

1. **Do retries consume another approval use?** Resolved by DR-01 (D-30). One use per (approval, job), consumed at claim. Retries share it up to a maximum attempt count bound in the approval. Intrusive operations are not retried automatically. Expiry, revocation and operation match are checked again at launch. The patch states this in sections 3 and 4. Reviewer finding V-07 is kept as the alternative that was not chosen.
2. **Development exception to "isolation unavailable means no run".** Resolved by DR-02 (D-31). There is no exception. No environment variable, flag, development mode or lab registration bypasses isolation. The patch states this in section 10.
3. **Lab registration of private ranges.** Resolved by DR-03 (D-32). The exemption applies only when the operation's own target is the registered literal address, CIDR or lab-flagged hostname rule, and loopback registrations list ports. The patch states this in section 2.
4. **P-22 interpretation.** Resolved by DR-04 (D-33). The localhost web application is its own security mode. P-22 is superseded.

## Round 3 resolutions

- Wildcard interim: confirmed as intentional fail-closed behavior. Every wildcard rule is refused until the public suffix source is chosen (O-32). `security.md` 2 says so and forbids temporary permissive handling.
- Lab hostname and own listeners: confirmed. `security.md` 2 records the exact target and kind, and never authorizes the application's own listeners and infrastructure endpoints, even by explicit registration.
- Maximum attempts: three execution attempts in total per job, bound in the approval record. `security.md` 4.
- Desktop protections: preserved and classified as shared, specific or Desktop-specific. `security.md` 15.
- `security.md` gained section 19, a decision traceability table. Nothing else was removed or renumbered.

## Still open after Round 3

- The public suffix source (O-32), which keeps wildcard rules refused until it is chosen.
- The stop time after a scope edit, heartbeat and kill timing, and other attempt parameters (O-35).
- The form of re-authentication for intrusive approvals (depends on O-16).
- Whether to name `100.100.100.200` and `168.63.129.16` in the metadata list (O-34).

## Decision requested

Review `security.md` as applied, and the revised S1.
