# 03 · Features and Pages

> [!WARNING]
> **Archived design record.** This page predates the completed V2 frontend and
> may contain obsolete versions, paths, or implementation status.

> Part of: [Frontend Architecture Design (Index)](./README.md)

## Feature Areas and Queries/Mutations

| Feature | Key Query / Mutation | Route / Page |
|---|---|---|
| auth | `useMe` `useLogin` `useRegister` `useLogout` | `/login` `/register` |
| images | `useImages` (infinite scrolling) `useImage` `useUpload` `useDeleteImage` `useToggleVisibility` `useImageAccessToken` `useRevokeAccessToken` | `/images` `/images/:id` `/upload` |
| captcha | `usePublicConfig` `useCaptcha` | Startup probe + embedded in login/register |
| user | `useProfile` `useChangePassword` | `/profile` |
| notifications | `useNotifications` `useMarkRead` `useMarkAllRead` `useDeleteNotification` `useClearNotifications` | Header `NotificationBell` dropdown |
| admin | `useAdminUsers` (paginated) `useSetUserStatus` `useAdminStats` `useConfigs` `useUpdateConfigs` | `/admin` `/admin/users` `/admin/configs` |

## Landing Page and Dashboard Destination

- **`/` (public):** always a marketing landing page (hero, features, and CTA), with **no** public gallery because the backend has no such endpoint.
- **An authenticated user who visits `/`** is redirected automatically to **`/dashboard`**.
- **`/dashboard` (protected, console destination):** the destination after login, rendered conditionally according to `auth.user.role`:
  - Common area (all users): personal statistics, storage quota, and recent images (`useProfile`, `useImages`).
  - Administrator area (`role==='admin'` only): system overview cards (`useAdminStats`) and an “Open administration” link to `/admin`. This area is lazy-loaded and is not downloaded by ordinary users.
- **Role differentiation:** use conditional rendering plus guarded routes, not separate destinations. `/admin/*` is protected by `AdminGuard` (frontend `role` check) and backend `RequireAdmin` (returns 403), so ordinary users cannot enter even by typing the URL directly.

## Key Interaction Flows (Post-Success Behavior)

- **Registration, `useRegister`:** the backend **does not log the user in automatically** (API.md). On success, navigate to `/login` and show the toast “Registration successful. Please sign in.” The username may be prefilled.
- **Password change, `useChangePassword`:** the backend **clears every session** (API.md), immediately invalidating the current cookie. On success, proactively call `queryClient.clear()`, clear `auth-store`, navigate to `/login`, and show “Password changed. Please sign in again.” Do not wait for a later 401 fallback, which would leave the UI stuck in a dead session.
- **Visibility change, `useToggleVisibility`:** making an image private **invalidates its shared token** (the backend returns `tokens_revoked`/`warning`). If `warning` is nonempty after success, show it in a toast/alert and invalidate `['images']` and `['images', id]`. The detail response includes `access_token`, so invalidation refreshes the token as well.
- **Upload/delete:** after success, invalidate `['images']` and call `refreshUser()` because quota and counts changed (see the snapshot-refresh mechanism in [02](02-architecture.md)).

## Image Details, `/images/:id`

Display the large image, metadata, visibility control, and, for a private image, shared-token management. Only the owner may retrieve the record (backend authorization).

## Private Images (Shared-Token Model)

**Access rules** for `/i/:link` when `visibility=private`:

| Requester | Behavior |
|---|---|
| Owner or administrator (same-origin session) | **Automatically authorized** for direct viewing (`<img src="/i/<link>">` needs no token); the API returns the image's current shared token for display/sharing |
| Third party with a **valid, unexpired** token | Allow access with `no-store` |
| Third party with a **missing, incorrect, or expired** token | Return **403** |
| Third party with a **revoked** token | Return **404** |

**Token rules:**
- Each private image has **one shared token**. Its **text is immutable** while the token exists.
- **TTL:** default **1 hour (3,600,000 ms)**, validated by the backend in **milliseconds**. The owner/admin may select a duration when generating the token, from **10 minutes (600,000 ms) to 72 hours (259,200,000 ms)**. An expired token is invalid and returns 403 to a third party.
- **Revocation:** only the owner/admin may revoke a token. **Revocation does not issue a replacement automatically**; the owner/admin must **request another token manually**. Until then, the image cannot be shared with third parties, although owner/admin sessions can still view it directly.
- Share URL: public image = `/i/<link>`; private image = `/i/<link>?token=<token>`. Placing the token in the URL creates logging/referrer exposure and is an accepted design decision.

**Frontend behavior:**
- The detail page for the owner/admin shows the current token with copy support and the share URL; a TTL selector (default 1h, range 10min–72h, values centralized in `constants.ts`); and explicit “Generate/reissue token” and “Revoke token” buttons. Nothing happens automatically.
- A third party opens the share URL and the backend validates `?token=` directly; no frontend session is involved.
- Lists/details render the owner's private images through same-origin session authorization without a token.
- UI copy distinguishes 403 (not authorized, or token invalid/expired) from 404 (token revoked).
- Hooks: `useImageAccessToken(id)` retrieves/displays the current token and generates/reissues one by sending the selected TTL with `POST .../tokens`; `useRevokeAccessToken(id)` revokes it.

## Pluggable CAPTCHA (Administrator-Selected, Default: None)

- `captcha_provider` belongs to `none` (default), `recaptcha`, `turnstile`, or `geetest_v4`; an administrator configures it through `/admin/configs`.
- At startup, probe public `GET /public/config` to obtain `captcha_provider` and that provider's client key. With `provider=none`, **load no external script** (satisfying [07 Local External Resources](07-production-standards.md#local-external-resources)).
- When enabled, embed `<Captcha>` in the login and registration pages, rendered by provider branch. `useCaptcha()` returns that provider's verification payload for form submission.
- Normalize errors through `2009` (verification failed) and `1004` (service unavailable), with copy supplied by i18n.

**Provider comparison:**

| Dimension | reCAPTCHA v3 | Cloudflare Turnstile | Geetest CAPTCHA v4 |
|---|---|---|---|
| Client script | `google.com/recaptcha/api.js?render=<site_key>` (or the recaptcha.net mirror) | `challenges.cloudflare.com/turnstile/v0/api.js` | `gcaptcha4.js` under `static.geetest.com` |
| Client public key | site_key | site_key | captcha_id |
| Evidence call | `grecaptcha.execute(siteKey,{action})` -> token | `turnstile.render(el,{sitekey,action,callback})` -> token | `initGeetest4({captcha_id,product})` -> `{lot_number,captcha_output,pass_token,gen_time}` |
| Submission payload | `token` + `action` | `token` | `lot_number` + `captcha_output` + `pass_token` + `gen_time` |
| Server verification | POST `…/api/siteverify` (`secret` + `response` + `remoteip`) | POST `challenges.cloudflare.com/turnstile/v0/siteverify` (`secret` + `response`) | POST `gcaptcha4.geetest.com/verify` (`captcha_id` + four parameters, with an HMAC signature using `captcha_key`) |
| Acceptance | `success` & `score≥threshold` & `action` & `hostname` | `success` | `result=='success'` |
| Availability in China | Poor | Good | Good (domestic provider) |

> **Payload envelope (aligned with backend `CaptchaPayload`; see [API.md](../../API.md) §3):** embed `captcha: { provider, token, action, lot_number, captcha_output, pass_token, gen_time }` in the login/registration request body. Populate only the fields for the current `provider`: recaptcha -> `token` + `action`; turnstile -> `token`; geetest_v4 -> the four parameters. A mismatched provider is rejected. For recaptcha, `action` must be exactly **`"login"` or `"register"`**, matching the server's `ExpectedAction`; `auth_service.go` injects the value separately for login and registration. Error codes `2009` and `1004` are provider-independent; the ErrRecaptcha* names are retained, but their numeric codes are correct.

## Dashboard Data Sources (Explicit)

`/dashboard` has no dedicated endpoint and combines two queries:
- **Personal statistics + quota** -> `useProfile` (`GET /user/profile`, including `storage_percent` and `image_count`).
- **Recent images** -> the first page from `useImages` (limited to the first several records).

Use `auth-store` and `/auth/me` only for **identity and `role`** (guards and the header username), not as display-data sources. `storage_used` appears in three places; consistently use `useProfile` as the source of truth to avoid ambiguity.

## Backend Capability Alignment

Every feature follows the endpoints, fields, pagination, and error codes in `docs/API.md`. Capabilities not covered by the backend, including a public gallery, categories/tags, administrative image review, and user deletion, are outside this frontend's scope; see [09 Explicit Exclusions](09-decisions-and-scope.md#explicit-exclusions-yagni).

**Known backend limitation:**
- **Notifications have no pagination or upper bound** (`GET /notifications` accepts no pagination parameters; see API.md §7). Render the returned list directly, using frontend scrolling/truncation for large result sets. If backend pagination is added later, switch to `useInfiniteQuery`.

---

<- [02 Architecture](02-architecture.md) · [Index](./README.md) · Next: [04 Theme and UI](04-theme-and-ui.md)
