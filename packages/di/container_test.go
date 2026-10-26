package di

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSingletonReturnsSameInstance(t *testing.T) {
	c := New()
	type svc struct{ n int }
	if err := c.RegisterSingleton("svc", &svc{n: 1}); err != nil {
		t.Fatal(err)
	}
	a, err := c.Resolve("svc")
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.Resolve("svc")
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("singleton must return the same instance")
	}
}

func TestTransientInvokesFactoryEachTime(t *testing.T) {
	c := New()
	calls := 0
	if err := c.RegisterTransient("svc", func(c *Container) (any, error) {
		calls++
		return calls, nil
	}); err != nil {
		t.Fatal(err)
	}
	a, _ := c.Resolve("svc")
	b, _ := c.Resolve("svc")
	if a == b {
		t.Fatal("transient must invoke the factory on every resolve")
	}
	if calls != 2 {
		t.Fatalf("want 2 factory calls, got %d", calls)
	}
}

func TestConstructorInjection(t *testing.T) {
	c := New()
	type db struct{ dsn string }
	type svc struct{ db *db }
	c.MustRegisterSingleton("db", &db{dsn: "postgres://localhost/app"})
	c.MustRegisterTransient("svc", func(c *Container) (any, error) {
		dep, err := c.Resolve("db")
		if err != nil {
			return nil, err
		}
		return &svc{db: dep.(*db)}, nil
	})
	v, err := c.Resolve("svc")
	if err != nil {
		t.Fatal(err)
	}
	if v.(*svc).db.dsn != "postgres://localhost/app" {
		t.Fatal("constructor injection did not resolve the dependency")
	}
}

func TestDuplicateRegistrationFails(t *testing.T) {
	c := New()
	c.MustRegisterSingleton("a", 1)
	if err := c.RegisterSingleton("a", 2); err == nil {
		t.Fatal("expected duplicate registration error")
	}
	if err := c.RegisterTransient("a", func(c *Container) (any, error) { return 1, nil }); err == nil {
		t.Fatal("expected duplicate registration error")
	}
}

func TestResolveMissingFails(t *testing.T) {
	c := New()
	if _, err := c.Resolve("nope"); err == nil {
		t.Fatal("expected missing registration error")
	}
}

func TestCircularDependencyDetected(t *testing.T) {
	c := New()
	c.MustRegisterTransient("a", func(c *Container) (any, error) { return c.Resolve("b") })
	c.MustRegisterTransient("b", func(c *Container) (any, error) { return c.Resolve("a") })
	if _, err := c.Resolve("a"); err == nil {
		t.Fatal("expected circular dependency error")
	}
}

func TestFactoryErrorPropagates(t *testing.T) {
	c := New()
	sentinel := errors.New("boom")
	c.MustRegisterTransient("bad", func(c *Container) (any, error) { return nil, sentinel })
	if _, err := c.Resolve("bad"); !errors.Is(err, sentinel) {
		t.Fatalf("expected %v, got %v", sentinel, err)
	}
}

func TestGenericHelpers(t *testing.T) {
	c := New()
	type repo struct{ dsn string }
	if err := RegisterSingletonAs(c, &repo{dsn: "x"}); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveAs[*repo](c)
	if err != nil {
		t.Fatal(err)
	}
	if got.dsn != "x" {
		t.Fatal("generic resolve returned wrong value")
	}
	if err := RegisterTransientAs(c, func(c *Container) (int, error) { return 42, nil }); err != nil {
		t.Fatal(err)
	}
	n, err := ResolveAs[int](c)
	if err != nil || n != 42 {
		t.Fatalf("generic transient resolve = %d, %v", n, err)
	}
}

// TestConcurrentResolveSameLazySingleton guards the P0 fix: concurrent
// first-resolves of the same lazy singleton must not report a false
// cycle and must coalesce through singleflight (factory runs once).
func TestConcurrentResolveSameLazySingleton(t *testing.T) {
	c := New()
	type svcInst struct{}
	var calls int32
	c.MustRegisterSingletonFactory("svc", func(c *Container) (any, error) {
		if atomic.AddInt32(&calls, 1) == 1 {
			time.Sleep(5 * time.Millisecond) // widen the race window
		}
		return &svcInst{}, nil
	})
	const n = 20
	results := make([]error, n)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := c.Resolve("svc")
			mu.Lock()
			results[i] = err
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	for i, err := range results {
		if err != nil {
			t.Errorf("resolve %d failed: %v", i, err)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("factory should run exactly once, got %d", got)
	}
}

// TestPanicInFactoryDoesNotWedge guards the P0 fix: a factory panic is
// returned as an error and does not wedge the key for later resolves.
func TestPanicInFactoryDoesNotWedge(t *testing.T) {
	c := New()
	c.MustRegisterSingletonFactory("boom", func(c *Container) (any, error) {
		panic("kaboom")
	})
	if _, err := c.Resolve("boom"); err == nil {
		t.Fatal("expected panic propagated as error")
	}
	// A subsequent resolve must retry (not block forever on a closed
	// singleflight channel).
	if _, err := c.Resolve("boom"); err == nil {
		t.Fatal("expected panic propagated as error on retry")
	}
}

// TestScopeSingleflightScopedFactory guards the P0 fix: concurrent
// first-resolves of the same request-scoped key within one Scope must
// coalesce so the factory runs exactly once (not once per goroutine).
func TestScopeSingleflightScopedFactory(t *testing.T) {
	c := New()
	type svc struct{}
	var calls int32
	c.MustRegisterScoped("svc", func(c *Container, ctx context.Context) (any, error) {
		if atomic.AddInt32(&calls, 1) == 1 {
			time.Sleep(5 * time.Millisecond) // widen the race window
		}
		return &svc{}, nil
	})
	s := c.Scope(context.Background())
	const n = 20
	results := make([]error, n)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := s.Resolve("svc")
			mu.Lock()
			results[i] = err
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	for i, err := range results {
		if err != nil {
			t.Errorf("resolve %d failed: %v", i, err)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("scoped factory should run exactly once per Scope, got %d", got)
	}
}
