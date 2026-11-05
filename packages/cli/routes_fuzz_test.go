package cli

import (
	"strings"
	"testing"
)

// FuzzJoinRoutePath keeps the route-table path joiner on its invariant:
// a non-empty path with exactly one leading slash.
func FuzzJoinRoutePath(f *testing.F) {
	f.Add("", "")
	f.Add("/users", "")
	f.Add("/users", "/{id}")
	f.Add("users/", "id")
	f.Add("/", "/health")
	f.Fuzz(func(t *testing.T, prefix, path string) {
		got := joinRoutePath(prefix, path)
		if got == "" || !strings.HasPrefix(got, "/") {
			t.Fatalf("joinRoutePath(%q, %q) = %q", prefix, path, got)
		}
		if len(got) > 1 && strings.HasSuffix(got, "/") {
			t.Fatalf("joinRoutePath(%q, %q) = %q (trailing slash)", prefix, path, got)
		}
	})
}
