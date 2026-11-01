package config

import (
	"encoding"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// LoadAs decodes configuration into T using struct tags. It is the
// ConfigModule.forRoot({ validate }) equivalent without decorators:
//
//	type AppConfig struct {
//	    Host string `env:"HOST" default:"0.0.0.0"`
//	    Port int    `env:"PORT" default:"3000"`
//	    DB   struct {
//	        URL string `env:"DATABASE_URL" required:"true" secret:"true"`
//	    }
//	}
//	cfg, err := config.LoadAs[AppConfig](config.WithEnvFile(""))
//
// Precedence is the same as Load: defaults, .env, process environment,
// WithOverrides. Explicit env tags are absolute names; fields without a
// tag derive PREFIX_FIELD from their struct prefix. Supported tags:
//
//	env:"NAME"        container/env key (use "-" to skip)
//	aliases:"A,B"     accepted alternative names
//	default:"value"   used when no env value is set
//	required:"true"   fail when blank or absent
//	secret:"true"     mask the value in Redacted
//	lower:"true"      lower-case the decoded string
//
// After decoding, a Validate() error method on T is called. Nested
// structs, pointers, slices (comma-separated), time.Duration, time.Time,
// and encoding.TextUnmarshaler values are supported.
func LoadAs[T any](opts ...Option) (*T, error) {
	o := &options{envPath: findDotEnv()}
	for _, opt := range opts {
		opt(o)
	}
	merged, err := mergedEnv(o)
	if err != nil {
		return nil, err
	}

	var out T
	known := make(map[string]bool)
	if err := decodeStruct(reflect.ValueOf(&out).Elem(), "", merged, known); err != nil {
		return nil, err
	}
	for _, name := range o.required {
		key := strings.ToUpper(strings.TrimSpace(name))
		if strings.TrimSpace(merged[key]) == "" {
			return nil, fmt.Errorf("config: required variable %q is not set", name)
		}
	}
	if o.strict {
		for k := range merged {
			if strings.HasPrefix(k, "GROVE_") && !known[k] {
				return nil, fmt.Errorf("config: unknown variable %q (strict mode)", k)
			}
		}
	}
	if v, ok := any(&out).(interface{ Validate() error }); ok {
		if err := v.Validate(); err != nil {
			return nil, err
		}
	}
	return &out, nil
}

// MustLoadAs is like LoadAs but panics on error.
func MustLoadAs[T any](opts ...Option) *T {
	cfg, err := LoadAs[T](opts...)
	if err != nil {
		panic(err)
	}
	return cfg
}

// decodeStruct walks a struct (or pointer to struct) and fills fields
// from env. prefix is the implicit env namespace for untagged fields.
func decodeStruct(v reflect.Value, prefix string, env map[string]string, known map[string]bool) error {
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			if !v.CanSet() {
				return nil
			}
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("config: LoadAs target must be a struct, got %s", v.Kind())
	}
	if v.Type() == reflect.TypeOf(time.Time{}) {
		return nil
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if sf.PkgPath != "" {
			continue
		}
		tag := strings.TrimSpace(sf.Tag.Get("env"))
		if tag == "-" {
			continue
		}
		field := v.Field(i)
		if isNestedStruct(sf.Type) {
			childPrefix := tag
			if childPrefix == "" {
				childPrefix = joinEnv(prefix, toEnv(sf.Name))
			}
			if err := decodeStruct(field, childPrefix, env, known); err != nil {
				return fmt.Errorf("config: %s: %w", sf.Name, err)
			}
			continue
		}
		key := tag
		if key == "" {
			key = joinEnv(prefix, toEnv(sf.Name))
		}
		key = strings.ToUpper(strings.TrimSpace(key))
		known[key] = true
		aliases := splitList(sf.Tag.Get("aliases"))
		for _, alias := range aliases {
			known[strings.ToUpper(alias)] = true
		}

		raw, ok := lookupKey(env, key)
		if !ok {
			for _, alias := range aliases {
				if raw, ok = lookupKey(env, alias); ok {
					break
				}
			}
		}
		if !ok {
			if def, has := sf.Tag.Lookup("default"); has {
				raw, ok = def, true
			}
		}
		if !ok || strings.TrimSpace(raw) == "" {
			if isTrue(sf.Tag.Get("required")) {
				return fmt.Errorf("config: field %s: required variable %q is not set", sf.Name, key)
			}
			continue
		}
		if isTrue(sf.Tag.Get("lower")) {
			raw = strings.ToLower(raw)
		}
		if err := setValue(field, raw, key); err != nil {
			return fmt.Errorf("config: field %s: %w", sf.Name, err)
		}
	}
	return nil
}

var textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()

func isNestedStruct(t reflect.Type) bool {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || t == reflect.TypeOf(time.Time{}) {
		return false
	}
	if t.Implements(textUnmarshalerType) || reflect.PointerTo(t).Implements(textUnmarshalerType) {
		return false
	}
	return true
}

func setValue(v reflect.Value, raw, key string) error {
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			if !v.CanSet() {
				return fmt.Errorf("cannot set %s", v.Type())
			}
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
	}
	if v.CanAddr() {
		if u, ok := v.Addr().Interface().(encoding.TextUnmarshaler); ok {
			if err := u.UnmarshalText([]byte(raw)); err != nil {
				return fmt.Errorf("%q: %w", key, err)
			}
			return nil
		}
	}
	if v.Type() == reflect.TypeOf(time.Duration(0)) {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return fmt.Errorf("%q: %w", key, err)
		}
		v.SetInt(int64(d))
		return nil
	}
	switch v.Kind() {
	case reflect.String:
		v.SetString(raw)
	case reflect.Bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("%q: %w", key, err)
		}
		v.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(raw, 10, v.Type().Bits())
		if err != nil {
			return fmt.Errorf("%q: %w", key, err)
		}
		v.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(raw, 10, v.Type().Bits())
		if err != nil {
			return fmt.Errorf("%q: %w", key, err)
		}
		v.SetUint(n)
	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(raw, v.Type().Bits())
		if err != nil {
			return fmt.Errorf("%q: %w", key, err)
		}
		v.SetFloat(n)
	case reflect.Slice:
		parts := splitList(raw)
		out := reflect.MakeSlice(v.Type(), len(parts), len(parts))
		for i, part := range parts {
			if err := setValue(out.Index(i), part, key); err != nil {
				return err
			}
		}
		v.Set(out)
	default:
		return fmt.Errorf("%q: unsupported field type %s", key, v.Type())
	}
	return nil
}

// Redacted renders v as KEY=value lines, masking fields tagged
// secret:"true". Use it in startup logs instead of printing config.
func Redacted(v any) string {
	var lines []string
	redactValue(reflect.ValueOf(v), "", &lines)
	if len(lines) == 0 {
		return fmt.Sprint(v)
	}
	return strings.Join(lines, "\n")
}

func redactValue(v reflect.Value, prefix string, lines *[]string) {
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if sf.PkgPath != "" {
			continue
		}
		tag := strings.TrimSpace(sf.Tag.Get("env"))
		if tag == "-" {
			continue
		}
		field := v.Field(i)
		if isNestedStruct(sf.Type) {
			childPrefix := tag
			if childPrefix == "" {
				childPrefix = joinEnv(prefix, toEnv(sf.Name))
			}
			redactValue(field, childPrefix, lines)
			continue
		}
		key := tag
		if key == "" {
			key = joinEnv(prefix, toEnv(sf.Name))
		}
		key = strings.ToUpper(strings.TrimSpace(key))
		value := fmt.Sprint(field.Interface())
		if field.CanAddr() {
			if m, ok := field.Addr().Interface().(encoding.TextMarshaler); ok {
				if b, err := m.MarshalText(); err == nil {
					value = string(b)
				}
			}
		}
		if isTrue(sf.Tag.Get("secret")) {
			value = "[REDACTED]"
		}
		*lines = append(*lines, key+"="+value)
	}
}

func lookupKey(env map[string]string, key string) (string, bool) {
	key = strings.ToUpper(strings.TrimSpace(key))
	if v, ok := env[key]; ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v), true
	}
	return "", false
}

func joinEnv(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "_" + name
}

// toEnv converts CamelCase to UPPER_SNAKE (HTTPPort -> HTTP_PORT).
func toEnv(name string) string {
	var b strings.Builder
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c >= 'A' && c <= 'Z' && i > 0 {
			prev := name[i-1]
			nextLower := i+1 < len(name) && name[i+1] >= 'a' && name[i+1] <= 'z'
			if (prev >= 'a' && prev <= 'z') || (prev >= '0' && prev <= '9') ||
				(prev >= 'A' && prev <= 'Z' && nextLower) {
				b.WriteByte('_')
			}
		}
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		b.WriteByte(c)
	}
	return b.String()
}

func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func isTrue(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
