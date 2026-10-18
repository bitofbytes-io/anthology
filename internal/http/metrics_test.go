package http

import (
	"bytes"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"anthology/internal/config"
	"anthology/internal/importer"
	"anthology/internal/metrics"
)

func scrapeMetrics(t *testing.T, m *metrics.Metrics) string {
	t.Helper()
	rec := httptest.NewRecorder()
	metrics.NewMux(m.Handler()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /metrics status = %d, want %d", rec.Code, http.StatusOK)
	}
	return rec.Body.String()
}

func TestRouterRecordsREDMetricsByRoutePattern(t *testing.T) {
	m := metrics.New()
	router := NewRouter(
		config.Config{Environment: "development"},
		nil, nil, nil, nil, nil, nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		m,
	)

	const itemID = "3d4c1f0e-6a55-4d5e-9a3b-1c2d3e4f5a6b"
	// Without a session cookie the auth middleware rejects protected routes
	// before their subrouters run; they must still be labelled by pattern.
	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/items/"+itemID, nil),
		httptest.NewRequest(http.MethodGet, "/api/items/"+itemID+"?q=private", nil),
		httptest.NewRequest(http.MethodGet, "/api/series/detail?name=private", nil),
		httptest.NewRequest(http.MethodGet, "/api/unknown/"+itemID, nil),
		httptest.NewRequest(http.MethodGet, "/health", nil),
		httptest.NewRequest(http.MethodGet, "/api/health", nil),
	} {
		router.ServeHTTP(httptest.NewRecorder(), req)
	}

	// The public router must not serve metrics.
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("public GET /metrics status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	body := scrapeMetrics(t, m)
	for _, want := range []string{
		`http_requests_total{method="GET",route="/api/items/{id}",status="401"} 2`,
		`http_requests_total{method="GET",route="/api/series/detail",status="401"} 1`,
		`http_requests_total{method="GET",route="unmatched",status="404"} 1`,
		`http_request_duration_seconds_count{method="GET",route="/api/items/{id}",status="401"} 2`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics output missing %q", want)
		}
	}
	for _, unwanted := range []string{itemID, "private", "/health", "/metrics"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("metrics output contains %q", unwanted)
		}
	}
}

func TestOAuthCallbackFailureIsCounted(t *testing.T) {
	m := metrics.New()
	handler := NewOAuthHandler(&fakeGoogleAuthenticator{}, nil, "http://frontend.test", "development", slog.New(slog.NewTextHandler(io.Discard, nil)))
	handler.metrics = m

	// No state cookie: the callback fails before reaching Google.
	handler.CallbackGoogle(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/auth/google/callback?state=x&code=y", nil))

	if body := scrapeMetrics(t, m); !strings.Contains(body, `anthology_oauth_failures_total{provider="google"} 1`) {
		t.Fatalf("expected one Google OAuth failure, got:\n%s", body)
	}
}

func TestCSVUploadsAreCountedByResult(t *testing.T) {
	m := metrics.New()
	handler := NewItemHandler(nil, nil, importer.NewCSVImporter(&csvStoreStub{}, nil), slog.New(slog.NewTextHandler(io.Discard, nil)))
	handler.metrics = m

	handler.ImportCSV(httptest.NewRecorder(), reqWithUser(newMultipartCSVRequest(t, strings.Join([]string{
		"title,creator,itemType,releaseYear,pageCount,isbn13,isbn10,description,coverImage,notes",
		"Title A,Creator,book,2020,300,9780000000001,0000000001,Desc,,Notes",
	}, "\n"))))
	handler.PreviewCSVImport(httptest.NewRecorder(), reqWithUser(newMultipartCSVRequest(t, "title,itemType\nbad,csv\n")))

	var missingFile bytes.Buffer
	writer := multipart.NewWriter(&missingFile)
	_ = writer.WriteField("other", "value")
	_ = writer.Close()
	req := httptest.NewRequest(http.MethodPost, csvImportPath, &missingFile)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	handler.ImportCSV(httptest.NewRecorder(), reqWithUser(req))

	body := scrapeMetrics(t, m)
	for _, want := range []string{
		`anthology_uploads_total{result="success"} 1`,
		`anthology_uploads_total{result="invalid"} 2`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics output missing %q", want)
		}
	}
}
