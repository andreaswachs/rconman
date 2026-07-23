package auth

import (
	"errors"
	"net/http"
	"time"

	"github.com/gorilla/sessions"
)

// Session represents a user session with email, role, and expiration.
type Session struct {
	Email     string
	Role      string
	ExpiresAt time.Time
}

// SessionManager handles session creation, retrieval, and clearing.
type SessionManager struct {
	store        sessions.Store
	sessionName  string
	expiry       time.Duration
	insecureMode bool
}

// NewSessionManager creates a new session manager with encrypted cookie storage.
func NewSessionManager(secret string, sessionExpiry time.Duration, insecureMode bool) *SessionManager {
	store := sessions.NewCookieStore([]byte(secret))
	return &SessionManager{
		store:        store,
		sessionName:  "rconman_session",
		expiry:       sessionExpiry,
		insecureMode: insecureMode,
	}
}

// CreateSession creates a new encrypted session cookie with email and role.
// A stale/undecodable existing cookie is ignored — we're overwriting all values.
func (sm *SessionManager) CreateSession(w http.ResponseWriter, r *http.Request, email, role string) error {
	session, _ := sm.store.Get(r, sm.sessionName)

	expiresAt := time.Now().Add(sm.expiry)

	session.Values["email"] = email
	session.Values["role"] = role
	session.Values["expires_at"] = expiresAt.Unix() // Store as Unix timestamp for gob compatibility

	session.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   int(sm.expiry.Seconds()),
		HttpOnly: true,
		Secure:   !sm.insecureMode, // Allow non-HTTPS in development
		SameSite: http.SameSiteLaxMode,
	}

	return session.Save(r, w)
}

// GetSession retrieves and validates a session from the request.
func (sm *SessionManager) GetSession(r *http.Request) (*Session, error) {
	session, err := sm.store.Get(r, sm.sessionName)
	if err != nil {
		return nil, err
	}

	if session.IsNew {
		return nil, errors.New("session not found")
	}

	email, ok := session.Values["email"].(string)
	if !ok {
		return nil, errors.New("invalid email in session")
	}

	role, ok := session.Values["role"].(string)
	if !ok {
		return nil, errors.New("invalid role in session")
	}

	expiresAtUnix, ok := session.Values["expires_at"].(int64)
	if !ok {
		return nil, errors.New("invalid expires_at in session")
	}
	expiresAt := time.Unix(expiresAtUnix, 0)

	if time.Now().After(expiresAt) {
		return nil, errors.New("session expired")
	}

	return &Session{
		Email:     email,
		Role:      role,
		ExpiresAt: expiresAt,
	}, nil
}

// ClearSession clears the session cookie.
// A stale/undecodable cookie is ignored — we're deleting it anyway.
func (sm *SessionManager) ClearSession(w http.ResponseWriter, r *http.Request) error {
	session, _ := sm.store.Get(r, sm.sessionName)

	session.Options.MaxAge = -1
	return session.Save(r, w)
}
