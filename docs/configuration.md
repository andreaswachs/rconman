# Configuration Reference

rconman is configured via a YAML file (default: `config.yaml`) with optional environment variable overrides for secret fields. All secrets support inline values, environment variable references, or file references — see [Secret Resolution](#secret-resolution).

## Table of Contents

- [Quick Start](#quick-start)
- [Secret Resolution](#secret-resolution)
- [Full Schema Reference](#full-schema-reference)
  - [server](#server)
  - [log](#log)
  - [store](#store)
  - [auth](#auth)
  - [minecraft](#minecraft)
    - [servers](#minecraftservers)
    - [commands and templates](#commands-and-templates)
    - [template parameter types](#template-parameter-types)
  - [lists](#lists)
- [Complete Example](#complete-example)
- [Kubernetes / Helm](#kubernetes--helm)
- [Running Locally](#running-locally)

---

## Quick Start

Minimal configuration to get rconman running:

```yaml
server:
  base_url: "http://localhost:8080"
  session_secret: "change-me-to-a-random-64-byte-string-before-going-to-production!!"

auth:
  oidc:
    issuer_url: "https://accounts.google.com"
    client_id: "your-client-id.apps.googleusercontent.com"
    client_secret: "your-client-secret"
  admin:
    email_allowlist:
      - "you@example.com"

minecraft:
  servers:
    - name: "My Server"
      id: "main"
      rcon:
        host: "localhost"
        port: 25575
        password: "your-rcon-password"
      commands:
        - category: "Essentials"
          templates:
            - name: "Set Time to Day"
              command: "/time set day"
```

Run it:

```bash
rconman --config config.yaml
```

---

## Secret Resolution

Any field marked **secret** in this reference supports three resolution strategies. You can mix strategies freely across different fields.

### Inline value

Suitable for local development and simple setups. The value is used literally.

```yaml
password: "my-rcon-password"
```

### Environment variable reference

The value of the named environment variable is read at startup. rconman exits with a clear error if the variable is unset or empty.

```yaml
password:
  valueFrom:
    env: "MC_RCON_PASSWORD"
```

### File reference

The contents of the file at the given path are read at startup and used as the value. Suitable for Kubernetes Secret volume mounts and Docker secrets.

```yaml
password:
  valueFrom:
    file: "/run/secrets/mc-rcon-password"
```

> **Startup behaviour:** All secrets are resolved before the HTTP server starts. A missing, unreadable, or empty secret is a fatal error — rconman will not start and will print the name of the offending config field.

---

## Full Schema Reference

### `server`

HTTP server settings.

| Field | Type | Default | Description |
|---|---|---|---|
| `server.host` | string | `"0.0.0.0"` | Address to bind the HTTP server to |
| `server.port` | integer | `8080` | Port to listen on |
| `server.base_url` | string | **required** | Public base URL of rconman — used to construct the OIDC callback URL. Include scheme and host, no trailing slash. |
| `server.session_secret` | **secret** | **required** | Key used to encrypt session cookies. Minimum 32 bytes; 64 bytes recommended. Generate with: `openssl rand -base64 64` |
| `server.session_expiry` | duration | `"24h"` | How long a session cookie is valid. After expiry the user must log in again. Supports Go duration strings: `"1h"`, `"12h"`, `"24h"`, `"168h"`. |

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  base_url: "https://rconman.example.com"
  session_secret:
    valueFrom:
      env: "RCONMAN_SESSION_SECRET"
  session_expiry: "24h"
```

---

### `log`

Logging output settings. rconman uses Go's `slog` library.

| Field | Type | Default | Description |
|---|---|---|---|
| `log.level` | string | `"info"` | Minimum log level. One of: `debug`, `info`, `warn`, `error` |
| `log.format` | string | `"text"` | Log output format. `text` is human-readable; `json` is structured and suitable for log aggregators (Loki, Elasticsearch, etc.) |

**`text` format** (default — human-readable, ideal for local dev and simple setups):

```
time=2026-03-14T21:00:00Z level=INFO msg="command executed" server=survival user=you@example.com command="/time set day"
```

**`json` format** (structured — ideal for Kubernetes with Loki or similar):

```json
{"time":"2026-03-14T21:00:00Z","level":"INFO","msg":"command executed","server":"survival","user":"you@example.com","command":"/time set day"}
```

```yaml
log:
  level: "info"
  format: "text"   # or "json" for Kubernetes
```

---

### `store`

Command log database settings. rconman records every executed command in a SQLite database.

| Field | Type | Default | Description |
|---|---|---|---|
| `store.path` | string | `"/data/rconman.db"` | Path to the SQLite database file. The directory must be writable. In Kubernetes, this path should be on a mounted PersistentVolume. |
| `store.retention` | duration | `"30d"` | How long to retain command log entries. Entries older than this are deleted asynchronously at startup and then every 24 hours. Supports: `"7d"`, `"30d"`, `"90d"`, `"365d"`. |

```yaml
store:
  path: "/data/rconman.db"
  retention: "30d"
```

> **Local development:** Set `store.path` to a local path like `"./rconman.db"` so no `/data` directory is needed.

---

### `auth`

Authentication and authorisation settings. rconman delegates login to an external OIDC provider — it does not manage users itself.

#### `auth.oidc`

| Field | Type | Default | Description |
|---|---|---|---|
| `auth.oidc.issuer_url` | string | **required** | The OIDC issuer URL. rconman fetches `<issuer_url>/.well-known/openid-configuration` on startup to discover endpoints. |
| `auth.oidc.client_id` | **secret** | **required** | OAuth2 client ID issued by your OIDC provider. |
| `auth.oidc.client_secret` | **secret** | **required** | OAuth2 client secret. Required even with PKCE — rconman is a confidential server-side client. |
| `auth.oidc.scopes` | list of strings | `["openid", "email", "profile"]` | OIDC scopes to request. `openid` and `email` are required for rconman to function. Add provider-specific scopes for custom claims (e.g. `"groups"` for Keycloak). |

**Common provider issuer URLs:**

| Provider | `issuer_url` |
|---|---|
| Google | `https://accounts.google.com` |
| Auth0 | `https://<your-tenant>.auth0.com` |
| Keycloak | `https://<host>/realms/<realm>` |
| Authentik | `https://<host>/application/o/<app-slug>/` |
| GitHub (via Dex) | `https://<dex-host>` |

```yaml
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
```

> **OIDC callback URL:** Register `<server.base_url>/auth/callback` as the redirect URI in your OIDC provider. For example: `https://rconman.example.com/auth/callback`.

#### `auth.admin`

Determines which authenticated users receive the **admin** role. Admins can execute all templates and send custom freeform commands. Any authenticated user who does not qualify as admin receives the **viewer** role (templates only, no custom commands).

Admin role is evaluated once at login using these two checks in order:

1. **OIDC claim check** — if a claim named `auth.admin.claim.name` exists in the ID token and contains `auth.admin.claim.value`, the user is an admin.
2. **Email allowlist** — if the user's email address is in `auth.admin.email_allowlist`, the user is an admin.

Either condition is sufficient. Both can be configured simultaneously.

| Field | Type | Default | Description |
|---|---|---|---|
| `auth.admin.claim.name` | string | `""` | JWT claim name to inspect (e.g. `"roles"`, `"groups"`). Leave empty to disable claim-based admin detection. |
| `auth.admin.claim.value` | string | `""` | Required value in the claim. For list-type claims, the value must be present as one element. |
| `auth.admin.email_allowlist` | list of strings | `[]` | Email addresses that are always granted admin access, regardless of OIDC claims. |

```yaml
auth:
  admin:
    # Option 1: OIDC claim (e.g. Keycloak groups, Auth0 roles)
    claim:
      name: "roles"
      value: "rconman-admin"

    # Option 2: email allowlist (simplest — no IdP configuration needed)
    email_allowlist:
      - "you@example.com"
      - "partner@example.com"
```

> **Role staleness:** Role is stored in the session cookie at login time. Changes to the allowlist or OIDC claims take effect when the user's session expires and they log in again. Default session expiry is 24h.

---

### `minecraft`

#### `minecraft.servers`

A list of Minecraft servers rconman can send commands to. Each server has its own RCON connection, command templates, and status polling.

| Field | Type | Default | Description |
|---|---|---|---|
| `id` | string | **required** | Unique identifier used in URLs. Must be lowercase alphanumeric with hyphens (e.g. `"survival"`, `"creative"`). |
| `name` | string | **required** | Display name shown in the UI server selector. |
| `rcon.host` | string | **required** | Hostname or IP of the RCON server. In Kubernetes this is typically a cluster-internal service name. |
| `rcon.port` | integer | `25575` | RCON port. Default is the Minecraft RCON default. |
| `rcon.password` | **secret** | **required** | RCON password set in `server.properties` as `rcon.password`. |
| `status_poll_interval` | duration | `"30s"` | How often to poll the server's online status and player list in the background. |

```yaml
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
      commands: []

    - name: "Creative"
      id: "creative"
      rcon:
        host: "mc-creative.minecraft.svc.cluster.local"
        port: 25575
        password:
          valueFrom:
            file: "/run/secrets/mc-creative-rcon-password"
      status_poll_interval: "60s"
      commands: []
```

> **Enabling RCON on your Minecraft server:** In `server.properties`, set:
> ```
> enable-rcon=true
> rcon.port=25575
> rcon.password=your-password-here
> ```

---

### Commands and Templates

Commands are defined per-server under `minecraft.servers[*].commands`. They are grouped into **categories** which appear as tabs in the UI.

```yaml
commands:
  - category: "Category Name"
    templates:
      - name: "Template Name"
        description: "Optional description shown below the name"
        command: "/minecraft command {param1} {param2}"
        params:
          - name: param1
            type: text
          - name: param2
            type: select
            options: ["value1", "value2"]
```

| Field | Type | Required | Description |
|---|---|---|---|
| `category` | string | yes | Tab label in the UI |
| `templates[*].name` | string | yes | Card title |
| `templates[*].description` | string | no | Subtitle shown on the card |
| `templates[*].command` | string | yes | Command string with `{param_name}` placeholders |
| `templates[*].params` | list | no | Parameter definitions. If empty, the command runs with no inputs. |

---

### Template Parameter Types

Each `{placeholder}` in a command template corresponds to a parameter definition in `params`. Parameters are rendered as form inputs on the command card.

#### `text`

A free-text input field.

```yaml
- name: message
  type: text
  default: "Hello!"    # optional
```

#### `number`

A numeric input with optional range and default.

```yaml
- name: count
  type: number
  default: 1           # optional
  min: 1               # optional
  max: 64              # optional
```

#### `select`

A dropdown with a fixed list of options.

```yaml
- name: time
  type: select
  options: ["day", "night", "noon", "midnight"]
  default: "day"       # optional — must be one of the options
```

#### `boolean`

A toggle switch. Substituted as `"true"` or `"false"` in the command string.

```yaml
- name: announce
  type: boolean
  default: false       # optional
```

#### `player`

A dropdown populated from a live `/list` RCON query when the tab is rendered, showing currently online players. Falls back to a free-text input with a warning indicator if the RCON connection is unavailable.

```yaml
- name: player
  type: player
  # no options needed — populated at runtime
```

#### `list`

A strict autocomplete input. The user types and sees suggestions from a named list defined in the top-level [`lists`](#lists) config key. Only values present in the list are accepted — submitting an unlisted value is blocked in the UI.

```yaml
- name: pokemon
  type: list
  list: pokemon    # must match a key defined under the top-level lists key
```

> **Cross-reference:** The value of `list` must exactly match a key in `lists`. rconman will not start if the referenced list does not exist.

> **Validation:** All parameter values are validated before the command is sent. Null bytes are rejected. The final assembled command string must not exceed 4096 bytes (the RCON packet limit). Violations return a 400 error.

---

### `lists`

Defines globally-scoped named lists of string values. Lists are referenced by command template parameters with `type: list`, which render as strict autocomplete inputs — the user sees suggestions from the list and can only submit a value that appears in it.

| Field | Type | Required | Description |
|---|---|---|---|
| `lists.<name>` | list of strings | no | A named list. The name must contain only alphanumeric characters, hyphens (`-`), and underscores (`_`). Must have at least one entry. |

```yaml
lists:
  pokemon:
    - bulbasaur
    - charmander
    - squirtle
  gamemodes:
    - survival
    - creative
    - adventure
```

> **Startup validation:** All lists are validated at startup. An invalid list name (e.g. containing spaces), an empty list, or a `type: list` param that references a list name not defined here will cause rconman to exit with a clear error message.

---

## Complete Example

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
  level: "info"
  format: "json"       # json for Kubernetes, text for local dev

store:
  path: "/data/rconman.db"
  retention: "30d"

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
      - "you@example.com"

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
          templates:
            - name: "Give Item"
              description: "Give items to a player"
              command: "/give {player} {item} {count}"
              params:
                - name: player
                  type: player
                - name: item
                  type: select
                  options: ["diamond", "iron_sword", "bow", "golden_apple", "elytra"]
                - name: count
                  type: number
                  default: 1
                  min: 1
                  max: 64

            - name: "Teleport to Player"
              description: "Teleport one player to another"
              command: "/tp {player} {target}"
              params:
                - name: player
                  type: player
                - name: target
                  type: player

            - name: "Heal Player"
              command: "/effect give {player} minecraft:regeneration 10 255 true"
              params:
                - name: player
                  type: player

        - category: "World"
          templates:
            - name: "Set Time"
              command: "/time set {time}"
              params:
                - name: time
                  type: select
                  options: ["day", "night", "noon", "midnight"]

            - name: "Set Weather"
              command: "/weather {type}"
              params:
                - name: type
                  type: select
                  options: ["clear", "rain", "thunder"]

            - name: "Set Difficulty"
              command: "/difficulty {difficulty}"
              params:
                - name: difficulty
                  type: select
                  options: ["peaceful", "easy", "normal", "hard"]

        - category: "Server"
          templates:
            - name: "Broadcast Message"
              description: "Send a message to all players"
              command: "/say {message}"
              params:
                - name: message
                  type: text

            - name: "Save World"
              command: "/save-all"

            - name: "Kick Player"
              command: "/kick {player} {reason}"
              params:
                - name: player
                  type: player
                - name: reason
                  type: text
                  default: "Kicked by admin"

    - name: "Creative"
      id: "creative"
      rcon:
        host: "mc-creative.minecraft.svc.cluster.local"
        port: 25575
        password:
          valueFrom:
            file: "/run/secrets/mc-creative-rcon-password"
      status_poll_interval: "60s"
      commands:
        - category: "Creative Tools"
          templates:
            - name: "Creative Mode"
              command: "/gamemode creative {player}"
              params:
                - name: player
                  type: player

            - name: "Survival Mode"
              command: "/gamemode survival {player}"
              params:
                - name: player
                  type: player
```

---

## Kubernetes / Helm

Install via Helm from GHCR:

```bash
helm install rconman oci://ghcr.io/<owner>/charts/rconman \
  --version 1.0.0 \
  --namespace rconman \
  --create-namespace \
  -f my-values.yaml
```

The Helm `values.yaml` mirrors the `config.yaml` structure. Secrets should be provided via Kubernetes Secrets and referenced using `valueFrom.file` or `valueFrom.env` with `envFrom`.

**Recommended pattern — Kubernetes Secret + volume mount:**

```yaml
# secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: rconman-secrets
stringData:
  session-secret: "your-64-byte-random-string"
  oidc-client-id: "your-client-id"
  oidc-client-secret: "your-client-secret"
  mc-survival-rcon-password: "your-rcon-password"
```

```yaml
# config.yaml (mounted into pod)
server:
  session_secret:
    valueFrom:
      file: "/run/secrets/session-secret"
auth:
  oidc:
    client_id:
      valueFrom:
        file: "/run/secrets/oidc-client-id"
    client_secret:
      valueFrom:
        file: "/run/secrets/oidc-client-secret"
minecraft:
  servers:
    - rcon:
        password:
          valueFrom:
            file: "/run/secrets/mc-survival-rcon-password"
```

**Gateway API (HTTPRoute):**

Enable in Helm values to create an `HTTPRoute` instead of an `Ingress`:

```yaml
# values.yaml
gateway:
  enabled: true
  parentRef:
    name: "my-gateway"
    namespace: "gateway-system"
    sectionName: "https"
```

**Persistence:**

rconman uses a StatefulSet with a `volumeClaimTemplate`. The SQLite database is stored on the PVC and survives pod restarts and rolling updates.

```yaml
# values.yaml
persistence:
  size: "1Gi"
  storageClass: ""    # leave empty to use cluster default
```

---

## Running Locally

```bash
# Clone and build
git clone https://github.com/<owner>/rconman
cd rconman
make build

# Copy and edit the example config
cp config.example.yaml config.yaml
# Edit config.yaml — set base_url, OIDC credentials, RCON host/password

# Run
./rconman --config config.yaml

# Or with environment variables for secrets
RCONMAN_SESSION_SECRET="..." \
RCONMAN_OIDC_CLIENT_ID="..." \
RCONMAN_OIDC_CLIENT_SECRET="..." \
MC_SURVIVAL_RCON_PASSWORD="..." \
./rconman --config config.yaml
```

For local development with a text log format and a local SQLite file:

```yaml
log:
  format: "text"
  level: "debug"

store:
  path: "./rconman.db"

server:
  base_url: "http://localhost:8080"
```
