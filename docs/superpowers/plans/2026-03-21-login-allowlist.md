# Login Allowlist Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restrict OIDC login to a configurable set of email addresses and/or email domains, denying all logins when neither list is configured.

**Architecture:** A new `auth.AllowlistConfig` type (mirroring `auth.RoleConfig`) holds the allowlist and is stored on `Middleware`. After extracting the email from the OIDC token, `HandleCallback` calls `isAllowed` before returning; on denial it returns `ErrLoginDenied`. The `Callback` handler redirects to `/auth/login?error=access_denied`, and the `Login` handler renders a new standalone error page for that query parameter.

**Tech Stack:** Go 1.25, `github.com/a-h/templ` (views), chi router, `go-oidc/v3`, Helm 3

---

## File Map

| Action | Path | Responsibility |
|--------|------|---------------|
| Create | `internal/auth/allowlist.go` | `AllowlistConfig` type, `ErrLoginDenied` sentinel |
| Modify | `internal/auth/middleware.go` | Add `allowlist` field, update `NewMiddleware`, add `isAllowed`, update `HandleCallback` |
| Create | `internal/auth/allowlist_test.go` | Unit tests for `isAllowed` |
| Modify | `internal/config/config.go` | Add `AllowlistConfig` struct and `Allowlist` field to `AuthConfig` |
| Modify | `cmd/rconman/main.go` | Pass `auth.AllowlistConfig` to `NewMiddleware` |
| Create | `internal/views/error.templ` | `LoginErrorPage` templ component |
| Modify | `internal/handlers/handlers.go` | Update `Login` and `Callback` handlers |
| Modify | `internal/handlers/handlers_test.go` | Fix compile break, add new test cases |
| Modify | `helm/rconman/values.yaml` | Add `config.auth.allowlist` |
| Modify | `helm/rconman/templates/configmap.yaml` | Render allowlist into config.yaml |

---

### Task 1: Define `AllowlistConfig` type and `ErrLoginDenied` sentinel

**Files:**
- Create: `internal/auth/allowlist.go`

- [ ] **Step 1: Create the file**

```go
package auth

import "errors"

// AllowlistConfig controls which email addresses and domains may log in.
// If both Emails and Domains are empty, all logins are denied.
type AllowlistConfig struct {
	Emails  []string
	Domains []string
}

// ErrLoginDenied is returned by HandleCallback when the user's email is not
// in the configured allowlist.
var ErrLoginDenied = errors.New("login denied: email not in allowlist")
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./internal/auth/...
```
Expected: no output (clean compile)

- [ ] **Step 3: Commit**

```bash
git add internal/auth/allowlist.go
git commit -m "feat(auth): add AllowlistConfig type and ErrLoginDenied sentinel"
```

---

### Task 2: Write failing tests for `isAllowed`

**Files:**
- Create: `internal/auth/allowlist_test.go`

Note: `isAllowed` is an unexported method on `*Middleware`. Tests in package `auth` (same package) can access it directly. We construct a `Middleware` with only the `allowlist` field populated — the other fields (`provider`, `config`, `sessionManager`) will be nil/zero but `isAllowed` does not use them.

- [ ] **Step 1: Create the test file**

```go
package auth

import "testing"

// newTestMiddlewareWithAllowlist creates a Middleware with only the allowlist
// field set. Safe for testing isAllowed, which does not touch other fields.
func newTestMiddlewareWithAllowlist(cfg AllowlistConfig) *Middleware {
	return &Middleware{allowlist: cfg}
}

func TestIsAllowed_EmptyConfigDeniesAll(t *testing.T) {
	m := newTestMiddlewareWithAllowlist(AllowlistConfig{})
	if m.isAllowed("anyone@example.com") {
		t.Error("expected empty allowlist to deny all, got allowed")
	}
}

func TestIsAllowed_ExactEmailMatch(t *testing.T) {
	m := newTestMiddlewareWithAllowlist(AllowlistConfig{
		Emails: []string{"alice@example.com"},
	})
	if !m.isAllowed("alice@example.com") {
		t.Error("expected alice@example.com to be allowed")
	}
}

func TestIsAllowed_ExactEmailMatchCaseInsensitive(t *testing.T) {
	m := newTestMiddlewareWithAllowlist(AllowlistConfig{
		Emails: []string{"alice@example.com"},
	})
	if !m.isAllowed("Alice@Example.COM") {
		t.Error("expected case-insensitive email match to be allowed")
	}
}

func TestIsAllowed_DomainMatch(t *testing.T) {
	m := newTestMiddlewareWithAllowlist(AllowlistConfig{
		Domains: []string{"wachs.software"},
	})
	if !m.isAllowed("user@wachs.software") {
		t.Error("expected user@wachs.software to be allowed")
	}
}

func TestIsAllowed_DomainMatchCaseInsensitive(t *testing.T) {
	m := newTestMiddlewareWithAllowlist(AllowlistConfig{
		Domains: []string{"wachs.software"},
	})
	if !m.isAllowed("user@WACHS.SOFTWARE") {
		t.Error("expected case-insensitive domain match to be allowed")
	}
}

func TestIsAllowed_NoMatch(t *testing.T) {
	m := newTestMiddlewareWithAllowlist(AllowlistConfig{
		Emails:  []string{"alice@example.com"},
		Domains: []string{"wachs.software"},
	})
	if m.isAllowed("stranger@other.com") {
		t.Error("expected stranger@other.com to be denied")
	}
}

func TestIsAllowed_EmailWithNoAtSign(t *testing.T) {
	m := newTestMiddlewareWithAllowlist(AllowlistConfig{
		Domains: []string{"example.com"},
	})
	// Must not panic; no @ means domain extraction fails gracefully
	if m.isAllowed("notanemail") {
		t.Error("expected malformed email to be denied")
	}
}
```

- [ ] **Step 2: Run tests, confirm they all fail**

```bash
go test ./internal/auth/... -run TestIsAllowed -v
```
Expected: `FAIL` — compile error referencing `AllowlistConfig` or `isAllowed` undefined (neither exists yet — that's correct)

- [ ] **Step 3: Commit the test file**

Note: the repo will not compile cleanly until Task 3 is complete. Commit anyway to preserve the red-test history.

```bash
git add internal/auth/allowlist_test.go
git commit -m "test(auth): add failing unit tests for isAllowed"
```

---

### Task 3: Implement `isAllowed` and update `Middleware`

**Files:**
- Modify: `internal/auth/middleware.go`

- [ ] **Step 1: Add `allowlist` field to `Middleware` struct**

In `middleware.go`, update the `Middleware` struct (currently at the top of the file):

```go
type Middleware struct {
	provider       *oidc.Provider
	config         *oauth2.Config
	sessionManager *SessionManager
	roleConfig     *RoleConfig
	allowlist      AllowlistConfig   // new field
	insecureMode   bool
}
```

- [ ] **Step 2: Add the `allowlist` parameter to `NewMiddleware` and store it**

Update the `NewMiddleware` function signature (add `allowlist AllowlistConfig` between `roleConfig` and `insecureMode`) and store it:

```go
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
	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, err
	}

	config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  baseURL + "/auth/callback",
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}

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
```

- [ ] **Step 3: Implement `isAllowed`**

Add the following method to `middleware.go` (add `"strings"` to the import block):

```go
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
```

- [ ] **Step 4: Call `isAllowed` in `HandleCallback`**

In `HandleCallback`, after the `if claims.Email == ""` check and before the `DetermineRole` call, add:

```go
if !m.isAllowed(claims.Email) {
	return "", "", ErrLoginDenied
}
```

- [ ] **Step 5: Run the allowlist unit tests — all 7 should now pass**

```bash
go test ./internal/auth/... -run TestIsAllowed -v
```
Expected: all 7 tests (`EmptyConfigDeniesAll`, `ExactEmailMatch`, `ExactEmailMatchCaseInsensitive`, `DomainMatch`, `DomainMatchCaseInsensitive`, `NoMatch`, `EmailWithNoAtSign`) PASS

- [ ] **Step 6: Verify the package still compiles (other callers of `NewMiddleware` will break — that's expected and will be fixed in later tasks)**

```bash
go build ./internal/auth/...
```
Expected: clean

- [ ] **Step 7: Commit**

```bash
git add internal/auth/allowlist.go internal/auth/middleware.go
git commit -m "feat(auth): implement isAllowed and wire into HandleCallback"
```

---

### Task 4: Add `AllowlistConfig` to the config package

**Files:**
- Modify: `internal/config/config.go`

- [ ] **Step 1: Add `AllowlistConfig` struct**

After the `ClaimConfig` struct definition, add:

```go
// AllowlistConfig specifies which emails and domains are permitted to log in.
// If both lists are empty, all logins are denied.
type AllowlistConfig struct {
	Emails  []string `yaml:"emails"`
	Domains []string `yaml:"domains"`
}
```

- [ ] **Step 2: Add `Allowlist` field to `AuthConfig`**

Update `AuthConfig`:

```go
type AuthConfig struct {
	OIDC      OIDCConfig      `yaml:"oidc"`
	Admin     AdminConfig     `yaml:"admin"`
	Allowlist AllowlistConfig `yaml:"allowlist"`
}
```

- [ ] **Step 3: Verify config package compiles**

```bash
go build ./internal/config/...
```
Expected: clean

- [ ] **Step 4: Commit**

```bash
git add internal/config/config.go
git commit -m "feat(config): add AllowlistConfig for login email/domain restriction"
```

---

### Task 5: Fix the `main.go` compile break

**Files:**
- Modify: `cmd/rconman/main.go`

- [ ] **Step 1: Add `auth.AllowlistConfig` to the `NewMiddleware` call**

Find the `auth.NewMiddleware(...)` call (around line 80) and insert the new argument between the `&auth.RoleConfig{...}` block and `cfg.Server.InsecureMode`:

```go
authMiddleware, err := auth.NewMiddleware(
	context.Background(),
	cfg.Auth.OIDC.IssuerURL,
	clientID,
	clientSecret,
	cfg.Server.BaseURL,
	sessionSecret,
	sessionExpiry,
	&auth.RoleConfig{
		ClaimName:      cfg.Auth.Admin.Claim.Name,
		ClaimValue:     cfg.Auth.Admin.Claim.Value,
		EmailAllowlist: cfg.Auth.Admin.EmailAllowlist,
	},
	auth.AllowlistConfig{
		Emails:  cfg.Auth.Allowlist.Emails,
		Domains: cfg.Auth.Allowlist.Domains,
	},
	cfg.Server.InsecureMode,
)
```

- [ ] **Step 2: Build the binary**

```bash
go build ./cmd/rconman/...
```
Expected: clean

- [ ] **Step 3: Commit**

```bash
git add cmd/rconman/main.go
git commit -m "feat(main): pass AllowlistConfig to NewMiddleware"
```

---

### Task 6: Create the `LoginErrorPage` templ component

**Files:**
- Create: `internal/views/error.templ`

`LoginErrorPage` must be a standalone page (no session required — the user was just denied). It cannot use `Layout` because that component requires a `*auth.Session`. Write it as a self-contained HTML page that shares the same visual style (dark background, same fonts, same CSS).

- [ ] **Step 1: Create the file**

```go
package views

// LoginErrorPage renders a standalone access-denied page. It does not use
// Layout because no session is available at this point.
templ LoginErrorPage(message string) {
	<!DOCTYPE html>
	<html lang="en">
	<head>
		<meta charset="UTF-8"/>
		<meta name="viewport" content="width=device-width, initial-scale=1"/>
		<title>rconman - Access Denied</title>
		<link rel="stylesheet" href="/static/app.css"/>
		<style>
			body {
				background: linear-gradient(135deg, #0f172a 0%, #1e293b 50%, #0f172a 100%);
				min-height: 100vh;
				display: flex;
				align-items: center;
				justify-content: center;
			}
		</style>
	</head>
	<body>
		<div class="card card-modern p-10 max-w-md w-full mx-4 text-center">
			<div class="w-16 h-16 bg-gradient-to-br from-red-500 to-pink-600 rounded-full flex items-center justify-center mx-auto mb-6">
				<svg class="w-8 h-8 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z"/>
				</svg>
			</div>
			<h1 class="text-2xl font-bold text-white mb-4">Access Denied</h1>
			<p class="text-gray-400 mb-8">{ message }</p>
			<a href="/auth/login" class="btn btn-primary">Back to Login</a>
		</div>
	</body>
	</html>
}
```

- [ ] **Step 2: Generate the Go code from the templ file**

```bash
templ generate ./internal/views/...
```
Expected: creates `internal/views/error_templ.go` (the existing `pages_templ.go` and `layout_templ.go` were produced the same way — there are no `//go:generate` directives, so `go generate` is a no-op here)

- [ ] **Step 3: Verify the views package compiles**

```bash
go build ./internal/views/...
```
Expected: clean

- [ ] **Step 4: Commit**

```bash
git add internal/views/error.templ internal/views/error_templ.go
git commit -m "feat(views): add LoginErrorPage component for access denied"
```

---

### Task 7: Update the `Login` and `Callback` handlers, fix handler tests

**Files:**
- Modify: `internal/handlers/handlers.go`
- Modify: `internal/handlers/handlers_test.go`

#### Step A — Fix the compile break in `handlers_test.go` first

The existing `TestAuthHandlerLogin` passes positional args to `auth.NewMiddleware`. Adding the new `allowlist` parameter breaks compilation.

- [ ] **Step 1: Fix `TestAuthHandlerLogin` — insert `auth.AllowlistConfig{}`**

In `handlers_test.go`, find the `auth.NewMiddleware(...)` call (around line 64). Insert `auth.AllowlistConfig{}` between the `&auth.RoleConfig{...}` block and `true`:

```go
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
    auth.AllowlistConfig{},  // empty = deny all (test skips on OIDC init failure anyway)
    true, // insecureMode for testing
)
```

- [ ] **Step 2: Verify tests compile and existing tests pass**

```bash
go test ./internal/handlers/... -v
```
Expected: `TestAuthHandlerLogin` either PASS or SKIP (it skips when the OIDC provider is unreachable) — no compile error

#### Step B — Extract a middleware interface (required to enable stub testing)

`AuthHandler.middleware` currently holds `*auth.Middleware` (concrete type). To allow stub-based testing of `Callback`, extract a minimal interface in `handlers.go`. The four methods called on `h.middleware` are `AuthCodeURL`, `HandleCallback`, `CreateSession`, and `ClearSession`.

- [ ] **Step 3: Add the `authMiddleware` interface and update `AuthHandler`**

In `handlers.go`, before the `AuthHandler` struct definition (around line 189), add:

```go
// authMiddleware is the interface AuthHandler requires from the auth middleware.
type authMiddleware interface {
	AuthCodeURL(ctx context.Context) string
	HandleCallback(ctx context.Context, code, state string) (string, string, error)
	CreateSession(w http.ResponseWriter, r *http.Request, email, role string) error
	ClearSession(w http.ResponseWriter, r *http.Request) error
}
```

Then update the `AuthHandler` struct and constructor to use it (the `context` package is already imported via other files — add it to this file's imports if needed):

```go
type AuthHandler struct {
	config     *config.Config
	middleware authMiddleware
}

func NewAuthHandler(cfg *config.Config, m authMiddleware) *AuthHandler {
	return &AuthHandler{config: cfg, middleware: m}
}
```

`*auth.Middleware` already implements all four methods so no changes to the `auth` package are needed.

Add `"context"` to the import block of `handlers.go` if not already present.

- [ ] **Step 4: Verify the handlers package compiles**

```bash
go build ./internal/handlers/...
```
Expected: clean

#### Step C — Update the `Login` handler

- [ ] **Step 5: Add the `error=access_denied` branch to `Login`**

In `handlers.go`, find the `Login` method. Add the check at the top, before the existing `AuthCodeURL` call:

```go
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("error") == "access_denied" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		views.LoginErrorPage("You are not authorised to access this application.").Render(r.Context(), w)
		return
	}
	// existing code below, unchanged
	...
}
```

Also add `"github.com/your-org/rconman/internal/views"` to the import block if not already present. (The module path `github.com/your-org/rconman` is confirmed in `go.mod`.)

#### Step D — Update the `Callback` handler

- [ ] **Step 7: Add `ErrLoginDenied` handling to `Callback`**

In `handlers.go`, find the `Callback` method. In the `if err != nil` block after `HandleCallback`, prepend the `ErrLoginDenied` check:

```go
email, role, err := h.middleware.HandleCallback(r.Context(), code, state)
if err != nil {
    if errors.Is(err, auth.ErrLoginDenied) {
        http.Redirect(w, r, "/auth/login?error=access_denied", http.StatusFound)
        return
    }
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusUnauthorized)
    json.NewEncoder(w).Encode(CallbackResponse{
        Status: "error",
        Error:  "authentication failed",
    })
    return
}
```

Add `"errors"` to the import block if not already present.

- [ ] **Step 5: Verify the handlers package compiles**

```bash
go build ./internal/handlers/...
```
Expected: clean

#### Step E — Write the new handler tests

- [ ] **Step 8: Write failing tests for the new handler behaviour**

In `handlers_test.go`, add `"strings"` and `"context"` to the import block (if not already present), then add the stub and two new tests after `TestAuthHandlerLogin`:

```go
// stubMiddleware satisfies the authMiddleware interface defined in handlers.go.
// Matches the four methods: AuthCodeURL, HandleCallback, CreateSession, ClearSession.
type stubMiddleware struct {
	callbackErr error
}

func (s *stubMiddleware) AuthCodeURL(ctx context.Context) string { return "https://provider/auth" }
func (s *stubMiddleware) HandleCallback(ctx context.Context, code, state string) (string, string, error) {
	return "", "", s.callbackErr
}
func (s *stubMiddleware) CreateSession(w http.ResponseWriter, r *http.Request, email, role string) error {
	return nil
}
func (s *stubMiddleware) ClearSession(w http.ResponseWriter, r *http.Request) error { return nil }

func TestAuthHandlerLogin_AccessDenied(t *testing.T) {
	// The access_denied branch in Login returns before touching middleware.
	handler := NewAuthHandler(&config.Config{}, &stubMiddleware{})

	req := httptest.NewRequest("GET", "/auth/login?error=access_denied", nil)
	w := httptest.NewRecorder()

	handler.Login(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
	if !strings.Contains(w.Body.String(), "not authorised") {
		t.Errorf("expected body to contain 'not authorised', got: %s", w.Body.String())
	}
}

func TestAuthHandlerCallback_LoginDenied(t *testing.T) {
	handler := NewAuthHandler(&config.Config{}, &stubMiddleware{callbackErr: auth.ErrLoginDenied})

	req := httptest.NewRequest("GET", "/auth/callback?code=testcode&state=teststate", nil)
	w := httptest.NewRecorder()

	handler.Callback(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("expected redirect %d, got %d", http.StatusFound, w.Code)
	}
	location := w.Header().Get("Location")
	if location != "/auth/login?error=access_denied" {
		t.Errorf("expected redirect to /auth/login?error=access_denied, got %q", location)
	}
}
```

- [ ] **Step 9: Run all handler tests**

```bash
go test ./internal/handlers/... -v
```
Expected: `TestAuthHandlerLogin_AccessDenied` and `TestAuthHandlerCallback_LoginDenied` PASS; others PASS or SKIP

- [ ] **Step 10: Run the full test suite**

```bash
go test ./...
```
Expected: all tests PASS or SKIP (no failures)

- [ ] **Step 11: Commit**

```bash
git add internal/handlers/handlers.go internal/handlers/handlers_test.go
git commit -m "feat(handlers): handle ErrLoginDenied and render access denied page"
```

---

### Task 8: Update the Helm chart

**Files:**
- Modify: `helm/rconman/values.yaml`
- Modify: `helm/rconman/templates/configmap.yaml`

- [ ] **Step 1: Add `allowlist` to `values.yaml`**

In `helm/rconman/values.yaml`, after the `admin:` block (ending around line 64), add:

```yaml
    allowlist:
      # Restrict login to specific email addresses and/or domains.
      # If both lists are empty, all logins are denied.
      # emails:
      #   - alice@example.com
      # domains:
      #   - wachs.software
      emails: []
      domains: []
```

- [ ] **Step 2: Add allowlist to `configmap.yaml`**

In `helm/rconman/templates/configmap.yaml`, after line 48 (the `{{- end }}` that closes the `emailAllowlist` block) and before line 49 (`    minecraft:`), insert:

```yaml
{{- with .Values.config.auth.allowlist.emails }}
      allowlist:
        emails:
{{- range . }}
          - {{ . | quote }}
{{- end }}
{{- with $.Values.config.auth.allowlist.domains }}
        domains:
{{- range . }}
          - {{ . | quote }}
{{- end }}
{{- end }}
{{- else }}
{{- with .Values.config.auth.allowlist.domains }}
      allowlist:
        domains:
{{- range . }}
          - {{ . | quote }}
{{- end }}
{{- end }}
{{- end }}
```

This follows the same `{{- with }}` guard pattern used for `emailAllowlist`. When both lists are empty (the default), the entire `allowlist:` key is omitted from the rendered YAML, and Go's `config.AllowlistConfig` will have zero-length slices — correctly triggering the deny-all behaviour.

- [ ] **Step 3: Verify Helm template renders correctly with defaults**

```bash
helm template rconman helm/rconman --set config.server.baseURL=https://example.com | grep -A5 "auth:"
```
Expected: `allowlist:` key absent from rendered output (both lists empty)

- [ ] **Step 4: Verify Helm template renders correctly with values set**

```bash
helm template rconman helm/rconman \
  --set config.server.baseURL=https://example.com \
  --set "config.auth.allowlist.domains[0]=wachs.software" \
  --set "config.auth.allowlist.emails[0]=alice@example.com" \
  | grep -A10 "allowlist:"
```
Expected output (inside the config.yaml block):
```yaml
      allowlist:
        emails:
          - "alice@example.com"
        domains:
          - "wachs.software"
```

- [ ] **Step 5: Commit**

```bash
git add helm/rconman/values.yaml helm/rconman/templates/configmap.yaml
git commit -m "feat(helm): add auth.allowlist configuration to values and configmap"
```

---

### Task 9: Bump image tag and verify

- [ ] **Step 1: Bump `image.yaml`**

Read the current tag from `image.yaml`, increment the patch version by 1, and write the new value back. For example if it currently reads `0.1.3`, write `0.1.4`. Do not hardcode either the old or new version — always read the file first.

- [ ] **Step 2: Run the full test suite one final time**

```bash
go test ./...
```
Expected: all tests PASS or SKIP

- [ ] **Step 3: Commit**

```bash
git add image.yaml
git commit -m "chore: bump image tag for login allowlist release"
```

(Replace the version number in the message with the actual new version you wrote.)

- [ ] **Step 4: Push the branch**

```bash
git push
```
