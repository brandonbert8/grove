package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/brandonbert8/grove/packages/cli/ui"
)

// validName constrains generated package/module names to safe Go identifiers.
var validName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// GenerateResult is one generator run: file operations for the renderer
// plus warning notices (manual steps). Nothing prints here.
type GenerateResult struct {
	Changes []ui.FileChange
	Notices []string
}

// Generate runs `grove generate <kind> <name>` inside the project found
// above rootDir. Invalid names/kinds are UsageErrors (exit 2); anything
// else is a runtime error with hints added by the command layer.
func Generate(rootDir, kind, name string) (GenerateResult, error) {
	var res GenerateResult
	name = strings.ToLower(strings.TrimSpace(name))
	if !validName.MatchString(name) {
		return res, &UsageError{Msg: fmt.Sprintf("invalid name %q: use lowercase letters, digits, underscores", name)}
	}
	root, modPath, err := findModuleRoot(rootDir)
	if err != nil {
		return res, err
	}
	target := filepath.Join(root, "modules", name)
	switch strings.ToLower(kind) {
	case "module":
		changes, err := writeFilesCollect(target, moduleFiles(name))
		if err != nil {
			return res, err
		}
		res.Changes = append(res.Changes, rebase(target, changes)...)
		if chg, notice := registerMainGo(root, modPath, name); chg != nil {
			res.Changes = append(res.Changes, *chg)
		} else if notice != "" {
			res.Notices = append(res.Notices, notice)
		}
	case "controller":
		changes, err := writeFilesCollect(target, map[string]string{
			"controller.go": controllerFile(name),
		})
		if err != nil {
			return res, err
		}
		res.Changes = append(res.Changes, rebase(target, changes)...)
		if notice := wireController(target, name); notice != "" {
			res.Notices = append(res.Notices, notice)
		}
	case "resource":
		// The `nest generate resource` equivalent: a full CRUD slice
		// (module + service + controller) plus a smoke test, wired
		// into main.go like `module`.
		changes, err := writeFilesCollect(target, resourceFiles(name))
		if err != nil {
			return res, err
		}
		res.Changes = append(res.Changes, rebase(target, changes)...)
		if chg, notice := registerMainGo(root, modPath, name); chg != nil {
			res.Changes = append(res.Changes, *chg)
		} else if notice != "" {
			res.Notices = append(res.Notices, notice)
		}
	case "service":
		changes, err := writeFilesCollect(target, map[string]string{
			"service.go": serviceFile(name, title(name)),
		})
		if err != nil {
			return res, err
		}
		res.Changes = append(res.Changes, rebase(target, changes)...)
		if chg, notice := wireService(target, name); chg != nil {
			res.Changes = append(res.Changes, *chg)
		} else if notice != "" {
			res.Notices = append(res.Notices, notice)
		}
	default:
		return res, &UsageError{Msg: fmt.Sprintf("unknown generate kind %q: want module|controller|service|resource", kind)}
	}
	// A fully-skipped run says so explicitly instead of a bare success.
	if len(res.Changes) > 0 {
		allSkip := true
		for _, c := range res.Changes {
			if c.Action != ui.Skip {
				allSkip = false
				break
			}
		}
		if allSkip {
			res.Notices = append(res.Notices,
				fmt.Sprintf("%s %q already exists — nothing to do", title(strings.ToLower(kind)), name))
		}
	}
	return res, nil
}

// rebase rewrites change paths (relative to dir) as cwd-relative display
// paths, e.g. module.go under modules/billing.
func rebase(dir string, changes []ui.FileChange) []ui.FileChange {
	for i, c := range changes {
		changes[i].Path = relPath(filepath.Join(dir, c.Path))
	}
	return changes
}

// moduleFiles returns the full file set for a new module.
func moduleFiles(name string) map[string]string {
	return map[string]string{
		"module.go":     genModuleFile(name),
		"service.go":    serviceFile(name, title(name)),
		"controller.go": controllerFile(name),
	}
}

// resourceFiles returns the full file set for `generate resource`: the
// module slice plus a compilable smoke test (the `nest g resource`
// equivalent).
func resourceFiles(name string) map[string]string {
	files := moduleFiles(name)
	files["resource_test.go"] = resourceTestFile(name)
	return files
}

// findModuleRoot walks up from dir to the directory holding go.mod,
// returning the root and the module path declared inside.
func findModuleRoot(dir string) (root, modPath string, err error) {
	if dir == "" {
		var e error
		dir, e = os.Getwd()
		if e != nil {
			return "", "", e
		}
	}
	d := dir
	for {
		gomod := filepath.Join(d, "go.mod")
		if data, e := os.ReadFile(gomod); e == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if path, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
					return d, strings.TrimSpace(path), nil
				}
			}
			return "", "", fmt.Errorf("go.mod has no module line: %s", gomod)
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", "", fmt.Errorf("no go.mod found above %s: run inside a Grove project", dir)
		}
		d = parent
	}
}

// relPath shortens p for display.
func relPath(p string) string {
	if cwd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(cwd, p); err == nil && !strings.HasPrefix(rel, "..") {
			return rel
		}
	}
	return p
}

// title capitalizes the first letter for type names.
func title(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// genModuleFile renders module.go wiring service + controller in the
// NestJS ModuleDef style: providers for DI, BuildControllers resolving
// the controller with constructor injection.
func genModuleFile(name string) string {
	t := title(name)
	return fmt.Sprintf(`// Package %[1]s implements the %[1]s module.
package %[1]s

import (
	grove "github.com/brandonbert8/grove/packages/core"
	"github.com/brandonbert8/grove/packages/di"
)

// Module wires the %[1]s providers and controller. Register it with
// app.MustRegister(%[1]s.Module.AsModule()).
var Module = &grove.ModuleDef{
	Name: "%[1]s",
	Providers: []grove.Provider{
		grove.Provide0(di.Singleton, New%[2]sService),
		// grove:providers
	},
	BuildControllers: func(app *grove.App) ([]grove.ControllerDef, error) {
		ctrl, err := grove.Wire(app, New%[2]sController)
		if err != nil {
			return nil, err
		}
		return []grove.ControllerDef{{
			Prefix: "/%[1]s",
			Tags:   []string{"%[1]s"},
			Endpoints: []grove.Endpoint{
				// Empty paths mount exactly at the prefix: GET/POST /%[1]s.
				grove.GET("", ctrl.List, grove.WithSummary("List %[1]s")),
				grove.POST("", ctrl.Create, grove.WithSummary("Create %[1]s")),
				// grove:endpoints
			},
		}}, nil
	},
}
`, name, t)
}

// resourceTestFile renders a smoke test for a generated resource: it
// boots the module in-memory and drives GET/POST through httptest,
// the Test.createTestingModule equivalent.
func resourceTestFile(name string) string {
	t := title(name)
	return fmt.Sprintf(`package %[1]s

import (
	"testing"

	grove "github.com/brandonbert8/grove/packages/core"
	"github.com/brandonbert8/grove/packages/grovtest"
)

// TestResourceSmoke boots the %[1]s module without a server and drives
// its list/create endpoints through httptest.
func TestResourceSmoke(t *testing.T) {
	app := grove.New()
	app.MustRegister(Module.AsModule())

	cli := grovtest.New(app)
	grovtest.RequireStatus(t, cli.Get(t, "/%[1]s"), 200)
	grovtest.RequireStatus(t, cli.Post(t, "/%[1]s", map[string]string{"name": "%[2]s item"}), 201)
}
`, name, t)
}
