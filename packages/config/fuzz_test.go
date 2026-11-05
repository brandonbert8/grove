package config

import (
	"strings"
	"testing"
)

// FuzzParseDotEnv guards the .env parser against panics on arbitrary
// input (quotes, comments, huge lines, odd key/value separators).
func FuzzParseDotEnv(f *testing.F) {
	f.Add("")
	f.Add("PORT=3000\n")
	f.Add("export JWT_SECRET=\"abc # not a comment\"\n")
	f.Add("# comment\n\nA=1 # inline\n")
	f.Add("NO_EQUALS\n=value\nkey=\"unterminated\n")
	f.Fuzz(func(t *testing.T, input string) {
		m, err := parseDotEnv(strings.NewReader(input))
		if err != nil {
			return
		}
		for k := range m {
			if k == "" {
				t.Fatalf("empty key parsed from %q", input)
			}
		}
	})
}
