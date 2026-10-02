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

// TestConformance_SE010_APendingTupleOrObjectEncodesItsMembers pins the item
// of a pending tuple or object holding members, [3, 0, items] and [3, 1,
// [name, item] pairs], each member a whole item, read back as itself, marks
// on it and on its members included; a failure within a member located at
// it; and the input that describes no such value refused.
func TestConformance_SE010_APendingTupleOrObjectEncodesItsMembers(t *testing.T) {
	conformance.Covers(t, "SE-010", "SE-002", "UN-025")
	p := tenon.Pending(tenon.Any())
	const pendingAny, one = "83 01 81 02 00", "83 00 02 01"
	for _, tt := range []struct {
		name string
		v    tenon.Value
		item string
	}{
		{"a tuple", tenon.Tuple(p, n(1)), "83 03 00 82 " + pendingAny + " " + one},
		{"an object", tenon.Object(map[string]tenon.Value{"b": n(1), "a": p}), "83 03 01 82 82 6161 " + pendingAny + " 82 6162 " + one},
	} {
		wantEncoding(t, tt.name, tt.v, tt.item)
	}
	for _, v := range []tenon.Value{
		tenon.Tuple(tenon.Tuple(p), s("x")),
		tenon.WithMarks(tenon.Tuple(p, n(1)), markPlain),
		tenon.Tuple(tenon.WithMarks(p, markPlain), tenon.WithMarks(n(1), markPlain)),
		tenon.Object(map[string]tenon.Value{"a": tenon.Narrow(tenon.Pending(tenon.ListOf(tenon.Any())), tenon.LengthMin(2)), "b": tenon.Unknown(num)}),
	} {
		b, _, ok := trySerialize(v)
		if !ok {
			t.Errorf("%v did not serialize", v)
			continue
		}
		if got, failure, ok := tryDeserialize(b, decoders); !ok || !tenon.Identical(got, v) {
			t.Errorf("%v came back as %v, %v", v, got, failure)
		}
	}
	wantSerializeFailure(t, "an unencodable mark on a member", tenon.Tuple(p, tenon.WithMarks(n(1), stamp{id: "x"})),
		wantDiag{tenon.CodeSerializeUnencodableMark, ".[1]"})

	failed := "82 02 81 83 65 6170702e78 61 6d 80"
	for _, tt := range []struct {
		name, item string
		code       tenon.Code
	}{
		{"no member pending", "83 03 00 81 " + one, tenon.CodeSerializeMalformed},
		{"no member at all", "83 03 00 80", tenon.CodeSerializeMalformed},
		{"an error value as a member", "83 03 00 82 " + pendingAny + " " + failed, tenon.CodeSerializeMalformed},
		{"a name twice", "83 03 01 82 82 6161 " + pendingAny + " 82 6161 " + one, tenon.CodeSerializeMalformed},
		{"an empty name", "83 03 01 81 82 60 " + pendingAny, tenon.CodeSerializeMalformed},
		{"a shape that is neither", "83 03 02 81 " + pendingAny, tenon.CodeSerializeMalformed},
		{"names out of order", "83 03 01 82 82 6162 " + one + " 82 6161 " + pendingAny, tenon.CodeSerializeNotCanonical},
	} {
		wantDecodeFailure(t, tt.name, document+tt.item, tt.code)
	}
}

// TestConformance_UN023_EqualsComparesAPendingTuplesMembers holds Equals with a
// pending tuple or object holding members, and Contains through it, to its
// members: known false where a pair of members is known unequal or the shapes
// differ, known true where every pair is known equal, unknown otherwise.
func TestConformance_UN023_EqualsComparesAPendingTuplesMembers(t *testing.T) {
	conformance.Covers(t, "UN-023", "UN-025", "EQ-003")
	p := tenon.Pending(tenon.Any())
	tup := tenon.Tuple(p, n(1))
	nullStr := tenon.Narrow(tenon.Pending(is(str)), tenon.NullOnly())
	for _, tt := range []struct {
		name string
		a, b tenon.Value
		want string
	}{
		{"a member known unequal", tup, tenon.Tuple(s("x"), n(2)), "false"},
		{"the rest left open", tup, tenon.Tuple(s("x"), n(1)), "unknown(bool, not null)"},
		{"two alike", tup, tenon.Tuple(p, n(1)), "unknown(bool, not null)"},
		{"every pair known equal", tenon.Tuple(nullStr, n(1)), tenon.Tuple(tenon.Null(str), n(1)), "true"},
		{"another length", tup, tenon.Tuple(s("x")), "false"},
		{"an object", tup, tenon.Object(map[string]tenon.Value{"a": n(1)}), "false"},
		{"objects of other names", tenon.Object(map[string]tenon.Value{"a": p}), tenon.Object(map[string]tenon.Value{"b": n(1)}), "false"},
		{"an object's member known unequal", tenon.Object(map[string]tenon.Value{"a": p, "b": n(1)}),
			tenon.Object(map[string]tenon.Value{"a": s("x"), "b": n(2)}), "false"},
		{"an unknown tuple", tup, tenon.Unknown(tenon.TupleType(str, num)), "unknown(bool, not null)"},
		{"null", tup, tenon.Null(tenon.TupleType(str, num)), "false"},
	} {
		if got := tenon.Equals(tt.a, tt.b).String(); got != tt.want {
			t.Errorf("%s: Equals(%v, %v) = %s, want %s", tt.name, tt.a, tt.b, got, tt.want)
		}
		if got := tenon.Equals(tt.b, tt.a).String(); got != tt.want {
			t.Errorf("%s, the other way: Equals(%v, %v) = %s, want %s", tt.name, tt.b, tt.a, got, tt.want)
		}
	}
	pairs := tenon.TupleType(str, num)
	if got := tenon.Contains(tenon.Set(pairs, tenon.Tuple(s("x"), n(2))), tup).String(); got != "false" {
		t.Errorf("a set of no member that could be %v contains it: %s", tup, got)
	}
	if got := tenon.Contains(tenon.Set(pairs, tenon.Tuple(s("x"), n(1))), tup).String(); got != "unknown(bool, not null)" {
		t.Errorf("a set of a member that could be %v contains it: %s, want unknown", tup, got)
	}
}

// heldMembers returns the members of a tuple or object v, a pending one
// holding them among them, in order or in name order.
func heldMembers(v tenon.Value) []tenon.Value {
	if v.Constraint().Kind() == tenon.ConstraintTupleOf {
		return v.Elements()
	}
	var members []tenon.Value
	for _, m := range v.Attributes() {
		members = append(members, m)
	}
	return members
}

// TestConformance_MK008_ADeepMarkReachesAPendingTuplesMembers holds a deep mark
// on a pending tuple or object holding members to every member, as on a
// tuple, and UnmarkDeep to taking the marks from them all.
func TestConformance_MK008_ADeepMarkReachesAPendingTuplesMembers(t *testing.T) {
	conformance.Covers(t, "MK-008", "UN-025")
	deep := stamp{id: "d", deep: true}
	p := tenon.Pending(tenon.Any())
	for _, v := range []tenon.Value{tenon.Tuple(p, n(1)), tenon.Object(map[string]tenon.Value{"a": p, "b": n(1)})} {
		marked := tenon.WithMarks(v, deep)
		for _, m := range heldMembers(marked) {
			if !tenon.HasMark(m, deep) {
				t.Errorf("a member of %v does not carry the deep mark: %v", marked, m)
			}
		}
		plain, marks := tenon.UnmarkDeep(marked)
		if !tenon.Identical(plain, v) || !slices.ContainsFunc(marks, func(m tenon.Mark) bool { return m == deep }) {
			t.Errorf("UnmarkDeep(%v) = %v, %v; want %v and the deep mark", marked, plain, marks, v)
		}
		// Read back from its encoding, the deep mark reaches the members
		// again.
		withDeep := tenon.WithMarks(v, markDeep)
		b, _, ok := trySerialize(withDeep)
		if got, failure, back := tryDeserialize(b, decoders); !ok || !back || !tenon.Identical(got, withDeep) {
			t.Errorf("%v came back as %v, %v", withDeep, got, failure)
		}
	}
}
