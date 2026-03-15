# rconman — Design Specification

**Date:** 2026-03-14
**Status:** Approved

---

## Overview

`rconman` is an authenticated web UI for sending RCON commands to one or more Minecraft servers. It is designed to be simple to self-host, Kubernetes-native, and open source. A single Go binary serves the frontend and proxies RCON — RCON is never exposed outside the pod.

**Primary use case:** A parent running a homelab Minecraft server wants to issue admin commands (give items, teleport players, change time/weather) from their phone without needing a Minecraft client.

---

## Tech Stack

| Layer | Choice |
|---|---|
| Backend | Go |
| Routing | chi |
| HTML templating | templ (compiled, type-safe) |
| CSS framework | Tailwind CSS (compiled, embedded) |
| UI components | DaisyUI |
| Frontend interactivity | HTMX |
| Auth | coreos/go-oidc + golang.org/x/oauth2 |
| Sessions | gorilla/sessions (encrypted cookie) |
| Database | SQLite via modernc.org/sqlite (pure Go, no CGo) |
| SQL queries | sqlc (type-safe generated queries) |
| Logging | Go slog |

---

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    rconman pod (k8s)                     │
│                                                         │
│  ┌─────────────┐   ┌──────────────┐   ┌─────────────┐  │
│  │  chi router │──▶│  templ views │   │  SQLite DB  │  │
│  │  + HTMX     │   │  + DaisyUI   │   │  (command   │  │
│  │  handlers   │   │  + Tailwind  │   │   log)      │  │
│  └──────┬──────┘   └──────────────┘   └─────────────┘  │
│         │                                               │
│  ┌──────▼──────┐   ┌──────────────┐                    │
│  │  OIDC/OAuth │   │  RCON client │                    │
│  │  middleware │   │  (per server)│                    │
│  └─────────────┘   └──────┬───────┘                    │
└────────────────────────────┼────────────────────────────┘
                             │ internal k8s service
                    ┌────────▼────────┐
                    │  Minecraft RCON │
                    │  (port 25575)   │
                    └─────────────────┘
```

- Single Go binary serves HTML, handles auth, proxies RCON
- RCON is **never exposed** — the Go process communicates with it over the internal k8s network only
- Static assets (compiled Tailwind CSS) embedded via `embed.FS` — no CDN calls, air-gap friendly
- SQLite stored at a configurable path (mountable as a PVC in k8s)
- OIDC/OAuth2 via `coreos/go-oidc` — no heavy framework

**Request flow:** Browser → chi router → OIDC middleware (validates session cookie) → handler → RCON client → response written as templ partial → HTMX swaps into DOM.

---

## Auth Flow

- **PKCE Authorization Code flow** — no client secret in the browser
- **Encrypted session cookie** via `gorilla/sessions` — stateless server, survives pod restarts with a stable key. The resolved `session_secret` must be at least 32 bytes; config validation fails fast with a clear error if the value is absent, empty, or shorter than the minimum. 64 bytes recommended (AES-256 encryption + HMAC-SHA256 authentication).
- **Role determination at login:**
  1. Check configured OIDC claim (e.g. `roles` contains `rconman-admin`)
  2. Fall back to email allowlist in config
  - Role is stored in the encrypted session cookie and is **evaluated once at login**. It is not re-evaluated on subsequent requests. A role change (e.g. removing someone from the email allowlist) takes effect after the user's session expires and they log in again. With a default 24h session expiry, the maximum staleness window is 24h. This is a known and accepted limitation for v1.
- **Session expiry** is configurable (default 24h)
- **`/auth/logout`** clears cookie; optionally redirects to OIDC provider's logout endpoint

```
Browser → GET / (no session) → 302 /auth/login
        → 302 provider/auth?state=...&code_challenge=... (PKCE)
        → user logs in at provider
        → 302 /auth/callback?code=...
        → exchange code for id_token
        → validate token, extract email + claims
        → determine role (claim check → email allowlist)
        → set encrypted session cookie
        → 302 /
```

---

## Multi-Server Support

Multiple Minecraft servers are defined in `config.yaml`. The UI shows a server selector strip at the top. All commands, templates, status polling, and RCON connections are scoped per server. Each server has its own RCON connection pool entry.

## RCON Connection Lifecycle

The Minecraft RCON protocol is a single TCP connection with sequential request-response — it does not support multiplexing. Each configured server has one persistent RCON connection managed by the `rcon.Client` interface.

**Connection behaviour:**
- On startup, rconman establishes a connection to each configured RCON server and authenticates. Startup does not fail if a server is unreachable — it logs a warning and marks the server offline.
- Concurrent command sends to the same server are serialised with a per-server mutex.
- On any connection error (send failure, read timeout, broken pipe), the client attempts **one immediate reconnect**. If the reconnect succeeds, the original command is retried and the result returned to the caller. If the reconnect fails, an error is returned to the caller immediately — the call does not block waiting for backoff. A background goroutine then handles further reconnect attempts with exponential backoff (initial 1s, max 30s) until the connection is restored.
- **At most one background reconnect goroutine runs per client at any time.** This is enforced with an atomic flag (e.g. `sync/atomic` bool or a `sync.Once`-style guard reset on successful reconnect). If a reconnect goroutine is already running, a subsequent connection error from a concurrent caller (e.g. the status poller) does not spawn an additional goroutine — it simply returns an error immediately.
- The `rcon.Client` interface:

```go
type Client interface {
    Send(ctx context.Context, command string) (string, error)
    PlayerList(ctx context.Context) ([]string, error)
    IsConnected() bool
    Close() error
}
```

**Status polling failure behaviour:**
- Status is polled every `status_poll_interval` (default 30s) via a background goroutine, not HTMX polling — HTMX polls a `/api/servers/{id}/status` endpoint that returns the last cached status.
- The cache is updated by the background goroutine. On RCON error, the cache is updated to `offline` and the error is logged at `warn` level.
- The background goroutine does not open a new connection more frequently than `status_poll_interval` regardless of failure rate.

---

## Roles & Permissions

Two roles for v1:

| Role | Capabilities |
|---|---|
| **admin** | All templates + custom freeform command input |
| **viewer** | Pre-configured templates only — no custom commands |

**Role determination:** admin if the configured OIDC claim matches OR the user's email is in the allowlist. Any successfully authenticated user who does not qualify as admin is a **viewer** — authentication alone grants viewer access. There is no concept of "authenticated but no access" in v1.

**Player list endpoint:** The `/api/servers/{id}/players` endpoint (used to populate `player` param dropdowns) is accessible to all authenticated users regardless of role. Viewers need it to fill template inputs. It is not an unauthenticated endpoint.

Future versions may add per-template role restrictions.

---

## Command Templates

Templates are defined in `config.yaml` under each server. They are grouped into named **categories** rendered as tabs in the UI.

### Parameter Types

| Type | UI Control | Notes |
|---|---|---|
| `text` | Text input | Free string |
| `number` | Number input | Supports `min`, `max`, `default` |
| `select` | Dropdown | Requires `options` list |
| `boolean` | Checkbox | Renders as toggle |
| `player` | Dropdown | Populated from live RCON `/list` query on tab render; falls back to a `text` input with a warning indicator if the RCON call fails or returns empty |

### Parameter Input Validation

Parameter values are substituted as string literals into the command template. No shell escaping is applied (the RCON protocol has no shell — it sends a raw command string to the server). Before substitution, all parameter values are validated to:

- Contain no null bytes (`\x00`)
- Result in a final command string no longer than **4096 bytes** (the RCON protocol packet payload limit)

Validation failures are returned as a 400 error to the user with a clear message. The `player` type is additionally validated against the player list fetched from RCON (or accepted as free text if the fallback text input is used).

### Template Config Example

```yaml
- category: "Player Management"
  templates:
    - name: "Give Item"
      description: "Give items to a player"
      command: "/give {player} {item} {count}"
      params:
        - name: player
          type: player
        - name: item
          type: select
          options: ["diamond", "iron_sword", "golden_apple"]
        - name: count
          type: number
          default: 1
          min: 1
          max: 64
```

---

## Secret Value Resolution

Any sensitive config field (RCON passwords, OIDC client secret, session secret) supports three resolution strategies, resolved at startup — a missing value is a fatal error with a clear message:

```yaml
# Inline (local dev / simple setups)
password: "supersecret123"

# Environment variable reference
password:
  valueFrom:
    env: "MC_SURVIVAL_RCON_PASSWORD"

# File reference (k8s Secret volume mount, Docker secrets)
password:
  valueFrom:
    file: "/run/secrets/mc-survival-rcon-password"
```

Applies to: `server.session_secret`, `auth.oidc.client_id`, `auth.oidc.client_secret`, and each server's `rcon.password`.

`auth.oidc.client_secret` is **required** for v1. rconman is a confidential server-side client — even with PKCE, the token exchange at the server sends the client secret to the OIDC provider (as required by Google and most providers). A missing or empty `client_secret` is a fatal startup error.

---

## Configuration Schema

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  base_url: "https://rconman.example.com"
  session_secret:
    valueFrom:
      env: "RCONMAN_SESSION_SECRET"
  session_expiry: "24h"

log:
  level: "info"       # debug | info | warn | error
  format: "text"      # text (slog TextHandler) | json (slog JSONHandler)

store:
  path: "/data/rconman.db"   # SQLite file path; override for local dev
  retention: "30d"            # prune command log entries older than this on startup and every 24h

auth:
  oidc:
    issuer_url: "https://accounts.google.com"
    client_id:
      valueFrom:
        env: "RCONMAN_OIDC_CLIENT_ID"
    client_secret:
      valueFrom:
        env: "RCONMAN_OIDC_CLIENT_SECRET"
    scopes: ["openid", "email", "profile"]
  admin:
    claim:
      name: "roles"
      value: "rconman-admin"
    email_allowlist:
      - "andreas@example.com"

minecraft:
  servers:
    - name: "Survival"
      id: "survival"
      rcon:
        host: "mc-survival.minecraft.svc.cluster.local"
        port: 25575
        password:
          valueFrom:
            env: "MC_SURVIVAL_RCON_PASSWORD"
      status_poll_interval: "30s"
      commands:
        - category: "Player Management"
          templates: []
        - category: "World"
          templates: []
```

---

## UI Layout

Mobile-first. DaisyUI dark theme.

```
┌─────────────────────────┐
│ rconman     ● Survival  │  ← header: app name + status (online + player count)
│             3 online    │
├─────────────────────────┤
│ [Survival] [Creative]   │  ← server selector strip
├─────────────────────────┤
│ [Players][World][Admin] │  ← category tabs (per server)
│          [Custom]       │
├─────────────────────────┤
│  ┌───────────────────┐  │
│  │ Give Item         │  │  ← command card
│  │ /give {p} {i} {n} │  │
│  │ [▾ Player       ] │  │
│  │ [▾ diamond    ▾ ] │  │
│  │ [    64          ]│  │
│  │ [▶ Execute      ] │  │
│  │ ✓ Given 64 to ... │  │  ← inline response (HTMX swap)
│  └───────────────────┘  │
│  ┌───────────────────┐  │
│  │ Teleport          │  │
│  └───────────────────┘  │
├─────────────────────────┤
│ ▲ Command Log (5)       │  ← collapsible bottom drawer
└─────────────────────────┘
```

- **Status bar:** online/offline indicator + player count, polled every 30s via HTMX polling
- **Server selector:** pill/tab strip — switching server reloads the command area via HTMX
- **Category tabs:** horizontal tabs, HTMX partial swap on click
- **Command cards:** each template renders as a card with its param inputs inline; Execute triggers an HTMX POST
- **Inline response:** server response appears below the Execute button after submission
- **Command log drawer:** slides up from the bottom, shows last N entries with timestamp, user, server, command, response
- **Custom command tab** (admin only): freeform text input + Execute

---

## Command Log

- Stored in SQLite at the path defined by `store.path` (default `/data/rconman.db`)
- Retention period is configurable via `store.retention` (e.g. `30d`) — a background goroutine prunes entries older than the retention window. The first prune run is triggered asynchronously shortly after startup (does not block the HTTP listener becoming ready) and then repeats every 24h.
- Schema: `id`, `timestamp`, `user_email`, `server_id`, `command`, `response`, `duration_ms`
- Rendered in the log drawer via HTMX; auto-refreshes when new commands are executed

---

## Repository Structure

```
rconman/
├── Containerfile
├── Makefile                          # test, e2e, build, helm targets
├── config.example.yaml
├── .github/
│   └── workflows/
│       ├── build.yml                 # build + push image to GHCR
│       ├── helm.yml                  # package + push Helm chart to GHCR
│       └── e2e.yml                   # Kind-based e2e on PRs
├── cmd/
│   └── rconman/
│       └── main.go
├── internal/
│   ├── config/                       # YAML loading, valueFrom resolution, validation
│   ├── auth/                         # OIDC middleware, session, role check
│   │   └── mock/                     # fake OIDC provider for testing
│   ├── rcon/
│   │   ├── client.go                 # RCON client interface
│   │   └── mock/                     # in-process mock RCON for unit tests
│   ├── server/                       # chi router, handler wiring
│   ├── handlers/                     # HTTP handlers (deps injected via interfaces)
│   ├── views/                        # templ components
│   ├── store/                        # SQLite command log (sqlc-generated)
│   │   └── mock/                     # in-memory store mock for unit tests
│   └── model/                        # shared types
├── web/
│   ├── static/                       # compiled Tailwind CSS (embedded)
│   └── tailwind.config.js
├── helm/
│   └── rconman/
│       ├── Chart.yaml
│       ├── values.yaml
│       └── templates/
│           ├── statefulset.yaml      # STS, single replica, volumeClaimTemplate
│           ├── service.yaml
│           ├── configmap.yaml
│           ├── secret.yaml           # optional
│           └── httproute.yaml        # Gateway API HTTPRoute (gated by .Values.gateway.enabled)
├── test/
│   ├── e2e/                          # Go e2e test suite (runs as k8s Job in Kind)
│   │   ├── suite_test.go
│   │   ├── commands_test.go
│   │   ├── auth_test.go
│   │   └── Containerfile
│   ├── mock-rcon/                    # tiny Go RCON server for e2e
│   │   ├── main.go
│   │   └── Containerfile
│   └── kind/
│       ├── kind-config.yaml
│       ├── values-test.yaml          # Helm values for Kind e2e
│       ├── setup.sh
│       └── teardown.sh
└── docs/
    └── superpowers/specs/
```

---

## Helm Chart

### StatefulSet with VolumeClaimTemplate

```yaml
kind: StatefulSet
spec:
  replicas: 1
  volumeClaimTemplates:
    - metadata:
        name: data
      spec:
        accessModes: ["ReadWriteOnce"]
        resources:
          requests:
            storage: {{ .Values.persistence.size | default "1Gi" }}
  template:
    spec:
      containers:
        - name: rconman
          volumeMounts:
            - name: data
              mountPath: /data
```

Rolling pod replacement reattaches the same PVC — no data loss.

### Gateway API (gated)

```yaml
# values.yaml
gateway:
  enabled: false
  parentRef:
    name: ""
    namespace: ""
    sectionName: ""
```

When `gateway.enabled: true`, the chart renders an `HTTPRoute`. No `Ingress` resources anywhere in the chart.

---

## CI/CD

| Trigger | Workflow | Action |
|---|---|---|
| PR | `build.yml` | Build image (no push), run unit tests |
| PR | `e2e.yml` | Kind cluster, full e2e suite |
| Push to `main` | `build.yml` | Push `ghcr.io/<owner>/rconman:main` |
| Push tag `v*.*.*` | `build.yml` | Push `:vX.Y.Z` + `:latest` |
| Push tag `v*.*.*` | `helm.yml` | Package + push chart to GHCR OCI |

Multi-arch image: `linux/amd64` + `linux/arm64`.

### Containerfile (multi-stage)

```dockerfile
FROM node:22-alpine AS css
WORKDIR /web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=css /web/static/app.css web/static/app.css
RUN go generate ./...
RUN CGO_ENABLED=0 GOOS=linux go build -o rconman ./cmd/rconman

FROM gcr.io/distroless/static-debian13
COPY --from=builder /app/rconman /rconman
ENTRYPOINT ["/rconman"]
```

`distroless/static-debian13` provides CA certs, a `/tmp` directory (required by `modernc.org/sqlite` for WAL journal and temp files), and a `nobody` user — while still having no shell or package manager. Image size is ~5MB larger than `scratch` but avoids SQLite write failures at runtime.

---

## e2e Testing

```
make e2e
  ├── kind create cluster
  ├── build + kind load: rconman, mock-rcon, e2e-runner images
  ├── helm install rconman (test values: mock-rcon as RCON host, mock OIDC)
  ├── kubectl wait --for=condition=ready pod -l app=rconman
  ├── kubectl apply e2e-runner Job
  ├── kubectl wait --for=condition=complete job/e2e-runner --timeout=120s
  ├── propagate job exit code → make pass/fail
  └── kind delete cluster
```

- e2e runner is a **Kubernetes Job** — no port-forwarding, clean pass/fail via exit code
- `mock-rcon`: ~100-line Go binary implementing the RCON protocol, configurable command→response mappings via env/JSON
- `mock-rcon` used in both unit tests (in-process interface mock) and e2e (deployed pod)

---

## Dependency Injection Pattern

All handlers receive dependencies via constructors — no globals, no `init()`.

Because rconman supports multiple servers, handlers that issue RCON commands receive a `map[string]rcon.Client` keyed by server ID, and look up the correct client from the URL parameter at request time:

```go
type CommandHandler struct {
    rcons  map[string]rcon.Client  // keyed by server ID; interface → real or mock
    store  store.Store             // interface → SQLite or in-memory mock
    config *config.Config
}

func NewCommandHandler(
    rcons map[string]rcon.Client,
    s store.Store,
    cfg *config.Config,
) *CommandHandler {
    return &CommandHandler{rcons: rcons, store: s, config: cfg}
}

func (h *CommandHandler) Execute(w http.ResponseWriter, r *http.Request) {
    serverID := chi.URLParam(r, "id")
    client, ok := h.rcons[serverID]
    if !ok {
        http.Error(w, "unknown server", http.StatusNotFound)
        return
    }
    // ... use client
}
```

Unit tests inject mock clients per server ID; e2e and production inject real implementations. The `map` is built once at startup in `main.go` and is read-only thereafter — no mutex required on the map itself.
