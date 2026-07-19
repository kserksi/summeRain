# 06 · Testing Strategy

> [!WARNING]
> **Archived design record.** This page predates the completed V2 frontend and
> may contain obsolete versions, paths, or implementation status.

> Part of: [Frontend Architecture Design (Index)](./README.md)

## Scope

**Vitest + React Testing Library:**

- Unit-test `lib/api.ts` (envelope unwrapping, CSRF injection, **logout on 401, disabled account on 4030, rate limiting on 429, and i18n error-code mapping**).
- Test each feature's `hooks.ts` with MSW-mocked endpoints to verify query and mutation behavior.

Write component tests for critical interactions: upload, infinite scrolling, form validation, visibility changes, reauthentication after a password change, and post-registration navigation.

## Trade-offs

Do not pursue exhaustive coverage. Prioritize the **API layer and authentication/query hooks** because they are central to system correctness and especially prone to regressions.

## Layered Quality Gates

- **pre-commit (local and fast):** `prettier --check` + `eslint` + `tsc --noEmit` (aligned with [08](08-coding-standards.md)).
- **CI (blocks merge and release):** all three checks above **plus `vitest run`**. Failing tests block merges, and release builds run the same test suite.

---

<- [05 Build and Deployment](05-build-and-deploy.md) · [Index](./README.md) · Next: [07 Production Artifact Standards](07-production-standards.md)
