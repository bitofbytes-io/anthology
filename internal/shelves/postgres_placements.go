package shelves

import (
	"context"
	"time"

	"github.com/google/uuid"
)

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
