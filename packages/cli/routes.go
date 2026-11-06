package cli

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// RouteEntry is one source-level HTTP route.
type RouteEntry struct {
	Method  string `json:"method"`
	Path    string `json:"path"`
	Handler string `json:"handler,omitempty"`
	Summary string `json:"summary,omitempty"`
	Version string `json:"version,omitempty"`
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
}

// RoutesResult is the `grove routes` report.
type RoutesResult struct {
	Routes  []RouteEntry `json:"routes"`
	Notices []string     `json:"notices,omitempty"`
}

var routeMethods = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "DELETE": true,
	"PATCH": true, "HEAD": true, "OPTIONS": true,
}

// Routes scans a Grove project for ControllerDef literals and lists their
// endpoints. It is intentionally source-level and conservative: route
// tables built dynamically (loops, maps, helpers returning variables) may
// be missing. Use App.ModuleGraph and app.Docs for the runtime view.
func Routes(dir string) (RoutesResult, error) {
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		return RoutesResult{}, fmt.Errorf("no go.mod in %s", dir)
	}

	fset := token.NewFileSet()
	var res RoutesResult
	controllerCount := 0
	skipped := 0
	skippedPrefixes := 0

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if skipGraphDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return nil
		}
		rel, rerr := filepath.Rel(dir, path)
		if rerr != nil {
			rel = path
		}
		addController := func(lit *ast.CompositeLit) {
			controllerCount++
			prefix := ""
			if expr, ok := compositeField(lit, "Prefix"); ok {
				value, literal := stringExpr(expr)
				if !literal {
					// HealthModule("") and friends compute the prefix at
					// runtime; do not report a misleading "/" route.
					skippedPrefixes++
					return
				}
				prefix = value
			}
			// ControllerDef.Version applies to every endpoint unless the
			// endpoint overrides it (NestJS @Controller({version})).
			controllerVersion := compositeString(lit, "Version")
			endpoints := compositeComposite(lit, "Endpoints")
			if endpoints == nil {
				return
			}
			for _, elt := range endpoints.Elts {
				entry, ok := routeFromEndpoint(elt)
				if !ok {
					skipped++
					continue
				}
				if entry.Version == "" {
					entry.Version = controllerVersion
				}
				entry.Path = joinRoutePath(prefix, entry.Path)
				entry.File = filepath.ToSlash(rel)
				entry.Line = fset.Position(elt.Pos()).Line
				res.Routes = append(res.Routes, entry)
			}
		}
		ast.Inspect(src, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			// []grove.ControllerDef{{...}} elides the element type, so
			// detect the slice and handle its elements as controllers.
			if arr, ok := lit.Type.(*ast.ArrayType); ok && isControllerDefType(arr.Elt) {
				for _, elt := range lit.Elts {
					if cl, ok := elt.(*ast.CompositeLit); ok {
						addController(cl)
					}
				}
				return false
			}
			if isControllerDefType(lit.Type) {
				addController(lit)
			}
			return true
		})
		return nil
	})
	if err != nil {
		return RoutesResult{}, err
	}

	sort.Slice(res.Routes, func(i, j int) bool {
		if res.Routes[i].Path != res.Routes[j].Path {
			return res.Routes[i].Path < res.Routes[j].Path
		}
		if res.Routes[i].Method != res.Routes[j].Method {
			return res.Routes[i].Method < res.Routes[j].Method
		}
		return res.Routes[i].Line < res.Routes[j].Line
	})
	if controllerCount == 0 {
		res.Notices = append(res.Notices, "no *grove.ControllerDef literals found (routes built dynamically are not visible)")
	}
	if skippedPrefixes > 0 {
		res.Notices = append(res.Notices, fmt.Sprintf("%d controller(s) with non-literal prefixes skipped", skippedPrefixes))
	}
	if skipped > 0 {
		res.Notices = append(res.Notices, fmt.Sprintf("%d endpoint(s) with non-literal paths skipped", skipped))
	}
	return res, nil
}

// routeFromEndpoint extracts a route from a grove.GET/POST/... call or an
// Endpoint composite literal.
func routeFromEndpoint(expr ast.Expr) (RouteEntry, bool) {
	switch e := expr.(type) {
	case *ast.CallExpr:
		method, ok := endpointMethod(e.Fun)
		if !ok || len(e.Args) == 0 {
			return RouteEntry{}, false
		}
		path, ok := stringExpr(e.Args[0])
		if !ok {
			return RouteEntry{}, false
		}
		entry := RouteEntry{Method: method, Path: path}
		if len(e.Args) > 1 {
			entry.Handler = handlerName(e.Args[1])
		}
		for _, arg := range e.Args[1:] {
			readEndpointOption(arg, &entry)
		}
		return entry, true
	case *ast.CompositeLit:
		if e.Type != nil && !isEndpointType(e.Type) {
			return RouteEntry{}, false
		}
		methodExpr, ok := compositeField(e, "Method")
		if !ok {
			return RouteEntry{}, false
		}
		method, ok := stringExpr(methodExpr)
		if !ok {
			return RouteEntry{}, false
		}
		method = strings.ToUpper(method)
		// An empty path is valid: the endpoint mounts at the controller
		// prefix. A non-literal path is not visible to this scanner.
		path := ""
		if pathExpr, ok := compositeField(e, "Path"); ok {
			path, ok = stringExpr(pathExpr)
			if !ok {
				return RouteEntry{}, false
			}
		}
		if !routeMethods[method] {
			return RouteEntry{}, false
		}
		return RouteEntry{
			Method:  method,
			Path:    path,
			Summary: compositeString(e, "Summary"),
			Version: compositeString(e, "Version"),
		}, true
	}
	return RouteEntry{}, false
}

// endpointMethod maps grove.GET / GET (and the other verbs) to an HTTP
// method. Non-route helpers such as grove.HandleBody are not endpoints.
func endpointMethod(fun ast.Expr) (string, bool) {
	name := ""
	switch f := fun.(type) {
	case *ast.Ident:
		name = f.Name
	case *ast.SelectorExpr:
		if id, ok := f.X.(*ast.Ident); !ok || (id.Name != "grove" && id.Name != "core") {
			return "", false
		}
		name = f.Sel.Name
	default:
		return "", false
	}
	name = strings.ToUpper(name)
	if !routeMethods[name] {
		return "", false
	}
	return name, true
}

// readEndpointOption extracts docs/version metadata from grove.With* args.
func readEndpointOption(expr ast.Expr, entry *RouteEntry) {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil {
		return
	}
	name := sel.Sel.Name
	if len(call.Args) == 0 {
		return
	}
	value, ok := stringExpr(call.Args[0])
	if !ok {
		return
	}
	switch name {
	case "WithSummary":
		entry.Summary = value
	case "WithVersion":
		entry.Version = value
	}
}

// handlerName renders the handler expression for the table: ctrl.List,
// pkg.Handler, HandleBody's inner handler, or "func" for closures.
func handlerName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		if id, ok := e.X.(*ast.Ident); ok {
			return id.Name + "." + e.Sel.Name
		}
	case *ast.Ident:
		return e.Name
	case *ast.CallExpr:
		if len(e.Args) > 0 {
			return handlerName(e.Args[0])
		}
	case *ast.FuncLit:
		return "func"
	}
	return ""
}

func isControllerDefType(expr ast.Expr) bool {
	switch t := expr.(type) {
	case *ast.SelectorExpr:
		return t.Sel != nil && t.Sel.Name == "ControllerDef"
	case *ast.Ident:
		return t.Name == "ControllerDef"
	case *ast.StarExpr:
		return isControllerDefType(t.X)
	}
	return false
}

func isEndpointType(expr ast.Expr) bool {
	switch t := expr.(type) {
	case *ast.SelectorExpr:
		return t.Sel != nil && t.Sel.Name == "Endpoint"
	case *ast.Ident:
		return t.Name == "Endpoint"
	case *ast.StarExpr:
		return isEndpointType(t.X)
	}
	return false
}

func compositeField(lit *ast.CompositeLit, field string) (ast.Expr, bool) {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != field {
			continue
		}
		return kv.Value, true
	}
	return nil, false
}

func compositeString(lit *ast.CompositeLit, field string) string {
	expr, ok := compositeField(lit, field)
	if !ok {
		return ""
	}
	value, _ := stringExpr(expr)
	return value
}

func compositeComposite(lit *ast.CompositeLit, field string) *ast.CompositeLit {
	expr, ok := compositeField(lit, field)
	if !ok {
		return nil
	}
	cl, _ := expr.(*ast.CompositeLit)
	return cl
}

func stringExpr(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return s, true
}

func joinRoutePath(prefix, path string) string {
	prefix = strings.TrimSpace(prefix)
	path = strings.TrimSpace(path)
	if prefix == "" {
		prefix = "/"
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	if path == "" {
		return prefix
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if prefix == "/" {
		return path
	}
	return strings.TrimSuffix(prefix, "/") + path
}

// newRoutesCommand builds `grove routes`: list source-level HTTP routes.
func newRoutesCommand(g *globalOpts) *cobra.Command {
	var format, method, prefix string
	cmd := &cobra.Command{
		Use:   "routes",
		Short: "List HTTP routes from source",
		Long: "List routes declared in *grove.ControllerDef literals.\n" +
			"Source-level and conservative: route tables built dynamically may be\n" +
			"missing. Use app.Docs()/App.ModuleGraph() for the runtime view.",
		Example: "  grove routes\n  grove routes --method GET\n  grove routes --format json",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			r := diagnostics(cmd, g)
			cwd, err := os.Getwd()
			if err != nil {
				return fail(r, 1, "Could not list routes", err.Error(), "", err)
			}
			res, err := Routes(cwd)
			if err != nil {
				return fail(r, 1, "Could not list routes", err.Error(), "Run inside a Grove project (a directory with go.mod).", err)
			}
			res.Routes = filterRoutes(res.Routes, method, prefix)
			if res.Routes == nil {
				res.Routes = []RouteEntry{}
			}
			out := cmd.OutOrStdout()
			format = strings.ToLower(strings.TrimSpace(format))
			for _, n := range res.Notices {
				if format == "json" {
					// Notices travel inside the JSON payload so stdout
					// stays valid for tooling.
					continue
				}
				r.Warning(n)
			}
			switch format {
			case "", "table":
				writeRoutesTable(out, res.Routes)
			case "json":
				data, err := json.MarshalIndent(res, "", "  ")
				if err != nil {
					return fail(r, 1, "Could not render routes", err.Error(), "", err)
				}
				fmt.Fprintln(out, string(data))
			default:
				r.Failure("Unknown routes format", fmt.Sprintf("format %q is not supported", format), "Use --format table|json.")
				return &exitError{code: 2, err: &UsageError{Msg: "unknown routes format"}}
			}
			if len(res.Routes) == 0 && format != "json" {
				if strings.TrimSpace(method) != "" || strings.TrimSpace(prefix) != "" {
					r.Muted("No routes matched the current filters.")
					r.Muted("Try: grove routes")
				} else {
					r.Muted("No routes found.")
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "table", "output format: table|json")
	cmd.Flags().StringVar(&method, "method", "", "filter by HTTP method (GET, POST, ...)")
	cmd.Flags().StringVar(&prefix, "prefix", "", "filter by path prefix")
	return cmd
}

func filterRoutes(routes []RouteEntry, method, prefix string) []RouteEntry {
	method = strings.ToUpper(strings.TrimSpace(method))
	prefix = strings.TrimSpace(prefix)
	if method == "" && prefix == "" {
		return routes
	}
	out := make([]RouteEntry, 0, len(routes))
	for _, rt := range routes {
		if method != "" && rt.Method != method {
			continue
		}
		if prefix != "" && !strings.HasPrefix(rt.Path, prefix) {
			continue
		}
		out = append(out, rt)
	}
	return out
}

func writeRoutesTable(out io.Writer, routes []RouteEntry) {
	if len(routes) == 0 {
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "METHOD\tPATH\tHANDLER\tSUMMARY\tSOURCE")
	for _, rt := range routes {
		source := rt.File
		if rt.Line > 0 {
			source = fmt.Sprintf("%s:%d", rt.File, rt.Line)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", rt.Method, rt.Path, rt.Handler, rt.Summary, source)
	}
	_ = tw.Flush()
}
