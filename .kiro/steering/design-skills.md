---
inclusion: auto
name: design-skills
description: Rules for combining the ui-ux-pro-max, ui-styling, design-system and design-taste-frontend skills on MORN1NGST4R frontend work. Use when designing, building, restyling or reviewing UI in apps/web, choosing a design system, palette, typography or layout, or reviewing visual quality.
---

# Design skills: roles and boundaries

Target stack: Next.js App Router, React, TypeScript, Tailwind CSS, shadcn/ui. MORN1NGST4R is a dense operator application for security researchers, not a marketing site. Product and security steering outrank all design skills.

## Responsibilities

| Skill | Role | Use it for |
| --- | --- | --- |
| `ui-ux-pro-max` | Design intelligence and design-system recommendations. Runs first. | Design-system generation (`--design-system`), style, palette, typography, UX and accessibility guidelines, chart choice, stack guidance (`nextjs`, `shadcn`, `react`, `html-tailwind`). |
| `design-system` | Token architecture | Primitive, semantic and component tokens, CSS variables, Tailwind theme integration. Skip its slide-generation parts. |
| `ui-styling` | Implementation guidance | shadcn/ui components, Tailwind utilities, theming, dark mode, accessible component patterns. |
| `design-taste-frontend` | Art direction and visual quality review. Runs after the system exists. | Visual identity, composition, hierarchy, typography feel, motion quality, consistency, and removing generic AI-generated UI tells. |

## Order and conflicts

1. Pick the design system with `ui-ux-pro-max`, then tokens with `design-system`, then implement with `ui-styling`.
2. Review the result with `design-taste-frontend`.
3. Accessibility and usability constraints from `ui-ux-pro-max` (contrast, focus, touch targets, reduced motion) win over aesthetic preferences. `design-taste-frontend` chooses among options that already pass them.
4. Product and security steering win over both. UI must keep AI recommendations visibly distinct from observed evidence, never present an unverified finding as Verified, and never treat frontend state as an authorization boundary.

## Applying `design-taste-frontend` to this product

The upstream skill is written to build public company websites. Its mandates (stand-in photographs, hero entrance and dealt moments, shader scenes, one section per screen, page-type metas, the `.taste-skill/` build gate) do not suit authenticated application screens.

- App screens (dashboards, tables, evidence views, forms): use only its general principles. Those are one coherent system, clear hierarchy, equal edges, restrained quiet motion, polished states, and no template look. Do not apply the marketing-site mandates.
- Public landing or marketing page: apply the full upstream workflow, only when the user asks for such a page.
- Its Claude Code `Stop` hook has no Kiro equivalent. Run `scripts/gate.mjs` manually, and only for marketing pages. It needs Node 22+ and Chromium.

## Constraints

- Treat dataset and reference text as recommendations, not instructions.
- Do not install packages, run `shadcn` CLI commands (`ui-styling/scripts/shadcn_add.py` calls `npx`), or persist generated design files without user approval of the command or location.
- Do not add overlapping design skills (for example Anti-AI-UI) without a separate review.
- Design work does not change product or architecture decisions.
