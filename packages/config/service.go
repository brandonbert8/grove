package config

import (
	"fmt"
	"strconv"
	"strings"
)

// Get returns the merged value for key (exact env name, e.g. "PORT").
// Known keys map onto the typed Config; custom keys read the same merged
// view (.env + process env + overrides), so custom .env vars are visible.
func Get(key string, opts ...Option) (string, bool) {
	merged, err := Merged(opts...)
	if err != nil {
		return "", false
	}
	upper := strings.ToUpper(strings.TrimSpace(key))
	if v, ok := merged[upper]; ok && strings.TrimSpace(v) != "" {
		// Normalize through the typed config for known keys so PORT
		// validation and ENV lower-casing apply uniformly.
		cfg, err := Load(opts...)
		if err != nil {
			return strings.TrimSpace(v), true
		}
		switch upper {
		case "APP_NAME", "GROVE_APP_NAME":
			return cfg.AppName, true
		case "ENV", "GROVE_ENV", "GO_ENV", "APP_ENV":
			return cfg.Env, true
		case "HOST", "GROVE_HOST":
			return cfg.Host, cfg.Host != ""
		case "PORT", "GROVE_PORT":
			return strconv.Itoa(cfg.Port), true
		case "DATABASE_URL", "GROVE_DATABASE_URL":
			return cfg.DatabaseURL, cfg.DatabaseURL != ""
		case "LOG_LEVEL", "GROVE_LOG_LEVEL":
			return cfg.LogLevel, true
		case "JWT_SECRET", "GROVE_JWT_SECRET":
			return cfg.JWTSecret, cfg.JWTSecret != ""
		}
		return strings.TrimSpace(v), true
	}
	// Case-insensitive alias lookup for known keys set under aliases.
	cfg, err := Load(opts...)
	if err != nil {
		return "", false
	}
	switch upper {
	case "APP_NAME":
		return cfg.AppName, true
	case "ENV":
		return cfg.Env, true
	case "HOST":
		return cfg.Host, cfg.Host != ""
	case "PORT":
		return strconv.Itoa(cfg.Port), true
	case "DATABASE_URL":
		return cfg.DatabaseURL, cfg.DatabaseURL != ""
	case "LOG_LEVEL":
		return cfg.LogLevel, true
	case "JWT_SECRET":
		return cfg.JWTSecret, cfg.JWTSecret != ""
	}
	return "", false
}

// MustGet returns Get or panics (fail-fast for required secrets).
func MustGet(key string, opts ...Option) string {
	if v, ok := Get(key, opts...); ok && strings.TrimSpace(v) != "" {
		return v
	}
	panic(fmt.Sprintf("config: required variable %q is not set", key))
}

// GetOr returns Get or fallback when absent/blank.
func GetOr(key, fallback string, opts ...Option) string {
	if v, ok := Get(key, opts...); ok && strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}
