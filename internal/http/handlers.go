package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"anthology/internal/catalog"
	"anthology/internal/exporter"
	"anthology/internal/importer"
	"anthology/internal/items"
	"anthology/internal/metrics"
)

// ItemHandler exposes item CRUD endpoints.
type ItemHandler struct {
	service    *items.Service
	catalogSvc *catalog.Service
	importer   *importer.CSVImporter
	exporter   *exporter.CSVExporter
	logger     *slog.Logger
	metrics    *metrics.Metrics
}

// NewItemHandler creates a handler.
func NewItemHandler(service *items.Service, catalogSvc *catalog.Service, importer *importer.CSVImporter, logger *slog.Logger) *ItemHandler {
	return &ItemHandler{
		service:    service,
		catalogSvc: catalogSvc,
		importer:   importer,
		exporter:   exporter.NewCSVExporter(),
		logger:     logger,
	}
}

// List returns all items.
func (h *ItemHandler) List(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	opts, err := parseListOptions(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	opts.OwnerID = user.ID

	items, err := h.service.List(r.Context(), opts)
	if err != nil {
		h.logger.Error("list items", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list items")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func parseListOptions(values url.Values) (items.ListOptions, error) {
	opts := items.ListOptions{}
	const maxListLimit = 50
	const maxSearchQueryLength = 500

	if rawType := strings.TrimSpace(values.Get("type")); rawType != "" {
		typeValue := items.ItemType(rawType)
		switch typeValue {
		case items.ItemTypeBook, items.ItemTypeGame, items.ItemTypeMovie, items.ItemTypeMusic:
			opts.ItemType = &typeValue
		default:
			return items.ListOptions{}, fmt.Errorf("invalid type filter")
		}
	}

	if rawStatus := strings.TrimSpace(values.Get("status")); rawStatus != "" {
		status := items.BookStatus(rawStatus)
		switch status {
		case items.BookStatusNone, items.BookStatusRead, items.BookStatusReading, items.BookStatusWantToRead:
			opts.ReadingStatus = &status
		default:
			return items.ListOptions{}, fmt.Errorf("invalid status filter")
		}
	}

	if rawShelfStatus := strings.TrimSpace(values.Get("shelf_status")); rawShelfStatus != "" {
		shelfStatus := items.ShelfStatus(rawShelfStatus)
		switch shelfStatus {
		case items.ShelfStatusOn, items.ShelfStatusOff:
			opts.ShelfStatus = &shelfStatus
		case items.ShelfStatusAll:
			// "all" means no filter - leave ShelfStatus nil
		default:
			return items.ListOptions{}, fmt.Errorf("invalid shelf_status filter")
		}
	}

	if rawLetter := strings.TrimSpace(values.Get("letter")); rawLetter != "" {
		letter := strings.ToUpper(rawLetter)
		if letter == "#" || (len(letter) == 1 && letter[0] >= 'A' && letter[0] <= 'Z') {
			opts.Initial = &letter
		} else {
			return items.ListOptions{}, fmt.Errorf("invalid letter filter")
		}
	}

	if rawQuery := strings.TrimSpace(values.Get("query")); rawQuery != "" {
		if len(rawQuery) > maxSearchQueryLength {
			return items.ListOptions{}, fmt.Errorf("query too long (max %d characters)", maxSearchQueryLength)
		}
		query := rawQuery
		opts.Query = &query
	}

	if rawLimit := strings.TrimSpace(values.Get("limit")); rawLimit != "" {
		value, err := strconv.Atoi(rawLimit)
		if err != nil || value <= 0 || value > maxListLimit {
			return items.ListOptions{}, fmt.Errorf("invalid limit filter")
		}
		opts.Limit = &value
	}

	return opts, nil
}

// Create stores a new item.
func (h *ItemHandler) Create(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	var input items.CreateItemInput
	if err := decodeJSONBody(w, r, &input); err != nil {
		writeJSONError(w, err)
		return
	}
	input.OwnerID = user.ID

	item, err := h.service.Create(r.Context(), input)
	if err != nil {
		if errors.Is(err, items.ErrValidation) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.logger.Error("create item", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create item")
		return
	}

	h.logger.Info("item created", "user_id", user.ID, "item_id", item.ID, "item_type", item.ItemType)
	writeJSON(w, http.StatusCreated, item)
}

// Get returns a single item.
func (h *ItemHandler) Get(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	item, err := h.service.Get(r.Context(), id, user.ID)
	if err != nil {
		handleServiceError(w, err, h.logger)
		return
	}

	writeJSON(w, http.StatusOK, item)
}

// Update modifies an item. Keys omitted from the body keep their stored
// values; items.ParseItemPatch describes how null is handled.
func (h *ItemHandler) Update(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var body json.RawMessage
	if err := decodeJSONBody(w, r, &body); err != nil {
		writeJSONError(w, err)
		return
	}
	patch, err := items.ParseItemPatch(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	item, err := h.service.Update(r.Context(), id, user.ID, patch)
	if err != nil {
		handleServiceError(w, err, h.logger)
		return
	}

	h.logger.Info("item updated", "user_id", user.ID, "item_id", item.ID, "item_type", item.ItemType)
	writeJSON(w, http.StatusOK, item)
}

// Delete removes an item.
func (h *ItemHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.Delete(r.Context(), id, user.ID); err != nil {
		handleServiceError(w, err, h.logger)
		return
	}

	h.logger.Info("item deleted", "user_id", user.ID, "item_id", id)
	w.WriteHeader(http.StatusNoContent)
}

// Resync refreshes metadata from Google Books for an existing item.
func (h *ItemHandler) Resync(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.catalogSvc == nil {
		writeError(w, http.StatusNotImplemented, "metadata re-sync is not available")
		return
	}

	item, err := h.service.ResyncMetadata(r.Context(), id, user.ID, h.catalogSvc)
	if err != nil {
		handleServiceError(w, err, h.logger)
		return
	}

	h.logger.Info("item metadata resynced", "user_id", user.ID, "item_id", item.ID, "item_type", item.ItemType)
	writeJSON(w, http.StatusOK, item)
}

// Duplicates checks for potential duplicate items by title or identifier.
func (h *ItemHandler) Duplicates(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	title := strings.TrimSpace(r.URL.Query().Get("title"))
	isbn13 := strings.TrimSpace(r.URL.Query().Get("isbn13"))
	isbn10 := strings.TrimSpace(r.URL.Query().Get("isbn10"))

	if title == "" && isbn13 == "" && isbn10 == "" {
		writeJSON(w, http.StatusOK, map[string]any{"duplicates": []items.DuplicateMatch{}})
		return
	}

	input := items.DuplicateCheckInput{
		Title:  title,
		ISBN13: isbn13,
		ISBN10: isbn10,
	}

	matches, err := h.service.FindDuplicates(r.Context(), input, user.ID)
	if err != nil {
		h.logger.Error("find duplicates", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to check for duplicates")
		return
	}

	if matches == nil {
		matches = []items.DuplicateMatch{}
	}

	writeJSON(w, http.StatusOK, map[string]any{"duplicates": matches})
}

// Histogram returns letter counts for the alphabet rail.
func (h *ItemHandler) Histogram(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	opts, err := parseHistogramOptions(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	opts.OwnerID = user.ID

	histogram, total, err := h.service.Histogram(r.Context(), opts)
	if err != nil {
		h.logger.Error("histogram", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to compute histogram")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"histogram": histogram,
		"total":     total,
	})
}

func parseHistogramOptions(values url.Values) (items.HistogramOptions, error) {
	opts := items.HistogramOptions{}

	if rawType := strings.TrimSpace(values.Get("type")); rawType != "" {
		typeValue := items.ItemType(rawType)
		switch typeValue {
		case items.ItemTypeBook, items.ItemTypeGame, items.ItemTypeMovie, items.ItemTypeMusic:
			opts.ItemType = &typeValue
		default:
			return items.HistogramOptions{}, fmt.Errorf("invalid type filter")
		}
	}

	if rawStatus := strings.TrimSpace(values.Get("status")); rawStatus != "" {
		status := items.BookStatus(rawStatus)
		switch status {
		case items.BookStatusNone, items.BookStatusRead, items.BookStatusReading, items.BookStatusWantToRead:
			opts.ReadingStatus = &status
		default:
			return items.HistogramOptions{}, fmt.Errorf("invalid status filter")
		}
	}

	return opts, nil
}

func parseUUIDParam(w http.ResponseWriter, r *http.Request, key string) (uuid.UUID, bool) {
	value := chi.URLParam(r, key)
	id, err := uuid.Parse(value)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return uuid.Nil, false
	}
	return id, true
}
