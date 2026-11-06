package items

import (
	"context"
	"errors"
	"fmt"
	"time"

	"anthology/internal/catalog"

	"github.com/google/uuid"
)

// ResyncMetadata refreshes an item's metadata from Google Books.
// Uses googleVolumeId if available, otherwise falls back to ISBN lookup.
// Only updates fields that Google provides (genre, retailPriceUsd, googleVolumeId).
// Does NOT overwrite user-entered fields like format and rating.
func (s *Service) ResyncMetadata(ctx context.Context, id uuid.UUID, ownerID uuid.UUID, catalogSvc *catalog.Service) (Item, error) {
	existing, err := s.repo.Get(ctx, id, ownerID)
	if err != nil {
		return Item{}, err
	}

	if existing.ItemType != ItemTypeBook {
		return Item{}, validationErr("re-sync is only available for books")
	}

	var metadata catalog.Metadata
	var lookupErr error

	// Prefer volume ID for precise lookup
	if existing.GoogleVolumeId != "" {
		metadata, lookupErr = catalogSvc.LookupByVolumeID(ctx, existing.GoogleVolumeId)
		if lookupErr != nil && !errors.Is(lookupErr, catalog.ErrNotFound) {
			return Item{}, fmt.Errorf("lookup by volume ID: %w", lookupErr)
		}
	}

	// Fall back to ISBN if volume ID lookup failed or wasn't available
	if metadata.Title == "" {
		query := existing.ISBN13
		if query == "" {
			query = existing.ISBN10
		}
		if query == "" {
			return Item{}, validationErr("no googleVolumeId or ISBN available for re-sync")
		}

		results, err := catalogSvc.Lookup(ctx, query, catalog.CategoryBook)
		if err != nil {
			if errors.Is(err, catalog.ErrNotFound) {
				return Item{}, validationErr("no metadata found for this item")
			}
			return Item{}, fmt.Errorf("lookup by ISBN: %w", err)
		}
		if len(results) == 0 {
			return Item{}, validationErr("no metadata found for this item")
		}
		metadata = results[0]
	}

	// Apply refreshed metadata - only update Google-provided fields
	if metadata.GoogleVolumeId != "" {
		existing.GoogleVolumeId = metadata.GoogleVolumeId
	}
	if metadata.Genre != "" {
		existing.Genre = normalizeGenre(Genre(metadata.Genre))
	}
	if metadata.RetailPriceUsd != nil {
		existing.RetailPriceUsd = metadata.RetailPriceUsd
	}

	// Also refresh standard fields if they were empty and Google provides them
	if existing.CoverImage == "" && metadata.CoverImage != "" {
		existing.CoverImage = metadata.CoverImage
	}
	if existing.Description == "" && metadata.Description != "" {
		existing.Description = metadata.Description
	}

	existing.UpdatedAt = time.Now().UTC()
	return s.repo.Update(ctx, existing)
}
