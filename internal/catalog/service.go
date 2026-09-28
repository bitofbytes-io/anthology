package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Category represents supported lookup categories.
type Category string

const (
	// CategoryBook resolves metadata for books via Google Books.
	CategoryBook Category = "book"
	// CategoryGame is reserved for future expansion.
	CategoryGame Category = "game"
	// CategoryMovie is reserved for future expansion.
	CategoryMovie Category = "movie"
	// CategoryMusic is reserved for future expansion.
	CategoryMusic Category = "music"
)

var (
	// ErrInvalidQuery is returned when the lookup query is empty or too short.
	ErrInvalidQuery = errors.New("query must be at least 3 characters")
	// ErrUnsupportedCategory is returned when the category is not implemented.
	ErrUnsupportedCategory = errors.New("unsupported lookup category")
	// ErrNotFound is returned when no metadata could be located for the query.
	ErrNotFound = errors.New("no metadata found for the supplied query")
)

// Metadata captures the subset of item fields populated by lookups.
type Metadata struct {
	Title          string   `json:"title"`
	Creator        string   `json:"creator"`
	ItemType       string   `json:"itemType"`
	ReleaseYear    *int     `json:"releaseYear,omitempty"`
	PageCount      *int     `json:"pageCount,omitempty"`
	ISBN13         string   `json:"isbn13"`
	ISBN10         string   `json:"isbn10"`
	Description    string   `json:"description"`
	CoverImage     string   `json:"coverImage"`
	Notes          string   `json:"notes"`
	Genre          string   `json:"genre,omitempty"`
	RetailPriceUsd *float64 `json:"retailPriceUsd,omitempty"`
	GoogleVolumeId string   `json:"googleVolumeId,omitempty"`
}

// Service performs metadata lookups against third-party catalog APIs.
type Service struct {
	client  *http.Client
	baseURL string
	apiKey  string
}

const defaultGoogleBooksURL = "https://www.googleapis.com/books/v1"

// Option configures the Service during construction.
type Option func(*Service)

// WithGoogleBooksBaseURL overrides the base URL for Google Books requests.
func WithGoogleBooksBaseURL(baseURL string) Option {
	return func(s *Service) {
		s.baseURL = strings.TrimRight(baseURL, "/")
	}
}

// WithGoogleBooksAPIKey configures the API key used for Google Books requests.
func WithGoogleBooksAPIKey(key string) Option {
	return func(s *Service) {
		s.apiKey = strings.TrimSpace(key)
	}
}

// NewService constructs a Service.
func NewService(client *http.Client, opts ...Option) *Service {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	// Copy the client so the redirect policy does not leak into callers.
	guarded := *client
	guarded.CheckRedirect = sameOriginHTTPSRedirects

	svc := &Service{
		client:  &guarded,
		baseURL: defaultGoogleBooksURL,
	}

	for _, opt := range opts {
		opt(svc)
	}

	return svc
}

// Lookup attempts to fetch metadata for the supplied query and category.
func (s *Service) Lookup(ctx context.Context, query string, category Category) ([]Metadata, error) {
	cleaned := strings.TrimSpace(query)
	if len(cleaned) < 3 {
		return nil, ErrInvalidQuery
	}

	switch category {
	case CategoryBook:
		return s.lookupBook(ctx, cleaned)
	case CategoryGame, CategoryMovie, CategoryMusic:
		return nil, ErrUnsupportedCategory
	default:
		return nil, ErrUnsupportedCategory
	}
}

type googleBooksResponse struct {
	Items []googleVolume `json:"items"`
}

type googleVolume struct {
	ID         string           `json:"id"`
	VolumeInfo googleVolumeInfo `json:"volumeInfo"`
	SaleInfo   googleSaleInfo   `json:"saleInfo"`
}

type googleVolumeInfo struct {
	Title               string                     `json:"title"`
	Subtitle            string                     `json:"subtitle"`
	Authors             []string                   `json:"authors"`
	Description         string                     `json:"description"`
	PublishedDate       string                     `json:"publishedDate"`
	PageCount           int                        `json:"pageCount"`
	Categories          []string                   `json:"categories"`
	IndustryIdentifiers []googleIndustryIdentifier `json:"industryIdentifiers"`
	ImageLinks          googleImageLinks           `json:"imageLinks"`
}

type googleSaleInfo struct {
	IsEbook     bool             `json:"isEbook"`
	RetailPrice *googlePriceInfo `json:"retailPrice"`
}

type googlePriceInfo struct {
	Amount       float64 `json:"amount"`
	CurrencyCode string  `json:"currencyCode"`
}

type googleIndustryIdentifier struct {
	Type       string `json:"type"`
	Identifier string `json:"identifier"`
}

type googleImageLinks struct {
	Thumbnail      string `json:"thumbnail"`
	SmallThumbnail string `json:"smallThumbnail"`
}

func (s *Service) lookupBook(ctx context.Context, query string) ([]Metadata, error) {
	if isbn := normalizeISBN(query); isbn != "" {
		metadata, err := s.lookupBookByISBN(ctx, isbn)
		if err == nil {
			return []Metadata{metadata}, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return nil, err
		}
	}

	// If the query looks like a barcode (digits/X only) but isn't a valid ISBN
	// length (10 or 13), return not found instead of doing a keyword search.
	if isInvalidBarcodeQuery(query) {
		return nil, ErrNotFound
	}

	return s.lookupBookByQuery(ctx, query)
}

func (s *Service) lookupBookByISBN(ctx context.Context, isbn string) (Metadata, error) {
	volumes, err := s.searchGoogleBooks(ctx, "isbn:"+isbn, 1)
	if err != nil {
		return Metadata{}, err
	}
	if len(volumes) == 0 {
		return Metadata{}, ErrNotFound
	}

	metadata, err := s.metadataFromVolume(volumes[0])
	if err != nil {
		return Metadata{}, err
	}

	if metadata.ISBN13 == "" && len(isbn) == 13 {
		metadata.ISBN13 = isbn
	}
	if metadata.ISBN10 == "" && len(isbn) == 10 {
		metadata.ISBN10 = isbn
	}

	return metadata, nil
}

func (s *Service) lookupBookByQuery(ctx context.Context, query string) ([]Metadata, error) {
	volumes, err := s.searchGoogleBooks(ctx, query, 5)
	if err != nil {
		return nil, err
	}

	if len(volumes) == 0 {
		return nil, ErrNotFound
	}

	results := make([]Metadata, 0, len(volumes))
	for _, volume := range volumes {
		metadata, err := s.metadataFromVolume(volume)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, err
		}
		results = append(results, metadata)
	}

	if len(results) == 0 {
		return nil, ErrNotFound
	}

	return results, nil
}

// LookupByVolumeID fetches metadata for a specific Google Books volume ID.
// This is used for re-syncing existing items with updated metadata.
func (s *Service) LookupByVolumeID(ctx context.Context, volumeID string) (Metadata, error) {
	if strings.TrimSpace(volumeID) == "" {
		return Metadata{}, ErrInvalidQuery
	}

	endpoint, err := url.Parse(s.baseURL + "/volumes/" + url.PathEscape(volumeID))
	if err != nil {
		return Metadata{}, fmt.Errorf("build google books url: %w", err)
	}

	resp, err := s.doGoogleBooksRequest(ctx, endpoint)
	if err != nil {
		return Metadata{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return Metadata{}, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return Metadata{}, fmt.Errorf("google books returned status %d", resp.StatusCode)
	}

	var volume googleVolume
	if err := json.NewDecoder(resp.Body).Decode(&volume); err != nil {
		return Metadata{}, fmt.Errorf("decode google books response: %w", err)
	}

	return s.metadataFromVolume(volume)
}

func (s *Service) searchGoogleBooks(ctx context.Context, q string, maxResults int) ([]googleVolume, error) {
	endpoint, err := url.Parse(s.baseURL + "/volumes")
	if err != nil {
		return nil, fmt.Errorf("build google books url: %w", err)
	}

	values := url.Values{}
	values.Set("q", q)
	if maxResults > 0 {
		values.Set("maxResults", strconv.Itoa(maxResults))
	}
	values.Set("printType", "books")
	values.Set("orderBy", "relevance")
	endpoint.RawQuery = values.Encode()

	resp, err := s.doGoogleBooksRequest(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google books returned status %d", resp.StatusCode)
	}

	var payload googleBooksResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode google books response: %w", err)
	}

	return payload.Items, nil
}

// sameOriginHTTPSRedirects only follows HTTPS redirects to the origin of the
// original request. net/http forwards custom headers such as X-Goog-Api-Key on
// redirects, so any other target could receive the API key.
func sameOriginHTTPSRedirects(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	origin := via[0].URL
	if req.URL.Scheme != "https" || req.URL.Scheme != origin.Scheme || req.URL.Host != origin.Host {
		return fmt.Errorf("refusing google books redirect to %s://%s", req.URL.Scheme, req.URL.Host)
	}
	return nil
}

// doGoogleBooksRequest issues a GET request to Google Books. The API key is sent
// in the X-Goog-Api-Key header rather than the query string so it never appears
// in request URLs, which net/http embeds in *url.Error messages.
func (s *Service) doGoogleBooksRequest(ctx context.Context, endpoint *url.URL) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create google books request: %w", err)
	}
	if s.apiKey != "" {
		req.Header.Set("X-Goog-Api-Key", s.apiKey)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		// Drop the *url.Error wrapper so the request URL (including the
		// caller's query) is not propagated into logs or API responses.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return nil, fmt.Errorf("call google books: %w", err)
	}
	return resp, nil
}

func (s *Service) metadataFromVolume(volume googleVolume) (Metadata, error) {
	info := volume.VolumeInfo
	title := strings.TrimSpace(info.Title)
	creator := strings.TrimSpace(strings.Join(info.Authors, ", "))

	if title == "" && creator == "" {
		return Metadata{}, ErrNotFound
	}

	isbn13, isbn10 := selectISBNs(flattenIdentifiers(info.IndustryIdentifiers))
	description := strings.TrimSpace(firstNonEmpty(info.Description, info.Subtitle))

	metadata := Metadata{
		Title:          title,
		Creator:        creator,
		ItemType:       "book",
		PageCount:      nil,
		ISBN13:         isbn13,
		ISBN10:         isbn10,
		Description:    description,
		CoverImage:     normalizeCoverURL(info.ImageLinks),
		Notes:          "",
		GoogleVolumeId: volume.ID,
		Genre:          MapCategoriesToGenre(info.Categories),
	}

	if info.PageCount > 0 {
		pages := info.PageCount
		metadata.PageCount = &pages
	}

	if year := parsePublishYear(info.PublishedDate); year != nil {
		metadata.ReleaseYear = year
	}

	// Extract retail price (USD only)
	if volume.SaleInfo.RetailPrice != nil && volume.SaleInfo.RetailPrice.CurrencyCode == "USD" {
		price := volume.SaleInfo.RetailPrice.Amount
		metadata.RetailPriceUsd = &price
	}

	return metadata, nil
}

func flattenIdentifiers(values []googleIndustryIdentifier) []string {
	candidates := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value.Identifier)
		if trimmed == "" {
			continue
		}
		candidates = append(candidates, trimmed)
	}
	return candidates
}

func normalizeCoverURL(links googleImageLinks) string {
	candidates := []string{links.Thumbnail, links.SmallThumbnail}
	for _, raw := range candidates {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "http://") {
			trimmed = "https://" + strings.TrimPrefix(trimmed, "http://")
		}
		return trimmed
	}
	return ""
}

func selectISBNs(values []string) (string, string) {
	var isbn13 string
	var isbn10 string
	for _, value := range values {
		clean := strings.TrimSpace(value)
		if len(clean) == 13 && isbn13 == "" {
			isbn13 = clean
			continue
		}
		if len(clean) == 10 && isbn10 == "" {
			isbn10 = clean
		}
	}
	return isbn13, isbn10
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

var publishYearPattern = regexp.MustCompile(`(1[0-9]{3}|20[0-9]{2})`)

func parsePublishYear(raw string) *int {
	match := publishYearPattern.FindString(raw)
	if match == "" {
		return nil
	}
	year := 0
	_, err := fmt.Sscanf(match, "%d", &year)
	if err != nil {
		return nil
	}
	return &year
}

// isInvalidBarcodeQuery returns true if the query looks like a barcode
// (digits with optional X for ISBN-10 check digit) but is not a valid
// ISBN length (10 or 13). This catches UPCs (12 digits) and other formats.
func isInvalidBarcodeQuery(query string) bool {
	charCount := 0
	for _, r := range query {
		if unicode.IsDigit(r) || r == 'X' || r == 'x' {
			charCount++
		} else if !unicode.IsSpace(r) && r != '-' {
			// Contains other characters - likely a title search
			return false
		}
	}

	// If it looks like a barcode (digits/X only), check length
	// Valid ISBNs are exactly 10 or 13 characters
	if charCount > 0 && charCount != 10 && charCount != 13 {
		return true
	}

	return false
}

func normalizeISBN(value string) string {
	cleaned := make([]rune, 0, len(value))
	for _, r := range value {
		if unicode.IsDigit(r) {
			cleaned = append(cleaned, r)
			continue
		}
		if r == 'X' || r == 'x' {
			cleaned = append(cleaned, 'X')
			continue
		}
	}

	switch len(cleaned) {
	case 10:
		for i := 0; i < 9; i++ {
			if !unicode.IsDigit(cleaned[i]) {
				return ""
			}
		}
		last := cleaned[9]
		if !unicode.IsDigit(last) && last != 'X' {
			return ""
		}
	case 13:
		for _, r := range cleaned {
			if !unicode.IsDigit(r) {
				return ""
			}
		}
	default:
		return ""
	}

	return string(cleaned)
}
