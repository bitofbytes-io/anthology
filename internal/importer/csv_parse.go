package importer

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"anthology/internal/exporter"
	"anthology/internal/items"
)

var requiredColumns = []string{
	"title",
	"creator",
	"itemtype",
	"releaseyear",
	"pagecount",
	"isbn13",
	"isbn10",
	"description",
	"coverimage",
	"notes",
}

// parsedRow is one non-empty CSV data row and its physical line number in the
// file (the header is row 1).
type parsedRow struct {
	number int
	values map[string]string
}

// readRows parses the whole upload before any row is prepared, so a malformed
// or oversized file is rejected without side effects.
func readRows(reader io.Reader) ([]parsedRow, error) {
	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1
	csvReader.TrimLeadingSpace = true

	header, err := csvReader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("%w: file is empty", ErrInvalidCSV)
		}
		return nil, fmt.Errorf("%w: failed to read header", ErrInvalidCSV)
	}

	columns, err := normalizeHeader(header)
	if err != nil {
		return nil, err
	}

	// endLine is the file line the most recently read record ends on; quoted
	// values may span lines.
	endLine := func(record []string) int {
		last := len(record) - 1
		line, _ := csvReader.FieldPos(last)
		return line + strings.Count(record[last], "\n")
	}

	var rows []parsedRow
	rowNumber := 1
	previousEnd := endLine(header)

	for {
		record, err := csvReader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("%w: failed to read row %d", ErrInvalidCSV, rowNumber+1)
		}
		// The reader skips blank lines, but a spreadsheet still shows them as
		// rows, so count them to keep row numbers matching the file.
		start, _ := csvReader.FieldPos(0)
		rowNumber += 1 + max(0, start-previousEnd-1)
		previousEnd = endLine(record)
		values := mapRecord(columns, record)
		if isRowEmpty(values) {
			continue
		}

		if len(rows) == MaxImportRows {
			return nil, fmt.Errorf("%w: CSV exceeds maximum of %d rows", ErrInvalidCSV, MaxImportRows)
		}

		rows = append(rows, parsedRow{
			number: rowNumber,
			values: values,
		})
	}
	return rows, nil
}

type rowMeta struct {
	itemType   string
	title      string
	identifier string
}

// rowDraft is a CSV row parsed into item fields before any catalog lookup.
type rowDraft struct {
	input items.CreateItemInput
	meta  rowMeta
	// needsLookup marks a book row without a title: its metadata must come
	// from the catalog using meta.identifier.
	needsLookup bool
}

// parseRow converts CSV values into item fields. The returned draft carries
// the row's display metadata even when err is not nil.
func parseRow(values map[string]string) (rowDraft, error) {
	draft := rowDraft{}
	meta := &draft.meta

	rawType := strings.ToLower(values["itemtype"])
	itemType := items.ItemType(strings.TrimSpace(rawType))
	meta.itemType = string(itemType)

	title := strings.TrimSpace(values["title"])
	meta.title = title
	isbn13 := strings.TrimSpace(values["isbn13"])
	isbn10 := strings.TrimSpace(values["isbn10"])
	meta.identifier = firstNonEmpty(isbn13, isbn10)

	switch itemType {
	case items.ItemTypeBook, items.ItemTypeGame, items.ItemTypeMovie, items.ItemTypeMusic:
	default:
		return draft, fmt.Errorf("itemType must be one of book, game, movie, or music")
	}

	releaseYear, err := parseOptionalInt(values["releaseyear"], "releaseYear")
	if err != nil {
		return draft, err
	}

	pageCount, err := parseOptionalInt(values["pagecount"], "pageCount")
	if err != nil {
		return draft, err
	}

	currentPage, err := parseOptionalNonNegativeInt(values["currentpage"], "currentPage")
	if err != nil {
		return draft, err
	}

	rating, err := parseOptionalIntAllowAny(values["rating"], "rating")
	if err != nil {
		return draft, err
	}
	retailPriceUsd, err := parseOptionalFloat(values["retailpriceusd"], "retailPriceUsd")
	if err != nil {
		return draft, err
	}

	statusValue := strings.ToLower(strings.TrimSpace(values["readingstatus"]))
	var readingStatus items.BookStatus
	if statusValue != "" {
		readingStatus = items.BookStatus(statusValue)
	}
	readAt, err := parseOptionalTime(values["readat"], "readAt")
	if err != nil {
		return draft, err
	}

	createdAt, err := parseOptionalTime(values["createdat"], "createdAt")
	if err != nil {
		return draft, err
	}
	updatedAt, err := parseOptionalTime(values["updatedat"], "updatedAt")
	if err != nil {
		return draft, err
	}

	volumeNumber, err := parseOptionalInt(values["volumenumber"], "volumeNumber")
	if err != nil {
		return draft, err
	}
	totalVolumes, err := parseOptionalInt(values["totalvolumes"], "totalVolumes")
	if err != nil {
		return draft, err
	}

	if title == "" {
		if itemType != items.ItemTypeBook {
			return draft, fmt.Errorf("title is required for %s rows", itemType)
		}
		if meta.identifier == "" {
			return draft, fmt.Errorf("provide a title or ISBN/UPC for books")
		}
		draft.needsLookup = true
	}

	draft.input = items.CreateItemInput{
		Title:          title,
		Creator:        strings.TrimSpace(values["creator"]),
		ItemType:       itemType,
		ReleaseYear:    releaseYear,
		PageCount:      pageCount,
		CurrentPage:    currentPage,
		ISBN13:         isbn13,
		ISBN10:         isbn10,
		Description:    strings.TrimSpace(values["description"]),
		CoverImage:     strings.TrimSpace(values["coverimage"]),
		Format:         items.Format(strings.TrimSpace(values["format"])),
		Genre:          items.Genre(strings.TrimSpace(values["genre"])),
		Rating:         rating,
		RetailPriceUsd: retailPriceUsd,
		GoogleVolumeId: strings.TrimSpace(values["googlevolumeid"]),
		Platform:       strings.TrimSpace(values["platform"]),
		AgeGroup:       strings.TrimSpace(values["agegroup"]),
		PlayerCount:    strings.TrimSpace(values["playercount"]),
		SeriesName:     strings.TrimSpace(values["seriesname"]),
		VolumeNumber:   volumeNumber,
		TotalVolumes:   totalVolumes,
		ReadingStatus:  readingStatus,
		ReadAt:         readAt,
		Notes:          strings.TrimSpace(values["notes"]),
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}
	return draft, nil
}

func normalizeHeader(header []string) (map[int]string, error) {
	columns := make(map[int]string, len(header))
	seen := map[string]bool{}
	for idx, raw := range header {
		cleaned := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(raw, "\ufeff")))
		if cleaned == "" {
			continue
		}
		columns[idx] = cleaned
		seen[cleaned] = true
	}

	missing := make([]string, 0)
	for _, column := range requiredColumns {
		if !seen[column] {
			missing = append(missing, column)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%w: missing required columns: %s", ErrInvalidCSV, strings.Join(missing, ", "))
	}
	return columns, nil
}

// reversibleEscapeSchemaVersion is the first export schema whose formula-guard
// apostrophes can be removed exactly (see exporter.UnescapeCSVCell).
const reversibleEscapeSchemaVersion = 2

// mapRecord maps a CSV record onto its column names. Formula-guard apostrophes
// are removed only for rows exported by Anthology with schemaVersion 2 or
// later: version 1 exports did not escape values already starting with an
// apostrophe, and third-party files carry no guard, so both stay verbatim.
func mapRecord(columns map[int]string, record []string) map[string]string {
	values := make(map[string]string, len(columns))
	for idx, column := range columns {
		if idx >= len(record) {
			values[column] = ""
			continue
		}
		values[column] = strings.TrimSpace(record[idx])
	}
	if version, err := strconv.Atoi(values["schemaversion"]); err == nil && version >= reversibleEscapeSchemaVersion {
		for column, value := range values {
			values[column] = exporter.UnescapeCSVCell(value)
		}
	}
	return values
}

func isRowEmpty(values map[string]string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}
