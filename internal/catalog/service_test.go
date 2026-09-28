package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestLookupBookByQueryReturnsMetadata(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/volumes" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		values := r.URL.Query()
		if got := values.Get("q"); got != "example keywords" {
			t.Fatalf("expected q=example keywords, got %s", got)
		}
		if got := values.Get("maxResults"); got != "5" {
			t.Fatalf("expected maxResults=5, got %s", got)
		}
		if values.Has("key") {
			t.Fatalf("expected API key to be omitted from the query string, got %s", r.URL.RawQuery)
		}
		if got := r.Header.Get("X-Goog-Api-Key"); got != "test-key" {
			t.Fatalf("expected X-Goog-Api-Key header to be set, got %q", got)
		}

		resp := googleBooksResponse{
			Items: []googleVolume{
				{
					ID: "abc",
					VolumeInfo: googleVolumeInfo{
						Title:         "Example Title",
						Authors:       []string{"Author One", "Author Two"},
						Description:   "Full description",
						PublishedDate: "2001-09-17",
						PageCount:     352,
						IndustryIdentifiers: []googleIndustryIdentifier{
							{Type: "ISBN_13", Identifier: "9781234567897"},
							{Type: "ISBN_10", Identifier: "1234567890"},
						},
						ImageLinks: googleImageLinks{
							Thumbnail: "http://books.google.com/thumbnail.jpg",
						},
					},
				},
			},
		}

		return jsonResponse(t, http.StatusOK, resp), nil
	})
	svc := NewService(client, WithGoogleBooksBaseURL("http://example.test"), WithGoogleBooksAPIKey("test-key"))

	results, err := svc.Lookup(context.Background(), "example keywords", CategoryBook)
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	entry := results[0]
	if entry.Title != "Example Title" {
		t.Fatalf("expected title to be populated, got %q", entry.Title)
	}
	if entry.Creator != "Author One, Author Two" {
		t.Fatalf("expected creator to match authors, got %q", entry.Creator)
	}
	if entry.PageCount == nil || *entry.PageCount != 352 {
		t.Fatalf("expected page count to be set, got %+v", entry.PageCount)
	}
	if entry.ReleaseYear == nil || *entry.ReleaseYear != 2001 {
		t.Fatalf("expected release year 2001, got %+v", entry.ReleaseYear)
	}
	if entry.ISBN13 != "9781234567897" || entry.ISBN10 != "1234567890" {
		t.Fatalf("expected both ISBNs to be set, got %q, %q", entry.ISBN13, entry.ISBN10)
	}
	if entry.CoverImage != "https://books.google.com/thumbnail.jpg" {
		t.Fatalf("expected cover image to be normalized, got %q", entry.CoverImage)
	}
}

func TestLookupBookLeavesGenreEmptyWhenCategoriesMissing(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(r *http.Request) (*http.Response, error) {
		resp := googleBooksResponse{
			Items: []googleVolume{
				{
					ID: "no-categories",
					VolumeInfo: googleVolumeInfo{
						Title:         "Example Title",
						Authors:       []string{"Author One"},
						Description:   "Full description",
						PublishedDate: "2001-09-17",
						PageCount:     352,
						IndustryIdentifiers: []googleIndustryIdentifier{
							{Type: "ISBN_13", Identifier: "9781234567897"},
						},
						ImageLinks: googleImageLinks{
							Thumbnail: "https://books.google.com/thumbnail.jpg",
						},
						// Categories intentionally omitted (nil) to simulate Google Books volumes
						// that provide no category information.
					},
				},
			},
		}

		return jsonResponse(t, http.StatusOK, resp), nil
	})
	svc := NewService(client, WithGoogleBooksBaseURL("http://example.test"), WithGoogleBooksAPIKey("test-key"))
	results, err := svc.Lookup(context.Background(), "example keywords", CategoryBook)
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Genre != "" {
		t.Fatalf("expected empty genre when categories are missing, got %q", results[0].Genre)
	}
}

func TestLookupBookLeavesGenreEmptyWhenCategoriesUnmatched(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(r *http.Request) (*http.Response, error) {
		resp := googleBooksResponse{
			Items: []googleVolume{
				{
					ID: "unmatched-categories",
					VolumeInfo: googleVolumeInfo{
						Title:         "Example Title",
						Authors:       []string{"Author One"},
						PublishedDate: "2001-09-17",
						IndustryIdentifiers: []googleIndustryIdentifier{
							{Type: "ISBN_13", Identifier: "9781234567897"},
						},
						Categories: []string{"Totally Unrelated Category"},
					},
				},
			},
		}

		return jsonResponse(t, http.StatusOK, resp), nil
	})
	svc := NewService(client, WithGoogleBooksBaseURL("http://example.test"), WithGoogleBooksAPIKey("test-key"))
	results, err := svc.Lookup(context.Background(), "example keywords", CategoryBook)
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Genre != "" {
		t.Fatalf("expected empty genre when categories are unmatched, got %q", results[0].Genre)
	}
}

func TestLookupBookByISBNFillsMissingIdentifiers(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(r *http.Request) (*http.Response, error) {
		values := r.URL.Query()
		if got := values.Get("q"); got != "isbn:1234567890" {
			t.Fatalf("expected ISBN query, got %s", got)
		}
		if got := values.Get("maxResults"); got != "1" {
			t.Fatalf("expected maxResults=1, got %s", got)
		}

		resp := googleBooksResponse{
			Items: []googleVolume{
				{
					ID: "isbn-volume",
					VolumeInfo: googleVolumeInfo{
						Title:         "ISBN Lookup",
						Authors:       []string{"Solo Author"},
						Subtitle:      "Subtitle",
						PublishedDate: "1998",
						PageCount:     200,
						IndustryIdentifiers: []googleIndustryIdentifier{
							{Type: "ISBN_13", Identifier: "9781111111111"},
						},
						ImageLinks: googleImageLinks{
							SmallThumbnail: "https://books.google.com/small-thumb.jpg",
						},
					},
				},
			},
		}
		return jsonResponse(t, http.StatusOK, resp), nil
	})
	svc := NewService(client, WithGoogleBooksBaseURL("http://example.test"), WithGoogleBooksAPIKey("test-key"))
	results, err := svc.Lookup(context.Background(), "1234567890", CategoryBook)
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	entry := results[0]
	if entry.ISBN10 != "1234567890" {
		t.Fatalf("expected ISBN10 to fall back to query value, got %q", entry.ISBN10)
	}
	if entry.ISBN13 != "9781111111111" {
		t.Fatalf("expected ISBN13 from response, got %q", entry.ISBN13)
	}
	if entry.Description != "Subtitle" {
		t.Fatalf("expected subtitle fallback for description, got %q", entry.Description)
	}
	if entry.CoverImage != "https://books.google.com/small-thumb.jpg" {
		t.Fatalf("expected cover image to match link, got %q", entry.CoverImage)
	}
}

func TestLookupBookByQueryReturnsNotFoundWhenEmpty(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(r *http.Request) (*http.Response, error) {
		return jsonResponse(t, http.StatusOK, googleBooksResponse{}), nil
	})
	svc := NewService(client, WithGoogleBooksBaseURL("http://example.test"), WithGoogleBooksAPIKey("test-key"))
	_, err := svc.Lookup(context.Background(), "unknown", CategoryBook)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGoogleBooksAPIKeyNeverAppearsInURLOrErrors(t *testing.T) {
	t.Parallel()
	const secret = "super-secret-key"
	var seenURLs []string
	client := newTestClient(t, func(r *http.Request) (*http.Response, error) {
		seenURLs = append(seenURLs, r.URL.String())
		return nil, errors.New("connection refused")
	})
	svc := NewService(client, WithGoogleBooksBaseURL("http://example.test"), WithGoogleBooksAPIKey(secret))

	_, lookupErr := svc.Lookup(context.Background(), "example keywords", CategoryBook)
	if lookupErr == nil {
		t.Fatal("expected lookup error")
	}
	_, volumeErr := svc.LookupByVolumeID(context.Background(), "vol-1")
	if volumeErr == nil {
		t.Fatal("expected volume lookup error")
	}

	for _, err := range []error{lookupErr, volumeErr} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error leaks API key: %v", err)
		}
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			t.Fatalf("expected *url.Error to be unwrapped, got %v", err)
		}
	}
	if len(seenURLs) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(seenURLs))
	}
	for _, u := range seenURLs {
		if strings.Contains(u, secret) {
			t.Fatalf("request URL leaks API key: %s", u)
		}
	}
}

func TestLookupRejectsShortQueries(t *testing.T) {
	svc := NewService(nil, WithGoogleBooksAPIKey("test"))
	if _, err := svc.Lookup(context.Background(), "no", CategoryBook); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("expected ErrInvalidQuery, got %v", err)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}

func newTestClient(t *testing.T, handler func(*http.Request) (*http.Response, error)) *http.Client {
	t.Helper()
	return &http.Client{
		Transport: roundTripperFunc(handler),
	}
}

func jsonResponse(t *testing.T, status int, payload any) *http.Response {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	return &http.Response{
		StatusCode: status,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body: io.NopCloser(bytes.NewReader(raw)),
	}
}
