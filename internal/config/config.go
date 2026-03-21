package config

import (
	"fmt"
	"os"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the complete application configuration
type Config struct {
	Server     ServerConfig        `yaml:"server"`
	Log        LogConfig           `yaml:"log"`
	Store      StoreConfig         `yaml:"store"`
	Auth       AuthConfig          `yaml:"auth"`
	Minecraft  MinecraftConfig     `yaml:"minecraft"`
	Lists      map[string][]string `yaml:"lists"`
}

// ServerConfig contains HTTP server settings
type ServerConfig struct {
	Host          string        `yaml:"host"`
	Port          int           `yaml:"port"`
	BaseURL       string        `yaml:"base_url"`
	SessionSecret *SecretValue  `yaml:"session_secret"`
	SessionExpiry string        `yaml:"session_expiry"`
	InsecureMode  bool          `yaml:"insecure_mode"` // Allow non-HTTPS cookies for development
}

// LogConfig contains logging settings
type LogConfig struct {
	Level  string `yaml:"level"`  // debug, info, warn, error
	Format string `yaml:"format"` // text, json
}

// StoreConfig contains data store settings
type StoreConfig struct {
	Path      string `yaml:"path"`
	Retention string `yaml:"retention"`
}

// AuthConfig contains authentication settings
type AuthConfig struct {
	OIDC  OIDCConfig  `yaml:"oidc"`
	Admin AdminConfig `yaml:"admin"`
}

// OIDCConfig contains OpenID Connect configuration
type OIDCConfig struct {
	IssuerURL    string        `yaml:"issuer_url"`
	ClientID     *SecretValue  `yaml:"client_id"`
	ClientSecret *SecretValue  `yaml:"client_secret"`
	Scopes       []string      `yaml:"scopes"`
}

// AdminConfig contains admin authorization settings
type AdminConfig struct {
	Claim          ClaimConfig `yaml:"claim"`
	EmailAllowlist []string    `yaml:"email_allowlist"`
}

// ClaimConfig specifies a claim name and value for admin access
type ClaimConfig struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

// MinecraftConfig contains Minecraft server configurations
type MinecraftConfig struct {
	Servers []ServerDef `yaml:"servers"`
}

// ServerDef defines a single Minecraft server configuration
type ServerDef struct {
	Name               string              `yaml:"name"`
	ID                 string              `yaml:"id"`
	RCON               RCONConfig          `yaml:"rcon"`
	StatusPollInterval string              `yaml:"status_poll_interval"`
	Commands           []CommandCategory   `yaml:"commands"`
}

// RCONConfig contains RCON connection settings
type RCONConfig struct {
	Host     string       `yaml:"host"`
	Port     int          `yaml:"port"`
	Password *SecretValue `yaml:"password"`
}

// CommandCategory represents a group of command templates
type CommandCategory struct {
	Category  string            `yaml:"category"`
	Templates []CommandTemplate `yaml:"templates"`
}

// CommandTemplate represents a template for executing a command
type CommandTemplate struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Command     string         `yaml:"command"`
	Params      []TemplateParam `yaml:"params"`
}

// TemplateParam represents a parameter in a command template
type TemplateParam struct {
	Name    string        `yaml:"name"`
	Type    string        `yaml:"type"`
	Options []string      `yaml:"options"`
	List    string        `yaml:"list"`
	Default interface{}   `yaml:"default"`
	Min     int           `yaml:"min"`
	Max     int           `yaml:"max"`
}

// LoadConfig loads and parses the YAML configuration file, then validates it
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// Validate checks the configuration for required fields and constraints
func (c *Config) Validate() error {
	// Check session secret is present
	if c.Server.SessionSecret == nil {
		return fmt.Errorf("session_secret is required")
	}

	// Resolve and validate session secret
	sessionSecret, err := c.Server.SessionSecret.Resolve()
	if err != nil {
		return fmt.Errorf("failed to resolve session_secret: %w", err)
	}

	// Check session secret minimum length (32 bytes)
	if len(sessionSecret) < 32 {
		return fmt.Errorf("session_secret must be at least 32 bytes")
	}

	// Check OIDC client secret is present
	if c.Auth.OIDC.ClientSecret == nil {
		return fmt.Errorf("auth.oidc.client_secret is required")
	}

	// Verify all minecraft server passwords are present and resolvable
	for _, server := range c.Minecraft.Servers {
		if server.RCON.Password == nil {
			return fmt.Errorf("minecraft server %q must have rcon.password configured", server.ID)
		}

		if _, err := server.RCON.Password.Resolve(); err != nil {
			return fmt.Errorf("failed to resolve password for minecraft server %q: %w", server.ID, err)
		}
	}

	// Validate global lists
	listNameRe := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	for name, entries := range c.Lists {
		if !listNameRe.MatchString(name) {
			return fmt.Errorf("list name %q is invalid: must contain only alphanumeric characters, hyphens, and underscores", name)
		}
		if len(entries) == 0 {
			return fmt.Errorf("list %q must have at least one entry", name)
		}
	}

	// Validate list-type params reference an existing list
	for _, server := range c.Minecraft.Servers {
		for _, category := range server.Commands {
			for _, tmpl := range category.Templates {
				for _, param := range tmpl.Params {
					if param.Type == "list" {
						if param.List == "" {
							return fmt.Errorf("server %q category %q template %q param %q: type=list requires a 'list' field", server.ID, category.Category, tmpl.Name, param.Name)
						}
						if _, ok := c.Lists[param.List]; !ok {
							return fmt.Errorf("server %q category %q template %q param %q: references undefined list %q", server.ID, category.Category, tmpl.Name, param.Name, param.List)
						}
					}
				}
			}
		}
	}

	return nil
}

// ResolveSessionSecret resolves and returns the session secret
func (c *Config) ResolveSessionSecret() (string, error) {
	if c.Server.SessionSecret == nil {
		return "", fmt.Errorf("session_secret not configured")
	}
	return c.Server.SessionSecret.Resolve()
}

// SessionExpiryDuration parses the session expiry string and returns a time.Duration
func (c *Config) SessionExpiryDuration() (time.Duration, error) {
	if c.Server.SessionExpiry == "" {
		return 0, fmt.Errorf("session_expiry not configured")
	}
	return time.ParseDuration(c.Server.SessionExpiry)
}
