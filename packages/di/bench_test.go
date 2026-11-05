package di

import "testing"

type benchService struct{ Value int }

func BenchmarkResolveSingleton(b *testing.B) {
	c := New()
	if err := RegisterSingletonAs(c, &benchService{Value: 1}); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ResolveAs[*benchService](c); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkResolveLazySingleton(b *testing.B) {
	c := New()
	if err := c.RegisterSingletonFactory(KeyFor[*benchService](), func(*Container) (any, error) {
		return &benchService{Value: 1}, nil
	}); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ResolveAs[*benchService](c); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkResolveTransient(b *testing.B) {
	c := New()
	if err := RegisterTransientAs(c, func(*Container) (*benchService, error) {
		return &benchService{Value: 1}, nil
	}); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ResolveAs[*benchService](c); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkKeyFor(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = KeyFor[*benchService]()
	}
}
