package grove

import (
	"github.com/brandonbert8/grove/packages/di"
)

// Inject resolves the default key for T from the app container, the Go
// equivalent of NestJS constructor injection:
//
//	svc, err := grove.Inject[*UsersService](app)
//	if err != nil { return nil, err }
func Inject[T any](app *App) (T, error) {
	return di.ResolveAs[T](app.Container)
}

// MustInject is like Inject but panics on error. Useful in
// BuildControllers closures where a missing provider is a programming
// error:
//
//	svc := grove.MustInject[*UsersService](app)
func MustInject[T any](app *App) T {
	v, err := di.ResolveAs[T](app.Container)
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
func ControllerFor[T any](app *App, define func(T) ControllerDef) (ControllerDef, error) {
	ctrl, err := di.ResolveAs[T](app.Container)
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
func Wire[T any, C any](app *App, ctor func(T) C) (C, error) {
	var zero C
	dep, err := di.ResolveAs[T](app.Container)
	if err != nil {
		return zero, err
	}
	return ctor(dep), nil
}

// Wire2 resolves two dependencies and applies ctor:
//
//	h, err := grove.Wire2(app, NewAuthController) // func(svc *Service, store *Store) *Controller
func Wire2[T1 any, T2 any, C any](app *App, ctor func(T1, T2) C) (C, error) {
	var zero C
	d1, err := di.ResolveAs[T1](app.Container)
	if err != nil {
		return zero, err
	}
	d2, err := di.ResolveAs[T2](app.Container)
	if err != nil {
		return zero, err
	}
	return ctor(d1, d2), nil
}

// Wire3 resolves three dependencies and applies ctor.
// For 6+ dependencies prefer the `grove wire` codegen (zero ResolveAs).
func Wire3[T1 any, T2 any, T3 any, C any](app *App, ctor func(T1, T2, T3) C) (C, error) {
	var zero C
	d1, err := di.ResolveAs[T1](app.Container)
	if err != nil {
		return zero, err
	}
	d2, err := di.ResolveAs[T2](app.Container)
	if err != nil {
		return zero, err
	}
	d3, err := di.ResolveAs[T3](app.Container)
	if err != nil {
		return zero, err
	}
	return ctor(d1, d2, d3), nil
}

// Wire4 resolves four dependencies and applies ctor.
func Wire4[T1 any, T2 any, T3 any, T4 any, C any](app *App, ctor func(T1, T2, T3, T4) C) (C, error) {
	var zero C
	d1, err := di.ResolveAs[T1](app.Container)
	if err != nil {
		return zero, err
	}
	d2, err := di.ResolveAs[T2](app.Container)
	if err != nil {
		return zero, err
	}
	d3, err := di.ResolveAs[T3](app.Container)
	if err != nil {
		return zero, err
	}
	d4, err := di.ResolveAs[T4](app.Container)
	if err != nil {
		return zero, err
	}
	return ctor(d1, d2, d3, d4), nil
}

// MustWire is like Wire but panics on error (for BuildControllers).
func MustWire[T any, C any](app *App, ctor func(T) C) C {
	c, err := Wire(app, ctor)
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
func Controllers[T any, C any](app *App, ctor func(T) C, define func(C) ControllerDef) ([]ControllerDef, error) {
	ctrl, err := Wire(app, ctor)
	if err != nil {
		return nil, err
	}
	return []ControllerDef{define(ctrl)}, nil
}

// Controllers2 is Controllers for two-dependency constructors
// (see Wire2).
func Controllers2[T1 any, T2 any, C any](app *App, ctor func(T1, T2) C, define func(C) ControllerDef) ([]ControllerDef, error) {
	ctrl, err := Wire2(app, ctor)
	if err != nil {
		return nil, err
	}
	return []ControllerDef{define(ctrl)}, nil
}
