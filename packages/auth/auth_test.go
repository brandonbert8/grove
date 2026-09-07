package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/brandonbert8/grove/packages/router"
)

func testService() *Service {
	return NewService([]byte("test-secret-12345"), "test", WithTTL(time.Hour))
}

func TestSignVerifyRoundtrip(t *testing.T) {
	svc := testService()
	token, err := svc.Sign("user-1", map[string]any{"roles": []any{"admin"}})
	if err != nil {
		t.Fatal(err)
	}
	claims, err := svc.Verify(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "user-1" {
		t.Fatalf("subject = %q", claims.Subject)
	}
	if !claims.Valid() {
		t.Fatal("claims should be valid")
	}
}

func TestVerifyTamperedFails(t *testing.T) {
	svc := testService()
	token, _ := svc.Sign("user-1", nil)
	parts := strings.Split(token, ".")
	parts[1] = parts[1][:len(parts[1])-2] + "xx"
	if _, err := svc.Verify(strings.Join(parts, ".")); err == nil {
		t.Fatal("expected tampered token to fail")
	}
	other := NewService([]byte("different-secret"), "test")
	if _, err := other.Verify(token); err == nil {
		t.Fatal("expected wrong-secret verification to fail")
	}
}

func TestVerifyExpiredFails(t *testing.T) {
	svc := NewService([]byte("test-secret-12345"), "test", WithTTL(-time.Hour))
	token, err := svc.Sign("user-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Verify(token); err == nil {
		t.Fatal("expected expired token to fail")
	}
}

func TestEmptySecretPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on empty secret")
		}
	}()
	NewService(nil, "test")
}

func serveWith(t *testing.T, mw router.Middleware, h router.HandlerFunc, header string) int {
	t.Helper()
	r := router.New()
	r.GET("/x", h, mw)
	req := httptest.NewRequest("GET", "/x", nil)
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec.Code
}

func TestAuthGuard(t *testing.T) {
	svc := testService()
	ok := func(c router.Context) error {
		claims, found := CurrentUser(c)
		if !found || claims.Subject != "user-1" {
			t.Errorf("CurrentUser missing inside handler")
		}
		return c.NoContent(200)
	}
	if code := serveWith(t, AuthGuard(svc), ok, ""); code != http.StatusUnauthorized {
		t.Fatalf("no token = %d, want 401", code)
	}
	if code := serveWith(t, AuthGuard(svc), ok, "Bearer junk"); code != http.StatusUnauthorized {
		t.Fatalf("bad token = %d, want 401", code)
	}
	token, _ := svc.Sign("user-1", nil)
	if code := serveWith(t, AuthGuard(svc), ok, "Bearer "+token); code != http.StatusOK {
		t.Fatalf("good token = %d, want 200", code)
	}
}

func TestRequireRole(t *testing.T) {
	svc := testService()
	chain := func(next router.HandlerFunc) router.HandlerFunc {
		return AuthGuard(svc)(RequireRole("admin")(next))
	}
	ok := func(c router.Context) error { return c.NoContent(200) }

	userToken, _ := svc.Sign("u", map[string]any{"roles": []any{"user"}})
	if code := serveWith(t, chain, ok, "Bearer "+userToken); code != http.StatusForbidden {
		t.Fatalf("user role = %d, want 403", code)
	}
	adminToken, _ := svc.Sign("a", map[string]any{"roles": []any{"admin"}})
	if code := serveWith(t, chain, ok, "Bearer "+adminToken); code != http.StatusOK {
		t.Fatalf("admin role = %d, want 200", code)
	}
}
