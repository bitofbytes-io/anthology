package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// GoogleAuthenticator handles Google OAuth 2.0 / OIDC authentication.
type GoogleAuthenticator struct {
	config    *oauth2.Config
	verifier  *oidc.IDTokenVerifier
	allowlist Allowlist
}

// NewGoogleAuthenticator creates a new GoogleAuthenticator that admits sign-ins
// permitted by allowlist.
func NewGoogleAuthenticator(ctx context.Context, clientID, clientSecret, redirectURL string, allowlist Allowlist) (*GoogleAuthenticator, error) {
	provider, err := oidc.NewProvider(ctx, "https://accounts.google.com")
	if err != nil {
		return nil, fmt.Errorf("oidc provider: %w", err)
	}

	config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Endpoint:     google.Endpoint,
		Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: clientID})

	return &GoogleAuthenticator{
		config:    config,
		verifier:  verifier,
		allowlist: allowlist,
	}, nil
}

// AuthURL generates the Google OAuth consent URL with the given state.
func (g *GoogleAuthenticator) AuthURL(state string) string {
	return g.config.AuthCodeURL(
		state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "select_account"),
	)
}

// Exchange exchanges the authorization code for tokens and returns the user claims.
func (g *GoogleAuthenticator) Exchange(ctx context.Context, code string) (*GoogleClaims, error) {
	token, err := g.config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return nil, fmt.Errorf("no id_token in response")
	}

	idToken, err := g.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("verify id_token: %w", err)
	}

	var claims GoogleClaims
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("parse claims: %w", err)
	}

	return &claims, nil
}

// IsAllowed reports whether the signed-in Google account passes the allowlist.
// Domain entries require Google's hd claim, not just the email suffix.
func (g *GoogleAuthenticator) IsAllowed(claims *GoogleClaims) bool {
	return g.allowlist.AllowsLogin(claims)
}

// HasAllowlist returns true if any allowlist restrictions are configured.
func (g *GoogleAuthenticator) HasAllowlist() bool {
	return !g.allowlist.IsEmpty()
}

// GenerateState generates a cryptographically secure random state string.
func GenerateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
