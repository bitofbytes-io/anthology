package auth

import "testing"

func TestAllowlistAllowsLogin(t *testing.T) {
	allowlist := NewAllowlist([]string{" Example.com ", "@team.test"}, []string{"Friend@Gmail.com"})

	tests := []struct {
		name   string
		claims *GoogleClaims
		want   bool
	}{
		{"listed email, any case", &GoogleClaims{Email: "friend@gmail.com"}, true},
		{"listed email ignores hd", &GoogleClaims{Email: "FRIEND@gmail.com", HostedDomain: "other.test"}, true},
		{"workspace account on allowed domain", &GoogleClaims{Email: "user@example.com", HostedDomain: "example.com"}, true},
		{"hd compared case-insensitively", &GoogleClaims{Email: "User@Example.com", HostedDomain: "EXAMPLE.COM"}, true},
		{"leading @ on domain entry is ignored", &GoogleClaims{Email: "user@team.test", HostedDomain: "team.test"}, true},
		{"consumer account with allowed email suffix but no hd", &GoogleClaims{Email: "user@example.com"}, false},
		{"hd of another domain", &GoogleClaims{Email: "user@example.com", HostedDomain: "evil.test"}, false},
		{"allowed hd but email on another domain", &GoogleClaims{Email: "user@evil.test", HostedDomain: "example.com"}, false},
		{"unlisted email", &GoogleClaims{Email: "user@other.test", HostedDomain: "other.test"}, false},
		{"nil claims", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := allowlist.AllowsLogin(tt.claims); got != tt.want {
				t.Fatalf("AllowsLogin(%+v) = %v, want %v", tt.claims, got, tt.want)
			}
		})
	}
}

func TestAllowlistAllowsEmail(t *testing.T) {
	allowlist := NewAllowlist([]string{"example.com"}, []string{"friend@gmail.com"})

	tests := []struct {
		email string
		want  bool
	}{
		{"friend@gmail.com", true},
		{" Friend@Gmail.com ", true},
		{"user@example.com", true},
		{"other@gmail.com", false},
		{"user@sub.example.com", false},
		{"example.com", false},
		{"@example.com", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := allowlist.AllowsEmail(tt.email); got != tt.want {
			t.Errorf("AllowsEmail(%q) = %v, want %v", tt.email, got, tt.want)
		}
	}
}

func TestEmptyAllowlistAllowsEveryone(t *testing.T) {
	allowlist := NewAllowlist(nil, []string{" "})
	if !allowlist.IsEmpty() {
		t.Fatal("expected blank entries to leave the allowlist empty")
	}
	if !allowlist.AllowsLogin(&GoogleClaims{Email: "user@other.test"}) {
		t.Fatal("expected empty allowlist to allow login")
	}
	if !allowlist.AllowsEmail("user@other.test") {
		t.Fatal("expected empty allowlist to allow email")
	}
}
