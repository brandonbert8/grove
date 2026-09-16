package router

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	chi "github.com/go-chi/chi/v5"
)

// errNilBodyTarget and errEmptyBody describe Body misuse.
var (
	errNilBodyTarget = errors.New("router: Body target must not be nil")
	errEmptyBody     = errors.New("router: request body is empty")
)

// Context is the per-request abstraction passed to handlers.
//
// It exposes the underlying net/http primitives for escape hatches
// while providing convenience helpers for the common cases. Future
// transports (gRPC, WebSockets) will get their own context types;
// this one is HTTP-only by design.
type Context interface {
	// Request returns the incoming HTTP request.
	Request() *http.Request
	// ResponseWriter returns the underlying writer.
	ResponseWriter() http.ResponseWriter

	// Param returns the path parameter named name (e.g. "/users/{id}").
	Param(name string) string
	// Query returns the first value of query parameter name.
	Query(name string) string

	// QueryOr returns the query value or fallback when absent.
	QueryOr(name, fallback string) string

	// Header returns the first value of request header name.
	Header(name string) string

	// Cookie returns the named request cookie or an error.
	Cookie(name string) (*http.Cookie, error)
	// SetCookie writes a Set-Cookie header.
	SetCookie(c *http.Cookie)

	// Body decodes the JSON request body into v. It caps the read at
	// 1 MiB and rejects empty bodies; validation belongs in a pipe
	// (see packages/pipes), not here.
	Body(v any) error
	// BodyWithLimit is Body with a custom byte cap.
	BodyWithLimit(v any, limit int64) error

	// Status stashes the status code; the next JSON/String/NoContent
	// call writes it (a single WriteHeader per response).
	Status(code int) Context
	// JSON writes v as application/json with the given status code.
	JSON(code int, v any) error
	// String writes s as text/plain with the given status code.
	String(code int, s string) error
	// NoContent writes a status code with an empty body.
	NoContent(code int) error
	// Redirect issues a Location redirect.
	Redirect(code int, url string) error
}

// httpContext is the default Context implementation.
type httpContext struct {
	w http.ResponseWriter
	r *http.Request
	// status stashes Status() until a body writer flushes it, so
	// Status(201).JSON(201, v) emits exactly one WriteHeader.
	status    int
	statusSet bool
}

// NewContext wraps w and r in a Context.
func NewContext(w http.ResponseWriter, r *http.Request) Context {
	return &httpContext{w: w, r: r}
}

// Request returns the incoming HTTP request.
func (c *httpContext) Request() *http.Request { return c.r }

// SetRequest swaps the underlying request (used by guards to attach
// auth context without mutating a shared *http.Request in place).
func (c *httpContext) SetRequest(r *http.Request) {
	if r != nil {
		c.r = r
	}
}

// ResponseWriter returns the underlying writer.
func (c *httpContext) ResponseWriter() http.ResponseWriter { return c.w }

// Param returns the path parameter named name.
//
// Parameters are matched by Chi (e.g. "/users/{id}"). The lookup falls
// back to net/http's PathValue so contexts built outside the mux (unit
// tests with httptest) keep working.
func (c *httpContext) Param(name string) string {
	if v := chi.URLParam(c.r, name); v != "" {
		return v
	}
	return c.r.PathValue(name)
}

// Query returns the first value of query parameter name.
func (c *httpContext) Query(name string) string { return c.r.URL.Query().Get(name) }

// QueryOr returns the query value or fallback when absent.
// Presence is exact: ?q= is present-but-empty (returns ""), only a
// missing key falls back — so empty searches don't silently become defaults.
func (c *httpContext) QueryOr(name, fallback string) string {
	if !c.r.URL.Query().Has(name) {
		return fallback
	}
	return c.r.URL.Query().Get(name)
}

// Header returns the first value of request header name.
func (c *httpContext) Header(name string) string { return c.r.Header.Get(name) }

// Cookie returns the named request cookie.
func (c *httpContext) Cookie(name string) (*http.Cookie, error) { return c.r.Cookie(name) }

// SetCookie writes a Set-Cookie header.
func (c *httpContext) SetCookie(cookie *http.Cookie) {
	if cookie != nil {
		http.SetCookie(c.w, cookie)
	}
}

// maxBodyBytes caps JSON body reads to prevent abuse.
const maxBodyBytes = 1 << 20

// DefaultMaxBody exposes the default JSON cap for pipes/config.
const DefaultMaxBody = int64(maxBodyBytes)

// Body decodes the JSON request body into v.
func (c *httpContext) Body(v any) error { return c.BodyWithLimit(v, maxBodyBytes) }

// BodyWithLimit is Body with a custom byte cap.
func (c *httpContext) BodyWithLimit(v any, limit int64) error {
	if v == nil {
		return errNilBodyTarget
	}
	if c.r.Body == nil {
		return errEmptyBody
	}
	if limit <= 0 {
		limit = maxBodyBytes
	}
	dec := json.NewDecoder(http.MaxBytesReader(c.w, c.r.Body, limit))
	if err := dec.Decode(v); err != nil {
		return err
	}
	// Reject trailing garbage: {"a":1} garbage / {} {} must fail
	// instead of silently validating only the first value.
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errors.New("router: unexpected data after JSON body")
	}
	return nil
}

// Status stashes the status code header; the next JSON/String/NoContent
// call flushes it (its own code wins when Status was not called).
// Nothing is written until a body writer runs, so chained calls emit a
// single WriteHeader.
func (c *httpContext) Status(code int) Context {
	c.status, c.statusSet = code, true
	return c
}

// resolveStatus returns the stashed status when set, else fallback.
func (c *httpContext) resolveStatus(fallback int) int {
	if c.statusSet {
		return c.status
	}
	return fallback
}

// JSON writes v as application/json with the given status code.
func (c *httpContext) JSON(code int, v any) error {
	code = c.resolveStatus(code)
	c.statusSet = false
	c.w.Header().Set("Content-Type", "application/json")
	c.w.WriteHeader(code)
	return json.NewEncoder(c.w).Encode(v)
}

// String writes s as text/plain with the given status code.
func (c *httpContext) String(code int, s string) error {
	code = c.resolveStatus(code)
	c.statusSet = false
	c.w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	c.w.WriteHeader(code)
	_, err := c.w.Write([]byte(s))
	return err
}

// NoContent writes a status code with an empty body.
func (c *httpContext) NoContent(code int) error {
	code = c.resolveStatus(code)
	c.statusSet = false
	c.w.WriteHeader(code)
	return nil
}

// Redirect issues a Location redirect.
func (c *httpContext) Redirect(code int, url string) error {
	if code < 300 || code > 308 {
		code = http.StatusFound
	}
	http.Redirect(c.w, c.r, url, code)
	return nil
}
