# MORN1NGST4R

An AI-assisted security assessment platform for independent bug bounty hunters and security researchers. Its philosophy is evidence-driven assessment with human-verified findings. A scanner result or an AI hypothesis never becomes a verified vulnerability by itself.

License: Apache-2.0 (see `LICENSE`).

## Status

**Foundation plus a first scaffold.** The application code is a Go server skeleton with two health endpoints, configuration loading, a Next.js shell (see `docs/development/getting-started.md`) the M1 scope package (a pure standard-library domain package that parses scope rules and evaluates names and literals) and the M2 policy and approval domain (actor, operation, approval and the pure decision function). M3 adds the persistence layer: a per-user PostgreSQL 18 cluster script, goose migrations for 33 tables, atomic consumption of approval uses, the recovery generation and the database readiness probe. M4 adds authentication: one local account, server-side sessions, CSRF, Host and Origin checks, a login rate limit, the bootstrap secret and the recovery code. Nothing else calls the policy and scope packages from a use case yet. M5 adds the audit event model and sink: one write path with bounded retries and a persistent alert. There is no worker, no address policy, no approval use case and no login screen. The repository holds the product definition, the architecture, the security standards, the threat model, the product requirements, the decision records and the MVP implementation plan. Two throwaway feasibility spikes (`scripts/spikes/`) tested a sandbox and an egress path on one reference host. A passing spike does not mean a control is implemented. No security tool is executed by this repository, and no external target is tested.

The MVP is a local-first web application on Linux localhost: a Go backend (Gin, PostgreSQL), a separate worker process, and a Next.js frontend. The first domain milestone is M1, a pure standard-library scope canonicalization package. It is implemented (decision record 0005). The next milestones follow the plan.

## Where to read

| What | Where |
| --- | --- |
| Product definition and requirements | `.kiro/steering/product.md`, `docs/PRD.md` |
| Security standards | `.kiro/steering/security.md` |
| Architecture and repository structure | `.kiro/steering/architecture.md`, `.kiro/steering/structure.md` |
| Technology decisions and versions | `.kiro/steering/tech.md` |
| Decision records and the source-of-truth hierarchy | `docs/decisions/` |
| Threat model | `docs/security/threat-model.md` |
| Feasibility evidence | `docs/security/feasibility/` |
| Implementation plan | `.kiro/specs/mvp/plan.md` |
| Running and testing the code | `docs/development/getting-started.md` |

## Checking the foundation

```
python3 scripts/validate-foundation.py
```

It checks the decision register, document references, milestone dependencies, the ignore rules and a scan for secrets and local paths. It needs Python 3 and Git.

## Agent skills

Contributors may use third-party agent skills as optional local tooling. They are installed locally, are git-ignored, and are not part of MORN1NGST4R. Building, running and testing the product does not need them. See `docs/development/agent-skills.md`.

## License and third-party material

The project is licensed under the Apache License 2.0 (`LICENSE`). That license covers the material this project authored. It does not relicense third-party material. Third-party agent skills are local-only development tooling. They are not committed, not redistributed and not part of the product.

## Safety

Use this project only against systems you own or are explicitly authorized to test. Never commit credentials, evidence, captured traffic or target data. `.gitignore` is set up to keep local data out of Git, and a pushed secret stays in Git history. Rotate any credential that is ever exposed.
