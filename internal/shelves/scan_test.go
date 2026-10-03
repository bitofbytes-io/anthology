package shelves

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"anthology/internal/catalog"
	"anthology/internal/items"
)

type catalogStub struct {
	calls    int
	metadata []catalog.Metadata
}

func (c *catalogStub) Lookup(context.Context, string, catalog.Category) ([]catalog.Metadata, error) {
	c.calls++
	return c.metadata, nil
}

// newScanFixture creates a one-slot shelf and returns the service, shelf ID and slot ID.
func newScanFixture(t *testing.T, existing []items.Item, lookup *catalogStub) (*Service, uuid.UUID, uuid.UUID) {
	t.Helper()
	repo := NewInMemoryRepository()
	itemsRepo := items.NewInMemoryRepository(existing)
	svc := NewService(repo, itemsRepo, lookup, items.NewService(itemsRepo))
	created, err := svc.CreateShelf(context.Background(), CreateShelfInput{Name: "Shelf", PhotoURL: "https://example.com/shelf.jpg"}, testOwnerID)
	if err != nil {
		t.Fatalf("create shelf: %v", err)
	}
	return svc, created.Shelf.ID, created.Slots[0].ID
}

func TestScanAndAssignMatchesExistingBookAcrossISBNForms(t *testing.T) {
	now := time.Now().UTC()
	book := items.Item{ID: uuid.New(), OwnerID: testOwnerID, Title: "Owned", ItemType: items.ItemTypeBook, ISBN13: "9780306406157", CreatedAt: now, UpdatedAt: now}
	lookup := &catalogStub{}
	svc, shelfID, slotID := newScanFixture(t, []items.Item{book}, lookup)

	result, err := svc.ScanAndAssign(context.Background(), shelfID, slotID, "0-306-40615-2", testOwnerID)
	if err != nil {
		t.Fatalf("ScanAndAssign: %v", err)
	}
	if result.Item.ID != book.ID || result.Status != ScanStatusMoved {
		t.Fatalf("got item %s status %s; want existing %s moved", result.Item.ID, result.Status, book.ID)
	}
	if lookup.calls != 0 {
		t.Fatalf("expected no catalog lookup for an owned book, got %d", lookup.calls)
	}
}

func TestScanAndAssignRejectsNonISBN(t *testing.T) {
	lookup := &catalogStub{}
	svc, shelfID, slotID := newScanFixture(t, nil, lookup)

	_, err := svc.ScanAndAssign(context.Background(), shelfID, slotID, "012345678905", testOwnerID)
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("expected ErrValidation for a UPC, got %v", err)
	}
	if lookup.calls != 0 {
		t.Fatalf("expected no catalog lookup for an invalid ISBN, got %d", lookup.calls)
	}
}
