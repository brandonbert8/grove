package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWatchedFile(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"main.go", true},
		{"main_test.go", false},
		{"go.mod", true},
		{"go.sum", true},
		{"go.work", true},
		{"go.work.sum", true},
		{".env", true},
		{".env.local", true},
		{"config.yaml", true},
		{"config.yml", true},
		{"config.json", true},
		{"config.toml", true},
		{"app.ini", true},
		{"nginx.conf", true},
		{"README.md", false},
		{"notes.txt", false},
	}
	for _, tt := range tests {
		if got := watchedFile(tt.name); got != tt.want {
			t.Errorf("watchedFile(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestFingerprintIgnoresTestFiles(t *testing.T) {
	dir := t.TempDir()
	app := filepath.Join(dir, "main.go")
	testFile := filepath.Join(dir, "main_test.go")
	if err := os.WriteFile(app, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(testFile, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	before, err := fingerprintFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(testFile, []byte("package main\n\nfunc TestX(t *testing.T) {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	afterTest, err := fingerprintFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if before != afterTest {
		t.Fatal("test file change should not alter the fingerprint")
	}
	if err := os.WriteFile(app, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	afterApp, err := fingerprintFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if afterTest == afterApp {
		t.Fatal("source file change should alter the fingerprint")
	}
}

func TestFingerprintSkipsIgnoredDirs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := fingerprintFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, ignored := range []string{"node_modules", "dist", "build", "vendor"} {
		sub := filepath.Join(dir, ignored)
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sub, "generated.go"), []byte("package generated\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	after, err := fingerprintFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("ignored directories should not alter the fingerprint")
	}
}
