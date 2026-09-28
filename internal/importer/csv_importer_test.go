package importer

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"anthology/internal/catalog"
	"anthology/internal/items"
)

// testOwnerID is a fixed UUID for tests
var testOwnerID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

type stubStore struct {
	items         []items.Item
	createErr     error
	listed        bool
	createdInputs []items.CreateItemInput
}

func (s *stubStore) Create(ctx context.Context, input items.CreateItemInput) (items.Item, error) {
	if s.createErr != nil {
		return items.Item{}, s.createErr
	}
	s.createdInputs = append(s.createdInputs, input)
	item := items.Item{ID: uuid.New(), Title: input.Title, Creator: input.Creator, ItemType: input.ItemType}
	s.items = append(s.items, item)
	return item, nil
}

func (s *stubStore) List(ctx context.Context, opts items.ListOptions) ([]items.Item, error) {
	s.listed = true
	copies := make([]items.Item, len(s.items))
	copy(copies, s.items)
	return copies, nil
}

type stubCatalog struct {
	metadata []catalog.Metadata
	err      error
}

func (s *stubCatalog) Lookup(ctx context.Context, query string, category catalog.Category) ([]catalog.Metadata, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.metadata, nil
}

func TestCSVImporter_ImportCreatesItemsAndSkipsDuplicates(t *testing.T) {
	store := &stubStore{items: []items.Item{{Title: "Existing Title", OwnerID: testOwnerID}}}
	importer := NewCSVImporter(store, &stubCatalog{})
	csv := "title,creator,itemType,releaseYear,pageCount,isbn13,isbn10,description,coverImage,notes\n" +
		"New Book,Author,book,2020,320,9780000000001,0000000001,Desc,,Note\n" +
		"Existing Title,Someone,book,,,,,,,,\n"
	summary, err := importer.Import(context.Background(), bytes.NewBufferString(csv), testOwnerID)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if summary.Imported != 1 {
		t.Fatalf("expected 1 import, got %d", summary.Imported)
	}
	if len(summary.SkippedDuplicates) != 1 {
		t.Fatalf("expected 1 skipped record, got %d", len(summary.SkippedDuplicates))
	}
}

func TestCSVImporter_PopulatesBookFromLookup(t *testing.T) {
	store := &stubStore{}
	catalog := &stubCatalog{metadata: []catalog.Metadata{{
		Title:    "Lookup Title",
		Creator:  "Lookup Author",
		ItemType: string(items.ItemTypeBook),
		ISBN13:   "9780000000000",
	}}}
	importer := NewCSVImporter(store, catalog)
	csv := "title,creator,itemType,releaseYear,pageCount,isbn13,isbn10,description,coverImage,notes\n" +
		",,book,, ,9780000000000,,,,,\n"
	summary, err := importer.Import(context.Background(), bytes.NewBufferString(csv), testOwnerID)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if summary.Imported != 1 {
		t.Fatalf("expected 1 import, got %d", summary.Imported)
	}
}

func TestCSVImporter_ReturnsRowErrors(t *testing.T) {
	store := &stubStore{}
	importer := NewCSVImporter(store, &stubCatalog{})
	csv := "title,creator,itemType,releaseYear,pageCount,isbn13,isbn10,description,coverImage,notes\n" +
		"Bad Year,Author,book,year,100,,,,,\n"
	summary, err := importer.Import(context.Background(), bytes.NewBufferString(csv), testOwnerID)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if len(summary.Failed) != 1 {
		t.Fatalf("expected 1 failed record, got %d", len(summary.Failed))
	}
}

func TestCSVImporter_MissingColumns(t *testing.T) {
	store := &stubStore{}
	importer := NewCSVImporter(store, &stubCatalog{})
	csv := "title,itemType\nTest,book\n"
	_, err := importer.Import(context.Background(), strings.NewReader(csv), testOwnerID)
	if err == nil {
		t.Fatal("expected error for missing columns")
	}
	if !strings.Contains(err.Error(), "missing required columns") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCSVImporter_RejectsOversizedUploadBeforeWriting(t *testing.T) {
	store := &stubStore{}
	importer := NewCSVImporter(store, &stubCatalog{})

	var builder strings.Builder
	builder.WriteString("title,creator,itemType,releaseYear,pageCount,isbn13,isbn10,description,coverImage,notes\n")
	for idx := 0; idx < MaxImportRows+1; idx++ {
		fmt.Fprintf(&builder, "Title %d,Creator %d,book,2024,100,,,,,\n", idx, idx)
	}

	_, err := importer.Import(context.Background(), strings.NewReader(builder.String()), testOwnerID)
	if err == nil {
		t.Fatal("expected error for oversized CSV")
	}
	if len(store.items) != 0 {
		t.Fatalf("expected no items to be created, got %d", len(store.items))
	}
}

func TestCSVImporter_ImportsExtendedFields(t *testing.T) {
	store := &stubStore{}
	importer := NewCSVImporter(store, &stubCatalog{})
	csv := "title,creator,itemType,releaseYear,pageCount,currentPage,isbn13,isbn10,description,coverImage,format,genre,rating,retailPriceUsd,googleVolumeId,platform,ageGroup,playerCount,readingStatus,readAt,notes,createdAt,updatedAt\n" +
		"Exported Book,Author,book,2020,300,42,9780000000001,0000000001,Desc,https://example.com/cover.jpg,HARDCOVER,FICTION,8,19.99,vol123,,,,read,2024-01-10T00:00:00Z,Note,2024-01-01T00:00:00Z,2024-01-02T00:00:00Z\n"

	summary, err := importer.Import(context.Background(), bytes.NewBufferString(csv), testOwnerID)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if summary.Imported != 1 {
		t.Fatalf("expected 1 import, got %d", summary.Imported)
	}
	if len(store.createdInputs) != 1 {
		t.Fatalf("expected 1 create input, got %d", len(store.createdInputs))
	}

	input := store.createdInputs[0]
	if input.OwnerID != testOwnerID {
		t.Fatalf("expected ownerID to be %s, got %s", testOwnerID, input.OwnerID)
	}
	if input.CurrentPage == nil || *input.CurrentPage != 42 {
		t.Fatalf("expected currentPage to be 42")
	}
	if input.Format != items.FormatHardcover {
		t.Fatalf("expected format to be HARDCOVER, got %s", input.Format)
	}
	if input.Genre != items.GenreFiction {
		t.Fatalf("expected genre to be FICTION, got %s", input.Genre)
	}
	if input.Rating == nil || *input.Rating != 8 {
		t.Fatalf("expected rating to be 8")
	}
	if input.RetailPriceUsd == nil || *input.RetailPriceUsd != 19.99 {
		t.Fatalf("expected retailPriceUsd to be 19.99")
	}
	if input.GoogleVolumeId != "vol123" {
		t.Fatalf("expected googleVolumeId to be vol123")
	}
	if input.ReadingStatus != items.BookStatusRead {
		t.Fatalf("expected readingStatus to be read, got %s", input.ReadingStatus)
	}

	readAt := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)
	if input.ReadAt == nil || !input.ReadAt.Equal(readAt) {
		t.Fatalf("expected readAt to be %s", readAt.Format(time.RFC3339))
	}

	createdAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	if input.CreatedAt == nil || !input.CreatedAt.Equal(createdAt) {
		t.Fatalf("expected createdAt to be %s", createdAt.Format(time.RFC3339))
	}
	updatedAt := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	if input.UpdatedAt == nil || !input.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("expected updatedAt to be %s", updatedAt.Format(time.RFC3339))
	}
}

func TestCSVImporter_LookupFailureDoesNotLeakUpstreamError(t *testing.T) {
	store := &stubStore{}
	upstream := fmt.Errorf("call google books: Get \"https://www.googleapis.com/books/v1/volumes?key=secret-key\": dial tcp: timeout")
	importer := NewCSVImporter(store, &stubCatalog{err: upstream})
	csv := "title,creator,itemType,releaseYear,pageCount,isbn13,isbn10,description,coverImage,notes\n" +
		",,book,,,9780000000000,,,,\n"

	summary, err := importer.Import(context.Background(), strings.NewReader(csv), testOwnerID)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if len(summary.Failed) != 1 {
		t.Fatalf("expected 1 failed record, got %d", len(summary.Failed))
	}
	message := summary.Failed[0].Error
	if strings.Contains(message, "secret-key") || strings.Contains(message, "googleapis") {
		t.Fatalf("failed record leaks upstream error: %q", message)
	}
	if message != errMetadataLookupFailed.Error() {
		t.Fatalf("expected generic lookup failure message, got %q", message)
	}
}

type countingCatalog struct {
	calls int
	block bool
}

func (c *countingCatalog) Lookup(ctx context.Context, query string, category catalog.Category) ([]catalog.Metadata, error) {
	c.calls++
	if c.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return []catalog.Metadata{{Title: "Title " + query, ItemType: string(items.ItemTypeBook)}}, nil
}

func titleLessRows(n int) string {
	var builder strings.Builder
	builder.WriteString("title,creator,itemType,releaseYear,pageCount,isbn13,isbn10,description,coverImage,notes\n")
	for idx := 0; idx < n; idx++ {
		fmt.Fprintf(&builder, ",,book,,,978%010d,,,,\n", idx)
	}
	return builder.String()
}

func TestCSVImporter_CapsCatalogLookupsPerImport(t *testing.T) {
	store := &stubStore{}
	lookups := &countingCatalog{}
	importer := NewCSVImporter(store, lookups)

	summary, err := importer.Import(context.Background(), strings.NewReader(titleLessRows(MaxLookupsPerImport+3)), testOwnerID)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if lookups.calls != MaxLookupsPerImport {
		t.Fatalf("expected %d lookups, got %d", MaxLookupsPerImport, lookups.calls)
	}
	if summary.Imported != MaxLookupsPerImport {
		t.Fatalf("expected %d imports, got %d", MaxLookupsPerImport, summary.Imported)
	}
	if len(summary.Failed) != 3 {
		t.Fatalf("expected 3 failed rows, got %d", len(summary.Failed))
	}
	for _, failed := range summary.Failed {
		if !strings.Contains(failed.Error, "limit of 100 ISBN lookups") {
			t.Fatalf("unexpected failure message: %q", failed.Error)
		}
	}
}

func TestCSVImporter_StopsLookupsWhenTimeBudgetIsSpent(t *testing.T) {
	store := &stubStore{}
	lookups := &countingCatalog{block: true}
	importer := NewCSVImporter(store, lookups)
	importer.lookupBudget = 20 * time.Millisecond

	csv := "title,creator,itemType,releaseYear,pageCount,isbn13,isbn10,description,coverImage,notes\n" +
		",,book,,,9780000000001,,,,\n" +
		",,book,,,9780000000002,,,,\n" +
		"Titled Book,Author,book,,,,,,,\n"

	summary, err := importer.Import(context.Background(), strings.NewReader(csv), testOwnerID)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if lookups.calls != 1 {
		t.Fatalf("expected a single lookup before the budget ran out, got %d", lookups.calls)
	}
	if summary.Imported != 1 {
		t.Fatalf("expected titled row to import, got %d", summary.Imported)
	}
	if len(summary.Failed) != 2 {
		t.Fatalf("expected 2 failed rows, got %d", len(summary.Failed))
	}
	for _, failed := range summary.Failed {
		if failed.Error != errLookupBudgetExhausted.Error() {
			t.Fatalf("unexpected failure message: %q", failed.Error)
		}
	}
}

// cancellingStore cancels the import context during its cancelAt-th Create,
// modelling the request deadline expiring mid-import.
type cancellingStore struct {
	stubStore
	cancel   context.CancelFunc
	calls    int
	cancelAt int
}

func (s *cancellingStore) Create(ctx context.Context, input items.CreateItemInput) (items.Item, error) {
	s.calls++
	if s.calls == s.cancelAt {
		s.cancel()
		return items.Item{}, fmt.Errorf("insert item: %w", context.Canceled)
	}
	return s.stubStore.Create(ctx, input)
}

func TestCSVImporter_ReportsRowsLeftWhenRequestContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := &cancellingStore{cancel: cancel, cancelAt: 2}
	importer := NewCSVImporter(store, &stubCatalog{})
	csv := "title,creator,itemType,releaseYear,pageCount,isbn13,isbn10,description,coverImage,notes\n" +
		"One,,book,,,,,,,\n" +
		"Two,,book,,,,,,,\n" +
		"Three,,book,,,9780000000003,,,,\n" +
		"Four,,movie,,,,,,,\n"

	summary, err := importer.Import(ctx, strings.NewReader(csv), testOwnerID)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if !summary.Interrupted {
		t.Fatal("expected summary to be marked interrupted")
	}
	if summary.Imported != 1 {
		t.Fatalf("expected 1 import before the deadline, got %d", summary.Imported)
	}
	if store.calls != 2 {
		t.Fatalf("expected no inserts after the context ended, got %d calls", store.calls)
	}
	if len(summary.Failed) != 3 {
		t.Fatalf("expected 3 unprocessed rows, got %+v", summary.Failed)
	}
	for idx, want := range []string{"Two", "Three", "Four"} {
		failed := summary.Failed[idx]
		if failed.Title != want || failed.Error != errImportInterrupted.Error() {
			t.Fatalf("unexpected failed record %d: %+v", idx, failed)
		}
	}
	if summary.Failed[1].Identifier != "9780000000003" {
		t.Fatalf("expected identifier for unprocessed row, got %+v", summary.Failed[1])
	}
}

func TestCSVImporter_UnescapesFormulaGuardOnlyForAnthologyExports(t *testing.T) {
	const legacyHeader = "title,creator,itemType,releaseYear,pageCount,isbn13,isbn10,description,coverImage,notes\n"
	cases := []struct {
		name      string
		csv       string
		wantTitle string
		wantNotes string
	}{
		{
			name:      "third-party file keeps apostrophes",
			csv:       legacyHeader + "'=42,,book,,,,,,,'-draft\n",
			wantTitle: "'=42",
			wantNotes: "'-draft",
		},
		{
			name:      "anthology export strips formula guard",
			csv:       "schemaVersion," + legacyHeader + "2,'=42,,book,,,,,,,'-draft\n",
			wantTitle: "=42",
			wantNotes: "-draft",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &stubStore{}
			summary, err := NewCSVImporter(store, &stubCatalog{}).Import(context.Background(), strings.NewReader(tc.csv), testOwnerID)
			if err != nil {
				t.Fatalf("import failed: %v", err)
			}
			if summary.Imported != 1 || len(store.createdInputs) != 1 {
				t.Fatalf("expected one import, got %+v", summary)
			}
			input := store.createdInputs[0]
			if input.Title != tc.wantTitle || input.Notes != tc.wantNotes {
				t.Fatalf("got title %q notes %q; want %q, %q", input.Title, input.Notes, tc.wantTitle, tc.wantNotes)
			}
		})
	}
}
