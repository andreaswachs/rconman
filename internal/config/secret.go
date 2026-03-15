package config

import (
	"fmt"
	"os"
	"strings"
)

// SecretValue represents a secret that can be provided inline or resolved from external sources
type SecretValue struct {
	Value     string      `yaml:"value"`
	ValueFrom *ValueFrom  `yaml:"valueFrom"`
}

// ValueFrom specifies external sources for resolving a secret
type ValueFrom struct {
	Env  string `yaml:"env"`
	File string `yaml:"file"`
}

// Resolve returns the secret value, resolving from the appropriate source.
// Resolution order:
// 1. Inline Value (if non-empty)
// 2. Environment variable (if ValueFrom.Env is set)
// 3. File contents (if ValueFrom.File is set)
// Returns an error if no value is available from any source
func (s *SecretValue) Resolve() (string, error) {
	// Check inline value first
	if s.Value != "" {
		return s.Value, nil
	}

	// Check ValueFrom for external sources
	if s.ValueFrom != nil {
		// Check environment variable
		if s.ValueFrom.Env != "" {
			value, exists := os.LookupEnv(s.ValueFrom.Env)
			if !exists {
				return "", fmt.Errorf("environment variable %q not found", s.ValueFrom.Env)
			}
			if value == "" {
				return "", fmt.Errorf("environment variable %q is empty", s.ValueFrom.Env)
			}
			return value, nil
		}

		// Check file
		if s.ValueFrom.File != "" {
			data, err := os.ReadFile(s.ValueFrom.File)
			if err != nil {
				return "", fmt.Errorf("failed to read secret file %q: %w", s.ValueFrom.File, err)
			}
			value := strings.TrimSpace(string(data))
			if value == "" {
				return "", fmt.Errorf("secret file %q is empty", s.ValueFrom.File)
			}
			return value, nil
		}
	}

	return "", fmt.Errorf("secret value not provided: must specify either 'value' or 'valueFrom'")
}
