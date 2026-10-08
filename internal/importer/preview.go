package importer

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"

	"anthology/internal/items"
)

// MaxCandidatesPerRow caps the catalog matches offered for one titleless book
// row.
const MaxCandidatesPerRow = 5

// RowStatus classifies a previewed CSV row.
type RowStatus string

const (
	// RowReady rows can be imported as shown.
	RowReady RowStatus = "ready"
	// RowDuplicate rows match a library item or an earlier row of the file
	// and are skipped.
	RowDuplicate RowStatus = "duplicate"
	// RowNeedsMatch rows cannot be imported as they are: a catalog match must
	// be chosen, or the row has a problem that has to be fixed in the CSV.
	RowNeedsMatch RowStatus = "needs_match"
)

// ReviewedItem is the exact, normalized item a reviewed row creates. It is
// the item create request body plus the CSV's optional timestamps; the owner
// is always the signed-in user.
type ReviewedItem struct {
	items.CreateItemInput
	// These shadow the embedded fields, which are hidden from JSON.
	CreatedAt *time.Time `json:"createdAt,omitempty"`
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
}

func newReviewedItem(input items.CreateItemInput) ReviewedItem {
	input.OwnerID = uuid.Nil
	return ReviewedItem{CreateItemInput: input, CreatedAt: input.CreatedAt, UpdatedAt: input.UpdatedAt}
}

// createInput returns the create input for the item, owned by ownerID.
func (r ReviewedItem) createInput(ownerID uuid.UUID) items.CreateItemInput {
	input := r.CreateItemInput
	input.OwnerID = ownerID
	input.CreatedAt = r.CreatedAt
	input.UpdatedAt = r.UpdatedAt
	return input
}

// LibraryMatch describes an existing library item that a row duplicates.
type LibraryMatch struct {
	// Field is the matching field: title, isbn13, or isbn10.
	Field       string         `json:"field"`
	ItemID      uuid.UUID      `json:"itemId"`
	Title       string         `json:"title"`
	Creator     string         `json:"creator"`
	ItemType    items.ItemType `json:"itemType"`
	ReleaseYear *int           `json:"releaseYear,omitempty"`
	ISBN13      string         `json:"isbn13"`
	ISBN10      string         `json:"isbn10"`
}

// FileMatch describes an earlier row of the same import that a row duplicates.
type FileMatch struct {
	Field string `json:"field"`
	Row   int    `json:"row"`
}

// Candidate is one catalog match a titleless book row can be imported as,
// already merged with the row's own CSV values.
type Candidate struct {
	Item           ReviewedItem   `json:"item"`
	Keys           []string       `json:"keys"`
	LibraryMatches []LibraryMatch `json:"libraryMatches"`
	// CSVOverrides lists fields where the CSV value was kept over a
	// different catalog value.
	CSVOverrides []string `json:"csvOverrides"`
}

// PreviewRow is the review state of one CSV data row.
type PreviewRow struct {
	// Row is the physical line number in the file; the header is row 1.
	Row        int       `json:"row"`
	Status     RowStatus `json:"status"`
	ItemType   string    `json:"itemType"`
	Title      string    `json:"title"`
	Identifier string    `json:"identifier"`
	// Item, Keys and LibraryMatches are set when the row parsed into a valid
	// item without needing a catalog match.
	Item           *ReviewedItem  `json:"item,omitempty"`
	Keys           []string       `json:"keys,omitempty"`
	LibraryMatches []LibraryMatch `json:"libraryMatches,omitempty"`
	FileMatch      *FileMatch     `json:"fileMatch,omitempty"`
	// Problem explains why a needs_match row cannot be imported as is.
	Problem    string      `json:"problem,omitempty"`
	Candidates []Candidate `json:"candidates,omitempty"`
}

// Preview is the read-only review of a CSV upload.
type Preview struct {
	TotalRows  int          `json:"totalRows"`
	Ready      int          `json:"ready"`
	Duplicates int          `json:"duplicates"`
	NeedsMatch int          `json:"needsMatch"`
	Rows       []PreviewRow `json:"rows"`
}

const chooseMatchProblem = "Choose the catalog edition to import for this ISBN."

// Preview parses the whole upload and reports what importing each row would
// do, without creating anything. Rows are validated and normalized exactly as
// items.Service.Create would, and checked against the owner's library and the
// earlier rows of the file. Titleless book rows are looked up in the catalog,
// within the same lookup limits as Import, and every match is offered instead
// of picking the first one.
func (i *CSVImporter) Preview(ctx context.Context, reader io.Reader, ownerID uuid.UUID) (Preview, error) {
	if i.items == nil {
		return Preview{}, fmt.Errorf("%w: item store is not configured", ErrInvalidCSV)
	}

	rows, err := readRows(reader)
	if err != nil {
		return Preview{}, err
	}

	existing, err := i.items.List(ctx, items.ListOptions{OwnerID: ownerID})
	if err != nil {
		return Preview{}, err
	}
	library := newDuplicateTracker(existing)
	file := newDuplicateTracker(nil)

	lookupCtx, cancelLookups := context.WithTimeout(ctx, i.lookupBudget)
	defer cancelLookups()
	allowance := &lookupAllowance{ctx: lookupCtx, remaining: i.maxLookups}

	preview := Preview{TotalRows: len(rows), Rows: make([]PreviewRow, 0, len(rows))}
	for _, row := range rows {
		previewRow := i.previewRow(allowance, library, file, row)
		switch previewRow.Status {
		case RowReady:
			preview.Ready++
		case RowDuplicate:
			preview.Duplicates++
		case RowNeedsMatch:
			preview.NeedsMatch++
		}
		preview.Rows = append(preview.Rows, previewRow)
	}
	return preview, nil
}

func (i *CSVImporter) previewRow(lookups *lookupAllowance, library, file *duplicateTracker, row parsedRow) PreviewRow {
	draft, err := parseRow(row.values)
	result := PreviewRow{
		Row:        row.number,
		Status:     RowNeedsMatch,
		ItemType:   draft.meta.itemType,
		Title:      draft.meta.title,
		Identifier: draft.meta.identifier,
	}
	if err != nil {
		result.Problem = err.Error()
		return result
	}

	if draft.needsLookup {
		metadata, err := i.lookupBook(lookups, draft.meta.identifier)
		if err != nil {
			result.Problem = err.Error()
			return result
		}
		var invalid error
		for _, match := range metadata {
			if len(result.Candidates) == MaxCandidatesPerRow {
				break
			}
			merged, overrides := mergeCatalogMetadata(draft.input, match)
			normalized, err := items.NormalizeCreateInput(merged)
			if err != nil {
				invalid = err
				continue
			}
			result.Candidates = append(result.Candidates, Candidate{
				Item:           newReviewedItem(normalized),
				Keys:           keyStrings(inputKeys(normalized)),
				LibraryMatches: libraryMatches(library.Matches(normalized)),
				CSVOverrides:   append([]string{}, overrides...),
			})
		}
		if len(result.Candidates) == 0 {
			result.Problem = fmt.Sprintf("the catalog match for %s cannot be imported: %v", draft.meta.identifier, invalid)
			return result
		}
		result.Problem = chooseMatchProblem
		return result
	}

	normalized, err := items.NormalizeCreateInput(draft.input)
	if err != nil {
		result.Problem = err.Error()
		return result
	}
	item := newReviewedItem(normalized)
	result.Item = &item
	result.Title = normalized.Title
	result.Keys = keyStrings(inputKeys(normalized))

	if matches := library.Matches(normalized); len(matches) > 0 {
		result.Status = RowDuplicate
		result.LibraryMatches = libraryMatches(matches)
		return result
	}
	if matches := file.Matches(normalized); len(matches) > 0 {
		result.Status = RowDuplicate
		result.FileMatch = &FileMatch{Field: matches[0].field, Row: matches[0].row}
		return result
	}
	result.Status = RowReady
	file.AddRow(normalized, row.number)
	return result
}

// libraryMatches describes the library items among sources, skipping sources
// that are rows of the current import.
func libraryMatches(sources []duplicateSource) []LibraryMatch {
	matches := make([]LibraryMatch, 0, len(sources))
	for _, source := range sources {
		if source.item == nil {
			continue
		}
		matches = append(matches, LibraryMatch{
			Field:       source.field,
			ItemID:      source.item.ID,
			Title:       source.item.Title,
			Creator:     source.item.Creator,
			ItemType:    source.item.ItemType,
			ReleaseYear: source.item.ReleaseYear,
			ISBN13:      source.item.ISBN13,
			ISBN10:      source.item.ISBN10,
		})
	}
	return matches
}
