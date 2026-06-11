package http

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"anthology/internal/items"
	"anthology/internal/shelves"
)

// ShelfHandler exposes HTTP endpoints for shelf management.
type ShelfHandler struct {
	svc    *shelves.Service
	logger *slog.Logger
}

func (h *ShelfHandler) handleShelfError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, shelves.ErrNotFound):
		writeError(w, http.StatusNotFound, "shelf not found")
	case errors.Is(err, shelves.ErrSlotNotFound):
		writeError(w, http.StatusNotFound, "slot not found")
	case errors.Is(err, items.ErrNotFound):
		writeError(w, http.StatusNotFound, "item not found")
	case errors.Is(err, shelves.ErrISBNNotFound):
		writeError(w, http.StatusNotFound, "no results found for scanned barcode")
	case errors.Is(err, shelves.ErrValidation):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		h.logger.Error("shelf operation failed", "error", err)
		writeError(w, http.StatusInternalServerError, "unexpected error")
	}
}

// NewShelfHandler constructs a ShelfHandler.
func NewShelfHandler(svc *shelves.Service, logger *slog.Logger) *ShelfHandler {
	return &ShelfHandler{svc: svc, logger: logger}
}

// List returns shelf summaries.
func (h *ShelfHandler) List(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	shelvesList, err := h.svc.ListShelves(r.Context(), user.ID)
	if err != nil {
		h.logger.Error("list shelves", "error", err)
		writeError(w, http.StatusInternalServerError, "Unable to list shelves")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"shelves": shelvesList})
}

// Create registers a new shelf with a default layout.
func (h *ShelfHandler) Create(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	var input shelves.CreateShelfInput
	if err := decodeJSONBody(w, r, &input); err != nil {
		writeJSONError(w, err)
		return
	}

	created, err := h.svc.CreateShelf(r.Context(), input, user.ID)
	if err != nil {
		h.handleShelfError(w, err)
		return
	}

	h.logger.Info("shelf created", "user_id", user.ID, "shelf_id", created.Shelf.ID)
	writeJSON(w, http.StatusCreated, created)
}

// Get returns a shelf and its layout.
func (h *ShelfHandler) Get(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	shelfID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid shelf id")
		return
	}

	shelf, err := h.svc.GetShelf(r.Context(), shelfID, user.ID)
	if err != nil {
		h.handleShelfError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, shelf)
}

// UpdateLayout applies a new layout and returns displaced items.
func (h *ShelfHandler) UpdateLayout(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	shelfID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid shelf id")
		return
	}

	var input shelves.UpdateLayoutInput
	if err := decodeJSONBody(w, r, &input); err != nil {
		writeJSONError(w, err)
		return
	}

	updated, displaced, err := h.svc.UpdateLayout(r.Context(), shelfID, user.ID, input)
	if err != nil {
		h.handleShelfError(w, err)
		return
	}

	h.logger.Info("shelf layout updated", "user_id", user.ID, "shelf_id", shelfID, "slot_count", len(updated.Slots), "displaced_count", len(displaced))
	writeJSON(w, http.StatusOK, map[string]any{
		"shelf":     updated,
		"displaced": displaced,
	})
}

// AssignItem assigns an item to a slot on the shelf.
func (h *ShelfHandler) AssignItem(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	shelfID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid shelf id")
		return
	}
	slotID, err := uuid.Parse(chi.URLParam(r, "slotId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid slot id")
		return
	}

	var payload struct {
		ItemID string `json:"itemId"`
	}
	if err := decodeJSONBody(w, r, &payload); err != nil {
		writeJSONError(w, err)
		return
	}
	if payload.ItemID == "" {
		writeError(w, http.StatusBadRequest, "itemId is required")
		return
	}
	itemID, err := uuid.Parse(payload.ItemID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid item id")
		return
	}

	shelf, err := h.svc.AssignItem(r.Context(), shelfID, slotID, itemID, user.ID)
	if err != nil {
		h.handleShelfError(w, err)
		return
	}

	h.logger.Info("shelf item assigned", "user_id", user.ID, "shelf_id", shelfID, "slot_id", slotID, "item_id", itemID)
	writeJSON(w, http.StatusOK, shelf)
}

// RemoveItem removes an item placement from a slot.
func (h *ShelfHandler) RemoveItem(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	shelfID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid shelf id")
		return
	}
	slotID, err := uuid.Parse(chi.URLParam(r, "slotId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid slot id")
		return
	}
	itemID, err := uuid.Parse(chi.URLParam(r, "itemId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid item id")
		return
	}

	shelf, err := h.svc.RemoveItem(r.Context(), shelfID, slotID, itemID, user.ID)
	if err != nil {
		h.handleShelfError(w, err)
		return
	}

	h.logger.Info("shelf item removed", "user_id", user.ID, "shelf_id", shelfID, "slot_id", slotID, "item_id", itemID)
	writeJSON(w, http.StatusOK, shelf)
}

// ScanAndAssign scans an ISBN and assigns the item to a slot.
func (h *ShelfHandler) ScanAndAssign(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	shelfID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid shelf id")
		return
	}
	slotID, err := uuid.Parse(chi.URLParam(r, "slotId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid slot id")
		return
	}

	var payload struct {
		ISBN string `json:"isbn"`
	}
	if err := decodeJSONBody(w, r, &payload); err != nil {
		writeJSONError(w, err)
		return
	}
	if payload.ISBN == "" {
		writeError(w, http.StatusBadRequest, "isbn is required")
		return
	}

	result, err := h.svc.ScanAndAssign(r.Context(), shelfID, slotID, payload.ISBN, user.ID)
	if err != nil {
		h.handleShelfError(w, err)
		return
	}

	h.logger.Info("shelf scan completed", "user_id", user.ID, "shelf_id", shelfID, "slot_id", slotID, "item_id", result.Item.ID, "status", result.Status)
	writeJSON(w, http.StatusOK, result)
}
