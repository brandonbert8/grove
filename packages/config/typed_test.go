package config

import (
	"errors"
	"strings"
	"testing"
	"time"
)

type typedDB struct {
	URL  string `env:"DATABASE_URL" required:"true" secret:"true"`
	Pool int    `default:"10"`
}

type typedApp struct {
	Host    string        `env:"HOST" default:"0.0.0.0"`
	Port    int           `env:"PORT" default:"3000"`
	Env     string        `env:"ENV" default:"development" lower:"true"`
	DB      typedDB       `env:"DB"`
	Origins []string      `env:"CORS_ORIGINS" default:"http://localhost:3000,http://localhost:5173"`
	Timeout time.Duration `env:"TIMEOUT" default:"5s"`
}

func (c typedApp) Validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return errors.New("port out of range")
	}
	return nil
}

func TestLoadAsDefaultsAndNested(t *testing.T) {
	cfg, err := LoadAs[typedApp](WithEnvFile(""), WithOverrides(map[string]string{
		"DATABASE_URL": "postgres://localhost/app",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "0.0.0.0" || cfg.Port != 3000 || cfg.Env != "development" {
		t.Fatalf("cfg = %+v", cfg)
	}
	if cfg.DB.URL != "postgres://localhost/app" || cfg.DB.Pool != 10 {
		t.Fatalf("db = %+v", cfg.DB)
	}
	if len(cfg.Origins) != 2 || cfg.Origins[1] != "http://localhost:5173" {
		t.Fatalf("origins = %v", cfg.Origins)
	}
	if cfg.Timeout != 5*time.Second {
		t.Fatalf("timeout = %v", cfg.Timeout)
	}
}

func TestLoadAsOverridesAndLower(t *testing.T) {
	cfg, err := LoadAs[typedApp](WithEnvFile(""), WithOverrides(map[string]string{
		"DATABASE_URL": "postgres://prod/app",
		"DB_POOL":      "20",
		"PORT":         "8080",
		"ENV":          "PRODUCTION",
		"CORS_ORIGINS": "https://a.example,https://b.example",
		"TIMEOUT":      "1500ms",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 8080 || cfg.Env != "production" {
		t.Fatalf("cfg = %+v", cfg)
	}
	if cfg.DB.Pool != 20 {
		t.Fatalf("pool = %d", cfg.DB.Pool)
	}
	if cfg.Timeout != 1500*time.Millisecond {
		t.Fatalf("timeout = %v", cfg.Timeout)
	}
	if len(cfg.Origins) != 2 || cfg.Origins[0] != "https://a.example" {
		t.Fatalf("origins = %v", cfg.Origins)
	}
}

func TestLoadAsRequired(t *testing.T) {
	_, err := LoadAs[typedApp](WithEnvFile(""))
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("LoadAs() = %v, want required DATABASE_URL", err)
	}
}

func TestLoadAsValidate(t *testing.T) {
	_, err := LoadAs[typedApp](WithEnvFile(""), WithOverrides(map[string]string{
		"DATABASE_URL": "postgres://localhost/app",
		"PORT":         "0",
	}))
	if err == nil || !strings.Contains(err.Error(), "port out of range") {
		t.Fatalf("LoadAs() = %v, want validation error", err)
	}
}

func TestLoadAsStrict(t *testing.T) {
	_, err := LoadAs[typedApp](WithEnvFile(""), WithStrict(), WithOverrides(map[string]string{
		"DATABASE_URL": "postgres://localhost/app",
		"GROVE_TYPO":   "x",
	}))
	if err == nil || !strings.Contains(err.Error(), "GROVE_TYPO") {
		t.Fatalf("LoadAs() = %v, want strict error", err)
	}
}

func TestLoadAsConfigAliases(t *testing.T) {
	cfg, err := LoadAs[Config](WithEnvFile(""), WithOverrides(map[string]string{
		"GROVE_PORT": "9090",
		"GO_ENV":     "TEST",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 9090 || cfg.Env != "test" {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestRedacted(t *testing.T) {
	cfg, err := LoadAs[typedApp](WithEnvFile(""), WithOverrides(map[string]string{
		"DATABASE_URL": "postgres://secret",
	}))
	if err != nil {
		t.Fatal(err)
	}
	out := Redacted(cfg)
	if strings.Contains(out, "postgres://secret") || !strings.Contains(out, "DATABASE_URL=[REDACTED]") {
		t.Fatalf("redacted:\n%s", out)
	}
	if !strings.Contains(out, "PORT=3000") {
		t.Fatalf("redacted missing port:\n%s", out)
	}
}

func TestConfigRedacted(t *testing.T) {
	out := Config{JWTSecret: "super-secret", Port: 3000}.Redacted()
	if strings.Contains(out, "super-secret") || !strings.Contains(out, "JWT_SECRET=[REDACTED]") {
		t.Fatalf("redacted:\n%s", out)
	}
}

func TestLoadAsNonStruct(t *testing.T) {
	if _, err := LoadAs[string](WithEnvFile("")); err == nil {
		t.Fatal("LoadAs[string]() = nil, want struct error")
	}
}

type textValue struct {
	V string
}

func (t *textValue) UnmarshalText(b []byte) error {
	t.V = "text:" + string(b)
	return nil
}

func (t textValue) MarshalText() ([]byte, error) {
	return []byte(t.V), nil
}

type textConfig struct {
	Value textValue `env:"VALUE"`
}

func TestLoadAsTextUnmarshalerStruct(t *testing.T) {
	cfg, err := LoadAs[textConfig](WithEnvFile(""), WithOverrides(map[string]string{"VALUE": "x"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Value.V != "text:x" {
		t.Fatalf("value = %q", cfg.Value.V)
	}
	if got := Redacted(cfg); !strings.Contains(got, "VALUE=text:x") {
		t.Fatalf("redacted = %q", got)
	}
}
