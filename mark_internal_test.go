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
	// state of the value: no empty container, no allocation.
	for _, v := range []Value{
		Bool(true),
		NumberFromInt(1),
		String("x"),
		NullVal(str),
		Unknown(str),
		Narrow(Unknown(num), NotNull()),
		ListVal(str, String("a")),
		Pending(Any()),
		ErrorVal(Diagnostic{Code: "app.x", Message: "m"}),
	} {
		if v.n.marks != nil {
			t.Errorf("%v was never marked but carries mark storage", v)
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
