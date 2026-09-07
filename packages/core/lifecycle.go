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
// Run blocks until the server stops. It returns nil on graceful shutdown
// and a non-nil error on abnormal failure.
func (a *App) Run(addr string) error {
	if addr == "" {
		addr = a.Config.Addr()
	}
	srv := &http.Server{Addr: addr, Handler: a.Router}

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
	select {
	case sig := <-quit:
		a.Logger.Info("shutdown signal received", "signal", sig.String())
	case err := <-stopped:
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("grove: graceful shutdown failed: %w", err)
	}
	a.Logger.Info("grove stopped cleanly")
	return nil
}

// Handler exposes the underlying http.Handler for embedding Grove in an
// existing server or test harness.
func (a *App) Handler() http.Handler { return a.Router }

// Route registers a handler for an arbitrary method on the app router.
// It is shorthand for module code that prefers app-level helpers.
func (a *App) Route(method, path string, h router.HandlerFunc, mw ...router.Middleware) {
	a.Router.Handle(method, path, h, mw...)
}
