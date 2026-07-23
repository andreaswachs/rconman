package e2e

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func baseURL() string {
	if u := os.Getenv("RCONMAN_URL"); u != "" {
		return u
	}
	return "http://localhost:8080"
}

func TestHealthEndpoint(t *testing.T) {
	resp, err := http.Get(baseURL() + "/health")
	if err != nil {
		t.Skipf("rconman not reachable: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestLoginPageRedirects(t *testing.T) {
	resp, err := http.Get(baseURL() + "/")
	if err != nil {
		t.Skipf("rconman not reachable: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Errorf("expected 302 redirect to login, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc != "/auth/login" {
		t.Errorf("expected redirect to /auth/login, got %q", loc)
	}
}

func TestLoginEndpointRedirectsToOIDC(t *testing.T) {
	resp, err := http.Get(baseURL() + "/auth/login")
	if err != nil {
		t.Skipf("rconman not reachable: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Errorf("expected 302, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.HasPrefix(loc, "https://") && !strings.HasPrefix(loc, "http://") {
		t.Errorf("expected redirect to OIDC provider, got %q", loc)
	}
}

func TestStaticAssetsServed(t *testing.T) {
	resp, err := http.Get(baseURL() + "/static/htmx.min.js")
	if err != nil {
		t.Skipf("rconman not reachable: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for htmx.min.js, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if len(body) < 1000 {
		t.Errorf("htmx.min.js too small: %d bytes", len(body))
	}
}

func TestMetricsEndpoint(t *testing.T) {
	resp, err := http.Get(baseURL() + "/metrics")
	if err != nil {
		t.Skipf("rconman not reachable: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Skipf("/metrics not available yet (expected in Phase 2), got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "rconman_server_desired_state") {
		t.Errorf("expected rconman_server_desired_state metric, got: %s", string(body))
	}
}

func TestCommandExecutionWithAuth(t *testing.T) {
	// Without auth, command execution should fail
	resp, err := http.Post(baseURL()+"/api/commands/my-server", "application/json",
		strings.NewReader(`{"command":"list"}`))
	if err != nil {
		t.Skipf("rconman not reachable: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Errorf("expected 302 redirect for unauthenticated command, got %d", resp.StatusCode)
	}
}

func TestServerStatusAPI(t *testing.T) {
	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Get(baseURL() + "/api/servers/my-server/status")
	if err != nil {
		t.Skipf("rconman not reachable: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Errorf("expected 302 for unauthenticated status request, got %d", resp.StatusCode)
	}
}

func TestMain(m *testing.M) {
	// Quick check if rconman is reachable — skip waiting if not
	for i := 0; i < 3; i++ {
		resp, err := http.Get(baseURL() + "/health")
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(time.Second)
	}

	fmt.Println("Starting e2e tests against", baseURL())
	os.Exit(m.Run())
}
