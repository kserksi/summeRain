# 09 · Decisions and Scope

> [!WARNING]
> **Archived design record.** This page predates the completed V2 frontend and
> may contain obsolete versions, paths, or implementation status.

> Part of: [Frontend Architecture Design (Index)](./README.md)

## Migration and Cleanup

- `frontend/` will replace the existing native-JavaScript frontend in the repository-root `index.html`, `css/`, and `js/`.
- Delete the old native-frontend files after the new frontend is ready and verified.
- Both implementations coexist during the transition without interfering with one another: the old frontend is accessed from the repository root, while the new frontend uses Vite during development and builds into `backend/web/`.

## Explicit Exclusions (YAGNI)

- Public gallery, discovery page, or community picks (the backend has no public-list endpoint)
- Image categories or tags (the backend Image model has no corresponding fields)
- Administrative image-review list (the backend has no corresponding endpoint)
- User deletion (the backend supports status changes only)
- Server-side rendering or a Node middleware layer
- Field-mapping transformation layer (use the backend's snake_case fields directly)

## Decision Record

| Decision | Choice | Rationale |
|---|---|---|
| Technology stack | React + Vite + TypeScript + shadcn/ui + Tailwind | Broad ecosystem, mature component library, and a good fit for a static SPA |
| Feature scope | Precisely match the backend | Avoid building capabilities the backend does not support |
| Project structure | Repository-root `frontend/` -> build to `backend/web/` | Separate frontend and backend source while satisfying the backend's static-serving contract |
| Code organization | Feature-based + TanStack Query | Clear boundaries and high cohesion across five areas, with centralized server state |
| shadcn preset | `apply --preset b3RZAU6YV`, ignoring its colors and typography | Reuse the preset's component structure while retaining the coffee palette |
| Routing | BrowserRouter + lazy loading | Supported by the backend SPA fallback and compatible with deep links |
| i18n | `en-US` by default, with `zh-CN` and `ja-JP` bundled; all copy stored in i18n resources | Make English primary while retaining complete localization support |
| Production artifacts | SHA512 SRI + SemVer + local resources + no magic values | Mandatory user-defined production-quality requirements; development is exempt |
| TypeScript/ECMAScript baseline | Target `ES2025` (the highest named target in TS 6.0) | TS 6.0 does not support a literal ES2026 target; choose a stable named target and leave actual downleveling to Vite |
| Coding standards | [08 Coding Standards](08-coding-standards.md): official guidance + user requirements, enforced by Prettier/ESLint/tsc gates | One measurable, verifiable implementation rule set |
| Session-switch safety | On logout/login, call `queryClient.clear()` and hard-refresh; do not persist `auth-store`; retain backend `RequireAdmin` 403 enforcement | Prevent in-memory data from crossing user boundaries and reaffirm that the frontend is not a security boundary (see [02](02-architecture.md) and [08](08-coding-standards.md)) |
| Private-image token model | One shared token per image; TTL defaults to 1h and is configurable from 10min to 72h, validated in milliseconds; token text is immutable; revocation does not reissue automatically and owner/admin must request another token manually; owner/admin sessions bypass the token for direct access; missing/wrong/expired -> 403, revoked -> 404 | User-defined behavior, already implemented by the backend (API.md §4.3/§5, error codes 4037/4042) |
| CAPTCHA | Three pluggable providers (reCAPTCHA/Turnstile/Geetest v4), selected by an administrator, defaulting to none | Prefer Turnstile/Geetest for audiences in China; CAPTCHA is the sole controlled external-resource exception in §07, and the default none setting preserves the rule |
| Visual design and page UI/UX | Warm Soft Studio (maia card style + large radii + coffee tokens); Tabler Icons; page-level specifications in the [Design System MASTER](design-system/MASTER.md) and [10-pages-ui-ux.md](10-pages-ui-ux.md) | Validated by the prototype; use shadcn/ui and prohibit native controls |
| Icon library | **Tabler Icons** (`@tabler/icons-react`), overriding the preset's Phosphor icons | One outline style and consistent appearance; `components.json` uses `iconLibrary=tabler` |
| Theme switching | View Transitions as the primary path (circular reveal of real content) with a `.theme-mask` WAAPI fallback; switch immediately for reduced motion | Explicit header entry point, best available effect, broad browser support, and accessibility |

## References

- Backend API contract: `docs/API.md`
- Backend entry point (SPA fallback/static serving): `backend/cmd/server/main.go`
- Backend authentication and CSRF: `backend/internal/middleware/{auth,csrf}.go`
- Existing coffee theme (color source to migrate): `css/style.css`
- Official coding guidance: [React](https://react.dev/learn/thinking-in-react) · [TypeScript Handbook](https://www.typescriptlang.org/docs/handbook/intro.html) · [typescript-eslint](https://typescript-eslint.io) · [Tailwind CSS](https://tailwindcss.com/docs) · [Prettier](https://prettier.io) · [WCAG](https://www.w3.org/WAI/standards-guidelines/wcag/)

---

<- [08 Coding Standards](08-coding-standards.md) · [Index](./README.md)
