package items

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestInMemoryRepositoryUpdateRequiresOwner(t *testing.T) {
	repo := NewInMemoryRepository(nil)
	ctx := context.Background()

	item := Item{
		ID:       uuid.New(),
		OwnerID:  testOwnerID,
		Title:    "Original",
		ItemType: ItemTypeBook,
	}

	if _, err := repo.Create(ctx, item); err != nil {
		t.Fatalf("expected item to be created: %v", err)
	}

	updated := item
	updated.OwnerID = uuid.New()
	updated.Title = "Changed"

	if _, err := repo.Update(ctx, updated); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for owner mismatch, got %v", err)
	}

	got, err := repo.Get(ctx, item.ID, item.OwnerID)
	if err != nil {
		t.Fatalf("expected item to be retrievable: %v", err)
	}
	if got.Title != "Original" {
		t.Fatalf("expected original title to remain, got %q", got.Title)
	}
}
