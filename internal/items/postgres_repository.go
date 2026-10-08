package items

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// PostgresRepository persists items to a Postgres database.
type PostgresRepository struct {
	db *sqlx.DB
}

// NewPostgresRepository constructs a repository backed by sqlx.
func NewPostgresRepository(db *sqlx.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

const baseSelect = `
SELECT
    i.id,
    i.owner_id,
    i.title,
    i.creator,
    i.item_type,
    i.release_year,
    i.page_count,
    i.current_page,
    i.isbn_13,
    i.isbn_10,
    i.description,
    i.cover_image,
    i.format,
    i.genre,
    i.rating,
    i.retail_price_usd,
    i.google_volume_id,
    i.platform,
    i.age_group,
    i.player_count,
    i.reading_status,
    i.read_at,
    i.notes,
    i.series_name,
    i.volume_number,
    i.total_volumes,
    i.created_at,
    i.updated_at,
    placement.shelf_id AS placement_shelf_id,
    placement.shelf_slot_id AS placement_shelf_slot_id,
    placement.shelf_name AS placement_shelf_name,
    placement.row_index AS placement_row_index,
    placement.col_index AS placement_col_index
FROM items i
LEFT JOIN LATERAL (
    SELECT
        isl.shelf_id,
        isl.shelf_slot_id,
        ss.row_index,
        ss.col_index,
        s.name AS shelf_name
    FROM item_shelf_locations isl
    JOIN shelves s ON s.id = isl.shelf_id
    JOIN shelf_slots ss ON ss.id = isl.shelf_slot_id
    WHERE isl.item_id = i.id AND isl.shelf_slot_id IS NOT NULL
    ORDER BY isl.created_at DESC
    LIMIT 1
) AS placement ON true
`

type itemRow struct {
	Item
	PlacementShelfID     *uuid.UUID `db:"placement_shelf_id"`
	PlacementShelfSlotID *uuid.UUID `db:"placement_shelf_slot_id"`
	PlacementShelfName   *string    `db:"placement_shelf_name"`
	PlacementRowIndex    *int       `db:"placement_row_index"`
	PlacementColIndex    *int       `db:"placement_col_index"`
}

func (row itemRow) toItem() Item {
	item := row.Item
	if row.PlacementShelfID != nil &&
		row.PlacementShelfSlotID != nil &&
		row.PlacementShelfName != nil &&
		row.PlacementRowIndex != nil &&
		row.PlacementColIndex != nil {
		item.ShelfPlacement = &ShelfPlacement{
			ShelfID:   *row.PlacementShelfID,
			ShelfName: *row.PlacementShelfName,
			SlotID:    *row.PlacementShelfSlotID,
			RowIndex:  *row.PlacementRowIndex,
			ColIndex:  *row.PlacementColIndex,
		}
	}
	return item
}

// Create inserts a new row and returns the stored representation. It takes
// the owner's insert lock for its transaction, so it waits while an insert
// session (see BeginExclusiveInserts) holds the lock in any process. The
// stored row comes back from the INSERT itself; an error wraps
// ErrSaveOutcomeUnknown when the row may have been stored anyway.
func (r *PostgresRepository) Create(ctx context.Context, item Item) (Item, error) {
	query, args, err := r.db.BindNamed(insertItemSQL, item)
	if err != nil {
		return Item{}, fmt.Errorf("bind item insert: %w", err)
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return Item{}, fmt.Errorf("begin item insert: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, lockOwnerInsertsSQL, itemInsertLockClass, ownerLockKey(item.OwnerID)); err != nil {
		return Item{}, fmt.Errorf("lock owner inserts: %w", err)
	}
	var stored Item
	if err := tx.QueryRowxContext(ctx, query, args...).StructScan(&stored); err != nil {
		// Nothing is committed until Commit, so a failure here stores nothing.
		return Item{}, fmt.Errorf("insert item: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Item{}, saveOutcomeErr(ctx, "commit item insert", err)
	}
	return stored, nil
}

// Get retrieves a row by primary key and owner.
func (r *PostgresRepository) Get(ctx context.Context, id uuid.UUID, ownerID uuid.UUID) (Item, error) {
	var row itemRow
	if err := r.db.GetContext(ctx, &row, baseSelect+" WHERE i.id = $1 AND i.owner_id = $2", id, ownerID); err != nil {
		if err == sql.ErrNoRows {
			return Item{}, ErrNotFound
		}
		return Item{}, fmt.Errorf("get item: %w", err)
	}
	return row.toItem(), nil
}

// List returns items ordered by creation timestamp descending, filtered by the provided options.
func (r *PostgresRepository) List(ctx context.Context, opts ListOptions) ([]Item, error) {
	query := baseSelect
	clauses := []string{}
	args := []any{}

	// Always filter by owner_id first
	clauses = append(clauses, fmt.Sprintf("i.owner_id = $%d", len(args)+1))
	args = append(args, opts.OwnerID)

	if opts.ItemType != nil {
		clauses = append(clauses, fmt.Sprintf("i.item_type = $%d", len(args)+1))
		args = append(args, *opts.ItemType)
	}

	// When filtering by status with no type filter (All), only apply to books (BE-2)
	if opts.ReadingStatus != nil {
		if opts.ItemType == nil {
			// All items with status filter:
			// - "none" status: show all items (books with none + all non-books)
			// - Other statuses: show only books with that status
			if *opts.ReadingStatus == BookStatusNone {
				clauses = append(clauses, fmt.Sprintf("(i.item_type != 'book' OR i.reading_status = $%d)", len(args)+1))
				args = append(args, BookStatusNone)
			} else {
				clauses = append(clauses, "i.item_type = 'book'")
				clauses = append(clauses, fmt.Sprintf("i.reading_status = $%d", len(args)+1))
				args = append(args, *opts.ReadingStatus)
			}
		} else {
			clauses = append(clauses, fmt.Sprintf("i.reading_status = $%d", len(args)+1))
			args = append(args, *opts.ReadingStatus)
		}
	}

	if opts.Initial != nil {
		initial := strings.ToUpper(strings.TrimSpace(*opts.Initial))
		if initial == "#" {
			clauses = append(clauses, "NOT ("+titleInitialSQL("i.title")+" BETWEEN 'A' AND 'Z')")
		} else {
			clauses = append(clauses, fmt.Sprintf("%s = $%d", titleInitialSQL("i.title"), len(args)+1))
			args = append(args, initial)
		}
	}

	if opts.Query != nil {
		search := strings.TrimSpace(*opts.Query)
		if search != "" {
			clauses = append(clauses, fmt.Sprintf("i.title ILIKE $%d", len(args)+1))
			args = append(args, "%"+search+"%")
		}
	}

	if opts.ShelfStatus != nil {
		switch *opts.ShelfStatus {
		case ShelfStatusOn:
			clauses = append(clauses, "placement.shelf_id IS NOT NULL")
		case ShelfStatusOff:
			clauses = append(clauses, "placement.shelf_id IS NULL")
		}
	}

	if len(clauses) > 0 {
		query = query + " WHERE " + strings.Join(clauses, " AND ")
	}

	query = query + " ORDER BY i.created_at DESC, i.title ASC"

	if opts.Limit != nil && *opts.Limit > 0 {
		query = fmt.Sprintf("%s LIMIT %d", query, *opts.Limit)
	}

	rows := []itemRow{}
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}

	items := make([]Item, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.toItem())
	}
	return items, nil
}

// ListByIDs returns the owner's items with the given IDs.
func (r *PostgresRepository) ListByIDs(ctx context.Context, ids []uuid.UUID, ownerID uuid.UUID) ([]Item, error) {
	if len(ids) == 0 {
		return []Item{}, nil
	}

	rows := []itemRow{}
	query := baseSelect + " WHERE i.id = ANY($1) AND i.owner_id = $2 ORDER BY i.created_at DESC, i.title ASC"
	if err := r.db.SelectContext(ctx, &rows, query, pq.Array(ids), ownerID); err != nil {
		return nil, fmt.Errorf("list items by id: %w", err)
	}

	items := make([]Item, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.toItem())
	}
	return items, nil
}

// FindByISBN returns the owner's most recently created book matching isbn, in
// either its ISBN-10 or ISBN-13 form, against either ISBN column (see
// isbnLookupKeys and isbnMatchKey).
//
// The per-row regexp_replace is not index-backed, but the owner_id predicate
// uses idx_items_owner_id, so the scan is bounded by one personal library.
func (r *PostgresRepository) FindByISBN(ctx context.Context, isbn string, ownerID uuid.UUID) (Item, error) {
	keys := isbnLookupKeys(isbn)
	if len(keys) == 0 {
		return Item{}, ErrNotFound
	}

	query := baseSelect + ` WHERE i.owner_id = $1
  AND i.item_type = 'book'
  AND (upper(regexp_replace(i.isbn_13, '[^0-9Xx]', '', 'g')) = ANY($2) OR upper(regexp_replace(i.isbn_10, '[^0-9Xx]', '', 'g')) = ANY($2))
ORDER BY i.created_at DESC, i.title ASC
LIMIT 1`

	var row itemRow
	if err := r.db.GetContext(ctx, &row, query, ownerID, pq.Array(keys)); err != nil {
		if err == sql.ErrNoRows {
			return Item{}, ErrNotFound
		}
		return Item{}, fmt.Errorf("find item by isbn: %w", err)
	}
	return row.toItem(), nil
}

// Update modifies an existing row.
func (r *PostgresRepository) Update(ctx context.Context, item Item) (Item, error) {
	query := `UPDATE items
SET title = :title,
    creator = :creator,
    item_type = :item_type,
    release_year = :release_year,
    page_count = :page_count,
    current_page = :current_page,
    isbn_13 = :isbn_13,
    isbn_10 = :isbn_10,
    description = :description,
    cover_image = :cover_image,
    format = :format,
    genre = :genre,
    rating = :rating,
    retail_price_usd = :retail_price_usd,
    google_volume_id = :google_volume_id,
    platform = :platform,
    age_group = :age_group,
    player_count = :player_count,
    reading_status = :reading_status,
    read_at = :read_at,
    notes = :notes,
    series_name = :series_name,
    volume_number = :volume_number,
    total_volumes = :total_volumes,
    updated_at = :updated_at
WHERE id = :id AND owner_id = :owner_id`

	res, err := r.db.NamedExecContext(ctx, query, item)
	if err != nil {
		return Item{}, fmt.Errorf("update item: %w", err)
	}
	rows, err := res.RowsAffected()
	if err == nil && rows == 0 {
		return Item{}, ErrNotFound
	}

	return r.Get(ctx, item.ID, item.OwnerID)
}

// Delete removes an item.
func (r *PostgresRepository) Delete(ctx context.Context, id uuid.UUID, ownerID uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM items WHERE id = $1 AND owner_id = $2", id, ownerID)
	if err != nil {
		return fmt.Errorf("delete item: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete item rows: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// titleInitialSQL returns the upper-cased first character of column under the
// "C" collation. The letter filter and the histogram both compare it with
// BETWEEN 'A' AND 'Z', and under a linguistic collation such as en_US 'É' sorts
// inside that range: accented titles got their own histogram bucket but
// matched no rail letter. With "C" only ASCII A-Z count as letters and
// everything else falls under "#", matching the in-memory repository.
func titleInitialSQL(column string) string {
	return `upper(substr(trim(` + column + `), 1, 1)) COLLATE "C"`
}

// Histogram returns a count of items grouped by first letter of title.
func (r *PostgresRepository) Histogram(ctx context.Context, opts HistogramOptions) (LetterHistogram, error) {
	query := `
SELECT
    CASE
        WHEN ` + titleInitialSQL("title") + ` BETWEEN 'A' AND 'Z'
        THEN ` + titleInitialSQL("title") + `
        ELSE '#'
    END AS letter,
    COUNT(*) AS count
FROM items`

	clauses := []string{}
	args := []any{}

	// Always filter by owner_id first
	clauses = append(clauses, fmt.Sprintf("owner_id = $%d", len(args)+1))
	args = append(args, opts.OwnerID)

	if opts.ItemType != nil {
		clauses = append(clauses, fmt.Sprintf("item_type = $%d", len(args)+1))
		args = append(args, *opts.ItemType)
	}

	// When filtering by status with no type filter (All), only apply to books (BE-2)
	if opts.ReadingStatus != nil {
		if opts.ItemType == nil {
			if *opts.ReadingStatus == BookStatusNone {
				clauses = append(clauses, fmt.Sprintf("(item_type != 'book' OR reading_status = $%d)", len(args)+1))
				args = append(args, BookStatusNone)
			} else {
				clauses = append(clauses, "item_type = 'book'")
				clauses = append(clauses, fmt.Sprintf("reading_status = $%d", len(args)+1))
				args = append(args, *opts.ReadingStatus)
			}
		} else {
			clauses = append(clauses, fmt.Sprintf("reading_status = $%d", len(args)+1))
			args = append(args, *opts.ReadingStatus)
		}
	}

	if len(clauses) > 0 {
		query = query + " WHERE " + strings.Join(clauses, " AND ")
	}

	query = query + " GROUP BY letter"

	type letterCount struct {
		Letter string `db:"letter"`
		Count  int    `db:"count"`
	}

	rows := []letterCount{}
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, fmt.Errorf("histogram: %w", err)
	}

	histogram := make(LetterHistogram)
	for _, row := range rows {
		histogram[row.Letter] = row.Count
	}

	return histogram, nil
}

// FindDuplicates searches for items matching the given title or identifiers.
// Title matching is case-insensitive. Identifier matching normalizes by stripping non-digits.
// Returns up to 5 matches.
func (r *PostgresRepository) FindDuplicates(ctx context.Context, input DuplicateCheckInput, ownerID uuid.UUID) ([]DuplicateMatch, error) {
	normalizedTitle := NormalizeTitle(input.Title)
	normalizedISBN13 := NormalizeIdentifier(input.ISBN13)
	normalizedISBN10 := NormalizeIdentifier(input.ISBN10)

	// Build OR conditions for matching
	clauses := []string{}
	args := []any{}

	if normalizedTitle != "" {
		clauses = append(clauses, fmt.Sprintf("lower(trim(i.title)) = $%d", len(args)+1))
		args = append(args, normalizedTitle)
	}

	if normalizedISBN13 != "" {
		clauses = append(clauses, fmt.Sprintf("regexp_replace(i.isbn_13, '[^0-9]', '', 'g') = $%d", len(args)+1))
		args = append(args, normalizedISBN13)
	}

	if normalizedISBN10 != "" {
		clauses = append(clauses, fmt.Sprintf("regexp_replace(i.isbn_10, '[^0-9]', '', 'g') = $%d", len(args)+1))
		args = append(args, normalizedISBN10)
	}

	if len(clauses) == 0 {
		return []DuplicateMatch{}, nil
	}

	// Add owner_id filter
	ownerClause := fmt.Sprintf("i.owner_id = $%d", len(args)+1)
	args = append(args, ownerID)

	query := baseSelect + " WHERE " + ownerClause + " AND (" + strings.Join(clauses, " OR ") + ") ORDER BY i.updated_at DESC LIMIT 5"

	rows := []itemRow{}
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, fmt.Errorf("find duplicates: %w", err)
	}

	matches := make([]DuplicateMatch, 0, len(rows))
	for _, row := range rows {
		item := row.toItem()
		matches = append(matches, itemToDuplicateMatch(item))
	}

	return matches, nil
}

// ListSeries returns all unique series with their items grouped.
func (r *PostgresRepository) ListSeries(ctx context.Context, opts SeriesRepoListOptions, ownerID uuid.UUID) ([]SeriesSummary, error) {
	query := baseSelect + ` WHERE i.owner_id = $1 AND i.series_name != '' AND i.item_type = 'book' ORDER BY i.series_name, i.volume_number NULLS LAST, i.title`

	rows := []itemRow{}
	if err := r.db.SelectContext(ctx, &rows, query, ownerID); err != nil {
		return nil, fmt.Errorf("list series: %w", err)
	}

	// Group items by series name
	seriesMap := make(map[string][]Item)
	seriesOrder := []string{}

	for _, row := range rows {
		item := row.toItem()
		if _, exists := seriesMap[item.SeriesName]; !exists {
			seriesOrder = append(seriesOrder, item.SeriesName)
		}
		seriesMap[item.SeriesName] = append(seriesMap[item.SeriesName], item)
	}

	// Build summaries
	summaries := make([]SeriesSummary, 0, len(seriesOrder))
	for _, name := range seriesOrder {
		items := seriesMap[name]
		summary := SeriesSummary{
			SeriesName: name,
			OwnedCount: len(items),
		}

		if opts.IncludeItems {
			summary.Items = items
		}

		// Find max total_volumes from items
		for _, item := range items {
			if item.TotalVolumes != nil {
				if summary.TotalVolumes == nil || *item.TotalVolumes > *summary.TotalVolumes {
					summary.TotalVolumes = item.TotalVolumes
				}
			}
		}

		summaries = append(summaries, summary)
	}

	return summaries, nil
}

// GetSeriesByName returns detailed info about a single series.
func (r *PostgresRepository) GetSeriesByName(ctx context.Context, name string, ownerID uuid.UUID) (SeriesSummary, error) {
	query := baseSelect + ` WHERE i.owner_id = $1 AND i.series_name = $2 AND i.item_type = 'book' ORDER BY i.volume_number NULLS LAST, i.title`

	rows := []itemRow{}
	if err := r.db.SelectContext(ctx, &rows, query, ownerID, name); err != nil {
		return SeriesSummary{}, fmt.Errorf("get series: %w", err)
	}

	if len(rows) == 0 {
		return SeriesSummary{}, ErrNotFound
	}

	items := make([]Item, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.toItem())
	}

	summary := SeriesSummary{
		SeriesName: name,
		OwnedCount: len(items),
		Items:      items,
	}

	// Find max total_volumes from items
	for _, item := range items {
		if item.TotalVolumes != nil {
			if summary.TotalVolumes == nil || *item.TotalVolumes > *summary.TotalVolumes {
				summary.TotalVolumes = item.TotalVolumes
			}
		}
	}

	return summary, nil
}

// ListSeriesNamesByNameCI returns distinct series names matching case-insensitively.
func (r *PostgresRepository) ListSeriesNamesByNameCI(ctx context.Context, name string, ownerID uuid.UUID) ([]string, error) {
	if name == "" {
		return nil, nil
	}

	query := `SELECT DISTINCT series_name
		FROM items
		WHERE owner_id = $1
		  AND item_type = 'book'
		  AND series_name != ''
		  AND LOWER(series_name) = LOWER($2)
		ORDER BY series_name`

	names := []string{}
	if err := r.db.SelectContext(ctx, &names, query, ownerID, name); err != nil {
		return nil, fmt.Errorf("list series names: %w", err)
	}
	return names, nil
}

// UpdateSeriesName updates series_name on all items matching oldName for the given owner.
func (r *PostgresRepository) UpdateSeriesName(ctx context.Context, oldName, newName string, ownerID uuid.UUID) (int64, error) {
	query := `UPDATE items SET series_name = $1, updated_at = NOW() WHERE series_name = $2 AND owner_id = $3 AND item_type = 'book'`
	res, err := r.db.ExecContext(ctx, query, newName, oldName, ownerID)
	if err != nil {
		return 0, fmt.Errorf("update series name: %w", err)
	}
	return res.RowsAffected()
}

// ClearSeriesName clears series_name, volume_number, and total_volumes on all items matching seriesName for the given owner.
func (r *PostgresRepository) ClearSeriesName(ctx context.Context, seriesName string, ownerID uuid.UUID) (int64, error) {
	query := `UPDATE items SET series_name = '', volume_number = NULL, total_volumes = NULL, updated_at = NOW() WHERE series_name = $1 AND owner_id = $2 AND item_type = 'book'`
	res, err := r.db.ExecContext(ctx, query, seriesName, ownerID)
	if err != nil {
		return 0, fmt.Errorf("clear series name: %w", err)
	}
	return res.RowsAffected()
}
