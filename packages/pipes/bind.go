package pipes

import (
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
func ValidateBody[T any](c router.Context, v *T) error {
	if v == nil {
		return grove.Internal("pipes: ValidateBody target must not be nil")
	}
	if err := c.Body(v); err != nil {
		return grove.BadRequest("invalid request body: " + err.Error())
	}
	if ferrs := Validate(v); len(ferrs) > 0 {
		return grove.Unprocessable("", ferrs)
	}
	return nil
}

// BindQuery maps URL query parameters onto v using `query` tags, then
// validates the result. Supported field kinds: string, bool, all int/uint
// widths, float32/64, and slices of those (repeated params). Fields
// without a `query` tag are left untouched:
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
				if setScalar(ev, r) {
					s = reflect.Append(s, ev)
				}
			}
			fv.Set(s)
			continue
		}
		setScalar(fv, raw[0])
	}
	if ferrs := Validate(v); len(ferrs) > 0 {
		return grove.Unprocessable("", ferrs)
	}
	return nil
}

// setScalar assigns s to v for basic kinds, reporting success.
func setScalar(v reflect.Value, s string) bool {
	if !v.CanSet() {
		return false
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
		v.SetFloat(n)
		return true
	default:
		return false
	}
}
