package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/your-org/rconman/internal/auth"
	"github.com/your-org/rconman/internal/config"
	"github.com/your-org/rconman/internal/rcon"
	"github.com/your-org/rconman/internal/rcon/mock"
)

func TestCommandHandlerExecute(t *testing.T) {
	// Setup
	rcons := make(map[string]rcon.Client)
	mockClient := mock.New()
	rcons["test-server"] = mockClient

	handler := NewCommandHandler(rcons, nil, &config.Config{})

	// Test missing server
	req := httptest.NewRequest("POST", "/api/commands/unknown-server", nil)
	w := httptest.NewRecorder()

	// Simulate chi URL param (simplified test)
	req.URL.RawQuery = "id=unknown-server"
	handler.Execute(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestStatusHandlerGetStatus(t *testing.T) {
	// Setup
	rcons := make(map[string]rcon.Client)
	mockClient := mock.New()
	rcons["test-server"] = mockClient

	handler := NewStatusHandler(rcons)

	// Test
	req := httptest.NewRequest("GET", "/api/status/test-server", nil)
	w := httptest.NewRecorder()

	handler.GetStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected content type application/json, got %s", contentType)
	}
}

func TestAuthHandlerLogin(t *testing.T) {
	// Setup
	ctx := context.Background()
	middleware, err := auth.NewMiddleware(
		ctx,
		"https://accounts.google.com",
		"test-client-id",
		"test-client-secret",
		"http://localhost:8080",
		"test-session-secret-32-bytes-long",
		24*time.Hour,
		&auth.RoleConfig{
			ClaimName:      "hd",
			ClaimValue:     "example.com",
			EmailAllowlist: []string{},
		},
		true, // insecureMode for testing
	)
	if err != nil {
		t.Skipf("skipping test due to OIDC provider initialization: %v", err)
	}

	handler := NewAuthHandler(&config.Config{}, middleware)

	// Test
	req := httptest.NewRequest("GET", "/auth/login", nil)
	w := httptest.NewRecorder()

	handler.Login(w, req)

	// Login should redirect to OIDC provider
	if w.Code != http.StatusFound {
		t.Errorf("expected status %d, got %d", http.StatusFound, w.Code)
	}

	// Check redirect location exists
	location := w.Header().Get("Location")
	if location == "" {
		t.Error("expected Location header, got empty string")
	}
}
