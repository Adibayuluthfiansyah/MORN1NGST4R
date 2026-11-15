# Pending decisions

Proposals awaiting owner approval, and records of items that were applied. Nothing marked as a proposal is in force. Approved items move into the steering files or into `docs/decisions/` as ADRs, and this folder keeps a short record.

| File | Item | What it holds | Status |
| --- | --- | --- | --- |
| `B1-security.md.diff` | B1 | The additive patch applied to `.kiro/steering/security.md` in Round 3 (210 non-blank added lines, none removed). | **Applied in Round 3.** Record only. `security.md` was edited again in Round 5 at the owner's instruction (see `0002`). |
| `B1-security-review.md` | B1 | Requirement-by-requirement threat check, independent review results, Round 2 and 3 outcomes. | Record |
| `S1-security-semantics.md` | O-24, O-07, O-13, P-24, P-25, P-26, P-30 | The detailed semantics behind `security.md`, revised for Rounds 2, 3 and 5. Spike results are noted next to the statements they test. | **Approved in Round 6 (D-64).** Kept as the detailed semantics. |
| `C-product-decisions.md` | C1, C2 | Ten product decisions. Items 1, 2, 5 and 9 are decided (D-45, D-59 to D-61). Items 3, 4, 6, 7, 8 and 10 remain proposals. | Decided in Rounds 5 and 6 (D-45, D-59 to D-61, D-75, D-76, D-78, D-80, D-81, D-83). Kept as history. |
| `F-password-recovery-proposal.md` | O-37 | Password recovery options for the single local account. No reset path exists until approved. | **Option B adopted in Round 6 (D-68).** |
| `E1-architecture-split-mapping.md` | E1 | Mapping of `architecture.md` into `docs/architecture/` and `docs/decisions/` with a lossless dry run and an ID registry. Regenerated in Round 5 against revision 7 (39 pieces, 165 IDs). | **Stale against architecture revision 8.** Regenerate before use. Nothing has been moved. |
| `E1-architecture-steering.draft.md` | E1 | Draft of the concise steering file. Updated in Round 5. Not active. | **Stale draft.** Not active. |

Decided in audit Round 1 and applied: A1 to A4 (`architecture.md` section 19, D-23 to D-29).
Decided in Rounds 2 and 3: D-30 to D-46, recorded in `docs/decisions/0001-owner-security-decisions-round-2.md`.
Decided in Round 5: D-47 to D-62, recorded in `docs/decisions/0002-foundation-completion-decisions.md`.
Decided in Round 6: D-63 to D-85, recorded in `docs/decisions/0003-foundation-closure-decisions.md`. Eight open items stay open as gates (O-07, O-13, O-22, O-32, O-33, O-35, O-36, O-38).

Related, outside this folder: `.kiro/steering/tech.md` (technology decisions and versions), `docs/security/threat-model.md`, `docs/security/feasibility/` (spike results), `docs/development/tech-verification/` (primary-source verification records), `.kiro/specs/mvp/plan.md` (implementation plan).
