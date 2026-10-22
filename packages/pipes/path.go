package pipes

import (
	"fmt"
	"reflect"
	"strings"

	httperr "github.com/brandonbert8/grove/packages/httperr"
	"github.com/brandonbert8/grove/packages/router"
)

// BindPath maps Chi path parameters onto v using `path` tags, then
// validates the result — completing the ValidateBody/BindQuery/
// BindHeader family for the three parameter sources:
//
//	type UserParams struct {
//	    ID int `path:"id" validate:"gte=1"`
//	}
//	var p UserParams
//	if err := pipes.BindPath(c, &p); err != nil { return err }
//
// A tag naming no matched parameter (programmer error, since routing
// guarantees declared {names} exist) and any unparsable value are 400
// *httperr.HttpError; rule violations are 422. Unexported fields and
// fields without a `path` tag are skipped.
func BindPath[T any](c router.Context, v *T) error {
	if v == nil {
		return httperr.Internal("pipes: BindPath target must not be nil")
	}
	rv := reflect.ValueOf(v).Elem()
	if rv.Kind() != reflect.Struct {
		return httperr.Internal("pipes: BindPath target must be a struct pointer")
	}
	rt := rv.Type()
	var bound []string
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		key := f.Tag.Get("path")
		if key == "" || key == "-" {
			continue
		}
		if !f.IsExported() {
			return httperr.Internal(fmt.Sprintf("pipes: BindPath field %q has a path tag but is unexported", f.Name))
		}
		// El campo está bound (tiene path tag; el router garantiza que
		// {name} existe). Se valida aunque el valor falle el setScalar
		// (ese error es 400 antes de validar).
		jsonName := f.Name
		if tag, ok := f.Tag.Lookup("json"); ok {
			if parts := strings.Split(tag, ","); parts[0] != "" && parts[0] != "-" {
				jsonName = parts[0]
			}
		}
		bound = append(bound, jsonName)

		raw := c.Param(key)
		if raw == "" {
			return httperr.BadRequest(fmt.Sprintf("missing path parameter %q", key))
		}
		fv := rv.Field(i)
		if !setScalar(fv, raw) {
			return httperr.BadRequest(fmt.Sprintf(
				"value for path parameter %q is not a valid %s", key, fv.Kind()))
		}
	}
	if ferrs := ValidateStrictFields(v, bound...); len(ferrs) > 0 {
		return httperr.Unprocessable("", ferrs)
	}
	return nil
}
