# 0001: Owner security decisions, Rounds 2 and 3

Status: owner decisions recorded. Round 3 resolved the seven alignment items left by Round 2 and the owner authorized applying the B1 patch. The patch was applied to `.kiro/steering/security.md` in Round 3. The semantics in `docs/decisions/pending/S1-security-semantics.md` are updated and wait for the owner's final review.

Date of the decisions: 2026-11-15, in the working session, as an answer to the Consolidated Security Decision Review (items DR-01 to DR-16 and the remote AI provider definition).

## Purpose and limits

This record states what the owner decided, which files carry the wording, what stays conditional on a feasibility spike, and what is still open. It does not approve any tool, library or vendor, and it does not authorize running any security tool against any target.

The feasibility spike approved in DR-12 and DR-13 is a controlled local experiment only. It was started and completed on 2026-11-15 (see 0002). It may use only a local, self-owned lab. It is not production tool execution.

Each decision has an architecture register entry (`D-30` to `D-46` in `.kiro/steering/architecture.md` section 19). `DR-17` is the remote AI provider definition, which the owner gave as item 30 of the decision sheet.

## Evidence labels

| Label | Meaning |
| --- | --- |
| Owner decision | The owner chose it. It is not a fact claim. |
| Verified | Checked against a primary source by the author of this record. The source is named. |
| Reviewer-reported | A reviewer reported it and it was not rechecked. |
| Unverified | An assumption that needs a test or a source. |

A recommendation made in the review is not evidence. Where this record adopts a recommendation, it is adopted because the owner chose it.

## Decision register

| DR | Register ID | Decision | Status | What stays open |
| --- | --- | --- | --- | --- |
| DR-01 | D-30 | One approval use per (approval, job), consumed atomically at claim. Retries share it, bounded by a maximum attempt count. Intrusive operations are not retried automatically. Expiry, revocation and operation match are checked again before launch. | Decided. Default of three execution attempts per job set in Round 3 | Other attempt parameters (O-35) |
| DR-02 | D-31 | No unsandboxed exception. Isolation is required for every tool execution, including development and registered labs. Failed or missing isolation fails closed. | Decided. Mechanism conditional (see DR-13) | None for the rule |
| DR-03 | D-32 | Narrow lab exemption. Loopback lab registrations list ports. The application's own listeners are never authorized. | Decided. Confirmed and tightened in Round 3 | Registration interface (implementation detail) |
| DR-04 | D-33 | The localhost web application is its own security mode and does not inherit the Desktop requirements. | Decided. Desktop protections preserved in Round 3. Details depend on authentication | O-03, O-16 |
| DR-05 | D-34 | Four rule kinds only: domain, wildcard, IP, CIDR. Everything else is refused at registration. | Decided | None |
| DR-06 | D-35 | A wildcard matches one or more labels and never the apex. A maintained public suffix data source will be used. Until it is selected and validated, wildcard bases that cannot be confidently distinguished from public suffixes are refused. | Decided. Interim fail-closed rule confirmed intentional in Round 3 | O-32 |
| DR-07 | D-36 | Default ports. In-scope rules without a port cover 80 and 443. Exclusions without a port cover all ports. | Decided | None |
| DR-08 | D-37 | Application-layer canonicalization with a strict, pinned, non-transitional IDNA profile. Domain-level check of canonical ASCII. Underscores and mixed-script labels refused by default. | Decided. Library behavior conditional | O-33 |
| DR-09 | D-38 | Exclusions win. Ambiguous, malformed or non-canonical targets are denied. Default deny. Minimum CIDR prefix /24 (IPv4) and /48 (IPv6), changed only by explicit policy. | Decided. /24 and /48 retained in Round 3 | None |
| DR-10 | D-39 | Denied addresses come from generated IANA special-purpose registry data plus explicit multicast. Initial metadata list is two addresses. One bad address denies the target. | Decided | O-34 |
| DR-11 | D-40 | Every redirect hop is a new target. Connection is pinned to a validated address. CNAME resolution into excluded or non-public space is denied. | Decided. Go resolver and dialer behavior conditional | O-07 |
| DR-12 | D-41 | First-party HTTP client for probing and replay. Isolated policy proxy for external HTTP tools. No network access outside an approved path. Port scanners outside the MVP. | Decided as a direction. Conditional on the spike | O-07 |
| DR-13 | D-42 | Rootless Linux namespaces with Bubblewrap and additional controls are the preferred feasibility direction. No execution if any required isolation or resource limit is missing. | Decided as a direction. Conditional on the spike | O-13, O-22 |
| DR-14 | D-43 | Six tool hygiene requirements approved. A tool that cannot meet them is unsupported for authenticated testing. | Decided. Mechanism for running verified bytes conditional | O-13 |
| DR-15 | D-44 | One-time human enablement per program for each declared passive source. Non-target destinations are declared and disabled by default. | Decided | O-28 (meaning of target authorization) |
| DR-16 | D-45 | Canonical approval record and screen. Revocation stops queued and running operations within 30 seconds. Re-authentication for intrusive approvals. Initial defaults per risk class. | Decided. Re-authentication depends on authentication design | O-06, O-16, O-35 |
| DR-17 | D-46 | Any non-loopback AI provider endpoint is remote, including a LAN host. | Decided | O-10 (final egress policy) |

## Decision details

### DR-01 Approval retry and atomic consumption (D-30)

Owner direction: option A, timing T1. One use per (approval, job). The use is consumed in the transaction in which the worker claims the job. Retries of that job share the use up to a maximum attempt count. Intrusive operations are not retried automatically. Expiry, revocation and operation binding are checked again immediately before launch. Database constraints and transactional updates prevent double consumption.

Where it is written: `B1-security.md.diff` section 4 and section 3, `S1` item S1.6.

Notes:
- Round 3: the default is three execution attempts in total per job, including the first. Retries share the original approval consumption. Intrusive operations have one attempt and are not retried. The maximum is bound in the canonical approval record, and changing it requires explicit security policy approval. Passive jobs consume no approval, so their retry limit belongs to the queue design (O-12).
- The review's reviewer finding V-07 read each retry as a new start that consumes a use. The owner chose the other reading. V-07 is kept in `B1-security-review.md` as a recorded alternative.
- The earlier B1 sentence "A use is consumed atomically when an execution starts" conflicted with the claim-time rule and is replaced.

### DR-02 Development sandbox exception (D-31)

Owner direction: no exception. No environment variable, development flag or lab registration bypasses isolation. Registered labs may receive narrowly scoped network authorization and never a process-isolation exemption. Bubblewrap and rootless namespaces are a feasibility candidate and not an approved implementation.

Where it is written: `B1` section 10, `S1` item S1.3. The earlier S1.3 option of an audited development-only unsandboxed setting is removed. The contradiction between S1.3 and B1 section 10 is resolved in favor of B1 section 10.

### DR-03 Lab-range registration (D-32)

Owner direction: option B and own-listener option A. A lab exemption applies only when the operation's own target is the registered literal address, the registered CIDR, or a hostname rule registered as a lab target. A public hostname that resolves into a registered range does not inherit it. Loopback lab registrations list ports. The application's own API, web and database listeners are never authorized.

Where it is written: `B1` section 2, `S1` item S1.2.

Alignment notes:
- The earlier B1 text allowed any denied address once a human registered it. That is replaced by the narrow rule.
- The earlier B1 sentence "Registering a hostname must not allow its address" conflicted with S1.2, which allows a hostname rule flagged as a lab. `security.md` 2 now says that registering an ordinary hostname does not allow its address and that only a hostname registered as a lab target does. This is the one place where the applied text allows something the first B1 draft did not, and the owner confirmed it in Round 3.
- Round 3: the owner confirmed the stronger rule. The application's own listeners and infrastructure endpoints (the API, the web server, the database and any other endpoint the application runs or depends on) are never authorized as testing targets, and an explicit registration does not override this.
- Round 3: the owner confirmed that a hostname may authorize private lab addresses only when it is explicitly registered as a lab target. The registration records the exact target and its kind and applies only to an operation whose own target is that exact target.
- Link-local, cloud metadata and unspecified addresses stay never-authorizable.

### DR-04 Localhost security interpretation (D-33)

Owner direction: option B. The localhost web application is its own deployment and security mode. Required: explicit loopback binding, local authentication, Host allow-list, Origin validation on state-changing requests, restricted CORS and CSRF protection. SameSite alone is insufficient. A missing Secret Service fails closed for credential-storage features, with no plaintext fallback. Details that depend on the authentication decision stay open.

Where it is written: `B1` sections 8 and 15, `S1` item S1.7. Register entry P-22 is superseded by D-33.

Round 3 classification of the existing Desktop protections (`security.md` 15 as applied):

| Desktop protection | Treatment in the local web application mode |
| --- | --- |
| Restrict local data access | Shared. Restated, including the database and local files. |
| Protect evidence directories | Shared. Restated with owner-only permissions. |
| Avoid exposing backend services unnecessarily | Shared. Restated, with remote access only when explicitly configured and separately reviewed. |
| Use OS-native credential storage | Shared in purpose. On Linux it is Secret Service-compatible storage and fails closed without it, with no plaintext fallback. Windows and macOS stores stay future goals. |
| Controls that depend on a native desktop shell or launcher | Desktop-specific. Not applicable to the MVP. |

The localhost-specific requirements (explicit loopback binding, local authentication, Host and Origin checks, CORS, CSRF) are listed separately as specific to the local web application. No protection was removed or weakened.

### DR-05 Rule kinds (D-34)

Owner direction: domain, wildcard, IP and CIDR only. URL paths, regular expressions, unsupported schemes and unknown constructs are refused at registration. Scope is never widened to fit them.

Where it is written: `B1` section 2, `S1` item S1.1. A program whose rules need path restrictions cannot be fully enforced in the MVP, and the product must say so.

### DR-06 Wildcard depth and public suffix (D-35)

Owner direction: D1 and P3. A wildcard matches one or more labels below its base and never the apex. A maintained public suffix list implementation or data source will be used, after license review, an update strategy and deterministic tests. No vendor or license is chosen here. Until the dependency is selected and validated, wildcard bases that cannot be confidently distinguished from public suffixes are refused. A human confirmation alone is not enough for obviously dangerous public-suffix wildcards.

Interim wildcard consequence, confirmed by the owner in Round 3 as intentional fail-closed behavior: without a list, no wildcard base can be confidently told apart from a public suffix, so every wildcard rule is refused until O-32 is resolved. No temporary or permissive wildcard handling is allowed. Rules of kind domain, IP and CIDR are not affected. The S1 reviewer's proposal (reject bases with fewer than three labels, which would also reject `*.example.com`) and the earlier "audited confirmation" approach are not adopted. A hand-written suffix list was also not adopted, because it would replace the reviewed list the owner asked for. A narrower interim would be a new decision.

Where it is written: `B1` section 2, `S1` item S1.1 step 8.

### DR-07 Default ports (D-36)

Owner direction: option A. An in-scope rule with no port covers 80 and 443. An exclusion with no port covers 1 to 65535. Port 0, 65536, leading-zero forms and duplicates are refused. A redirect to another port is a new target. Written in `B1` section 2 and `S1` item S1.1.

### DR-08 IDNA canonicalization (D-37)

Owner direction: conversion in the application layer, with the domain verifying that its input is canonical ASCII. The IDNA profile is explicitly configured, strict, pinned and non-transitional. Underscores are refused. Mixed-script labels are refused at registration by default. An override must be explicit, audited and narrowly scoped. Ambiguous numeric host forms and malformed IPv4 or IPv6 forms are refused. Permissive package defaults are not relied on. Unverified UTS #39 and library behavior is recorded as validation work.

Where it is written: `S1` item S1.1 (steps 1 to 7 and the override sentence). `B1` keeps the general rule that non-canonical or ambiguous targets are denied.

Validation work (not established fact): see O-33 and the register of unverified items below.

### DR-09 Exclusion precedence and CIDR limits (D-38)

Owner direction: exclusions always win. Ambiguous, malformed or non-canonical targets are denied. Default deny. Minimum CIDR prefix is /24 for IPv4 and /48 for IPv6. Changing the minimum requires explicit policy, never AI output. The documented residual risk remains: a hostname exclusion does not automatically block every IP alias or alternate access path, and no document may claim that it does.

The owner chose /24 for IPv4 and /48 for IPv6, and retained both in Round 3. They are not changed silently. A /48 IPv6 prefix covers a very large address space. That is an observation and not a change.

Where it is written: `B1` section 2, `S1` item S1.1.

### DR-10 Address deny classes (D-39)

Owner direction: option A, M1, R1. Generated IANA special-purpose registry data plus explicit multicast handling. Initial metadata list is `169.254.169.254` and `fd00:ec2::254`. Every A and AAAA result is evaluated. If any result is denied, the target is denied. Registry generation, source version and tests are recorded.

Where it is written: `B1` section 2, `S1` item S1.2.

Source status:
- `169.254.169.254` (IPv4) and `fd00:ec2::254` (IPv6) as the EC2 instance metadata addresses: Verified by the author against the AWS EC2 user guide page "Access instance metadata for an EC2 instance" (fetched 2026-11-15).
- `100.100.100.200`: the Alibaba Cloud ECS page "View instance metadata" (fetched 2026-11-15) uses it as the instance metadata address and describes it as a link-local address reachable only from within an instance. It is not on the M1 list. Whether the generated IANA data already denies it is Unverified and depends on the generated list.
- `168.63.129.16`: the Microsoft Learn page "Azure IP Address 168.63.129.16 Overview" (fetched 2026-11-15) describes it as a virtual public IP address used for the VM agent (WireServer), DNS, DHCP and load balancer health probes. The page excerpt read did not call it a metadata service. It is a public address, so the generated IANA data would not deny it. It is not on the M1 list.
- Adding either address to the deny list is a change to the owner's M1 choice and is therefore left to the owner (O-34). Until then the residual gap is stated: the list does not name these two addresses.
- The IANA registries themselves, including the "globally reachable" flag behavior and the absence of multicast, are Reviewer-reported and were not rechecked.

### DR-11 Redirect and DNS revalidation (D-40)

Owner direction: option A. Each redirect hop is a new target and re-runs canonicalization, scope, port and address checks before it is followed. CNAME resolutions into excluded or non-public space are rejected. The connection is pinned to a validated address and DNS rebinding between validation and connection is prevented. A redirect that cannot be fully evaluated is not followed.

The owner did not choose the alternative of requiring the CNAME target name itself to be in scope.

Where it is written: `B1` section 2, `S1` item S1.2. Whether the Go resolver and dialer allow a per-attempt address check, including parallel IPv4 and IPv6 attempts (Happy Eyeballs), is Unverified and is part of the spike.

### DR-12 Egress enforcement (D-41)

Owner direction: option 1. Documentation and a controlled local feasibility spike are approved. Direction: first-party HTTP client for probing and replay, isolated policy proxy for external HTTP tools, no network access outside an approved enforcement path, port scanners outside the MVP. If the approach fails, the work returns with measured evidence and alternatives. Scope enforcement is not silently weakened.

This approval does not authorize production tool execution. See "Feasibility spike scope".

### DR-13 Sandbox mechanism and failure behavior (D-42)

Owner direction: option A as the preferred feasibility direction, failure behavior F1. No execution if any required isolation or resource limit is unavailable. Required properties: filesystem, network, process, IPC, capability, syscall, inherited file descriptor and resource-limit restrictions. A start-up self-test uses the production sandbox arguments. Bubblewrap alone is not claimed to give complete isolation. The supported Linux baseline is tested before the final choice. There is no unsandboxed fallback.

Where it is written: `B1` section 10, `S1` item S1.3.

### DR-14 Tool invocation hygiene (D-43)

Owner direction: option A. All six hygiene requirements are approved: explicit parameter schemas, option and argument injection prevention (including `@file`, CR or LF and Host overrides), job-scoped HOME and XDG paths, secrets never in argv or environment (owner-only file or inherited descriptor), pinned binaries and templates with SHA-256, explicit closing of inherited descriptors, environment allow-list, redaction of stored and logged arguments. A tool that cannot satisfy them is unsupported for authenticated testing. Hash-then-execute on a mutable path is not the final guarantee.

Where it is written: `B1` section 10, `S1` item S1.4. How the exact verified bytes are executed (for example from an open descriptor) is part of the spike, because whether the sandbox helper supports it is Unverified.

### DR-15 Non-target destinations and passive reconnaissance (D-44)

Owner direction: option C. A human enables each declared passive source once for each program. Every tool manifest declares non-target destinations. Updates, telemetry, callbacks and undeclared destinations are disabled by default. A tool that cannot disable or govern undeclared destinations is unsupported. The researcher is shown what target information goes to each passive provider. Passive provider disclosure is treated as a privacy and policy decision.

Where it is written: `B1` section 10, `S1` item S1.5. This answers part of O-28 (whether passive recon passes the policy evaluator). The rest of O-28, the meaning of "Validate target authorization", stays open.

### DR-16 Approval lifecycle (D-45)

Owner direction: approve the canonical approval record and screen. The record binds the exact canonical operation, target, parameters, risk class, declared destinations, approver, timestamp, expiry, use count and maximum duration. The screen renders from the canonical operation, never from AI-written text. AI agents and API tokens cannot approve. Revocation stops queued and running operations within 30 seconds. Intrusive approvals require re-authentication, subject to the authentication design. Expiry and revocation are checked at launch.

Initial defaults, configurable only through explicit security policy and changed only with owner approval:

| Risk class | Uses | Validity | Retry |
| --- | --- | --- | --- |
| Intrusive | 1 | 15 minutes | None |
| Low-impact active | 5 | 24 hours | Shared use per job, bounded attempts |
| Passive | Policy evaluation and per-program source enablement | Not applicable | Not applicable |

Where it is written: `B1` sections 3 and 4, `S1` item S1.6. The 30-second limit is the owner's requirement for revocation. The limit for scope changes is still "a configured time limit" and has no number (O-35). Heartbeat and kill timing must fit inside 30 seconds, which is a design constraint for the queue (O-12).

### DR-17 Remote AI provider definition (D-46)

Owner direction: any non-loopback AI provider endpoint is remote, including a LAN-hosted Ollama or compatible endpoint. Target data, evidence, credentials and findings are not sent to a remote provider without explicit user configuration and disclosure. A trusted-LAN category may be considered later and is not part of this decision.

Where it is written: `B1` section 5. Credentials are never sent to any provider, which is the earlier B1 rule and is unchanged. The final egress policy and prompt retention stay open as O-10. Round 5 note: D-50 (record 0002) supersedes the "credentials" wording of the owner direction above. Target credentials, cookies, tokens and session secrets are never sent to a provider, and a provider's own API key goes only to its configured endpoint.

## Reviewer disagreements

| Item | Interpretations preserved | Status |
| --- | --- | --- |
| Approval retry (V-07 versus S1.6) | V-07: each retry is a new start and consumes a use. S1.6: one use per (approval, job). | Resolved by owner decision DR-01 in favor of S1.6. V-07 is kept as the rejected alternative. |
| Consumption timing (claim versus launch) | Consume at claim, or at launch. | Resolved by DR-01 (T1). Launch re-checks without consuming. |
| Lab registration (V-04 versus S1.2) | Any registered range, or only when the operation's own target is registered. | Resolved by DR-03 (narrow). |
| Development exception (S1.3 versus B1 section 10) | An audited off-by-default setting, or none. | Resolved by DR-02 (none). |
| Wildcard base rule (S1 reviewer's three-label rule versus audited confirmation) | Reject bases under three labels, or confirm manually. | Neither adopted. DR-06 uses a public suffix data source with an interim refusal. |
| Address wording (B1 "globally routable unicast" versus S1.2 registry list) | Prose class list, or generated registry list. | Resolved by DR-10. B1 keeps both: generated list and the original sentence. |
| Remote provider definition (V-05) | A LAN endpoint is remote, or trusted-LAN is local. | Resolved by DR-17. A trusted-LAN category is deferred. |
| Localhost interpretation (P-22 and B1 row 14) | Desktop requirements plus localhost rules, or a separate mode. | Resolved by DR-04 (separate mode). |
| IDNA placement (S1 reviewer versus an earlier S1 draft) | Application layer, or a library inside the domain. | Resolved by DR-08 (application layer). |

## Register of unverified items

These stay unverified until tested or sourced. None is presented as fact in the documents.

| Item | Status | Where it stays visible |
| --- | --- | --- |
| Go `Dialer.Control` runs for each connection attempt, including parallel attempts | Unverified | S1.2, O-07 |
| HTTP/2 request coalescing over a CONNECT tunnel | Unverified | S1.2, O-07 |
| Reaching a rootless proxy from an empty network namespace (relay or user-mode helper) | Unverified | S1.2, O-07 |
| Whether the sandbox helper closes inherited file descriptors | Unverified | S1.3, O-13 |
| Running exactly the verified bytes through the sandbox helper | Unverified | S1.4, O-13 |
| Cgroup v2 memory and process limits on target distributions | Unverified | S1.3, O-13, O-22 |
| Unprivileged user namespace policy on distributions other than Ubuntu, and the Ubuntu 24.04 default | Unverified | S1.3, O-22 |
| Landlock scoping on the baseline kernels (reviewer reports it needs newer kernels) | Reviewer-reported | S1.3, O-13 |
| UTS #39 mixed-script rules applied to DNS labels | Unverified | S1.1, O-33 |
| Behavior of the chosen IDNA library under the pinned profile (reviewer reports on UTS #46 details) | Reviewer-reported and Unverified | S1.1, O-33 |
| IANA registry details: multicast absence, "globally reachable" quirks | Reviewer-reported | S1.2, O-34 |
| Azure `168.63.129.16` and Alibaba `100.100.100.200` handling | Source found, not on the M1 list, owner decision needed | S1.2, O-34 |
| The Linux baseline machine facts (bwrap, namespaces, Landlock, Secret Service present) | Developer machine only, not a support baseline | S1.3, O-22 |

## Feasibility spike scope (approved in Round 2, executed on 2026-11-15)

Authorized: documentation and a controlled local feasibility spike only. Not authorized: production tool execution, any scan of a third-party or unregistered target, any change to application behavior.

The spike must use only a local, self-owned lab. It must measure, and report with evidence:

1. DNS pinning: a resolver that changes its answer between validation and connection.
2. Redirect behavior: absolute, scheme-relative and backslash `Location` forms, and a redirect to another port.
3. IPv4 and IPv6, including a name with one allowed and one denied address.
4. Happy Eyeballs: whether every parallel connection attempt is checked.
5. Proxy behavior: reachability from an isolated network namespace, CONNECT handling and its limits, absolute-URI requests, and request coalescing.
6. Bypass attempts from a sandboxed process: direct sockets, UDP and QUIC, tool-initiated DNS, inherited file descriptors, the worker's process memory and environment, abstract Unix sockets, and the session bus.
7. Sandbox self-test: production arguments, refusal when a layer or a resource limit is missing, on each distribution of the proposed baseline (O-22).
8. Running exactly the verified tool bytes.

If any result contradicts the proposed approach, the spike reports the measurements and alternatives. It does not weaken scope enforcement to make a design pass. The spike ran on the reference host only. Other distributions are NOT TESTED (O-22).

## Open and deferred items created or kept by this round

| ID | Item |
| --- | --- |
| O-06 | Risk scale and which MVP operations are risky stay open. The initial defaults are set by D-45. |
| O-07 | Egress mechanism. Direction decided (D-41). Mechanism conditional on the spike. |
| O-13 | Sandbox mechanism. Direction decided (D-42). Mechanism conditional on the spike. |
| O-24 | Narrowed. Grammar, ports, CIDR and canonicalization are decided (D-34 to D-38). The pure-computation libraries the domain may import, the IDNA library and the public suffix source remain open (O-32, O-33). |
| O-28 | Narrowed. Passive recon gating is decided (D-44). The meaning of "Validate target authorization" remains open (C2 item 5). |
| O-32 | Public suffix data source and library: selection, license review, update strategy, deterministic tests. Until resolved, wildcard rules are refused (D-35). |
| O-33 | IDNA library and pinned profile, and validation of UTS #39 and library behavior. |
| O-34 | Metadata address list: whether to add `100.100.100.200` and `168.63.129.16` (and others) after owner review, and the IANA registry generation process. |
| O-35 | Approval timing and attempt parameters still undefined: the time limit for stopping operations after a scope change, the kill and heartbeat timing that fits inside 30 seconds, and other attempt parameters such as a delay between attempts. The default of three execution attempts per job is decided (Round 3). |
| O-36 | Remote provider categories: a trusted-LAN category is deferred and is not decided. |

## Round 3 resolutions

The owner resolved the seven alignment items of Round 2 and authorized applying B1.

| Item | Resolution | Where |
| --- | --- | --- |
| DR-06 wildcard interim | All wildcard rules stay refused until O-32. Intentional fail-closed. No temporary permissive handling. | D-35, `security.md` 2, S1.1 |
| DR-03 lab hostname | Only a hostname explicitly registered as a lab target. A public hostname resolving into a lab range does not inherit it. Exact target and kind. | D-32, `security.md` 2, S1.2 |
| DR-03 own listeners | Own listeners and infrastructure endpoints are never testing targets. Explicit registration does not override. | D-32, `security.md` 2, S1.2 |
| DR-09 CIDR | /24 IPv4 and /48 IPv6 retained. | D-38, `security.md` 2 |
| DR-01 attempts | Three execution attempts per job including the first. Shared consumption. Intrusive non-retryable. Bound in the approval record. Change needs explicit security policy approval. | D-30, D-45, `security.md` 4, S1.6 |
| DR-04 Desktop protections | Preserved. Shared, specific and Desktop-specific controls separated. | D-33, `security.md` 15, S1.7 |
| C2 overlap | One canonical definition per topic. Authentication and target authorization validation marked UNRESOLVED and DEPENDENT. | `C-product-decisions.md` |

B1 was applied to `.kiro/steering/security.md` after verifying that the file matched the patch base (SHA-256 `a6c70466f5667ce2e08b056a3024948c8babfd6ef135dbf7fd453429296c8fca`), with 0 removed lines. `security.md` 19 now carries a decision traceability table.

Still unresolved and not claimed as verified: O-32, O-33, O-34, O-35, O-36, Go dialer and Happy Eyeballs behavior, HTTP/2 coalescing, proxy reachability from an isolated network namespace, bwrap file descriptor handling, exact verified-byte execution, cgroup limits, the supported Linux baseline, user namespace availability, and the re-authentication mechanism.

## Files changed by Round 2

- `docs/decisions/0001-owner-security-decisions-round-2.md` (this file, new).
- `docs/decisions/pending/B1-security.md.diff`, `B1-security-review.md`, `S1-security-semantics.md`, `README.md` (updated).
- `.kiro/steering/architecture.md` section 19 (register entries only).

Unchanged in Round 2: `.kiro/steering/security.md`, `product.md`, `structure.md`, `tech.md` (empty in Round 2, populated in Round 5), the installed skills, and the diagrams. Round 3 then changed `.kiro/steering/security.md` (B1 applied), `C-product-decisions.md`, the E1 mapping and draft, and the files listed above. `product.md`, `structure.md` and `tech.md` are unchanged and `tech.md` is empty.
