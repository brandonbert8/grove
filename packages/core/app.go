package grove

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/brandonbert8/grove/packages/config"
	"github.com/brandonbert8/grove/packages/di"
	"github.com/brandonbert8/grove/packages/logger"
	"github.com/brandonbert8/grove/packages/router"
)

// App is the Grove application object.
//
// It composes the replaceable framework pieces: Router for HTTP,
// Container for dependency injection, Config for typed settings, and
// Logger for structured output. Construct with New and functional
// options, attach behavior with Register, then serve with Run.
//
// v0.3: use NewE/MustNew for fail-fast config, SetGlobalPrefix for
// versioning (NestJS setGlobalPrefix), and UseFilters for @Catch-style
// exception filters.
type App struct {
	// Router handles HTTP. Swap via WithRouter.
	Router router.Router
	// Container holds application dependencies. Swap via WithContainer.
	Container *di.Container
	// Config holds typed settings. Swap via WithConfig.
	Config *config.Config
	// Logger receives framework and (optionally) app logs.
	Logger logger.Logger

	modules []Module
	// builtModules dedupes *ModuleDef registration so shared imports
	// build exactly once even when imported by several modules.
	builtModules map[*ModuleDef]bool
	// globalModules tracks @Global modules already applied.
	globalModules map[*ModuleDef]bool

	// starts runs before listening (OnModuleInit / OnApplicationBootstrap);
	// stops runs during shutdown in reverse order (OnApplicationShutdown).
	starts []StartFunc
	stops  []StopFunc
	// shutdownTimeout bounds starts, stops, and graceful drain.
	shutdownTimeout time.Duration
	// docs mirrors mounted endpoint metadata for docs/tooling.
	docs []EndpointDoc
	// globalPrefix scopes every controller route (e.g. "v1" → "/v1/...").
	// Excluded prefixes (health, docs) bypass it.
	globalPrefix string
	excludedPrefixes []string
	// filters run on handler errors, outermost-first (NestJS @Catch).
	filters []ExceptionFilter
	// strictProviders makes duplicate provider keys fail fast instead
	// of first-wins (TestingModule overrides aside). Opt-in via
	// EnableStrictProviders; default keeps v0.2 compat.
	strictProviders bool
	// enforceExports validates ModuleDef.Exports refer to registered
	// providers via VerifyExports. Opt-in via EnableExportScoping;
	// resolution itself stays app-wide until v1 scoping lands.
	enforceExports bool
	// providerOwners tracks which module registered each provider key,
	// powering duplicate diagnostics and VerifyExports.
	providerOwners map[string]string
}

// StartFunc boots a resource (DB pool, cron, queue) before serving.
type StartFunc func(ctx context.Context) error

// StopFunc releases a resource during shutdown.
type StopFunc func(ctx context.Context) error

// Option customizes an App.
type Option func(*App)

// WithRouter replaces the default ServeMux-based router.
func WithRouter(r router.Router) Option {
	return func(a *App) {
		if r != nil {
			a.Router = r
		}
	}
}

// WithContainer replaces the default DI container.
func WithContainer(c *di.Container) Option {
	return func(a *App) {
		if c != nil {
			a.Container = c
		}
	}
}

// WithConfig replaces the loaded configuration.
func WithConfig(cfg *config.Config) Option {
	return func(a *App) {
		if cfg != nil {
			a.Config = cfg
		}
	}
}

// WithLogger replaces the default slog-based logger.
func WithLogger(l logger.Logger) Option {
	return func(a *App) {
		if l != nil {
			a.Logger = l
		}
	}
}

// WithShutdownTimeout bounds startup hooks, shutdown hooks, and the
// graceful drain (default 10s).
func WithShutdownTimeout(d time.Duration) Option {
	return func(a *App) {
		if d > 0 {
			a.shutdownTimeout = d
		}
	}
}

// WithGlobalPrefix scopes every controller route (NestJS setGlobalPrefix).
// Use WithGlobalPrefixExcluded for health/docs probes that stay unprefixed.
func WithGlobalPrefix(prefix string, excluded ...string) Option {
	return func(a *App) { a.SetGlobalPrefix(prefix, excluded...) }
}

// SetGlobalPrefix scopes every subsequently registered controller route.
// An empty prefix clears versioning. Excluded prefixes bypass it exactly
// (e.g. "/healthz", "/openapi.json").
func (a *App) SetGlobalPrefix(prefix string, excluded ...string) {
	p := normalizePrefix(prefix)
	a.globalPrefix = p
	a.excludedPrefixes = append([]string(nil), excluded...)
}

// GlobalPrefix returns the configured prefix ("" when unset).
func (a *App) GlobalPrefix() string { return a.globalPrefix }

// normalizePrefix ensures "" or "/v1" shape.
func normalizePrefix(p string) string {
	if p == "" || p == "/" {
		return ""
	}
	if p[0] != '/' {
		p = "/" + p
	}
	for len(p) > 1 && p[len(p)-1] == '/' {
		p = p[:len(p)-1]
	}
	return p
}

// prefixedPath applies the global prefix unless path is excluded.
// Exclusions match exactly or on a "/" boundary (/healthz excludes
// /healthz and /healthz/live, never /healthz2).
func (a *App) prefixedPath(path string) string {
	if a.globalPrefix == "" {
		return path
	}
	for _, ex := range a.excludedPrefixes {
		if ex == "" {
			continue
		}
		if path == ex || strings.HasPrefix(path, ex+"/") {
			return path
		}
	}
	if path == "" || path == "/" {
		return a.globalPrefix
	}
	return a.globalPrefix + path
}

// mountController registers one ControllerDef honoring the global prefix
// and the active exception filters.
func (a *App) mountController(c ControllerDef) {
	prefix := a.prefixedPath(c.Prefix)
	g := a.Router.Group(prefix, append([]router.Middleware{a.filterMiddleware()}, c.Middleware...)...)
	for _, e := range c.Endpoints {
		g.Handle(e.Method, e.Path, e.Handler, e.Middleware...)
	}
	doc := c
	doc.Prefix = prefix
	a.recordDocs(doc)
}

// New builds an App with sane defaults and the given options.
//
// v0.2 compatibility: config load failures fall back to development
// defaults. For fail-fast behavior use NewE/MustNew.
func New(opts ...Option) *App {
	cfg, err := config.Load()
	if err != nil {
		// Config is best-effort here; callers needing strict loading
		// should pass WithConfig(config.MustLoad(...)) explicitly.
		// Fall back to an empty config rather than panicking in a library.
		cfg = &config.Config{Port: 3000, LogLevel: "info", Env: "development"}
	}
	app := &App{
		Router:          router.New(),
		Container:       di.New(),
		Config:          cfg,
		Logger:          logger.New(logger.StdOptions{Level: logger.ParseLevel(cfg.LogLevel)}),
		shutdownTimeout: 10 * time.Second,
	}
	for _, opt := range opts {
		opt(app)
	}
	// Wire exception filters into the router error path (best-effort:
	// DefaultRouter exposes the hook; custom routers keep Statuser).
	if dr, ok := app.Router.(interface{ SetErrorHandler(func(router.Context, error)) }); ok {
		dr.SetErrorHandler(app.handleError)
	}
	return app
}

// NewE builds an App but returns config load failures instead of
// silently falling back to development defaults (v0.3 fail-fast).
func NewE(opts ...Option) (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	app := &App{
		Router:          router.New(),
		Container:       di.New(),
		Config:          cfg,
		Logger:          logger.New(logger.StdOptions{Level: logger.ParseLevel(cfg.LogLevel)}),
		shutdownTimeout: 10 * time.Second,
	}
	for _, opt := range opts {
		opt(app)
	}
	if dr, ok := app.Router.(interface{ SetErrorHandler(func(router.Context, error)) }); ok {
		dr.SetErrorHandler(app.handleError)
	}
	return app, nil
}

// MustNew is like NewE but panics on config failure. Prefer it in main.
func MustNew(opts ...Option) *App {
	app, err := NewE(opts...)
	if err != nil {
		panic(err)
	}
	return app
}

// Register attaches modules in order. Registration errors abort the chain.
// Re-registering the same *ModuleDef is a no-op (shared imports build
// once): it is skipped without duplicate logs or Modules() entries.
func (a *App) Register(mods ...Module) error {
	for _, m := range mods {
		if m == nil {
			continue
		}
		if ad, ok := m.(moduleDefAdapter); ok && ad.m != nil {
			if a.builtModules != nil && a.builtModules[ad.m] {
				continue
			}
		}
		if err := m.Register(a); err != nil {
			return err
		}
		a.modules = append(a.modules, m)
		a.Logger.Info("module registered", "module", m.Name())
	}
	return nil
}

// MustRegister is like Register but panics on error. Useful in examples.
func (a *App) MustRegister(mods ...Module) {
	if err := a.Register(mods...); err != nil {
		panic(err)
	}
}

// Modules returns the registered modules in registration order.
func (a *App) Modules() []Module {
	return append([]Module(nil), a.modules...)
}

// EnableStrictProviders makes duplicate provider keys an error at
// Register time (fail fast on copy-paste modules). Testing overrides
// via Container.Replace before Register still win: pre-replaced keys
// are treated as intentional mocks and skipped, not errored.
func (a *App) EnableStrictProviders() { a.strictProviders = true }

// EnableExportScoping opts into validating ModuleDef.Exports: every
// entry must name a provider the module (or its imports) registered.
// Call VerifyExports after Register to fail fast on typos. This is the
// stepping stone to full NestJS exports enforcement without breaking
// v0.2 apps that rely on app-wide visibility.
func (a *App) EnableExportScoping() { a.enforceExports = true }

// VerifyExports checks every registered ModuleDef's Exports name a
// known provider. It returns an error naming the module + missing key.
// No-op unless EnableExportScoping was called.
func (a *App) VerifyExports() error {
	if !a.enforceExports {
		return nil
	}
	for _, m := range a.modules {
		ad, ok := m.(moduleDefAdapter)
		if !ok || ad.m == nil {
			continue
		}
		for _, e := range ad.m.Exports {
			if e == "" {
				return fmt.Errorf("grove: module %q exports empty provider name", ad.m.displayName())
			}
			if !a.Container.Has(e) {
				return fmt.Errorf("grove: module %q exports unknown provider %q (did you add grove.Provide for it?)", ad.m.displayName(), e)
			}
		}
	}
	return nil
}

// OnStart registers hooks that run in order before Run starts
// listening — the Go equivalent of NestJS OnModuleInit /
// OnApplicationBootstrap (open pools, start crons, warm caches).
// A hook failure aborts startup and Run returns the error.
func (a *App) OnStart(fns ...StartFunc) {
	for _, fn := range fns {
		if fn != nil {
			a.starts = append(a.starts, fn)
		}
	}
}

// OnStop registers hooks that run in reverse order during shutdown —
// the equivalent of NestJS OnApplicationShutdown / enableShutdownHooks
// (close pools, flush queues). Hooks run after the HTTP drain, so
// in-flight handlers still have their resources while draining.
func (a *App) OnStop(fns ...StopFunc) {
	for _, fn := range fns {
		if fn != nil {
			a.stops = append(a.stops, fn)
		}
	}
}
