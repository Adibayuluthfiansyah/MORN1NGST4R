# Addendum: checks made by the coordinator

The four verification records were written by separate research agents. Before `tech.md` was populated, the coordinator fetched these primary sources directly on 2026-11-15, to fill gaps that the records list as NOT VERIFIED and to spot-check three headline versions. Nothing here replaces a record. It adds provenance.

| Fact | Source fetched on 2026-11-15 | What was read |
| --- | --- | --- |
| Go toolchain licence | https://go.dev/LICENSE | Three-clause BSD text (copyright The Go Authors, no endorsement clause). Recorded as BSD-3-Clause. |
| Go latest stable | https://go.dev/dl/?mode=json | The first stable entry is `go1.27.4`. This agrees with `backend-and-database.md`. |
| PostgreSQL licence | https://www.postgresql.org/about/licence/ | "released under the PostgreSQL License, a liberal Open Source license, similar to the BSD or MIT licenses". |
| PostgreSQL supported versions | https://www.postgresql.org/support/versioning/ | Version 18, current minor 18.7, first release September 25, 2025, final release November 14, 2030. Version 14 final release November 12, 2026. This agrees with `backend-and-database.md`. |
| Node.js licence | https://raw.githubusercontent.com/nodejs/node/main/LICENSE | Node.js core is under an MIT licence text. The file also lists externally maintained libraries under their own licences. |
| Next.js latest and `engines` | https://registry.npmjs.org/next/latest | Version 16.4.6, `engines` node `>=20.9.0`, licence MIT. This agrees with `frontend.md`. |

Not checked by the coordinator: every other version and licence in `tech.md`. Those rest on the four records and carry the limits stated in them.
