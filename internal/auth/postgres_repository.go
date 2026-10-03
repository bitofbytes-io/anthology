package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// PostgresRepository implements Repository using PostgreSQL.
type PostgresRepository struct {
	db *sqlx.DB
}

// NewPostgresRepository creates a new PostgresRepository.
func NewPostgresRepository(db *sqlx.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

// FindUserByOAuth looks up a user by their OAuth provider and provider ID.
func (r *PostgresRepository) FindUserByOAuth(ctx context.Context, provider, providerID string) (*User, error) {
	const query = `
		SELECT id, email, name, avatar_url, oauth_provider, oauth_provider_id, created_at, updated_at, last_login_at
		FROM users
		WHERE oauth_provider = $1 AND oauth_provider_id = $2
	`

	var row userRow
	if err := r.db.GetContext(ctx, &row, query, provider, providerID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return row.toUser(), nil
}

// FindUserByEmail looks up a user by their email address.
func (r *PostgresRepository) FindUserByEmail(ctx context.Context, email string) (*User, error) {
	const query = `
		SELECT id, email, name, avatar_url, oauth_provider, oauth_provider_id, created_at, updated_at, last_login_at
		FROM users
		WHERE email = $1
	`

	var row userRow
	if err := r.db.GetContext(ctx, &row, query, email); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return row.toUser(), nil
}

// CreateUser inserts a new user into the database.
func (r *PostgresRepository) CreateUser(ctx context.Context, user User) (User, error) {
	const query = `
		INSERT INTO users (id, email, name, avatar_url, oauth_provider, oauth_provider_id, created_at, updated_at, last_login_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	_, err := r.db.ExecContext(ctx, query,
		user.ID,
		user.Email,
		user.Name,
		user.AvatarURL,
		user.OAuthProvider,
		user.OAuthProviderID,
		user.CreatedAt,
		user.UpdatedAt,
		user.LastLoginAt,
	)
	if err != nil {
		return User{}, err
	}

	return user, nil
}

// UpdateUserLogin updates the user's last login time and refreshes the email
// and profile data from the latest Google sign-in.
func (r *PostgresRepository) UpdateUserLogin(ctx context.Context, id uuid.UUID, email, name, avatarURL string) error {
	const query = `
		UPDATE users
		SET email = $2, name = $3, avatar_url = $4, last_login_at = $5, updated_at = $5
		WHERE id = $1
	`

	now := time.Now()
	_, err := r.db.ExecContext(ctx, query, id, email, name, avatarURL, now)
	return err
}

// CreateSession inserts a new session into the database.
func (r *PostgresRepository) CreateSession(ctx context.Context, session Session, tokenHash string) error {
	const query = `
		INSERT INTO user_sessions (id, user_id, session_token_hash, expires_at, created_at, user_agent, ip_address)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err := r.db.ExecContext(ctx, query,
		session.ID,
		session.UserID,
		tokenHash,
		session.ExpiresAt,
		session.CreatedAt,
		session.UserAgent,
		session.IPAddress,
	)
	return err
}

// FindSessionByTokenHash looks up a session and its associated user by token hash.
func (r *PostgresRepository) FindSessionByTokenHash(ctx context.Context, tokenHash string) (*Session, *User, error) {
	const query = `
		SELECT
			s.id, s.user_id, s.expires_at, s.created_at, s.user_agent, s.ip_address,
			u.id AS user_id, u.email, u.name, u.avatar_url, u.oauth_provider, u.oauth_provider_id,
			u.created_at AS user_created_at, u.updated_at AS user_updated_at, u.last_login_at
		FROM user_sessions s
		JOIN users u ON s.user_id = u.id
		WHERE s.session_token_hash = $1
	`

	var row sessionUserRow
	if err := r.db.GetContext(ctx, &row, query, tokenHash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	return row.toSession(), row.toUser(), nil
}

// DeleteSession removes a session from the database.
func (r *PostgresRepository) DeleteSession(ctx context.Context, id uuid.UUID) error {
	const query = `DELETE FROM user_sessions WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

// DeleteExpiredSessions removes all expired sessions.
func (r *PostgresRepository) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	const query = `DELETE FROM user_sessions WHERE expires_at < $1`
	result, err := r.db.ExecContext(ctx, query, time.Now())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// userRow is a database row representation of User.
type userRow struct {
	ID              uuid.UUID `db:"id"`
	Email           string    `db:"email"`
	Name            string    `db:"name"`
	AvatarURL       string    `db:"avatar_url"`
	OAuthProvider   string    `db:"oauth_provider"`
	OAuthProviderID string    `db:"oauth_provider_id"`
	CreatedAt       time.Time `db:"created_at"`
	UpdatedAt       time.Time `db:"updated_at"`
	LastLoginAt     time.Time `db:"last_login_at"`
}

func (r *userRow) toUser() *User {
	return &User{
		ID:              r.ID,
		Email:           r.Email,
		Name:            r.Name,
		AvatarURL:       r.AvatarURL,
		OAuthProvider:   r.OAuthProvider,
		OAuthProviderID: r.OAuthProviderID,
		CreatedAt:       r.CreatedAt,
		UpdatedAt:       r.UpdatedAt,
		LastLoginAt:     r.LastLoginAt,
	}
}

// sessionUserRow is a database row for the session + user join query.
type sessionUserRow struct {
	// Session fields
	ID        uuid.UUID `db:"id"`
	UserID    uuid.UUID `db:"user_id"`
	ExpiresAt time.Time `db:"expires_at"`
	CreatedAt time.Time `db:"created_at"`
	UserAgent string    `db:"user_agent"`
	IPAddress string    `db:"ip_address"`

	// User fields
	Email           string    `db:"email"`
	Name            string    `db:"name"`
	AvatarURL       string    `db:"avatar_url"`
	OAuthProvider   string    `db:"oauth_provider"`
	OAuthProviderID string    `db:"oauth_provider_id"`
	UserCreatedAt   time.Time `db:"user_created_at"`
	UserUpdatedAt   time.Time `db:"user_updated_at"`
	LastLoginAt     time.Time `db:"last_login_at"`
}

func (r *sessionUserRow) toSession() *Session {
	return &Session{
		ID:        r.ID,
		UserID:    r.UserID,
		ExpiresAt: r.ExpiresAt,
		CreatedAt: r.CreatedAt,
		UserAgent: r.UserAgent,
		IPAddress: r.IPAddress,
	}
}

func (r *sessionUserRow) toUser() *User {
	return &User{
		ID:              r.UserID,
		Email:           r.Email,
		Name:            r.Name,
		AvatarURL:       r.AvatarURL,
		OAuthProvider:   r.OAuthProvider,
		OAuthProviderID: r.OAuthProviderID,
		CreatedAt:       r.UserCreatedAt,
		UpdatedAt:       r.UserUpdatedAt,
		LastLoginAt:     r.LastLoginAt,
	}
}
