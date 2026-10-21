package metrics

import (
	"database/sql"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "github.com/lib/pq"
)

func scrape(t *testing.T, m *Metrics) string {
	t.Helper()
	rec := httptest.NewRecorder()
	NewMux(m.Handler()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /metrics status = %d, want %d", rec.Code, http.StatusOK)
	}
	return rec.Body.String()
}

func assertContains(t *testing.T, body string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(body, w) {
			t.Errorf("metrics output missing %q", w)
		}
	}
}

func assertNotContains(t *testing.T, body string, unwanted ...string) {
	t.Helper()
	for _, u := range unwanted {
		if strings.Contains(body, u) {
			t.Errorf("metrics output unexpectedly contains %q", u)
		}
	}
}

func newTestRouter(m *Metrics) http.Handler {
	r := chi.NewRouter()
	r.Use(m.Middleware(r))
	ok := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }
	r.Get("/health", ok)
	r.Route("/api", func(r chi.Router) {
		r.Get("/ready", ok)
		// Middleware that rejects before a mounted subrouter routes the
		// request, as the app's auth middleware does.
		r.Group(func(r chi.Router) {
			r.Use(func(http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusUnauthorized)
				})
			})
			r.Route("/private", func(r chi.Router) {
				r.Get("/{id}", ok)
			})
		})
		r.Route("/items", func(r chi.Router) {
			r.Get("/", ok)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", ok)
				r.Delete("/", func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNoContent)
				})
			})
		})
	})
	return r
}

func TestMiddlewareLabelsRoutePatterns(t *testing.T) {
	m := New()
	router := newTestRouter(m)

	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/items", nil),
		httptest.NewRequest(http.MethodGet, "/api/items/8f9c0e1a-1111-2222-3333-444455556666?secret=1", nil),
		httptest.NewRequest(http.MethodGet, "/api/items/another-id", nil),
		httptest.NewRequest(http.MethodDelete, "/api/items/8f9c0e1a-1111-2222-3333-444455556666", nil),
		httptest.NewRequest(http.MethodPatch, "/api/items/some-id", nil),
		httptest.NewRequest(http.MethodGet, "/api/private/secret-id", nil),
		httptest.NewRequest(http.MethodGet, "/api/private/secret-id/extra", nil),
		httptest.NewRequest(http.MethodGet, "/api/does-not-exist/42", nil),
		httptest.NewRequest(http.MethodGet, "/nope", nil),
		httptest.NewRequest("BREW", "/api/items", nil),
		httptest.NewRequest(http.MethodGet, "/health", nil),
		httptest.NewRequest(http.MethodGet, "/api/ready", nil),
	} {
		router.ServeHTTP(httptest.NewRecorder(), req)
	}

	body := scrape(t, m)
	assertContains(t, body,
		`http_requests_total{method="GET",route="/api/items",status="200"} 1`,
		`http_requests_total{method="GET",route="/api/items/{id}",status="200"} 2`,
		`http_requests_total{method="DELETE",route="/api/items/{id}",status="204"} 1`,
		`http_requests_total{method="PATCH",route="/api/items/{id}",status="405"} 1`,
		`http_requests_total{method="GET",route="/api/private/{id}",status="401"} 1`,
		`http_requests_total{method="GET",route="unmatched",status="401"} 1`,
		`http_requests_total{method="GET",route="unmatched",status="404"} 2`,
		`http_requests_total{method="OTHER",route="unmatched",status="405"} 1`,
		`http_request_duration_seconds_bucket{method="GET",route="/api/items/{id}",status="200",le="+Inf"} 2`,
		`http_request_duration_seconds_count{method="DELETE",route="/api/items/{id}",status="204"} 1`,
	)
	assertNotContains(t, body,
		"8f9c0e1a", "another-id", "some-id", "secret", "does-not-exist", "/nope",
		`route="/health"`, `route="/api/ready"`, "BREW",
	)
}

func TestHandlerExposesDomainRuntimeAndDBMetrics(t *testing.T) {
	m := New()
	db, err := sql.Open("postgres", "postgres://user:pass@127.0.0.1:1/none?sslmode=disable")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(10)
	m.RegisterDB(db, "anthology")

	m.OAuthFailure(ProviderGoogle, OAuthExchange)
	m.OAuthFailure(ProviderGoogle, OAuthExchange)
	m.Upload(UploadSuccess)
	m.Upload(UploadTooLarge)
	m.Job(JobSessionCleanup, JobSuccess)
	m.Job(JobSessionCleanup, JobError)

	body := scrape(t, m)
	assertContains(t, body,
		`anthology_oauth_failures_total{provider="google",reason="exchange"} 2`,
		`anthology_uploads_total{result="success"} 1`,
		`anthology_uploads_total{result="too_large"} 1`,
		`anthology_jobs_total{job="session_cleanup",result="success"} 1`,
		`anthology_jobs_total{job="session_cleanup",result="error"} 1`,
		`go_sql_max_open_connections{db_name="anthology"} 10`,
		`go_sql_in_use_connections{db_name="anthology"}`,
		`go_sql_wait_duration_seconds_total{db_name="anthology"}`,
		"go_goroutines",
		"process_resident_memory_bytes",
	)
}

func TestNilMetricsIsNoOp(t *testing.T) {
	var m *Metrics
	m.OAuthFailure(ProviderGoogle, OAuthExchange)
	m.Upload(UploadSuccess)
	m.Job(JobSessionCleanup, JobSuccess)
	m.RegisterDB(nil, "anthology")

	rec := httptest.NewRecorder()
	newTestRouter(m).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/items", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestStartServesOnlyMetrics(t *testing.T) {
	m := New()
	m.Upload(UploadSuccess)
	srv := Start("127.0.0.1:0", m.Handler(), discardLogger())
	if srv == nil {
		t.Fatal("Start returned nil for a bindable address")
	}
	defer func() { _ = srv.Shutdown(t.Context()) }()
	base := "http://" + srv.Addr().String()

	resp, err := http.Get(base + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /metrics status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	assertContains(t, string(body), `anthology_uploads_total{result="success"} 1`)

	resp, err = http.Get(base + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET / status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}

	if err := srv.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if resp, err := http.Get(base + "/metrics"); err == nil {
		_ = resp.Body.Close()
		t.Fatal("metrics listener still serving after Shutdown")
	}
}

func TestStartDisabledOrUnbindableReturnsNil(t *testing.T) {
	for _, addr := range []string{"", "  ", "off", " OFF ", "oFf"} {
		var logs strings.Builder
		logger := slog.New(slog.NewTextHandler(&logs, nil))
		if srv := Start(addr, New().Handler(), logger); srv != nil {
			_ = srv.Shutdown(t.Context())
			t.Fatalf("Start(%q) should disable the listener", addr)
		}
		if !strings.Contains(logs.String(), "metrics listener disabled") || strings.Contains(logs.String(), "failed to bind") {
			t.Fatalf("Start(%q) tried to bind instead of disabling: %s", addr, logs.String())
		}
	}

	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	defer func() { _ = taken.Close() }()
	srv := Start(taken.Addr().String(), New().Handler(), discardLogger())
	if srv != nil {
		_ = srv.Shutdown(t.Context())
		t.Fatal("Start on a taken port should log and return nil")
	}
	if err := srv.Shutdown(t.Context()); err != nil {
		t.Fatalf("nil Server Shutdown: %v", err)
	}
}

func TestOAuthFailureLabelsAreBounded(t *testing.T) {
	m := New()
	for _, reason := range []string{OAuthState, OAuthProviderDenied, OAuthExchange, OAuthNotAllowed, OAuthInternal} {
		m.OAuthFailure(ProviderGoogle, reason)
	}
	m.OAuthFailure("private@example.test", "raw provider error")
	families, err := m.registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, family := range families {
		if family.GetName() != "anthology_oauth_failures_total" {
			continue
		}
		found = true
		if len(family.Metric) != 6 {
			t.Fatalf("got %d series, want 6", len(family.Metric))
		}
		for _, metric := range family.Metric {
			if metric.GetCounter().GetValue() != 1 {
				t.Fatalf("counter = %v, want 1", metric.GetCounter().GetValue())
			}
			for _, label := range metric.Label {
				if label.GetName() == "provider" && label.GetValue() != ProviderGoogle && label.GetValue() != "unknown" {
					t.Fatalf("unbounded provider: %q", label.GetValue())
				}
				if label.GetName() == "reason" {
					switch label.GetValue() {
					case OAuthState, OAuthProviderDenied, OAuthExchange, OAuthNotAllowed, OAuthInternal:
					default:
						t.Fatalf("unbounded reason: %q", label.GetValue())
					}
				}
			}
		}
	}
	if !found {
		t.Fatal("missing OAuth counter")
	}
	assertContains(t, scrape(t, m), `anthology_oauth_failures_total{provider="unknown",reason="internal"} 1`)
	assertNotContains(t, scrape(t, m), "private@example.test", "raw provider error")
}
