package grove

import (
	"fmt"
	"sort"
	"strings"
)

// ValidationError aggregates architecture problems found by App.Validate.
// It is a normal error: callers can inspect Problems for structured output.
type ValidationError struct {
	// Problems are human-readable, deterministic diagnostics.
	Problems []string
}

// Error implements error.
func (e *ValidationError) Error() string {
	if len(e.Problems) == 1 {
		return "grove: " + e.Problems[0]
	}
	return fmt.Sprintf("grove: %d architecture problems:\n  - %s", len(e.Problems), strings.Join(e.Problems, "\n  - "))
}

// Validate audits the registered module graph and routes. It is
// deliberately additive and side-effect free: existing v0.2 apps keep
// working, and the checks catch wiring mistakes before requests.
//
// Checks: duplicate module names, import cycles, imports that were
// never built, duplicate provider keys, unknown exports, and duplicate
// METHOD+PATH routes. Raw Router use is invisible by design.
//
// Enable it at startup with ValidateOnStart / WithValidation, or call it
// from a test (grovtest) and CI.
func (a *App) Validate() error {
	var problems []string

	// Duplicate module names make logs, ownership, and diagnostics
	// ambiguous even though pointers still register independently.
	byName := make(map[string]*ModuleDef, len(a.moduleDefs))
	for _, m := range a.moduleDefs {
		if m == nil {
			continue
		}
		name := m.displayName()
		if prev, ok := byName[name]; ok && prev != m {
			problems = append(problems, fmt.Sprintf("module name %q is used by more than one ModuleDef", name))
			continue
		}
		byName[name] = m
	}

	problems = append(problems, a.importProblems()...)
	problems = append(problems, a.providerProblems()...)
	problems = append(problems, a.exportProblems()...)
	problems = append(problems, a.routeProblems()...)

	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return &ValidationError{Problems: problems}
}

// ValidateOnStart makes Run/Listen call Validate before OnStart hooks.
func (a *App) ValidateOnStart() { a.validateOnStart = true }

// importProblems reports import cycles and imports that were not built.
func (a *App) importProblems() []string {
	var problems []string
	for _, m := range a.moduleDefs {
		if m == nil {
			continue
		}
		for _, imp := range m.Imports {
			if imp == nil {
				continue
			}
			if a.builtModules == nil || !a.builtModules[imp] {
				problems = append(problems, fmt.Sprintf("module %q imports %q, which was not built", m.displayName(), imp.displayName()))
			}
		}
	}
	problems = append(problems, a.cycleProblems()...)
	return problems
}

// cycleProblems runs a DFS over ModuleDef imports and reports each back
// edge as a readable module path.
func (a *App) cycleProblems() []string {
	const (
		unvisited = iota
		visiting
		visited
	)
	state := make(map[*ModuleDef]int, len(a.moduleDefs))
	pos := make(map[*ModuleDef]int, len(a.moduleDefs))
	var stack []*ModuleDef
	var problems []string

	var visit func(m *ModuleDef)
	visit = func(m *ModuleDef) {
		state[m] = visiting
		pos[m] = len(stack)
		stack = append(stack, m)
		for _, imp := range m.Imports {
			if imp == nil {
				continue
			}
			switch state[imp] {
			case unvisited:
				visit(imp)
			case visiting:
				cycle := make([]string, 0, len(stack)-pos[imp]+1)
				for _, n := range stack[pos[imp]:] {
					cycle = append(cycle, n.displayName())
				}
				cycle = append(cycle, imp.displayName())
				problems = append(problems, "module import cycle: "+strings.Join(cycle, " -> "))
			}
		}
		stack = stack[:len(stack)-1]
		delete(pos, m)
		state[m] = visited
	}

	for _, m := range a.moduleDefs {
		if m != nil && state[m] == unvisited {
			visit(m)
		}
	}
	return problems
}

// providerProblems reports duplicate provider keys across modules. In
// non-strict mode Register skips the duplicate (first wins), which makes
// the mistake silent; Validate surfaces it.
func (a *App) providerProblems() []string {
	owners := make(map[string][]string, len(a.providerOwners))
	for _, m := range a.moduleDefs {
		if m == nil {
			continue
		}
		for _, p := range m.Providers {
			if p.Name == "" {
				continue
			}
			owners[p.Name] = append(owners[p.Name], m.displayName())
		}
	}
	var problems []string
	for name, mods := range owners {
		if len(mods) > 1 {
			problems = append(problems, fmt.Sprintf("provider %q is registered by multiple modules: %s", name, strings.Join(mods, ", ")))
		}
	}
	return problems
}

// exportProblems reports ModuleDef.Exports that name no known provider.
func (a *App) exportProblems() []string {
	var problems []string
	for _, m := range a.moduleDefs {
		if m == nil {
			continue
		}
		for _, e := range m.Exports {
			if e == "" {
				problems = append(problems, fmt.Sprintf("module %q exports an empty provider name", m.displayName()))
				continue
			}
			if !a.Container.Has(e) {
				problems = append(problems, fmt.Sprintf("module %q exports unknown provider %q (did you add grove.Provide for it?)", m.displayName(), e))
			}
		}
	}
	return problems
}

// routeProblems reports duplicate METHOD+PATH registrations. The router
// is last-wins, so without this check a duplicated route silently
// shadows the first handler.
func (a *App) routeProblems() []string {
	seen := make(map[string]string, len(a.routes))
	var problems []string
	for _, r := range a.routes {
		key := r.Method + " " + r.Path
		if prev, ok := seen[key]; ok {
			problems = append(problems, fmt.Sprintf("duplicate route %s registered by %q and %q", key, prev, r.Module))
			continue
		}
		seen[key] = r.Module
	}
	return problems
}
