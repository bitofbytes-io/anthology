package auth

import (
	"time"

	"github.com/google/uuid"
)

// User represents an authenticated user in the system.
type User struct {
	ID              uuid.UUID
	Email           string
	Name            string
	AvatarURL       string
	OAuthProvider   string
	OAuthProviderID string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	LastLoginAt     time.Time
}

// Session represents an authenticated user session.
type Session struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	ExpiresAt time.Time
	CreatedAt time.Time
	UserAgent string
	IPAddress string
}

// GoogleClaims contains the relevant claims from a Google ID token.
type GoogleClaims struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
	// HostedDomain is Google's hd claim: the Workspace domain that manages the
	// account. It is empty for consumer Google accounts.
	HostedDomain string `json:"hd"`
}
