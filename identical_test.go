package tenon_test

import (
	"testing"

	"tenon"
	"tenon/conformance"
	"tenon/conformance/values"
)

func TestConformance_EQ010_IdenticalComparesEverything(t *testing.T) {
	conformance.Covers(t, "EQ-010")
	num, str := tenon.NumberType(), tenon.StringType()
	one := tenon.NumberFromInt(1)
	pending := tenon.Pending(tenon.Any())
	bounded := func(incl bool) tenon.Value {
		return tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(one, incl))
	}
	failed := tenon.Diagnostic{Code: "app.failed", Message: "it failed"}
	other := tenon.Diagnostic{Code: "app.other", Message: "and again"}
	for _, tt := range []struct {
		name string
		a, b tenon.Value
		want bool
	}{
		// One value reached two ways is one value.
		{"a number written two ways", one, tenon.NumberFromText("1.000"), true},
		{"a string in two normal forms", tenon.String("e\U00000301"), tenon.String("\U000000e9"), true},
		{
			"a set given its members in either order",
			tenon.SetVal(str, tenon.String("a"), tenon.String("b")),
			tenon.SetVal(str, tenon.String("b"), tenon.String("a")),
			true,
		},
		// A set holding an unknown member twice has a range that holding it
		// once does not: it could have two members.
		{
			"a set holding an unknown twice and once",
			tenon.SetVal(str, tenon.Unknown(str), tenon.Unknown(str)),
			tenon.SetVal(str, tenon.Unknown(str)),
			false,
		},
		{
			"sets holding an unknown twice, built apart",
			tenon.SetVal(str, tenon.String("a"), tenon.Unknown(str), tenon.Unknown(str)),
			tenon.SetVal(str, tenon.Unknown(str), tenon.String("a"), tenon.Unknown(str)),
			true,
		},
		// The state is part of it.
		{"a value and an unknown of its type", one, tenon.Unknown(num), false},
		{"an unknown and a null", tenon.Unknown(str), tenon.NullVal(str), false},
		{"an error and a pending value", tenon.ErrorVal(failed), pending, false},
		// The type is part of it.
		{"nulls of different types", tenon.NullVal(str), tenon.NullVal(num), false},
		{"unknowns of different types", tenon.Unknown(str), tenon.Unknown(num), false},
		// The range is part of it, which is a question Equals cannot answer.
		{"unknowns with one range", bounded(true), bounded(true), true},
		{"an unknown with a bound and one without", bounded(true), tenon.Unknown(num), false},
		{"bounds that differ only in what they include", bounded(true), bounded(false), false},
		// The diagnostics are part of it, in the order they are carried.
		{"errors with one diagnostic", tenon.ErrorVal(failed), tenon.ErrorVal(failed), true},
		{
			"errors whose messages differ", tenon.ErrorVal(failed),
			tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: "it failed differently"}), false,
		},
		{
			"errors whose diagnostics are in different orders",
			tenon.ErrorVal(failed, other), tenon.ErrorVal(other, failed), false,
		},
		{
			"errors whose paths differ", tenon.ErrorVal(failed),
			tenon.ErrorVal(tenon.Diagnostic{Code: failed.Code, Message: failed.Message, Path: tenon.Path{}.Attribute("a")}),
			false,
		},
		// A pending value is its constraint and what it says about null.
		{"pendings with one constraint", tenon.Pending(tenon.Exactly(str)), tenon.Pending(tenon.Exactly(str)), true},
		{"pendings with different constraints", tenon.Pending(tenon.Exactly(str)), pending, false},
		{"pendings that differ about null", tenon.Narrow(pending, tenon.Null()), pending, false},
		{
			"pendings that agree about null",
			tenon.Narrow(pending, tenon.NotNull()), tenon.Narrow(pending, tenon.NotNull()), true,
		},
		// Members are compared as values in their own right, so a container
		// holding an unknown is identical to one holding the same unknown.
		{
			"lists holding one unknown",
			tenon.ListVal(num, tenon.Unknown(num)), tenon.ListVal(num, tenon.Unknown(num)), true,
		},
		{
			"lists holding different unknowns",
			tenon.ListVal(num, tenon.Unknown(num)), tenon.ListVal(num, bounded(true)), false,
		},
	} {
		if got := tenon.Identical(tt.a, tt.b); got != tt.want {
			t.Errorf("%s: identical is %t, want %t", tt.name, got, tt.want)
		}
		if got := tenon.Identical(tt.b, tt.a); got != tt.want {
			t.Errorf("%s: the other way about is %t, want %t", tt.name, got, tt.want)
		}
	}
	// It answers with a plain bool where the language's own equality cannot
	// answer at all.
	if !tenon.Identical(tenon.Unknown(num), tenon.Unknown(num)) {
		t.Error("two unknowns with one range are not identical")
	}
	if got := tenon.Equals(tenon.Unknown(num), tenon.Unknown(num)).String(); got != "unknown(bool, not null)" {
		t.Errorf("Equals of those two unknowns is %s, want an unknown Bool", got)
	}
	mustPanicUsage(t, "use of the zero Value", func() { tenon.Identical(tenon.Value{}, one) })
}

func TestConformance_EQ011_IdenticalIsAnEquivalenceRelation(t *testing.T) {
	conformance.Covers(t, "EQ-011")
	all := values.All()
	// The generator holds values that are identical without being the same
	// node, so what follows is not comparing everything only with itself.
	pairs := 0
	for i, a := range all {
		for _, b := range all[i+1:] {
			if tenon.Identical(a, b) {
				pairs++
			}
		}
	}
	if pairs < 3 {
		t.Errorf("the generator holds %d pairs of distinct values that are identical, too few to test transitivity", pairs)
	}
	for _, a := range all {
		if !tenon.Identical(a, a) {
			t.Errorf("%v is not identical to itself", a)
		}
		for _, b := range all {
			ab, ba := tenon.Identical(a, b), tenon.Identical(b, a)
			if ab != ba {
				t.Errorf("%v and %v: identical is %t one way about and %t the other", a, b, ab, ba)
				continue
			}
			if !ab {
				continue
			}
			for _, c := range all {
				if tenon.Identical(b, c) && !tenon.Identical(a, c) {
					t.Errorf("%v is %v and %v is %v, but the first and the last are not identical", a, b, b, c)
				}
			}
		}
	}
}

func TestConformance_EQ012_IdenticalDoesNotDependOnMapOrder(t *testing.T) {
	conformance.Covers(t, "EQ-012")
	num := tenon.NumberType()
	one := tenon.NumberFromInt(1)
	attrs := map[string]tenon.Value{
		"a": one,
		"b": tenon.Unknown(num),
		"c": tenon.NullVal(num),
		"d": tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(one, true)),
		"e": tenon.NumberFromInt(2),
		"f": tenon.Unknown(num),
		"g": tenon.NumberFromInt(3),
		"h": tenon.Unknown(num),
	}
	differs := map[string]tenon.Value{}
	for k, v := range attrs {
		differs[k] = v
	}
	differs["f"] = tenon.Narrow(tenon.Unknown(num), tenon.NotNull())
	// Objects and maps are built by walking a Go map, whose order changes from
	// one walk to the next. The answer does not.
	for i := range conformance.Iterations(t, 500) {
		if !tenon.Identical(tenon.ObjectVal(attrs), tenon.ObjectVal(attrs)) {
			t.Fatalf("pass %d: two objects built from one map are not identical", i)
		}
		if tenon.Identical(tenon.ObjectVal(attrs), tenon.ObjectVal(differs)) {
			t.Fatalf("pass %d: two objects that differ in one attribute are identical", i)
		}
		if !tenon.Identical(tenon.MapVal(num, attrs), tenon.MapVal(num, attrs)) {
			t.Fatalf("pass %d: two maps built from one Go map are not identical", i)
		}
		if tenon.Identical(tenon.MapVal(num, attrs), tenon.MapVal(num, differs)) {
			t.Fatalf("pass %d: two maps that differ in one entry are identical", i)
		}
	}
}
