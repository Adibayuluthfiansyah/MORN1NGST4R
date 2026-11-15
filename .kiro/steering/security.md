# MORN1NGST4R — Security Engineering Standards

## 1. Security Philosophy

Security must be enforced by architecture, not merely by
documentation or user interface restrictions.

Core principles:
- Secure by design.
- Least privilege.
- Explicit authorization.
- Fail closed.
- Evidence integrity.
- Auditability.
- Human verification.
- Defense in depth.

## 2. Authorization and Scope

Every assessment must belong to a registered security program.

Every target must be evaluated against explicit scope rules.

Requirements:
- Explicit target registration.
- In-scope rules.
- Out-of-scope rules.
- Exclusion handling.
- Target validation.
- Operation-specific policy checks.
- Execution limits.
- Approval records.

Out-of-scope targets must be denied.

Unknown or ambiguous targets must not be tested automatically.

Scope rules must follow the grammar recorded in the security semantics record (`docs/decisions/pending/S1-security-semantics.md`, section S1.1, with the owner decisions in `docs/decisions/0001-owner-security-decisions-round-2.md`). A rule outside the grammar must be refused at
registration. The first release supports only domain, wildcard, IP,
and CIDR rules. A URL path, regular expression, unsupported scheme,
or any other construct must be refused. Scope must never be widened to
make an unsupported rule fit.

Rules and targets are compared only in canonical form. A target that
fails to parse, or has more than one possible canonical form, is
ambiguous and must be denied. A target that no in-scope rule matches
must be denied.

Out-of-scope rules and exclusions take precedence over in-scope rules.
Deny always wins. An exclusion must be accepted even when the matching
in-scope syntax is refused, so a wildcard or CIDR exclusion is accepted although a wildcard inclusion is not. An exclusion that the grammar refuses must never be dropped silently. The assessment must not become ready while a refused exclusion exists, unless a human records an explicit acknowledgement bound to the target set or replaces it with a supported exclusion that denies at least as much.

A wildcard rule matches one or more labels below its base and never
the base itself. A wildcard whose base is a public suffix, or cannot
be confidently distinguished from one, must be refused at
registration. A confirmation by the user alone must not make such a
wildcard acceptable. Until a reviewed public suffix source is in place, every wildcard inclusion rule must be refused. A wildcard exclusion is accepted. No temporary or permissive wildcard handling is allowed.

An in-scope rule without a port covers only ports 80 and 443. An out-of-scope rule or exclusion without a port covers all ports unless it
explicitly narrows them. A redirect to a different port is a new
target.

A CIDR rule must be refused when its prefix is shorter than the minimum
set by security policy. The default minimum is /24 for IPv4 and /48
for IPv6. The minimum changes only through an explicit security policy change,
never through AI output. It applies to in-scope rules and to lab
ranges. It does not apply to exclusions, which only deny.

Every address a target name resolves to, both A and AAAA results,
must be evaluated. If any address is denied, the target is denied.

The denied address list must be generated from the IANA
special-purpose address registries and must add multicast addresses
explicitly. It includes loopback, private, link-local, unique-local,
multicast, reserved, and cloud metadata addresses. Addresses that are
not globally routable unicast addresses must be denied. The cloud metadata addresses 169.254.169.254, fd00:ec2::254,
168.63.129.16, fd20:ce::254 and 100.100.100.200 must always be denied.
Each is a named single-address entry that a lab registration cannot
authorize, even when a lab range contains it (D-72). The IANA globally reachable flag does not by itself
authorize an address. An address assigned to any local interface of
the host, IPv4 or IPv6, must be denied unless a lab authorization
allows it under the rules below.

Lab authorization is evaluated before the general denial of non-public
addresses. A denied address class is allowed only for an operation
whose own target is a literal address, a CIDR range, or a hostname
that an authenticated human registered as a lab target. The exemption
covers only the registered target and the permitted address range
recorded with it. Every other excluded or non-public resolution
remains denied. A registration only relaxes the address denial. It
never replaces program scope validation: the target must still match
an in-scope rule and no exclusion. This keeps explicitly registered
local labs possible, which section 17 requires for initial
development. A public hostname that resolves into a registered lab
range does not inherit the registration. Registering an ordinary
hostname does not allow its address.

Lab registration requirements:

- Only an authenticated human may register, change or remove a lab target. An AI agent, a worker or an API token must not.
- A registration is bound to one program and one assessment.
- A registration records the exact target, its kind (address, CIDR range or hostname) and the permitted address range. A hostname lab must list the range its name may resolve to. A resolution outside that range is denied, and a change in the resolved addresses requires a new evaluation before the next connection.
- A registration has an expiry and can be revoked at any time. Expiry, removal or revocation invalidates every queued and running job that depended on it, and running jobs must stop within 30 seconds.
- A CIDR lab range must meet the minimum prefix length above. A literal loopback registration, any hostname or address that resolves to loopback, and any registration whose address is assigned to a local interface of the host must list its ports explicitly.
- A registration cannot authorize link-local, cloud metadata, unspecified, multicast, broadcast, reserved or application-owned addresses.
- The exemption applies only to the initial target of an operation. A redirect into a lab target is denied. A new, independently authorized operation is required.
- Every registration, change, expiry and removal is audited.

The application's own endpoints must never be authorized as testing
targets, and an explicit registration must not override this. The
protected set is derived from configuration and from the endpoints
registered at start-up: the API and web listeners (a wildcard bind
protects every local interface address, IPv4 and IPv6), the database,
per-job proxies, and local model services. The actual connected address must be checked against this set at connection time as well as at registration. The application must register every listener and database endpoint it starts or depends on, including the web server and PostgreSQL, through mandatory configuration. A tool operation must not start while that set is incomplete.

The connection must go to an address that passed evaluation. A later
lookup must not change it. Except where the lab authorization above
allows it, a name that resolves, through a CNAME or otherwise, into an
excluded or non-public address must be denied. Every redirect must be
treated as a new target and evaluated in full before it is followed. A
redirect that cannot be fully evaluated must not be followed.

A tool may reach the network only through an enforcement path that
applies the rules of this section. A change in scope, in program rules
or in a lab registration, whether it widens or narrows what is
allowed, invalidates the target authorization validation and every authorization derived from it for the affected assessment.

Active operations require a current target authorization validation for the assessment. It has three parts: the mechanical scope validation above, a stored reference to the program rules, and a human attestation made by the authenticated researcher. It is bound to the canonical target-set hash and the rules version. A lab target needs lab-specific attestation wording. An AI agent must not attest.

## 3. Policy Enforcement

The backend policy engine is the mandatory authorization boundary.

Every security operation must be checked before execution.

Evaluation must consider:
- Target.
- Program.
- Operation.
- Parameters.
- Risk.
- Approval.
- Limits.
- Current policy state.

AI instructions and frontend controls cannot replace policy enforcement.

Workers must not trust previously generated plans without
revalidating execution policy.

Scope and approval must be evaluated before scheduling and again
immediately before the tool process is launched.

Changing scope, whether it widens or narrows it, and revoking an
approval or a lab registration must cancel queued operations and stop
running operations within 30 seconds. The heartbeat and termination
timing that meets this limit must be measured before it is relied on.

Security policy changes must be made only by an authenticated human,
through the single canonical security policy store of the application.
Policy includes scope, lab registrations, the CIDR minimum, approval
defaults and attempt maximums, risk classification overrides, passive
source enablement and remote provider enablement. An AI agent, a
worker or an API token must not change policy. Every change must be
audited with the actor, the old value, the new value, the time and a
reason. A change that widens what can be tested or sent requires the
human to authenticate again.

## 4. Approval Model

Risky operations require explicit approval.

Approval must be bound to:
- Exact target.
- Exact operation.
- Relevant parameters.
- Risk classification.
- Execution limits.
- Approval timestamp.
- Approving user.
- Canonical operation hash.
- Declared network destinations.

A general approval must not authorize unrelated operations.

Each approval must have an expiry and a maximum number of uses.
Approvals must be revocable.

Expiry prevents new launches. It does not stop a process that was
already launched, and it never extends that process beyond its
approved maximum duration. Revocation and a scope change invalidate
the running authorization and must stop the operation within 30
seconds.

One use is consumed per approval and job. The worker consumes it
atomically when it claims the job, and database constraints must
prevent a second use for the same job. A consumed use is never
restored silently. A denial before launch is not a process launch: it
does not count as an execution attempt, it does not restore the use,
and it is audited, as is every failed launch. A retry claim reuses the
original consumption record for that approval and job.

An approval-bound active job has at most three execution attempts in
total, counting launched processes and including the first, unless
explicit security policy sets a different maximum. The maximum is part
of the canonical approval record, and changing the default requires
explicit security policy approval. Passive jobs use the separate queue
policy.

An intrusive operation has exactly one process launch. It must not be
retried automatically, including after a lease expiry, and it must not
be run again under the same approval. A manual rerun requires a new job and a new approval. No policy setting may allow a second launch of an intrusive operation under the same approval. A retry launch of any job must not start until the process tree of the previous attempt is confirmed terminated.

Expiry, revocation, and the match between the operation and the
approval must be checked again immediately before the tool process is
launched.

An approval must bind a maximum number of concurrent jobs, a total
duration and a request rate. Jobs that share an approval must not
multiply the approved request rate. If a safe numeric default for an
active operation has not been established, that operation must stay
blocked.

The risk classification comes from the reviewed tool manifest and the
policy engine, never from an AI. An operation with no reviewed
classification must be denied. Passive reconnaissance is Passive. HTTP
probing and safe-method replay are Low-impact. State-changing replay
and known-vulnerability template checks are Intrusive.

The expiry, use count, and duration defaults for each risk class are
set by explicit security policy. A change to them requires owner
approval.

Intrusive operations must stay disabled until local authentication and
re-authentication are implemented and tested. Approval of an intrusive
operation requires the approver to enter the account password again
within a recency window set by security policy. If no window is configured, password entry is required for every such approval and for every policy change that widens what can be tested or sent. An AI agent, a worker
or an API token must not be able to approve.

The approval screen must be rendered only from the canonical
operation. Text written by an AI must not be shown as the basis for
approval. The worker must verify that the operation matches the
approval before it starts.

## 5. AI Security

AI output is untrusted.

Requirements:
- AI cannot bypass scope enforcement.
- AI cannot grant its own approval.
- AI cannot modify authorization policies.
- AI cannot independently mark findings Verified.
- AI cannot directly execute arbitrary system commands.
- AI tool calls must pass through controlled backend interfaces.
- Retrieved content must not be treated as trusted instructions.
- Prompt injection must be considered in external content processing.

AI recommendations must remain distinguishable from observed facts.

Data sent to an AI provider must be limited to what the task requires.

A provider whose endpoint is not on this machine's loopback interface
is a remote provider. A provider on the local network is a remote provider. Locality is
classified from the resolved address and from the address actually
connected, at every connection. A forwarding proxy or an SSH tunnel
makes a provider remote. A hostname that later resolves to a different
address must not change the classification, and a connection that does
not match the configured classification must be refused. A trusted
local-network category does not exist. Target data, evidence, and findings must not be sent to a
remote provider unless a human has configured that provider for that
program and has been shown what will be sent.

Target credentials, cookies, access tokens, and session secrets must
never be sent to any AI provider. A provider's own API key may be sent
only to that provider's configured endpoint. Provider authentication
stays separate from target credentials.

Each transmission must be audited with provider, program, size, and a
content digest, not the content. A transmission must not occur if
that audit record cannot be written.

Redaction reduces disclosure but must not be treated as a guarantee.

## 6. Credential Protection

Credentials include:
- API keys.
- Passwords.
- Access tokens.
- Refresh tokens.
- Database credentials.
- Third-party authentication secrets.

Rules:
- Never hardcode credentials.
- Never commit credentials.
- Never expose credentials to frontend bundles.
- Never return stored secrets in API responses.
- Never log secrets.
- Never store plaintext credentials in PostgreSQL.
- Never silently persist credentials in plaintext.
- Never automatically transfer credentials between devices.

## 7. Cross-Platform Credential Storage

### Windows Desktop

Prefer Windows Credential Manager and DPAPI-backed storage.

Use user-scoped credential access by default.

### macOS Desktop

Prefer Keychain.

Use user-scoped credential access by default.

### Linux Desktop

Prefer Secret Service-compatible credential storage.

Support GNOME Keyring and KDE Wallet where available.

Detect unavailable keyring services.

### Linux Headless

Support:
- systemd credentials.
- Environment-based secret injection.
- Restricted mounted secret files.
- External secret managers through adapters.

Environment variables are secret delivery mechanisms,
not encrypted storage.

If secure storage is unavailable, fail safely and explain
the available alternatives.

### Linux Local Web Application (MVP)

Use Secret Service-compatible credential storage only. If it is unavailable, credential features fail closed and the user is told the available alternatives. Do not use persistent plaintext files, environment variables or standard input to hold or deliver a secret. The headless mechanisms above apply to the future headless mode only.

## 8. Authentication and Session Security

Authentication architecture must be appropriate to deployment mode.

Requirements:
- Secure session management.
- Password hashing where passwords are used.
- CSRF protection where applicable.
- Secure cookie configuration.
- Session expiration.
- Session revocation.
- Rate limiting for authentication endpoints.
- No credential exposure in URLs.

Do not assume desktop deployment and headless server
deployment have identical authentication requirements.

A web application served on localhost must not treat the loopback
interface as trusted.

Requirements for a locally served web application:
- Listen only on the loopback interface unless remote access is explicitly configured.
- Set the listen address of the API and of the web server explicitly. Do not rely on a framework default.
- Authenticate requests.
- Reject requests whose Host is not in the configured allow-list.
- Reject state-changing requests whose Origin is missing or not in the configured allow-list.
- Restrict CORS to explicitly configured origins.
- Protect state-changing requests against cross-site request forgery.
- Do not rely on SameSite alone.
- Require transport encryption and a separate security review for remote access.
- Use one local researcher account. Hash its password with a memory-hard function. Keep sessions on the server in PostgreSQL and deliver them in an HttpOnly cookie.
- Rate limit login attempts and password re-entry attempts, and audit them.
- Store session tokens in PostgreSQL only as hashes.
- Create no API tokens in the first release.
- Create the account only through a single-use bootstrap secret. It is generated at start-up while no account exists, stored only as a hash, delivered through a file with owner-only permissions on a memory-backed filesystem, and never printed, logged or placed in a URL. If no memory-backed location is available, account creation is refused (D-68).
- Password recovery uses a one-time recovery code of at least 128 bits, shown once and stored only as a hash. A reset needs the code, revokes every session, is rate limited and audited, and keeps intrusive operations disabled until the password has been entered again. No other reset path exists, and a reset that bypasses authentication is not allowed (D-68).
- Second-factor authentication is deferred.

## 9. Evidence Security

Evidence may contain sensitive data.

Requirements:
- SHA-256 integrity metadata.
- Evidence provenance.
- Access restrictions.
- Credential redaction.
- Sensitive data minimization.
- Safe file handling.
- Configurable retention.
- Controlled export.

Never assume scanner output is safe to display or execute.

Treat all external tool output as untrusted data.

Evidence, tool output, and reports must not be able to execute when
displayed or exported.

Requirements:
- Send a Content-Security-Policy that does not allow inline scripts.
- Send X-Content-Type-Options: nosniff.
- Forbid framing on every response with Content-Security-Policy frame-ancestors 'none' and X-Frame-Options: DENY (D-79).
- Send Content-Disposition: attachment for evidence files.
- Escape output according to each export format.
- Neutralize spreadsheet formula prefixes in CSV exports.
- Never render tool output as HTML in the application origin.

## 10. Process and Worker Security

Requirements:
- Restricted execution permissions.
- Process timeouts.
- Output size limits.
- Cancellation.
- Per-job isolation.
- Safe argument handling.
- No unsafe shell interpolation.
- Controlled working directories.
- Audit execution metadata.

Additional requirements:
- Every tool operation requires filesystem, network, process, IPC, capability, system call, and resource-limit isolation. This includes development use and registered labs.
- The required resource limits are wall-clock timeout, memory, process count, CPU time, file size, output size, and a size-bounded job working directory. A limit that cannot be applied means the operation does not run (D-70).
- Before every launch, check the pinned digest of the sandbox helper and re-check the required isolation layers, in addition to the start-up self-test (D-70).
- Only statically linked tool binaries are supported until the library tree of a dynamic tool is pinned (D-70).
- If any isolation mechanism or resource limit is unavailable, the operation must not run. Audit the denial and tell the user which mechanism is missing.
- No environment variable, configuration flag, development mode, or lab registration may disable or bypass isolation. There is no unsandboxed fallback.
- A start-up self-test must run the isolation with the production arguments. If it fails, no tool operation may run.
- Start the sandbox helper itself with an empty environment, because clearing the environment inside the sandbox does not remove the helper's own copy of it.
- Mount the sandbox root and device directories read-only. Close every inherited file descriptor in the launcher, because the sandbox helper does not.
- Run tools as an unprivileged user without added capabilities.
- Prevent tools from gaining privileges.
- Deny tools access to application credentials.
- Deny tools access to other jobs' directories and to evidence outside their own job directory.
- Close every inherited file descriptor the tool is not meant to receive.
- Start tools with an environment built from an allow-list. Point the home and XDG directories into the job directory.
- Validate arguments against a per-tool schema.
- Never let a value be interpreted as an option. Reject values that begin with a dash, values in @file form, values that contain a carriage return or line feed, and any Host header override.
- Never pass secrets in arguments, environment variables or standard input. Delivery through an owner-only file or an inherited file descriptor is allowed only after the sandbox has been verified to protect it. The file must live on a memory-backed filesystem inside the job sandbox and be deleted when the job ends. Until then a tool that needs a secret is unsupported.
- Redact arguments before they are stored or logged.
- Pin tool binaries and templates to a reviewed version with a SHA-256 digest.
- Verify the pinned binary against its digest before execution.
- Run the exact bytes that were verified, from an absolute path. Hashing a mutable path and then running the path does not satisfy this.
- A tool that cannot meet the argument, secret, or isolation requirements is unsupported for authenticated testing.
- Declare every network destination other than the target in the tool integration.
- Govern declared destinations by policy.
- Keep automatic updates, telemetry, callbacks, and undeclared destinations disabled unless a human policy setting enables them.
- A tool that cannot disable or govern undeclared network destinations is unsupported.
- A human must enable each declared passive reconnaissance source once for each program. Show the researcher what target information is sent to each source.

Do not execute arbitrary commands directly from AI-generated strings.

## 11. API Security

Requirements:
- Input validation.
- Authorization checks.
- Consistent error handling.
- Rate limiting where appropriate.
- Secure secret handling.
- Restricted CORS configuration.
- Request size limits.
- Audit logging.

Do not rely on frontend validation for security.

## 12. Database Security

Requirements:
- Least-privilege database accounts.
- Parameterized queries.
- Migration review.
- Backup considerations.
- Backups of evidence or audit data keep permissions no wider than the source.
- Restricted database access.
- No plaintext API keys.
- No unnecessary sensitive data retention.

## 13. Logging and Audit

Maintain structured audit events for:
- Authentication.
- Authorization decisions.
- Scope evaluations.
- Approval decisions.
- Job creation.
- Job execution.
- Tool execution.
- Evidence access.
- Finding state changes.
- Credential configuration changes.
- Scope and policy changes.
- Lab registrations and passive source enablement.
- Approval use consumption, denials and failed launches.
- Policy changes with the old and new value.
- Session revocation.
- Evidence and report export.
- AI provider transmissions.
- Retention purges.
- Credential access (never the credential value).

Denied operations and failed authentications must also be audited.

An operation must not start if its audit record cannot be written.
Cancellation, revocation, scope narrowing, session revocation, and
denials must still take effect when audit cannot be written. The missing record must be retried and reported. Retries must be
bounded by security policy. When the bound is reached the system must
raise a persistent alert and keep denying new operations.

The database role used by the server and worker must have INSERT and
SELECT but not UPDATE, DELETE, or TRUNCATE on audit tables. Retention
purges must run under a separate role and must be audited.

Logs must not contain:
- API keys.
- Passwords.
- Access tokens.
- Refresh tokens.
- Database credentials.
- Third-party authentication secrets.
- Target session cookies and authorization headers.
- Unredacted sensitive request bodies.

## 14. Supply Chain Security

Requirements:
- Track external dependencies.
- Review dependency licenses.
- Prefer maintained dependencies.
- Pin or manage dependency versions.
- Monitor known vulnerabilities.
- Review scanner and adapter versions.
- Avoid unnecessary dependencies.

## 15. Deployment Security

### Desktop
- Restrict local data access.
- Use OS-native credential storage.
- Protect evidence directories.
- Avoid exposing backend services unnecessarily.

### Local Web Application
Applies to the MVP localhost deployment. It is a separate deployment mode with its own list. It does not drop a Desktop protection because it runs in a browser. A Desktop control that applies to a locally run application is repeated below as a shared protection. A control that depends on a native desktop application is Desktop-specific and does not apply.

Shared with the Desktop mode:
- Restrict local data access to the owning user, including the database and local files.
- Protect evidence directories with owner-only permissions.
- Avoid exposing backend services unnecessarily. Do not expose them beyond the loopback interface unless remote access is explicitly configured and separately reviewed.
- Use platform credential storage. On Linux this is Secret Service-compatible storage as in section 7. If it is unavailable, credential-storage features must fail closed and the user must be told the available alternatives. Never fall back to plaintext.

Specific to the local web application:
- Bind the API and the web server to the loopback interface explicitly. Do not rely on a framework default.
- Apply the localhost requirements in section 8.

Desktop-specific and not applicable here: controls that depend on a native desktop application shell or launcher, and the Windows and macOS credential stores, which stay future compatibility goals.

### Headless Server
- Use dedicated service accounts.
- Restrict filesystem permissions.
- Use secure secret provisioning.
- Minimize network exposure.
- Protect database access.
- Monitor service health.
- Avoid privileged execution.

## 16. Testing Requirements

Security-critical functionality must have regression tests.

Required areas:
- Scope enforcement.
- Out-of-scope denial.
- Approval enforcement.
- Credential redaction.
- Credential storage behavior.
- Evidence integrity.
- Worker isolation.
- Cancellation.
- Authentication.
- API authorization.
- Audit event generation.
- Cross-platform behavior.
- Behavior on each supported platform.
- Scope parsing and normalization.
- Wildcard, public suffix, port, and CIDR rule handling.
- Resolved-address and redirect enforcement.
- Approval expiry, bounded use, single consumption per job, and revocation.
- Cancellation on scope change or revocation.
- Tool argument and secret handling.
- Denial when isolation is unavailable.
- No isolation bypass through a flag, an environment variable, or a lab registration.
- Lab registration lifecycle, address binding, expiry and redirect denial.
- Exclusion acceptance when wildcard inclusion is refused.
- Application-owned endpoint protection for IPv4 and IPv6.
- Approval use accounting, single launch of intrusive operations and rerun denial.
- Policy change authorization and audit.
- Remote AI provider classification by connected address.
- Audit write failure and append-only enforcement.
- AI data egress and provider enablement.
- Output escaping and download headers.
- Localhost request validation (Host, Origin, and CSRF).
- Account bootstrap, recovery code use and single-use behavior.
- Framing denial headers on every response.
- Denial when a required resource limit or the per-launch preflight fails.

## 17. Authorized Testing

Initial development must use:
- Local labs.
- Self-owned systems.
- Explicitly authorized targets.

All assessment activity must respect the applicable
program rules, scope, rate limits, and restrictions.

## 18. Security Review Rule

Before implementing a security-sensitive feature, identify:

1. Threats.
2. Trust boundaries.
3. Required permissions.
4. Abuse cases.
5. Failure modes.
6. Logging requirements.
7. Tests needed to verify the security properties.

Do not consider a security feature complete without
appropriate tests and review.

## 19. Decision traceability

Each requirement area added in the security decision rounds maps to
the decisions that approved it. The register is in
`.kiro/steering/architecture.md` section 19. The owner decisions of
Round 2 and Round 3 are recorded in
`docs/decisions/0001-owner-security-decisions-round-2.md`. The
detailed semantics are in `docs/decisions/pending/S1-security-semantics.md`.

| Requirement area | Sections here | Decisions | Related proposals and open items |
| --- | --- | --- | --- |
| Scope grammar, wildcards, ports, CIDR minimum, canonicalization, exclusions | 2 | D-34 to D-38, D-57 | O-24, O-32, O-33 |
| Resolved addresses, lab registration, own endpoints, redirects, invalidation | 2 | D-32, D-39, D-40, D-47, D-54, D-56 | P-24, O-07, O-34 |
| Egress enforcement direction | 2, 10 | D-41 | O-07 |
| Approval lifecycle, retry, consumption, revocation, classes | 3, 4 | D-30, D-45, D-48, D-49, D-51, D-55, D-58, D-59 | P-30, P-31, P-39, O-06, O-35, O-38 |
| AI provider data egress | 5 | D-46, D-50 | P-34 (partly adopted), O-10, O-36 |
| Localhost web application, authentication | 8, 15 | D-33, D-60, D-68, D-69, D-79 | P-19, P-22 (superseded), O-03, O-16, O-37 (decided), P-48 |
| Tool isolation, no bypass, resource limits | 10 | D-31, D-42 | O-13, O-22 |
| Tool invocation hygiene | 10 | D-43 | P-25 |
| Non-target destinations, passive sources | 10 | D-44 | P-26, O-28 |
| Policy modification | 3 | D-53 | none |
| Credential delivery on Linux | 6, 7, 10 | D-52 | O-17 |
| Target authorization validation | 2 | D-61 | P-40 |
| Output hardening | 9 | approved with B1, D-79 | P-35 (adopted with framing denial) |
| Foundation closure: approval of this file, interpretive choices, authentication bootstrap and recovery, sandbox limits, metadata list, residual risks | 2, 4, 8, 9, 10 | D-63 to D-85 (record `docs/decisions/0003-foundation-closure-decisions.md`) | O-07, O-13, O-22, O-32, O-33, O-35, O-36, O-38 stay open with gates |
| Audit integrity and events | 13 | approved with B1 | P-33 (partly adopted) |
