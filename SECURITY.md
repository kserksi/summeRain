# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability, **do not open a public issue**.

Report it privately through
[GitHub Security Advisories](https://github.com/kserksi/summerain/security/advisories/new).
We will acknowledge the report and assess its impact as soon as possible.

Please include as much of the following information as possible:

- A clear description of the issue and its impact
- Reproduction steps, preferably with a minimal reproducible example
- Affected versions
- A suggested remediation, if available

## Response Process

1. We acknowledge the report within 72 hours.
2. We assess the severity and verify the vulnerability.
3. We develop a fix, using a private branch when warranted by the severity.
4. We publish a corrected release and credit the reporter publicly with their
   consent.

## Supported Versions

Only the latest release on the `main` branch receives security fixes. Older
versions do not receive separate security patches.

## Deployment Security

See [docs/USAGE.md](docs/USAGE.md) for the complete guidance. Key requirements
include:

- Set strong, random values for `COOKIE_SECRET`, `IMGPROXY_KEY`, and
  `IMGPROXY_SALT` in production.
- Cookies with the `__Host-` prefix require HTTPS and a same-origin deployment.
  Local development must use a self-signed certificate.
- Keep the MySQL, Redis, and imgproxy containers on the private network without
  exposing their ports publicly.
