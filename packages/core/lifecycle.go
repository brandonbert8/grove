package grove

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/brandonbert8/grove/packages/router"
)

// ServerOptions tunes the HTTP server (v0.3 production knobs).
type ServerOptions struct {
	// Addr overrides Config.Addr when non-empty.
	Addr string
	// CertFile/KeyFile enable TLS (https) when both set.
	CertFile string
	KeyFile  string
	// Timeouts override the Slowloris-safe defaults (0 keeps default).
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

// Run serves the application on addr (e.g. ":3000") with graceful
// shutdown on SIGINT/SIGTERM. An empty addr falls back to Config.Addr().
//
// Startup runs OnStart hooks first (abort on first failure), then
// listens with Slowloris-safe header timeouts. Shutdown drains HTTP
// first so in-flight handlers keep their resources, then runs OnStop
// hooks in reverse order to release them. Run blocks until the
// server stops: nil on graceful shutdown, non-nil on abnormal failure.
func (a *App) Run(addr string) error {
	return a.RunWith(ServerOptions{Addr: addr})
}

// Listen serves on Config.Addr() — the NestJS app.listen() equivalent.
// Prefer it over Run(""): no magic empty string, the address always
// comes from configuration (PORT/GROVE_PORT/HOST).
func (a *App) Listen() error { return a.RunWith(ServerOptions{}) }

// RunWith serves with explicit server options (TLS, timeouts).
func (a *App) RunWith(o ServerOptions) error {
	addr := o.Addr
	if addr == "" {
		addr = a.Config.Addr()
	}
	timeout := a.shutdownTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	startCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for i, fn := range a.starts {
		if err := fn(startCtx); err != nil {
			return fmt.Errorf("grove: start hook %d failed: %w", i, err)
		}
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           a.Router,
		ReadHeaderTimeout: withDefault(o.ReadHeaderTimeout, 10*time.Second),
		ReadTimeout:       withDefault(o.ReadTimeout, 30*time.Second),
		WriteTimeout:      withDefault(o.WriteTimeout, 30*time.Second),
		IdleTimeout:       withDefault(o.IdleTimeout, 60*time.Second),
	}
	serve := srv.ListenAndServe
	if o.CertFile != "" && o.KeyFile != "" {
		serve = func() error { return srv.ListenAndServeTLS(o.CertFile, o.KeyFile) }
	}

	stopped := make(chan error, 1)
	go func() {
		a.Logger.Info("grove listening", "addr", addr, "env", a.Config.Env)
		for _, m := range a.modules {
			a.Logger.Debug("module active", "module", m.Name())
		}
		if err := serve(); err != nil && err != http.ErrServerClosed {
			stopped <- err
			return
		}
		stopped <- nil
	}()

	quit := make(chan os.Signal, 2)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)
	select {
	case sig := <-quit:
		a.Logger.Info("shutdown signal received", "signal", sig.String())
		// A second signal forces immediate close (hung drain escape).
		go func() {
			<-quit
			a.Logger.Info("second shutdown signal: forcing close")
			_ = srv.Close()
		}()
	case err := <-stopped:
		// The server exited on its own (failure or external close):
		// nothing left to drain, but stop hooks still release resources.
		stopCtx, cancel := context.WithTimeout(context.Background(), timeout)
		a.stop(stopCtx)
		cancel()
		return err
	}

	if err := a.shutdown(srv, timeout); err != nil {
		return err
	}
	a.Logger.Info("grove stopped cleanly")
	return nil
}

// withDefault returns d when v <= 0.
func withDefault(v, d time.Duration) time.Duration {
	if v <= 0 {
		return d
	}
	return v
}

// shutdown drains the HTTP server first, then runs OnStop hooks —
// in that order. In-flight handlers keep their pools and queues while
// draining; hooks release them after. Each phase gets a fresh timeout
// so a slow drain cannot starve pool/queue closers. A failed drain
// still runs the hooks before reporting the error.
func (a *App) shutdown(srv *http.Server, timeout time.Duration) error {
	drainCtx, cancel := context.WithTimeout(context.Background(), timeout)
	drainErr := srv.Shutdown(drainCtx)
	cancel()
	stopCtx, stopCancel := context.WithTimeout(context.Background(), timeout)
	defer stopCancel()
	a.stop(stopCtx)
	if drainErr != nil {
		return fmt.Errorf("grove: graceful shutdown failed: %w", drainErr)
	}
	return nil
}

// stop runs OnStop hooks in reverse order, best-effort: every hook runs
// even when an earlier one fails, and failures are logged, not returned,
// so one bad closer cannot wedge shutdown.
func (a *App) stop(ctx context.Context) {
	for i := len(a.stops) - 1; i >= 0; i-- {
		if err := a.stops[i](ctx); err != nil {
			a.Logger.Error("stop hook failed", "hook", i, "error", err.Error())
		}
	}
}

// Handler exposes the underlying http.Handler for embedding Grove in an
// existing server or test harness.
func (a *App) Handler() http.Handler { return a.Router }

// Route registers a handler for an arbitrary method on the app router
// and records it in Docs (unlike raw Router use, which stays invisible
// to docs). It honors the global prefix and exception filters.
// Prefer ControllerDef for documented routes; Undocumented() skips docs.
func (a *App) Route(method, path string, h router.HandlerFunc, mw ...router.Middleware) {
	full := a.prefixedPath(path)
	a.Router.Handle(method, full, h, append([]router.Middleware{a.filterMiddleware()}, mw...)...)
	a.RecordDocs(ControllerDef{Prefix: full, Endpoints: []Endpoint{{Method: method, Path: ""}}})
}

// RouteDoc registers a documented route (method+path+docs options).
func (a *App) RouteDoc(method, path string, h router.HandlerFunc, opts ...any) {
	e := buildEndpoint(method, "", h, opts)
	full := a.prefixedPath(path)
	a.Router.Handle(method, full, e.Handler, append([]router.Middleware{a.filterMiddleware()}, e.Middleware...)...)
	e.Method, e.Path = method, ""
	a.RecordDocs(ControllerDef{Prefix: full, Endpoints: []Endpoint{e}})
}
