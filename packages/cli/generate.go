package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// validName constrains generated package/module names to safe Go identifiers.
var validName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// generate dispatches `grove generate <kind> <name>`.
func (r *Runner) generate(kind, name string, out func(string)) error {
	name = strings.ToLower(strings.TrimSpace(name))
	if !validName.MatchString(name) {
		return fmt.Errorf("invalid name %q: use lowercase letters, digits, underscores", name)
	}
	root, _, err := findModuleRoot(r.Dir)
	if err != nil {
		return err
	}
	target := filepath.Join(root, "modules", name)
	switch strings.ToLower(kind) {
	case "module":
		if err := writeFiles(target, moduleFiles(name)); err != nil {
			return err
		}
		out(fmt.Sprintf("created module %s in %s", name, relPath(target)))
	case "controller":
		ensureDir(target)
		if err := writeFiles(target, map[string]string{
			"handler.go": controllerFile(name),
		}); err != nil {
			return err
		}
		out(wireControllerReport(target, name))
	case "service":
		ensureDir(target)
		if err := writeFiles(target, map[string]string{
			"service.go": serviceFile(name, title(name)),
		}); err != nil {
			return err
		}
		out(wireServiceReport(target, name))
	default:
		return fmt.Errorf("unknown generate kind %q: want module|controller|service", kind)
	}
	return nil
}

// providersAnchor marks the Providers entry point in generated module.go.
// `grove generate service` inserts above it; its presence proves the
// file is in generated shape and safe to patch.
const providersAnchor = "\t\t// grove:providers\n"

// wireServiceReport patches module.go Providers when possible and
// reports the wiring state explicitly — never silently.
func wireServiceReport(target, name string) string {
	rel := relPath(target)
	ctor := "New" + title(name) + "Service"
	p := filepath.Join(target, "module.go")
	data, err := os.ReadFile(p)
	if err != nil {
		return fmt.Sprintf("created service in %s (no module.go: run `grove generate module %s` first, then add %s to Providers)", rel, name, ctor)
	}
	src := string(data)
	if strings.Contains(src, ctor) {
		return fmt.Sprintf("created service in %s (already wired in module.go providers)", rel)
	}
	if strings.Count(src, providersAnchor) != 1 {
		return fmt.Sprintf("created service in %s (module.go is not in generated shape: add grove.Provide0(di.Singleton, %s) to Providers manually)", rel, ctor)
	}
	line := "\t\tgrove.Provide0(di.Singleton, " + ctor + "),\n"
	patched := strings.Replace(src, providersAnchor, line+providersAnchor, 1)
	if err := os.WriteFile(p, []byte(patched), 0o644); err != nil {
		return fmt.Sprintf("created service in %s (patch failed: %v — add %s to Providers manually)", rel, err, ctor)
	}
	return fmt.Sprintf("created service in %s (wired into module.go providers)", rel)
}

// wireControllerReport verifies the generated handler is picked up by
// module.go BuildControllers and reports the wiring state explicitly.
func wireControllerReport(target, name string) string {
	rel := relPath(target)
	ctor := "New" + title(name) + "Handler"
	data, err := os.ReadFile(filepath.Join(target, "module.go"))
	if err != nil {
		return fmt.Sprintf("created controller in %s (no module.go: run `grove generate module %s` first, then wire %s)", rel, name, ctor)
	}
	if strings.Contains(string(data), ctor) {
		return fmt.Sprintf("created controller in %s (picked up by module.go BuildControllers)", rel)
	}
	return fmt.Sprintf("created controller in %s (module.go does not reference %s: wire it into BuildControllers)", rel, ctor)
}

// moduleFiles returns the full file set for a new module.
func moduleFiles(name string) map[string]string {
	return map[string]string{
		"module.go":  genModuleFile(name),
		"service.go": serviceFile(name, title(name)),
		"handler.go": controllerFile(name),
	}
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

// writeFiles creates dir and writes each file, refusing to overwrite.
func writeFiles(dir string, files map[string]string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for rel, content := range files {
		p := filepath.Join(dir, rel)
		if _, err := os.Stat(p); err == nil {
			return fmt.Errorf("file already exists: %s", p)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// ensureDir creates dir when missing, ignoring errors (writeFiles reports).
func ensureDir(dir string) { _ = os.MkdirAll(dir, 0o755) }

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

// genModuleFile renders module.go wiring service + controller.
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
		svc, err := di.ResolveAs[*%[2]sService](app.Container)
		if err != nil {
			return nil, err
		}
		h := New%[2]sHandler(svc)
		return []grove.ControllerDef{{
			Prefix: "/%[1]s",
			Endpoints: []grove.Endpoint{
				// Empty paths mount exactly at the prefix: GET/POST /%[1]s.
				grove.GET("", h.List),
				grove.POST("", h.Create),
			},
		}}, nil
	},
}
`, name, t)
}
