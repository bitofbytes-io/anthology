package shelves

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"anthology/internal/items"
	"anthology/internal/platform/migrate"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// ANTHOLOGY_TEST_DATABASE_URL must name a dedicated, disposable test database.
func TestPostgresLayoutRemovalPreservesSurvivors(t *testing.T) {
	dsn := os.Getenv("ANTHOLOGY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set ANTHOLOGY_TEST_DATABASE_URL for PostgreSQL regression tests")
	}
	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	ctx := context.Background()
	if err := migrate.Apply(ctx, db, nil); err != nil {
		t.Fatal(err)
	}
	for _, dimension := range []string{"row", "column"} {
		for removedIndex := 0; removedIndex < 3; removedIndex++ {
			t.Run(fmt.Sprintf("%s%d", dimension, removedIndex), func(t *testing.T) {
				owner := uuid.New()
				if _, err := db.Exec(`INSERT INTO users(id,email,oauth_provider,oauth_provider_id) VALUES($1,$2,'test',$2)`, owner, owner.String()); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM users WHERE id=$1`, owner) })
				now := time.Now().UTC()
				shelf := Shelf{ID: uuid.New(), OwnerID: owner, Name: "Test", PhotoURL: "https://example.com/shelf.jpg", CreatedAt: now, UpdatedAt: now}
				var inputs []LayoutSlotInput
				for i := 0; i < 3; i++ {
					row, col := 0, i
					if dimension == "row" {
						row, col = i, 0
					}
					inputs = append(inputs, LayoutSlotInput{RowIndex: row, ColIndex: col, XStartNorm: 0, YStartNorm: 0, XEndNorm: 1, YEndNorm: 1})
				}
				rows, cols, slots, err := normalizeSlots(inputs, shelf.ID, nil, nil, nil, nil)
				if err != nil {
					t.Fatal(err)
				}
				repo := NewPostgresRepository(db)
				if _, err = repo.CreateShelf(ctx, shelf, rows, cols, slots); err != nil {
					t.Fatal(err)
				}
				books := make([]items.Item, 4)
				for i := range books {
					books[i] = items.Item{ID: uuid.New(), OwnerID: owner, Title: fmt.Sprintf("Book %d", i), ItemType: items.ItemTypeBook, CreatedAt: now, UpdatedAt: now}
					if _, err = db.Exec(`INSERT INTO items(id,title,item_type,owner_id,created_at,updated_at) VALUES($1,$2,'book',$3,$4,$4)`, books[i].ID, books[i].Title, owner, now); err != nil {
						t.Fatal(err)
					}
					if i < 3 {
						if _, err = repo.AssignItemToSlot(ctx, shelf.ID, owner, slots[i].ID, books[i].ID); err != nil {
							t.Fatal(err)
						}
					} else {
						if _, err = repo.UpsertUnplaced(ctx, shelf.ID, owner, books[i].ID); err != nil {
							t.Fatal(err)
						}
					}
				}
				// The memory item cache exercises the same placement-cache interface used by the service.
				itemRepo := items.NewInMemoryRepository(books)
				service := NewService(repo, itemRepo, nil, items.NewService(itemRepo))
				before, err := service.GetShelf(ctx, shelf.ID, owner)
				if err != nil {
					t.Fatal(err)
				}
				if err = service.updateItemPlacementCache(ctx, before, itemIDsFromLayout(before)); err != nil {
					t.Fatal(err)
				}
				var next []LayoutSlotInput
				for i, slot := range slots {
					if i == removedIndex {
						continue
					}
					row, col := 0, len(next)
					if dimension == "row" {
						row, col = len(next), 0
					}
					next = append(next, LayoutSlotInput{SlotID: &slot.ID, RowIndex: row, ColIndex: col, XStartNorm: 0, YStartNorm: 0, XEndNorm: 1, YEndNorm: 1})
				}
				after, displaced, err := service.UpdateLayout(ctx, shelf.ID, owner, UpdateLayoutInput{Slots: next})
				if err != nil {
					t.Fatal(err)
				}
				if len(after.Slots) != 2 || len(after.Placements) != 2 || len(after.Unplaced) != 1 || len(displaced) != 1 {
					t.Fatalf("unexpected layout: %+v displaced=%v", after, displaced)
				}
				for _, p := range after.Placements {
					if p.Item.ID == books[removedIndex].ID {
						t.Fatal("removed book still placed")
					}
				}
				book, err := itemRepo.Get(ctx, books[removedIndex].ID, owner)
				if err != nil || book.ShelfPlacement != nil {
					t.Fatalf("stale cache: %+v %v", book, err)
				}
				var count int
				if err = db.Get(&count, `SELECT count(*) FROM items WHERE owner_id=$1`, owner); err != nil || count != 4 {
					t.Fatalf("catalog changed: %d %v", count, err)
				}
				if err = db.Get(&count, `SELECT count(*) FROM item_shelf_locations WHERE item_id=$1`, books[removedIndex].ID); err != nil || count != 0 {
					t.Fatalf("placement was not deleted: %d %v", count, err)
				}
				// A later SQL error must roll back earlier placement deletion.
				badSlots := append([]ShelfSlot(nil), after.Slots...)
				badSlots[0].ShelfRowID = uuid.New()
				afterRows, afterCols := []ShelfRow{}, []ShelfColumn{}
				for _, r := range after.Rows {
					afterRows = append(afterRows, r.ShelfRow)
					afterCols = append(afterCols, r.Columns...)
				}
				if err = repo.SaveLayout(ctx, shelf.ID, owner, afterRows, afterCols, badSlots, []uuid.UUID{after.Slots[1].ID}); err == nil {
					t.Fatal("expected foreign key error")
				}
				placements, err := repo.ListPlacements(ctx, shelf.ID, owner)
				if err != nil || len(placements) != 3 {
					t.Fatalf("rollback lost placements: %v %v", placements, err)
				}
				// Remove every remaining slot, including the final row/column.
				empty, detached, err := service.UpdateLayout(ctx, shelf.ID, owner, UpdateLayoutInput{Slots: []LayoutSlotInput{}})
				if err != nil {
					t.Fatal(err)
				}
				if len(empty.Slots) != 0 || len(empty.Rows) != 0 || len(empty.Placements) != 0 || len(empty.Unplaced) != 1 || len(detached) != 2 {
					t.Fatalf("unexpected empty layout: %+v, detached=%v", empty, detached)
				}
				if err = db.Get(&count, `SELECT count(*) FROM items WHERE owner_id=$1`, owner); err != nil || count != 4 {
					t.Fatalf("catalog changed after clear: %d %v", count, err)
				}
				for _, book := range books[:3] {
					cached, err := itemRepo.Get(ctx, book.ID, owner)
					if err != nil || cached.ShelfPlacement != nil {
						t.Fatalf("stale empty-layout cache: %+v %v", cached, err)
					}
				}
				rebuilt, _, err := service.UpdateLayout(ctx, shelf.ID, owner, UpdateLayoutInput{Slots: []LayoutSlotInput{{NewSlot: true, XEndNorm: 1, YEndNorm: 1}}})
				if err != nil || len(rebuilt.Slots) != 1 || len(rebuilt.Placements) != 0 || len(rebuilt.Unplaced) != 1 {
					t.Fatalf("cannot rebuild cleared shelf: %+v %v", rebuilt, err)
				}

			})
		}
	}
}

func TestNormalizeSlotsRejectsDuplicateAndForeignIDs(t *testing.T) {
	existing := uuid.New()
	for _, foreign := range []bool{false, true} {
		id := existing
		if foreign {
			id = uuid.New()
		}
		slots := []LayoutSlotInput{{SlotID: &id, XEndNorm: 1, YEndNorm: 1}, {SlotID: &id, ColIndex: 1, XEndNorm: 1, YEndNorm: 1}}
		if foreign {
			slots = slots[:1]
		}
		_, _, _, err := normalizeSlots(slots, uuid.New(), nil, nil, nil, map[uuid.UUID]struct{}{existing: {}})
		if err == nil {
			t.Fatal("expected invalid slot IDs to be rejected")
		}
	}
}

func TestLegacyAndNewSlotIdentity(t *testing.T) {
	old := uuid.New()
	for _, isNew := range []bool{false, true} {
		_, _, slots, err := normalizeSlots([]LayoutSlotInput{{NewSlot: isNew, XEndNorm: 1, YEndNorm: 1}}, uuid.New(), nil, nil, map[string]uuid.UUID{"0-0": old}, map[uuid.UUID]struct{}{old: {}})
		if err != nil {
			t.Fatal(err)
		}
		if (slots[0].ID == old) == isNew {
			t.Fatalf("newSlot=%v should preserve legacy identity only when false", isNew)
		}
	}
	_, _, _, err := normalizeSlots([]LayoutSlotInput{{NewSlot: true, SlotID: &old, XEndNorm: 1, YEndNorm: 1}}, uuid.New(), nil, nil, nil, map[uuid.UUID]struct{}{old: {}})
	if err == nil {
		t.Fatal("newSlot and slotId must be mutually exclusive")
	}
}
