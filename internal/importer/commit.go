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
	// OutcomeInterrupted marks a row whose save could not be confirmed (the
	// request ended or the connection failed mid-save); it may or may not
	// have been saved.
	OutcomeInterrupted OutcomeStatus = "interrupted"
	// OutcomeUnprocessed marks rows never attempted, because the request
	// ended or an earlier row's save could not be confirmed.
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
	msgCommitInterrupted  = "We couldn't confirm whether this row was saved, so it may or may not be in your library. Preview the file again to check: saved rows show as duplicates."
	msgCommitUnprocessed  = "Not attempted because the import ran out of time. Preview the file again to import the remaining rows."
	msgCommitAfterUnknown = "Not attempted because the previous row's save couldn't be confirmed. Preview the file again to import the remaining rows."
	msgCommitLockTimeout  = "Not attempted because another import or save for your library was still running when this request ran out of time. Preview the file again to import these rows."
	msgCommitSaveFailed   = "This row could not be saved."
)

// ErrReviewedImportUnavailable reports an item store that cannot save
// reviewed rows with an atomic duplicate check, so commits are refused rather
// than run with an unsafe check.
var ErrReviewedImportUnavailable = errors.New("reviewed CSV import is not available")

// batchStore starts an import batch: while it is open, no other item insert
// for the owner can happen, in this process or any other sharing the store.
type batchStore interface {
	BeginImportBatch(ctx context.Context, ownerID uuid.UUID) (*items.ImportBatch, error)
}

// Commit saves reviewed rows in row order. It never looks anything up in the
// catalog: each row is saved with exactly the item it carries, after the same
// validation items.Service.Create applies, owned by ownerID.
//
// Rows are saved inside an import batch, which waits for and then holds the
// owner's exclusive insert right (a PostgreSQL advisory lock shared by every
// API replica). Duplicates are checked against the library read once the
// batch holds that right, and against rows saved earlier in the request, so
// overlapping commits for the same owner cannot both save a row: the later
// one waits, then skips what the earlier one saved. Each row is still saved
// on its own, so rows saved before a later failure stay saved.
//
// When a row's save cannot be confirmed either way, it is reported as
// interrupted and no further rows are attempted.
func (i *CSVImporter) Commit(ctx context.Context, request CommitRequest, ownerID uuid.UUID) (CommitResult, error) {
	if i.items == nil {
		return CommitResult{}, fmt.Errorf("%w: item store is not configured", ErrInvalidCommit)
	}
	store, ok := i.items.(batchStore)
	if !ok {
		return CommitResult{}, ErrReviewedImportUnavailable
	}
	rows, err := validateCommitRows(request.Rows)
	if err != nil {
		return CommitResult{}, err
	}

	result := CommitResult{Rows: make([]RowOutcome, 0, len(rows))}
	batch, err := store.BeginImportBatch(ctx, ownerID)
	if err != nil {
		if errors.Is(err, items.ErrExclusiveInsertsUnsupported) {
			return CommitResult{}, ErrReviewedImportUnavailable
		}
		if ctx.Err() != nil {
			// Still waiting for the owner's insert right: nothing was saved.
			result.addUnprocessed(rows, msgCommitLockTimeout)
			return result, nil
		}
		return CommitResult{}, err
	}
	defer batch.Close()

	existing, err := batch.Existing(ctx)
	if err != nil {
		if ctx.Err() != nil {
			result.addUnprocessed(rows, msgCommitUnprocessed)
			return result, nil
		}
		return CommitResult{}, err
	}
	tracker := newDuplicateTracker(existing)

	for idx, row := range rows {
		if ctx.Err() != nil {
			result.addUnprocessed(rows[idx:], msgCommitUnprocessed)
			break
		}
		outcome := commitRow(ctx, batch, tracker, row, ownerID)
		result.add(outcome)
		if outcome.Status == OutcomeInterrupted {
			message := msgCommitAfterUnknown
			if ctx.Err() != nil {
				message = msgCommitUnprocessed
			}
			result.addUnprocessed(rows[idx+1:], message)
			break
		}
	}
	return result, nil
}

func (r *CommitResult) addUnprocessed(rows []ReviewedRow, message string) {
	for _, row := range rows {
		r.add(RowOutcome{
			Row:        row.Row,
			Status:     OutcomeUnprocessed,
			Title:      row.Item.Title,
			Identifier: firstIdentifier(row.Item.CreateItemInput),
			Message:    message,
		})
	}
}

func commitRow(ctx context.Context, batch *items.ImportBatch, tracker *duplicateTracker, row ReviewedRow, ownerID uuid.UUID) RowOutcome {
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

	item, err := batch.Create(ctx, input)
	if err != nil {
		switch {
		case errors.Is(err, items.ErrSaveOutcomeUnknown) || ctx.Err() != nil:
			outcome.Status = OutcomeInterrupted
			outcome.Message = msgCommitInterrupted
		case errors.Is(err, items.ErrValidation):
			outcome.Status = OutcomeFailed
			outcome.Message = err.Error()
		default:
			// The store confirmed nothing was saved.
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
