# Anthology API deep dive

This document reflects the current API implementation under `cmd/api` and `internal/` as of this commit. It is intended for engineers onboarding to the backend and focuses on endpoints, request flows, data models, validation, and infrastructure concerns.

## High-level shape

* Go 1.25, chi router, `slog` logging.
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

Base URL: `http://<host>:<port>`. `/api/*` endpoints require the session cookie; `/health` remains public.

| Method | Path | Description | Handler |
| --- | --- | --- | --- |
| GET | `/health` | Liveness; returns `{status, environment}`. | inline in router |
| GET | `/api/auth/google` | Initiate Google OAuth. | `OAuthHandler.InitiateGoogle` |
| GET | `/api/auth/google/callback` | Handle OAuth callback, set session. | `OAuthHandler.CallbackGoogle` |
| GET | `/api/session` | Report session status and user (if authenticated). | `SessionHandler.Status` |
| DELETE | `/api/session` | Clear session cookie. | `SessionHandler.Logout` |
| GET | `/api/session/user` | Return current user profile. | `SessionHandler.CurrentUser` |
| GET | `/api/items` | List items with filters (type/status/letter/query/limit). | `ItemHandler.List` |
| GET | `/api/items/histogram` | Letter counts for alphabet rail. | `ItemHandler.Histogram` |
| GET | `/api/items/duplicates` | Check potential duplicates by title/ISBN. | `ItemHandler.Duplicates` |
| POST | `/api/items` | Create item. | `ItemHandler.Create` |
| POST | `/api/items/import` | CSV upload (5 MiB limit) for bulk import. | `ItemHandler.ImportCSV` |
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
* Unknown fields rejected on create/update (strict JSON decode); payload limited to 1 MiB.
* CSV upload returns 400 on invalid/missing file, 413 on size overflow, 500 on importer errors.
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
* Upload capped at 5 MiB (HTTP handler).

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
* Timeouts: Request timeout middleware 60s; HTTP server read/write 15s, idle 60s.
* Logging: `slog` text handler; HTTP middleware logs method/path/status/duration.
* CSV upload size guard at handler level; JSON max 1 MiB.
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

Health: `curl http://localhost:8080/health`  
List items (dev without OAuth): `curl http://localhost:8080/api/items`
