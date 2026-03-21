package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfig_Valid(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()

	// Create a sample config file
	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := `
server:
  host: "localhost"
  port: 8080
  base_url: "http://localhost:8080"
  session_secret:
    value: "this_is_a_very_long_secret_that_is_at_least_32_bytes_long_12345"
  session_expiry: "24h"

log:
  level: "info"
  format: "text"

store:
  path: "/tmp/rconman.db"
  retention: "720h"

auth:
  oidc:
    issuer_url: "https://accounts.google.com"
    client_id:
      value: "test_client_id"
    client_secret:
      value: "test_client_secret_that_is_long_enough"
    scopes:
      - "openid"
      - "email"
  admin:
    claim:
      name: "email"
      value: "admin@example.com"
    email_allowlist:
      - "admin@example.com"

minecraft:
  servers:
    - name: "Main Server"
      id: "main"
      rcon:
        host: "localhost"
        port: 25575
        password:
          value: "rcon_password_123"
      status_poll_interval: "30s"
      commands:
        - category: "Server"
          templates:
            - name: "Save"
              description: "Save the world"
              command: "save-all"
              params: []
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to create test config file: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("expected no error loading config, got %v", err)
	}

	// Verify basic structure was loaded
	if cfg.Server.Host != "localhost" {
		t.Errorf("expected host 'localhost', got %q", cfg.Server.Host)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}

	if cfg.Log.Level != "info" {
		t.Errorf("expected log level 'info', got %q", cfg.Log.Level)
	}

	if len(cfg.Minecraft.Servers) != 1 {
		t.Errorf("expected 1 minecraft server, got %d", len(cfg.Minecraft.Servers))
	}
}

func TestValidateConfig_MissingSessionSecret(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Host: "localhost",
			Port: 8080,
		},
		Auth: AuthConfig{
			OIDC: OIDCConfig{
				ClientSecret: &SecretValue{Value: "secret"},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatalf("expected validation error for missing session secret")
	}

	if err.Error() != "session_secret is required" {
		t.Errorf("expected 'session_secret is required', got %q", err.Error())
	}
}

func TestValidateConfig_ShortSessionSecret(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Host: "localhost",
			Port: 8080,
			SessionSecret: &SecretValue{
				Value: "tooshort", // Only 8 bytes
			},
		},
		Auth: AuthConfig{
			OIDC: OIDCConfig{
				ClientSecret: &SecretValue{Value: "secret"},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatalf("expected validation error for short session secret")
	}

	if err.Error() != "session_secret must be at least 32 bytes" {
		t.Errorf("expected 'session_secret must be at least 32 bytes', got %q", err.Error())
	}
}

func TestValidateConfig_MissingOIDCClientSecret(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Host: "localhost",
			Port: 8080,
			SessionSecret: &SecretValue{
				Value: "this_is_a_very_long_secret_that_is_at_least_32_bytes_long",
			},
		},
		Auth: AuthConfig{
			OIDC: OIDCConfig{
				ClientSecret: nil,
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatalf("expected validation error for missing OIDC client secret")
	}

	if err.Error() != "auth.oidc.client_secret is required" {
		t.Errorf("expected 'auth.oidc.client_secret is required', got %q", err.Error())
	}
}

func TestValidateConfig_ValidSessionSecret(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Host: "localhost",
			Port: 8080,
			SessionSecret: &SecretValue{
				Value: "this_is_a_very_long_secret_that_is_at_least_32_bytes_long",
			},
		},
		Auth: AuthConfig{
			OIDC: OIDCConfig{
				ClientSecret: &SecretValue{Value: "secret"},
			},
		},
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("expected no validation error, got %v", err)
	}
}

func TestResolveSessionSecret(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			SessionSecret: &SecretValue{
				Value: "test_session_secret_123",
			},
		},
	}

	secret, err := cfg.ResolveSessionSecret()
	if err != nil {
		t.Fatalf("expected no error resolving session secret, got %v", err)
	}

	if secret != "test_session_secret_123" {
		t.Errorf("expected 'test_session_secret_123', got %q", secret)
	}
}

func TestSessionExpiryDuration(t *testing.T) {
	tests := []struct {
		name     string
		expiry   string
		expected time.Duration
		wantErr  bool
	}{
		{
			name:     "valid 24h",
			expiry:   "24h",
			expected: 24 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "valid 30m",
			expiry:   "30m",
			expected: 30 * time.Minute,
			wantErr:  false,
		},
		{
			name:     "invalid format",
			expiry:   "invalid",
			expected: 0,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Server: ServerConfig{
					SessionExpiry: tt.expiry,
				},
			}

			duration, err := cfg.SessionExpiryDuration()
			if (err != nil) != tt.wantErr {
				t.Errorf("expected error=%v, got %v", tt.wantErr, err)
			}

			if !tt.wantErr && duration != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, duration)
			}
		})
	}
}

func TestValidateConfig_ListNameInvalidChars(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			SessionSecret: &SecretValue{Value: "this_is_a_very_long_secret_that_is_at_least_32_bytes_long"},
		},
		Auth: AuthConfig{
			OIDC: OIDCConfig{ClientSecret: &SecretValue{Value: "secret"}},
		},
		Lists: map[string][]string{
			"my list": {"a", "b"}, // space is invalid
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for list name with space")
	}
}

func TestValidateConfig_ListEmpty(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			SessionSecret: &SecretValue{Value: "this_is_a_very_long_secret_that_is_at_least_32_bytes_long"},
		},
		Auth: AuthConfig{
			OIDC: OIDCConfig{ClientSecret: &SecretValue{Value: "secret"}},
		},
		Lists: map[string][]string{
			"pokemon": {}, // empty list
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty list")
	}
}

func TestValidateConfig_ListParamReferenceMissing(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			SessionSecret: &SecretValue{Value: "this_is_a_very_long_secret_that_is_at_least_32_bytes_long"},
		},
		Auth: AuthConfig{
			OIDC: OIDCConfig{ClientSecret: &SecretValue{Value: "secret"}},
		},
		Lists: map[string][]string{},
		Minecraft: MinecraftConfig{
			Servers: []ServerDef{
				{
					ID: "test",
					RCON: RCONConfig{
						Host:     "localhost",
						Port:     25575,
						Password: &SecretValue{Value: "pass"},
					},
					Commands: []CommandCategory{
						{
							Category: "Test",
							Templates: []CommandTemplate{
								{
									Name:    "cmd",
									Command: "/cmd {x}",
									Params: []TemplateParam{
										{Name: "x", Type: "list", List: "missing-list"},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for list param referencing missing list")
	}
}

func TestValidateConfig_ListParamValid(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			SessionSecret: &SecretValue{Value: "this_is_a_very_long_secret_that_is_at_least_32_bytes_long"},
		},
		Auth: AuthConfig{
			OIDC: OIDCConfig{ClientSecret: &SecretValue{Value: "secret"}},
		},
		Lists: map[string][]string{
			"pokemon": {"bulbasaur", "charmander", "squirtle"},
		},
		Minecraft: MinecraftConfig{
			Servers: []ServerDef{
				{
					ID: "test",
					RCON: RCONConfig{
						Host:     "localhost",
						Port:     25575,
						Password: &SecretValue{Value: "pass"},
					},
					Commands: []CommandCategory{
						{
							Category: "Test",
							Templates: []CommandTemplate{
								{
									Name:    "cmd",
									Command: "/cmd {x}",
									Params: []TemplateParam{
										{Name: "x", Type: "list", List: "pokemon"},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	err := cfg.Validate()
	if err != nil {
		t.Fatalf("expected no error for valid list param, got %v", err)
	}
}

func TestValidateConfig_MinecraftServerMissingPassword(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Host: "localhost",
			Port: 8080,
			SessionSecret: &SecretValue{
				Value: "this_is_a_very_long_secret_that_is_at_least_32_bytes_long",
			},
		},
		Auth: AuthConfig{
			OIDC: OIDCConfig{
				ClientSecret: &SecretValue{Value: "secret"},
			},
		},
		Minecraft: MinecraftConfig{
			Servers: []ServerDef{
				{
					Name: "Test Server",
					ID:   "test",
					RCON: RCONConfig{
						Host: "localhost",
						Port: 25575,
						// Password missing
					},
				},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatalf("expected validation error for missing minecraft server password")
	}
}
