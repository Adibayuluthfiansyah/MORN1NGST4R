# MORN1NGST4R — Product Definition

## 1. Product Identity

- Official Name: MORN1NGST4R
- Canonical Technical Name: morningstar
- Category: AI-Powered Security Assessment Platform
- License: Apache-2.0
- Target Users: Independent Bug Bounty Hunters and Security Researchers
- Deployment: Local-first web application on Linux localhost (MVP). Native desktop and Linux headless server are future deployment modes.
- Product Approach: Local-first and Open Source

## 2. Product Vision

Build an extensible security assessment platform that helps
security researchers manage the complete lifecycle of authorized
security assessments.

MORN1NGST4R combines reconnaissance, asset intelligence,
controlled security testing, evidence validation, AI-assisted
analysis, and human verification into one integrated workflow.

## 3. Core Philosophy

"Evidence-driven security assessment, with human-verified findings."

The platform must prioritize verifiable evidence over automated
assumptions.

A scanner result or AI-generated hypothesis must never
automatically become a verified vulnerability.

## 4. Problems to Solve

Security researchers frequently work with disconnected tools,
fragmented evidence, repeated manual workflows, and inconsistent
finding documentation.

MORN1NGST4R aims to provide:

- Centralized assessment management.
- Continuous asset and endpoint inventory.
- Structured security testing workflows.
- Evidence provenance and integrity.
- Reproducible finding validation.
- AI-assisted analysis and planning.
- Consistent reporting.

## 5. Product Pillars

### 5.1 Evidence Validation

Primary product differentiator.

Responsibilities:
- Evidence collection.
- Request and response storage.
- Evidence integrity verification.
- Reproduction workflow.
- Validation result tracking.
- False-positive classification.
- Human verification lifecycle.

### 5.2 Asset Intelligence

Responsibilities:
- Domain and subdomain inventory.
- IP and service discovery.
- HTTP service identification.
- Technology fingerprinting.
- Endpoint and parameter inventory.
- Asset relationship mapping.
- Asset observation history.

### 5.3 Advanced Security Assessment

Assessment capabilities may include:

- Authentication security.
- Authorization and access control.
- API security.
- JWT and session security.
- Input validation.
- Business logic testing.
- Common web application vulnerabilities.

Advanced capabilities must be developed incrementally.

### 5.4 AI Agent Orchestration

Responsibilities:
- Assessment planning.
- Context-aware analysis.
- Asset analysis.
- Testing recommendations.
- Tool selection assistance.
- Evidence interpretation.
- Research assistance.
- Reporting assistance.

AI must remain under backend-controlled policy enforcement.

## 6. Core User Workflow

1. Register a security program.
2. Define authorized targets and scope restrictions.
3. Create an assessment.
4. Validate target authorization (mechanical scope validation, a stored program-rule reference and a human attestation, as decided in D-61).
5. Perform reconnaissance.
6. Build an asset inventory.
7. Analyze discovered attack surface.
8. Plan controlled security testing.
9. Obtain approval when required.
10. Execute approved tests.
11. Collect evidence.
12. Validate candidate findings.
13. Perform human verification.
14. Generate assessment reports.

## 7. Finding Lifecycle

Initial lifecycle:

- Candidate
- Needs Review
- Inconclusive
- False Positive
- Not Reproducible
- Awaiting Human Verification
- Verified

Validation status and report status must remain separate.

Automated systems may recommend a validation result but cannot
independently assign final human verification.

## 8. MVP Scope

The first MVP must complete one end-to-end authorized assessment.

Required capabilities:

- Program management.
- Target and scope management.
- Scope enforcement.
- Assessment management.
- Persistent job execution.
- Passive reconnaissance.
- HTTP probing.
- Asset inventory.
- Endpoint inventory.
- At least 2-3 external tool integrations.
- Evidence collection.
- Finding management.
- Validation workflow.
- Human verification.
- Basic AI-assisted analysis.
- Report generation.

## 9. Explicit MVP Exclusions

Do not prioritize these before the first complete vertical slice:

- Enterprise multi-tenant SaaS.
- Complex distributed microservices.
- Kubernetes orchestration.
- Autonomous exploitation.
- Fully autonomous vulnerability verification.
- Advanced business logic engine.
- Large-scale graph database infrastructure.
- Every possible scanner integration.

These may be considered in future releases when justified.

## 10. Deployment Requirements

### MVP: Localhost Web Application

- Local-first web application used through a browser on localhost.
- Initial development and primary MVP environment: Linux.
- Components: web frontend, backend API, background worker, PostgreSQL, local evidence storage, and local security tool integrations.
- Secret Service-compatible credential storage on Linux. Credential features fail closed when it is unavailable.
- Locally run background worker.
- Native desktop packaging is not an MVP requirement.
- Native desktop frameworks are not introduced into the MVP without explicit owner approval.

### Future Deployment Modes

These are compatibility goals. They are not initial delivery commitments, and the MVP must keep its boundaries clean enough to make them possible.

#### Native Desktop

- Windows.
- macOS.
- Linux.
- Local-first operation.
- OS-native credential storage.
- Locally managed workers.

#### Headless Server

- Linux server environments.
- Non-interactive service execution.
- Secure secret provisioning.
- Persistent workers.
- Restricted service permissions.

All deployment modes, present and future, must share the same core business logic.

## 11. AI Provider Independence

The platform must not depend on a single AI vendor.

Support extensible provider integrations such as:

- Anthropic Claude.
- OpenAI GPT.
- Google Gemini.
- OpenRouter.
- DeepSeek.
- Ollama.
- LM Studio.
- Other compatible providers.

Provider selection must not change authorization or security policy.

## 12. Product Success Criteria

The MVP is successful when a researcher can:

- Register an authorized target.
- Enforce scope restrictions.
- Run reconnaissance.
- Explore discovered assets.
- Execute controlled tests.
- Preserve evidence.
- Review and validate findings.
- Perform human verification.
- Generate a usable report.

The MVP workflow must run end to end on Linux localhost.
Future deployment modes must keep the same core workflow
functional when they are supported.

## 13. Interpretation Notes (Round 6)

Added by owner instruction on 2026-11-16 (`docs/decisions/0003-foundation-closure-decisions.md`).
The text of sections 1 to 12 above is unchanged. These notes record how
that text is read where documents disagreed.

- Section 4 and section 8 name a "continuous" asset and endpoint
  inventory. In the MVP this means a persistent inventory kept across
  runs with an append-only observation history. Discovery is started
  by the user. There is no scheduler and no autonomous loop (D-76).
- Section 8 lists the required MVP capabilities. The following are
  also required because other sections depend on them: local
  authentication, an approval workflow with expiry and revocation,
  audit logging, evidence integrity verification, a persistent policy
  store, and a finding transition table (D-60, D-45, D-48, D-75).
- Section 7 lists seven finding states. They are the only finding
  states. Terms such as validated or confirmed describe a validation
  record or the Verified state and are not extra states (D-75).
- Section 6 step 4, "Validate target authorization", means a
  mechanical scope validation, a stored reference to the program
  rules, and a human attestation (D-61).
- Section 10 and section 12 are read for the MVP as one deployment
  mode: a local web application on Linux (D-16, D-29).
