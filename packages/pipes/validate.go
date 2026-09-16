package pipes

import (
	"github.com/brandonbert8/grove/packages/validate"
)

// FieldError describes one validation failure (alias of
// validate.FieldError: the engine lives in the leaf package so core
// can validate DTOs without importing pipes).
type FieldError = validate.FieldError

// RuleFunc validates one field for a custom rule (alias of
// validate.RuleFunc).
type RuleFunc = validate.RuleFunc

// Validate checks the `validate` tag rules, ignoring unknown rules
// (forward-compatible mode). Prefer ValidateStrict for contracts.
func Validate(v any) []FieldError { return validate.Validate(v) }

// ValidateStrict behaves like Validate but fails closed: unknown rules
// and malformed rule parameters report errors instead of passing.
func ValidateStrict(v any) []FieldError { return validate.ValidateStrict(v) }

// RegisterRule adds (or overrides) a `validate` tag rule, the equivalent
// of a class-validator custom decorator. It panics on empty name or nil
// fn: call it from init or module wiring, never per request.
func RegisterRule(name string, fn RuleFunc) { validate.RegisterRule(name, fn) }
