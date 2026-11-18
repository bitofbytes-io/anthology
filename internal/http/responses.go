package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"anthology/internal/items"
)

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message})
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
	return decodeJSONBodyLimit(w, r, dst, maxJSONBodyBytes)
}

// decodeJSONBodyLimit decodes a JSON body of at most limit bytes into dst,
// rejecting unknown fields.
func decodeJSONBodyLimit(w http.ResponseWriter, r *http.Request, dst any, limit int64) error {
	limited := http.MaxBytesReader(w, r.Body, limit)
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
