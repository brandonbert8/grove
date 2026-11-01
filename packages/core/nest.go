package grove

import (
	"context"
	"fmt"

	"github.com/brandonbert8/grove/packages/di"
	"github.com/brandonbert8/grove/packages/router"
)

// Provider describes one DI registration owned by a module, the Go
// equivalent of a NestJS @Injectable listed in `providers`.
//
// Name defaults to di.KeyFor[T] when built with Provide/ProvideValue.
// Build receives the container so constructors resolve their own
// dependencies explicitly. Request-scoped providers use BuildScoped
// (see ProvideRequest) and resolve once per request Scope.
type Provider struct {
	// Name is the container key. Empty means the provider must be
	// constructed with Provide/ProvideValue, which fill it in.
	Name string
	// Lifetime is di.Singleton, di.Transient, or di.RequestScoped.
	Lifetime di.Lifetime
	// Build constructs the value. For singletons it runs once and the
	// result is cached by the container.
	Build func(c *di.Container) (any, error)
	// BuildScoped constructs a request-scoped value. Only set when
	// Lifetime == di.RequestScoped (via ProvideRequest).
	BuildScoped func(c *di.Container, ctx context.Context) (any, error)
}

// Provide builds a Provider for type T using a constructor function.
func Provide[T any](lifetime di.Lifetime, build func(c *di.Container) (T, error)) Provider {
	return Provider{
		Name:     di.KeyFor[T](),
		Lifetime: lifetime,
		Build: func(c *di.Container) (any, error) {
			v, err := build(c)
			if err != nil {
				return nil, err
			}
			return v, nil
		},
	}
}

// ProvideNamed builds a Provider under an explicit name instead of the
// default di.KeyFor[T] key. Use it when a module exposes the same type
// under multiple keys or when callers resolve by string name:
//
//	grove.ProvideNamed("readDB", di.Singleton, func(c *di.Container) (any, error) {
//	    return sql.Open("postgres", c.MustResolve("readDSN").(string)), nil
//	})
func ProvideNamed(name string, lifetime di.Lifetime, build func(c *di.Container) (any, error)) Provider {
	if name == "" {
		panic("grove: ProvideNamed requires a non-empty name")
	}
	return Provider{
		Name:     name,
		Lifetime: lifetime,
		Build:    build,
	}
}

// ProvideValue builds a singleton Provider for an existing value.
func ProvideValue[T any](value T) Provider {
	return Provider{
		Name:     di.KeyFor[T](),
		Lifetime: di.Singleton,
		Build:    func(c *di.Container) (any, error) { return value, nil },
	}
}

// Provide0 builds a Provider from a zero-argument constructor, the common
// case for services without dependencies:
//
//	grove.Provide0(di.Singleton, NewUsersService)
func Provide0[T any](lifetime di.Lifetime, build func() T) Provider {
	return Provide(lifetime, func(*di.Container) (T, error) {
		return build(), nil
	})
}

// Provide0Named builds a Provider from a zero-argument constructor under
// an explicit name (see ProvideNamed).
func Provide0Named(name string, lifetime di.Lifetime, build func() any) Provider {
	return ProvideNamed(name, lifetime, func(*di.Container) (any, error) {
		return build(), nil
	})
}

// ProvideRequest builds a request-scoped Provider for type T (NestJS
// Scope.REQUEST equivalent). The factory runs at most once per request
// Scope created by grove.RequestScope middleware and resolves with
// grove.InjectRequest:
//
//	grove.ProvideRequest(func(c *di.Container, ctx context.Context) (*ReqCtx, error) {
//	    return NewReqCtx(ctx), nil
//	})
//
// Values must not retain the context, request, or writer.
func ProvideRequest[T any](build func(c *di.Container, ctx context.Context) (T, error)) Provider {
	return Provider{
		Name:     di.KeyFor[T](),
		Lifetime: di.RequestScoped,
		BuildScoped: func(c *di.Container, ctx context.Context) (any, error) {
			v, err := build(c, ctx)
			if err != nil {
				return nil, err
			}
			return v, nil
		},
	}
}

// ProvideRequestNamed builds a request-scoped Provider under an explicit
// name (see ProvideNamed).
func ProvideRequestNamed(name string, build func(c *di.Container, ctx context.Context) (any, error)) Provider {
	if name == "" {
		panic("grove: ProvideRequestNamed requires a non-empty name")
	}
	return Provider{
		Name:        name,
		Lifetime:    di.RequestScoped,
		BuildScoped: build,
	}
}

// Endpoint is one route in a controller's table: the compile-time,
// reflection-free answer to decorators like @Get(). Method is an HTTP
// verb ("GET", "POST", ...), Path is relative to the controller prefix,
// and Middleware holds route-scoped guards, interceptors, and pipes.
//
// Summary, Description, Tags, and Deprecated are documentation metadata
// (the @ApiOperation/@ApiTags equivalent): they feed App.Docs and the
// openapi package, and never affect routing. Query, Responses, and
// Security are the @ApiQuery/@ApiResponse/@ApiBearerAuth equivalent.
type Endpoint struct {
	Method  string
	Path    string
	Handler router.HandlerFunc
	// Version overrides the controller/app version for this endpoint
	// (NestJS @Version). Empty uses ControllerDef.Version, then the
	// app default.
	Version    string
	Middleware []router.Middleware
	// Summary is a short operation title for docs.
	Summary string
	// Description is long-form operation docs.
	Description string
	// Tags groups the operation (merged with controller tags).
	Tags []string
	// Deprecated marks the operation deprecated in docs.
	Deprecated bool
	// Query declares query parameters for docs.
	Query []QueryDef
	// Responses maps status codes to descriptions for docs.
	// Unlisted operations get a default response.
	Responses map[int]string
	// Security lists required security schemes for docs
	// (e.g. []string{"bearerAuth"}).
	Security []string
	// BodySchema is an optional request-body JSON Schema
	// (use grove.WithBodySchema(openapi.SchemaFor[DTO]()) — openapi is
	// imported by the caller to avoid a core→openapi cycle).
	BodySchema map[string]any
	// ResponseSchemas optionally carries per-status response JSON Schemas
	// (use openapi.WithResponse[DTO](code, desc)).
	ResponseSchemas map[int]any
}

// QueryDef declares one query parameter for docs (the @ApiQuery
// equivalent): explicit metadata instead of handler inference, which
// Go cannot do without magic.
type QueryDef struct {
	// Name is the query parameter name.
	Name string
	// Description documents the parameter.
	Description string
	// Required marks a required parameter.
	Required bool
	// Type is the OpenAPI primitive: string (default), integer,
	// number, or boolean. Unknown values render as string.
	Type string
}

// GET builds an Endpoint for the GET verb.
//
// Extra args accept router.Middleware (route-scoped guards and
// interceptors, e.g. grove.POST("", h.Create, auth.RequireRole("admin"))),
// EndpointOption docs and guard helpers (grove.WithSummary(...),
// grove.Use(...)), and CanActivate guards (wrapped with UseGuard
// automatically). Unknown argument types panic with a Grove message so
// wiring mistakes fail fast at startup, never per request.
func GET(path string, h router.HandlerFunc, opts ...any) Endpoint {
	return buildEndpoint("GET", path, h, opts)
}

// POST builds an Endpoint for the POST verb. See GET for opts.
func POST(path string, h router.HandlerFunc, opts ...any) Endpoint {
	return buildEndpoint("POST", path, h, opts)
}

// PUT builds an Endpoint for the PUT verb. See GET for opts.
func PUT(path string, h router.HandlerFunc, opts ...any) Endpoint {
	return buildEndpoint("PUT", path, h, opts)
}

// DELETE builds an Endpoint for the DELETE verb. See GET for opts.
func DELETE(path string, h router.HandlerFunc, opts ...any) Endpoint {
	return buildEndpoint("DELETE", path, h, opts)
}

// PATCH builds an Endpoint for the PATCH verb. See GET for opts.
func PATCH(path string, h router.HandlerFunc, opts ...any) Endpoint {
	return buildEndpoint("PATCH", path, h, opts)
}

// ControllerDef groups endpoints under a prefix with optional shared
// middleware, the Go equivalent of a NestJS @Controller. The idiomatic
// shape is a XxxController struct (methods are the route handlers)
// plus one ControllerDef describing its prefix:
//
//	type UsersController struct{ svc *UsersService }
//	func NewUsersController(svc *UsersService) *UsersController {
//	    return &UsersController{svc: svc}
//	}
//	usersController := grove.ControllerDef{
//	    Prefix: "/users",
//	    Middleware: []router.Middleware{authGuard},
//	    Endpoints: []grove.Endpoint{
//	        grove.GET("/", ctrl.List, grove.WithSummary("List users")),
//	        grove.POST("/", ctrl.Create, grove.Use(adminOnly)),
//	    },
//	}
//
// (The older XxxHandler naming still works: it is the same pattern
// under the previous name. Prefer Controller for new code.)
type ControllerDef struct {
	// Prefix scopes every endpoint (e.g. "/users").
	Prefix string
	// Version applies to every endpoint without its own Version
	// (NestJS @Controller({version})). Empty uses the app default.
	Version string
	// Middleware runs for every endpoint in the controller.
	Middleware []router.Middleware
	// Endpoints is the route table.
	Endpoints []Endpoint
	// Tags groups every endpoint for docs (merged with endpoint tags).
	Tags []string
	// Security applies required schemes to every endpoint for docs
	// (merged with endpoint security, e.g. JWT-guarded controllers).
	Security []string
}

// register mounts the controller on r.
func (c ControllerDef) register(r router.Router) {
	g := r.Group(c.Prefix, c.Middleware...)
	for _, e := range c.Endpoints {
		g.Handle(e.Method, e.Path, e.Handler, e.Middleware...)
	}
}

// ModuleDef is Grove's NestJS-style module: imports for ordering,
// providers for DI, controllers for routes.
//
// Because Go has no decorators, a module is a plain struct. Convert it
// with AsModule and register it like any other Module:
//
//	users := &grove.ModuleDef{
//	    Name:    "users",
//	    Imports: []*grove.ModuleDef{auth},
//	    Providers: []grove.Provider{
//	        grove.Provide0(di.Singleton, NewUsersService),
//	    },
//	    Exports: []string{di.KeyFor[*UsersService]()}, // visible outside
//	    Controllers: []grove.ControllerDef{usersController},
//	}
//	app.MustRegister(users.AsModule())
//
// Imports are built depth-first with dedupe, so shared modules (auth,
// database) initialize once no matter how many modules import them.
// By default providers stay visible app-wide (v0.2 compatibility). Opt
// into NestJS-style encapsulation with ModuleDef.Isolated or
// App.EnableModuleIsolation: isolated modules can resolve their own
// providers, imported modules' Exports, and @Global exports only. Build
// receives a *ModuleContext so grove.Inject / grove.Wire enforce those
// rules. Set Global: true for @Global() equivalents (logger, config).
type ModuleDef struct {
	// Name identifies the module in logs and diagnostics.
	Name string
	// Imports lists modules that must be built first.
	Imports []*ModuleDef
	// Providers registers DI constructors/values.
	Providers []Provider
	// Exports lists provider names visible to importers. For legacy
	// (non-isolated) modules empty means all providers are public
	// (v0.2 compat). Isolated modules export only explicit entries.
	Exports []string
	// Global makes providers visible without importing (@Global).
	Global bool
	// Isolated opts this module into per-module provider visibility
	// (NestJS-style encapsulation). It is opt-in until v1: non-isolated
	// modules keep app-wide visibility for v0.2 compatibility.
	Isolated bool
	// Controllers mounts static route tables.
	//
	// Deprecated: use BuildControllers instead. It is the single, idiomatic
	// path for registering routes: the controller is resolved from the
	// container with constructor injection and returns its route table.
	// Keeping both fields invites mixing styles; BuildControllers is the
	// only supported form going forward.
	Controllers []ControllerDef
	// BuildControllers builds controllers after providers are registered,
	// so handlers resolve services from the container with constructor
	// injection. It is the single way to mount routes in a ModuleDef.
	//
	// Deprecated: prefer Build, which receives a ModuleContext and
	// enforces provider visibility for isolated modules.
	BuildControllers func(app *App) ([]ControllerDef, error)
	// Build wires the module after providers and legacy controllers are
	// registered. It receives the module-scoped context, so grove.Inject
	// and grove.Wire honor Exports/Imports when the module is isolated.
	Build func(m *ModuleContext) error
	// Hooks runs module-scoped lifecycle callbacks. Build may append
	// more through ModuleContext.OnModuleInit and friends; all hooks are
	// snapshotted when the module finishes registering.
	Hooks ModuleHooks
}

// ModuleHooks holds the NestJS lifecycle callbacks for one module.
// Init and Bootstrap run in registration order; Destroy and Shutdown run
// in reverse registration order. Hooks within one phase run in the
// order they were added; destroy/shutdown hooks run in reverse.
type ModuleHooks struct {
	Init      []func(context.Context) error
	Bootstrap []func(context.Context) error
	Destroy   []func(context.Context) error
	Shutdown  []func(context.Context) error
}

// DynamicModule builds a configured module instance
// (NestJS forRoot/forFeature equivalent):
//
//	func ForRoot(dsn string) *grove.ModuleDef {
//	    return grove.DynamicModule("db", []grove.Provider{
//	        grove.ProvideValue(dsn),
//	    }, nil)
//	}
func DynamicModule(name string, providers []Provider, imports []*ModuleDef) *ModuleDef {
	return &ModuleDef{Name: name, Providers: providers, Imports: imports}
}

// displayName returns the module name or a fallback for logs.
func (m *ModuleDef) displayName() string {
	if m.Name == "" {
		return "anonymous"
	}
	return m.Name
}

// AsModule adapts the definition to a Module for App.Register.
// (A separate adapter is used because Go forbids a struct field and
// method sharing the name `Name`.)
func (m *ModuleDef) AsModule() Module {
	return moduleDefAdapter{m: m}
}

// moduleDefAdapter implements Module for *ModuleDef.
type moduleDefAdapter struct{ m *ModuleDef }

// Name identifies the module.
func (a moduleDefAdapter) Name() string { return a.m.displayName() }

// Register builds the module into the app.
func (a moduleDefAdapter) Register(app *App) error { return a.m.Register(app) }

// Register builds imports, providers, and controllers into the app.
// It is idempotent per App: re-registering the same *ModuleDef pointer
// is a no-op (nested imports hit this path). Prefer registering through
// AsModule so lifecycle logs show the module name.
func (m *ModuleDef) Register(app *App) error {
	if m == nil {
		return fmt.Errorf("grove: cannot register nil module")
	}
	if app.builtModules == nil {
		app.builtModules = make(map[*ModuleDef]bool)
	}
	if app.globalModules == nil {
		app.globalModules = make(map[*ModuleDef]bool)
	}
	if app.builtModules[m] {
		return nil
	}
	// Mark early to prevent re-entry on cyclic imports, but if
	// registration fails (malformed provider/controller) the mark is
	// undone so a retry can register the module.
	moduleRegistered := false
	defer func() {
		if !moduleRegistered {
			delete(app.builtModules, m)
		}
	}()
	app.builtModules[m] = true
	if m.Global {
		app.globalModules[m] = true
	}

	for _, imp := range m.Imports {
		if imp == nil {
			continue
		}
		if err := imp.Register(app); err != nil {
			return fmt.Errorf("grove: module %q import failed: %w", m.displayName(), err)
		}
	}
	if app.moduleScopes == nil {
		app.moduleScopes = make(map[*ModuleDef]*ModuleContext)
	}
	isolated := m.Isolated || app.moduleIsolation
	scope := &ModuleContext{
		App:      app,
		Module:   m,
		isolated: isolated,
		own:      make(map[string]bool),
	}
	for _, imp := range m.Imports {
		if s := app.moduleScopes[imp]; s != nil {
			scope.imports = append(scope.imports, s)
		}
	}
	app.moduleScopes[m] = scope
	globalAdded := false
	if m.Global {
		app.globalScopes = append(app.globalScopes, scope)
		globalAdded = true
	}
	scopeRegistered := false
	defer func() {
		if scopeRegistered {
			return
		}
		delete(app.moduleScopes, m)
		if globalAdded {
			for i, g := range app.globalScopes {
				if g == scope {
					app.globalScopes = append(app.globalScopes[:i], app.globalScopes[i+1:]...)
					break
				}
			}
		}
	}()

	for _, p := range m.Providers {
		if err := app.registerProvider(m.displayName(), p, isolated); err != nil {
			return err
		}
		scope.own[p.Name] = true
	}
	for _, c := range m.Controllers {
		app.mountControllerFor(m.displayName(), c)
	}
	if m.BuildControllers != nil {
		built, err := m.BuildControllers(app)
		if err != nil {
			return fmt.Errorf("grove: module %q controllers: %w", m.displayName(), err)
		}
		for _, c := range built {
			app.mountControllerFor(m.displayName(), c)
		}
	}
	if m.Build != nil {
		if err := m.Build(scope); err != nil {
			return fmt.Errorf("grove: module %q build: %w", m.displayName(), err)
		}
	}
	if h := cloneModuleHooks(m.Hooks); !h.empty() {
		app.lifecycle = append(app.lifecycle, moduleLifecycle{name: m.displayName(), hooks: h})
	}
	app.moduleDefs = append(app.moduleDefs, m)
	scopeRegistered = true
	moduleRegistered = true
	return nil
}

// registerProvider validates and registers one provider, applying
// strict/isolated duplicate-key rules. Testing overrides registered
// before Register (Container.Replace) still win and are skipped.
func (a *App) registerProvider(moduleName string, p Provider, isolated bool) error {
	if p.Name == "" {
		return fmt.Errorf("grove: module %q has a provider with empty name or nil build", moduleName)
	}
	if p.Lifetime == di.RequestScoped && p.BuildScoped == nil {
		return fmt.Errorf("grove: module %q provider %q is request-scoped but has no factory (use grove.ProvideRequest)", moduleName, p.Name)
	}
	if p.Lifetime != di.RequestScoped && p.Build == nil {
		return fmt.Errorf("grove: module %q has a provider with empty name or nil build", moduleName)
	}
	if a.Container.Has(p.Name) {
		if owner, ok := a.providerOwners[p.Name]; ok {
			if a.strictProviders || isolated {
				return fmt.Errorf("grove: module %q provider %q duplicates %q (isolated modules cannot share provider keys; rename it or move it to a shared/global module)", moduleName, p.Name, owner)
			}
			// Non-strict legacy mode: first registration wins.
			return nil
		}
		// Has without owner = test Replace mock: keep compat skip.
		return nil
	}
	var err error
	switch p.Lifetime {
	case di.Transient:
		err = a.Container.RegisterTransient(p.Name, p.Build)
	case di.RequestScoped:
		err = a.Container.RegisterScoped(p.Name, p.BuildScoped)
	default:
		// Lazy singleton: built on first resolution so provider
		// order within (and across) modules never matters.
		err = a.Container.RegisterSingletonFactory(p.Name, p.Build)
	}
	if err != nil {
		return fmt.Errorf("grove: module %q provider %q: %w", moduleName, p.Name, err)
	}
	if a.providerOwners == nil {
		a.providerOwners = make(map[string]string)
	}
	a.providerOwners[p.Name] = moduleName
	return nil
}

// Exported reports whether provider name is part of the module public API.
// Empty Exports means fully public (v0.2 compat).
func (m *ModuleDef) Exported(name string) bool {
	if len(m.Exports) == 0 {
		return true
	}
	for _, e := range m.Exports {
		if e == name {
			return true
		}
	}
	return false
}
