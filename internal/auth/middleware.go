package auth

import (
	"context"
	"fmt"
	"net/http"
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
		insecureMode:   insecureMode,
	}, nil
}

// contextKey is used for storing values in context.
type contextKey string

const sessionContextKey contextKey = "session"

// RequireAuth is middleware that checks for a valid session.
func (m *Middleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, err := m.sessionManager.GetSession(r)
		if err != nil {
			// No valid session, redirect to login
			http.Redirect(w, r, "/auth/login", http.StatusFound)
			return
		}

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

// AuthCodeURL returns the OAuth2 authorization URL.
func (m *Middleware) AuthCodeURL(ctx context.Context) string {
	return m.config.AuthCodeURL("state")
}

// HandleCallback processes the OAuth2 callback and returns user email and role.
func (m *Middleware) HandleCallback(ctx context.Context, code, state string) (string, string, error) {
	// Exchange code for token
	token, err := m.config.Exchange(ctx, code)
	if err != nil {
		return "", "", fmt.Errorf("failed to exchange code: %w", err)
	}

	// Get the ID token
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return "", "", fmt.Errorf("no id_token in token response")
	}

	// Verify the ID token
	verifier := m.provider.Verifier(&oidc.Config{ClientID: m.config.ClientID})
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return "", "", fmt.Errorf("failed to verify ID token: %w", err)
	}

	// Extract claims
	var claims struct {
		Email string        `json:"email"`
		Name  string        `json:"name"`
		Roles interface{}   `json:"roles"`
	}

	if err := idToken.Claims(&claims); err != nil {
		return "", "", fmt.Errorf("failed to extract claims: %w", err)
	}

	if claims.Email == "" {
		return "", "", fmt.Errorf("email claim not found")
	}

	// Determine role based on claims
	claimsMap := map[string]interface{}{
		"email": claims.Email,
	}
	if claims.Roles != nil {
		claimsMap["roles"] = claims.Roles
	}

	role := DetermineRole(claimsMap, m.roleConfig)

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
