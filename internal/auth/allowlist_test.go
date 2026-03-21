package auth

import "testing"

// newTestMiddlewareWithAllowlist creates a Middleware with only the allowlist
// field set. Safe for testing isAllowed, which does not touch other fields.
func newTestMiddlewareWithAllowlist(cfg AllowlistConfig) *Middleware {
	return &Middleware{allowlist: cfg}
}

func TestIsAllowed_EmptyConfigDeniesAll(t *testing.T) {
	m := newTestMiddlewareWithAllowlist(AllowlistConfig{})
	if m.isAllowed("anyone@example.com") {
		t.Error("expected empty allowlist to deny all, got allowed")
	}
}

func TestIsAllowed_ExactEmailMatch(t *testing.T) {
	m := newTestMiddlewareWithAllowlist(AllowlistConfig{
		Emails: []string{"alice@example.com"},
	})
	if !m.isAllowed("alice@example.com") {
		t.Error("expected alice@example.com to be allowed")
	}
}

func TestIsAllowed_ExactEmailMatchCaseInsensitive(t *testing.T) {
	m := newTestMiddlewareWithAllowlist(AllowlistConfig{
		Emails: []string{"alice@example.com"},
	})
	if !m.isAllowed("Alice@Example.COM") {
		t.Error("expected case-insensitive email match to be allowed")
	}
}

func TestIsAllowed_DomainMatch(t *testing.T) {
	m := newTestMiddlewareWithAllowlist(AllowlistConfig{
		Domains: []string{"wachs.software"},
	})
	if !m.isAllowed("user@wachs.software") {
		t.Error("expected user@wachs.software to be allowed")
	}
}

func TestIsAllowed_DomainMatchCaseInsensitive(t *testing.T) {
	m := newTestMiddlewareWithAllowlist(AllowlistConfig{
		Domains: []string{"wachs.software"},
	})
	if !m.isAllowed("user@WACHS.SOFTWARE") {
		t.Error("expected case-insensitive domain match to be allowed")
	}
}

func TestIsAllowed_NoMatch(t *testing.T) {
	m := newTestMiddlewareWithAllowlist(AllowlistConfig{
		Emails:  []string{"alice@example.com"},
		Domains: []string{"wachs.software"},
	})
	if m.isAllowed("stranger@other.com") {
		t.Error("expected stranger@other.com to be denied")
	}
}

func TestIsAllowed_EmailWithNoAtSign(t *testing.T) {
	m := newTestMiddlewareWithAllowlist(AllowlistConfig{
		Domains: []string{"example.com"},
	})
	// Must not panic; no @ means domain extraction fails gracefully
	if m.isAllowed("notanemail") {
		t.Error("expected malformed email to be denied")
	}
}
