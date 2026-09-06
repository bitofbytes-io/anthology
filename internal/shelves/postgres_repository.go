package shelves

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

type postgresRepository struct {
	db *sqlx.DB
}

// NewPostgresRepository creates a shelves repository backed by Postgres.
func NewPostgresRepository(db *sqlx.DB) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) CreateShelf(ctx context.Context, shelf Shelf, rows []ShelfRow, columns []ShelfColumn, slots []ShelfSlot) (ShelfWithLayout, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return ShelfWithLayout{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.NamedExecContext(ctx, `
        INSERT INTO shelves (id, owner_id, name, description, photo_url, created_at, updated_at)
        VALUES (:id, :owner_id, :name, :description, :photo_url, :created_at, :updated_at)
    `, shelf); err != nil {
		return ShelfWithLayout{}, err
	}

	if err := insertRows(ctx, tx, rows); err != nil {
		return ShelfWithLayout{}, err
	}
	if err := insertColumns(ctx, tx, columns); err != nil {
		return ShelfWithLayout{}, err
	}
	if err := insertSlots(ctx, tx, slots); err != nil {
		return ShelfWithLayout{}, err
	}

	if err := tx.Commit(); err != nil {
		return ShelfWithLayout{}, err
	}

	return r.GetShelf(ctx, shelf.ID, shelf.OwnerID)
}

func (r *postgresRepository) ListShelves(ctx context.Context, ownerID uuid.UUID) ([]ShelfSummary, error) {
	rows, err := r.db.QueryxContext(ctx, `
        SELECT s.id, s.owner_id, s.name, s.description, s.photo_url, s.created_at, s.updated_at,
               COALESCE(COUNT(isl.id), 0) AS item_count,
               COALESCE(SUM(CASE WHEN isl.shelf_slot_id IS NOT NULL THEN 1 ELSE 0 END), 0) AS placed_count,
               COALESCE(slot_counts.slot_count, 0) AS slot_count
        FROM shelves s
        LEFT JOIN item_shelf_locations isl ON isl.shelf_id = s.id
        LEFT JOIN (
            SELECT shelf_id, COUNT(*) AS slot_count FROM shelf_slots GROUP BY shelf_id
        ) AS slot_counts ON slot_counts.shelf_id = s.id
        WHERE s.owner_id = $1
        GROUP BY s.id, s.owner_id, s.name, s.description, s.photo_url, s.created_at, s.updated_at, slot_counts.slot_count
        ORDER BY s.created_at DESC
    `, ownerID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	summaries := make([]ShelfSummary, 0)
	for rows.Next() {
		var shelf Shelf
		var itemCount, placedCount, slotCount int
		if err := rows.Scan(&shelf.ID, &shelf.OwnerID, &shelf.Name, &shelf.Description, &shelf.PhotoURL, &shelf.CreatedAt, &shelf.UpdatedAt, &itemCount, &placedCount, &slotCount); err != nil {
			return nil, err
		}
		summaries = append(summaries, ShelfSummary{Shelf: shelf, ItemCount: itemCount, PlacedCount: placedCount, SlotCount: slotCount})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return summaries, nil
}

func (r *postgresRepository) GetShelf(ctx context.Context, shelfID uuid.UUID, ownerID uuid.UUID) (ShelfWithLayout, error) {
	var shelf Shelf
	if err := r.db.GetContext(ctx, &shelf, `SELECT * FROM shelves WHERE id = $1 AND owner_id = $2`, shelfID, ownerID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ShelfWithLayout{}, ErrNotFound
		}
		return ShelfWithLayout{}, err
	}

	rows, err := r.fetchRows(ctx, shelfID)
	if err != nil {
		return ShelfWithLayout{}, err
	}
	cols, err := r.fetchColumns(ctx, shelfID)
	if err != nil {
		return ShelfWithLayout{}, err
	}
	slots, err := r.fetchSlots(ctx, shelfID)
	if err != nil {
		return ShelfWithLayout{}, err
	}
	placements, err := r.ListPlacements(ctx, shelfID, ownerID)
	if err != nil {
		return ShelfWithLayout{}, err
	}

	rowColumns := make(map[uuid.UUID][]ShelfColumn)
	for _, col := range cols {
		rowColumns[col.ShelfRowID] = append(rowColumns[col.ShelfRowID], col)
	}

	var rowWithColumns []RowWithColumns
	for _, row := range rows {
		rowWithColumns = append(rowWithColumns, RowWithColumns{ShelfRow: row, Columns: rowColumns[row.ID]})
	}

	var placementWithItems []PlacementWithItem
	for _, placement := range placements {
		placementWithItems = append(placementWithItems, PlacementWithItem{Placement: placement})
	}

	return ShelfWithLayout{
		Shelf:      shelf,
		Rows:       rowWithColumns,
		Slots:      slots,
		Placements: placementWithItems,
	}, nil
}

func (r *postgresRepository) SaveLayout(ctx context.Context, shelfID uuid.UUID, ownerID uuid.UUID, rows []ShelfRow, columns []ShelfColumn, slots []ShelfSlot, removedSlotIDs []uuid.UUID) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var exists bool
	if err := tx.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM shelves WHERE id=$1 AND owner_id=$2)`, shelfID, ownerID); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}

	if len(removedSlotIDs) > 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM item_shelf_locations WHERE shelf_id=$2 AND shelf_slot_id = ANY($1)`, pq.Array(removedSlotIDs), shelfID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM shelf_slots WHERE shelf_id=$1 AND id = ANY($2)`, shelfID, pq.Array(removedSlotIDs)); err != nil {
			return err
		}
	}

	rowIDs := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		rowIDs = append(rowIDs, row.ID)
	}
	colIDs := make([]uuid.UUID, 0, len(columns))
	for _, col := range columns {
		colIDs = append(colIDs, col.ID)
	}
	slotIDs := make([]uuid.UUID, 0, len(slots))
	for _, slot := range slots {
		slotIDs = append(slotIDs, slot.ID)
	}

	// Move surviving slots to their new parents before removing old parents;
	// otherwise ON DELETE CASCADE would erase the slots and their placements.
	if err := insertRows(ctx, tx, rows); err != nil {
		return err
	}
	if err := insertColumns(ctx, tx, columns); err != nil {
		return err
	}
	if err := insertSlots(ctx, tx, slots); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM item_shelf_locations WHERE shelf_id=$1 AND shelf_slot_id IS NOT NULL AND shelf_slot_id <> ALL($2)`, shelfID, pq.Array(slotIDs)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM shelf_slots WHERE shelf_id=$1 AND id <> ALL($2)`, shelfID, pq.Array(slotIDs)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM shelf_columns WHERE shelf_row_id IN (SELECT id FROM shelf_rows WHERE shelf_id=$1) AND id <> ALL($2)`, shelfID, pq.Array(colIDs)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM shelf_rows WHERE shelf_id=$1 AND id <> ALL($2)`, shelfID, pq.Array(rowIDs)); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *postgresRepository) AssignItemToSlot(ctx context.Context, shelfID uuid.UUID, ownerID uuid.UUID, slotID uuid.UUID, itemID uuid.UUID) (ItemPlacement, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return ItemPlacement{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var shelfExists bool
	if err := tx.GetContext(ctx, &shelfExists, `SELECT EXISTS(SELECT 1 FROM shelves WHERE id=$1 AND owner_id=$2)`, shelfID, ownerID); err != nil {
		return ItemPlacement{}, err
	}
	if !shelfExists {
		return ItemPlacement{}, ErrNotFound
	}

	var slotExists bool
	if err := tx.GetContext(ctx, &slotExists, `SELECT EXISTS(SELECT 1 FROM shelf_slots WHERE id=$1 AND shelf_id=$2)`, slotID, shelfID); err != nil {
		return ItemPlacement{}, err
	}
	if !slotExists {
		return ItemPlacement{}, ErrSlotNotFound
	}

	// Delete any existing placements for this item across ALL shelves (not just this shelf)
	// to ensure an item can only be on one shelf at a time
	if _, err := tx.ExecContext(ctx, `DELETE FROM item_shelf_locations WHERE item_id=$1`, itemID); err != nil {
		return ItemPlacement{}, err
	}

	placement := ItemPlacement{
		ID:          uuid.New(),
		ItemID:      itemID,
		ShelfID:     shelfID,
		ShelfSlotID: &slotID,
		CreatedAt:   time.Now().UTC(),
	}

	if _, err := tx.NamedExecContext(ctx, `
        INSERT INTO item_shelf_locations (id, item_id, shelf_id, shelf_slot_id, created_at)
        VALUES (:id, :item_id, :shelf_id, :shelf_slot_id, :created_at)
    `, placement); err != nil {
		return ItemPlacement{}, err
	}

	if err := tx.Commit(); err != nil {
		return ItemPlacement{}, err
	}

	return placement, nil
}

func (r *postgresRepository) RemoveItemFromSlot(ctx context.Context, shelfID uuid.UUID, ownerID uuid.UUID, slotID uuid.UUID, itemID uuid.UUID) error {
	// First verify shelf belongs to owner
	var shelfExists bool
	if err := r.db.GetContext(ctx, &shelfExists, `SELECT EXISTS(SELECT 1 FROM shelves WHERE id=$1 AND owner_id=$2)`, shelfID, ownerID); err != nil {
		return err
	}
	if !shelfExists {
		return ErrNotFound
	}

	result, err := r.db.ExecContext(ctx, `
        UPDATE item_shelf_locations
        SET shelf_slot_id = NULL
        WHERE shelf_id=$1 AND item_id=$2 AND shelf_slot_id=$3
    `, shelfID, itemID, slotID)
	if err != nil {
		return err
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrSlotNotFound
	}
	return nil
}

func (r *postgresRepository) ListPlacements(ctx context.Context, shelfID uuid.UUID, ownerID uuid.UUID) ([]ItemPlacement, error) {
	// Verify shelf belongs to owner
	var shelfExists bool
	if err := r.db.GetContext(ctx, &shelfExists, `SELECT EXISTS(SELECT 1 FROM shelves WHERE id=$1 AND owner_id=$2)`, shelfID, ownerID); err != nil {
		return nil, err
	}
	if !shelfExists {
		return nil, ErrNotFound
	}

	var placements []ItemPlacement
	if err := r.db.SelectContext(ctx, &placements, `SELECT * FROM item_shelf_locations WHERE shelf_id=$1`, shelfID); err != nil {
		return nil, err
	}
	return placements, nil
}

func (r *postgresRepository) UpsertUnplaced(ctx context.Context, shelfID uuid.UUID, ownerID uuid.UUID, itemID uuid.UUID) (ItemPlacement, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return ItemPlacement{}, err
	}
	defer func() { _ = tx.Rollback() }()

	// Verify shelf belongs to owner
	var shelfExists bool
	if err := tx.GetContext(ctx, &shelfExists, `SELECT EXISTS(SELECT 1 FROM shelves WHERE id=$1 AND owner_id=$2)`, shelfID, ownerID); err != nil {
		return ItemPlacement{}, err
	}
	if !shelfExists {
		return ItemPlacement{}, ErrNotFound
	}

	placement := ItemPlacement{
		ID:          uuid.New(),
		ItemID:      itemID,
		ShelfID:     shelfID,
		ShelfSlotID: nil,
		CreatedAt:   time.Now().UTC(),
	}

	if err := tx.GetContext(ctx, &placement, `
        INSERT INTO item_shelf_locations (id, item_id, shelf_id, shelf_slot_id, created_at)
        VALUES ($1, $2, $3, $4, $5)
        ON CONFLICT (shelf_id, item_id)
        DO UPDATE SET shelf_slot_id = NULL, created_at = EXCLUDED.created_at
        RETURNING id, item_id, shelf_id, shelf_slot_id, created_at
    `, placement.ID, placement.ItemID, placement.ShelfID, placement.ShelfSlotID, placement.CreatedAt); err != nil {
		return ItemPlacement{}, err
	}

	if err := tx.Commit(); err != nil {
		return ItemPlacement{}, err
	}
	return placement, nil
}

func (r *postgresRepository) fetchRows(ctx context.Context, shelfID uuid.UUID) ([]ShelfRow, error) {
	var rows []ShelfRow
	if err := r.db.SelectContext(ctx, &rows, `SELECT * FROM shelf_rows WHERE shelf_id=$1 ORDER BY row_index`, shelfID); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *postgresRepository) fetchColumns(ctx context.Context, shelfID uuid.UUID) ([]ShelfColumn, error) {
	var cols []ShelfColumn
	if err := r.db.SelectContext(ctx, &cols, `
        SELECT sc.* FROM shelf_columns sc
        JOIN shelf_rows sr ON sc.shelf_row_id = sr.id
        WHERE sr.shelf_id = $1
        ORDER BY sc.col_index
    `, shelfID); err != nil {
		return nil, err
	}
	return cols, nil
}

func (r *postgresRepository) fetchSlots(ctx context.Context, shelfID uuid.UUID) ([]ShelfSlot, error) {
	var slots []ShelfSlot
	if err := r.db.SelectContext(ctx, &slots, `SELECT * FROM shelf_slots WHERE shelf_id=$1 ORDER BY row_index, col_index`, shelfID); err != nil {
		return nil, err
	}
	return slots, nil
}

func insertRows(ctx context.Context, tx *sqlx.Tx, rows []ShelfRow) error {
	for _, row := range rows {
		if _, err := tx.NamedExecContext(ctx, `
            INSERT INTO shelf_rows (id, shelf_id, row_index, y_start_norm, y_end_norm)
            VALUES (:id, :shelf_id, :row_index, :y_start_norm, :y_end_norm)
            ON CONFLICT (id) DO UPDATE SET row_index = EXCLUDED.row_index, y_start_norm = EXCLUDED.y_start_norm, y_end_norm = EXCLUDED.y_end_norm
        `, row); err != nil {
			return err
		}
	}
	return nil
}

func insertColumns(ctx context.Context, tx *sqlx.Tx, columns []ShelfColumn) error {
	for _, col := range columns {
		if _, err := tx.NamedExecContext(ctx, `
            INSERT INTO shelf_columns (id, shelf_row_id, col_index, x_start_norm, x_end_norm)
            VALUES (:id, :shelf_row_id, :col_index, :x_start_norm, :x_end_norm)
            ON CONFLICT (id) DO UPDATE SET shelf_row_id = EXCLUDED.shelf_row_id, col_index = EXCLUDED.col_index, x_start_norm = EXCLUDED.x_start_norm, x_end_norm = EXCLUDED.x_end_norm
        `, col); err != nil {
			return err
		}
	}
	return nil
}

func insertSlots(ctx context.Context, tx *sqlx.Tx, slots []ShelfSlot) error {
	for _, slot := range slots {
		if _, err := tx.NamedExecContext(ctx, `
            INSERT INTO shelf_slots (id, shelf_id, shelf_row_id, shelf_column_id, row_index, col_index, x_start_norm, x_end_norm, y_start_norm, y_end_norm)
            VALUES (:id, :shelf_id, :shelf_row_id, :shelf_column_id, :row_index, :col_index, :x_start_norm, :x_end_norm, :y_start_norm, :y_end_norm)
            ON CONFLICT (id) DO UPDATE SET shelf_row_id = EXCLUDED.shelf_row_id, shelf_column_id = EXCLUDED.shelf_column_id, row_index = EXCLUDED.row_index, col_index = EXCLUDED.col_index, x_start_norm = EXCLUDED.x_start_norm, x_end_norm = EXCLUDED.x_end_norm, y_start_norm = EXCLUDED.y_start_norm, y_end_norm = EXCLUDED.y_end_norm
        `, slot); err != nil {
			return err
		}
	}
	return nil
}
