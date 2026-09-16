package pipes

import (
	"fmt"
	"math"
	"reflect"
	"strconv"

	grove "github.com/brandonbert8/grove/packages/core"
	"github.com/brandonbert8/grove/packages/router"
)

// ValidateBody decodes the JSON body into v and validates it, returning
// *grove.HttpError failures the router maps automatically:
//
//	400 when the JSON is malformed or missing,
//	422 with field details when validation fails.
//
// It is the Go equivalent of NestJS's ValidationPipe for @Body() DTOs.
func ValidateBody[T any](c router.Context, v *T, opts ...bodyOption) error {
	if v == nil {
		return grove.Internal("pipes: ValidateBody target must not be nil")
	}
	if err := bodyInto(c, v, bodyOpts(opts)); err != nil {
		return grove.BadRequest("invalid request body: " + err.Error())
	}
	if ferrs := Validate(v); len(ferrs) > 0 {
		return grove.Unprocessable("", ferrs)
	}
	return nil
}

// Body decodes the JSON body into a new T and validates it, returning
// the value directly — the Go equivalent of NestJS `@Body() dto` with
// the ValidationPipe applied:
//
//	in, err := pipes.Body[CreateUserInput](ctx)
//	if err != nil {
//	    return err // 400 bad JSON, 422 failed validation (+ field details)
//	}
//
// v0.3: validation is strict by default (unknown rules and malformed
// params fail instead of passing). Use pipes.Lenient() to restore the
// v0.2 forward-compatible behavior for interop tags.
// Use pipes.WithMaxBody(n) to raise/lower the 1MiB default cap.
func Body[T any](c router.Context, opts ...bodyOption) (T, error) {
	var zero T
	v := new(T)
	o := bodyOpts(opts)
	var err error
	if o.lenient {
		err = ValidateBody(c, v, opts...)
	} else {
		err = ValidateBodyStrict(c, v, opts...)
	}
	if err != nil {
		return zero, err
	}
	return *v, nil
}

// bodyOption customizes Body.
type bodyOption func(*bodyOptions)

type bodyOptions struct {
	strict  bool
	lenient bool
	maxBody int64
}

// Strict validates with ValidateStrict: unknown rules and malformed
// rule parameters fail the request instead of passing.
//
// v0.3: this is now the default; Strict() is kept for explicitness.
//
//	in, err := pipes.Body[DTO](ctx, pipes.Strict())
func Strict() bodyOption { return func(o *bodyOptions) { o.strict = true } }

// Lenient restores v0.2 behavior: unknown rules pass silently.
func Lenient() bodyOption { return func(o *bodyOptions) { o.lenient = true } }

// WithMaxBody overrides the per-request JSON cap (default 1MiB).
func WithMaxBody(n int64) bodyOption {
	return func(o *bodyOptions) {
		if n > 0 {
			o.maxBody = n
		}
	}
}

func bodyOpts(opts []bodyOption) bodyOptions {
	var o bodyOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	return o
}

// ValidateBodyStrict is ValidateBody over ValidateStrict: unknown rules
// and malformed rule parameters fail the request instead of passing.
// Use it when DTO tags are contracts; default to ValidateBody otherwise.
func ValidateBodyStrict[T any](c router.Context, v *T, opts ...bodyOption) error {
	if v == nil {
		return grove.Internal("pipes: ValidateBodyStrict target must not be nil")
	}
	if err := bodyInto(c, v, bodyOpts(opts)); err != nil {
		return grove.BadRequest("invalid request body: " + err.Error())
	}
	if ferrs := ValidateStrict(v); len(ferrs) > 0 {
		return grove.Unprocessable("", ferrs)
	}
	return nil
}

// bodyInto decodes JSON honoring the per-request cap.
func bodyInto(c router.Context, v any, o bodyOptions) error {
	if o.maxBody > 0 {
		return c.BodyWithLimit(v, o.maxBody)
	}
	return c.Body(v)
}

// BindQuery maps URL query parameters onto v using `query` tags, then
// validates the result. Supported field kinds: string, bool, all int/uint
// widths, float32/64, and slices of those (repeated params). Fields
// without a `query` tag are left untouched.
//
// Malformed values are 400 *grove.HttpError (fail-closed, consistent
// with Query): `?limit=abc` never silently becomes zero.
//
//	type ListQuery struct {
//	    Search string `query:"q"`
//	    Limit  int    `query:"limit" validate:"gte=1,lte=100"`
//	}
//	var q ListQuery
//	if err := pipes.BindQuery(c, &q); err != nil { return err }
func BindQuery[T any](c router.Context, v *T) error {
	if v == nil {
		return grove.Internal("pipes: BindQuery target must not be nil")
	}
	rv := reflect.ValueOf(v).Elem()
	if rv.Kind() != reflect.Struct {
		return grove.Internal("pipes: BindQuery target must be a struct pointer")
	}
	values := c.Request().URL.Query()
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		key := f.Tag.Get("query")
		if key == "" || key == "-" {
			continue
		}
		if !f.IsExported() {
			return grove.Internal(fmt.Sprintf("pipes: BindQuery field %q has a query tag but is unexported", f.Name))
		}
		raw, ok := values[key]
		if !ok || len(raw) == 0 {
			continue
		}
		fv := rv.Field(i)
		if !fv.CanSet() {
			continue
		}
		if fv.Kind() == reflect.Slice {
			elem := f.Type.Elem()
			s := reflect.MakeSlice(f.Type, 0, len(raw))
			for _, r := range raw {
				ev := reflect.New(elem).Elem()
				if !setScalar(ev, r) {
					return grove.BadRequest(fmt.Sprintf(
						"value for query parameter %q is not a valid %s", key, elem.Kind()))
				}
				s = reflect.Append(s, ev)
			}
			fv.Set(s)
			continue
		}
		if !setScalar(fv, raw[0]) {
			return grove.BadRequest(fmt.Sprintf(
				"value for query parameter %q is not a valid %s", key, fv.Kind()))
		}
	}
	if ferrs := Validate(v); len(ferrs) > 0 {
		return grove.Unprocessable("", ferrs)
	}
	return nil
}

// BindHeader maps request headers onto v using `header` tags, then
// validates the result — the BindQuery twin for headers:
//
//	type TraceCtx struct {
//	    RequestID string `header:"X-Request-ID" validate:"required"`
//	    Retries   int    `header:"X-Retries"`
//	}
//
// Header names are canonicalized (case-insensitive per RFC 9110), absent
// headers leave the zero value, and malformed values are 400.
func BindHeader[T any](c router.Context, v *T) error {
	if v == nil {
		return grove.Internal("pipes: BindHeader target must not be nil")
	}
	rv := reflect.ValueOf(v).Elem()
	if rv.Kind() != reflect.Struct {
		return grove.Internal("pipes: BindHeader target must be a struct pointer")
	}
	h := c.Request().Header
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		key := f.Tag.Get("header")
		if key == "" || key == "-" {
			continue
		}
		if !f.IsExported() {
			return grove.Internal(fmt.Sprintf("pipes: BindHeader field %q has a header tag but is unexported", f.Name))
		}
		raw := h.Get(key)
		if raw == "" {
			continue
		}
		fv := rv.Field(i)
		if !fv.CanSet() {
			continue
		}
		if !setScalar(fv, raw) {
			return grove.BadRequest(fmt.Sprintf(
				"value for header %q is not a valid %s", key, fv.Kind()))
		}
	}
	if ferrs := Validate(v); len(ferrs) > 0 {
		return grove.Unprocessable("", ferrs)
	}
	return nil
}

// setScalar assigns s to v for basic kinds, reporting success.
func setScalar(v reflect.Value, s string) bool {
	return setField(v, s)
}

// setField assigns s to v, allocating through pointers so optional
// fields (*int, *string) bind. Nested pointer chains allocate once.
func setField(v reflect.Value, s string) bool {
	if !v.CanSet() {
		return false
	}
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
		if !v.CanSet() && v.CanAddr() {
			v = v.Addr().Elem()
		}
	}
	switch v.Kind() {
	case reflect.String:
		v.SetString(s)
		return true
	case reflect.Bool:
		b, err := strconv.ParseBool(s)
		if err != nil {
			return false
		}
		v.SetBool(b)
		return true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil || v.OverflowInt(n) {
			return false
		}
		v.SetInt(n)
		return true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(s, 10, 64)
		if err != nil || v.OverflowUint(n) {
			return false
		}
		v.SetUint(n)
		return true
	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(s, 64)
		if err != nil || v.OverflowFloat(n) {
			return false
		}
		// NaN/±Inf parse successfully but poison every comparison
		// (NaN fails gte/lte vacuously): reject them as invalid input.
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return false
		}
		v.SetFloat(n)
		return true
	default:
		return false
	}
}
