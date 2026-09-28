package http

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"log/slog"

	"github.com/google/uuid"

	"anthology/internal/auth"
	"anthology/internal/catalog"
	"anthology/internal/importer"
	"anthology/internal/items"
)

// testOwnerID is a fixed UUID for tests
var testOwnerID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

func TestDecodeJSONBody_AllowsPayloadWithinLimit(t *testing.T) {
	body := strings.NewReader(`{"name":"anthology"}`)
	req := httptest.NewRequest("POST", "/api/items", body)
	rec := httptest.NewRecorder()

	var dst map[string]string
	if err := decodeJSONBody(rec, req, &dst); err != nil {
		t.Fatalf("decodeJSONBody returned error: %v", err)
	}
	if dst["name"] != "anthology" {
		t.Fatalf("expected key to be decoded, got %v", dst)
	}
}

func TestDecodeJSONBody_RejectsPayloadExceedingLimit(t *testing.T) {
	var b strings.Builder
	b.Grow(int(maxJSONBodyBytes) + 32)
	b.WriteString(`{"data":"`)
	for i := int64(0); i < maxJSONBodyBytes; i++ {
		b.WriteByte('a')
	}
	b.WriteString(`"}`)

	req := httptest.NewRequest("POST", "/api/items", strings.NewReader(b.String()))
	rec := httptest.NewRecorder()

	var dst map[string]string
	err := decodeJSONBody(rec, req, &dst)
	if err == nil {
		t.Fatal("expected error for oversized payload")
	}
	if !strings.Contains(err.Error(), "payload too large") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestItemHandlerImportCSVSuccess(t *testing.T) {
	store := &csvStoreStub{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	importerSvc := importer.NewCSVImporter(store, nil)
	handler := NewItemHandler(nil, nil, importerSvc, logger)
	req := newMultipartCSVRequest(t, strings.Join([]string{
		"title,creator,itemType,releaseYear,pageCount,isbn13,isbn10,description,coverImage,notes",
		"Title A,Creator,book,2020,300,9780000000001,0000000001,Desc,,Notes",
		"Title B,Director,movie,,,,,,,",
	}, "\n"))
	req = reqWithUser(req)
	rec := httptest.NewRecorder()

	handler.ImportCSV(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var summary importer.Summary
	if err := json.Unmarshal(rec.Body.Bytes(), &summary); err != nil {
		t.Fatalf("response should decode: %v", err)
	}

	if summary.Imported != 2 || summary.TotalRows != 2 {
		t.Fatalf("expected summary to be returned, got %+v", summary)
	}

	if len(store.items) != 2 {
		t.Fatalf("expected importer to persist rows, got %d", len(store.items))
	}
}

func TestItemHandlerImportCSVValidationError(t *testing.T) {
	store := &csvStoreStub{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	importerSvc := importer.NewCSVImporter(store, nil)
	handler := NewItemHandler(nil, nil, importerSvc, logger)
	req := newMultipartCSVRequest(t, "title,itemType\nbad,csv\n")
	req = reqWithUser(req)
	rec := httptest.NewRecorder()

	handler.ImportCSV(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestItemHandlerImportCSVUnavailable(t *testing.T) {
	handler := NewItemHandler(nil, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := newMultipartCSVRequest(t, "title\nA\n")
	req = reqWithUser(req)
	rec := httptest.NewRecorder()

	handler.ImportCSV(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected status 501, got %d", rec.Code)
	}
}

func TestItemHandlerExportCSV(t *testing.T) {
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	itemOld := items.Item{
		ID:        uuid.New(),
		Title:     "Old Title",
		Creator:   "Old Creator",
		ItemType:  items.ItemTypeBook,
		OwnerID:   testOwnerID,
		CreatedAt: now.Add(-24 * time.Hour),
		UpdatedAt: now.Add(-24 * time.Hour),
	}
	itemNew := items.Item{
		ID:        uuid.New(),
		Title:     "New Title",
		Creator:   "New Creator",
		ItemType:  items.ItemTypeBook,
		OwnerID:   testOwnerID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	repo := &exportRepoStub{
		items: []items.Item{itemOld, itemNew},
	}
	service := items.NewService(repo)
	handler := NewItemHandler(service, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/items/export?type=book&status=reading&shelf_status=on&limit=25",
		nil,
	)
	req = reqWithUser(req)
	rec := httptest.NewRecorder()

	handler.ExportCSV(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if repo.lastOpts.ItemType == nil || *repo.lastOpts.ItemType != items.ItemTypeBook {
		t.Fatalf("expected item type filter to be book, got %+v", repo.lastOpts.ItemType)
	}
	if repo.lastOpts.ReadingStatus == nil || *repo.lastOpts.ReadingStatus != items.BookStatusReading {
		t.Fatalf("expected reading status filter to be reading, got %+v", repo.lastOpts.ReadingStatus)
	}
	if repo.lastOpts.ShelfStatus == nil || *repo.lastOpts.ShelfStatus != items.ShelfStatusOn {
		t.Fatalf("expected shelf status filter to be on, got %+v", repo.lastOpts.ShelfStatus)
	}
	if repo.lastOpts.Limit != nil {
		t.Fatalf("expected export to remove limit filter, got %+v", *repo.lastOpts.Limit)
	}

	if contentType := rec.Header().Get("Content-Type"); contentType != "text/csv; charset=utf-8" {
		t.Fatalf("expected content type to be csv, got %q", contentType)
	}

	contentDisposition := rec.Header().Get("Content-Disposition")
	if !strings.HasPrefix(contentDisposition, "attachment; filename=\"anthology-export-") ||
		!strings.HasSuffix(contentDisposition, ".csv\"") {
		t.Fatalf("unexpected content disposition: %q", contentDisposition)
	}

	reader := csv.NewReader(strings.NewReader(rec.Body.String()))
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to read csv: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	if rows[0][0] != "schemaVersion" {
		t.Fatalf("expected header row, got %v", rows[0])
	}
	if rows[1][1] != "New Title" {
		t.Fatalf("expected newest item first, got %v", rows[1])
	}
}

func newMultipartCSVRequest(t *testing.T, csv string) *http.Request {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "import.csv")
	if err != nil {
		t.Fatalf("failed to create multipart form: %v", err)
	}
	if _, err := io.Copy(part, strings.NewReader(csv)); err != nil {
		t.Fatalf("failed to write csv: %v", err)
	}
	_ = writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/items/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

// reqWithUser adds test user context to a request
func reqWithUser(req *http.Request) *http.Request {
	user := &auth.User{
		ID:    testOwnerID,
		Email: "test@example.com",
	}
	ctx := context.WithValue(req.Context(), userContextKey, user)
	return req.WithContext(ctx)
}

type csvStoreStub struct {
	items []items.Item
}

func (s *csvStoreStub) Create(ctx context.Context, input items.CreateItemInput) (items.Item, error) {
	item := items.Item{ID: uuid.New(), Title: input.Title, Creator: input.Creator, ItemType: input.ItemType}
	s.items = append(s.items, item)
	return item, nil
}

func (s *csvStoreStub) List(ctx context.Context, opts items.ListOptions) ([]items.Item, error) {
	itemsCopy := make([]items.Item, len(s.items))
	copy(itemsCopy, s.items)
	return itemsCopy, nil
}

type exportRepoStub struct {
	items    []items.Item
	lastOpts items.ListOptions
}

func (s *exportRepoStub) Create(ctx context.Context, item items.Item) (items.Item, error) {
	return item, nil
}

func (s *exportRepoStub) Get(ctx context.Context, id uuid.UUID, ownerID uuid.UUID) (items.Item, error) {
	return items.Item{}, items.ErrNotFound
}

func (s *exportRepoStub) List(ctx context.Context, opts items.ListOptions) ([]items.Item, error) {
	s.lastOpts = opts
	itemsCopy := make([]items.Item, len(s.items))
	copy(itemsCopy, s.items)
	return itemsCopy, nil
}

func (s *exportRepoStub) Update(ctx context.Context, item items.Item) (items.Item, error) {
	return item, nil
}

func (s *exportRepoStub) Delete(ctx context.Context, id uuid.UUID, ownerID uuid.UUID) error {
	return nil
}

func (s *exportRepoStub) Histogram(ctx context.Context, opts items.HistogramOptions) (items.LetterHistogram, error) {
	return items.LetterHistogram{}, nil
}

func (s *exportRepoStub) FindDuplicates(ctx context.Context, input items.DuplicateCheckInput, ownerID uuid.UUID) ([]items.DuplicateMatch, error) {
	return nil, nil
}

func (s *exportRepoStub) ListSeries(ctx context.Context, opts items.SeriesRepoListOptions, ownerID uuid.UUID) ([]items.SeriesSummary, error) {
	return nil, nil
}

func (s *exportRepoStub) GetSeriesByName(ctx context.Context, name string, ownerID uuid.UUID) (items.SeriesSummary, error) {
	return items.SeriesSummary{}, items.ErrNotFound
}

func (s *exportRepoStub) ListSeriesNamesByNameCI(ctx context.Context, name string, ownerID uuid.UUID) ([]string, error) {
	return nil, nil
}

func (s *exportRepoStub) UpdateSeriesName(ctx context.Context, oldName, newName string, ownerID uuid.UUID) (int64, error) {
	return 0, nil
}

func (s *exportRepoStub) ClearSeriesName(ctx context.Context, seriesName string, ownerID uuid.UUID) (int64, error) {
	return 0, nil
}

type slowCatalogStub struct {
	delay time.Duration
}

func (s slowCatalogStub) Lookup(ctx context.Context, query string, category catalog.Category) ([]catalog.Metadata, error) {
	select {
	case <-time.After(s.delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return []catalog.Metadata{{Title: "Looked Up " + query, ItemType: string(items.ItemTypeBook)}}, nil
}

func TestItemHandlerImportCSVOutlivesServerWriteTimeout(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	importerSvc := importer.NewCSVImporter(&csvStoreStub{}, slowCatalogStub{delay: 300 * time.Millisecond})
	handler := NewItemHandler(nil, nil, importerSvc, logger)

	srv := httptest.NewUnstartedServer(newSlogMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ImportCSV(w, reqWithUser(r))
	})))
	srv.Config.WriteTimeout = 100 * time.Millisecond
	srv.Start()
	defer srv.Close()

	req := newMultipartCSVRequest(t, strings.Join([]string{
		"title,creator,itemType,releaseYear,pageCount,isbn13,isbn10,description,coverImage,notes",
		",,book,,,9780000000001,,,,",
	}, "\n"))
	httpReq, err := http.NewRequest(http.MethodPost, srv.URL+"/api/items/import", req.Body)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	httpReq.Header.Set("Content-Type", req.Header.Get("Content-Type"))

	resp, err := srv.Client().Do(httpReq)
	if err != nil {
		t.Fatalf("import response lost after server WriteTimeout: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	var summary importer.Summary
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if summary.Imported != 1 {
		t.Fatalf("expected 1 import, got %+v", summary)
	}
}
