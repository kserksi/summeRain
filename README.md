# summeRain

> A self-hosted image hosting and photo album service with a resource-aware
> upload pipeline, fixed image variants, watermarking, private sharing, and
> compatibility for existing V1 images.

[![CI and Docker](https://github.com/kserksi/summeRain/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/kserksi/summeRain/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/kserksi/summeRain)](https://github.com/kserksi/summeRain/releases)
[![Docker Hub](https://img.shields.io/docker/v/jaykserks/summerain?label=docker)](https://hub.docker.com/r/jaykserks/summerain)
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](./LICENSE)

> **Early-release notice:** V2 is still an early release. The upload protocol,
> browser processing pipeline, schema, compatibility behavior, and operational
> defaults may change frequently. Review every changelog, back up MySQL and the image
> volume, and pin an exact release tag or OCI index digest in production.

[Documentation](./SUMMARY.md) | [V2.0.0 release notes](./docs/releases/v2.0.0.md) | [Docker Hub](https://hub.docker.com/r/jaykserks/summerain) | [GHCR](https://github.com/kserksi/summeRain/pkgs/container/summerain)

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
- Animated images and GIF uploads are not supported in V2.0.0.
- Source files are limited to 15 MiB and 50 megapixels.
- Browser processing concurrency is 1 and upload-pipeline concurrency is 2.
- Part uploads start at concurrency 2 and may adapt to 3 on capable clients.
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
- Optional reCAPTCHA v3 and Cloudflare Turnstile integration. GeeTest v4 is
  available only when cross-origin isolation is explicitly disabled.
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
locked by `backend/go.sum` and `frontend/package-lock.json`.

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

```bash
cp backend/.env.example backend/.env
chmod 0600 backend/.env
# Edit backend/.env and set an exact DOCKER_IMAGE, database password,
# cookie secret, and imgproxy key/salt.

docker compose --env-file backend/.env \
  -f backend/docker-compose.deploy.yml pull

docker compose --env-file backend/.env \
  -f backend/docker-compose.deploy.yml up -d --no-build
```

Example stable image:

```text
jaykserks/summerain:2.0.0
```

Published registries:

- Docker Hub: `jaykserks/summerain`
- GHCR: `ghcr.io/kserksi/summerain`

Use the OCI multi-platform index digest when pinning by digest across
architectures. Do not reuse an architecture-specific child-manifest digest on
both `amd64` and `arm64` hosts.

See [Deployment and Usage](./docs/USAGE.md) for the complete environment,
nginx/CDN, health-check, upgrade, and rollback reference.

## Release Channels

- Regular pushes to `main` publish `edge` and `sha-<12-character-commit>`.
- A stable `VERSION` such as `2.0.0` publishes `v2.0.0`, `2.0.0`, `2.0`, `2`,
  `latest`, and the commit tag.
- A prerelease such as `2.1.0-rc.1` publishes exact version tags and the commit
  tag without updating stable aliases.
- Exact semantic-version tags are immutable. Moving aliases remain movable.
- Release reruns reconcile Docker Hub and GHCR by verified manifest digest and
  stop if exact tags conflict.
- The root README is synchronized to Docker Hub after a successful publication.

The full release contract is documented in
[Release and Tag Management](./docs/RELEASING.md).

## Resource Profile

The default Compose profile is designed for a 3-core, 4 GiB host shared with
other services.

| Service | CPU limit | Memory limit |
|---|---:|---:|
| Backend | 0.75 CPU | 640 MiB |
| MySQL | 0.75 CPU | 1024 MiB |
| Redis | 0.15 CPU | 192 MiB |
| imgproxy | 0.70 CPU | 512 MiB |

The backend defaults to eight global and four per-user concurrent part uploads.
MySQL and Redis pools are bounded. New V2 sessions are rejected at the default
80% disk soft limit, and new parts/publish outputs are rejected at the 90% hard
limit. Reduce imgproxy and watermark workers from two to one when the host is
under sustained contention.

These limits are a conservative starting point, not a universal capacity
guarantee. Disk latency, database latency, watermark complexity, and colocated
workloads affect throughput.

## Repository Layout

```text
summeRain/
|-- backend/
|   |-- cmd/server/                 application entry point
|   |-- internal/                   handlers, services, repositories, workers
|   |-- migrations/                 versioned SQL migrations
|   `-- web/                        generated frontend build output
|-- frontend/
|   |-- src/features/               domain-oriented application features
|   |-- src/components/             shared UI and layout
|   |-- src/lib/                    API, CSRF, errors, and utilities
|   |-- src/store/                  user and theme state
|   `-- src/i18n/                   English, Chinese, and Japanese resources
|-- docs/                           API, operations, releases, and architecture
|-- scripts/                        development and repository verification
|-- .gitbook.yaml                   GitBook Git Sync configuration
`-- SUMMARY.md                      GitBook navigation
```

Backend requests follow this boundary:

```text
request -> middleware -> handler -> service -> repository -> MySQL / Redis
                                      `-> filesystem / R2 / imgproxy
```

## Documentation

All tracked project documentation is included in the GitBook navigation in
[`SUMMARY.md`](./SUMMARY.md). The primary references are:

- [Deployment and Usage](./docs/USAGE.md)
- [API Reference](./docs/API.md)
- [Release and Tag Management](./docs/RELEASING.md)
- [Frontend Architecture](./docs/design/frontend-architecture/README.md)
- [Schema Migrations](./backend/migrations/README.md)
- [Contributing](./CONTRIBUTING.md)
- [Security Policy](./SECURITY.md)

The repository is the source of truth for GitBook Git Sync. Manage README and
navigation changes in Git rather than creating duplicate README pages in the
GitBook editor.

## Known Limitations

- V2.0.0 accepts static images only; animated-image support is planned.
- V2 persists fixed WebP variants and does not expose arbitrary dynamic resize
  combinations.
- V2 does not retain the original encoded source bytes; `master` is a
  full-resolution quality-80 WebP.
- Existing V1 images are not automatically converted or assigned V2 variants.
- Historical R2 migration is intentionally delegated to a separate migration
  tool and is not performed by the main server.
- Disabling cross-origin isolation enables GeeTest v4 but removes the
  high-capacity wasm-vips browser path.

## Contributing and Security

Read [CONTRIBUTING.md](./CONTRIBUTING.md) before opening a pull request and
follow [CODE_OF_CONDUCT.md](./CODE_OF_CONDUCT.md) in project spaces. Report
security issues through the private process in [SECURITY.md](./SECURITY.md), not
through a public issue. GitHub provides a private
[security advisory form](https://github.com/kserksi/summeRain/security/advisories/new)
for this repository.

## License

Copyright 2026 The summeRain Authors

Licensed under the [Apache License 2.0](./LICENSE).
