package auth

import "testing"

// FuzzVerify ensures malformed or adversarial tokens never panic the
// verifier or the refresh-token path.
func FuzzVerify(f *testing.F) {
	svc := NewService([]byte("fuzz-secret"), "fuzz")
	f.Add("")
	f.Add("a.b.c")
	f.Add("eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.e30.x")
	f.Add("eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.e30.")
	f.Fuzz(func(t *testing.T, token string) {
		_, _ = svc.Verify(token)
		_, _ = svc.VerifyRefresh(token)
	})
}
