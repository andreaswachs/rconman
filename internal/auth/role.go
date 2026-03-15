package auth

type RoleConfig struct {
	ClaimName      string
	ClaimValue     string
	EmailAllowlist []string
}

// DetermineRole determines admin or viewer role based on claims.
func DetermineRole(claims map[string]interface{}, cfg *RoleConfig) string {
	// Check claim match first
	if cfg.ClaimName != "" && cfg.ClaimValue != "" {
		if val, ok := claims[cfg.ClaimName]; ok {
			if val == cfg.ClaimValue {
				return "admin"
			}
		}
	}

	// Check email allowlist
	if email, ok := claims["email"].(string); ok {
		for _, allowed := range cfg.EmailAllowlist {
			if email == allowed {
				return "admin"
			}
		}
	}

	// Default to viewer for any authenticated user
	return "viewer"
}

// IsAdmin returns true if role is admin.
func IsAdmin(role string) bool {
	return role == "admin"
}
