# Configuration Boundaries

summeRain uses separate configuration sources for deployment settings, image
protocol settings, and administrator-managed business settings. A value must
have one authoritative source; the administrator API must not become a second
deployment control plane.

## Configuration Sources

### Environment

The deployment environment supplies startup-only settings such as credentials,
storage paths, connection pools, resource limits, worker capacity, and request
limits. These values are loaded and validated during process startup. Changes
require a restart and are not exposed through the administrator configuration
API.

Secrets are never printed in startup configuration logs. Non-sensitive startup
values may be summarized for operational diagnostics.

### Image Recipe File

The server owns the fixed V2 image recipe. It is a server-side versioned file,
not a database record and not an administrator setting. The recipe defines the
pipeline version, recipe version, accepted source constraints, and fixed
variant geometry. The server validates it before accepting traffic.

The recipe is not shown by `/api/v1/admin/configs` or the administrator UI.
`GET /api/v1/uploads/recipe` may expose the client-required portion so the
frontend can produce a manifest that the backend will validate again.

### Database System Config

The `system_configs` table stores business settings that are intentionally
editable through the existing administrator API, including site language,
CAPTCHA settings, watermark settings, R2 settings, and the private-image token
TTL.

`AdminService.UpdateConfigs` uses an explicit allowlist. Unknown keys and
deployment/protocol keys are rejected before any database transaction starts.

## API Contract

The existing endpoints remain authoritative:

- `GET /api/v1/admin/configs` reads administrator-managed business settings.
- `PATCH /api/v1/admin/configs` updates only allowlisted business settings.
- `GET /api/v1/uploads/recipe` returns the client-required fixed recipe data.

Startup-only environment values and the complete server recipe must not be
added to the writable administrator configuration set.

## Change Rules

- Changing an environment value requires a restart and a deployment change.
- Changing the fixed image recipe requires a recipe-version change and a
  compatible backend/frontend release.
- Changing a database business setting uses the existing batch update API and
  is audited by the normal administrator operation path.
- Backend validation remains the final security boundary; frontend values are
  hints and must never increase a server limit.
