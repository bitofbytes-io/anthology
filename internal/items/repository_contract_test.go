package items

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
)

// assertTargetedLookups exercises ListByIDs and FindByISBN against any
// Repository implementation so the in-memory and Postgres repositories stay in
// parity. ownerA and ownerB must be distinct, existing owners.
func assertTargetedLookups(t *testing.T, repo Repository, ownerA, ownerB uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Second)

	createTyped := func(owner uuid.UUID, itemType ItemType, title, isbn13, isbn10 string, offset time.Duration) Item {
		t.Helper()
		item, err := repo.Create(ctx, Item{
			ID:            uuid.New(),
			OwnerID:       owner,
			Title:         title,
			ItemType:      itemType,
			ISBN13:        isbn13,
			ISBN10:        isbn10,
			ReadingStatus: BookStatusNone,
			CreatedAt:     base.Add(offset),
			UpdatedAt:     base.Add(offset),
		})
		if err != nil {
			t.Fatalf("create %q: %v", title, err)
		}
		return item
	}
	create := func(owner uuid.UUID, title, isbn13, isbn10 string, offset time.Duration) Item {
		t.Helper()
		return createTyped(owner, ItemTypeBook, title, isbn13, isbn10, offset)
	}

	older := create(ownerA, "Older", "978-0-00-000000-1", "", 0)
	newer := create(ownerA, "Newer", "9780000000001", "", time.Minute)
	isbn10Only := create(ownerA, "ISBN-10 Only", "", "0-00-000000-2", 2*time.Minute)
	foreign := create(ownerB, "Foreign", "9780000000009", "", 3*time.Minute)
	checkDigitX := create(ownerA, "Check Digit X", "", "0-8044-2957-X", 4*time.Minute)
	isbn13Only := create(ownerA, "ISBN-13 Only", "978-0-306-40615-7", "", 6*time.Minute)
	// Written straight to the repository, bypassing service normalization, to
	// model a legacy non-book row that still carries an ISBN.
	createTyped(ownerA, ItemTypeMovie, "Legacy Movie", "9780000000017", "", 5*time.Minute)

	listed, err := repo.ListByIDs(ctx, []uuid.UUID{older.ID, isbn10Only.ID, foreign.ID, uuid.New()}, ownerA)
	if err != nil {
		t.Fatalf("ListByIDs: %v", err)
	}
	gotIDs := make([]uuid.UUID, 0, len(listed))
	for _, item := range listed {
		gotIDs = append(gotIDs, item.ID)
	}
	if len(gotIDs) != 2 || !slices.Contains(gotIDs, older.ID) || !slices.Contains(gotIDs, isbn10Only.ID) {
		t.Fatalf("ListByIDs returned %v, want only %s and %s", gotIDs, older.ID, isbn10Only.ID)
	}

	empty, err := repo.ListByIDs(ctx, nil, ownerA)
	if err != nil || len(empty) != 0 {
		t.Fatalf("ListByIDs with no IDs = %v, %v; want empty", empty, err)
	}

	found, err := repo.FindByISBN(ctx, "9780000000001", ownerA)
	if err != nil || found.ID != newer.ID {
		t.Fatalf("FindByISBN ISBN-13 = %s, %v; want most recent %s", found.ID, err, newer.ID)
	}
	found, err = repo.FindByISBN(ctx, "0000000002", ownerA)
	if err != nil || found.ID != isbn10Only.ID {
		t.Fatalf("FindByISBN normalized ISBN-10 = %s, %v; want %s", found.ID, err, isbn10Only.ID)
	}
	if _, err := repo.FindByISBN(ctx, "9780000000009", ownerA); !errors.Is(err, ErrNotFound) {
		t.Fatalf("FindByISBN across owners = %v; want ErrNotFound", err)
	}
	found, err = repo.FindByISBN(ctx, "978-0000000009", ownerB)
	if err != nil || found.ID != foreign.ID {
		t.Fatalf("FindByISBN for owner B = %s, %v; want %s", found.ID, err, foreign.ID)
	}
	found, err = repo.FindByISBN(ctx, "080442957x", ownerA)
	if err != nil || found.ID != checkDigitX.ID {
		t.Fatalf("FindByISBN with X check digit = %s, %v; want %s", found.ID, err, checkDigitX.ID)
	}
	found, err = repo.FindByISBN(ctx, "0-306-40615-2", ownerA)
	if err != nil || found.ID != isbn13Only.ID {
		t.Fatalf("FindByISBN ISBN-10 scan of an ISBN-13 book = %s, %v; want %s", found.ID, err, isbn13Only.ID)
	}
	found, err = repo.FindByISBN(ctx, "9780804429573", ownerA)
	if err != nil || found.ID != checkDigitX.ID {
		t.Fatalf("FindByISBN ISBN-13 scan of an ISBN-10 book = %s, %v; want %s", found.ID, err, checkDigitX.ID)
	}
	for _, misread := range []string{"0306406153", "9780804429574"} {
		if _, err := repo.FindByISBN(ctx, misread, ownerA); !errors.Is(err, ErrNotFound) {
			t.Fatalf("FindByISBN(%q) with a wrong check digit matched across forms: %v", misread, err)
		}
	}
	if _, err := repo.FindByISBN(ctx, "9790306406157", ownerA); !errors.Is(err, ErrNotFound) {
		t.Fatalf("FindByISBN matched a 979 ISBN-13 to an ISBN-10 book: %v", err)
	}
	if _, err := repo.FindByISBN(ctx, "9780000000017", ownerA); !errors.Is(err, ErrNotFound) {
		t.Fatalf("FindByISBN matched a non-book: %v", err)
	}
	for _, blank := range []string{"", "not-an-isbn", "080442957", "97800000000", "08044X2957"} {
		if _, err := repo.FindByISBN(ctx, blank, ownerA); !errors.Is(err, ErrNotFound) {
			t.Fatalf("FindByISBN(%q) = %v; want ErrNotFound", blank, err)
		}
	}
}

func TestInMemoryRepositoryTargetedLookups(t *testing.T) {
	assertTargetedLookups(t, NewInMemoryRepository(nil), uuid.New(), uuid.New())
}

// assertLetterFiltering checks that every title counted in a histogram bucket
// is returned by the matching letter filter, including titles that start with
// an accented letter (counted under "#").
func assertLetterFiltering(t *testing.T, repo Repository, owner uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	for i, title := range []string{"Émile", "éclair", "Apple", " zebra", "42 Things", "Ørsted"} {
		if _, err := repo.Create(ctx, Item{
			ID:            uuid.New(),
			OwnerID:       owner,
			Title:         title,
			ItemType:      ItemTypeBook,
			ReadingStatus: BookStatusNone,
			CreatedAt:     now.Add(time.Duration(i) * time.Second),
			UpdatedAt:     now.Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatalf("create %q: %v", title, err)
		}
	}

	histogram, err := repo.Histogram(ctx, HistogramOptions{OwnerID: owner})
	if err != nil {
		t.Fatalf("Histogram: %v", err)
	}
	want := LetterHistogram{"A": 1, "Z": 1, "#": 4}
	if len(histogram) != len(want) {
		t.Fatalf("Histogram = %v, want %v", histogram, want)
	}
	for letter, count := range want {
		if histogram[letter] != count {
			t.Fatalf("Histogram = %v, want %v", histogram, want)
		}
	}

	for letter, count := range histogram {
		initial := letter
		listed, err := repo.List(ctx, ListOptions{OwnerID: owner, Initial: &initial})
		if err != nil {
			t.Fatalf("List(%q): %v", letter, err)
		}
		if len(listed) != count {
			titles := make([]string, 0, len(listed))
			for _, item := range listed {
				titles = append(titles, item.Title)
			}
			t.Fatalf("List(%q) returned %v; histogram counted %d", letter, titles, count)
		}
	}
}

func TestInMemoryRepositoryLetterFiltering(t *testing.T) {
	assertLetterFiltering(t, NewInMemoryRepository(nil), uuid.New())
}
