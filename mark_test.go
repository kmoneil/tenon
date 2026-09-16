package tenon_test

import (
	"testing"

	"tenon"
	"tenon/conformance"
)

// stamp is a Mark for tests: comparable, with a policy and a redaction flag.
type stamp struct {
	id     string
	policy tenon.Propagation
	redact bool
}

func (m stamp) MarkID() string                 { return m.id }
func (m stamp) Propagation() tenon.Propagation { return m.policy }
func (m stamp) Redacting() bool                { return m.redact }

// slippery is a Mark whose type is not comparable, which WithMarks refuses.
type slippery struct {
	stamp
	payload []byte
}

func TestConformance_MK001_MarksAreTypedMetadata(t *testing.T) {
	conformance.Covers(t, "MK-001")
	num := tenon.NumberType()
	secret := stamp{id: "secret", redact: true}
	origin := stamp{id: "origin"}
	one := tenon.NumberFromInt(1)

	// A mark declares what the interface asks of it.
	if secret.MarkID() != "secret" || secret.Propagation() != tenon.Propagate || !secret.Redacting() {
		t.Errorf("the mark %v does not declare what it was built with", secret)
	}

	// Attaching a mark makes a marked value and leaves the original alone.
	m1 := tenon.WithMarks(one, secret)
	if !tenon.HasMark(m1, secret) || tenon.HasMark(m1, origin) {
		t.Errorf("%v carries the wrong marks", m1)
	}
	if tenon.HasMark(one, secret) {
		t.Error("marking a value changed the original")
	}

	// The marked value is still the value: its content reads as before.
	if got, ok := m1.AsInt64(); !ok || got != 1 {
		t.Errorf("the marked value reads as %d, %t, want 1, true", got, ok)
	}

	// A mark attaches once however often it is given, and Unmark returns the
	// marks sorted by identifier.
	m2 := tenon.WithMarks(tenon.WithMarks(m1, secret), origin, secret)
	u, ms := tenon.Unmark(m2)
	if len(ms) != 2 || ms[0].MarkID() != "origin" || ms[1].MarkID() != "secret" {
		t.Errorf("Unmark returned %v, want origin then secret", ms)
	}
	if tenon.HasMark(u, secret) || tenon.HasMark(u, origin) {
		t.Errorf("%v still carries marks after Unmark", u)
	}

	// Unmarking an unmarked value and attaching nothing are both the value.
	if u2, ms2 := tenon.Unmark(one); u2 != one || ms2 != nil {
		t.Errorf("Unmark of an unmarked value returned %v, %v, want the value and no marks", u2, ms2)
	}
	if got := tenon.WithMarks(m1); got != m1 {
		t.Errorf("attaching no marks returned %v, want the value itself", got)
	}

	// Every state can carry a mark: marks are metadata, not content.
	for _, v := range []tenon.Value{
		tenon.NullVal(num),
		tenon.Unknown(num),
		tenon.Pending(tenon.Any()),
		tenon.ErrorVal(tenon.Diagnostic{Code: "app.x", Message: "m"}),
	} {
		if !tenon.HasMark(tenon.WithMarks(v, secret), secret) {
			t.Errorf("%v did not take a mark", v)
		}
	}

	// A mark must be comparable, and must not be nil.
	mustPanicUsage(t, "not comparable", func() {
		tenon.WithMarks(one, slippery{stamp: stamp{id: "s"}})
	})
	mustPanicUsage(t, "nil Mark", func() {
		tenon.WithMarks(one, nil)
	})
}

func TestConformance_MK002_PropagationPolicies(t *testing.T) {
	conformance.Covers(t, "MK-002")
	num := tenon.NumberType()
	prop := stamp{id: "prop"}
	iso := stamp{id: "iso", policy: tenon.Isolate}
	five := tenon.NumberFromInt(5)
	marked := tenon.WithMarks(tenon.NumberFromInt(1), prop, iso)

	// A Propagate mark appears on the result of every operation consuming
	// the marked value; an Isolate mark stays behind.
	for _, tt := range []struct {
		name string
		r    tenon.Value
	}{
		{"Add", tenon.Add(marked, five)},
		{"And", tenon.And(tenon.WithMarks(tenon.Bool(true), prop, iso), tenon.Bool(false))},
		{"Equals", tenon.Equals(marked, five)},
	} {
		if !tenon.HasMark(tt.r, prop) {
			t.Errorf("%s: the Propagate mark did not reach %v", tt.name, tt.r)
		}
		if tenon.HasMark(tt.r, iso) {
			t.Errorf("%s: the Isolate mark transferred to %v", tt.name, tt.r)
		}
	}
	if !tenon.HasMark(marked, iso) || !tenon.HasMark(marked, prop) {
		t.Error("the operand lost its own marks")
	}

	// Narrowing refines the value rather than deriving a new one, so every
	// mark stays, Isolate included, whether the result is still a range or
	// has come down to one value.
	u := tenon.WithMarks(tenon.Unknown(num), prop, iso)
	nu := tenon.Narrow(u, tenon.NotNull())
	if !tenon.HasMark(nu, prop) || !tenon.HasMark(nu, iso) {
		t.Errorf("narrowing dropped marks: %v", nu)
	}
	k := tenon.Narrow(u, tenon.NotNull(), tenon.NumberMin(five, true), tenon.NumberMax(five, true))
	if !k.IsKnown() {
		t.Fatalf("bounds that meet produced %v, want the known value", k)
	}
	if !tenon.HasMark(k, prop) || !tenon.HasMark(k, iso) {
		t.Errorf("collapsing to one value dropped marks: %v", k)
	}

	// Resolving a pending value refines it the same way.
	p := tenon.WithMarks(tenon.Pending(tenon.Any()), prop, iso)
	rv := tenon.Resolve(p, num)
	if !tenon.HasMark(rv, prop) || !tenon.HasMark(rv, iso) {
		t.Errorf("resolving dropped marks: %v", rv)
	}
}

func TestConformance_MK003_ResultMarksAreTheUnion(t *testing.T) {
	conformance.Covers(t, "MK-003")
	num := tenon.NumberType()
	a, b, shared := stamp{id: "a"}, stamp{id: "b"}, stamp{id: "shared"}
	iso := stamp{id: "iso", policy: tenon.Isolate}
	x := tenon.WithMarks(tenon.NumberFromInt(1), a, shared, iso)
	y := tenon.WithMarks(tenon.NumberFromInt(2), b, shared)

	// The result carries each operand's Propagate marks, a mark on both
	// operands once, and no Isolate mark.
	sum := tenon.Add(x, y)
	if got, ok := sum.AsInt64(); !ok || got != 3 {
		t.Errorf("the marked sum reads as %d, %t, want 3, true", got, ok)
	}
	if _, ms := tenon.Unmark(sum); len(ms) != 3 {
		t.Errorf("the sum carries %v, want the union a, b, shared", ms)
	}
	for _, m := range []stamp{a, b, shared} {
		if !tenon.HasMark(sum, m) {
			t.Errorf("the sum is missing %v", m)
		}
	}
	if tenon.HasMark(sum, iso) {
		t.Error("the sum carries the Isolate mark")
	}

	// A result decided by one operand still carries the union: consuming is
	// what propagates, not deciding.
	f := tenon.And(tenon.WithMarks(tenon.Bool(false), a), tenon.WithMarks(tenon.Bool(true), b))
	if f.String() != "false" || !tenon.HasMark(f, a) || !tenon.HasMark(f, b) {
		t.Errorf("And decided by false is %v with the wrong marks", f)
	}

	// An unknown result carries the union too.
	un := tenon.Equals(tenon.WithMarks(tenon.Unknown(num), a), tenon.NumberFromInt(3))
	if un.IsKnown() || !tenon.HasMark(un, a) {
		t.Errorf("the unknown result %v does not carry the operand's mark", un)
	}
}
