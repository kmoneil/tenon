package tenon_test

import (
	"slices"
	"testing"

	"tenon"
	"tenon/conformance"
)

// located returns each diagnostic of an error value as "message at path", or
// just the message when the path is empty.
func located(v tenon.Value) []string {
	var out []string
	for _, d := range v.Diagnostics() {
		if d.Path.Len() == 0 {
			out = append(out, d.Message)
			continue
		}
		out = append(out, d.Message+" at "+d.Path.String())
	}
	return out
}

// failed returns an error value carrying one diagnostic with the given
// message.
func failed(message string) tenon.Value {
	return tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: message})
}

func TestConformance_ER008_ContainersHoistErrors(t *testing.T) {
	conformance.Covers(t, "ER-008")
	str := tenon.StringType()
	a := tenon.String("a")
	first, second := failed("first"), failed("second")

	// A container built from an error member is an error value, not a
	// container holding errors, and every diagnostic says where the member was.
	for _, tt := range []struct {
		name string
		got  tenon.Value
		want []string
	}{
		{"list", tenon.ListVal(str, a, first), []string{"first at .[1]"}},
		{"set", tenon.SetVal(str, first), []string{"first at .[0]"}},
		{"tuple", tenon.TupleVal(a, first, second), []string{"first at .[1]", "second at .[2]"}},
		{
			"object",
			tenon.ObjectVal(map[string]tenon.Value{"name": first, "count": second}),
			[]string{"second at .count", "first at .name"},
		},
		{"map", tenon.MapVal(str, map[string]tenon.Value{"k": first}), []string{`first at .["k"]`}},
		{"several, in element order", tenon.ListVal(str, first, a, second), []string{"first at .[0]", "second at .[2]"}},
	} {
		if !tt.got.IsError() || tt.got.IsResolved() {
			t.Errorf("%s: %v is not an error value", tt.name, tt.got)
			continue
		}
		if got := located(tt.got); !slices.Equal(got, tt.want) {
			t.Errorf("%s: diagnostics %q, want %q", tt.name, got, tt.want)
		}
	}

	// An error deeper down surfaces with a step for every container it came
	// through.
	inner := tenon.ObjectVal(map[string]tenon.Value{"name": failed("deep")})
	outer := tenon.ListVal(tenon.Object(map[string]tenon.Type{"name": str}), inner)
	if got := located(outer); !slices.Equal(got, []string{"deep at .[0].name"}) {
		t.Errorf("an error two levels deep gave %q", got)
	}
	deeper := tenon.MapVal(tenon.List(str), map[string]tenon.Value{"list": tenon.ListVal(str, a, failed("leaf"))})
	if got := located(deeper); !slices.Equal(got, []string{`leaf at .["list"][1]`}) {
		t.Errorf("an error three levels deep gave %q", got)
	}

	// Exact duplicates are dropped, as in the propagation of ER-005, but the
	// same message in two places is not a duplicate.
	twice := tenon.ErrorVal(
		tenon.Diagnostic{Code: "app.failed", Message: "twice"},
		tenon.Diagnostic{Code: "app.failed", Message: "twice"},
	)
	if got := located(tenon.ListVal(str, twice)); !slices.Equal(got, []string{"twice at .[0]"}) {
		t.Errorf("a repeated diagnostic gave %q", got)
	}
	if got := located(tenon.ListVal(str, first, first)); !slices.Equal(got, []string{"first at .[0]", "first at .[1]"}) {
		t.Errorf("one message at two positions gave %q", got)
	}

	// A key that is not a string cannot locate its element, so that element's
	// diagnostics keep the paths they came with.
	mixed := tenon.MapVal(str, map[string]tenon.Value{"a\xff": failed("under a bad key"), "k": first})
	if d := mixed.Diagnostics(); len(d) != 3 || d[0].Code != tenon.CodeStringInvalidUTF8 {
		t.Errorf("a map with a bad key and error elements gave %v", d)
	} else if got := located(mixed)[1:]; !slices.Equal(got, []string{"under a bad key", `first at .["k"]`}) {
		t.Errorf("diagnostics %q", got)
	}

	// Members that are not error values are still checked, and a host's own
	// mistake still panics.
	mustPanicUsage(t, "has type number, not string", func() { tenon.ListVal(str, first, tenon.NumberFromInt(1)) })
	mustPanicUsage(t, "is a pending value", func() { tenon.TupleVal(first, tenon.Pending(tenon.Any())) })
}
