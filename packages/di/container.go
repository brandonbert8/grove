package di

import (
	"fmt"
	"reflect"
	"sync"
)

// Lifetime describes how a registration is resolved.
type Lifetime int

const (
	// Singleton resolves to the same shared instance on every call.
	Singleton Lifetime = iota
	// Transient invokes the factory on every resolution.
	Transient
)

// entry holds one registration.
type entry struct {
	lifetime Lifetime
	instance any
	factory  func(c *Container) (any, error)
}

// Container is a minimal dependency injection container.
//
// It supports singleton and transient lifetimes with explicit constructor
// injection. Resolve paths are protected by a mutex plus per-name
// singleflight so a single Container is safe for concurrent use and a
// lazy singleton factory runs exactly once even under contention.
//
// The zero value is not usable; construct with New.
type Container struct {
	mu        sync.Mutex
	entries   map[string]*entry
	resolving map[string]bool
	inflight  map[string]*buildCall
}

// buildCall coalesces concurrent first-resolves of one lazy singleton.
type buildCall struct {
	done chan struct{}
	val  any
	err  error
}

// New returns an empty Container.
func New() *Container {
	return &Container{
		entries:   make(map[string]*entry),
		resolving: make(map[string]bool),
		inflight:  make(map[string]*buildCall),
	}
}

// RegisterSingleton stores a shared instance under name.
// It returns an error if name is empty, instance is nil, or name is
// already registered. Use Replace in tests to override.
func (c *Container) RegisterSingleton(name string, instance any) error {
	if name == "" {
		return fmt.Errorf("di: singleton name must not be empty")
	}
	if instance == nil {
		return fmt.Errorf("di: singleton %q instance must not be nil", name)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[name]; exists {
		return fmt.Errorf("di: %q is already registered", name)
	}
	c.entries[name] = &entry{lifetime: Singleton, instance: instance}
	return nil
}

// Replace upserts a shared singleton instance, intended for tests
// (TestingModule.overrideProvider equivalent). Production wiring should
// prefer Register*; Replace never fails.
func (c *Container) Replace(name string, instance any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[name] = &entry{lifetime: Singleton, instance: instance}
}

// ReplaceAs upserts instance under the default key for T.
func ReplaceAs[T any](c *Container, instance T) {
	c.Replace(keyFor[T](), instance)
}

// RegisterTransient stores a factory invoked on every Resolve of name.
// The factory receives the container so constructors can resolve their
// own dependencies explicitly (constructor injection).
func (c *Container) RegisterTransient(name string, factory func(c *Container) (any, error)) error {
	if name == "" {
		return fmt.Errorf("di: transient name must not be empty")
	}
	if factory == nil {
		return fmt.Errorf("di: transient %q factory must not be nil", name)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[name]; exists {
		return fmt.Errorf("di: %q is already registered", name)
	}
	c.entries[name] = &entry{lifetime: Transient, factory: factory}
	return nil
}

// MustRegisterSingleton is like RegisterSingleton but panics on error.
// It is convenient for wiring done at startup.
func (c *Container) MustRegisterSingleton(name string, instance any) {
	if err := c.RegisterSingleton(name, instance); err != nil {
		panic(err)
	}
}

// RegisterSingletonFactory stores a factory that runs once: the first
// Resolve builds the value and caches it for later resolves. It combines
// lazy construction with shared-instance semantics.
func (c *Container) RegisterSingletonFactory(name string, factory func(c *Container) (any, error)) error {
	if name == "" {
		return fmt.Errorf("di: singleton name must not be empty")
	}
	if factory == nil {
		return fmt.Errorf("di: singleton %q factory must not be nil", name)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[name]; exists {
		return fmt.Errorf("di: %q is already registered", name)
	}
	c.entries[name] = &entry{lifetime: Singleton, factory: factory}
	return nil
}

// MustRegisterSingletonFactory is like RegisterSingletonFactory but panics on error.
func (c *Container) MustRegisterSingletonFactory(name string, factory func(c *Container) (any, error)) {
	if err := c.RegisterSingletonFactory(name, factory); err != nil {
		panic(err)
	}
}

// MustRegisterTransient is like RegisterTransient but panics on error.
func (c *Container) MustRegisterTransient(name string, factory func(c *Container) (any, error)) {
	if err := c.RegisterTransient(name, factory); err != nil {
		panic(err)
	}
}

// Has reports whether name is registered.
func (c *Container) Has(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.entries[name]
	return ok
}

// Resolve returns the dependency registered under name.
//
// Singletons return the shared instance. Transients invoke the factory
// on each call. A circular resolution chain returns an error instead of
// recursing forever. Concurrent first-resolves of one lazy singleton
// coalesce: the factory runs once and all callers share the result.
func (c *Container) Resolve(name string) (any, error) {
	c.mu.Lock()
	e, ok := c.entries[name]
	if !ok {
		c.mu.Unlock()
		return nil, fmt.Errorf("di: no registration for %q (did you add grove.Provide/Provide0 for it to ModuleDef.Providers, or import the module that provides it?)", name)
	}
	if c.resolving[name] {
		c.mu.Unlock()
		return nil, fmt.Errorf("di: circular dependency detected for %q", name)
	}
	if e.lifetime == Singleton && e.factory == nil {
		inst := e.instance
		c.mu.Unlock()
		return inst, nil
	}
	if e.lifetime == Transient {
		factory := e.factory
		c.resolving[name] = true
		c.mu.Unlock()
		defer func() {
			c.mu.Lock()
			delete(c.resolving, name)
			c.mu.Unlock()
		}()
		if factory == nil {
			return nil, fmt.Errorf("di: registration %q has no factory or instance", name)
		}
		inst, err := factory(c)
		if err != nil {
			return nil, fmt.Errorf("di: factory for %q failed: %w", name, err)
		}
		if inst == nil {
			return nil, fmt.Errorf("di: factory for %q returned nil", name)
		}
		return inst, nil
	}
	// Singleton with factory: singleflight.
	if call, building := c.inflight[name]; building {
		c.mu.Unlock()
		<-call.done
		if call.err != nil {
			return nil, call.err
		}
		return call.val, nil
	}
	call := &buildCall{done: make(chan struct{})}
	c.inflight[name] = call
	c.resolving[name] = true
	factory := e.factory
	c.mu.Unlock()

	inst, err := func() (any, error) {
		defer func() {
			c.mu.Lock()
			delete(c.resolving, name)
			c.mu.Unlock()
		}()
		if factory == nil {
			return nil, fmt.Errorf("di: registration %q has no factory or instance", name)
		}
		v, ferr := factory(c)
		if ferr != nil {
			return nil, fmt.Errorf("di: factory for %q failed: %w", name, ferr)
		}
		if v == nil {
			return nil, fmt.Errorf("di: factory for %q returned nil", name)
		}
		return v, nil
	}()

	c.mu.Lock()
	if err == nil {
		e.instance = inst
		e.factory = nil // cache: later resolves skip the factory
	}
	call.val, call.err = inst, err
	delete(c.inflight, name)
	close(call.done)
	c.mu.Unlock()
	return inst, err
}

// MustResolve is like Resolve but panics on error.
func (c *Container) MustResolve(name string) any {
	v, err := c.Resolve(name)
	if err != nil {
		panic(err)
	}
	return v
}

// keyFor derives the default registration key for type T.
// Pointer, slice, and other wrappers are unwound to the named element
// type so *Service and Service share one stable, human-readable key.
// This is the only reflection in the package, used solely for key
// derivation so future code generation can substitute static keys.
func keyFor[T any]() string {
	t := reflect.TypeOf((*T)(nil)).Elem()
	for {
		switch t.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map, reflect.Chan:
			t = t.Elem()
		default:
			goto done
		}
	}
done:
	if t.Name() == "" {
		// Anonymous/unnamed (e.g. func, interface literal): fall back.
		return t.PkgPath() + "." + t.String()
	}
	if t.PkgPath() == "" {
		return t.String()
	}
	return t.PkgPath() + "." + t.Name()
}

// KeyFor returns the default registration key for type T.
// Exported so modules, controllers, and future generated code can share
// stable keys without stringly-typed duplication.
func KeyFor[T any]() string { return keyFor[T]() }

// RegisterSingletonAs registers instance under the default key for T.
func RegisterSingletonAs[T any](c *Container, instance T) error {
	return c.RegisterSingleton(keyFor[T](), instance)
}

// RegisterTransientAs registers factory under the default key for T.
func RegisterTransientAs[T any](c *Container, factory func(c *Container) (T, error)) error {
	wrapped := func(c *Container) (any, error) {
		v, err := factory(c)
		if err != nil {
			return nil, err
		}
		return v, nil
	}
	return c.RegisterTransient(keyFor[T](), wrapped)
}

// ResolveAs resolves the default key for T and type-asserts the result.
func ResolveAs[T any](c *Container) (T, error) {
	var zero T
	v, err := c.Resolve(keyFor[T]())
	if err != nil {
		return zero, err
	}
	typed, ok := v.(T)
	if !ok {
		return zero, fmt.Errorf("di: registration for %q is %T, not %s", keyFor[T](), v, keyFor[T]())
	}
	return typed, nil
}
