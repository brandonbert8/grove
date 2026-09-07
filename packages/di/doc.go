// Package di provides a lightweight dependency injection container.
//
// The container supports singleton and transient lifetimes with explicit
// constructor injection. A tiny amount of reflection is used only to
// derive default keys from generic type parameters; it never inspects
// struct fields or calls constructors magically. This keeps the door open
// for compile-time code generation to replace runtime resolution later.
package di
