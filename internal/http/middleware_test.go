package http

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"anthology/internal/auth"

	"github.com/google/uuid"
)

func TestAuthMiddlewareRejectsMissingCookie(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	authService := auth.NewService(&authRepoStub{}, time.Hour)
	next := newAuthMiddleware(authService, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/items", nil)
	rec := httptest.NewRecorder()

	next.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}
}

func TestAuthMiddlewareInjectsUser(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	expectedUser := &auth.User{ID: uuid.New(), Email: "user@example.com"}
	repo := &authRepoStub{
		findSessionByHash: func(ctx context.Context, tokenHash string) (*auth.Session, *auth.User, error) {
			return &auth.Session{ID: uuid.New(), ExpiresAt: time.Now().Add(time.Minute)}, expectedUser, nil
		},
	}
	authService := auth.NewService(repo, time.Hour)

	next := newAuthMiddleware(authService, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		if user == nil || user.Email != expectedUser.Email {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/items", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "token"})
	rec := httptest.NewRecorder()

	next.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestAuthMiddlewareRejectsInvalidSession(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := &authRepoStub{
		findSessionByHash: func(ctx context.Context, tokenHash string) (*auth.Session, *auth.User, error) {
			return nil, nil, nil
		},
	}
	authService := auth.NewService(repo, time.Hour)
	next := newAuthMiddleware(authService, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/items", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "token"})
	rec := httptest.NewRecorder()

	next.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}
}

func TestSameOriginMiddlewareAllowsSafeMethodsWithoutOrigin(t *testing.T) {
	next := newSameOriginMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://api.example.test/api/items", nil)
	rec := httptest.NewRecorder()

	next.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", rec.Code)
	}
}

func TestSameOriginMiddlewareRejectsUnsafeMethodWithoutOrigin(t *testing.T) {
	next := newSameOriginMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "http://api.example.test/api/items", nil)
	rec := httptest.NewRecorder()

	next.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", rec.Code)
	}
}

func TestSameOriginMiddlewareAllowsConfiguredFrontendOrigin(t *testing.T) {
	next := newSameOriginMiddleware([]string{"http://localhost:4200"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "http://localhost:8080/api/items", nil)
	req.Header.Set("Origin", "http://localhost:4200")
	rec := httptest.NewRecorder()

	next.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", rec.Code)
	}
}

func TestSameOriginMiddlewareAllowsForwardedHTTPSOrigin(t *testing.T) {
	next := newSameOriginMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodDelete, "http://anthology.example.test/api/session", nil)
	req.Host = "anthology.example.test"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("Origin", "https://anthology.example.test")
	rec := httptest.NewRecorder()

	next.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", rec.Code)
	}
}

func TestSameOriginMiddlewareAllowsRefererFallback(t *testing.T) {
	next := newSameOriginMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "https://anthology.example.test/api/items", nil)
	req.Header.Set("Referer", "https://anthology.example.test/items/new")
	rec := httptest.NewRecorder()

	next.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", rec.Code)
	}
}

func TestSameOriginMiddlewareRejectsCrossSiteReferer(t *testing.T) {
	next := newSameOriginMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "https://anthology.example.test/api/items", nil)
	req.Header.Set("Referer", "https://evil.example.test/attack")
	rec := httptest.NewRecorder()

	next.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", rec.Code)
	}
}

func TestSameOriginMiddlewareIgnoresInvalidForwardedProto(t *testing.T) {
	next := newSameOriginMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "http://anthology.example.test/api/items", nil)
	req.Header.Set("X-Forwarded-Proto", "gopher")
	req.Header.Set("Origin", "http://anthology.example.test")
	rec := httptest.NewRecorder()

	next.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", rec.Code)
	}
}

func TestSameOriginMiddlewareRejectsCrossSiteOrigin(t *testing.T) {
	next := newSameOriginMiddleware([]string{"http://localhost:4200"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "http://localhost:8080/api/items", nil)
	req.Header.Set("Origin", "https://evil.example.test")
	rec := httptest.NewRecorder()

	next.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", rec.Code)
	}
}

func TestSlogMiddlewareEmitsDebugAccessLog(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	next := newSlogMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	next.ServeHTTP(rec, req)

	logs := buf.String()
	if !strings.Contains(logs, "http request") {
		t.Fatalf("log output = %q, want request log", logs)
	}
	if !strings.Contains(logs, "path=/health") {
		t.Fatalf("log output = %q, want path", logs)
	}
	if !strings.Contains(logs, "status=204") {
		t.Fatalf("log output = %q, want status", logs)
	}
	if !strings.Contains(logs, "level=DEBUG") {
		t.Fatalf("log output = %q, want debug level", logs)
	}
}

func TestSlogMiddlewareElevatesClientAndServerErrors(t *testing.T) {
	tests := []struct {
		name  string
		code  int
		level string
	}{
		{name: "client error", code: http.StatusNotFound, level: "level=WARN"},
		{name: "server error", code: http.StatusInternalServerError, level: "level=ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
			next := newSlogMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.code)
			}))

			req := httptest.NewRequest(http.MethodGet, "/missing", nil)
			rec := httptest.NewRecorder()

			next.ServeHTTP(rec, req)

			if logs := buf.String(); !strings.Contains(logs, tt.level) {
				t.Fatalf("log output = %q, want %s", logs, tt.level)
			}
		})
	}
}
