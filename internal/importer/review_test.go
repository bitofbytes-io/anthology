package importer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"anthology/internal/catalog"
	"anthology/internal/items"
)

var otherOwnerID = uuid.MustParse("00000000-0000-0000-0000-000000000002")

const reviewHeader = "title,creator,itemType,releaseYear,pageCount,isbn13,isbn10,description,coverImage,notes,genre,readingStatus\n"

// faultRepo wraps the in-memory repository at the persistence boundary: it
// counts every insert (plain creates and import batch inserts) and lets tests
// pause after a batch reads the library or replace a batch insert's result.
type faultRepo struct {
	*items.InMemoryRepository
	creates atomic.Int32
	// onList runs after an import batch reads the library.
	onList func()
	// fault, when set, decides the result of each batch insert; insert
	// performs the real insert.
	fault func(ctx context.Context, call int32, insert func() (items.Item, error)) (items.Item, error)
}

func (r *faultRepo) Create(ctx context.Context, item items.Item) (items.Item, error) {
	r.creates.Add(1)
	return r.InMemoryRepository.Create(ctx, item)
}

func (r *faultRepo) BeginExclusiveInserts(ctx context.Context, ownerID uuid.UUID) (items.InsertSession, error) {
	session, err := r.InMemoryRepository.BeginExclusiveInserts(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	return &faultSession{InsertSession: session, repo: r}, nil
}

type faultSession struct {
	items.InsertSession
	repo *faultRepo
}

func (s *faultSession) List(ctx context.Context) ([]items.Item, error) {
	list, err := s.InsertSession.List(ctx)
	if s.repo.onList != nil {
		s.repo.onList()
	}
	return list, err
}

func (s *faultSession) Insert(ctx context.Context, item items.Item) (items.Item, error) {
	call := s.repo.creates.Add(1)
	insert := func() (items.Item, error) { return s.InsertSession.Insert(ctx, item) }
	if s.repo.fault != nil {
		return s.repo.fault(ctx, call, insert)
	}
	return insert()
}

// libraryStore is a real items.Service over a faultRepo.
type libraryStore struct {
	*items.Service
	repo    *faultRepo
	creates *atomic.Int32
}

func newLibraryStore(t *testing.T, seed ...items.CreateItemInput) *libraryStore {
	t.Helper()
	repo := &faultRepo{InMemoryRepository: items.NewInMemoryRepository(nil)}
	store := &libraryStore{Service: items.NewService(repo), repo: repo, creates: &repo.creates}
	for _, input := range seed {
		if _, err := store.Service.Create(context.Background(), input); err != nil {
			t.Fatalf("seed %q: %v", input.Title, err)
		}
	}
	repo.creates.Store(0)
	return store
}

func (s *libraryStore) owned(t *testing.T, ownerID uuid.UUID) []items.Item {
	t.Helper()
	list, err := s.Service.List(context.Background(), items.ListOptions{OwnerID: ownerID})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	return list
}

// editionCatalog returns fixed matches per query and counts lookups.
type editionCatalog struct {
	mu      sync.Mutex
	calls   int
	results map[string][]catalog.Metadata
	errs    map[string]error
}

func (c *editionCatalog) Lookup(ctx context.Context, query string, category catalog.Category) ([]catalog.Metadata, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if err := c.errs[query]; err != nil {
		return nil, err
	}
	if results, ok := c.results[query]; ok {
		return results, nil
	}
	return nil, catalog.ErrNotFound
}

func (c *editionCatalog) lookups() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func intPtr(v int) *int { return &v }

func rowByNumber(t *testing.T, preview Preview, number int) PreviewRow {
	t.Helper()
	for _, row := range preview.Rows {
		if row.Row == number {
			return row
		}
	}
	t.Fatalf("row %d missing from preview", number)
	return PreviewRow{}
}

func twoEditions() *editionCatalog {
	return &editionCatalog{results: map[string][]catalog.Metadata{
		"9780000000100": {
			{Title: "First Edition", Creator: "Catalog Author", ISBN13: "9780000000100", GoogleVolumeId: "vol-first", Genre: "FICTION", Description: "Catalog description", PageCount: intPtr(200)},
			{Title: "Second Edition", Creator: "Catalog Author", ISBN13: "9780000000100", GoogleVolumeId: "vol-second", Genre: "HISTORY", PageCount: intPtr(250)},
		},
	}}
}

func TestPreviewClassifiesRowsWithoutWriting(t *testing.T) {
	store := newLibraryStore(t,
		items.CreateItemInput{OwnerID: testOwnerID, Title: "Owned Title", ItemType: items.ItemTypeBook},
		items.CreateItemInput{OwnerID: testOwnerID, Title: "Owned By ISBN", ItemType: items.ItemTypeBook, ISBN13: "9780000000555"},
	)
	lookups := twoEditions()
	csv := reviewHeader +
		"Ready Book,Author,book,2020,100,9780000000001,,,,,,\n" + // row 2
		"\n" + // row 3 is blank and skipped
		"owned title ,,book,,,,,,,,,\n" + // row 4: library title, case-insensitive
		"Another,,book,,,978-0-00-000055-5,,,,,,\n" + // row 5: library ISBN-13
		"READY BOOK,,movie,,,,,,,,,\n" + // row 6: earlier row of the file
		",,book,,,9780000000100,,,,Kept note,,\n" + // row 7: two catalog matches
		"Bad Year,,book,year,,,,,,,,\n" + // row 8: parse error
		"Read Book,,book,,,,,,,,,read\n" + // row 9: Create validation error
		"Insecure Cover,,game,,,,,,http://example.com/c.jpg,,,\n" + // row 10: Create validation error
		",,book,,,9789999999999,,,,,,\n" // row 11: no catalog match

	preview, err := NewCSVImporter(store, lookups).Preview(context.Background(), strings.NewReader(csv), testOwnerID)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if got := store.creates.Load(); got != 0 {
		t.Fatalf("preview must not create items, got %d creates", got)
	}
	if got := len(store.owned(t, testOwnerID)); got != 2 {
		t.Fatalf("library changed during preview: %d items", got)
	}
	if preview.TotalRows != 9 || preview.Ready != 1 || preview.Duplicates != 3 || preview.NeedsMatch != 5 {
		t.Fatalf("unexpected counts: %+v", preview)
	}

	want := map[int]RowStatus{2: RowReady, 4: RowDuplicate, 5: RowDuplicate, 6: RowDuplicate, 7: RowNeedsMatch, 8: RowNeedsMatch, 9: RowNeedsMatch, 10: RowNeedsMatch, 11: RowNeedsMatch}
	for number, status := range want {
		if row := rowByNumber(t, preview, number); row.Status != status {
			t.Fatalf("row %d: status %s, want %s (%+v)", number, row.Status, status, row)
		}
	}

	ready := rowByNumber(t, preview, 2)
	if ready.Item == nil || ready.Item.Title != "Ready Book" || ready.Item.Format != items.FormatUnknown || ready.Item.ReadingStatus != items.BookStatusNone {
		t.Fatalf("expected normalized ready item, got %+v", ready.Item)
	}
	if !slices.Equal(ready.Keys, []string{"title:ready book", "isbn13:9780000000001"}) {
		t.Fatalf("unexpected keys %v", ready.Keys)
	}

	byTitle := rowByNumber(t, preview, 4)
	if len(byTitle.LibraryMatches) != 1 || byTitle.LibraryMatches[0].Field != "title" || byTitle.LibraryMatches[0].Title != "Owned Title" || byTitle.LibraryMatches[0].ItemID == uuid.Nil {
		t.Fatalf("expected library title match, got %+v", byTitle.LibraryMatches)
	}
	byISBN := rowByNumber(t, preview, 5)
	if len(byISBN.LibraryMatches) != 1 || byISBN.LibraryMatches[0].Field != "isbn13" {
		t.Fatalf("expected library isbn13 match, got %+v", byISBN.LibraryMatches)
	}
	inFile := rowByNumber(t, preview, 6)
	if inFile.FileMatch == nil || inFile.FileMatch.Row != 2 || inFile.FileMatch.Field != "title" || len(inFile.LibraryMatches) != 0 {
		t.Fatalf("expected in-file match with row 2, got %+v", inFile)
	}

	choose := rowByNumber(t, preview, 7)
	if choose.Problem != chooseMatchProblem || len(choose.Candidates) != 2 || choose.Item != nil {
		t.Fatalf("expected two candidates to choose from, got %+v", choose)
	}

	problems := map[int]string{
		8:  "releaseYear must be a number",
		9:  "readAt is required when readingStatus is read",
		10: "coverImage URL must use HTTPS",
		11: "no metadata found for 9789999999999",
	}
	for number, message := range problems {
		if row := rowByNumber(t, preview, number); row.Problem != message {
			t.Fatalf("row %d: problem %q, want %q", number, row.Problem, message)
		}
	}
}

func TestPreviewOffersEveryCatalogMatchKeepingCSVValues(t *testing.T) {
	store := newLibraryStore(t, items.CreateItemInput{OwnerID: testOwnerID, Title: "Second Edition", ItemType: items.ItemTypeBook})
	csv := reviewHeader + ",CSV Author,book,,,9780000000100,,,,CSV note,BIOGRAPHY,\n"

	preview, err := NewCSVImporter(store, twoEditions()).Preview(context.Background(), strings.NewReader(csv), testOwnerID)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	row := rowByNumber(t, preview, 2)
	if len(row.Candidates) != 2 {
		t.Fatalf("expected both editions, got %+v", row.Candidates)
	}
	first, second := row.Candidates[0], row.Candidates[1]
	if first.Item.Title != "First Edition" || second.Item.Title != "Second Edition" {
		t.Fatalf("expected catalog order, got %q, %q", first.Item.Title, second.Item.Title)
	}
	for _, candidate := range row.Candidates {
		if candidate.Item.Creator != "CSV Author" || candidate.Item.Notes != "CSV note" || candidate.Item.Genre != items.GenreBiography {
			t.Fatalf("CSV values must win, got %+v", candidate.Item)
		}
		if !slices.Equal(candidate.CSVOverrides, []string{"creator", "genre"}) {
			t.Fatalf("unexpected overrides %v", candidate.CSVOverrides)
		}
	}
	if first.Item.GoogleVolumeId != "vol-first" || *first.Item.PageCount != 200 || first.Item.Description != "Catalog description" {
		t.Fatalf("expected first edition metadata, got %+v", first.Item)
	}
	if len(first.LibraryMatches) != 0 {
		t.Fatalf("first edition is not owned, got %+v", first.LibraryMatches)
	}
	if len(second.LibraryMatches) != 1 || second.LibraryMatches[0].Field != "title" {
		t.Fatalf("second edition duplicates the library title, got %+v", second.LibraryMatches)
	}
}

func TestPreviewExplainsLookupProblems(t *testing.T) {
	upstream := fmt.Errorf("call google books: Get \"https://www.googleapis.com/books/v1/volumes?key=secret\": timeout")
	lookups := &editionCatalog{errs: map[string]error{
		"12":            catalog.ErrInvalidQuery,
		"9780000000003": upstream,
	}}
	csv := reviewHeader +
		",,book,,,12,,,,,,\n" +
		",,book,,,9780000000003,,,,,,\n" +
		",,book,,,,,,,,,\n" +
		",,movie,,,,,,,,,\n" +
		"Typo,,comic,,,,,,,,,\n"
	preview, err := NewCSVImporter(newLibraryStore(t), lookups).Preview(context.Background(), strings.NewReader(csv), testOwnerID)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	want := []string{
		"ISBN/UPC 12 is not valid",
		errMetadataLookupFailed.Error(),
		"provide a title or ISBN/UPC for books",
		"title is required for movie rows",
		"itemType must be one of book, game, movie, or music",
	}
	for idx, message := range want {
		row := preview.Rows[idx]
		if row.Status != RowNeedsMatch || row.Problem != message {
			t.Fatalf("row %d: got %s %q, want %q", row.Row, row.Status, row.Problem, message)
		}
	}
	if row := preview.Rows[4]; row.Title != "Typo" || row.ItemType != "comic" {
		t.Fatalf("expected original row details for display, got %+v", row)
	}
}

func TestPreviewKeepsLookupLimits(t *testing.T) {
	lookups := &countingCatalog{}
	importer := NewCSVImporter(newLibraryStore(t), lookups)
	importer.maxLookups = 2

	preview, err := importer.Preview(context.Background(), strings.NewReader(titleLessRows(3)), testOwnerID)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if lookups.calls != 2 {
		t.Fatalf("expected 2 lookups, got %d", lookups.calls)
	}
	if len(preview.Rows[0].Candidates) != 1 || len(preview.Rows[1].Candidates) != 1 {
		t.Fatalf("expected candidates for the looked-up rows, got %+v", preview.Rows[:2])
	}
	if !strings.Contains(preview.Rows[2].Problem, "limit of 2 ISBN lookups") {
		t.Fatalf("expected lookup limit problem, got %q", preview.Rows[2].Problem)
	}
}

func TestPreviewRejectsInvalidFilesBeforeLookups(t *testing.T) {
	lookups := &countingCatalog{}
	var oversized strings.Builder
	oversized.WriteString(titleLessRows(MaxImportRows + 1))
	for name, csv := range map[string]string{
		"missing columns": "title,itemType\nA,book\n",
		"empty":           "",
		"too many rows":   oversized.String(),
	} {
		t.Run(name, func(t *testing.T) {
			store := newLibraryStore(t)
			_, err := NewCSVImporter(store, lookups).Preview(context.Background(), strings.NewReader(csv), testOwnerID)
			if !errors.Is(err, ErrInvalidCSV) {
				t.Fatalf("expected invalid CSV error, got %v", err)
			}
			if lookups.calls != 0 || store.creates.Load() != 0 {
				t.Fatalf("expected no lookups or writes, got %d lookups, %d creates", lookups.calls, store.creates.Load())
			}
		})
	}
}

func TestPreviewWithCanceledContextWritesNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := newLibraryStore(t)
	lookups := &countingCatalog{}
	preview, err := NewCSVImporter(store, lookups).Preview(ctx, strings.NewReader(reviewHeader+"Ready,,book,,,,,,,,,\n,,book,,,9780000000001,,,,,,\n"), testOwnerID)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if store.creates.Load() != 0 || lookups.calls != 0 {
		t.Fatalf("expected no writes or lookups, got %d creates, %d lookups", store.creates.Load(), lookups.calls)
	}
	if preview.Rows[1].Problem != errLookupBudgetExhausted.Error() {
		t.Fatalf("expected lookup budget problem, got %q", preview.Rows[1].Problem)
	}
}

func TestPreviewDuplicateRulesAreExactAndPerOwner(t *testing.T) {
	store := newLibraryStore(t,
		items.CreateItemInput{OwnerID: testOwnerID, Title: "Dune", ItemType: items.ItemTypeBook, ISBN13: "9780441172719", ISBN10: "0441172717"},
		items.CreateItemInput{OwnerID: otherOwnerID, Title: "Someone Else's Book", ItemType: items.ItemTypeBook, ISBN13: "9780000000999"},
	)
	csv := reviewHeader +
		"Dune Messiah,,book,,,,,,,,,\n" + // no fuzzy title matching
		"The Dune,,book,,,,,,,,,\n" +
		"Other,,book,,,0441172717,,,,,,\n" + // an ISBN-10 in the ISBN-13 column only matches ISBN-13s
		"Someone Else's Book,,book,,,9780000000999,,,,,,\n" + // another owner's item
		"Spaced,,book,,,,0-441-17271-7,,,,,\n" // ISBN-10 digits with separators
	preview, err := NewCSVImporter(store, nil).Preview(context.Background(), strings.NewReader(csv), testOwnerID)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	statuses := make([]RowStatus, 0, len(preview.Rows))
	for _, row := range preview.Rows {
		statuses = append(statuses, row.Status)
	}
	want := []RowStatus{RowReady, RowReady, RowReady, RowReady, RowDuplicate}
	if !slices.Equal(statuses, want) {
		t.Fatalf("statuses %v, want %v", statuses, want)
	}
	if match := preview.Rows[4].LibraryMatches; len(match) != 1 || match[0].Field != "isbn10" || match[0].Title != "Dune" {
		t.Fatalf("expected isbn10 match with Dune, got %+v", match)
	}
}

// reviewedRows mimics the browser: it sends every ready row's item, plus the
// chosen candidate for rows in choices, through a JSON round trip.
func reviewedRows(t *testing.T, preview Preview, choices map[int]int) CommitRequest {
	t.Helper()
	request := CommitRequest{}
	for _, row := range preview.Rows {
		switch {
		case row.Status == RowReady:
			request.Rows = append(request.Rows, ReviewedRow{Row: row.Row, Item: *row.Item})
		case row.Status == RowNeedsMatch:
			if choice, ok := choices[row.Row]; ok {
				request.Rows = append(request.Rows, ReviewedRow{Row: row.Row, Item: row.Candidates[choice].Item})
			}
		}
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}
	var decoded CommitRequest
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	return decoded
}

func TestCommitSavesChosenEditionWithoutLookups(t *testing.T) {
	store := newLibraryStore(t)
	lookups := twoEditions()
	importer := NewCSVImporter(store, lookups)
	csv := reviewHeader +
		"Plain,Author,book,2001,,,,,,Plain note,,\n" +
		",CSV Author,book,,,9780000000100,,,,CSV note,,\n"

	preview, err := importer.Preview(context.Background(), strings.NewReader(csv), testOwnerID)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	previewLookups := lookups.lookups()
	request := reviewedRows(t, preview, map[int]int{3: 1})

	result, err := importer.Commit(context.Background(), request, testOwnerID)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if lookups.lookups() != previewLookups {
		t.Fatalf("commit must not look anything up, got %d extra lookups", lookups.lookups()-previewLookups)
	}
	if result.Added != 2 || len(result.Rows) != 2 || result.Rows[1].ItemID == nil {
		t.Fatalf("unexpected result: %+v", result)
	}

	saved := store.owned(t, testOwnerID)
	var chosen items.Item
	for _, item := range saved {
		if item.Title == "Second Edition" {
			chosen = item
		}
	}
	if chosen.ID != *result.Rows[1].ItemID {
		t.Fatalf("expected the chosen edition to be saved, got %+v", saved)
	}
	if chosen.GoogleVolumeId != "vol-second" || chosen.Genre != items.GenreHistory || *chosen.PageCount != 250 || chosen.Creator != "CSV Author" || chosen.Notes != "CSV note" {
		t.Fatalf("saved item lost the chosen metadata or CSV values: %+v", chosen)
	}
}

func TestCommitForcesOwnerAndKeepsReviewedFields(t *testing.T) {
	store := newLibraryStore(t)
	csv := "schemaVersion,title,creator,itemType,releaseYear,pageCount,isbn13,isbn10,description,coverImage,notes,createdAt,updatedAt\n" +
		"2,'=Formula,Author,book,1999,320,,,,https://example.com/c.jpg,Note,2024-01-01T00:00:00Z,2024-01-02T00:00:00Z\n"
	importer := NewCSVImporter(store, nil)
	preview, err := importer.Preview(context.Background(), strings.NewReader(csv), testOwnerID)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if _, err := importer.Commit(context.Background(), reviewedRows(t, preview, nil), testOwnerID); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if len(store.owned(t, otherOwnerID)) != 0 {
		t.Fatal("commit wrote to another owner")
	}
	saved := store.owned(t, testOwnerID)
	if len(saved) != 1 {
		t.Fatalf("expected one item, got %d", len(saved))
	}
	item := saved[0]
	if item.Title != "=Formula" || item.CreatedAt.Format("2006-01-02") != "2024-01-01" || item.UpdatedAt.Format("2006-01-02") != "2024-01-02" {
		t.Fatalf("expected reviewed title and CSV timestamps, got %+v", item)
	}
}

func TestCommitRechecksDuplicatesAgainstLibraryAndSavedRows(t *testing.T) {
	store := newLibraryStore(t, items.CreateItemInput{OwnerID: otherOwnerID, Title: "Shared Title", ItemType: items.ItemTypeBook})
	importer := NewCSVImporter(store, nil)
	preview, err := importer.Preview(context.Background(), strings.NewReader(reviewHeader+
		"Shared Title,,book,,,,,,,,,\n"+
		"Added Since Preview,,book,,,,,,,,,\n"+
		"Unique,,book,,,9780000000001,,,,,,\n"), testOwnerID)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	request := reviewedRows(t, preview, nil)
	if len(request.Rows) != 3 {
		t.Fatalf("expected three ready rows, got %+v", preview.Rows)
	}
	// Another tab saves a matching item after the preview, and a tampered
	// request repeats an ISBN under a new row number.
	if _, err := store.Service.Create(context.Background(), items.CreateItemInput{OwnerID: testOwnerID, Title: "added since preview", ItemType: items.ItemTypeBook}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	repeat := request.Rows[2]
	repeat.Row = 9
	repeat.Item.Title = "Unique Again"
	request.Rows = append(request.Rows, repeat)

	result, err := importer.Commit(context.Background(), request, testOwnerID)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if result.Added != 2 || result.Skipped != 2 {
		t.Fatalf("unexpected counts: %+v", result)
	}
	if outcome := result.Rows[1]; outcome.Status != OutcomeSkipped || len(outcome.LibraryMatches) != 1 || outcome.LibraryMatches[0].Title != "added since preview" {
		t.Fatalf("expected library duplicate, got %+v", outcome)
	}
	if outcome := result.Rows[3]; outcome.Status != OutcomeSkipped || outcome.FileMatch == nil || outcome.FileMatch.Row != 4 || outcome.FileMatch.Field != "isbn13" {
		t.Fatalf("expected in-import duplicate of row 4, got %+v", outcome)
	}
	if result.Rows[0].Status != OutcomeAdded {
		t.Fatalf("another owner's item must not block the import, got %+v", result.Rows[0])
	}
}

func TestCommitRepeatSkipsRowsAlreadySaved(t *testing.T) {
	store := newLibraryStore(t)
	importer := NewCSVImporter(store, nil)
	preview, err := importer.Preview(context.Background(), strings.NewReader(reviewHeader+"One,,book,,,,,,,,,\nTwo,,movie,,,,,,,,,\n"), testOwnerID)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	request := reviewedRows(t, preview, nil)
	if _, err := importer.Commit(context.Background(), request, testOwnerID); err != nil {
		t.Fatalf("first commit: %v", err)
	}
	result, err := importer.Commit(context.Background(), request, testOwnerID)
	if err != nil {
		t.Fatalf("repeat commit: %v", err)
	}
	if result.Added != 0 || result.Skipped != 2 || len(store.owned(t, testOwnerID)) != 2 {
		t.Fatalf("repeat must skip saved rows, got %+v", result)
	}
}

func TestCommitReportsPartialSuccessDistinctly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := newLibraryStore(t)
	store.repo.fault = func(ctx context.Context, call int32, insert func() (items.Item, error)) (items.Item, error) {
		switch call {
		case 2:
			// The repository confirms nothing was stored.
			return items.Item{}, errors.New("insert item: pq: value too long for type at 10.0.0.5")
		case 4:
			cancel()
			return items.Item{}, fmt.Errorf("insert item: %w: %w", items.ErrSaveOutcomeUnknown, context.Canceled)
		}
		return insert()
	}
	importer := NewCSVImporter(store, nil)

	request := CommitRequest{}
	for idx, title := range []string{"One", "Two", "", "Four", "Five", "Six"} {
		request.Rows = append(request.Rows, ReviewedRow{Row: idx + 2, Item: ReviewedItem{CreateItemInput: items.CreateItemInput{Title: title, ItemType: items.ItemTypeBook}}})
	}
	result, err := importer.Commit(ctx, request, testOwnerID)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	statuses := make([]OutcomeStatus, 0, len(result.Rows))
	for _, row := range result.Rows {
		statuses = append(statuses, row.Status)
	}
	want := []OutcomeStatus{OutcomeAdded, OutcomeFailed, OutcomeFailed, OutcomeAdded, OutcomeInterrupted, OutcomeUnprocessed}
	if !slices.Equal(statuses, want) {
		t.Fatalf("statuses %v, want %v", statuses, want)
	}
	if result.Added != 2 || result.Failed != 2 || result.Interrupted != 1 || result.Unprocessed != 1 || result.Skipped != 0 {
		t.Fatalf("unexpected counts: %+v", result)
	}
	if message := result.Rows[1].Message; message != msgCommitSaveFailed {
		t.Fatalf("store errors must not leak, got %q", message)
	}
	if message := result.Rows[2].Message; message != "title is required" {
		t.Fatalf("expected Create validation message, got %q", message)
	}
	if result.Rows[4].Message != msgCommitInterrupted || result.Rows[5].Message != msgCommitUnprocessed {
		t.Fatalf("unexpected interruption messages: %+v", result.Rows[4:])
	}
	if got := store.creates.Load(); got != 4 {
		t.Fatalf("expected no create after the context ended, got %d calls", got)
	}
}

func TestCommitWithCanceledContextSavesNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := newLibraryStore(t)
	request := CommitRequest{Rows: []ReviewedRow{{Row: 2, Item: ReviewedItem{CreateItemInput: items.CreateItemInput{Title: "One", ItemType: items.ItemTypeBook}}}}}
	result, err := NewCSVImporter(store, nil).Commit(ctx, request, testOwnerID)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if store.creates.Load() != 0 || result.Unprocessed != 1 {
		t.Fatalf("expected nothing saved, got %d creates, %+v", store.creates.Load(), result)
	}
}

func TestCommitRejectsInvalidRequestsBeforeWriting(t *testing.T) {
	item := ReviewedItem{CreateItemInput: items.CreateItemInput{Title: "One", ItemType: items.ItemTypeBook}}
	tooMany := make([]ReviewedRow, MaxImportRows+1)
	for idx := range tooMany {
		tooMany[idx] = ReviewedRow{Row: idx + 2, Item: item}
	}
	cases := map[string][]ReviewedRow{
		"no rows":       nil,
		"too many rows": tooMany,
		"header row":    {{Row: 1, Item: item}},
		"repeated row":  {{Row: 3, Item: item}, {Row: 2, Item: item}, {Row: 3, Item: item}},
		"negative row":  {{Row: -5, Item: item}},
	}
	for name, rows := range cases {
		t.Run(name, func(t *testing.T) {
			store := newLibraryStore(t)
			_, err := NewCSVImporter(store, nil).Commit(context.Background(), CommitRequest{Rows: rows}, testOwnerID)
			if !errors.Is(err, ErrInvalidCommit) {
				t.Fatalf("expected invalid commit error, got %v", err)
			}
			if store.creates.Load() != 0 {
				t.Fatalf("expected no writes, got %d", store.creates.Load())
			}
		})
	}
}

func TestCommitProcessesRowsInRowOrder(t *testing.T) {
	store := newLibraryStore(t)
	request := CommitRequest{Rows: []ReviewedRow{
		{Row: 7, Item: ReviewedItem{CreateItemInput: items.CreateItemInput{Title: "Same", ItemType: items.ItemTypeBook}}},
		{Row: 3, Item: ReviewedItem{CreateItemInput: items.CreateItemInput{Title: "same", ItemType: items.ItemTypeMovie}}},
	}}
	result, err := NewCSVImporter(store, nil).Commit(context.Background(), request, testOwnerID)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if result.Rows[0].Row != 3 || result.Rows[0].Status != OutcomeAdded || result.Rows[1].FileMatch == nil || result.Rows[1].FileMatch.Row != 3 {
		t.Fatalf("expected the earlier row to win, got %+v", result.Rows)
	}
}

// TestReviewedImportMatchesLegacyImport checks that previewing and committing
// a file saves the same items the single-request import saves.
func TestReviewedImportMatchesLegacyImport(t *testing.T) {
	csv := reviewHeader +
		"Book,Author,book,2020,300,978-0-00-000000-1,,Desc,https://example.com/c.jpg,Note,FICTION,want_to_read\n" +
		"Game,Studio,game,2019,,,,,,,,\n" +
		"Movie,,movie,,,,,,,,,\n" +
		"book,,book,,,,,,,,,\n"

	legacy := newLibraryStore(t)
	if _, err := NewCSVImporter(legacy, nil).Import(context.Background(), strings.NewReader(csv), testOwnerID); err != nil {
		t.Fatalf("import: %v", err)
	}
	reviewed := newLibraryStore(t)
	importer := NewCSVImporter(reviewed, nil)
	preview, err := importer.Preview(context.Background(), strings.NewReader(csv), testOwnerID)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if _, err := importer.Commit(context.Background(), reviewedRows(t, preview, nil), testOwnerID); err != nil {
		t.Fatalf("commit: %v", err)
	}

	strip := func(list []items.Item) map[string]items.Item {
		out := map[string]items.Item{}
		for _, item := range list {
			item.ID = uuid.Nil
			out[item.Title] = item
		}
		return out
	}
	want, got := strip(legacy.owned(t, testOwnerID)), strip(reviewed.owned(t, testOwnerID))
	if len(want) != 3 || len(got) != 3 {
		t.Fatalf("expected 3 items each (the lowercase repeat is a duplicate), got %d and %d", len(want), len(got))
	}
	for title, wantItem := range want {
		// Without CSV timestamps both imports stamp the current time.
		gotItem := got[title]
		wantItem.CreatedAt, wantItem.UpdatedAt = gotItem.CreatedAt, gotItem.UpdatedAt
		if !reflect.DeepEqual(wantItem, gotItem) {
			t.Fatalf("%q differs:\nlegacy   %+v\nreviewed %+v", title, wantItem, gotItem)
		}
	}
}

// TestPreviewAndCommitAreSafeConcurrently runs previews and commits for two
// owners at once; run with -race.
func TestPreviewAndCommitAreSafeConcurrently(t *testing.T) {
	store := newLibraryStore(t)
	importer := NewCSVImporter(store, twoEditions())
	var wg sync.WaitGroup
	for worker := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			owner := testOwnerID
			if worker%2 == 1 {
				owner = otherOwnerID
			}
			csv := reviewHeader + fmt.Sprintf("Worker %d,,book,,,,,,,,,\n,,book,,,9780000000100,,,,,,\n", worker)
			preview, err := importer.Preview(context.Background(), strings.NewReader(csv), owner)
			if err != nil {
				t.Errorf("preview: %v", err)
				return
			}
			request := CommitRequest{}
			for _, row := range preview.Rows {
				if row.Item != nil {
					request.Rows = append(request.Rows, ReviewedRow{Row: row.Row, Item: *row.Item})
				}
			}
			if _, err := importer.Commit(context.Background(), request, owner); err != nil {
				t.Errorf("commit: %v", err)
			}
		}()
	}
	wg.Wait()
	if got := len(store.owned(t, testOwnerID)) + len(store.owned(t, otherOwnerID)); got != 8 {
		t.Fatalf("expected one item per worker, got %d", got)
	}
}

func TestRowNumbersMatchSpreadsheetRows(t *testing.T) {
	csv := reviewHeader +
		"\"Multi\r\nLine\",,book,,,,,,\"two\nlines\",,,\n" + // row 2 spans three file lines
		"\n" + // row 3 is a blank line
		",,,,,,,,,,,\n" + // row 4 has only empty cells
		"Last,,book,,,,,,,,,\n" // row 5
	preview, err := NewCSVImporter(newLibraryStore(t), nil).Preview(context.Background(), strings.NewReader(csv), testOwnerID)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	numbers := make([]int, 0, len(preview.Rows))
	for _, row := range preview.Rows {
		numbers = append(numbers, row.Row)
	}
	if !slices.Equal(numbers, []int{2, 5}) {
		t.Fatalf("row numbers %v, want [2 5]", numbers)
	}
}
