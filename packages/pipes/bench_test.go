package pipes

import "testing"

type benchSignup struct {
	Name  string `json:"name" validate:"required,min=2,max=80"`
	Email string `json:"email" validate:"required,email"`
	Age   int    `json:"age" validate:"gte=0,lte=150"`
}

func BenchmarkValidate(b *testing.B) {
	v := benchSignup{Name: "Ada Lovelace", Email: "ada@example.com", Age: 36}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if errs := Validate(v); len(errs) != 0 {
			b.Fatalf("unexpected errors: %v", errs)
		}
	}
}

func BenchmarkValidateStrict(b *testing.B) {
	v := benchSignup{Name: "Ada Lovelace", Email: "ada@example.com", Age: 36}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if errs := ValidateStrict(v); len(errs) != 0 {
			b.Fatalf("unexpected errors: %v", errs)
		}
	}
}
