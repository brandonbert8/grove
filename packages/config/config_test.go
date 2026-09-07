package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	// Neutralize process environment so the test is hermetic.
	for _, k := range []string{"PORT", "GROVE_PORT", "LOG_LEVEL", "GROVE_LOG_LEVEL", "DATABASE_URL", "GROVE_DATABASE_URL"} {
		t.Setenv(k, "")
	}
	cfg, err := Load(WithEnvFile(""))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 3000 {
		t.Fatalf("default port = %d, want 3000", cfg.Port)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("default log level = %q, want info", cfg.LogLevel)
	}
}

func TestEnvOverrides(t *testing.T) {
	cfg, err := Load(WithEnvFile(""), WithOverrides(map[string]string{
		"PORT":         "8080",
		"DATABASE_URL": "postgres://localhost/test",
		"LOG_LEVEL":    "debug",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 8080 {
		t.Fatalf("port = %d, want 8080", cfg.Port)
	}
	if cfg.DatabaseURL != "postgres://localhost/test" {
		t.Fatalf("database url = %q", cfg.DatabaseURL)
	}
	if cfg.Addr() != ":8080" {
		t.Fatalf("addr = %q, want :8080", cfg.Addr())
	}
}

func TestDotEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "# comment\nPORT=4001\nDATABASE_URL=postgres://file/db\nQUOTED=\"hello world\"\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(WithEnvFile(path))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 4001 {
		t.Fatalf("port = %d, want 4001", cfg.Port)
	}
	if cfg.DatabaseURL != "postgres://file/db" {
		t.Fatalf("database url = %q", cfg.DatabaseURL)
	}
}

func TestInvalidPortFails(t *testing.T) {
	if _, err := Load(WithEnvFile(""), WithOverrides(map[string]string{"PORT": "abc"})); err == nil {
		t.Fatal("expected error for non-numeric port")
	}
	if _, err := Load(WithEnvFile(""), WithOverrides(map[string]string{"PORT": "99999"})); err == nil {
		t.Fatal("expected error for out-of-range port")
	}
}

func TestInvalidLogLevelFails(t *testing.T) {
	if _, err := Load(WithEnvFile(""), WithOverrides(map[string]string{"LOG_LEVEL": "verbose"})); err == nil {
		t.Fatal("expected error for invalid log level")
	}
}
