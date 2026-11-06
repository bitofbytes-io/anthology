package items

import (
	"context"
	"testing"
	"time"
)

func TestServiceListOrdersByCreatedAt(t *testing.T) {
	repo := NewInMemoryRepository(nil)
	svc := NewService(repo)

	ctx := context.Background()

	_, _ = svc.Create(ctx, CreateItemInput{OwnerID: testOwnerID, Title: "First", ItemType: ItemTypeBook})
	time.Sleep(10 * time.Millisecond)
	second, _ := svc.Create(ctx, CreateItemInput{OwnerID: testOwnerID, Title: "Second", ItemType: ItemTypeBook})

	items, err := svc.List(ctx, ListOptions{OwnerID: testOwnerID})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected two items")
	}
	if items[0].ID != second.ID {
		t.Fatalf("expected newest item first")
	}
}

func TestServiceListAppliesFilters(t *testing.T) {
	repo := NewInMemoryRepository(nil)
	svc := NewService(repo)

	ctx := context.Background()

	_, _ = svc.Create(ctx, CreateItemInput{OwnerID: testOwnerID, Title: "Alpha", ItemType: ItemTypeBook})
	_, _ = svc.Create(ctx, CreateItemInput{OwnerID: testOwnerID, Title: "Zulu", ItemType: ItemTypeGame})
	_, _ = svc.Create(ctx, CreateItemInput{OwnerID: testOwnerID, Title: "99 Luftballons", ItemType: ItemTypeMusic})

	finishedDate := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)
	finished, _ := svc.Create(ctx, CreateItemInput{OwnerID: testOwnerID, Title: "Finished Book", ItemType: ItemTypeBook, ReadingStatus: BookStatusRead, ReadAt: &finishedDate})
	_, _ = svc.Create(ctx, CreateItemInput{OwnerID: testOwnerID, Title: "In Progress", ItemType: ItemTypeBook, ReadingStatus: BookStatusReading})

	letter := "A"
	items, err := svc.List(ctx, ListOptions{OwnerID: testOwnerID, Initial: &letter})
	if err != nil {
		t.Fatalf("list with initial failed: %v", err)
	}
	if len(items) != 1 || items[0].Title != "Alpha" {
		t.Fatalf("expected only alpha result, got %#v", items)
	}

	itemType := ItemTypeMusic
	hash := "#"
	items, err = svc.List(ctx, ListOptions{OwnerID: testOwnerID, ItemType: &itemType, Initial: &hash})
	if err != nil {
		t.Fatalf("list with combined filters failed: %v", err)
	}
	if len(items) != 1 || items[0].Title != "99 Luftballons" {
		t.Fatalf("expected non-alphabetic music result")
	}

	status := BookStatusRead
	items, err = svc.List(ctx, ListOptions{OwnerID: testOwnerID, ReadingStatus: &status})
	if err != nil {
		t.Fatalf("list with status filter failed: %v", err)
	}
	if len(items) != 1 || items[0].ID != finished.ID {
		t.Fatalf("expected only finished book, got %#v", items)
	}
}

func TestServiceListSupportsSearchAndLimit(t *testing.T) {
	repo := NewInMemoryRepository(nil)
	svc := NewService(repo)

	ctx := context.Background()

	_, _ = svc.Create(ctx, CreateItemInput{OwnerID: testOwnerID, Title: "The Pragmatic Programmer", ItemType: ItemTypeBook})
	_, _ = svc.Create(ctx, CreateItemInput{OwnerID: testOwnerID, Title: "Children of Dune", ItemType: ItemTypeBook})
	time.Sleep(10 * time.Millisecond)
	latest, _ := svc.Create(ctx, CreateItemInput{OwnerID: testOwnerID, Title: "Dune", ItemType: ItemTypeBook})

	query := "dune"
	limit := 1
	items, err := svc.List(ctx, ListOptions{OwnerID: testOwnerID, Query: &query, Limit: &limit})
	if err != nil {
		t.Fatalf("list with search failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].ID != latest.ID {
		t.Fatalf("expected newest matching item, got %q", items[0].Title)
	}
}

func TestServiceListStatusFilterWithAllItemType(t *testing.T) {
	repo := NewInMemoryRepository(nil)
	svc := NewService(repo)
	ctx := context.Background()

	// Create test data: books with various statuses and non-book items
	finishedDate := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)
	readBook, _ := svc.Create(ctx, CreateItemInput{OwnerID: testOwnerID, Title: "Read Book", ItemType: ItemTypeBook, ReadingStatus: BookStatusRead, ReadAt: &finishedDate})
	readingBook, _ := svc.Create(ctx, CreateItemInput{OwnerID: testOwnerID, Title: "Reading Book", ItemType: ItemTypeBook, ReadingStatus: BookStatusReading})
	noneBook, _ := svc.Create(ctx, CreateItemInput{OwnerID: testOwnerID, Title: "No Status Book", ItemType: ItemTypeBook})
	movie, _ := svc.Create(ctx, CreateItemInput{OwnerID: testOwnerID, Title: "A Movie", ItemType: ItemTypeMovie})
	game, _ := svc.Create(ctx, CreateItemInput{OwnerID: testOwnerID, Title: "A Game", ItemType: ItemTypeGame})

	t.Run("All+Read shows only read books", func(t *testing.T) {
		status := BookStatusRead
		items, err := svc.List(ctx, ListOptions{OwnerID: testOwnerID, ReadingStatus: &status})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(items))
		}
		if items[0].ID != readBook.ID {
			t.Fatalf("expected read book, got %q", items[0].Title)
		}
	})

	t.Run("All+Reading shows only reading books", func(t *testing.T) {
		status := BookStatusReading
		items, err := svc.List(ctx, ListOptions{OwnerID: testOwnerID, ReadingStatus: &status})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(items))
		}
		if items[0].ID != readingBook.ID {
			t.Fatalf("expected reading book, got %q", items[0].Title)
		}
	})

	t.Run("All+None shows books with none status plus all non-books", func(t *testing.T) {
		status := BookStatusNone
		items, err := svc.List(ctx, ListOptions{OwnerID: testOwnerID, ReadingStatus: &status})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Should include: noneBook, movie, game
		if len(items) != 3 {
			t.Fatalf("expected 3 items, got %d: %v", len(items), items)
		}
		ids := map[string]bool{}
		for _, item := range items {
			ids[item.ID.String()] = true
		}
		if !ids[noneBook.ID.String()] {
			t.Fatalf("expected none book to be included")
		}
		if !ids[movie.ID.String()] {
			t.Fatalf("expected movie to be included")
		}
		if !ids[game.ID.String()] {
			t.Fatalf("expected game to be included")
		}
	})

	t.Run("Book+Read shows only read books", func(t *testing.T) {
		itemType := ItemTypeBook
		status := BookStatusRead
		items, err := svc.List(ctx, ListOptions{OwnerID: testOwnerID, ItemType: &itemType, ReadingStatus: &status})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(items))
		}
		if items[0].ID != readBook.ID {
			t.Fatalf("expected read book, got %q", items[0].Title)
		}
	})

	t.Run("Movie type ignores status filter", func(t *testing.T) {
		itemType := ItemTypeMovie
		status := BookStatusRead
		items, err := svc.List(ctx, ListOptions{OwnerID: testOwnerID, ItemType: &itemType, ReadingStatus: &status})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Since movies have BookStatusNone by default, they won't match BookStatusRead
		if len(items) != 0 {
			t.Fatalf("expected 0 items, got %d", len(items))
		}
	})
}

func TestServiceHistogramStatusFilterWithAllItemType(t *testing.T) {
	repo := NewInMemoryRepository(nil)
	svc := NewService(repo)
	ctx := context.Background()

	// Create test data
	finishedDate := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)
	_, _ = svc.Create(ctx, CreateItemInput{OwnerID: testOwnerID, Title: "Alpha Read Book", ItemType: ItemTypeBook, ReadingStatus: BookStatusRead, ReadAt: &finishedDate})
	_, _ = svc.Create(ctx, CreateItemInput{OwnerID: testOwnerID, Title: "Beta No Status Book", ItemType: ItemTypeBook})
	_, _ = svc.Create(ctx, CreateItemInput{OwnerID: testOwnerID, Title: "Charlie Movie", ItemType: ItemTypeMovie})
	_, _ = svc.Create(ctx, CreateItemInput{OwnerID: testOwnerID, Title: "Delta Game", ItemType: ItemTypeGame})

	t.Run("All+Read histogram shows only read books", func(t *testing.T) {
		status := BookStatusRead
		histogram, total, err := svc.Histogram(ctx, HistogramOptions{OwnerID: testOwnerID, ReadingStatus: &status})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if total != 1 {
			t.Fatalf("expected total 1, got %d", total)
		}
		if histogram["A"] != 1 {
			t.Fatalf("expected A=1, got %d", histogram["A"])
		}
	})

	t.Run("All+None histogram shows books with none status plus non-books", func(t *testing.T) {
		status := BookStatusNone
		histogram, total, err := svc.Histogram(ctx, HistogramOptions{OwnerID: testOwnerID, ReadingStatus: &status})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Should include: Beta No Status Book, Charlie Movie, Delta Game
		if total != 3 {
			t.Fatalf("expected total 3, got %d", total)
		}
		if histogram["B"] != 1 {
			t.Fatalf("expected B=1, got %d", histogram["B"])
		}
		if histogram["C"] != 1 {
			t.Fatalf("expected C=1, got %d", histogram["C"])
		}
		if histogram["D"] != 1 {
			t.Fatalf("expected D=1, got %d", histogram["D"])
		}
	})
}
