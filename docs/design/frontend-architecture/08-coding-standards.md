# 08 · Coding Standards

> [!WARNING]
> **Archived design record.** This page predates the completed V2 frontend and
> may contain obsolete versions, paths, or implementation status.

> Part of: [Frontend Architecture Design (Index)](./README.md)

> These standards combine the project's production requirements ([07 Production Standards](07-production-standards.md)) with official React, TypeScript, and Tailwind guidance to form one implementation rule set. Authoritative references: [React documentation](https://react.dev/learn/thinking-in-react) and [Rules of Hooks](https://react.dev/reference/rules), the [TypeScript Handbook](https://www.typescriptlang.org/docs/handbook/intro.html), [typescript-eslint](https://typescript-eslint.io), and [Tailwind CSS](https://tailwindcss.com/docs).

## General Rules

- Follow the three ecosystems' official guidance. [07 Production Standards](07-production-standards.md) defines mandatory production requirements, while this section defines everyday coding details; both apply together.
- Prefer self-documenting code. Do not add comments by default. Add `// why` only when the rationale is not obvious, never `// what`. Document public hooks and utilities with TSDoc.

## Naming

- Variables, functions, and ordinary files: camelCase. Components, types, and interfaces: PascalCase; component filenames match their components.
- Constants: `UPPER_SNAKE_CASE`, centralized in `config/constants.ts`. Hooks use the `use` prefix. i18n keys use dot-separated namespaces.
- Booleans use an `is`, `has`, `can`, or `should` prefix. Event handlers use `handleXxx` or `onXxx`.
- CSS custom properties use kebab-case as required by the language; everything else follows the camelCase rule above.

## TypeScript

- Set `"strict": true` and **prohibit `any`**. If `any` is genuinely unavoidable, explain why in a comment. Public APIs declare types explicitly; local code relies on inference.
- Use `interface` for extensible or mergeable object shapes and `type` for unions and utility types; do not mix the two conventions.
- Use `const` or `let`, never `var`. Prefer `async/await`. Treat `catch` variables as `unknown` and use the typed `ApiError` for errors.
- Eliminate magic values by centralizing literals in the constants module.

## React (Following Official Component and Hook Guidance)

- Use function components and hooks only. Follow the [Rules of Hooks](https://react.dev/reference/rules/rules-of-hooks). Each file has one primary component.
- Define props with `interface` or `type` and prefer required fields. Lists use stable `key` values, never array indices.
- **Prefer derived state over `useEffect`.** Effect dependencies must be accurate, and each effect must have one responsibility; avoid chains of effects.
- Prefer composition over inheritance and pure components over impure ones. Isolate side effects and return early from guards.
- Use React Hook Form + Zod for forms; the schema is the single source of truth for validation.

## State and Data

- **All server state goes through TanStack Query.** Do not cache API data in `useState` or zustand.
- zustand stores only genuine client state: the theme and current-user snapshot. Standardize and centralize Query keys.
- Mutations invalidate data with `invalidateQueries`; do not edit the cache manually unless implementing an optimistic update with rollback.

## Styling (Official Tailwind v4 Conventions)

- Prefer utility classes. **Bare hex colors are prohibited inside components**; use shadcn token classes such as `bg-primary`.
- Extract complex styling into components instead of overusing `@apply`. Use `cn()` (clsx + tailwind-merge) for conditional classes.
- Build responsive layouts **mobile-first** with `sm:`, `md:`, and `lg:`. Use the `dark:` variant based on the `.dark` class.

## Files and Exports

- Organize code by feature (see [02 Architecture](02-architecture.md)). Prefer **named exports**; page and route components may use default exports.
- Use `index.ts` barrel files sparingly to avoid circular dependencies and bundle growth.

## Formatting and Linting (Quality Gates)

- **Prettier:** two-space indentation, semicolons, double quotes, trailing commas set to `all`, line width 100, `LF`, and UTF-8 without a BOM.
- **ESLint (flat config):** `typescript-eslint` recommended + `react` recommended + `react-hooks` + `jsx-a11y`.
- **Layered gates** (see [06 Testing](06-testing.md)):
  - pre-commit (fast): `prettier --check` + `eslint` + `tsc --noEmit`
  - CI (blocks merge/release): the three checks above + `vitest run`
- Prohibit `console.*`, `debugger`, commented-out dead code, and unused imports.

## Security and Accessibility

- **The frontend is not a security boundary.** The backend decides all authentication and authorization outcomes (`/admin/*` passes through `RequireAdmin`, which returns 403 for ordinary users). Treat frontend code and chunks as public; never rely on hidden code or routes for access control. Backend enforcement is the only guard for sensitive data operations.
- **Clear caches when switching users.** Login and logout must call `queryClient.clear()` and perform a **hard refresh** with `window.location.assign`, preventing in-memory data from the previous session, especially TanStack Query caches, from crossing user boundaries. `auth-store` is not persisted (see [02 Session-Switch Data Cleanup](02-architecture.md)).
- Never print or commit secrets. Sanitize user input. Centralize CSRF and credential behavior in `lib/api.ts`.
- Prohibit `dangerouslySetInnerHTML` unless the input has been sanitized.
- Preserve shadcn's accessibility baseline: semantic HTML, `label`, `aria`, keyboard access, visible focus, image `alt` text, and text contrast of at least 4.5:1 (WCAG AA).

## Performance

- Lazy-load routes (see [02 Architecture](02-architecture.md)). Use `loading="lazy"` and appropriate dimensions for images.
- Choose sensible Query `staleTime` and `gcTime` values to avoid excessive requests. Measure before applying `memo`.

## Commits

- Use Conventional Commits (`feat:`, `fix:`, and so on) and keep each commit small and single-purpose.
- Commit only when the user explicitly requests it, consistent with the global convention.

---

<- [07 Production Standards](07-production-standards.md) · [Index](./README.md) · Next: [09 Decisions and Scope](09-decisions-and-scope.md)
