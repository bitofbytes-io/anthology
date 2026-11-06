package items

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// testOwnerID is a fixed UUID for tests
var testOwnerID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

func TestServiceCreateValidatesInput(t *testing.T) {
	repo := NewInMemoryRepository(nil)
	svc := NewService(repo)

	_, err := svc.Create(context.Background(), CreateItemInput{OwnerID: testOwnerID})
	if err == nil {
		t.Fatalf("expected validation error when title missing")
	}
}

func TestServiceCreatePersistsItem(t *testing.T) {
	repo := NewInMemoryRepository(nil)
	svc := NewService(repo)
	year := 2023
	pages := 352

	item, err := svc.Create(context.Background(), CreateItemInput{
		OwnerID:     testOwnerID,
		Title:       "The Pragmatic Programmer",
		Creator:     "Andrew Hunt",
		ItemType:    ItemTypeBook,
		ReleaseYear: &year,
		PageCount:   &pages,
		ISBN13:      "9780135957059",
		ISBN10:      "0135957052",
		Description: "Practical advice for software craftspeople.",
		CoverImage:  "data:image/png;base64,aGVsbG8=",
		Notes:       "Initial dataset mirrors a curated media catalogue.",
	})
	if err != nil {
		t.Fatalf("expected item to be created: %v", err)
	}

	if item.ID == uuid.Nil {
		t.Fatalf("expected id to be set")
	}
	if item.CreatedAt.IsZero() || item.UpdatedAt.IsZero() {
		t.Fatalf("expected timestamps to be set")
	}
	if item.ReleaseYear == nil || *item.ReleaseYear != year {
		t.Fatalf("expected release year to persist")
	}
	if item.PageCount == nil || *item.PageCount != pages {
		t.Fatalf("expected page count to persist")
	}
	if item.CurrentPage != nil {
		t.Fatalf("expected current page to be nil for new item")
	}
	if item.ISBN13 != "9780135957059" {
		t.Fatalf("expected isbn13 to persist")
	}
	if item.ISBN10 != "0135957052" {
		t.Fatalf("expected isbn10 to persist")
	}
	if item.Description != "Practical advice for software craftspeople." {
		t.Fatalf("expected description to persist")
	}
	if item.CoverImage == "" {
		t.Fatalf("expected cover image to persist")
	}
	if item.ReadingStatus != BookStatusNone {
		t.Fatalf("expected default reading status to be none, got %q", item.ReadingStatus)
	}
	if item.ReadAt != nil {
		t.Fatalf("expected readAt to be nil for non-read status")
	}
}

func TestServiceCreateUsesProvidedTimestamps(t *testing.T) {
	repo := NewInMemoryRepository(nil)
	svc := NewService(repo)
	createdAt := time.Date(2024, 2, 1, 12, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2024, 2, 2, 15, 30, 0, 0, time.UTC)

	item, err := svc.Create(context.Background(), CreateItemInput{
		OwnerID:   testOwnerID,
		Title:     "Timestamped",
		ItemType:  ItemTypeBook,
		CreatedAt: &createdAt,
		UpdatedAt: &updatedAt,
	})
	if err != nil {
		t.Fatalf("expected item to be created: %v", err)
	}

	if !item.CreatedAt.Equal(createdAt) {
		t.Fatalf("expected createdAt to be %s", createdAt.Format(time.RFC3339))
	}
	if !item.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("expected updatedAt to be %s", updatedAt.Format(time.RFC3339))
	}
}

func TestServiceCreateValidatesCurrentPage(t *testing.T) {
	repo := NewInMemoryRepository(nil)
	svc := NewService(repo)
	ctx := context.Background()
	pageCount := 200
	current := 250

	_, err := svc.Create(ctx, CreateItemInput{
		OwnerID:       testOwnerID,
		Title:         "Progress",
		ItemType:      ItemTypeBook,
		PageCount:     &pageCount,
		ReadingStatus: BookStatusReading,
		CurrentPage:   &current,
	})
	if err == nil {
		t.Fatalf("expected error when current page exceeds total")
	}

	current = -1
	_, err = svc.Create(ctx, CreateItemInput{
		OwnerID:       testOwnerID,
		Title:         "Negative",
		ItemType:      ItemTypeBook,
		ReadingStatus: BookStatusReading,
		CurrentPage:   &current,
	})
	if err == nil {
		t.Fatalf("expected error when current page negative")
	}
}

func TestServiceUpdate(t *testing.T) {
	repo := NewInMemoryRepository(nil)
	svc := NewService(repo)

	created, err := svc.Create(context.Background(), CreateItemInput{OwnerID: testOwnerID, Title: "Initial", ItemType: ItemTypeBook})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	title := "Updated"
	notes := "Now includes expansion content"
	newDescription := "Updated overview"
	isbn13 := "9781984801265"
	isbn10 := "1984801263"
	pageCountValue := 640

	updated, err := svc.Update(context.Background(), created.ID, testOwnerID, patchOf(t, map[string]any{
		"title":       title,
		"notes":       notes,
		"itemType":    ItemTypeBook,
		"description": newDescription,
		"isbn13":      isbn13,
		"isbn10":      isbn10,
		"pageCount":   pageCountValue,
	}))
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	if updated.Title != title {
		t.Fatalf("expected title %q, got %q", title, updated.Title)
	}
	if updated.Notes != notes {
		t.Fatalf("expected notes to be updated")
	}
	if updated.Description != newDescription {
		t.Fatalf("expected description update")
	}
	if updated.ISBN13 != isbn13 || updated.ISBN10 != isbn10 {
		t.Fatalf("expected isbn values to update")
	}
	if updated.PageCount == nil || *updated.PageCount != pageCountValue {
		t.Fatalf("expected page count update")
	}
	if !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Fatalf("expected updated timestamp to increase")
	}
}

func TestServiceUpdateSupportsExplicitNullSeriesNumbers(t *testing.T) {
	repo := NewInMemoryRepository(nil)
	svc := NewService(repo)
	ctx := context.Background()

	volume := 2
	total := 5
	book, err := svc.Create(ctx, CreateItemInput{
		OwnerID:      testOwnerID,
		Title:        "Series Book",
		ItemType:     ItemTypeBook,
		SeriesName:   "Saga",
		VolumeNumber: &volume,
		TotalVolumes: &total,
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	updated, err := svc.Update(ctx, book.ID, testOwnerID, patchOf(t, map[string]any{"volumeNumber": nil}))
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if updated.VolumeNumber != nil {
		t.Fatalf("expected volumeNumber to clear, got %d", *updated.VolumeNumber)
	}
	if updated.TotalVolumes == nil || *updated.TotalVolumes != total {
		t.Fatalf("expected totalVolumes to remain %d, got %#v", total, updated.TotalVolumes)
	}
	if updated.SeriesName != "Saga" {
		t.Fatalf("expected seriesName to remain Saga, got %q", updated.SeriesName)
	}

	updated, err = svc.Update(ctx, book.ID, testOwnerID, patchOf(t, map[string]any{"totalVolumes": nil}))
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if updated.TotalVolumes != nil {
		t.Fatalf("expected totalVolumes to clear, got %d", *updated.TotalVolumes)
	}
	if updated.SeriesName != "Saga" {
		t.Fatalf("expected seriesName to remain Saga, got %q", updated.SeriesName)
	}
}

func TestServiceCreateTrimsInputAndNormalizesYear(t *testing.T) {
	repo := NewInMemoryRepository(nil)
	svc := NewService(repo)

	negativeYear := -1990
	negativePages := -10
	isbn13 := " 9780441172719 "
	isbn10 := " 0441172717"
	description := "  Classic sci-fi  "
	item, err := svc.Create(context.Background(), CreateItemInput{
		OwnerID:     testOwnerID,
		Title:       "  Dune  ",
		Creator:     "  Frank Herbert ",
		ItemType:    ItemTypeBook,
		ReleaseYear: &negativeYear,
		PageCount:   &negativePages,
		ISBN13:      isbn13,
		ISBN10:      isbn10,
		Description: description,
		Notes:       "  Classic sci-fi  ",
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	if item.Title != "Dune" {
		t.Fatalf("expected trimmed title, got %q", item.Title)
	}
	if item.Creator != "Frank Herbert" {
		t.Fatalf("expected trimmed creator, got %q", item.Creator)
	}
	if item.ReleaseYear != nil {
		t.Fatalf("expected negative year to be normalized to nil, got %v", item.ReleaseYear)
	}
	if item.Notes != "Classic sci-fi" {
		t.Fatalf("expected trimmed notes, got %q", item.Notes)
	}
	if item.PageCount != nil {
		t.Fatalf("expected invalid page count to normalize to nil")
	}
	if item.ISBN13 != "9780441172719" {
		t.Fatalf("expected isbn13 to be trimmed")
	}
	if item.ISBN10 != "0441172717" {
		t.Fatalf("expected isbn10 to be trimmed")
	}
	if item.Description != "Classic sci-fi" {
		t.Fatalf("expected description to be trimmed, got %q", item.Description)
	}
}

func TestServiceUpdateRejectsBlankTitleOrItemType(t *testing.T) {
	repo := NewInMemoryRepository(nil)
	svc := NewService(repo)

	item, err := svc.Create(context.Background(), CreateItemInput{OwnerID: testOwnerID, Title: "Initial", ItemType: ItemTypeBook})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	ctx := context.Background()

	_, err = svc.Update(ctx, item.ID, testOwnerID, patchOf(t, map[string]any{"title": "   "}))
	if err == nil || !strings.Contains(err.Error(), "title is required") {
		t.Fatalf("expected title validation error, got %v", err)
	}

	_, err = svc.Update(ctx, item.ID, testOwnerID, patchOf(t, map[string]any{"itemType": ""}))
	if err == nil || !strings.Contains(err.Error(), "itemType is required") {
		t.Fatalf("expected itemType validation error, got %v", err)
	}
}

func ptrInt(value int) *int {
	v := value
	return &v
}

// patchOf builds an update patch from fields, as the API would parse the
// equivalent JSON body. A nil value sends an explicit null.
func patchOf(t *testing.T, fields map[string]any) ItemPatch {
	t.Helper()
	body, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("marshal patch: %v", err)
	}
	patch, err := ParseItemPatch(body)
	if err != nil {
		t.Fatalf("parse patch %s: %v", body, err)
	}
	return patch
}
