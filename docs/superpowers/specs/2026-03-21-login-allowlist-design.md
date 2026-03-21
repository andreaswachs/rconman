# Login Allowlist Design

**Date:** 2026-03-21
**Status:** Approved

## Overview

Add an allowlist to restrict which users may log in via the OIDC provider, based on specific email addresses or email domains. If no allowlist is configured, all logins are denied (fail-closed).

## Requirements

- Users can be allowed by exact email address or by email domain (part after `@`)
- If both lists are empty, all logins are denied
- A user satisfying either condition (email OR domain match) is allowed
- All comparison is case-insensitive (normalize to lowercase before comparing)
- Denied users are redirected to `/auth/login?error=access_denied` and shown a human-readable error page
- Configuration lives in a new `auth.allowlist` section, separate from `auth.admin`
- No startup validation — empty allowlist is valid and intentional (operator responsibility)

## Type Ownership

Following the existing pattern for `RoleConfig`:

- `config.AllowlistConfig` (in `internal/config/config.go`) — used only for YAML parsing
- `auth.AllowlistConfig` (in `internal/auth/`) — used by the middleware; mirrors the config type
- `cmd/rconman/main.go` maps one to the other (same as it does for `RoleConfig`)

This keeps the `auth` package free of any import from `internal/config`.

## Configuration

New `AllowlistConfig` struct in `internal/config/config.go`:

```go
type AllowlistConfig struct {
    Emails  []string `yaml:"emails"`
    Domains []string `yaml:"domains"`
}
```

Added to `AuthConfig`:

```go
type AuthConfig struct {
    OIDC      OIDCConfig      `yaml:"oidc"`
    Admin     AdminConfig     `yaml:"admin"`
    Allowlist AllowlistConfig `yaml:"allowlist"`
}
```

Example `config.yaml`:

```yaml
auth:
  allowlist:
    emails:
      - alice@example.com
    domains:
      - wachs.software
      - wachs.email
```

## Auth Package Changes

### New type and sentinel error in `internal/auth/`

Define `AllowlistConfig` and `ErrLoginDenied` (e.g., in a new `internal/auth/allowlist.go`):

```go
package auth

import "errors"

// AllowlistConfig controls which email addresses and domains may log in.
type AllowlistConfig struct {
    Emails  []string
    Domains []string
}

// ErrLoginDenied is returned by HandleCallback when the user's email is not
// in the configured allowlist.
var ErrLoginDenied = errors.New("login denied: email not in allowlist")
```

### Updated `NewMiddleware` signature

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
    allowlist AllowlistConfig,   // new — note: auth.AllowlistConfig, not config.AllowlistConfig
    insecureMode bool,
) (*Middleware, error)
```

`allowlist` is stored as a field on `Middleware`.

### `isAllowed` helper (unexported method on `*Middleware`)

```go
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

### `HandleCallback` change

After extracting the email claim from the ID token, before returning:

```go
if !m.isAllowed(claims.Email) {
    return "", "", ErrLoginDenied
}
```

## Handler Changes

### `internal/handlers/handlers.go` — `Login`

The existing `Login` handler unconditionally redirects to the OIDC provider. Update it to check the `error` query parameter first. If `error=access_denied`, render a new `views.LoginErrorPage` templ component instead of redirecting:

```go
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
    if r.URL.Query().Get("error") == "access_denied" {
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        w.WriteHeader(http.StatusForbidden)
        views.LoginErrorPage("You are not authorised to access this application.").Render(r.Context(), w)
        return
    }
    // existing OIDC redirect logic unchanged
    ...
}
```

A new `LoginErrorPage(message string)` templ component must be created in `internal/views/`. It should be a minimal standalone page (not requiring a session) that displays the message and a link to retry login at `/auth/login`.

### `internal/handlers/handlers.go` — `Callback`

Add `ErrLoginDenied` detection before the existing generic JSON error path. The generic path is intentionally left as-is (it handles provider/token errors which are not browser-navigation failures):

```go
email, role, err := h.middleware.HandleCallback(r.Context(), code, state)
if err != nil {
    if errors.Is(err, auth.ErrLoginDenied) {
        http.Redirect(w, r, "/auth/login?error=access_denied", http.StatusFound)
        return
    }
    // existing JSON 401 path unchanged
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusUnauthorized)
    json.NewEncoder(w).Encode(CallbackResponse{Status: "error", Error: "authentication failed"})
    return
}
```

## Helm Chart

### `helm/rconman/values.yaml`

```yaml
config:
  auth:
    allowlist:
      emails: []
      domains: []
```

### `helm/rconman/templates/configmap.yaml`

Insert the following block after the closing `{{- end }}` of the `admin` block (line 48) and before the `minecraft:` line (line 49), using the same `with`-guard pattern as `emailAllowlist`:

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

Note: Indentation is 6 spaces for `allowlist:` to align with `admin:` and `oidc:` (which are themselves under the `auth:` block rendered at 4-space indent in the config.yaml content). If both lists are empty the block is omitted entirely; the Go config will use zero-length slices, which correctly triggers the deny-all behavior.

## Wiring (`cmd/rconman/main.go`)

Following the `RoleConfig` pattern, construct `auth.AllowlistConfig` from the parsed config and pass it into `NewMiddleware`:

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
    auth.AllowlistConfig{                        // new
        Emails:  cfg.Auth.Allowlist.Emails,
        Domains: cfg.Auth.Allowlist.Domains,
    },
    cfg.Server.InsecureMode,
)
```

## Testing

### Unit tests (`internal/auth/`)

New test file `internal/auth/allowlist_test.go` covering `isAllowed`:

- Empty `AllowlistConfig{}` → denies all emails
- Exact email match (case-insensitive, e.g., `Alice@Example.com` matches `alice@example.com`)
- Domain match (e.g., `user@wachs.software` matches domain `wachs.software`, case-insensitive)
- No match returns `false`
- Email with no `@` does not panic and returns `false`

### Handler tests (`internal/handlers/handlers_test.go`)

- **Fix compilation break:** the existing `TestAuthHandlerLogin` call to `auth.NewMiddleware` uses positional arguments and must have `auth.AllowlistConfig{}` inserted between `roleConfig` and `insecureMode`.
- **New test:** add a case for `GET /auth/login?error=access_denied` that asserts `HTTP 403` and that the response body contains the error message (not a redirect).
- **New test:** add a case for `GET /auth/callback` where `HandleCallback` returns `ErrLoginDenied`, asserting a redirect to `/auth/login?error=access_denied`.

### E2e (`test/kind/setup.sh`)

The e2e test only verifies pod readiness via the `/health` endpoint — it does not exercise the OIDC login flow. The allowlist check only runs during callback processing, so no changes to `setup.sh` are required for the e2e to pass. If a future e2e test exercises login, the `--set config.auth.allowlist.domains[0]=<domain>` flag should be added at that point.

## Out of Scope

- Wildcard domain matching (e.g., `*.example.com`)
- Per-server allowlists
- Dynamic allowlist updates without restart
