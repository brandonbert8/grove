package grove

import (
	"fmt"
	"strings"
)

// Resolver resolves a dependency by container key. *App, *di.Container,
// and *ModuleContext implement it, so grove.Inject / grove.Wire work
// against the root app or an isolated module scope.
type Resolver interface {
	Resolve(name string) (any, error)
}

// ModuleContext is a module's view of the app during Register/Build.
// Providers are still stored in the app container; the context enforces
// NestJS-style visibility: own providers + imports' exports + @Global
// modules. It is passed to ModuleDef.Build.
type ModuleContext struct {
	// App is the root application.
	App *App
	// Module is the definition being built.
	Module *ModuleDef

	isolated  bool
	own       map[string]bool
	imports   []*ModuleContext
	reexports []*ModuleContext
}

// Isolated reports whether this module enforces provider visibility.
func (m *ModuleContext) Isolated() bool { return m != nil && m.isolated }

// Resolve resolves name with module visibility rules. Non-isolated
// modules delegate to the app container (v0.2 behavior). Isolated
// modules allow their own providers, imported modules' exports, and
// @Global exports; anything owned by another non-exporting module is
// rejected with a diagnostic. Ambient providers registered directly on
// the container (di.RegisterSingletonAs, test overrides) stay visible
// as an escape hatch.
func (m *ModuleContext) Resolve(name string) (any, error) {
	if m == nil || m.App == nil || m.Module == nil {
		return nil, fmt.Errorf("grove: nil ModuleContext")
	}
	if !m.isolated {
		return m.App.Container.Resolve(name)
	}
	if m.own[name] {
		return m.App.Container.Resolve(name)
	}
	for _, imp := range m.imports {
		if imp.exported(name) {
			return m.App.Container.Resolve(name)
		}
	}
	for _, g := range m.App.globalScopes {
		if g != m && g.exported(name) {
			return m.App.Container.Resolve(name)
		}
	}
	if owner, ok := m.App.providerOwners[name]; ok {
		return nil, fmt.Errorf("grove: module %q cannot resolve provider %q owned by module %q: not exported (add it to Exports or import that module)", m.Module.displayName(), name, owner)
	}
	return m.App.Container.Resolve(name)
}

// exported reports whether name is visible to importers. Isolated
// modules export only explicit Exports entries; legacy modules keep the
// v0.2 rule (empty Exports means fully public).
func (m *ModuleContext) exported(name string) bool {
	if m == nil || m.Module == nil {
		return false
	}
	if m.isolated {
		for _, e := range m.Module.Exports {
			if e == name {
				return true
			}
		}
	} else if m.Module.Exported(name) {
		return true
	}
	for _, re := range m.reexports {
		if re.exported(name) {
			return true
		}
	}
	return false
}

// Provide registers one provider inside the module during Build. Most
// modules should declare providers in ModuleDef.Providers; Provide is
// for dynamic modules that need configuration before registration.
func (m *ModuleContext) Provide(p Provider) error {
	if m == nil || m.App == nil || m.Module == nil {
		return fmt.Errorf("grove: Provide called on nil ModuleContext")
	}
	if err := m.App.registerProvider(m.Module.displayName(), p, m.isolated); err != nil {
		return err
	}
	if m.own == nil {
		m.own = make(map[string]bool)
	}
	m.own[p.Name] = true
	m.Module.Providers = append(m.Module.Providers, p)
	return nil
}

// Controller mounts one controller in this module (BuildControllers
// equivalent for ModuleDef.Build).
func (m *ModuleContext) Controller(c ControllerDef) {
	if m == nil || m.App == nil || m.Module == nil {
		return
	}
	m.App.mountControllerFor(m.Module.displayName(), c)
}

// Controllers mounts zero or more controllers in this module.
func (m *ModuleContext) Controllers(defs ...ControllerDef) {
	for _, c := range defs {
		m.Controller(c)
	}
}

// String renders the module context for diagnostics.
func (m *ModuleContext) String() string {
	if m == nil || m.Module == nil {
		return "<nil module>"
	}
	imports := make([]string, 0, len(m.imports))
	for _, imp := range m.imports {
		imports = append(imports, imp.Module.displayName())
	}
	return fmt.Sprintf("module %q (isolated=%v imports=%s)", m.Module.displayName(), m.isolated, strings.Join(imports, ","))
}
