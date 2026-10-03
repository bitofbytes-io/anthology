package http

import (
	"context"

	"anthology/internal/auth"

	"github.com/google/uuid"
)

type authRepoStub struct {
	findUserByOAuth       func(ctx context.Context, provider, providerID string) (*auth.User, error)
	createUser            func(ctx context.Context, user auth.User) (auth.User, error)
	updateUserLogin       func(ctx context.Context, id uuid.UUID, email, name, avatarURL string) error
	createSession         func(ctx context.Context, session auth.Session, tokenHash string) error
	findSessionByHash     func(ctx context.Context, tokenHash string) (*auth.Session, *auth.User, error)
	deleteSession         func(ctx context.Context, id uuid.UUID) error
	deleteExpiredSessions func(ctx context.Context) (int64, error)
}

func (r *authRepoStub) FindUserByOAuth(ctx context.Context, provider, providerID string) (*auth.User, error) {
	if r.findUserByOAuth != nil {
		return r.findUserByOAuth(ctx, provider, providerID)
	}
	return nil, nil
}

func (r *authRepoStub) FindUserByEmail(ctx context.Context, email string) (*auth.User, error) {
	return nil, nil
}

func (r *authRepoStub) CreateUser(ctx context.Context, user auth.User) (auth.User, error) {
	if r.createUser != nil {
		return r.createUser(ctx, user)
	}
	return user, nil
}

func (r *authRepoStub) UpdateUserLogin(ctx context.Context, id uuid.UUID, email, name, avatarURL string) error {
	if r.updateUserLogin != nil {
		return r.updateUserLogin(ctx, id, email, name, avatarURL)
	}
	return nil
}

func (r *authRepoStub) CreateSession(ctx context.Context, session auth.Session, tokenHash string) error {
	if r.createSession != nil {
		return r.createSession(ctx, session, tokenHash)
	}
	return nil
}

func (r *authRepoStub) FindSessionByTokenHash(ctx context.Context, tokenHash string) (*auth.Session, *auth.User, error) {
	if r.findSessionByHash != nil {
		return r.findSessionByHash(ctx, tokenHash)
	}
	return nil, nil, nil
}

func (r *authRepoStub) DeleteSession(ctx context.Context, id uuid.UUID) error {
	if r.deleteSession != nil {
		return r.deleteSession(ctx, id)
	}
	return nil
}

func (r *authRepoStub) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	if r.deleteExpiredSessions != nil {
		return r.deleteExpiredSessions(ctx)
	}
	return 0, nil
}
