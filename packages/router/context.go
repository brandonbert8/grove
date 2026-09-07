package router

import (
	"encoding/json"
	"errors"
	"net/http"
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

	// Body decodes the JSON request body into v. It caps the read at
	// 1 MiB and rejects empty bodies; validation belongs in a pipe
	// (see packages/pipes), not here.
	Body(v any) error

	// Status writes the status code header.
	Status(code int) Context
	// JSON writes v as application/json with the given status code.
	JSON(code int, v any) error
	// String writes s as text/plain with the given status code.
	String(code int, s string) error
	// NoContent writes a status code with an empty body.
	NoContent(code int) error
}

// httpContext is the default Context implementation.
type httpContext struct {
	w http.ResponseWriter
	r *http.Request
}

// NewContext wraps w and r in a Context.
func NewContext(w http.ResponseWriter, r *http.Request) Context {
	return &httpContext{w: w, r: r}
}

// Request returns the incoming HTTP request.
func (c *httpContext) Request() *http.Request { return c.r }

// ResponseWriter returns the underlying writer.
func (c *httpContext) ResponseWriter() http.ResponseWriter { return c.w }

// Param returns the path parameter named name.
func (c *httpContext) Param(name string) string { return c.r.PathValue(name) }

// Query returns the first value of query parameter name.
func (c *httpContext) Query(name string) string { return c.r.URL.Query().Get(name) }

// QueryOr returns the query value or fallback when absent.
func (c *httpContext) QueryOr(name, fallback string) string {
	if v := c.r.URL.Query().Get(name); v != "" {
		return v
	}
	return fallback
}

// Header returns the first value of request header name.
func (c *httpContext) Header(name string) string { return c.r.Header.Get(name) }

// maxBodyBytes caps JSON body reads to prevent abuse.
const maxBodyBytes = 1 << 20

// Body decodes the JSON request body into v.
func (c *httpContext) Body(v any) error {
	if v == nil {
		return errNilBodyTarget
	}
	if c.r.Body == nil {
		return errEmptyBody
	}
	dec := json.NewDecoder(http.MaxBytesReader(c.w, c.r.Body, maxBodyBytes))
	if err := dec.Decode(v); err != nil {
		return err
	}
	return nil
}

// Status writes the status code header.
func (c *httpContext) Status(code int) Context {
	c.w.WriteHeader(code)
	return c
}

// JSON writes v as application/json with the given status code.
func (c *httpContext) JSON(code int, v any) error {
	c.w.Header().Set("Content-Type", "application/json")
	c.w.WriteHeader(code)
	return json.NewEncoder(c.w).Encode(v)
}

// String writes s as text/plain with the given status code.
func (c *httpContext) String(code int, s string) error {
	c.w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	c.w.WriteHeader(code)
	_, err := c.w.Write([]byte(s))
	return err
}

// NoContent writes a status code with an empty body.
func (c *httpContext) NoContent(code int) error {
	c.w.WriteHeader(code)
	return nil
}
