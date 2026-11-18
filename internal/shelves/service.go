package shelves

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"anthology/internal/catalog"
	"anthology/internal/items"
)

const defaultSlotMargin = 0.02

// CatalogService defines the interface for metadata lookups.
type CatalogService interface {
	Lookup(ctx context.Context, query string, category catalog.Category) ([]catalog.Metadata, error)
}

// Service coordinates layout validation and persistence.
type Service struct {
	repo        Repository
	itemsRepo   items.Repository
	catalogSvc  CatalogService
	itemService *items.Service
}

// NewService wires a shelf service.
func NewService(repo Repository, itemsRepo items.Repository, catalogSvc CatalogService, itemService *items.Service) *Service {
	return &Service{
		repo:        repo,
		itemsRepo:   itemsRepo,
		catalogSvc:  catalogSvc,
		itemService: itemService,
	}
}

// CreateShelfInput captures the fields required to create a shelf.
type CreateShelfInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	PhotoURL    string `json:"photoUrl"`
}

// UpdateLayoutInput wraps the new slots for a shelf layout.
type UpdateLayoutInput struct {
	Slots []LayoutSlotInput `json:"slots"`
}

// CreateShelf creates a shelf with an initial single-slot layout.
func (s *Service) CreateShelf(ctx context.Context, input CreateShelfInput, ownerID uuid.UUID) (ShelfWithLayout, error) {
	if ownerID == (uuid.UUID{}) {
		return ShelfWithLayout{}, fmt.Errorf("%w: ownerID is required", ErrValidation)
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return ShelfWithLayout{}, fmt.Errorf("%w: name is required", ErrValidation)
	}
	photoURL, err := sanitizePhotoURL(input.PhotoURL)
	if err != nil {
		return ShelfWithLayout{}, err
	}
	if photoURL == "" {
		return ShelfWithLayout{}, fmt.Errorf("%w: photoUrl is required", ErrValidation)
	}

	now := time.Now().UTC()
	shelf := Shelf{
		ID:          uuid.New(),
		OwnerID:     ownerID,
		Name:        name,
		Description: strings.TrimSpace(input.Description),
		PhotoURL:    photoURL,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	rowID := uuid.New()
	colID := uuid.New()
	slotID := uuid.New()

	row := ShelfRow{
		ID:         rowID,
		ShelfID:    shelf.ID,
		RowIndex:   0,
		YStartNorm: defaultSlotMargin,
		YEndNorm:   1 - defaultSlotMargin,
	}
	column := ShelfColumn{
		ID:         colID,
		ShelfRowID: rowID,
		ColIndex:   0,
		XStartNorm: defaultSlotMargin,
		XEndNorm:   1 - defaultSlotMargin,
	}
	slot := ShelfSlot{
		ID:            slotID,
		ShelfID:       shelf.ID,
		ShelfRowID:    rowID,
		ShelfColumnID: colID,
		RowIndex:      0,
		ColIndex:      0,
		XStartNorm:    defaultSlotMargin,
		XEndNorm:      1 - defaultSlotMargin,
		YStartNorm:    defaultSlotMargin,
		YEndNorm:      1 - defaultSlotMargin,
	}

	created, err := s.repo.CreateShelf(ctx, shelf, []ShelfRow{row}, []ShelfColumn{column}, []ShelfSlot{slot})
	if err != nil {
		return ShelfWithLayout{}, err
	}

	return created, nil
}

// ListShelves returns shelf summaries.
func (s *Service) ListShelves(ctx context.Context, ownerID uuid.UUID) ([]ShelfSummary, error) {
	return s.repo.ListShelves(ctx, ownerID)
}

// GetShelf returns a shelf with layout and placements hydrated with item details.
func (s *Service) GetShelf(ctx context.Context, shelfID uuid.UUID, ownerID uuid.UUID) (ShelfWithLayout, error) {
	layout, err := s.repo.GetShelf(ctx, shelfID, ownerID)
	if err != nil {
		return ShelfWithLayout{}, err
	}

	return s.attachItems(ctx, layout, ownerID)
}

// UpdateLayout replaces the layout while keeping stable slot IDs when possible.
func (s *Service) UpdateLayout(ctx context.Context, shelfID uuid.UUID, ownerID uuid.UUID, input UpdateLayoutInput) (ShelfWithLayout, []PlacementWithItem, error) {
	if input.Slots == nil {
		return ShelfWithLayout{}, nil, fmt.Errorf("%w: slots must be an array", ErrValidation)
	}

	existing, err := s.repo.GetShelf(ctx, shelfID, ownerID)
	if err != nil {
		return ShelfWithLayout{}, nil, err
	}

	slotKey := func(rowIdx, colIdx int) string {
		return fmt.Sprintf("%d-%d", rowIdx, colIdx)
	}

	rowIDs := make(map[int]uuid.UUID)
	columnIDs := make(map[string]uuid.UUID)
	slotIDs := make(map[string]uuid.UUID)
	existingSlotIDSet := make(map[uuid.UUID]struct{})
	for _, row := range existing.Rows {
		rowIDs[row.RowIndex] = row.ID
		for _, col := range row.Columns {
			columnIDs[slotKey(row.RowIndex, col.ColIndex)] = col.ID
		}
	}
	for _, slot := range existing.Slots {
		slotIDs[slotKey(slot.RowIndex, slot.ColIndex)] = slot.ID
		existingSlotIDSet[slot.ID] = struct{}{}
	}

	normalizedRows, normalizedColumns, normalizedSlots, err := normalizeSlots(input.Slots, shelfID, rowIDs, columnIDs, slotIDs, existingSlotIDSet)
	if err != nil {
		return ShelfWithLayout{}, nil, err
	}

	removedSlotIDs := removedSlots(existing.Slots, normalizedSlots)
	displacedItemIDs := make(map[uuid.UUID]struct{})
	if len(removedSlotIDs) > 0 {
		removedSet := make(map[uuid.UUID]struct{}, len(removedSlotIDs))
		for _, id := range removedSlotIDs {
			removedSet[id] = struct{}{}
		}
		for _, placement := range existing.Placements {
			if placement.Placement.ShelfSlotID == nil {
				continue
			}
			if _, removed := removedSet[*placement.Placement.ShelfSlotID]; removed {
				displacedItemIDs[placement.Placement.ItemID] = struct{}{}
			}
		}
	}
	if err := s.repo.SaveLayout(ctx, shelfID, ownerID, slices.Clone(normalizedRows), slices.Clone(normalizedColumns), normalizedSlots, removedSlotIDs); err != nil {
		return ShelfWithLayout{}, nil, err
	}

	updated, err := s.repo.GetShelf(ctx, shelfID, ownerID)
	if err != nil {
		return ShelfWithLayout{}, nil, err
	}

	hydrated, err := s.attachItems(ctx, updated, ownerID)
	if err != nil {
		return ShelfWithLayout{}, nil, err
	}

	cacheItemIDs := itemIDsFromLayout(hydrated)
	for id := range displacedItemIDs {
		cacheItemIDs = append(cacheItemIDs, id)
	}
	if err := s.updateItemPlacementCache(ctx, hydrated, cacheItemIDs); err != nil {
		return ShelfWithLayout{}, nil, err
	}

	var displaced []PlacementWithItem
	if len(displacedItemIDs) > 0 {
		original, err := s.attachItems(ctx, existing, ownerID)
		if err != nil {
			return ShelfWithLayout{}, nil, err
		}
		for _, placement := range original.Placements {
			if _, removed := displacedItemIDs[placement.Placement.ItemID]; removed {
				placement.Placement.ShelfSlotID = nil
				displaced = append(displaced, placement)
			}
		}
	}

	return hydrated, displaced, nil
}
