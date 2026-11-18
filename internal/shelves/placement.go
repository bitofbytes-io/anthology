package shelves

import (
	"context"

	"github.com/google/uuid"

	"anthology/internal/items"
)

type placementCacheUpdater interface {
	UpdateShelfPlacement(ctx context.Context, itemID uuid.UUID, placement *items.ShelfPlacement) error
}

func (s *Service) attachItems(ctx context.Context, layout ShelfWithLayout, ownerID uuid.UUID) (ShelfWithLayout, error) {
	itemIDs := make([]uuid.UUID, 0, len(layout.Placements))
	for _, placement := range layout.Placements {
		itemIDs = append(itemIDs, placement.Placement.ItemID)
	}
	itemsList, err := s.itemsRepo.ListByIDs(ctx, itemIDs, ownerID)
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
