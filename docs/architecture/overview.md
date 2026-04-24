# Anthology architecture overview

Anthology is a split-stack application with a stateless Go API and an Angular Material UI. The services talk over HTTPS and can be deployed independently, but their responsibilities line up with the following layers:

| Layer | Technology | Responsibilities |
| ----- | ---------- | ---------------- |
| Frontend | Angular 21 + Angular Material | Authentication UI, catalogue browse experience, Add Item workflows (search, manual entry, CSV import). |
| Backend | Go 1.25 (`cmd/api`) | REST API for CRUD, CSV ingestion endpoint, metadata lookup proxy, session management. |
| Data | Postgres (`internal/items`) | Persists catalog items. |
| External APIs | Google Books API | Provides metadata for ISBN/keyword searches and CSV enrichment. |

## Directory layout

* `cmd/api` — chi router setup, middleware, and HTTP handler wiring.
* `internal/items` — domain logic, validation, repository interfaces.
* `internal/shelves` — shelf management, layout definition, and item placement logic.
* `internal/catalog` — Google Books client plus metadata aggregation helpers.
* `internal/importer` — CSV importer used by both the HTTP endpoint and CLI tests.
* `internal/http` — request/response helpers, item handler, catalog handler, and router definitions.
* `web/src/app` — Angular standalone application. Each feature (e.g., Add Item) lives in `pages/` with supporting services under `services/`.
* `web/public` — static assets copied to the dist bundle, including `csv-import-template.csv` so the UI can offer a downloadable example.

## Request flows

### Authentication
1. The UI renders `LoginPageComponent` and starts Google OAuth.
2. The browser completes the OAuth flow and hits `/api/auth/google/callback`.
3. `internal/http/oauth_handler.go` validates the callback, creates a user/session, and sets an HttpOnly cookie so subsequent API calls are authenticated automatically.
4. In `APP_ENV=development`, cookies are non-secure to support localhost during OAuth.

### Metadata search
1. `AddItemPageComponent` submits `GET /api/catalog/lookup?query=...&category=...` when the Search tab runs.
2. `internal/http/catalog_handler.go` validates inputs and calls `internal/catalog.Service`.
3. `internal/catalog.Service` queries Google Books (`/volumes?q=...`) to assemble normalized `Metadata` entries.
4. Results stream back to the UI, which can either quick-add an item or copy the metadata into the manual form.

### CSV import
1. The Angular CSV tab builds a `FormData` payload and calls `POST /api/items/import`.
2. `internal/http.ItemHandler.ImportCSV` enforces the upload size limit, passes the file to `internal/importer.CSVImporter`, and returns the summary as JSON.
3. `CSVImporter` loads existing catalog items to detect duplicates, parses each row, and, when ISBN data exists, calls `internal/catalog.Service` to fill missing metadata via Google Books.
4. The HTTP response includes counts of imported, skipped, and failed rows so the UI can display the progress timeline.

## Data contracts

* **Catalog items** — `internal/items.Item` and `web/src/app/models/item.ts` share the same fields: `title`, `creator`, `itemType`, optional `releaseYear`, `pageCount`, notes/description/ISBNs, and an optional `coverImage` (either a remote URL or a small data URI capped at 500KB).
* **Metadata lookup** — `internal/catalog.Metadata` is mapped to the UI via `ItemLookupService` so selected previews can be passed directly to `ItemService.create`.
* **CSV summary** — `internal/importer.Summary` mirrors `web/src/app/models/import.ts`. Each summary includes `skippedDuplicates` and `failed` entries with the original row number for easy debugging.

## Testing expectations

* `go test ./internal/catalog ./internal/importer ./internal/http` covers lookup, CSV enrichment, and the HTTP surface area (including the new import endpoint).
* `web/src/app/pages/add-item/add-item-page.component.spec.ts` exercises the three Add Item tabs, ensuring UI regressions are caught when importer/search behavior changes.
* Full regression runs should also include `go test ./...` and `cd web && npm test -- --watch=false` before publishing a release.
