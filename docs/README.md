# Documentation

MORN1NGST4R is in the design phase. The application code is limited to the Phase 1 scaffold: a Go server skeleton with two health endpoints and a Next.js shell. The documents below describe what the product must do, why the main decisions were made, and the design that the implementation will follow.

## Where to start

| If you want | Read |
| --- | --- |
| The product requirements | `PRD.md` |
| The threat model | `security/threat-model.md` |
| The decisions and their order of authority | `decisions/README.md` |
| The database design | `design/database/README.md` |
| The API contract | `design/api/README.md` and `design/api/openapi.yaml` |
| The open design questions and their status | `design/dq-decisions.md` |
| How the design was cross-checked | `design/design-consistency-review.md` |
| Evidence that the sandbox and egress ideas work on one host | `security/feasibility/` |
| Version and behavior checks of the chosen technologies | `development/tech-verification/` |
| Which documents are public and why | `development/documentation-inventory.md` |
| How to run and test the current code | `development/getting-started.md` |

The steering files (`../.kiro/steering/`) are the authoritative product, security, architecture, structure and technology statements. The implementation plan is `../.kiro/specs/mvp/plan.md`.

## Layout

```text
docs/
├── README.md
├── PRD.md
├── decisions/            owner decision records, pending/ holds proposals and applied-proposal history
├── design/
│   ├── database/         schema design, ERDs, dictionary, migrations, recovery generation
│   ├── api/              OpenAPI contract and endpoint specification
│   ├── dq-decisions.md
│   └── design-consistency-review.md
├── development/
│   ├── tech-verification/
│   ├── agent-skills.md
│   ├── getting-started.md
│   └── documentation-inventory.md
└── security/
    ├── threat-model.md
    └── feasibility/
```

## Status words

A requirement or decision described as designed, specified or proposed is not implemented. "Decided" means the owner decision records say so. "Proposed" is a recommendation that is not approved. The threat model uses the status "mitigated and tested" for no row, because nothing has been built.

## Checks

`python3 scripts/validate-foundation.py` and `python3 scripts/validate-design.py` check the documents for consistency. They do not prove that the design is secure, and the OpenAPI file is not validated against the official OpenAPI 3.1 schema (see `design/api/README.md`).
