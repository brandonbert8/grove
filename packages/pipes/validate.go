package pipes

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// FieldError describes one validation failure.
type FieldError struct {
	// Field is the dotted path (e.g. "address.zip").
	Field string `json:"field"`
	// Tag is the rule that failed (e.g. "required").
	Tag string `json:"tag"`
	// Param is the rule parameter, if any (e.g. "3" for min=3).
	Param string `json:"param,omitempty"`
	// Message is human-readable.
	Message string `json:"message"`
}

// Error implements error.
func (e FieldError) Error() string { return e.Message }

// Supported rules in the `validate` tag (comma-separated):
//
//	required  non-zero value (non-empty string/slice, non-nil pointer)
//	min=n     min length (string/slice) or min value (number)
//	max=n     max length (string/slice) or max value (number)
//	len=n     exact length (string/slice)
//	gte=n     >= n (number)
//	lte=n     <= n (number)
//	email     simple email shape (string)
//	oneof=a b space-separated allowed values (string)
//
// Rules apply per field kind where sensible; unknown rules are ignored so
// custom tags can coexist. Unexported fields are skipped. Nested structs,
// pointers, and slices of structs are validated recursively with dotted
// paths.
func Validate(v any) []FieldError {
	if v == nil {
		return []FieldError{{Field: "", Tag: "required", Message: "value is required"}}
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return []FieldError{{Field: "", Tag: "required", Message: "value is required"}}
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}
	return validateStruct(rv, "")
}

func validateStruct(rv reflect.Value, prefix string) []FieldError {
	var out []FieldError
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if !f.IsExported() {
			continue
		}
		fv := rv.Field(i)
		name := f.Name
		if tag, ok := f.Tag.Lookup("json"); ok {
			if parts := strings.Split(tag, ","); parts[0] != "" && parts[0] != "-" {
				name = parts[0]
			}
		}
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		tag := f.Tag.Get("validate")

		// Recurse into nested structs (unless the field itself failed
		// `required` while nil — a nil pointer is one error, not many).
		deref := fv
		for deref.Kind() == reflect.Pointer {
			if deref.IsNil() {
				break
			}
			deref = deref.Elem()
		}
		if deref.IsValid() && deref.Kind() == reflect.Struct && !isTimeLike(deref) {
			if tag == "" || !hasRule(tag, "required") || !isNilOrZeroPointer(fv) {
				out = append(out, validateStruct(deref, path)...)
			}
		}
		// Recurse into slices of structs for element validation.
		if fv.Kind() == reflect.Slice || fv.Kind() == reflect.Array {
			for j := 0; j < fv.Len(); j++ {
				ev := fv.Index(j)
				for ev.Kind() == reflect.Pointer {
					if ev.IsNil() {
						break
					}
					ev = ev.Elem()
				}
				if ev.IsValid() && ev.Kind() == reflect.Struct && !isTimeLike(ev) {
					out = append(out, validateStruct(ev, fmt.Sprintf("%s[%d]", path, j))...)
				}
			}
		}

		if tag == "" {
			continue
		}
		for _, rule := range strings.Split(tag, ",") {
			rule = strings.TrimSpace(rule)
			if rule == "" {
				continue
			}
			name, param, _ := strings.Cut(rule, "=")
			if msg := checkRule(path, fv, name, param); msg != "" {
				out = append(out, FieldError{Field: path, Tag: name, Param: param, Message: msg})
			}
		}
	}
	return out
}

// hasRule reports whether a validate tag enables rule.
func hasRule(tag, rule string) bool {
	for _, r := range strings.Split(tag, ",") {
		if n, _, _ := strings.Cut(strings.TrimSpace(r), "="); n == rule {
			return true
		}
	}
	return false
}

// isNilOrZeroPointer reports nil pointers/interfaces.
func isNilOrZeroPointer(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

// isTimeLike avoids recursing into time.Time and similar structs.
func isTimeLike(v reflect.Value) bool {
	if !v.CanInterface() {
		return false
	}
	t := v.Type()
	return t.PkgPath() == "time" && t.Name() == "Time"
}

// checkRule applies one rule, returning "" on success.
func checkRule(field string, v reflect.Value, rule, param string) string {
	fail := func(format string, args ...any) string {
		return fmt.Sprintf("field %q %s", field, fmt.Sprintf(format, args...))
	}
	switch rule {
	case "required":
		if isZero(v) {
			return fail("is required")
		}
	case "min", "max", "len":
		n, err := strconv.Atoi(param)
		if err != nil {
			return "" // malformed param: ignore, never fail closed on config
		}
		size, ok := fieldSize(v)
		if !ok {
			return ""
		}
		switch rule {
		case "min":
			if size < float64(n) {
				return fail("must have at least %d", n)
			}
		case "max":
			if size > float64(n) {
				return fail("must have at most %d", n)
			}
		case "len":
			if size != float64(n) {
				return fail("must have length %d", n)
			}
		}
	case "gte", "lte":
		n, err := strconv.ParseFloat(param, 64)
		if err != nil {
			return ""
		}
		f, ok := fieldNumber(v)
		if !ok {
			return ""
		}
		if rule == "gte" && f < n {
			return fail("must be >= %s", param)
		}
		if rule == "lte" && f > n {
			return fail("must be <= %s", param)
		}
	case "email":
		if v.Kind() != reflect.String {
			return ""
		}
		if !emailOK(v.String()) {
			return fail("must be a valid email")
		}
	case "oneof":
		if v.Kind() != reflect.String {
			return ""
		}
		for _, opt := range strings.Fields(param) {
			if v.String() == opt {
				return ""
			}
		}
		return fail("must be one of [%s]", strings.Join(strings.Fields(param), ", "))
	default:
		// Unknown rule: ignore for forward compatibility.
	}
	return ""
}

// isZero reports Go-zero values, treating empty strings/slices as zero.
func isZero(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		return v.IsNil()
	case reflect.Slice, reflect.Map:
		return v.IsNil() || v.Len() == 0
	default:
		return v.IsZero()
	}
}

// fieldSize returns the length of strings/slices or the numeric value.
func fieldSize(v reflect.Value) (float64, bool) {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return 0, false
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.String, reflect.Slice, reflect.Array, reflect.Map:
		return float64(v.Len()), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(v.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(v.Uint()), true
	case reflect.Float32, reflect.Float64:
		return v.Float(), true
	default:
		return 0, false
	}
}

// fieldNumber returns the numeric value of a number kind.
func fieldNumber(v reflect.Value) (float64, bool) {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return 0, false
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(v.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(v.Uint()), true
	case reflect.Float32, reflect.Float64:
		return v.Float(), true
	default:
		return 0, false
	}
}

// emailOK is a deliberately small sanity check, not RFC 5322.
func emailOK(s string) bool {
	if s == "" || strings.Contains(s, " ") {
		return false
	}
	local, domain, ok := strings.Cut(s, "@")
	if !ok || local == "" || domain == "" {
		return false
	}
	if strings.Contains(domain, "@") {
		return false
	}
	dot := strings.LastIndex(domain, ".")
	return dot > 0 && dot < len(domain)-1
}
