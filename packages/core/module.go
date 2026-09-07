package grove

// Module is Grove's unit of organization.
//
// A Module wires routes, providers, and sub-resources into the
// application. Keep modules cohesive (users, billing, health) and free
// of references to sibling modules; shared code belongs in packages the
// modules import, not in cross-module calls.
//
// Future framework features attach here: controllers register routes on
// app.Router, services register providers in app.Container, and guards /
// interceptors / pipes wrap routes as router.Middleware.
type Module interface {
	// Name identifies the module in logs and startup output.
	Name() string
	// Register wires the module into the application.
	Register(app *App) error
}

// ModuleFunc adapts a plain function into a Module.
type ModuleFunc struct {
	// ModuleName is returned by Name.
	ModuleName string
	// RegisterFn performs the wiring.
	RegisterFn func(app *App) error
}

// Name identifies the module.
func (m ModuleFunc) Name() string { return m.ModuleName }

// Register wires the module into the application.
func (m ModuleFunc) Register(app *App) error { return m.RegisterFn(app) }

// NewModule is shorthand for building a ModuleFunc.
func NewModule(name string, fn func(app *App) error) Module {
	return ModuleFunc{ModuleName: name, RegisterFn: fn}
}
