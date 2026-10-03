package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type repoStub struct {
	findUserByOAuth       func(ctx context.Context, provider, providerID string) (*User, error)
	createUser            func(ctx context.Context, user User) (User, error)
	updateUserLogin       func(ctx context.Context, id uuid.UUID, email, name, avatarURL string) error
	createSession         func(ctx context.Context, session Session, tokenHash string) error
	findSessionByHash     func(ctx context.Context, tokenHash string) (*Session, *User, error)
	deleteSession         func(ctx context.Context, id uuid.UUID) error
	deleteExpiredSessions func(ctx context.Context) (int64, error)
}

func (r *repoStub) FindUserByOAuth(ctx context.Context, provider, providerID string) (*User, error) {
	if r.findUserByOAuth != nil {
		return r.findUserByOAuth(ctx, provider, providerID)
	}
	return nil, nil
}

func (r *repoStub) FindUserByEmail(ctx context.Context, email string) (*User, error) {
	return nil, nil
}

func (r *repoStub) CreateUser(ctx context.Context, user User) (User, error) {
	if r.createUser != nil {
		return r.createUser(ctx, user)
	}
	return user, nil
}

func (r *repoStub) UpdateUserLogin(ctx context.Context, id uuid.UUID, email, name, avatarURL string) error {
	if r.updateUserLogin != nil {
		return r.updateUserLogin(ctx, id, email, name, avatarURL)
	}
	return nil
}

func (r *repoStub) CreateSession(ctx context.Context, session Session, tokenHash string) error {
	if r.createSession != nil {
		return r.createSession(ctx, session, tokenHash)
	}
	return nil
}

func (r *repoStub) FindSessionByTokenHash(ctx context.Context, tokenHash string) (*Session, *User, error) {
	if r.findSessionByHash != nil {
		return r.findSessionByHash(ctx, tokenHash)
	}
	return nil, nil, nil
}

func (r *repoStub) DeleteSession(ctx context.Context, id uuid.UUID) error {
	if r.deleteSession != nil {
		return r.deleteSession(ctx, id)
	}
	return nil
}

func (r *repoStub) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	if r.deleteExpiredSessions != nil {
		return r.deleteExpiredSessions(ctx)
	}
	return 0, nil
}

func TestServiceCreateOrUpdateUserExisting(t *testing.T) {
	userID := uuid.New()
	existing := &User{
		ID:              userID,
		Email:           "old@example.com",
		Name:            "Old Name",
		AvatarURL:       "old.png",
		OAuthProvider:   "google",
		OAuthProviderID: "sub-123",
	}
	var updatedEmail, updatedName, updatedAvatar string

	repo := &repoStub{
		findUserByOAuth: func(ctx context.Context, provider, providerID string) (*User, error) {
			return existing, nil
		},
		updateUserLogin: func(ctx context.Context, id uuid.UUID, email, name, avatarURL string) error {
			if id != userID {
				return errors.New("unexpected id")
			}
			updatedEmail = email
			updatedName = name
			updatedAvatar = avatarURL
			return nil
		},
	}
	svc := NewService(repo, time.Hour, nil)

	claims := &GoogleClaims{
		Sub:     "sub-123",
		Email:   "user@example.com",
		Name:    "New Name",
		Picture: "new.png",
	}

	user, err := svc.CreateOrUpdateUser(context.Background(), claims)
	if err != nil {
		t.Fatalf("CreateOrUpdateUser returned error: %v", err)
	}
	if user.Name != "New Name" || user.AvatarURL != "new.png" {
		t.Fatalf("expected updated profile, got name=%q avatar=%q", user.Name, user.AvatarURL)
	}
	if updatedName != "New Name" || updatedAvatar != "new.png" {
		t.Fatalf("expected UpdateUserLogin to be called with new profile, got name=%q avatar=%q", updatedName, updatedAvatar)
	}
	// The stored email follows the Google account, so the session allowlist
	// check sees the address that was just approved at sign-in.
	if updatedEmail != "user@example.com" || user.Email != "user@example.com" {
		t.Fatalf("expected email to be refreshed to user@example.com, got stored=%q returned=%q", updatedEmail, user.Email)
	}
}

func TestServiceCreateOrUpdateUserCreatesNew(t *testing.T) {
	var created User
	repo := &repoStub{
		findUserByOAuth: func(ctx context.Context, provider, providerID string) (*User, error) {
			return nil, nil
		},
		createUser: func(ctx context.Context, user User) (User, error) {
			created = user
			return user, nil
		},
	}
	svc := NewService(repo, time.Hour, nil)

	claims := &GoogleClaims{
		Sub:     "sub-999",
		Email:   "new@example.com",
		Name:    "New User",
		Picture: "avatar.png",
	}

	user, err := svc.CreateOrUpdateUser(context.Background(), claims)
	if err != nil {
		t.Fatalf("CreateOrUpdateUser returned error: %v", err)
	}
	if created.ID == uuid.Nil {
		t.Fatal("expected CreateUser to receive a user ID")
	}
	if created.Email != claims.Email || created.Name != claims.Name || created.AvatarURL != claims.Picture {
		t.Fatalf("expected CreateUser to receive claims data, got %+v", created)
	}
	if user.Email != claims.Email || user.OAuthProvider != "google" || user.OAuthProviderID != claims.Sub {
		t.Fatalf("unexpected created user: %+v", user)
	}
}

func TestServiceCreateOrUpdateUserFindError(t *testing.T) {
	repo := &repoStub{
		findUserByOAuth: func(ctx context.Context, provider, providerID string) (*User, error) {
			return nil, errors.New("boom")
		},
	}
	svc := NewService(repo, time.Hour, nil)

	_, err := svc.CreateOrUpdateUser(context.Background(), &GoogleClaims{Sub: "sub"})
	if err == nil || !strings.Contains(err.Error(), "find user") {
		t.Fatalf("expected find user error, got %v", err)
	}
}

func TestServiceCreateSessionStoresHash(t *testing.T) {
	var storedHash string
	var storedSession Session
	repo := &repoStub{
		createSession: func(ctx context.Context, session Session, tokenHash string) error {
			storedHash = tokenHash
			storedSession = session
			return nil
		},
	}
	svc := NewService(repo, time.Hour, nil)

	longUA := strings.Repeat("a", 600)
	longIP := strings.Repeat("b", 60)
	userID := uuid.New()
	token, err := svc.CreateSession(context.Background(), userID, longUA, longIP)
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if token == "" {
		t.Fatal("expected token to be returned")
	}
	if storedHash != hashToken(token) {
		t.Fatalf("expected token hash to match, got %q", storedHash)
	}
	if storedSession.UserID != userID {
		t.Fatalf("expected session user ID %s, got %s", userID, storedSession.UserID)
	}
	if len(storedSession.UserAgent) != 512 {
		t.Fatalf("expected user agent to be truncated to 512, got %d", len(storedSession.UserAgent))
	}
	if len(storedSession.IPAddress) != 45 {
		t.Fatalf("expected ip address to be truncated to 45, got %d", len(storedSession.IPAddress))
	}
}

func TestServiceValidateSessionEmptyToken(t *testing.T) {
	svc := NewService(&repoStub{}, time.Hour, nil)

	user, err := svc.ValidateSession(context.Background(), "")
	if err != nil {
		t.Fatalf("ValidateSession returned error: %v", err)
	}
	if user != nil {
		t.Fatalf("expected no user, got %+v", user)
	}
}

func TestServiceValidateSessionExpired(t *testing.T) {
	var deletedID uuid.UUID
	repo := &repoStub{
		findSessionByHash: func(ctx context.Context, tokenHash string) (*Session, *User, error) {
			return &Session{ID: uuid.New(), ExpiresAt: time.Now().Add(-time.Minute)}, &User{ID: uuid.New()}, nil
		},
		deleteSession: func(ctx context.Context, id uuid.UUID) error {
			deletedID = id
			return nil
		},
	}
	svc := NewService(repo, time.Hour, nil)

	user, err := svc.ValidateSession(context.Background(), "token")
	if err != nil {
		t.Fatalf("ValidateSession returned error: %v", err)
	}
	if user != nil {
		t.Fatalf("expected expired session to return nil user, got %+v", user)
	}
	if deletedID == uuid.Nil {
		t.Fatal("expected expired session to be deleted")
	}
}

func TestServiceValidateSessionValid(t *testing.T) {
	expected := &User{ID: uuid.New(), Email: "user@example.com"}
	repo := &repoStub{
		findSessionByHash: func(ctx context.Context, tokenHash string) (*Session, *User, error) {
			return &Session{ID: uuid.New(), ExpiresAt: time.Now().Add(time.Minute)}, expected, nil
		},
	}
	svc := NewService(repo, time.Hour, nil)

	user, err := svc.ValidateSession(context.Background(), "token")
	if err != nil {
		t.Fatalf("ValidateSession returned error: %v", err)
	}
	if user != expected {
		t.Fatalf("expected user to be returned")
	}
}

func TestServiceValidateSessionRevokesAccountRemovedFromAllowlist(t *testing.T) {
	var deletedID uuid.UUID
	sessionID := uuid.New()
	repo := &repoStub{
		findSessionByHash: func(ctx context.Context, tokenHash string) (*Session, *User, error) {
			return &Session{ID: sessionID, ExpiresAt: time.Now().Add(time.Hour)}, &User{ID: uuid.New(), Email: "removed@example.com"}, nil
		},
		deleteSession: func(ctx context.Context, id uuid.UUID) error {
			deletedID = id
			return nil
		},
	}
	allowlist := NewAllowlist(nil, []string{"kept@example.com"})
	svc := NewService(repo, time.Hour, allowlist.AllowsEmail)

	user, err := svc.ValidateSession(context.Background(), "token")
	if err != nil {
		t.Fatalf("ValidateSession returned error: %v", err)
	}
	if user != nil {
		t.Fatalf("expected no user for an account no longer allowlisted, got %+v", user)
	}
	if deletedID != sessionID {
		t.Fatalf("expected session %s to be deleted, got %s", sessionID, deletedID)
	}
}

func TestServiceValidateSessionKeepsAllowlistedAccount(t *testing.T) {
	expected := &User{ID: uuid.New(), Email: "Kept@Example.com"}
	repo := &repoStub{
		findSessionByHash: func(ctx context.Context, tokenHash string) (*Session, *User, error) {
			return &Session{ID: uuid.New(), ExpiresAt: time.Now().Add(time.Hour)}, expected, nil
		},
		deleteSession: func(ctx context.Context, id uuid.UUID) error {
			t.Fatal("allowlisted session must not be deleted")
			return nil
		},
	}
	allowlist := NewAllowlist(nil, []string{"kept@example.com"})
	svc := NewService(repo, time.Hour, allowlist.AllowsEmail)

	user, err := svc.ValidateSession(context.Background(), "token")
	if err != nil {
		t.Fatalf("ValidateSession returned error: %v", err)
	}
	if user != expected {
		t.Fatalf("expected allowlisted user to be returned, got %+v", user)
	}
}

func TestServiceValidateSessionRepoError(t *testing.T) {
	repo := &repoStub{
		findSessionByHash: func(ctx context.Context, tokenHash string) (*Session, *User, error) {
			return nil, nil, errors.New("boom")
		},
	}
	svc := NewService(repo, time.Hour, nil)

	_, err := svc.ValidateSession(context.Background(), "token")
	if err == nil || !strings.Contains(err.Error(), "find session") {
		t.Fatalf("expected find session error, got %v", err)
	}
}

func TestServiceDeleteSession(t *testing.T) {
	var deletedID uuid.UUID
	sessionID := uuid.New()
	repo := &repoStub{
		findSessionByHash: func(ctx context.Context, tokenHash string) (*Session, *User, error) {
			return &Session{ID: sessionID}, &User{ID: uuid.New()}, nil
		},
		deleteSession: func(ctx context.Context, id uuid.UUID) error {
			deletedID = id
			return nil
		},
	}
	svc := NewService(repo, time.Hour, nil)

	if err := svc.DeleteSession(context.Background(), "token"); err != nil {
		t.Fatalf("DeleteSession returned error: %v", err)
	}
	if deletedID != sessionID {
		t.Fatalf("expected session %s to be deleted, got %s", sessionID, deletedID)
	}
}

func TestServiceDeleteSessionMissing(t *testing.T) {
	repo := &repoStub{
		findSessionByHash: func(ctx context.Context, tokenHash string) (*Session, *User, error) {
			return nil, nil, nil
		},
	}
	svc := NewService(repo, time.Hour, nil)

	if err := svc.DeleteSession(context.Background(), "token"); err != nil {
		t.Fatalf("DeleteSession returned error: %v", err)
	}
}

func TestServiceCleanupExpiredSessions(t *testing.T) {
	repo := &repoStub{
		deleteExpiredSessions: func(ctx context.Context) (int64, error) {
			return 3, nil
		},
	}
	svc := NewService(repo, time.Hour, nil)

	count, err := svc.CleanupExpiredSessions(context.Background())
	if err != nil {
		t.Fatalf("CleanupExpiredSessions returned error: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 expired sessions removed, got %d", count)
	}
}

func TestServiceScheduleSessionCleanupRunsAtStartupAndOnEachTick(t *testing.T) {
	calls := make(chan struct{}, 10)
	repo := &repoStub{
		deleteExpiredSessions: func(ctx context.Context) (int64, error) {
			calls <- struct{}{}
			return 1, nil
		},
	}
	svc := NewService(repo, time.Hour, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		svc.ScheduleSessionCleanup(ctx, 10*time.Millisecond, nil)
		close(done)
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-calls:
		case <-time.After(2 * time.Second):
			t.Fatalf("expected cleanup run %d", i+1)
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ScheduleSessionCleanup did not stop after cancel")
	}
}
