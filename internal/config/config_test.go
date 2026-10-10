package config

import (
	"strings"
	"testing"

	"anthology/internal/auth"
)

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("PORT", "8080")
	t.Setenv("GOOGLE_BOOKS_API_KEY", "test-key")
	t.Setenv("AUTH_GOOGLE_CLIENT_ID", "client-id")
	t.Setenv("AUTH_GOOGLE_CLIENT_SECRET", "client-secret")
	t.Setenv("AUTH_GOOGLE_ALLOWED_EMAILS", "test@example.com")
	t.Setenv("DATABASE_URL", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when DATABASE_URL is missing")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRequiresOAuthClientID(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("PORT", "8080")
	t.Setenv("GOOGLE_BOOKS_API_KEY", "test-key")
	t.Setenv("AUTH_GOOGLE_CLIENT_ID", "")
	t.Setenv("AUTH_GOOGLE_CLIENT_SECRET", "client-secret")
	t.Setenv("AUTH_GOOGLE_ALLOWED_EMAILS", "test@example.com")
	t.Setenv("DATABASE_URL", "postgres://localhost/test")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when OAuth client ID is missing")
	}
	if !strings.Contains(err.Error(), "AUTH_GOOGLE_CLIENT_ID is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRequiresOAuthClientSecret(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("PORT", "8080")
	t.Setenv("GOOGLE_BOOKS_API_KEY", "test-key")
	t.Setenv("AUTH_GOOGLE_CLIENT_ID", "client-id")
	t.Setenv("AUTH_GOOGLE_CLIENT_SECRET", "")
	t.Setenv("AUTH_GOOGLE_ALLOWED_EMAILS", "test@example.com")
	t.Setenv("DATABASE_URL", "postgres://localhost/test")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when OAuth client secret is missing")
	}
	if !strings.Contains(err.Error(), "AUTH_GOOGLE_CLIENT_SECRET is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRequiresAllowlist(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("PORT", "8080")
	t.Setenv("GOOGLE_BOOKS_API_KEY", "test-key")
	t.Setenv("AUTH_GOOGLE_CLIENT_ID", "client-id")
	t.Setenv("AUTH_GOOGLE_CLIENT_SECRET", "client-secret")
	t.Setenv("AUTH_GOOGLE_ALLOWED_DOMAINS", "")
	t.Setenv("AUTH_GOOGLE_ALLOWED_EMAILS", "")
	t.Setenv("DATABASE_URL", "postgres://localhost/test")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when allowlist is missing")
	}
	if !strings.Contains(err.Error(), "AUTH_GOOGLE_ALLOWED_DOMAINS or AUTH_GOOGLE_ALLOWED_EMAILS is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadAcceptsValidConfig(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("PORT", "8080")
	t.Setenv("GOOGLE_BOOKS_API_KEY", "test-key")
	t.Setenv("AUTH_GOOGLE_CLIENT_ID", "client-id")
	t.Setenv("AUTH_GOOGLE_CLIENT_SECRET", "client-secret")
	t.Setenv("AUTH_GOOGLE_ALLOWED_DOMAINS", "example.com")
	t.Setenv("DATABASE_URL", "postgres://localhost/test")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.GoogleClientID != "client-id" {
		t.Fatalf("expected Google client ID to be preserved, got %q", cfg.GoogleClientID)
	}
	if cfg.DatabaseURL != "postgres://localhost/test" {
		t.Fatalf("expected database URL to be preserved, got %q", cfg.DatabaseURL)
	}
}

func TestLoadRejectsWildcardOriginsOutsideDevelopment(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("PORT", "8080")
	t.Setenv("GOOGLE_BOOKS_API_KEY", "test-key")
	t.Setenv("AUTH_GOOGLE_CLIENT_ID", "client-id")
	t.Setenv("AUTH_GOOGLE_CLIENT_SECRET", "client-secret")
	t.Setenv("AUTH_GOOGLE_ALLOWED_DOMAINS", "example.com")
	t.Setenv("ALLOWED_ORIGINS", "https://example.com,*")
	t.Setenv("DATABASE_URL", "postgres://localhost/test")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when ALLOWED_ORIGINS contains wildcard")
	}
	if !strings.Contains(err.Error(), "cannot contain wildcard") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRequiresAllowedOriginsOutsideDevelopment(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("PORT", "8080")
	t.Setenv("GOOGLE_BOOKS_API_KEY", "test-key")
	t.Setenv("AUTH_GOOGLE_CLIENT_ID", "client-id")
	t.Setenv("AUTH_GOOGLE_CLIENT_SECRET", "client-secret")
	t.Setenv("AUTH_GOOGLE_ALLOWED_DOMAINS", "example.com")
	t.Setenv("ALLOWED_ORIGINS", "   ")
	t.Setenv("DATABASE_URL", "postgres://localhost/test")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when ALLOWED_ORIGINS is empty")
	}
	if !strings.Contains(err.Error(), "must define at least one origin") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadDefaultsToProduction(t *testing.T) {
	t.Setenv("APP_ENV", "")
	t.Setenv("PORT", "8080")
	t.Setenv("GOOGLE_BOOKS_API_KEY", "test-key")
	t.Setenv("AUTH_GOOGLE_CLIENT_ID", "client-id")
	t.Setenv("AUTH_GOOGLE_CLIENT_SECRET", "client-secret")
	t.Setenv("AUTH_GOOGLE_ALLOWED_DOMAINS", "example.com")
	t.Setenv("ALLOWED_ORIGINS", "https://example.com")
	t.Setenv("DATABASE_URL", "postgres://localhost/test")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.Environment != "production" {
		t.Fatalf("expected APP_ENV to default to production, got %q", cfg.Environment)
	}
}

func setAllowlistTestEnv(t *testing.T, domains, emails string) {
	t.Helper()
	t.Setenv("APP_ENV", "development")
	t.Setenv("PORT", "8080")
	t.Setenv("HTTP_PORT", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("ALLOWED_ORIGINS", "")
	t.Setenv("FRONTEND_URL", "")
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("DATABASE_URL_FILE", "")
	t.Setenv("GOOGLE_BOOKS_API_KEY", "test-key")
	t.Setenv("GOOGLE_BOOKS_API_KEY_FILE", "")
	t.Setenv("AUTH_GOOGLE_CLIENT_ID", "client-id")
	t.Setenv("AUTH_GOOGLE_CLIENT_ID_FILE", "")
	t.Setenv("AUTH_GOOGLE_CLIENT_SECRET", "client-secret")
	t.Setenv("AUTH_GOOGLE_CLIENT_SECRET_FILE", "")
	t.Setenv("AUTH_GOOGLE_REDIRECT_URL", "")
	t.Setenv("AUTH_GOOGLE_ALLOWED_DOMAINS", domains)
	t.Setenv("AUTH_GOOGLE_ALLOWED_EMAILS", emails)
}

func TestLoadRejectsAllowlistWithNoUsableEntries(t *testing.T) {
	tests := []struct {
		name    string
		domains string
		emails  string
	}{
		{"bare @ domain", "@", ""},
		{"padded @ domain", "  @  ", ""},
		{"repeated @ domains in CSV", "@, @ ,,", ""},
		{"@ domain with blank emails", "@", " , "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setAllowlistTestEnv(t, tt.domains, tt.emails)

			// These entries normalize to an empty allowlist, which auth
			// deliberately treats as unrestricted, so Load must reject them.
			allowlist := auth.NewAllowlist(parseCSV(tt.domains), parseCSV(tt.emails))
			if !allowlist.IsEmpty() {
				t.Fatal("expected entries to normalize to an empty allowlist")
			}
			unrelated := &auth.GoogleClaims{Email: "user@other.test", EmailVerified: true}
			if !allowlist.AllowsLogin(unrelated) || !allowlist.AllowsEmail(unrelated.Email) {
				t.Fatal("expected the empty allowlist fallback to admit an unrelated account")
			}

			_, err := Load()
			if err == nil {
				t.Fatal("expected error when allowlist has no usable entries")
			}
			if !strings.Contains(err.Error(), "AUTH_GOOGLE_ALLOWED_DOMAINS or AUTH_GOOGLE_ALLOWED_EMAILS is required") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestLoadAllowlistConstrainsGoogleAccounts(t *testing.T) {
	type account struct {
		name      string
		claims    auth.GoogleClaims
		wantLogin bool
		wantEmail bool
	}
	tests := []struct {
		name     string
		domains  string
		emails   string
		accounts []account
	}{
		{
			name:    "leading @ domain",
			domains: "@example.com",
			accounts: []account{
				{"workspace account on domain", auth.GoogleClaims{Email: "user@example.com", HostedDomain: "example.com"}, true, true},
				{"hd compared case-insensitively", auth.GoogleClaims{Email: "User@Example.com", HostedDomain: "EXAMPLE.COM"}, true, true},
				{"missing hd", auth.GoogleClaims{Email: "user@example.com"}, false, true},
				{"mismatched hd", auth.GoogleClaims{Email: "user@example.com", HostedDomain: "other.test"}, false, true},
				{"hd without matching email", auth.GoogleClaims{Email: "user@other.test", HostedDomain: "example.com"}, false, false},
				{"unrelated account", auth.GoogleClaims{Email: "user@other.test", HostedDomain: "other.test"}, false, false},
				{"subdomain account", auth.GoogleClaims{Email: "user@sub.example.com", HostedDomain: "sub.example.com"}, false, false},
			},
		},
		{
			name:   "exact email normalized",
			emails: " Friend@Gmail.com ",
			accounts: []account{
				{"listed email without hd", auth.GoogleClaims{Email: "friend@gmail.com"}, true, true},
				{"listed email in other case with foreign hd", auth.GoogleClaims{Email: " FRIEND@gmail.COM ", HostedDomain: "other.test"}, true, true},
				{"unlisted email on same domain", auth.GoogleClaims{Email: "other@gmail.com"}, false, false},
			},
		},
		{
			name:    "malformed @ domain with valid email",
			domains: "@",
			emails:  "friend@gmail.com",
			accounts: []account{
				{"listed email", auth.GoogleClaims{Email: "friend@gmail.com"}, true, true},
				{"unlisted consumer account", auth.GoogleClaims{Email: "user@gmail.com"}, false, false},
				{"unrelated workspace account", auth.GoogleClaims{Email: "user@other.test", HostedDomain: "other.test"}, false, false},
			},
		},
		{
			name:    "malformed @ domain with valid domain",
			domains: "@, example.com",
			accounts: []account{
				{"workspace account on domain", auth.GoogleClaims{Email: "user@example.com", HostedDomain: "example.com"}, true, true},
				{"missing hd", auth.GoogleClaims{Email: "user@example.com"}, false, true},
				{"unrelated workspace account", auth.GoogleClaims{Email: "user@other.test", HostedDomain: "other.test"}, false, false},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setAllowlistTestEnv(t, tt.domains, tt.emails)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() returned error: %v", err)
			}
			allowlist := auth.NewAllowlist(cfg.GoogleAllowedDomains, cfg.GoogleAllowedEmails)
			for _, a := range tt.accounts {
				claims := a.claims
				claims.EmailVerified = true
				if got := allowlist.AllowsLogin(&claims); got != a.wantLogin {
					t.Errorf("%s: AllowsLogin(%+v) = %v, want %v", a.name, claims, got, a.wantLogin)
				}
				if got := allowlist.AllowsEmail(claims.Email); got != a.wantEmail {
					t.Errorf("%s: AllowsEmail(%q) = %v, want %v", a.name, claims.Email, got, a.wantEmail)
				}
			}
		})
	}
}
