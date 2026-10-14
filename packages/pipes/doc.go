// Package pipes validates and transforms request data, the Go equivalent
// of NestJS pipes. Validation runs off struct tags with a dependency-free
// engine, and failures surface as 422 HttpErrors the router maps
// automatically.
//
// The official form is declarative: grove.HandleBody* for bodies plus
// pipes.HandleQuery, pipes.HandlePath, and pipes.HandleBodyQuery for
// params (bind + strict-validate + dispatch, status in one place).
// Body[T], BindQuery, BindHeader, and BindPath stay as the manual
// escape hatch for handlers needing raw values mid-logic.
package pipes
