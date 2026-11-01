package grove

import (
	"context"
	"errors"
	"fmt"
)

// OnModuleInit runs after all modules are registered, before the server
// starts listening. Imports run before importers.
type OnModuleInit interface {
	OnModuleInit(context.Context) error
}

// OnApplicationBootstrap runs after every OnModuleInit hook succeeded,
// immediately before the server starts listening.
type OnApplicationBootstrap interface {
	OnApplicationBootstrap(context.Context) error
}

// OnModuleDestroy runs during shutdown, after HTTP draining. Modules run
// in reverse registration order (importers before imports).
type OnModuleDestroy interface {
	OnModuleDestroy(context.Context) error
}

// OnApplicationShutdown runs after OnModuleDestroy hooks, once all
// connections are closed.
type OnApplicationShutdown interface {
	OnApplicationShutdown(context.Context) error
}

// moduleLifecycle is one module's snapshotted hooks in registration order.
type moduleLifecycle struct {
	name  string
	hooks ModuleHooks
}

// OnModuleInit appends a module-scoped init hook. Prefer this over
// app.OnStart when the resource belongs to one module: it runs in module
// dependency order and is easy to test in isolation.
func (m *ModuleContext) OnModuleInit(fn func(context.Context) error) {
	if m == nil || m.Module == nil || fn == nil {
		return
	}
	m.Module.Hooks.Init = append(m.Module.Hooks.Init, fn)
}

// OnApplicationBootstrap appends a hook that runs after every module
// finished initializing.
func (m *ModuleContext) OnApplicationBootstrap(fn func(context.Context) error) {
	if m == nil || m.Module == nil || fn == nil {
		return
	}
	m.Module.Hooks.Bootstrap = append(m.Module.Hooks.Bootstrap, fn)
}

// OnModuleDestroy appends a shutdown hook. Hooks run in reverse order.
func (m *ModuleContext) OnModuleDestroy(fn func(context.Context) error) {
	if m == nil || m.Module == nil || fn == nil {
		return
	}
	m.Module.Hooks.Destroy = append(m.Module.Hooks.Destroy, fn)
}

// OnApplicationShutdown appends a hook that runs after module destroy
// hooks, once connections are closed. Hooks run in reverse order.
func (m *ModuleContext) OnApplicationShutdown(fn func(context.Context) error) {
	if m == nil || m.Module == nil || fn == nil {
		return
	}
	m.Module.Hooks.Shutdown = append(m.Module.Hooks.Shutdown, fn)
}

// RegisterLifecycle adapts a provider that implements one or more
// lifecycle interfaces into module hooks:
//
//	svc := grove.MustInject[*Service](m)
//	m.RegisterLifecycle(svc)
func (m *ModuleContext) RegisterLifecycle(v any) {
	if m == nil || m.Module == nil || v == nil {
		return
	}
	h := hooksFrom(v)
	m.Module.Hooks.Init = append(m.Module.Hooks.Init, h.Init...)
	m.Module.Hooks.Bootstrap = append(m.Module.Hooks.Bootstrap, h.Bootstrap...)
	m.Module.Hooks.Destroy = append(m.Module.Hooks.Destroy, h.Destroy...)
	m.Module.Hooks.Shutdown = append(m.Module.Hooks.Shutdown, h.Shutdown...)
}

// hooksFrom extracts lifecycle interfaces from v. Used for legacy
// modules (Module implementations) and RegisterLifecycle.
func hooksFrom(v any) ModuleHooks {
	var h ModuleHooks
	if x, ok := v.(OnModuleInit); ok {
		h.Init = append(h.Init, x.OnModuleInit)
	}
	if x, ok := v.(OnApplicationBootstrap); ok {
		h.Bootstrap = append(h.Bootstrap, x.OnApplicationBootstrap)
	}
	if x, ok := v.(OnModuleDestroy); ok {
		h.Destroy = append(h.Destroy, x.OnModuleDestroy)
	}
	if x, ok := v.(OnApplicationShutdown); ok {
		h.Shutdown = append(h.Shutdown, x.OnApplicationShutdown)
	}
	return h
}

func (h ModuleHooks) empty() bool {
	return len(h.Init) == 0 && len(h.Bootstrap) == 0 && len(h.Destroy) == 0 && len(h.Shutdown) == 0
}

func cloneModuleHooks(h ModuleHooks) ModuleHooks {
	return ModuleHooks{
		Init:      append([]func(context.Context) error(nil), h.Init...),
		Bootstrap: append([]func(context.Context) error(nil), h.Bootstrap...),
		Destroy:   append([]func(context.Context) error(nil), h.Destroy...),
		Shutdown:  append([]func(context.Context) error(nil), h.Shutdown...),
	}
}

func (a *App) initModules(ctx context.Context) error {
	return a.runModuleHooks(ctx, "OnModuleInit", func(h ModuleHooks) []func(context.Context) error {
		return h.Init
	}, false)
}

func (a *App) bootstrapModules(ctx context.Context) error {
	return a.runModuleHooks(ctx, "OnApplicationBootstrap", func(h ModuleHooks) []func(context.Context) error {
		return h.Bootstrap
	}, false)
}

func (a *App) destroyModules(ctx context.Context) error {
	return a.runModuleHooks(ctx, "OnModuleDestroy", func(h ModuleHooks) []func(context.Context) error {
		return h.Destroy
	}, true)
}

func (a *App) shutdownModules(ctx context.Context) error {
	return a.runModuleHooks(ctx, "OnApplicationShutdown", func(h ModuleHooks) []func(context.Context) error {
		return h.Shutdown
	}, true)
}

// runModuleHooks executes one phase in order (or reverse order) and
// aggregates failures so one broken module does not hide the rest.
func (a *App) runModuleHooks(ctx context.Context, phase string, pick func(ModuleHooks) []func(context.Context) error, reverse bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	var errs []error
	for i := 0; i < len(a.lifecycle); i++ {
		idx := i
		if reverse {
			idx = len(a.lifecycle) - 1 - i
		}
		entry := a.lifecycle[idx]
		fns := pick(entry.hooks)
		for j := 0; j < len(fns); j++ {
			fnIdx := j
			if reverse {
				fnIdx = len(fns) - 1 - j
			}
			fn := fns[fnIdx]
			if fn == nil {
				continue
			}
			if err := fn(ctx); err != nil {
				wrapped := fmt.Errorf("grove: module %q %s: %w", entry.name, phase, err)
				if a.Logger != nil {
					a.Logger.Error("module lifecycle hook failed", "module", entry.name, "phase", phase, "error", err.Error())
				}
				errs = append(errs, wrapped)
			}
		}
	}
	return errors.Join(errs...)
}
