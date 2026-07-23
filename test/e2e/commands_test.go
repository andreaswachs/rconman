package e2e

import (
	"net/http"
	"strings"
	"testing"
)

func TestExecuteCommandUnauthenticated(t *testing.T) {
	resp, err := http.Post(baseURL()+"/api/commands/my-server", "application/json",
		strings.NewReader(`{"command":"list"}`))
	if err != nil {
		t.Skipf("rconman not reachable: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Errorf("expected 302 for unauthenticated request, got %d", resp.StatusCode)
	}
}
