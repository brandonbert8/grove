package grove

import (
	"fmt"

	"github.com/brandonbert8/grove/packages/di"
)

// resolveAs resolves the default key for T from r and type-asserts it.
func resolveAs[T any](r Resolver) (T, error) {
	var zero T
	v, err := r.Resolve(di.KeyFor[T]())
	if err != nil {
		return zero, err
	}
	typed, ok := v.(T)
	if !ok {
		return zero, fmt.Errorf("grove: registration for %q is %T, not %s", di.KeyFor[T](), v, di.KeyFor[T]())
	}
	return typed, nil
}

// Inject resolves the default key for T from r, the Go equivalent of
// NestJS constructor injection. Pass an *App at the root or a
// *ModuleContext inside ModuleDef.Build:
//
//	svc, err := grove.Inject[*UsersService](app)
//	if err != nil { return nil, err }
func Inject[T any](r Resolver) (T, error) {
	return resolveAs[T](r)
}

// MustInject is like Inject but panics on error. Useful in
// BuildControllers closures where a missing provider is a programming
// error:
//
//	svc := grove.MustInject[*UsersService](app)
func MustInject[T any](r Resolver) T {
	v, err := resolveAs[T](r)
	if err != nil {
		panic(err)
	}
	return v
}

// ControllerFor resolves an already-provided controller of type T and
// maps it to one ControllerDef, removing the ResolveAs boilerplate from
// BuildControllers. Register the controller constructor as a provider
// first (the @Controller + @Injectable equivalent):
//
//	Providers: []grove.Provider{
//	    grove.Provide(di.Singleton, NewUsersController), // ctor takes *UsersService
//	},
//	BuildControllers: func(app *grove.App) ([]grove.ControllerDef, error) {
//	    ctrl, err := grove.ControllerFor(app, func(c *UsersController) grove.ControllerDef {
//	        return grove.ControllerDef{
//	            Prefix: "/users",
//	            Tags:   []string{"users"},
//	            Endpoints: []grove.Endpoint{
//	                grove.GET("", c.List, grove.WithSummary("List users")),
//	            },
//	        }
//	    })
//	    if err != nil { return nil, err }
//	    return []grove.ControllerDef{ctrl}, nil
//	},
func ControllerFor[T any](r Resolver, define func(T) ControllerDef) (ControllerDef, error) {
	ctrl, err := resolveAs[T](r)
	if err != nil {
		var zero ControllerDef
		return zero, err
	}
	return define(ctrl), nil
}

// Wire resolves dependency T from the app container and applies ctor,
// the one-line constructor injection for controllers built outside the
// container:
//
//	h, err := grove.Wire(app, NewUsersController) // NewUsersController(svc *Service) *Controller
func Wire[T any, C any](r Resolver, ctor func(T) C) (C, error) {
	var zero C
	dep, err := resolveAs[T](r)
	if err != nil {
		return zero, err
	}
	return ctor(dep), nil
}

// Wire2 resolves two dependencies and applies ctor.
//
//	h, err := grove.Wire2(app, NewAuthController) // func(svc *Service, store *Store) *Controller
//
// For three or more dependencies prefer grove.Wire3 / grove.Wire4; beyond
// that, resolve explicitly with grove.Inject / di.ResolveAs and call the
// constructor yourself. There is no variadic Wire by design: explicit
// type parameters keep constructor wiring compile-time checked.
func Wire2[T1 any, T2 any, C any](r Resolver, ctor func(T1, T2) C) (C, error) {
	var zero C
	d1, err := resolveAs[T1](r)
	if err != nil {
		return zero, err
	}
	d2, err := resolveAs[T2](r)
	if err != nil {
		return zero, err
	}
	return ctor(d1, d2), nil
}

// Wire3 resolves three dependencies and applies ctor.
//
// For more than four dependencies resolve explicitly with grove.Inject /
// di.ResolveAs and call the constructor yourself, or use the `grove wire`
// codegen (zero ResolveAs). There is no variadic Wire by design: explicit
// type parameters keep constructor wiring compile-time checked.
func Wire3[T1 any, T2 any, T3 any, C any](r Resolver, ctor func(T1, T2, T3) C) (C, error) {
	var zero C
	d1, err := resolveAs[T1](r)
	if err != nil {
		return zero, err
	}
	d2, err := resolveAs[T2](r)
	if err != nil {
		return zero, err
	}
	d3, err := resolveAs[T3](r)
	if err != nil {
		return zero, err
	}
	return ctor(d1, d2, d3), nil
}

// Wire4 resolves four dependencies and applies ctor.
//
// This is the last arity-specific helper. For five or more dependencies
// resolve explicitly with grove.Inject / di.ResolveAs and call the
// constructor yourself, or use the `grove wire` codegen.
func Wire4[T1 any, T2 any, T3 any, T4 any, C any](r Resolver, ctor func(T1, T2, T3, T4) C) (C, error) {
	var zero C
	d1, err := resolveAs[T1](r)
	if err != nil {
		return zero, err
	}
	d2, err := resolveAs[T2](r)
	if err != nil {
		return zero, err
	}
	d3, err := resolveAs[T3](r)
	if err != nil {
		return zero, err
	}
	d4, err := resolveAs[T4](r)
	if err != nil {
		return zero, err
	}
	return ctor(d1, d2, d3, d4), nil
}

// MustWire is like Wire but panics on error (for BuildControllers).
func MustWire[T any, C any](r Resolver, ctor func(T) C) C {
	c, err := Wire(r, ctor)
	if err != nil {
		panic(err)
	}
	return c
}

// Controllers wires one controller and maps it to ControllerDefs in a
// single return — the whole BuildControllers body in one expression:
//
//	BuildControllers: func(app *grove.App) ([]grove.ControllerDef, error) {
//	    return grove.Controllers(app, NewUsersController, func(ctrl *UsersController) grove.ControllerDef {
//	        return grove.ControllerDef{
//	            Prefix: "/users",
//	            Tags:   []string{"users"},
//	            Endpoints: []grove.Endpoint{
//	                grove.GET("", ctrl.List, grove.WithSummary("List users")),
//	            },
//	        }
//	    })
//	}
func Controllers[T any, C any](r Resolver, ctor func(T) C, define func(C) ControllerDef) ([]ControllerDef, error) {
	ctrl, err := Wire(r, ctor)
	if err != nil {
		return nil, err
	}
	return []ControllerDef{define(ctrl)}, nil
}

// Controllers2 is Controllers for two-dependency constructors
// (see Wire2).
//
// Controllers3 / Controllers4 cover three and four dependencies. Beyond
// that, resolve explicitly with grove.Inject / di.ResolveAs and build the
// ControllerDef yourself.
func Controllers2[T1 any, T2 any, C any](r Resolver, ctor func(T1, T2) C, define func(C) ControllerDef) ([]ControllerDef, error) {
	ctrl, err := Wire2(r, ctor)
	if err != nil {
		return nil, err
	}
	return []ControllerDef{define(ctrl)}, nil
}

// Controllers3 is Controllers for three-dependency constructors
// (see Wire3). It removes the Wire2→Wire3 rewrite cliff: adding a
// dependency only changes the constructor, not the helper family.
//
// Controllers4 covers four dependencies. Beyond that, resolve explicitly
// with grove.Inject / di.ResolveAs and build the ControllerDef yourself.
func Controllers3[T1 any, T2 any, T3 any, C any](r Resolver, ctor func(T1, T2, T3) C, define func(C) ControllerDef) ([]ControllerDef, error) {
	ctrl, err := Wire3(r, ctor)
	if err != nil {
		return nil, err
	}
	return []ControllerDef{define(ctrl)}, nil
}

// Controllers4 is Controllers for four-dependency constructors
// (see Wire4). This is the last arity-specific helper.
//
// For five or more dependencies resolve explicitly with grove.Inject /
// di.ResolveAs and build the ControllerDef yourself, or use the `grove
// wire` codegen.
func Controllers4[T1 any, T2 any, T3 any, T4 any, C any](r Resolver, ctor func(T1, T2, T3, T4) C, define func(C) ControllerDef) ([]ControllerDef, error) {
	ctrl, err := Wire4(r, ctor)
	if err != nil {
		return nil, err
	}
	return []ControllerDef{define(ctrl)}, nil
}

// MustControllers is like Controllers but panics on error. Useful in
// BuildControllers where wiring failures are programming errors and
// returning nil,err boilerplate only obscures the route table.
func MustControllers[T any, C any](r Resolver, ctor func(T) C, define func(C) ControllerDef) []ControllerDef {
	defs, err := Controllers(r, ctor, define)
	if err != nil {
		panic(err)
	}
	return defs
}

// Build returns a ModuleDef.Build function that wires ctor with
// module-scoped resolution (honoring isolated-module visibility) and mounts
// define's route table. It keeps the common module definition to one field:
//
//	var UsersModule = &grove.ModuleDef{
//	    Name: "users",
//	    Providers: []grove.Provider{
//	        grove.Provide0(di.Singleton, NewUsersService),
//	    },
//	    Build: grove.Build(NewUsersController, func(ctrl *UsersController) grove.ControllerDef {
//	        return grove.ControllerDef{
//	            Prefix: "/users",
//	            Endpoints: []grove.Endpoint{
//	                grove.GET("", ctrl.List),
//	            },
//	        }
//	    }),
//	}
//
// Build2/3/4 cover constructors with more dependencies (see Wire2/3/4).
func Build[T any, C any](ctor func(T) C, define func(C) ControllerDef) func(*ModuleContext) error {
	return BuildWith(ctor, func(_ *ModuleContext, ctrl C) (ControllerDef, error) {
		return define(ctrl), nil
	})
}

// Build2 is Build for two-dependency constructors.
func Build2[T1 any, T2 any, C any](ctor func(T1, T2) C, define func(C) ControllerDef) func(*ModuleContext) error {
	return BuildWith2(ctor, func(_ *ModuleContext, ctrl C) (ControllerDef, error) {
		return define(ctrl), nil
	})
}

// Build3 is Build for three-dependency constructors.
func Build3[T1 any, T2 any, T3 any, C any](ctor func(T1, T2, T3) C, define func(C) ControllerDef) func(*ModuleContext) error {
	return BuildWith3(ctor, func(_ *ModuleContext, ctrl C) (ControllerDef, error) {
		return define(ctrl), nil
	})
}

// Build4 is Build for four-dependency constructors.
func Build4[T1 any, T2 any, T3 any, T4 any, C any](ctor func(T1, T2, T3, T4) C, define func(C) ControllerDef) func(*ModuleContext) error {
	return BuildWith4(ctor, func(_ *ModuleContext, ctrl C) (ControllerDef, error) {
		return define(ctrl), nil
	})
}

// BuildWith is Build for route tables that need the module context too,
// such as resolving a guard from the container before building the
// ControllerDef:
//
//	Build: grove.BuildWith(NewUsersController, func(m *grove.ModuleContext, ctrl *UsersController) (grove.ControllerDef, error) {
//	    guard, err := auth.Guard(m.App.Container)
//	    if err != nil {
//	        return grove.ControllerDef{}, err
//	    }
//	    return grove.ControllerDef{Middleware: []router.Middleware{guard}, ...}, nil
//	}),
//
// BuildWith2/3/4 cover constructors with more dependencies.
func BuildWith[T any, C any](ctor func(T) C, define func(*ModuleContext, C) (ControllerDef, error)) func(*ModuleContext) error {
	return func(m *ModuleContext) error {
		ctrl, err := Wire(m, ctor)
		if err != nil {
			return err
		}
		def, err := define(m, ctrl)
		if err != nil {
			return err
		}
		m.Controllers(def)
		return nil
	}
}

// BuildWith2 is BuildWith for two-dependency constructors.
func BuildWith2[T1 any, T2 any, C any](ctor func(T1, T2) C, define func(*ModuleContext, C) (ControllerDef, error)) func(*ModuleContext) error {
	return func(m *ModuleContext) error {
		ctrl, err := Wire2(m, ctor)
		if err != nil {
			return err
		}
		def, err := define(m, ctrl)
		if err != nil {
			return err
		}
		m.Controllers(def)
		return nil
	}
}

// BuildWith3 is BuildWith for three-dependency constructors.
func BuildWith3[T1 any, T2 any, T3 any, C any](ctor func(T1, T2, T3) C, define func(*ModuleContext, C) (ControllerDef, error)) func(*ModuleContext) error {
	return func(m *ModuleContext) error {
		ctrl, err := Wire3(m, ctor)
		if err != nil {
			return err
		}
		def, err := define(m, ctrl)
		if err != nil {
			return err
		}
		m.Controllers(def)
		return nil
	}
}

// BuildWith4 is BuildWith for four-dependency constructors.
func BuildWith4[T1 any, T2 any, T3 any, T4 any, C any](ctor func(T1, T2, T3, T4) C, define func(*ModuleContext, C) (ControllerDef, error)) func(*ModuleContext) error {
	return func(m *ModuleContext) error {
		ctrl, err := Wire4(m, ctor)
		if err != nil {
			return err
		}
		def, err := define(m, ctrl)
		if err != nil {
			return err
		}
		m.Controllers(def)
		return nil
	}
}

// InjectNamed resolves the explicit container key for T, the escape
// hatch when the default di.KeyFor[T] key is not the registration key:
//
//	svc, err := grove.InjectNamed[*UsersService](app, "readDB")
func InjectNamed[T any](r Resolver, name string) (T, error) {
	var zero T
	v, err := r.Resolve(name)
	if err != nil {
		return zero, err
	}
	typed, ok := v.(T)
	if !ok {
		return zero, fmt.Errorf("grove: registration for %q is %T, not %s", name, v, di.KeyFor[T]())
	}
	return typed, nil
}

// MustInjectNamed is like InjectNamed but panics on error.
func MustInjectNamed[T any](r Resolver, name string) T {
	v, err := InjectNamed[T](r, name)
	if err != nil {
		panic(err)
	}
	return v
}

// ResolveNamed resolves the explicit container key name and type-asserts
// to T:
//
//	svc, err := grove.ResolveNamed[*UsersService](app.Container, "readDB")
func ResolveNamed[T any](r Resolver, name string) (T, error) {
	var zero T
	v, err := r.Resolve(name)
	if err != nil {
		return zero, err
	}
	typed, ok := v.(T)
	if !ok {
		return zero, fmt.Errorf("grove: registration for %q is %T, not %s", name, v, di.KeyFor[T]())
	}
	return typed, nil
}

// MustResolveNamed is like ResolveNamed but panics on error.
func MustResolveNamed[T any](r Resolver, name string) T {
	v, err := ResolveNamed[T](r, name)
	if err != nil {
		panic(err)
	}
	return v
}
