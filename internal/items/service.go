package items

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
)

// Service orchestrates validation and persistence for items.
type Service struct {
	repo Repository
}

// NewService wires a Service with the provided repository.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Create validates and persists a new item.
func (s *Service) Create(ctx context.Context, input CreateItemInput) (Item, error) {
	if input.OwnerID == (uuid.UUID{}) {
		return Item{}, validationErr("ownerID is required")
	}

	item, err := newItem(input)
	if err != nil {
		return Item{}, err
	}
	item.ID = uuid.New()
	stampNewItem(&item, input)

	return s.repo.Create(ctx, item)
}

// NormalizeCreateInput applies the validation and normalization Create would
// apply to input, without persisting anything, and returns the input Create
// would store. Normalizing the result again returns it unchanged. OwnerID and
// the timestamps are passed through as given.
func NormalizeCreateInput(input CreateItemInput) (CreateItemInput, error) {
	item, err := newItem(input)
	if err != nil {
		return CreateItemInput{}, err
	}
	return CreateItemInput{
		OwnerID:        input.OwnerID,
		Title:          item.Title,
		Creator:        item.Creator,
		ItemType:       item.ItemType,
		ReleaseYear:    item.ReleaseYear,
		PageCount:      item.PageCount,
		CurrentPage:    item.CurrentPage,
		ISBN13:         item.ISBN13,
		ISBN10:         item.ISBN10,
		Description:    item.Description,
		CoverImage:     item.CoverImage,
		Format:         item.Format,
		Genre:          item.Genre,
		Rating:         item.Rating,
		RetailPriceUsd: item.RetailPriceUsd,
		GoogleVolumeId: item.GoogleVolumeId,
		Platform:       item.Platform,
		AgeGroup:       item.AgeGroup,
		PlayerCount:    item.PlayerCount,
		ReadingStatus:  item.ReadingStatus,
		ReadAt:         item.ReadAt,
		Notes:          item.Notes,
		SeriesName:     item.SeriesName,
		VolumeNumber:   item.VolumeNumber,
		TotalVolumes:   item.TotalVolumes,
		CreatedAt:      input.CreatedAt,
		UpdatedAt:      input.UpdatedAt,
	}, nil
}

// newItem builds the item Create stores for input, normalized and validated,
// leaving the ID and timestamps unset.
func newItem(input CreateItemInput) (Item, error) {
	item := Item{
		OwnerID:        input.OwnerID,
		Title:          input.Title,
		Creator:        input.Creator,
		ItemType:       input.ItemType,
		ReleaseYear:    input.ReleaseYear,
		PageCount:      input.PageCount,
		CurrentPage:    input.CurrentPage,
		ISBN13:         input.ISBN13,
		ISBN10:         input.ISBN10,
		Description:    input.Description,
		CoverImage:     input.CoverImage,
		Format:         input.Format,
		Genre:          input.Genre,
		Rating:         input.Rating,
		RetailPriceUsd: input.RetailPriceUsd,
		GoogleVolumeId: input.GoogleVolumeId,
		Platform:       input.Platform,
		AgeGroup:       input.AgeGroup,
		PlayerCount:    input.PlayerCount,
		ReadingStatus:  input.ReadingStatus,
		ReadAt:         input.ReadAt,
		Notes:          input.Notes,
		SeriesName:     input.SeriesName,
		VolumeNumber:   input.VolumeNumber,
		TotalVolumes:   input.TotalVolumes,
	}
	if err := normalizeItem(&item, true); err != nil {
		return Item{}, err
	}
	return item, nil
}

// List returns catalogued items ordered by creation date descending.
func (s *Service) List(ctx context.Context, opts ListOptions) ([]Item, error) {
	items, err := s.repo.List(ctx, opts)
	if err != nil {
		return nil, err
	}

	slices.SortFunc(items, compareItemsByCreatedDesc)

	if opts.Limit != nil && *opts.Limit >= 0 && len(items) > *opts.Limit {
		items = items[:*opts.Limit]
	}

	return items, nil
}

// Get retrieves an item by ID and owner.
func (s *Service) Get(ctx context.Context, id uuid.UUID, ownerID uuid.UUID) (Item, error) {
	return s.repo.Get(ctx, id, ownerID)
}

// Update applies patch to the stored item, then normalizes and validates the
// result the same way Create does. The cover image is checked only when the
// patch sets it: rows saved before the current cover rules (such as HTTPS
// only) may hold a cover those rules reject, and that must not block an edit
// that leaves the cover alone.
func (s *Service) Update(ctx context.Context, id uuid.UUID, ownerID uuid.UUID, patch ItemPatch) (Item, error) {
	item, err := s.repo.Get(ctx, id, ownerID)
	if err != nil {
		return Item{}, err
	}
	if err := patch.applyTo(&item); err != nil {
		return Item{}, err
	}
	if err := normalizeItem(&item, patch.sets("coverImage")); err != nil {
		return Item{}, err
	}
	item.UpdatedAt = time.Now().UTC()
	return s.repo.Update(ctx, item)
}

// Delete removes an item by ID and owner.
func (s *Service) Delete(ctx context.Context, id uuid.UUID, ownerID uuid.UUID) error {
	return s.repo.Delete(ctx, id, ownerID)
}

// Histogram returns a count of items grouped by first letter of title.
func (s *Service) Histogram(ctx context.Context, opts HistogramOptions) (LetterHistogram, int, error) {
	histogram, err := s.repo.Histogram(ctx, opts)
	if err != nil {
		return nil, 0, err
	}

	total := 0
	for _, count := range histogram {
		total += count
	}

	return histogram, total, nil
}

// FindDuplicates searches for items matching the given title or identifiers.
// Title matching is case-insensitive with whitespace trimmed.
// Identifier matching strips non-digit characters for normalization.
func (s *Service) FindDuplicates(ctx context.Context, input DuplicateCheckInput, ownerID uuid.UUID) ([]DuplicateMatch, error) {
	return s.repo.FindDuplicates(ctx, input, ownerID)
}
