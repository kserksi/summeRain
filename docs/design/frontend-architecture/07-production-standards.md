# 07 · Production Artifact Standards (Mandatory)

> [!WARNING]
> **Archived design record.** This page predates the completed V2 frontend and
> may contain obsolete versions, paths, or implementation status.

> Part of: [Frontend Architecture Design (Index)](./README.md)

> The following rules are mandatory for the final production artifacts in `backend/web/`. **Development and testing are exempt** because development uses unsigned Vite output, which is not subject to SRI or release-version requirements.

## Modularity and Atomicity

- Split all JavaScript and CSS into the smallest practical atomic modules by feature and route (feature-based directories plus route-level React.lazy bundles; see [02 Architecture](02-architecture.md)). Do not produce a monolithic bundle.
- Every component, hook, and utility must be an independent, reusable unit with a single responsibility.

## SRI Integrity (SHA512) + SemVer Versions

- The production build must calculate a **SHA512** integrity hash for **every** JavaScript and CSS artifact and generate `backend/web/assets.manifest.json` in the form `{ "<file-name>": { "version": "<semver>", "integrity": "sha512-..." } }`.
- **Versions follow Semantic Versioning 2.0.0.** Version granularity is the **release version**: the whole build's SemVer (for example, `1.0.0`, sourced from the `version` in `package.json`) labels every artifact in that build, while each file records its own SRI hash. If per-module versions become necessary, the manifest may later add a `moduleVersion` field.
- **Bump rule:** before a release, manually change the `version` in `package.json`: `patch` for fixes, `minor` for new features, and `major` for incompatible changes. CI uses that `version` for the Git tag and manifest. Do not release without a bump; the artifact `version` must match the tag. Do not introduce tools such as changesets or standard-version yet (YAGNI); use a manual-plus-CI-tag process.
- Any cross-origin or externally hosted resource introduced in the future must include SRI or must not be used. The current project has none; see the next section.
- **Coverage (important):**
  - **Entrypoints and static artifacts** (entry JavaScript/CSS and `<link rel="modulepreload">` elements in `index.html`) -> `vite-plugin-sri3` injects `integrity="sha512-..."` into `index.html` for native browser verification.
  - **Dynamic bundles** (chunks loaded through React.lazy and `import()`) -> the browser's **native `integrity` mechanism does not cover runtime `import()`**. Injecting integrity only into static `index.html` elements is insufficient. Therefore, the implementation **must add a runtime integrity guard** that fetches each dynamic chunk before import, compares its SHA512 value with `assets.manifest.json`, and refuses execution and reports an error on mismatch. This is a mandatory implementation subtask.
  - As an alternative, a plugin may rewrite the Vite modulepreload polyfill and add `integrity` to preload links created at runtime, but it must be verified to cover lazy-loaded chunks in practice.
  - Risk context: all artifacts are same-origin and do not use a CDN (see the next section), which reduces the third-party-tampering threat that SRI primarily addresses. Nevertheless, because “verify every artifact” is a hard requirement, dynamic chunks must be covered by the guard above and may not be exempted.

## Local External Resources

- **Runtime references to cross-origin/CDN resources are prohibited.** Vite must bundle every third-party dependency into local artifacts; typography uses the system font stack with no webfont; and there are no remote images or icons.
- After building, verify that the artifacts contain no external links in `https://` or `http://` form. Only same-origin relative backend paths under `/api` and `/i` are allowed.
- **The only controlled exception is CAPTCHA.** Only when an administrator enables a provider (`captcha_provider ≠ none`; see [03](03-features.md#pluggable-captcha-administrator-selected-default-none)) may the official script for that provider be loaded externally. reCAPTCHA, Turnstile, and Geetest cannot be self-hosted. The default `none` setting means **zero external links**, so this section's mandatory rule is fully satisfied. Enabling a provider exempts only that provider, and its domain must be explicitly registered in the build-validation allowlist.

## Variables and Constants

- **No magic values:** centralize all literals, including numeric limits, timeouts, page sizes, storage units, route paths, Query keys, and storage keys, in `src/config/constants.ts`. Export constants in **UPPER_SNAKE_CASE**, following MDN's naming guidance (aligned with [08 Coding Standards](08-coding-standards.md)). Components may not contain bare numbers or strings.
- **JavaScript/TypeScript variables:** camelCase, following MDN guidance.
- **CSS custom properties:** kebab-case as required by CSS (`--coffee-bg`, for example); this language requirement does not conflict with the camelCase rule.
- **Encoding:** all source files and artifacts use **UTF-8** without a BOM; `index.html` contains `<meta charset="UTF-8">`.

## Minimal HTML

- `index.html` contains only the **minimum necessary** inline JavaScript and CSS. The sole permitted inline script is a short pre-paint bootstrap that reads the saved theme preference and applies `.dark` to prevent a flash; everything else belongs in external bundles.
- Because that script must execute before the bundle loads and cannot import the constants module, its local-storage key is the **only controlled exception**. Keep it manually synchronized with the corresponding constant in `config/constants.ts` and document the relationship in a code comment. The theme key is fixed as **`ic_theme`**, with `light` and `dark` values.

## i18n (English First, Multilingual Support)

- Use `en-US` by default and provide `zh-CN` and `ja-JP`. Components retrieve copy with `t()` from `react-i18next` and do not hard-code user-facing strings (see [04 Theme and UI](04-theme-and-ui.md)).
- Keep the structure extensible so that switching languages never requires component-code changes.

---

<- [06 Testing](06-testing.md) · [Index](./README.md) · Next: [08 Coding Standards](08-coding-standards.md)
