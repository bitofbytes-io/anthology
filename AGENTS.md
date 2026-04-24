# Repository Guidelines

## Stack overview
Anthology is a two-tier catalogue: a Go 1.25 API (under `cmd/api` + `internal/`) fronted by an Angular 21 Material UI (`web/`). Recent work adds metadata search (Google Books), CSV imports, and cover thumbnails that all flow through the Add Item page so validation and enrichment behave consistently. A new shelves module models real-world shelves with photo-backed layouts so items can be placed into slots and surfaced in the UI.

## Project Structure & Module Organization
- `cmd/api`: Go entrypoint; wire up config, repositories, chi router, middleware, and HTTP handlers.
- `internal/`: shared Go packages (domain logic, importer, catalog lookups, services, transport, config). Shelf layout + placement logic now lives in `internal/shelves`.
- `migrations/`: Postgres DDL managed by Goose. `0001_baseline.sql` captures the current schema.
- `web/`: Angular workspace (feature modules under `web/src/app`, Material theme in `web/src/styles.scss`, runtime config in `web/src/assets/runtime-config.js`). Shelf management views live under `web/src/app/pages/shelves`.
- `Docker/`: split Dockerfiles (`Dockerfile.api`, `Dockerfile.ui`) for the independently deployable API/UI containers.
- `docs/architecture/` & `docs/planning/`: architecture diagrams, startup flows, Material guidelines, and roadmap notes.

## Build, Test, and Development Commands
- `go run ./cmd/api` — boots the API using the current env vars; requires Postgres (`DATA_STORE=postgres` + `DATABASE_URL`) and applies Goose migrations on startup.
- `go test ./...` — Go unit tests (catalog lookups, importer, services, handlers).
- `cd web && npm install` — install Angular deps (Material, CLI, test runners).
- `cd web && npm start` — Angular dev server on `http://localhost:4200` proxying to the API URL defined in the meta tag/runtime config.
- `cd web && npm test -- --watch=false` and `npm run lint` — Jasmine/Karma suite plus ESLint.
- `make local` — convenience target to boot the API and Angular dev server together for end-to-end checks.

## Coding Style & Naming Conventions
- Go: auto-format with `gofmt`, keep imports sorted, use short receiver names, and follow package boundaries like `internal/items`. Exported types mirror the `ItemService`/`ItemRepository` style. Logging uses `slog`.
- Angular: 2-space indentation, kebab-case filenames (`items-page.component.ts`), standalone components, and SCSS scoped per component. Stick to Material 3 tokens defined in `styles.scss`. Keep environment variables in screaming snake case (e.g., `GOOGLE_BOOKS_API_KEY`).

## Configuration & Security
- Primary env vars: `DATA_STORE`, `DATABASE_URL`, `PORT`, `LOG_LEVEL`, `ALLOWED_ORIGINS`, `APP_ENV`, `GOOGLE_BOOKS_API_KEY`, `AUTH_GOOGLE_CLIENT_ID`, `AUTH_GOOGLE_CLIENT_SECRET`, `AUTH_GOOGLE_REDIRECT_URL`, `AUTH_GOOGLE_ALLOWED_DOMAINS`, `AUTH_GOOGLE_ALLOWED_EMAILS`, `FRONTEND_URL`. `_FILE` variants are respected (defaults point to `/run/secrets/anthology_*`, including `anthology_google_books_api_key`, `anthology_google_client_id`, and `anthology_google_client_secret`). `GOOGLE_BOOKS_API_KEY` is required even in local/dev; set a placeholder when testing.
- The in-memory store has been removed; `DATA_STORE=postgres` is required and expects Goose migrations to be applied (uses the `sqlx` repo implementation).
- CORS defaults allow `http://localhost:4200`/`8080`; override via `ALLOWED_ORIGINS`.
- OAuth is required in all environments (configure Google OAuth client ID/secret plus an allowlist) and relies on Postgres for sessions.
- Never commit secrets; rely on `.env` files locally. Docker secrets include `anthology_database_url`, `anthology_google_books_api_key`, `anthology_google_client_id`, and `anthology_google_client_secret`, which map to the `_FILE` envs.

## Feature behavior notes
- Reading status supports `none`/`want_to_read`/`reading`/`read`; defaults to `none`. Filtering by `status` with no type filter applies only to books (non-books remain visible when status = `none`, and are excluded for the other statuses). `read` requires `readAt`; `reading` enforces non-negative `currentPage` capped by `pageCount` when provided.
- Shelf APIs (`/api/shelves`) expose list/create/get, layout updates, and slot assignment/removal. Layout updates return displaced items; item placements update the cached `shelfPlacement` on items for list/grid views.

## Testing & Validation Guidelines
- Backend tests live next to their code (`*_test.go`); cover validation, repository behaviour, importer edge cases, catalog lookups, shelf layout validation, and displacement/placement flows.
- Frontend specs (`*.spec.ts`) mirror component paths, covering search flow, manual entry, CSV imports, and UI copy.
- Run `go test ./...`, `npm test -- --watch=false`, and `npm run lint` before every PR. Hook `githooks/pre-commit` into `.git/hooks` to enforce `golangci-lint run ./...` plus `npm run lint` unless `SKIP_PRECOMMIT_LINT=1` is set.
- Validate UI work in the running Angular app when feasible: use Playwright automation by default for navigation/capture, grab at least one screenshot (include scrolled states if relevant) from `http://localhost:4200`, and log any console or network errors.
- Before running Playwright/browser verification, confirm with the user that the local server is running (`make local`, or API/UI started separately). If not running, ask the user to start it first.
- If browser automation needs cookies + localStorage (not just the API cookie), prefer capturing Playwright `storageState` via `node scripts/auth-capture.js` and storing it under `./.auth/<appName>.json`.
  - If the state file is missing/expired, ask the user to run `make auth-capture` (or `node scripts/auth-capture.js`) after logging in manually.

## Commit & Pull Request Guidelines
- Keep commits short, imperative, and scoped (e.g., “Add Google OAuth login”). Reference issues in the body when helpful.
- PRs must include a change summary, manual test notes, confirmation that both `go test` and `npm test`/`npm run lint` were run, and screenshots or GIFs for UI changes. Mention deployment/migration steps if applicable.

## Deployment Notes
- Docker images are split: API (`Docker/Dockerfile.api`) and UI (`Docker/Dockerfile.ui`). Makefile targets (`docker-build-*`, `docker-push-*`, `docker-buildx-*`) wrap builds/pushes.
- UI container rewrites `assets/runtime-config.js` from `NG_APP_API_URL` at startup so you can repoint environments without rebuilding Angular assets.
- Apply Goose migrations before booting the Postgres-backed API (or let the API run them on startup), and ensure services load secrets through env vars or Swarm/Stack secret mounts.

## Local Auth
Local dev runs without auth unless OAuth is configured. To exercise OAuth locally, use Postgres plus the Google OAuth env vars and keep `APP_ENV=development` so cookies stay non-secure.
- For local agent/browser verification, do not add auth bypass routes. Reuse a saved Playwright `storageState` captured after manual login:
  - Capture/refresh: `make auth-capture`
  - Saved state lives in `./.auth/<appName>.json` (gitignored, treat as secret)

## Workspace Bootstrap
- If `local.mk` is missing in a new workspace/worktree, place a `local.mk` file in the project root before running `make run` or `make local`.
- Create `local.mk` by either:
  - copying `local.mk.example`, or
  - copying `local.mk` from the primary Anthology repo/workspace.
- If the source `local.mk` location is unclear in a new worktree, ask the user where to copy it from.
