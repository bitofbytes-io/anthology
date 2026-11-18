package shelves

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"anthology/internal/catalog"
	"anthology/internal/items"
)

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

// ScanAndAssign scans an ISBN, creates the item if needed, and assigns it to a slot.
func (s *Service) ScanAndAssign(ctx context.Context, shelfID, slotID uuid.UUID, isbn string, ownerID uuid.UUID) (ScanAndAssignResult, error) {
	isbn = strings.TrimSpace(isbn)
	if isbn == "" {
		return ScanAndAssignResult{}, fmt.Errorf("%w: isbn is required", ErrValidation)
	}
	if !items.IsISBN(isbn) {
		return ScanAndAssignResult{}, fmt.Errorf("%w: isbn must be an ISBN-10 or ISBN-13", ErrValidation)
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
	var existingItem *items.Item
	found, err := s.itemsRepo.FindByISBN(ctx, isbn, ownerID)
	switch {
	case err == nil:
		existingItem = &found
	case !errors.Is(err, items.ErrNotFound):
		return ScanAndAssignResult{}, err
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
