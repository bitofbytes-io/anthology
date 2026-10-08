package items

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
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

// isbnMatchKey keeps the digits and any X check digit of an ISBN, uppercased,
// so ISBN-10s ending in X compare correctly. The Postgres repository mirrors
// this by stripping [^0-9Xx] with regexp_replace and applying upper().
func isbnMatchKey(value string) string {
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == 'x' || r == 'X':
			builder.WriteByte('X')
		}
	}
	return builder.String()
}

// normalizeISBN returns the match key for a scanned ISBN, or "" unless it has
// the shape of an ISBN-13 (13 digits) or ISBN-10 (9 digits plus a digit or X).
func normalizeISBN(value string) string {
	key := isbnMatchKey(value)
	switch {
	case len(key) == 13 && !strings.Contains(key, "X"):
		return key
	case len(key) == 10 && !strings.Contains(key[:9], "X"):
		return key
	default:
		return ""
	}
}

// IsISBN reports whether value has the shape of an ISBN-10 or ISBN-13 once
// separators are removed.
func IsISBN(value string) bool {
	return normalizeISBN(value) != ""
}

// isbnLookupKeys returns the match keys a scanned ISBN may be stored under: its
// own normalized form and, when its check digit is valid, the equivalent
// ISBN-13 or ISBN-10, so a scan of either form finds a book saved with the
// other. Only 978-prefixed ISBN-13s have an ISBN-10 form. A code with a wrong
// check digit (a misread) matches only itself: converting it would recompute
// the check digit and could match a different, valid ISBN. It returns nil when
// value is not an ISBN.
func isbnLookupKeys(value string) []string {
	key := normalizeISBN(value)
	switch len(key) {
	case 10:
		if isbn13 := isbn10To13(key); isbn13To10(isbn13) == key {
			return []string{key, isbn13}
		}
		return []string{key}
	case 13:
		if !strings.HasPrefix(key, "978") {
			return []string{key}
		}
		if isbn10 := isbn13To10(key); isbn10To13(isbn10) == key {
			return []string{key, isbn10}
		}
		return []string{key}
	default:
		return nil
	}
}

// isbn10To13 converts a normalized ISBN-10 to its ISBN-13 form by prefixing 978
// and recomputing the check digit.
func isbn10To13(isbn10 string) string {
	body := "978" + isbn10[:9]
	sum := 0
	for i, r := range body {
		digit := int(r - '0')
		if i%2 == 1 {
			digit *= 3
		}
		sum += digit
	}
	return body + string(rune('0'+(10-sum%10)%10))
}

// isbn13To10 converts a normalized 978-prefixed ISBN-13 to its ISBN-10 form by
// dropping the prefix and recomputing the check digit.
func isbn13To10(isbn13 string) string {
	body := isbn13[3:12]
	sum := 0
	for i, r := range body {
		sum += (10 - i) * int(r-'0')
	}
	check := (11 - sum%11) % 11
	if check == 10 {
		return body + "X"
	}
	return body + string(rune('0'+check))
}

func validationErr(msg string) error {
	return &ValidationError{Message: msg}
}

func validateItemInput(title string, itemType ItemType) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return validationErr("title is required")
	}
	return validateItemType(itemType)
}

func validateItemType(itemType ItemType) error {
	switch itemType {
	case ItemTypeBook, ItemTypeGame, ItemTypeMovie, ItemTypeMusic:
		return nil
	case "":
		return validationErr("itemType is required")
	default:
		return validationErr("itemType must be one of book, game, movie, or music")
	}
}

// normalizeItem trims and validates an item's editable fields and clears the
// ones its item type does not use. Create runs it on the new item, and Update
// on the stored item with the patch applied, so both enforce the same rules.
// checkCover controls whether the cover image is sanitized and validated.
func normalizeItem(item *Item, checkCover bool) error {
	item.Title = strings.TrimSpace(item.Title)
	if err := validateItemInput(item.Title, item.ItemType); err != nil {
		return err
	}
	currentPage, err := normalizeCurrentPage(item.CurrentPage)
	if err != nil {
		return err
	}
	if price := item.RetailPriceUsd; price != nil && (math.IsNaN(*price) || math.IsInf(*price, 0)) {
		return validationErr("retailPriceUsd must be a finite number")
	}
	if checkCover {
		if item.CoverImage, err = sanitizeCoverImage(item.CoverImage); err != nil {
			return err
		}
	}

	item.Creator = strings.TrimSpace(item.Creator)
	item.Description = strings.TrimSpace(item.Description)
	item.Notes = strings.TrimSpace(item.Notes)
	item.ReleaseYear = normalizeYear(item.ReleaseYear)
	item.ISBN13, item.ISBN10, item.PageCount = normalizeBookIdentifiers(
		item.ItemType,
		strings.TrimSpace(item.ISBN13),
		strings.TrimSpace(item.ISBN10),
		normalizePositiveInt(item.PageCount),
	)
	item.Format, item.Genre, item.Rating, item.RetailPriceUsd, item.GoogleVolumeId = normalizeExtendedBookFields(
		item.ItemType,
		item.Format,
		item.Genre,
		item.Rating,
		item.RetailPriceUsd,
		item.GoogleVolumeId,
	)
	item.Platform, item.AgeGroup, item.PlayerCount = normalizeGameFields(
		item.ItemType,
		item.Platform,
		item.AgeGroup,
		item.PlayerCount,
	)

	if item.SeriesName, item.VolumeNumber, item.TotalVolumes, err = normalizeSeriesFields(
		item.ItemType,
		item.SeriesName,
		item.VolumeNumber,
		item.TotalVolumes,
	); err != nil {
		return err
	}

	item.ReadingStatus, item.ReadAt, item.CurrentPage, err = normalizeBookFields(
		item.ItemType,
		item.ReadingStatus,
		item.ReadAt,
		item.PageCount,
		currentPage,
	)
	return err
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

// normalizeBookIdentifiers clears ISBNs and page count for non-book items.
// The UI only exposes these fields in the book details section, and shelf
// barcode scans treat ISBNs as identifying books.
func normalizeBookIdentifiers(itemType ItemType, isbn13, isbn10 string, pageCount *int) (string, string, *int) {
	if itemType != ItemTypeBook {
		return "", "", nil
	}
	return isbn13, isbn10, pageCount
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
