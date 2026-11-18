package importer

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"

	"anthology/internal/items"
)

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

func firstIdentifier(input items.CreateItemInput) string {
	if input.ISBN13 != "" {
		return input.ISBN13
	}
	if input.ISBN10 != "" {
		return input.ISBN10
	}
	return ""
}
