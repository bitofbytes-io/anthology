package importer

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"anthology/internal/catalog"
	"anthology/internal/items"
)

// lookupAllowance tracks how many catalog lookups and how much lookup time
// remain for a single import.
type lookupAllowance struct {
	ctx       context.Context
	remaining int
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
