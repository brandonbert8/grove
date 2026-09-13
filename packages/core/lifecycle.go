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

// Run serves the application on addr (e.g. ":3000") with graceful
// shutdown on SIGINT/SIGTERM. An empty addr falls back to Config.Addr().
//
// Startup runs OnStart hooks first (abort on first failure), then
// listens with Slowloris-safe header timeouts. Shutdown drains HTTP
// first so in-flight handlers keep their resources, then runs OnStop
// hooks in reverse order to release them. Run blocks until the
// server stops: nil on graceful shutdown, non-nil on abnormal failure.
func (a *App) Run(addr string) error {
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
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	stopped := make(chan error, 1)
	go func() {
		a.Logger.Info("grove listening", "addr", addr, "env", a.Config.Env)
		for _, m := range a.modules {
			a.Logger.Debug("module active", "module", m.Name())
		}
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			stopped <- err
			return
		}
		stopped <- nil
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)
	select {
	case sig := <-quit:
		a.Logger.Info("shutdown signal received", "signal", sig.String())
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

// shutdown drains the HTTP server first, then runs OnStop hooks —
// in that order, under one bounded timeout. In-flight handlers keep
// their pools and queues while draining; hooks release them after.
// A failed drain still runs the hooks before reporting the error.
func (a *App) shutdown(srv *http.Server, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	drainErr := srv.Shutdown(ctx)
	a.stop(ctx)
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
// to docs). It is shorthand for module code that prefers app-level
// helpers.
func (a *App) Route(method, path string, h router.HandlerFunc, mw ...router.Middleware) {
	a.Router.Handle(method, path, h, mw...)
	a.RecordDocs(ControllerDef{Endpoints: []Endpoint{{Method: method, Path: path}}})
}
