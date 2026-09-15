package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config is Grove's typed application configuration.
//
// Values resolve in this order (later wins): defaults, .env file,
// environment variables.
type Config struct {
	// AppName identifies the application in logs and the CLI.
	AppName string
	// Env is the runtime environment: development, test, or production.
	Env string
	// Host is the interface to bind (empty means all interfaces).
	Host string
	// Port is the TCP port to listen on.
	Port int
	// DatabaseURL is the primary database connection string.
	DatabaseURL string
	// LogLevel controls the logger verbosity: debug, info, warn, error.
	LogLevel string
	// JWTSecret signs auth tokens. Empty is legal here (auth refuses to
	// boot without one); use WithRequired("JWT_SECRET") to fail fast.
	JWTSecret string
}

// defaults returns the baseline configuration.
func defaults() Config {
	return Config{
		AppName:     "grove-app",
		Env:         "development",
		Host:        "",
		Port:        3000,
		DatabaseURL: "",
		LogLevel:    "info",
	}
}

// Option customizes loading behavior.
type Option func(*options)

type options struct {
	envPath  string
	over     map[string]string
	required []string
	strict   bool
}

// WithEnvFile points the loader at a specific .env file.
// An empty path disables .env loading.
func WithEnvFile(path string) Option {
	return func(o *options) { o.envPath = path }
}

// WithOverrides injects key/value pairs that win over files and the
// process environment. Keys are upper-case env names ("PORT").
func WithOverrides(kv map[string]string) Option {
	return func(o *options) { o.over = kv }
}

// WithRequired fails Load when any named env var is blank or absent.
// Names are exact (e.g. "JWT_SECRET", "DATABASE_URL") and see the
// merged view: .env, process environment, and WithOverrides — in that
// precedence. Fail-fast for secrets that must never default.
func WithRequired(names ...string) Option {
	return func(o *options) { o.required = append(o.required, names...) }
}

// WithStrict fails Load on unknown GROVE_*-namespaced variables,
// catching typos like GROVE_PROT. Unnamespaced process variables
// (PATH, HOME, ...) are never inspected.
func WithStrict() Option {
	return func(o *options) { o.strict = true }
}

// knownGroveNames is the GROVE_* vocabulary Load understands.
var knownGroveNames = map[string]bool{
	"GROVE_APP_NAME": true, "GROVE_ENV": true, "GROVE_HOST": true,
	"GROVE_PORT": true, "GROVE_DATABASE_URL": true,
	"GROVE_LOG_LEVEL": true, "GROVE_JWT_SECRET": true,
}

// Load reads .env (if present) plus environment variables into a Config.
//
// It looks for ".env" in the current directory and then in each parent
// directory up to the filesystem root, so tests and nested commands work
// regardless of where the binary is invoked from.
func Load(opts ...Option) (*Config, error) {
	o := &options{envPath: findDotEnv()}
	for _, opt := range opts {
		opt(o)
	}

	merged := map[string]string{}
	if o.envPath != "" {
		fileVars, err := parseDotEnvFile(o.envPath)
		if err != nil {
			return nil, err
		}
		for k, v := range fileVars {
			merged[k] = v
		}
	}
	for _, kv := range os.Environ() {
		name, value, _ := strings.Cut(kv, "=")
		merged[name] = value
	}
	for k, v := range o.over {
		merged[strings.ToUpper(k)] = v
	}

	cfg := defaults()
	if v, ok := lookup(merged, "APP_NAME", "GROVE_APP_NAME"); ok {
		cfg.AppName = v
	}
	if v, ok := lookup(merged, "ENV", "GROVE_ENV", "GO_ENV", "APP_ENV"); ok {
		cfg.Env = strings.ToLower(v)
	}
	if v, ok := lookup(merged, "HOST", "GROVE_HOST"); ok {
		cfg.Host = v
	}
	if v, ok := lookup(merged, "PORT", "GROVE_PORT"); ok {
		port, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("config: invalid PORT %q: must be 1-65535", v)
		}
		cfg.Port = port
	}
	if v, ok := lookup(merged, "DATABASE_URL", "GROVE_DATABASE_URL"); ok {
		cfg.DatabaseURL = v
	}
	if v, ok := lookup(merged, "LOG_LEVEL", "GROVE_LOG_LEVEL"); ok {
		cfg.LogLevel = strings.ToLower(v)
	}
	if v, ok := lookup(merged, "JWT_SECRET", "GROVE_JWT_SECRET"); ok {
		cfg.JWTSecret = v
	}

	for _, name := range o.required {
		if strings.TrimSpace(merged[name]) == "" {
			return nil, fmt.Errorf("config: required variable %q is not set", name)
		}
	}
	if o.strict {
		for k := range merged {
			if strings.HasPrefix(k, "GROVE_") && !knownGroveNames[k] {
				return nil, fmt.Errorf("config: unknown variable %q (strict mode)", k)
			}
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// MustLoad is like Load but panics on error. Useful in main functions.
func MustLoad(opts ...Option) *Config {
	cfg, err := Load(opts...)
	if err != nil {
		panic(err)
	}
	return cfg
}

// Validate checks the configuration for inconsistent values.
func (c Config) Validate() error {
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("config: invalid LOG_LEVEL %q: want debug|info|warn|error", c.LogLevel)
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("config: invalid Port %d: must be 1-65535", c.Port)
	}
	return nil
}

// SchemaVar describes one known configuration variable: the declarative
// config-schema entry (the ConfigModule validation equivalent) used by
// Schema to document and audit the environment.
type SchemaVar struct {
	// Name is the primary env var name.
	Name string
	// Aliases are accepted alternative names.
	Aliases []string
	// Description documents the variable.
	Description string
	// Default is the value used when nothing sets it.
	Default string
}

// Schema returns the known configuration variables in stable order, so
// apps and docs can audit the environment:
//
//	for _, v := range config.Schema() {
//	    fmt.Println(v.Name, "-", v.Description)
//	}
func Schema() []SchemaVar {
	return []SchemaVar{
		{Name: "APP_NAME", Aliases: []string{"GROVE_APP_NAME"}, Description: "Application name in logs and the CLI", Default: "grove-app"},
		{Name: "ENV", Aliases: []string{"GROVE_ENV", "GO_ENV", "APP_ENV"}, Description: "Runtime environment: development, test, or production", Default: "development"},
		{Name: "HOST", Aliases: []string{"GROVE_HOST"}, Description: "Interface to bind (empty means all interfaces)", Default: ""},
		{Name: "PORT", Aliases: []string{"GROVE_PORT"}, Description: "TCP port to listen on (1-65535)", Default: "3000"},
		{Name: "DATABASE_URL", Aliases: []string{"GROVE_DATABASE_URL"}, Description: "Primary database connection string", Default: ""},
		{Name: "LOG_LEVEL", Aliases: []string{"GROVE_LOG_LEVEL"}, Description: "Logger verbosity: debug, info, warn, error", Default: "info"},
		{Name: "JWT_SECRET", Aliases: []string{"GROVE_JWT_SECRET"}, Description: "Secret signing auth tokens (fail fast with WithRequired)", Default: ""},
	}
}

// Addr returns the host:port pair suitable for net/http.
func (c Config) Addr() string {
	if c.Host == "" {
		return ":" + strconv.Itoa(c.Port)
	}
	return c.Host + ":" + strconv.Itoa(c.Port)
}

// IsProduction reports Env == "production".
func (c Config) IsProduction() bool { return c.Env == "production" }

// IsProd is shorthand for IsProduction.
func (c Config) IsProd() bool { return c.IsProduction() }

// IsDevelopment reports Env == "development" (the default).
func (c Config) IsDevelopment() bool { return c.Env == "development" }

// IsTest reports Env == "test".
func (c Config) IsTest() bool { return c.Env == "test" }

// lookup returns the first non-empty value for any of the names.
func lookup(vars map[string]string, names ...string) (string, bool) {
	for _, n := range names {
		if v, ok := vars[n]; ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v), true
		}
	}
	return "", false
}

// findDotEnv searches the current directory and its parents for ".env".
func findDotEnv() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(dir, ".env")
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// parseDotEnvFile parses a simple .env file: KEY=VALUE lines with support
// for # comments, blank lines, `export` prefixes, and single/double quotes.
func parseDotEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("config: open %s: %w", path, err)
	}
	defer f.Close()

	out := map[string]string{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		name, value, ok := strings.Cut(line, "=")
		if !ok {
			continue // tolerate flag-like lines
		}
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		// Strip trailing inline comments for unquoted values.
		if !strings.Contains(line, "\"") && !strings.Contains(line, "'") {
			if idx := strings.Index(value, " #"); idx >= 0 {
				value = strings.TrimSpace(value[:idx])
			}
		}
		if name != "" {
			out[name] = value
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	return out, nil
}
