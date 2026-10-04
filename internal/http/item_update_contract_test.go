package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"anthology/internal/items"
)

// updateContractSeeds are the stored items the update contract cases start
// from. Every editable field of the relevant item type is set.
var updateContractSeeds = map[string]items.CreateItemInput{
	"readingBook": {
		Title:          "Dune",
		Creator:        "Frank Herbert",
		ItemType:       items.ItemTypeBook,
		ReleaseYear:    intPtr(1965),
		PageCount:      intPtr(412),
		CurrentPage:    intPtr(100),
		ISBN13:         "9780441013593",
		ISBN10:         "0441013597",
		Description:    "Desert planet.",
		CoverImage:     "https://example.com/dune.jpg",
		Format:         items.FormatPaperback,
		Genre:          items.GenreFiction,
		Rating:         intPtr(9),
		RetailPriceUsd: floatPtr(9.99),
		GoogleVolumeId: "vol-1",
		ReadingStatus:  items.BookStatusReading,
		Notes:          "Gift",
		SeriesName:     "Dune Chronicles",
		VolumeNumber:   intPtr(1),
		TotalVolumes:   intPtr(6),
	},
	"readBook": {
		Title:         "Emma",
		Creator:       "Jane Austen",
		ItemType:      items.ItemTypeBook,
		PageCount:     intPtr(474),
		ReadingStatus: items.BookStatusRead,
		ReadAt:        timePtr(time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)),
	},
	"game": {
		Title:       "Catan",
		Creator:     "Klaus Teuber",
		ItemType:    items.ItemTypeGame,
		ReleaseYear: intPtr(1995),
		Platform:    "Tabletop",
		AgeGroup:    "10+",
		PlayerCount: "3-4",
		Notes:       "Missing a road",
	},
}

// itemEditableKeys lists the JSON keys PUT /api/items/{id} accepts.
var itemEditableKeys = []string{
	"title", "creator", "itemType", "releaseYear", "pageCount", "currentPage",
	"isbn13", "isbn10", "description", "coverImage", "format", "genre",
	"rating", "retailPriceUsd", "googleVolumeId", "platform", "ageGroup",
	"playerCount", "readingStatus", "readAt", "notes", "seriesName",
	"volumeNumber", "totalVolumes",
}

type updateContractCase struct {
	name   string
	seed   string
	body   string
	status int
	// errMsg is the expected error message when status is not 200.
	errMsg string
	// want edits the seed's JSON into the expected item. Keys set to nil are
	// expected to be absent (omitempty pointers that were cleared).
	want map[string]any
}

// TestItemUpdateContract pins the semantics of PUT /api/items/{id}: an
// omitted key keeps the stored value; null clears pointer fields, leaves
// plain string fields unchanged, resets format, genre and readingStatus, and
// is rejected for itemType; a value is normalized as on create.
func TestItemUpdateContract(t *testing.T) {
	cases := []updateContractCase{
		// title
		{name: "title value", seed: "readingBook", body: `{"title":"  Dune Messiah  "}`, want: map[string]any{"title": "Dune Messiah"}},
		{name: "title null", seed: "readingBook", body: `{"title":null}`},
		{name: "title blank", seed: "readingBook", body: `{"title":"   "}`, status: 400, errMsg: "title is required"},
		// creator
		{name: "creator value", seed: "readingBook", body: `{"creator":" F. Herbert "}`, want: map[string]any{"creator": "F. Herbert"}},
		{name: "creator null", seed: "readingBook", body: `{"creator":null}`},
		{name: "creator empty", seed: "readingBook", body: `{"creator":""}`, want: map[string]any{"creator": ""}},
		// itemType
		{name: "itemType value", seed: "readingBook", body: `{"itemType":"movie"}`, want: map[string]any{
			"itemType": "movie", "pageCount": nil, "currentPage": nil, "isbn13": "", "isbn10": "",
			"format": "", "genre": "", "rating": nil, "retailPriceUsd": nil, "googleVolumeId": "",
			"readingStatus": "none", "seriesName": "", "volumeNumber": nil, "totalVolumes": nil,
		}},
		{name: "itemType game clears book fields", seed: "readingBook", body: `{"itemType":"game","platform":" PC "}`, want: map[string]any{
			"itemType": "game", "platform": "PC", "pageCount": nil, "currentPage": nil, "isbn13": "", "isbn10": "",
			"format": "", "genre": "", "rating": nil, "retailPriceUsd": nil, "googleVolumeId": "",
			"readingStatus": "none", "seriesName": "", "volumeNumber": nil, "totalVolumes": nil,
		}},
		{name: "itemType game to book clears game fields", seed: "game", body: `{"itemType":"book"}`, want: map[string]any{
			"itemType": "book", "platform": "", "ageGroup": "", "playerCount": "", "format": "UNKNOWN",
		}},
		{name: "itemType null", seed: "readingBook", body: `{"itemType":null}`, status: 400, errMsg: "itemType is required"},
		{name: "itemType empty", seed: "readingBook", body: `{"itemType":""}`, status: 400, errMsg: "itemType is required"},
		{name: "itemType unknown", seed: "readingBook", body: `{"itemType":"vinyl"}`, status: 400, errMsg: "itemType must be one of book, game, movie, or music"},
		// releaseYear
		{name: "releaseYear value", seed: "readingBook", body: `{"releaseYear":1966}`, want: map[string]any{"releaseYear": 1966.0}},
		{name: "releaseYear null", seed: "readingBook", body: `{"releaseYear":null}`, want: map[string]any{"releaseYear": nil}},
		{name: "releaseYear negative", seed: "readingBook", body: `{"releaseYear":-5}`, want: map[string]any{"releaseYear": nil}},
		{name: "releaseYear zero", seed: "readingBook", body: `{"releaseYear":0}`, want: map[string]any{"releaseYear": 0.0}},
		// pageCount
		{name: "pageCount value", seed: "readingBook", body: `{"pageCount":500}`, want: map[string]any{"pageCount": 500.0}},
		{name: "pageCount null", seed: "readingBook", body: `{"pageCount":null}`, want: map[string]any{"pageCount": nil}},
		{name: "pageCount zero", seed: "readingBook", body: `{"pageCount":0}`, want: map[string]any{"pageCount": nil}},
		{name: "pageCount below currentPage", seed: "readingBook", body: `{"pageCount":50}`, status: 400, errMsg: "currentPage cannot exceed pageCount"},
		{name: "pageCount on game", seed: "game", body: `{"pageCount":10}`},
		// currentPage
		{name: "currentPage value", seed: "readingBook", body: `{"currentPage":150}`, want: map[string]any{"currentPage": 150.0}},
		{name: "currentPage null", seed: "readingBook", body: `{"currentPage":null}`, want: map[string]any{"currentPage": nil}},
		{name: "currentPage negative", seed: "readingBook", body: `{"currentPage":-1}`, status: 400, errMsg: "currentPage must be zero or greater"},
		{name: "currentPage past pageCount", seed: "readingBook", body: `{"currentPage":999}`, status: 400, errMsg: "currentPage cannot exceed pageCount"},
		{name: "currentPage when not reading", seed: "readBook", body: `{"currentPage":5}`},
		// isbn13
		{name: "isbn13 value", seed: "readingBook", body: `{"isbn13":" 978-0-441-01359-3 "}`, want: map[string]any{"isbn13": "978-0-441-01359-3"}},
		{name: "isbn13 null", seed: "readingBook", body: `{"isbn13":null}`},
		{name: "isbn13 empty", seed: "readingBook", body: `{"isbn13":""}`, want: map[string]any{"isbn13": ""}},
		// isbn10
		{name: "isbn10 value", seed: "readingBook", body: `{"isbn10":" 0-441-01359-7 "}`, want: map[string]any{"isbn10": "0-441-01359-7"}},
		{name: "isbn10 null", seed: "readingBook", body: `{"isbn10":null}`},
		// description
		{name: "description value", seed: "readingBook", body: `{"description":"  Spice.  "}`, want: map[string]any{"description": "Spice."}},
		{name: "description null", seed: "readingBook", body: `{"description":null}`},
		// coverImage
		{name: "coverImage value", seed: "readingBook", body: `{"coverImage":" https://example.com/new.jpg "}`, want: map[string]any{"coverImage": "https://example.com/new.jpg"}},
		{name: "coverImage null", seed: "readingBook", body: `{"coverImage":null}`},
		{name: "coverImage empty", seed: "readingBook", body: `{"coverImage":""}`, want: map[string]any{"coverImage": ""}},
		{name: "coverImage http", seed: "readingBook", body: `{"coverImage":"http://example.com/new.jpg"}`, status: 400, errMsg: "coverImage URL must use HTTPS"},
		// format
		{name: "format value", seed: "readingBook", body: `{"format":"HARDCOVER"}`, want: map[string]any{"format": "HARDCOVER"}},
		{name: "format null", seed: "readingBook", body: `{"format":null}`, want: map[string]any{"format": "UNKNOWN"}},
		{name: "format unknown", seed: "readingBook", body: `{"format":"SCROLL"}`, want: map[string]any{"format": "UNKNOWN"}},
		{name: "format on game", seed: "game", body: `{"format":"HARDCOVER"}`},
		// genre
		{name: "genre value", seed: "readingBook", body: `{"genre":"HISTORY"}`, want: map[string]any{"genre": "HISTORY"}},
		{name: "genre null", seed: "readingBook", body: `{"genre":null}`, want: map[string]any{"genre": ""}},
		{name: "genre unknown", seed: "readingBook", body: `{"genre":"POETRY"}`, want: map[string]any{"genre": ""}},
		// rating
		{name: "rating value", seed: "readingBook", body: `{"rating":7}`, want: map[string]any{"rating": 7.0}},
		{name: "rating null", seed: "readingBook", body: `{"rating":null}`, want: map[string]any{"rating": nil}},
		{name: "rating out of range", seed: "readingBook", body: `{"rating":11}`, want: map[string]any{"rating": nil}},
		// retailPriceUsd
		{name: "retailPriceUsd value", seed: "readingBook", body: `{"retailPriceUsd":12.5}`, want: map[string]any{"retailPriceUsd": 12.5}},
		{name: "retailPriceUsd null", seed: "readingBook", body: `{"retailPriceUsd":null}`, want: map[string]any{"retailPriceUsd": nil}},
		{name: "retailPriceUsd negative", seed: "readingBook", body: `{"retailPriceUsd":-1}`, want: map[string]any{"retailPriceUsd": nil}},
		// googleVolumeId
		{name: "googleVolumeId value", seed: "readingBook", body: `{"googleVolumeId":" vol-2 "}`, want: map[string]any{"googleVolumeId": "vol-2"}},
		{name: "googleVolumeId null", seed: "readingBook", body: `{"googleVolumeId":null}`},
		// platform, ageGroup, playerCount
		{name: "platform value", seed: "game", body: `{"platform":" PC "}`, want: map[string]any{"platform": "PC"}},
		{name: "platform null", seed: "game", body: `{"platform":null}`},
		{name: "platform empty", seed: "game", body: `{"platform":""}`, want: map[string]any{"platform": ""}},
		{name: "platform on book", seed: "readingBook", body: `{"platform":"PC"}`},
		{name: "ageGroup value", seed: "game", body: `{"ageGroup":" 8+ "}`, want: map[string]any{"ageGroup": "8+"}},
		{name: "ageGroup null", seed: "game", body: `{"ageGroup":null}`},
		{name: "playerCount value", seed: "game", body: `{"playerCount":" 2-6 "}`, want: map[string]any{"playerCount": "2-6"}},
		{name: "playerCount null", seed: "game", body: `{"playerCount":null}`},
		// readingStatus
		{name: "readingStatus value", seed: "readingBook", body: `{"readingStatus":"want_to_read"}`, want: map[string]any{"readingStatus": "want_to_read", "currentPage": nil}},
		{name: "readingStatus null", seed: "readingBook", body: `{"readingStatus":null}`, want: map[string]any{"readingStatus": "none", "currentPage": nil}},
		{name: "readingStatus null on read book", seed: "readBook", body: `{"readingStatus":null}`, want: map[string]any{"readingStatus": "none", "readAt": nil}},
		{name: "readingStatus empty", seed: "readBook", body: `{"readingStatus":""}`, want: map[string]any{"readingStatus": "none", "readAt": nil}},
		{name: "readingStatus read without readAt", seed: "readingBook", body: `{"readingStatus":"read"}`, status: 400, errMsg: "readAt is required when readingStatus is read"},
		{name: "readingStatus read with readAt", seed: "readingBook", body: `{"readingStatus":"read","readAt":"2025-03-04T05:06:07+02:00"}`, want: map[string]any{"readingStatus": "read", "readAt": "2025-03-04T03:06:07Z", "currentPage": nil}},
		{name: "readingStatus unknown", seed: "readingBook", body: `{"readingStatus":"abandoned"}`, status: 400, errMsg: "readingStatus must be one of none, read, reading, or want_to_read"},
		{name: "readingStatus on game", seed: "game", body: `{"readingStatus":"read"}`},
		// readAt
		{name: "readAt value", seed: "readBook", body: `{"readAt":"2025-03-04T05:06:07Z"}`, want: map[string]any{"readAt": "2025-03-04T05:06:07Z"}},
		{name: "readAt null", seed: "readBook", body: `{"readAt":null}`, status: 400, errMsg: "readAt is required when readingStatus is read"},
		{name: "readAt while reading", seed: "readingBook", body: `{"readAt":"2025-03-04T05:06:07Z"}`},
		{name: "readAt null while reading", seed: "readingBook", body: `{"readAt":null}`},
		// notes
		{name: "notes value", seed: "readingBook", body: `{"notes":"  Signed  "}`, want: map[string]any{"notes": "Signed"}},
		{name: "notes null", seed: "readingBook", body: `{"notes":null}`},
		{name: "notes empty", seed: "readingBook", body: `{"notes":""}`, want: map[string]any{"notes": ""}},
		// seriesName
		{name: "seriesName value", seed: "readingBook", body: `{"seriesName":" Dune Saga "}`, want: map[string]any{"seriesName": "Dune Saga"}},
		{name: "seriesName null", seed: "readingBook", body: `{"seriesName":null}`},
		{name: "seriesName empty", seed: "readingBook", body: `{"seriesName":""}`, want: map[string]any{"seriesName": ""}},
		// volumeNumber
		{name: "volumeNumber value", seed: "readingBook", body: `{"volumeNumber":2}`, want: map[string]any{"volumeNumber": 2.0}},
		{name: "volumeNumber null", seed: "readingBook", body: `{"volumeNumber":null}`, want: map[string]any{"volumeNumber": nil}},
		{name: "volumeNumber zero", seed: "readingBook", body: `{"volumeNumber":0}`, want: map[string]any{"volumeNumber": nil}},
		{name: "volumeNumber past totalVolumes", seed: "readingBook", body: `{"volumeNumber":7}`, status: 400, errMsg: "volumeNumber cannot exceed totalVolumes"},
		{name: "volumeNumber past 200", seed: "readingBook", body: `{"volumeNumber":201}`, status: 400, errMsg: "volumeNumber cannot exceed 200"},
		// totalVolumes
		{name: "totalVolumes value", seed: "readingBook", body: `{"totalVolumes":8}`, want: map[string]any{"totalVolumes": 8.0}},
		{name: "totalVolumes null", seed: "readingBook", body: `{"totalVolumes":null}`, want: map[string]any{"totalVolumes": nil}},
		{name: "totalVolumes zero", seed: "readingBook", body: `{"totalVolumes":0}`, want: map[string]any{"totalVolumes": nil}},
		{name: "totalVolumes past 200", seed: "readingBook", body: `{"totalVolumes":201}`, status: 400, errMsg: "totalVolumes cannot exceed 200"},
		{name: "series fields null together", seed: "readingBook", body: `{"seriesName":"","volumeNumber":null,"totalVolumes":null}`, want: map[string]any{"seriesName": "", "volumeNumber": nil, "totalVolumes": nil}},

		// Request body shape.
		{name: "empty object", seed: "readingBook", body: `{}`},
		{name: "null body", seed: "readingBook", body: `null`},
		{name: "read-only and unknown keys ignored", seed: "readingBook", body: fmt.Sprintf(
			`{"id":%q,"createdAt":"2000-01-01T00:00:00Z","updatedAt":"2000-01-01T00:00:00Z","shelfPlacement":{"shelfName":"X"},"ownerId":%q,"bogus":1}`,
			uuid.NewString(), uuid.NewString())},
		{name: "read-only keys with wrong types ignored", seed: "readingBook", body: `{"id":5,"createdAt":false,"shelfPlacement":"x"}`},
		{name: "keys match case-sensitively", seed: "readingBook", body: `{"Title":"Other","ITEMTYPE":"game"}`},
		{name: "wrong type", seed: "readingBook", body: `{"title":5}`, status: 400, errMsg: "invalid request body"},
		{name: "wrong type under other casing", seed: "readingBook", body: `{"TITLE":5}`, status: 400, errMsg: "invalid request body"},
		{name: "wrong type for pointer field", seed: "readingBook", body: `{"releaseYear":"1966"}`, status: 400, errMsg: "invalid request body"},
		{name: "fractional int", seed: "readingBook", body: `{"rating":7.5}`, status: 400, errMsg: "invalid request body"},
		{name: "bad time", seed: "readBook", body: `{"readAt":"yesterday"}`, status: 400, errMsg: "invalid request body"},
		{name: "array body", seed: "readingBook", body: `[]`, status: 400, errMsg: "invalid request body"},
		{name: "malformed body", seed: "readingBook", body: `{"title":`, status: 400, errMsg: "invalid request body"},
		{name: "empty body", seed: "readingBook", body: ``, status: 400, errMsg: "invalid request body"},
	}

	// An omitted key keeps the stored value: resend every other key unchanged,
	// as the edit form does, and expect the item to stay the same.
	for _, seedName := range []string{"readingBook", "readBook", "game"} {
		for _, key := range itemEditableKeys {
			cases = append(cases, updateContractCase{
				name: fmt.Sprintf("%s omitted on %s", key, seedName),
				seed: seedName,
				body: omittingKeyBody(t, seedName, key),
			})
		}
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler, svc, seeded := newUpdateContractFixture(t, tc.seed)
			before := itemJSONMap(t, seeded)

			rec := putItem(handler, seeded.ID.String(), tc.body)

			status := tc.status
			if status == 0 {
				status = http.StatusOK
			}
			if rec.Code != status {
				t.Fatalf("status = %d, want %d; body %s", rec.Code, status, rec.Body.String())
			}

			stored, err := svc.Get(context.Background(), seeded.ID, testOwnerID)
			if err != nil {
				t.Fatalf("get after update: %v", err)
			}

			if status != http.StatusOK {
				var response map[string]any
				if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
					t.Fatalf("decode error response: %v", err)
				}
				if response["error"] != tc.errMsg {
					t.Fatalf("error = %v, want %q", response["error"], tc.errMsg)
				}
				if got := itemJSONMap(t, stored); !reflect.DeepEqual(got, before) {
					t.Fatalf("rejected update changed the stored item:\n got %v\nwant %v", got, before)
				}
				return
			}

			want := maps.Clone(before)
			for key, value := range tc.want {
				if value == nil {
					delete(want, key)
				} else {
					want[key] = value
				}
			}
			delete(want, "updatedAt")

			var got map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if got["updatedAt"] == before["updatedAt"] {
				t.Fatalf("updatedAt was not bumped")
			}
			delete(got, "updatedAt")
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("response item mismatch:\n got %v\nwant %v", got, want)
			}

			storedMap := itemJSONMap(t, stored)
			delete(storedMap, "updatedAt")
			if !reflect.DeepEqual(storedMap, want) {
				t.Fatalf("stored item mismatch:\n got %v\nwant %v", storedMap, want)
			}
		})
	}
}

func TestItemUpdateContractUnknownItem(t *testing.T) {
	handler, _, _ := newUpdateContractFixture(t, "readingBook")

	rec := putItem(handler, uuid.NewString(), `{"title":"Other"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body %s", rec.Code, rec.Body.String())
	}

	// The body is checked before the item is looked up.
	rec = putItem(handler, uuid.NewString(), `{"title":5}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body %s", rec.Code, rec.Body.String())
	}

	rec = putItem(handler, "not-a-uuid", `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body %s", rec.Code, rec.Body.String())
	}
}

func TestItemUpdateContractPayloadTooLarge(t *testing.T) {
	handler, _, seeded := newUpdateContractFixture(t, "readingBook")

	body := `{"notes":"` + strings.Repeat("a", int(maxJSONBodyBytes)) + `"}`
	rec := putItem(handler, seeded.ID.String(), body)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
}

func newUpdateContractFixture(t *testing.T, seedName string) (*ItemHandler, *items.Service, items.Item) {
	t.Helper()
	seed, ok := updateContractSeeds[seedName]
	if !ok {
		t.Fatalf("unknown seed %q", seedName)
	}
	seed.OwnerID = testOwnerID
	createdAt := time.Date(2020, 5, 6, 7, 8, 9, 0, time.UTC)
	seed.CreatedAt = &createdAt

	svc := items.NewService(items.NewInMemoryRepository(nil))
	seeded, err := svc.Create(context.Background(), seed)
	if err != nil {
		t.Fatalf("seed %s: %v", seedName, err)
	}
	handler := NewItemHandler(svc, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return handler, svc, seeded
}

func putItem(handler *ItemHandler, id, body string) *httptest.ResponseRecorder {
	req := reqWithUser(httptest.NewRequest(http.MethodPut, "/api/items/"+id, strings.NewReader(body)))
	route := chi.NewRouteContext()
	route.URLParams.Add("id", id)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, route))
	rec := httptest.NewRecorder()
	handler.Update(rec, req)
	return rec
}

// omittingKeyBody returns the seed's editable fields as a JSON object without
// key, the way the edit form sends every field it shows.
func omittingKeyBody(t *testing.T, seedName, key string) string {
	t.Helper()
	_, _, seeded := newUpdateContractFixture(t, seedName)
	fields := itemJSONMap(t, seeded)
	body := map[string]any{}
	for _, editable := range itemEditableKeys {
		if value, ok := fields[editable]; ok && editable != key {
			body[editable] = value
		}
	}
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return string(data)
}

func itemJSONMap(t *testing.T, item items.Item) map[string]any {
	t.Helper()
	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal item: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("unmarshal item: %v", err)
	}
	return fields
}

func intPtr(v int) *int { return &v }

func floatPtr(v float64) *float64 { return &v }

func timePtr(v time.Time) *time.Time { return &v }

// TestItemCreateContract pins how POST /api/items decodes and normalizes a
// new item, since create and update share one normalization step.
func TestItemCreateContract(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		status int
		errMsg string
		want   map[string]any
	}{
		{
			name: "book normalized",
			body: `{"title":"  Dune ","creator":" Frank Herbert ","itemType":"book","releaseYear":-1,"pageCount":0,
				"currentPage":3,"isbn13":" 9780441013593 ","isbn10":" 0441013597 ","description":" Desert. ",
				"coverImage":" https://example.com/d.jpg ","format":"SCROLL","genre":"POETRY","rating":11,
				"retailPriceUsd":-2,"googleVolumeId":" v1 ","platform":"PC","ageGroup":"9+","playerCount":"1",
				"readingStatus":"reading","readAt":"2025-01-01T00:00:00Z","notes":" n ","seriesName":" S ",
				"volumeNumber":0,"totalVolumes":4}`,
			want: map[string]any{
				"title": "Dune", "creator": "Frank Herbert", "itemType": "book", "currentPage": 3.0,
				"isbn13": "9780441013593", "isbn10": "0441013597", "description": "Desert.",
				"coverImage": "https://example.com/d.jpg", "format": "UNKNOWN", "genre": "",
				"googleVolumeId": "v1", "platform": "", "ageGroup": "", "playerCount": "",
				"readingStatus": "reading", "notes": "n", "seriesName": "S", "totalVolumes": 4.0,
			},
		},
		{
			name: "game with nulls",
			body: `{"title":"Catan","creator":null,"itemType":"game","platform":" Tabletop ","format":null,
				"genre":null,"readingStatus":null,"rating":null,"releaseYear":1995}`,
			want: map[string]any{
				"title": "Catan", "creator": "", "itemType": "game", "releaseYear": 1995.0, "isbn13": "",
				"isbn10": "", "description": "", "coverImage": "", "format": "", "genre": "",
				"googleVolumeId": "", "platform": "Tabletop", "ageGroup": "", "playerCount": "",
				"readingStatus": "none", "notes": "", "seriesName": "",
			},
		},
		{
			name: "key casing is ignored",
			body: `{"Title":"Heat","ITEMTYPE":"movie"}`,
			want: map[string]any{
				"title": "Heat", "creator": "", "itemType": "movie", "isbn13": "", "isbn10": "",
				"description": "", "coverImage": "", "format": "", "genre": "", "googleVolumeId": "",
				"platform": "", "ageGroup": "", "playerCount": "", "readingStatus": "none", "notes": "",
				"seriesName": "",
			},
		},
		{name: "missing title", body: `{"itemType":"book"}`, status: 400, errMsg: "title is required"},
		{name: "missing itemType", body: `{"title":"Dune"}`, status: 400, errMsg: "itemType is required"},
		{name: "null itemType", body: `{"title":"Dune","itemType":null}`, status: 400, errMsg: "itemType is required"},
		{name: "read without readAt", body: `{"title":"Dune","itemType":"book","readingStatus":"read"}`, status: 400, errMsg: "readAt is required when readingStatus is read"},
		{name: "http cover", body: `{"title":"Dune","itemType":"book","coverImage":"http://example.com/d.jpg"}`, status: 400, errMsg: "coverImage URL must use HTTPS"},
		{name: "unknown key", body: `{"title":"Dune","itemType":"book","bogus":1}`, status: 400, errMsg: "invalid request body"},
		{name: "id key", body: fmt.Sprintf(`{"title":"Dune","itemType":"book","id":%q}`, uuid.NewString()), status: 400, errMsg: "invalid request body"},
		{name: "ownerId key", body: fmt.Sprintf(`{"title":"Dune","itemType":"book","ownerId":%q}`, uuid.NewString()), status: 400, errMsg: "invalid request body"},
		{name: "createdAt key", body: `{"title":"Dune","itemType":"book","createdAt":"2000-01-01T00:00:00Z"}`, status: 400, errMsg: "invalid request body"},
		{name: "updatedAt key", body: `{"title":"Dune","itemType":"book","updatedAt":"2000-01-01T00:00:00Z"}`, status: 400, errMsg: "invalid request body"},
		{name: "wrong type", body: `{"title":"Dune","itemType":"book","pageCount":"12"}`, status: 400, errMsg: "invalid request body"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := items.NewService(items.NewInMemoryRepository(nil))
			handler := NewItemHandler(svc, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
			req := reqWithUser(httptest.NewRequest(http.MethodPost, "/api/items", strings.NewReader(tc.body)))
			rec := httptest.NewRecorder()
			handler.Create(rec, req)

			status := tc.status
			if status == 0 {
				status = http.StatusCreated
			}
			if rec.Code != status {
				t.Fatalf("status = %d, want %d; body %s", rec.Code, status, rec.Body.String())
			}
			var got map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if status != http.StatusCreated {
				if got["error"] != tc.errMsg {
					t.Fatalf("error = %v, want %q", got["error"], tc.errMsg)
				}
				return
			}

			id, err := uuid.Parse(fmt.Sprint(got["id"]))
			if err != nil {
				t.Fatalf("response id: %v", err)
			}
			stored, err := svc.Get(context.Background(), id, testOwnerID)
			if err != nil {
				t.Fatalf("created item not stored for the caller: %v", err)
			}
			if got["createdAt"] != got["updatedAt"] {
				t.Fatalf("createdAt %v != updatedAt %v", got["createdAt"], got["updatedAt"])
			}
			for _, key := range []string{"id", "createdAt", "updatedAt"} {
				delete(got, key)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("created item mismatch:\n got %v\nwant %v", got, tc.want)
			}
			storedMap := itemJSONMap(t, stored)
			for _, key := range []string{"id", "createdAt", "updatedAt"} {
				delete(storedMap, key)
			}
			if !reflect.DeepEqual(storedMap, tc.want) {
				t.Fatalf("stored item mismatch:\n got %v\nwant %v", storedMap, tc.want)
			}
		})
	}
}
