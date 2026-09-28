package exporter

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"anthology/internal/items"
)

// SchemaVersion identifies the CSV export format version.
// This version should be incremented when adding new columns or changing the format.
const SchemaVersion = "2"

// csvColumns defines the column order for export. These columns are a superset
// of the import format to ensure round-trip compatibility.
// Note: Shelf placement data is intentionally excluded as shelf import is not
// yet supported. A separate shelf export/import feature will handle that.
// Cover images stored as data: URIs are also excluded (see exportCoverImage).
// New columns are appended so existing positional consumers keep working.
var csvColumns = []string{
	"schemaVersion",
	"title",
	"creator",
	"itemType",
	"releaseYear",
	"pageCount",
	"currentPage",
	"isbn13",
	"isbn10",
	"description",
	"coverImage",
	"format",
	"genre",
	"rating",
	"retailPriceUsd",
	"googleVolumeId",
	"platform",
	"ageGroup",
	"playerCount",
	"readingStatus",
	"readAt",
	"notes",
	"createdAt",
	"updatedAt",
	"seriesName",
	"volumeNumber",
	"totalVolumes",
}

// CSVExporter exports items to CSV format.
type CSVExporter struct{}

// NewCSVExporter creates a new CSV exporter.
func NewCSVExporter() *CSVExporter {
	return &CSVExporter{}
}

// Export writes items to the given writer in CSV format.
// The export format is designed to be compatible with the CSV import feature.
func (e *CSVExporter) Export(w io.Writer, itemList []items.Item) error {
	writer := csv.NewWriter(w)

	// Write header row
	if err := writer.Write(csvColumns); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write item rows
	for _, item := range itemList {
		row := e.itemToRow(item)
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	writer.Flush()
	return writer.Error()
}

// itemToRow converts an item to a CSV row following the column order.
func (e *CSVExporter) itemToRow(item items.Item) []string {
	row := make([]string, len(csvColumns))

	row[0] = SchemaVersion
	row[1] = item.Title
	row[2] = item.Creator
	row[3] = string(item.ItemType)
	row[4] = formatPositiveInt(item.ReleaseYear)
	row[5] = formatPositiveInt(item.PageCount)
	row[6] = formatOptionalInt(item.CurrentPage)
	row[7] = item.ISBN13
	row[8] = item.ISBN10
	row[9] = item.Description
	row[10] = exportCoverImage(item.CoverImage)
	row[11] = string(item.Format)
	row[12] = string(item.Genre)
	row[13] = formatOptionalInt(item.Rating)
	row[14] = formatOptionalFloat(item.RetailPriceUsd)
	row[15] = item.GoogleVolumeId
	row[16] = item.Platform
	row[17] = item.AgeGroup
	row[18] = item.PlayerCount
	row[19] = string(item.ReadingStatus)
	row[20] = formatOptionalTime(item.ReadAt)
	row[21] = item.Notes
	row[22] = formatTime(item.CreatedAt)
	row[23] = formatTime(item.UpdatedAt)
	row[24] = item.SeriesName
	row[25] = formatPositiveInt(item.VolumeNumber)
	row[26] = formatPositiveInt(item.TotalVolumes)

	for i := range row {
		row[i] = sanitizeCSVCell(row[i])
	}

	return row
}

// exportCoverImage omits covers stored inline as data: URIs. Each can be
// hundreds of kilobytes, which exceeds spreadsheet cell limits and quickly
// pushes an export past the CSV import upload limit. URL covers are kept.
func exportCoverImage(cover string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(cover)), "data:") {
		return ""
	}
	return cover
}

// sanitizeCSVCell prefixes a single quote to values that spreadsheet software
// could interpret as a formula. Values that already begin with quotes followed
// by a formula trigger are escaped too, so UnescapeCSVCell can reverse the
// transformation exactly.
func sanitizeCSVCell(value string) string {
	if needsFormulaEscape(value) {
		return "'" + value
	}
	return value
}

// UnescapeCSVCell reverses sanitizeCSVCell for values read back during import.
func UnescapeCSVCell(value string) string {
	if strings.HasPrefix(value, "'") && needsFormulaEscape(value) {
		return value[1:]
	}
	return value
}

// needsFormulaEscape reports whether value, ignoring any leading single
// quotes, begins with a formula trigger character.
func needsFormulaEscape(value string) bool {
	rest := strings.TrimLeft(value, "'")
	if rest == "" {
		return false
	}
	switch rest[0] {
	case '=', '+', '-', '@', '\t':
		return true
	default:
		return false
	}
}

// formatOptionalInt formats an optional integer pointer to a string.
func formatOptionalInt(value *int) string {
	if value == nil {
		return ""
	}
	return strconv.Itoa(*value)
}

// formatPositiveInt formats an optional integer pointer to a string,
// treating zero as empty to maintain round-trip compatibility with the
// importer which rejects non-positive values.
func formatPositiveInt(value *int) string {
	if value == nil || *value <= 0 {
		return ""
	}
	return strconv.Itoa(*value)
}

// formatOptionalFloat formats an optional float pointer to a string.
func formatOptionalFloat(value *float64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatFloat(*value, 'f', 2, 64)
}

// formatOptionalTime formats an optional time pointer to RFC3339 string.
func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format(time.RFC3339)
}

// formatTime formats a time to RFC3339 string.
func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}
