package cli

import (
	"strings"
	"testing"
)

// run builds an isolated tree, executes args in dir, and captures all
// output (cobra sends help to stderr). Output never touches the terminal.
func run(t *testing.T, dir string, args ...string) (string, int) {
	t.Helper()
	if dir != "" {
		t.Chdir(dir)
	}
	root := NewRootCommand()
	var out strings.Builder
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	code := runRoot(root)
	return out.String(), code
}

func TestHelp(t *testing.T) {
	for _, args := range [][]string{nil, {"--help"}, {"help"}} {
		stdout, code := run(t, "", args...)
		if code != 0 {
			t.Fatalf("help %v: exit = %d", args, code)
		}
		for _, want := range []string{"◆ Grove", "new", "generate", "version", "grove new api", "grove g module users"} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("help %v missing %q:\n%s", args, want, stdout)
			}
		}
	}
}

func TestVersion(t *testing.T) {
	stdout, code := run(t, "", "version")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	for _, want := range []string{"◆ Grove", "Grove CLI", "Go", "OS"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("version missing %q:\n%s", want, stdout)
		}
	}
}

func TestVersionFlag(t *testing.T) {
	for _, args := range [][]string{{"--version"}, {"-v"}} {
		stdout, code := run(t, "", args...)
		if code != 0 {
			t.Fatalf("exit = %d", code)
		}
		if !strings.Contains(stdout, "grove version") {
			t.Fatalf("flag %v output:\n%s", args, stdout)
		}
	}
}

func TestGenerateAlias(t *testing.T) {
	dir := setupProject(t)
	stdout, code := run(t, dir, "g", "module", "shop")
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, stdout)
	}
	if !strings.Contains(stdout, "CREATE") || !strings.Contains(stdout, "✔") {
		t.Fatalf("alias output:\n%s", stdout)
	}
}

func TestNewMissingName(t *testing.T) {
	_, code := run(t, "", "new")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestNewExistingDir(t *testing.T) {
	dir := setupProject(t)
	stdout, code := run(t, dir, "new", "demo", "--grove-version", "v0.0.0-test")
	if code != 0 {
		t.Fatalf("setup new: exit = %d\n%s", code, stdout)
	}
	stdout, code = run(t, dir, "new", "demo", "--grove-version", "v0.0.0-test")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stdout, "✖") || !strings.Contains(stdout, "already exists") || !strings.Contains(stdout, "--force") {
		t.Fatalf("error block:\n%s", stdout)
	}
}

func TestUnknownCommand(t *testing.T) {
	stdout, code := run(t, "", "frobnicate")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stdout, "unknown command") {
		t.Fatalf("output:\n%s", stdout)
	}
}

func TestUnknownGenerateKind(t *testing.T) {
	dir := setupProject(t)
	_, code := run(t, dir, "generate", "repository", "users")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestNoColorOutput(t *testing.T) {
	dir := setupProject(t)
	stdout, code := run(t, dir, "new", "demo", "--no-color", "--grove-version", "v0.0.0-test")
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, stdout)
	}
	if strings.Contains(stdout, "\x1b[") {
		t.Fatalf("no-color output must not contain ANSI:\n%q", stdout)
	}
	for _, want := range []string{"CREATE", "✔", "Next steps:", "cd demo"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("output missing %q:\n%s", want, stdout)
		}
	}
}

func TestNoColorEnv(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	dir := setupProject(t)
	stdout, code := run(t, dir, "new", "demo", "--grove-version", "v0.0.0-test")
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, stdout)
	}
	if strings.Contains(stdout, "\x1b[") {
		t.Fatalf("NO_COLOR must disable ANSI:\n%q", stdout)
	}
}

func TestVerboseOutput(t *testing.T) {
	dir := setupProject(t)
	stdout, code := run(t, dir, "--verbose", "new", "demo", "--grove-version", "v0.0.0-test")
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, stdout)
	}
	if !strings.Contains(stdout, "[verbose]") {
		t.Fatalf("verbose must emit debug lines:\n%s", stdout)
	}
}

func TestNewFullOutput(t *testing.T) {
	dir := setupProject(t)
	stdout, code := run(t, dir, "new", "demo", "--grove-version", "v9.9.9")
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, stdout)
	}
	for _, want := range []string{
		"◆ Grove", "CREATE", "go.mod", "main.go", "README.md",
		"modules/hello/module.go", "✔", "Next steps:",
		"go mod tidy", "http://localhost:3000",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("output missing %q:\n%s", want, stdout)
		}
	}
}
