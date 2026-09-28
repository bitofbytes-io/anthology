package items

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound is returned when an item cannot be located.
var ErrNotFound = errors.New("item not found")

// ErrValidation is returned when input validation fails.
var ErrValidation = errors.New("validation error")

// ValidationError wraps a validation message so callers can distinguish
// client errors from internal failures.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

func (e *ValidationError) Unwrap() error {
	return ErrValidation
}

// ItemType enumerates the primary categories supported by the MVP.
type ItemType string

const (
	ItemTypeBook  ItemType = "book"
	ItemTypeGame  ItemType = "game"
	ItemTypeMovie ItemType = "movie"
	ItemTypeMusic ItemType = "music"
)

// BookStatus tracks a reader's engagement with a book.
type BookStatus string

const (
	// BookStatusNone represents an explicit "No status" selection.
	BookStatusNone       BookStatus = "none"
	BookStatusRead       BookStatus = "read"
	BookStatusReading    BookStatus = "reading"
	BookStatusWantToRead BookStatus = "want_to_read"
)

// Format describes the physical or digital format of an item.
// This is a generic type that can be extended for different item types.
type Format string

const (
	FormatUnknown   Format = "UNKNOWN"
	FormatHardcover Format = "HARDCOVER"
	FormatPaperback Format = "PAPERBACK"
	FormatSoftcover Format = "SOFTCOVER"
	FormatEbook     Format = "EBOOK"
	FormatMagazine  Format = "MAGAZINE"
)

// Genre represents a normalized top-level genre category.
type Genre string

const (
	GenreFiction           Genre = "FICTION"
	GenreNonFiction        Genre = "NON_FICTION"
	GenreScienceTech       Genre = "SCIENCE_TECH"
	GenreHistory           Genre = "HISTORY"
	GenreBiography         Genre = "BIOGRAPHY"
	GenreChildrens         Genre = "CHILDRENS"
	GenreArtsEntertainment Genre = "ARTS_ENTERTAINMENT"
	GenreReferenceOther    Genre = "REFERENCE_OTHER"
)

// Item represents a catalog entry in Anthology.
type Item struct {
	ID             uuid.UUID       `db:"id" json:"id"`
	OwnerID        uuid.UUID       `db:"owner_id" json:"-"`
	Title          string          `db:"title" json:"title"`
	Creator        string          `db:"creator" json:"creator"`
	ItemType       ItemType        `db:"item_type" json:"itemType"`
	ReleaseYear    *int            `db:"release_year" json:"releaseYear,omitempty"`
	PageCount      *int            `db:"page_count" json:"pageCount,omitempty"`
	CurrentPage    *int            `db:"current_page" json:"currentPage,omitempty"`
	ISBN13         string          `db:"isbn_13" json:"isbn13"`
	ISBN10         string          `db:"isbn_10" json:"isbn10"`
	Description    string          `db:"description" json:"description"`
	CoverImage     string          `db:"cover_image" json:"coverImage"`
	Format         Format          `db:"format" json:"format"`
	Genre          Genre           `db:"genre" json:"genre"`
	Rating         *int            `db:"rating" json:"rating,omitempty"`
	RetailPriceUsd *float64        `db:"retail_price_usd" json:"retailPriceUsd,omitempty"`
	GoogleVolumeId string          `db:"google_volume_id" json:"googleVolumeId"`
	Platform       string          `db:"platform" json:"platform"`
	AgeGroup       string          `db:"age_group" json:"ageGroup"`
	PlayerCount    string          `db:"player_count" json:"playerCount"`
	ReadingStatus  BookStatus      `db:"reading_status" json:"readingStatus"`
	ReadAt         *time.Time      `db:"read_at" json:"readAt,omitempty"`
	Notes          string          `db:"notes" json:"notes"`
	SeriesName     string          `db:"series_name" json:"seriesName"`
	VolumeNumber   *int            `db:"volume_number" json:"volumeNumber,omitempty"`
	TotalVolumes   *int            `db:"total_volumes" json:"totalVolumes,omitempty"`
	CreatedAt      time.Time       `db:"created_at" json:"createdAt"`
	UpdatedAt      time.Time       `db:"updated_at" json:"updatedAt"`
	ShelfPlacement *ShelfPlacement `db:"-" json:"shelfPlacement,omitempty"`
}

// ShelfPlacement summarizes where an item lives on a shelf layout.
type ShelfPlacement struct {
	ShelfID   uuid.UUID `json:"shelfId"`
	ShelfName string    `json:"shelfName"`
	SlotID    uuid.UUID `json:"slotId"`
	RowIndex  int       `json:"rowIndex"`
	ColIndex  int       `json:"colIndex"`
}

// CreateItemInput captures the data needed to create a new Item.
type CreateItemInput struct {
	OwnerID        uuid.UUID
	Title          string
	Creator        string
	ItemType       ItemType
	ReleaseYear    *int
	PageCount      *int
	CurrentPage    *int
	ISBN13         string
	ISBN10         string
	Description    string
	CoverImage     string
	Format         Format
	Genre          Genre
	Rating         *int
	RetailPriceUsd *float64
	GoogleVolumeId string
	Platform       string
	AgeGroup       string
	PlayerCount    string
	ReadingStatus  BookStatus
	ReadAt         *time.Time
	Notes          string
	SeriesName     string
	VolumeNumber   *int
	TotalVolumes   *int
	CreatedAt      *time.Time
	UpdatedAt      *time.Time
}

// UpdateItemInput captures the editable fields for an existing item.
type UpdateItemInput struct {
	Title          *string
	Creator        *string
	ItemType       *ItemType
	ReleaseYear    **int
	PageCount      **int
	CurrentPage    **int
	ISBN13         *string
	ISBN10         *string
	Description    *string
	CoverImage     *string
	Format         *Format
	Genre          *Genre
	Rating         **int
	RetailPriceUsd **float64
	GoogleVolumeId *string
	Platform       *string
	AgeGroup       *string
	PlayerCount    *string
	ReadingStatus  *BookStatus
	ReadAt         **time.Time
	Notes          *string
	SeriesName     *string
	VolumeNumber   **int
	TotalVolumes   **int
}

// ShelfStatus describes whether an item has been assigned to a shelf.
type ShelfStatus string

const (
	// ShelfStatusAll shows all items regardless of shelf assignment.
	ShelfStatusAll ShelfStatus = "all"
	// ShelfStatusOn shows only items with a shelf location.
	ShelfStatusOn ShelfStatus = "on"
	// ShelfStatusOff shows only items without a shelf location.
	ShelfStatusOff ShelfStatus = "off"
)

// ListOptions describes filters for listing items.
type ListOptions struct {
	OwnerID       uuid.UUID
	ItemType      *ItemType
	ReadingStatus *BookStatus
	ShelfStatus   *ShelfStatus
	Initial       *string
	Query         *string
	Limit         *int
}

// HistogramOptions describes filters for histogram aggregation.
type HistogramOptions struct {
	OwnerID       uuid.UUID
	ItemType      *ItemType
	ReadingStatus *BookStatus
}

// LetterHistogram maps first letters (A-Z, #) to item counts.
type LetterHistogram map[string]int

// DuplicateCheckInput captures the identifiers to check for duplicates.
type DuplicateCheckInput struct {
	Title  string
	ISBN13 string
	ISBN10 string
}

// DuplicateMatch represents a potential duplicate item found in the catalog.
type DuplicateMatch struct {
	ID                uuid.UUID `json:"id"`
	Title             string    `json:"title"`
	PrimaryIdentifier string    `json:"primaryIdentifier"`
	IdentifierType    string    `json:"identifierType"`
	CoverURL          string    `json:"coverUrl,omitempty"`
	Location          string    `json:"location,omitempty"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

// SeriesStatus indicates the completion status of a book series.
type SeriesStatus string

const (
	// SeriesStatusComplete indicates all known volumes are owned.
	SeriesStatusComplete SeriesStatus = "complete"
	// SeriesStatusIncomplete indicates some volumes are missing.
	SeriesStatusIncomplete SeriesStatus = "incomplete"
	// SeriesStatusUnknown indicates the total number of volumes is not known.
	SeriesStatusUnknown SeriesStatus = "unknown"
)

// SeriesSummary provides aggregated information about a book series.
type SeriesSummary struct {
	SeriesName     string       `json:"seriesName"`
	OwnedCount     int          `json:"ownedCount"`
	TotalVolumes   *int         `json:"totalVolumes,omitempty"`
	MissingCount   *int         `json:"missingCount,omitempty"`
	Status         SeriesStatus `json:"status"`
	Items          []Item       `json:"items,omitempty"`
	MissingVolumes []int        `json:"missingVolumes,omitempty"`
}

// SeriesListOptions describes service-level filters for listing series.
type SeriesListOptions struct {
	IncludeItems bool
	Status       *SeriesStatus
}

// SeriesRepoListOptions describes repository options for listing series.
// Repository-level options should not include derived fields such as SeriesStatus.
type SeriesRepoListOptions struct {
	IncludeItems bool
}

// SeriesListResponse contains series for the series view.
type SeriesListResponse struct {
	Series []SeriesSummary `json:"series"`
}

// Repository defines persistence operations for Items.
type Repository interface {
	Create(ctx context.Context, item Item) (Item, error)
	Get(ctx context.Context, id uuid.UUID, ownerID uuid.UUID) (Item, error)
	List(ctx context.Context, opts ListOptions) ([]Item, error)
	// ListByIDs returns the owner's items whose IDs are in ids. Unknown IDs and
	// items belonging to other owners are omitted.
	ListByIDs(ctx context.Context, ids []uuid.UUID, ownerID uuid.UUID) ([]Item, error)
	// FindByISBN returns the owner's most recently created item whose ISBN-13 or
	// ISBN-10 matches isbn after NormalizeIdentifier, or ErrNotFound.
	FindByISBN(ctx context.Context, isbn string, ownerID uuid.UUID) (Item, error)
	Update(ctx context.Context, item Item) (Item, error)
	Delete(ctx context.Context, id uuid.UUID, ownerID uuid.UUID) error
	Histogram(ctx context.Context, opts HistogramOptions) (LetterHistogram, error)
	FindDuplicates(ctx context.Context, input DuplicateCheckInput, ownerID uuid.UUID) ([]DuplicateMatch, error)
	ListSeries(ctx context.Context, opts SeriesRepoListOptions, ownerID uuid.UUID) ([]SeriesSummary, error)
	GetSeriesByName(ctx context.Context, name string, ownerID uuid.UUID) (SeriesSummary, error)
	ListSeriesNamesByNameCI(ctx context.Context, name string, ownerID uuid.UUID) ([]string, error)
	UpdateSeriesName(ctx context.Context, oldName, newName string, ownerID uuid.UUID) (int64, error)
	ClearSeriesName(ctx context.Context, seriesName string, ownerID uuid.UUID) (int64, error)
}
