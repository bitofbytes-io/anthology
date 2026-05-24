package http

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"anthology/internal/auth"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func newSlogMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(recorder, r)
			duration := time.Since(start)
			logger.Info("http request", "method", r.Method, "path", r.URL.Path, "status", recorder.status, "duration", duration.String())
		})
	}
}

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

const userContextKey contextKey = "user"

// UserFromContext extracts the authenticated user from the request context.
// Returns nil if the auth middleware hasn't populated the context.
func UserFromContext(ctx context.Context) *auth.User {
	user, _ := ctx.Value(userContextKey).(*auth.User)
	return user
}

func newAuthMiddleware(authService *auth.Service, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check session cookie
			cookie, err := r.Cookie(sessionCookieName)
			if err != nil || cookie.Value == "" {
				unauthorized(w)
				return
			}

			// Validate session
			user, err := authService.ValidateSession(r.Context(), cookie.Value)
			if err != nil {
				logger.Error("session validation error", "error", err)
				unauthorized(w)
				return
			}

			if user == nil {
				unauthorized(w)
				return
			}

			// Inject user into context
			ctx := context.WithValue(r.Context(), userContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func newSameOriginMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins)+1)
	for _, origin := range allowedOrigins {
		if normalized := normalizeOrigin(origin); normalized != "" {
			allowed[normalized] = struct{}{}
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isUnsafeMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			if originAllowed(r, allowed) {
				next.ServeHTTP(w, r)
				return
			}

			writeError(w, http.StatusForbidden, "invalid cross-site request")
		})
	}
}

func originAllowed(r *http.Request, allowed map[string]struct{}) bool {
	requestOrigin := requestOrigin(r)

	if origin := r.Header.Get("Origin"); origin != "" {
		return originMatches(origin, requestOrigin, allowed)
	}

	if referer := r.Header.Get("Referer"); referer != "" {
		return originMatches(referer, requestOrigin, allowed)
	}

	return false
}

func originMatches(raw string, requestOrigin string, allowed map[string]struct{}) bool {
	normalized := normalizeOrigin(raw)
	if normalized == "" {
		return false
	}
	if requestOrigin != "" && normalized == requestOrigin {
		return true
	}
	if _, ok := allowed["*"]; ok {
		return true
	}
	_, ok := allowed[normalized]
	return ok
}

func requestOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwardedProto := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0])); forwardedProto == "http" || forwardedProto == "https" {
		scheme = forwardedProto
	}
	return normalizeOrigin(scheme + "://" + r.Host)
}

func normalizeOrigin(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "null") {
		return ""
	}
	if raw == "*" {
		return "*"
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host)
}

func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	writeError(w, http.StatusUnauthorized, "authentication required")
}

func newSecurityHeadersMiddleware(environment string) func(http.Handler) http.Handler {
	isDev := strings.EqualFold(environment, "development")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("X-XSS-Protection", "1; mode=block")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			w.Header().Set("Permissions-Policy", "geolocation=(), camera=(), microphone=()")

			if !isDev {
				w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}

			next.ServeHTTP(w, r)
		})
	}
}
