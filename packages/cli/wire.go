package cli

import (
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/brandonbert8/grove/packages/cli/ui"
)

// providersAnchor marks the Providers entry point in generated module.go.
// `grove generate service` inserts above it; its presence proves the
// file is in generated shape and safe to patch.
const providersAnchor = "\t\t// grove:providers\n"

// modulesAnchor marks the MustRegister entry point in a grove-managed
// main.go. `grove generate module` inserts above it.
const modulesAnchor = "\t\t// grove:modules\n"

// importsAnchor marks the import entry point in a grove-managed main.go.
const importsAnchor = "\t// grove:imports\n"

// registerMainGo wires a fresh module into main.go when it carries the
// grove anchors (scaffold shape). It returns the UPDATE change on
// success, or a guidance notice when the file is missing, hand-written,
// or already wired. Never fails, never guesses.
func registerMainGo(root, modPath, name string) (*ui.FileChange, string) {
	mainPath := filepath.Join(root, "main.go")
	data, err := os.ReadFile(mainPath)
	if err != nil {
		return nil, fmt.Sprintf("add %s.Module.AsModule() to main() imports and MustRegister manually", name)
	}
	src := string(data)
	ref := name + ".Module.AsModule()"
	if strings.Contains(src, ref) {
		return nil, ""
	}
	if strings.Count(src, modulesAnchor) != 1 || strings.Count(src, importsAnchor) != 1 {
		return nil, fmt.Sprintf("main.go is not in generated shape: add %q to imports and %s to MustRegister manually", modPath+"/modules/"+name, ref)
	}
	patched := strings.Replace(src,
		importsAnchor, "\t"+strconv.Quote(modPath+"/modules/"+name)+"\n"+importsAnchor, 1)
	patched = strings.Replace(patched,
		modulesAnchor, "\t\t"+ref+",\n"+modulesAnchor, 1)
	formatted, err := format.Source([]byte(patched))
	if err != nil {
		return nil, fmt.Sprintf("main.go patch failed validation: add %s manually", ref)
	}
	if err := os.WriteFile(mainPath, formatted, 0o644); err != nil {
		return nil, fmt.Sprintf("main.go write failed: %v — add %s manually", err, ref)
	}
	return &ui.FileChange{Action: ui.Update, Path: relPath(mainPath)}, ""
}

// wireService patches module.go Providers when possible. It returns the
// UPDATE change on success, or a guidance notice otherwise.
func wireService(target, name string) (*ui.FileChange, string) {
	ctor := "New" + title(name) + "Service"
	p := filepath.Join(target, "module.go")
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Sprintf("no module.go: run `grove generate module %s` first, then add %s to Providers", name, ctor)
	}
	src := string(data)
	if strings.Contains(src, ctor) {
		return nil, ""
	}
	if strings.Count(src, providersAnchor) != 1 {
		return nil, fmt.Sprintf("module.go is not in generated shape: add grove.Provide0(di.Singleton, %s) to Providers manually", ctor)
	}
	line := "\t\tgrove.Provide0(di.Singleton, " + ctor + "),\n"
	patched := strings.Replace(src, providersAnchor, line+providersAnchor, 1)
	if err := os.WriteFile(p, []byte(patched), 0o644); err != nil {
		return nil, fmt.Sprintf("module.go patch failed: %v — add %s to Providers manually", err, ctor)
	}
	return &ui.FileChange{Action: ui.Update, Path: relPath(p)}, ""
}

// wireController verifies the generated handler is picked up by
// module.go BuildControllers, returning guidance when it is not.
func wireController(target, name string) string {
	ctor := "New" + title(name) + "Handler"
	data, err := os.ReadFile(filepath.Join(target, "module.go"))
	if err != nil {
		return fmt.Sprintf("no module.go: run `grove generate module %s` first, then wire %s", name, ctor)
	}
	if strings.Contains(string(data), ctor) {
		return ""
	}
	return fmt.Sprintf("module.go does not reference %s: wire it into BuildControllers", ctor)
}
