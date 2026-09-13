package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repoRoot resolves the Grove checkout containing this test.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	root, err := filepath.Abs(filepath.Join(filepath.Dir(file), "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// goTool runs go with a hermetic environment (module cache only, no
// network, no sumdb) inside dir.
func goTool(t *testing.T, dir string, args ...string) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GOPROXY=off",
		"GOSUMDB=off",
		"GOFLAGS=-mod=mod",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// TestScaffoldSmoke is the end-to-end DX gate the task demands:
//
//	grove new → generated project → go mod tidy → go vet → go build → go test
//
// The framework dependency resolves via replace to this checkout, so the
// test is hermetic and specifically guards the "unknown revision"
// regression class: whatever version Scaffold pins, the project must
// resolve and compile.
func TestScaffoldSmoke(t *testing.T) {
	parent := t.TempDir()
	base, changes, err := Scaffold(parent, "demo", false, "v0.0.0-smoke")
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 8 {
		t.Fatalf("changes = %v, want 8 files", changes)
	}

	gomod, err := os.ReadFile(filepath.Join(base, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	gomod = append(gomod, "\nreplace "+GroveModule+" => "+repoRoot(t)+"\n"...)
	if err := os.WriteFile(filepath.Join(base, "go.mod"), gomod, 0o644); err != nil {
		t.Fatal(err)
	}

	goTool(t, base, "mod", "tidy")
	goTool(t, base, "vet", "./...")
	goTool(t, base, "build", "./...")
	goTool(t, base, "test", "./...")
}
