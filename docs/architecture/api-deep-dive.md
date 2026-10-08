# Anthology API deep dive

This document reflects the current API implementation under `cmd/api` and `internal/` as of this commit. It is intended for engineers onboarding to the backend and focuses on endpoints, request flows, data models, validation, and infrastructure concerns.

## High-level shape

* Go 1.26, chi router, `slog` logging.
* Runtime config from env/_FILE (see **Configuration**).
* Requires Postgres (sqlx, embedded Goose migrations).
* Metadata lookups proxy Google Books; CSV import reuses that pipeline.
* Authentication: Google OAuth with an HttpOnly session cookie for API calls.

### Runtime topology

```mermaid
flowchart LR
    subgraph Client["Angular UI / API consumer"]
        A[HTTP requests]
    end
    subgraph API["Anthology API (Go)"]
        R[Router & middleware\nchi + cors + slog]
        H[Handlers\nitems/catalog/shelves/session]
        Svc[Services\nitems/importer/catalog/shelves]
        Repo[Repos\nPostgres]
        GB[Google Books]
        DB[(Postgres)]
    end
    A -->|OAuth + cookie| R --> H --> Svc --> Repo
    Svc -->|ISBN/keyword| GB
    Repo --> DB
```

## Configuration

* `DATA_STORE` (`postgres` required).
* `DATABASE_URL` (`postgres://…`) or `DATABASE_URL_FILE`.
* `PORT`/`HTTP_PORT` (default 8080).
* `ALLOWED_ORIGINS` CSV list; wildcards only allowed in `APP_ENV=development`.
* `LOG_LEVEL` (`debug`, `info`, `warn`, `error`).
* `GOOGLE_BOOKS_API_KEY` (required; `_FILE` supported with default `/run/secrets/anthology_google_books_api_key`).
* `AUTH_GOOGLE_CLIENT_ID` / `AUTH_GOOGLE_CLIENT_SECRET` (required in all environments).
* `AUTH_GOOGLE_ALLOWED_DOMAINS` or `AUTH_GOOGLE_ALLOWED_EMAILS` (allowlist required).
* `AUTH_GOOGLE_REDIRECT_URL` (defaults to `http://localhost:8080/api/auth/google/callback`).
* `FRONTEND_URL` (defaults to `http://localhost:4200`).
* `APP_ENV` (defaults to `production`) toggles cookie `Secure` flag for OAuth cookies.

OAuth sessions are stored in Postgres; Postgres is required for all deployments.

`cmd/api/main.go` loads config, builds logger, connects to Postgres, applies Goose migrations, then binds `http.Server` with sensible timeouts.

## Authentication and sessions

* **OAuth**: `GET /api/auth/google` initiates the flow, and `GET /api/auth/google/callback` creates a user/session and sets `anthology_session`.
* **Session cookie**: `anthology_session` is HttpOnly, SameSite=Lax, Secure outside dev, with a 12h TTL.
* Auth middleware requires a valid session cookie; otherwise 401 with `WWW-Authenticate: Bearer`.
* In `APP_ENV=development`, cookies are non-secure to support localhost during OAuth.

## Endpoints (current)

Base URL: `http://<host>:<port>`. Application endpoints under `/api/*` require the session cookie; `/health` and `/api/health` remain public.

| Method | Path | Description | Handler |
| --- | --- | --- | --- |
| GET | `/health`, `/api/health` | Liveness; returns `{"status":"ok"}`. | inline in router |
| GET | `/api/auth/google` | Initiate Google OAuth. | `OAuthHandler.InitiateGoogle` |
| GET | `/api/auth/google/callback` | Handle OAuth callback, set session. | `OAuthHandler.CallbackGoogle` |
| GET | `/api/session` | Report session status and user (if authenticated). | `SessionHandler.Status` |
| DELETE | `/api/session` | Clear session cookie. | `SessionHandler.Logout` |
| GET | `/api/session/user` | Return current user profile. | `SessionHandler.CurrentUser` |
| GET | `/api/items` | List items with filters (type/status/letter/query/limit). | `ItemHandler.List` |
| GET | `/api/items/histogram` | Letter counts for alphabet rail. | `ItemHandler.Histogram` |
| GET | `/api/items/duplicates` | Check potential duplicates by title/ISBN. | `ItemHandler.Duplicates` |
| POST | `/api/items` | Create item. | `ItemHandler.Create` |
| POST | `/api/items/import` | CSV upload (5 MiB limit) for single-request bulk import (kept for compatibility). | `ItemHandler.ImportCSV` |
| POST | `/api/items/import/preview` | CSV upload (5 MiB limit); read-only review of every row. Saves nothing. | `ItemHandler.PreviewCSVImport` |
| POST | `/api/items/import/commit` | Save reviewed rows exactly as previewed (JSON, 16 MiB limit); no catalog lookups. | `ItemHandler.CommitCSVImport` |
| GET | `/api/items/{id}` | Get item by UUID. | `ItemHandler.Get` |
| PUT | `/api/items/{id}` | Update mutable fields (partial). | `ItemHandler.Update` |
| DELETE | `/api/items/{id}` | Delete item. | `ItemHandler.Delete` |
| GET | `/api/catalog/lookup` | Proxy metadata lookup (currently books only). | `CatalogHandler.Lookup` |
| GET | `/api/shelves` | List shelf summaries. | `ShelfHandler.List` |
| POST | `/api/shelves` | Create shelf with default single-slot layout. | `ShelfHandler.Create` |
| GET | `/api/shelves/{id}` | Get shelf layout + placements. | `ShelfHandler.Get` |
| PUT | `/api/shelves/{id}/layout` | Replace layout; returns displaced items. | `ShelfHandler.UpdateLayout` |
| POST | `/api/shelves/{id}/slots/{slotId}/items` | Assign item to slot. | `ShelfHandler.AssignItem` |
| DELETE | `/api/shelves/{id}/slots/{slotId}/items/{itemId}` | Remove item from slot (unplaced). | `ShelfHandler.RemoveItem` |

### Error contract

* JSON responses with `{"error": "<message>"}` for errors.
* Unknown fields rejected on create/update and import commits (strict JSON decode); payload limited to 8 MiB (16 MiB for import commits).
* CSV upload and preview return 400 on invalid/missing file, 413 on size overflow, 500 on importer errors.
* Import commit returns 400 (`invalid import request: ...`) or 413 before saving anything; per-row problems are reported in the result instead.
* Catalog lookup maps validation to 400, unsupported category to 400, not-found to 404, upstream failure to 502.

## Data model (API surface)

Items (`internal/items.Item`):

```json
{
  "id": "uuid",
  "title": "string",
  "creator": "string",
  "itemType": "book|game|movie|music",
  "releaseYear": 2024,
  "pageCount": 300,
  "currentPage": 120,
  "isbn13": "978...",
  "isbn10": "1234567890",
  "description": "string",
  "coverImage": "https://… or data:<mime>;base64,…",
  "readingStatus": "read|reading|want_to_read|none",
  "readAt": "RFC3339 datetime",
  "notes": "string",
  "createdAt": "RFC3339 datetime",
  "updatedAt": "RFC3339 datetime",
  "shelfPlacement": {
    "shelfId": "uuid",
    "shelfName": "string",
    "slotId": "uuid",
    "rowIndex": 0,
    "colIndex": 1
  }
}
```

Shelf layout (`ShelfWithLayout`):

```json
{
  "shelf": { "id": "uuid", "name": "string", "description": "string", "photoUrl": "string", "createdAt": "...", "updatedAt": "..." },
  "rows": [{ "id": "uuid", "shelfId": "uuid", "rowIndex": 0, "yStartNorm": 0.0, "yEndNorm": 0.5, "columns": [{ "id": "uuid", "shelfRowId": "uuid", "colIndex": 0, "xStartNorm": 0.0, "xEndNorm": 0.5 }] }],
  "slots": [{ "id": "uuid", "shelfId": "uuid", "shelfRowId": "uuid", "shelfColumnId": "uuid", "rowIndex": 0, "colIndex": 0, "xStartNorm": 0.0, "xEndNorm": 0.5, "yStartNorm": 0.0, "yEndNorm": 0.5 }],
  "placements": [{ "item": { /* Item */ }, "placement": { "id": "uuid", "itemId": "uuid", "shelfId": "uuid", "shelfSlotId": "uuid", "createdAt": "..." } }],
  "unplaced": [{ "item": { /* Item */ }, "placement": { "id": "uuid", "itemId": "uuid", "shelfId": "uuid", "shelfSlotId": null, "createdAt": "..." } }]
}
```

CSV import summary (`internal/importer.Summary`):

```json
{ "totalRows": 5, "imported": 3, "skippedDuplicates": [{ "row": 3, "title": "Dune", "identifier": "9780441172719", "reason": "duplicate isbn13" }], "failed": [{ "row": 4, "title": "", "identifier": "999", "error": "ISBN/UPC 999 is not valid" }] }
```

CSV import preview (`internal/importer.Preview`), abbreviated:

```json
{ "totalRows": 3, "ready": 1, "duplicates": 1, "needsMatch": 1, "rows": [
  { "row": 2, "status": "ready", "itemType": "book", "title": "Dune", "identifier": "", "item": { /* create body + createdAt/updatedAt */ }, "keys": ["title:dune"] },
  { "row": 3, "status": "duplicate", "item": { /* ... */ }, "libraryMatches": [{ "field": "title", "itemId": "uuid", "title": "Dune", "creator": "", "itemType": "book", "isbn13": "", "isbn10": "" }] },
  { "row": 4, "status": "needs_match", "identifier": "9780441172719", "problem": "Choose the catalog edition to import for this ISBN.", "candidates": [{ "item": { /* ... */ }, "keys": [], "libraryMatches": [], "csvOverrides": ["creator"] }] }
] }
```

CSV import commit request and result (`internal/importer.CommitRequest`, `CommitResult`):

```json
{ "rows": [{ "row": 2, "item": { /* exactly the previewed item or chosen candidate */ } }] }
{ "added": 1, "skipped": 0, "failed": 0, "interrupted": 0, "unprocessed": 0, "rows": [{ "row": 2, "status": "added", "title": "Dune", "identifier": "", "itemId": "uuid" }] }
```

## Validation rules (service layer)

Items:
* Required: `title`, `itemType`.
* Year/page counts must be positive if provided; `currentPage` >= 0 and cannot exceed `pageCount`.
* Cover image: empty allowed; data URI must be valid base64 and <= 500 KB; URL length <= 4096 chars.
* Reading status only valid for books. Rules:
  * Default: `none`.
  * `read` requires `readAt`.
  * `reading` requires `currentPage` and (if present) must not exceed `pageCount`.
  * `want_to_read` clears read/progress.

CSV importer:
* Requires header columns: `title,creator,itemType,releaseYear,pageCount,isbn13,isbn10,description,coverImage,notes` (case-insensitive).
* Empty rows skipped; per-row errors reported in `failed`.
* Duplicate detection across title/ISBN13/ISBN10 using existing catalog + rows processed in-session.
* Book rows with missing title but ISBN/UPC will call catalog lookup to backfill metadata; otherwise title is required.
* Lookups are capped at 100 per import and share a 30s time budget; rows beyond either limit are reported in `failed` and can be re-imported with a title or in a smaller file.
* Upload capped at 5 MiB (HTTP handler).
* Row numbers are the row in the file (header is row 1), counting blank lines and multi-line quoted values the way a spreadsheet does.

CSV import preview and commit (the flow the UI uses):
* Preview parses the whole file first, then validates and normalizes each row with `items.NormalizeCreateInput` (the same rules `items.Service.Create` applies) without writing. Rows are `ready`, `duplicate` (exact title/ISBN-13/ISBN-10 match with the owner's library or an earlier ready row of the file), or `needs_match` (a parse or validation problem, a failed lookup, or a titleless book whose catalog editions must be chosen). Titleless book rows offer every catalog result (up to 5) instead of the first; CSV values always win over catalog values, and `csvOverrides` lists where they differ. Lookups keep the 100-lookup / 30 s limits.
* Commit is stateless, so any API replica can serve it: the request carries the reviewed items themselves, not a preview ID. It grants nothing beyond `POST /api/items` plus the CSV timestamp columns the import already accepts: the owner is always the signed-in user, every item is re-validated, row numbers must be unique data rows, at most 1000 rows. It never calls the catalog.
* Commit runs inside an import batch (`items.Service.BeginImportBatch`). The batch pins a database connection and takes a session-level PostgreSQL advisory lock for the owner (`pg_try_advisory_lock(0x616e7468, <owner key>)`). Every other item insert for that owner, including ordinary `Create` (which takes the same lock with `pg_try_advisory_xact_lock` in its own transaction), waits until the batch finishes, on every API replica, because the lock lives in the shared database. Waiting is done by retrying: each attempt borrows a pooled connection, proceeds only if the lock was actually granted, and otherwise returns the connection (rolling back first for `Create`) and backs off with jitter (10–250 ms) until the request's context ends. A waiting request therefore doesn't hold a pooled connection. If a lock attempt fails without a clear answer, its connection is closed, ending the database session, so a lock granted at that moment can't leak into the pool. Retrying isn't first-come-first-served: waiters for the same owner may acquire the lock in any order. Under the lock the commit reads the owner's library once and checks each row against it and against rows saved earlier in the request, using the same exact title/ISBN-13/ISBN-10 rule as before. An overlapping commit for the same owner therefore waits, then skips rows the first one saved. Owners don't wait for each other (unless their lock keys collide, which only adds waiting). If the lock is released, the connection is returned to the pool; if not, the connection is closed, which ends the database session and drops the lock.
* Rows are saved one at a time in row order, each as an autocommitted `INSERT … RETURNING` with no separate read-back, and reported as `added`, `skipped`, `failed` (validation error, or PostgreSQL rejected the insert, so nothing was stored), `interrupted` (the save couldn't be confirmed: the connection failed, the request ended mid-save, or the server reported a fatal or connection-level error, so the row may or may not be stored), or `unprocessed` (never attempted: the request ran out of time, a commit waited too long for another import or save for the owner, or an earlier row's save couldn't be confirmed). The commit stops at the first unconfirmed row. Counts always add up to the submitted rows. There is no rollback: rows saved before a failure stay saved.
* Repeats are safe in any order: a repeat waits for the earlier commit and skips what it saved. Item updates (for example renaming an item to match a CSV row) don't take the lock and aren't covered. Stores that can't provide the lock make commits fail with 501 instead of running an unsafe check. While a commit runs (normally a few seconds, at most its 75 s request timeout), manual adds for the same owner wait for it.
* Optional columns match the export format (including `seriesName`, `volumeNumber`, `totalVolumes`); older files without them still import. For rows whose `schemaVersion` is 2 or later (current Anthology exports), a leading `'` added by the exporter before `=`, `+`, `-`, `@`, or tab is stripped; version 1 exports and third-party files are imported verbatim.

CSV exporter:
* Columns are a superset of the import format so exports re-import cleanly.
* Values that start with a formula trigger are prefixed with `'` for spreadsheet safety.
* Covers stored inline as `data:` URIs are omitted (URL covers are kept); they exceed spreadsheet cell limits and push exports past the import upload cap.

Catalog lookup:
* Query trimmed, must be >=3 characters.
* Only `category=book` supported; others return `ErrUnsupportedCategory`.
* ISBN normalization strips non-digits and validates length 10/13 (supports trailing X).
* Publish year parsed from Google Books `publishedDate` via regex; cover URLs forced to https.

Shelves:
* Layout updates require at least one slot; row/col indexes must be non-negative; slot boundaries must be within [0,1] and non-overlapping per key.
* Slot IDs preserved when coordinates refer to existing rows/cols to keep placements stable; removed slots trigger displaced items returned to client and unplaced in persistence.

## Persistence

* Postgres repos (`internal/items/postgres_repository.go`, `internal/shelves/postgres_repository.go`) use `sqlx`:
  * Items: CRUD with lateral join to latest placement (`item_shelf_locations` ordered by created_at).
  * Shelves: transactional upserts for rows/cols/slots; placements stored in `item_shelf_locations`; layout updates delete missing slots/columns/rows and null out placements for removed slots.
* Connection pool defaults: max open 10, max idle 5, conn max lifetime 30m, idle time 5m.

### Schema (migrations)

* `migrations/0001_baseline.sql` captures the current schema as a Goose baseline (built from a schema-only dump).
* Future schema changes live in new Goose migrations (Up/Down) in `migrations/`.

`internal/platform/migrate` embeds migrations and runs `goose.Up` on startup, tracking applied versions in `goose_db_version`.

## Request flows

### Login flow

```mermaid
sequenceDiagram
    participant UI
    participant API as Anthology API
    participant Google
    UI->>API: GET /api/auth/google?redirectTo=/items
    API-->>UI: 302 Redirect to Google
    UI->>Google: OAuth consent
    Google-->>API: GET /api/auth/google/callback?code=...
    API-->>UI: 302 Redirect + Set-Cookie anthology_session
    UI->>API: Subsequent /api/* with cookie
    API-->>UI: 200/… (authorized)
```

### Item creation

```mermaid
sequenceDiagram
    participant UI
    participant API
    participant Items as items.Service
    participant Repo
    UI->>API: POST /api/items {payload}
    API->>Items: Validate & normalize
    Items->>Repo: Create item
    Repo-->>Items: Stored item
    Items-->>API: Item
    API-->>UI: 201 Created + item JSON
```

### CSV import + metadata enrichment

```mermaid
sequenceDiagram
    participant UI
    participant API
    participant Importer as CSVImporter
    participant Items as items.Service
    participant Catalog as catalog.Service
    UI->>API: POST /api/items/import (multipart file)
    API->>Importer: Import(ctx, file)
    Importer->>Items: List existing (for duplicates)
    loop rows
        Importer->>Catalog: Lookup ISBN (if needed)
        Catalog-->>Importer: Metadata or error
        Importer->>Items: Create item
    end
    Importer-->>API: Summary {imported/skipped/failed}
    API-->>UI: 200 OK + summary
```

### CSV import preview and commit

```mermaid
sequenceDiagram
    participant UI
    participant API
    participant Importer as CSVImporter
    participant Items as items.Service
    participant Catalog as catalog.Service
    UI->>API: POST /api/items/import/preview (multipart file)
    API->>Importer: Preview(ctx, file, owner)
    Importer->>Items: List existing (read only)
    loop rows
        Importer->>Catalog: Lookup ISBN (titleless books only)
    end
    Importer-->>API: Preview {rows, statuses, candidates}
    API-->>UI: 200 OK (nothing saved)
    Note over UI: User reviews rows and chooses editions
    UI->>API: POST /api/items/import/commit {rows: [{row, item}]}
    API->>Importer: Commit(ctx, rows, owner)
    Importer->>Items: List existing (recheck duplicates)
    loop selected rows
        Importer->>Items: Create item (no catalog lookup)
    end
    Importer-->>API: CommitResult {per-row outcomes}
    API-->>UI: 200 OK + result
```

### Shelf layout update

```mermaid
sequenceDiagram
    participant UI
    participant API
    participant Shelves as shelves.Service
    participant Repo
    UI->>API: PUT /api/shelves/{id}/layout {slots}
    API->>Shelves: Validate slots, map IDs
    Shelves->>Repo: SaveLayout (tx)
    Repo-->>Shelves: Updated layout persisted
    Shelves->>Repo: GetShelf + placements
    Shelves-->>API: Hydrated layout + displaced items
    API-->>UI: 200 OK
```

## Operational notes

* CORS defaults: `http://localhost:4200,http://localhost:8080`; override via `ALLOWED_ORIGINS`.
* Timeouts: Request timeout middleware 60s (75s for `POST /api/items/import`, `/import/preview`, and `/import/commit`); HTTP server read/write 15s, idle 60s. The CSV routes extend their write deadline to 90s so the response outlives the request timeout; if the import context expires, remaining rows are reported in `failed` and the summary sets `interrupted` (commits report them as `interrupted`/`unprocessed`).
* Logging: `slog` text handler; HTTP middleware logs method/path/status/duration.
* CSV upload size guard at handler level (5 MiB); JSON max 8 MiB, 16 MiB for import commits.
* Postgres is required; local dev should point at a local database.

## How to run locally

```bash
export DATA_STORE=postgres
export DATABASE_URL="postgres://anthology:anthology@localhost:5432/anthology?sslmode=disable"
export PORT=8080
export APP_ENV=development
export GOOGLE_BOOKS_API_KEY="your-key"
go run ./cmd/api
```

Health: `curl http://localhost:8080/health` or `curl http://localhost:8080/api/health`
List items (dev without OAuth): `curl http://localhost:8080/api/items`
