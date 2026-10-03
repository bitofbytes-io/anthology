package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// SessionCleanupInterval is how often expired sessions are purged.
const SessionCleanupInterval = 24 * time.Hour

// Service provides authentication business logic.
type Service struct {
	repo       Repository
	sessionTTL time.Duration
	allowed    func(email string) bool
}

// NewService creates a new auth Service. Sessions are validated against
// allowed on every request, so removing an account from the allowlist revokes
// its existing sessions. A nil allowed admits every user.
func NewService(repo Repository, sessionTTL time.Duration, allowed func(email string) bool) *Service {
	if sessionTTL == 0 {
		sessionTTL = 12 * time.Hour
	}
	return &Service{
		repo:       repo,
		sessionTTL: sessionTTL,
		allowed:    allowed,
	}
}

// CreateOrUpdateUser finds an existing user by OAuth credentials or creates a new one.
func (s *Service) CreateOrUpdateUser(ctx context.Context, claims *GoogleClaims) (*User, error) {
	existing, err := s.repo.FindUserByOAuth(ctx, "google", claims.Sub)
	if err != nil {
		return nil, fmt.Errorf("find user: %w", err)
	}

	if existing != nil {
		// Update last login and refresh the email and profile data. The email
		// must follow the account: sessions are re-checked against the
		// allowlist by stored email, so a stale address would revoke the new
		// session of an account whose Google email changed.
		if err := s.repo.UpdateUserLogin(ctx, existing.ID, claims.Email, claims.Name, claims.Picture); err != nil {
			return nil, fmt.Errorf("update user login: %w", err)
		}
		existing.Email = claims.Email
		existing.Name = claims.Name
		existing.AvatarURL = claims.Picture
		existing.LastLoginAt = time.Now()
		return existing, nil
	}

	// Create new user
	now := time.Now()
	newUser := User{
		ID:              uuid.New(),
		Email:           claims.Email,
		Name:            claims.Name,
		AvatarURL:       claims.Picture,
		OAuthProvider:   "google",
		OAuthProviderID: claims.Sub,
		CreatedAt:       now,
		UpdatedAt:       now,
		LastLoginAt:     now,
	}

	created, err := s.repo.CreateUser(ctx, newUser)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	return &created, nil
}

// CreateSession creates a new session for the given user and returns the session token.
func (s *Service) CreateSession(ctx context.Context, userID uuid.UUID, userAgent, ipAddress string) (string, error) {
	// Generate cryptographically secure session token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	token := base64.URLEncoding.EncodeToString(tokenBytes)
	tokenHash := hashToken(token)

	now := time.Now()
	session := Session{
		ID:        uuid.New(),
		UserID:    userID,
		ExpiresAt: now.Add(s.sessionTTL),
		CreatedAt: now,
		UserAgent: truncateString(userAgent, 512),
		IPAddress: truncateString(ipAddress, 45),
	}

	if err := s.repo.CreateSession(ctx, session, tokenHash); err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}

	return token, nil
}

// ValidateSession checks if the token is valid and returns the associated user.
func (s *Service) ValidateSession(ctx context.Context, token string) (*User, error) {
	if token == "" {
		return nil, nil
	}

	tokenHash := hashToken(token)
	session, user, err := s.repo.FindSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		return nil, fmt.Errorf("find session: %w", err)
	}

	if session == nil || user == nil {
		return nil, nil
	}

	// Drop sessions that have expired or whose account is no longer allowlisted.
	if time.Now().After(session.ExpiresAt) || (s.allowed != nil && !s.allowed(user.Email)) {
		_ = s.repo.DeleteSession(ctx, session.ID)
		return nil, nil
	}

	return user, nil
}

// DeleteSession removes the session associated with the given token.
func (s *Service) DeleteSession(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}

	tokenHash := hashToken(token)
	session, _, err := s.repo.FindSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		return fmt.Errorf("find session: %w", err)
	}

	if session == nil {
		return nil
	}

	return s.repo.DeleteSession(ctx, session.ID)
}

// CleanupExpiredSessions removes all expired sessions from the database.
func (s *Service) CleanupExpiredSessions(ctx context.Context) (int64, error) {
	return s.repo.DeleteExpiredSessions(ctx)
}

// ScheduleSessionCleanup deletes expired sessions at startup and then every
// interval until ctx is cancelled.
func (s *Service) ScheduleSessionCleanup(ctx context.Context, interval time.Duration, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	if interval <= 0 {
		interval = SessionCleanupInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if deleted, err := s.CleanupExpiredSessions(ctx); err != nil {
			if ctx.Err() == nil {
				logger.Error("expired session cleanup failed", "error", err)
			}
		} else if deleted > 0 {
			logger.Info("expired sessions deleted", "count", deleted)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// hashToken returns the SHA-256 hash of the token as a hex string.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// truncateString truncates a string to the given max length.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
