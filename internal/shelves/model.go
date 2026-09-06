package shelves

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"anthology/internal/items"
)

// ErrNotFound is returned when a shelf cannot be found.
var ErrNotFound = errors.New("shelf not found")

// ErrSlotNotFound is returned when a slot cannot be located for a shelf.
var ErrSlotNotFound = errors.New("shelf slot not found")

// ErrValidation wraps user-correctable validation errors safe to expose to clients.
var ErrValidation = errors.New("validation error")

// ErrISBNNotFound is returned when a scanned barcode cannot be found in the catalog.
var ErrISBNNotFound = errors.New("no results found for scanned barcode")

// ScanStatus indicates the result of a scan operation.
type ScanStatus string

const (
	// ScanStatusCreated indicates a new item was created and assigned.
	ScanStatusCreated ScanStatus = "created"
	// ScanStatusMoved indicates an existing item was moved from another slot.
	ScanStatusMoved ScanStatus = "moved"
	// ScanStatusPresent indicates the item was already in this slot.
	ScanStatusPresent ScanStatus = "present"
)

// ScanAndAssignResult wraps the result of scanning and assigning an item.
type ScanAndAssignResult struct {
	Item   items.Item `json:"item"`
	Status ScanStatus `json:"status"`
}

// Shelf represents a physical shelf image and metadata.
type Shelf struct {
	ID          uuid.UUID `db:"id" json:"id"`
	OwnerID     uuid.UUID `db:"owner_id" json:"-"`
	Name        string    `db:"name" json:"name"`
	Description string    `db:"description" json:"description"`
	PhotoURL    string    `db:"photo_url" json:"photoUrl"`
	CreatedAt   time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt   time.Time `db:"updated_at" json:"updatedAt"`
}

// ShelfRow captures the vertical boundaries for a row in normalized coordinates.
type ShelfRow struct {
	ID         uuid.UUID `db:"id" json:"id"`
	ShelfID    uuid.UUID `db:"shelf_id" json:"shelfId"`
	RowIndex   int       `db:"row_index" json:"rowIndex"`
	YStartNorm float64   `db:"y_start_norm" json:"yStartNorm"`
	YEndNorm   float64   `db:"y_end_norm" json:"yEndNorm"`
}

// ShelfColumn captures horizontal boundaries for a column within a row.
type ShelfColumn struct {
	ID         uuid.UUID `db:"id" json:"id"`
	ShelfRowID uuid.UUID `db:"shelf_row_id" json:"shelfRowId"`
	ColIndex   int       `db:"col_index" json:"colIndex"`
	XStartNorm float64   `db:"x_start_norm" json:"xStartNorm"`
	XEndNorm   float64   `db:"x_end_norm" json:"xEndNorm"`
}

// ShelfSlot represents a grid cell derived from a row and column pair.
type ShelfSlot struct {
	ID            uuid.UUID `db:"id" json:"id"`
	ShelfID       uuid.UUID `db:"shelf_id" json:"shelfId"`
	ShelfRowID    uuid.UUID `db:"shelf_row_id" json:"shelfRowId"`
	ShelfColumnID uuid.UUID `db:"shelf_column_id" json:"shelfColumnId"`
	RowIndex      int       `db:"row_index" json:"rowIndex"`
	ColIndex      int       `db:"col_index" json:"colIndex"`
	XStartNorm    float64   `db:"x_start_norm" json:"xStartNorm"`
	XEndNorm      float64   `db:"x_end_norm" json:"xEndNorm"`
	YStartNorm    float64   `db:"y_start_norm" json:"yStartNorm"`
	YEndNorm      float64   `db:"y_end_norm" json:"yEndNorm"`
}

// ItemPlacement links an item to a shelf, optionally to a specific slot.
type ItemPlacement struct {
	ID          uuid.UUID  `db:"id" json:"id"`
	ItemID      uuid.UUID  `db:"item_id" json:"itemId"`
	ShelfID     uuid.UUID  `db:"shelf_id" json:"shelfId"`
	ShelfSlotID *uuid.UUID `db:"shelf_slot_id" json:"shelfSlotId"`
	CreatedAt   time.Time  `db:"created_at" json:"createdAt"`
}

// PlacementWithItem includes the hydrated Item for UI convenience.
type PlacementWithItem struct {
	Item      items.Item    `json:"item"`
	Placement ItemPlacement `json:"placement"`
}

// ShelfWithLayout contains the shelf metadata plus all layout and placement details.
type ShelfWithLayout struct {
	Shelf      Shelf               `json:"shelf"`
	Rows       []RowWithColumns    `json:"rows"`
	Slots      []ShelfSlot         `json:"slots"`
	Placements []PlacementWithItem `json:"placements"`
	Unplaced   []PlacementWithItem `json:"unplaced"`
}

// RowWithColumns bundles a row and its columns for transport.
type RowWithColumns struct {
	ShelfRow
	Columns []ShelfColumn `json:"columns"`
}

// ShelfSummary describes list-friendly information.
type ShelfSummary struct {
	Shelf       Shelf `json:"shelf"`
	ItemCount   int   `json:"itemCount"`
	PlacedCount int   `json:"placedCount"`
	SlotCount   int   `json:"slotCount"`
}

// LayoutSlotInput captures layout updates for a slot's bounding box.
type LayoutSlotInput struct {
	// NewSlot distinguishes a replacement from legacy inputs that omit slotId.
	NewSlot    bool       `json:"newSlot,omitempty"`
	SlotID     *uuid.UUID `json:"slotId"`
	RowIndex   int        `json:"rowIndex"`
	ColIndex   int        `json:"colIndex"`
	XStartNorm float64    `json:"xStartNorm"`
	XEndNorm   float64    `json:"xEndNorm"`
	YStartNorm float64    `json:"yStartNorm"`
	YEndNorm   float64    `json:"yEndNorm"`
}

// Repository defines persistence for shelves and layouts.
type Repository interface {
	CreateShelf(ctx context.Context, shelf Shelf, rows []ShelfRow, columns []ShelfColumn, slots []ShelfSlot) (ShelfWithLayout, error)
	ListShelves(ctx context.Context, ownerID uuid.UUID) ([]ShelfSummary, error)
	GetShelf(ctx context.Context, shelfID uuid.UUID, ownerID uuid.UUID) (ShelfWithLayout, error)
	SaveLayout(ctx context.Context, shelfID uuid.UUID, ownerID uuid.UUID, rows []ShelfRow, columns []ShelfColumn, slots []ShelfSlot, removedSlotIDs []uuid.UUID) error
	AssignItemToSlot(ctx context.Context, shelfID uuid.UUID, ownerID uuid.UUID, slotID uuid.UUID, itemID uuid.UUID) (ItemPlacement, error)
	RemoveItemFromSlot(ctx context.Context, shelfID uuid.UUID, ownerID uuid.UUID, slotID uuid.UUID, itemID uuid.UUID) error
	ListPlacements(ctx context.Context, shelfID uuid.UUID, ownerID uuid.UUID) ([]ItemPlacement, error)
	UpsertUnplaced(ctx context.Context, shelfID uuid.UUID, ownerID uuid.UUID, itemID uuid.UUID) (ItemPlacement, error)
}
