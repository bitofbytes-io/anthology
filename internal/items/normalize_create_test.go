package items

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestNormalizeCreateInputMatchesCreateWithoutWriting(t *testing.T) {
	repo := NewInMemoryRepository(nil)
	svc := NewService(repo)
	pages, current, rating, volume := 300, 42, 12, 2
	at := time.Date(2024, time.March, 1, 10, 0, 0, 0, time.FixedZone("CET", 3600))
	input := CreateItemInput{
		OwnerID:       testOwnerID,
		Title:         "  Spaced Title  ",
		Creator:       " Author ",
		ItemType:      ItemTypeBook,
		PageCount:     &pages,
		CurrentPage:   &current,
		ISBN13:        " 9780000000001 ",
		Format:        "unknown-format",
		Rating:        &rating,
		ReadingStatus: BookStatusReading,
		Platform:      "ignored for books",
		SeriesName:    " Series ",
		VolumeNumber:  &volume,
		CreatedAt:     &at,
		UpdatedAt:     &at,
	}

	normalized, err := NormalizeCreateInput(input)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	stored, _ := repo.List(context.Background(), ListOptions{OwnerID: testOwnerID})
	if len(stored) != 0 {
		t.Fatalf("normalizing must not write, found %d items", len(stored))
	}
	if normalized.Title != "Spaced Title" || normalized.Creator != "Author" || normalized.ISBN13 != "9780000000001" {
		t.Fatalf("expected trimmed fields, got %+v", normalized)
	}
	if normalized.Format != FormatUnknown || normalized.Rating != nil || normalized.Platform != "" {
		t.Fatalf("expected normalized enums and cleared fields, got %+v", normalized)
	}
	if normalized.OwnerID != testOwnerID || normalized.CreatedAt != &at || normalized.UpdatedAt != &at {
		t.Fatalf("expected owner and timestamps to pass through, got %+v", normalized)
	}

	again, err := NormalizeCreateInput(normalized)
	if err != nil {
		t.Fatalf("normalize again: %v", err)
	}
	if !reflect.DeepEqual(again, normalized) {
		t.Fatalf("normalizing twice changed the input:\nfirst  %+v\nsecond %+v", normalized, again)
	}

	fromRaw, err := svc.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("create raw: %v", err)
	}
	fromNormalized, err := svc.Create(context.Background(), normalized)
	if err != nil {
		t.Fatalf("create normalized: %v", err)
	}
	fromNormalized.ID = fromRaw.ID
	if !reflect.DeepEqual(fromRaw, fromNormalized) {
		t.Fatalf("create stored different items:\nraw        %+v\nnormalized %+v", fromRaw, fromNormalized)
	}
}

func TestNormalizeCreateInputReportsCreateValidationErrors(t *testing.T) {
	cases := map[string]CreateItemInput{
		"missing title":         {ItemType: ItemTypeBook},
		"bad type":              {Title: "T", ItemType: "comic"},
		"read without date":     {Title: "T", ItemType: ItemTypeBook, ReadingStatus: BookStatusRead},
		"http cover":            {Title: "T", ItemType: ItemTypeMovie, CoverImage: "http://example.com/c.jpg"},
		"volume beyond maximum": {Title: "T", ItemType: ItemTypeBook, VolumeNumber: ptr(201)},
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := NormalizeCreateInput(input)
			if !errors.Is(err, ErrValidation) {
				t.Fatalf("expected validation error, got %v", err)
			}
		})
	}
}

func ptr(v int) *int { return &v }
