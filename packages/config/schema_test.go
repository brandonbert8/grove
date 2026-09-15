package config

import "testing"

func TestSchemaCoversKnownVars(t *testing.T) {
	schema := Schema()
	if len(schema) == 0 {
		t.Fatal("schema must not be empty")
	}
	seen := map[string]bool{}
	for _, v := range schema {
		if v.Name == "" || v.Description == "" {
			t.Fatalf("schema entry incomplete: %+v", v)
		}
		if seen[v.Name] {
			t.Fatalf("duplicate schema entry %q", v.Name)
		}
		seen[v.Name] = true
	}
	for _, want := range []string{"PORT", "JWT_SECRET", "LOG_LEVEL", "DATABASE_URL"} {
		if !seen[want] {
			t.Fatalf("schema missing %q", want)
		}
	}
}
