package importer

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"

	"anthology/internal/items"
)

// ErrInvalidCommit marks a commit request that was rejected before anything
// was saved.
var ErrInvalidCommit = errors.New("invalid import request")

// ReviewedRow is one previewed CSV row the user chose to import, with the
// exact item the preview showed for it (or the catalog match they picked).
type ReviewedRow struct {
	Row  int          `json:"row"`
	Item ReviewedItem `json:"item"`
}

// CommitRequest is the body of a reviewed import. It holds only the rows to
// import; skipped and unselected rows stay in the browser.
type CommitRequest struct {
	Rows []ReviewedRow `json:"rows"`
}

// OutcomeStatus reports what happened to one submitted row.
type OutcomeStatus string

const (
	OutcomeAdded   OutcomeStatus = "added"
	OutcomeSkipped OutcomeStatus = "skipped"
	OutcomeFailed  OutcomeStatus = "failed"
	// OutcomeInterrupted marks the row whose save was cut short when the
	// request ended; it may or may not have been saved.
	OutcomeInterrupted OutcomeStatus = "interrupted"
	// OutcomeUnprocessed marks rows never attempted because the request ended.
	OutcomeUnprocessed OutcomeStatus = "unprocessed"
)

// RowOutcome is the result for one submitted row.
type RowOutcome struct {
	Row            int            `json:"row"`
	Status         OutcomeStatus  `json:"status"`
	Title          string         `json:"title"`
	Identifier     string         `json:"identifier"`
	ItemID         *uuid.UUID     `json:"itemId,omitempty"`
	Message        string         `json:"message,omitempty"`
	LibraryMatches []LibraryMatch `json:"libraryMatches,omitempty"`
	FileMatch      *FileMatch     `json:"fileMatch,omitempty"`
}

// CommitResult reports every submitted row. The counts always add up to the
// number of submitted rows.
type CommitResult struct {
	Added       int          `json:"added"`
	Skipped     int          `json:"skipped"`
	Failed      int          `json:"failed"`
	Interrupted int          `json:"interrupted"`
	Unprocessed int          `json:"unprocessed"`
	Rows        []RowOutcome `json:"rows"`
}

func (r *CommitResult) add(outcome RowOutcome) {
	switch outcome.Status {
	case OutcomeAdded:
		r.Added++
	case OutcomeSkipped:
		r.Skipped++
	case OutcomeFailed:
		r.Failed++
	case OutcomeInterrupted:
		r.Interrupted++
	case OutcomeUnprocessed:
		r.Unprocessed++
	}
	r.Rows = append(r.Rows, outcome)
}

const (
	msgCommitInterrupted = "The import stopped while saving this row, so it may or may not have been saved. Preview the file again to check: saved rows show as duplicates."
	msgCommitUnprocessed = "Not attempted because the import ran out of time. Preview the file again to import the remaining rows."
	msgCommitSaveFailed  = "This row could not be saved."
)

// Commit saves reviewed rows in row order. It never looks anything up in the
// catalog: each row is saved with exactly the item it carries, after the same
// validation items.Service.Create applies, owned by ownerID. Duplicates are
// rechecked against the owner's library as it is now and against rows saved
// earlier in this request, and are skipped.
//
// The check reads the library once, so it cannot see items another request
// saves concurrently, and the item store has no unique constraint to fall back
// on; repeating a commit is safe only because the repeat skips rows the first
// one saved.
func (i *CSVImporter) Commit(ctx context.Context, request CommitRequest, ownerID uuid.UUID) (CommitResult, error) {
	if i.items == nil {
		return CommitResult{}, fmt.Errorf("%w: item store is not configured", ErrInvalidCommit)
	}
	rows, err := validateCommitRows(request.Rows)
	if err != nil {
		return CommitResult{}, err
	}

	existing, err := i.items.List(ctx, items.ListOptions{OwnerID: ownerID})
	if err != nil {
		return CommitResult{}, err
	}
	tracker := newDuplicateTracker(existing)

	result := CommitResult{Rows: make([]RowOutcome, 0, len(rows))}
	for idx, row := range rows {
		if ctx.Err() != nil {
			for _, rest := range rows[idx:] {
				result.add(RowOutcome{
					Row:        rest.Row,
					Status:     OutcomeUnprocessed,
					Title:      rest.Item.Title,
					Identifier: firstIdentifier(rest.Item.CreateItemInput),
					Message:    msgCommitUnprocessed,
				})
			}
			break
		}
		result.add(i.commitRow(ctx, tracker, row, ownerID))
	}
	return result, nil
}

func (i *CSVImporter) commitRow(ctx context.Context, tracker *duplicateTracker, row ReviewedRow, ownerID uuid.UUID) RowOutcome {
	outcome := RowOutcome{
		Row:        row.Row,
		Title:      row.Item.Title,
		Identifier: firstIdentifier(row.Item.CreateItemInput),
	}

	input, err := items.NormalizeCreateInput(row.Item.createInput(ownerID))
	if err != nil {
		outcome.Status = OutcomeFailed
		outcome.Message = err.Error()
		return outcome
	}
	outcome.Title = input.Title
	outcome.Identifier = firstIdentifier(input)

	if matches := tracker.Matches(input); len(matches) > 0 {
		outcome.Status = OutcomeSkipped
		if library := libraryMatches(matches); len(library) > 0 {
			outcome.LibraryMatches = library
		} else {
			outcome.FileMatch = &FileMatch{Field: matches[0].field, Row: matches[0].row}
		}
		return outcome
	}

	item, err := i.items.Create(ctx, input)
	if err != nil {
		switch {
		case ctx.Err() != nil:
			outcome.Status = OutcomeInterrupted
			outcome.Message = msgCommitInterrupted
		case errors.Is(err, items.ErrValidation):
			outcome.Status = OutcomeFailed
			outcome.Message = err.Error()
		default:
			outcome.Status = OutcomeFailed
			outcome.Message = msgCommitSaveFailed
		}
		return outcome
	}

	tracker.AddRow(input, row.Row)
	outcome.Status = OutcomeAdded
	outcome.ItemID = &item.ID
	return outcome
}

// validateCommitRows checks the request shape and returns the rows in row
// order. Row numbers are physical CSV lines, so the header is row 1.
func validateCommitRows(rows []ReviewedRow) ([]ReviewedRow, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("%w: select at least one row to import", ErrInvalidCommit)
	}
	if len(rows) > MaxImportRows {
		return nil, fmt.Errorf("%w: an import can include at most %d rows", ErrInvalidCommit, MaxImportRows)
	}
	sorted := slices.Clone(rows)
	slices.SortStableFunc(sorted, func(a, b ReviewedRow) int { return cmp.Compare(a.Row, b.Row) })
	for idx, row := range sorted {
		if row.Row < 2 {
			return nil, fmt.Errorf("%w: row %d is not a CSV data row", ErrInvalidCommit, row.Row)
		}
		if idx > 0 && sorted[idx-1].Row == row.Row {
			return nil, fmt.Errorf("%w: row %d is included more than once", ErrInvalidCommit, row.Row)
		}
	}
	return sorted, nil
}
