# 05 · Build and Deployment

> [!WARNING]
> **Archived design record.** This page predates the completed V2 frontend and
> may contain obsolete versions, paths, or implementation status.

> Part of: [Frontend Architecture Design (Index)](./README.md)

## Build

`vite build` -> `build.outDir: '../backend/web'`, `build.emptyOutDir: true`, and `base: '/'`.

> **`base` must be `'/'`, not the relative value `'./'`:** BrowserRouter uses HTML5 history routing. When a deep link such as `/images/123` is refreshed, the backend `NoRoute` handler falls back to `index.html`, and the browser resolves asset paths against the **current document URL**. With a relative base of `./`, `./assets/index.js` would resolve to `/images/assets/index.js`, hit the HTML fallback, and fail to load as a script. Vite's official documentation also warns that a relative base is incompatible with history-route refreshes. This frontend is deployed at the **domain root** (Go serves it from `./web/` on the same origin), so `base: '/'` is correct. A future subpath deployment would require changing both `base` and React Router's `basename` (not needed now).

The production build enables `vite-plugin-sri3` to inject SHA512 `integrity` attributes and emits `assets.manifest.json` (one `{ version, integrity }` entry per artifact; see [07 Production Standards](07-production-standards.md)).

## Production Deployment

A single Go process serves `backend/web/`, `/api`, and `/i` on the same origin, so cookies are naturally same-origin.

## Development Workflow

Vite runs on `:5173` over HTTPS. `server.proxy` forwards `/api` and `/i` to Go at `http://localhost:8080`, preserving same-origin cookies during development.

**Proxy requirements for `__Host-` cookies** (otherwise the browser rejects them and the session cannot persist):
- Set `changeOrigin: true`, and **do not** set `cookieDomainRewrite` or `cookiePathRewrite` (`__Host-` requires no Domain attribute and Path=/; either rewrite would violate the prefix rules).
- Forward the response `Set-Cookie` header unchanged. HTTPS on :5173 satisfies the `Secure` flag; although upstream Go uses HTTP, `c.SetCookie` still emits `Secure`, and the browser accepts it through the HTTPS-facing proxy.
- Cookies are stored for `localhost:5173` (the proxy origin) and are then sent automatically on `/api` and `/i` requests.
- This works in conjunction with the HTTPS prerequisite below.

## ⚠️ HTTPS Is Required (Hard Development Prerequisite)

Cookies with the `__Host-` prefix require `Secure` and HTTPS. On local HTTP, the browser rejects `__Host-session_token`, so the login session cannot persist. The development environment **must** enable HTTPS for Vite (with `@vitejs/plugin-basic-ssl` or mkcert); otherwise, login integration cannot work correctly.

---

<- [04 Theme and UI](04-theme-and-ui.md) · [Index](./README.md) · Next: [06 Testing Strategy](06-testing.md)
