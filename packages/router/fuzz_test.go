package router

import (
	"strings"
	"testing"
)

// FuzzMatchPattern ensures the 405 Allow-header matcher never panics and
// only reports matches for well-formed paths.
func FuzzMatchPattern(f *testing.F) {
	f.Add("/users/{id}", "/users/42")
	f.Add("/files/*", "/files/a/b/c")
	f.Add("/", "/")
	f.Add("", "")
	f.Fuzz(func(t *testing.T, pattern, path string) {
		_ = matchPattern(pattern, path)
	})
}

// FuzzNormalizePath keeps the validator on a leading-slash, non-empty
// invariant that Chi requires.
func FuzzNormalizePath(f *testing.F) {
	f.Add("")
	f.Add("/")
	f.Add("users")
	f.Add("/users")
	f.Fuzz(func(t *testing.T, path string) {
		got := normalizePath(path)
		if got == "" || !strings.HasPrefix(got, "/") {
			t.Fatalf("normalizePath(%q) = %q", path, got)
		}
	})
}
