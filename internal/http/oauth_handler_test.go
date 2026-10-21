package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"anthology/internal/auth"
	"anthology/internal/metrics"
)

// encodeOAuthState creates a base64-encoded JSON state payload for testing
func encodeOAuthState(state, redirectTo string) string {
	payload := oauthStatePayload{State: state, RedirectTo: redirectTo}
	data, _ := json.Marshal(payload)
	return base64.RawURLEncoding.EncodeToString(data)
}

type fakeGoogleAuthenticator struct {
	authURLBase    string
	lastState      string
	exchangeClaims *auth.GoogleClaims
	exchangeErr    error
	allowEmail     bool
}

func (f *fakeGoogleAuthenticator) AuthURL(state string) string {
	f.lastState = state
	if f.authURLBase == "" {
		f.authURLBase = "https://accounts.google.com/auth?state="
	}
	return f.authURLBase + state
}

func (f *fakeGoogleAuthenticator) Exchange(ctx context.Context, code string) (*auth.GoogleClaims, error) {
	if f.exchangeErr != nil {
		return nil, f.exchangeErr
	}
	return f.exchangeClaims, nil
}

func (f *fakeGoogleAuthenticator) IsAllowed(claims *auth.GoogleClaims) bool {
	return f.allowEmail
}

func TestOAuthInitiateGoogleSetsStateCookieAndRedirects(t *testing.T) {
	google := &fakeGoogleAuthenticator{allowEmail: true}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewOAuthHandler(google, nil, "http://frontend.test", "development", logger)
	handler.metrics = metrics.New()

	req := httptest.NewRequest(http.MethodGet, "/api/auth/google?redirectTo=/items", nil)
	rec := httptest.NewRecorder()

	handler.InitiateGoogle(rec, req)
	assertOAuthFailureMetric(t, handler.metrics, "")

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected status 307, got %d", rec.Code)
	}
	cookies := rec.Result().Cookies()
	var stateCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == oauthStateCookieName {
			stateCookie = c
			break
		}
	}
	if stateCookie == nil || stateCookie.Value == "" {
		t.Fatal("expected state cookie to be set")
	}

	// Decode the base64 JSON state to verify it contains the cookie value and redirectTo
	stateBytes, err := base64.RawURLEncoding.DecodeString(google.lastState)
	if err != nil {
		t.Fatalf("failed to decode state: %v", err)
	}
	var statePayload oauthStatePayload
	if err := json.Unmarshal(stateBytes, &statePayload); err != nil {
		t.Fatalf("failed to parse state JSON: %v", err)
	}
	if statePayload.State != stateCookie.Value {
		t.Fatalf("expected state to match cookie value %q, got %q", stateCookie.Value, statePayload.State)
	}
	if statePayload.RedirectTo != "/items" {
		t.Fatalf("expected redirectTo to be /items, got %q", statePayload.RedirectTo)
	}

	location := rec.Header().Get("Location")
	if location != google.authURLBase+google.lastState {
		t.Fatalf("expected redirect to %q, got %q", google.authURLBase+google.lastState, location)
	}
}

func TestOAuthCallbackRejectsMissingStateCookie(t *testing.T) {
	google := &fakeGoogleAuthenticator{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewOAuthHandler(google, nil, "http://frontend.test", "development", logger)
	handler.metrics = metrics.New()

	req := httptest.NewRequest(http.MethodGet, "/api/auth/google/callback?state=abc", nil)
	rec := httptest.NewRecorder()

	handler.CallbackGoogle(rec, req)
	assertOAuthFailureMetric(t, handler.metrics, "state")

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected status 307, got %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Location"), "/login?error=invalid_request") {
		t.Fatalf("expected invalid_request redirect, got %q", rec.Header().Get("Location"))
	}
}

func TestOAuthCallbackRejectsStateMismatch(t *testing.T) {
	google := &fakeGoogleAuthenticator{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewOAuthHandler(google, nil, "http://frontend.test", "development", logger)
	handler.metrics = metrics.New()

	// Encode state with wrong value
	encodedState := encodeOAuthState("other", "")
	req := httptest.NewRequest(http.MethodGet, "/api/auth/google/callback?state="+url.QueryEscape(encodedState), nil)
	req.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: "expected"})
	rec := httptest.NewRecorder()

	handler.CallbackGoogle(rec, req)
	assertOAuthFailureMetric(t, handler.metrics, "state")

	if !strings.Contains(rec.Header().Get("Location"), "/login?error=invalid_request") {
		t.Fatalf("expected invalid_request redirect, got %q", rec.Header().Get("Location"))
	}
}

func TestOAuthCallbackPropagatesProviderError(t *testing.T) {
	google := &fakeGoogleAuthenticator{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewOAuthHandler(google, nil, "http://frontend.test", "development", logger)
	handler.metrics = metrics.New()

	encodedState := encodeOAuthState("abc", "")
	req := httptest.NewRequest(http.MethodGet, "/api/auth/google/callback?state="+url.QueryEscape(encodedState)+"&error=access_denied&error_description=Denied", nil)
	req.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: "abc"})
	rec := httptest.NewRecorder()

	handler.CallbackGoogle(rec, req)
	assertOAuthFailureMetric(t, handler.metrics, "provider_denied")

	location := rec.Header().Get("Location")
	if !strings.Contains(location, "/login?error=access_denied") || !strings.Contains(location, "message=Denied") {
		t.Fatalf("expected provider error redirect, got %q", location)
	}
}

func TestOAuthCallbackRequiresCode(t *testing.T) {
	google := &fakeGoogleAuthenticator{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewOAuthHandler(google, nil, "http://frontend.test", "development", logger)
	handler.metrics = metrics.New()

	encodedState := encodeOAuthState("abc", "")
	req := httptest.NewRequest(http.MethodGet, "/api/auth/google/callback?state="+url.QueryEscape(encodedState), nil)
	req.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: "abc"})
	rec := httptest.NewRecorder()

	handler.CallbackGoogle(rec, req)
	assertOAuthFailureMetric(t, handler.metrics, "state")

	if !strings.Contains(rec.Header().Get("Location"), "/login?error=invalid_request") {
		t.Fatalf("expected invalid_request redirect, got %q", rec.Header().Get("Location"))
	}
}

func TestOAuthCallbackHandlesExchangeError(t *testing.T) {
	google := &fakeGoogleAuthenticator{exchangeErr: errors.New("boom")}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewOAuthHandler(google, nil, "http://frontend.test", "development", logger)
	handler.metrics = metrics.New()

	encodedState := encodeOAuthState("abc", "")
	req := httptest.NewRequest(http.MethodGet, "/api/auth/google/callback?state="+url.QueryEscape(encodedState)+"&code=123", nil)
	req.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: "abc"})
	rec := httptest.NewRecorder()

	handler.CallbackGoogle(rec, req)
	assertOAuthFailureMetric(t, handler.metrics, "exchange")

	if !strings.Contains(rec.Header().Get("Location"), "/login?error=exchange_error") {
		t.Fatalf("expected exchange_error redirect, got %q", rec.Header().Get("Location"))
	}
}

func TestOAuthCallbackRequiresVerifiedEmail(t *testing.T) {
	google := &fakeGoogleAuthenticator{
		exchangeClaims: &auth.GoogleClaims{Email: "user@example.com", EmailVerified: false},
		allowEmail:     true,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewOAuthHandler(google, nil, "http://frontend.test", "development", logger)
	handler.metrics = metrics.New()

	encodedState := encodeOAuthState("abc", "")
	req := httptest.NewRequest(http.MethodGet, "/api/auth/google/callback?state="+url.QueryEscape(encodedState)+"&code=123", nil)
	req.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: "abc"})
	rec := httptest.NewRecorder()

	handler.CallbackGoogle(rec, req)
	assertOAuthFailureMetric(t, handler.metrics, "not_allowed")

	if !strings.Contains(rec.Header().Get("Location"), "/login?error=email_not_verified") {
		t.Fatalf("expected email_not_verified redirect, got %q", rec.Header().Get("Location"))
	}
}

func TestOAuthCallbackRejectsUnauthorizedEmail(t *testing.T) {
	google := &fakeGoogleAuthenticator{
		exchangeClaims: &auth.GoogleClaims{Email: "user@example.com", EmailVerified: true},
		allowEmail:     false,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewOAuthHandler(google, nil, "http://frontend.test", "development", logger)
	handler.metrics = metrics.New()

	encodedState := encodeOAuthState("abc", "")
	req := httptest.NewRequest(http.MethodGet, "/api/auth/google/callback?state="+url.QueryEscape(encodedState)+"&code=123", nil)
	req.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: "abc"})
	rec := httptest.NewRecorder()

	handler.CallbackGoogle(rec, req)
	assertOAuthFailureMetric(t, handler.metrics, "not_allowed")

	if !strings.Contains(rec.Header().Get("Location"), "/login?error=access_denied") {
		t.Fatalf("expected access_denied redirect, got %q", rec.Header().Get("Location"))
	}
}

func TestOAuthCallbackHandlesUserCreationError(t *testing.T) {
	google := &fakeGoogleAuthenticator{
		exchangeClaims: &auth.GoogleClaims{Email: "user@example.com", EmailVerified: true, Sub: "sub"},
		allowEmail:     true,
	}
	repo := &authRepoStub{
		findUserByOAuth: func(ctx context.Context, provider, providerID string) (*auth.User, error) {
			return nil, errors.New("db down")
		},
	}
	authService := auth.NewService(repo, time.Hour, nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewOAuthHandler(google, authService, "http://frontend.test", "development", logger)
	handler.metrics = metrics.New()

	encodedState := encodeOAuthState("abc", "")
	req := httptest.NewRequest(http.MethodGet, "/api/auth/google/callback?state="+url.QueryEscape(encodedState)+"&code=123", nil)
	req.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: "abc"})
	rec := httptest.NewRecorder()

	handler.CallbackGoogle(rec, req)
	assertOAuthFailureMetric(t, handler.metrics, "internal")

	if !strings.Contains(rec.Header().Get("Location"), "/login?error=internal_error") {
		t.Fatalf("expected internal_error redirect, got %q", rec.Header().Get("Location"))
	}
}

func TestOAuthCallbackHandlesSessionCreationError(t *testing.T) {
	google := &fakeGoogleAuthenticator{
		exchangeClaims: &auth.GoogleClaims{Email: "user@example.com", EmailVerified: true, Sub: "sub"},
		allowEmail:     true,
	}
	repo := &authRepoStub{
		createUser: func(ctx context.Context, user auth.User) (auth.User, error) {
			return user, nil
		},
		createSession: func(ctx context.Context, session auth.Session, tokenHash string) error {
			return errors.New("session fail")
		},
	}
	authService := auth.NewService(repo, time.Hour, nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewOAuthHandler(google, authService, "http://frontend.test", "development", logger)
	handler.metrics = metrics.New()

	encodedState := encodeOAuthState("abc", "")
	req := httptest.NewRequest(http.MethodGet, "/api/auth/google/callback?state="+url.QueryEscape(encodedState)+"&code=123", nil)
	req.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: "abc"})
	rec := httptest.NewRecorder()

	handler.CallbackGoogle(rec, req)
	assertOAuthFailureMetric(t, handler.metrics, "internal")

	if !strings.Contains(rec.Header().Get("Location"), "/login?error=internal_error") {
		t.Fatalf("expected internal_error redirect, got %q", rec.Header().Get("Location"))
	}
}

func TestOAuthCallbackSuccessRedirectsToFrontend(t *testing.T) {
	google := &fakeGoogleAuthenticator{
		exchangeClaims: &auth.GoogleClaims{Email: "user@example.com", EmailVerified: true, Sub: "sub", Name: "User"},
		allowEmail:     true,
	}
	repo := &authRepoStub{
		createUser: func(ctx context.Context, user auth.User) (auth.User, error) {
			return user, nil
		},
	}
	authService := auth.NewService(repo, time.Hour, nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewOAuthHandler(google, authService, "http://frontend.test", "development", logger)
	handler.metrics = metrics.New()

	state := "state123"
	encodedState := encodeOAuthState(state, "/items")
	req := httptest.NewRequest(http.MethodGet, "/api/auth/google/callback?state="+url.QueryEscape(encodedState)+"&code=123", nil)
	req.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: state})
	rec := httptest.NewRecorder()

	handler.CallbackGoogle(rec, req)
	assertOAuthFailureMetric(t, handler.metrics, "")

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected status 307, got %d", rec.Code)
	}
	location := rec.Header().Get("Location")
	if location != "http://frontend.test/items" {
		t.Fatalf("expected redirect to frontend, got %q", location)
	}

	var sessionCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil || sessionCookie.Value == "" {
		t.Fatal("expected session cookie to be set")
	}
}

func TestOAuthCallbackSanitizesRedirectTo(t *testing.T) {
	google := &fakeGoogleAuthenticator{
		exchangeClaims: &auth.GoogleClaims{Email: "user@example.com", EmailVerified: true, Sub: "sub", Name: "User"},
		allowEmail:     true,
	}
	repo := &authRepoStub{
		createUser: func(ctx context.Context, user auth.User) (auth.User, error) {
			return user, nil
		},
	}
	authService := auth.NewService(repo, time.Hour, nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewOAuthHandler(google, authService, "http://frontend.test", "development", logger)
	handler.metrics = metrics.New()

	state := "state123"
	// The evil redirect URL should be rejected by isValidRedirectPath
	encodedState := encodeOAuthState(state, "https://evil.test")
	req := httptest.NewRequest(http.MethodGet, "/api/auth/google/callback?state="+url.QueryEscape(encodedState)+"&code=123", nil)
	req.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: state})
	rec := httptest.NewRecorder()

	handler.CallbackGoogle(rec, req)
	assertOAuthFailureMetric(t, handler.metrics, "")

	location := rec.Header().Get("Location")
	if location != "http://frontend.test/" {
		t.Fatalf("expected redirect to root, got %q", location)
	}
}

func TestIsValidRedirectPath(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		valid bool
	}{
		// Valid paths
		{"root", "/", true},
		{"simple path", "/items", true},
		{"nested path", "/items/123", true},
		{"path with query", "/items?page=1", true},
		{"path with fragment", "/items#section", true},

		// Invalid - empty
		{"empty string", "", false},

		// Invalid - absolute URLs / open redirect attempts
		{"http URL", "http://evil.com", false},
		{"https URL", "https://evil.com", false},
		{"protocol-relative", "//evil.com", false},
		{"protocol-relative with path", "//evil.com/path", false},

		// Invalid - encoded bypass attempts
		{"encoded double slash", "/%2f%2fevil.com", false},
		{"encoded slash", "/%2fevil.com", false},
		// Note: double-encoded is safe - after one decode it's /%2f%2fevil.com (literal path)
		{"double encoded is safe", "/%252f%252fevil.com", true},

		// Invalid - no leading slash
		{"no leading slash", "items", false},
		{"relative path", "items/123", false},

		// Invalid - other schemes
		{"javascript protocol", "javascript:alert(1)", false},
		{"data protocol", "data:text/html,<script>", false},

		// Edge cases
		{"backslash", "\\\\evil.com", false},
		{"mixed slashes", "/\\evil.com", true}, // This is OK - just a weird but safe path
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidRedirectPath(tt.path)
			if got != tt.valid {
				t.Errorf("isValidRedirectPath(%q) = %v, want %v", tt.path, got, tt.valid)
			}
		})
	}
}

func assertOAuthFailureMetric(t *testing.T, m *metrics.Metrics, reason string) {
	t.Helper()
	var samples []string
	for _, line := range strings.Split(scrapeMetrics(t, m), "\n") {
		if strings.HasPrefix(line, "anthology_oauth_failures_total{") {
			samples = append(samples, line)
		}
	}
	if reason == "" {
		if len(samples) != 0 {
			t.Fatalf("successful login recorded failures: %v", samples)
		}
		return
	}
	want := `anthology_oauth_failures_total{provider="google",reason="` + reason + `"} 1`
	if len(samples) != 1 || samples[0] != want {
		t.Fatalf("OAuth failure samples = %v, want exactly %q", samples, want)
	}
}

func TestOAuthCallbackMalformedStateMetrics(t *testing.T) {
	for _, state := range []string{"%%%", base64.RawURLEncoding.EncodeToString([]byte("not JSON"))} {
		t.Run(state, func(t *testing.T) {
			handler := NewOAuthHandler(&fakeGoogleAuthenticator{}, nil, "http://frontend.test", "development", slog.New(slog.NewTextHandler(io.Discard, nil)))
			handler.metrics = metrics.New()
			req := httptest.NewRequest(http.MethodGet, "/api/auth/google/callback?state="+url.QueryEscape(state)+"&code=private-code", nil)
			req.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: "expected"})
			rec := httptest.NewRecorder()
			handler.CallbackGoogle(rec, req)
			assertOAuthFailureMetric(t, handler.metrics, "state")
			if rec.Code != http.StatusTemporaryRedirect || !strings.Contains(rec.Header().Get("Location"), "/login?error=invalid_request") {
				t.Fatalf("unexpected state failure response: %d %s", rec.Code, rec.Header().Get("Location"))
			}
		})
	}
}
