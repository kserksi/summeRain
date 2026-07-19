# Frontend Architecture Design · summeRain (Index)

> [!WARNING]
> **Archived design record.** This June 18, 2026 design set predates the
> completed V2 frontend. Some paths, versions, status statements, and scope
> decisions no longer match the implementation. Use it as historical design
> context, not as current operational guidance.

- **Date:** 2026-06-18
- **Status:** Finalized, awaiting implementation
- **Scope:** Rewrite the image-hosting frontend with a modern React stack, precisely aligned with the Go API in `backend/` (see `docs/API.md`)

This design is divided into standalone documents by topic for easier reading and maintenance. The sections cross-reference one another through relative links.

## Document Map

| # | Section | Contents | File |
|---|---|---|---|
| 01 | Overview and Technology Stack | Background, goals, hard constraints, technology stack, pinned dependency versions, and the TypeScript/ECMAScript baseline | [01-overview.md](01-overview.md) |
| 02 | Architecture and Infrastructure | Directory structure, API wrapper, authentication flow, TanStack Query, zustand, routing, and code splitting | [02-architecture.md](02-architecture.md) |
| 03 | Features and Pages | Five feature areas, queries and mutations, routes, and home-page strategy | [03-features.md](03-features.md) |
| 04 | Theme and UI | shadcn preset strategy, coffee palette, typography, and i18n | [04-theme-and-ui.md](04-theme-and-ui.md) |
| 05 | Build and Deployment | Vite build, artifact output, development proxy, and HTTPS prerequisite | [05-build-and-deploy.md](05-build-and-deploy.md) |
| 06 | Testing Strategy | Scope for Vitest, RTL, and MSW | [06-testing.md](06-testing.md) |
| 07 | Production Artifact Standards | Modularity, SRI/SemVer, local assets, variables and constants, minimal HTML, and i18n (mandatory requirements) | [07-production-standards.md](07-production-standards.md) |
| 08 | Coding Standards | Naming, TypeScript, React, state, styling, lint gates, security and accessibility, performance, and commits | [08-coding-standards.md](08-coding-standards.md) |
| 09 | Decisions and Scope | Migration cleanup, explicit exclusions (YAGNI), decision records, and references | [09-decisions-and-scope.md](09-decisions-and-scope.md) |
| 10 | Page-by-Page UI/UX | Eight pages plus global components, shadcn mapping, lightbox, private-token panel, and responsive behavior | [10-pages-ui-ux.md](10-pages-ui-ux.md) |
| DS | Design System | Style, tokens, components, states, responsiveness, motion, and theme switching (single source of truth) | [design-system/MASTER.md](design-system/MASTER.md) |

## Recommended Reading Order

1. **01 Overview** -> establish the overall context: what is being built, which tools are used, and which constraints apply
2. **02 Architecture** -> understand the code structure and data flow
3. **03 Features** + **04 Theme** -> cover the product and visual dimensions
4. **05 Build** -> learn how to run and deploy the application
5. **07 Production Standards** + **08 Coding Standards** -> follow the implementation rules
6. **06 Testing** + **09 Decisions** -> understand validation and the rationale behind historical decisions

> The original monolithic `2026-06-18-frontend-architecture-design.md` has been superseded by the documents in this directory.
