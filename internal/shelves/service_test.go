package shelves

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"anthology/internal/items"
)

// testOwnerID is a fixed UUID for tests
var testOwnerID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

func TestUpdateLayoutReturnsOnlyDisplacedItems(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := NewInMemoryRepository()
	now := time.Now().UTC()

	shelfID := uuid.New()
	rowID := uuid.New()
	colLeftID := uuid.New()
	colRightID := uuid.New()
	slotLeftID := uuid.New()
	slotRightID := uuid.New()

	shelf := Shelf{
		ID:        shelfID,
		Name:      "Shelf",
		PhotoURL:  "https://example.com/shelf.jpg",
		OwnerID:   testOwnerID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	row := ShelfRow{
		ID:         rowID,
		ShelfID:    shelfID,
		RowIndex:   0,
		YStartNorm: 0,
		YEndNorm:   1,
	}
	columns := []ShelfColumn{
		{ID: colLeftID, ShelfRowID: rowID, ColIndex: 0, XStartNorm: 0, XEndNorm: 0.5},
		{ID: colRightID, ShelfRowID: rowID, ColIndex: 1, XStartNorm: 0.5, XEndNorm: 1},
	}
	slots := []ShelfSlot{
		{
			ID:            slotLeftID,
			ShelfID:       shelfID,
			ShelfRowID:    rowID,
			ShelfColumnID: colLeftID,
			RowIndex:      0,
			ColIndex:      0,
			XStartNorm:    0,
			XEndNorm:      0.5,
			YStartNorm:    0,
			YEndNorm:      1,
		},
		{
			ID:            slotRightID,
			ShelfID:       shelfID,
			ShelfRowID:    rowID,
			ShelfColumnID: colRightID,
			RowIndex:      0,
			ColIndex:      1,
			XStartNorm:    0.5,
			XEndNorm:      1,
			YStartNorm:    0,
			YEndNorm:      1,
		},
	}

	if _, err := repo.CreateShelf(ctx, shelf, []ShelfRow{row}, columns, slots); err != nil {
		t.Fatalf("create shelf: %v", err)
	}

	displacedItem := items.Item{
		ID:        uuid.New(),
		Title:     "Placed Book",
		ItemType:  items.ItemTypeBook,
		OwnerID:   testOwnerID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	preexistingUnplaced := items.Item{
		ID:        uuid.New(),
		Title:     "Already Unplaced",
		ItemType:  items.ItemTypeBook,
		OwnerID:   testOwnerID,
		CreatedAt: now,
		UpdatedAt: now,
	}

	itemsRepo := items.NewInMemoryRepository([]items.Item{displacedItem, preexistingUnplaced})
	itemSvc := items.NewService(itemsRepo)
	svc := NewService(repo, itemsRepo, nil, itemSvc)

	if _, err := repo.AssignItemToSlot(ctx, shelfID, testOwnerID, slotRightID, displacedItem.ID); err != nil {
		t.Fatalf("assign displaced item: %v", err)
	}
	if _, err := repo.UpsertUnplaced(ctx, shelfID, testOwnerID, preexistingUnplaced.ID); err != nil {
		t.Fatalf("seed unplaced item: %v", err)
	}

	input := UpdateLayoutInput{
		Slots: []LayoutSlotInput{
			{
				SlotID:     &slotLeftID,
				RowIndex:   0,
				ColIndex:   0,
				XStartNorm: 0,
				XEndNorm:   0.5,
				YStartNorm: 0,
				YEndNorm:   1,
			},
		},
	}

	updated, displaced, err := svc.UpdateLayout(ctx, shelfID, testOwnerID, input)
	if err != nil {
		t.Fatalf("update layout: %v", err)
	}

	if len(displaced) != 1 {
		t.Fatalf("expected 1 displaced item, got %d", len(displaced))
	}
	if displaced[0].Item.ID != displacedItem.ID {
		t.Fatalf("expected displaced item %s, got %s", displacedItem.ID, displaced[0].Item.ID)
	}

	if len(updated.Unplaced) != 1 || updated.Unplaced[0].Item.ID != preexistingUnplaced.ID {
		t.Fatal("only the preexisting unplaced item should remain on the shelf")
	}
	item, err := itemsRepo.Get(ctx, displacedItem.ID, testOwnerID)
	if err != nil || item.ShelfPlacement != nil {
		t.Fatalf("displaced book should remain in library without cached placement: %+v, %v", item, err)
	}
}

func TestAssignItemUpdatesItemPlacementInMemoryRepo(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := NewInMemoryRepository()
	now := time.Now().UTC()

	shelfID := uuid.New()
	rowID := uuid.New()
	colID := uuid.New()
	slotID := uuid.New()

	shelf := Shelf{
		ID:        shelfID,
		Name:      "Demo Shelf",
		PhotoURL:  "https://example.com/shelf.jpg",
		OwnerID:   testOwnerID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	row := ShelfRow{ID: rowID, ShelfID: shelfID, RowIndex: 0, YStartNorm: 0, YEndNorm: 1}
	column := ShelfColumn{ID: colID, ShelfRowID: rowID, ColIndex: 0, XStartNorm: 0, XEndNorm: 1}
	slot := ShelfSlot{
		ID:            slotID,
		ShelfID:       shelfID,
		ShelfRowID:    rowID,
		ShelfColumnID: colID,
		RowIndex:      0,
		ColIndex:      0,
		XStartNorm:    0,
		XEndNorm:      1,
		YStartNorm:    0,
		YEndNorm:      1,
	}

	if _, err := repo.CreateShelf(ctx, shelf, []ShelfRow{row}, []ShelfColumn{column}, []ShelfSlot{slot}); err != nil {
		t.Fatalf("create shelf: %v", err)
	}

	item := items.Item{ID: uuid.New(), Title: "Book", ItemType: items.ItemTypeBook, OwnerID: testOwnerID, CreatedAt: now, UpdatedAt: now}
	itemsRepo := items.NewInMemoryRepository([]items.Item{item})
	itemSvc := items.NewService(itemsRepo)
	svc := NewService(repo, itemsRepo, nil, itemSvc)

	if _, err := svc.AssignItem(ctx, shelfID, slotID, item.ID, testOwnerID); err != nil {
		t.Fatalf("assign item: %v", err)
	}

	stored, err := itemsRepo.Get(ctx, item.ID, testOwnerID)
	if err != nil {
		t.Fatalf("get item: %v", err)
	}
	if stored.ShelfPlacement == nil {
		t.Fatalf("expected shelf placement to be set")
	}
	if stored.ShelfPlacement.ShelfID != shelfID || stored.ShelfPlacement.SlotID != slotID {
		t.Fatalf("shelf placement not updated")
	}
	if stored.ShelfPlacement.RowIndex != slot.RowIndex || stored.ShelfPlacement.ColIndex != slot.ColIndex {
		t.Fatalf("expected row/col to match slot")
	}

	if _, err := svc.RemoveItem(ctx, shelfID, slotID, item.ID, testOwnerID); err != nil {
		t.Fatalf("remove item: %v", err)
	}
	stored, err = itemsRepo.Get(ctx, item.ID, testOwnerID)
	if err != nil {
		t.Fatalf("get item after remove: %v", err)
	}
	if stored.ShelfPlacement != nil {
		t.Fatalf("expected shelf placement to be cleared after removal")
	}
}

func TestUpdateLayoutRejectsSlotIDFromDifferentShelf(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := NewInMemoryRepository()
	now := time.Now().UTC()

	shelfID := uuid.New()
	rowID := uuid.New()
	colID := uuid.New()
	slotID := uuid.New()

	shelf := Shelf{
		ID:        shelfID,
		Name:      "Shelf",
		PhotoURL:  "https://example.com/shelf.jpg",
		OwnerID:   testOwnerID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	row := ShelfRow{ID: rowID, ShelfID: shelfID, RowIndex: 0, YStartNorm: 0, YEndNorm: 1}
	column := ShelfColumn{ID: colID, ShelfRowID: rowID, ColIndex: 0, XStartNorm: 0, XEndNorm: 1}
	slot := ShelfSlot{
		ID:            slotID,
		ShelfID:       shelfID,
		ShelfRowID:    rowID,
		ShelfColumnID: colID,
		RowIndex:      0,
		ColIndex:      0,
		XStartNorm:    0,
		XEndNorm:      1,
		YStartNorm:    0,
		YEndNorm:      1,
	}

	if _, err := repo.CreateShelf(ctx, shelf, []ShelfRow{row}, []ShelfColumn{column}, []ShelfSlot{slot}); err != nil {
		t.Fatalf("create shelf: %v", err)
	}

	itemsRepo := items.NewInMemoryRepository(nil)
	itemSvc := items.NewService(itemsRepo)
	svc := NewService(repo, itemsRepo, nil, itemSvc)

	foreignSlotID := uuid.New()
	_, _, err := svc.UpdateLayout(ctx, shelfID, testOwnerID, UpdateLayoutInput{
		Slots: []LayoutSlotInput{
			{
				SlotID:     &foreignSlotID,
				RowIndex:   0,
				ColIndex:   0,
				XStartNorm: 0,
				XEndNorm:   1,
				YStartNorm: 0,
				YEndNorm:   1,
			},
		},
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}
