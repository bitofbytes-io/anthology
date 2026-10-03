package auth

import "strings"

// Allowlist admits accounts by exact email address or by Google Workspace
// domain. An empty allowlist admits everyone (configuration requires at least
// one entry, so this only happens in tests).
type Allowlist struct {
	domains map[string]struct{}
	emails  map[string]struct{}
}

// NewAllowlist builds an Allowlist from domain and email entries. Entries are
// trimmed and lower-cased; a leading "@" on a domain is ignored.
func NewAllowlist(domains, emails []string) Allowlist {
	a := Allowlist{
		domains: make(map[string]struct{}, len(domains)),
		emails:  make(map[string]struct{}, len(emails)),
	}
	for _, d := range domains {
		d = strings.TrimPrefix(normalizeAddress(d), "@")
		if d != "" {
			a.domains[d] = struct{}{}
		}
	}
	for _, e := range emails {
		e = normalizeAddress(e)
		if e != "" {
			a.emails[e] = struct{}{}
		}
	}
	return a
}

// IsEmpty reports whether no domains or emails are configured.
func (a Allowlist) IsEmpty() bool {
	return len(a.domains) == 0 && len(a.emails) == 0
}

// AllowsLogin reports whether a Google sign-in may proceed. An exact email
// entry always matches. A domain entry matches only when Google's hosted
// domain (hd) claim names that domain and the email is on it, so a consumer
// Google account registered with a non-Gmail address on an allowed domain is
// rejected: only accounts managed by that Workspace domain get in.
func (a Allowlist) AllowsLogin(claims *GoogleClaims) bool {
	if claims == nil {
		return false
	}
	if a.IsEmpty() {
		return true
	}
	email := normalizeAddress(claims.Email)
	if _, ok := a.emails[email]; ok {
		return true
	}
	hostedDomain := normalizeAddress(claims.HostedDomain)
	if hostedDomain == "" || emailDomain(email) != hostedDomain {
		return false
	}
	_, ok := a.domains[hostedDomain]
	return ok
}

// AllowsEmail reports whether an existing account's email is still covered by
// the allowlist. Sessions are re-checked with it on every request, so removing
// an email or domain revokes that account's sessions. The hd claim was
// enforced at sign-in; only the email is stored, so this matches on its domain.
func (a Allowlist) AllowsEmail(email string) bool {
	if a.IsEmpty() {
		return true
	}
	email = normalizeAddress(email)
	if _, ok := a.emails[email]; ok {
		return true
	}
	domain := emailDomain(email)
	if domain == "" {
		return false
	}
	_, ok := a.domains[domain]
	return ok
}

func normalizeAddress(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// emailDomain returns the part after the single "@" in email, or "" when
// email is not of the form local@domain.
func emailDomain(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 || parts[0] == "" {
		return ""
	}
	return parts[1]
}
