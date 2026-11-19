package importer

import (
	"context"
	"errors"
	"time"

	"anthology/internal/catalog"
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
