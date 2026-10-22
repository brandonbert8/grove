package config

import (
	"fmt"
	"strings"
)

// Get returns the merged value for key (.env + process env + overrides)
// and ok=true only when the key was explicitly set. Defaults (from
// config.Load / defaults) are NOT reported as set — use GetOr for a
// fallback. Lookup is case-insensitive and covers every known-alias form
// (e.g. "PORT" and "GROVE_PORT" are equivalent).
func Get(key string, opts ...Option) (string, bool) {
	merged, err := Merged(opts...)
	if err != nil {
		return "", false
	}
	// Case-insensitive view: .env may use lowercase, overrides are upper,
	// process env is usually upper. Normalize once.
	upperMerged := make(map[string]string, len(merged))
	for k, v := range merged {
		upperMerged[strings.ToUpper(k)] = strings.TrimSpace(v)
	}
	upper := strings.ToUpper(strings.TrimSpace(key))
	for _, n := range aliasNames(upper) {
		if v, ok := upperMerged[n]; ok && v != "" {
			return v, true
		}
	}
	return "", false
}

// aliasNames returns every env-name form for a known config key so
// GROVE_-prefixed and bare names are equivalent.
func aliasNames(upper string) []string {
	switch upper {
	case "APP_NAME", "GROVE_APP_NAME":
		return []string{"APP_NAME", "GROVE_APP_NAME"}
	case "ENV", "GROVE_ENV", "GO_ENV", "APP_ENV":
		return []string{"ENV", "GROVE_ENV", "GO_ENV", "APP_ENV"}
	case "HOST", "GROVE_HOST":
		return []string{"HOST", "GROVE_HOST"}
	case "PORT", "GROVE_PORT":
		return []string{"PORT", "GROVE_PORT"}
	case "DATABASE_URL", "GROVE_DATABASE_URL":
		return []string{"DATABASE_URL", "GROVE_DATABASE_URL"}
	case "LOG_LEVEL", "GROVE_LOG_LEVEL":
		return []string{"LOG_LEVEL", "GROVE_LOG_LEVEL"}
	case "JWT_SECRET", "GROVE_JWT_SECRET":
		return []string{"JWT_SECRET", "GROVE_JWT_SECRET"}
	default:
		return []string{upper}
	}
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
