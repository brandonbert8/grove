package di

import (
	"context"
	"sync/atomic"
	"testing"
)

type reqCtx struct{ id int64 }

func TestRequestScopedCachesPerScope(t *testing.T) {
	c := New()
	var builds atomic.Int64
	if err := RegisterScopedAs(c, func(c *Container, ctx context.Context) (*reqCtx, error) {
		return &reqCtx{id: builds.Add(1)}, nil
	}); err != nil {
		t.Fatal(err)
	}
	s1 := c.Scope(context.Background())
	a, err := ResolveScoped[*reqCtx](s1)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ResolveScoped[*reqCtx](s1)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("same scope must return same instance")
	}
	s2 := c.Scope(context.Background())
	d, err := ResolveScoped[*reqCtx](s2)
	if err != nil {
		t.Fatal(err)
	}
	if d == a {
		t.Fatal("different scopes must return different instances")
	}
	if builds.Load() != 2 {
		t.Fatalf("want 2 builds, got %d", builds.Load())
	}
}

func TestRequestScopedRejectsRootResolve(t *testing.T) {
	c := New()
	_ = c.RegisterScoped("x", func(c *Container, ctx context.Context) (any, error) { return 1, nil })
	if _, err := c.Resolve("x"); err == nil {
		t.Fatal("root Resolve of scoped provider must fail with guidance")
	}
}

type closable struct{ closed *atomic.Bool }

func (c *closable) Close() error { c.closed.Store(true); return nil }

func TestScopeCloseReleasesClosers(t *testing.T) {
	c := New()
	var closed atomic.Bool
	_ = c.RegisterScoped("w", func(c *Container, ctx context.Context) (any, error) {
		return &closable{closed: &closed}, nil
	})
	s := c.Scope(context.Background())
	if _, err := s.Resolve("w"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if !closed.Load() {
		t.Fatal("Close must release scoped io.Closers")
	}
}

func TestScopeDelegatesSingleton(t *testing.T) {
	c := New()
	if err := c.RegisterSingleton("s", "shared"); err != nil {
		t.Fatal(err)
	}
	s := c.Scope(context.Background())
	v, err := s.Resolve("s")
	if err != nil {
		t.Fatal(err)
	}
	if v != "shared" {
		t.Fatalf("want shared, got %v", v)
	}
}
