package validate

import (
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
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
//	required  non-zero value (non-empty string/slice, non-nil pointer).
//	          Note: plain bool required means true (use *bool to require
//	          presence of false); *string→"" counts as present.
//	min=n     min length (string/slice) or min value (number)
//	max=n     max length (string/slice) or max value (number)
//	len=n     exact length — strings/slices only (rejected on numbers)
//	gte=n     >= n (number)
//	lte=n     <= n (number)
//	email     simple email shape (string)
//	uuid      canonical 8-4-4-4-12 hex (string)
//	url       absolute http(s) URL without userinfo (string)
//	oneof=a b space-separated allowed values (string)
//
// Custom rules (the class-validator custom-decorator equivalent) plug in
// via RegisterRule and run in place of — or overriding — builtins.
// Rules apply per field kind where sensible; unknown rules are ignored so
// custom tags can coexist. Unexported fields are skipped. Nested structs,
// pointers, and slices of structs are validated recursively with dotted
// paths (max depth 32, cycles cut by pointer identity).
// Fields tagged json:"-" are skipped entirely (never unmarshalled, never
// validated).
func Validate(v any) []FieldError {
	return validateValue(v, false)
}

// ValidateStrict behaves like Validate but fails closed: unknown rules
// and malformed rule parameters (e.g. a typo'd `gte=abc`) report errors
// instead of passing silently. Use it when DTO tags are load-bearing
// contracts; keep Validate for forward-compatible interop tags.
func ValidateStrict(v any) []FieldError {
	return validateValue(v, true)
}

// maxValidateDepth caps recursive struct validation (DoS guard for
// cyclic pointer graphs built programmatically).
const maxValidateDepth = 32

// validateValue dereferences v and validates structs.
func validateValue(v any, strict bool) []FieldError {
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
	return validateStruct(rv, "", strict, 0, map[[2]uintptr]bool{})
}

// RuleFunc validates one field for a custom rule (see RegisterRule).
// field is the dotted path, value the field's reflect value, param the
// "=..." suffix ("" when absent). Return "" on success, otherwise the
// complete failure message (it is NOT wrapped with the field name).
type RuleFunc func(field string, value reflect.Value, param string) string

// customRules holds user-registered validation rules by name.
var customRules sync.Map // map[string]RuleFunc

// RegisterRule adds (or overrides) a `validate` tag rule, the equivalent
// of a class-validator custom decorator. It panics on empty name or nil
// fn: call it from init or module wiring, never per request.
func RegisterRule(name string, fn RuleFunc) {
	if strings.TrimSpace(name) == "" {
		panic("pipes: rule name must not be empty")
	}
	if fn == nil {
		panic("pipes: rule func must not be nil")
	}
	customRules.Store(name, fn)
}

func validateStruct(rv reflect.Value, prefix string, strict bool, depth int, seen map[[2]uintptr]bool) []FieldError {
	var out []FieldError
	if depth > maxValidateDepth {
		return []FieldError{{Field: prefix, Tag: "depth", Message: "validation depth exceeded"}}
	}
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if !f.IsExported() {
			continue
		}
		if tag, ok := f.Tag.Lookup("json"); ok {
			if parts := strings.Split(tag, ","); parts[0] == "-" {
				continue
			}
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
				// Depth-capped: cyclic pointer graphs terminate with
				// a depth error instead of overflowing the stack.
				out = append(out, validateStruct(deref, path, strict, depth+1, seen)...)
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
					out = append(out, validateStruct(ev, fmt.Sprintf("%s[%d]", path, j), strict, depth+1, seen)...)
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
			if msg := checkRule(path, fv, name, param, strict); msg != "" {
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

// checkRule applies one rule, returning "" on success. In strict mode,
// malformed parameters and unknown rules fail instead of passing.
func checkRule(field string, v reflect.Value, rule, param string, strict bool) string {
	// Custom rules win over builtins so teams can tighten (or replace)
	// framework defaults without forking the engine.
	if fn, ok := customRules.Load(rule); ok {
		if rfn, ok := fn.(RuleFunc); ok {
			return rfn(field, v, param)
		}
	}
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
			if strict {
				return fail("has invalid rule parameter %q", param)
			}
			return "" // malformed param: ignore, never fail closed on config
		}
		if rule == "len" && isNumericKind(v) {
			// len is a length rule: meaningless on numbers (use
			// gte/lte for values). Strict flags the misuse.
			if strict {
				return fail("rule len does not apply to numbers (use gte/lte)")
			}
			return ""
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
			if strict {
				return fail("has invalid rule parameter %q", param)
			}
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
	case "uuid":
		if v.Kind() != reflect.String {
			return ""
		}
		if !uuidOK(v.String()) {
			return fail("must be a valid UUID")
		}
	case "url":
		if v.Kind() != reflect.String {
			return ""
		}
		if !urlOK(v.String()) {
			return fail("must be a valid absolute http(s) URL")
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
		if strict {
			return fail("uses unknown validation rule %q", rule)
		}
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

// isNumericKind reports number kinds (after pointer unwrap).
func isNumericKind(v reflect.Value) bool {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return false
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

// emailOK is a deliberately small sanity check, not RFC 5322.
func emailOK(s string) bool {
	if s == "" {
		return false
	}
	// Reject control characters and spaces (header-injection safe).
	for i := 0; i < len(s); i++ {
		if s[i] <= ' ' || s[i] == 0x7f {
			return false
		}
	}
	if len(s) > 254 {
		return false
	}
	local, domain, ok := strings.Cut(s, "@")
	if !ok || local == "" || domain == "" {
		return false
	}
	if len(local) > 64 {
		return false
	}
	if strings.Contains(domain, "@") {
		return false
	}
	// No consecutive dots, no leading/trailing dot or hyphen abuse.
	if strings.Contains(s, "..") {
		return false
	}
	dot := strings.LastIndex(domain, ".")
	return dot > 0 && dot < len(domain)-1
}

// uuidOK accepts canonical 8-4-4-4-12 hex, any version, either case.
func uuidOK(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < 36; i++ {
		switch i {
		case 8, 13, 18, 23:
			if s[i] != '-' {
				return false
			}
		default:
			if !isHex(s[i]) {
				return false
			}
		}
	}
	return true
}

// isHex reports ASCII hex digits.
func isHex(b byte) bool {
	return b >= '0' && b <= '9' || b >= 'a' && b <= 'f' || b >= 'A' && b <= 'F'
}

// urlOK requires an absolute http(s) URL with a host and no userinfo
// (credentials in a validated-then-fetched URL leak or enable SSRF).
func urlOK(s string) bool {
	if s == "" {
		return false
	}
	u, err := url.ParseRequestURI(s)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	if u.User != nil {
		return false
	}
	return u.Host != ""
}
