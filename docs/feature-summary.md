# Anthology feature summary (code-derived)

This document summarizes application features based on the current API and UI code.

## Catalog items
- Track four item types: books, games, movies, and music.
- Capture core metadata: title, creator, release year, description, ISBN-13/ISBN-10, cover image, notes.
- Book-specific fields include page count, current page, format, genre, rating, retail price, Google volume id, reading status, read date, and series metadata (name, volume number, total volumes).
- Game-specific fields include platform, age group, and player count.
- Items can optionally include shelf placement metadata for shelf-aware views.

## Browse and filter
- List view and grid view for the catalog, with sorting by title for display.
- Filters by item type, reading status (books), shelf status (on/off shelf), and search query.
- Alphabet rail with a histogram endpoint for A-Z and non-letter counts.

## Series handling
- Series view groups book items by series with owned counts, missing volume detection, and completion status.
- Series detail page lists volumes in order and highlights missing entries.
- Add Item supports series-prefill flows to quickly add missing volumes.

## Add and edit items
- Manual entry form for all supported fields.
- Edit existing items and update any editable fields.
- Cover images accept URLs or data URI images (JPEG, PNG, GIF, WebP, SVG) with size limits.

## Metadata lookup and enrichment
- Metadata search endpoint for catalog lookup using Google Books.
- Add Item workflow can search by ISBN or keyword and then quick-add or copy into the manual form.
- Re-sync endpoint to refresh an existing item from Google Books metadata.

## Duplicate detection
- Duplicate check endpoint that matches by title, ISBN-10, or ISBN-13.
- Add Item UI prompts with a duplicate dialog before creating an item.

## CSV import and export
- CSV importer accepts bulk uploads up to 5 MB.
- Import workflow detects duplicates, tracks skipped and failed rows, and enriches missing book data via Google Books when possible.
- UI provides a downloadable CSV template and an import summary with counts.
- CSV exporter outputs the catalog as a downloadable CSV for backups or analysis.

## Shelf management
- Create shelves with a name, description, and photo (stored as a data URL in the UI flow).
- Shelf layouts modeled as rows, columns, and slots with normalized coordinates for visual placement on photos.
- Edit shelf layouts, save updates, and surface displaced items after layout changes.
- Assign items to slots, remove items, and track unplaced items.
- Scan ISBNs to create or move items into shelf slots, with status feedback (created, moved, present).

## Authentication and sessions
- Google OAuth is required in all environments; OAuth sets an HttpOnly session cookie for API access.
- Session endpoints support status, current user, and logout.
- Domain allowlist entries require Google's `hd` claim; the allowlist is rechecked on every request, and expired sessions are purged daily.

## API utilities
- Health check endpoint for service status.
- Postgres-backed repositories, with migrations defining shelves and reading status defaults.
