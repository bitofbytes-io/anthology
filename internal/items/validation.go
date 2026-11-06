package items

import (
	"encoding/base64"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"
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
