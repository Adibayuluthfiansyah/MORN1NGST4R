# F: Password recovery proposal for the single local account (O-37)

**Round 6 (2026-11-16): option B is ADOPTED (D-68).** The one-time recovery code is shown once, stored only as a hash, used in the web UI on the loopback listener, and a lost code means the documented offline re-initialization procedure that must be written and reviewed before the first release. No command-line reset exists. Record: `docs/decisions/0003-foundation-closure-decisions.md`.

Status: PROPOSAL. Nothing here is decided. The owner decided in Round 5 (D-60) that the MVP has one local researcher account with a memory-hard password hash, and asked for recovery to be proposed separately. **Until this proposal is approved, no reset path exists.** A forgotten password then means the application cannot be unlocked. That is the safe default and the owner accepted it by deferring recovery.

## Why recovery is a security feature

A reset path is an authentication bypass by design. It decides who can regain control of an installation that holds target data, approval authority and stored credentials. It must not be weaker than the login it replaces, and it must not let another local process take over the account silently.

## Facts that shape the options

- The application is a localhost web application used by one person on one machine (D-16, D-60). There are no accounts to administer and no email service.
- Any process running as the same operating-system user can already reach the same files, the PostgreSQL unix socket (peer authentication identifies the user, not the process) and the proxy socket (threat model T-61, Unresolved). A recovery design cannot defend against full compromise of that user.
- Stored provider and tool credentials live in the Secret Service, not in the password (D-52). A password reset does not expose them. It does give the new session the power to use them.
- Intrusive approval needs password re-entry (D-60). A reset therefore changes who can approve intrusive operations.
- A native command-line entry point is not an MVP deliverable (D-18). A recovery command would need its own approval.

## Options

| Option | Mechanism | Strength | Weakness |
| --- | --- | --- | --- |
| A. No recovery | A lost password cannot be recovered. The user restores from a backup or re-initializes. | No bypass exists. | Data loss risk for evidence tied to the account. High friction. |
| B. One-time recovery code | At first run the application shows a high-entropy code once. Only a hash is stored. A reset needs the code and a new password. The code is single use and a new one is shown after a reset. Attempts are rate limited and audited. | Does not rely on OS access alone. The secret is held offline by the user. | The user must store the code. A lost code falls back to option A. |
| C. Local reset command | A server subcommand run by the same OS user resets the password after a confirmation. | Simple. | Any process of that user can run it. It is a bypass for same-user malware. Needs a CLI entry (D-18). |
| D. Option B delivered through option C | The reset command exists, but it accepts only a valid recovery code. | Keeps B's secret and gives a defined procedure. | Needs the CLI approval. More surface. |

## Recommendation (proposed, not decided)

Option B, with the reset performed in the web UI on the loopback listener so that no command-line entry point is needed. Required properties:

1. The code is generated with a cryptographically secure source, at least 128 bits, shown once, and stored only as a hash made with the same function as passwords (P-48).
2. A reset requires the code and a new password that meets the password rules. The code becomes invalid after use and a new code is shown.
3. A successful reset revokes every session, is audited with the old and new state, and keeps intrusive operations disabled until the owner has re-entered the password once.
4. Failed attempts are rate limited and audited, with no information about whether a code exists.
5. The reset page is served only on the loopback listener with the Host, Origin and CSRF rules of `security.md` 8.
6. A lost code means option A: export evidence through a documented offline procedure and re-initialize. The procedure must be reviewed before the first release, because it is destructive.

## Consequences if approved

- New work in the `identity` and `auth` packages (D-23), a new audit event type, a first-run screen, and a documented offline procedure for the lost-code case.
- A threat to add and test: recovery-code theft, brute force and replay.
- It does not remove the same-user weakness (T-61). It only avoids adding a second bypass.

## Owner questions

1. Option B, A, C or D?
2. Is the offline lost-code procedure acceptable, or must a lost password never destroy data?
3. If a command-line entry point is acceptable, should it exist for recovery only?

## Dependencies

D-60, D-52, D-58, D-18, P-48, O-16, O-37, `security.md` 8, threat model T-61.
