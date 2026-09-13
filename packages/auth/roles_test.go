package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/brandonbert8/grove/packages/router"
)

func authedRouter(t *testing.T, svc *Service, mw router.Middleware) *router.DefaultRouter {
	t.Helper()
	r := router.New()
	r.GET("/admin", func(c router.Context) error { return c.NoContent(204) }, AuthGuard(svc), mw)
	return r
}

func tokenFor(t *testing.T, svc *Service, sub string, roles any) string {
	t.Helper()
	tok, err := svc.Sign(sub, map[string]any{"roles": roles})
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func get(t *testing.T, r *router.DefaultRouter, token string) int {
	t.Helper()
	req := httptest.NewRequest("GET", "/admin", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec.Code
}

func TestRequireRoleShapes(t *testing.T) {
	svc := NewService([]byte("test-secret-1234567890"), "test")
	r := authedRouter(t, svc, RequireRole("admin"))

	cases := []struct {
		name  string
		roles any
		want  int
	}{
		{"json array", []any{"admin", "user"}, 204},
		{"string slice", []string{"admin"}, 204},
		{"single string", "admin", 204},
		{"wrong role", []string{"user"}, 403},
		{"no roles", nil, 403},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			extra := map[string]any{}
			if tc.roles != nil {
				extra["roles"] = tc.roles
			}
			tok, err := svc.Sign("u", extra)
			if err != nil {
				t.Fatal(err)
			}
			if got := get(t, r, tok); got != tc.want {
				t.Fatalf("status = %d, want %d", got, tc.want)
			}
		})
	}
	if got := get(t, r, ""); got != http.StatusUnauthorized {
		t.Fatalf("missing token = %d, want 401", got)
	}
}

func TestRequireAnyRole(t *testing.T) {
	svc := NewService([]byte("test-secret-1234567890"), "test")
	r := authedRouter(t, svc, RequireAnyRole("admin", "ops"))
	if got := get(t, r, tokenFor(t, svc, "u", []string{"ops"})); got != 204 {
		t.Fatalf("ops = %d, want 204", got)
	}
	if got := get(t, r, tokenFor(t, svc, "u", []string{"viewer"})); got != http.StatusForbidden {
		t.Fatalf("viewer = %d, want 403", got)
	}
}
