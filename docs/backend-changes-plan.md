# Backend Adjustment Plan (Design Only, No Implementation)

> [!WARNING]
> **Archived design record.** This document captures a pre-V2 implementation
> plan from June 18, 2026. The described token and CAPTCHA work has since been
> implemented or superseded. Use the current source, API reference, and release
> notes as the authoritative behavior.

- **Date:** June 18, 2026
- **Status:** Design; pending implementation at the time of writing
- **Scope:** List the changes required in the Go backend (`backend/`) to support
  two established frontend rules documented in
  `docs/design/frontend-architecture/`.
- **Principle:** This document describes what to change, where to change it, and
  why. It does not contain implementation code.

---

## 1. Private Images: Unified Token Model

### Previous State (Did Not Meet the Target)

- `internal/service/image_service.go`:
  `GenerateAccessToken/ListTokens/RevokeAccessToken` implemented a
  **multiple-token model with expiration**.
- `internal/handler/public_handler.go::ServeImage`: a private image request
  without a token returned `4010` (401), with **no owner/admin session bypass**;
  access with a revoked token had no `404` semantics.
- `internal/handler/image_handler.go::Get`: the `Image` response did not include
  the current token.

### Target Rules Used by the Frontend Design

- Each private image has **one unified token**, and the token's characters are
  **immutable**.
- **TTL:** 1h (3,600,000 ms) by default, validated by the backend in
  **milliseconds**. The owner/admin may choose a value when issuing it, within
  **600,000 ms to 259,200,000 ms**.
- **Revocation:** only the owner/admin may revoke a token. Revocation must **not
  automatically reissue** a token; the owner/admin must explicitly request a new
  one. Until then, the image remains permanently unshareable with third parties.
- **Direct owner/admin access through a same-origin session:** authorize
  automatically, so `/i/` does not require a token. The API returns the current
  unified token for display.
- Third parties: missing, incorrect, or expired token -> **403**; revoked token
  -> **404**.

### Required Changes

**1) Model / repository** (`model/image_access_token.go`,
`repository/image_access_token_repo.go`)

- Enforce at most one active token per image. An active token is neither revoked
  nor expired. Token fields include the string, `expires_at`, `revoked_at`, and
  `status`.
- Add or adjust `FindActiveByImageID(imageID)`, `Issue(imageID, ttlMs)` (invalidate
  an existing active token before issuing another to preserve one unified active
  token), `Revoke(imageID)`, and `Validate(imageID, token)`. `Validate` returns
  `valid`, `expired`, `revoked`, or `not_found`.

**2) Service layer** (`service/image_service.go`)

- Replace the multiple-token suite with
  `IssueAccessToken(userID, imageID, ttlMs)` (including owner/admin validation)
  and `RevokeAccessToken(userID, imageID)`.
- The handler supplies TTL. The service clamps it to `[600000, 259200000]`, with
  `3600000` as the default.
- Remove every automatic reissue path. Issuance must be an explicit action.

**3) Direct image URL** (`handler/public_handler.go::ServeImage`)

- In the private-image branch, first perform **optional session parsing**. Read
  `__Host-session_token` / Bearer by reusing the query logic from `middleware`,
  but do not require authentication. Allow access immediately when the session
  belongs to the owner or an admin.
- Otherwise validate the token and branch on the `Validate` result: allow
  `valid` with `no-store`; return **403** for `expired` / `not_found`; return
  **404** for `revoked`.
- A missing token also maps to **403**, replacing the previous `4010` behavior.

**4) Image details** (`handler/image_handler.go::Get`)

- When the requester is the owner/admin, add `access_token` (the plaintext
  current unified token, returned only by this endpoint) and `token_expires_at`
  to the response.

**5) Error codes** (`internal/pkg/errcode/errcode.go`)

- Add `4037 Private image token is invalid or expired` (403).
- Add `4042 Private image token has been revoked` (404).
- The `/i/` missing-token case uses `4037` instead of the previous `4010`.

**6) Configuration** (`model/system_config.go` + `/admin/configs`)

- Add `private_token_ttl_default_ms`, defaulting to `3600000`. An admin may
  configure it, and the service clamps it again to the fixed bounds.
- Keep the `600000` / `259200000` bounds as **code constants**, not configuration,
  so configuration changes cannot violate the rule.

**7) Update `docs/API.md`:** document private access semantics for `/i/:link`
(owner/admin bypass and 403/404), change the `POST /images/:id/tokens` input to
`ttl_ms`, and add `access_token` to the `GET /images/:id` response.

---

## 2. CAPTCHA: Three Pluggable Providers

### Previous State (Did Not Meet the Target)

- Only `internal/service/recaptcha.go` (reCAPTCHA v3) existed, injected into
  `auth_service` through the `recaptchaVerifier` interface.
- `config.go::RecaptchaConfig` contained only reCAPTCHA fields;
  `public_config_service.go` returned only `recaptcha_enabled` /
  `recaptcha_site_key`.
- `LoginInput` / `RegisterInput` contained only `recaptcha_token` /
  `recaptcha_action`.

### Target Rules

- `captcha_provider` is one of `none` (default), `recaptcha`, `turnstile`, or
  `geetest_v4`, configured by an administrator through `/admin/configs`.
- With `none`, the backend performs **no validation** and the frontend loads no
  script.
- Login and registration accept the payload for the selected provider, and the
  backend calls that provider's validator.

### Required Changes

**1) Configuration** (`config.go`)

- Replace `RecaptchaConfig` with `CaptchaConfig`. Add `Provider string`,
  Turnstile (`SiteKey` / `Secret`), and GeeTest (`CaptchaID` / `CaptchaKey`),
  while retaining the reCAPTCHA fields.
- Default to `Provider=none`.
- Add environment variables `CAPTCHA_PROVIDER`,
  `TURNSTILE_SITE_KEY` / `TURNSTILE_SECRET`, and
  `GEETEST_CAPTCHA_ID` / `GEETEST_CAPTCHA_KEY`, while retaining compatibility
  with the old `RECAPTCHA_*` variables.

**2) Validation abstraction and implementations** (`service/`)

- Extract a `CaptchaVerifier` interface:
  `Verify(ctx, payload, remoteIP, requestHost) *errcode.AppError`.
- Make `recaptcha.go` implement that interface without changing its behavior.
- Add `turnstile.go`: POST to
  `https://challenges.cloudflare.com/turnstile/v0/siteverify` with form fields
  `secret` + `response` (+ `remoteip`), and check `success`. Handle timeouts and
  failures according to `FailClosed`.
- Add `geetest_v4.go`: use `lot_number` + `captcha_output` + `pass_token` +
  `gen_time` + `captcha_id`, sign with `captcha_key` using HMAC-SHA256, POST to
  `https://gcaptcha4.geetest.com/verify`, and require `result=="success"`.
- Add a `NewCaptchaVerifier(cfg)` factory that returns the selected provider
  implementation, or nil for `none`.

**3) Auth service / handler** (`service/auth_service.go`,
`handler/auth_handler.go`)

- Inject `CaptchaVerifier` instead of `recaptchaVerifier`.
- Extend `LoginInput` / `RegisterInput` with a generic CAPTCHA payload:
  `captcha_provider` plus either `{recaptcha_token, recaptcha_action}`,
  `{turnstile_token}`, or
  `{lot_number, captcha_output, pass_token, gen_time}`. A nested `captcha` object
  selected by provider is recommended.
- `Login` / `Register` validate the fields for the **configured provider** and
  skip validation when `provider=none`. Reject a provider sent by the frontend
  when it does not match the configured provider.

**4) Public configuration** (`service/public_config_service.go`,
`handler/public_handler.go::GetConfig`)

- Return `captcha_provider` plus the client public key for that provider:
  `site_key` for reCAPTCHA/Turnstile or `captcha_id` for GeeTest. Return an empty
  public key when `provider=none`.

**5) Rate limiting and errors**

- Reuse the existing login rate limit. Keep `2009` (validation failed) and
  `1004` (service unavailable) unchanged.
- Count validation failures toward the rate limit to reduce brute-force abuse.

**6) Update `docs/API.md`:** document the new `/public/config` fields, the
polymorphic login/registration CAPTCHA payloads, and the new CAPTCHA settings in
`/admin/configs`.

---

## 3. Database Migration (AutoMigrate Handles Schema Changes; Data Still Needs Attention)

- `image_access_tokens`: add `revoked_at` and adjust the uniqueness constraint
  to one active token per image. Existing multiple-token data needs a cleanup
  script that either retains the newest active token or revokes all tokens.
- `system_configs`: add `private_token_ttl_default_ms`, `captcha_provider`, and
  records for each provider key.
- In addition to `cmd/server/main.go::AutoMigrate`, provide a **data migration
  script** that cleans existing tokens and writes default configuration values.

## 4. Out of Scope

- Implementation code, unit tests, and CI changes, which were to be scheduled
  after design approval.
- Frontend implementation, already defined throughout `frontend-architecture/`.

## 5. Risks and Trade-offs

- **Private-image owner/admin bypass:** the public route `/i/:link` needs
  optional session parsing. Do not place the route behind mandatory
  authentication, because that would break third-party token access. Attempt to
  parse the session and bypass token validation only for the owner/admin.
- **Millisecond TTL validation:** Go uses nanoseconds internally. Store and
  compare consistently using `UnixMilli` to avoid precision errors.
- **GeeTest signing algorithm:** follow its official v4 documentation exactly,
  including signing-string order and HMAC-SHA256. Validate the implementation
  against the official demo before use.
- **Availability in China:** reCAPTCHA remains unreliable in mainland China even
  through the `recaptcha.net` mirror. For the primary audience, prefer
  `turnstile` or `geetest_v4`.
