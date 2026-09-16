package openapi

import (
	"reflect"
	"strconv"
	"strings"
)

// JSONSchema is a subset of JSON Schema for DTO documentation.
type JSONSchema map[string]any

// SchemaFor builds an object schema from a struct type T using
// json + validate tags (the NestJS DTO → OpenAPI equivalent):
//
//	type CreateUser struct {
//	    Name  string `json:"name" validate:"required,min=2,max=80"`
//	    Email string `json:"email" validate:"required,email"`
//	    Age   int    `json:"age" validate:"gte=0,lte=150"`
//	}
//	schema := openapi.SchemaFor[CreateUser]()
func SchemaFor[T any]() JSONSchema {
	var v T
	return schemaOf(reflect.TypeOf(v))
}

func schemaOf(t reflect.Type) JSONSchema {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return JSONSchema{"type": goKindToJSON(t.Kind().String())}
	}
	props := map[string]any{}
	var required []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name, omit := jsonFieldName(f)
		if omit {
			continue
		}
		fs := fieldSchema(f)
		props[name] = fs
		if isRequired(f.Tag.Get("validate")) {
			required = append(required, name)
		}
	}
	out := JSONSchema{"type": "object", "properties": props}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

func jsonFieldName(f reflect.StructField) (string, bool) {
	tag, ok := f.Tag.Lookup("json")
	if !ok {
		return f.Name, false
	}
	parts := strings.Split(tag, ",")
	if parts[0] == "-" {
		return "", true
	}
	if parts[0] != "" {
		return parts[0], false
	}
	return f.Name, false
}

func fieldSchema(f reflect.StructField) JSONSchema {
	t := f.Type
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	var base string
	switch t.Kind() {
	case reflect.String:
		base = "string"
	case reflect.Bool:
		base = "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		if strings.Contains(f.Tag.Get("validate"), "email") {
			base = "string"
			break
		}
		base = kindNumeric(t.Kind())
	case reflect.Slice, reflect.Array:
		items := JSONSchema{"type": goKindToJSON(t.Elem().Kind().String())}
		s := JSONSchema{"type": "array", "items": items}
		applyValidateConstraints(s, f.Tag.Get("validate"))
		if d := f.Tag.Get("description"); d != "" {
			s["description"] = d
		}
		return s
	case reflect.Struct:
		if t.PkgPath() == "time" && t.Name() == "Time" {
			return JSONSchema{"type": "string", "format": "date-time"}
		}
		nested := schemaOf(t)
		applyValidateConstraints(nested, f.Tag.Get("validate"))
		return nested
	default:
		base = "string"
	}
	s := JSONSchema{"type": base}
	applyValidateConstraints(s, f.Tag.Get("validate"))
	if v := f.Tag.Get("validate"); strings.Contains(v, "email") {
		s["format"] = "email"
	}
	if v := f.Tag.Get("validate"); strings.Contains(v, "uuid") {
		s["format"] = "uuid"
	}
	if v := f.Tag.Get("validate"); strings.Contains(v, "url") {
		s["format"] = "uri"
	}
	if d := f.Tag.Get("description"); d != "" {
		s["description"] = d
	}
	return s
}

func kindNumeric(k reflect.Kind) string {
	switch k {
	case reflect.Float32, reflect.Float64:
		return "number"
	default:
		return "integer"
	}
}

func goKindToJSON(k string) string {
	switch k {
	case "String":
		return "string"
	case "Bool":
		return "boolean"
	case "Int", "Int8", "Int16", "Int32", "Int64",
		"Uint", "Uint8", "Uint16", "Uint32", "Uint64":
		return "integer"
	case "Float32", "Float64":
		return "number"
	case "Slice", "Array":
		return "array"
	case "Map", "Struct":
		return "object"
	case "Bool2":
		return "boolean"
	default:
		l := strings.ToLower(k)
		switch l {
		case "string":
			return "string"
		case "bool":
			return "boolean"
		case "int", "int8", "int16", "int32", "int64",
			"uint", "uint8", "uint16", "uint32", "uint64":
			return "integer"
		case "float32", "float64":
			return "number"
		}
		return "string"
	}
}

func isRequired(validate string) bool {
	for _, r := range strings.Split(validate, ",") {
		if n, _, _ := strings.Cut(strings.TrimSpace(r), "="); n == "required" {
			return true
		}
	}
	return false
}

func applyValidateConstraints(s JSONSchema, validate string) {
	for _, r := range strings.Split(validate, ",") {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		name, param, _ := strings.Cut(r, "=")
		switch name {
		case "min":
			// Length for strings, value for numbers: emit both when ambiguous
			// is impossible without kind; minLength is the common DTO case.
			if n, ok := parseNum(param); ok {
				if s["type"] == "string" || s["type"] == "array" {
					s["minLength"] = n
				} else {
					s["minimum"] = n
				}
			}
		case "max":
			if n, ok := parseNum(param); ok {
				if s["type"] == "string" || s["type"] == "array" {
					s["maxLength"] = n
				} else {
					s["maximum"] = n
				}
			}
		case "len":
			if n, ok := parseNum(param); ok {
				s["minLength"] = n
				s["maxLength"] = n
			}
		case "gte":
			if n, ok := parseNum(param); ok {
				s["minimum"] = n
			}
		case "lte":
			if n, ok := parseNum(param); ok {
				s["maximum"] = n
			}
		case "oneof":
			s["enum"] = strings.Fields(param)
		}
	}
}

func parseNum(s string) (any, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, false
	}
	if i, err := strconv.Atoi(s); err == nil {
		return i, true
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f, true
	}
	return nil, false
}
