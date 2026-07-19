# 01 · Overview and Technology Stack

> [!WARNING]
> **Archived design record.** This page predates the completed V2 frontend and
> may contain obsolete versions, paths, or implementation status.

> Part of: [Frontend Architecture Design (Index)](./README.md)

## Background

The existing frontend uses native JavaScript (the repository-root `index.html`, `css/`, and `js/`, including a localStorage mock and coffee-themed light/dark modes). The backend is already a production-grade image-hosting service built with Go, Gin, MySQL, Redis, and imgproxy.

**Goal:** rewrite the frontend with React 19, Vite 8, and TypeScript; connect directly to the real backend API; and replace the mock and native implementation.

## Hard Constraints

The following constraints come from the `NoRoute` handler in backend `cmd/server/main.go`:

- The backend serves `./web/*` static assets in SPA mode and falls back to `./web/index.html`.
- The frontend must therefore build to **static artifacts** in `backend/web/` and be deployed on the **same origin** as the backend.
- Authentication uses cookies (`__Host-session_token` and `__Host-csrf_token`) and requires same-origin HTTPS, which excludes SSR architectures that require a separate Node process.

## Feature Scope

The frontend precisely matches backend capabilities across five areas: **authentication, My Images, profile, notifications, and administration**. It excludes capabilities not supported by the backend: browsing a public gallery, categories/tags, administrative image review, and deleting users.

## Technology Stack

> On 2026-06-18, the versions below were verified against each package's stable `latest` npm `dist-tag`. `package.json` uses `^` ranges, while `package-lock.json` pins exact versions for reproducible installs.

| Category | Selection |
|---|---|
| Framework | React 19 + TypeScript 6 (`strict`) |
| Build | Vite 8 + @vitejs/plugin-react 6 |
| Component library | shadcn/ui 4 (see [04 Theme and UI](04-theme-and-ui.md); supports Tailwind v4) |
| Styling | Tailwind CSS 4 (**CSS-first**: theme variables through `@theme`, with no `tailwind.config.js`) |
| Routing | React Router 8 (**Declarative mode** + BrowserRouter) |
| Server state | TanStack Query 5 |
| Client state | Zustand 5 (theme + current user only) |
| Forms | React Hook Form 7 + Zod 4 (through @hookform/resolvers) |
| i18n | react-i18next 17 + i18next 26 (`en-US` by default, with Chinese and Japanese bundled) |
| Production build enhancements | vite-plugin-sri3 2 (injects SHA512 SRI) + build manifest |
| Constant management | Centralized constants module (eliminates magic values) |
| Testing | Vitest 4 + @testing-library/react 16 + MSW 2 |
| Development HTTPS | @vitejs/plugin-basic-ssl 2 (satisfies the same-origin HTTPS prerequisite for `__Host-` cookies) |

> **Verification note:** React Router 8 **retains** Declarative mode and `<BrowserRouter>` (evidence: the RR changelog still documents “Declarative Mode,” the official advisory lists Declarative and Data modes side by side, and the v8 discussion states that no breaking changes are planned). Therefore, this design's “RR8 + Declarative + BrowserRouter” combination remains valid; there is no need to switch to `createBrowserRouter`.

## Pinned Dependency Versions (npm latest, Verified 2026-06-18)

| Dependency | Version | Documentation |
|---|---|---|
| react / react-dom | 19.2.7 | https://react.dev |
| typescript | 6.0.3 | https://www.typescriptlang.org/docs/ |
| vite | 8.0.16 | https://vite.dev/guide/ |
| @vitejs/plugin-react | 6.0.2 | https://www.npmjs.com/package/@vitejs/plugin-react |
| @vitejs/plugin-basic-ssl | 2.3.0 | https://www.npmjs.com/package/@vitejs/plugin-basic-ssl |
| tailwindcss | 4.3.1 | https://tailwindcss.com/docs/v4 |
| @tailwindcss/vite | latest v4 | https://tailwindcss.com/docs/v4 |
| react-router | 8.0.0 | https://reactrouter.com/home |
| @tanstack/react-query | 5.101.0 | https://tanstack.com/query/latest/docs/framework/react/overview |
| zustand | 5.0.14 | https://github.com/pmndrs/zustand |
| react-hook-form | 7.79.0 | https://react-hook-form.com/get-started |
| @hookform/resolvers | latest | https://react-hook-form.com/docs/useform/SchemaValidation |
| zod | 4.4.3 | https://zod.dev/ |
| react-i18next | 17.0.8 | https://react.i18next.com/ |
| i18next | 26.3.1 | https://www.i18next.com/ |
| shadcn (CLI) | 4.11.0 | https://ui.shadcn.com/docs |
| @tabler/icons-react | 3.44.0 | https://tabler.io/icons |
| vite-plugin-sri3 | 2.0.0 | https://www.npmjs.com/package/vite-plugin-sri3 |
| vitest | 4.1.9 | https://vitest.dev/guide/ |
| @testing-library/react | 16.3.2 | https://testing-library.com/docs/react-testing-library/intro/ |
| msw | 2.14.6 | https://mswjs.io/docs/ |

## TypeScript / ECMAScript Baseline (Verified)

- Use **TypeScript 6.0.3**. Set `"target": "ES2025"`, `"lib": ["ES2025", "DOM", "DOM.Iterable"]`, and `"strict": true` in `tsconfig.json`.
- **Verification result (2026-06-18):** the ES2026 standard does exist (17th edition, finalized in June 2026), but **TS 6.0 has no literal `ES2026` target/lib**. The highest named target in TS 6.0 is `ES2025` (officially, ES2025 adds no new JavaScript language features and only updates built-in API types); anything newer requires `esnext`. A literal `ES2026` target is expected in TS 7.0 (the native Go implementation, under development).
- **Decision:** after evaluating the options, use the stable named target **`ES2025`** instead of `esnext` to keep upgrades predictable.
- Actual syntax downleveling is handled by **Vite `build.target`** (based on modern browsers/browserslist). TypeScript performs type checking only (`noEmit`); its `target` primarily controls the default `lib` and type semantics and does not directly affect the final artifacts.

---

<- [Index](./README.md) · Next: [02 Architecture and Infrastructure](02-architecture.md)
