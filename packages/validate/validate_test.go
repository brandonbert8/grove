package validate

import "testing"

// TestValidateSkipDash guards that validate:"-" is a skip-validation
// marker, not an unknown-rule error (strict mode must not reject it).
func TestValidateSkipDash(t *testing.T) {
	type S struct {
		Skip string `validate:"-"`
		Name string `validate:"required"`
	}
	// Only the required field (Name) fails; the skipped field (Skip)
	// must not produce an error.
	errs := ValidateStrict(&S{Skip: "", Name: ""})
	if len(errs) != 1 || errs[0].Field != "Name" || errs[0].Tag != "required" {
		t.Fatalf("expected only Name required, got %v", errs)
	}
	// A struct whose only tagged field is "-" validates cleanly.
	type OnlyDash struct {
		X string `validate:"-"`
	}
	if errs := ValidateStrict(&OnlyDash{}); len(errs) != 0 {
		t.Fatalf("expected no errors for validate:\"-\", got %v", errs)
	}
}
