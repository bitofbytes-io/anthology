package shelves

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"anthology/internal/catalog"
	"anthology/internal/items"
)

const (
	defaultSlotMargin = 0.02

	// maxPhotoURLBytes limits the decoded size of data URI photos.
	maxPhotoURLBytes = 5 * 1024 * 1024 // 5MB
	// maxPhotoURLLength limits the character length of non-data-URI URLs.
	maxPhotoURLLength = 4096
)

// allowedImageMIMETypes lists permitted MIME types for data URI images.
var allowedImageMIMETypes = map[string]bool{
	"image/jpeg":    true,
	"image/png":     true,
	"image/gif":     true,
	"image/webp":    true,
	"image/svg+xml": true,
}

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

type placementCacheUpdater interface {
	UpdateShelfPlacement(ctx context.Context, itemID uuid.UUID, placement *items.ShelfPlacement) error
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

// AssignItem assigns an item to a slot, clearing any previous placement on the shelf.
func (s *Service) AssignItem(ctx context.Context, shelfID, slotID, itemID uuid.UUID, ownerID uuid.UUID) (ShelfWithLayout, error) {
	if _, err := s.itemsRepo.Get(ctx, itemID, ownerID); err != nil {
		return ShelfWithLayout{}, err
	}

	if _, err := s.repo.AssignItemToSlot(ctx, shelfID, ownerID, slotID, itemID); err != nil {
		return ShelfWithLayout{}, err
	}

	updated, err := s.repo.GetShelf(ctx, shelfID, ownerID)
	if err != nil {
		return ShelfWithLayout{}, err
	}

	hydrated, err := s.attachItems(ctx, updated, ownerID)
	if err != nil {
		return ShelfWithLayout{}, err
	}

	if err := s.updateItemPlacementCache(ctx, hydrated, []uuid.UUID{itemID}); err != nil {
		return ShelfWithLayout{}, err
	}

	return hydrated, nil
}

// RemoveItem removes an item from a slot, leaving it unplaced on the shelf.
func (s *Service) RemoveItem(ctx context.Context, shelfID, slotID, itemID uuid.UUID, ownerID uuid.UUID) (ShelfWithLayout, error) {
	if err := s.repo.RemoveItemFromSlot(ctx, shelfID, ownerID, slotID, itemID); err != nil {
		return ShelfWithLayout{}, err
	}

	updated, err := s.repo.GetShelf(ctx, shelfID, ownerID)
	if err != nil {
		return ShelfWithLayout{}, err
	}

	hydrated, err := s.attachItems(ctx, updated, ownerID)
	if err != nil {
		return ShelfWithLayout{}, err
	}

	if err := s.updateItemPlacementCache(ctx, hydrated, []uuid.UUID{itemID}); err != nil {
		return ShelfWithLayout{}, err
	}

	return hydrated, nil
}

func removedSlots(previous, next []ShelfSlot) []uuid.UUID {
	nextSet := make(map[uuid.UUID]struct{}, len(next))
	for _, slot := range next {
		nextSet[slot.ID] = struct{}{}
	}

	var removed []uuid.UUID
	for _, slot := range previous {
		if _, exists := nextSet[slot.ID]; !exists {
			removed = append(removed, slot.ID)
		}
	}
	return removed
}

func normalizeSlots(
	slots []LayoutSlotInput,
	shelfID uuid.UUID,
	existingRowIDs map[int]uuid.UUID,
	existingColumnIDs map[string]uuid.UUID,
	existingSlotIDs map[string]uuid.UUID,
	existingSlotIDSet map[uuid.UUID]struct{},
) ([]ShelfRow, []ShelfColumn, []ShelfSlot, error) {
	if len(slots) == 0 {
		return []ShelfRow{}, []ShelfColumn{}, []ShelfSlot{}, nil
	}

	key := func(rowIdx, colIdx int) string {
		return fmt.Sprintf("%d-%d", rowIdx, colIdx)
	}

	rowGroups := make(map[int][]LayoutSlotInput)
	seenKeys := make(map[string]struct{})
	explicitIDs := make(map[uuid.UUID]struct{})
	for _, slot := range slots {
		if slot.SlotID != nil {
			if slot.NewSlot {
				return nil, nil, nil, fmt.Errorf("%w: newSlot cannot include slotId", ErrValidation)
			}
			if _, exists := explicitIDs[*slot.SlotID]; exists {
				return nil, nil, nil, fmt.Errorf("%w: duplicate slotId", ErrValidation)
			}
			explicitIDs[*slot.SlotID] = struct{}{}
		}
	}

	for _, slot := range slots {
		if slot.RowIndex < 0 || slot.ColIndex < 0 {
			return nil, nil, nil, fmt.Errorf("%w: row and column indexes must be non-negative", ErrValidation)
		}
		if slot.XStartNorm < 0 || slot.XEndNorm > 1 || slot.XEndNorm <= slot.XStartNorm {
			return nil, nil, nil, fmt.Errorf("%w: slot %d/%d has invalid x boundaries", ErrValidation, slot.RowIndex, slot.ColIndex)
		}
		if slot.YStartNorm < 0 || slot.YEndNorm > 1 || slot.YEndNorm <= slot.YStartNorm {
			return nil, nil, nil, fmt.Errorf("%w: slot %d/%d has invalid y boundaries", ErrValidation, slot.RowIndex, slot.ColIndex)
		}
		slotKey := key(slot.RowIndex, slot.ColIndex)
		if _, exists := seenKeys[slotKey]; exists {
			return nil, nil, nil, fmt.Errorf("%w: duplicate definition for row %d column %d", ErrValidation, slot.RowIndex, slot.ColIndex)
		}
		seenKeys[slotKey] = struct{}{}
		rowGroups[slot.RowIndex] = append(rowGroups[slot.RowIndex], slot)
	}

	rowIndexes := make([]int, 0, len(rowGroups))
	for idx := range rowGroups {
		rowIndexes = append(rowIndexes, idx)
	}
	sort.Ints(rowIndexes)

	rows := make([]ShelfRow, 0, len(rowIndexes))
	columns := make([]ShelfColumn, 0, len(slots))
	normalizedSlots := make([]ShelfSlot, 0, len(slots))

	for _, rowIdx := range rowIndexes {
		rowSlots := rowGroups[rowIdx]
		sort.Slice(rowSlots, func(i, j int) bool { return rowSlots[i].ColIndex < rowSlots[j].ColIndex })

		rowYStart := rowSlots[0].YStartNorm
		rowYEnd := rowSlots[0].YEndNorm
		for _, slot := range rowSlots[1:] {
			if slot.YStartNorm < rowYStart {
				rowYStart = slot.YStartNorm
			}
			if slot.YEndNorm > rowYEnd {
				rowYEnd = slot.YEndNorm
			}
		}

		rowID, ok := existingRowIDs[rowIdx]
		if !ok {
			rowID = uuid.New()
		}
		rows = append(rows, ShelfRow{
			ID:         rowID,
			ShelfID:    shelfID,
			RowIndex:   rowIdx,
			YStartNorm: rowYStart,
			YEndNorm:   rowYEnd,
		})

		for _, slot := range rowSlots {
			colKey := key(rowIdx, slot.ColIndex)
			colID, ok := existingColumnIDs[colKey]
			if !ok {
				colID = uuid.New()
			}

			columns = append(columns, ShelfColumn{
				ID:         colID,
				ShelfRowID: rowID,
				ColIndex:   slot.ColIndex,
				XStartNorm: slot.XStartNorm,
				XEndNorm:   slot.XEndNorm,
			})

			slotID := uuid.New()
			if slot.SlotID != nil {
				if _, ok := existingSlotIDSet[*slot.SlotID]; !ok {
					return nil, nil, nil, fmt.Errorf("%w: slotId %s does not belong to this shelf", ErrValidation, slot.SlotID.String())
				}
				slotID = *slot.SlotID
			} else if existingID, ok := existingSlotIDs[colKey]; ok && !slot.NewSlot {
				// Legacy clients omit IDs for unchanged slots. Do not steal an ID
				// explicitly retained elsewhere in a reordered layout.
				if _, reserved := explicitIDs[existingID]; !reserved {
					slotID = existingID
				}
			}

			normalizedSlots = append(normalizedSlots, ShelfSlot{
				ID:            slotID,
				ShelfID:       shelfID,
				ShelfRowID:    rowID,
				ShelfColumnID: colID,
				RowIndex:      rowIdx,
				ColIndex:      slot.ColIndex,
				XStartNorm:    slot.XStartNorm,
				XEndNorm:      slot.XEndNorm,
				YStartNorm:    slot.YStartNorm,
				YEndNorm:      slot.YEndNorm,
			})
		}
	}

	return rows, columns, normalizedSlots, nil
}

func (s *Service) attachItems(ctx context.Context, layout ShelfWithLayout, ownerID uuid.UUID) (ShelfWithLayout, error) {
	itemsList, err := s.itemsRepo.List(ctx, items.ListOptions{OwnerID: ownerID})
	if err != nil {
		return ShelfWithLayout{}, err
	}
	itemMap := make(map[uuid.UUID]items.Item, len(itemsList))
	for _, item := range itemsList {
		itemMap[item.ID] = item
	}

	placements := make([]PlacementWithItem, 0)
	unplaced := make([]PlacementWithItem, 0)
	if layout.Rows == nil {
		layout.Rows = []RowWithColumns{}
	}
	if layout.Slots == nil {
		layout.Slots = []ShelfSlot{}
	}
	for _, placement := range layout.Placements {
		item, ok := itemMap[placement.Placement.ItemID]
		if !ok {
			continue
		}
		enriched := PlacementWithItem{Item: item, Placement: placement.Placement}
		if placement.Placement.ShelfSlotID == nil {
			unplaced = append(unplaced, enriched)
		} else {
			placements = append(placements, enriched)
		}
	}

	layout.Placements = placements
	layout.Unplaced = unplaced
	return layout, nil
}

func (s *Service) updateItemPlacementCache(ctx context.Context, layout ShelfWithLayout, itemIDs []uuid.UUID) error {
	updater, ok := s.itemsRepo.(placementCacheUpdater)
	if !ok || len(itemIDs) == 0 {
		return nil
	}

	slotLookup := make(map[uuid.UUID]ShelfSlot, len(layout.Slots))
	for _, slot := range layout.Slots {
		slotLookup[slot.ID] = slot
	}

	placementByItem := make(map[uuid.UUID]*items.ShelfPlacement, len(layout.Placements))
	for _, placement := range layout.Placements {
		if placement.Placement.ShelfSlotID == nil {
			continue
		}
		slot, ok := slotLookup[*placement.Placement.ShelfSlotID]
		if !ok {
			continue
		}
		slotID := *placement.Placement.ShelfSlotID
		placementCopy := items.ShelfPlacement{
			ShelfID:   layout.Shelf.ID,
			ShelfName: layout.Shelf.Name,
			SlotID:    slotID,
			RowIndex:  slot.RowIndex,
			ColIndex:  slot.ColIndex,
		}
		placementByItem[placement.Placement.ItemID] = &placementCopy
	}

	for _, id := range itemIDs {
		if err := updater.UpdateShelfPlacement(ctx, id, placementByItem[id]); err != nil {
			return err
		}
	}

	return nil
}

func itemIDsFromLayout(layout ShelfWithLayout) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{})
	var ids []uuid.UUID
	for _, placement := range layout.Placements {
		itemID := placement.Placement.ItemID
		if _, ok := seen[itemID]; ok {
			continue
		}
		seen[itemID] = struct{}{}
		ids = append(ids, itemID)
	}
	for _, placement := range layout.Unplaced {
		itemID := placement.Placement.ItemID
		if _, ok := seen[itemID]; ok {
			continue
		}
		seen[itemID] = struct{}{}
		ids = append(ids, itemID)
	}
	return ids
}

// sanitizePhotoURL validates and normalizes a photo URL or data URI.
func sanitizePhotoURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}

	if strings.HasPrefix(trimmed, "data:") {
		parts := strings.SplitN(trimmed, ",", 2)
		if len(parts) != 2 {
			return "", fmt.Errorf("%w: photoUrl data URI is invalid", ErrValidation)
		}

		// Extract and validate MIME type from the data URI header (e.g., "data:image/png;base64")
		header := parts[0]
		mimeType := strings.TrimPrefix(header, "data:")
		mimeType = strings.TrimSuffix(mimeType, ";base64")
		mimeType = strings.ToLower(mimeType)
		if !allowedImageMIMETypes[mimeType] {
			return "", fmt.Errorf("%w: photoUrl must be a valid image type (JPEG, PNG, GIF, WebP, or SVG)", ErrValidation)
		}

		if _, err := base64.StdEncoding.DecodeString(parts[1]); err != nil {
			return "", fmt.Errorf("%w: photoUrl must contain valid base64 image data", ErrValidation)
		}

		estimatedBytes := len(parts[1]) * 3 / 4
		if estimatedBytes > maxPhotoURLBytes {
			return "", fmt.Errorf("%w: photoUrl must be smaller than %dMB", ErrValidation, maxPhotoURLBytes/(1024*1024))
		}

		return trimmed, nil
	}

	if len(trimmed) > maxPhotoURLLength {
		return "", fmt.Errorf("%w: photoUrl must be shorter than %d characters", ErrValidation, maxPhotoURLLength)
	}

	// Validate external URL: must be valid URL with https scheme
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("%w: photoUrl must be a valid URL", ErrValidation)
	}
	if parsed.Scheme != "https" {
		return "", fmt.Errorf("%w: photoUrl must use HTTPS", ErrValidation)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("%w: photoUrl must have a valid host", ErrValidation)
	}

	return trimmed, nil
}

// ScanAndAssign scans an ISBN, creates the item if needed, and assigns it to a slot.
func (s *Service) ScanAndAssign(ctx context.Context, shelfID, slotID uuid.UUID, isbn string, ownerID uuid.UUID) (ScanAndAssignResult, error) {
	isbn = strings.TrimSpace(isbn)
	if isbn == "" {
		return ScanAndAssignResult{}, fmt.Errorf("%w: isbn is required", ErrValidation)
	}

	// Verify shelf and slot exist
	shelf, err := s.repo.GetShelf(ctx, shelfID, ownerID)
	if err != nil {
		return ScanAndAssignResult{}, err
	}

	slotExists := false
	for _, slot := range shelf.Slots {
		if slot.ID == slotID {
			slotExists = true
			break
		}
	}
	if !slotExists {
		return ScanAndAssignResult{}, ErrSlotNotFound
	}

	// Check if item already exists with this ISBN
	existingItems, err := s.itemsRepo.List(ctx, items.ListOptions{OwnerID: ownerID})
	if err != nil {
		return ScanAndAssignResult{}, err
	}

	var existingItem *items.Item
	for _, item := range existingItems {
		if item.ISBN13 == isbn || item.ISBN10 == isbn {
			existingItem = &item
			break
		}
	}

	var itemID uuid.UUID
	status := ScanStatusCreated

	if existingItem != nil {
		// Item exists - check if it's already in this slot
		itemID = existingItem.ID
		if existingItem.ShelfPlacement != nil &&
			existingItem.ShelfPlacement.ShelfID == shelfID &&
			existingItem.ShelfPlacement.SlotID == slotID {
			status = ScanStatusPresent
			return ScanAndAssignResult{
				Item:   *existingItem,
				Status: status,
			}, nil
		}
		status = ScanStatusMoved
	} else {
		// Item doesn't exist - lookup metadata and create it
		metadata, err := s.catalogSvc.Lookup(ctx, isbn, catalog.CategoryBook)
		if err != nil {
			if errors.Is(err, catalog.ErrNotFound) || errors.Is(err, catalog.ErrInvalidQuery) {
				return ScanAndAssignResult{}, ErrISBNNotFound
			}
			return ScanAndAssignResult{}, fmt.Errorf("failed to lookup ISBN: %w", err)
		}

		if len(metadata) == 0 {
			return ScanAndAssignResult{}, ErrISBNNotFound
		}

		// Use the first result
		meta := metadata[0]

		// Create the item
		createInput := items.CreateItemInput{
			OwnerID:        ownerID,
			Title:          meta.Title,
			Creator:        meta.Creator,
			ItemType:       items.ItemType(meta.ItemType),
			ReleaseYear:    meta.ReleaseYear,
			PageCount:      meta.PageCount,
			ISBN13:         meta.ISBN13,
			ISBN10:         meta.ISBN10,
			Description:    meta.Description,
			CoverImage:     meta.CoverImage,
			Notes:          meta.Notes,
			Genre:          items.Genre(meta.Genre),
			RetailPriceUsd: meta.RetailPriceUsd,
			GoogleVolumeId: meta.GoogleVolumeId,
		}

		newItem, err := s.itemService.Create(ctx, createInput)
		if err != nil {
			return ScanAndAssignResult{}, fmt.Errorf("failed to create item: %w", err)
		}

		itemID = newItem.ID
	}

	// Assign item to slot
	if _, err := s.repo.AssignItemToSlot(ctx, shelfID, ownerID, slotID, itemID); err != nil {
		return ScanAndAssignResult{}, err
	}

	// Get the updated shelf
	updated, err := s.repo.GetShelf(ctx, shelfID, ownerID)
	if err != nil {
		return ScanAndAssignResult{}, err
	}

	hydrated, err := s.attachItems(ctx, updated, ownerID)
	if err != nil {
		return ScanAndAssignResult{}, err
	}

	if err := s.updateItemPlacementCache(ctx, hydrated, []uuid.UUID{itemID}); err != nil {
		return ScanAndAssignResult{}, err
	}

	// Get the final item to return
	finalItem, err := s.itemsRepo.Get(ctx, itemID, ownerID)
	if err != nil {
		return ScanAndAssignResult{}, err
	}

	return ScanAndAssignResult{
		Item:   finalItem,
		Status: status,
	}, nil
}
