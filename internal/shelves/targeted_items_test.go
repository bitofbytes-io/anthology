package shelves

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"anthology/internal/items"
)

// libraryScanGuard fails the test if the shelf service loads the owner's whole
// library instead of fetching only the items it needs.
type libraryScanGuard struct {
	*items.InMemoryRepository
	t *testing.T
}

func (g libraryScanGuard) List(context.Context, items.ListOptions) ([]items.Item, error) {
	g.t.Helper()
	g.t.Fatal("shelf service must not list the whole library")
	return nil, nil
}

func TestShelfOperationsFetchOnlyNeededItems(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	repo := NewInMemoryRepository()

	shelfID, rowID, colID, slotID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	shelf := Shelf{ID: shelfID, Name: "Shelf", PhotoURL: "https://example.com/shelf.jpg", OwnerID: testOwnerID, CreatedAt: now, UpdatedAt: now}
	row := ShelfRow{ID: rowID, ShelfID: shelfID, YEndNorm: 1}
	column := ShelfColumn{ID: colID, ShelfRowID: rowID, XEndNorm: 1}
	slot := ShelfSlot{ID: slotID, ShelfID: shelfID, ShelfRowID: rowID, ShelfColumnID: colID, XEndNorm: 1, YEndNorm: 1}
	if _, err := repo.CreateShelf(ctx, shelf, []ShelfRow{row}, []ShelfColumn{column}, []ShelfSlot{slot}); err != nil {
		t.Fatalf("create shelf: %v", err)
	}

	placed := items.Item{ID: uuid.New(), OwnerID: testOwnerID, Title: "Placed", ItemType: items.ItemTypeBook, CreatedAt: now, UpdatedAt: now}
	scanned := items.Item{ID: uuid.New(), OwnerID: testOwnerID, Title: "Scanned", ItemType: items.ItemTypeBook, ISBN13: "978-0-00-000000-1", CreatedAt: now, UpdatedAt: now}
	offShelf := items.Item{ID: uuid.New(), OwnerID: testOwnerID, Title: "Elsewhere", ItemType: items.ItemTypeBook, CreatedAt: now, UpdatedAt: now}
	itemsRepo := libraryScanGuard{InMemoryRepository: items.NewInMemoryRepository([]items.Item{placed, scanned, offShelf}), t: t}
	svc := NewService(repo, itemsRepo, nil, items.NewService(itemsRepo))

	assigned, err := svc.AssignItem(ctx, shelfID, slotID, placed.ID, testOwnerID)
	if err != nil {
		t.Fatalf("assign item: %v", err)
	}
	if len(assigned.Placements) != 1 || assigned.Placements[0].Item.Title != "Placed" {
		t.Fatalf("unexpected placements after assign: %+v", assigned.Placements)
	}

	// The catalog service is nil, so this only succeeds if the existing item is
	// found by its normalized ISBN.
	result, err := svc.ScanAndAssign(ctx, shelfID, slotID, "9780000000001", testOwnerID)
	if err != nil {
		t.Fatalf("scan and assign: %v", err)
	}
	if result.Item.ID != scanned.ID || result.Status != ScanStatusMoved {
		t.Fatalf("expected existing item to be moved, got %+v", result)
	}

	got, err := svc.GetShelf(ctx, shelfID, testOwnerID)
	if err != nil {
		t.Fatalf("get shelf: %v", err)
	}
	titles := map[string]bool{}
	for _, placement := range got.Placements {
		titles[placement.Item.Title] = true
	}
	if len(got.Placements) != 2 || !titles["Placed"] || !titles["Scanned"] || titles["Elsewhere"] {
		t.Fatalf("unexpected placements: %+v", got.Placements)
	}
}
