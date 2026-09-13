package pipes

import (
	"fmt"
	"reflect"

	grove "github.com/brandonbert8/grove/packages/core"
	"github.com/brandonbert8/grove/packages/router"
)

// Value is the set of scalar kinds coercible from path/query strings —
// the ParseIntPipe/ParseBoolPipe/ParseUUIDPipe equivalent, but generic:
// one function instead of one pipe per type.
type Value interface {
	~string | ~bool |
		~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64
}

// Parse coerces s into T, reporting overflow and malformed input.
// It shares the engine with BindQuery, so query-tested shapes behave
// identically in path params.
func Parse[T Value](s string) (T, error) {
	var zero T
	rv := reflect.ValueOf(&zero).Elem()
	if !setScalar(rv, s) {
		return zero, fmt.Errorf("pipes: cannot parse %q as %s", s, typeName[T]())
	}
	return zero, nil
}

// Path reads the Chi path parameter name and coerces it into T,
// returning a 400 *grove.HttpError when absent or unparsable:
//
//	page, err := pipes.Path[int](c, "page")
func Path[T Value](c router.Context, name string) (T, error) {
	var zero T
	raw := c.Param(name)
	if raw == "" {
		return zero, grove.BadRequest(fmt.Sprintf("missing path parameter %q", name))
	}
	v, err := Parse[T](raw)
	if err != nil {
		return zero, grove.BadRequest(
			fmt.Sprintf("invalid path parameter %q: want %s", name, typeName[T]()))
	}
	return v, nil
}

// Query reads query parameter name with a DefaultValuePipe-style
// fallback: absent means fallback (no error); present-but-malformed is
// a 400 *grove.HttpError:
//
//	limit, err := pipes.Query(c, "limit", 20)
func Query[T Value](c router.Context, name string, fallback T) (T, error) {
	raw := c.Query(name)
	if raw == "" {
		return fallback, nil
	}
	v, err := Parse[T](raw)
	if err != nil {
		var zero T
		return zero, grove.BadRequest(
			fmt.Sprintf("invalid query parameter %q: want %s", name, typeName[T]()))
	}
	return v, nil
}

// typeName renders T for error messages.
func typeName[T Value]() string {
	var zero T
	if t := reflect.TypeOf(zero); t != nil {
		return t.String()
	}
	return "value"
}
