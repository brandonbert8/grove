package config

import (
	"testing"
)

func TestJWTSecretLoading(t *testing.T) {
	cfg, err := Load(WithEnvFile(""), WithOverrides(map[string]string{
		"JWT_SECRET": "s3cr3t",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.JWTSecret != "s3cr3t" {
		t.Fatalf("jwt secret = %q", cfg.JWTSecret)
	}
	cfg, err = Load(WithEnvFile(""), WithOverrides(map[string]string{
		"GROVE_JWT_SECRET": "prefixed",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.JWTSecret != "prefixed" {
		t.Fatalf("jwt secret = %q", cfg.JWTSecret)
	}
}

func TestRequiredFailsFast(t *testing.T) {
	// Hermetic: neutralize ambient secrets.
	t.Setenv("JWT_SECRET", "")
	t.Setenv("GROVE_JWT_SECRET", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("GROVE_DATABASE_URL", "")
	if _, err := Load(WithEnvFile(""), WithRequired("JWT_SECRET")); err == nil {
		t.Fatal("expected error for missing JWT_SECRET")
	}
	if _, err := Load(WithEnvFile(""),
		WithOverrides(map[string]string{"JWT_SECRET": "x"}),
		WithRequired("JWT_SECRET", "DATABASE_URL")); err == nil {
		t.Fatal("expected error for missing DATABASE_URL")
	}
	cfg, err := Load(WithEnvFile(""),
		WithOverrides(map[string]string{"JWT_SECRET": "x", "DATABASE_URL": "postgres://db"}),
		WithRequired("JWT_SECRET", "DATABASE_URL"))
	if err != nil {
		t.Fatalf("all required present must pass: %v", err)
	}
	if cfg.JWTSecret != "x" {
		t.Fatalf("jwt secret = %q", cfg.JWTSecret)
	}
}

func TestStrictUnknownGroveNames(t *testing.T) {
	if _, err := Load(WithEnvFile(""),
		WithOverrides(map[string]string{"GROVE_PROT": "3000"}),
		WithStrict()); err == nil {
		t.Fatal("expected error for unknown GROVE_PROT in strict mode")
	}
	if _, err := Load(WithEnvFile(""),
		WithOverrides(map[string]string{"GROVE_PORT": "3000"}),
		WithStrict()); err != nil {
		t.Fatalf("known names must pass strict mode: %v", err)
	}
	// Non-strict ignores the typo (backward compatible).
	if _, err := Load(WithEnvFile(""),
		WithOverrides(map[string]string{"GROVE_PROT": "3000"})); err != nil {
		t.Fatalf("non-strict must ignore typos: %v", err)
	}
}

func TestIsProdAlias(t *testing.T) {
	cfg, err := Load(WithEnvFile(""), WithOverrides(map[string]string{"ENV": "production"}))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.IsProduction() || !cfg.IsProd() {
		t.Fatal("production env must report IsProduction/IsProd")
	}
}
