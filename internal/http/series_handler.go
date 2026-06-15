package http

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"anthology/internal/items"
)

// SeriesHandler exposes series-related endpoints.
type SeriesHandler struct {
	service *items.Service
	logger  *slog.Logger
}

// NewSeriesHandler creates a handler.
func NewSeriesHandler(service *items.Service, logger *slog.Logger) *SeriesHandler {
	return &SeriesHandler{service: service, logger: logger}
}

// List returns all series with summaries and standalone books.
func (h *SeriesHandler) List(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	opts := parseSeriesListOptions(r)

	response, err := h.service.ListSeries(r.Context(), opts, user.ID)
	if err != nil {
		h.logger.Error("list series", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list series")
		return
	}

	writeJSON(w, http.StatusOK, response)
}

// Get returns details for a single series.
func (h *SeriesHandler) Get(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		writeError(w, http.StatusBadRequest, "series name is required")
		return
	}

	summary, err := h.service.GetSeriesByName(r.Context(), name, user.ID)
	if err != nil {
		switch {
		case errors.Is(err, items.ErrNotFound):
			writeError(w, http.StatusNotFound, "series not found")
		default:
			h.logger.Error("get series", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to get series")
		}
		return
	}

	writeJSON(w, http.StatusOK, summary)
}

// Update renames a series.
func (h *SeriesHandler) Update(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		writeError(w, http.StatusBadRequest, "series name is required")
		return
	}

	var body struct {
		NewName string `json:"newName"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		writeJSONError(w, err)
		return
	}

	newName := strings.TrimSpace(body.NewName)
	if newName == "" {
		writeError(w, http.StatusBadRequest, "new series name is required")
		return
	}

	summary, err := h.service.UpdateSeriesName(r.Context(), name, newName, user.ID)
	if err != nil {
		switch {
		case errors.Is(err, items.ErrNotFound):
			writeError(w, http.StatusNotFound, "series not found")
		case errors.Is(err, items.ErrValidation):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			h.logger.Error("update series", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to update series")
		}
		return
	}

	h.logger.Info("series updated", "user_id", user.ID, "items_updated", len(summary.Items))
	writeJSON(w, http.StatusOK, summary)
}

// Delete removes series association from all items in a series.
func (h *SeriesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		writeError(w, http.StatusBadRequest, "series name is required")
		return
	}

	count, err := h.service.DeleteSeries(r.Context(), name, user.ID)
	if err != nil {
		switch {
		case errors.Is(err, items.ErrNotFound):
			writeError(w, http.StatusNotFound, "series not found")
		case errors.Is(err, items.ErrValidation):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			h.logger.Error("delete series", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to delete series")
		}
		return
	}

	h.logger.Info("series deleted", "user_id", user.ID, "items_updated", count)
	writeJSON(w, http.StatusOK, map[string]any{
		"itemsUpdated": count,
	})
}

func parseSeriesListOptions(r *http.Request) items.SeriesListOptions {
	opts := items.SeriesListOptions{}

	if r.URL.Query().Get("include_items") == "true" {
		opts.IncludeItems = true
	}

	if rawStatus := strings.TrimSpace(r.URL.Query().Get("status")); rawStatus != "" {
		status := items.SeriesStatus(rawStatus)
		switch status {
		case items.SeriesStatusComplete, items.SeriesStatusIncomplete, items.SeriesStatusUnknown:
			opts.Status = &status
		}
	}

	return opts
}
