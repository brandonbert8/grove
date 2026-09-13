package router

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	chi "github.com/go-chi/chi/v5"
)

// DefaultRouter is Grove's built-in Router, backed by Chi.
//
// Grove owns the framework-level contract (HandlerFunc, Middleware,
// Context, groups, route introspection, JSON error mapping) while Chi
// owns method+path matching, path parameters, and 404/405 dispatch.
// Controllers, guards, pipes, and middleware never import Chi; only
// this file (plus Context.Param) touches it.
//
// Composition rule: global middleware wraps the whole mux in
// ServeHTTP (covering routes, 404/405, and preflights); group and
// route middleware compose per route, outermost group first and route
// middleware innermost — without depending on Chi internals.
type DefaultRouter struct {
	mu          sync.RWMutex
	mux         *chi.Mux
	middlewares []Middleware
	prefix      string
	parent      *DefaultRouter
	routes      []RouteInfo
}

// New returns a DefaultRouter ready for registration.
//
// The underlying Chi mux answers unmatched paths and wrong methods
// with Grove-shaped JSON bodies ({"error","status"}) instead of the
// stdlib text/plain defaults, so JSON APIs never leak text errors.
func New() *DefaultRouter {
	m := chi.NewRouter()
	m.NotFound(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSONError(w, http.StatusNotFound, "not found")
	}))
	m.MethodNotAllowed(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}))
	return &DefaultRouter{mux: m}
}

// rootRouter returns the shared root (self for roots, top parent for groups).
func (r *DefaultRouter) rootRouter() *DefaultRouter {
	root := r
	for root.parent != nil {
		root = root.parent
	}
	return root
}

// Use appends middleware scoped to this router node: global when called
// on the root, group-scoped when called on a Group.
//
// Global middleware runs for every request — including 404s, 405s, and
// CORS preflights — because ServeHTTP applies it around the Chi mux.
// Group middleware composes per route at registration time.
func (r *DefaultRouter) Use(mw ...Middleware) {
	root := r.rootRouter()
	root.mu.Lock()
	defer root.mu.Unlock()
	r.middlewares = append(r.middlewares, mw...)
}

// GET registers a GET handler for path.
func (r *DefaultRouter) GET(path string, h HandlerFunc, mw ...Middleware) {
	r.Handle(http.MethodGet, path, h, mw...)
}

// POST registers a POST handler for path.
func (r *DefaultRouter) POST(path string, h HandlerFunc, mw ...Middleware) {
	r.Handle(http.MethodPost, path, h, mw...)
}

// PUT registers a PUT handler for path.
func (r *DefaultRouter) PUT(path string, h HandlerFunc, mw ...Middleware) {
	r.Handle(http.MethodPut, path, h, mw...)
}

// DELETE registers a DELETE handler for path.
func (r *DefaultRouter) DELETE(path string, h HandlerFunc, mw ...Middleware) {
	r.Handle(http.MethodDelete, path, h, mw...)
}

// PATCH registers a PATCH handler for path.
func (r *DefaultRouter) PATCH(path string, h HandlerFunc, mw ...Middleware) {
	r.Handle(http.MethodPatch, path, h, mw...)
}

// Handle registers a handler for an arbitrary method and path.
//
// An empty path mounts exactly at the group prefix (GET "" under
// "/users" serves GET /users). Method is case-insensitive; "*" (or
// empty) matches all methods via Chi's Handle. Other methods delegate
// fully to Chi's matching, except one stdlib parity rule: GET routes
// also answer HEAD (body stripped by net/http), mirroring ServeMux.
// Explicit HEAD registrations win over the automatic mirror.
// Re-registering an identical method+path replaces the route
// (last wins, like ServeMux) instead of shadowing it.
func (r *DefaultRouter) Handle(method, path string, h HandlerFunc, mw ...Middleware) {
	if h == nil {
		panic("router: handler must not be nil")
	}
	root := r.rootRouter()

	// Deferred: chi panics on invalid patterns, and any future panic
	// below must never poison the mutex into a process-wide deadlock.
	root.mu.Lock()
	defer root.mu.Unlock()
	// NB: root (global) middleware is NOT composed here — ServeHTTP
	// applies it around the whole mux so it also covers 404/405 and
	// preflights. chainLocked collects group ancestry only.
	chain := r.chainLocked()
	chain = append(chain, mw...)
	fullPath := normalizePath(JoinPath(r.prefix, path))
	final := applyMiddleware(h, chain)
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := NewContext(w, req)
		// Handler errors default to 500 JSON unless they carry their
		// own status via Statuser; handlers that already wrote a
		// response should return nil.
		if err := final(ctx); err != nil {
			writeHandlerError(ctx, err)
		}
	})
	switch m := strings.ToUpper(strings.TrimSpace(method)); m {
	case "", "*":
		root.mux.Handle(fullPath, handler)
	default:
		if !supportedMethod(m) {
			panic(fmt.Sprintf("router: unsupported method %q", method))
		}
		root.mux.Method(m, fullPath, handler)
	}
	upsertRouteLocked(root, RouteInfo{Method: strings.ToUpper(strings.TrimSpace(method)), Path: fullPath, Handler: final})
	if strings.ToUpper(strings.TrimSpace(method)) == http.MethodGet {
		mirrorHeadLocked(root, fullPath, handler, final)
	}
}

// upsertRouteLocked records ri, replacing any identical method+path so
// Routes mirrors what serves (last wins). Callers hold root.mu.
func upsertRouteLocked(root *DefaultRouter, ri RouteInfo) {
	for i, prev := range root.routes {
		if prev.Method == ri.Method && prev.Path == ri.Path {
			root.routes[i] = ri
			return
		}
	}
	root.routes = append(root.routes, ri)
}

// mirrorHeadLocked answers HEAD on GET routes for ServeMux parity,
// unless the app registered HEAD explicitly (explicit wins).
// Callers hold root.mu.
func mirrorHeadLocked(root *DefaultRouter, fullPath string, handler http.Handler, final HandlerFunc) {
	for _, prev := range root.routes {
		if prev.Method == http.MethodHead && prev.Path == fullPath {
			return
		}
	}
	root.mux.Method(http.MethodHead, fullPath, handler)
	root.routes = append(root.routes, RouteInfo{Method: http.MethodHead, Path: fullPath, Handler: final})
}

// supportedMethods are the verbs chi (and net/http) route. Anything
// else is a programmer error: fail fast at registration with a Grove
// message instead of a chi panic deep inside the mux.
var supportedMethods = map[string]bool{
	http.MethodGet: true, http.MethodHead: true, http.MethodPost: true,
	http.MethodPut: true, http.MethodPatch: true, http.MethodDelete: true,
	http.MethodConnect: true, http.MethodOptions: true, http.MethodTrace: true,
}

// supportedMethod reports whether m (upper-cased, trimmed) routes.
func supportedMethod(m string) bool { return supportedMethods[m] }

// chainLocked collects middleware from the parent groups down to this
// node, excluding the root: global middleware runs in ServeHTTP, not
// per route, so it covers unmatched paths too. Callers must hold root.mu.
func (r *DefaultRouter) chainLocked() []Middleware {
	var nodes []*DefaultRouter
	for n := r; n != nil && n.parent != nil; n = n.parent {
		nodes = append(nodes, n)
	}
	var out []Middleware
	for i := len(nodes) - 1; i >= 0; i-- {
		out = append(out, nodes[i].middlewares...)
	}
	return out
}

// Group returns a sub-router whose routes carry prefix.
// Group middleware runs after root middleware but before route
// middleware; nested groups compose outermost-first.
func (r *DefaultRouter) Group(prefix string, mw ...Middleware) Router {
	root := r.rootRouter()
	root.mu.Lock()
	defer root.mu.Unlock()
	g := &DefaultRouter{
		prefix:      JoinPath(r.prefix, prefix),
		parent:      r,
		middlewares: append([]Middleware{}, mw...),
	}
	return g
}

// Routes returns a snapshot of registered routes sorted by path.
func (r *DefaultRouter) Routes() []RouteInfo {
	root := r.rootRouter()
	root.mu.RLock()
	defer root.mu.RUnlock()
	out := append([]RouteInfo(nil), root.routes...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path == out[j].Path {
			return out[i].Method < out[j].Method
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// ServeHTTP dispatches to the underlying Chi mux wrapped in global
// middleware, so Recovery/Logging/CORS observe every request —
// matched routes, 404s, 405s, and preflights alike.
func (r *DefaultRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	root := r.rootRouter()
	root.mu.RLock()
	global := append([]Middleware(nil), root.middlewares...)
	root.mu.RUnlock()
	ctx := NewContext(w, req)
	final := applyMiddleware(func(c Context) error {
		root.mux.ServeHTTP(c.ResponseWriter(), c.Request())
		return nil
	}, global)
	if err := final(ctx); err != nil {
		writeHandlerError(ctx, err)
	}
}

// Statuser is implemented by errors that carry their own HTTP status,
// e.g. grove.HttpError. The router maps them to that status instead of
// a generic 500, which is the single integration point for exception
// filters: return a typed error from any handler.
type Statuser interface {
	error
	StatusCode() int
}

func writeHandlerError(ctx Context, err error) {
	status := http.StatusInternalServerError
	var se Statuser
	if errors.As(err, &se) && se.StatusCode() >= 400 && se.StatusCode() <= 599 {
		status = se.StatusCode()
	}
	body := map[string]any{"error": err.Error(), "status": status}
	// Preserve machine-readable context (e.g. pipes validation
	// failures carried by *grove.HttpError.Details) so clients can
	// render field errors without parsing messages.
	if d, ok := detaill(err); ok {
		body["details"] = d
	}
	// Best effort: if headers were already sent this is a no-op write.
	_ = ctx.JSON(status, body)
}

// detaill unwraps err to an optional machine-readable payload (e.g.
// *grove.HttpError.Details). Router is a leaf package and cannot import
// core, so the contract is structural: any Statuser exposing
// ErrorDetails provides it. See grove.HttpError.ErrorDetails.
func detaill(err error) (any, bool) {
	var de interface {
		Statuser
		ErrorDetails() any
	}
	if errors.As(err, &de) {
		if d := de.ErrorDetails(); d != nil {
			return d, true
		}
	}
	return nil, false
}

// writeJSONError renders infrastructure errors (404/405) in the same
// Grove JSON shape as handler errors.
func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": msg, "status": status})
}

// applyMiddleware composes mw around h (last registered runs innermost).
func applyMiddleware(h HandlerFunc, mw []Middleware) HandlerFunc {
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}

// FromHTTP adapts a standard net/http middleware into Grove middleware.
//
// The std middleware sees the live request writer (including writers
// swapped in by outer Grove middleware such as Logging), so logging,
// compression, and tracing middleware from the std ecosystem keep
// working without importing Chi. Prefer native Grove middleware for
// handlers that need ctx.Param or typed errors.
func FromHTTP(mw func(http.Handler) http.Handler) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(c Context) error {
			var hErr error
			inner := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				hErr = next(NewContext(w, req))
			})
			mw(inner).ServeHTTP(c.ResponseWriter(), c.Request())
			return hErr
		}
	}
}

// normalizePath guarantees a Chi-compatible pattern: non-empty with a
// leading slash. Empty mounts at "/" (callers join prefix first, so an
// empty endpoint path mounts exactly at its group prefix).
func normalizePath(p string) string {
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		return "/" + p
	}
	return p
}

// JoinPath concatenates prefix and path with exactly one separator.
//
// It is exported so modules, docs, and tooling compute the same full
// paths the router mounts (e.g. ControllerDef prefix + endpoint path).
func JoinPath(prefix, path string) string {
	if prefix == "" {
		return path
	}
	prefix = strings.TrimSuffix(prefix, "/")
	if path == "" {
		return prefix
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return prefix + path
}
