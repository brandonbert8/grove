package cli

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	if err := r.generate("module", "orders", func(string) {}); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(r.Dir, "modules", "orders")
	// Regenerating a single file completes a partial module: drop the
	// generated handler/service and recreate them through the commands.
	mustRemove := func(f string) {
		t.Helper()
		if err := os.Remove(filepath.Join(base, f)); err != nil {
			t.Fatal(err)
		}
	}
	mustRemove("handler.go")
	mustRemove("service.go")

	var msgs []string
	if err := r.generate("controller", "orders", func(m string) { msgs = append(msgs, m) }); err != nil {
		t.Fatal(err)
	}
	if err := r.generate("service", "orders", func(m string) { msgs = append(msgs, m) }); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"handler.go", "service.go"} {
		if _, err := os.Stat(filepath.Join(base, f)); err != nil {
			t.Fatalf("expected %s: %v", f, err)
		}
	}
	// Wiring state must be explicit: the controller is picked up by the
	// existing BuildControllers, the service resolves to its provider.
	joined := strings.Join(msgs, "\n")
	if !strings.Contains(joined, "picked up by module.go") {
		t.Fatalf("controller must report wiring, got:\n%s", joined)
	}
	if !strings.Contains(joined, "already wired") {
		t.Fatalf("service must report wiring, got:\n%s", joined)
	}
}

func TestGenerateControllerNeedsModule(t *testing.T) {
	r := setupProject(t)
	var msgs []string
	if err := r.generate("controller", "ghost", func(m string) { msgs = append(msgs, m) }); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(msgs, "\n"), "generate module ghost") {
		t.Fatalf("must guide toward module generation, got %v", msgs)
	}
}

func TestGenerateServicePatchesProviders(t *testing.T) {
	r := setupProject(t)
	if err := r.generate("module", "billing", func(string) {}); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(r.Dir, "modules", "billing")
	modPath := filepath.Join(base, "module.go")

	// Simulate a module written without the service: drop service.go and
	// strip its provider line, keeping the anchor.
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

	var msgs []string
	if err := r.generate("service", "billing", func(m string) { msgs = append(msgs, m) }); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(msgs, "\n"), "wired into module.go") {
		t.Fatalf("must report patching, got %v", msgs)
	}
	patched, err := os.ReadFile(modPath)
	if err != nil {
		t.Fatal(err)
	}
	// The provider line is back exactly once; the anchor survives for
	// future generations.
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

func TestScaffoldShapes(t *testing.T) {
	// Plain name: dir + module share the name.
	r := &Runner{Out: new(strings.Builder), Dir: t.TempDir()}
	if err := r.scaffold("myapp", false); err != nil {
		t.Fatal(err)
	}
	mod, err := os.ReadFile(filepath.Join(r.Dir, "myapp", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mod), "module myapp\n") {
		t.Fatalf("go.mod = %q", mod)
	}

	// Module path: dir is the base, module is the full path.
	r2 := &Runner{Out: new(strings.Builder), Dir: t.TempDir()}
	if err := r2.scaffold("github.com/foo/bar", false); err != nil {
		t.Fatal(err)
	}
	main, err := os.ReadFile(filepath.Join(r2.Dir, "bar", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(main), `"github.com/foo/bar/modules/hello"`) {
		t.Fatalf("main.go imports = wrong module path:\n%s", main)
	}

	// Absolute dir: honored as given, module is the base.
	abs := filepath.Join(t.TempDir(), "absapp")
	r3 := &Runner{Out: new(strings.Builder)}
	if err := r3.scaffold(abs, false); err != nil {
		t.Fatal(err)
	}
	mod, err = os.ReadFile(filepath.Join(abs, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mod), "module absapp\n") {
		t.Fatalf("go.mod = %q", mod)
	}

	// Existing dir aborts without --force, replaces with it.
	if err := r.scaffold("myapp", false); err == nil {
		t.Fatal("expected existing-dir error")
	}
	if err := r.scaffold("myapp", true); err != nil {
		t.Fatalf("force must replace: %v", err)
	}
}

// TestGeneratedProjectBuilds is the end-to-end DX gate: a generated
// module inside a scratch project must compile against the working
// Grove copy (via replace), hermetically (module cache only).
func TestGeneratedProjectBuilds(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/demo\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{Out: new(strings.Builder), Dir: dir}
	if err := r.generate("module", "shop", func(string) {}); err != nil {
		t.Fatal(err)
	}
	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot, err := filepath.Abs(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	gomod := "module example.com/demo\n\ngo 1.25\n\nrequire github.com/brandonbert8/grove v0.0.0\n\nreplace github.com/brandonbert8/grove => " + repoRoot + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOPROXY=off", "GOSUMDB=off", "GOFLAGS=-mod=mod")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated project must build: %v\n%s", err, out)
	}
}
