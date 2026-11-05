package validate

import (
	"reflect"
	"testing"
)

// FuzzValidateTags feeds arbitrary `validate` tags into the rule engine.
// StructOf lets the fuzzer build tags that cannot be written by hand,
// covering rule parsing and malformed parameters without panics.
func FuzzValidateTags(f *testing.F) {
	f.Add("required")
	f.Add("required,min=2,max=8")
	f.Add("email")
	f.Add("oneof=a b c")
	f.Add("min=not-a-number")
	f.Fuzz(func(t *testing.T, tag string) {
		typ := reflect.StructOf([]reflect.StructField{{
			Name: "Value",
			Type: reflect.TypeOf(""),
			Tag:  reflect.StructTag(`validate:"` + tag + `"`),
		}})
		v := reflect.New(typ).Elem()
		v.Field(0).SetString("x")
		_ = Validate(v.Interface())
		_ = ValidateStrict(v.Interface())
	})
}
