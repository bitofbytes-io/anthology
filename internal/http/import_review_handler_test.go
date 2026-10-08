package http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"anthology/internal/auth"
	"anthology/internal/catalog"
	"anthology/internal/config"
	"anthology/internal/importer"
	"anthology/internal/items"
)

func newReviewHandler(t *testing.T, lookups importer.CatalogLookup) (*ItemHandler, *items.Service) {
	t.Helper()
	service := items.NewService(items.NewInMemoryRepository(nil))
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewItemHandler(service, nil, importer.NewCSVImporter(service, lookups), logger), service
}

func ownedItems(t *testing.T, service *items.Service) []items.Item {
	t.Helper()
	list, err := service.List(context.Background(), items.ListOptions{OwnerID: testOwnerID})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	return list
}

func TestItemHandlerPreviewCSVImportWritesNothing(t *testing.T) {
	handler, service := newReviewHandler(t, nil)
	req := reqWithUser(newMultipartCSVRequest(t, strings.Join([]string{
		"title,creator,itemType,releaseYear,pageCount,isbn13,isbn10,description,coverImage,notes",
		"Title A,Creator,book,2020,300,9780000000001,,Desc,,Notes",
		"title a,,movie,,,,,,,",
		"Bad,,book,year,,,,,,",
	}, "\n")))
	rec := httptest.NewRecorder()

	handler.PreviewCSVImport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var preview importer.Preview
	if err := json.Unmarshal(rec.Body.Bytes(), &preview); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if preview.TotalRows != 3 || preview.Ready != 1 || preview.Duplicates != 1 || preview.NeedsMatch != 1 {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	if got := len(ownedItems(t, service)); got != 0 {
		t.Fatalf("preview must not save items, found %d", got)
	}
}

func TestItemHandlerPreviewCSVImportReportsNonFinitePricesPerRow(t *testing.T) {
	handler, service := newReviewHandler(t, nil)
	csv := strings.Join([]string{
		"title,creator,itemType,releaseYear,pageCount,isbn13,isbn10,description,coverImage,notes,retailPriceUsd",
		"Valid,,book,,,,,,,,12.34",
		"Not a number,,book,,,,,,,,NaN",
		"Plus infinity,,book,,,,,,,,+Inf",
		"Minus infinity,,book,,,,,,,,-Inf",
		"Spelled out,,book,,,,,,,,Infinity",
	}, "\n")
	rec := httptest.NewRecorder()

	handler.PreviewCSVImport(rec, reqWithUser(newMultipartCSVRequest(t, csv)))

	if rec.Code != http.StatusOK || !json.Valid(rec.Body.Bytes()) {
		t.Fatalf("expected a valid JSON preview, got %d %q", rec.Code, rec.Body.String())
	}
	var preview importer.Preview
	if err := json.Unmarshal(rec.Body.Bytes(), &preview); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if preview.Ready != 1 || preview.NeedsMatch != 4 || preview.Rows[0].Status != importer.RowReady {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	if price := preview.Rows[0].Item.RetailPriceUsd; price == nil || *price != 12.34 {
		t.Fatalf("valid row lost its price: %+v", preview.Rows[0].Item)
	}
	for _, row := range preview.Rows[1:] {
		if row.Status != importer.RowNeedsMatch || row.Problem != "retailPriceUsd must be a finite number" || row.Item != nil {
			t.Fatalf("row %d: got %s %q", row.Row, row.Status, row.Problem)
		}
	}
	if got := len(ownedItems(t, service)); got != 0 {
		t.Fatalf("preview saved %d items", got)
	}
}

func TestItemHandlerPreviewCSVImportRejectsInvalidFiles(t *testing.T) {
	handler, _ := newReviewHandler(t, nil)
	rec := httptest.NewRecorder()
	handler.PreviewCSVImport(rec, reqWithUser(newMultipartCSVRequest(t, "title,itemType\nbad,csv\n")))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}

	big := strings.Repeat("x", int(maxCSVUploadBytes)+1)
	rec = httptest.NewRecorder()
	handler.PreviewCSVImport(rec, reqWithUser(newMultipartCSVRequest(t, big)))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected status 413, got %d", rec.Code)
	}
}

func commitRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, csvImportCommitPath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return reqWithUser(req)
}

func TestItemHandlerCommitCSVImportSavesReviewedItems(t *testing.T) {
	handler, service := newReviewHandler(t, nil)
	body := `{"rows":[
		{"row":3,"item":{"title":"Reviewed","creator":"Author","itemType":"book","googleVolumeId":"vol-1","createdAt":"2024-01-01T00:00:00Z"}},
		{"row":2,"item":{"title":"Movie","itemType":"movie"}}
	]}`
	rec := httptest.NewRecorder()

	handler.CommitCSVImport(rec, commitRequest(body))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var result importer.CommitResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.Added != 2 || result.Rows[0].Row != 2 || result.Rows[1].Row != 3 {
		t.Fatalf("unexpected result: %+v", result)
	}
	saved := ownedItems(t, service)
	if len(saved) != 2 {
		t.Fatalf("expected items saved for the signed-in user, got %d", len(saved))
	}
	for _, item := range saved {
		if item.Title == "Reviewed" && (item.GoogleVolumeId != "vol-1" || item.CreatedAt.Year() != 2024) {
			t.Fatalf("reviewed fields not kept: %+v", item)
		}
	}
}

func TestItemHandlerCommitCSVImportRejectsBadRequestsBeforeSaving(t *testing.T) {
	cases := map[string]struct {
		body string
		code int
	}{
		"owner in body":  {`{"rows":[{"row":2,"item":{"title":"A","itemType":"book","ownerId":"00000000-0000-0000-0000-000000000002"}}]}`, http.StatusBadRequest},
		"unknown field":  {`{"rows":[],"lookup":true}`, http.StatusBadRequest},
		"no rows":        {`{"rows":[]}`, http.StatusBadRequest},
		"repeated rows":  {`{"rows":[{"row":2,"item":{"title":"A","itemType":"book"}},{"row":2,"item":{"title":"B","itemType":"book"}}]}`, http.StatusBadRequest},
		"malformed json": {`{"rows":`, http.StatusBadRequest},
		"too large":      {`{"rows":[{"row":2,"item":{"title":"` + strings.Repeat("a", int(maxImportCommitBytes)) + `","itemType":"book"}}]}`, http.StatusRequestEntityTooLarge},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			handler, service := newReviewHandler(t, nil)
			rec := httptest.NewRecorder()
			handler.CommitCSVImport(rec, commitRequest(tc.body))
			if rec.Code != tc.code {
				t.Fatalf("expected status %d, got %d: %s", tc.code, rec.Code, rec.Body.String())
			}
			if got := len(ownedItems(t, service)); got != 0 {
				t.Fatalf("expected nothing saved, found %d", got)
			}
		})
	}
}

func TestItemHandlerCommitCSVImportUnavailable(t *testing.T) {
	handler := NewItemHandler(nil, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	rec := httptest.NewRecorder()
	handler.CommitCSVImport(rec, commitRequest(`{"rows":[]}`))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected status 501, got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	handler.PreviewCSVImport(rec, reqWithUser(newMultipartCSVRequest(t, "title\nA\n")))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected status 501, got %d", rec.Code)
	}
}

func TestItemHandlerCommitCSVImportRefusesUnsafeStore(t *testing.T) {
	// The plain Repository interface hides the in-memory exclusive inserts.
	service := items.NewService(struct{ items.Repository }{items.NewInMemoryRepository(nil)})
	handler := NewItemHandler(service, nil, importer.NewCSVImporter(service, nil), slog.New(slog.NewTextHandler(io.Discard, nil)))
	rec := httptest.NewRecorder()
	handler.CommitCSVImport(rec, commitRequest(`{"rows":[{"row":2,"item":{"title":"A","itemType":"book"}}]}`))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected status 501, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := len(ownedItems(t, service)); got != 0 {
		t.Fatalf("refused commit saved %d items", got)
	}
}

func TestItemHandlerPreviewCSVImportOutlivesServerWriteTimeout(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewItemHandler(nil, nil, importer.NewCSVImporter(&csvStoreStub{}, slowCatalogStub{delay: 300 * time.Millisecond}), logger)
	srv := httptest.NewUnstartedServer(newSlogMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.PreviewCSVImport(w, reqWithUser(r))
	})))
	srv.Config.WriteTimeout = 100 * time.Millisecond
	srv.Start()
	defer srv.Close()

	req := newMultipartCSVRequest(t, strings.Join([]string{
		"title,creator,itemType,releaseYear,pageCount,isbn13,isbn10,description,coverImage,notes",
		",,book,,,9780000000001,,,,",
	}, "\n"))
	httpReq, err := http.NewRequest(http.MethodPost, srv.URL+csvImportPreviewPath, req.Body)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	httpReq.Header.Set("Content-Type", req.Header.Get("Content-Type"))

	resp, err := srv.Client().Do(httpReq)
	if err != nil {
		t.Fatalf("preview response lost after server WriteTimeout: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var preview importer.Preview
	if err := json.NewDecoder(resp.Body).Decode(&preview); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if len(preview.Rows) != 1 || len(preview.Rows[0].Candidates) != 1 {
		t.Fatalf("expected one candidate, got %+v", preview)
	}
}

// TestImportReviewRoundTripThroughRouter previews and commits through the real
// router, as the browser does, against a fake Google Books upstream.
func TestImportReviewRoundTripThroughRouter(t *testing.T) {
	const origin = "http://localhost:4200"
	var lookups atomic.Int32
	books := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lookups.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"items":[{"id":"vol-1","volumeInfo":{"title":"Looked Up Book","authors":["Catalog Author"],"industryIdentifiers":[{"type":"ISBN_13","identifier":"9780000000100"}]}}]}`)
	}))
	defer books.Close()

	user := &auth.User{ID: testOwnerID, Email: "reader@example.test"}
	authService := auth.NewService(&authRepoStub{
		findSessionByHash: func(ctx context.Context, tokenHash string) (*auth.Session, *auth.User, error) {
			return &auth.Session{ID: uuid.New(), ExpiresAt: time.Now().Add(time.Hour)}, user, nil
		},
	}, time.Hour, nil)
	service := items.NewService(items.NewInMemoryRepository(nil))
	catalogService := catalog.NewService(books.Client(), catalog.WithGoogleBooksBaseURL(books.URL))
	router := NewRouter(
		config.Config{Environment: "development", AllowedOrigins: []string{origin}},
		service, catalogService, nil, authService, nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	send := func(req *http.Request) *httptest.ResponseRecorder {
		req.Header.Set("Origin", origin)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "token"})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	csv := strings.Join([]string{
		"title,creator,itemType,releaseYear,pageCount,isbn13,isbn10,description,coverImage,notes",
		"Ready Book,Author,book,2020,,,,,,",
		",,book,,,9780000000100,,,,CSV note",
		"ready book,,movie,,,,,,,",
	}, "\n")
	preview := func() importer.Preview {
		t.Helper()
		upload := newMultipartCSVRequest(t, csv)
		req := httptest.NewRequest(http.MethodPost, csvImportPreviewPath, upload.Body)
		req.Header.Set("Content-Type", upload.Header.Get("Content-Type"))
		rec := send(req)
		if rec.Code != http.StatusOK {
			t.Fatalf("preview: status %d: %s", rec.Code, rec.Body.String())
		}
		var result importer.Preview
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatalf("decode preview: %v", err)
		}
		return result
	}

	first := preview()
	if first.Ready != 1 || first.Duplicates != 1 || first.NeedsMatch != 1 || len(first.Rows[1].Candidates) != 1 {
		t.Fatalf("unexpected first preview: %+v", first)
	}
	body, err := json.Marshal(importer.CommitRequest{Rows: []importer.ReviewedRow{
		{Row: first.Rows[0].Row, Item: *first.Rows[0].Item},
		{Row: first.Rows[1].Row, Item: first.Rows[1].Candidates[0].Item},
	}})
	if err != nil {
		t.Fatalf("encode commit: %v", err)
	}
	lookupsBeforeCommit := lookups.Load()
	commit := httptest.NewRequest(http.MethodPost, csvImportCommitPath, bytes.NewReader(body))
	commit.Header.Set("Content-Type", "application/json")
	rec := send(commit)
	if rec.Code != http.StatusOK {
		t.Fatalf("commit: status %d: %s", rec.Code, rec.Body.String())
	}
	var result importer.CommitResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode commit: %v", err)
	}
	if result.Added != 2 || lookups.Load() != lookupsBeforeCommit {
		t.Fatalf("expected 2 added without lookups, got %+v and %d lookups", result, lookups.Load()-lookupsBeforeCommit)
	}
	saved := ownedItems(t, service)
	if len(saved) != 2 {
		t.Fatalf("expected 2 saved items, got %d", len(saved))
	}

	second := preview()
	if second.Ready != 0 || second.Duplicates != 2 || len(second.Rows[1].Candidates[0].LibraryMatches) == 0 {
		t.Fatalf("a new preview must show saved rows as duplicates, got %+v", second)
	}
}

func TestImportReviewRoutesRequireAuthAndSameOrigin(t *testing.T) {
	const origin = "http://localhost:4200"
	router := NewRouter(
		config.Config{Environment: "development", AllowedOrigins: []string{origin}},
		items.NewService(items.NewInMemoryRepository(nil)),
		nil,
		nil,
		nil,
		nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	for _, path := range []string{csvImportPreviewPath, csvImportCommitPath} {
		t.Run(path, func(t *testing.T) {
			crossSite := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
			crossSite.Header.Set("Origin", "https://evil.example")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, crossSite)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("cross-site request: expected 403, got %d", rec.Code)
			}

			anonymous := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
			anonymous.Header.Set("Origin", origin)
			rec = httptest.NewRecorder()
			router.ServeHTTP(rec, anonymous)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("anonymous request: expected 401, got %d", rec.Code)
			}
		})
	}
}
