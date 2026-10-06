package grove

import (
	"fmt"

	"github.com/brandonbert8/grove/packages/router"
)

// EndpointOption customizes an Endpoint built by GET, POST, PUT, DELETE,
// or PATCH: the compile-time, reflection-free answer to stacking
// decorators like @ApiOperation() + @UseGuards() on one route.
//
// Options compose freely with plain router.Middleware values:
//
//	grove.GET("", ctrl.List,
//	    grove.WithSummary("List users"),
//	    grove.WithTags("users"),
//	    grove.WithResponses(map[int]string{200: "users"}),
//	    grove.Use(authGuard),
//	)
type EndpointOption func(*Endpoint)

// Use attaches route-scoped middleware (guards, interceptors) to an
// Endpoint built by GET/POST/... Prefer it over raw middleware values
// when combining with docs options:
//
//	grove.POST("", ctrl.Create, grove.Use(adminOnly), grove.WithSummary("Create user"))
func Use(mw ...router.Middleware) EndpointOption {
	return func(e *Endpoint) {
		for _, m := range mw {
			if m != nil {
				e.Middleware = append(e.Middleware, m)
			}
		}
	}
}

// WithMiddleware attaches route-scoped middleware. It is an alias of
// Use, kept for readers who think in middleware terms.
func WithMiddleware(mw ...router.Middleware) EndpointOption { return Use(mw...) }

// WithGuards attaches CanActivate guards (wrapped with UseGuard
// automatically), the Go equivalent of NestJS @UseGuards(RolesGuard):
//
//	grove.POST("", ctrl.Create, grove.WithGuards(adminGuard))
func WithGuards(guards ...CanActivate) EndpointOption {
	return func(e *Endpoint) {
		for _, g := range guards {
			if g != nil {
				e.Middleware = append(e.Middleware, UseGuard(g))
			}
		}
	}
}

// WithSummary sets the short operation title for docs
// (the @ApiOperation summary equivalent).
func WithSummary(s string) EndpointOption {
	return func(e *Endpoint) { e.Summary = s }
}

// WithDescription sets long-form operation docs.
func WithDescription(s string) EndpointOption {
	return func(e *Endpoint) { e.Description = s }
}

// WithTags groups the operation for docs (merged with controller tags,
// the @ApiTags equivalent).
func WithTags(tags ...string) EndpointOption {
	return func(e *Endpoint) { e.Tags = append(e.Tags, tags...) }
}

// WithDeprecated marks the operation deprecated in docs.
func WithDeprecated() EndpointOption {
	return func(e *Endpoint) { e.Deprecated = true }
}

// WithQuery declares query parameters for docs
// (the @ApiQuery equivalent).
func WithQuery(q ...QueryDef) EndpointOption {
	return func(e *Endpoint) { e.Query = append(e.Query, q...) }
}

// QueryParam declares one query parameter inline for WithQuery:
//
//	grove.WithQuery(grove.QueryParam("page", "Page number", false))
func QueryParam(name, description string, required bool) QueryDef {
	return QueryDef{Name: name, Description: description, Required: required}
}

// WithResponses maps status codes to descriptions for docs
// (the @ApiResponse equivalent). Maps merge across options.
func WithResponses(r map[int]string) EndpointOption {
	return func(e *Endpoint) {
		if e.Responses == nil && len(r) > 0 {
			e.Responses = map[int]string{}
		}
		for k, v := range r {
			e.Responses[k] = v
		}
	}
}

// WithSecurity lists required security schemes for docs
// (e.g. grove.WithSecurity("bearerAuth"), the @ApiBearerAuth
// equivalent). Names merge with controller security.
func WithSecurity(schemes ...string) EndpointOption {
	return func(e *Endpoint) { e.Security = append(e.Security, schemes...) }
}

// WithOperation sets summary + responses in one call to cut CRUD
// boilerplate (the @ApiOperation + @ApiResponse combo):
//
//	grove.GET("", ctrl.List, grove.WithOperation("List users", map[int]string{200: "users"}))
func WithOperation(summary string, responses map[int]string) EndpointOption {
	return func(e *Endpoint) {
		e.Summary = summary
		WithResponses(responses)(e)
	}
}

// WithBodySchema attaches a request-body JSON Schema for docs
// (the @ApiBody equivalent). Build it with openapi.SchemaFor[T]:
//
//	grove.POST("", h.Create, grove.WithBodySchema(openapi.SchemaFor[CreateUser]()))
//
// Prefer openapi.WithBody[T](): same result in one generic call.
func WithBodySchema(schema map[string]any) EndpointOption {
	return func(e *Endpoint) { e.BodySchema = schema }
}

// WithResponseSchema attaches a per-status response JSON Schema for
// docs. Prefer openapi.WithResponse[T](code, desc): it sets both the
// description and the schema from the DTO type.
func WithResponseSchema(code int, schema map[string]any) EndpointOption {
	return func(e *Endpoint) {
		if e.ResponseSchemas == nil {
			e.ResponseSchemas = map[int]any{}
		}
		e.ResponseSchemas[code] = schema
	}
}

// buildEndpoint assembles an Endpoint from mixed route options. Accepted
// option types:
//
//	router.Middleware — appended to Endpoint.Middleware as-is (so code
//	    written before EndpointOption keeps compiling).
//	EndpointOption    — applied in order.
//	CanActivate       — wrapped with UseGuard and appended.
//
// Anything else panics: endpoint wiring must fail fast at startup,
// never per request.
func buildEndpoint(method, path string, h router.HandlerFunc, opts []any) Endpoint {
	e := Endpoint{Method: method, Path: path, Handler: h}
	for _, opt := range opts {
		switch o := opt.(type) {
		case nil:
			// Skip typed and untyped nils so conditional wiring
			// (grove.GET("", h, maybeGuard)) stays ergonomic.
		case router.Middleware:
			if o != nil {
				e.Middleware = append(e.Middleware, o)
			}
		case func(router.HandlerFunc) router.HandlerFunc:
			// Unnamed func literal with the middleware shape
			// (e.g. inline func(next router.HandlerFunc) {...}).
			if o != nil {
				e.Middleware = append(e.Middleware, router.Middleware(o))
			}
		case EndpointOption:
			if o != nil {
				o(&e)
			}
		case CanActivate:
			if o != nil {
				e.Middleware = append(e.Middleware, UseGuard(o))
			}
		default:
			panic(fmt.Sprintf("grove: invalid endpoint option %T for %s %s: want router.Middleware, grove.EndpointOption (e.g. grove.WithSummary(\"...\"), grove.Use(guard)), or grove.CanActivate", opt, method, path))
		}
	}
	return e
}
