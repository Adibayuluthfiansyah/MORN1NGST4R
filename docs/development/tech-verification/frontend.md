# Frontend technology verification record

## 1. Header

- Fetch date (UTC, from `date -u +%F`): **2026-11-15**. Every source below was fetched on this date.
- Host environment (read-only, not a baseline): Linux 7.2.11-zen1-1-zen, Node v26.13.0, npm 11.20.0, 7.5 GiB RAM (4.9 GiB used at the time of the check).
- Method: `web_fetch` only, against primary sources: the npm registry (`/<pkg>/latest`, `/-/package/<pkg>/dist-tags`, packuments for publish times), `api.github.com/repos/<owner>/<repo>/releases`, `raw.githubusercontent.com` LICENSE files, nodejs.org, nextjs.org, react.dev, tailwindcss.com, ui.shadcn.com, tanstack.com, vitest.dev, playwright.dev, openapi-ts.dev, heyapi.dev, orval.dev, pnpm.io, MDN, rfc-editor.org and datatracker.ietf.org. No secondary aggregator was used. Nothing was installed, built or run. No file other than this one was created.
- "Release date" is the GitHub `published_at` or the npm packument `time` entry for that exact version. Where neither was fetched the cell says NOT VERIFIED.
- "Latest" means the npm `latest` dist-tag. "Supported/LTS" is stated separately and only where a primary source defines it.
- Registry "latest" values are a snapshot. Re-run the registry fetches before pinning in `package.json`.

### Not verified (summary; full list in section 6)

Corepack/pnpm memory use, Playwright browser download sizes, Next.js behaviour of `rewrites` under streaming/large bodies and its forwarded `Host`, Next.js tested-on-Node-26 statement, Zod's documented minimum TypeScript, release dates of several secondary packages (pnpm, vite, jsdom, ESLint 9.39.5, typescript-eslint, TypeScript 6.0.3), Hey API LICENSE file (404; license taken from npm), Orval OpenAPI 3.1 statement, nonce-CSP behaviour of Radix/shadcn inline style attributes, whether `127.0.0.1` is treated like `localhost` for `Secure` cookies.

## 2. Technology tables

### 2.1 Node.js runtime

| Field | Value |
| --- | --- |
| Role | Runtime for Next.js server, build and all tooling |
| Latest stable | v26.13.1 (2026-11-10, LTS "Lithium", security release). Newest v24: v24.22.3 (2026-11-10, "Krypton", security release). v27.0.0-alpha.0.1.0 (2026-11-12) is an alpha. |
| Support window | `schedule.json`: v26 LTS from 2026-10-28, maintenance from 2027-10-20, end 2029-04-30. v24 LTS from 2025-10-28, maintenance from 2026-10-20, end 2028-04-30. v22 maintenance from 2025-10-21, end 2027-04-30. v20 end 2026-04-30 (EOL). v25 EOL. Derived on 2026-11-15: **Active LTS = v26; Maintenance LTS = v24 and v22.** The releases page shows only "LTS"/"EOL"; the Active/Maintenance split is derived from the `schedule.json` dates. Page text: "Production applications should only use Active LTS or Maintenance LTS releases." From Node 27 the cycle is annual. |
| License | Not fetched (NOT VERIFIED) |
| engines/peer | n/a |
| Sources (fetched 2026-11-15) | https://nodejs.org/dist/index.json ; https://nodejs.org/en/about/previous-releases ; https://raw.githubusercontent.com/nodejs/Release/main/schedule.json |
| Trade-offs | Next.js minimum is 20.9, but Node 20 is EOL. Several tools in the set need newer Node (see 4). |
| Verdict | PROPOSE Node 24 (>= 24.15.0, currently 24.22.3). Reason: it satisfies every `engines` range verified below (jsdom needs ^24.15.0), and it is the line under which the `next` and `tailwindcss` packages were published (registry `_nodeVersion` 24.22.2 for both). Node 26 satisfies the same ranges but no Next.js statement that 26 is tested was found (NOT VERIFIED). Node 26 gives the longest window (to 2029-04-30). **Owner decision: 24 vs 26.** |

### 2.2 Package manager

| Field | Value |
| --- | --- |
| Role | Dependency installation |
| Latest stable | pnpm 12.21.0 (date NOT VERIFIED). npm 11.19.1 ships with Node 24.22.x; 11.20.0 with Node 26.13.x (from `index.json`). |
| Support window | Not stated by a source. |
| License | pnpm: MIT (registry). npm: not fetched. |
| engines/peer | pnpm npm-package `engines.node >=18`. pnpm 12 is a native executable and needs no Node once installed. Compatibility table: pnpm 12 works with Node 18, 20, 22, 24, 26. |
| Docs position | Next.js installation page lists pnpm, npm, yarn and bun in parallel tabs and names none as recommended. |
| Corepack | nodejs/corepack README: distributed with Node 14.19.0 up to but not including 25.0.0. So Node 24 has it (`corepack enable`), Node 26 does not (install with `npm install -g corepack` if wanted). |
| Disk/memory | pnpm motivation page: one content-addressable store, files hard-linked or copy-on-write cloned, so no per-project copies. npm: one copy per project (same page). No primary memory figure for any package manager (NOT VERIFIED). |
| Sources (fetched 2026-11-15) | https://registry.npmjs.org/pnpm/latest ; https://pnpm.io/installation ; https://pnpm.io/motivation ; https://raw.githubusercontent.com/nodejs/corepack/HEAD/README.md ; https://nextjs.org/docs/app/getting-started/installation |
| Verdict | PROPOSE pnpm 12.21.0, pinned through the `packageManager` field. Fallback: npm bundled with Node (zero extra install). Owner decision. |

### 2.3 Next.js (App Router)

| Field | Value |
| --- | --- |
| Role | Web framework, serves the operator UI on 127.0.0.1 |
| Latest stable | **16.4.6**, 2026-11-09 (GitHub release v16.4.6; registry `latest`). Not for use: `canary` 16.5.0-canary.51, `preview` 16.3.0-preview.10, `beta` 16.0.0-beta.0, `rc` 15.0.0-rc.1. Backport line `backport` 15.5.29. |
| Support window | Support policy page: 16.x is **Active LTS** (released Oct. 21, 2025). 15.x is **Maintenance LTS**, extended past its original end of Oct. 21, 2026, with security releases through Jan. 31, 2027. Each major stays in Maintenance LTS for two years after the next major. "Serve production applications with either the latest Active or Maintenance LTS release." |
| License | MIT (`license.md`, "Copyright (c) 2025 Vercel, Inc."; registry `MIT`). |
| engines/peer | `engines.node >=20.9.0`. peer `react ^18.2.0 \|\| 19.0.0-rc-de68d2f4-20241204 \|\| ^19.0.0`, same for `react-dom`. Optional peers: `sass ^1.3.0`, `@playwright/test ^1.51.1`, `@opentelemetry/api ^1.1.0`, `babel-plugin-react-compiler *`. Optional dep `sharp ^0.35.5`. Docs: minimum TypeScript 5.1.0; supported browsers Chrome 111+, Edge 111+, Firefox 111+, Safari 16.4+. |
| Sources (fetched 2026-11-15) | https://registry.npmjs.org/next/latest ; https://registry.npmjs.org/-/package/next/dist-tags ; https://api.github.com/repos/vercel/next.js/releases/latest ; https://nextjs.org/support-policy ; https://nextjs.org/docs/app/getting-started/installation ; https://raw.githubusercontent.com/vercel/next.js/HEAD/license.md |
| Trade-offs | `next dev` and `next start` default to hostname 0.0.0.0 (see 3.2). Nonce CSP forces dynamic rendering (see 3.7). App Router uses a bundled React canary regardless of the `react` version in `package.json` (installation page, "Good to know"). Turbopack is the default bundler. `next build` no longer runs the linter. `middleware` is now `proxy`. |
| Verdict | KEEP. PIN `next@16.4.6`. |

### 2.4 React and React DOM

| Field | Value |
| --- | --- |
| Role | UI library |
| Latest stable | `react` **19.3.1**, 2026-10-14 (GitHub release v19.3.1; registry `latest`). `react-dom` was not fetched separately (NOT VERIFIED; same release line assumed, check before pinning). Not for use: `canary` 19.3.0-canary-df6aa4ec-20261112, `experimental`, `next`, `rc`, `beta`. |
| Support window | react.dev versioning policy: no LTS scheme. Stable "Latest" follows semver. "If we learn of a security vulnerability in React, we release a backported fix for all major versions that are affected." Backport tags exist (`backport` 19.1.10). |
| License | MIT (LICENSE, "Meta Platforms, Inc. and affiliates"; registry `MIT`). |
| engines/peer | `engines.node >=0.10.0` (no practical constraint). |
| Sources (fetched 2026-11-15) | https://registry.npmjs.org/react/latest ; https://registry.npmjs.org/-/package/react/dist-tags ; https://api.github.com/repos/facebook/react/releases/latest (redirects to react/react) ; https://react.dev/versions ; https://react.dev/community/versioning-policy ; https://raw.githubusercontent.com/facebook/react/HEAD/LICENSE |
| Verdict | KEEP. PIN `react@19.3.1` and `react-dom@19.3.1` (confirm react-dom number at install). |

### 2.5 TypeScript

| Field | Value |
| --- | --- |
| Role | Type checking |
| Latest stable | **7.0.2**, 2026-08-20 (npm `latest`; GitHub release v7.0.2). `rc` 7.0.1-rc, `beta` 7.1.0-beta, `next` 7.1.0-dev.20261115.1 are pre-releases. Prior line used in the set: **5.9.3**, 2025-10-01 (GitHub release). 6.0.3 appears as an exact devDependency in TanStack Query, Orval and Hey API; its date is NOT VERIFIED. |
| Support window | No support policy fetched. NOT VERIFIED. |
| License | Apache-2.0 (LICENSE.txt; registry `Apache-2.0`). |
| engines/peer | 7.0.2: `engines.node >=16.20.0`; ships native binaries as optional dependencies. |
| Constraints from other packages | `typescript-eslint@8.72.2` peer `typescript >=4.8.4 <6.1.0` (excludes 7). `openapi-typescript@7.13.0` peer `typescript ^5.x` (excludes 6 and 7). `@hey-api/openapi-ts@0.99.0` peer `>=5.5.3 \|\| >=6.0.0`. `eslint-config-next@16.4.6` peer `typescript >=3.3.1` (optional). Next.js docs: min 5.1.0; "TypeScript 7 does not currently provide the JavaScript compiler API"; Next.js then uses the project-local `tsc` CLI and does not apply its own error rewriting. `@types/react` tags: `ts5.9`, `ts6.0`, `ts7.0` all map to 19.3.0. |
| Sources (fetched 2026-11-15) | https://registry.npmjs.org/typescript/latest ; https://registry.npmjs.org/-/package/typescript/dist-tags ; https://api.github.com/repos/microsoft/TypeScript/releases/latest ; https://api.github.com/repos/microsoft/TypeScript/releases/tags/v5.9.3 ; https://nextjs.org/docs/app/api-reference/config/typescript ; https://registry.npmjs.org/typescript-eslint/latest ; https://registry.npmjs.org/@types/react/dist-tags |
| Trade-offs | The newest release (7.0.2) breaks two verified peer ranges (typescript-eslint, openapi-typescript). 6.x breaks one (openapi-typescript). 5.9.x satisfies all verified peers. |
| Verdict | PROPOSE TypeScript **5.9.3** while openapi-typescript is the generator. Whether a later 5.9.x exists is NOT VERIFIED. If the owner picks Orval or Hey API instead, the highest version that still satisfies typescript-eslint is 6.0.x (6.0.3 verified to exist as a dependency pin; date not verified). Not 7.x until typescript-eslint widens its peer range. |

### 2.6 TanStack Query

| Field | Value |
| --- | --- |
| Role | Server-state cache and fetching in client components |
| Latest stable | `@tanstack/react-query` **5.107.1**, 2026-11-06 (GitHub release `release-2026-11-06-2304`). Pre-release tags `alpha`/`beta`/`rc` are old v5 pre-releases; `previous` 4.44.0. |
| Support window | Not stated. NOT VERIFIED. |
| License | MIT (LICENSE "2021-present Tanner Linsley"; registry `MIT`). |
| engines/peer | peer `react ^18 \|\| ^19`; optional peer `@types/react ^18 \|\| ^19`. Dependency `@tanstack/query-core 5.107.1`. Docs: "compatible with React v18+"; browsers Chrome >= 91, Firefox >= 90, Edge >= 91, Safari >= 15. Its own CI type-tests TypeScript 5.7, 5.8, 5.9, 7.0 and 7.1 (package scripts). |
| Sources (fetched 2026-11-15) | https://registry.npmjs.org/@tanstack/react-query/latest ; https://registry.npmjs.org/-/package/@tanstack/react-query/dist-tags ; https://api.github.com/repos/TanStack/query/releases/latest ; https://tanstack.com/query/latest/docs/framework/react/installation ; https://raw.githubusercontent.com/TanStack/query/HEAD/LICENSE |
| Verdict | KEEP. PIN `@tanstack/react-query@5.107.1`. Optional `@tanstack/eslint-plugin-query` (recommended by the docs; version NOT VERIFIED). |

### 2.7 Tailwind CSS

| Field | Value |
| --- | --- |
| Role | Utility CSS |
| Latest stable | `tailwindcss` **4.3.5** and `@tailwindcss/postcss` **4.3.5**, 2026-11-06 (GitHub release v4.3.5). Tag `v3-lts` 3.4.19. |
| Support window | Not stated. NOT VERIFIED. |
| License | MIT (LICENSE "Tailwind Labs, Inc."; registry `MIT`). |
| engines/peer | No `engines` field on either package. `@tailwindcss/postcss` deps: `postcss ^8.5.25`, `@tailwindcss/node 4.3.5`, `@tailwindcss/oxide 4.3.5`. `@tailwindcss/node@4.3.5` depends on `magic-string ^1.1.0`. Tailwind docs: v4 is designed for Chrome 111, Safari 16.4, Firefox 128. Next.js lists Firefox 111, so the effective floor is **Firefox 128**. |
| Known issue | Next.js PR #101061 (merged 2026-11-03): `@tailwindcss/node@4.3.4` (2026-11-02) moved to ESM-only `magic-string` 1.x while Tailwind's CommonJS build still `require()`s it, so builds fail on Node before 20.19 and 22.0 to 22.11. Next.js `create-next-app` pins Tailwind 4.3.3 "temporarily" (also in the 16.4.6 release notes). 4.3.5 still depends on `magic-string ^1.1.0`. Not an issue on Node 24. |
| Sources (fetched 2026-11-15) | https://registry.npmjs.org/tailwindcss/latest ; https://registry.npmjs.org/@tailwindcss/postcss/latest ; https://registry.npmjs.org/@tailwindcss/node/latest ; https://api.github.com/repos/tailwindlabs/tailwindcss/releases/latest ; https://api.github.com/repos/vercel/next.js/pulls/101061 ; https://tailwindcss.com/docs/installation/framework-guides/nextjs ; https://tailwindcss.com/docs/compatibility |
| Verdict | KEEP. PIN `tailwindcss@4.3.5` + `@tailwindcss/postcss@4.3.5` on Node >= 22.12 (Node 24 proposed). Do not use Node 20.9 to 20.18 with this version. |

### 2.8 shadcn/ui

| Field | Value |
| --- | --- |
| Role | Copy-in component source (not a runtime dependency) plus a CLI |
| Latest stable | CLI package `shadcn` **4.23.0**, 2026-10-27 (GitHub release `shadcn@4.23.0`). |
| Support window | Not stated. NOT VERIFIED. Docs say the code is copied into the project ("exactly what you'd write yourself"), so upgrades follow the underlying libraries. |
| License | MIT (LICENSE.md "2023 shadcn"; registry `MIT`). |
| engines/peer | CLI `engines.node >=20.18.1`. CLI dependencies include `zod ^4.2.0`, `postcss ^8.5.23`, `ts-morph ^26.0.0`, `undici ^7.30.1`, `@modelcontextprotocol/server ^2.3.1`, `@dotenvx/dotenvx`. `shadcn init --base` accepts `base`, `radix` or `aria`. Docs: new projects start on Tailwind v4 and React 19; `toast` is deprecated in favour of `sonner`; `tailwindcss-animate` replaced by `tw-animate-css`. |
| Sources (fetched 2026-11-15) | https://registry.npmjs.org/shadcn/latest ; https://api.github.com/repos/shadcn-ui/ui/releases/latest ; https://ui.shadcn.com/docs/installation/next ; https://ui.shadcn.com/docs/tailwind-v4 ; https://ui.shadcn.com/docs/cli ; https://raw.githubusercontent.com/shadcn-ui/ui/HEAD/LICENSE.md |
| Trade-offs | Copied components become project code and need review and tests (they are not covered by a dependency audit). Running `shadcn@latest` via `dlx` downloads and executes a package; pin the CLI version. |
| Verdict | KEEP. PIN CLI `shadcn@4.23.0` (dev-time only; run with the Radix base). |

### 2.9 Radix primitives (`radix-ui`)

| Field | Value |
| --- | --- |
| Role | Accessible headless primitives used by shadcn components (Radix base) |
| Latest stable | `radix-ui` unified package **1.8.1**, 2026-10-12 (npm `time`). `@radix-ui/react-dialog` 1.3.1 as a sample member. Tag `next` is an rc. |
| Support window | Not stated. NOT VERIFIED. Repo owner string in the LICENSE is WorkOS. |
| License | MIT (LICENSE "Copyright (c) 2022 WorkOS"; registry `MIT` for `radix-ui` and `react-dialog`). |
| engines/peer | No `engines`. peer `react` and `react-dom ^16.8 \|\| ^17.0 \|\| ^18.0 \|\| ^19.0 \|\| ^19.0.0-rc`; optional `@types/react`, `@types/react-dom`. |
| Sources (fetched 2026-11-15) | https://registry.npmjs.org/radix-ui/latest ; https://registry.npmjs.org/radix-ui (packument, `time`) ; https://registry.npmjs.org/@radix-ui/react-dialog/latest ; https://raw.githubusercontent.com/radix-ui/primitives/HEAD/LICENSE |
| Verdict | KEEP. PIN `radix-ui@1.8.1`. License review of transitive Radix packages (`react-remove-scroll`, `aria-hidden`, etc.) is NOT VERIFIED. |

### 2.10 Zustand

| Field | Value |
| --- | --- |
| Role | Client UI state, only where needed |
| Latest stable | **5.1.0**, 2026-11-06 (GitHub release v5.1.0). |
| Support window | Not stated. NOT VERIFIED. |
| License | MIT (LICENSE "2019 Paul Henschel"; registry `MIT`). |
| engines/peer | `engines.node >=12.20.0`. peer `react >=18.0.0` (optional), `@types/react >=18` (optional), `immer >=9.0.6` (optional), `use-sync-external-store >=1.2.0` (optional). Types need TypeScript >= 4.5 (`typesVersions`). |
| Sources (fetched 2026-11-15) | https://registry.npmjs.org/zustand/latest ; https://registry.npmjs.org/-/package/zustand/dist-tags ; https://api.github.com/repos/pmndrs/zustand/releases/latest ; https://raw.githubusercontent.com/pmndrs/zustand/HEAD/LICENSE |
| Trade-offs | The Zustand Next.js guide URL returned 404, so its per-request-store guidance is NOT VERIFIED. Keep Zustand out of server components. |
| Verdict | KEEP. PIN `zustand@5.1.0`. |

### 2.11 React Hook Form and `@hookform/resolvers`

| Field | Value |
| --- | --- |
| Role | Form state and schema resolver |
| Latest stable | `react-hook-form` **7.92.0**, 2026-11-11 (GitHub release v7.92.0). `@hookform/resolvers` **7.0.6**, 2026-11-11 (GitHub release v7.0.6). RHF tags `next`, `alpha` (8.0.0-alpha.5), `beta` (8.0.0-beta.0) are pre-releases. |
| Support window | Not stated. NOT VERIFIED. |
| License | MIT for both (LICENSE files; registry `MIT`). |
| engines/peer | RHF: `engines.node >=18.0.0`; peer `react ^16.8.0 \|\| ^17 \|\| ^18 \|\| ^19`; `@types/react *` optional. Resolvers: peer `react-hook-form ^7.55.0`; `zod ^3.25.0 \|\| ^4.0.0` (optional); `@standard-schema/spec ^1.0.0` (optional); dependency `@standard-schema/utils ^0.3.0`. So **Zod 4 is supported by resolvers 7.0.6.** |
| Sources (fetched 2026-11-15) | https://registry.npmjs.org/react-hook-form/latest ; https://registry.npmjs.org/-/package/react-hook-form/dist-tags ; https://registry.npmjs.org/@hookform/resolvers/latest ; https://api.github.com/repos/react-hook-form/react-hook-form/releases/latest ; https://api.github.com/repos/react-hook-form/resolvers/releases/latest ; https://react-hook-form.com/docs/useform |
| Verdict | KEEP. PIN `react-hook-form@7.92.0` and `@hookform/resolvers@7.0.6`. |

### 2.12 Zod

| Field | Value |
| --- | --- |
| Role | Schema validation |
| Latest stable | **4.6.7**, 2026-10-12 (GitHub release v4.6.7). Tags `canary` 4.5.0-canary.20260828T171753, `beta`, `alpha`, `next` are not for use. Package also exports `zod/v3` for the old API. |
| Support window | Not stated. NOT VERIFIED. |
| License | MIT (LICENSE "2025 Colin McDonnell"; registry `MIT`). |
| engines/peer | No `engines`, no peers. Minimum TypeScript version not verified (zod.dev fetch had no match). |
| Sources (fetched 2026-11-15) | https://registry.npmjs.org/zod/latest ; https://registry.npmjs.org/-/package/zod/dist-tags ; https://api.github.com/repos/colinhacks/zod/releases/latest ; https://raw.githubusercontent.com/colinhacks/zod/HEAD/LICENSE |
| Verdict | KEEP. PIN `zod@4.6.7` (works with `@hookform/resolvers@7.0.6`). |

### 2.13 OpenAPI generators (O-04), three candidates

| Field | openapi-typescript + openapi-fetch | Orval | @hey-api/openapi-ts |
| --- | --- | --- | --- |
| Latest stable | 7.13.0 and 0.17.0, both 2026-02-11 (npm `time`). GitHub repo "latest release" is `openapi-react-query@0.5.4`, also 2026-02-11, so no release in the repo since then (9 months at fetch). Packument `modified` 2026-06-15. | 8.48.0, 2026-11-14 (GitHub). `rc` tag 8.0.0-rc.6 is old. | 0.99.0, 2026-06-22 (GitHub release note "@hey-api/openapi-ts 0.99.0"). |
| License | MIT (LICENSE "2020 Drew Powers") | MIT (LICENSE "2022-present Victor Bury") | MIT per registry. LICENSE file fetch returned 404 (not verified from the repo). |
| OpenAPI versions | 3.0 and 3.1 (openapi-ts.dev; npm description). 2.x only on 5.x. | Docs example uses 3.0.0. 3.1 support not confirmed from a fetched page (NOT VERIFIED). | "accepts any OpenAPI specification" (docs). Exact versions not stated (NOT VERIFIED). |
| engines/peer | openapi-typescript: peer `typescript ^5.x`; deps include `@redocly/openapi-core ^1.34.6`, `parse-json`, `yargs-parser`, `change-case`. openapi-fetch: one dep, `openapi-typescript-helpers ^0.1.0`; no engines. Docs: Node 20.x or higher recommended. | `engines.node >=22.18.0`; optional peers `prettier >=3`, `typedoc`. Many `@orval/*` deps (zod, valibot, hono, mock, etc.). | `engines.node >=22.18.0`; peer `typescript >=5.5.3 \|\| >=6.0.0 \|\| 6.0.1-rc`. |
| Output weight | openapi-typescript emits a `.d.ts` only ("runtime-free types"). openapi-fetch is "only 6 kb (min)". | Emits clients, hooks (React Query), mocks (MSW), validators. Generated code imports `@tanstack/react-query`. | Emits SDK, types, schemas; 20+ plugins; client packages for Fetch, Next.js and others. |
| Maintenance | Slow: no release since 2026-02-11. | Very active (release 2026-11-14). | Active, but docs say "initial development ... pin an exact version" and 0.99.0 lists four breaking changes. |
| Sources (fetched 2026-11-15) | https://registry.npmjs.org/openapi-typescript/latest ; https://registry.npmjs.org/openapi-fetch/latest ; https://registry.npmjs.org/openapi-typescript (packument) ; https://api.github.com/repos/openapi-ts/openapi-typescript/releases/latest ; https://openapi-ts.dev/introduction ; https://raw.githubusercontent.com/openapi-ts/openapi-typescript/HEAD/LICENSE | https://registry.npmjs.org/orval/latest ; https://registry.npmjs.org/-/package/orval/dist-tags ; https://api.github.com/repos/orval-labs/orval/releases/latest ; https://orval.dev ; https://raw.githubusercontent.com/orval-labs/orval/HEAD/LICENSE | https://registry.npmjs.org/@hey-api/openapi-ts/latest ; https://api.github.com/repos/hey-api/openapi-ts/releases/latest ; https://heyapi.dev/docs/openapi/typescript/get-started |
| Verdict | **PROPOSE as default**: openapi-typescript 7.13.0 + openapi-fetch 0.17.0, with TypeScript 5.9.3. Smallest runtime surface, types-only generated artifact. Risk: slow maintenance and the `^5.x` TypeScript peer. | **Fallback** if the peer range or the stale release cadence blocks us: Orval 8.48.0 (works with TypeScript 6.0.x). | Not proposed: 0.x with repeated breaking changes. |

### 2.14 Test tooling (O-18)

| Package | Latest stable and date | License | engines/peer | Sources (fetched 2026-11-15) |
| --- | --- | --- | --- | --- |
| `vitest` | **5.1.0**, 2026-11-02 (GitHub) | MIT (LICENSE "VoidZero Inc. and Vitest contributors") | `engines.node ^22.12.0 \|\| ^24.0.0 \|\| >=26.0.0`; peer `vite ^6.4.0 \|\| ^7.0.0 \|\| ^8.0.0` (**required**), optional `jsdom *`, `happy-dom`, `@vitest/coverage-v8 5.1.0`, `@vitest/ui`, `@types/node ^22 \|\| >=24` | https://registry.npmjs.org/vitest/latest ; https://api.github.com/repos/vitest-dev/vitest/releases/latest ; https://vitest.dev/guide/ ; https://raw.githubusercontent.com/vitest-dev/vitest/HEAD/LICENSE |
| `vite` | 8.3.8 (date NOT VERIFIED) | MIT (registry) | `engines.node ^20.19.0 \|\| >=22.12.0` | https://registry.npmjs.org/vite/latest |
| `jsdom` | 31.0.1 (date NOT VERIFIED) | MIT (registry) | `engines.node ^22.22.2 \|\| ^24.15.0 \|\| >=26.0.0` | https://registry.npmjs.org/jsdom/latest |
| `@testing-library/react` | **16.3.3**, 2026-08-27 (npm `time`) | MIT (LICENSE "2017-Present Kent C. Dodds") | `engines.node >=18`; peer `react ^18 \|\| ^19`, `react-dom ^18 \|\| ^19`, `@testing-library/dom ^10.0.0` (**required**), optional `@types/react(-dom)` | https://registry.npmjs.org/@testing-library/react/latest ; https://registry.npmjs.org/@testing-library/react (packument) ; https://raw.githubusercontent.com/testing-library/react-testing-library/HEAD/LICENSE |
| `@playwright/test` | **1.65.1**, 2026-11-13 (npm `time`). Tags `beta`, `next` (1.66.0-alpha) not for use. | Apache-2.0 (LICENSE; registry) | `engines.node >=20`. Docs: "latest 22.x, 24.x or 26.x"; Debian 12/13, Ubuntu 22.04/24.04/26.04. Next.js lists it as an optional peer `^1.51.1`. | https://registry.npmjs.org/@playwright/test/latest ; https://registry.npmjs.org/@playwright/test (packument) ; https://playwright.dev/docs/intro ; https://playwright.dev/docs/browsers ; https://raw.githubusercontent.com/microsoft/playwright/HEAD/LICENSE |

Notes:
- Next.js Vitest guide (https://nextjs.org/docs/app/guides/testing/vitest, fetched 2026-11-15): manual setup installs `vitest @vitejs/plugin-react jsdom @testing-library/react @testing-library/dom vite-tsconfig-paths`. Vitest does not support async Server Components; use E2E tests for those. `@vitejs/plugin-react` and `vite-tsconfig-paths` versions were not fetched (NOT VERIFIED).
- Playwright browsers are downloaded by `npx playwright install` into a cache; the docs show `--only-shell` (skip full Chromium, headless shell only) and installing a single named browser. No primary download size was found (NOT VERIFIED). Default test config runs Chromium, Firefox and WebKit.
- Per `structure.md`, black-box E2E tests live in the root `tests/e2e/`; component tests live beside the code.

Verdict: KEEP/PROPOSE `vitest@5.1.0`, `vite@8.3.8`, `jsdom@31.0.1`, `@testing-library/react@16.3.3` (+ `@testing-library/dom ^10`). Playwright `1.65.1` deferred until the first E2E test is needed. See 3.5.

### 2.15 ESLint (required by Next.js docs only as one of two options)

| Field | Value |
| --- | --- |
| Role | Linter. Next.js docs (installation page): "Next.js supports linting with either ESLint or Biome." `next lint` is replaced by the ESLint CLI; `next build` no longer lints. Prettier is not required by those docs. |
| Latest stable | `eslint` 10.15.0 (`latest`); `maintenance` tag **9.39.5**. Dates NOT VERIFIED. `eslint-config-next` **16.4.6** (same release as Next.js). `typescript-eslint` 8.72.2 (date NOT VERIFIED). |
| License | MIT (registry) for eslint, eslint-config-next, typescript-eslint, eslint-plugin-* below. |
| engines/peer | `eslint-config-next@16.4.6`: peer `eslint >=9.0.0`, `typescript >=3.3.1` (optional); deps `eslint-plugin-import ^2.32.0`, `eslint-plugin-react ^7.37.0`, `eslint-plugin-jsx-a11y ^6.10.0`, `eslint-plugin-react-hooks ^7.1.0`, `typescript-eslint ^8.56.0`. Their latest versions declare peer `eslint`: `eslint-plugin-import@2.32.0 ^2 ... \|\| ^9`; `eslint-plugin-react@7.37.5 ... \|\| ^9.7`; `eslint-plugin-jsx-a11y@6.10.2 ... \|\| ^9`; `eslint-plugin-react-hooks@7.1.1` allows `^10`. `eslint@10.15.0` engines: `^20.19.0 \|\| ^22.13.0 \|\| >=24`. |
| Sources (fetched 2026-11-15) | https://registry.npmjs.org/eslint/latest ; https://registry.npmjs.org/-/package/eslint/dist-tags ; https://registry.npmjs.org/eslint-config-next/latest ; https://registry.npmjs.org/eslint-plugin-import/latest ; https://registry.npmjs.org/eslint-plugin-react/latest ; https://registry.npmjs.org/eslint-plugin-jsx-a11y/latest ; https://registry.npmjs.org/eslint-plugin-react-hooks/latest ; https://nextjs.org/docs/app/getting-started/installation |
| Verdict | PROPOSE `eslint@9.39.5` (maintenance tag) with `eslint-config-next@16.4.6`. ESLint 10.15.0 is the latest release but three plugins pulled in by `eslint-config-next` declare peer ranges that stop at ESLint 9. Biome is the other option the docs list (not evaluated). |

## 3. Open items

### 3.1 Node.js runtime baseline

Facts:
- Active LTS: v26. Maintenance LTS: v24 and v22 (derived from `schedule.json` against 2026-11-15). Node 20 and 25 are EOL.
- Next.js 16.4.6: `engines.node >=20.9.0`; docs "Minimum Node.js version: 20.9".
- Tool ranges: vitest `^22.12 || ^24 || >=26`; jsdom `^22.22.2 || ^24.15.0 || >=26`; vite `^20.19 || >=22.12`; ESLint 10 `^20.19 || ^22.13 || >=24`; shadcn CLI `>=20.18.1`; Orval and Hey API `>=22.18.0`; Playwright docs "latest 22.x, 24.x or 26.x".
- Tailwind 4.3.4 and later need `require()` of ESM: fails on Node before 20.19 and 22.0 to 22.11 (Next.js PR #101061).
- Package manager and Corepack: see 2.2.

Options: Node 22 (Maintenance, to 2027-04-30), Node 24 (Maintenance, to 2028-04-30), Node 26 (Active, to 2029-04-30).

**PROPOSED default:** Node 24, `>= 24.15.0`, pinned in `.nvmrc`/`engines`; revisit Node 26 once Next.js documents or tests it. Package manager pnpm 12.21.0 via `packageManager`.

### 3.2 O-02: how the Next.js app is served on localhost

Facts (Next.js docs, fetched 2026-11-15):
- `next dev -H, --hostname <hostname>`: "Default: 0.0.0.0". `next start -H, --hostname`: "(default: 0.0.0.0)". Port: `-p, --port`, default 3000, env `PORT` (`PORT` cannot be set in `.env`). Source: https://nextjs.org/docs/app/api-reference/cli/next . No hostname environment variable is documented on that page (NOT VERIFIED whether one exists elsewhere).
- The `allowedDevOrigins` page says the dev server's initial hostname is "`localhost` by default". That conflicts with the CLI page. Treat the default as unreliable and always pass the flag, as `security.md` section 8 requires.
- Exact bind to loopback: `next dev -H 127.0.0.1 -p <port>` and `next start -H 127.0.0.1 -p <port>`. With `npm run`, put `--` before flags (CLI page).
- `next dev` writes to `.next/dev`; `next build` writes to `.next`, so both can run concurrently.
- Static export (`output: 'export'`, https://nextjs.org/docs/app/guides/static-exports): "Unsupported Features" list: Dynamic Routes with `dynamicParams: true`; Dynamic Routes without `generateStaticParams()`; Route Handlers that rely on Request; **Cookies; Rewrites; Redirects; Headers; Proxy**; Incremental Static Regeneration; Image Optimization with the default loader; Draft Mode; **Server Actions**; Intercepting Routes. Output is HTML/CSS/JS "hosted on any web server".
- Self-hosting page: with `next start`, Proxy, Image Optimization and Cache Components work. "It's recommended to use a reverse proxy (like nginx) in front of your Next.js server rather than exposing it directly to the internet."
- Memory: no figures in the docs. The memory guide says the server "preloads each page's JavaScript modules into memory" at start, "a larger initial memory footprint"; `experimental.preloadEntriesOnStart: false` disables it. Self-hosting page: cache is stored in memory (default 50 MB) and on disk. Type-checking during `next build` "may require a lot of memory".

Options:
- A. `next dev` only. Not a product mode: dev CSP needs `'unsafe-eval'` and inline styles (3.7).
- B. `next build` + `next start -H 127.0.0.1`. Node server process (extra process and memory; not measured). Supports cookies, rewrites, headers, Proxy, Server Actions, nonce CSP.
- C. `output: 'export'` served by another process (for example the Go server). No Node process at runtime. But no Proxy, so no per-request nonce; no `headers()`, so CSP must be set by the serving process; no rewrites, so the browser to API path is direct cross-origin or a separate reverse proxy.

**PROPOSED default: B for the MVP** (and `next dev -H 127.0.0.1` for development). Reason from the facts: the product requires a CSP without inline scripts, and the documented nonce path needs Proxy plus dynamic rendering, which static export does not allow. C remains possible with the experimental hash-based SRI approach (3.7) but that is marked experimental.

### 3.3 O-03: browser-to-API path

Facts:
- Cookies: RFC 6265 section 8.5 (https://www.rfc-editor.org/rfc/rfc6265.txt): "Cookies do not provide isolation by port. If a cookie is readable by a service running on one port, the cookie is also readable by a service running on another port of the same server. If a cookie is writable by a service on one port, the cookie is also writable by a service running on another port." Servers "SHOULD NOT both run mutually distrusting services on different ports of the same host and use cookies to store security-sensitive information."
- SameSite: MDN (https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Set-Cookie) says cross-site means a request from a different site "including the scheme". `Strict` sends only for same-site requests; `Lax` also sends on cross-site top-level navigations with a safe method; `None` needs `Secure`. Some browsers default to `Lax`, in a "more permissive version" that also sends on POST for two minutes after the cookie is set. The cookies draft (draft-ietf-httpbis-rfc6265bis, https://datatracker.ietf.org/doc/draft-ietf-httpbis-rfc6265bis/, text dated December 2025 as served on the fetch date; publication as an RFC NOT VERIFIED) defines the "site for cookies" as an origin with "a scheme and registered domain". Port is not part of that definition. **Derived:** `http://127.0.0.1:3000` and `http://127.0.0.1:8080` are same-site, so SameSite does not separate the Next.js origin from the Go API origin.
- MDN: `Secure` cookies need `https:` "(except on localhost)"; `__Host-` and `__Secure-` prefixes require a secure page. Whether `127.0.0.1` counts as localhost for this rule is NOT VERIFIED.
- Next.js rewrites (https://nextjs.org/docs/app/api-reference/config/next-config-js/rewrites): "Rewrites act as a URL proxy and mask the destination path". Destination can be an external URL. Order: headers, redirects, proxy, `beforeFiles`, filesystem, `afterFiles`, dynamic routes, `fallback`. Rewrites are listed as unsupported in static export (3.2). Behaviour for streaming, large bodies, timeouts and the forwarded `Host` header is not described (NOT VERIFIED).
- Server Actions (https://nextjs.org/docs/app/guides/data-security): only `POST`; "Server Actions in Next.js also compare the Origin header to the Host header (or X-Forwarded-Host). If these don't match, the request will be aborted." `serverActions.allowedOrigins` lists extra safe origins. Server Actions "should still [be treated] as reachable via direct POST requests"; verify authentication and authorization inside each.
- Route Handlers (https://nextjs.org/docs/app/api-reference/file-conventions/route): the reference page describes no built-in CSRF or Origin check. The data-security guide says `proxy.ts` and `route.ts` "have a lot of power. Spend extra time auditing these."
- Reverse proxy: self-hosting page recommends one in front of `next start`; nginx needs `X-Accel-Buffering: no` for streaming.

Options and trade-offs (facts only):
1. **Direct cross-origin** (browser at Next origin calls Go API origin). Needs CORS with credentials and an explicit origin list, a CSRF design in Go (SameSite will not help between ports), and CSP `connect-src` listing the API origin. Cookie is readable by any local service on the same host (section 8.5).
2. **Same-origin via Next `rewrites`**. No CORS. CSP `connect-src 'self'` suffices. Needs `next start` (not static export). The Next.js process sits in every API call. Go still must validate Host and Origin itself, and must know which `Host`/`X-Forwarded-Host` Next forwards (NOT VERIFIED). Cookie port-sharing issue is unchanged.
3. **Reverse proxy** (one origin for UI and API). Same-origin benefits, works with either serve mode, adds one more process and its config. Next.js docs recommend a reverse proxy for self-hosting in general.

**PROPOSED default: option 2** (rewrites under `next start`), with the Go API on a different loopback port, session cookie host-only (no `Domain`), and Go-side Host allow-list, Origin check and CSRF token on every state-changing request regardless of path. Re-evaluate option 3 if rewrites show problems with streaming job output (needs a test). Owner decision.

### 3.4 O-04: OpenAPI TypeScript client generation

Facts and options are in table 2.13. Summary: openapi-typescript 7.13.0 (MIT, OpenAPI 3.0 and 3.1, `.d.ts` only, peer `typescript ^5.x`, no release since 2026-02-11) with openapi-fetch 0.17.0 (MIT, one dependency, "6 kb"); Orval 8.48.0 (MIT, very active, Node >= 22.18, emits React Query hooks and mocks); Hey API 0.99.0 (0.x, pin exact, four breaking changes in this release, Node >= 22.18).

**PROPOSED default:** openapi-typescript 7.13.0 + openapi-fetch 0.17.0 + TypeScript 5.9.3. Fallback: Orval 8.48.0 with TypeScript 6.0.x. The Go API's OpenAPI version (3.0 or 3.1) is not yet decided; both options accept 3.0 and 3.1 per their sites except Orval 3.1 (not confirmed).

### 3.5 O-18: frontend test tooling

Facts: table 2.14. Vitest requires Vite >= 6.4.0 and Node >= 22.12.0 (guide). RTL requires `@testing-library/dom` as a peer. Next.js docs: Vitest cannot test async Server Components; use E2E for them. Playwright browsers are separate downloads; docs mention `--only-shell` and per-browser install. No primary size figure.

**PROPOSED minimal default (8 GB laptop):** Vitest 5.1.0 + jsdom 31.0.1 + `@testing-library/react` 16.3.3 + `@testing-library/dom` ^10 + `@vitejs/plugin-react` (version to be resolved), run `vitest run` on demand. Add `@playwright/test` 1.65.1 later with only Chromium installed (`--only-shell` if headless only), run in root `tests/e2e/`. Do not run the dev server, Vitest watch and Playwright at the same time (resource use not measured; advice, not a verified fact).

### 3.6 Cookies and session (localhost, server-side session cookie)

See 3.3 for the verified facts. Consequences for the design (inference from those facts): port does not isolate cookies; SameSite does not separate localhost ports; `Secure`/`__Host-` are not usable over plain `http://127.0.0.1` unless the browser treats it like `localhost` (NOT VERIFIED); CSRF protection must not rely on SameSite (matches `security.md` section 8).

### 3.7 Content-Security-Policy in Next.js

Source: https://nextjs.org/docs/app/guides/content-security-policy (page `lastUpdated: 2026-03-20`, fetched 2026-11-15).
- Nonce path: create `proxy.ts`, generate a nonce per request, set the `Content-Security-Policy` response header and an `x-nonce` request header. Production example: `script-src 'self' 'nonce-<nonce>' 'strict-dynamic'`; `style-src 'self' 'nonce-<nonce>'`; plus `default-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'`. Development: `'unsafe-eval'` for scripts and `'unsafe-inline'` for styles. "`unsafe-eval` is not required for production."
- Next.js reads the nonce from the request CSP header during server rendering and applies it to framework scripts, page bundles, inline scripts/styles it generates, and `<Script nonce>`.
- "You **must use dynamic rendering** to add nonces." Static optimization and ISR are disabled; PPR is incompatible; CDN caching is not available; more server load.
- Without nonces, the documented config uses `'unsafe-inline'` for scripts, which the product forbids.
- Alternative: `experimental.sri` (hash-based CSP via Subresource Integrity), keeps static generation; marked "Experimental ... may change or be removed", App Router only.
- Not verified: whether Radix/shadcn components emit inline `style` attributes that a nonce-only `style-src` would block. Needs a test before the UI design is fixed.

Consequence: nonce CSP needs `next start` with `proxy.ts` (3.2 option B).

## 4. Compatibility matrix (verified constraints only)

| Pair | Constraint (source) | Result |
| --- | --- | --- |
| next 16.4.6 and react/react-dom 19.3.1 | peer `^18.2.0 \|\| ... \|\| ^19.0.0` (registry) | OK |
| next 16.4.6 and Node 24.x | `engines >=20.9.0` | OK |
| next 16.4.6 and TypeScript 5.9.3 | docs minimum 5.1.0 | OK |
| next 16.4.6 and TypeScript 7.0.2 | docs: works through `tsc` CLI, no JS compiler API | Works with limits; blocked by typescript-eslint peer |
| @tanstack/react-query 5.107.1 and react 19.3.1 | peer `^18 \|\| ^19` | OK |
| @tanstack/react-query and TypeScript 5.9 | CI type-tests 5.7 to 5.9, 7.0, 7.1 | OK |
| tailwindcss 4.3.5 / @tailwindcss/postcss 4.3.5 and Node 24 | needs `require(esm)`; fails on < 20.19 and 22.0 to 22.11 | OK on Node 24 |
| tailwindcss v4 and browsers | Chrome 111, Safari 16.4, Firefox 128 vs Next.js Firefox 111 | Effective Firefox floor 128 |
| shadcn 4.23.0 (Tailwind v4, React 19) | docs: new projects use Tailwind v4 and React 19 | OK |
| radix-ui 1.8.1 and react 19.3.1 | peer includes `^19.0` | OK |
| zustand 5.1.0 and react 19.3.1 | peer `react >=18` (optional) | OK |
| react-hook-form 7.92.0 and react 19.3.1 | peer includes `^19` | OK |
| @hookform/resolvers 7.0.6 with react-hook-form 7.92.0 | peer `^7.55.0` | OK |
| @hookform/resolvers 7.0.6 with zod 4.6.7 | peer `zod ^3.25.0 \|\| ^4.0.0` | OK |
| zod 4.6.7 | no peers | OK |
| openapi-typescript 7.13.0 and TypeScript 5.9.3 | peer `^5.x` | OK |
| openapi-typescript 7.13.0 and TypeScript 6.x or 7.x | peer `^5.x` | **Conflict** |
| typescript-eslint 8.72.2 and TypeScript 5.9.3 / 6.0.x | peer `>=4.8.4 <6.1.0` | OK |
| typescript-eslint 8.72.2 and TypeScript 7.0.2 | same range | **Conflict** |
| eslint-config-next 16.4.6 and eslint 9.39.5 | peer `>=9.0.0`; plugin peers up to `^9` | OK |
| eslint-config-next 16.4.6 and eslint 10.15.0 | import/react/jsx-a11y plugin peers stop at `^9` | **Conflict (peer ranges)** |
| vitest 5.1.0 and vite 8.3.8 | peer `^6.4.0 \|\| ^7 \|\| ^8` | OK |
| vitest 5.1.0 and Node 24.x | `^22.12.0 \|\| ^24.0.0 \|\| >=26.0.0` | OK |
| jsdom 31.0.1 and Node 24.x | `^24.15.0` | OK only if Node >= 24.15.0 |
| @testing-library/react 16.3.3 and react 19.3.1 | peer `^18 \|\| ^19`, needs `@testing-library/dom ^10` | OK |
| @playwright/test 1.65.1 and Node 24.x | `engines >=20`; docs list 22.x, 24.x, 26.x | OK |
| Orval 8.48.0 / Hey API 0.99.0 and Node 24 | `>=22.18.0` | OK |

Not verified in this matrix: `react-dom` 19.3.1 exact publication, `@vitejs/plugin-react`, `@testing-library/dom`, `@types/react-dom`, `@types/node` exact versions, `tailwind-merge`, `class-variance-authority`, `clsx`, `lucide-react`, `tw-animate-css` (installed by shadcn components).

## 5. Versions to pin (stable only; no canary, rc, beta or preview)

| Technology | Pin | Note |
| --- | --- | --- |
| Node.js | 24.x, >= 24.15.0 (24.22.3 on 2026-11-10) | Owner decision vs 26 |
| pnpm | 12.21.0 | Or npm bundled |
| next, eslint-config-next | 16.4.6 | Active LTS line |
| react, react-dom | 19.3.1 | Confirm react-dom number at install |
| typescript | 5.9.3 | Because of openapi-typescript and typescript-eslint peers. Do not take 7.0.2 |
| @types/react | 19.3.0 | npm `latest`; `ts5.9` tag also 19.3.0 |
| @types/node | Match the Node major (24.x) | Exact version NOT VERIFIED (only `old-version` 24.19.2 and `latest` 26.6.6 seen) |
| @tanstack/react-query | 5.107.1 | |
| tailwindcss, @tailwindcss/postcss | 4.3.5 | Needs Node >= 22.12 |
| shadcn (CLI) | 4.23.0 | Radix base |
| radix-ui | 1.8.1 | |
| zustand | 5.1.0 | Only where needed |
| react-hook-form | 7.92.0 | |
| @hookform/resolvers | 7.0.6 | |
| zod | 4.6.7 | |
| openapi-typescript, openapi-fetch | 7.13.0, 0.17.0 | Fallback Orval 8.48.0 |
| vitest, vite, jsdom, @testing-library/react | 5.1.0, 8.3.8, 31.0.1, 16.3.3 | + `@testing-library/dom ^10` |
| @playwright/test | 1.65.1 | Deferred; Chromium only |
| eslint | 9.39.5 (maintenance tag) | Not 10.15.0 |

Latest vs supported: Next.js 16.4.6 is both latest and Active LTS. Next.js 15.5.x is Maintenance LTS only (security fixes). Node 26 is Active LTS, Node 24 and 22 are Maintenance LTS. TypeScript 7.0.2 and ESLint 10.15.0 are the latest releases but are not pinned because of the peer-range conflicts above.

## 6. Claims attempted but not verified

1. Memory use of `next dev`, `next start`, Vitest, Playwright, or any package manager (no primary figure exists in the fetched docs; nothing was measured).
2. Playwright browser download sizes (docs give commands, no sizes).
3. Whether any documented environment variable sets the `next dev`/`next start` hostname (CLI page lists only `PORT`).
4. Why the `allowedDevOrigins` page says the default hostname is `localhost` while the CLI page says 0.0.0.0.
5. Next.js `rewrites` behaviour with streaming responses, large request bodies, timeouts, and which `Host`/`X-Forwarded-*` headers are forwarded to the destination.
6. Whether Next.js 16.4 is tested on Node 26.
7. Zod's minimum TypeScript version (zod.dev fetch returned no match).
8. Zustand's Next.js guide (URL returned 404).
9. Release dates: pnpm 12.21.0, vite 8.3.8, jsdom 31.0.1, eslint 9.39.5 and 10.15.0, typescript-eslint 8.72.2, TypeScript 6.0.3.
10. Whether a TypeScript 5.9.x later than 5.9.3 exists.
11. Hey API LICENSE file (404 at the tried path; MIT taken from the npm registry only).
12. Orval's explicit OpenAPI 3.1 support; Hey API's exact supported OpenAPI versions.
13. Support-window statements for TanStack Query, Tailwind CSS, shadcn/ui, Radix, Zustand, React Hook Form, Zod, TypeScript, Vitest, Playwright.
14. The Playwright and TypeScript LICENSE files were read; licenses for `eslint-*`, `jsdom`, `vite`, `pnpm`, `typescript-eslint`, `tailwind-merge` were taken from the npm `license` field only (not the repository LICENSE).
15. Whether `127.0.0.1` is treated as `localhost` by browsers for `Secure` cookies and `__Host-` prefixes.
16. Whether the cookies draft has been published as an RFC; the fetched draft text is dated December 2025.
17. Radix/shadcn inline `style` attributes under a nonce-only `style-src`.
18. Transitive licenses of shadcn-installed dependencies (`tw-animate-css`, `lucide-react`, `class-variance-authority`, `clsx`, `tailwind-merge`, `sonner`, Radix internals).
19. Prettier: not required by the Next.js docs fetched; not evaluated.
20. `@vitejs/plugin-react`, `vite-tsconfig-paths`, `@testing-library/dom`, `@types/react-dom`, `@tanstack/eslint-plugin-query` versions.
