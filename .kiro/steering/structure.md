# MORN1NGST4R — Repository Structure & Engineering Conventions

## 1. Project Identity

- Project: MORN1NGST4R
- Canonical Name: morningstar
- Repository Strategy: Monorepo
- Architecture: Modular Monolith
- Backend Language: Go
- HTTP Framework: Gin
- Frontend: Next.js, React, TypeScript
- Database: PostgreSQL
- Supported Platforms: Linux (MVP). Windows and macOS are future compatibility goals, not initial delivery commitments.
- Deployment Modes: Localhost web application (MVP). Native desktop and Linux headless server are future deployment modes.

## 2. Repository Philosophy

The repository must enforce clear boundaries between:

- Product domains.
- Domain models and business rules.
- Application services and use cases.
- Repository contracts and implementations.
- HTTP transport and middleware.
- Infrastructure and external adapters.
- Platform-specific integrations.
- AI orchestration, providers, and skills.

Rules:

- Each module has one clearly defined responsibility.
- Modules communicate through explicit interfaces and application contracts.
- No circular dependencies.
- No cross-domain database access that bypasses the owning module's contract.
- No business logic in HTTP handlers.
- No business workflows inside repository implementations.
- No direct external tool or LLM provider calls from domain logic.
- Avoid generic shared packages that become dumping grounds.
- Introduce abstractions only when they enforce a real boundary or enable a required replacement.

## 3. Planned Repository Structure

```text
morningstar/
├── .kiro/
│   ├── agents/
│   ├── hooks/
│   ├── specs/
│   └── steering/
│
├── apps/
│   └── web/
│       ├── app/
│       ├── components/
│       ├── features/
│       ├── hooks/
│       ├── lib/
│       ├── providers/
│       ├── services/
│       └── types/
│
├── services/
│   └── core/
│       ├── cmd/
│       │   ├── server/
│       │   ├── worker/
│       │   └── morningstar/
│       │
│       ├── internal/
│       │   ├── bootstrap/
│       │   ├── config/
│       │   ├── domain/
│       │   │   ├── program/
│       │   │   ├── target/
│       │   │   ├── scope/
│       │   │   ├── assessment/
│       │   │   ├── asset/
│       │   │   ├── job/
│       │   │   ├── finding/
│       │   │   ├── evidence/
│       │   │   ├── validation/
│       │   │   ├── approval/
│       │   │   ├── operation/
│       │   │   ├── actor/
│       │   │   ├── identity/
│       │   │   └── audit/
│       │   │
│       │   ├── application/
│       │   │   ├── program/
│       │   │   ├── target/
│       │   │   ├── scope/
│       │   │   ├── assessment/
│       │   │   ├── asset/
│       │   │   ├── job/
│       │   │   ├── finding/
│       │   │   ├── evidence/
│       │   │   ├── validation/
│       │   │   ├── approval/
│       │   │   ├── auth/
│       │   │   ├── ai/
│       │   │   └── reporting/
│       │   │
│       │   ├── ports/
│       │   │   ├── inbound/
│       │   │   └── outbound/
│       │   │
│       │   ├── adapters/
│       │   │   ├── inbound/
│       │   │   │   ├── http/
│       │   │   │   │   ├── handlers/
│       │   │   │   │   ├── middleware/
│       │   │   │   │   ├── request/
│       │   │   │   │   ├── response/
│       │   │   │   │   └── router/
│       │   │   │   └── worker/
│       │   │   │
│       │   │   └── outbound/
│       │   │       ├── persistence/
│       │   │       ├── queue/
│       │   │       ├── credentials/
│       │   │       ├── evidence/
│       │   │       ├── ai/
│       │   │       └── tools/
│       │   ├── infrastructure/
│       │   │
│       │   └── platform/
│       │       ├── windows/
│       │       ├── linux/
│       │       └── darwin/
│       │
│       ├── migrations/
│       ├── queries/
│       ├── tests/
│       │   ├── integration/
│       │   ├── security/
│       │   └── architecture/
│       ├── sqlc.yaml
│       └── go.mod
│
├── packages/
│   └── api-client/
│
├── skills/
│   ├── manifest/
│   ├── security-assessment/
│   ├── reconnaissance/
│   ├── web-security/
│   ├── api-security/
│   ├── authentication/
│   ├── authorization/
│   ├── business-logic/
│   ├── evidence-analysis/
│   └── reporting/
│
├── docs/
│   ├── product/
│   ├── architecture/
│   ├── security/
│   ├── development/
│   └── decisions/
│
├── tests/
│   ├── contract/
│   ├── e2e/
│   ├── security/
│   └── fixtures/
│
├── scripts/
├── data/
├── .env.example
├── .gitignore
├── LICENSE
├── README.md
└── Makefile
```

This is a proposed logical structure. Kiro must review whether every directory is necessary for the approved MVP. Do not create all directories or implementations automatically.

MVP scope of this structure:

- The MVP is a localhost web application on Linux.
- The MVP entry points are `cmd/server/` and `cmd/worker/`. The MVP platform package is `platform/linux/`.
- `cmd/morningstar/`, `platform/windows/` and `platform/darwin/` are reserved for future deployment modes. They are not created until explicitly approved.
- Directories are created when the first code needs them. New domain, application or adapter packages beyond this tree need review.

Test placement:

- Tests that import `internal/` packages are white-box tests. This includes database and adapter integration tests, security regression tests that need internal access, and architecture tests. They live inside the Go module, either beside the package as `_test.go` files or under `services/core/tests/`. The Go `internal/` rule makes packages under `internal/` importable only from the tree rooted at the parent of `internal/`, so a root-level `tests/` directory cannot import them.
- The root `tests/` directory holds black-box tests that use only public interfaces (the HTTP API and the web UI), plus shared fixtures.

## 4. Backend Entry Points and Runtime Responsibilities

Each executable must have one explicit runtime responsibility.

### `cmd/server/`

HTTP API process.

Responsible for:

- Starting the Gin HTTP server.
- Starting the HTTP server that `bootstrap` composes. Routes and middleware are registered by the inbound HTTP adapter and wired in `bootstrap`.
- Initializing application dependencies.
- Exposing API endpoints.
- Graceful shutdown and server lifecycle.

Must not contain business rules or repository implementations.

### `cmd/worker/`

Background job process.

Responsible for:

- Loading worker configuration.
- Initializing job handlers and dependencies.
- Consuming persistent jobs.
- Executing approved background workflows.
- Handling retries, cancellation, timeouts, and graceful shutdown.

Must not expose HTTP routes or duplicate application business logic.

### `cmd/morningstar/`

Evaluate this entry point as the optional Desktop/CLI launcher.

It is not an MVP deliverable. Do not create it, and do not introduce a native desktop framework, without explicit approval.

It may initialize local runtime services or coordinate the desktop application lifecycle. It must reuse the same application and domain packages. Do not duplicate backend logic.

### Shared bootstrap

`internal/bootstrap/` assembles dependencies for each runtime.

Provide separate composition functions for server, worker, and optional desktop/CLI runtime (the optional runtime is not an MVP deliverable). Avoid one global initializer that creates every dependency for every process.

## 5. Backend Layer Responsibilities

### 5.1 Domain

`internal/domain/`

Owns:

- Domain entities and value objects.
- Business invariants.
- Domain policies and state transitions.
- Domain errors.
- Domain events where justified.

Each domain must remain focused on its own business concepts.

Examples:

- `scope`: scope rules and target eligibility.
- `assessment`: assessment lifecycle and state.
- `finding`: finding identity and lifecycle.
- `evidence`: evidence metadata and integrity requirements.
- `approval`: approval state and constraints.
- `operation`: operation descriptor, canonical form, risk class and execution limits. A leaf package that other domains reference.
- `actor`: actor kind (human, system, AI) and identity reference. A leaf package that other domains reference.
- `identity`: user principal and session lifecycle rules. Its contents follow the owner's authentication decision.
- `audit`: audit event model and event categories.

Domain packages may reference each other only through ID types and value objects, following the dependency order in section 5.6.

Domain packages must not import Gin, PostgreSQL drivers, sqlc-generated code, external SDKs, OS-specific credential libraries, or frontend packages.

### 5.2 Application Services

`internal/application/`

Owns application use cases and orchestration.

Examples:

- `program`: create and update programs.
- `target`: register and manage targets.
- `scope`: evaluate whether an action is allowed.
- `assessment`: create, start, pause, and complete assessments.
- `job`: schedule, cancel, and inspect jobs.
- `finding`: create and update candidate findings.
- `validation`: manage evidence-based validation workflows.
- `approval`: request and verify human approvals.
- `auth`: authenticate users and manage sessions.
- `ai`: coordinate AI-assisted tasks through defined ports.
- `reporting`: generate assessment reports.

Rules:

- Each service has a clearly bounded use case responsibility.
- Services coordinate domain objects and ports.
- Services must not depend on Gin or HTTP request/response objects.
- Services must not contain SQL queries or database-driver logic.
- Services must not directly execute arbitrary shell commands.
- Services must not bypass scope, policy, or approval checks.
- Avoid a giant `AssessmentService` or `AIService` that owns unrelated workflows.
- Cross-domain workflows should use explicit application contracts rather than importing another module's internal implementation.

### 5.3 Ports

`internal/ports/`

Defines interfaces at application boundaries.

Separate inbound and outbound contracts.

Inbound ports represent application use cases where useful.

Outbound ports represent dependencies required by application services.

Examples:

- `ProgramRepository`
- `TargetRepository`
- `AssessmentRepository`
- `FindingRepository`
- `EvidenceRepository`
- `JobRepository`
- `JobQueue`
- `CredentialStore`
- `LLMProvider`
- `ToolExecutor`
- `EvidenceStore`
- `AuditSink`

Keep interfaces small and owned by the consumer. Ownership means the application area that uses an interface defines what it needs from it. `internal/ports/outbound/` is therefore organized by consuming application area (for example one file per area), not as one shared file of interfaces. Do not create one universal repository or one universal provider interface for unrelated responsibilities.

### 5.4 Adapters

`internal/adapters/`

Implements external boundaries.

Inbound adapters:

- Gin handlers.
- HTTP middleware.
- Request parsing and validation.
- Response serialization.
- Route registration.
- Worker loop that claims persistent jobs and calls an application use case. It contains no business logic.

Outbound adapters:

- PostgreSQL repository implementations.
- Persistent queue implementation.
- Credential storage implementations.
- Evidence storage implementations.
- LLM provider integrations.
- External security tool integrations.

Adapters translate between external systems and internal contracts. They must not become the owners of business workflows.

### 5.5 Infrastructure and Platform

Infrastructure provides shared technical capabilities such as database connections, logging, telemetry, and queue infrastructure.

`internal/infrastructure/` holds technical code that more than one adapter needs, such as the database connection pool and the transaction helper shared by the persistence and queue adapters. It must not import domain, application, or adapter packages. Adapters may import it. This keeps adapters independent of each other.

Platform integrations handle OS-specific behavior:

- Windows.
- Linux.
- macOS (`darwin`).

The MVP implements Linux only. Windows and macOS are future compatibility goals, and their platform packages are not created until approved.

Use Go build constraints and platform-specific implementations where required. Keep platform-specific code behind shared interfaces.

### 5.6 Allowed Dependency Directions

Imports point toward the domain and the ports. Anything not allowed below is not allowed.

| Package group | May import | Must not import |
| --- | --- | --- |
| `domain/*` | Go standard library. Other domain packages only as set by the dependency order below. | Gin, PostgreSQL drivers, sqlc-generated code, external SDKs, OS credential libraries, frontend packages, `application`, `ports`, `adapters` |
| `application/*` | `domain`, `ports`, and the reviewed pure-computation libraries that `tech.md` lists (for example host name conversion) | Gin or HTTP types, SQL, database drivers, `os/exec`, other application packages' internals (use inbound ports) |
| `ports/*` | `domain` | `application`, anything concrete |
| `adapters/inbound/*` | Inbound ports, `domain` for mapping | Repositories, tool adapters, other adapters |
| `adapters/outbound/*` | Outbound ports they implement, `domain`, `platform`, `infrastructure` | `application`, other adapters |
| `infrastructure/*` | Standard library and drivers | `domain`, `application`, `adapters` |
| `platform/*` | Standard library, OS APIs, and the reviewed OS-binding libraries that `tech.md` lists (for example a Secret Service client) | `application`, domain business rules |
| `bootstrap` | Everything (the only package that composes all runtimes) | Business logic |
| `cmd/*` | `bootstrap`, `config` | Everything else |

Libraries are not added to the domain without explicit review, even when they perform no I/O. A library enters any package group only when `tech.md` lists it with a version, a licence and a reason.

Domain dependency order. A domain package may import only the packages listed for it, and only ID types and value objects:

| Domain package | May import |
| --- | --- |
| `operation` | none |
| `actor` | none |
| `program` | none |
| `identity` | `actor` |
| `audit` | `actor` |
| `target` | `program` |
| `approval` | `operation`, `actor` |
| `scope` | `target`, `operation`, `approval` |
| `assessment` | `program`, `target` |
| `asset` | `assessment`, `target` |
| `job` | `assessment`, `operation`, `approval`, `actor` |
| `evidence` | `assessment`, `job`, `operation` |
| `finding` | `assessment`, `asset`, `evidence` |
| `validation` | `finding`, `evidence`, `job` |

A domain import cycle is a build failure. Fix it by changing which package holds the reference, not by adding a shared package. Application error categories live with the inbound ports so that adapters can map them without importing `application`.

## 6. Repository and Persistence Rules

Repositories are responsible for persistence, not business orchestration.

- Each domain owns its persistence contract.
- Repository implementations live in outbound persistence adapters.
- Use pgx for PostgreSQL connectivity.
- Use sqlc for type-safe SQL generation where appropriate.
- Use Goose for schema migrations.
- Keep SQL queries version-controlled.
- Use transactions for operations requiring atomicity.
- Use explicit context propagation and cancellation.
- Avoid cross-domain table manipulation outside an agreed persistence boundary.
- Do not return database-specific models directly to HTTP handlers.
- Do not put scope or authorization policy exclusively inside SQL queries.

Kiro must propose the appropriate repository and transaction boundaries before implementation.

## 7. Gin HTTP Transport

Gin is the selected HTTP framework.

Keep Gin-specific code isolated in the inbound HTTP adapter.

Expected responsibilities:

- Route groups and versioning.
- Authentication middleware.
- Authorization integration.
- Request validation.
- Rate limiting where required.
- Consistent error handling.
- Request IDs and structured logging.
- Timeouts and graceful shutdown.

Handlers should be thin:

1. Parse and validate transport input.
2. Call an application use case.
3. Map the result to an HTTP response.

Handlers must not implement domain rules, access repositories directly, or invoke external tools directly.

## 8. Frontend Organization

Technology:

- Next.js App Router.
- React.
- TypeScript.
- Tailwind CSS.
- shadcn/ui.
- TanStack Query.
- Zustand where appropriate.
- React Hook Form and Zod.

`apps/web/app/` owns routes, layouts, and route-level boundaries.

`apps/web/features/` organizes product functionality by feature.

`apps/web/components/` contains reusable UI components.

`apps/web/services/` contains typed API communication.

`apps/web/providers/` contains application-level React providers.

Rules:

- Keep API communication separate from UI components.
- Keep security-critical business logic in the backend.
- Never expose provider credentials to browser code.
- Frontend state is not an authorization boundary.
- Prefer generated API types from the OpenAPI contract.

## 9. AI Architecture and Skills

AI must be separated into distinct responsibilities:

1. AI orchestration and task planning.
2. Provider/model integrations.
3. Skill definitions and skill discovery.
4. Tool execution.
5. Policy and approval enforcement.
6. Evidence analysis and output validation.

### Skills

The `skills/` directory is a proposed location for reusable, versioned AI skill packages.

Potential skill categories:

- Reconnaissance and asset analysis.
- HTTP and web security testing.
- API security assessment.
- Authentication and session analysis.
- Authorization and access-control testing.
- JWT analysis.
- Business logic assessment.
- Evidence analysis and finding validation assistance.
- Report generation.

Each skill should have:

- Unique identifier and version.
- Clear purpose and description.
- Explicit input/output contract.
- Required capabilities and tool permissions.
- Execution constraints.
- Relevant references and instructions.
- Tests or evaluation cases where practical.

Skills provide task-specific knowledge and workflow guidance. They are not a security authority.

Mandatory constraints:

- Skills cannot override scope enforcement.
- Skills cannot grant their own approvals.
- Skills cannot directly access unrestricted host execution.
- Tool calls must pass backend policy checks.
- Risky actions require explicit human approval.
- AI output remains a candidate until evidence and human verification are complete.

Do not assume a particular vendor-specific skill format. Evaluate a portable skill manifest and loader design.

## 10. Cross-Platform Support

MVP platform:

- Linux (localhost web application).

Future compatibility goals, not initial delivery commitments:

- Windows Desktop.
- macOS Desktop.
- Linux Desktop (native packaging).
- Linux Headless Server.

Evaluate Windows and macOS headless support as separate deployment requirements rather than assuming they are included.

Platform-specific capabilities include:

- Credential storage.
- Filesystem locations and permissions.
- Process execution.
- Application packaging.
- Desktop integration.
- Service lifecycle.
- Logging and configuration.

Shared business logic must remain platform-independent.

Credential storage should use platform-appropriate secure mechanisms:

- Windows: Windows Credential Manager or appropriate DPAPI-backed storage.
- Linux Desktop: Secret Service-compatible keyring.
- macOS: Keychain.
- Linux Headless: systemd credentials, restricted secret files, environment-based secret injection, or external secret managers.

The MVP uses the Linux Desktop entry (a Secret Service-compatible keyring) only. Credential features fail closed when it is unavailable, and no plaintext, environment-variable or standard-input fallback exists (D-52). The Linux Headless mechanisms apply only to the future headless mode. The other entries apply when those platforms or modes are approved.

Never silently fall back to plaintext credential storage.

## 11. Database and API Contracts

Database:

- PostgreSQL.
- pgx.
- sqlc.
- Goose.

API:

- Gin.
- OpenAPI.
- Generated TypeScript API client where practical.

Rules:

- Backend owns business contracts.
- OpenAPI documents the HTTP API.
- Use explicit migrations.
- Keep API errors consistent.
- Avoid manually duplicating API schemas.
- Keep credentials out of application data tables.

## 12. External Tool Integrations

External tools must be accessed through dedicated adapters.

Potential integrations include:

- Subfinder.
- httpx.
- Nuclei.
- Nmap (outside the MVP: port scanners are not part of the MVP, D-41).
- ffuf (not an MVP integration).
- OWASP ZAP (not an MVP integration).
- Other tools after capability and licensing review.

The MVP selection is recorded in `tech.md`.

Rules:

- No direct CLI execution from HTTP handlers or domain packages.
- Validate target scope before scheduling tool execution, and again immediately before execution. The worker must not trust an earlier check.
- Apply execution timeouts and resource limits.
- Capture tool version, sanitized arguments, exit status, and output metadata.
- Normalize results into internal contracts.
- Treat tool output as untrusted input.
- Keep each tool integration replaceable.

## 13. Testing Organization

- Unit tests close to the packages they test.
- Integration tests for database and adapter behavior. These import `internal/` packages and live inside the Go module (see section 3).
- Contract tests for API and external adapters.
- End-to-end tests for complete assessment workflows.
- Security regression tests for scope, approval, authorization, and evidence integrity. Tests that need `internal/` access live inside the Go module. Black-box tests of the public interfaces live in the root `tests/` directory.
- Architecture tests that enforce the dependency directions in section 5.6. These live inside the Go module.
- Platform-specific tests for the supported platform (Linux in the MVP). Windows and macOS tests are added when those platforms are approved.

## 14. Documentation and Architecture Decisions

`docs/product/`: product requirements and workflows.

`docs/architecture/`: system architecture and data flows.

`docs/security/`: threat models and security boundaries.

`docs/development/`: setup and contribution guides.

`docs/decisions/`: Architecture Decision Records.

Document decisions that affect module boundaries, security controls, platform support, or external dependencies.

## 15. Repository Hygiene

- Never commit real credentials or private secret stores.
- `.env.example` contains placeholders only.
- Ignore local runtime data and build artifacts.
- Review dependency licenses.
- Keep changes small and reviewable.
- Add tests for security-critical changes.
- Update documentation when implementation changes.

## 16. Engineering Enforcement

Before implementation, establish:

- Allowed package dependency directions.
- Domain ownership and cross-domain communication rules.
- Application service boundaries.
- Repository ownership and transaction boundaries.
- Adapter contracts.
- Platform interface contracts.
- AI skill loading and execution boundaries.

Architecture tests or static checks should enforce critical dependency rules where practical. The allowed package dependency directions are set in section 5.6.

Do not introduce unnecessary abstractions, services, packages, or infrastructure simply to satisfy this document. Any proposed deviation must include its rationale and be reviewed.
