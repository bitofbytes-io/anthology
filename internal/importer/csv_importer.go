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

func (i *CSVImporter) Import(ctx context.Context, reader io.Reader, ownerID uuid.UUID) (Summary, error) {
	if i.items == nil {
		return Summary{}, fmt.Errorf("%w: item store is not configured", ErrInvalidCSV)
	}

	existing, err := i.items.List(ctx, items.ListOptions{OwnerID: ownerID})
	if err != nil {
		return Summary{}, err
	}

	tracker := newDuplicateTracker(existing)

	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1
	csvReader.TrimLeadingSpace = true

	header, err := csvReader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return Summary{}, fmt.Errorf("%w: file is empty", ErrInvalidCSV)
		}
		return Summary{}, fmt.Errorf("%w: failed to read header", ErrInvalidCSV)
	}

	columns, err := normalizeHeader(header)
	if err != nil {
		return Summary{}, err
	}

	type parsedRow struct {
		number int
		values map[string]string
	}

	var rows []parsedRow
	rowNumber := 1
	totalRows := 0

	for {
		record, err := csvReader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return Summary{}, fmt.Errorf("%w: failed to read row %d", ErrInvalidCSV, rowNumber+1)
		}
		rowNumber++
		values := mapRecord(columns, record)
		if isRowEmpty(values) {
			continue
		}

		totalRows++
		if totalRows > MaxImportRows {
			return Summary{}, fmt.Errorf("%w: CSV exceeds maximum of %d rows", ErrInvalidCSV, MaxImportRows)
		}

		rows = append(rows, parsedRow{
			number: rowNumber,
			values: values,
		})
	}

	summary := Summary{TotalRows: totalRows}

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
	title      string
	identifier string
}

func (i *CSVImporter) buildInput(lookups *lookupAllowance, values map[string]string, ownerID uuid.UUID) (items.CreateItemInput, rowMeta, error) {
	meta := rowMeta{}

	rawType := strings.ToLower(values["itemtype"])
	itemType := items.ItemType(strings.TrimSpace(rawType))
	switch itemType {
	case items.ItemTypeBook, items.ItemTypeGame, items.ItemTypeMovie, items.ItemTypeMusic:
	default:
		return items.CreateItemInput{}, meta, fmt.Errorf("itemType must be one of book, game, movie, or music")
	}

	title := strings.TrimSpace(values["title"])
	meta.title = title
	creator := strings.TrimSpace(values["creator"])
	isbn13 := strings.TrimSpace(values["isbn13"])
	isbn10 := strings.TrimSpace(values["isbn10"])
	meta.identifier = firstNonEmpty(isbn13, isbn10)

	releaseYear, err := parseOptionalInt(values["releaseyear"], "releaseYear")
	if err != nil {
		return items.CreateItemInput{}, meta, err
	}

	pageCount, err := parseOptionalInt(values["pagecount"], "pageCount")
	if err != nil {
		return items.CreateItemInput{}, meta, err
	}

	currentPage, err := parseOptionalNonNegativeInt(values["currentpage"], "currentPage")
	if err != nil {
		return items.CreateItemInput{}, meta, err
	}

	format := items.Format(strings.TrimSpace(values["format"]))
	genre := items.Genre(strings.TrimSpace(values["genre"]))
	rating, err := parseOptionalIntAllowAny(values["rating"], "rating")
	if err != nil {
		return items.CreateItemInput{}, meta, err
	}
	retailPriceUsd, err := parseOptionalFloat(values["retailpriceusd"], "retailPriceUsd")
	if err != nil {
		return items.CreateItemInput{}, meta, err
	}
	googleVolumeId := strings.TrimSpace(values["googlevolumeid"])

	statusValue := strings.ToLower(strings.TrimSpace(values["readingstatus"]))
	var readingStatus items.BookStatus
	if statusValue != "" {
		readingStatus = items.BookStatus(statusValue)
	}
	readAt, err := parseOptionalTime(values["readat"], "readAt")
	if err != nil {
		return items.CreateItemInput{}, meta, err
	}

	createdAt, err := parseOptionalTime(values["createdat"], "createdAt")
	if err != nil {
		return items.CreateItemInput{}, meta, err
	}
	updatedAt, err := parseOptionalTime(values["updatedat"], "updatedAt")
	if err != nil {
		return items.CreateItemInput{}, meta, err
	}

	description := strings.TrimSpace(values["description"])
	coverImage := strings.TrimSpace(values["coverimage"])
	notes := strings.TrimSpace(values["notes"])
	platform := strings.TrimSpace(values["platform"])
	ageGroup := strings.TrimSpace(values["agegroup"])
	playerCount := strings.TrimSpace(values["playercount"])
	seriesName := strings.TrimSpace(values["seriesname"])
	volumeNumber, err := parseOptionalInt(values["volumenumber"], "volumeNumber")
	if err != nil {
		return items.CreateItemInput{}, meta, err
	}
	totalVolumes, err := parseOptionalInt(values["totalvolumes"], "totalVolumes")
	if err != nil {
		return items.CreateItemInput{}, meta, err
	}

	if itemType == items.ItemTypeBook && title == "" {
		identifier := meta.identifier
		if identifier == "" {
			return items.CreateItemInput{}, meta, fmt.Errorf("provide a title or ISBN/UPC for books")
		}
		metadata, err := i.lookupBook(lookups, identifier)
		if err != nil {
			return items.CreateItemInput{}, meta, err
		}

		title = metadata.Title
		if creator == "" {
			creator = metadata.Creator
		}
		if releaseYear == nil && metadata.ReleaseYear != nil {
			releaseYear = metadata.ReleaseYear
		}
		if pageCount == nil && metadata.PageCount != nil {
			pageCount = metadata.PageCount
		}
		if isbn13 == "" {
			isbn13 = metadata.ISBN13
		}
		if isbn10 == "" {
			isbn10 = metadata.ISBN10
		}
		if description == "" {
			description = metadata.Description
		}
		if coverImage == "" {
			coverImage = metadata.CoverImage
		}
		genre = items.Genre(metadata.Genre)
		retailPriceUsd = metadata.RetailPriceUsd
		googleVolumeId = metadata.GoogleVolumeId
	}

	if title == "" {
		return items.CreateItemInput{}, meta, fmt.Errorf("title is required for %s rows", itemType)
	}

	return items.CreateItemInput{
		OwnerID:        ownerID,
		Title:          title,
		Creator:        creator,
		ItemType:       itemType,
		ReleaseYear:    releaseYear,
		PageCount:      pageCount,
		CurrentPage:    currentPage,
		ISBN13:         isbn13,
		ISBN10:         isbn10,
		Description:    description,
		CoverImage:     coverImage,
		Format:         format,
		Genre:          genre,
		Rating:         rating,
		RetailPriceUsd: retailPriceUsd,
		GoogleVolumeId: googleVolumeId,
		Platform:       platform,
		AgeGroup:       ageGroup,
		PlayerCount:    playerCount,
		SeriesName:     seriesName,
		VolumeNumber:   volumeNumber,
		TotalVolumes:   totalVolumes,
		ReadingStatus:  readingStatus,
		ReadAt:         readAt,
		Notes:          notes,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}, meta, nil
}

func (i *CSVImporter) lookupBook(lookups *lookupAllowance, query string) (catalog.Metadata, error) {
	if i.catalog == nil {
		return catalog.Metadata{}, fmt.Errorf("%w: metadata lookup is unavailable", ErrInvalidCSV)
	}
	if lookups.remaining <= 0 {
		return catalog.Metadata{}, fmt.Errorf("metadata lookup skipped: this import reached its limit of %d ISBN lookups; add a title to this row or import it in a smaller file", i.maxLookups)
	}
	if lookups.ctx.Err() != nil {
		return catalog.Metadata{}, errLookupBudgetExhausted
	}
	lookups.remaining--

	metadata, err := i.catalog.Lookup(lookups.ctx, query, catalog.CategoryBook)
	if err != nil {
		if lookups.ctx.Err() != nil {
			return catalog.Metadata{}, errLookupBudgetExhausted
		}
		if errors.Is(err, catalog.ErrNotFound) {
			return catalog.Metadata{}, fmt.Errorf("no metadata found for %s", query)
		}
		if errors.Is(err, catalog.ErrInvalidQuery) {
			return catalog.Metadata{}, fmt.Errorf("ISBN/UPC %s is not valid", query)
		}
		// Unexpected lookup failures can carry upstream transport details, so
		// report a generic message instead of returning them to the browser.
		return catalog.Metadata{}, errMetadataLookupFailed
	}
	if len(metadata) == 0 {
		return catalog.Metadata{}, fmt.Errorf("no metadata found for %s", query)
	}
	return metadata[0], nil
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

type duplicateTracker struct {
	known map[string]string
}

func newDuplicateTracker(existing []items.Item) *duplicateTracker {
	tracker := &duplicateTracker{known: map[string]string{}}
	for _, item := range existing {
		tracker.store("title", strings.ToLower(strings.TrimSpace(item.Title)))
		tracker.store("isbn13", items.NormalizeIdentifier(item.ISBN13))
		tracker.store("isbn10", items.NormalizeIdentifier(item.ISBN10))
	}
	return tracker
}

func (t *duplicateTracker) store(field string, value string) {
	if value == "" {
		return
	}
	t.known[field+":"+value] = field
}

func (t *duplicateTracker) Check(input items.CreateItemInput) (string, bool) {
	title := strings.ToLower(strings.TrimSpace(input.Title))
	if title != "" {
		if reason, ok := t.known["title:"+title]; ok {
			return fmt.Sprintf("duplicate %s", reason), true
		}
	}
	if isbn := items.NormalizeIdentifier(input.ISBN13); isbn != "" {
		if reason, ok := t.known["isbn13:"+isbn]; ok {
			return fmt.Sprintf("duplicate %s", reason), true
		}
	}
	if isbn := items.NormalizeIdentifier(input.ISBN10); isbn != "" {
		if reason, ok := t.known["isbn10:"+isbn]; ok {
			return fmt.Sprintf("duplicate %s", reason), true
		}
	}
	return "", false
}

func (t *duplicateTracker) Add(input items.CreateItemInput) {
	t.store("title", strings.ToLower(strings.TrimSpace(input.Title)))
	t.store("isbn13", items.NormalizeIdentifier(input.ISBN13))
	t.store("isbn10", items.NormalizeIdentifier(input.ISBN10))
}
