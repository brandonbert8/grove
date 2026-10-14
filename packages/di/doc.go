// Package di provides a lightweight dependency injection container.
//
// The container supports singleton, transient, and request-scoped
// lifetimes with explicit constructor injection. A tiny amount of
// reflection is used only to derive default keys from generic type
// parameters; it never inspects struct fields or calls constructors
// magically. v1 is runtime-only on purpose: Scope carries the
// per-request cache (NestJS Scope.REQUEST equivalent).
package di
