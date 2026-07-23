package e2e

import (
	"net/http"
	"testing"
)

func TestOIDCLoginRedirectsToProvider(t *testing.T) {
	resp, err := http.Get(baseURL() + "/auth/login")
	if err != nil {
		t.Skipf("rconman not reachable: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Errorf("expected 302, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc == "" {
		t.Error("expected Location header")
	}
}
