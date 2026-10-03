package http

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"anthology/internal/items"
	"anthology/internal/shelves"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestShelfUpdateLayoutExplicitEmptyArray(t *testing.T) {
	req := reqWithUser(httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"slots":[]}`)))
	owner := UserFromContext(req.Context()).ID
	repo := shelves.NewInMemoryRepository()
	shelf := shelves.Shelf{ID: uuid.New(), OwnerID: owner, Name: "Shelf"}
	if _, err := repo.CreateShelf(context.Background(), shelf, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	itemRepo := items.NewInMemoryRepository(nil)
	handler := NewShelfHandler(shelves.NewService(repo, itemRepo, nil, items.NewService(itemRepo)), slog.New(slog.NewTextHandler(io.Discard, nil)))
	route := chi.NewRouteContext()
	route.URLParams.Add("id", shelf.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, route))
	recorder := httptest.NewRecorder()
	handler.UpdateLayout(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("empty layout failed: %d %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Shelf map[string]json.RawMessage `json:"shelf"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"rows", "slots", "placements", "unplaced"} {
		if string(response.Shelf[field]) != "[]" {
			t.Fatalf("%s not an array: %s", field, response.Shelf[field])
		}
	}
	for _, body := range []string{`{}`, `{"slots":null}`} {
		bad := reqWithUser(httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body)))
		bad = bad.WithContext(context.WithValue(bad.Context(), chi.RouteCtxKey, route))
		recorder = httptest.NewRecorder()
		handler.UpdateLayout(recorder, bad)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("missing explicit slots array accepted: %s", body)
		}
	}
}

func TestShelfCreateDuplicateNameReturnsConflict(t *testing.T) {
	repo := shelves.NewInMemoryRepository()
	itemRepo := items.NewInMemoryRepository(nil)
	handler := NewShelfHandler(shelves.NewService(repo, itemRepo, nil, items.NewService(itemRepo)), slog.New(slog.NewTextHandler(io.Discard, nil)))
	body := `{"name":"Living room","photoUrl":"https://example.com/shelf.jpg"}`

	first := reqWithUser(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
	recorder := httptest.NewRecorder()
	handler.Create(recorder, first)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("first create failed: %d %s", recorder.Code, recorder.Body.String())
	}

	second := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)).WithContext(first.Context())
	recorder = httptest.NewRecorder()
	handler.Create(recorder, second)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("duplicate name: expected 409, got %d %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "already exists") {
		t.Fatalf("expected duplicate-name message, got %s", recorder.Body.String())
	}
}
