package http

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"anthology/internal/config"
)

func TestHealthRoutes(t *testing.T) {
	router := NewRouter(
		config.Config{Environment: "development"},
		nil,
		nil,
		nil,
		nil,
		nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	for _, path := range []string{"/health", "/api/health"} {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, path, nil)

			router.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
			}
			if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
				t.Fatalf("Content-Type = %q, want %q", contentType, "application/json")
			}
			if body := recorder.Body.String(); body != "{\"status\":\"ok\"}\n" {
				t.Fatalf("body = %q, want %q", body, "{\"status\":\"ok\"}\\n")
			}
		})
	}
}
