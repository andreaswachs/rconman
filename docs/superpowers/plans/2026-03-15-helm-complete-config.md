# Helm Complete Config Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expand the Helm chart so `values.yaml` exposes every app config field and renders a complete `config.yaml`, with multi-option secret resolution including referencing existing K8s Secrets.

**Architecture:** Four files change: `values.yaml` is fully rewritten with structured config + secrets blocks; `configmap.yaml` is rewritten to render the complete `config.yaml`; `secret.yaml` gains per-server RCON password keys; `statefulset.yaml` gains `existingSecret` support for all env vars and per-server RCON password injection.

**Tech Stack:** Helm 3, Kubernetes, Go `gopkg.in/yaml.v3`

**Spec:** `docs/superpowers/specs/2026-03-15-helm-complete-config-design.md`

---

## Chunk 1: values.yaml and configmap.yaml

### Task 1: Rewrite values.yaml

**Files:**
- Modify: `helm/rconman/values.yaml` (full rewrite)

**Context:**
The current `values.yaml` has flat `config.logLevel`/`config.sessionExpiry` and flat `secrets.*` strings. We replace these with a structured `config` block (matching the Go config struct) and a structured `secrets` block (with multi-option comments). All other top-level keys (`replicaCount`, `image`, `service`, `persistence`, `gateway`) are preserved unchanged.

The Go config struct (in `internal/config/config.go`) uses snake_case yaml tags. `values.yaml` uses camelCase (Helm convention) — the ConfigMap template maps between them.

- [ ] **Step 1: Read the current values.yaml to note exact existing content**

Run: `cat helm/rconman/values.yaml`

Expected: See the 32-line file with flat config and secrets blocks.

- [ ] **Step 2: Rewrite values.yaml**

Replace the entire file with:

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
    # Option A: chart-managed
    # value: "your-client-id"
    #
    # Option B: reference an existing K8s Secret
    # existingSecret:
    #   name: "my-oidc-secret"
    #   key: "client-id"
    #
    # Option C: env var (default)
    valueFrom:
      env: RCONMAN_OIDC_CLIENT_ID
    #
    # Option D: file mount
    # valueFrom:
    #   file: /run/secrets/oidc-client-id

  oidcClientSecret:
    # Option A: chart-managed
    # value: "your-client-secret"
    #
    # Option B: reference an existing K8s Secret
    # existingSecret:
    #   name: "my-oidc-secret"
    #   key: "client-secret"
    #
    # Option C: env var (default)
    valueFrom:
      env: RCONMAN_OIDC_CLIENT_SECRET
    #
    # Option D: file mount
    # valueFrom:
    #   file: /run/secrets/oidc-client-secret

  minecraft:
    servers:
      - id: "my-server"
        rconPassword:
          # Option A: chart-managed
          # value: "my-rcon-password"
          #
          # Option B: reference an existing K8s Secret (most common for existing servers)
          # existingSecret:
          #   name: "my-minecraft-secret"
          #   key: "rcon-password"
          #
          # Option C: env var (default)
          valueFrom:
            env: RCONMAN_MINECRAFT_MY_SERVER_RCON_PASSWORD
          #
          # Option D: file mount
          # valueFrom:
          #   file: /run/secrets/minecraft-rcon-password
```

- [ ] **Step 3: Validate the YAML is syntactically correct**

Run: `python3 -c "import yaml; yaml.safe_load(open('helm/rconman/values.yaml'))" && echo "YAML valid"`

Expected: prints `YAML valid`

- [ ] **Step 4: Commit**

```bash
git add helm/rconman/values.yaml
git commit -m "helm: rewrite values.yaml with complete config and multi-option secrets"
```

---

### Task 2: Rewrite configmap.yaml

**Files:**
- Modify: `helm/rconman/templates/configmap.yaml` (full rewrite)

**Context:**
The ConfigMap renders `config.yaml` inside a YAML literal block scalar (`config.yaml: |`). Helm template expressions inside block scalars must be handled carefully:

- **Trim markers (`{{-` and `-}}`) inside the block scalar** strip the surrounding whitespace/newlines, which can break YAML indentation if used carelessly.
- The pattern used here: `{{- range }}` and `{{- end }}` tags are placed at **column 0** (no leading spaces). This causes `{{-` to trim only the preceding newline, leaving the output well-formed.
- Secrets (`session_secret`, `client_id`, etc.) use hardcoded `valueFrom.env` names. The `valueFrom` block in `values.yaml` is documentation only — the StatefulSet controls which secret the env var is pulled from.
- camelCase values keys map to snake_case config.yaml keys (e.g. `values.config.server.baseURL` → `base_url`).

- [ ] **Step 1: Read the current configmap.yaml**

Run: `cat helm/rconman/templates/configmap.yaml`

Expected: See the partial 19-line file.

- [ ] **Step 2: Rewrite configmap.yaml**

Replace the entire file with:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ include "rconman.fullname" . }}-config
  labels:
    {{- include "rconman.labels" . | nindent 4 }}
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
{{- range .Values.config.auth.oidc.scopes }}
          - {{ . | quote }}
{{- end }}
      admin:
        claim:
          name: {{ .Values.config.auth.admin.claim.name | quote }}
          value: {{ .Values.config.auth.admin.claim.value | quote }}
{{- with .Values.config.auth.admin.emailAllowlist }}
        email_allowlist:
{{- range . }}
          - {{ . | quote }}
{{- end }}
{{- end }}
    minecraft:
      servers:
{{- range .Values.config.minecraft.servers }}
        - id: {{ .id | quote }}
          name: {{ .name | quote }}
          rcon:
            host: {{ .rcon.host | quote }}
            port: {{ .rcon.port }}
            password:
              valueFrom:
                env: {{ printf "RCONMAN_MINECRAFT_%s_RCON_PASSWORD" (.id | upper | replace "-" "_") | quote }}
          status_poll_interval: {{ .statusPollInterval | quote }}
{{- with .commands }}
          commands:
{{- toYaml . | indent 12 }}
{{- end }}
{{- end }}
```

- [ ] **Step 3: Verify helm template renders valid output**

Run: `helm template test ./helm/rconman --set secrets.sessionSecret.value=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa 2>&1`

Expected: Output includes a `ConfigMap` with a `config.yaml` key containing properly indented YAML with `server:`, `log:`, `store:`, `auth:`, and `minecraft:` sections. No Helm template errors.

- [ ] **Step 4: Verify rendered config.yaml is valid YAML**

Run:
```bash
helm template test ./helm/rconman \
  --set secrets.sessionSecret.value=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
  | python3 -c "
import sys, yaml
docs = list(yaml.safe_load_all(sys.stdin))
cm = next(d for d in docs if d['kind'] == 'ConfigMap')
cfg = yaml.safe_load(cm['data']['config.yaml'])
for section in ['server', 'log', 'store', 'auth', 'minecraft']:
    assert section in cfg, f'missing section: {section}'
assert cfg['log']['level'] == 'info'
assert cfg['store']['path'] == '/data/rconman.db'
assert cfg['auth']['oidc']['issuer_url'] == 'https://accounts.google.com'
assert cfg['minecraft']['servers'][0]['id'] == 'my-server'
print('config.yaml is valid and complete')
"
```

Expected: prints `config.yaml is valid and complete`

- [ ] **Step 5: Commit**

```bash
git add helm/rconman/templates/configmap.yaml
git commit -m "helm: rewrite configmap to render complete config.yaml"
```

---

## Chunk 2: secret.yaml and statefulset.yaml

### Task 3: Update secret.yaml

**Files:**
- Modify: `helm/rconman/templates/secret.yaml`

**Context:**
The old `values.yaml` had `secrets.sessionSecret` as a plain string. The new structure has it as an object with `.value`, `.existingSecret`, `.valueFrom` fields. The Secret template must now read `.value` from these objects (defaulting to `""` so the key is always present even when `existingSecret` is used).

Per-server RCON passwords are iterated from `secrets.minecraft.servers`.

- [ ] **Step 1: Read current secret.yaml**

Run: `cat helm/rconman/templates/secret.yaml`

Expected: 11-line file with flat `secrets.*` references.

- [ ] **Step 2: Rewrite secret.yaml**

Replace the entire file with:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: {{ include "rconman.fullname" . }}
  labels:
    {{- include "rconman.labels" . | nindent 4 }}
type: Opaque
data:
  session-secret: {{ .Values.secrets.sessionSecret.value | default "" | b64enc | quote }}
  oidc-client-id: {{ .Values.secrets.oidcClientID.value | default "" | b64enc | quote }}
  oidc-client-secret: {{ .Values.secrets.oidcClientSecret.value | default "" | b64enc | quote }}
  {{- range .Values.secrets.minecraft.servers }}
  {{ printf "minecraft-%s-rcon-password" .id }}: {{ .rconPassword.value | default "" | b64enc | quote }}
  {{- end }}
```

- [ ] **Step 3: Verify helm template renders the Secret correctly**

Run:
```bash
helm template test ./helm/rconman \
  --set secrets.sessionSecret.value=mysessionkey12345678901234567890 \
  --set secrets.oidcClientID.value=myclientid \
  --set secrets.oidcClientSecret.value=myclientsecret \
  | python3 -c "
import sys, yaml, base64
docs = list(yaml.safe_load_all(sys.stdin))
secret = next(d for d in docs if d['kind'] == 'Secret')
val = base64.b64decode(secret['data']['session-secret']).decode()
assert val == 'mysessionkey12345678901234567890', f'got: {val}'
assert 'minecraft-my-server-rcon-password' in secret['data']
print('Secret is correct')
"
```

Expected: prints `Secret is correct`

- [ ] **Step 4: Commit**

```bash
git add helm/rconman/templates/secret.yaml
git commit -m "helm: update secret.yaml for structured secrets and per-server RCON passwords"
```

---

### Task 4: Update statefulset.yaml

**Files:**
- Modify: `helm/rconman/templates/statefulset.yaml`

**Context:**
The StatefulSet needs to:
1. Update the three existing `secretKeyRef` entries (`RCONMAN_SESSION_SECRET`, `RCONMAN_OIDC_CLIENT_ID`, `RCONMAN_OIDC_CLIENT_SECRET`) to conditionally use `existingSecret` when set, falling back to the chart-managed Secret.
2. Add per-server RCON password env vars, iterated from `secrets.minecraft.servers`, with the same `existingSecret` fallback pattern.

The env var name for each server is derived from its ID: `my-server` → `RCONMAN_MINECRAFT_MY_SERVER_RCON_PASSWORD`.

Inside a `{{- range }}` loop, use `$` (not `.`) to access the root context for `include "rconman.fullname" $`.

- [ ] **Step 1: Read current statefulset.yaml**

Run: `cat helm/rconman/templates/statefulset.yaml`

Expected: 69-line file with three hardcoded `secretKeyRef` entries all pointing to the chart-managed secret.

- [ ] **Step 2: Replace the three existing env var entries**

In the `env:` section, replace:

```yaml
          env:
            - name: RCONMAN_SESSION_SECRET
              valueFrom:
                secretKeyRef:
                  name: {{ include "rconman.fullname" . }}
                  key: session-secret
            - name: RCONMAN_OIDC_CLIENT_ID
              valueFrom:
                secretKeyRef:
                  name: {{ include "rconman.fullname" . }}
                  key: oidc-client-id
            - name: RCONMAN_OIDC_CLIENT_SECRET
              valueFrom:
                secretKeyRef:
                  name: {{ include "rconman.fullname" . }}
                  key: oidc-client-secret
```

With:

```yaml
          env:
            - name: RCONMAN_SESSION_SECRET
              valueFrom:
                secretKeyRef:
                  name: {{ if .Values.secrets.sessionSecret.existingSecret }}{{ .Values.secrets.sessionSecret.existingSecret.name }}{{ else }}{{ include "rconman.fullname" . }}{{ end }}
                  key: {{ if .Values.secrets.sessionSecret.existingSecret }}{{ .Values.secrets.sessionSecret.existingSecret.key }}{{ else }}session-secret{{ end }}
            - name: RCONMAN_OIDC_CLIENT_ID
              valueFrom:
                secretKeyRef:
                  name: {{ if .Values.secrets.oidcClientID.existingSecret }}{{ .Values.secrets.oidcClientID.existingSecret.name }}{{ else }}{{ include "rconman.fullname" . }}{{ end }}
                  key: {{ if .Values.secrets.oidcClientID.existingSecret }}{{ .Values.secrets.oidcClientID.existingSecret.key }}{{ else }}oidc-client-id{{ end }}
            - name: RCONMAN_OIDC_CLIENT_SECRET
              valueFrom:
                secretKeyRef:
                  name: {{ if .Values.secrets.oidcClientSecret.existingSecret }}{{ .Values.secrets.oidcClientSecret.existingSecret.name }}{{ else }}{{ include "rconman.fullname" . }}{{ end }}
                  key: {{ if .Values.secrets.oidcClientSecret.existingSecret }}{{ .Values.secrets.oidcClientSecret.existingSecret.key }}{{ else }}oidc-client-secret{{ end }}
            {{- range .Values.secrets.minecraft.servers }}
            - name: {{ printf "RCONMAN_MINECRAFT_%s_RCON_PASSWORD" (.id | upper | replace "-" "_") }}
              valueFrom:
                secretKeyRef:
                  name: {{ if .rconPassword.existingSecret }}{{ .rconPassword.existingSecret.name }}{{ else }}{{ include "rconman.fullname" $ }}{{ end }}
                  key: {{ if .rconPassword.existingSecret }}{{ .rconPassword.existingSecret.key }}{{ else }}{{ printf "minecraft-%s-rcon-password" .id }}{{ end }}
            {{- end }}
```

- [ ] **Step 3: Verify helm template renders env vars for chart-managed secret path**

Run:
```bash
helm template test ./helm/rconman \
  --set secrets.sessionSecret.value=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
  | python3 -c "
import sys, yaml
docs = list(yaml.safe_load_all(sys.stdin))
sts = next(d for d in docs if d['kind'] == 'StatefulSet')
envs = {e['name']: e['valueFrom']['secretKeyRef'] for e in sts['spec']['template']['spec']['containers'][0]['env']}
assert envs['RCONMAN_SESSION_SECRET']['key'] == 'session-secret'
assert envs['RCONMAN_SESSION_SECRET']['name'] == 'test-rconman', f"got: {envs['RCONMAN_SESSION_SECRET']['name']}"
assert envs['RCONMAN_OIDC_CLIENT_ID']['key'] == 'oidc-client-id'
assert 'RCONMAN_MINECRAFT_MY_SERVER_RCON_PASSWORD' in envs
assert envs['RCONMAN_MINECRAFT_MY_SERVER_RCON_PASSWORD']['key'] == 'minecraft-my-server-rcon-password'
assert envs['RCONMAN_MINECRAFT_MY_SERVER_RCON_PASSWORD']['name'] == 'test-rconman'
print('StatefulSet env vars correct (chart-managed path)')
"
```

Expected: prints `StatefulSet env vars correct (chart-managed path)`

- [ ] **Step 4: Verify helm template renders env vars for existingSecret path**

Run:
```bash
helm template test ./helm/rconman \
  --set secrets.sessionSecret.existingSecret.name=my-cluster-secret \
  --set secrets.sessionSecret.existingSecret.key=session \
  --set "secrets.minecraft.servers[0].id=my-server" \
  --set "secrets.minecraft.servers[0].rconPassword.existingSecret.name=mc-secret" \
  --set "secrets.minecraft.servers[0].rconPassword.existingSecret.key=rcon-pass" \
  | python3 -c "
import sys, yaml
docs = list(yaml.safe_load_all(sys.stdin))
sts = next(d for d in docs if d['kind'] == 'StatefulSet')
envs = {e['name']: e['valueFrom']['secretKeyRef'] for e in sts['spec']['template']['spec']['containers'][0]['env']}
assert envs['RCONMAN_SESSION_SECRET']['name'] == 'my-cluster-secret', f\"got: {envs['RCONMAN_SESSION_SECRET']}\"
assert envs['RCONMAN_SESSION_SECRET']['key'] == 'session'
assert envs['RCONMAN_MINECRAFT_MY_SERVER_RCON_PASSWORD']['name'] == 'mc-secret'
assert envs['RCONMAN_MINECRAFT_MY_SERVER_RCON_PASSWORD']['key'] == 'rcon-pass'
print('StatefulSet env vars correct (existingSecret path)')
"
```

Expected: prints `StatefulSet env vars correct (existingSecret path)`

- [ ] **Step 5: Commit**

```bash
git add helm/rconman/templates/statefulset.yaml
git commit -m "helm: add existingSecret support and per-server RCON password env vars"
```

---

## Chunk 3: Chart version bump and final verification

### Task 5: Bump chart version and run full helm template verification

**Files:**
- Modify: `helm/rconman/Chart.yaml`

**Context:**
The `values.yaml` structure changed significantly (breaking change for anyone upgrading from 0.1.0 — flat `secrets.sessionSecret` string is now an object). Bump the minor version: `0.1.0` → `0.2.0`.

- [ ] **Step 1: Read current Chart.yaml**

Run: `cat helm/rconman/Chart.yaml`

Expected: `version: 0.1.0` and `appVersion: "0.1.0"`

- [ ] **Step 2: Bump chart version to 0.2.0**

Edit `helm/rconman/Chart.yaml`, changing both `version` and `appVersion` fields:

```yaml
apiVersion: v2
name: rconman
description: Authenticated web UI for RCON server management
type: application
version: 0.2.0
appVersion: "0.1.0"
```

- [ ] **Step 3: Run helm lint**

Run: `helm lint helm/rconman/`

Expected: `1 chart(s) linted, 0 chart(s) failed`

- [ ] **Step 4: Run full helm template smoke test**

Run:
```bash
helm template test ./helm/rconman \
  --set secrets.sessionSecret.value=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
  --set secrets.oidcClientID.value=client123 \
  --set secrets.oidcClientSecret.value=secret456 \
  --set "secrets.minecraft.servers[0].id=my-server" \
  --set "secrets.minecraft.servers[0].rconPassword.value=rconpass" \
  > /tmp/rconman-rendered.yaml && echo "Template rendered OK"
```

Expected: prints `Template rendered OK`, no errors.

- [ ] **Step 5: Verify all K8s resources are present in rendered output**

Run:
```bash
python3 -c "
import yaml
docs = list(yaml.safe_load_all(open('/tmp/rconman-rendered.yaml')))
kinds = {d['kind'] for d in docs if d}
assert 'StatefulSet' in kinds, 'missing StatefulSet'
assert 'ConfigMap' in kinds, 'missing ConfigMap'
assert 'Secret' in kinds, 'missing Secret'
assert 'Service' in kinds, 'missing Service'
print('All resources present:', sorted(kinds))
"
```

Expected: prints `All resources present: ['ConfigMap', 'Secret', 'Service', 'StatefulSet']` (plus HTTPRoute if gateway enabled)

- [ ] **Step 6: Verify rendered config.yaml is complete and valid**

Run:
```bash
python3 -c "
import yaml
docs = list(yaml.safe_load_all(open('/tmp/rconman-rendered.yaml')))
cm = next(d for d in docs if d and d['kind'] == 'ConfigMap')
cfg = yaml.safe_load(cm['data']['config.yaml'])

# Check all top-level sections
for section in ['server', 'log', 'store', 'auth', 'minecraft']:
    assert section in cfg, f'missing section: {section}'

# Check server fields
assert cfg['server']['base_url'] == 'https://rconman.example.com'
assert cfg['server']['insecure_mode'] == False
assert cfg['server']['session_secret']['valueFrom']['env'] == 'RCONMAN_SESSION_SECRET'

# Check auth section
assert cfg['auth']['oidc']['issuer_url'] == 'https://accounts.google.com'
assert cfg['auth']['oidc']['client_id']['valueFrom']['env'] == 'RCONMAN_OIDC_CLIENT_ID'
assert 'openid' in cfg['auth']['oidc']['scopes']
assert cfg['auth']['admin']['claim']['name'] == 'email'

# Check minecraft section
servers = cfg['minecraft']['servers']
assert len(servers) == 1
assert servers[0]['id'] == 'my-server'
assert servers[0]['rcon']['password']['valueFrom']['env'] == 'RCONMAN_MINECRAFT_MY_SERVER_RCON_PASSWORD'

print('config.yaml passes all assertions')
"
```

Expected: prints `config.yaml passes all assertions`

- [ ] **Step 7: Commit**

```bash
git add helm/rconman/Chart.yaml
git commit -m "helm: bump chart version to 0.2.0

Breaking change: secrets.* values changed from plain strings to objects.
Upgrade from 0.1.0 requires updating values.yaml secrets block."
```

---

## Summary

**Files changed:**
- `helm/rconman/values.yaml` — complete structured config + multi-option secrets
- `helm/rconman/templates/configmap.yaml` — renders full `config.yaml`
- `helm/rconman/templates/secret.yaml` — per-server RCON password keys
- `helm/rconman/templates/statefulset.yaml` — `existingSecret` for all env vars
- `helm/rconman/Chart.yaml` — version 0.2.0

**Key behaviours after this change:**
- `helm template` renders a complete, parseable `config.yaml` with all sections
- Users referencing an existing secret set `existingSecret.name` + `existingSecret.key` under the relevant `secrets.*` field
- Per-server RCON env var name: `RCONMAN_MINECRAFT_<ID_UPPERCASED>_RCON_PASSWORD`
- Chart-managed secret still works via `value:` field
- `commands` block is commented out in example but functional when uncommented
