# S1: Security semantics for the scope evaluator and tool execution

**Round 6 (2026-11-16): APPROVED as the detailed security specification (D-64).** `security.md` stays the authority for requirement wording. Where this record says "awaiting final owner review" below, that sentence is history. Gates and limits in this record are unchanged. Record: `docs/decisions/0003-foundation-closure-decisions.md`.

Status: owner decisions of Rounds 2, 3 and 5 are incorporated (record: `docs/decisions/0001-owner-security-decisions-round-2.md` for Rounds 2 and 3, `docs/decisions/0002-foundation-completion-decisions.md` for Round 5, decisions D-30 to D-62). The requirements are in `.kiro/steering/security.md`, which is the authority for the requirement wording. This document holds the detailed semantics behind it. Items marked CONDITIONAL depend on a feasibility result. Where a statement below is marked UNVERIFIED and a "Round 5 spike" note follows it, the note records what the local spike on the reference host observed. Anything the spike did not test stays UNVERIFIED or NOT TESTED. No scope evaluator, tool adapter or approval code may be written until the owner approves this revision. Proposals considered and not chosen stay in the alternatives tables as a record.

Covers: O-24 (scope grammar), O-07 and P-24 (resolved addresses and redirects), O-13 (process isolation), P-25 (tool invocation hygiene), P-26 (non-target destinations), P-30 (approval lifecycle). Related: O-06, P-31. The `security.md` wording that expresses these as requirements is in `B1-security.md.diff`.

Basis for the threats: trust boundaries TB1 to TB8 in `.kiro/steering/architecture.md` section 2, the three independent audit reports, and the `security.md` section 18 checklist (threats, trust boundaries, permissions, abuse cases, failure modes, logging, tests).

Facts about the developer's own machine were probed read-only and are not a support baseline (O-22): a recent kernel, `bwrap` present, unprivileged user and network namespaces work, Landlock listed as an LSM, cgroup v2, a Secret Service on the session bus. A reviewer reports that Ubuntu restricts unprivileged user namespaces for unconfined programs through AppArmor. That was not rechecked, and the default on Ubuntu 24.04 is uncertain. Other distributions are UNVERIFIED and must be checked before O-13 is finalized.

Review: this document was reviewed by an independent reader. Corrections from that review are included (exclusion port handling, canonicalization order, IANA registry handling, the limits of the proxy option, the sandbox arguments, hashing the executed bytes, and the approval consume race). Items marked UNVERIFIED were not confirmed against primary sources.

## How to read each item

Each item has: semantics, threats addressed, alternatives, tests that prove it, what it blocks, and the owner decision of Round 2 with what remains open. "Proposed semantics" headings now state the semantics as revised by the owner decisions.

## S1.1 Scope grammar and matching (O-24)

**Blocks:** the scope evaluator, target registration, the first vertical slice. This is the highest-priority item.

### Proposed semantics

Rule kinds supported in the MVP:

| Kind | Example | Matches |
| --- | --- | --- |
| `domain` | `example.com` | Exactly that hostname |
| `wildcard` | `*.example.com` | Any hostname with one or more additional labels under `example.com`. Never the apex `example.com`. |
| `ip` | `203.0.113.7`, `2001:db8::7` | Exactly that address |
| `cidr` | `203.0.113.0/24` | Any address in the range |

Each rule is either in-scope or out-of-scope (an exclusion). An out-of-scope rule or exclusion without a port covers all ports unless it explicitly narrows them. Exclusions are accepted even when the matching inclusion syntax is refused (for example a wildcard exclusion while wildcard inclusion is refused), and the CIDR minimum prefix does not apply to them, because deny always wins (owner decision, Round 5, D-57).

Ports (owner decision DR-07, D-36):

- An in-scope rule with no port set covers ports 80 and 443 only. Other ports must be listed. This is a deliberately conservative default.
- An exclusion with no port set covers all ports 1 to 65535. An exclusion with a port set covers only those ports.
- Ports `0`, `65536`, leading-zero forms such as `08080`, and duplicates are rejected at registration.
- A redirect to a different port is a new target and is evaluated again.

Not supported in the MVP, and refused at registration (fail closed): URL path restrictions, scheme-specific rules other than the port default, regular expressions, and any other construct. Scope is never widened to make such a rule fit. A program whose rules need path restrictions cannot be fully enforced yet, and the product must say so instead of silently widening scope. (Owner decision DR-05, D-34.)

CIDR rules: reject host bits set (`203.0.113.5/24`), a prefix of `/0`, a prefix shorter than the minimum, shorthand such as `10/8`, and any CIDR that falls inside the translation and tunnel ranges listed below. The minimum prefix length is /24 for IPv4 and /48 for IPv6 (owner decision DR-09, D-38). It is changed only through an explicit security policy change, never through AI output or an AI suggestion. The minimum applies to in-scope rules and lab ranges. It does not apply to exclusions, which only deny (D-57).

Canonicalization of a host target, applied to rules at registration and to targets before every comparison. The order matters:

1. Reject empty input, whitespace, control characters, Unicode format characters (for example U+200B and U+202E), and any of `/ \ @ ? # % : [ ] * ,`. The wildcard form is recognized first as the literal prefix `*.` and is removed before this check, then re-added.
2. Remove exactly one trailing dot. A name that is empty after that, or that still ends in a dot, is ambiguous.
3. Convert to ASCII with IDNA processing under an explicitly configured, strict, pinned profile: non-transitional processing, hyphen and joiner checks, bidi rule, DNS length verification and strict domain-name checking. The profile is built explicitly. The package-level default of the Go IDNA library is a permissive profile and is not used (verified in the library documentation). This step maps compatibility characters such as fullwidth digits and the ideographic full stop, and deletes ignored code points, so it must come before the remaining checks. A label that fails validation makes the target ambiguous. How the chosen library behaves under this profile is validation work (O-33), not an established fact.
4. Re-run the character check on the result. Only letters, digits and hyphen are allowed in a label. Underscores are rejected in the MVP. Labels are at most 63 octets and the name at most 253 octets, counted in ASCII form.
5. Lowercase.
6. Decide whether the name is a domain or an address. A name whose last label is all ASCII digits, or `0x` followed by hexadecimal digits, is not a domain. It is an IPv4 address only if it has exactly four decimal parts, each 0 to 255, with no leading zeros. Anything else is ambiguous. This rule runs after step 3.
7. IPv6 literals are parsed to canonical form. Zone identifiers are rejected. IPv4-mapped IPv6 (`::ffff:0:0/96`) is evaluated as its embedded IPv4 address. Other embedded-IPv4 forms (NAT64 `64:ff9b::/96`, `64:ff9b:1::/48`, 6to4 `2002::/16`, Teredo `2001::/32`, and `::/96`) are rejected as ambiguous literals, because the position of the embedded address depends on the prefix. Record 0005 (D-88) adds the IPv4-translated SIIT range `::ffff:0:0:0/96` to this list.
8. A wildcard rule is `*.` followed by a name that passed steps 1 to 5. It matches one or more labels below its base and never the base itself (owner decision DR-06, D-35). The base must have at least two labels. The base must also not be a public suffix. A maintained public suffix data source will be used, subject to license review, an update strategy and deterministic tests. No vendor or license is chosen here (O-32). **Interim rule, confirmed by the owner in Round 3 as intentional fail-closed behavior:** until that source is selected and validated, every wildcard inclusion rule is refused and wildcard exclusions are accepted (D-57), because no base can be confidently distinguished from a public suffix. No temporary or permissive wildcard handling is allowed, and a confirmation by the user alone does not make a wildcard acceptable. Rules of kind `domain`, `ip` and `cidr` are unaffected. The two earlier approaches (reject bases under three labels, which would also reject `*.example.com`, and an audited confirmation for suffix-like bases) are not adopted.

Both the original string and the canonical form are stored. Matching uses only the canonical form. Tools receive only the canonical form, never the original string. The approval and registration screens show both the ASCII form and the Unicode form. Mixed-script labels are refused at rule registration by default, using the restriction levels of Unicode Technical Standard 39. How those restriction levels apply to DNS labels is UNVERIFIED and is validation work (O-33). An exceptional override must be explicit, audited and narrowly scoped to the named label. It is a human action and never an AI action.

Decision order:

1. Normalize the target. Failure means ambiguous, which means deny.
2. If any out-of-scope rule or exclusion matches the name, deny. Exclusions always win.
3. If no in-scope rule matches, deny. Default is deny.
4. Continue to the resolved-address check (S1.2). Every resolved address is also evaluated against all `ip` and `cidr` exclusions, and a match denies. An exclusion on a hostname does not stop the same server being reached by IP or under another name. That residual risk is documented, and the program's own rules text remains the source of truth for such cases.

Where normalization runs: the domain package accepts only canonical ASCII names and verifies that canonicalizing its input returns the same string. The IDNA conversion runs in the application layer, so a library defect cannot widen scope beyond letters, digits and hyphen. This also avoids adding an external library to the domain.

### Threats addressed

Scope bypass through parsing differences: apex versus subdomain, trailing dot, case, lookalike and IDN homographs, numeric IP spellings, userinfo tricks, and exclusion precedence. This is TB4 and the highest-impact failure of the product, since it authorizes active traffic against third parties.

### Alternatives

| Option | Effect |
| --- | --- |
| Wildcard matches exactly one label | Narrower. Matches some program wording. `a.b.example.com` would need its own rule. |
| Absent ports mean all ports | Easier to use. Allows probing of non-standard ports that programs often exclude. |
| Support URL path rules in the MVP | More accurate to some programs. Requires layer-7 enforcement in the egress guard (S1.2). |
| Allow underscores | Covers odd DNS names. Widens the grammar. |

### Tests that must exist before the evaluator is accepted

Table-driven cases, at minimum:

- Case and dots: `Example.COM.` equals `example.com`; `example.com..` and `.` are ambiguous.
- Wildcards (after the public suffix source is in place): `*.example.com` does not match `example.com`, matches `a.example.com` and `a.b.example.com`, and does not match `evil-example.com` or `example.com.evil.net`. An exclusion `example.com` does not cover `a.example.com`. An exclusion `*.x.example.com` does not cover `x.example.com`.
- Public suffix and interim: a wildcard over a public suffix (`*.com`, `*.co.uk`) is refused with and without a user confirmation. Until O-32 is resolved, every wildcard registration is refused with an explicit message, and `domain`, `ip` and `cidr` rules still register. A deterministic test pins the public suffix data version once it exists.
- Hostname exclusion residual risk: a hostname exclusion does not block the same server when it is reached by IP address, and no screen or report claims that it does.
- Numeric hosts: `127.1`, `2130706433`, `0x7f.0.0.1`, `0177.0.0.1`, `127.000.000.001`, `1.2.3.256`, `1.2.3.4.5`, `foo.0x7f` and fullwidth digits `１２７.0.0.1` are all ambiguous.
- IPv6: `::ffff:127.0.0.1` and `::ffff:7f00:1` evaluate as `127.0.0.1`; `::1%eth0`, `64:ff9b::cb00:7107`, `2002::/16`, `::127.0.0.1` and an uppercase-hex literal behave as specified above.
- IDNA: `fuß.de` becomes `xn--fu-hia.de` and not `fuss.de`; `example。com` (ideographic full stop) and `exa\u00admple.com` (soft hyphen) are handled by step 3; an uppercase `XN--` label; an invalid `xn--` label that does not round-trip is ambiguous; a 253-octet name passes and a 254-octet name is denied.
- Hostile forms: `user@host`, a name with an underscore, a 64-octet label, and a mixed-script lookalike (refused at registration by default). An override for a mixed-script label requires an explicit human action, produces an audit event, applies only to the named label, and cannot be performed by an AI actor or an API token.
- Ports: an exclusion with no ports plus an in-scope wildcard with port `8443` denies `:8443`; ports `0`, `65536`, `08080`, `80,80` are rejected.
- CIDR: host bits set, `/0`, `10/8`, a prefix shorter than /24 (IPv4) or /48 (IPv6), and inside the translation ranges are rejected. The minimum cannot be changed by an AI actor.
- Precedence: an exclusion beats a matching in-scope rule; an in-scope name that resolves into an excluded CIDR is denied.

Property and fuzz tests: normalization is idempotent, normalized output never contains a forbidden character, the domain package rejects any string that is not already canonical, and adding an exclusion never turns a denial into an allow.

### Owner decisions (Round 2)

Decided: the four rule kinds and refusal of everything else (DR-05, D-34), wildcard depth D1 with a public suffix source and an interim refusal (DR-06, D-35), the default port set (DR-07, D-36), application-layer canonicalization with a strict pinned non-transitional IDNA profile, refusal of underscores and mixed-script labels by default, and refusal of ambiguous numeric hosts (DR-08, D-37), deny-wins precedence, ambiguity denial, default deny and the /24 and /48 minimum prefixes (DR-09, D-38).

Open: the public suffix source and dependency (O-32), the IDNA library and its behavior under the pinned profile, and UTS #39 validation (O-33), and which pure-computation libraries the `domain` package may import (part of O-24). Round 3 confirmed that wildcard rules stay refused until O-32 is resolved and that the /24 and /48 minimum prefixes stay as set.

## S1.2 Resolved addresses, redirects and DNS (P-24, O-07)

**Blocks:** HTTP probing, any tool that resolves names or follows redirects, and the replay executor (O-31).

### Proposed semantics

The resolved network address is part of the target. For every operation:

1. Resolve the hostname once. Evaluate every returned A and AAAA address. If any returned address is denied, the target is denied. This closes round-robin tricks and the case where only the AAAA record is private.
2. Allowed addresses: IPv6 only inside `2000::/3`, minus every entry of the IANA IPv6 special-purpose registry regardless of its "globally reachable" flag. IPv4 only if the address is in none of the IANA IPv4 special-purpose registry entries, is not in `224.0.0.0/4`, and is not assigned to any local interface. Multicast is absent from those registries (verified in `docs/development/tech-verification/security-libraries-and-data.md` section 4) and must be added explicitly. The embedded-address forms listed in S1.1 step 7 are denied as literals. IPv4-mapped IPv6 is checked as IPv4.
3. The lists are generated from the IANA registries by a documented process and covered by tests. They are not hand-written in prose.
4. Cloud metadata addresses are a separate, non-exemptable deny list. The initial list (owner decision M1, DR-10, D-39) is `169.254.169.254` and `fd00:ec2::254`, which the AWS EC2 documentation gives as the IPv4 and IPv6 instance metadata addresses (verified against the AWS user guide page "Access instance metadata for an EC2 instance"). Further provider addresses are research items and are not on the list (O-34). For information: the Alibaba Cloud ECS documentation uses `100.100.100.200` as its instance metadata address, and the Microsoft Learn page for `168.63.129.16` describes it as the Azure virtual public address used by the VM agent, DNS, DHCP and load balancer probes. Neither is on the list. The generated IANA data would not deny `168.63.129.16`, which is a public address, so until the owner decides, that address is not denied by name. The owner may add either address, which changes the M1 choice. Link-local, metadata and unspecified addresses can never be authorized.
5. **Lab authorization** (owner decisions DR-03 and Round 5, D-32, D-47). It is evaluated before the general denial of non-public addresses, and it covers only the registered target and the permitted address range recorded with it. Every other excluded or non-public resolution stays denied. A denied address class is allowed only for an operation whose own target is a registered literal address, a registered CIDR, or a hostname that an authenticated human registered as a lab target. The registration records the exact target, its kind and the permitted address range, and it applies only to an operation whose own target is that exact target of that kind. A hostname lab lists the range its name may resolve to. A resolution outside the range is denied, and a change in the resolved addresses needs a new evaluation before the next connection. A literal address registered as a lab target does not authorize a hostname that resolves to it, and a registered lab hostname authorizes only operations that target that hostname. A public hostname that resolves into a registered lab range does not inherit the registration. A registration is bound to one program and one assessment, has an expiry, and can be revoked. Expiry, removal or revocation invalidates every queued and running job that depended on it, and running jobs must stop within 30 seconds. The registration never replaces program scope validation: the target must still match an in-scope rule and no exclusion. A lab CIDR obeys the /24 and /48 minimum. A loopback registration, any hostname or address that resolves to loopback, and any registration whose address is assigned to a local interface of the host must list its ports explicitly. A refused exclusion is never dropped silently: the assessment is not ready until a human acknowledges it or replaces it with a supported exclusion that denies at least as much. A registration cannot authorize link-local, metadata, unspecified, multicast, broadcast, reserved or application-owned addresses. The exemption applies only to the initial target of an operation, so a redirect into a lab target is denied and a new, independently authorized operation is needed. Only an authenticated human may register, change or remove a lab target. Every change is audited. The application's own listeners and infrastructure endpoints (the API, the web server, the database, per-job proxies, local model services, and any other endpoint the application runs or depends on) can never be authorized as testing targets, and an explicit registration does not override this. The protected set is derived from configuration and from the endpoints registered at start-up, and the actual connected address is checked against it at connection time (D-54). A lab registration is a network authorization only and never relaxes process isolation (S1.3). The enforcement path must carry the registered initial target of the operation, so that only that connection can use a lab exemption (design requirement for the egress milestone). The application must register every listener and database endpoint it starts or depends on, including the web server and PostgreSQL, and a tool operation does not start while that set is incomplete.
6. Every resolved address is also checked against all `ip` and `cidr` exclusions (S1.1).
7. The connection must go to an address that passed the check, and a second DNS lookup by the tool or the client must not be able to change the answer, so that DNS rebinding between validation and connection is prevented.
8. Every redirect hop is a new target. Canonicalization, scope, port and address checks run again before the hop is followed. Redirect `Location` values in all forms (absolute, scheme-relative `//host`, and backslash forms) go through the canonicalizer. A redirect that cannot be fully evaluated is not followed and the failure is reported.
9. Scope applies to the requested name. A CNAME resolution into excluded or non-public address space is denied, because every resolved address still passes steps 1 to 6 (owner decision DR-11, D-40). Requiring the CNAME target name itself to be in scope was the alternative and was not chosen. Exception: a lab authorization may allow a resolution that this step would otherwise deny, under the conditions of item 5 (D-47).
10. **Local interface rule, IPv4 and IPv6.** An address assigned to any local interface of the host is denied for both address families unless a lab authorization allows it, and a wildcard-bound listener protects every local interface address (D-54).
11. **Revalidation.** A change in the addresses a name resolves to needs a new evaluation before the next connection. A change in scope, program rules or a lab registration, whether it widens or narrows what is allowed, invalidates the target authorization validation and every authorization derived from it for the affected assessment (D-56).
12. **IANA reachable flag.** The globally reachable flag never authorizes an address by itself, and deny always wins (D-57).

Owner decisions covered here: DR-03 (D-32), DR-10 (D-39), DR-11 (D-40). Generation of the IANA lists records the registry source version and date in the generated output, and tests cover the lists. Round 5 added D-47 (lab lifecycle), D-54 (application-owned endpoints and IPv4 and IPv6 symmetry), D-56 (revalidation and invalidation) and D-57 (exclusions, ports, loopback, reachable flag).

### Mechanism options (O-07, direction decided, mechanism CONDITIONAL on the spike)

| Option | What it really covers | Limits |
| --- | --- | --- |
| A. First-party HTTP client | Proxy use disabled in its transport. It resolves names itself, dials `IP:port` with the Host and SNI taken from the canonical name, checks the peer address on every connection attempt (parallel IPv4 and IPv6 attempts must each be checked), disables automatic redirects, and follows redirects through the full evaluation. | Covers only what the first-party client does. The per-attempt check of the Go dialer is UNVERIFIED here and must be confirmed in the spike. [Round 5 spike E3, go1.27.3, reference host: `Dialer.Control` runs for every attempt including the fallback after `FallbackDelay`. A deny from `Control` fails only that attempt, and the dialer then falls back to the other address, which is also checked. Resolving once and dialing one validated IP is simpler and was sufficient (E1, E2). PASS on this host. See `docs/security/feasibility/egress-spike.md`.] |
| B. Policy proxy. External tools run in an isolated network namespace whose only route is a per-job local proxy. | Works for HTTP and HTTPS tools that honor a proxy setting. The proxy resolves, checks and connects, and enforces the port set on `CONNECT`. | Under TLS the proxy sees only `CONNECT host:port`, not the Host header, SNI, path or redirect, so redirects are enforced per connection, not per hop. Request coalescing over one tunnel (HTTP/2) is UNVERIFIED. Reaching a proxy from an empty network namespace without root needs a socket relay or a user-mode network helper (UNVERIFIED). Absolute-URI HTTP requests must also be handled. [Round 5 spike E5 to E8: Go and curl 8.23.0 did not coalesce two names onto one HTTP/2 connection, and the dial hook saw every origin (other clients NOT TESTED). A path-based Unix socket is reachable from a fresh network namespace, and a relay listening on 127.0.0.1 inside it reached only the policy proxy (a small Go relay and socat both worked, and the Go relay is preferred). An abstract socket is not reachable. Direct, UDP, IPv6 and DNS attempts failed because the namespace has no route. Gaps found: HTTPS gives the proxy only CONNECT host:port, a Host override is neutralized but not refused or audited, any process that can reach the proxy socket can use it (so each job needs its own socket), and a running tunnel has no revocation hook.] |
| C. Resolve and pin: pass the checked address to the tool | Simple. | Tool-dependent. Redirects and secondary requests escape it. |
| D. Kernel egress filter allow-listing checked addresses | Enforces at the network layer for any protocol. | Layer 3 and 4 only, with a static address list. Does not stop other virtual hosts on a shared address, or other in-scope names on other addresses. Needs privileges or a more complex setup. |

Not covered by A or B: non-HTTP protocols, UDP including QUIC on 443, ICMP, tool-initiated DNS, and certificate-status or issuer-fetch requests. They are blocked only because the sandbox has no route for them. A tool that needs them is not enabled.

Direction (owner decision DR-12, option 1, D-41): a first-party HTTP client for probing and replay, and an isolated policy proxy for external HTTP tools. No tool may reach the network outside an approved enforcement path, and no tool that can is enabled in the MVP. Port scanners are out of the MVP. This approval covers documentation and a controlled local feasibility spike only. It does not authorize production tool execution.

The spike (scope in `docs/decisions/0001-owner-security-decisions-round-2.md`) must test DNS pinning, redirect behavior, IPv4 and IPv6, Happy Eyeballs, proxy behavior and bypass attempts, using only a local, self-owned lab. It was executed on 2026-11-15 (results below). If the proposed approach fails, the work returns with measured evidence and alternatives. Scope enforcement is not weakened to make a design pass. Until the spike reports, every item marked UNVERIFIED in the table above stays unverified and the mechanism stays CONDITIONAL. Executed on 2026-11-15 on the reference host (Omarchy, kernel 7.2.11-zen1, Go 1.27.3, bubblewrap 0.13.1): results PASS for E1 to E9 and S1 to S12 and S14, S13 NOT TESTED. See `docs/security/feasibility/egress-spike.md` and `docs/security/feasibility/sandbox-spike.md`. Production tool execution is still not authorized, and other hosts are NOT TESTED.

### Threats addressed

TB4: internal-network and cloud-metadata probing through an in-scope name, DNS rebinding between check and use, redirect-based scope escape, and traffic to the worker's own machine.

### Tests

A resolver that returns a public address then `127.0.0.1`; a name with one public and one private address (including AAAA only); a redirect to `169.254.169.254` and to `[fd00:ec2::254]` in absolute, `//host` and backslash forms; a redirect that cannot be evaluated, which must not be followed; a CNAME chain into private space; `::ffff:` and NAT64 forms; a registered lab address that is allowed for its own target while an unregistered neighbor and an in-scope public name pointing into it are denied; a loopback lab registration without ports, which is refused; a registration of the application's own listener address and port, which is refused; an in-scope name resolving into an excluded CIDR; a tool configured to follow redirects that is blocked; a `CONNECT` to `inscope:25`; a Host header that differs from the connected name; a QUIC attempt from the sandbox. Lab registration cases: a literal address registered as a lab target that does not authorize a hostname resolving to it; a registered lab hostname that authorizes only operations targeting that hostname; a registration of the application's own database, web or API endpoint, which is refused even when explicitly requested; a registration of a link-local, metadata or unspecified address, which is refused.

### Owner decisions (Round 2)

Decided: the address policy from generated IANA data plus explicit multicast, the two-address metadata list, "any bad address denies the target" (DR-10, D-39), the narrow lab rule and port-explicit loopback registrations (DR-03, D-32), the redirect and CNAME rules and address pinning (DR-11, D-40), the mechanism direction and the spike (DR-12, D-41), and port scanners out of the MVP.

Open: the Go resolver and dialer behavior including Happy Eyeballs, proxy reachability and coalescing (all UNVERIFIED, O-07), the registry generation process and any further metadata addresses (O-34). Round 5: the Go dialer result and the namespace-to-proxy result are now observed on the reference host (see above). HTTP/2 coalescing in clients other than Go and curl, namespaces that have a route, and non-GET redirect handling remain NOT TESTED.

## S1.3 Process isolation (O-13)

**Blocks:** the first tool adapter. Without it, tools run as the worker's OS user and a parser flaw triggered by hostile target output reaches the keyring session, the database credentials, all evidence and the audit table.

### Proposed semantics (requirements, not a mechanism)

A tool process must not be able to: read the worker's credentials or session bus; read or write outside its own job directory; read evidence or other jobs' directories; reach network destinations other than those allowed by S1.2 and S1.5; outlive its job or its timeout; consume unbounded CPU, memory, processes or output.

If these cannot be provided on the host, the operation must not run. There is no silent fallback and no unsandboxed exception (owner decision DR-02, D-31). No environment variable, configuration flag, development mode or lab registration may bypass isolation. A registered lab may receive a narrowly scoped network authorization (S1.2) and never a process-isolation exemption. Every tool execution requires isolation, including development use.

### Mechanism options (Linux), direction CONDITIONAL on the spike

| Option | Notes |
| --- | --- |
| A. Rootless sandbox with user, mount, PID, IPC, UTS and network namespaces (for example bubblewrap), plus a syscall filter and resource limits | Works without root where unprivileged user namespaces are allowed. Adds an external binary that needs dependency review. Some distributions restrict unprivileged user namespaces. A reviewer reports an AppArmor setting on Ubuntu that restricts them to confined profiles. That was not rechecked, and the default on Ubuntu 24.04 is uncertain. Other distributions are UNVERIFIED. |
| B. Dedicated OS user for tools | Simple model. Needs administrator setup, which is friction for a local-first install. |
| C. Rootless containers | Strong isolation. Heavy dependency. |
| D. Resource limits and process groups only | Not isolation. Does not satisfy the requirements above on its own. |

Direction (owner decision DR-13, D-42): option A, with additional controls, as the preferred feasibility direction. It is a candidate and not an approved implementation. Bubblewrap alone is not claimed to give complete isolation: its documentation states that protection is entirely determined by the arguments given to it (verified in the bubblewrap README). The supported Linux baseline is tested before the final mechanism is chosen. The isolation depends entirely on the arguments given to the sandbox, so the required arguments are part of the requirement, not an implementation detail:

- Unshare every namespace kind (user, IPC, PID, network, UTS, cgroup). A separate PID namespace and a fresh `/proc` are mandatory. Without them a tool running as the same user can read the worker's process environment and memory unless the kernel's ptrace policy blocks it.
- Kill the sandbox when its parent dies. Start a new session so the tool cannot inject input into the controlling terminal.
- Drop all capabilities. Start with an empty environment. Set no-new-privileges.
- Mount only what the tool needs: a read-only view of the pinned tool and templates, a writable job directory, a minimal device tree, and no `/sys`.
- Do not bind the runtime directory, the session bus socket, X11 or Wayland sockets, GPU devices or any credential store.
- Syscall filter at least denying `ptrace`, `bpf`, `io_uring`, `keyctl`, `perf_event_open`, `mount` and creating further user namespaces.
- Close every inherited file descriptor above the standard three before execution (UNVERIFIED whether the sandbox helper does this for you). [Round 5 spike S4: bubblewrap 0.13.1 does not close inherited descriptors, so the launcher must. Python `close_fds=True` with explicit `pass_fds` was verified. Go `ExtraFiles` NOT TESTED.]
- Abstract Unix sockets and shared memory are cut off by the network and IPC namespaces. Landlock scoping of abstract sockets and signals exists only on newer kernels, so record which layers are active in the audit and fail closed when a required layer is missing.
- Resource limits: unprivileged cgroup v2 limits need a delegated cgroup that the user can write, with only the delegated controllers (reviewer-reported, not rechecked). Whether memory and process-count limits are available on the target systems is UNVERIFIED. If any required limit cannot be applied, the job does not run (owner decision F1). [Round 5 spike S9, reference host only: a delegated cgroup with the cpu, memory and pids controllers is usable through `systemd-run --user` scopes. The io controller is not delegated. `RLIMIT_NPROC` cannot be used per job because bubblewrap cannot start under it, so the process count relies on `pids.max`. There is no per-job disk quota. Other hosts NOT TESTED.]
- Round 5 spike findings that change the required arguments: start the sandbox helper itself with an empty environment, because `--clearenv` alone left the parent's environment readable in the helper's own process; mount the sandbox root and the device directory read-only (`--remount-ro`), because the defaults are writable tmpfs; and close inherited descriptors in the launcher. Landlock ABI 10 is available on the reference host, with filesystem restriction and abstract-socket and signal scoping working (S10). The seccomp filter denying the listed syscalls was verified on x86_64 only (S6).
- A worker start-up self-test runs the sandbox with the exact production arguments. If it fails, no tool job runs and the failure is audited.
- Required isolation properties, whatever mechanism is chosen: filesystem, network, process, IPC, capability, system call, inherited file descriptor and resource-limit restrictions.

The supported Linux baseline (O-22) must name the distributions and confirm these on each. PROPOSED (P-64): run a cheap preflight before every launch that checks the helper's digest against its pin and re-checks the required layers, in addition to the full start-up self-test. The spike lists "per-job re-check" and "pin the helper" as needed before production.

### Tests

A tool that tries to: read `/proc/<worker pid>/environ` and write its memory; use an inherited database or keyring file descriptor; connect to an abstract Unix socket; signal or ptrace the worker; read the worker's configuration, the session bus socket, another job's directory and the evidence directory; fork, loop and write without limit; outlive its timeout. Each must fail or be killed. A start-up self-test on a host where the sandbox cannot start must refuse tool jobs. No configuration key, environment variable, development flag or lab registration may cause a tool to run without isolation, and a test searches the code and configuration for any such switch.

### Owner decisions (Round 2)

Decided: no unsandboxed exception (DR-02, D-31), rootless namespaces with additional controls as the preferred feasibility direction, fail closed when any isolation or resource limit is unavailable, and a start-up self-test with the production arguments (DR-13, D-42).

Open (CONDITIONAL): the final mechanism and baseline (O-13, O-22), the UNVERIFIED items above, and whether the baseline distributions allow the required layers. The earlier question of a development-only unsandboxed setting is closed by DR-02, and the contradiction with B1 section 10 is resolved in favor of B1 section 10.

## S1.4 Tool invocation hygiene (P-25)

**Blocks:** every tool adapter.

### Proposed semantics

1. Each adapter defines a schema for every parameter: a type (host, port, enumeration, integer range, path inside the job directory, or an identifier from a pinned allow-list such as a template ID). Free-form strings from a user or an AI are never concatenated into arguments.
2. Fixed flags live in adapter code. Only validated values vary. A value that starts with `-` is rejected in every value slot, not only positional ones. Whether a tool supports the `--` separator is recorded per tool in its manifest and tested, since it is not universal. Values in the forms `@file`, `file:KEYWORD` and values containing carriage return or line feed are rejected. A Host header override is never allowed. `HOME` and the `XDG_*` variables point into the job directory, so tools do not load a user's configuration or provider keys.
3. Secrets never appear in the argument vector, the environment or standard input, which other local users can read through the process list and, for the same user, process environment files. Delivery through an owner-only file in the job directory or an inherited file descriptor is allowed only after the sandbox has been verified to protect it (owner decision, Round 5, D-52). Until delivery is verified for a given tool, a tool that needs a secret is unsupported. The file lives on a memory-backed filesystem inside the job sandbox, is deleted when the job ends and is excluded from evidence collection, and tool output is redacted before storage. A tool that cannot take a secret this way cannot be used for authenticated testing.
4. Tool binaries and templates are pinned in a reviewed manifest in the repository: name, version, source and SHA-256. They run from an absolute path. The worker opens the pinned binary once, hashes the bytes it reads from that open descriptor, and gives the sandbox exactly those bytes (for example through a read-only bind of data from a descriptor). A hash of a path followed by an execution of the path is not accepted as the final guarantee, because the file can change in between. How the verified bytes are given to the sandbox is CONDITIONAL on the spike (whether the sandbox helper supports it is UNVERIFIED). Shared libraries and interpreters are pinned by tree hash, or the tool must be statically linked. The pin is recorded in evidence provenance. [Round 5 spike S11: hashing a path and then running the path failed the swap test for both rename and in-place overwrite. A plain open descriptor survives a rename swap but not an in-place overwrite. Only a sealed in-memory copy of the hashed bytes passed with `--ro-bind-data` guarantees that the bytes run equal the bytes hashed. The pin covers the tool binary only, so shared libraries and interpreters still need a pinned tree or static linking.]
5. The environment is an allow-list. Proxy settings are set only by the platform, never inherited. Every inherited file descriptor the tool is not meant to receive is closed explicitly before execution (S1.3).
6. Stored and logged arguments are the redacted form.

A tool that cannot satisfy these rules is unsupported for authenticated testing.

### Threats addressed

TB3: option injection, secret disclosure through `/proc`, substituted binaries or templates, environment leakage.

### Tests

A host value of `-config /etc/passwd`, `--output=/x`, `-iL /etc/passwd` or `@/etc/passwd`; a path outside the job directory; a Host override; a secret search across argv, environment, files and logs; a binary replaced between hash and execution (the run must use the verified bytes or fail); a binary whose hash differs; an inherited proxy variable.

### Owner decisions (Round 2)

Decided (DR-14, D-43): all six rules and the repository-reviewed tool manifest are approved. A tool that cannot satisfy them is unsupported for authenticated testing. Hash-then-execute on a mutable path is not the final guarantee.

Open (CONDITIONAL): how the verified bytes reach the sandbox (O-13).

## S1.5 Non-target destinations (P-26)

**Blocks:** every tool adapter, and passive reconnaissance.

### Proposed semantics

The tool manifest lists every destination class a tool can contact: the target, the resolved target address, declared passive data sources (by hostname), update servers, template or wordlist sources, out-of-band callback services, and telemetry endpoints. The evaluator allows or denies each class per operation. By default only the target, its checked addresses and the declared passive sources that a human has enabled for the program are allowed. Automatic updates, telemetry, callback services and undeclared destinations are disabled by default and in the tool configuration, and templates and wordlists come from the pinned local store. A tool that cannot disable or govern undeclared destinations is unsupported.

Passive reconnaissance is not free of risk. It sends names that belong to the program to third parties (owner decision DR-15, option C, D-44). A human enables each declared passive source once for each program, as an audited action. The researcher is shown what target information is sent to each passive provider before enabling it, and sees which sources were used. Passive provider disclosure is treated as a privacy and policy decision. A passive operation still passes the policy evaluator with its declared sources, and it needs no per-run approval.

### Threats addressed

Ungoverned traffic and disclosure to third parties, and callback services that leak findings.

### Tests

A tool configured to update itself is blocked at the proxy or refused at startup; a callback host is blocked; a passive source outside the declared list is blocked; a declared source that no human enabled for the program is blocked; the enablement screen lists the target information sent to the source; a manifest missing the destination field is rejected; a tool that cannot disable an undeclared destination is rejected.

### Owner decisions (Round 2)

Decided (DR-15, D-44): per-program one-time enablement for each declared passive source, declared non-target destinations, disabled-by-default updates, telemetry and callbacks, and disclosure to the researcher. This settles whether passive recon needs per-run approval (no). The meaning of "Validate target authorization" stays open under O-28 and C2 item 5.

## S1.6 Approval lifecycle (P-30, O-06)

**Blocks:** approval implementation and job creation for any risky operation.

### Proposed semantics

An approval record binds: approver (a human actor), creation time, expiry, maximum uses, a duration limit for the running operation, the canonical operation (target, tool and pinned version, parameters, rate limits), the risk class, and the declared destinations. It can be revoked. The fields that decide validity (revocation time, expiry, uses and operation hash) are columns of the same row, so a single conditional statement can check them together.

1. **Start and run limits.** Expiry, revocation and the match between the operation and the approval are checked again immediately before the process is launched (the E3 point). This check does not consume a use. Expiry prevents new launches. It does not stop a process that was already launched, and it never extends that process beyond its approved maximum duration. A running operation is killed at its approved duration limit. Revocation, a scope change (widening or narrowing) and a lab revocation invalidate the running authorization and trigger termination of queued and running operations within 30 seconds (owner decisions DR-16 and Round 5, D-45, D-48). The heartbeat interval and kill timing that meet this limit are CONDITIONAL until measured (O-35). A retry that starts after expiry is denied.
2. **Consume atomically at claim.** In the same transaction that leases the job, one conditional statement increments the use count only if the approval is unrevoked, unexpired (using the database clock in the statement), has uses remaining, and matches the operation hash (owner decision DR-01, timing T1, D-30). If it updates no row, the job is marked denied in that transaction. Under the default PostgreSQL isolation, a concurrent update re-evaluates the condition only for the row it targets (verified in the PostgreSQL documentation), which is why every validity field lives on the approval row.
3. **Uses and retries.** One use is recorded per `(approval, job)` pair, enforced by a unique constraint. A second consumption for the same pair is impossible by constraint and by the conditional update. A retry claim reuses the original consumption record. A consumed use is never restored silently, a denial before launch is not a process launch and does not count as an execution attempt, and every denial and failed launch is audited (D-55). An approval-bound active job has at most three execution attempts in total, counting launched processes and including the first (D-30). The maximum attempt count is part of the canonical approval record. Changing the default requires explicit security policy approval, and a change is never made by AI output. An intrusive operation has exactly one process launch: no automatic retry, no retry after a lease expiry and no rerun under the same approval. A manual rerun needs a new job and a new approval (D-49). Each retry of a non-intrusive job still requires the approval to be unexpired and unrevoked, and passes the launch check of item 1. The alternative that each retry consumes a new use (review finding V-07) was not chosen. Passive jobs consume no approval, so their retry limit belongs to the queue design (O-12).
4. **Canonical-form-only approval screen.** The screen renders from the canonical form only: target, the addresses resolved at approval time (informational), declared destinations, limits and expiry. It never shows AI-written rationale as the basis of consent. Target strings are escaped and length-capped.
5. **Protection of the approve action.** Every response carries a policy that forbids framing. The approve action requires an authenticated session, valid Host and Origin, and the operation hash in the request body. The AI actor, workers and API tokens cannot approve, and the first release has no API tokens (D-60). Approval of an intrusive operation requires the approver to enter the account password again within a recency window (D-60). The proposed window is five minutes (P-39). The number stays PROPOSED until the owner confirms it. Intrusive operations stay disabled until local authentication and re-authentication are implemented and tested (D-58).
6. **Initial defaults** by risk class (owner decision DR-16, D-45). They are starting values, not constants. They are configurable only through explicit security policy, and changing them requires owner approval.

| Risk class | Uses | Validity | Retry |
| --- | --- | --- | --- |
| Intrusive | 1 | 15 minutes | None (one attempt) |
| Low-impact active | 5 | 24 hours | Shared use per job, at most 3 execution attempts in total including the first |
| Passive | Policy evaluation and per-program source enablement (S1.5), no per-run approval | Not applicable | Not applicable |

Five uses of a low-impact active approval means five jobs, not five attempts of one job. Each of those jobs may run up to three attempts in total.

7. **Fan-out limits** (D-55). An approval binds a maximum number of concurrent jobs, a total duration and a request rate. Jobs that share an approval must not multiply the approved request rate. The numeric values (request rate, concurrency and duration per active operation) are not established, because no tool has been measured (O-38). Until they are, the affected active operations stay blocked.
8. **Approval classes** (owner decision Round 5, D-59). Passive reconnaissance is Passive. HTTP probing and safe-method replay are Low-impact. State-changing replay and known-vulnerability template checks are Intrusive in the MVP. An operation without a reviewed classification is denied. The class comes from the reviewed tool manifest and the policy engine and never from an AI, and a parameter such as a state-changing HTTP method can raise the class.
9. **Policy changes** (owner decision Round 5, D-53). Policy changes are made only by an authenticated human through the single canonical security policy store. The AI actor, workers and API tokens cannot change policy. Every change is audited with the actor, old value, new value, time and reason, and a change that widens what can be tested or sent needs re-authentication.

### Threats addressed

Approval replay and reuse after redelivery, approvals that outlive the context they were given in, consent obtained through misleading AI text, running work after authorization is withdrawn.

### Tests

An expired approval; a revoked approval during a run (the process tree is gone within 30 seconds); a second claim exceeding the use limit; concurrent claims on a one-use approval; two claims for the same job, which consume one use; a retry that shares the use and stops at the maximum attempt count; an intrusive job that is not retried; a retry after expiry or revocation, which is denied at launch; a revocation racing a claim; a crash between claim and launch; a redelivered claim; a changed parameter that changes the operation hash; an approval screen containing injected AI text; a framed approval page; a cross-origin approve request; an approve request from an AI actor or an API token, which is refused; an intrusive approval without re-authentication, which is refused once the authentication design defines it; a change of a default through anything other than the explicit policy path, which is refused.

### Owner decisions (Round 2)

Decided: the record fields and canonical screen, the claim-time atomic consumption, per-(approval, job) use with shared retries, no automatic retry of intrusive operations, the 30-second revocation limit, re-authentication for intrusive approvals (subject to the authentication design), and the initial defaults (DR-01 D-30, DR-16 D-45).

Open (O-35, O-16): the time limit for stopping operations after a scope edit, heartbeat and kill timing inside the 30-second limit, any other attempt parameters such as a delay between attempts, and the form of re-authentication. The default maximum of three attempts is decided (Round 3). The risk scale and which MVP operations are risky stay under O-06 and C2. `C-product-decisions.md` now refers to D-45 and S1.6 for approval classes and defaults instead of restating them.

## S1.7 Localhost serving (P-19), included for completeness

The localhost web application is its own security mode (owner decision DR-04, D-33, confirmed in Round 3). It does not drop a Desktop protection because it runs in a browser. `security.md` 15 now separates three things. **Shared with the Desktop mode:** restricting local data access, owner-only evidence directories, not exposing backend services unnecessarily, and platform credential storage (Secret Service on Linux, failing closed without it and never falling back to plaintext). **Specific to the local web application:** explicit loopback binding of the API and the web server, and the localhost requirements of `security.md` 8. **Desktop-specific and not applicable:** controls that depend on a native desktop shell or launcher, and the Windows and macOS credential stores, which stay future goals. The UI and API listeners bind to loopback explicitly (the Next.js development and start commands default to all interfaces, verified in the Next.js documentation). The API authenticates even on loopback, validates `Host` and `Origin` for state-changing requests, restricts CORS to configured origins, and protects state-changing requests against cross-site forgery. Cookies are not isolated by port (RFC 6265 section 8.5, verified), so `SameSite` alone is not relied on. No credential travels in a URL. The mechanism details depend on O-02, O-03 and O-16 and on the authentication decision (C2 item 2), and stay open. Authentication (owner decision Round 5, D-60): one local researcher account, a memory-hard password hash (the algorithm and parameters are PROPOSED in P-48), server-side sessions stored in PostgreSQL, an HttpOnly cookie, CSRF protection, rate-limited and audited logins, no API tokens in the MVP, and password re-entry for intrusive approval. Second-factor authentication is deferred. No password reset path exists until a recovery proposal is approved (O-37).

## Summary of what is blocked until the owner approves this revision

| Item | Blocked work | Status of the semantics |
| --- | --- | --- |
| S1.1 O-24 | Scope evaluator, target registration. Wildcard inclusion is refused until O-32 is resolved (intentional fail-closed), exclusions are accepted. | Decided (D-34 to D-38, D-57). Public suffix source and IDNA library PROPOSED (P-56, P-57). |
| S1.2 P-24, O-07 | HTTP probing, replay executor, any network-touching tool (port scanners are out of the MVP) | Address policy, lab lifecycle and redirect rules decided (D-32, D-39, D-40, D-47, D-54, D-56). Mechanism: spike PASS on the reference host only, CONDITIONAL until the production enforcement path is implemented and tested. |
| S1.3 O-13 | First tool adapter | Requirements and fail-closed rule decided (D-31). Mechanism: spike PASS on the reference host only (D-42). Other distributions NOT TESTED. |
| S1.4 P-25 | Every tool adapter | Decided (D-43). Running the verified bytes CONDITIONAL (O-13). |
| S1.5 P-26 | Every tool adapter, passive reconnaissance | Decided (D-44). |
| S1.6 P-30, O-06 | Approval implementation, job creation for risky operations | Decided (D-30, D-45). Default of three attempts decided. Remaining timing open (O-35). |
| S1.7 P-19 | HTTP server skeleton | Decided as a separate mode (D-33). Details open (O-03, O-16). |
