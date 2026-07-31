package auth

import (
	"net/http/httptest"
	"testing"

	"golang.org/x/oauth2"
)

func TestAuthCodeURL_ReusesExistingVerifier(t *testing.T) {
	m := &Middleware{
		insecureMode: true,
		config: &oauth2.Config{
			ClientID:    "test",
			RedirectURL: "https://example.com/auth/callback",
			Endpoint:    oauth2.Endpoint{AuthURL: "https://idp.example.com/auth", TokenURL: "https://idp.example.com/token"},
		},
	}

	req := httptest.NewRequest("GET", "/auth/login", nil)
	resp := httptest.NewRecorder()
	firstURL := m.AuthCodeURL(resp, req)
	firstCookie := resp.Result().Cookies()[0]

	req = httptest.NewRequest("GET", "/auth/login", nil)
	req.AddCookie(firstCookie)
	resp = httptest.NewRecorder()
	secondURL := m.AuthCodeURL(resp, req)

	if firstURL != secondURL {
		t.Errorf("expected same auth URL on repeat login, got %q then %q", firstURL, secondURL)
	}
}
