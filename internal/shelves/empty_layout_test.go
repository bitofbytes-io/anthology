package shelves

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"anthology/internal/items"
	"github.com/google/uuid"
)

func TestEmptyLayoutKeepsShelfBooksAndUnplacedItems(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	shelf := Shelf{ID: uuid.New(), OwnerID: testOwnerID, Name: "Empty layout", PhotoURL: "https://example.com/shelf.jpg", CreatedAt: now, UpdatedAt: now}
	repo := NewInMemoryRepository()
	rows, cols, slots, err := normalizeSlots([]LayoutSlotInput{{XEndNorm: 1, YEndNorm: 1}}, shelf.ID, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.CreateShelf(ctx, shelf, rows, cols, slots); err != nil {
		t.Fatal(err)
	}
	books := []items.Item{{ID: uuid.New(), OwnerID: testOwnerID, Title: "Placed", ItemType: items.ItemTypeBook}, {ID: uuid.New(), OwnerID: testOwnerID, Title: "Unplaced", ItemType: items.ItemTypeBook}}
	itemRepo := items.NewInMemoryRepository(books)
	svc := NewService(repo, itemRepo, nil, items.NewService(itemRepo))
	if _, err = svc.AssignItem(ctx, shelf.ID, slots[0].ID, books[0].ID, testOwnerID); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.UpsertUnplaced(ctx, shelf.ID, testOwnerID, books[1].ID); err != nil {
		t.Fatal(err)
	}
	empty, displaced, err := svc.UpdateLayout(ctx, shelf.ID, testOwnerID, UpdateLayoutInput{Slots: []LayoutSlotInput{}})
	if err != nil {
		t.Fatal(err)
	}
	if empty.Shelf.ID != shelf.ID || len(empty.Rows) != 0 || len(empty.Slots) != 0 || len(empty.Placements) != 0 || len(empty.Unplaced) != 1 || len(displaced) != 1 {
		t.Fatalf("unexpected empty result %+v / %v", empty, displaced)
	}
	book, err := itemRepo.Get(ctx, books[0].ID, testOwnerID)
	if err != nil || book.ShelfPlacement != nil {
		t.Fatalf("lost book or stale placement: %+v %v", book, err)
	}
	persisted, err := svc.GetShelf(ctx, shelf.ID, testOwnerID)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(persisted)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]json.RawMessage
	if err = json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"rows", "slots", "placements"} {
		if string(result[field]) != "[]" {
			t.Fatalf("%s must serialize as [], got %s", field, result[field])
		}
	}
	rebuilt, _, err := svc.UpdateLayout(ctx, shelf.ID, testOwnerID, UpdateLayoutInput{Slots: []LayoutSlotInput{{NewSlot: true, XEndNorm: 1, YEndNorm: 1}}})
	if err != nil || len(rebuilt.Slots) != 1 || len(rebuilt.Unplaced) != 1 || len(rebuilt.Placements) != 0 {
		t.Fatalf("failed rebuild: %+v %v", rebuilt, err)
	}
}
