package grove

import (
	"context"

	"github.com/brandonbert8/grove/packages/router"
)

// This file reserves Grove's extension points for Phase 2 and beyond.
// Each interface documents where its future implementation should
// integrate. Nothing here is wired into App yet on purpose: the hooks
// exist so later packages can build against stable contracts.

type ctxKey string

// Controller groups route handlers for a resource.
//
// Future home: packages/controller (or per-module controllers).
// Integration: a Controller registers its routes in Module.Register via
// app.Router, typically under app.Router.Group("/prefix").
type Controller interface {
	// Prefix returns the route group prefix (e.g. "/users").
	Prefix() string
	// Routes registers handlers on the given router group.
	Routes(g router.Router)
}

// Service is a marker for business-logic components.
//
// Future home: per-module service structs plus packages/di providers.
// Integration: constructors are registered in the DI container
// (transient for stateless services, singleton for shared state) and
// resolved by controllers/handlers.
type Service interface {
	// ServiceName identifies the service for logs and diagnostics.
	ServiceName() string
}

// Guard decides whether a request may proceed (authN/authZ).
//
// Implemented in Phase 2: implement grove.CanActivate (or use
// grove.GuardFunc), adapt with grove.UseGuard, and attach per-route,
// per-group, or globally. packages/auth provides JWT verification plus
// AuthGuard and RequireRole.
type Guard interface {
	router.Middleware
	// GuardName identifies the guard for logs and diagnostics.
	GuardName() string
}

// Interceptors (logging, tracing, metrics, response mapping) are
// implemented in Phase 2: see pipeline.go. Plain router.Middleware values
// already are interceptors; grove.InterceptorFunc adapts wrapping
// functions and grove.Chain composes them for reuse across routes.

// Pipe validates or transforms a single value (params, query, bodies).
//
// Implemented in Phase 2: see packages/pipes (ValidateBody, BindQuery,
// Validate). Call pipes explicitly inside handlers; declarative binding
// via struct tags on handlers remains a Phase 3 codegen task.
type Pipe[T any] interface {
	// Transform validates value and returns the coerced result.
	Transform(ctx context.Context, value T) (T, error)
}

// Plugin extends the framework at startup (tracing backends, admin
// panels, dev tools).
//
// Future home: packages/plugins.
// Integration: applied in main before app.Run, e.g.
// grove.ApplyPlugins(app, tracing.Plugin(), metrics.Plugin()).
type Plugin interface {
	// PluginName identifies the plugin.
	PluginName() string
	// Apply wires the plugin into the application.
	Apply(app *App) error
}

// ApplyPlugins applies each plugin in order, aborting on first error.
func ApplyPlugins(app *App, plugins ...Plugin) error {
	for _, p := range plugins {
		if p == nil {
			continue
		}
		if err := p.Apply(app); err != nil {
			return err
		}
		app.Logger.Info("plugin applied", "plugin", p.PluginName())
	}
	return nil
}

// The following future transports each get a dedicated package that
// composes with App without changing core:
//
//	WebSockets   packages/ws    — hubs and handlers mounted beside Router.
//	gRPC         packages/grpc  — service descriptors registered from modules.
//	GraphQL      packages/graphql — schema + resolvers under a route group.
//	Cron Jobs    packages/cron  — scheduled funcs started/stopped with App.
//	Queue Workers packages/queue — producers/consumers sharing the Container.
var _ = ctxKey("grove.future")
