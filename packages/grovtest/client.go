package grovtest

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	grove "github.com/brandonbert8/grove/packages/core"
)

// Client drives an App's Handler with per-client default headers:
//
//	cli := grovtest.New(app).Bearer(token)
//	rec := cli.Post(t, "/users", map[string]string{"name": "Ada"})
//	grovtest.RequireStatus(t, rec, 201)
type Client struct {
	app     *grove.App
	headers map[string]string
}

// New returns a Client for app.
func New(app *grove.App) *Client {
	return &Client{app: app, headers: map[string]string{}}
}

// Header sets a default header for every request from this client.
func (c *Client) Header(k, v string) *Client {
	c.headers[k] = v
	return c
}

// Bearer sets Authorization: Bearer <token> for every request.
func (c *Client) Bearer(token string) *Client {
	return c.Header("Authorization", "Bearer "+token)
}

// Do issues one request: a non-nil body is marshalled as JSON with the
// matching Content-Type. Extra headers merge over client defaults.
func (c *Client) Do(t testing.TB, method, path string, body any, headers ...map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("grovtest: marshal body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	for _, h := range headers {
		for k, v := range h {
			req.Header.Set(k, v)
		}
	}
	rec := httptest.NewRecorder()
	c.app.Handler().ServeHTTP(rec, req)
	return rec
}

// Get issues a GET request.
func (c *Client) Get(t testing.TB, path string, headers ...map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	return c.Do(t, "GET", path, nil, headers...)
}

// Post issues a POST request with a JSON body.
func (c *Client) Post(t testing.TB, path string, body any, headers ...map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	return c.Do(t, "POST", path, body, headers...)
}

// Put issues a PUT request with a JSON body.
func (c *Client) Put(t testing.TB, path string, body any, headers ...map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	return c.Do(t, "PUT", path, body, headers...)
}

// Patch issues a PATCH request with a JSON body.
func (c *Client) Patch(t testing.TB, path string, body any, headers ...map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	return c.Do(t, "PATCH", path, body, headers...)
}

// Delete issues a DELETE request.
func (c *Client) Delete(t testing.TB, path string, headers ...map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	return c.Do(t, "DELETE", path, nil, headers...)
}

// Decode unmarshals a JSON response body into T, failing the test on
// malformed payloads (so tests never assert on zero values by accident).
func Decode[T any](t testing.TB, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("grovtest: decode response: %v (body %q)", err, rec.Body.String())
	}
	return v
}

// RequireStatus fails the test unless the response carries want,
// printing the body for debuggability.
func RequireStatus(t testing.TB, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("grovtest: status = %d, want %d (body %q)", rec.Code, want, rec.Body.String())
	}
}
