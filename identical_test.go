package tenon_test

import (
	"fmt"
	"runtime"
	"slices"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/internal/conformance/values"
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

// A range is compared by what it records. A listing that holds a requirement
// the others imply allows the sets the listing without it allows, and the two
// are distinct values: not identical, encoded differently, each read back as
// itself, and answering alike for every set they are compared with.
func TestConformance_EQ010_RangesCompareByRecord(t *testing.T) {
	conformance.Covers(t, "EQ-010", "UN-002", "SE-001")
	num := tenon.NumberType()
	set := tenon.Set(num)
	one := tenon.NumberFromInt(1)
	atLeastZero := tenon.Narrow(tenon.Unknown(num), tenon.NotNull(), tenon.NumberMin(tenon.NumberFromInt(0), true))
	// 1 is at least 0, so a set holding 1 holds a member the second
	// requirement allows.
	implied := tenon.Narrow(tenon.Unknown(set), tenon.Members(one, atLeastZero))
	plain := tenon.Narrow(tenon.Unknown(set), tenon.Members(one))
	if tenon.Identical(implied, plain) {
		t.Errorf("%v and %v are identical, want them told apart by what they record", implied, plain)
	}
	a, _, okA := tenon.Serialize(implied)
	b, _, okB := tenon.Serialize(plain)
	if !okA || !okB || string(a) == string(b) {
		t.Errorf("the two encode alike, or not at all: %x and %x", a, b)
	}
	for _, v := range []tenon.Value{implied, plain} {
		data, _, _ := tenon.Serialize(v)
		if back, _, ok := tenon.Deserialize(data, tenon.Decoders{}); !ok || !tenon.Identical(back, v) {
			t.Errorf("%v reads back as %v", v, back)
		}
	}
	for _, s := range []tenon.Value{
		tenon.SetVal(num, one),
		tenon.SetVal(num, one, tenon.NumberFromInt(-1)),
		tenon.SetVal(num, tenon.NumberFromInt(2)),
		tenon.SetVal(num),
	} {
		if x, y := tenon.Equals(implied, s), tenon.Equals(plain, s); !tenon.Identical(x, y) {
			t.Errorf("against %v the two answer %v and %v, want them alike", s, x, y)
		}
	}
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

// TestConformance_EQ010_ManyMarksAreComparedThroughASet holds what comparing
// two values carrying many marks costs. Marks are a set, so the comparison
// asks of every mark one value carries whether the other carries it as well,
// and asking by a scan of the other's marks costs the square of them. A value
// can carry thousands: one arrives whenever a deep mark reaches it. A value
// holds its marks in one order, so two that carry the same marks usually hold
// them alike and one walk settles it; what does not line up is looked up
// through a set of them, which a handful never builds.
func TestConformance_EQ010_ManyMarksAreComparedThroughASet(t *testing.T) {
	conformance.Covers(t, "EQ-010", "MK-001")
	one := tenon.NumberFromInt(1)
	// Marks in runs of a few that share an identifier, each carrying a
	// payload of its own. Marks that share an identifier tie in the order a
	// value holds them in, the one attached first coming first, so two values
	// given them in opposite orders hold each run in opposite orders: the
	// case where the lists do not line up and every mark is looked for.
	marks := func(m, run int) []tenon.Mark {
		ms := make([]tenon.Mark, m)
		for i := range ms {
			ms[i] = note{id: fmt.Sprintf("m%06d", i%max(m/run, 1)), text: fmt.Sprintf("p%06d", i)}
		}
		return ms
	}
	backwards := func(ms []tenon.Mark) []tenon.Mark {
		r := slices.Clone(ms)
		slices.Reverse(r)
		return r
	}
	// The answer is what it was, on either side of the count past which the
	// marks are looked up through a set rather than scanned for.
	for _, m := range []int{4, 16, 17, 100} {
		ms := marks(m, 2)
		x := tenon.WithMarks(one, ms...)
		if y := tenon.WithMarks(one, backwards(ms)...); !tenon.Identical(x, y) {
			t.Errorf("two values carrying the same %d marks, attached in opposite orders, are not identical", m)
		}
		exchanged := slices.Clone(ms)
		exchanged[m-1] = note{id: "zz", text: "z"}
		if y := tenon.WithMarks(one, exchanged...); tenon.Identical(x, y) {
			t.Errorf("%d marks and the same marks with one exchanged are identical", m)
		}
		if y := tenon.WithMarks(one, ms[:m-1]...); tenon.Identical(x, y) {
			t.Errorf("%d marks and %d of them are identical", m, m-1)
		}
	}

	// A handful of marks is scanned for, which allocates nothing.
	few := marks(8, 4)
	x, y := tenon.WithMarks(one, few...), tenon.WithMarks(one, backwards(few)...)
	if allocs := testing.AllocsPerRun(100, func() { tenon.Identical(x, y) }); allocs != 0 {
		t.Errorf("comparing two values carrying eight marks made %.0f allocations, want none", allocs)
	}

	// Two values holding their marks alike, which is every pair whose marks
	// have identifiers of their own, are walked a mark at a time and build
	// nothing, however many marks they carry.
	lined := marks(1000, 1)
	a, b := tenon.WithMarks(one, lined...), tenon.WithMarks(one, backwards(lined)...)
	if !tenon.Identical(a, b) {
		t.Fatal("two values carrying the same 1,000 marks, attached in opposite orders, are not identical")
	}
	if allocs := testing.AllocsPerRun(10, func() { tenon.Identical(a, b) }); allocs != 0 {
		t.Errorf("comparing two values carrying 1,000 marks of identifiers of their own made %.0f allocations, want none: the two hold them alike", allocs)
	}

	// Past a handful the marks are looked up through a set, which takes a slot
	// for each of them: the reading is the bytes, which are the same on every
	// run where a wall clock is not. A scan allocates nothing and costs the
	// square of the marks, so the floor is what tells the two apart, and the
	// growth from a size to four times it says the set is built once rather
	// than for every mark.
	var allocated []uint64
	for _, m := range []int{1000, 4000} {
		ms := marks(m, 8)
		x, y := tenon.WithMarks(one, ms...), tenon.WithMarks(one, backwards(ms)...)
		if !tenon.Identical(x, y) {
			t.Fatalf("two values carrying the same %d marks, attached in opposite orders, are not identical", m)
		}
		const rounds = 10
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		for range rounds {
			tenon.Identical(x, y)
		}
		runtime.ReadMemStats(&after)
		grew := after.TotalAlloc - before.TotalAlloc
		allocated = append(allocated, grew)
		if floor := uint64(rounds * 4 * m); grew < floor {
			t.Errorf("comparing two values carrying %d marks %d times allocated %d bytes, fewer than the %d a set of them takes: the marks are being scanned for, which costs the square of them",
				m, rounds, grew, floor)
		}
	}
	if grew := float64(allocated[1]) / float64(allocated[0]); grew > 5 {
		t.Errorf("comparing two values carrying four times the marks allocated %.1f times as much (%d bytes, then %d)", grew, allocated[0], allocated[1])
	}
}

// BenchmarkMarkComparisons measures comparing two values that carry the same
// marks, attached in opposite orders, at a count of marks and four times it:
// the growth from one to the other is the reading, not the wall clock. Marks
// with identifiers of their own are held alike by both values, so the walk
// settles them and nothing is looked up; marks sharing four identifiers are
// held in opposite orders, so every one of them is.
func BenchmarkMarkComparisons(b *testing.B) {
	one := tenon.NumberFromInt(1)
	for _, shape := range []struct {
		name string
		ids  func(m int) int
	}{
		{"distinct", func(m int) int { return m }},
		{"shared", func(m int) int { return m / 8 }},
	} {
		for _, m := range []int{1000, 4000} {
			ms := make([]tenon.Mark, m)
			for i := range ms {
				ms[i] = note{id: fmt.Sprintf("m%06d", i%shape.ids(m)), text: fmt.Sprintf("p%06d", i)}
			}
			backwards := slices.Clone(ms)
			slices.Reverse(backwards)
			x, y := tenon.WithMarks(one, ms...), tenon.WithMarks(one, backwards...)
			if !tenon.Identical(x, y) {
				b.Fatalf("two values carrying the same %d %s marks are not identical", m, shape.name)
			}
			b.Run(fmt.Sprintf("%s/%d", shape.name, m), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if !tenon.Identical(x, y) {
						b.Fatal("the values differ")
					}
				}
			})
		}
	}
}
