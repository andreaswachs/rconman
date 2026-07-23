package auth

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Middleware handles OIDC authentication and session management.
type Middleware struct {
	provider       *oidc.Provider
	config         *oauth2.Config
	sessionManager *SessionManager
	roleConfig     *RoleConfig
	allowlist      AllowlistConfig
	insecureMode   bool
}

// NewMiddleware creates a new OIDC middleware with provider and config.
func NewMiddleware(
	ctx context.Context,
	issuerURL string,
	clientID string,
	clientSecret string,
	baseURL string,
	sessionSecret string,
	sessionExpiry time.Duration,
	roleConfig *RoleConfig,
	allowlist AllowlistConfig,
	insecureMode bool,
) (*Middleware, error) {
	// Create OIDC provider
	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, err
	}

	// Create OAuth2 config
	config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  baseURL + "/auth/callback",
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}

	// Create session manager
	sessionManager := NewSessionManager(sessionSecret, sessionExpiry, insecureMode)

	return &Middleware{
		provider:       provider,
		config:         config,
		sessionManager: sessionManager,
		roleConfig:     roleConfig,
		allowlist:      allowlist,
		insecureMode:   insecureMode,
	}, nil
}

// isAllowed reports whether the given email is permitted to log in.
// If both Emails and Domains are empty, all logins are denied.
func (m *Middleware) isAllowed(email string) bool {
	email = strings.ToLower(email)
	if len(m.allowlist.Emails) == 0 && len(m.allowlist.Domains) == 0 {
		return false
	}
	for _, e := range m.allowlist.Emails {
		if strings.ToLower(e) == email {
			return true
		}
	}
	parts := strings.SplitN(email, "@", 2)
	if len(parts) == 2 {
		domain := parts[1]
		for _, d := range m.allowlist.Domains {
			if strings.ToLower(d) == domain {
				return true
			}
		}
	}
	return false
}

// contextKey is used for storing values in context.
type contextKey string

const sessionContextKey contextKey = "session"

// RequireAuth is middleware that checks for a valid session.
func (m *Middleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, err := m.sessionManager.GetSession(r)
		if err != nil {
			slog.Debug("unauthenticated request, redirecting to login", "path", r.URL.Path, "err", err)
			// No valid session, redirect to login
			http.Redirect(w, r, "/auth/login", http.StatusFound)
			return
		}

		slog.Debug("authenticated request", "path", r.URL.Path, "user", session.Email, "role", session.Role)
		// Store session in context and continue
		ctx := context.WithValue(r.Context(), sessionContextKey, session)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetSessionFromContext retrieves session from request context.
func GetSessionFromContext(r *http.Request) (*Session, bool) {
	session, ok := r.Context().Value(sessionContextKey).(*Session)
	return session, ok
}

// AuthCodeURL generates a PKCE authorization URL and stores the verifier in a short-lived cookie.
func (m *Middleware) AuthCodeURL(w http.ResponseWriter, r *http.Request) string {
	verifier := oauth2.GenerateVerifier()
	url := m.config.AuthCodeURL("state", oauth2.S256ChallengeOption(verifier))

	http.SetCookie(w, &http.Cookie{
		Name:     "pkce_verifier",
		Value:    verifier,
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   !m.insecureMode,
		SameSite: http.SameSiteLaxMode,
	})

	return url
}

// HandleCallback processes the OAuth2 callback, verifies PKCE, and returns user email and role.
func (m *Middleware) HandleCallback(w http.ResponseWriter, r *http.Request, code, state string) (string, string, error) {
	verifierCookie, err := r.Cookie("pkce_verifier")
	if err != nil {
		slog.Debug("PKCE verifier cookie not found", "err", err)
		return "", "", fmt.Errorf("missing PKCE verifier: %w", err)
	}

	// Clear the PKCE cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "pkce_verifier",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   !m.insecureMode,
		SameSite: http.SameSiteLaxMode,
	})

	slog.Debug("exchanging auth code for token")

	token, err := m.config.Exchange(r.Context(), code, oauth2.VerifierOption(verifierCookie.Value))
	if err != nil {
		slog.Debug("auth code exchange failed", "err", err)
		return "", "", fmt.Errorf("failed to exchange code: %w", err)
	}

	slog.Debug("auth code exchanged successfully, verifying ID token")

	// Get the ID token
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return "", "", fmt.Errorf("no id_token in token response")
	}

	// Verify the ID token
	oidcVerifier := m.provider.Verifier(&oidc.Config{ClientID: m.config.ClientID})
	idToken, err := oidcVerifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		slog.Debug("ID token verification failed", "err", err)
		return "", "", fmt.Errorf("failed to verify ID token: %w", err)
	}

	slog.Debug("ID token verified, extracting claims")

	// Extract claims
	var claims struct {
		Email string      `json:"email"`
		Name  string      `json:"name"`
		Roles interface{} `json:"roles"`
	}

	if err := idToken.Claims(&claims); err != nil {
		return "", "", fmt.Errorf("failed to extract claims: %w", err)
	}

	if claims.Email == "" {
		return "", "", fmt.Errorf("email claim not found")
	}

	slog.Debug("claims extracted", "email", claims.Email, "name", claims.Name)

	if !m.isAllowed(claims.Email) {
		slog.Warn("login denied: email not in allowlist", "email", claims.Email)
		return "", "", ErrLoginDenied
	}

	// Determine role based on claims
	claimsMap := map[string]interface{}{
		"email": claims.Email,
	}
	if claims.Roles != nil {
		claimsMap["roles"] = claims.Roles
	}

	role := DetermineRole(claimsMap, m.roleConfig)

	slog.Debug("authentication successful", "email", claims.Email, "role", role)
	return claims.Email, role, nil
}

// CreateSession creates an authenticated session for the user.
func (m *Middleware) CreateSession(w http.ResponseWriter, r *http.Request, email, role string) error {
	return m.sessionManager.CreateSession(w, r, email, role)
}

// ClearSession clears the user's session.
func (m *Middleware) ClearSession(w http.ResponseWriter, r *http.Request) error {
	return m.sessionManager.ClearSession(w, r)
}
