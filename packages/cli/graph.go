package cli

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// GraphModule is one node in the source-level module graph.
type GraphModule struct {
	Name    string   `json:"name"`
	Global  bool     `json:"global,omitempty"`
	Imports []string `json:"imports,omitempty"`
}

// GraphEdge is one import edge (From imports To).
type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// GraphResult is the `grove graph` report.
type GraphResult struct {
	Modules []GraphModule `json:"modules"`
	Edges   []GraphEdge   `json:"edges"`
	Notices []string      `json:"-"`
}

// Graph scans a Grove project for `*grove.ModuleDef` literals and builds
// a module import graph. It is intentionally source-level and
// conservative: modules created dynamically at runtime (factory
// functions, loops, maps) may be missing. Use it as a fast architecture
// overview, not as a replacement for App.ModuleGraph at runtime.
func Graph(dir string) (GraphResult, error) {
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		return GraphResult{}, fmt.Errorf("no go.mod in %s", dir)
	}

	fset := token.NewFileSet()
	type parsedFile struct {
		path string
		file *ast.File
	}
	var files []parsedFile
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			if info != nil && info.IsDir() && skipGraphDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil
		}
		files = append(files, parsedFile{path: path, file: src})
		return nil
	})

	// First pass: variable/function name -> module name.
	defs := make(map[string]string)
	ambiguous := make(map[string]bool)
	modules := make(map[string]*GraphModule)
	var order []string

	addModule := func(name string) *GraphModule {
		if name == "" {
			return nil
		}
		if m, ok := modules[name]; ok {
			return m
		}
		m := &GraphModule{Name: name}
		modules[name] = m
		order = append(order, name)
		return m
	}

	recordDef := func(varName, moduleName string, global bool) {
		if moduleName == "" {
			return
		}
		m := addModule(moduleName)
		m.Global = m.Global || global
		if varName == "" {
			return
		}
		if prev, ok := defs[varName]; ok && prev != moduleName {
			ambiguous[varName] = true
			return
		}
		defs[varName] = moduleName
	}

	for _, pf := range files {
		ast.Inspect(pf.file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.ValueSpec:
				for _, v := range node.Values {
					lit := moduleDefLit(v)
					if lit == nil {
						continue
					}
					name, global := moduleDefMeta(lit)
					for _, id := range node.Names {
						recordDef(id.Name, name, global)
					}
				}
			case *ast.AssignStmt:
				for i, rhs := range node.Rhs {
					lit := moduleDefLit(rhs)
					if lit == nil || i >= len(node.Lhs) {
						continue
					}
					name, global := moduleDefMeta(lit)
					if id, ok := node.Lhs[i].(*ast.Ident); ok {
						recordDef(id.Name, name, global)
					}
				}
			case *ast.FuncDecl:
				if node.Name == nil || node.Type.Results == nil {
					return true
				}
				if !returnsModuleDef(node.Type.Results) {
					return true
				}
				if name, global := firstReturnedModule(node.Body); name != "" {
					recordDef(node.Name.Name, name, global)
				}
			}
			return true
		})
	}

	// Second pass: resolve Imports fields into edges.
	for _, pf := range files {
		ast.Inspect(pf.file, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok || !isModuleDefType(lit.Type) {
				return true
			}
			name, _ := moduleDefMeta(lit)
			if name == "" {
				return true
			}
			for _, imp := range moduleImports(lit) {
				target := resolveModuleRef(imp, defs, ambiguous)
				if target == "" {
					continue
				}
				addModule(target)
				m := modules[name]
				if !containsString(m.Imports, target) {
					m.Imports = append(m.Imports, target)
				}
			}
			return true
		})

		// Top-level app.MustRegister(...) / app.Register(...) calls add
		// modules that are not imported by another ModuleDef. Only main
		// packages are scanned: library wiring uses its own ModuleDefs
		// and would otherwise pollute the graph with parameter names.
		if pf.file.Name.Name != "main" {
			continue
		}
		ast.Inspect(pf.file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !isRegisterCall(call.Fun) {
				return true
			}
			for _, arg := range call.Args {
				if name := resolveModuleRef(arg, defs, ambiguous); name != "" {
					addModule(name)
				}
			}
			return true
		})
	}

	// Build deterministic output.
	sort.Strings(order)
	res := GraphResult{Modules: make([]GraphModule, 0, len(order))}
	seenEdges := make(map[string]bool)
	for _, name := range order {
		m := modules[name]
		sort.Strings(m.Imports)
		res.Modules = append(res.Modules, *m)
		for _, to := range m.Imports {
			key := name + "\x00" + to
			if seenEdges[key] {
				continue
			}
			seenEdges[key] = true
			res.Edges = append(res.Edges, GraphEdge{From: name, To: to})
		}
	}
	sort.Slice(res.Edges, func(i, j int) bool {
		if res.Edges[i].From != res.Edges[j].From {
			return res.Edges[i].From < res.Edges[j].From
		}
		return res.Edges[i].To < res.Edges[j].To
	})
	if len(res.Modules) == 0 {
		res.Notices = append(res.Notices, "no *grove.ModuleDef literals found (dynamic modules are not visible to the source graph)")
	}
	if len(ambiguous) > 0 {
		names := make([]string, 0, len(ambiguous))
		for n := range ambiguous {
			names = append(names, n)
		}
		sort.Strings(names)
		res.Notices = append(res.Notices, "ambiguous module variables: "+strings.Join(names, ", "))
	}
	return res, nil
}

// skipGraphDir filters generated/vendor directories.
func skipGraphDir(name string) bool {
	switch name {
	case ".git", "vendor", "node_modules", "dist", "build":
		return true
	}
	return false
}

// moduleDefLit unwraps &grove.ModuleDef{...} / &ModuleDef{...} to the
// composite literal, or nil when expr is not a module definition.
func moduleDefLit(expr ast.Expr) *ast.CompositeLit {
	switch e := expr.(type) {
	case *ast.UnaryExpr:
		return moduleDefLit(e.X)
	case *ast.CompositeLit:
		if isModuleDefType(e.Type) {
			return e
		}
	}
	return nil
}

// isModuleDefType reports whether expr names ModuleDef (grove.ModuleDef
// or a bare ModuleDef).
func isModuleDefType(expr ast.Expr) bool {
	switch t := expr.(type) {
	case *ast.SelectorExpr:
		return t.Sel != nil && t.Sel.Name == "ModuleDef"
	case *ast.Ident:
		return t.Name == "ModuleDef"
	case *ast.StarExpr:
		return isModuleDefType(t.X)
	}
	return false
}

// moduleDefMeta extracts Name and Global from a ModuleDef literal.
func moduleDefMeta(lit *ast.CompositeLit) (name string, global bool) {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch key.Name {
		case "Name":
			name = stringLit(kv.Value)
		case "Global":
			global = identTrue(kv.Value)
		}
	}
	return name, global
}

// moduleImports returns the elements of a ModuleDef.Imports field.
func moduleImports(lit *ast.CompositeLit) []ast.Expr {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != "Imports" {
			continue
		}
		cl, ok := kv.Value.(*ast.CompositeLit)
		if !ok {
			return nil
		}
		return cl.Elts
	}
	return nil
}

// resolveModuleRef maps an import expression to a module name.
func resolveModuleRef(expr ast.Expr, defs map[string]string, ambiguous map[string]bool) string {
	switch e := expr.(type) {
	case *ast.UnaryExpr:
		return resolveModuleRef(e.X, defs, ambiguous)
	case *ast.SelectorExpr:
		if e.Sel == nil {
			return ""
		}
		if name, ok := defs[e.Sel.Name]; ok && !ambiguous[e.Sel.Name] {
			return name
		}
		return selectorName(e)
	case *ast.Ident:
		if name, ok := defs[e.Name]; ok && !ambiguous[e.Name] {
			return name
		}
		return e.Name
	case *ast.CallExpr:
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok && sel.Sel != nil && sel.Sel.Name == "AsModule" {
			return resolveModuleRef(sel.X, defs, ambiguous)
		}
		return callModuleName(e)
	case *ast.CompositeLit:
		if isModuleDefType(e.Type) {
			name, _ := moduleDefMeta(e)
			return name
		}
	}
	return ""
}

// isRegisterCall reports whether fn is app.Register / app.MustRegister
// (or the bare Register / MustRegister forms). Receiver names are
// restricted to the conventional app/application so library internals
// never leak parameter names into the graph.
func isRegisterCall(fn ast.Expr) bool {
	switch f := fn.(type) {
	case *ast.SelectorExpr:
		id, ok := f.X.(*ast.Ident)
		if !ok || (id.Name != "app" && id.Name != "application") {
			return false
		}
		return f.Sel != nil && (f.Sel.Name == "Register" || f.Sel.Name == "MustRegister")
	case *ast.Ident:
		return f.Name == "Register" || f.Name == "MustRegister"
	}
	return false
}

// selectorName renders pkg.Var for unresolved selectors.
func selectorName(sel *ast.SelectorExpr) string {
	if id, ok := sel.X.(*ast.Ident); ok {
		return id.Name + "." + sel.Sel.Name
	}
	return sel.Sel.Name
}

// callModuleName maps known constructors to module names.
func callModuleName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		switch fn.Sel.Name {
		case "HealthModule", "HealthModuleWithChecks":
			return "health"
		case "DynamicModule":
			if len(call.Args) > 0 {
				return stringLit(call.Args[0])
			}
		}
	case *ast.Ident:
		switch fn.Name {
		case "HealthModule", "HealthModuleWithChecks":
			return "health"
		case "DynamicModule":
			if len(call.Args) > 0 {
				return stringLit(call.Args[0])
			}
		}
	}
	return ""
}

// returnsModuleDef reports whether a function returns ModuleDef or
// *ModuleDef.
func returnsModuleDef(results *ast.FieldList) bool {
	if results == nil {
		return false
	}
	for _, f := range results.List {
		if isModuleDefType(f.Type) {
			return true
		}
	}
	return false
}

// firstReturnedModule finds the first ModuleDef literal returned by a
// function body (dynamic modules with loops are intentionally skipped).
func firstReturnedModule(body *ast.BlockStmt) (string, bool) {
	if body == nil {
		return "", false
	}
	var name string
	var global bool
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		ret, ok := n.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		for _, r := range ret.Results {
			lit := moduleDefLit(r)
			if lit == nil {
				continue
			}
			if n, g := moduleDefMeta(lit); n != "" {
				name, global, found = n, g, true
				return false
			}
		}
		return true
	})
	return name, global
}

// stringLit returns the unquoted value of a string literal, or "".
func stringLit(expr ast.Expr) string {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return ""
	}
	return s
}

// identTrue reports whether expr is the identifier true.
func identTrue(expr ast.Expr) bool {
	id, ok := expr.(*ast.Ident)
	return ok && id.Name == "true"
}

// containsString reports whether ss contains s.
func containsString(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// RenderGraphMermaid renders the graph as a Mermaid flowchart.
func RenderGraphMermaid(res GraphResult) string {
	var b strings.Builder
	b.WriteString("graph TD\n")
	for _, m := range res.Modules {
		fmt.Fprintf(&b, "  %s[\"%s\"]\n", mermaidID(m.Name), m.Name)
	}
	for _, e := range res.Edges {
		fmt.Fprintf(&b, "  %s --> %s\n", mermaidID(e.From), mermaidID(e.To))
	}
	return b.String()
}

// mermaidID makes a safe Mermaid node id.
func mermaidID(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "module"
	}
	return b.String()
}

// RenderGraphDOT renders the graph as Graphviz DOT.
func RenderGraphDOT(res GraphResult) string {
	var b strings.Builder
	b.WriteString("digraph grove {\n  rankdir=LR;\n")
	for _, m := range res.Modules {
		fmt.Fprintf(&b, "  %q;\n", m.Name)
	}
	for _, e := range res.Edges {
		fmt.Fprintf(&b, "  %q -> %q;\n", e.From, e.To)
	}
	b.WriteString("}\n")
	return b.String()
}

// RenderGraphJSON renders the graph as indented JSON.
func RenderGraphJSON(res GraphResult) (string, error) {
	out, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out) + "\n", nil
}
