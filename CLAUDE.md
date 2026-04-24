# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Development Commands

### Go API (from repository root)
- `make api-run` — Run API (requires local.mk configuration)
- `make api-test` — Run Go unit tests (`go test ./...`)
- `make fmt` — Format Go code with gofmt
- `make tidy` — Run `go mod tidy`
- `go run ./cmd/api` — Direct API start (configure via env vars)

### Angular UI (from `web/` directory)
- `npm start` — Dev server on http://localhost:4200
- `npm test -- --watch=false` — Run Jasmine/Karma tests once
- `npm run lint` — ESLint checks
- `npm run build` — Production build to `web/dist/web/browser`

### Combined Development
- `make local` — Run API and Angular dev server concurrently

### Docker
- `make docker-build` — Build both API and UI images
- `make docker-buildx` — Multi-arch build and push

## Architecture

Two-tier catalogue: Go 1.25 API + Angular 21 Material UI, deployed as independent containers.

### Go Backend (`cmd/api`, `internal/`)
- **chi router** with middleware for request IDs, timeouts, structured logging (slog)
- **internal/items** — Domain logic, validation, Postgres repository
- **internal/catalog** — Google Books API client for metadata lookups
- **internal/importer** — CSV import with duplicate detection and auto-enrichment
- **internal/http** — Handlers, router definitions, request/response helpers
- **internal/config** — Environment-driven configuration
- **internal/shelves** — Shelf/collection management
- **internal/auth** — Google OAuth authentication and session management
- **migrations/** — Goose-managed SQL; `0001_baseline.sql` captures the current schema and runs on startup

### Angular Frontend (`web/src/app/`)
- Standalone components with Angular Material 3
- **pages/** — Feature pages (items, add-item, login)
- **services/** — API communication, state management
- **components/** — Shared UI components
- **models/** — TypeScript interfaces matching Go types

### Key Request Flows
1. **Auth**: Google OAuth mints HttpOnly session cookies (required in all environments)
2. **Metadata Search**: `GET /api/catalog/lookup` proxies Google Books queries
3. **CSV Import**: `POST /api/items/import` processes uploads through importer with enrichment

## Configuration

Key environment variables (see `docs/LOCAL_DEVELOPMENT.md` for setup):
- `DATA_STORE` — `postgres` (required)
- `DATABASE_URL` — Postgres connection string (required)
- `GOOGLE_BOOKS_API_KEY` — For metadata lookups (required)
- `AUTH_GOOGLE_CLIENT_ID` / `AUTH_GOOGLE_CLIENT_SECRET` — Google OAuth credentials (required)
- `AUTH_GOOGLE_ALLOWED_DOMAINS` / `AUTH_GOOGLE_ALLOWED_EMAILS` — OAuth allowlist (required)
- `AUTH_GOOGLE_REDIRECT_URL` — OAuth callback (defaults to localhost)
- `FRONTEND_URL` — UI URL used for OAuth redirects
- `APP_ENV` — `development` or `production` (defaults to production for safety)
- `ALLOWED_ORIGINS` — CORS origins (default: localhost:4200,8080)
- `PORT` — API port (default: 8080)

## Code Style

- **Go**: gofmt, sorted imports, short receiver names, slog for logging
- **Angular**: 2-space indent, kebab-case files, standalone components, SCSS scoped per component
- **Both**: Run `golangci-lint run ./...` and `npm run lint` before commits

The `githooks/pre-commit` hook enforces lint checks. Install with:
```bash
cp githooks/pre-commit .git/hooks/pre-commit
```

## Testing

- Go tests are colocated with code (`*_test.go`)
- Angular specs mirror component paths (`*.spec.ts`)
- Run both suites before PRs: `make api-test` and `cd web && npm test -- --watch=false`
