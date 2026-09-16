package tenon

import (
	"testing"

	"tenon/conformance"
)

// probe is a Mark for the internal tests.
type probe struct{ id string }

func (m probe) MarkID() string           { return m.id }
func (m probe) Propagation() Propagation { return Propagate }
func (m probe) Redacting() bool          { return false }

func TestConformance_MK007_UnmarkedValuesPayNothing(t *testing.T) {
	conformance.Covers(t, "MK-007")
	str, num := Type{stringType}, Type{numberType}
	// The mark storage of an unmarked value is a nil pointer, whatever the
	// state of the value: no empty container, no allocation. A container of
	// unmarked members does not claim to hold a marked one either.
	for _, v := range []Value{
		Bool(true),
		NumberFromInt(1),
		String("x"),
		NullVal(str),
		Unknown(str),
		Narrow(Unknown(num), NotNull()),
		ListVal(str, String("a")),
		SetVal(str, String("a")),
		TupleVal(String("a")),
		ObjectVal(map[string]Value{"a": String("a")}),
		MapVal(str, map[string]Value{"k": String("a")}),
		Pending(Any()),
		ErrorVal(Diagnostic{Code: "app.x", Message: "m"}),
	} {
		if v.n.marks != nil {
			t.Errorf("%v was never marked but carries mark storage", v)
		}
		if v.n.markedWithin {
			t.Errorf("%v holds no marked value but says it does", v)
		}
	}
	// Unmarking returns to the nil pointer, not to an empty set.
	u, _ := Unmark(WithMarks(NumberFromInt(1), probe{id: "m"}))
	if u.n.marks != nil {
		t.Error("Unmark left mark storage behind")
	}
	// Building unmarked values allocates exactly what a system without
	// marks would: the node and its content. These counts are the pre-marks
	// profile, and a mark-caused allocation on this path fails here.
	for _, tt := range []struct {
		name string
		want float64
		f    func()
	}{
		{"Bool", 0, func() { Bool(true) }},
		{"NullVal", 1, func() { NullVal(str) }},
		{"Unknown", 2, func() { Unknown(str) }},
		{"NumberFromInt", 2, func() { NumberFromInt(42) }},
	} {
		if got := testing.AllocsPerRun(200, tt.f); got != tt.want {
			t.Errorf("%s allocates %v times, want %v", tt.name, got, tt.want)
		}
	}
}

// TestMarkedWithinAgreesWithTheMembers holds the flag that says a value holds a
// marked member, which saves hashing, ordering and set construction a walk, to
// what that walk finds, over containers built every way the package builds
// them.
func TestMarkedWithinAgreesWithTheMembers(t *testing.T) {
	m := probe{id: "m"}
	num := Type{numberType}
	one := NumberFromInt(1)
	marked := WithMarks(one, m)
	nested := ListVal(List(num), ListVal(num, one, marked))
	ownTaken, _ := Unmark(WithMarks(nested, m))
	allTaken, _ := UnmarkDeep(WithMarks(nested, m))
	for _, v := range []Value{
		ListVal(num, one),
		ListVal(num, marked),
		nested,
		SetVal(num, one),
		SetVal(List(num), ListVal(num, one)),
		TupleVal(one, marked),
		TupleVal(WithMarks(Unknown(num), m)),
		ObjectVal(map[string]Value{"a": marked, "b": one}),
		ObjectVal(map[string]Value{"a": one}),
		MapVal(num, map[string]Value{"k": marked, "j": one}),
		MapVal(num, map[string]Value{"k": one}),
		WithMarks(nested, m),
		ownTaken,
		allTaken,
		Narrow(nested, LengthMin(1)),
		Narrow(Unknown(Set(num)), NotNull(), Members(one), LengthMax(1)),
		Narrow(Unknown(Tuple(Tuple())), NotNull()),
	} {
		checkMarkedWithin(t, v)
	}
	if !nested.n.markedWithin || !ownTaken.n.markedWithin || allTaken.n.markedWithin {
		t.Error("taking a value's own marks or all of them left the flag wrong")
	}
}

// checkMarkedWithin fails t unless v, and every value v holds, says it holds a
// marked member exactly when one of its members carries a mark or holds one.
func checkMarkedWithin(t *testing.T, v Value) {
	t.Helper()
	var members []Value
	if v.n.state == stateKnown {
		switch data := v.n.data.(type) {
		case []Value:
			members = data
		case []mapEntry:
			for _, e := range data {
				members = append(members, e.val)
			}
		}
	}
	want := false
	for _, member := range members {
		checkMarkedWithin(t, member)
		want = want || member.n.marks != nil || member.n.markedWithin
	}
	if v.n.markedWithin != want {
		t.Errorf("%v says it holds a marked member: %t, want %t", v, v.n.markedWithin, want)
	}
}

// BenchmarkUnmarkedValues is the memory profile of building values that are
// never marked, for comparison against the counts above and across changes
// to the mark representation.
func BenchmarkUnmarkedValues(b *testing.B) {
	b.ReportAllocs()
	str := Type{stringType}
	for range b.N {
		l := ListVal(str, String("a"), String("b"))
		_ = Length(l)
		_ = Narrow(Unknown(str), NotNull(), LengthMin(1))
	}
}
