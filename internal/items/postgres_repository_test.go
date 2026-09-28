package items

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"anthology/internal/platform/testdb"
)

// createTestUser inserts a user row (items reference users) and removes it,
// along with its items, when the test finishes.
func createTestUser(t *testing.T, db *sqlx.DB) uuid.UUID {
	t.Helper()
	owner := uuid.New()
	if _, err := db.Exec(`INSERT INTO users(id,email,oauth_provider,oauth_provider_id) VALUES($1,$2,'test',$2)`, owner, owner.String()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM items WHERE owner_id=$1`, owner)
		_, _ = db.Exec(`DELETE FROM users WHERE id=$1`, owner)
	})
	return owner
}

func TestPostgresRepositoryTargetedLookups(t *testing.T) {
	db := testdb.Open(t)
	assertTargetedLookups(t, NewPostgresRepository(db), createTestUser(t, db), createTestUser(t, db))
}

func TestPostgresRepositoryOwnerIsolation(t *testing.T) {
	db := testdb.Open(t)
	repo := NewPostgresRepository(db)
	ctx := context.Background()
	alice, bob := createTestUser(t, db), createTestUser(t, db)
	now := time.Now().UTC().Truncate(time.Second)

	create := func(owner uuid.UUID, title, isbn, series string) Item {
		t.Helper()
		volume, total := 1, 3
		item, err := repo.Create(ctx, Item{
			ID:            uuid.New(),
			OwnerID:       owner,
			Title:         title,
			ItemType:      ItemTypeBook,
			ISBN13:        isbn,
			ReadingStatus: BookStatusNone,
			SeriesName:    series,
			VolumeNumber:  &volume,
			TotalVolumes:  &total,
			CreatedAt:     now,
			UpdatedAt:     now,
		})
		if err != nil {
			t.Fatalf("create %q: %v", title, err)
		}
		return item
	}
	aliceItem := create(alice, "Alice Book", "9782222222222", "Alice Saga")
	bobItem := create(bob, "Bob Secret", "9781111111111", "Bob Saga")

	if _, err := repo.Get(ctx, bobItem.ID, alice); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get across owners = %v; want ErrNotFound", err)
	}

	listed, err := repo.List(ctx, ListOptions{OwnerID: alice})
	if err != nil || len(listed) != 1 || listed[0].ID != aliceItem.ID {
		t.Fatalf("List for alice = %+v, %v; want only her item", listed, err)
	}

	byID, err := repo.ListByIDs(ctx, []uuid.UUID{bobItem.ID}, alice)
	if err != nil || len(byID) != 0 {
		t.Fatalf("ListByIDs across owners = %+v, %v; want none", byID, err)
	}

	if _, err := repo.FindByISBN(ctx, bobItem.ISBN13, alice); !errors.Is(err, ErrNotFound) {
		t.Fatalf("FindByISBN across owners = %v; want ErrNotFound", err)
	}

	duplicates, err := repo.FindDuplicates(ctx, DuplicateCheckInput{Title: bobItem.Title, ISBN13: bobItem.ISBN13}, alice)
	if err != nil || len(duplicates) != 0 {
		t.Fatalf("FindDuplicates across owners = %+v, %v; want none", duplicates, err)
	}

	histogram, err := repo.Histogram(ctx, HistogramOptions{OwnerID: alice})
	if err != nil || histogram["A"] != 1 || histogram["B"] != 0 {
		t.Fatalf("Histogram for alice = %v, %v; want only her item", histogram, err)
	}

	hijack := bobItem
	hijack.OwnerID = alice
	hijack.Title = "Hijacked"
	if _, err := repo.Update(ctx, hijack); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update across owners = %v; want ErrNotFound", err)
	}

	if err := repo.Delete(ctx, bobItem.ID, alice); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete across owners = %v; want ErrNotFound", err)
	}

	series, err := repo.ListSeries(ctx, SeriesRepoListOptions{IncludeItems: true}, alice)
	if err != nil || len(series) != 1 || series[0].SeriesName != "Alice Saga" || len(series[0].Items) != 1 {
		t.Fatalf("ListSeries for alice = %+v, %v; want only her series", series, err)
	}

	if _, err := repo.GetSeriesByName(ctx, "Bob Saga", alice); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetSeriesByName across owners = %v; want ErrNotFound", err)
	}

	names, err := repo.ListSeriesNamesByNameCI(ctx, "bob saga", alice)
	if err != nil || len(names) != 0 {
		t.Fatalf("ListSeriesNamesByNameCI across owners = %v, %v; want none", names, err)
	}

	if renamed, err := repo.UpdateSeriesName(ctx, "Bob Saga", "Stolen Saga", alice); err != nil || renamed != 0 {
		t.Fatalf("UpdateSeriesName across owners = %d, %v; want 0 rows", renamed, err)
	}

	if cleared, err := repo.ClearSeriesName(ctx, "Bob Saga", alice); err != nil || cleared != 0 {
		t.Fatalf("ClearSeriesName across owners = %d, %v; want 0 rows", cleared, err)
	}

	stored, err := repo.Get(ctx, bobItem.ID, bob)
	if err != nil {
		t.Fatalf("Get for bob: %v", err)
	}
	if stored.Title != "Bob Secret" || stored.OwnerID != bob || stored.SeriesName != "Bob Saga" ||
		stored.VolumeNumber == nil || *stored.VolumeNumber != 1 || stored.TotalVolumes == nil || *stored.TotalVolumes != 3 {
		t.Fatalf("bob's item was modified by alice's calls: %+v", stored)
	}
	if summary, err := repo.GetSeriesByName(ctx, "Bob Saga", bob); err != nil || summary.OwnedCount != 1 {
		t.Fatalf("GetSeriesByName for bob = %+v, %v; want his series intact", summary, err)
	}
}
