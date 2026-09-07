package router

import (
	"net/http"
)

// HandlerFunc handles a request within a Context.
type HandlerFunc func(c Context) error

// Middleware wraps a HandlerFunc, adding behavior before and/or after it.
type Middleware func(next HandlerFunc) HandlerFunc

// RouteInfo describes a registered route for introspection and tests.
type RouteInfo struct {
	Method  string
	Path    string
	Handler HandlerFunc
}

// Router is Grove's HTTP routing abstraction.
//
// Implementations must support the four core verbs, global middleware,
// route groups, and http.Handler so the router can be mounted in any
// net/http server. Keep implementations small; advanced routing
// (parameter constraints, host routing) belongs in a replacement router,
// not in this interface.
type Router interface {
	http.Handler

	// Use appends global middleware applied to every route.
	Use(mw ...Middleware)

	// GET registers a GET handler for path.
	GET(path string, h HandlerFunc, mw ...Middleware)
	// POST registers a POST handler for path.
	POST(path string, h HandlerFunc, mw ...Middleware)
	// PUT registers a PUT handler for path.
	PUT(path string, h HandlerFunc, mw ...Middleware)
	// DELETE registers a DELETE handler for path.
	DELETE(path string, h HandlerFunc, mw ...Middleware)
	// PATCH registers a PATCH handler for path.
	PATCH(path string, h HandlerFunc, mw ...Middleware)

	// Handle registers a handler for an arbitrary method.
	Handle(method, path string, h HandlerFunc, mw ...Middleware)

	// Group returns a sub-router whose routes are prefixed with prefix.
	// Middleware passed here applies only to the group.
	Group(prefix string, mw ...Middleware) Router

	// Routes returns a snapshot of registered routes.
	Routes() []RouteInfo
}
