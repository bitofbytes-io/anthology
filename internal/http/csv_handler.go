package http

import (
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"time"

	"anthology/internal/importer"
	"anthology/internal/metrics"
)

const maxCSVUploadBytes int64 = 5 << 20

// csvImportWriteDeadline replaces the server-wide WriteTimeout for CSV imports.
// It outlasts the import's request context (csvImportTimeout) so the summary,
// including rows reported as not processed when the context expires, is
// always delivered after rows have been committed.
const csvImportWriteDeadline = csvImportTimeout + 15*time.Second

// ImportCSV ingests a CSV file of catalog items.
func (h *ItemHandler) ImportCSV(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	file, closeUpload, ok := h.openCSVUpload(w, r)
	if !ok {
		return
	}
	defer closeUpload()

	summary, err := h.importer.Import(r.Context(), file, user.ID)
	h.metrics.Upload(csvUploadResult(err))
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

// PreviewCSVImport reports what importing an uploaded CSV would do without
// saving anything.
func (h *ItemHandler) PreviewCSVImport(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	file, closeUpload, ok := h.openCSVUpload(w, r)
	if !ok {
		return
	}
	defer closeUpload()

	preview, err := h.importer.Preview(r.Context(), file, user.ID)
	h.metrics.Upload(csvUploadResult(err))
	if err != nil {
		if errors.Is(err, importer.ErrInvalidCSV) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.logger.Error("csv import preview failed", "error", err)
		writeError(w, http.StatusInternalServerError, "import preview failed")
		return
	}

	h.logger.Info("csv import previewed",
		"user_id", user.ID,
		"total_rows", preview.TotalRows,
		"ready", preview.Ready,
		"duplicates", preview.Duplicates,
		"needs_match", preview.NeedsMatch,
	)
	writeJSON(w, http.StatusOK, preview)
}

// maxImportCommitBytes bounds a reviewed import body. It leaves room for a
// full 5 MiB CSV after JSON escaping plus the catalog metadata merged into
// up to importer.MaxLookupsPerImport rows.
const maxImportCommitBytes int64 = 16 << 20

// CommitCSVImport saves the rows the user reviewed in a preview, exactly as
// reviewed, without catalog lookups.
func (h *ItemHandler) CommitCSVImport(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	if h.importer == nil {
		writeError(w, http.StatusNotImplemented, "CSV import is not available")
		return
	}
	h.extendCSVImportWriteDeadline(w)

	var request importer.CommitRequest
	if err := decodeJSONBodyLimit(w, r, &request, maxImportCommitBytes); err != nil {
		writeJSONError(w, err)
		return
	}

	result, err := h.importer.Commit(r.Context(), request, user.ID)
	if err != nil {
		if errors.Is(err, importer.ErrInvalidCommit) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, importer.ErrReviewedImportUnavailable) {
			writeError(w, http.StatusNotImplemented, "CSV import is not available")
			return
		}
		h.logger.Error("csv import commit failed", "error", err)
		writeError(w, http.StatusInternalServerError, "import failed before any rows were saved")
		return
	}

	h.logger.Info("csv import committed",
		"user_id", user.ID,
		"rows", len(result.Rows),
		"added", result.Added,
		"skipped", result.Skipped,
		"failed", result.Failed,
		"interrupted", result.Interrupted,
		"unprocessed", result.Unprocessed,
	)
	writeJSON(w, http.StatusOK, result)
}

func (h *ItemHandler) extendCSVImportWriteDeadline(w http.ResponseWriter) {
	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(csvImportWriteDeadline)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		h.logger.Warn("extend csv import write deadline", "error", err)
	}
}

// openCSVUpload extends the write deadline and returns the uploaded "file"
// part with a function that closes it and removes the parsed form, or writes
// an error response and returns false.
func (h *ItemHandler) openCSVUpload(w http.ResponseWriter, r *http.Request) (multipart.File, func(), bool) {
	if h.importer == nil {
		writeError(w, http.StatusNotImplemented, "CSV import is not available")
		return nil, nil, false
	}

	h.extendCSVImportWriteDeadline(w)

	r.Body = http.MaxBytesReader(w, r.Body, maxCSVUploadBytes)
	if err := r.ParseMultipartForm(maxCSVUploadBytes); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			h.metrics.Upload(metrics.UploadTooLarge)
			writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("CSV upload is too large (max %d bytes)", maxErr.Limit))
			return nil, nil, false
		}
		h.metrics.Upload(metrics.UploadInvalid)
		writeError(w, http.StatusBadRequest, "invalid CSV upload")
		return nil, nil, false
	}
	removeForm := func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		removeForm()
		h.metrics.Upload(metrics.UploadInvalid)
		writeError(w, http.StatusBadRequest, "CSV file is required")
		return nil, nil, false
	}
	return file, func() {
		_ = file.Close()
		removeForm()
	}, true
}

// csvUploadResult maps the outcome of importing or previewing an uploaded CSV
// to an anthology_uploads_total result.
func csvUploadResult(err error) string {
	switch {
	case err == nil:
		return metrics.UploadSuccess
	case errors.Is(err, importer.ErrInvalidCSV):
		return metrics.UploadInvalid
	default:
		return metrics.UploadError
	}
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
	h.logger.Info("csv export completed", "user_id", user.ID, "item_count", len(itemList))

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
