package items

import (
	"context"
	"testing"
)

func TestServiceFindDuplicatesByTitle(t *testing.T) {
	repo := NewInMemoryRepository(nil)
	svc := NewService(repo)
	ctx := context.Background()

	existing, _ := svc.Create(ctx, CreateItemInput{OwnerID: testOwnerID, Title: "The Great Gatsby", ItemType: ItemTypeBook, ISBN13: "9780743273565"})

	tests := []struct {
		name        string
		searchTitle string
		wantMatch   bool
	}{
		{"exact match", "The Great Gatsby", true},
		{"case insensitive", "the great gatsby", true},
		{"case insensitive uppercase", "THE GREAT GATSBY", true},
		{"mixed case", "ThE GrEaT GaTsBY", true},
		{"with leading whitespace", "  The Great Gatsby", true},
		{"with trailing whitespace", "The Great Gatsby  ", true},
		{"with both whitespace", "  The Great Gatsby  ", true},
		{"partial match", "Great Gatsby", false},
		{"different title", "The Old Man and the Sea", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches, err := svc.FindDuplicates(ctx, DuplicateCheckInput{Title: tt.searchTitle}, testOwnerID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantMatch {
				if len(matches) != 1 {
					t.Fatalf("expected 1 match, got %d", len(matches))
				}
				if matches[0].ID != existing.ID {
					t.Fatalf("expected matching ID")
				}
			} else {
				if len(matches) != 0 {
					t.Fatalf("expected no matches, got %d", len(matches))
				}
			}
		})
	}
}

func TestServiceFindDuplicatesByISBN(t *testing.T) {
	repo := NewInMemoryRepository(nil)
	svc := NewService(repo)
	ctx := context.Background()

	existing, _ := svc.Create(ctx, CreateItemInput{
		OwnerID:  testOwnerID,
		Title:    "Clean Code",
		ItemType: ItemTypeBook,
		ISBN13:   "978-0132350884",
		ISBN10:   "0132350882",
	})

	tests := []struct {
		name      string
		isbn13    string
		isbn10    string
		wantMatch bool
	}{
		{"exact isbn13", "978-0132350884", "", true},
		{"isbn13 without hyphens", "9780132350884", "", true},
		{"isbn13 with spaces", "978 0132350884", "", true},
		{"exact isbn10", "", "0132350882", true},
		{"isbn10 with hyphen", "", "0-132350882", true},
		{"different isbn13", "9780321125217", "", false},
		{"different isbn10", "", "0321125215", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches, err := svc.FindDuplicates(ctx, DuplicateCheckInput{ISBN13: tt.isbn13, ISBN10: tt.isbn10}, testOwnerID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantMatch {
				if len(matches) != 1 {
					t.Fatalf("expected 1 match, got %d", len(matches))
				}
				if matches[0].ID != existing.ID {
					t.Fatalf("expected matching ID")
				}
			} else {
				if len(matches) != 0 {
					t.Fatalf("expected no matches, got %d", len(matches))
				}
			}
		})
	}
}

func TestServiceFindDuplicatesReturnsMaxFiveMatches(t *testing.T) {
	repo := NewInMemoryRepository(nil)
	svc := NewService(repo)
	ctx := context.Background()

	// Create 7 items with the same title
	for i := 0; i < 7; i++ {
		_, _ = svc.Create(ctx, CreateItemInput{OwnerID: testOwnerID, Title: "Duplicate Title", ItemType: ItemTypeBook})
	}

	matches, err := svc.FindDuplicates(ctx, DuplicateCheckInput{Title: "Duplicate Title"}, testOwnerID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(matches) != 5 {
		t.Fatalf("expected max 5 matches, got %d", len(matches))
	}
}

func TestServiceFindDuplicatesReturnsCorrectFields(t *testing.T) {
	repo := NewInMemoryRepository(nil)
	svc := NewService(repo)
	ctx := context.Background()

	existing, _ := svc.Create(ctx, CreateItemInput{
		OwnerID:    testOwnerID,
		Title:      "Test Book",
		ItemType:   ItemTypeBook,
		ISBN13:     "9780123456789",
		CoverImage: "https://example.com/cover.jpg",
	})

	matches, err := svc.FindDuplicates(ctx, DuplicateCheckInput{Title: "Test Book"}, testOwnerID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}

	match := matches[0]
	if match.ID != existing.ID {
		t.Fatalf("expected ID to match")
	}
	if match.Title != "Test Book" {
		t.Fatalf("expected title to match")
	}
	if match.PrimaryIdentifier != "9780123456789" {
		t.Fatalf("expected primary identifier to be ISBN-13")
	}
	if match.IdentifierType != "ISBN-13" {
		t.Fatalf("expected identifier type to be ISBN-13")
	}
	if match.CoverURL != "https://example.com/cover.jpg" {
		t.Fatalf("expected cover URL to match")
	}
}

func TestServiceFindDuplicatesEmptyInput(t *testing.T) {
	repo := NewInMemoryRepository(nil)
	svc := NewService(repo)
	ctx := context.Background()

	_, _ = svc.Create(ctx, CreateItemInput{OwnerID: testOwnerID, Title: "Some Book", ItemType: ItemTypeBook})

	matches, err := svc.FindDuplicates(ctx, DuplicateCheckInput{}, testOwnerID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(matches) != 0 {
		t.Fatalf("expected no matches for empty input, got %d", len(matches))
	}
}
