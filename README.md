# summeRain

> A self-hosted image hosting and photo album service with a resource-aware
> upload pipeline, fixed image variants, watermarking, private sharing, and
> compatibility for existing V1 images.

[![CI and Docker](https://github.com/kserksi/summeRain/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/kserksi/summeRain/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/kserksi/summeRain)](https://github.com/kserksi/summeRain/releases)
[![Docker Hub](https://img.shields.io/docker/v/jaykserks/summerain?label=docker)](https://hub.docker.com/r/jaykserks/summerain)
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](https://github.com/kserksi/summeRain/blob/main/LICENSE)

> **Early-release notice:** V2 is still an early release. The upload protocol,
> browser processing pipeline, schema, compatibility behavior, and operational
> defaults may change frequently. Review every changelog, back up MySQL and the image
> volume, and pin an exact release tag or OCI index digest in production.

[Documentation](https://summerain-1.gitbook.io/summerain/) | [Releases](https://github.com/kserksi/summeRain/releases) | [Docker Hub](https://hub.docker.com/r/jaykserks/summerain) | [GHCR](https://github.com/kserksi/summeRain/pkgs/container/summerain)

## Overview

summeRain uses a modular Go monolith and a React single-page application. The
Go service exposes the `/api/v1/*` API, serves images under `/i/*`, and hosts
the compiled frontend from the same origin. MySQL is the authoritative store
for application and job state. Redis provides bounded caching, rate limiting,
view buffering, and device replay protection. imgproxy provides bounded V1
dynamic transformations and the optional V2 watermark stage.

```text
Browser / Android / Windows
            |
            v
      Go + Gin (:8080)
        |-- /api/v1/* --> middleware --> handler --> service --> repository
        |-- /i/*
        |     |-- V2 --> authorization --> fixed local assets
        |     `-- V1 --> stored asset or bounded imgproxy transform
        |-- publish worker --> optional imgproxy watermark --> local publish asset
        `-- /*        --> backend/web (React SPA)
                                      |
                         +------------+------------+
                         |            |            |
                         v            v            v
                     MySQL 8.4     Redis 8    local image volume
```

## V2 Image Pipeline

V2 moves the expensive decode, resize, format conversion, and compression work
to the browser. The backend accepts fixed-recipe, manifest-declared WebP parts, validates them
while streaming to staging storage, and promotes fixed variants without
re-reading the upload solely for validation.

| Asset | Geometry | Quality | Lifecycle and use |
|---|---|---:|---|
| `master` | Original oriented dimensions | 80 | Persisted full-resolution WebP; owner/admin access |
| `gallery` | 400x400 cover crop | 60 | Persisted My Images and dashboard preview |
| `admin` | 120x160 cover crop | 60 | Persisted Image Management preview, displayed at 60x80 |
| `publish_source` | Longest edge at most 2048 px | 80 | Temporary input for server-side publishing |
| `publish` | Derived from `publish_source` | 80 | Persisted sharing asset with the optional watermark |

`publish_source` and session staging data are removed after publishing. The
server applies watermarks only to the final `publish` asset. V2 does not create
arbitrary sizes on first access, which avoids the unbounded thumbnail work that
made V1 vulnerable to hot-resource bursts.

### Upload Behavior

- Static JPEG, PNG, BMP, WebP, and AVIF input is supported.
- Animated images and GIF uploads are not supported.
- Source size, pixel, and concurrency limits follow the server-side recipe and
  configuration; the defaults are listed in
  [Limits and Thresholds](./docs/USAGE.md#7-limits-and-thresholds).
- Upload sessions are resumable and idempotent, with durable status polling and
  a ten-minute client polling deadline after which status can be resumed.
- The high-capacity browser path uses `wasm-vips`; a bounded Canvas/Pica path is
  used when the required browser isolation and memory capabilities are absent.
- Existing V1 images remain readable through their original links and retain
  bounded dynamic transformation support.

## Features

### Images and Storage

- Immutable, content-addressed `master`, `gallery`, and `admin` assets, plus a
  revision-scoped `publish` asset.
- Server-side text watermarks with configurable text, position, opacity, size,
  and color.
- SHA-256 content identity and reference-counted physical file lifecycles.
- Local storage for V2 assets and storage-lineage-aware local/R2 reads for the
  V1 compatibility path.
- Durable outbox delivery for CDN purges and physical local/R2 deletion.
- A streamed account archive during the pending-deletion lock period, without
  assembling the ZIP in application memory.
- Storage quotas, notifications, disk-pressure admission, and bounded cleanup.

### Accounts, Privacy, and Sharing

- A Secure, HttpOnly `__Host-session_token` plus a browser-readable,
  server-backed `__Host-csrf_token` submitted through the CSRF request header.
- Bearer-token bootstrap and sessions for Android and Windows clients.
- One active share token per private image, with a configurable lifetime from
  ten minutes to three days.
- User and administrator roles, session management, audit logs, and a durable
  delayed account-deletion workflow.
- Optional reCAPTCHA v3, Cloudflare Turnstile, and GeeTest v4 CAPTCHA
  integration.
- Immediate public/private origin-alias transitions with durable CDN purge work.

### Web and Operations

- Authentication, dashboard, upload queue, image management, profile,
  notifications, and administration views.
- English, Simplified Chinese, and Japanese interface resources.
- Light and dark themes with reduced-motion support.
- `/health`, `/ready`, and `/metrics` operational endpoints.
- Versioned MySQL migrations with advisory locking and immutable checksums.
- Multi-platform `linux/amd64` and `linux/arm64` images built only by GitHub
  Actions and published to Docker Hub and GHCR.

## Technology

| Layer | Stack |
|---|---|
| Frontend | React 19, React Router 8, Vite 8, TypeScript 6, Tailwind CSS 4, shadcn/ui |
| Frontend data | TanStack Query 5, Zustand 5, React Hook Form 7, Zod 4 |
| Backend | Go, Gin, GORM, MySQL 8.4, Redis 8 |
| Image processing | wasm-vips, Pica, Web APIs, imgproxy 4 |
| Storage | Local V2 filesystem; Cloudflare R2 and compatible path-style S3 endpoints for V1 lineage |
| Testing | Go testing, Vitest, Testing Library, MSW |

Exact CI, service, and browser-processing versions are recorded in
[`requirements.lock`](./requirements.lock). Go and npm dependency graphs are
locked by `backend/go.sum` and `frontend/package-lock.json`. Licenses and
attribution for every adopted library and service are listed in
[Third-Party Software](./docs/THIRD-PARTY.md).

## Quick Start for WSL

### Requirements

- Go 1.24 or newer. CI and container builds currently use Go 1.26.5.
- Node.js `^20.19.0 || >=22.12.0`. CI and container builds currently use
  Node.js 24.18.0 LTS.
- Docker Engine with Docker Compose.
- OpenSSL for the generated local HTTPS certificate.

### 1. Start Development Dependencies

```bash
./scripts/dev-wsl.sh deps-up
```

This starts pinned MySQL, Redis, and imgproxy containers. It does not build the
summeRain application image locally.

| Service | Default address |
|---|---|
| MySQL | `127.0.0.1:13306` |
| Redis | `127.0.0.1:16379` |
| imgproxy | `127.0.0.1:18081` |

The ports can be changed with `SUMMERAIN_DEV_MYSQL_PORT`,
`SUMMERAIN_DEV_REDIS_PORT`, and `SUMMERAIN_DEV_IMGPROXY_PORT`.

### 2. Start the Backend

```bash
./scripts/dev-wsl.sh backend
```

The API listens on `http://127.0.0.1:18080` by default. The first start applies
pending database migrations. Use `SUMMERAIN_DEV_BACKEND_PORT` to change the
port.

### 3. Start the Frontend

```bash
cd frontend
npm ci
cd ..
./scripts/dev-wsl.sh frontend
```

The development server uses `https://127.0.0.1:5173` by default and proxies
`/api/` and `/i/` to the local backend. Accept the generated development
certificate on first use. HTTPS and same-origin proxying are required by the
`__Host-` session cookie.

Development enables COOP/COEP by default for the 50 MP wasm-vips path. Every
third-party script, font, and image must therefore provide compatible CORS or
Cross-Origin-Resource-Policy headers.

### 4. Run Validation

```bash
cd backend
go build ./...
go vet ./...
go test ./...
```

```bash
cd frontend
npm run lint
npm run build
npx vitest run
```

## Production Deployment

Application images are built by GitHub Actions. Production hosts should pull an
exact published tag or OCI index digest and must use `--no-build`.

Create a deployment directory with the Compose file, the environment file, and
the image recipe:

```text
/srv/summerain/
|-- docker-compose.yml    copy of backend/docker-compose.deploy.yml
|-- .env                  copy of backend/.env.example, mode 0600
`-- config/
    `-- image-recipe.json copy of backend/internal/config/image-recipe.json
```

```bash
docker compose --env-file .env pull
docker compose --env-file .env up -d --no-build
```

Edit `.env` before starting: set an exact `DOCKER_IMAGE`, the database
password, the cookie secret, and the imgproxy key/salt. The recipe file is
mounted read-only over the recipe baked into the image; edit it and restart to
change variants, limits, or accepted formats.

Example stable image:

```text
jaykserks/summerain:<version>
```

Published registries:

- Docker Hub: `jaykserks/summerain`
- GHCR: `ghcr.io/kserksi/summerain`

Use the OCI multi-platform index digest when pinning by digest across
architectures.

See [Deployment and Usage](./docs/USAGE.md) for the complete environment,
nginx/CDN, health-check, upgrade, and rollback reference.

## Release Channels

Pushes to `dev` publish development images under `dev` and
`dev-sha-<12-character-commit>`. Pushes to `main` publish stable tags, and only
a stable release moves `latest` and `main`. Exact semantic-version tags are
immutable, and release reruns reconcile Docker Hub and GHCR without overwriting
them.

The full tag contract, including digest-pinning guidance, is documented in
[Release and Tag Management](./docs/RELEASING.md).

## Resource Profile

The default Compose profile applies per-service CPU and memory limits so the
stack can share a host with other workloads.

| Service | CPU limit | Memory limit |
|---|---:|---:|
| Backend | 0.75 CPU | 640 MiB |
| MySQL | 0.75 CPU | 1024 MiB |
| Redis | 0.15 CPU | 192 MiB |
| imgproxy | 0.70 CPU | 512 MiB |

Upload concurrency, connection pools, and disk-pressure thresholds are listed
in [Limits and Thresholds](./docs/USAGE.md#7-limits-and-thresholds), and their
configuration is described in
[Configuration Reference](./docs/USAGE.md#36-v2-upload-and-publication).

These limits are a conservative starting point, not a universal capacity
guarantee. Disk latency, database latency, watermark complexity, and colocated
workloads affect throughput.

## Repository Layout

```text
summeRain/
|-- backend/
|   |-- cmd/server/                 application entry point
|   |-- internal/                   handlers, services, repositories, workers
|   |-- config/                    deployment image recipe override
|   |-- migrations/                 schema migration snapshots and reference
|   `-- web/                        generated frontend build output
|-- frontend/
|   |-- src/features/               domain-oriented application features
|   |-- src/components/             shared UI and layout
|   |-- src/lib/                    API, CSRF, errors, and utilities
|   |-- src/store/                  user and theme state
|   `-- src/i18n/                   English, Chinese, and Japanese resources
|-- docs/                           API, operations, releases, and architecture
|-- translations/                   Simplified Chinese and Japanese documentation mirrors
|-- scripts/                        development and repository verification
|-- .gitbook.yaml                   GitBook Git Sync configuration
`-- SUMMARY.md                      GitBook navigation
```

Backend requests follow a single boundary: `request -> middleware -> handler -> service -> repository -> MySQL / Redis`, with the service layer reaching the filesystem, R2/S3, and imgproxy.

## Documentation

The canonical online documentation is published at
[summerain-1.gitbook.io/summerain](https://summerain-1.gitbook.io/summerain/).
All tracked project documentation is included in the GitBook navigation source
in [`SUMMARY.md`](./SUMMARY.md). The primary references are:

- [Deployment and Usage](./docs/USAGE.md)
- [API Reference](./docs/API.md)
- [Release and Tag Management](./docs/RELEASING.md)
- [Third-Party Software](./docs/THIRD-PARTY.md)
- [Frontend Architecture (archived design records)](./docs/design/frontend-architecture/README.md)
- [Schema Migrations](./backend/migrations/README.md)
- [Contributing](./CONTRIBUTING.md)
- [Security Policy](./SECURITY.md)

This documentation describes the latest stable release. Unreleased changes live
on the `dev` branch and appear in the release notes when published.

English is the authoritative documentation language. Reviewable Simplified
Chinese and Japanese translations mirror the same page paths under
`translations/zh-CN` and `translations/ja-JP`. GitBook publishes them as
language variants of the same documentation site.

The repository is the source of truth for GitBook Git Sync. Manage README and
navigation changes in Git rather than creating duplicate README pages in the
GitBook editor.

## Known Limitations

- V2 accepts static images only; animated-image support is planned.
- V2 persists fixed WebP variants and does not expose arbitrary dynamic resize
  combinations.
- V2 does not retain the original encoded source bytes; `master` is a
  full-resolution quality-80 WebP.
- Existing V1 images are not automatically converted or assigned V2 variants.
- Historical R2 migration is intentionally delegated to a separate migration
  tool and is not performed by the main server.
- GeeTest v4 is mutually exclusive with the default cross-origin isolation mode;
  see [CAPTCHA configuration](./docs/USAGE.md#38-captcha-pluggable-and-optional).

## Contributing and Security

Read [CONTRIBUTING.md](./CONTRIBUTING.md) before opening a pull request and
follow [CODE_OF_CONDUCT.md](./CODE_OF_CONDUCT.md) in project spaces. Report
security issues through the private process in [SECURITY.md](./SECURITY.md), not
through a public issue. GitHub provides a private
[security advisory form](https://github.com/kserksi/summeRain/security/advisories/new)
for this repository.

## License

Copyright 2026 kserksi

Licensed under the [Apache License 2.0](https://github.com/kserksi/summeRain/blob/main/LICENSE).
