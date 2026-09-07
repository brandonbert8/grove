// Package pipes validates and transforms request data, the Go equivalent
// of NestJS pipes. Validation runs off struct tags with a dependency-free
// engine, and failures surface as 422 HttpErrors the router maps
// automatically.
package pipes
