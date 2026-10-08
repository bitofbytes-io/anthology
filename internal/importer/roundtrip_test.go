package importer

import (
	"bytes"
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"anthology/internal/exporter"
	"anthology/internal/items"
)

func TestCSVExportImportRoundTrip(t *testing.T) {
	ctx := context.Background()
	sourceOwner := uuid.MustParse("00000000-0000-0000-0000-00000000000a")
	targetOwner := uuid.MustParse("00000000-0000-0000-0000-00000000000b")

	intPtr := func(v int) *int { return &v }
	floatPtr := func(v float64) *float64 { return &v }
	at := func(day int) *time.Time {
		v := time.Date(2024, time.March, day, 10, 30, 0, 0, time.UTC)
		return &v
	}

	source := items.NewService(items.NewInMemoryRepository(nil))
	inputs := []items.CreateItemInput{
		{
			Title:          "=SUM(A1:A2)",
			Creator:        "@author",
			ItemType:       items.ItemTypeBook,
			ReleaseYear:    intPtr(2001),
			PageCount:      intPtr(320),
			CurrentPage:    intPtr(42),
			ISBN13:         "9780000000001",
			ISBN10:         "0000000001",
			Description:    "Line one\nLine two, with comma and \"quotes\"",
			CoverImage:     "https://example.com/cover.jpg",
			Format:         items.FormatHardcover,
			Genre:          items.GenreFiction,
			Rating:         intPtr(8),
			RetailPriceUsd: floatPtr(19.99),
			GoogleVolumeId: "vol-123",
			ReadingStatus:  items.BookStatusReading,
			Notes:          "-starts with a dash",
			SeriesName:     "+Plus Series",
			VolumeNumber:   intPtr(2),
			TotalVolumes:   intPtr(5),
			CreatedAt:      at(1),
			UpdatedAt:      at(2),
		},
		{
			Title:         "Finished Book",
			Creator:       "'=already quoted",
			ItemType:      items.ItemTypeBook,
			ReadingStatus: items.BookStatusRead,
			ReadAt:        at(3),
			Notes:         "'plain apostrophe",
			SeriesName:    "Plain Series",
			VolumeNumber:  intPtr(1),
			CreatedAt:     at(3),
			UpdatedAt:     at(4),
		},
		{
			Title:       "Inline Cover Game",
			ItemType:    items.ItemTypeGame,
			CoverImage:  "data:image/png;base64,iVBORw0KGgo=",
			Platform:    "+Switch",
			AgeGroup:    "12+",
			PlayerCount: "1-4",
			CreatedAt:   at(5),
			UpdatedAt:   at(5),
		},
		{
			Title:     "A Movie",
			Creator:   "Director",
			ItemType:  items.ItemTypeMovie,
			CreatedAt: at(6),
			UpdatedAt: at(6),
		},
	}
	for _, input := range inputs {
		input.OwnerID = sourceOwner
		if _, err := source.Create(ctx, input); err != nil {
			t.Fatalf("seed %q: %v", input.Title, err)
		}
	}
	exported, err := source.List(ctx, items.ListOptions{OwnerID: sourceOwner})
	if err != nil {
		t.Fatalf("list source items: %v", err)
	}

	var buf bytes.Buffer
	if err := exporter.NewCSVExporter().Export(&buf, exported); err != nil {
		t.Fatalf("export: %v", err)
	}

	csvBytes := buf.Bytes()

	t.Run("import", func(t *testing.T) {
		target := items.NewService(items.NewInMemoryRepository(nil))
		summary, err := NewCSVImporter(target, nil).Import(ctx, bytes.NewReader(csvBytes), targetOwner)
		if err != nil {
			t.Fatalf("import: %v", err)
		}
		if summary.Imported != len(inputs) || len(summary.Failed) != 0 || len(summary.SkippedDuplicates) != 0 {
			t.Fatalf("unexpected import summary: %+v", summary)
		}
		assertRoundTrip(t, target, targetOwner, exported)
	})

	t.Run("preview and commit", func(t *testing.T) {
		target := items.NewService(items.NewInMemoryRepository(nil))
		importer := NewCSVImporter(target, nil)
		preview, err := importer.Preview(ctx, bytes.NewReader(csvBytes), targetOwner)
		if err != nil {
			t.Fatalf("preview: %v", err)
		}
		if preview.Ready != len(inputs) {
			t.Fatalf("expected every exported row to be ready, got %+v", preview)
		}
		result, err := importer.Commit(ctx, reviewedRows(t, preview, nil), targetOwner)
		if err != nil {
			t.Fatalf("commit: %v", err)
		}
		if result.Added != len(inputs) {
			t.Fatalf("unexpected commit result: %+v", result)
		}
		assertRoundTrip(t, target, targetOwner, exported)
	})
}

func assertRoundTrip(t *testing.T, target *items.Service, targetOwner uuid.UUID, exported []items.Item) {
	t.Helper()
	imported, err := target.List(context.Background(), items.ListOptions{OwnerID: targetOwner})
	if err != nil {
		t.Fatalf("list imported items: %v", err)
	}
	byTitle := make(map[string]items.Item, len(imported))
	for _, item := range imported {
		byTitle[item.Title] = item
	}

	for _, want := range exported {
		got, ok := byTitle[want.Title]
		if !ok {
			t.Fatalf("item %q missing after round trip", want.Title)
		}
		// Identity differs by design, and inline data: URI covers are omitted
		// from exports to keep files importable.
		want.ID, got.ID = uuid.Nil, uuid.Nil
		want.OwnerID, got.OwnerID = uuid.Nil, uuid.Nil
		if want.Title == "Inline Cover Game" {
			if got.CoverImage != "" {
				t.Fatalf("expected data URI cover to be omitted, got %q", got.CoverImage)
			}
			want.CoverImage = ""
		}
		if !reflect.DeepEqual(want, got) {
			t.Fatalf("round trip mismatch for %q:\nwant %+v\ngot  %+v", want.Title, want, got)
		}
	}
}
