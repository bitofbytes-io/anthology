package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"log/slog"

	"anthology/internal/auth"

	"github.com/google/uuid"
)

func TestSessionHandlerStatusWithNoCookie(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	authService := auth.NewService(&authRepoStub{}, time.Hour, nil)
	handler := NewSessionHandler(authService, "development", logger)

	req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	rec := httptest.NewRecorder()

	handler.Status(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var response map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response["authenticated"] != false {
		t.Fatalf("expected authenticated=false without cookie, got %v", response["authenticated"])
	}
}

func TestSessionHandlerStatusWithValidSession(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	expectedUser := &auth.User{ID: uuid.New(), Email: "user@example.com", Name: "User", AvatarURL: "avatar.png"}
	repo := &authRepoStub{
		findSessionByHash: func(ctx context.Context, tokenHash string) (*auth.Session, *auth.User, error) {
			return &auth.Session{ID: uuid.New(), ExpiresAt: time.Now().Add(time.Minute)}, expectedUser, nil
		},
	}
	authService := auth.NewService(repo, time.Hour, nil)
	handler := NewSessionHandler(authService, "development", logger)

	req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "token"})
	rec := httptest.NewRecorder()

	handler.Status(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var response map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response["authenticated"] != true {
		t.Fatalf("expected authenticated=true, got %v", response["authenticated"])
	}
	user, ok := response["user"].(map[string]any)
	if !ok {
		t.Fatalf("expected user object, got %T", response["user"])
	}
	if user["email"] != expectedUser.Email {
		t.Fatalf("expected user email %q, got %v", expectedUser.Email, user["email"])
	}
}

func TestSessionHandlerStatusWithInvalidSession(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := &authRepoStub{
		findSessionByHash: func(ctx context.Context, tokenHash string) (*auth.Session, *auth.User, error) {
			return nil, nil, nil
		},
	}
	authService := auth.NewService(repo, time.Hour, nil)
	handler := NewSessionHandler(authService, "development", logger)

	req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "token"})
	rec := httptest.NewRecorder()

	handler.Status(rec, req)

	var response map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response["authenticated"] != false {
		t.Fatalf("expected authenticated=false, got %v", response["authenticated"])
	}
}

func TestSessionHandlerLogoutDeletesSession(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var deletedID uuid.UUID
	sessionID := uuid.New()
	repo := &authRepoStub{
		findSessionByHash: func(ctx context.Context, tokenHash string) (*auth.Session, *auth.User, error) {
			return &auth.Session{ID: sessionID, ExpiresAt: time.Now().Add(time.Minute)}, &auth.User{ID: uuid.New()}, nil
		},
		deleteSession: func(ctx context.Context, id uuid.UUID) error {
			deletedID = id
			return nil
		},
	}
	authService := auth.NewService(repo, time.Hour, nil)
	handler := NewSessionHandler(authService, "development", logger)

	req := httptest.NewRequest(http.MethodDelete, "/api/session", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "token"})
	rec := httptest.NewRecorder()

	handler.Logout(rec, req)

	if deletedID != sessionID {
		t.Fatalf("expected session %s to be deleted, got %s", sessionID, deletedID)
	}
}

func TestClientIPFromRequestRemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.1:12345"

	ip := clientIPFromRequest(req)
	if ip != "192.0.2.1" {
		t.Fatalf("expected 192.0.2.1, got %s", ip)
	}
}

func TestClientIPFromRequestIgnoresForwardedHeaders(t *testing.T) {
	// Forwarding headers are client-controlled, so only RemoteAddr counts.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "198.51.100.7:8080"
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	req.Header.Set("X-Real-IP", "203.0.113.9")

	ip := clientIPFromRequest(req)
	if ip != "198.51.100.7" {
		t.Fatalf("expected 198.51.100.7, got %s", ip)
	}
}

func TestClientIPFromRequestNoPort(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.1"

	ip := clientIPFromRequest(req)
	if ip != "192.0.2.1" {
		t.Fatalf("expected 192.0.2.1, got %s", ip)
	}
}
