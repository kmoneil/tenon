package stdlib_test

import (
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/stdlib"
)

func TestConformance_LN020_Logic(t *testing.T) {
	conformance.Covers(t, "LN-020")
	bools := []tenon.Value{
		tenon.Bool(true), tenon.Bool(false), tenon.Unknown(tenon.BoolType()),
		tenon.WithMarks(tenon.Bool(true), secret{}),
	}
	for _, a := range bools {
		if got, want := call(stdlib.NotFunc, a), tenon.Not(a); !tenon.Identical(got, want) {
			t.Errorf("Not(%v) = %v, want %v", a, got, want)
		}
		for _, b := range bools {
			if got, want := call(stdlib.AndFunc, a, b), tenon.And(a, b); !tenon.Identical(got, want) {
				t.Errorf("And(%v, %v) = %v, want %v", a, b, got, want)
			}
			if got, want := call(stdlib.OrFunc, a, b), tenon.Or(a, b); !tenon.Identical(got, want) {
				t.Errorf("Or(%v, %v) = %v, want %v", a, b, got, want)
			}
		}
	}
	// A false operand decides And whatever the other turns out to be; an
	// error operand is never passed over.
	if got := call(stdlib.AndFunc, tenon.Bool(false), tenon.Unknown(tenon.BoolType())); !got.Equal(tenon.Bool(false)) {
		t.Errorf("And(false, unknown) = %v, want false", got)
	}
	boom := tenon.ErrorVal(tenon.Diagnostic{Code: "app.boom", Message: "boom"})
	if got := call(stdlib.OrFunc, tenon.Bool(true), boom); !got.IsError() {
		t.Errorf("Or(true, an error) = %v, want the error", got)
	}
}
