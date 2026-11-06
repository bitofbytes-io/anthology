package items

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"
)

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
