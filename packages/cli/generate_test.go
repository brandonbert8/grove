package cli

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brandonbert8/grove/packages/cli/ui"
)

// setupProject creates a temp dir with a go.mod, returning its path.
func setupProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/demo\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestGenerateModule(t *testing.T) {
	dir := setupProject(t)
	res, err := Generate(dir, "module", "billing")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Changes) != 3 {
		t.Fatalf("changes = %v, want 3 CREATE", res.Changes)
	}
	for _, c := range res.Changes {
		if c.Action != ui.Create {
			t.Fatalf("change = %+v, want CREATE", c)
		}
	}
	base := filepath.Join(dir, "modules", "billing")
	for _, f := range []string{"module.go", "service.go", "controller.go"} {
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
	// Type and constructor names must agree across generated files.
	svc, _ := os.ReadFile(filepath.Join(base, "service.go"))
	hdl, _ := os.ReadFile(filepath.Join(base, "controller.go"))
	for _, f := range []string{string(svc), string(hdl), string(mod)} {
		if !strings.Contains(f, "BillingService") {
			t.Fatalf("expected reference to BillingService:\n%s", f)
		}
	}
	for _, f := range []string{string(svc), string(mod)} {
		if !strings.Contains(f, "NewBillingService") {
			t.Fatalf("expected constructor NewBillingService in:\n%s", f)
		}
	}
}

func TestGenerateControllerAndService(t *testing.T) {
	dir := setupProject(t)
	if _, err := Generate(dir, "module", "orders"); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(dir, "modules", "orders")
	for _, f := range []string{"controller.go", "service.go"} {
		if err := os.Remove(filepath.Join(base, f)); err != nil {
			t.Fatal(err)
		}
	}
	res, err := Generate(dir, "controller", "orders")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Notices) != 0 {
		t.Fatalf("controller must be picked up silently, got %v", res.Notices)
	}
	res, err = Generate(dir, "service", "orders")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Notices) != 0 {
		t.Fatalf("service must rewire silently, got %v", res.Notices)
	}
	for _, f := range []string{"controller.go", "service.go"} {
		if _, err := os.Stat(filepath.Join(base, f)); err != nil {
			t.Fatalf("expected %s: %v", f, err)
		}
	}
}

func TestGenerateControllerNeedsModule(t *testing.T) {
	dir := setupProject(t)
	res, err := Generate(dir, "controller", "ghost")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Notices) != 1 || !strings.Contains(res.Notices[0], "generate module ghost") {
		t.Fatalf("must guide toward module generation, got %v", res.Notices)
	}
	// The file is still created; only wiring needs a human.
	if _, err := os.Stat(filepath.Join(dir, "modules", "ghost", "controller.go")); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateServicePatchesProviders(t *testing.T) {
	dir := setupProject(t)
	if _, err := Generate(dir, "module", "billing"); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(dir, "modules", "billing")
	modPath := filepath.Join(base, "module.go")

	// Simulate a module written without the service.
	if err := os.Remove(filepath.Join(base, "service.go")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(modPath)
	if err != nil {
		t.Fatal(err)
	}
	stripped := strings.Replace(string(data),
		"\t\tgrove.Provide0(di.Singleton, NewBillingService),\n", "", 1)
	if stripped == string(data) {
		t.Fatal("fixture: provider line not found")
	}
	if err := os.WriteFile(modPath, []byte(stripped), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Generate(dir, "service", "billing")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Notices) != 0 {
		t.Fatalf("patch must be silent, got %v", res.Notices)
	}
	updated := false
	for _, c := range res.Changes {
		if strings.HasSuffix(c.Path, "module.go") {
			updated = true
		}
	}
	if !updated {
		t.Fatalf("module.go must be UPDATE, got %v", res.Changes)
	}
	patched, err := os.ReadFile(modPath)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(patched), "grove.Provide0(di.Singleton, NewBillingService),"); n != 1 {
		t.Fatalf("provider line count = %d, want 1:\n%s", n, patched)
	}
	if !strings.Contains(string(patched), "// grove:providers") {
		t.Fatalf("anchor must survive patching:\n%s", patched)
	}
	if err := checkGoSyntax(filepath.Join(base, "service.go"), modPath); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateResource(t *testing.T) {
	dir := setupProject(t)
	res, err := Generate(dir, "resource", "billing")
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, c := range res.Changes {
		paths = append(paths, c.Path)
	}
	for _, want := range []string{"module.go", "service.go", "controller.go", "resource_test.go"} {
		found := false
		for _, p := range paths {
			if strings.HasSuffix(p, want) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("resource must create %s, got %v", want, paths)
		}
	}
	base := filepath.Join(dir, "modules", "billing")
	for _, f := range []string{"module.go", "service.go", "controller.go", "resource_test.go"} {
		data, err := os.ReadFile(filepath.Join(base, f))
		if err != nil {
			t.Fatalf("expected %s: %v", f, err)
		}
		if !strings.Contains(string(data), "package billing") {
			t.Fatalf("%s missing package clause", f)
		}
	}
	// New Controller naming (not legacy Handler).
	ctrl, _ := os.ReadFile(filepath.Join(base, "controller.go"))
	if !strings.Contains(string(ctrl), "BillingController") || !strings.Contains(string(ctrl), "NewBillingController") {
		t.Fatalf("controller.go must use BillingController:\n%s", ctrl)
	}
	mod, _ := os.ReadFile(filepath.Join(base, "module.go"))
	if !strings.Contains(string(mod), "NewBillingController") {
		t.Fatalf("module.go must wire NewBillingController:\n%s", mod)
	}
	if err := checkGoSyntax(
		filepath.Join(base, "module.go"),
		filepath.Join(base, "service.go"),
		filepath.Join(base, "controller.go"),
		filepath.Join(base, "resource_test.go"),
	); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateIdempotentSkip(t *testing.T) {
	dir := setupProject(t)
	if _, err := Generate(dir, "module", "billing"); err != nil {
		t.Fatal(err)
	}
	res, err := Generate(dir, "module", "billing")
	if err != nil {
		t.Fatalf("re-generation must not fail: %v", err)
	}
	if len(res.Changes) != 3 {
		t.Fatalf("changes = %v, want 3 SKIP", res.Changes)
	}
	for _, c := range res.Changes {
		if c.Action != ui.Skip {
			t.Fatalf("change = %+v, want SKIP", c)
		}
	}
}

func TestGenerateInvalid(t *testing.T) {
	dir := setupProject(t)
	if _, err := Generate(dir, "module", "Bad-Name!"); err == nil {
		t.Fatal("expected invalid name error")
	} else if _, ok := err.(*UsageError); !ok {
		t.Fatalf("invalid name must be UsageError, got %T", err)
	}
	if _, err := Generate(dir, "repository", "users"); err == nil {
		t.Fatal("expected unknown kind error")
	} else if _, ok := err.(*UsageError); !ok {
		t.Fatalf("unknown kind must be UsageError, got %T", err)
	}
	if _, err := Generate(t.TempDir(), "module", "x"); err == nil {
		t.Fatal("expected no-go.mod error")
	}
}

// checkGoSyntax parses files, catching template/render regressions that
// produce invalid Go before any compile step runs.
func checkGoSyntax(paths ...string) error {
	for _, p := range paths {
		if _, err := parser.ParseFile(token.NewFileSet(), p, nil, 0); err != nil {
			return err
		}
	}
	return nil
}
