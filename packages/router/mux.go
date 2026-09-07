package router

import (
	"errors"
	"net/http"
	"sort"
	"strings"
	"sync"
)

// DefaultRouter is Grove's built-in Router.
//
// It is a thin layer over http.ServeMux using Go 1.22+ method+pattern
// routing (e.g. "GET /users/{id}"). Middleware runs as a composed chain
// around each handler. For anything fancier, implement Router and pass
// it to core via WithRouter.
type DefaultRouter struct {
	mu          sync.RWMutex
	mux         *http.ServeMux
	middlewares []Middleware
	prefix      string
	root        *DefaultRouter
	routes      []RouteInfo
}

// New returns a DefaultRouter ready for registration.
func New() *DefaultRouter {
	return &DefaultRouter{mux: http.NewServeMux()}
}

// rootRouter returns the shared root (self for roots, parent for groups).
func (r *DefaultRouter) rootRouter() *DefaultRouter {
	if r.root != nil {
		return r.root
	}
	return r
}

// Use appends global (or group-scoped) middleware.
func (r *DefaultRouter) Use(mw ...Middleware) {
	r.mu.Lock()
	defer r.mu.Unlock()
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
func (r *DefaultRouter) Handle(method, path string, h HandlerFunc, mw ...Middleware) {
	if h == nil {
		panic("router: handler must not be nil")
	}
	root := r.rootRouter()

	root.mu.Lock()
	chain := make([]Middleware, 0, len(root.middlewares)+len(r.middlewares)+len(mw))
	chain = append(chain, root.middlewares...)
	if r != root {
		chain = append(chain, r.middlewares...)
	}
	chain = append(chain, mw...)
	fullPath := joinPath(r.prefix, path)
	final := applyMiddleware(h, chain)
	pattern := method + " " + fullPath
	root.mux.HandleFunc(pattern, func(w http.ResponseWriter, req *http.Request) {
		ctx := NewContext(w, req)
		// Handler errors default to 500 JSON unless they carry their
		// own status via Statuser; handlers that already wrote a
		// response should return nil.
		if err := final(ctx); err != nil {
			writeHandlerError(ctx, err)
		}
	})
	root.routes = append(root.routes, RouteInfo{Method: method, Path: fullPath, Handler: final})
	root.mu.Unlock()
}

// Group returns a sub-router whose routes carry prefix.
// Group middleware runs after root middleware but before route middleware.
func (r *DefaultRouter) Group(prefix string, mw ...Middleware) Router {
	root := r.rootRouter()
	g := &DefaultRouter{
		mux:         root.mux,
		prefix:      joinPath(r.prefix, prefix),
		root:        root,
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

// ServeHTTP dispatches to the underlying mux.
func (r *DefaultRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.rootRouter().mux.ServeHTTP(w, req)
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
	// Best effort: if headers were already sent this is a no-op write.
	_ = ctx.JSON(status, body)
}

// applyMiddleware composes mw around h (last registered runs innermost).
func applyMiddleware(h HandlerFunc, mw []Middleware) HandlerFunc {
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}

// joinPath concatenates prefix and path with exactly one separator.
func joinPath(prefix, path string) string {
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
