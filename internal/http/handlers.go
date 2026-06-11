package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"anthology/internal/catalog"
	"anthology/internal/exporter"
	"anthology/internal/importer"
	"anthology/internal/items"
)

// ItemHandler exposes item CRUD endpoints.
type ItemHandler struct {
	service    *items.Service
	catalogSvc *catalog.Service
	importer   *importer.CSVImporter
	exporter   *exporter.CSVExporter
	logger     *slog.Logger
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

	var payload struct {
		Title          string     `json:"title"`
		Creator        string     `json:"creator"`
		ItemType       string     `json:"itemType"`
		ReleaseYear    *int       `json:"releaseYear"`
		PageCount      *int       `json:"pageCount"`
		CurrentPage    *int       `json:"currentPage"`
		ISBN13         string     `json:"isbn13"`
		ISBN10         string     `json:"isbn10"`
		Description    string     `json:"description"`
		CoverImage     string     `json:"coverImage"`
		Format         string     `json:"format"`
		Genre          string     `json:"genre"`
		Rating         *int       `json:"rating"`
		RetailPriceUsd *float64   `json:"retailPriceUsd"`
		GoogleVolumeId string     `json:"googleVolumeId"`
		Platform       string     `json:"platform"`
		AgeGroup       string     `json:"ageGroup"`
		PlayerCount    string     `json:"playerCount"`
		ReadingStatus  string     `json:"readingStatus"`
		ReadAt         *time.Time `json:"readAt"`
		Notes          string     `json:"notes"`
		SeriesName     string     `json:"seriesName"`
		VolumeNumber   *int       `json:"volumeNumber"`
		TotalVolumes   *int       `json:"totalVolumes"`
	}

	if err := decodeJSONBody(w, r, &payload); err != nil {
		writeJSONError(w, err)
		return
	}

	item, err := h.service.Create(r.Context(), items.CreateItemInput{
		OwnerID:        user.ID,
		Title:          payload.Title,
		Creator:        payload.Creator,
		ItemType:       items.ItemType(payload.ItemType),
		ReleaseYear:    payload.ReleaseYear,
		PageCount:      payload.PageCount,
		CurrentPage:    payload.CurrentPage,
		ISBN13:         payload.ISBN13,
		ISBN10:         payload.ISBN10,
		Description:    payload.Description,
		CoverImage:     payload.CoverImage,
		Format:         items.Format(payload.Format),
		Genre:          items.Genre(payload.Genre),
		Rating:         payload.Rating,
		RetailPriceUsd: payload.RetailPriceUsd,
		GoogleVolumeId: payload.GoogleVolumeId,
		Platform:       payload.Platform,
		AgeGroup:       payload.AgeGroup,
		PlayerCount:    payload.PlayerCount,
		ReadingStatus:  items.BookStatus(payload.ReadingStatus),
		ReadAt:         payload.ReadAt,
		Notes:          payload.Notes,
		SeriesName:     payload.SeriesName,
		VolumeNumber:   payload.VolumeNumber,
		TotalVolumes:   payload.TotalVolumes,
	})
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

// Update modifies an item.
func (h *ItemHandler) Update(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	raw := map[string]json.RawMessage{}
	if err := decodeJSONBody(w, r, &raw); err != nil {
		writeJSONError(w, err)
		return
	}

	var payload struct {
		Title          *string    `json:"title"`
		Creator        *string    `json:"creator"`
		ItemType       *string    `json:"itemType"`
		ReleaseYear    *int       `json:"releaseYear"`
		PageCount      *int       `json:"pageCount"`
		CurrentPage    *int       `json:"currentPage"`
		ISBN13         *string    `json:"isbn13"`
		ISBN10         *string    `json:"isbn10"`
		Description    *string    `json:"description"`
		CoverImage     *string    `json:"coverImage"`
		Format         *string    `json:"format"`
		Genre          *string    `json:"genre"`
		Rating         *int       `json:"rating"`
		RetailPriceUsd *float64   `json:"retailPriceUsd"`
		GoogleVolumeId *string    `json:"googleVolumeId"`
		Platform       *string    `json:"platform"`
		AgeGroup       *string    `json:"ageGroup"`
		PlayerCount    *string    `json:"playerCount"`
		ReadingStatus  *string    `json:"readingStatus"`
		ReadAt         *time.Time `json:"readAt"`
		Notes          *string    `json:"notes"`
		SeriesName     *string    `json:"seriesName"`
		VolumeNumber   *int       `json:"volumeNumber"`
		TotalVolumes   *int       `json:"totalVolumes"`
	}

	if err := decodeInto(raw, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	input := items.UpdateItemInput{}
	if _, ok := raw["title"]; ok {
		input.Title = payload.Title
	}
	if _, ok := raw["creator"]; ok {
		input.Creator = payload.Creator
	}
	if _, ok := raw["itemType"]; ok {
		if payload.ItemType != nil {
			typeValue := items.ItemType(*payload.ItemType)
			input.ItemType = &typeValue
		} else {
			input.ItemType = new(items.ItemType)
		}
	}
	if _, ok := raw["releaseYear"]; ok {
		value := payload.ReleaseYear
		input.ReleaseYear = &value
	}
	if _, ok := raw["pageCount"]; ok {
		value := payload.PageCount
		input.PageCount = &value
	}
	if _, ok := raw["currentPage"]; ok {
		value := payload.CurrentPage
		input.CurrentPage = &value
	}
	if _, ok := raw["notes"]; ok {
		input.Notes = payload.Notes
	}
	if _, ok := raw["isbn13"]; ok {
		input.ISBN13 = payload.ISBN13
	}
	if _, ok := raw["isbn10"]; ok {
		input.ISBN10 = payload.ISBN10
	}
	if _, ok := raw["description"]; ok {
		input.Description = payload.Description
	}
	if _, ok := raw["coverImage"]; ok {
		input.CoverImage = payload.CoverImage
	}
	if _, ok := raw["format"]; ok {
		if payload.Format != nil {
			formatValue := items.Format(*payload.Format)
			input.Format = &formatValue
		} else {
			input.Format = new(items.Format)
		}
	}
	if _, ok := raw["genre"]; ok {
		if payload.Genre != nil {
			genreValue := items.Genre(*payload.Genre)
			input.Genre = &genreValue
		} else {
			input.Genre = new(items.Genre)
		}
	}
	if _, ok := raw["rating"]; ok {
		value := payload.Rating
		input.Rating = &value
	}
	if _, ok := raw["retailPriceUsd"]; ok {
		value := payload.RetailPriceUsd
		input.RetailPriceUsd = &value
	}
	if _, ok := raw["googleVolumeId"]; ok {
		input.GoogleVolumeId = payload.GoogleVolumeId
	}
	if _, ok := raw["platform"]; ok {
		input.Platform = payload.Platform
	}
	if _, ok := raw["ageGroup"]; ok {
		input.AgeGroup = payload.AgeGroup
	}
	if _, ok := raw["playerCount"]; ok {
		input.PlayerCount = payload.PlayerCount
	}

	if _, ok := raw["readingStatus"]; ok {
		if payload.ReadingStatus != nil {
			statusValue := items.BookStatus(*payload.ReadingStatus)
			input.ReadingStatus = &statusValue
		} else {
			input.ReadingStatus = new(items.BookStatus)
		}
	}

	if _, ok := raw["readAt"]; ok {
		value := payload.ReadAt
		input.ReadAt = &value
	}

	if _, ok := raw["seriesName"]; ok {
		input.SeriesName = payload.SeriesName
	}
	if _, ok := raw["volumeNumber"]; ok {
		value := payload.VolumeNumber
		input.VolumeNumber = &value
	}
	if _, ok := raw["totalVolumes"]; ok {
		value := payload.TotalVolumes
		input.TotalVolumes = &value
	}

	item, err := h.service.Update(r.Context(), id, user.ID, input)
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

const maxCSVUploadBytes int64 = 5 << 20

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

// ImportCSV ingests a CSV file of catalog items.
func (h *ItemHandler) ImportCSV(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	if h.importer == nil {
		writeError(w, http.StatusNotImplemented, "CSV import is not available")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxCSVUploadBytes)
	if err := r.ParseMultipartForm(maxCSVUploadBytes); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("CSV upload is too large (max %d bytes)", maxErr.Limit))
			return
		}
		writeError(w, http.StatusBadRequest, "invalid CSV upload")
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "CSV file is required")
		return
	}
	defer func() { _ = file.Close() }()

	summary, err := h.importer.Import(r.Context(), file, user.ID)
	if err != nil {
		if errors.Is(err, importer.ErrInvalidCSV) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.logger.Error("csv import failed", "error", err)
		writeError(w, http.StatusInternalServerError, "bulk import failed")
		return
	}

	h.logger.Info("csv import completed",
		"user_id", user.ID,
		"total_rows", summary.TotalRows,
		"imported", summary.Imported,
		"skipped_duplicates", len(summary.SkippedDuplicates),
		"failed", len(summary.Failed),
	)
	writeJSON(w, http.StatusOK, summary)
}

// ExportCSV exports all items matching the given filters to CSV format.
func (h *ItemHandler) ExportCSV(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	opts, err := parseListOptions(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	opts.OwnerID = user.ID

	// Remove limit for export - we want all matching items
	opts.Limit = nil

	itemList, err := h.service.List(r.Context(), opts)
	if err != nil {
		h.logger.Error("export items", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to export items")
		return
	}
	h.logger.Info("csv export started", "user_id", user.ID, "item_count", len(itemList))

	// Generate filename with timestamp
	timestamp := time.Now().UTC().Format("2006-01-02")
	filename := fmt.Sprintf("anthology-export-%s.csv", timestamp)

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	if err := h.exporter.Export(w, itemList); err != nil {
		h.logger.Error("write csv export", "error", err)
		// Response headers already sent, can't change status code
		return
	}
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

func handleServiceError(w http.ResponseWriter, err error, logger *slog.Logger) {
	if errors.Is(err, items.ErrNotFound) {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	if errors.Is(err, items.ErrValidation) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	logger.Error("service error", "error", err)
	writeError(w, http.StatusInternalServerError, "unexpected error")
}

const maxJSONBodyBytes int64 = 8 << 20 // 8 MiB (accommodates 5MB images after base64 encoding)

var errPayloadTooLarge = errors.New("payload too large")

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) error {
	limited := http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	defer func() {
		_ = limited.Close()
	}()

	decoder := json.NewDecoder(limited)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return fmt.Errorf("%w (max %d bytes)", errPayloadTooLarge, maxErr.Limit)
		}
		return err
	}
	return nil
}

func writeJSONError(w http.ResponseWriter, err error) {
	if errors.Is(err, errPayloadTooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "payload too large")
		return
	}
	// Return generic message to avoid leaking internal JSON parsing details
	writeError(w, http.StatusBadRequest, "invalid request body")
}

func decodeInto(raw map[string]json.RawMessage, payload any) error {
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, payload)
}
