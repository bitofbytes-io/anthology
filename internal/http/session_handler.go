package http

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"anthology/internal/auth"
)

const (
	sessionCookieName = "anthology_session"
	sessionCookieTTL  = 12 * time.Hour
)

// SessionHandler manages OAuth-authenticated sessions using HttpOnly cookies.
type SessionHandler struct {
	authService  *auth.Service
	secureCookie bool
	logger       *slog.Logger
}

// NewSessionHandler returns a handler for OAuth session management.
func NewSessionHandler(authService *auth.Service, env string, logger *slog.Logger) *SessionHandler {
	return &SessionHandler{
		authService:  authService,
		secureCookie: !strings.EqualFold(env, "development"),
		logger:       logger,
	}
}

// Status reports whether the request holds a valid session and returns user info.
func (h *SessionHandler) Status(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"authenticated": false,
		})
		return
	}

	user, err := h.authService.ValidateSession(r.Context(), cookie.Value)
	if err != nil {
		h.logger.Error("session validation error", "error", err)
		writeJSON(w, http.StatusOK, map[string]any{
			"authenticated": false,
		})
		return
	}

	if user == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"authenticated": false,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"user": map[string]any{
			"id":        user.ID.String(),
			"email":     user.Email,
			"name":      user.Name,
			"avatarUrl": user.AvatarURL,
		},
	})
}

// Logout removes the session cookie and deletes the session from the database.
func (h *SessionHandler) Logout(w http.ResponseWriter, r *http.Request) {
	hadSession := false

	// Delete session from database if auth service is configured
	if h.authService != nil {
		cookie, err := r.Cookie(sessionCookieName)
		if err == nil && cookie.Value != "" {
			hadSession = true
			if err := h.authService.DeleteSession(r.Context(), cookie.Value); err != nil {
				h.logger.Error("failed to delete session", "error", err)
			}
		}
	}

	// Clear the cookie
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.secureCookie,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})

	h.logger.Info("session logout", "had_session", hadSession)
	w.WriteHeader(http.StatusNoContent)
}

// CurrentUser returns the authenticated user's information.
func (h *SessionHandler) CurrentUser(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())

	writeJSON(w, http.StatusOK, map[string]any{
		"id":        user.ID.String(),
		"email":     user.Email,
		"name":      user.Name,
		"avatarUrl": user.AvatarURL,
	})
}

// clientIPFromRequest extracts the originating IP address, normalizing away any port
// so rate limiting buckets attempts by IP regardless of ephemeral ports.
// Note: chi's RealIP middleware (configured in router.go) has already processed
// X-Forwarded-For and X-Real-IP headers and set r.RemoteAddr appropriately.
// We rely on that middleware instead of parsing headers directly to avoid spoofing.
func clientIPFromRequest(r *http.Request) string {
	ip := r.RemoteAddr

	host, _, err := net.SplitHostPort(ip)
	if err == nil {
		return host
	}

	return ip
}
