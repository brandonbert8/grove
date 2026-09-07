// Package grove is the core of the Grove framework.
//
// It owns the Application object, module registration, bootstrapping,
// and lifecycle management. Transport concerns (HTTP routing, gRPC,
// WebSockets) live in their own packages and are composed here so they
// stay replaceable.
package grove
