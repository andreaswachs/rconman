# Helm Chart Complete Config Design Specification

**Date:** 2026-03-15
**Status:** Draft

## Overview

Expand the Helm chart so `values.yaml` exposes the complete application config structure, renders a full `config.yaml` via the ConfigMap, and supports multiple secret resolution strategies — including referencing existing K8s Secrets — for all sensitive fields.

## Problem

The current ConfigMap only renders 6 fields (`server.host`, `server.port`, `server.session_expiry`, `log.level`, `log.format`, `store.path`, `store.retention`). The entire `auth` section (OIDC issuer, scopes, admin claim/allowlist) and `minecraft.servers` list are absent, making the chart unusable without manual post-install patching.

Additionally, `secrets.minecraftRconPassword` exists in `values.yaml` but is never written to the K8s Secret or injected as an env var — it is a dead field.

## Requirements

1. `values.yaml` exposes every field of the application config struct
2. An example `minecraft.servers` entry is included with all fields present; the `commands` block is commented out
3. All sensitive fields show all resolution options as comments: inline value, existing secret reference, env var, file mount
4. Per-server RCON password supports referencing an existing K8s Secret by name and key
5. Global secrets (session secret, OIDC client ID/secret) support the same existing-secret pattern
6. ConfigMap renders the complete `config.yaml` consumed by the app
7. StatefulSet injects env vars using either the chart-managed Secret or a user-supplied existing Secret, depending on which `existingSecret` block is set

## Files Changed

| File | Change |
|------|--------|
| `helm/rconman/values.yaml` | Full rewrite — structured config block, all secrets with multi-option comments |
| `helm/rconman/templates/configmap.yaml` | Full rewrite — renders complete `config.yaml` |
| `helm/rconman/templates/secret.yaml` | Add `minecraft-rcon-password` key |
| `helm/rconman/templates/statefulset.yaml` | Add minecraft RCON password env var; support `existingSecret` for all secrets |

## Design

### values.yaml Structure

```yaml
replicaCount: 1

image:
  repository: ghcr.io/your-org/rconman
  pullPolicy: IfNotPresent
  tag: "latest"

service:
  type: ClusterIP
  port: 8080

persistence:
  enabled: true
  size: 1Gi
  storageClassName: ""

gateway:
  enabled: false
  parentRef:
    name: ""
    namespace: ""
    sectionName: ""

config:
  server:
    baseURL: "https://rconman.example.com"
    insecureMode: false
    sessionExpiry: "24h"
  log:
    level: "info"
    format: "text"
  store:
    path: "/data/rconman.db"
    retention: "30d"
  auth:
    oidc:
      issuerURL: "https://accounts.google.com"
      scopes:
        - openid
        - email
        - profile
    admin:
      claim:
        name: "email"
        value: ""
      # emailAllowlist:
      #   - "admin@example.com"
  minecraft:
    servers:
      - id: "my-server"
        name: "My Minecraft Server"
        rcon:
          host: "minecraft.example.com"
          port: 25575
        statusPollInterval: "30s"
        # commands:
        #   - category: "Player Management"
        #     templates:
        #       - name: "Kick Player"
        #         description: "Kick a player from the server"
        #         command: "kick <player> <reason>"
        #         params:
        #           - name: player
        #             type: string
        #           - name: reason
        #             type: string
        #             default: "Kicked by admin"

secrets:
  sessionSecret:
    # Option A: chart-managed — set value here, chart creates the K8s Secret
    # value: "your-32-byte-minimum-secret-here"
    #
    # Option B: reference an existing K8s Secret
    # existingSecret:
    #   name: "my-existing-secret"
    #   key: "session-secret"
    #
    # Option C: env var — default, works automatically with Option A
    valueFrom:
      env: RCONMAN_SESSION_SECRET
    #
    # Option D: file mount
    # valueFrom:
    #   file: /run/secrets/session-secret

  oidcClientID:
    # value: "your-client-id"
    # existingSecret:
    #   name: "my-oidc-secret"
    #   key: "client-id"
    valueFrom:
      env: RCONMAN_OIDC_CLIENT_ID
    # valueFrom:
    #   file: /run/secrets/oidc-client-id

  oidcClientSecret:
    # value: "your-client-secret"
    # existingSecret:
    #   name: "my-oidc-secret"
    #   key: "client-secret"
    valueFrom:
      env: RCONMAN_OIDC_CLIENT_SECRET
    # valueFrom:
    #   file: /run/secrets/oidc-client-secret

  minecraft:
    servers:
      - id: "my-server"
        rconPassword:
          # value: "my-rcon-password"
          #
          # Option B: reference an existing K8s Secret (most common for existing servers)
          # existingSecret:
          #   name: "my-minecraft-secret"
          #   key: "rcon-password"
          valueFrom:
            env: RCONMAN_MINECRAFT_MY_SERVER_RCON_PASSWORD
          # valueFrom:
          #   file: /run/secrets/minecraft-rcon-password
```

### ConfigMap Template

Renders the complete `config.yaml`. Secrets are **always** hardcoded to fixed env var names (`RCONMAN_SESSION_SECRET`, etc.) — the `valueFrom` block in `values.yaml` is documentation only. This is intentional: the StatefulSet always injects those env vars from whichever secret source is configured, so the config.yaml env var names never need to change.

**Note on camelCase → snake_case:** `values.yaml` uses camelCase keys (Helm convention); the rendered `config.yaml` uses snake_case (Go yaml tag convention). e.g. `values.config.server.baseURL` → `base_url:`.

**Note on Helm whitespace in literal block scalars:** the ConfigMap uses a YAML literal block scalar (`config.yaml: |`). All `{{- }}` trim markers inside it must be written as `{{ }}` (no trim) or use `indent`/`nindent` to preserve indentation. Whitespace is significant inside the block scalar.

```yaml
data:
  config.yaml: |
    server:
      host: "0.0.0.0"
      port: 8080
      base_url: {{ .Values.config.server.baseURL | quote }}
      insecure_mode: {{ .Values.config.server.insecureMode }}
      session_expiry: {{ .Values.config.server.sessionExpiry | quote }}
      session_secret:
        valueFrom:
          env: RCONMAN_SESSION_SECRET
    log:
      level: {{ .Values.config.log.level | quote }}
      format: {{ .Values.config.log.format | quote }}
    store:
      path: {{ .Values.config.store.path | quote }}
      retention: {{ .Values.config.store.retention | quote }}
    auth:
      oidc:
        issuer_url: {{ .Values.config.auth.oidc.issuerURL | quote }}
        client_id:
          valueFrom:
            env: RCONMAN_OIDC_CLIENT_ID
        client_secret:
          valueFrom:
            env: RCONMAN_OIDC_CLIENT_SECRET
        scopes:
{{ range .Values.config.auth.oidc.scopes }}        - {{ . | quote }}
{{ end }}      admin:
        claim:
          name: {{ .Values.config.auth.admin.claim.name | quote }}
          value: {{ .Values.config.auth.admin.claim.value | quote }}
{{ if .Values.config.auth.admin.emailAllowlist }}        email_allowlist:
{{ range .Values.config.auth.admin.emailAllowlist }}        - {{ . | quote }}
{{ end }}{{ end }}    minecraft:
      servers:
{{ range .Values.config.minecraft.servers }}      - id: {{ .id | quote }}
          name: {{ .name | quote }}
          rcon:
            host: {{ .rcon.host | quote }}
            port: {{ .rcon.port }}
            password:
              valueFrom:
                env: {{ printf "RCONMAN_MINECRAFT_%s_RCON_PASSWORD" (.id | upper | replace "-" "_") | quote }}
          status_poll_interval: {{ .statusPollInterval | quote }}
{{ if .commands }}          commands:
{{ toYaml .commands | indent 10 }}{{ end }}{{ end }}
```

In practice the implementor must verify indentation of the rendered output with `helm template` and adjust spacing accordingly.

### Secret Template

The chart-managed Secret is always created, even when `existingSecret` is configured (the keys will be empty in that case, which is harmless — the StatefulSet will reference the external secret instead). This avoids conditional template complexity.

```yaml
data:
  session-secret: {{ .Values.secrets.sessionSecret.value | default "" | b64enc | quote }}
  oidc-client-id: {{ .Values.secrets.oidcClientID.value | default "" | b64enc | quote }}
  oidc-client-secret: {{ .Values.secrets.oidcClientSecret.value | default "" | b64enc | quote }}
  {{- range .Values.secrets.minecraft.servers }}
  {{ printf "minecraft-%s-rcon-password" .id }}: {{ .rconPassword.value | default "" | b64enc | quote }}
  {{- end }}
```

### StatefulSet Env Var Injection

For each secret field, the StatefulSet uses a helper to resolve whether to use the chart-managed Secret or an existing Secret:

**Session Secret:**
```yaml
- name: RCONMAN_SESSION_SECRET
  valueFrom:
    secretKeyRef:
      name: {{ if .Values.secrets.sessionSecret.existingSecret }}
              {{ .Values.secrets.sessionSecret.existingSecret.name }}
            {{ else }}
              {{ include "rconman.fullname" . }}
            {{ end }}
      key: {{ if .Values.secrets.sessionSecret.existingSecret }}
              {{ .Values.secrets.sessionSecret.existingSecret.key }}
            {{ else }}
              session-secret
            {{ end }}
```

**Per-server RCON password** — iterated from `secrets.minecraft.servers` only (independent of `config.minecraft.servers`). The two lists must have matching IDs; no automatic cross-referencing is performed. A server present in `config` but absent from `secrets` will have no env var injected, causing an app startup failure.

```yaml
{{- range .Values.secrets.minecraft.servers }}
- name: {{ printf "RCONMAN_MINECRAFT_%s_RCON_PASSWORD" (.id | upper | replace "-" "_") }}
  valueFrom:
    secretKeyRef:
      name: {{ if .rconPassword.existingSecret }}{{ .rconPassword.existingSecret.name }}{{ else }}<chart-secret-name>{{ end }}
      key: {{ if .rconPassword.existingSecret }}{{ .rconPassword.existingSecret.key }}{{ else }}{{ printf "minecraft-%s-rcon-password" .id }}{{ end }}
{{- end }}
```

### Env Var Naming Convention

Each server's RCON password env var is derived from its ID:
- Server `my-server` → `RCONMAN_MINECRAFT_MY_SERVER_RCON_PASSWORD`
- Server `survival-1` → `RCONMAN_MINECRAFT_SURVIVAL_1_RCON_PASSWORD`

This allows multiple servers, each with independent secret references.

## Data Flow

```
values.yaml
  └── secrets.minecraft.servers[].rconPassword.existingSecret
        └── StatefulSet: secretKeyRef → existing K8s Secret
              └── env var: RCONMAN_MINECRAFT_MY_SERVER_RCON_PASSWORD
                    └── ConfigMap config.yaml: password.valueFrom.env
                          └── App: SecretValue.Resolve() → password string
```

## Error Cases

| Scenario | Behaviour |
|----------|-----------|
| `existingSecret` set but secret doesn't exist in cluster | Pod fails to start (K8s secretKeyRef error) |
| Neither `value` nor `existingSecret` set, chart secret has empty value | App fails at startup: `session_secret must be at least 32 bytes` |
| Server in `config.minecraft.servers` has no matching entry in `secrets.minecraft.servers` | Env var absent; app fails at startup with missing password error |
| `oidc.client_id` env var missing (Go's `Validate()` does not check for nil client_id) | App fails at OIDC init at runtime rather than at startup validation — known gap in `config.go` |
| `commands` block uses `{{ }}` syntax in values | Helm renders it via `toYaml` — no conflict as it's not a template expression |

## Success Criteria

- `helm template` renders a complete, valid `config.yaml` with all sections
- `existingSecret` reference for RCON password injects the correct `secretKeyRef` in the StatefulSet
- Chart-managed secret path still works (inline `value` in `secrets.*`)
- All sensitive fields show all four options in `values.yaml` comments
- `commands` block is commented out in the example but works correctly when uncommented
