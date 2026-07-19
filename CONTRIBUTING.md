# Contributing Guide

Thank you for your interest in summeRain. This guide explains how to contribute
to the project.

## Development Environment

### Requirements

- Go 1.24 or newer
- Node.js `^20.19.0 || >=22.12.0` for frontend builds; CI uses 24.18.0 LTS
- Docker and Docker Compose, used only for the MySQL, Redis, and imgproxy
  development dependencies
- OpenSSL for generating the local HTTPS certificate

### Start the Backend in WSL

```bash
./scripts/dev-wsl.sh deps-up   # Start MySQL, Redis, and imgproxy; do not build the application image
./scripts/dev-wsl.sh backend   # Run the Go service directly
```

The application listens on `127.0.0.1:18080` by default. Its health endpoint is
`GET http://127.0.0.1:18080/health`.

### Start the Frontend Development Server

```bash
cd frontend
npm ci
cd ..
./scripts/dev-wsl.sh frontend  # https://127.0.0.1:5173 with same-origin proxies for /api and /i
```

### Build the Frontend into the Backend

```bash
cd frontend
npm run build                  # Output: ../backend/web/
```

## Coding Standards

### Go Backend

- Follow [Effective Go](https://go.dev/doc/effective_go), `gofmt`, and `go vet`.
- Include tests for new functionality in `_test.go` files.
- Order imports as follows: standard library, third-party packages, then project
  packages under `github.com/kserksi/summerain/...`.
- Retain the copyright header in every source file:
  `// Copyright 2026 The summeRain Authors`.

Run these checks before committing:

```bash
cd backend
go build ./...
go vet ./...
go test ./...
```

### TypeScript Frontend

- Use the existing React 19, TanStack Query, and shadcn/ui stack.
- Include Vitest coverage for new functionality.
- Follow the existing ESLint configuration.

Run these checks before committing:

```bash
cd frontend
npm run lint
npm run build
npx vitest run
```

## Commit Convention

Use the [Conventional Commits](https://www.conventionalcommits.org/) format:

```text
<type>(<scope>): <description>

[optional body]
```

Common types are `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, and `i18n`.

Examples:

```text
feat(upload): add drag-and-drop reordering
fix(auth): handle expired session on token refresh
i18n(extract): move navbar strings to locale files
```

## Submit a Pull Request

1. Update your local `dev` branch and create a feature branch from it. Set
   `dev` as the pull request target.
2. Confirm that `go build`, `npm run build`, and the relevant tests pass.
3. Explain the purpose and impact of the change in the pull request description.
4. Include screenshots for UI changes.
5. Keep each pull request focused on one concern so it remains easy to review.

## Project Structure

```text
backend/    Go + Gin + GORM (MySQL) + Redis + imgproxy
frontend/   React 19 + Vite + TypeScript + Tailwind + shadcn/ui
docs/       API contract, deployment guide, and architecture records
```

Historical frontend architecture records are indexed in
[docs/design/frontend-architecture/README.md](docs/design/frontend-architecture/README.md).
Content marked as archived is retained only for design traceability.

## Documentation Translations

The English documentation at the repository root is authoritative. Complete
Simplified Chinese and Japanese mirrors live under `translations/zh-CN/` and
`translations/ja-JP/`, preserving the same relative paths as their canonical
pages. When changing an English document, update both translations in the same
change, then refresh the translation source hashes and run the GitBook
documentation verifier.

```bash
bash scripts/update-translation-source-hashes.sh
bash scripts/verify-gitbook-docs.sh
```

## Code of Conduct

By participating in this project, you agree to follow the
[Code of Conduct](CODE_OF_CONDUCT.md). Please be kind and respectful.
