package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupProject creates a temp dir with a go.mod and returns a Runner
// scoped to it.
func setupProject(t *testing.T) *Runner {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/demo\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return &Runner{Out: new(strings.Builder), Dir: dir}
}

func TestGenerateModule(t *testing.T) {
	r := setupProject(t)
	var msgs []string
	if err := r.generate("module", "billing", func(m string) { msgs = append(msgs, m) }); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(r.Dir, "modules", "billing")
	for _, f := range []string{"module.go", "service.go", "handler.go"} {
		data, err := os.ReadFile(filepath.Join(base, f))
		if err != nil {
			t.Fatalf("expected %s: %v", f, err)
		}
		if !strings.Contains(string(data), "package billing") {
			t.Fatalf("%s missing package clause", f)
		}
	}
	mod, _ := os.ReadFile(filepath.Join(base, "module.go"))
	if !strings.Contains(string(mod), "ModuleDef") || !strings.Contains(string(mod), "BuildControllers") {
		t.Fatal("module.go should use the NestJS-style ModuleDef")
	}
	// Type and constructor names must agree across generated files: the
	// type appears everywhere, the constructor where it is defined
	// (service.go) and called (module.go).
	svc, _ := os.ReadFile(filepath.Join(base, "service.go"))
	hdl, _ := os.ReadFile(filepath.Join(base, "handler.go"))
	for _, f := range []string{string(svc), string(hdl), string(mod)} {
		if !strings.Contains(f, "BillingService") {
			t.Fatalf("expected %q to reference BillingService:\n%s", f, f)
		}
	}
	for _, f := range []string{string(svc), string(mod)} {
		if !strings.Contains(f, "NewBillingService") {
			t.Fatalf("expected constructor NewBillingService in:\n%s", f)
		}
	}
}

func TestGenerateControllerAndService(t *testing.T) {
	r := setupProject(t)
	if err := r.generate("controller", "orders", func(string) {}); err != nil {
		t.Fatal(err)
	}
	if err := r.generate("service", "orders", func(string) {}); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"handler.go", "service.go"} {
		if _, err := os.Stat(filepath.Join(r.Dir, "modules", "orders", f)); err != nil {
			t.Fatalf("expected %s: %v", f, err)
		}
	}
}

func TestGenerateRefusesOverwrite(t *testing.T) {
	r := setupProject(t)
	if err := r.generate("module", "billing", func(string) {}); err != nil {
		t.Fatal(err)
	}
	if err := r.generate("module", "billing", func(string) {}); err == nil {
		t.Fatal("expected overwrite error")
	}
}

func TestGenerateInvalid(t *testing.T) {
	r := setupProject(t)
	if err := r.generate("module", "Bad-Name!", func(string) {}); err == nil {
		t.Fatal("expected invalid name error")
	}
	if err := r.generate("repository", "users", func(string) {}); err == nil {
		t.Fatal("expected unknown kind error")
	}
	if err := (&Runner{Out: new(strings.Builder), Dir: t.TempDir()}).generate("module", "x", func(string) {}); err == nil {
		t.Fatal("expected no-go.mod error")
	}
}

func TestRunGenerateEndToEnd(t *testing.T) {
	r := setupProject(t)
	if code := r.Run([]string{"generate", "module", "shop"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if code := r.Run([]string{"generate", "widget", "shop"}); code == 0 {
		t.Fatal("expected non-zero exit for unknown kind")
	}
}
