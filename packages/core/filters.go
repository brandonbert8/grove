package grove

import (
	"github.com/brandonbert8/grove/packages/router"
)

// ExceptionFilter handles handler errors (NestJS @Catch equivalent).
//
// Return nil when the error was handled (response already written).
// Return a non-nil error to fall through to the next filter / default
// Grove JSON mapping. Filters run outermost-first: global, then
// controller middleware, then route.
type ExceptionFilter interface {
	Catch(c router.Context, err error) error
}

// FilterFunc adapts a plain function into an ExceptionFilter.
type FilterFunc func(c router.Context, err error) error

// Catch implements ExceptionFilter.
func (f FilterFunc) Catch(c router.Context, err error) error { return f(c, err) }

// UseFilters registers global exception filters (APP_FILTER equivalent).
func (a *App) UseFilters(filters ...ExceptionFilter) {
	for _, f := range filters {
		if f != nil {
			a.filters = append(a.filters, f)
		}
	}
	if dr, ok := a.Router.(interface{ SetErrorHandler(func(router.Context, error)) }); ok {
		dr.SetErrorHandler(a.handleError)
	}
}

// handleError runs filters then falls back to the router default.
func (a *App) handleError(c router.Context, err error) {
	for _, f := range a.filters {
		if ferr := f.Catch(c, err); ferr == nil {
			return
		} else {
			err = ferr
		}
	}
	router.WriteError(c, err)
}

// filterMiddleware ensures controller-scoped routes also pass errors
// through global filters even on custom Router implementations without
// a SetErrorHandler hook.
func (a *App) filterMiddleware() router.Middleware {
	return func(next router.HandlerFunc) router.HandlerFunc {
		return func(c router.Context) error {
			if err := next(c); err != nil {
				a.handleError(c, err)
				return nil
			}
			return nil
		}
	}
}
