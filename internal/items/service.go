package items

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"anthology/internal/catalog"

	"github.com/google/uuid"
)

const (
	maxCoverImageBytes     = 500 * 1024
	maxCoverImageURLLength = 4096
)

// allowedImageMIMETypes lists permitted MIME types for data URI images.
var allowedImageMIMETypes = map[string]bool{
	"image/jpeg":    true,
	"image/png":     true,
	"image/gif":     true,
	"image/webp":    true,
	"image/svg+xml": true,
}

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
	if err := validateItemInput(input.Title, input.ItemType); err != nil {
		return Item{}, err
	}

	pageCount := normalizePositiveInt(input.PageCount)
	currentPage, err := normalizeCurrentPage(input.CurrentPage)
	if err != nil {
		return Item{}, err
	}

	readingStatus, readAt, normalizedCurrentPage, err := normalizeBookFields(input.ItemType, input.ReadingStatus, input.ReadAt, pageCount, currentPage)
	if err != nil {
		return Item{}, err
	}

	coverImage, err := sanitizeCoverImage(input.CoverImage)
	if err != nil {
		return Item{}, err
	}

	// Normalize book-specific extended fields
	format, genre, rating, retailPriceUsd, googleVolumeId := normalizeExtendedBookFields(
		input.ItemType,
		input.Format,
		input.Genre,
		input.Rating,
		input.RetailPriceUsd,
		input.GoogleVolumeId,
	)

	// Clear game-specific fields for non-game items
	platform, ageGroup, playerCount := normalizeGameFields(
		input.ItemType,
		input.Platform,
		input.AgeGroup,
		input.PlayerCount,
	)

	// Normalize series fields (books only)
	seriesName, volumeNumber, totalVolumes, err := normalizeSeriesFields(
		input.ItemType,
		input.SeriesName,
		input.VolumeNumber,
		input.TotalVolumes,
	)
	if err != nil {
		return Item{}, err
	}

	now := time.Now().UTC()
	createdAt := now
	if input.CreatedAt != nil && !input.CreatedAt.IsZero() {
		createdAt = input.CreatedAt.UTC()
	}
	updatedAt := createdAt
	if input.UpdatedAt != nil && !input.UpdatedAt.IsZero() {
		updatedAt = input.UpdatedAt.UTC()
	}
	item := Item{
		ID:             uuid.New(),
		OwnerID:        input.OwnerID,
		Title:          strings.TrimSpace(input.Title),
		Creator:        strings.TrimSpace(input.Creator),
		ItemType:       input.ItemType,
		ReleaseYear:    normalizeYear(input.ReleaseYear),
		PageCount:      pageCount,
		CurrentPage:    normalizedCurrentPage,
		ISBN13:         strings.TrimSpace(input.ISBN13),
		ISBN10:         strings.TrimSpace(input.ISBN10),
		Description:    strings.TrimSpace(input.Description),
		CoverImage:     coverImage,
		Format:         format,
		Genre:          genre,
		Rating:         rating,
		RetailPriceUsd: retailPriceUsd,
		GoogleVolumeId: googleVolumeId,
		Platform:       platform,
		AgeGroup:       ageGroup,
		PlayerCount:    playerCount,
		ReadingStatus:  readingStatus,
		ReadAt:         readAt,
		Notes:          strings.TrimSpace(input.Notes),
		SeriesName:     seriesName,
		VolumeNumber:   volumeNumber,
		TotalVolumes:   totalVolumes,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}

	return s.repo.Create(ctx, item)
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

// Update applies modifications to an item.
func (s *Service) Update(ctx context.Context, id uuid.UUID, ownerID uuid.UUID, input UpdateItemInput) (Item, error) {
	existing, err := s.repo.Get(ctx, id, ownerID)
	if err != nil {
		return Item{}, err
	}

	if input.Title != nil {
		title := strings.TrimSpace(*input.Title)
		if title == "" {
			return Item{}, validationErr("title is required")
		}
		existing.Title = title
	}

	if input.ItemType != nil {
		if *input.ItemType == "" {
			return Item{}, validationErr("itemType is required")
		}
		existing.ItemType = *input.ItemType
	}

	if input.Creator != nil {
		existing.Creator = strings.TrimSpace(*input.Creator)
	}

	if input.ReleaseYear != nil {
		existing.ReleaseYear = normalizeYear(*input.ReleaseYear)
	}

	if input.PageCount != nil {
		existing.PageCount = normalizePositiveInt(*input.PageCount)
	}

	if input.CurrentPage != nil {
		value, err := normalizeCurrentPage(*input.CurrentPage)
		if err != nil {
			return Item{}, err
		}
		existing.CurrentPage = value
	}

	if input.Notes != nil {
		existing.Notes = strings.TrimSpace(*input.Notes)
	}

	if input.ISBN13 != nil {
		existing.ISBN13 = strings.TrimSpace(*input.ISBN13)
	}

	if input.ISBN10 != nil {
		existing.ISBN10 = strings.TrimSpace(*input.ISBN10)
	}

	if input.Description != nil {
		existing.Description = strings.TrimSpace(*input.Description)
	}

	if input.CoverImage != nil {
		coverImage, err := sanitizeCoverImage(*input.CoverImage)
		if err != nil {
			return Item{}, err
		}
		existing.CoverImage = coverImage
	}

	if input.Platform != nil {
		existing.Platform = strings.TrimSpace(*input.Platform)
	}

	if input.AgeGroup != nil {
		existing.AgeGroup = strings.TrimSpace(*input.AgeGroup)
	}

	if input.PlayerCount != nil {
		existing.PlayerCount = strings.TrimSpace(*input.PlayerCount)
	}

	if input.Format != nil {
		existing.Format = normalizeFormat(*input.Format)
	}

	if input.Genre != nil {
		existing.Genre = normalizeGenre(*input.Genre)
	}

	if input.Rating != nil {
		existing.Rating = normalizeRating(*input.Rating)
	}

	if input.RetailPriceUsd != nil {
		existing.RetailPriceUsd = normalizePrice(*input.RetailPriceUsd)
	}

	if input.GoogleVolumeId != nil {
		existing.GoogleVolumeId = strings.TrimSpace(*input.GoogleVolumeId)
	}

	// Handle series fields
	if input.SeriesName != nil {
		existing.SeriesName = strings.TrimSpace(*input.SeriesName)
	}
	if input.VolumeNumber != nil {
		existing.VolumeNumber = normalizePositiveIntPtrPtr(input.VolumeNumber)
	}
	if input.TotalVolumes != nil {
		existing.TotalVolumes = normalizePositiveIntPtrPtr(input.TotalVolumes)
	}

	// Validate series fields after update

	seriesName, volumeNumber, totalVolumes, err := normalizeSeriesFields(
		existing.ItemType,
		existing.SeriesName,
		existing.VolumeNumber,
		existing.TotalVolumes,
	)
	if err != nil {
		return Item{}, err
	}
	existing.SeriesName = seriesName
	existing.VolumeNumber = volumeNumber
	existing.TotalVolumes = totalVolumes

	readingStatus := existing.ReadingStatus
	if input.ReadingStatus != nil {
		readingStatus = *input.ReadingStatus
	}

	readAt := existing.ReadAt
	if input.ReadAt != nil {
		readAt = *input.ReadAt
	}

	normalizedStatus, normalizedReadAt, normalizedCurrentPage, err := normalizeBookFields(existing.ItemType, readingStatus, readAt, existing.PageCount, existing.CurrentPage)
	if err != nil {
		return Item{}, err
	}

	existing.ReadingStatus = normalizedStatus
	existing.ReadAt = normalizedReadAt
	existing.CurrentPage = normalizedCurrentPage
	existing.UpdatedAt = time.Now().UTC()
	return s.repo.Update(ctx, existing)
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

// ListSeries returns all series with aggregated statistics and missing volume detection.
func (s *Service) ListSeries(ctx context.Context, opts SeriesListOptions, ownerID uuid.UUID) (SeriesListResponse, error) {
	// Always include items for missing volume calculation.
	repoOpts := SeriesRepoListOptions{
		IncludeItems: true,
	}

	summaries, err := s.repo.ListSeries(ctx, repoOpts, ownerID)
	if err != nil {
		return SeriesListResponse{}, err
	}

	// Enrich each series with missing volume detection
	for i := range summaries {
		summaries[i] = s.enrichSeriesSummary(summaries[i])
		// Clear items if not requested
		if !opts.IncludeItems {
			summaries[i].Items = nil
		}
	}

	// Filter by status if requested
	if opts.Status != nil {
		filtered := make([]SeriesSummary, 0)
		for _, summary := range summaries {
			if summary.Status == *opts.Status {
				filtered = append(filtered, summary)
			}
		}
		summaries = filtered
	}

	return SeriesListResponse{
		Series: summaries,
	}, nil
}

// GetSeriesByName returns detailed info about a single series with missing volume detection.
func (s *Service) GetSeriesByName(ctx context.Context, name string, ownerID uuid.UUID) (SeriesSummary, error) {
	summary, err := s.repo.GetSeriesByName(ctx, name, ownerID)
	if err != nil {
		return SeriesSummary{}, err
	}
	return s.enrichSeriesSummary(summary), nil
}

// UpdateSeriesName renames a series by updating series_name on all matching items.
// Returns the updated series summary.
func (s *Service) UpdateSeriesName(ctx context.Context, oldName, newName string, ownerID uuid.UUID) (SeriesSummary, error) {
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)

	if oldName == "" {
		return SeriesSummary{}, validationErr("series name is required")
	}
	if newName == "" {
		return SeriesSummary{}, validationErr("new series name is required")
	}
	if len(newName) > 200 {
		return SeriesSummary{}, validationErr("series name must be 200 characters or less")
	}

	// Verify series exists
	_, err := s.repo.GetSeriesByName(ctx, oldName, ownerID)
	if err != nil {
		return SeriesSummary{}, err
	}

	// Check if target name already exists (case-insensitive).
	existingNames, err := s.repo.ListSeriesNamesByNameCI(ctx, newName, ownerID)
	if err != nil {
		return SeriesSummary{}, err
	}
	for _, seriesName := range existingNames {
		if seriesName != oldName {
			return SeriesSummary{}, validationErr("a series with this name already exists")
		}
	}

	_, err = s.repo.UpdateSeriesName(ctx, oldName, newName, ownerID)
	if err != nil {
		return SeriesSummary{}, err
	}

	// Return the updated series
	return s.GetSeriesByName(ctx, newName, ownerID)
}

// DeleteSeries removes series association from all items in a series.
// Returns the count of affected items.
func (s *Service) DeleteSeries(ctx context.Context, seriesName string, ownerID uuid.UUID) (int64, error) {
	seriesName = strings.TrimSpace(seriesName)
	if seriesName == "" {
		return 0, validationErr("series name is required")
	}

	// Verify series exists
	_, err := s.repo.GetSeriesByName(ctx, seriesName, ownerID)
	if err != nil {
		return 0, err
	}

	return s.repo.ClearSeriesName(ctx, seriesName, ownerID)
}

// enrichSeriesSummary calculates missing volumes and status for a series.
func (s *Service) enrichSeriesSummary(summary SeriesSummary) SeriesSummary {
	summary.MissingVolumes, summary.MissingCount = s.detectMissingVolumes(summary)
	summary.Status = s.determineSeriesStatus(summary)

	return summary
}

const maxSeriesVolumes = 200

// detectMissingVolumes returns an exact count and at most 200 missing volume numbers.
// Work depends on the owned items and output limit, never the highest volume number.
func (s *Service) detectMissingVolumes(summary SeriesSummary) ([]int, *int) {
	ownedSet := make(map[int]struct{})
	upperBound := 0
	for _, item := range summary.Items {
		if item.VolumeNumber != nil && *item.VolumeNumber > 0 {
			ownedSet[*item.VolumeNumber] = struct{}{}
			upperBound = max(upperBound, *item.VolumeNumber)
		}
	}
	if len(ownedSet) == 0 {
		// Preserve count-based estimates when no volume numbers are available.
		if summary.TotalVolumes == nil {
			return nil, nil
		}
		count := max(0, *summary.TotalVolumes-summary.OwnedCount)
		return nil, &count
	}
	if summary.TotalVolumes != nil {
		upperBound = max(upperBound, *summary.TotalVolumes)
	}
	count := upperBound - len(ownedSet)
	owned := make([]int, 0, len(ownedSet))
	for volume := range ownedSet {
		owned = append(owned, volume)
	}
	slices.Sort(owned)
	missing := make([]int, 0, min(count, maxSeriesVolumes))
	appendGap := func(first, last int) {
		length := min(last-first+1, maxSeriesVolumes-len(missing))
		for offset := 0; offset < length; offset++ {
			missing = append(missing, first+offset)
		}
	}
	next := 1
	for _, volume := range owned {
		appendGap(next, volume-1)
		if len(missing) == maxSeriesVolumes || volume == upperBound {
			return missing, &count
		}
		next = volume + 1
	}
	appendGap(next, upperBound)
	return missing, &count
}

// determineSeriesStatus calculates the completion status of a series.
func (s *Service) determineSeriesStatus(summary SeriesSummary) SeriesStatus {
	if summary.TotalVolumes != nil {
		// Total is known - we can determine complete vs incomplete
		if summary.MissingCount != nil && *summary.MissingCount > 0 {
			return SeriesStatusIncomplete
		}
		return SeriesStatusComplete
	}

	// Total is unknown
	if summary.MissingCount != nil && *summary.MissingCount > 0 {
		// We found gaps via heuristic - incomplete but inferred
		return SeriesStatusIncomplete
	}

	// No gaps found and no total known - status is unknown
	return SeriesStatusUnknown
}

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

// NormalizeTitle prepares a title for duplicate comparison by lowercasing and trimming whitespace.
func NormalizeTitle(title string) string {
	return strings.ToLower(strings.TrimSpace(title))
}

// NormalizeIdentifier strips all non-digit characters from an identifier (ISBN, UPC, EAN).
func NormalizeIdentifier(value string) string {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return ""
	}
	var builder strings.Builder
	for _, r := range cleaned {
		if r >= '0' && r <= '9' {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func validationErr(msg string) error {
	return &ValidationError{Message: msg}
}

func validateItemInput(title string, itemType ItemType) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return validationErr("title is required")
	}
	if itemType == "" {
		return validationErr("itemType is required")
	}
	return nil
}

func compareItemsByCreatedDesc(a, b Item) int {
	if a.CreatedAt.Equal(b.CreatedAt) {
		return strings.Compare(a.Title, b.Title)
	}
	if a.CreatedAt.After(b.CreatedAt) {
		return -1
	}
	return 1
}

func normalizeYear(year *int) *int {
	if year == nil {
		return nil
	}
	if *year < 0 {
		return nil
	}
	value := *year
	return &value
}

func normalizePositiveInt(value *int) *int {
	if value == nil {
		return nil
	}
	if *value <= 0 {
		return nil
	}
	v := *value
	return &v
}

func normalizePositiveIntPtrPtr(value **int) *int {
	if value == nil {
		return nil
	}
	if *value == nil {
		return nil
	}
	return normalizePositiveInt(*value)
}

func normalizeCurrentPage(value *int) (*int, error) {
	if value == nil {
		return nil, nil
	}
	v := *value
	if v < 0 {
		return nil, validationErr("currentPage must be zero or greater")
	}
	return &v, nil
}

func sanitizeCoverImage(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}

	if strings.HasPrefix(trimmed, "data:") {
		parts := strings.SplitN(trimmed, ",", 2)
		if len(parts) != 2 {
			return "", validationErr("coverImage data URI is invalid")
		}

		// Extract and validate MIME type from the data URI header (e.g., "data:image/png;base64")
		header := parts[0]
		mimeType := strings.TrimPrefix(header, "data:")
		mimeType = strings.TrimSuffix(mimeType, ";base64")
		mimeType = strings.ToLower(mimeType)
		if !allowedImageMIMETypes[mimeType] {
			return "", validationErr("coverImage must be a valid image type (JPEG, PNG, GIF, WebP, or SVG)")
		}

		if _, err := base64.StdEncoding.DecodeString(parts[1]); err != nil {
			return "", validationErr("coverImage must contain valid base64 image data")
		}

		estimatedBytes := len(parts[1]) * 3 / 4
		if estimatedBytes > maxCoverImageBytes {
			return "", validationErr(fmt.Sprintf("coverImage must be smaller than %dKB", maxCoverImageBytes/1024))
		}

		return trimmed, nil
	}

	if len(trimmed) > maxCoverImageURLLength {
		return "", validationErr(fmt.Sprintf("coverImage must be shorter than %d characters", maxCoverImageURLLength))
	}

	// Validate external URL: must be valid URL with https scheme
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", validationErr("coverImage must be a valid URL")
	}
	if parsed.Scheme != "https" {
		return "", validationErr("coverImage URL must use HTTPS")
	}
	if parsed.Host == "" {
		return "", validationErr("coverImage URL must have a valid host")
	}

	return trimmed, nil
}

func normalizeBookFields(itemType ItemType, status BookStatus, readAt *time.Time, pageCount *int, currentPage *int) (BookStatus, *time.Time, *int, error) {
	if status == "" {
		status = BookStatusNone
	}

	if itemType != ItemTypeBook {
		return BookStatusNone, nil, nil, nil
	}

	switch status {
	case BookStatusNone:
		return BookStatusNone, nil, nil, nil
	case BookStatusWantToRead:
		return BookStatusWantToRead, nil, nil, nil
	case BookStatusRead:
		if readAt == nil || readAt.IsZero() {
			return BookStatusNone, nil, nil, validationErr("readAt is required when readingStatus is read")
		}

		normalized := readAt.UTC()
		return status, &normalized, nil, nil
	case BookStatusReading:
		normalizedPage, err := normalizeReadingProgress(currentPage, pageCount)
		if err != nil {
			return BookStatusNone, nil, nil, err
		}
		return status, nil, normalizedPage, nil
	default:
		return BookStatusNone, nil, nil, validationErr("readingStatus must be one of none, read, reading, or want_to_read")
	}
}

func normalizeReadingProgress(currentPage *int, pageCount *int) (*int, error) {
	if currentPage == nil {
		return nil, nil
	}
	if pageCount != nil && *currentPage > *pageCount {
		return nil, validationErr("currentPage cannot exceed pageCount")
	}
	return currentPage, nil
}

// normalizeExtendedBookFields normalizes book-specific extended metadata fields.
// For non-book items, all fields are cleared/zeroed.
func normalizeExtendedBookFields(itemType ItemType, format Format, genre Genre, rating *int, price *float64, volumeId string) (Format, Genre, *int, *float64, string) {
	if itemType != ItemTypeBook {
		return "", "", nil, nil, ""
	}
	return normalizeFormat(format), normalizeGenre(genre), normalizeRating(rating), normalizePrice(price), strings.TrimSpace(volumeId)
}

// normalizeGameFields normalizes game-specific fields.
// For non-game items, all fields are cleared.
func normalizeGameFields(itemType ItemType, platform, ageGroup, playerCount string) (string, string, string) {
	if itemType != ItemTypeGame {
		return "", "", ""
	}
	return strings.TrimSpace(platform), strings.TrimSpace(ageGroup), strings.TrimSpace(playerCount)
}

// normalizeSeriesFields validates and normalizes series-specific fields.
// For non-book items, all series fields are cleared.
// Validates that volumeNumber does not exceed totalVolumes if both are set.
func normalizeSeriesFields(itemType ItemType, seriesName string, volumeNumber *int, totalVolumes *int) (string, *int, *int, error) {
	if itemType != ItemTypeBook {
		return "", nil, nil, nil
	}

	if volumeNumber != nil && *volumeNumber > maxSeriesVolumes {
		return "", nil, nil, validationErr("volumeNumber cannot exceed 200")
	}
	if totalVolumes != nil && *totalVolumes > maxSeriesVolumes {
		return "", nil, nil, validationErr("totalVolumes cannot exceed 200")
	}

	name := strings.TrimSpace(seriesName)
	vol := normalizePositiveInt(volumeNumber)
	total := normalizePositiveInt(totalVolumes)

	// Validate volume doesn't exceed total
	if vol != nil && total != nil && *vol > *total {
		return "", nil, nil, validationErr("volumeNumber cannot exceed totalVolumes")
	}

	return name, vol, total, nil
}

// normalizeFormat validates and normalizes the format enum.
func normalizeFormat(format Format) Format {
	switch format {
	case FormatHardcover, FormatPaperback, FormatSoftcover, FormatEbook, FormatMagazine:
		return format
	default:
		return FormatUnknown
	}
}

// normalizeGenre validates and normalizes the genre enum.
func normalizeGenre(genre Genre) Genre {
	switch genre {
	case GenreFiction, GenreNonFiction, GenreScienceTech, GenreHistory,
		GenreBiography, GenreChildrens, GenreArtsEntertainment, GenreReferenceOther:
		return genre
	default:
		return ""
	}
}

// normalizeRating ensures rating is within valid range (1-10).
func normalizeRating(rating *int) *int {
	if rating == nil {
		return nil
	}
	if *rating < 1 || *rating > 10 {
		return nil
	}
	v := *rating
	return &v
}

// normalizePrice ensures price is non-negative.
func normalizePrice(price *float64) *float64 {
	if price == nil {
		return nil
	}
	if *price < 0 {
		return nil
	}
	v := *price
	return &v
}
