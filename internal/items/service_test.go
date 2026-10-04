package items

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"
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

func TestServiceUpdateSeriesNameReturnsUnexpectedLookupError(t *testing.T) {
	repo := &seriesUpdateRepo{
		t: t,
		summaries: map[string]SeriesSummary{
			"Old Series": {SeriesName: "Old Series"},
		},
	}
	lookupErr := errors.New("lookup failed")
	repo.errByName = map[string]error{"New Series": lookupErr}

	svc := NewService(repo)

	_, err := svc.UpdateSeriesName(context.Background(), "Old Series", "New Series", testOwnerID)
	if err == nil || !errors.Is(err, lookupErr) {
		t.Fatalf("expected lookup error to propagate, got %v", err)
	}
	if repo.updateCalled {
		t.Fatalf("expected UpdateSeriesName not to be called after lookup error")
	}
}

func TestServiceUpdateSeriesNameRejectsCaseInsensitiveCollision(t *testing.T) {
	repo := NewInMemoryRepository(nil)
	svc := NewService(repo)
	ctx := context.Background()

	_, err := svc.Create(ctx, CreateItemInput{
		OwnerID:    testOwnerID,
		Title:      "First Book",
		ItemType:   ItemTypeBook,
		SeriesName: "abc",
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	_, err = svc.Create(ctx, CreateItemInput{
		OwnerID:    testOwnerID,
		Title:      "Second Book",
		ItemType:   ItemTypeBook,
		SeriesName: "ABC",
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	_, err = svc.UpdateSeriesName(ctx, "abc", "Abc", testOwnerID)
	if err == nil || !errors.Is(err, ErrValidation) {
		t.Fatalf("expected validation error for case-insensitive collision, got %v", err)
	}
}

func TestServiceEnrichSeriesSummarySetsMissingCountWhenCompleteWithoutVolumeNumbers(t *testing.T) {
	repo := NewInMemoryRepository(nil)
	svc := NewService(repo)

	summary := SeriesSummary{
		SeriesName:   "Saga",
		OwnedCount:   3,
		TotalVolumes: ptrInt(3),
		Items: []Item{
			{ItemType: ItemTypeBook, SeriesName: "Saga"},
			{ItemType: ItemTypeBook, SeriesName: "Saga"},
			{ItemType: ItemTypeBook, SeriesName: "Saga"},
		},
	}

	enriched := svc.enrichSeriesSummary(summary)

	if enriched.MissingCount == nil || *enriched.MissingCount != 0 {
		t.Fatalf("expected missingCount to be 0, got %#v", enriched.MissingCount)
	}
	if enriched.Status != SeriesStatusComplete {
		t.Fatalf("expected status to be complete, got %q", enriched.Status)
	}
}

func TestServiceEnrichSeriesSummaryMarksIncompleteWhenTotalsExceedOwnedWithoutVolumeNumbers(t *testing.T) {
	repo := NewInMemoryRepository(nil)
	svc := NewService(repo)

	summary := SeriesSummary{
		SeriesName:   "Saga",
		OwnedCount:   2,
		TotalVolumes: ptrInt(4),
		Items: []Item{
			{ItemType: ItemTypeBook, SeriesName: "Saga"},
			{ItemType: ItemTypeBook, SeriesName: "Saga"},
		},
	}

	enriched := svc.enrichSeriesSummary(summary)

	if enriched.MissingCount == nil || *enriched.MissingCount != 2 {
		t.Fatalf("expected missingCount to be 2, got %#v", enriched.MissingCount)
	}
	if enriched.Status != SeriesStatusIncomplete {
		t.Fatalf("expected status to be incomplete, got %q", enriched.Status)
	}
}

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

func TestServiceRejectsUnknownItemType(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewInMemoryRepository(nil))

	_, err := svc.Create(ctx, CreateItemInput{OwnerID: testOwnerID, Title: "Mystery", ItemType: ItemType("comic")})
	var validation *ValidationError
	if !errors.As(err, &validation) || !strings.Contains(err.Error(), "itemType must be one of") {
		t.Fatalf("expected itemType validation error on create, got %v", err)
	}

	item, err := svc.Create(ctx, CreateItemInput{OwnerID: testOwnerID, Title: "Known", ItemType: ItemTypeBook})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	_, err = svc.Update(ctx, item.ID, testOwnerID, patchOf(t, map[string]any{"itemType": "Book"}))
	if !errors.As(err, &validation) || !strings.Contains(err.Error(), "itemType must be one of") {
		t.Fatalf("expected itemType validation error on update, got %v", err)
	}
	stored, err := svc.Get(ctx, item.ID, testOwnerID)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if stored.ItemType != ItemTypeBook {
		t.Fatalf("expected item type to remain book, got %q", stored.ItemType)
	}
}

func TestServiceUpdateChangingTypeClearsFieldsForOtherTypes(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewInMemoryRepository(nil))
	rating := 7
	price := 12.5

	book, err := svc.Create(ctx, CreateItemInput{
		OwnerID:        testOwnerID,
		Title:          "Book Then Game",
		ItemType:       ItemTypeBook,
		Format:         FormatPaperback,
		Genre:          GenreFiction,
		Rating:         &rating,
		RetailPriceUsd: &price,
		GoogleVolumeId: "vol-1",
	})
	if err != nil {
		t.Fatalf("create book failed: %v", err)
	}

	game, err := svc.Update(ctx, book.ID, testOwnerID, patchOf(t, map[string]any{"itemType": ItemTypeGame, "platform": "Switch"}))
	if err != nil {
		t.Fatalf("update to game failed: %v", err)
	}
	if game.Format != "" || game.Genre != "" || game.Rating != nil || game.RetailPriceUsd != nil || game.GoogleVolumeId != "" {
		t.Fatalf("expected book-only fields to be cleared, got %+v", game)
	}
	if game.Platform != "Switch" {
		t.Fatalf("expected platform to be kept for game, got %q", game.Platform)
	}

	movie, err := svc.Update(ctx, book.ID, testOwnerID, patchOf(t, map[string]any{"itemType": ItemTypeMovie}))
	if err != nil {
		t.Fatalf("update to movie failed: %v", err)
	}
	if movie.Platform != "" || movie.AgeGroup != "" || movie.PlayerCount != "" {
		t.Fatalf("expected game-only fields to be cleared, got %+v", movie)
	}
}

func TestServiceClearsBookIdentifiersForNonBooks(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryRepository(nil)
	svc := NewService(repo)
	pages := 320

	book, err := svc.Create(ctx, CreateItemInput{OwnerID: testOwnerID, Title: "Was A Book", ItemType: ItemTypeBook, ISBN13: "9780000000001", ISBN10: "0000000001", PageCount: &pages})
	if err != nil {
		t.Fatalf("create book failed: %v", err)
	}
	game, err := svc.Update(ctx, book.ID, testOwnerID, patchOf(t, map[string]any{"itemType": ItemTypeGame}))
	if err != nil {
		t.Fatalf("update to game failed: %v", err)
	}
	if game.ISBN13 != "" || game.ISBN10 != "" || game.PageCount != nil {
		t.Fatalf("expected ISBNs and page count to be cleared, got %+v", game)
	}
	if _, err := repo.FindByISBN(ctx, "9780000000001", testOwnerID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected former ISBN not to match the converted item, got %v", err)
	}

	movie, err := svc.Create(ctx, CreateItemInput{OwnerID: testOwnerID, Title: "Movie", ItemType: ItemTypeMovie, ISBN13: "9780000000002", PageCount: &pages})
	if err != nil {
		t.Fatalf("create movie failed: %v", err)
	}
	if movie.ISBN13 != "" || movie.PageCount != nil {
		t.Fatalf("expected book-only fields to be dropped on create, got %+v", movie)
	}
}

func TestServiceRejectsOversizedCoverImage(t *testing.T) {
	repo := NewInMemoryRepository(nil)
	svc := NewService(repo)

	bigPayload := makeDataURICoverBytes(maxCoverImageBytes + 1)
	_, err := svc.Create(context.Background(), CreateItemInput{OwnerID: testOwnerID, Title: "Big Cover", ItemType: ItemTypeBook, CoverImage: bigPayload})
	if err == nil {
		t.Fatalf("expected oversized cover image to be rejected")
	}

	updatePayload := bigPayload
	item, _ := svc.Create(context.Background(), CreateItemInput{OwnerID: testOwnerID, Title: "Small cover", ItemType: ItemTypeBook, CoverImage: "data:image/png;base64,aA=="})
	_, err = svc.Update(context.Background(), item.ID, testOwnerID, patchOf(t, map[string]any{"coverImage": updatePayload}))
	if err == nil {
		t.Fatalf("expected oversized cover image to be rejected on update")
	}
}

func TestServiceRejectsNonImageDataURI(t *testing.T) {
	repo := NewInMemoryRepository(nil)
	svc := NewService(repo)

	// Attempt to use a non-image MIME type (e.g., text/html for XSS)
	htmlPayload := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte("<script>alert(1)</script>"))
	_, err := svc.Create(context.Background(), CreateItemInput{OwnerID: testOwnerID, Title: "XSS Attempt", ItemType: ItemTypeBook, CoverImage: htmlPayload})
	if err == nil {
		t.Fatalf("expected non-image data URI to be rejected")
	}
	if !strings.Contains(err.Error(), "valid image type") {
		t.Fatalf("expected error about image type, got: %v", err)
	}

	// Attempt with application/javascript
	jsPayload := "data:application/javascript;base64," + base64.StdEncoding.EncodeToString([]byte("alert(1)"))
	_, err = svc.Create(context.Background(), CreateItemInput{OwnerID: testOwnerID, Title: "JS Attempt", ItemType: ItemTypeBook, CoverImage: jsPayload})
	if err == nil {
		t.Fatalf("expected non-image data URI to be rejected")
	}
}

func TestServiceValidatesBookStatusTransitions(t *testing.T) {
	repo := NewInMemoryRepository(nil)
	svc := NewService(repo)

	pageCount := 400
	book, err := svc.Create(context.Background(), CreateItemInput{OwnerID: testOwnerID, Title: "Status", ItemType: ItemTypeBook, PageCount: &pageCount})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	_, err = svc.Update(context.Background(), book.ID, testOwnerID, patchOf(t, map[string]any{"readingStatus": BookStatusRead}))
	if err == nil {
		t.Fatalf("expected read status to require readAt")
	}

	readAt := time.Date(2023, 3, 5, 0, 0, 0, 0, time.UTC)
	updated, err := svc.Update(context.Background(), book.ID, testOwnerID, patchOf(t, map[string]any{"readingStatus": BookStatusRead, "readAt": readAt}))
	if err != nil {
		t.Fatalf("expected valid status update, got %v", err)
	}
	if updated.ReadingStatus != BookStatusRead {
		t.Fatalf("expected status to update to read")
	}
	if updated.ReadAt == nil || !updated.ReadAt.Equal(readAt) {
		t.Fatalf("expected readAt to persist")
	}
	if updated.CurrentPage != nil {
		t.Fatalf("expected current page to clear for read status")
	}

	readingUpdate, err := svc.Update(context.Background(), book.ID, testOwnerID, patchOf(t, map[string]any{
		"readingStatus": BookStatusReading,
		"currentPage":   120,
	}))
	if err != nil {
		t.Fatalf("expected reading status update to succeed: %v", err)
	}
	if readingUpdate.CurrentPage == nil || *readingUpdate.CurrentPage != 120 {
		t.Fatalf("expected current page to persist")
	}

	_, err = svc.Update(context.Background(), book.ID, testOwnerID, patchOf(t, map[string]any{"currentPage": 999}))
	if err == nil {
		t.Fatalf("expected error when current page exceeds total")
	}

	cleared, err := svc.Update(context.Background(), book.ID, testOwnerID, patchOf(t, map[string]any{"readingStatus": BookStatusNone}))
	if err != nil {
		t.Fatalf("expected clearing reading status to succeed: %v", err)
	}
	if cleared.ReadingStatus != BookStatusNone {
		t.Fatalf("expected reading status to clear, got %q", cleared.ReadingStatus)
	}
	if cleared.ReadAt != nil {
		t.Fatalf("expected readAt to clear when status removed")
	}
	if cleared.CurrentPage != nil {
		t.Fatalf("expected current page to clear when status removed")
	}

	_, err = svc.Update(context.Background(), book.ID, testOwnerID, patchOf(t, map[string]any{"readingStatus": BookStatusReading, "currentPage": -5}))
	if err == nil {
		t.Fatalf("expected negative current page to fail")
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

type seriesUpdateRepo struct {
	t            *testing.T
	updateCalled bool
	summaries    map[string]SeriesSummary
	errByName    map[string]error
}

func (r *seriesUpdateRepo) Create(context.Context, Item) (Item, error) {
	r.t.Helper()
	r.t.Fatalf("unexpected Create call")
	return Item{}, nil
}

func (r *seriesUpdateRepo) Get(context.Context, uuid.UUID, uuid.UUID) (Item, error) {
	r.t.Helper()
	r.t.Fatalf("unexpected Get call")
	return Item{}, nil
}

func (r *seriesUpdateRepo) List(context.Context, ListOptions) ([]Item, error) {
	r.t.Helper()
	r.t.Fatalf("unexpected List call")
	return nil, nil
}

func (r *seriesUpdateRepo) ListByIDs(context.Context, []uuid.UUID, uuid.UUID) ([]Item, error) {
	r.t.Helper()
	r.t.Fatalf("unexpected ListByIDs call")
	return nil, nil
}

func (r *seriesUpdateRepo) FindByISBN(context.Context, string, uuid.UUID) (Item, error) {
	r.t.Helper()
	r.t.Fatalf("unexpected FindByISBN call")
	return Item{}, nil
}

func (r *seriesUpdateRepo) Update(context.Context, Item) (Item, error) {
	r.t.Helper()
	r.t.Fatalf("unexpected Update call")
	return Item{}, nil
}

func (r *seriesUpdateRepo) Delete(context.Context, uuid.UUID, uuid.UUID) error {
	r.t.Helper()
	r.t.Fatalf("unexpected Delete call")
	return nil
}

func (r *seriesUpdateRepo) Histogram(context.Context, HistogramOptions) (LetterHistogram, error) {
	r.t.Helper()
	r.t.Fatalf("unexpected Histogram call")
	return nil, nil
}

func (r *seriesUpdateRepo) FindDuplicates(context.Context, DuplicateCheckInput, uuid.UUID) ([]DuplicateMatch, error) {
	r.t.Helper()
	r.t.Fatalf("unexpected FindDuplicates call")
	return nil, nil
}

func (r *seriesUpdateRepo) ListSeries(context.Context, SeriesRepoListOptions, uuid.UUID) ([]SeriesSummary, error) {
	r.t.Helper()
	r.t.Fatalf("unexpected ListSeries call")
	return nil, nil
}

func (r *seriesUpdateRepo) GetSeriesByName(_ context.Context, name string, _ uuid.UUID) (SeriesSummary, error) {
	if err, ok := r.errByName[name]; ok {
		return SeriesSummary{}, err
	}
	if summary, ok := r.summaries[name]; ok {
		return summary, nil
	}
	return SeriesSummary{}, ErrNotFound
}

func (r *seriesUpdateRepo) ListSeriesNamesByNameCI(_ context.Context, name string, _ uuid.UUID) ([]string, error) {
	if err, ok := r.errByName[name]; ok {
		return nil, err
	}
	names := []string{}
	for seriesName := range r.summaries {
		if strings.EqualFold(seriesName, name) {
			names = append(names, seriesName)
		}
	}
	slices.Sort(names)
	return names, nil
}

func (r *seriesUpdateRepo) UpdateSeriesName(context.Context, string, string, uuid.UUID) (int64, error) {
	r.updateCalled = true
	return 1, nil
}

func (r *seriesUpdateRepo) ClearSeriesName(context.Context, string, uuid.UUID) (int64, error) {
	r.t.Helper()
	r.t.Fatalf("unexpected ClearSeriesName call")
	return 0, nil
}

func TestServiceAllowsDataURIsLongerThanURLLimitWhenUnderByteCap(t *testing.T) {
	repo := NewInMemoryRepository(nil)
	svc := NewService(repo)

	payload := makeDataURICoverBytes(4000)
	if len(payload) <= maxCoverImageURLLength {
		t.Fatalf("expected payload length to exceed url limit")
	}

	item, err := svc.Create(context.Background(), CreateItemInput{
		OwnerID:    testOwnerID,
		Title:      "Large data URI",
		ItemType:   ItemTypeBook,
		CoverImage: payload,
	})
	if err != nil {
		t.Fatalf("expected data URI under byte cap to be accepted, got error: %v", err)
	}
	if item.CoverImage != payload {
		t.Fatalf("expected cover image to be stored, got %q", item.CoverImage)
	}

	updatePayload := makeDataURICoverBytes(4200)
	updated, err := svc.Update(context.Background(), item.ID, testOwnerID, patchOf(t, map[string]any{"coverImage": updatePayload}))
	if err != nil {
		t.Fatalf("expected update to accept long data URI, got error: %v", err)
	}
	if updated.CoverImage != updatePayload {
		t.Fatalf("expected cover image to be updated, got %q", updated.CoverImage)
	}
}

func makeDataURICoverBytes(byteCount int) string {
	data := bytes.Repeat([]byte{0x42}, byteCount)
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
}

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

func TestNormalizeTitle(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"The Great Gatsby", "the great gatsby"},
		{"  The Great Gatsby  ", "the great gatsby"},
		{"THE GREAT GATSBY", "the great gatsby"},
		{"", ""},
		{"   ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeTitle(tt.input)
			if got != tt.want {
				t.Fatalf("NormalizeTitle(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeIdentifier(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"9780132350884", "9780132350884"},
		{"978-0132350884", "9780132350884"},
		{"978 0132350884", "9780132350884"},
		{"978-0-13235-088-4", "9780132350884"},
		{"", ""},
		{"   ", ""},
		{"abc", ""},
		{"0-13235-0882", "0132350882"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeIdentifier(tt.input)
			if got != tt.want {
				t.Fatalf("NormalizeIdentifier(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
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
