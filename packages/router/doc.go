// Package router defines Grove's HTTP routing abstraction.
//
// The default implementation (DefaultRouter) is backed by Chi
// (github.com/go-chi/chi/v5) over net/http: Chi owns method+path
// matching, path parameters, and 404/405 dispatch, while Grove owns
// the HandlerFunc/Middleware/Context contract, route introspection,
// and JSON error mapping. Application code never imports Chi; use
// ctx.Request, ctx.ResponseWriter, and App.Handler as escape hatches.
package router
