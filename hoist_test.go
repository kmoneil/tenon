package tenon_test

import (
	"fmt"
	"slices"
	"strconv"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
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

	// A map's elements come in the order of its keys, normalized, so two
	// spellings of one map hoist alike.
	composedE, decomposedE := "\U000000E9", "e\U00000301"
	one, two := failed("one"), failed("two")
	spelled := tenon.MapVal(str, map[string]tenon.Value{decomposedE: one, "f": two})
	if other := tenon.MapVal(str, map[string]tenon.Value{composedE: one, "f": two}); !tenon.Identical(spelled, other) {
		t.Errorf("two spellings of one map gave %q and %q", located(spelled), located(other))
	}
	if got, want := located(spelled), []string{`two at .["f"]`, "one at " + tenon.Path{}.Index(tenon.String(composedE)).String()}; !slices.Equal(got, want) {
		t.Errorf("a map with two error elements gave %q, want %q", got, want)
	}

	// A key that is not a string cannot locate its element, so that element's
	// diagnostics keep the paths they came with.
	mixed := tenon.MapVal(str, map[string]tenon.Value{"a\xff": failed("under a bad key"), "k": first})
	if d := mixed.Diagnostics(); len(d) != 3 || d[0].Code != tenon.CodeStringInvalidUTF8 {
		t.Errorf("a map with a bad key and error elements gave %v", d)
	} else if got := located(mixed)[1:]; !slices.Equal(got, []string{"under a bad key", `first at .["k"]`}) {
		t.Errorf("diagnostics %q", got)
	}

	// A container with many error members records each diagnostic once,
	// whether there are few or many: a diagnostic repeated within one member
	// is kept once, and one message at two positions is two diagnostics,
	// which holds on either side of the count where the lookup stops
	// comparing and starts looking up.
	for _, count := range []int{4, 40} {
		members := make([]tenon.Value, count)
		var want []string
		for i := range members {
			twice := tenon.Diagnostic{Code: "app.failed", Message: "twice"}
			members[i] = tenon.ErrorVal(twice, twice,
				tenon.Diagnostic{Code: "app.failed", Message: "member " + strconv.Itoa(i)})
			at := " at .[" + strconv.Itoa(i) + "]"
			want = append(want, "twice"+at, "member "+strconv.Itoa(i)+at)
		}
		if got := located(tenon.ListVal(str, members...)); !slices.Equal(got, want) {
			t.Errorf("a list of %d error members gave %q, want %q", count, got, want)
		}
	}

	// Many members, each failing once: every diagnostic is recorded, in
	// element order. A list this long took seconds to build when each
	// diagnostic was compared with every one recorded before it.
	// Sized so that comparing each diagnostic with every one recorded, which
	// is what this replaced, takes seconds rather than the milliseconds it
	// takes now: there is no count to assert, a diagnostic being compared by
	// the package itself, so the gate's own time is the signal.
	const many = 40_000
	members := make([]tenon.Value, many)
	for i := range members {
		members[i] = failed("member " + strconv.Itoa(i))
	}
	d := tenon.ListVal(str, members...).Diagnostics()
	if len(d) != many {
		t.Fatalf("a list of %d error members gave %d diagnostics", many, len(d))
	}
	if first, last := d[0].Path.String(), d[many-1].Path.String(); first != ".[0]" || last != ".["+strconv.Itoa(many-1)+"]" {
		t.Errorf("the diagnostics run from %s to %s", first, last)
	}

	// Members that are not error values are still checked, and a host's own
	// mistake still panics.
	mustPanicUsage(t, "has type number, not string", func() { tenon.ListVal(str, first, tenon.NumberFromInt(1)) })
	mustPanicUsage(t, "is a pending value", func() { tenon.TupleVal(first, tenon.Pending(tenon.Any())) })
}

// BenchmarkHoistedFailures measures building a list of error members, each a
// diagnostic at its own path, and propagating two operands' diagnostics
// through an operation, at a size and four times it: the growth from one to
// the other is the reading, not the wall clock.
func BenchmarkHoistedFailures(b *testing.B) {
	str := tenon.StringType()
	for _, size := range []int{5000, 20000} {
		members := make([]tenon.Value, size)
		left := make([]tenon.Diagnostic, size)
		right := make([]tenon.Diagnostic, size)
		for i := range members {
			members[i] = failed("member " + strconv.Itoa(i))
			left[i] = tenon.Diagnostic{Code: "app.left", Message: "l" + strconv.Itoa(i)}
			right[i] = tenon.Diagnostic{Code: "app.right", Message: "r" + strconv.Itoa(i)}
		}
		x, y := tenon.ErrorVal(left...), tenon.ErrorVal(right...)
		b.Run(fmt.Sprintf("hoist/%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if v := tenon.ListVal(str, members...); !v.IsError() {
					b.Fatal("the list was built")
				}
			}
		})
		b.Run(fmt.Sprintf("propagate/%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if v := tenon.And(x, y); !v.IsError() {
					b.Fatal("the operation gave a value")
				}
			}
		})
	}
}
