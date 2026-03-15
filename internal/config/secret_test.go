package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSecret_Inline(t *testing.T) {
	secret := &SecretValue{
		Value: "my_secret_value",
	}

	resolved, err := secret.Resolve()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if resolved != "my_secret_value" {
		t.Errorf("expected 'my_secret_value', got %q", resolved)
	}
}

func TestResolveSecret_EnvVar(t *testing.T) {
	envVarName := "TEST_SECRET"
	expectedValue := "secret_from_env"

	// Set environment variable
	t.Setenv(envVarName, expectedValue)

	secret := &SecretValue{
		ValueFrom: &ValueFrom{
			Env: envVarName,
		},
	}

	resolved, err := secret.Resolve()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if resolved != expectedValue {
		t.Errorf("expected %q, got %q", expectedValue, resolved)
	}
}

func TestResolveSecret_EnvVar_Missing(t *testing.T) {
	envVarName := "NONEXISTENT_SECRET_VAR_12345"

	// Make sure the env var doesn't exist
	os.Unsetenv(envVarName)

	secret := &SecretValue{
		ValueFrom: &ValueFrom{
			Env: envVarName,
		},
	}

	_, err := secret.Resolve()
	if err == nil {
		t.Fatalf("expected error for missing env var, got nil")
	}
}

func TestResolveSecret_File(t *testing.T) {
	// Create a temporary file with secret content
	tmpFile := t.TempDir()
	secretFile := filepath.Join(tmpFile, "secret.txt")
	secretContent := "secret_from_file"

	if err := os.WriteFile(secretFile, []byte(secretContent), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	secret := &SecretValue{
		ValueFrom: &ValueFrom{
			File: secretFile,
		},
	}

	resolved, err := secret.Resolve()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if resolved != secretContent {
		t.Errorf("expected %q, got %q", secretContent, resolved)
	}
}

func TestResolveSecret_File_WithWhitespace(t *testing.T) {
	// Create a temporary file with secret content and surrounding whitespace
	tmpFile := t.TempDir()
	secretFile := filepath.Join(tmpFile, "secret.txt")
	secretContent := "secret_from_file"
	fileContent := "  \n" + secretContent + "  \n"

	if err := os.WriteFile(secretFile, []byte(fileContent), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	secret := &SecretValue{
		ValueFrom: &ValueFrom{
			File: secretFile,
		},
	}

	resolved, err := secret.Resolve()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if resolved != secretContent {
		t.Errorf("expected %q, got %q", secretContent, resolved)
	}
}

func TestResolveSecret_File_Missing(t *testing.T) {
	secret := &SecretValue{
		ValueFrom: &ValueFrom{
			File: "/nonexistent/path/to/secret.txt",
		},
	}

	_, err := secret.Resolve()
	if err == nil {
		t.Fatalf("expected error for missing file, got nil")
	}
}

func TestResolveSecret_Missing(t *testing.T) {
	secret := &SecretValue{}

	_, err := secret.Resolve()
	if err == nil {
		t.Fatalf("expected error when no value or valueFrom provided")
	}
}

func TestResolveSecret_ValuePreferredOverValueFrom(t *testing.T) {
	// When both Value and ValueFrom are present, Value should take precedence
	secret := &SecretValue{
		Value: "inline_value",
		ValueFrom: &ValueFrom{
			Env: "SOME_ENV_VAR",
		},
	}

	resolved, err := secret.Resolve()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if resolved != "inline_value" {
		t.Errorf("expected 'inline_value' to take precedence, got %q", resolved)
	}
}
