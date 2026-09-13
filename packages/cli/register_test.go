package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scaffoldedProject runs Scaffold into a temp parent, returning the
// project dir.
func scaffoldedProject(t *testing.T, name string) string {
	t.Helper()
	parent := t.TempDir()
	base, _, err := Scaffold(parent, name, false, "v0.0.0-test")
	if err != nil {
		t.Fatal(err)
	}
	return base
}

func TestScaffoldChanges(t *testing.T) {
	parent := t.TempDir()
	t.Chdir(parent)
	base, changes, err := Scaffold(parent, "demo", false, "v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if base != filepath.Join(parent, "demo") {
		t.Fatalf("base = %q", base)
	}
	if len(changes) != 8 {
		t.Fatalf("changes = %v, want 8 CREATE", changes)
	}
	for _, c := range changes {
		if !strings.HasPrefix(c.Path, "demo"+string(filepath.Separator)) {
			t.Fatalf("change path = %q, want demo/ prefix", c.Path)
		}
	}
	mod, err := os.ReadFile(filepath.Join(base, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mod), "require github.com/brandonbert8/grove v1.2.3") {
		t.Fatalf("go.mod must pin the resolved version:\n%s", mod)
	}
}

func TestScaffoldShapes(t *testing.T) {
	// Module path: dir is the base, module is the full path.
	parent := t.TempDir()
	base, _, err := Scaffold(parent, "github.com/foo/bar", false, "v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(base) != "bar" {
		t.Fatalf("base = %q, want bar dir", base)
	}
	main, err := os.ReadFile(filepath.Join(base, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(main), `"github.com/foo/bar/modules/hello"`) {
		t.Fatalf("main.go imports wrong module path:\n%s", main)
	}

	// Absolute dir: honored as given, module is the base.
	abs := filepath.Join(t.TempDir(), "absapp")
	base, _, err = Scaffold("", abs, false, "v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if base != abs {
		t.Fatalf("base = %q, want %q", base, abs)
	}
	mod, err := os.ReadFile(filepath.Join(abs, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mod), "module absapp\n") {
		t.Fatalf("go.mod = %q", mod)
	}

	// Existing dir aborts without force, replaces with it.
	if _, _, err := Scaffold(parent, "github.com/foo/bar", false, "v1.2.3"); err == nil {
		t.Fatal("expected existing-dir error")
	} else if _, ok := err.(*ExistsError); !ok {
		t.Fatalf("existing dir must be ExistsError, got %T", err)
	}
	if _, _, err := Scaffold(parent, "github.com/foo/bar", true, "v1.2.3"); err != nil {
		t.Fatalf("force must replace: %v", err)
	}
}

func TestGenerateModuleRegistersInMain(t *testing.T) {
	base := scaffoldedProject(t, "demo")
	res, err := Generate(base, "module", "billing")
	if err != nil {
		t.Fatal(err)
	}
	updated := false
	for _, c := range res.Changes {
		if strings.HasSuffix(c.Path, "main.go") {
			updated = true
		}
	}
	if !updated {
		t.Fatalf("main.go must be UPDATE, got %v", res.Changes)
	}
	main, err := os.ReadFile(filepath.Join(base, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(main)
	if !strings.Contains(src, `"demo/modules/billing"`) {
		t.Fatalf("main.go must import the module:\n%s", src)
	}
	if !strings.Contains(src, "billing.Module.AsModule()") {
		t.Fatalf("main.go must register the module:\n%s", src)
	}
	if err := checkGoSyntax(filepath.Join(base, "main.go")); err != nil {
		t.Fatalf("patched main.go must parse: %v", err)
	}
}

func TestGenerateModuleWithoutMainGoGuides(t *testing.T) {
	dir := setupProject(t) // go.mod only, no main.go
	res, err := Generate(dir, "module", "billing")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(res.Notices, "\n")
	if !strings.Contains(joined, "manually") {
		t.Fatalf("must guide manual registration, got %v", res.Notices)
	}
}

func TestGenerateModuleHandwrittenMainGuides(t *testing.T) {
	dir := setupProject(t)
	main := "package main\n\nfunc main() {}\n"
	mainPath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(mainPath, []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Generate(dir, "module", "billing")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(res.Notices, "\n")
	if !strings.Contains(joined, "not in generated shape") {
		t.Fatalf("must warn about hand-written main.go, got %v", res.Notices)
	}
	after, _ := os.ReadFile(mainPath)
	if string(after) != main {
		t.Fatal("hand-written main.go must not be modified")
	}
}
