// Package router defines Grove's lightweight HTTP routing abstraction.
//
// The default implementation is a thin wrapper over the standard library
// net/http ServeMux (Go 1.22+ pattern syntax). The Router interface is
// deliberately small so it can be swapped for Fiber, chi, or a custom
// trie without touching application code.
package router
