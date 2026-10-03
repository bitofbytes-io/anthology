package auth

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines the interface for user and session persistence.
type Repository interface {
	// User operations
	FindUserByOAuth(ctx context.Context, provider, providerID string) (*User, error)
	FindUserByEmail(ctx context.Context, email string) (*User, error)
	CreateUser(ctx context.Context, user User) (User, error)
	UpdateUserLogin(ctx context.Context, id uuid.UUID, email, name, avatarURL string) error

	// Session operations
	CreateSession(ctx context.Context, session Session, tokenHash string) error
	FindSessionByTokenHash(ctx context.Context, tokenHash string) (*Session, *User, error)
	DeleteSession(ctx context.Context, id uuid.UUID) error
	DeleteExpiredSessions(ctx context.Context) (int64, error)
}
