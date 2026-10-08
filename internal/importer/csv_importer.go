package importer

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"anthology/internal/catalog"
	"anthology/internal/exporter"
	"anthology/internal/items"
)

type ItemStore interface {
	Create(ctx context.Context, input items.CreateItemInput) (items.Item, error)
	List(ctx context.Context, opts items.ListOptions) ([]items.Item, error)
}

type CatalogLookup interface {
	Lookup(ctx context.Context, query string, category catalog.Category) ([]catalog.Metadata, error)
}

type Summary struct {
	TotalRows         int             `json:"totalRows"`
	Imported          int             `json:"imported"`
	SkippedDuplicates []SkippedRecord `json:"skippedDuplicates"`
	Failed            []FailedRecord  `json:"failed"`
	TruncatedRecords  bool            `json:"truncatedRecords,omitempty"`
	// Interrupted reports that the request context ended before every row was
	// processed; the unprocessed rows are listed in Failed.
	Interrupted bool `json:"interrupted,omitempty"`
}

func (s *Summary) addFailed(record FailedRecord) {
	if len(s.Failed) < MaxFailedRecords {
		s.Failed = append(s.Failed, record)
		return
	}
	s.TruncatedRecords = true
}

type SkippedRecord struct {
	Row        int    `json:"row"`
	Title      string `json:"title,omitempty"`
	Identifier string `json:"identifier,omitempty"`
	Reason     string `json:"reason"`
}

type FailedRecord struct {
	Row        int    `json:"row"`
	Title      string `json:"title,omitempty"`
	Identifier string `json:"identifier,omitempty"`
	Error      string `json:"error"`
}

var ErrInvalidCSV = errors.New("invalid csv upload")

var errMetadataLookupFailed = errors.New("metadata lookup failed; add a title to this row or try again later")

var errImportInterrupted = errors.New("import stopped before this row finished because the request ran out of time; upload the file again to import the remaining rows (rows already imported are skipped as duplicates)")

var errLookupBudgetExhausted = errors.New("metadata lookup skipped: this import ran out of time for ISBN lookups; add a title to this row or import it in a smaller file")

// MaxImportRows limits the number of data rows processed per CSV import to
// prevent excessive memory usage and long-running requests.
const MaxImportRows = 1000

// MaxFailedRecords caps the number of failed/skipped records stored in the
// summary to avoid unbounded memory growth from malformed uploads.
const MaxFailedRecords = 100

// MaxLookupsPerImport caps the number of catalog lookups (rows that need
// metadata because they have an ISBN but no title) performed per import.
// Rows beyond the cap are reported as failed.
const MaxLookupsPerImport = 100

// LookupBudget bounds the total time an import may spend on catalog lookups so
// the request completes within the HTTP server's request timeout. Rows that
// still need a lookup once the budget is spent are reported as failed.
const LookupBudget = 30 * time.Second

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

type CSVImporter struct {
	items        ItemStore
	catalog      CatalogLookup
	maxLookups   int
	lookupBudget time.Duration
}

func NewCSVImporter(items ItemStore, catalog CatalogLookup) *CSVImporter {
	return &CSVImporter{
		items:        items,
		catalog:      catalog,
		maxLookups:   MaxLookupsPerImport,
		lookupBudget: LookupBudget,
	}
}

// lookupAllowance tracks how many catalog lookups and how much lookup time
// remain for a single import.
type lookupAllowance struct {
	ctx       context.Context
	remaining int
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

// Import creates an item for every valid row, looking up metadata for titleless
// books and using the first catalog result. It is the original single-request
// import; the reviewed flow uses Preview and Commit instead.
func (i *CSVImporter) Import(ctx context.Context, reader io.Reader, ownerID uuid.UUID) (Summary, error) {
	if i.items == nil {
		return Summary{}, fmt.Errorf("%w: item store is not configured", ErrInvalidCSV)
	}

	existing, err := i.items.List(ctx, items.ListOptions{OwnerID: ownerID})
	if err != nil {
		return Summary{}, err
	}

	tracker := newDuplicateTracker(existing)

	rows, err := readRows(reader)
	if err != nil {
		return Summary{}, err
	}

	summary := Summary{TotalRows: len(rows)}

	lookupCtx, cancelLookups := context.WithTimeout(ctx, i.lookupBudget)
	defer cancelLookups()
	allowance := &lookupAllowance{ctx: lookupCtx, remaining: i.maxLookups}

	for idx, row := range rows {
		if ctx.Err() != nil {
			// The request is out of time: report the remaining rows instead of
			// letting each insert fail with a context error.
			summary.Interrupted = true
			for _, rest := range rows[idx:] {
				summary.addFailed(FailedRecord{
					Row:        rest.number,
					Title:      strings.TrimSpace(rest.values["title"]),
					Identifier: firstNonEmpty(rest.values["isbn13"], rest.values["isbn10"]),
					Error:      errImportInterrupted.Error(),
				})
			}
			break
		}

		values := row.values
		input, meta, rowErr := i.buildInput(allowance, values, ownerID)
		if rowErr != nil {
			summary.addFailed(FailedRecord{
				Row:        row.number,
				Title:      meta.title,
				Identifier: meta.identifier,
				Error:      rowErr.Error(),
			})
			continue
		}

		if reason, ok := tracker.Check(input); ok {
			if len(summary.SkippedDuplicates) < MaxFailedRecords {
				summary.SkippedDuplicates = append(summary.SkippedDuplicates, SkippedRecord{
					Row:        row.number,
					Title:      input.Title,
					Identifier: firstIdentifier(input),
					Reason:     reason,
				})
			} else {
				summary.TruncatedRecords = true
			}
			continue
		}

		if _, err := i.items.Create(ctx, input); err != nil {
			message := err.Error()
			if ctx.Err() != nil {
				summary.Interrupted = true
				message = errImportInterrupted.Error()
			}
			summary.addFailed(FailedRecord{
				Row:        row.number,
				Title:      input.Title,
				Identifier: firstIdentifier(input),
				Error:      message,
			})
			continue
		}

		tracker.Add(input)
		summary.Imported++
	}

	return summary, nil
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

func (i *CSVImporter) buildInput(lookups *lookupAllowance, values map[string]string, ownerID uuid.UUID) (items.CreateItemInput, rowMeta, error) {
	draft, err := parseRow(values)
	if err != nil {
		return items.CreateItemInput{}, draft.meta, err
	}
	input := draft.input
	input.OwnerID = ownerID
	if draft.needsLookup {
		candidates, err := i.lookupBook(lookups, draft.meta.identifier)
		if err != nil {
			return items.CreateItemInput{}, draft.meta, err
		}
		input, _ = mergeCatalogMetadata(input, candidates[0])
		if input.Title == "" {
			return items.CreateItemInput{}, draft.meta, fmt.Errorf("title is required for %s rows", input.ItemType)
		}
	}
	return input, draft.meta, nil
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

// mergeCatalogMetadata fills the catalog-provided fields a titleless book row
// left blank. Values present in the CSV always win; overrides lists the fields
// where the CSV kept its own value over a different catalog value.
func mergeCatalogMetadata(input items.CreateItemInput, metadata catalog.Metadata) (merged items.CreateItemInput, overrides []string) {
	mergeString := func(field string, csvValue *string, catalogValue string) {
		switch {
		case *csvValue == "":
			*csvValue = catalogValue
		case catalogValue != "" && catalogValue != *csvValue:
			overrides = append(overrides, field)
		}
	}
	mergeInt := func(field string, csvValue **int, catalogValue *int) {
		switch {
		case *csvValue == nil:
			*csvValue = catalogValue
		case catalogValue != nil && *catalogValue != **csvValue:
			overrides = append(overrides, field)
		}
	}

	if input.Title == "" {
		input.Title = metadata.Title
	}
	mergeString("creator", &input.Creator, metadata.Creator)
	mergeInt("releaseYear", &input.ReleaseYear, metadata.ReleaseYear)
	mergeInt("pageCount", &input.PageCount, metadata.PageCount)
	mergeString("isbn13", &input.ISBN13, metadata.ISBN13)
	mergeString("isbn10", &input.ISBN10, metadata.ISBN10)
	mergeString("description", &input.Description, metadata.Description)
	mergeString("coverImage", &input.CoverImage, metadata.CoverImage)
	genre := string(input.Genre)
	mergeString("genre", &genre, metadata.Genre)
	input.Genre = items.Genre(genre)
	switch {
	case input.RetailPriceUsd == nil:
		input.RetailPriceUsd = metadata.RetailPriceUsd
	case metadata.RetailPriceUsd != nil && *metadata.RetailPriceUsd != *input.RetailPriceUsd:
		overrides = append(overrides, "retailPriceUsd")
	}
	mergeString("googleVolumeId", &input.GoogleVolumeId, metadata.GoogleVolumeId)
	return input, overrides
}

// lookupBook returns every catalog result for a titleless book row, in catalog
// order, or an error explaining why the row cannot be filled in.
func (i *CSVImporter) lookupBook(lookups *lookupAllowance, query string) ([]catalog.Metadata, error) {
	if i.catalog == nil {
		return nil, fmt.Errorf("%w: metadata lookup is unavailable", ErrInvalidCSV)
	}
	if lookups.remaining <= 0 {
		return nil, fmt.Errorf("metadata lookup skipped: this import reached its limit of %d ISBN lookups; add a title to this row or import it in a smaller file", i.maxLookups)
	}
	if lookups.ctx.Err() != nil {
		return nil, errLookupBudgetExhausted
	}
	lookups.remaining--

	metadata, err := i.catalog.Lookup(lookups.ctx, query, catalog.CategoryBook)
	if err != nil {
		if lookups.ctx.Err() != nil {
			return nil, errLookupBudgetExhausted
		}
		if errors.Is(err, catalog.ErrNotFound) {
			return nil, fmt.Errorf("no metadata found for %s", query)
		}
		if errors.Is(err, catalog.ErrInvalidQuery) {
			return nil, fmt.Errorf("ISBN/UPC %s is not valid", query)
		}
		// Unexpected lookup failures can carry upstream transport details, so
		// report a generic message instead of returning them to the browser.
		return nil, errMetadataLookupFailed
	}
	if len(metadata) == 0 {
		return nil, fmt.Errorf("no metadata found for %s", query)
	}
	return metadata, nil
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

func parseOptionalInt(value string, field string) (*int, error) {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return nil, nil
	}
	parsed, err := strconv.Atoi(cleaned)
	if err != nil {
		return nil, fmt.Errorf("%s must be a number", field)
	}
	if parsed <= 0 {
		return nil, fmt.Errorf("%s must be positive", field)
	}
	return &parsed, nil
}

func parseOptionalNonNegativeInt(value string, field string) (*int, error) {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return nil, nil
	}
	parsed, err := strconv.Atoi(cleaned)
	if err != nil {
		return nil, fmt.Errorf("%s must be a number", field)
	}
	if parsed < 0 {
		return nil, fmt.Errorf("%s must be zero or greater", field)
	}
	return &parsed, nil
}

func parseOptionalIntAllowAny(value string, field string) (*int, error) {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return nil, nil
	}
	parsed, err := strconv.Atoi(cleaned)
	if err != nil {
		return nil, fmt.Errorf("%s must be a number", field)
	}
	return &parsed, nil
}

func parseOptionalFloat(value string, field string) (*float64, error) {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return nil, fmt.Errorf("%s must be a number", field)
	}
	return &parsed, nil
}

func parseOptionalTime(value string, field string) (*time.Time, error) {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, cleaned)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339Nano, cleaned)
	}
	if err != nil {
		return nil, fmt.Errorf("%s must be an RFC3339 timestamp", field)
	}
	return &parsed, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstIdentifier(input items.CreateItemInput) string {
	if input.ISBN13 != "" {
		return input.ISBN13
	}
	if input.ISBN10 != "" {
		return input.ISBN10
	}
	return ""
}
