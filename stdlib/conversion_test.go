package stdlib_test

import (
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/stdlib"
)

func TestConformance_LN085_MakeTo(t *testing.T) {
	conformance.Covers(t, "LN-085")
	num, str := tenon.NumberType(), tenon.StringType()
	toNumber := stdlib.MakeToFunc(tenon.Exactly(num))
	toList := stdlib.MakeToFunc(tenon.ListOf(tenon.Any()))

	// The conversion is unsafe, whatever the call's policy.
	if got := call(toNumber, tenon.String("5")); !got.Equal(tenon.NumberFromInt(5)) {
		t.Errorf("tonumber(\"5\") = %v, want 5", got)
	}
	if got := call(toList, tenon.Tuple(tenon.NumberFromInt(1), tenon.String("a"))); !got.Equal(tenon.List(str, tenon.String("1"), tenon.String("a"))) {
		t.Errorf("tolist([1, \"a\"]) = %v, want a list of strings", got)
	}
	// A null converts to a null; what is not known yet stays so.
	if got := call(toNumber, tenon.Null(str)); !got.Equal(tenon.Null(num)) {
		t.Errorf("tonumber(null) = %v, want null(number)", got)
	}
	if got := call(toNumber, tenon.Narrow(tenon.Unknown(str), tenon.NotNull())); got.IsKnown() || !notNull(got) {
		t.Errorf("tonumber(unknown, not null) = %v, want an unknown number, not null", got)
	}

	// A failure is the conversion's, located within the argument.
	got := call(toList, tenon.Tuple(tenon.NumberFromInt(1), tenon.Tuple()))
	if !got.IsError() {
		t.Fatalf("tolist([1, []]) = %v, want a failure", got)
	}
	for _, d := range got.Diagnostics() {
		if d.Path.Len() == 0 || !d.Path.Steps()[0].Equal(at(0).Steps()[0]) {
			t.Errorf("tolist([1, []]) failed with %+v, want it located within argument 0", d)
		}
	}
	got = call(toNumber, tenon.String("inf"))
	if !got.IsError() || got.Diagnostics()[0].Code != tenon.CodeNumberInvalidSyntax || !got.Diagnostics()[0].Path.Equal(at(0)) {
		t.Errorf("tonumber(\"inf\") = %v, want %s at [0]", got, tenon.CodeNumberInvalidSyntax)
	}

	// The function sees its argument marked, so a failure withholds what a
	// redacting mark requires, and the answer carries the mark.
	got = call(toNumber, tenon.WithMarks(tenon.String("hunter2"), secret{}))
	if !got.IsError() || !tenon.HasMark(got, secret{}) || strings.Contains(got.Diagnostics()[0].Message, "hunter2") {
		t.Errorf("tonumber(a redacted string) = %v, want a failure withholding it", got)
	}
}
