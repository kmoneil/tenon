package tenon_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
)

// TestConformance_UN025_APendingTupleOrObjectHoldsItsMembers holds a tuple or
// an object of a member that is pending to the pending value holding its
// members: its members read by position or by name as a tuple's or an
// object's are; its constraint, TupleOf or a closed ObjectWith of required
// fields, each member's constraint; not null; resolving to a tuple or object
// type that satisfies the constraint, member by member; displaying as the
// tuple or object it will be; and its error members still winning.
func TestConformance_UN025_APendingTupleOrObjectHoldsItsMembers(t *testing.T) {
	conformance.Covers(t, "UN-025", "VA-001")
	p, one := tenon.Pending(tenon.Any()), n(1)
	tup := tenon.Tuple(p, one)
	if !tup.IsPending() || !tup.HasMembers() || tup.IsResolved() {
		t.Fatalf("%v is not a pending value holding its members", tup)
	}
	if tup.Len() != 2 || !tenon.Identical(tup.Index(1), one) || !tenon.Identical(tup.Index(0), p) {
		t.Errorf("%v reads as %d members, %v and %v", tup, tup.Len(), tup.Index(0), tup.Index(1))
	}
	if got := tup.Elements(); !slices.EqualFunc(got, []tenon.Value{p, one}, tenon.Identical) {
		t.Errorf("Elements of %v = %v", tup, got)
	}
	if got := slices.Collect(tup.ElementsSeq()); !slices.EqualFunc(got, []tenon.Value{p, one}, tenon.Identical) {
		t.Errorf("ElementsSeq of %v = %v", tup, got)
	}
	if want := tenon.TupleOf(tenon.Any(), is(num)); !tup.Constraint().Equal(want) {
		t.Errorf("the constraint of %v is %v, want %v", tup, tup.Constraint(), want)
	}
	if got := tenon.IsNull(tup); !got.IsKnown() || got.AsBool() {
		t.Errorf("IsNull(%v) = %v, want false", tup, got)
	}
	if got := tenon.Narrow(tup, tenon.NotNull()); !tenon.SameNode(got, tup) {
		t.Errorf("NotNull made a new value of %v: %v", tup, got)
	}
	if got := tenon.Narrow(tup, tenon.NullOnly()); !got.IsError() || got.Diagnostics()[0].Code != tenon.CodeRangeContradiction {
		t.Errorf("NullOnly of %v = %v, want a contradiction", tup, got)
	}

	obj := tenon.Object(map[string]tenon.Value{"b": one, "a": p})
	if !obj.IsPending() || !obj.HasMembers() || obj.Len() != 2 || !tenon.Identical(obj.Attribute("b"), one) {
		t.Errorf("%v does not read as the pending object holding a and b", obj)
	}
	if _, ok := obj.LookupAttribute("c"); ok {
		t.Errorf("%v has an attribute c", obj)
	}
	var names []string
	for name := range obj.Attributes() {
		names = append(names, name)
	}
	if !slices.Equal(names, []string{"a", "b"}) {
		t.Errorf("the attributes of %v are %v, want a and b in order", obj, names)
	}
	want := tenon.ObjectWith(map[string]tenon.Field{"a": tenon.Required(tenon.Any()), "b": tenon.Required(is(num))}, true)
	if !obj.Constraint().Equal(want) {
		t.Errorf("the constraint of %v is %v, want %v", obj, obj.Constraint(), want)
	}

	nested := tenon.Tuple(tenon.Tuple(p), s("x"))
	if want := tenon.TupleOf(tenon.TupleOf(tenon.Any()), is(str)); !nested.Constraint().Equal(want) {
		t.Errorf("the constraint of %v is %v, want %v", nested, nested.Constraint(), want)
	}

	for _, tt := range []struct {
		name string
		v    tenon.Value
		t    tenon.Type
		want tenon.Value
	}{
		{"a tuple", tup, tenon.TupleType(str, num), tenon.Tuple(tenon.Unknown(str), one)},
		{"an object", obj, tenon.ObjectType(map[string]tenon.Type{"a": tenon.BoolType(), "b": num}),
			tenon.Object(map[string]tenon.Value{"a": tenon.Unknown(tenon.BoolType()), "b": one})},
		{"nested", nested, tenon.TupleType(tenon.TupleType(num), str), tenon.Tuple(tenon.Tuple(tenon.Unknown(num)), s("x"))},
	} {
		wantValue(t, "resolving "+tt.name, tenon.Resolve(tt.v, tt.t), tt.want)
	}
	mustPanicUsage(t, "does not satisfy the constraint", func() { tenon.Resolve(tup, tenon.TupleType(str, str)) })

	for _, tt := range []struct {
		v    tenon.Value
		want string
	}{
		{tup, "[pending(any), 1]"},
		{obj, `{"a": pending(any), "b": 1}`},
		{nested, `[[pending(any)], "x"]`},
	} {
		if got := tt.v.String(); got != tt.want {
			t.Errorf("%#v displays as %s, want %s", tt.v, got, tt.want)
		}
	}

	failed := tenon.ErrorVal(tenon.Diagnostic{Code: "app.x", Message: "m"})
	if got := tenon.Tuple(failed, p); !got.IsError() {
		t.Errorf("a tuple of an error value and a pending one is %v, want the error value", got)
	}
	mustPanicUsage(t, "is a pending value", func() { tenon.List(num, p) })
	mustPanicUsage(t, "applies to a pending value only where", func() { tenon.Narrow(tup, tenon.LengthMin(1)) })

	if tenon.Pending(tenon.TupleOf(tenon.Any())).HasMembers() || tenon.Unknown(tenon.ListType(num)).HasMembers() || one.HasMembers() {
		t.Errorf("a value with no members to read says it has some")
	}
	if !tenon.List(num, one).HasMembers() || !tenon.Tuple(tenon.Unknown(num)).HasMembers() {
		t.Errorf("a container holding members says it has none")
	}
}

// TestConformance_EQ010_APendingTupleOrObjectIsItsMembers holds Identical to
// the members a pending tuple or object holds, and %#v to the call that makes
// it.
func TestConformance_EQ010_APendingTupleOrObjectIsItsMembers(t *testing.T) {
	conformance.Covers(t, "EQ-010", "UN-025")
	p := tenon.Pending(tenon.Any())
	tup := tenon.Tuple(p, n(1))
	if !tenon.Identical(tup, tenon.Tuple(tenon.Pending(tenon.Any()), n(1))) {
		t.Errorf("two pending tuples built alike are not identical")
	}
	for _, other := range []tenon.Value{
		tenon.Tuple(p, n(2)),
		tenon.Tuple(tenon.Narrow(p, tenon.NotNull()), n(1)),
		tenon.Narrow(tenon.Pending(tenon.TupleOf(tenon.Any(), is(num))), tenon.NotNull()),
	} {
		if tenon.Identical(tup, other) {
			t.Errorf("%v and %v are identical", tup, other)
		}
	}
	if got, want := fmt.Sprintf("%#v", tup), "tenon.Tuple(tenon.Pending(tenon.Any()), tenon.NumberFromInt(1))"; got != want {
		t.Errorf("%%#v is %s, want %s", got, want)
	}
	obj := tenon.Object(map[string]tenon.Value{"a": p})
	if got, want := fmt.Sprintf("%#v", obj), `tenon.Object(map[string]tenon.Value{"a":tenon.Pending(tenon.Any())})`; got != want {
		t.Errorf("%%#v is %s, want %s", got, want)
	}
}
