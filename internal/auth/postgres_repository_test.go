package auth

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"anthology/internal/platform/testdb"
)

func TestPostgresRepositorySessionExpiryAndDeletion(t *testing.T) {
	db := testdb.Open(t)
	repo := NewPostgresRepository(db)
	svc := NewService(repo, time.Hour, nil)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	user, err := repo.CreateUser(ctx, User{
		ID:              uuid.New(),
		Email:           uuid.NewString() + "@example.com",
		Name:            "Session Tester",
		OAuthProvider:   "test",
		OAuthProviderID: uuid.NewString(),
		CreatedAt:       now,
		UpdatedAt:       now,
		LastLoginAt:     now,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM users WHERE id=$1`, user.ID) })

	createSession := func(token string, expiresAt time.Time) Session {
		t.Helper()
		session := Session{ID: uuid.New(), UserID: user.ID, ExpiresAt: expiresAt, CreatedAt: now, UserAgent: "test", IPAddress: "127.0.0.1"}
		if err := repo.CreateSession(ctx, session, hashToken(token)); err != nil {
			t.Fatalf("create session: %v", err)
		}
		return session
	}
	sessionExists := func(token string) bool {
		t.Helper()
		session, _, err := repo.FindSessionByTokenHash(ctx, hashToken(token))
		if err != nil {
			t.Fatalf("find session: %v", err)
		}
		return session != nil
	}

	validToken := "valid-token-" + user.ID.String()
	expiredToken := "expired-token-" + user.ID.String()
	sweptToken := "swept-token-" + user.ID.String()
	valid := createSession(validToken, now.Add(time.Hour))
	createSession(expiredToken, now.Add(-time.Minute))
	createSession(sweptToken, now.Add(-time.Hour))

	session, found, err := repo.FindSessionByTokenHash(ctx, hashToken(validToken))
	if err != nil || session == nil || found == nil {
		t.Fatalf("find valid session = %+v, %+v, %v", session, found, err)
	}
	if session.ID != valid.ID || !session.ExpiresAt.Equal(valid.ExpiresAt) || found.ID != user.ID || found.Email != user.Email {
		t.Fatalf("unexpected session/user: %+v %+v", session, found)
	}

	if missing, missingUser, err := repo.FindSessionByTokenHash(ctx, hashToken("unknown-"+uuid.NewString())); err != nil || missing != nil || missingUser != nil {
		t.Fatalf("unknown token = %+v, %+v, %v; want nil, nil, nil", missing, missingUser, err)
	}

	// An expired session is rejected and removed on validation.
	if got, err := svc.ValidateSession(ctx, expiredToken); err != nil || got != nil {
		t.Fatalf("ValidateSession(expired) = %+v, %v; want nil", got, err)
	}
	if sessionExists(expiredToken) {
		t.Fatal("expired session should be deleted after validation")
	}
	if got, err := svc.ValidateSession(ctx, validToken); err != nil || got == nil || got.ID != user.ID {
		t.Fatalf("ValidateSession(valid) = %+v, %v; want user %s", got, err, user.ID)
	}

	// The sweeper removes expired sessions but keeps live ones.
	deleted, err := repo.DeleteExpiredSessions(ctx)
	if err != nil || deleted < 1 {
		t.Fatalf("DeleteExpiredSessions = %d, %v; want at least 1", deleted, err)
	}
	if sessionExists(sweptToken) {
		t.Fatal("expired session should be removed by DeleteExpiredSessions")
	}
	if !sessionExists(validToken) {
		t.Fatal("unexpired session should survive DeleteExpiredSessions")
	}

	// Logging out deletes the session.
	if err := svc.DeleteSession(ctx, validToken); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if sessionExists(validToken) {
		t.Fatal("session should be gone after DeleteSession")
	}
	if got, err := svc.ValidateSession(ctx, validToken); err != nil || got != nil {
		t.Fatalf("ValidateSession after logout = %+v, %v; want nil", got, err)
	}
}
