package xapi

import (
	"fmt"
	"testing"
)

// fieldCheck is one expected value in a table-driven assertion. Parser tests use
// it so a fixture with many fields stays flat instead of growing one branch per
// field, which is what pushes a test over the complexity bar.
type fieldCheck struct {
	name string
	got  any
	want any
}

// checkFields reports every field whose parsed value differs from the expected
// one, so a single run shows all the mismatches rather than only the first.
func checkFields(t *testing.T, checks []fieldCheck) {
	t.Helper()
	for _, c := range checks {
		if c.got == c.want {
			continue
		}
		// Two values that print alike but differ are a type mismatch, so name the
		// types; otherwise the failure reads as "42, want 42".
		if fmt.Sprint(c.got) == fmt.Sprint(c.want) {
			t.Errorf("%s = %v (%T), want %v (%T)", c.name, c.got, c.got, c.want, c.want)
			continue
		}
		t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
	}
}
