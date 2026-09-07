package di

import (
	"errors"
	"testing"
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
