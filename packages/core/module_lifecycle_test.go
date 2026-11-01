package grove

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func lifecycleRecorder(events *[]string, name string) func(context.Context) error {
	return func(context.Context) error {
		*events = append(*events, name)
		return nil
	}
}

func TestModuleLifecycleOrder(t *testing.T) {
	var events []string
	child := &ModuleDef{
		Name: "child",
		Hooks: ModuleHooks{
			Init:      []func(context.Context) error{lifecycleRecorder(&events, "init:child")},
			Bootstrap: []func(context.Context) error{lifecycleRecorder(&events, "bootstrap:child")},
			Destroy:   []func(context.Context) error{lifecycleRecorder(&events, "destroy:child")},
			Shutdown:  []func(context.Context) error{lifecycleRecorder(&events, "shutdown:child")},
		},
	}
	parent := &ModuleDef{
		Name:    "parent",
		Imports: []*ModuleDef{child},
		Build: func(m *ModuleContext) error {
			m.OnModuleInit(lifecycleRecorder(&events, "init:parent"))
			m.OnApplicationBootstrap(lifecycleRecorder(&events, "bootstrap:parent"))
			m.OnModuleDestroy(lifecycleRecorder(&events, "destroy:parent"))
			m.OnApplicationShutdown(lifecycleRecorder(&events, "shutdown:parent"))
			return nil
		},
	}
	app := New()
	app.MustRegister(parent.AsModule())

	ctx := context.Background()
	if err := app.initModules(ctx); err != nil {
		t.Fatal(err)
	}
	if err := app.bootstrapModules(ctx); err != nil {
		t.Fatal(err)
	}
	if err := app.destroyModules(ctx); err != nil {
		t.Fatal(err)
	}
	if err := app.shutdownModules(ctx); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"init:child", "init:parent",
		"bootstrap:child", "bootstrap:parent",
		"destroy:parent", "destroy:child",
		"shutdown:parent", "shutdown:child",
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
}

func TestModuleLifecycleHooksReverseWithinModule(t *testing.T) {
	var events []string
	m := &ModuleDef{
		Name: "m",
		Hooks: ModuleHooks{
			Init:    []func(context.Context) error{lifecycleRecorder(&events, "init1"), lifecycleRecorder(&events, "init2")},
			Destroy: []func(context.Context) error{lifecycleRecorder(&events, "destroy1"), lifecycleRecorder(&events, "destroy2")},
		},
	}
	app := New()
	app.MustRegister(m.AsModule())

	ctx := context.Background()
	if err := app.initModules(ctx); err != nil {
		t.Fatal(err)
	}
	if err := app.destroyModules(ctx); err != nil {
		t.Fatal(err)
	}
	want := []string{"init1", "init2", "destroy2", "destroy1"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
}

type lifecycleProvider struct {
	events *[]string
}

func (p *lifecycleProvider) OnModuleInit(context.Context) error {
	*p.events = append(*p.events, "provider:init")
	return nil
}

func (p *lifecycleProvider) OnApplicationBootstrap(context.Context) error {
	*p.events = append(*p.events, "provider:bootstrap")
	return nil
}

func (p *lifecycleProvider) OnModuleDestroy(context.Context) error {
	*p.events = append(*p.events, "provider:destroy")
	return nil
}

func (p *lifecycleProvider) OnApplicationShutdown(context.Context) error {
	*p.events = append(*p.events, "provider:shutdown")
	return nil
}

func TestRegisterLifecycleProvider(t *testing.T) {
	var events []string
	m := &ModuleDef{
		Name: "m",
		Build: func(m *ModuleContext) error {
			m.RegisterLifecycle(&lifecycleProvider{events: &events})
			return nil
		},
	}
	app := New()
	app.MustRegister(m.AsModule())

	ctx := context.Background()
	if err := app.initModules(ctx); err != nil {
		t.Fatal(err)
	}
	if err := app.bootstrapModules(ctx); err != nil {
		t.Fatal(err)
	}
	if err := app.destroyModules(ctx); err != nil {
		t.Fatal(err)
	}
	if err := app.shutdownModules(ctx); err != nil {
		t.Fatal(err)
	}
	want := []string{"provider:init", "provider:bootstrap", "provider:destroy", "provider:shutdown"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
}

func TestModuleLifecycleAggregatesErrors(t *testing.T) {
	app := New()
	app.MustRegister((&ModuleDef{
		Name: "a",
		Hooks: ModuleHooks{
			Init: []func(context.Context) error{func(context.Context) error { return errors.New("boom-a") }},
		},
	}).AsModule())
	app.MustRegister((&ModuleDef{
		Name: "b",
		Hooks: ModuleHooks{
			Init: []func(context.Context) error{func(context.Context) error { return errors.New("boom-b") }},
		},
	}).AsModule())

	err := app.initModules(context.Background())
	if err == nil {
		t.Fatal("expected aggregated error")
	}
	for _, want := range []string{`module "a"`, `module "b"`, "boom-a", "boom-b"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err, want)
		}
	}
}

type hookedLegacyModule struct {
	events *[]string
}

func (h hookedLegacyModule) Name() string { return "legacy" }

func (h hookedLegacyModule) Register(*App) error { return nil }

func (h hookedLegacyModule) OnModuleInit(context.Context) error {
	*h.events = append(*h.events, "legacy:init")
	return nil
}

func (h hookedLegacyModule) OnApplicationBootstrap(context.Context) error {
	*h.events = append(*h.events, "legacy:bootstrap")
	return nil
}

func (h hookedLegacyModule) OnModuleDestroy(context.Context) error {
	*h.events = append(*h.events, "legacy:destroy")
	return nil
}

func (h hookedLegacyModule) OnApplicationShutdown(context.Context) error {
	*h.events = append(*h.events, "legacy:shutdown")
	return nil
}

func TestLegacyModuleLifecycleHooks(t *testing.T) {
	var events []string
	app := New()
	if err := app.Register(hookedLegacyModule{events: &events}); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := app.initModules(ctx); err != nil {
		t.Fatal(err)
	}
	if err := app.bootstrapModules(ctx); err != nil {
		t.Fatal(err)
	}
	if err := app.destroyModules(ctx); err != nil {
		t.Fatal(err)
	}
	if err := app.shutdownModules(ctx); err != nil {
		t.Fatal(err)
	}
	want := []string{"legacy:init", "legacy:bootstrap", "legacy:destroy", "legacy:shutdown"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
}
