package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Version is the grove CLI version. Bumped per release.
const Version = "0.2.0"

// Runner executes CLI commands. Out receives help and status output;
// Dir is the working directory scaffolding is created in (empty means
// the process working directory).
type Runner struct {
	Out io.Writer
	Dir string
}

// New returns a Runner writing to stdout.
func New() *Runner { return &Runner{Out: os.Stdout} }

// Run dispatches args (without the program name) and returns the
// process exit code.
func (r *Runner) Run(args []string) int {
	out := r.Out
	if out == nil {
		out = os.Stdout
	}
	if len(args) == 0 {
		printHelp(out)
		return 0
	}
	switch args[0] {
	case "new":
		if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
			fmt.Fprintln(out, "usage: grove new <project> [--force]")
			return 2
		}
		force := len(args) > 2 && (args[2] == "--force" || args[2] == "-f")
		if err := r.scaffold(args[1], force); err != nil {
			fmt.Fprintln(out, "error:", err)
			return 1
		}
		fmt.Fprintln(out, "created project", args[1])
		return 0
	case "generate", "g":
		if len(args) < 3 {
			fmt.Fprintln(out, "usage: grove generate <module|controller|service> <name>")
			return 2
		}
		if err := r.generate(args[1], args[2], func(msg string) { fmt.Fprintln(out, msg) }); err != nil {
			fmt.Fprintln(out, "error:", err)
			return 1
		}
		return 0
	case "version", "--version", "-v":
		fmt.Fprintln(out, "grove version", Version)
		return 0
	case "help", "--help", "-h":
		printHelp(out)
		return 0
	default:
		fmt.Fprintln(out, "unknown command:", args[0])
		printHelp(out)
		return 2
	}
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "grove - the Grove backend framework CLI")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "commands:")
	fmt.Fprintln(w, "  grove new <project> [--force]          scaffold a new project")
	fmt.Fprintln(w, "                                           (plain name, module path, or absolute dir)")
	fmt.Fprintln(w, "  grove generate module <name>            scaffold modules/<name> (module+service+controller)")
	fmt.Fprintln(w, "  grove generate controller <name>        add a controller to modules/<name>")
	fmt.Fprintln(w, "  grove generate service <name>           add a service to modules/<name>")
	fmt.Fprintln(w, "  grove version                           print the CLI version")
}

// scaffold creates a minimal Grove project.
//
// name accepts three shapes (like `go` tooling):
//   - plain "myapp" → dir ./myapp (or Dir/myapp), module "myapp";
//   - module path "github.com/foo/bar" → dir ./bar, module "github.com/foo/bar";
//   - absolute "/tmp/myapp" → dir as given, module "myapp".
//
// Without force, an existing directory aborts; --force replaces it.
func (r *Runner) scaffold(name string, force bool) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("project name must not be empty")
	}
	parent := r.Dir
	if parent == "" {
		var err error
		parent, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	var base, modPath string
	switch {
	case filepath.IsAbs(name):
		base, modPath = name, filepath.Base(name)
	case strings.Contains(name, "/"):
		modPath = strings.TrimSuffix(name, "/")
		base = filepath.Join(parent, filepath.Base(modPath))
	default:
		base, modPath = filepath.Join(parent, name), name
	}
	if modPath == "" || modPath == "." || modPath == "/" {
		return fmt.Errorf("invalid project name %q", name)
	}
	if _, err := os.Stat(base); err == nil {
		if !force {
			return fmt.Errorf("directory %s already exists (use --force to replace)", base)
		}
		if err := os.RemoveAll(base); err != nil {
			return err
		}
	}
	dirs := []string{
		base,
		filepath.Join(base, "modules", "hello"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	modName := filepath.Base(base)
	files := map[string]string{
		"go.mod":                   scaffoldGoMod(modPath),
		"main.go":                  scaffoldMain(modPath),
		".env.example":             "PORT=3000\nLOG_LEVEL=info\n",
		".gitignore":               ".env\n",
		"modules/hello/module.go":  scaffoldHelloModule(),
		"modules/hello/service.go": scaffoldHelloService(),
		"modules/hello/handler.go": scaffoldHelloHandler(),
		"README.md":                "# " + modName + "\n\nCreated with `grove new " + modName + "`.\n\nRun with `go run .`.\n",
	}
	for rel, content := range files {
		if err := os.WriteFile(filepath.Join(base, rel), []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}
