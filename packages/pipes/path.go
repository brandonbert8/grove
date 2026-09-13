package pipes

import (
	"fmt"
	"reflect"

	grove "github.com/brandonbert8/grove/packages/core"
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
// *grove.HttpError; rule violations are 422. Unexported fields and
// fields without a `path` tag are skipped.
func BindPath[T any](c router.Context, v *T) error {
	if v == nil {
		return grove.Internal("pipes: BindPath target must not be nil")
	}
	rv := reflect.ValueOf(v).Elem()
	if rv.Kind() != reflect.Struct {
		return grove.Internal("pipes: BindPath target must be a struct pointer")
	}
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		key := f.Tag.Get("path")
		if key == "" || key == "-" {
			continue
		}
		if !rt.Field(i).IsExported() {
			continue
		}
		raw := c.Param(key)
		if raw == "" {
			return grove.BadRequest(fmt.Sprintf("missing path parameter %q", key))
		}
		fv := rv.Field(i)
		if !setScalar(fv, raw) {
			return grove.BadRequest(fmt.Sprintf(
				"value for path parameter %q is not a valid %s", key, fv.Kind()))
		}
	}
	if ferrs := Validate(v); len(ferrs) > 0 {
		return grove.Unprocessable("", ferrs)
	}
	return nil
}
