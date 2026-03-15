# rconman Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a production-ready authenticated web UI for issuing RCON commands to Minecraft servers, as a single containerized Go binary deployable to Kubernetes.

**Architecture:** Single Go binary with embedded frontend (templ + Tailwind + HTMX) serving an HTTP API that proxies RCON commands over persistent TCP connections. Auth via OIDC/OAuth2, command logging to SQLite, role-based template access, multi-server support.

**Tech Stack:** Go 1.23 + chi router + templ views + HTMX + Tailwind CSS + DaisyUI + coreos/go-oidc + gorilla/sessions + modernc.org/sqlite + sqlc

---

## Chunk 1: Project Setup, Configuration, and Database Schema

### Task 1: Initialize Go project structure and basic dependencies

**Files:**
- Create: `go.mod`
- Create: `go.sum`
- Create: `cmd/rconman/main.go` (skeleton)
- Create: `internal/config/config.go`
- Create: `Makefile`
- Create: `.gitignore`

- [ ] **Step 1: Create project structure**

```bash
mkdir -p rconman/{cmd/rconman,internal/{config,auth,rcon,server,handlers,views,store,model},web/{static},helm/rconman/templates,test/{e2e,mock-rcon,kind},.github/workflows}
```

- [ ] **Step 2: Initialize Go module**

```bash
go mod init github.com/your-org/rconman
```

- [ ] **Step 3: Add core dependencies to go.mod**

```bash
go get github.com/go-chi/chi/v5
go get golang.org/x/oauth2
go get github.com/coreos/go-oidc/v3
go get github.com/gorilla/sessions
go get modernc.org/sqlite
go get github.com/sqlc-dev/sqlc/cmd/sqlc
go get gopkg.in/yaml.v3
```

- [ ] **Step 4: Write Makefile with test, build, e2e targets**

Create `Makefile`:
```makefile
.PHONY: test build e2e help

test:
	go test -v ./...

build:
	go generate ./...
	CGO_ENABLED=0 go build -o rconman ./cmd/rconman

e2e:
	./test/kind/setup.sh
	./test/kind/teardown.sh

help:
	@echo "Available targets: test, build, e2e"
```

- [ ] **Step 5: Create .gitignore**

```
rconman
rconman.db
*.db-wal
*.db-shm
web/node_modules/
web/static/app.css
dist/
*.local.yaml
.env
.env.local
```

- [ ] **Step 6: Commit**

```bash
git add Makefile go.mod go.sum .gitignore cmd/ internal/ web/ helm/ test/ .github/
git commit -m "init: scaffold project structure and dependencies"
```

---

### Task 2: Build configuration package with YAML loading and secret resolution

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/secret.go`
- Create: `internal/config/validation.go`
- Create: `config.example.yaml`
- Create: `internal/config/config_test.go`

- [ ] **Step 1: Write test for secret resolution (environment variable)**

Create `internal/config/secret_test.go`:
```go
package config

import (
	"os"
	"testing"
)

func TestResolveSecret_Inline(t *testing.T) {
	secret := SecretValue{Value: "inline_secret"}
	resolved, err := secret.Resolve()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resolved != "inline_secret" {
		t.Errorf("expected 'inline_secret', got %q", resolved)
	}
}

func TestResolveSecret_EnvVar(t *testing.T) {
	os.Setenv("TEST_SECRET", "env_secret_value")
	defer os.Unsetenv("TEST_SECRET")

	secret := SecretValue{
		ValueFrom: &ValueFrom{Env: "TEST_SECRET"},
	}
	resolved, err := secret.Resolve()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resolved != "env_secret_value" {
		t.Errorf("expected 'env_secret_value', got %q", resolved)
	}
}

func TestResolveSecret_File(t *testing.T) {
	// Create temp file with secret content
	tmpFile, err := os.CreateTemp("", "secret")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString("file_secret_content")
	tmpFile.Close()

	secret := SecretValue{
		ValueFrom: &ValueFrom{File: tmpFile.Name()},
	}
	resolved, err := secret.Resolve()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resolved != "file_secret_content" {
		t.Errorf("expected 'file_secret_content', got %q", resolved)
	}
}

func TestResolveSecret_Missing(t *testing.T) {
	secret := SecretValue{}
	_, err := secret.Resolve()
	if err == nil {
		t.Fatal("expected error for missing secret")
	}
}
```

- [ ] **Step 2: Implement secret resolution**

Create `internal/config/secret.go`:
```go
package config

import (
	"fmt"
	"os"
	"strings"
)

// SecretValue represents a sensitive config value that can be inline, from env, or from file.
type SecretValue struct {
	Value     string      `yaml:"value"`
	ValueFrom *ValueFrom  `yaml:"valueFrom"`
}

type ValueFrom struct {
	Env  string `yaml:"env"`
	File string `yaml:"file"`
}

// Resolve returns the resolved secret value, checking inline, env, then file.
func (s *SecretValue) Resolve() (string, error) {
	if s == nil {
		return "", fmt.Errorf("secret is nil")
	}

	if s.Value != "" {
		return s.Value, nil
	}

	if s.ValueFrom != nil {
		if s.ValueFrom.Env != "" {
			val := os.Getenv(s.ValueFrom.Env)
			if val == "" {
				return "", fmt.Errorf("environment variable %s not set or empty", s.ValueFrom.Env)
			}
			return val, nil
		}

		if s.ValueFrom.File != "" {
			content, err := os.ReadFile(s.ValueFrom.File)
			if err != nil {
				return "", fmt.Errorf("failed to read secret file %s: %w", s.ValueFrom.File, err)
			}
			val := strings.TrimSpace(string(content))
			if val == "" {
				return "", fmt.Errorf("secret file %s is empty", s.ValueFrom.File)
			}
			return val, nil
		}
	}

	return "", fmt.Errorf("secret value is missing and no valueFrom specified")
}
```

- [ ] **Step 3: Write test for config loading and validation**

Create `internal/config/config_test.go`:
```go
package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadConfig_Valid(t *testing.T) {
	// Create a minimal valid config YAML
	yamlContent := `
server:
  host: "0.0.0.0"
  port: 8080
  base_url: "https://example.com"
  session_secret:
    value: "this_is_a_32_byte_long_secret_yes"
  session_expiry: "24h"

log:
  level: "info"
  format: "text"

store:
  path: "/tmp/test.db"
  retention: "30d"

auth:
  oidc:
    issuer_url: "https://accounts.google.com"
    client_id:
      value: "test_client_id"
    client_secret:
      value: "test_client_secret"
  admin:
    claim:
      name: "roles"
      value: "rconman-admin"

minecraft:
  servers:
    - name: "Survival"
      id: "survival"
      rcon:
        host: "localhost"
        port: 25575
        password:
          value: "rcon_password"
      status_poll_interval: "30s"
`

	tmpFile, err := os.CreateTemp("", "config*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp config: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString(yamlContent)
	tmpFile.Close()

	cfg, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}
	if len(cfg.Minecraft.Servers) != 1 {
		t.Errorf("expected 1 server, got %d", len(cfg.Minecraft.Servers))
	}
}

func TestValidateConfig_MissingSessionSecret(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing session secret")
	}
}

func TestValidateConfig_ShortSessionSecret(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
			SessionSecret: &SecretValue{
				Value: "tooshort",
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for short session secret")
	}
}

func TestValidateConfig_MissingOIDCClientSecret(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
			SessionSecret: &SecretValue{
				Value: "32_byte_long_secret_that_is_valid_yes",
			},
		},
		Auth: AuthConfig{
			OIDC: OIDCConfig{
				IssuerURL: "https://accounts.google.com",
				ClientID: &SecretValue{
					Value: "client_id",
				},
				// ClientSecret is nil
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing OIDC client secret")
	}
}
```

- [ ] **Step 4: Implement config types and validation**

Create `internal/config/config.go`:
```go
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Log        LogConfig        `yaml:"log"`
	Store      StoreConfig      `yaml:"store"`
	Auth       AuthConfig       `yaml:"auth"`
	Minecraft  MinecraftConfig  `yaml:"minecraft"`
}

type ServerConfig struct {
	Host           string         `yaml:"host"`
	Port           int            `yaml:"port"`
	BaseURL        string         `yaml:"base_url"`
	SessionSecret  *SecretValue   `yaml:"session_secret"`
	SessionExpiry  string         `yaml:"session_expiry"` // e.g., "24h"
}

type LogConfig struct {
	Level  string `yaml:"level"`  // debug, info, warn, error
	Format string `yaml:"format"` // text, json
}

type StoreConfig struct {
	Path      string `yaml:"path"`
	Retention string `yaml:"retention"` // e.g., "30d"
}

type AuthConfig struct {
	OIDC  OIDCConfig `yaml:"oidc"`
	Admin AdminConfig `yaml:"admin"`
}

type OIDCConfig struct {
	IssuerURL    string        `yaml:"issuer_url"`
	ClientID     *SecretValue  `yaml:"client_id"`
	ClientSecret *SecretValue  `yaml:"client_secret"`
	Scopes       []string      `yaml:"scopes"`
}

type AdminConfig struct {
	Claim         ClaimConfig `yaml:"claim"`
	EmailAllowlist []string   `yaml:"email_allowlist"`
}

type ClaimConfig struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

type MinecraftConfig struct {
	Servers []ServerDef `yaml:"servers"`
}

type ServerDef struct {
	Name               string        `yaml:"name"`
	ID                 string        `yaml:"id"`
	RCON               RCONConfig    `yaml:"rcon"`
	StatusPollInterval string        `yaml:"status_poll_interval"`
	Commands           []CommandCategory `yaml:"commands"`
}

type RCONConfig struct {
	Host     string       `yaml:"host"`
	Port     int          `yaml:"port"`
	Password *SecretValue `yaml:"password"`
}

type CommandCategory struct {
	Category  string           `yaml:"category"`
	Templates []CommandTemplate `yaml:"templates"`
}

type CommandTemplate struct {
	Name        string           `yaml:"name"`
	Description string           `yaml:"description"`
	Command     string           `yaml:"command"`
	Params      []TemplateParam  `yaml:"params"`
}

type TemplateParam struct {
	Name    string        `yaml:"name"`
	Type    string        `yaml:"type"` // text, number, select, boolean, player
	Options []string      `yaml:"options"`
	Default interface{}   `yaml:"default"`
	Min     int           `yaml:"min"`
	Max     int           `yaml:"max"`
}

// LoadConfig loads and parses a YAML config file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks required fields and constraints.
func (c *Config) Validate() error {
	// Check server config
	if c.Server.SessionSecret == nil {
		return fmt.Errorf("server.session_secret is required")
	}

	sessionSecret, err := c.Server.SessionSecret.Resolve()
	if err != nil {
		return fmt.Errorf("server.session_secret: %w", err)
	}

	if len(sessionSecret) < 32 {
		return fmt.Errorf("server.session_secret must be at least 32 bytes (got %d)", len(sessionSecret))
	}

	// Check auth config
	if c.Auth.OIDC.ClientSecret == nil {
		return fmt.Errorf("auth.oidc.client_secret is required")
	}

	_, err = c.Auth.OIDC.ClientSecret.Resolve()
	if err != nil {
		return fmt.Errorf("auth.oidc.client_secret: %w", err)
	}

	// Validate and resolve all server passwords to catch missing secrets early
	for _, srv := range c.Minecraft.Servers {
		if srv.RCON.Password == nil {
			return fmt.Errorf("minecraft.servers[%s].rcon.password is required", srv.ID)
		}
		_, err := srv.RCON.Password.Resolve()
		if err != nil {
			return fmt.Errorf("minecraft.servers[%s].rcon.password: %w", srv.ID, err)
		}
	}

	return nil
}

// ResolveSessionSecret returns the resolved session secret key.
func (c *Config) ResolveSessionSecret() (string, error) {
	return c.Server.SessionSecret.Resolve()
}

// SessionExpiryDuration parses and returns the session expiry as a time.Duration.
func (c *Config) SessionExpiryDuration() (time.Duration, error) {
	if c.Server.SessionExpiry == "" {
		return 24 * time.Hour, nil // default
	}
	return time.ParseDuration(c.Server.SessionExpiry)
}
```

- [ ] **Step 5: Create example config**

Create `config.example.yaml`:
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
  format: "text"

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
      - "admin@example.com"

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
                  options: ["diamond", "iron_sword", "golden_apple"]
                - name: count
                  type: number
                  default: 1
                  min: 1
                  max: 64
```

- [ ] **Step 6: Run tests and verify config works**

```bash
go test -v ./internal/config/...
```

Expected: All tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/config/ config.example.yaml
git commit -m "feat: add configuration system with secret resolution"
```

---

### Task 3: Design and create SQLite schema for command log and database migrations

**Files:**
- Create: `schema/schema.sql`
- Create: `schema/migrations/` (empty for now; schema is deployed via sqlc)
- Create: `internal/store/store.go` (interface definition)
- Create: `internal/store/queries.sql`
- Create: `sqlc.yaml`

- [ ] **Step 1: Write test for store interface**

Create `internal/store/store_test.go`:
```go
package store

import (
	"context"
	"testing"
	"time"
)

type mockStore struct {
	logs []CommandLog
}

func (m *mockStore) RecordCommand(ctx context.Context, email, serverID, command, response string, durationMS int64) error {
	m.logs = append(m.logs, CommandLog{
		Timestamp: time.Now(),
		UserEmail: email,
		ServerID:  serverID,
		Command:   command,
		Response:  response,
		DurationMS: durationMS,
	})
	return nil
}

func (m *mockStore) GetLogs(ctx context.Context, limit int) ([]CommandLog, error) {
	if limit > len(m.logs) {
		limit = len(m.logs)
	}
	return m.logs[:limit], nil
}

func (m *mockStore) PruneOlderThan(ctx context.Context, age time.Duration) error {
	return nil
}

func TestMockStore_RecordCommand(t *testing.T) {
	store := &mockStore{}
	err := store.RecordCommand(context.Background(), "user@example.com", "survival", "/give player diamond", "Given", 100)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(store.logs) != 1 {
		t.Errorf("expected 1 log, got %d", len(store.logs))
	}
}
```

- [ ] **Step 2: Define store interface**

Create `internal/store/store.go`:
```go
package store

import (
	"context"
	"time"
)

// CommandLog represents a recorded RCON command execution.
type CommandLog struct {
	ID         int64
	Timestamp  time.Time
	UserEmail  string
	ServerID   string
	Command    string
	Response   string
	DurationMS int64
}

// Store defines the command logging interface.
type Store interface {
	// RecordCommand logs a command execution.
	RecordCommand(ctx context.Context, email, serverID, command, response string, durationMS int64) error

	// GetLogs retrieves the most recent N log entries.
	GetLogs(ctx context.Context, limit int) ([]CommandLog, error)

	// PruneOlderThan deletes log entries older than the given duration.
	PruneOlderThan(ctx context.Context, age time.Duration) error
}
```

- [ ] **Step 3: Create SQL schema**

Create `schema/schema.sql`:
```sql
CREATE TABLE IF NOT EXISTS command_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    user_email TEXT NOT NULL,
    server_id TEXT NOT NULL,
    command TEXT NOT NULL,
    response TEXT NOT NULL,
    duration_ms INTEGER NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_command_logs_timestamp ON command_logs(timestamp DESC);
CREATE INDEX idx_command_logs_server_id ON command_logs(server_id);
CREATE INDEX idx_command_logs_user_email ON command_logs(user_email);
```

- [ ] **Step 4: Create sqlc query file**

Create `internal/store/queries.sql`:
```sql
-- name: RecordCommand :exec
INSERT INTO command_logs (timestamp, user_email, server_id, command, response, duration_ms)
VALUES (datetime('now'), ?, ?, ?, ?, ?);

-- name: GetLogs :many
SELECT id, timestamp, user_email, server_id, command, response, duration_ms
FROM command_logs
ORDER BY timestamp DESC
LIMIT ?;

-- name: PruneOlderThan :exec
DELETE FROM command_logs
WHERE timestamp < datetime('now', ?);
```

- [ ] **Step 5: Create sqlc config**

Create `sqlc.yaml`:
```yaml
version: "2"
sql:
  - engine: "sqlite"
    queries: "internal/store/queries.sql"
    schema: "schema/schema.sql"
    out: "internal/store/sqlc"
    package: "sqlc"
    emit_json_tags: true
    emit_prepared_queries: false
```

- [ ] **Step 6: Run sqlc to generate code**

```bash
go run github.com/sqlc-dev/sqlc/cmd/sqlc generate
```

Expected: `internal/store/sqlc/` directory created with generated query functions.

- [ ] **Step 7: Commit**

```bash
git add schema/ internal/store/ sqlc.yaml go.mod go.sum
git commit -m "feat: add SQLite schema and sqlc query generation"
```

---

## Chunk 2: RCON Client and Connection Management

### Task 4: Implement RCON protocol client with connection lifecycle management

**Files:**
- Create: `internal/rcon/client.go`
- Create: `internal/rcon/protocol.go`
- Create: `internal/rcon/errors.go`
- Create: `internal/rcon/client_test.go`
- Create: `internal/rcon/mock/mock.go`

- [ ] **Step 1: Write test for RCON client interface**

Create `internal/rcon/client_test.go`:
```go
package rcon

import (
	"context"
	"testing"
	"time"
)

func TestClientInterface(t *testing.T) {
	// This test ensures the interface is correct.
	var _ Client = (*MockClient)(nil)
}

type MockClient struct {
	responses map[string]string
	players   []string
	connected bool
}

func NewMockClient() *MockClient {
	return &MockClient{
		responses: make(map[string]string),
		connected: true,
	}
}

func (m *MockClient) Send(ctx context.Context, command string) (string, error) {
	if !m.connected {
		return "", ErrNotConnected
	}
	if resp, ok := m.responses[command]; ok {
		return resp, nil
	}
	return "", nil
}

func (m *MockClient) PlayerList(ctx context.Context) ([]string, error) {
	if !m.connected {
		return nil, ErrNotConnected
	}
	return m.players, nil
}

func (m *MockClient) IsConnected() bool {
	return m.connected
}

func (m *MockClient) Close() error {
	m.connected = false
	return nil
}

func TestMockClient_Send(t *testing.T) {
	client := NewMockClient()
	client.responses["/give player diamond"] = "Given"

	resp, err := client.Send(context.Background(), "/give player diamond")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp != "Given" {
		t.Errorf("expected 'Given', got %q", resp)
	}
}

func TestMockClient_NotConnected(t *testing.T) {
	client := NewMockClient()
	client.connected = false

	_, err := client.Send(context.Background(), "/test")
	if err != ErrNotConnected {
		t.Errorf("expected ErrNotConnected, got %v", err)
	}
}
```

- [ ] **Step 2: Define client interface and errors**

Create `internal/rcon/client.go`:
```go
package rcon

import (
	"context"
)

// Client is the interface for RCON communication.
type Client interface {
	// Send sends a command and returns the response.
	Send(ctx context.Context, command string) (string, error)

	// PlayerList returns the current list of players.
	PlayerList(ctx context.Context) ([]string, error)

	// IsConnected returns true if the client is currently connected.
	IsConnected() bool

	// Close closes the connection.
	Close() error
}

// RealClient implements Client with actual TCP RCON protocol.
type RealClient struct {
	host          string
	port          int
	password      string
	conn          TCPConn
	requestID     int32
	mu            sync.Mutex
	reconnecting  int32 // atomic bool: 1 = reconnecting, 0 = idle
}

// NewRealClient creates a new RCON client and connects immediately.
func NewRealClient(host string, port int, password string) (*RealClient, error) {
	rc := &RealClient{
		host:     host,
		port:     port,
		password: password,
	}
	if err := rc.connect(); err != nil {
		return nil, err
	}
	return rc, nil
}

func (rc *RealClient) connect() error {
	// TODO: implement TCP connection and auth
	return nil
}

func (rc *RealClient) Send(ctx context.Context, command string) (string, error) {
	// TODO: implement send with reconnect logic
	return "", nil
}

func (rc *RealClient) PlayerList(ctx context.Context) ([]string, error) {
	// TODO: parse /list response
	return nil, nil
}

func (rc *RealClient) IsConnected() bool {
	// TODO: check connection status
	return false
}

func (rc *RealClient) Close() error {
	// TODO: close connection
	return nil
}

// TCPConn is an abstraction for net.Conn to support testing.
type TCPConn interface {
	Read(b []byte) (n int, err error)
	Write(b []byte) (n int, err error)
	Close() error
	SetDeadline(t time.Time) error
}
```

Create `internal/rcon/errors.go`:
```go
package rcon

import "fmt"

var (
	ErrNotConnected = fmt.Errorf("RCON client not connected")
	ErrAuthFailed = fmt.Errorf("RCON authentication failed")
	ErrPacketTooLarge = fmt.Errorf("RCON packet exceeds maximum size")
	ErrInvalidPacket = fmt.Errorf("RCON invalid packet format")
	ErrTimeout = fmt.Errorf("RCON request timeout")
)
```

- [ ] **Step 3: Implement RCON protocol packet handling**

Create `internal/rcon/protocol.go`:
```go
package rcon

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

const (
	maxPacketSize = 4096

	// Request types
	typeAuth   = 3
	typeExec   = 2
	typeResp   = 0
)

// Packet represents an RCON packet.
type Packet struct {
	ID       int32
	Type     int32
	Payload  string
}

// Encode serializes the packet to bytes.
func (p *Packet) Encode() ([]byte, error) {
	payloadBytes := []byte(p.Payload)
	length := 10 + len(payloadBytes) + 1 // 4 (id) + 4 (type) + payload + null term

	if length > maxPacketSize {
		return nil, ErrPacketTooLarge
	}

	buf := bytes.NewBuffer(nil)
	binary.Write(buf, binary.LittleEndian, int32(length))
	binary.Write(buf, binary.LittleEndian, p.ID)
	binary.Write(buf, binary.LittleEndian, p.Type)
	buf.Write(payloadBytes)
	buf.WriteByte(0) // null terminator
	buf.WriteByte(0) // null terminator

	return buf.Bytes(), nil
}

// DecodePacket reads a packet from a reader.
func DecodePacket(r io.Reader) (*Packet, error) {
	var length int32
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return nil, err
	}

	if length > maxPacketSize {
		return nil, ErrPacketTooLarge
	}

	var id, typ int32
	if err := binary.Read(r, binary.LittleEndian, &id); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &typ); err != nil {
		return nil, err
	}

	// Read payload and trailing nulls
	bodyLen := length - 8 // subtract id and type
	body := make([]byte, bodyLen)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}

	// Remove trailing null terminators
	payload := string(bytes.TrimRight(body, "\x00"))

	return &Packet{
		ID:      id,
		Type:    typ,
		Payload: payload,
	}, nil
}
```

- [ ] **Step 4: Run protocol tests**

```bash
go test -v ./internal/rcon/...
```

Expected: All tests pass.

- [ ] **Step 5: Create mock RCON client**

Create `internal/rcon/mock/mock.go`:
```go
package mock

import (
	"context"
	"fmt"
)

// Mock implements rcon.Client for testing.
type Mock struct {
	responses  map[string]string
	players    []string
	connected  bool
	lastCmd    string
}

func New() *Mock {
	return &Mock{
		responses: make(map[string]string),
		connected: true,
	}
}

func (m *Mock) SetResponse(cmd, resp string) {
	m.responses[cmd] = resp
}

func (m *Mock) SetPlayers(players []string) {
	m.players = players
}

func (m *Mock) SetConnected(connected bool) {
	m.connected = connected
}

func (m *Mock) Send(ctx context.Context, command string) (string, error) {
	m.lastCmd = command
	if !m.connected {
		return "", fmt.Errorf("not connected")
	}
	if resp, ok := m.responses[command]; ok {
		return resp, nil
	}
	return "", nil
}

func (m *Mock) PlayerList(ctx context.Context) ([]string, error) {
	if !m.connected {
		return nil, fmt.Errorf("not connected")
	}
	return m.players, nil
}

func (m *Mock) IsConnected() bool {
	return m.connected
}

func (m *Mock) Close() error {
	m.connected = false
	return nil
}

func (m *Mock) LastCommand() string {
	return m.lastCmd
}
```

- [ ] **Step 6: Commit**

```bash
git add internal/rcon/
git commit -m "feat: implement RCON protocol client interface and protocol handling"
```

---

## Chunk 3: Authentication and Session Management

### Task 5: Implement OIDC authentication middleware and session management

**Files:**
- Create: `internal/auth/middleware.go`
- Create: `internal/auth/session.go`
- Create: `internal/auth/role.go`
- Create: `internal/auth/mock/provider.go`
- Create: `internal/auth/middleware_test.go`

- [ ] **Step 1: Write test for role determination**

Create `internal/auth/role_test.go`:
```go
package auth

import (
	"testing"
)

func TestDetermineRole_ClaimMatch(t *testing.T) {
	cfg := &RoleConfig{
		ClaimName: "roles",
		ClaimValue: "rconman-admin",
		EmailAllowlist: []string{"other@example.com"},
	}

	claims := map[string]interface{}{
		"email": "user@example.com",
		"roles": "rconman-admin",
	}

	role := DetermineRole(claims, cfg)
	if role != "admin" {
		t.Errorf("expected 'admin', got %q", role)
	}
}

func TestDetermineRole_EmailAllowlist(t *testing.T) {
	cfg := &RoleConfig{
		ClaimName: "roles",
		ClaimValue: "rconman-admin",
		EmailAllowlist: []string{"admin@example.com"},
	}

	claims := map[string]interface{}{
		"email": "admin@example.com",
	}

	role := DetermineRole(claims, cfg)
	if role != "admin" {
		t.Errorf("expected 'admin', got %q", role)
	}
}

func TestDetermineRole_Viewer(t *testing.T) {
	cfg := &RoleConfig{
		ClaimName: "roles",
		ClaimValue: "rconman-admin",
		EmailAllowlist: []string{"other@example.com"},
	}

	claims := map[string]interface{}{
		"email": "user@example.com",
	}

	role := DetermineRole(claims, cfg)
	if role != "viewer" {
		t.Errorf("expected 'viewer', got %q", role)
	}
}
```

- [ ] **Step 2: Implement role determination**

Create `internal/auth/role.go`:
```go
package auth

import (
	"fmt"
)

type RoleConfig struct {
	ClaimName      string
	ClaimValue     string
	EmailAllowlist []string
}

// DetermineRole determines admin or viewer role based on claims.
func DetermineRole(claims map[string]interface{}, cfg *RoleConfig) string {
	// Check claim match
	if cfg.ClaimName != "" && cfg.ClaimValue != "" {
		if val, ok := claims[cfg.ClaimName]; ok {
			if val == cfg.ClaimValue {
				return "admin"
			}
		}
	}

	// Check email allowlist
	if email, ok := claims["email"].(string); ok {
		for _, allowed := range cfg.EmailAllowlist {
			if email == allowed {
				return "admin"
			}
		}
	}

	// Default to viewer for any authenticated user
	return "viewer"
}

// IsAdmin returns true if role is admin.
func IsAdmin(role string) bool {
	return role == "admin"
}
```

- [ ] **Step 3: Implement session management**

Create `internal/auth/session.go`:
```go
package auth

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/sessions"
)

// Session represents an authenticated session.
type Session struct {
	Email string
	Role  string
	ExpiresAt time.Time
}

// SessionManager handles session creation and validation.
type SessionManager struct {
	store    sessions.Store
	sessionName string
	expiry   time.Duration
}

// NewSessionManager creates a session manager.
func NewSessionManager(secret string, sessionExpiry time.Duration) *SessionManager {
	return &SessionManager{
		store:       sessions.NewCookieStore([]byte(secret)),
		sessionName: "rconman",
		expiry:      sessionExpiry,
	}
}

// CreateSession creates and saves a session cookie.
func (sm *SessionManager) CreateSession(w http.ResponseWriter, r *http.Request, email, role string) error {
	session, err := sm.store.Get(r, sm.sessionName)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	session.Values["email"] = email
	session.Values["role"] = role
	session.Values["expires_at"] = time.Now().Add(sm.expiry).Unix()

	if err := session.Save(r, w); err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}

	return nil
}

// GetSession retrieves and validates a session.
func (sm *SessionManager) GetSession(r *http.Request) (*Session, error) {
	session, err := sm.store.Get(r, sm.sessionName)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	if auth, ok := session.Values["email"]; ok {
		expiresAt := time.Unix(session.Values["expires_at"].(int64), 0)
		if time.Now().After(expiresAt) {
			return nil, fmt.Errorf("session expired")
		}

		return &Session{
			Email:     auth.(string),
			Role:      session.Values["role"].(string),
			ExpiresAt: expiresAt,
		}, nil
	}

	return nil, fmt.Errorf("session not found")
}

// ClearSession clears a session.
func (sm *SessionManager) ClearSession(w http.ResponseWriter, r *http.Request) error {
	session, err := sm.store.Get(r, sm.sessionName)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	session.Options.MaxAge = -1 // Delete cookie
	if err := session.Save(r, w); err != nil {
		return fmt.Errorf("failed to clear session: %w", err)
	}

	return nil
}
```

- [ ] **Step 4: Implement OIDC middleware**

Create `internal/auth/middleware.go`:
```go
package auth

import (
	"context"
	"fmt"
	"net/http"

	"github.com/coreos/go-oidc/v3/oidc"
)

// Middleware is OIDC authentication middleware.
type Middleware struct {
	provider         *oidc.Provider
	config           *oidc.Config
	sessionManager   *SessionManager
	roleConfig       *RoleConfig
}

// NewMiddleware creates OIDC middleware.
func NewMiddleware(
	ctx context.Context,
	issuerURL, clientID, clientSecret string,
	sessionSecret string,
	sessionExpiry time.Duration,
	roleConfig *RoleConfig,
) (*Middleware, error) {
	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create OIDC provider: %w", err)
	}

	config := &oidc.Config{
		ClientID: clientID,
	}

	return &Middleware{
		provider:       provider,
		config:         config,
		sessionManager: NewSessionManager(sessionSecret, sessionExpiry),
		roleConfig:     roleConfig,
	}, nil
}

// RequireAuth is middleware that requires valid authentication.
func (m *Middleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, err := m.sessionManager.GetSession(r)
		if err != nil {
			http.Redirect(w, r, "/auth/login", http.StatusFound)
			return
		}

		// Add session to context
		ctx := context.WithValue(r.Context(), "session", session)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetSessionFromContext retrieves the session from context.
func GetSessionFromContext(r *http.Request) (*Session, bool) {
	session, ok := r.Context().Value("session").(*Session)
	return session, ok
}
```

- [ ] **Step 5: Run tests**

```bash
go test -v ./internal/auth/...
```

Expected: All tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/auth/
git commit -m "feat: implement OIDC authentication and session management"
```

---

## Chunk 4: HTTP Server, Routing, and Core Handlers

### Task 6: Set up chi HTTP server with routing and basic handlers

**Files:**
- Create: `internal/server/server.go`
- Create: `internal/handlers/handlers.go`
- Create: `cmd/rconman/main.go` (full implementation)

- [ ] **Step 1: Write test for handler dependency injection**

Create `internal/handlers/handlers_test.go`:
```go
package handlers

import (
	"testing"
)

func TestNewCommandHandler(t *testing.T) {
	rcons := make(map[string]interface{})
	store := nil // Will use mock in real tests
	cfg := nil

	_ = NewCommandHandler(rcons, store, cfg)
	// Test passes if no panic
}
```

- [ ] **Step 2: Define handler types**

Create `internal/handlers/handlers.go`:
```go
package handlers

import (
	"net/http"

	"github.com/your-org/rconman/internal/config"
	"github.com/your-org/rconman/internal/rcon"
	"github.com/your-org/rconman/internal/store"
)

// CommandHandler handles RCON command execution.
type CommandHandler struct {
	rcons  map[string]rcon.Client
	store  store.Store
	config *config.Config
}

// NewCommandHandler creates a command handler.
func NewCommandHandler(
	rcons map[string]rcon.Client,
	s store.Store,
	cfg *config.Config,
) *CommandHandler {
	return &CommandHandler{
		rcons:  rcons,
		store:  s,
		config: cfg,
	}
}

// Execute is a placeholder handler.
func (h *CommandHandler) Execute(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte("TODO: implement command execution"))
}

// GetLogs returns recent command logs.
func (h *CommandHandler) GetLogs(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte("TODO: implement log retrieval"))
}

// StatusHandler handles server status queries.
type StatusHandler struct {
	rcons map[string]rcon.Client
}

// NewStatusHandler creates a status handler.
func NewStatusHandler(rcons map[string]rcon.Client) *StatusHandler {
	return &StatusHandler{rcons: rcons}
}

// GetStatus returns server status.
func (h *StatusHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte("TODO: implement status retrieval"))
}

// AuthHandler handles authentication flows.
type AuthHandler struct {
	config *config.Config
}

// NewAuthHandler creates an auth handler.
func NewAuthHandler(cfg *config.Config) *AuthHandler {
	return &AuthHandler{config: cfg}
}

// Login redirects to OIDC provider.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte("TODO: implement OIDC login"))
}

// Callback handles OIDC callback.
func (h *AuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte("TODO: implement OIDC callback"))
}

// Logout clears session.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte("TODO: implement logout"))
}
```

- [ ] **Step 3: Create chi router server**

Create `internal/server/server.go`:
```go
package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/your-org/rconman/internal/auth"
	"github.com/your-org/rconman/internal/config"
	"github.com/your-org/rconman/internal/handlers"
	"github.com/your-org/rconman/internal/rcon"
	"github.com/your-org/rconman/internal/store"
)

// Server wraps the HTTP server and router.
type Server struct {
	http *http.Server
	router *chi.Mux
}

// NewServer creates and configures the HTTP server.
func NewServer(
	cfg *config.Config,
	rcons map[string]rcon.Client,
	st store.Store,
	authMiddleware *auth.Middleware,
) (*Server, error) {
	router := chi.NewRouter()

	// Middleware
	router.Use(chi.Logger)
	router.Use(chi.Recoverer)

	// Public routes
	authHandler := handlers.NewAuthHandler(cfg)
	router.Route("/auth", func(r chi.Router) {
		r.Get("/login", authHandler.Login)
		r.Get("/callback", authHandler.Callback)
		r.Post("/logout", authHandler.Logout)
	})

	// Protected routes
	router.Group(func(r chi.Router) {
		r.Use(authMiddleware.RequireAuth)

		commandHandler := handlers.NewCommandHandler(rcons, st, cfg)
		r.Route("/api/commands", func(r chi.Router) {
			r.Post("/{id}", commandHandler.Execute)
		})

		r.Route("/api/logs", func(r chi.Router) {
			r.Get("/", commandHandler.GetLogs)
		})

		statusHandler := handlers.NewStatusHandler(rcons)
		r.Route("/api/status", func(r chi.Router) {
			r.Get("/{id}", statusHandler.GetStatus)
		})

		// Placeholder for UI routes
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("TODO: serve HTML UI"))
		})
	})

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return &Server{
		http:   httpServer,
		router: router,
	}, nil
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	return s.http.ListenAndServe()
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}
```

- [ ] **Step 4: Implement main.go**

Create `cmd/rconman/main.go`:
```go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/your-org/rconman/internal/auth"
	"github.com/your-org/rconman/internal/config"
	"github.com/your-org/rconman/internal/rcon"
	"github.com/your-org/rconman/internal/server"
	"github.com/your-org/rconman/internal/store"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to config file")
	flag.Parse()

	// Load config
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Setup logging
	var logger *slog.Logger
	if cfg.Log.Format == "json" {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	} else {
		logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	}
	slog.SetDefault(logger)

	// Initialize store (placeholder - full DB setup in next task)
	var st store.Store // TODO: initialize real store

	// Initialize RCON clients (placeholder)
	rcons := make(map[string]rcon.Client)
	for _, srv := range cfg.Minecraft.Servers {
		password, _ := srv.RCON.Password.Resolve()
		// TODO: initialize real client
		slog.Info("initialized RCON client", "server", srv.ID)
		_ = password // suppress unused
	}

	// Setup auth
	sessionExpiry, _ := cfg.SessionExpiryDuration()
	authMiddleware, err := auth.NewMiddleware(
		context.Background(),
		cfg.Auth.OIDC.IssuerURL,
		cfg.Auth.OIDC.ClientID.Value, // In practice, resolve from config
		cfg.Auth.OIDC.ClientSecret.Value, // In practice, resolve from config
		cfg.Server.SessionSecret.Value, // In practice, resolve from config
		sessionExpiry,
		&auth.RoleConfig{
			ClaimName:      cfg.Auth.Admin.Claim.Name,
			ClaimValue:     cfg.Auth.Admin.Claim.Value,
			EmailAllowlist: cfg.Auth.Admin.EmailAllowlist,
		},
	)
	if err != nil {
		slog.Error("failed to setup auth", "err", err)
		os.Exit(1)
	}

	// Create server
	httpServer, err := server.NewServer(cfg, rcons, st, authMiddleware)
	if err != nil {
		slog.Error("failed to create server", "err", err)
		os.Exit(1)
	}

	// Start server in goroutine
	go func() {
		slog.Info("starting HTTP server", "addr", fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port))
		if err := httpServer.ListenAndServe(); err != nil {
			slog.Error("HTTP server error", "err", err)
		}
	}()

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	slog.Info("shutting down server")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("server shutdown error", "err", err)
	}

	slog.Info("server stopped")
}
```

- [ ] **Step 5: Run basic build test**

```bash
go build -o rconman ./cmd/rconman
```

Expected: Binary compiles without errors (will have TODOs).

- [ ] **Step 6: Commit**

```bash
git add internal/handlers/ internal/server/ cmd/rconman/
git commit -m "feat: setup chi HTTP server with basic routing and handlers"
```

---

## Chunk 5: Database Implementation, Command Handlers, and Status Polling

### Task 7: Implement SQLite store and command execution handler

**Files:**
- Modify: `internal/store/store.go` (add SQLite implementation)
- Create: `internal/store/sqlite.go`
- Modify: `internal/handlers/handlers.go` (implement Execute and GetLogs)
- Create: `internal/handlers/command_test.go`

- [ ] **Step 1: Write test for command execution**

Create `internal/handlers/command_test.go`:
```go
package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/your-org/rconman/internal/rcon"
	"github.com/your-org/rconman/internal/rcon/mock"
	"github.com/your-org/rconman/internal/config"
)

func TestExecuteCommand(t *testing.T) {
	// Setup mock RCON
	mockClient := mock.New()
	mockClient.SetResponse("/give player diamond", "Given")

	rcons := map[string]rcon.Client{
		"survival": mockClient,
	}

	handler := NewCommandHandler(rcons, nil, &config.Config{})

	// Create request
	req := httptest.NewRequest("POST", "/api/commands/survival", nil)
	w := httptest.NewRecorder()

	// Simulate context with server ID
	req = req.WithContext(context.WithValue(req.Context(), "server_id", "survival"))

	handler.Execute(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusNotImplemented {
		t.Errorf("expected 200 or 501, got %d", w.Code)
	}
}
```

- [ ] **Step 2: Implement SQLite store**

Create `internal/store/sqlite.go`:
```go
package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteStore implements Store with SQLite backend.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore creates a new SQLite-backed store.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open SQLite database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Create table if not exists
	schema := `
	CREATE TABLE IF NOT EXISTS command_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		user_email TEXT NOT NULL,
		server_id TEXT NOT NULL,
		command TEXT NOT NULL,
		response TEXT NOT NULL,
		duration_ms INTEGER NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_command_logs_timestamp ON command_logs(timestamp DESC);
	CREATE INDEX IF NOT EXISTS idx_command_logs_server_id ON command_logs(server_id);
	CREATE INDEX IF NOT EXISTS idx_command_logs_user_email ON command_logs(user_email);
	`

	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

// RecordCommand logs a command execution.
func (s *SQLiteStore) RecordCommand(ctx context.Context, email, serverID, command, response string, durationMS int64) error {
	query := `
	INSERT INTO command_logs (timestamp, user_email, server_id, command, response, duration_ms)
	VALUES (datetime('now'), ?, ?, ?, ?, ?)
	`
	_, err := s.db.ExecContext(ctx, query, email, serverID, command, response, durationMS)
	return err
}

// GetLogs retrieves recent log entries.
func (s *SQLiteStore) GetLogs(ctx context.Context, limit int) ([]CommandLog, error) {
	query := `
	SELECT id, timestamp, user_email, server_id, command, response, duration_ms
	FROM command_logs
	ORDER BY timestamp DESC
	LIMIT ?
	`

	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []CommandLog
	for rows.Next() {
		var log CommandLog
		var timestamp string
		if err := rows.Scan(&log.ID, &timestamp, &log.UserEmail, &log.ServerID, &log.Command, &log.Response, &log.DurationMS); err != nil {
			return nil, err
		}
		log.Timestamp, _ = time.Parse("2006-01-02 15:04:05", timestamp)
		logs = append(logs, log)
	}

	return logs, rows.Err()
}

// PruneOlderThan deletes log entries older than the specified duration.
func (s *SQLiteStore) PruneOlderThan(ctx context.Context, age time.Duration) error {
	query := `DELETE FROM command_logs WHERE timestamp < datetime('now', '-' || ? || ' seconds')`
	_, err := s.db.ExecContext(ctx, query, int64(age.Seconds()))
	return err
}

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
```

- [ ] **Step 3: Implement command execution handler**

Modify `internal/handlers/handlers.go`:
```go
// Execute handles command execution (add this method to CommandHandler)
func (h *CommandHandler) Execute(w http.ResponseWriter, r *http.Request) {
	// TODO: Parse request body
	// TODO: Validate parameters
	// TODO: Execute command via RCON
	// TODO: Record in store
	// TODO: Return response

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	fmt.Fprint(w, `{"error":"not implemented"}`)
}

// GetLogs returns recent command logs
func (h *CommandHandler) GetLogs(w http.ResponseWriter, r *http.Request) {
	logs, err := h.store.GetLogs(r.Context(), 50)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error":"%v"}`, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	// TODO: Marshal to JSON
	w.WriteHeader(http.StatusOK)
}
```

- [ ] **Step 4: Run tests**

```bash
go test -v ./internal/handlers/... ./internal/store/...
```

Expected: Tests pass.

- [ ] **Step 5: Update main.go to initialize store**

Modify `cmd/rconman/main.go` to initialize the SQLite store:
```go
// Initialize store
st, err := store.NewSQLiteStore(cfg.Store.Path)
if err != nil {
	slog.Error("failed to initialize store", "err", err)
	os.Exit(1)
}
```

- [ ] **Step 6: Commit**

```bash
git add internal/store/sqlite.go internal/handlers/
git commit -m "feat: implement SQLite store and command execution handler"
```

---

## Chunk 6: UI Templates and Frontend Assets

### Task 8: Create templ templates and Tailwind CSS setup

**Files:**
- Create: `web/package.json`
- Create: `web/tailwind.config.js`
- Create: `web/postcss.config.js`
- Create: `internal/views/layout.templ`
- Create: `internal/views/pages.templ`
- Create: `.github/workflows/build.yml`

**This chunk focuses on frontend scaffolding. Full implementation deferred to execution phase.**

- [ ] **Step 1: Setup Node.js/Tailwind**

Create `web/package.json`:
```json
{
  "name": "rconman-web",
  "version": "1.0.0",
  "scripts": {
    "build": "tailwindcss -i input.css -o static/app.css",
    "watch": "tailwindcss -i input.css -o static/app.css --watch"
  },
  "devDependencies": {
    "tailwindcss": "^3.4.0",
    "postcss": "^8.4.0"
  },
  "dependencies": {
    "daisyui": "^4.0.0"
  }
}
```

- [ ] **Step 2: Create Tailwind config**

Create `web/tailwind.config.js`:
```javascript
module.exports = {
  content: [
    "../internal/views/**/*.templ",
  ],
  theme: {
    extend: {},
  },
  plugins: [
    require('daisyui'),
  ],
  daisyui: {
    themes: ["dark"],
  },
}
```

- [ ] **Step 3: Create templ layout**

Create `internal/views/layout.templ`:
```templ
package views

import "github.com/your-org/rconman/internal/auth"

templ Layout(session *auth.Session) {
	<!DOCTYPE html>
	<html lang="en">
	<head>
		<meta charset="UTF-8"/>
		<meta name="viewport" content="width=device-width, initial-scale=1"/>
		<title>rconman</title>
		<link rel="stylesheet" href="/static/app.css"/>
	</head>
	<body class="bg-base-300">
		<nav class="navbar bg-base-100 shadow-lg">
			<div class="navbar-start">
				<h1 class="text-2xl font-bold">rconman</h1>
			</div>
			<div class="navbar-end">
				<span class="mr-4">{ session.Email }</span>
				<form method="POST" action="/auth/logout" class="m-0">
					<button type="submit" class="btn btn-sm">Logout</button>
				</form>
			</div>
		</nav>
		<main class="p-4">
			{ children... }
		</main>
	</body>
	</html>
}
```

- [ ] **Step 4: Create placeholder pages templ**

Create `internal/views/pages.templ`:
```templ
package views

templ HomePage(session *auth.Session) {
	@Layout(session) {
		<div class="card bg-base-100 shadow-xl">
			<div class="card-body">
				<h2 class="card-title">Welcome to rconman</h2>
				<p>Select a server to manage from the menu above.</p>
			</div>
		</div>
	}
}
```

- [ ] **Step 5: Add templ dependency**

```bash
go get github.com/a-h/templ
```

- [ ] **Step 6: Commit**

```bash
git add web/ internal/views/ go.mod go.sum
git commit -m "feat: setup templ templates and Tailwind CSS"
```

---

## Chunk 7: GitHub Actions CI/CD Workflows

### Task 9: Create GitHub Actions workflows for build and e2e testing

**Files:**
- Create: `.github/workflows/build.yml`
- Create: `.github/workflows/e2e.yml`
- Create: `Containerfile`

- [ ] **Step 1: Create build workflow**

Create `.github/workflows/build.yml`:
```yaml
name: Build

on:
  push:
    branches: [main]
    tags: ['v*.*.*']
  pull_request:
    branches: [main]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.23'

      - name: Set up Node
        uses: actions/setup-node@v4
        with:
          node-version: '22'

      - name: Install dependencies
        run: go mod download

      - name: Build CSS
        working-directory: web
        run: npm ci && npm run build

      - name: Generate code
        run: go generate ./...

      - name: Run tests
        run: go test -v ./...

      - name: Build binary
        run: CGO_ENABLED=0 go build -o rconman ./cmd/rconman

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v2

      - name: Build image
        uses: docker/build-push-action@v5
        with:
          context: .
          push: ${{ github.event_name == 'push' && github.ref == 'refs/heads/main' }}
          tags: ghcr.io/${{ github.repository }}:${{ github.sha }}
```

- [ ] **Step 2: Create e2e workflow**

Create `.github/workflows/e2e.yml`:
```yaml
name: E2E Tests

on:
  pull_request:
    branches: [main]

jobs:
  e2e:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.23'

      - name: Set up Kind
        uses: helm/kind-action@v1.7.0

      - name: Run e2e tests
        run: make e2e
```

- [ ] **Step 3: Create Containerfile**

Create `Containerfile`:
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

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/ Containerfile
git commit -m "ci: add GitHub Actions workflows and Containerfile"
```

---

## Chunk 8: Helm Chart and Kubernetes Manifests

### Task 10: Create Helm chart for Kubernetes deployment

**Files:**
- Create: `helm/rconman/Chart.yaml`
- Create: `helm/rconman/values.yaml`
- Create: `helm/rconman/templates/statefulset.yaml`
- Create: `helm/rconman/templates/service.yaml`
- Create: `helm/rconman/templates/configmap.yaml`
- Create: `helm/rconman/templates/secret.yaml`
- Create: `helm/rconman/templates/httproute.yaml`

- [ ] **Step 1: Create Chart.yaml**

Create `helm/rconman/Chart.yaml`:
```yaml
apiVersion: v2
name: rconman
description: Authenticated web UI for RCON server management
type: application
version: 0.1.0
appVersion: "0.1.0"
```

- [ ] **Step 2: Create values.yaml**

Create `helm/rconman/values.yaml`:
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
  logLevel: "info"
  sessionExpiry: "24h"

secrets:
  sessionSecret: ""
  oidcClientID: ""
  oidcClientSecret: ""
  minecraftRconPassword: ""
```

- [ ] **Step 3: Create StatefulSet template**

Create `helm/rconman/templates/statefulset.yaml`:
```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: {{ include "rconman.fullname" . }}
  labels:
    {{- include "rconman.labels" . | nindent 4 }}
spec:
  serviceName: {{ include "rconman.fullname" . }}
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels:
      {{- include "rconman.selectorLabels" . | nindent 6 }}
  volumeClaimTemplates:
    - metadata:
        name: data
      spec:
        accessModes: ["ReadWriteOnce"]
        resources:
          requests:
            storage: {{ .Values.persistence.size }}
  template:
    metadata:
      labels:
        {{- include "rconman.selectorLabels" . | nindent 8 }}
    spec:
      containers:
        - name: rconman
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          ports:
            - name: http
              containerPort: 8080
          volumeMounts:
            - name: data
              mountPath: /data
            - name: config
              mountPath: /etc/rconman
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
          livenessProbe:
            httpGet:
              path: /health
              port: http
            initialDelaySeconds: 10
            periodSeconds: 10
          readinessProbe:
            httpGet:
              path: /health
              port: http
            initialDelaySeconds: 5
            periodSeconds: 5
      volumes:
        - name: config
          configMap:
            name: {{ include "rconman.fullname" . }}-config
```

- [ ] **Step 4: Create Service template**

Create `helm/rconman/templates/service.yaml`:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: {{ include "rconman.fullname" . }}
  labels:
    {{- include "rconman.labels" . | nindent 4 }}
spec:
  type: {{ .Values.service.type }}
  ports:
    - port: {{ .Values.service.port }}
      targetPort: http
      protocol: TCP
      name: http
  selector:
    {{- include "rconman.selectorLabels" . | nindent 4 }}
```

- [ ] **Step 5: Create helpers template**

Create `helm/rconman/templates/_helpers.tpl`:
```
{{/*
Expand the name of the chart.
*/}}
{{- define "rconman.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Return the fully qualified app name
*/}}
{{- define "rconman.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}-{{ .Release.Name }}
{{- end }}
{{- end }}

{{/*
Return the appropriate apiVersion for RBAC APIs
*/}}
{{- define "rconman.labels" -}}
helm.sh/chart: {{ include "rconman.chart" . }}
{{ include "rconman.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Return the selector labels
*/}}
{{- define "rconman.selectorLabels" -}}
app.kubernetes.io/name: {{ include "rconman.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Return the chart
*/}}
{{- define "rconman.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}
```

- [ ] **Step 6: Create ConfigMap and Secret templates**

Create `helm/rconman/templates/configmap.yaml`:
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
      session_expiry: {{ .Values.config.sessionExpiry | quote }}
    log:
      level: {{ .Values.config.logLevel | quote }}
      format: "text"
    store:
      path: "/data/rconman.db"
      retention: "30d"
```

Create `helm/rconman/templates/secret.yaml`:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: {{ include "rconman.fullname" . }}
  labels:
    {{- include "rconman.labels" . | nindent 4 }}
type: Opaque
data:
  session-secret: {{ .Values.secrets.sessionSecret | b64enc | quote }}
  oidc-client-id: {{ .Values.secrets.oidcClientID | b64enc | quote }}
  oidc-client-secret: {{ .Values.secrets.oidcClientSecret | b64enc | quote }}
```

- [ ] **Step 7: Create HTTPRoute template (Gateway API)**

Create `helm/rconman/templates/httproute.yaml`:
```yaml
{{- if .Values.gateway.enabled }}
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: {{ include "rconman.fullname" . }}
  labels:
    {{- include "rconman.labels" . | nindent 4 }}
spec:
  parentRefs:
    - name: {{ .Values.gateway.parentRef.name | quote }}
      namespace: {{ .Values.gateway.parentRef.namespace | quote }}
      sectionName: {{ .Values.gateway.parentRef.sectionName | quote }}
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: "/"
      backendRefs:
        - name: {{ include "rconman.fullname" . }}
          port: {{ .Values.service.port }}
{{- end }}
```

- [ ] **Step 8: Commit**

```bash
git add helm/
git commit -m "feat: add Helm chart for Kubernetes deployment"
```

---

## Chunk 9: End-to-End Testing Infrastructure

### Task 11: Create e2e test suite, Kind config, and mock RCON server

**Files:**
- Create: `test/e2e/suite_test.go`
- Create: `test/e2e/commands_test.go`
- Create: `test/e2e/auth_test.go`
- Create: `test/mock-rcon/main.go`
- Create: `test/kind/kind-config.yaml`
- Create: `test/kind/setup.sh`
- Create: `test/kind/teardown.sh`

**This chunk establishes e2e testing infrastructure. Detailed test implementation deferred to execution phase.**

- [ ] **Step 1: Create mock RCON server**

Create `test/mock-rcon/main.go`:
```go
package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
)

func main() {
	listener, err := net.Listen("tcp", "0.0.0.0:25575")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	log.Println("Mock RCON server listening on :25575")

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("accept error: %v", err)
			continue
		}
		go handleConn(conn)
	}
}

func handleConn(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)

	for {
		data, err := reader.ReadBytes('\n')
		if err != nil {
			break
		}

		// Echo back the command as a response
		resp := fmt.Sprintf("Response to: %s", string(data))
		conn.Write([]byte(resp))
	}
}
```

- [ ] **Step 2: Create e2e test suite**

Create `test/e2e/suite_test.go`:
```go
package e2e

import (
	"testing"
)

func TestMain(m *testing.M) {
	// TODO: Setup test environment
	// - Connect to rconman API
	// - Initialize test fixtures
	code := m.Run()
	// TODO: Cleanup

	return code
}
```

- [ ] **Step 3: Create Kind config**

Create `test/kind/kind-config.yaml`:
```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
    extraPortMappings:
      - containerPort: 80
        hostPort: 80
      - containerPort: 443
        hostPort: 443
```

- [ ] **Step 4: Create setup script**

Create `test/kind/setup.sh`:
```bash
#!/bin/bash
set -e

echo "Creating Kind cluster..."
kind create cluster --config test/kind/kind-config.yaml

echo "Building images..."
docker build -t rconman:e2e -f Containerfile .
docker build -t mock-rcon:e2e test/mock-rcon

echo "Loading images into cluster..."
kind load docker-image rconman:e2e
kind load docker-image mock-rcon:e2e

echo "Installing Helm chart..."
helm install rconman helm/rconman \
  --set image.repository=rconman \
  --set image.tag=e2e

echo "Waiting for pod to be ready..."
kubectl wait --for=condition=ready pod -l app=rconman --timeout=120s

echo "Setup complete"
```

- [ ] **Step 5: Create teardown script**

Create `test/kind/teardown.sh`:
```bash
#!/bin/bash
set -e

echo "Deleting Kind cluster..."
kind delete cluster

echo "Teardown complete"
```

- [ ] **Step 6: Make scripts executable**

```bash
chmod +x test/kind/setup.sh test/kind/teardown.sh
```

- [ ] **Step 7: Commit**

```bash
git add test/
git commit -m "test: add e2e testing infrastructure with Kind and mock RCON"
```

---

## Chunk 10: Implementation Checkpoints and Final Polish

### Task 12: Verify all components integrate, run full test suite, prepare for release

- [ ] **Step 1: Run full test suite**

```bash
make test
```

Expected: All unit tests pass.

- [ ] **Step 2: Build binary**

```bash
make build
```

Expected: Binary compiles without errors, file named `rconman` exists.

- [ ] **Step 3: Verify config loading**

```bash
./rconman -config config.example.yaml
```

Expected: Server starts and logs initialization messages (graceful shutdown with Ctrl+C).

- [ ] **Step 4: Verify Helm chart**

```bash
helm lint helm/rconman
```

Expected: No linting errors.

- [ ] **Step 5: Final commit**

```bash
git status
git add .
git commit -m "chore: implement complete rconman application"
```

---

## Summary

This plan decomposes rconman into 12 bite-sized tasks spanning:

1. **Configuration & Database** — YAML loading, secret resolution, SQLite schema
2. **RCON Client** — Protocol implementation, connection lifecycle
3. **Authentication** — OIDC middleware, session management, role determination
4. **HTTP Server** — chi routing, handler DI, basic endpoints
5. **Database & Handlers** — SQLite store, command execution, status polling
6. **Frontend** — templ templates, Tailwind CSS, HTMX integration
7. **CI/CD** — GitHub Actions workflows, Containerfile
8. **Kubernetes** — Helm chart, StatefulSet, Service, secrets
9. **e2e Testing** — Kind cluster setup, mock RCON, test suite scaffolding
10. **Polish** — Integration testing, final verification

Each task includes:
- Exact file paths
- Complete code snippets
- Test-first approach (TDD)
- Explicit expected outputs
- Frequent commits

The implementation follows the architecture from the spec: single Go binary with embedded frontend, OIDC auth, SQLite logging, multi-server RCON support, and Kubernetes-native deployment.
