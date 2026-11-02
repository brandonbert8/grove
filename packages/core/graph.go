package grove

import (
	"strings"

	"github.com/brandonbert8/grove/packages/di"
	"github.com/brandonbert8/grove/packages/router"
)

// ProviderInfo is the graph/diagnostics view of one Provider. It carries
// no constructor or factory: only the data tooling can print safely.
type ProviderInfo struct {
	// Name is the container key.
	Name string
	// Lifetime is singleton, transient, or request.
	Lifetime string
}

// ControllerInfo is the graph/diagnostics view of one mounted
// ControllerDef (including controllers built by BuildControllers).
type ControllerInfo struct {
	// Prefix is the full mounted prefix (global prefix applied).
	Prefix string
	// Routes lists "METHOD path" entries in registration order.
	Routes []string
}

// ModuleInfo is one node in the module graph. Imports are module names,
// not pointers, so the graph is safe to serialize.
type ModuleInfo struct {
	// Name identifies the module.
	Name string
	// Global marks @Global modules (visible without importing).
	Global bool
	// Isolated marks modules using per-module provider visibility.
	Isolated bool
	// Imports lists imported module names in declaration order.
	Imports []string
	// Exports lists provider keys the module makes public.
	Exports []string
	// ExportModules lists imported modules whose public providers are
	// re-exported (NestJS `exports: [SomeModule]`).
	ExportModules []string
	// Providers lists the module's own DI registrations.
	Providers []ProviderInfo
	// Controllers lists mounted controllers and their routes.
	Controllers []ControllerInfo
}

// routeRegistration records one mounted route for duplicate detection.
type routeRegistration struct {
	Method string
	Path   string
	Module string
}

// ModuleGraph returns every module built into the app in registration
// order (imports included, depth-first), plus legacy ModuleFunc modules
// that are not ModuleDefs. It is a snapshot: mutating the returned slice
// never affects the app.
func (a *App) ModuleGraph() []ModuleInfo {
	out := make([]ModuleInfo, 0, len(a.moduleDefs)+len(a.modules))
	seen := make(map[*ModuleDef]bool, len(a.moduleDefs))
	for _, m := range a.moduleDefs {
		if m == nil || seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, a.moduleInfo(m))
	}
	for _, m := range a.modules {
		if ad, ok := m.(moduleDefAdapter); ok && ad.m != nil {
			continue // already represented by moduleDefs
		}
		out = append(out, ModuleInfo{Name: m.Name()})
	}
	return out
}

// moduleInfo builds one ModuleInfo from a ModuleDef.
func (a *App) moduleInfo(m *ModuleDef) ModuleInfo {
	info := ModuleInfo{
		Name:      m.displayName(),
		Global:    m.Global,
		Isolated:  m.Isolated || a.moduleIsolation,
		Exports:   append([]string(nil), m.Exports...),
		Providers: make([]ProviderInfo, 0, len(m.Providers)),
	}
	for _, imp := range m.Imports {
		if imp != nil {
			info.Imports = append(info.Imports, imp.displayName())
		}
	}
	for _, re := range m.ExportModules {
		if re != nil {
			info.ExportModules = append(info.ExportModules, re.displayName())
		}
	}
	for _, p := range m.Providers {
		info.Providers = append(info.Providers, ProviderInfo{Name: p.Name, Lifetime: lifetimeName(p.Lifetime)})
	}
	if controllers, ok := a.moduleControllers[info.Name]; ok {
		for _, c := range controllers {
			c.Routes = append([]string(nil), c.Routes...)
			info.Controllers = append(info.Controllers, c)
		}
	}
	return info
}

// lifetimeName renders a di.Lifetime for graph output.
func lifetimeName(l di.Lifetime) string {
	switch l {
	case di.Singleton:
		return "singleton"
	case di.Transient:
		return "transient"
	case di.RequestScoped:
		return "request"
	default:
		return "unknown"
	}
}

// recordRoutes appends the routes in c to the app route registry.
func (a *App) recordRoutes(module, prefix string, c ControllerDef) {
	for _, e := range c.Endpoints {
		p := router.JoinPath(prefix, e.Path)
		if p == "" {
			p = "/"
		} else if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		a.routes = append(a.routes, routeRegistration{
			Method: strings.ToUpper(strings.TrimSpace(e.Method)),
			Path:   p,
			Module: module,
		})
	}
}
