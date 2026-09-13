package grove

import (
	"context"
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

	// starts runs before listening (OnModuleInit / OnApplicationBootstrap);
	// stops runs during shutdown in reverse order (OnApplicationShutdown).
	starts []StartFunc
	stops  []StopFunc
	// shutdownTimeout bounds starts, stops, and graceful drain.
	shutdownTimeout time.Duration
	// docs mirrors mounted endpoint metadata for docs/tooling.
	docs []EndpointDoc
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

// New builds an App with sane defaults and the given options.
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
	return app
}

// Register attaches modules in order. Registration errors abort the chain.
func (a *App) Register(mods ...Module) error {
	for _, m := range mods {
		if m == nil {
			continue
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
