package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"anthology/internal/auth"
)

// Config aggregates runtime configuration for the Anthology services.
type Config struct {
	Environment       string
	HTTPPort          int
	DatabaseURL       string
	LogLevel          string
	AllowedOrigins    []string
	GoogleBooksAPIKey string

	// Google OAuth
	GoogleClientID       string
	GoogleClientSecret   string
	GoogleRedirectURL    string
	GoogleAllowedDomains []string
	GoogleAllowedEmails  []string
	FrontendURL          string
}

// Load reads configuration from environment variables, supporting secret-file fallbacks and sensible defaults for local development.
// 
// It validates and returns a Config populated from environment values (including parsing CSV lists and determining the HTTP port).
// Load enforces APP_ENV to be either "development" or "production", requires DATABASE_URL and GOOGLE_BOOKS_API_KEY, requires Google OAuth
// client ID and secret, and requires at least one of AUTH_GOOGLE_ALLOWED_DOMAINS or AUTH_GOOGLE_ALLOWED_EMAILS. It also sanitizes and
// deduplicates allowed origins and returns an error for invalid port values or other validation/read failures.
func Load() (Config, error) {
	databaseURL, err := getEnvOrFile("DATABASE_URL", "/run/secrets/anthology_database_url")
	if err != nil {
		return Config{}, err
	}

	googleBooksAPIKey, err := getEnvOrFile("GOOGLE_BOOKS_API_KEY", "/run/secrets/anthology_google_books_api_key")
	if err != nil {
		return Config{}, err
	}

	googleClientID, err := getEnvOrFile("AUTH_GOOGLE_CLIENT_ID", "/run/secrets/anthology_google_client_id")
	if err != nil {
		return Config{}, err
	}

	googleClientSecret, err := getEnvOrFile("AUTH_GOOGLE_CLIENT_SECRET", "/run/secrets/anthology_google_client_secret")
	if err != nil {
		return Config{}, err
	}

	trimmedGoogleClientID := strings.TrimSpace(googleClientID)
	trimmedGoogleClientSecret := strings.TrimSpace(googleClientSecret)

	environment := strings.TrimSpace(os.Getenv("APP_ENV"))
	if environment == "" {
		environment = "production"
	}
	environment = strings.ToLower(environment)
	if !isValidEnvironment(environment) {
		return Config{}, fmt.Errorf("APP_ENV must be one of development or production")
	}

	cfg := Config{
		Environment:       environment,
		DatabaseURL:       databaseURL,
		LogLevel:          strings.ToLower(getEnv("LOG_LEVEL", "info")),
		AllowedOrigins:    parseCSV(getEnv("ALLOWED_ORIGINS", "http://localhost:4200,http://localhost:8080")),
		GoogleBooksAPIKey: strings.TrimSpace(googleBooksAPIKey),

		// Google OAuth
		GoogleClientID:       trimmedGoogleClientID,
		GoogleClientSecret:   trimmedGoogleClientSecret,
		GoogleRedirectURL:    getEnv("AUTH_GOOGLE_REDIRECT_URL", "http://localhost:8080/api/auth/google/callback"),
		GoogleAllowedDomains: parseCSV(getEnv("AUTH_GOOGLE_ALLOWED_DOMAINS", "")),
		GoogleAllowedEmails:  parseCSV(getEnv("AUTH_GOOGLE_ALLOWED_EMAILS", "")),
		FrontendURL:          getEnv("FRONTEND_URL", "http://localhost:4200"),
	}

	portValue := getEnv("PORT", getEnv("HTTP_PORT", "8080"))
	port, err := strconv.Atoi(portValue)
	if err != nil {
		return Config{}, fmt.Errorf("invalid port %q: %w", portValue, err)
	}
	cfg.HTTPPort = port

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	if cfg.GoogleBooksAPIKey == "" {
		return Config{}, fmt.Errorf("GOOGLE_BOOKS_API_KEY is required")
	}

	// Google OAuth is required in all environments
	if cfg.GoogleClientID == "" {
		return Config{}, fmt.Errorf("AUTH_GOOGLE_CLIENT_ID is required")
	}
	if cfg.GoogleClientSecret == "" {
		return Config{}, fmt.Errorf("AUTH_GOOGLE_CLIENT_SECRET is required")
	}
	// Check the allowlist as auth normalizes it: an empty allowlist admits
	// everyone, so entries such as "@" that normalize away must not count.
	if auth.NewAllowlist(cfg.GoogleAllowedDomains, cfg.GoogleAllowedEmails).IsEmpty() {
		return Config{}, fmt.Errorf("AUTH_GOOGLE_ALLOWED_DOMAINS or AUTH_GOOGLE_ALLOWED_EMAILS is required")
	}

	allowedOrigins, err := sanitizeAllowedOrigins(cfg.AllowedOrigins, cfg.Environment)
	if err != nil {
		return Config{}, err
	}
	cfg.AllowedOrigins = allowedOrigins

	return cfg, nil
}

// HTTPAddress returns the address the HTTP server should bind to.
func (c Config) HTTPAddress() string {
	return fmt.Sprintf(":%d", c.HTTPPort)
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func parseCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// isValidEnvironment reports whether the provided environment string is one of the accepted values: "development" or "production".
// Comparison is case-insensitive and ignores surrounding whitespace.
func isValidEnvironment(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "development", "production":
		return true
	default:
		return false
	}
}

func sanitizeAllowedOrigins(origins []string, env string) ([]string, error) {
	cleaned := make([]string, 0, len(origins))
	seen := make(map[string]struct{}, len(origins))
	for _, origin := range enumerateOrigins(origins) {
		if strings.Contains(origin.value, "*") && !strings.EqualFold(env, "development") {
			return nil, fmt.Errorf("ALLOWED_ORIGINS cannot contain wildcard %q when APP_ENV=%s", origin.value, env)
		}

		if _, exists := seen[origin.key]; exists {
			continue
		}

		cleaned = append(cleaned, origin.value)
		seen[origin.key] = struct{}{}
	}

	if len(cleaned) == 0 {
		return nil, fmt.Errorf("ALLOWED_ORIGINS must define at least one origin when APP_ENV=%s", env)
	}

	return cleaned, nil
}

type originEntry struct {
	value string
	key   string
}

func enumerateOrigins(origins []string) []originEntry {
	entries := make([]originEntry, 0, len(origins))
	for _, raw := range origins {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		entries = append(entries, originEntry{
			value: trimmed,
			key:   strings.ToLower(trimmed),
		})
	}
	return entries
}

func getEnvOrFile(key, defaultPath string) (string, error) {
	if value := os.Getenv(key); value != "" {
		return value, nil
	}

	fileKey := key + "_FILE"
	if path := os.Getenv(fileKey); path != "" {
		return readSecret(path, fileKey)
	}

	if defaultPath != "" {
		return readSecret(defaultPath, key)
	}

	return "", nil
}

func readSecret(path, name string) (string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("config: reading %s (%s): %w", name, path, err)
	}

	value := strings.TrimSpace(string(contents))
	if value == "" {
		return "", fmt.Errorf("config: %s (%s) is empty", name, path)
	}
	return value, nil
}