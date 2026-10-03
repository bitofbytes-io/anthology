package auth

import (
	"encoding/json"
	"net/url"
	"testing"

	"golang.org/x/oauth2"
)

func TestHasAllowlist(t *testing.T) {
	if (&GoogleAuthenticator{}).HasAllowlist() {
		t.Fatal("expected HasAllowlist to be false")
	}

	authenticator := &GoogleAuthenticator{allowlist: NewAllowlist([]string{"example.com"}, nil)}
	if !authenticator.HasAllowlist() {
		t.Fatal("expected HasAllowlist to be true")
	}
}

func TestIsAllowedUsesAllowlist(t *testing.T) {
	authenticator := &GoogleAuthenticator{allowlist: NewAllowlist([]string{"example.com"}, nil)}

	if !authenticator.IsAllowed(&GoogleClaims{Email: "user@example.com", HostedDomain: "example.com"}) {
		t.Fatal("expected Workspace account on an allowed domain to be allowed")
	}
	if authenticator.IsAllowed(&GoogleClaims{Email: "user@example.com"}) {
		t.Fatal("expected account without an hd claim to be rejected by a domain rule")
	}
}

func TestGenerateState(t *testing.T) {
	state1, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState returned error: %v", err)
	}
	state2, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState returned error: %v", err)
	}
	if state1 == "" || state2 == "" {
		t.Fatal("expected non-empty state")
	}
	if state1 == state2 {
		t.Fatal("expected unique state values")
	}
}

func TestAuthURLIncludesPromptSelectAccount(t *testing.T) {
	authenticator := &GoogleAuthenticator{
		config: &oauth2.Config{
			ClientID:     "client-id",
			RedirectURL:  "http://localhost/callback",
			Endpoint:     oauth2.Endpoint{AuthURL: "https://auth.test/oauth"},
			Scopes:       []string{"openid"},
			ClientSecret: "secret",
		},
	}

	authURL := authenticator.AuthURL("state123")
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("failed to parse auth URL: %v", err)
	}

	if prompt := parsed.Query().Get("prompt"); prompt != "select_account" {
		t.Fatalf("expected prompt=select_account, got %q", prompt)
	}
}

func TestGoogleClaimsDecodeHostedDomain(t *testing.T) {
	var claims GoogleClaims
	if err := json.Unmarshal([]byte(`{"sub":"1","email":"user@example.com","hd":"example.com"}`), &claims); err != nil {
		t.Fatalf("unmarshal claims: %v", err)
	}
	if claims.HostedDomain != "example.com" {
		t.Fatalf("expected hd claim example.com, got %q", claims.HostedDomain)
	}
}
