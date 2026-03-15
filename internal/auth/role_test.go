package auth

import (
	"testing"
)

func TestDetermineRole_ClaimMatch(t *testing.T) {
	cfg := &RoleConfig{
		ClaimName:  "roles",
		ClaimValue: "rconman-admin",
	}

	claims := map[string]interface{}{
		"roles": "rconman-admin",
		"email": "user@example.com",
	}

	role := DetermineRole(claims, cfg)
	if role != "admin" {
		t.Errorf("expected admin, got %s", role)
	}
}

func TestDetermineRole_EmailAllowlist(t *testing.T) {
	cfg := &RoleConfig{
		ClaimName:      "roles",
		ClaimValue:     "rconman-admin",
		EmailAllowlist: []string{"admin@example.com", "moderator@example.com"},
	}

	claims := map[string]interface{}{
		"roles": "user",
		"email": "admin@example.com",
	}

	role := DetermineRole(claims, cfg)
	if role != "admin" {
		t.Errorf("expected admin, got %s", role)
	}
}

func TestDetermineRole_Viewer(t *testing.T) {
	cfg := &RoleConfig{
		ClaimName:      "roles",
		ClaimValue:     "rconman-admin",
		EmailAllowlist: []string{"admin@example.com"},
	}

	claims := map[string]interface{}{
		"roles": "user",
		"email": "regular@example.com",
	}

	role := DetermineRole(claims, cfg)
	if role != "viewer" {
		t.Errorf("expected viewer, got %s", role)
	}
}
