package tenon_test

import (
	"slices"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
)

// doubled answers a known number doubled, without its marks, and any other
// value as it is.
func doubled(_ tenon.Path, v tenon.Value) tenon.Value {
	if v.IsKnown() && !v.IsNull() && v.Type().Equal(tenon.NumberType()) {
		n, _ := v.AsInt64()
		return tenon.NumberFromInt(2 * n)
	}
	return v
}

// texts answers a known number as the string of its digits, and any other
// value as it is.
func texts(_ tenon.Path, v tenon.Value) tenon.Value {
	if v.IsKnown() && !v.IsNull() && v.Type().Equal(tenon.NumberType()) {
		return tenon.String(v.String())
	}
	return v
}

// transformsTo checks that transforming v with fn answers want.
func transformsTo(t *testing.T, v tenon.Value, fn func(tenon.Path, tenon.Value) tenon.Value, want tenon.Value) {
	t.Helper()
	if got := tenon.Transform(v, fn); !tenon.Identical(got, want) {
		t.Errorf("Transform(%v) = %v, want %v", v, got, want)
	}
}

// transformFails checks that transforming v with fn fails with code, located
// at loc.
func transformFails(t *testing.T, v tenon.Value, fn func(tenon.Path, tenon.Value) tenon.Value, code tenon.Code, loc tenon.Path) {
	t.Helper()
	got := tenon.Transform(v, fn)
	if !got.IsError() || got.Diagnostics()[0].Code != code || !got.Diagnostics()[0].Path.Equal(loc) {
		t.Errorf("Transform(%v) = %v, want %s at %s", v, got, code, loc)
	}
}

func TestConformance_VA028_Transform(t *testing.T) {
	conformance.Covers(t, "VA-028")
	str, num := tenon.StringType(), tenon.NumberType()
	one, two, three := tenon.NumberFromInt(1), tenon.NumberFromInt(2), tenon.NumberFromInt(3)
	doc := tenon.Object(map[string]tenon.Value{
		"z":    tenon.String("last"),
		"a":    tenon.List(num, one, two),
		"m":    tenon.Map(str, map[string]tenon.Value{"k2": tenon.String("2"), "k1": tenon.String("1")}),
		"s":    tenon.Set(num, three, one),
		"held": tenon.Tuple(tenon.Pending(tenon.Any()), tenon.Object(map[string]tenon.Value{"x": tenon.Bool(true)})),
		"u":    tenon.Unknown(tenon.ListType(str)),
		"n":    tenon.Null(tenon.ListType(str)),
		"p":    tenon.Pending(tenon.Any()),
	})
	// Each value after the values within it, siblings in the canonical order.
	var order []string
	same := tenon.Transform(doc, func(p tenon.Path, v tenon.Value) tenon.Value {
		order = append(order, p.String())
		return v
	})
	want := []string{".a[0]", ".a[1]", ".a", ".held[0]", ".held[1].x", ".held[1]", ".held",
		`.m["k1"]`, `.m["k2"]`, ".m", ".n", ".p", ".s[0]", ".s[1]", ".s", ".u", ".z", "."}
	if !slices.Equal(order, want) {
		t.Errorf("Transform visits %v, want %v", order, want)
	}
	if !tenon.Identical(same, doc) {
		t.Errorf("Transform answering each value as it is gives %v", same)
	}
	// Each value holding members is rebuilt from what they became: an object
	// and a tuple of whatever they became, a list, a map and a set of the
	// type they share.
	transformsTo(t, doc.Attribute("a"), doubled, tenon.List(num, two, tenon.NumberFromInt(4)))
	transformsTo(t, doc.Attribute("a"), texts, tenon.List(str, tenon.String("1"), tenon.String("2")))
	transformsTo(t, tenon.Map(num, map[string]tenon.Value{"k": one}), texts,
		tenon.Map(str, map[string]tenon.Value{"k": tenon.String("1")}))
	transformsTo(t, tenon.Tuple(one, tenon.String("x")), texts, tenon.Tuple(tenon.String("1"), tenon.String("x")))
	transformsTo(t, tenon.Object(map[string]tenon.Value{"o": one}), texts,
		tenon.Object(map[string]tenon.Value{"o": tenon.String("1")}))
	transformsTo(t, tenon.Set(num, one, two), texts, tenon.Set(str, tenon.String("1"), tenon.String("2")))
	// A pending tuple's members became known, and so it is a tuple.
	transformsTo(t, doc.Attribute("held"), func(_ tenon.Path, v tenon.Value) tenon.Value {
		if v.IsPending() {
			return tenon.String("p")
		}
		return v
	}, tenon.Tuple(tenon.String("p"), tenon.Object(map[string]tenon.Value{"x": tenon.Bool(true)})))
	// A set's members that became equal are one member.
	transformsTo(t, tenon.Set(num, one, two, three), func(_ tenon.Path, v tenon.Value) tenon.Value {
		if v.Type().Equal(num) {
			return tenon.NumberFromInt(0)
		}
		return v
	}, tenon.Set(num, tenon.NumberFromInt(0)))
	// A pending member resolves to the type the others share, or to the
	// element type where none is resolved; one its constraint keeps from it
	// fails.
	pendingAt := func(i int, c tenon.Constraint) func(tenon.Path, tenon.Value) tenon.Value {
		return func(p tenon.Path, v tenon.Value) tenon.Value {
			if p.Equal(at(i)) {
				return tenon.Pending(c)
			}
			return v
		}
	}
	transformsTo(t, tenon.List(num, one, two), pendingAt(0, tenon.Any()), tenon.List(num, tenon.Unknown(num), two))
	transformsTo(t, tenon.List(num, one), pendingAt(0, tenon.Any()), tenon.List(num, tenon.Unknown(num)))
	transformFails(t, tenon.List(num, one, two), pendingAt(1, tenon.Exactly(str)), tenon.CodeConvertNoCommonType, tenon.Path{})
	// Members that share no type fail at the value they were to be rebuilt
	// into, with no panic.
	firstText := func(p tenon.Path, v tenon.Value) tenon.Value {
		if p.Equal(at("a", 0)) {
			return tenon.String("one")
		}
		return v
	}
	transformFails(t, doc, firstText, tenon.CodeConvertNoCommonType, at("a"))
	transformFails(t, tenon.Object(map[string]tenon.Value{"a": tenon.Set(num, one, two)}), firstText,
		tenon.CodeConvertNoCommonType, at("a"))
	transformFails(t, tenon.Object(map[string]tenon.Value{"a": tenon.Map(num, map[string]tenon.Value{"0": one, "1": two})}),
		func(p tenon.Path, v tenon.Value) tenon.Value {
			if p.Equal(at("a", tenon.String("0"))) {
				return tenon.Bool(true)
			}
			return v
		}, tenon.CodeConvertNoCommonType, at("a"))
	// Each rebuilt value carries its own marks again, a deep mark reaching
	// what it now holds; a set takes the marks of its members.
	own, outer := stamp{id: "own"}, stamp{id: "outer"}
	deep := stamp{id: "deep", deep: true}
	transformsTo(t, tenon.WithMarks(tenon.List(num, tenon.WithMarks(one, own), two), outer), doubled,
		tenon.WithMarks(tenon.List(num, two, tenon.NumberFromInt(4)), outer))
	transformsTo(t, tenon.WithMarks(tenon.List(num, one), deep), doubled,
		tenon.WithMarks(tenon.List(num, two), deep))
	transformsTo(t, tenon.WithMarks(tenon.Set(num, one, two), deep), doubled,
		tenon.WithMarks(tenon.Set(num, two, tenon.NumberFromInt(4)), deep))
	transformsTo(t, tenon.Set(num, one, two), func(p tenon.Path, v tenon.Value) tenon.Value {
		if p.Len() == 1 {
			return tenon.WithMarks(doubled(p, v), own)
		}
		return v
	}, tenon.WithMarks(tenon.Set(num, two, tenon.NumberFromInt(4)), own))
	// An error value a function answers is the answer, and nothing is
	// called after it.
	failure := tenon.ErrorVal(tenon.Diagnostic{Code: tenon.CodeOperationWrongType, Message: "refused"})
	calls := 0
	got := tenon.Transform(doc, func(p tenon.Path, v tenon.Value) tenon.Value {
		calls++
		if p.Equal(at("a", 1)) {
			return failure
		}
		return v
	})
	if !tenon.Identical(got, failure) || calls != 2 {
		t.Errorf("an error answered at the second value gives %v after %d calls", got, calls)
	}
	// Enter may replace a value, whose members are then visited, pass over
	// them, or stop the transform.
	var exits []string
	exitDoubling := func(p tenon.Path, v tenon.Value) tenon.Value {
		exits = append(exits, p.String())
		return doubled(p, v)
	}
	small := tenon.Object(map[string]tenon.Value{"a": tenon.List(num, one), "b": tenon.List(num, two), "c": tenon.List(num, three)})
	got = tenon.TransformWith(small, func(p tenon.Path, v tenon.Value) (tenon.Value, tenon.WalkAction) {
		switch p.String() {
		case ".a":
			return tenon.List(num, three, three), tenon.WalkContinue
		case ".b":
			return v, tenon.WalkSkip
		}
		return v, tenon.WalkContinue
	}, exitDoubling)
	wantDoc := tenon.Object(map[string]tenon.Value{
		"a": tenon.List(num, tenon.NumberFromInt(6), tenon.NumberFromInt(6)), "b": tenon.List(num, two),
		"c": tenon.List(num, tenon.NumberFromInt(6)),
	})
	if !tenon.Identical(got, wantDoc) {
		t.Errorf("TransformWith replacing .a and passing over .b gives %v, want %v", got, wantDoc)
	}
	if want := []string{".a[0]", ".a[1]", ".a", ".b", ".c[0]", ".c", "."}; !slices.Equal(exits, want) {
		t.Errorf("exit is given %v, want %v", exits, want)
	}
	exits = nil
	var enters []string
	got = tenon.TransformWith(small, func(p tenon.Path, v tenon.Value) (tenon.Value, tenon.WalkAction) {
		enters = append(enters, p.String())
		if p.Equal(at("b")) {
			return tenon.List(str, tenon.String("b")), tenon.WalkStop
		}
		return v, tenon.WalkContinue
	}, exitDoubling)
	wantDoc = tenon.Object(map[string]tenon.Value{
		"a": tenon.List(num, two), "b": tenon.List(str, tenon.String("b")), "c": tenon.List(num, three),
	})
	if !tenon.Identical(got, wantDoc) {
		t.Errorf("TransformWith stopping at .b gives %v, want %v", got, wantDoc)
	}
	if !slices.Equal(enters, []string{".", ".a", ".a[0]", ".b"}) || !slices.Equal(exits, []string{".a[0]", ".a"}) {
		t.Errorf("stopping at .b entered %v and exited %v", enters, exits)
	}
	// Every path handed out stays valid.
	var nest func(int) tenon.Value
	nest = func(d int) tenon.Value {
		if d == 0 {
			return tenon.NumberFromInt(0)
		}
		inner := nest(d - 1)
		return tenon.Tuple(inner, inner)
	}
	deepDoc := nest(8)
	var kept []tenon.Path
	var keptVals []tenon.Value
	tenon.Transform(deepDoc, func(p tenon.Path, v tenon.Value) tenon.Value {
		kept, keptVals = append(kept, p), append(keptVals, v)
		return v
	})
	if len(kept) != 511 {
		t.Fatalf("transformed %d values, want 511", len(kept))
	}
	for i, p := range kept {
		if !tenon.Identical(p.Apply(deepDoc), keptVals[i]) {
			t.Fatalf("the path %s kept from the transform no longer reaches the value it was handed with", p)
		}
	}
}
