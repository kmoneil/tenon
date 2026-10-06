package tenon_test

import (
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
)

func TestConformance_MK014_MergeCompactFilter(t *testing.T) {
	conformance.Covers(t, "MK-014")
	str := tenon.StringType()
	a, b := stamp{id: "a"}, stamp{id: "b"}
	deep := stamp{id: "deep", deep: true}
	// Merging: one entry for each path, the marks united and sorted, in the
	// canonical order, the lists left as they were.
	first := []tenon.LocatedMarks{entry(at("b"), a), entry(tenon.Path{}, b)}
	second := []tenon.LocatedMarks{entry(at("b"), b, a), entry(at("a")), entry(tenon.Path{}, b)}
	if got := shown(tenon.MergeLocatedMarks(first, second)); !slices.Equal(got, []string{". b", ".b a b"}) {
		t.Errorf("MergeLocatedMarks gives %v", got)
	}
	if !slices.Equal(shown(first), []string{".b a", ". b"}) || !slices.Equal(shown(second), []string{".b b a", ".a", ". b"}) {
		t.Errorf("MergeLocatedMarks changed the lists it was given: %v, %v", shown(first), shown(second))
	}
	if got := tenon.MergeLocatedMarks(nil, []tenon.LocatedMarks{entry(at("a"))}); got != nil {
		t.Errorf("merging no marks gives %v", shown(got))
	}
	mustPanicUsage(t, "MergeLocatedMarks called with a nil Mark as mark 0", func() {
		tenon.MergeLocatedMarks([]tenon.LocatedMarks{entry(at("a"), nil)})
	})
	// Compacting: a deep mark is taken from within an entry carrying it,
	// and placing what is left gives what placing everything gives.
	nested := tenon.Object(map[string]tenon.Value{"o": tenon.Object(map[string]tenon.Value{"i": tenon.List(str, tenon.String("x"))})})
	whole := tenon.WithMarks(nested, deep)
	if got := shown(tenon.CompactLocatedMarks(tenon.MarkLocations(whole))); !slices.Equal(got, []string{". deep"}) {
		t.Errorf("a value marked deep at its root compacts to %v", got)
	}
	inner := tenon.Object(map[string]tenon.Value{"o": tenon.WithMarks(tenon.Object(map[string]tenon.Value{
		"i": tenon.WithMarks(tenon.List(str, tenon.String("x")), a),
	}), deep)})
	inner = tenon.WithMarks(inner, b)
	compacted := tenon.CompactLocatedMarks(tenon.MarkLocations(inner))
	if got := shown(compacted); !slices.Equal(got, []string{". b", ".o deep", ".o.i a"}) {
		t.Errorf("compacting gives %v", got)
	}
	plainInner, _ := tenon.UnmarkDeep(inner)
	if back, left := tenon.WithLocatedMarks(plainInner, compacted); !tenon.Identical(back, inner) || left != nil {
		t.Errorf("placing compacted marks gives %v", back)
	}
	// In whatever order the entries come, and on the random values the
	// located marks round trip on.
	shuffled := tenon.MarkLocations(inner)
	slices.Reverse(shuffled)
	if got := shown(tenon.CompactLocatedMarks(shuffled)); !slices.Equal(got, []string{". b", ".o deep", ".o.i a"}) {
		t.Errorf("compacting reversed entries gives %v", got)
	}
	marks := []tenon.Mark{a, b, deep, stamp{id: "deep-iso", deep: true, policy: tenon.Isolate}}
	rng := rand.New(rand.NewPCG(20261006, 14))
	for range conformance.Iterations(t, 2000) {
		v := randomMarked(rng, 4, marks)
		plain, _ := tenon.UnmarkDeep(v)
		if back, left := tenon.WithLocatedMarks(plain, tenon.CompactLocatedMarks(tenon.MarkLocations(v))); !tenon.Identical(back, v) || left != nil {
			t.Fatalf("%v placed back from compacted marks gives %v", v, back)
		}
	}
	// Filtering keeps the marks the predicate keeps, and the entries left
	// with marks, in their order.
	list := []tenon.LocatedMarks{entry(at("z"), a, b), entry(tenon.Path{}, b), entry(at("c"), a)}
	got := tenon.FilterLocatedMarks(list, func(m tenon.Mark) bool { return m.MarkID() == "a" })
	if !slices.Equal(shown(got), []string{".z a", ".c a"}) || !slices.Equal(shown(list), []string{".z a b", ". b", ".c a"}) {
		t.Errorf("filtering gives %v from %v", shown(got), shown(list))
	}
}

func TestConformance_MK015_SameMarksAndMarksAnywhere(t *testing.T) {
	conformance.Covers(t, "MK-015")
	str, num := tenon.StringType(), tenon.NumberType()
	a, b := stamp{id: "a"}, stamp{id: "b"}
	redacting := stamp{id: "a", redact: true}
	deep := stamp{id: "deep", deep: true}
	x, y := tenon.String("x"), tenon.String("y")
	listOf := func(first tenon.Value) tenon.Value { return tenon.List(str, first, y) }
	// The same marks at the same paths, whatever the values are.
	for _, tt := range []struct {
		a, b tenon.Value
		same bool
	}{
		{listOf(x), listOf(y), true},
		{listOf(tenon.WithMarks(x, a)), listOf(tenon.WithMarks(y, a)), true},
		{listOf(tenon.WithMarks(x, a, b)), listOf(tenon.WithMarks(x, b, a)), true},
		{listOf(tenon.WithMarks(x, a)), tenon.Tuple(tenon.WithMarks(x, a), tenon.NumberFromInt(1)), true},
		{listOf(tenon.WithMarks(x, a)), listOf(x), false},
		{listOf(tenon.WithMarks(x, a)), tenon.WithMarks(listOf(x), a), false},
		{listOf(tenon.WithMarks(x, a)), listOf(tenon.WithMarks(x, b)), false},
		{listOf(tenon.WithMarks(x, a)), listOf(tenon.WithMarks(x, redacting)), false},
		{listOf(tenon.WithMarks(x, a)), listOf(tenon.WithMarks(x, a, b)), false},
		{tenon.WithMarks(listOf(x), deep), tenon.List(str, tenon.WithMarks(x, deep), tenon.WithMarks(y, deep)), false},
	} {
		if got := tenon.SameMarks(tt.a, tt.b); got != tt.same {
			t.Errorf("SameMarks(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.same)
		}
		if got := tenon.SameMarks(tt.b, tt.a); got != tt.same {
			t.Errorf("SameMarks(%v, %v) = %v, want %v", tt.b, tt.a, got, tt.same)
		}
	}
	// A mark anywhere in a value: its own, a member's at any depth, a
	// set's.
	doc := tenon.Object(map[string]tenon.Value{
		"l": listOf(tenon.WithMarks(x, a)),
		"m": tenon.Map(num, map[string]tenon.Value{"k": tenon.WithMarks(tenon.NumberFromInt(1), deep)}),
		"s": tenon.WithMarks(tenon.Set(num, tenon.NumberFromInt(1)), b),
		"h": tenon.Tuple(tenon.Pending(tenon.Any()), tenon.WithMarks(tenon.Unknown(str), redacting)),
	})
	for _, tt := range []struct {
		m    tenon.Mark
		want bool
	}{{a, true}, {b, true}, {deep, true}, {redacting, true}, {stamp{id: "c"}, false}} {
		if got := tenon.HasMarkDeep(doc, tt.m); got != tt.want {
			t.Errorf("HasMarkDeep(doc, %s) = %v, want %v", tt.m.MarkID(), got, tt.want)
		}
	}
	if tenon.HasMark(doc, a) || !tenon.HasMarkDeep(tenon.WithMarks(doc, a), a) {
		t.Errorf("HasMarkDeep and HasMark disagree on a value's own marks")
	}
	mustPanicUsage(t, "HasMarkDeep called with a nil Mark", func() { tenon.HasMarkDeep(doc, nil) })
	// A value holding no mark is answered without a walk.
	elems := make([]tenon.Value, 10000)
	for i := range elems {
		elems[i] = tenon.NumberFromInt(int64(i))
	}
	big := tenon.List(num, elems...)
	other := tenon.List(num, elems[:5000]...)
	if !tenon.SameMarks(big, other) || tenon.HasMarkDeep(big, a) {
		t.Errorf("unmarked values have different marks, or a mark")
	}
	var boxed tenon.Mark = a
	if allocs := testing.AllocsPerRun(10, func() { tenon.SameMarks(big, other); tenon.HasMarkDeep(big, boxed) }); allocs != 0 {
		t.Errorf("asking of unmarked values allocates %v times", allocs)
	}
}

func TestConformance_MK016_RewriteMarks(t *testing.T) {
	conformance.Covers(t, "MK-016")
	str, num := tenon.StringType(), tenon.NumberType()
	one := tenon.NumberFromInt(1)
	a, b, c := stamp{id: "a"}, stamp{id: "b"}, stamp{id: "c"}
	deep := stamp{id: "deep", deep: true}
	build := func(root, z1, z2, l1, s []tenon.Mark) tenon.Value {
		return tenon.WithMarks(tenon.Object(map[string]tenon.Value{
			"z": tenon.WithMarks(tenon.WithMarks(tenon.String("z"), z1...), z2...),
			"l": tenon.List(str, tenon.String("x"), tenon.WithMarks(tenon.String("y"), l1...)),
			"s": tenon.WithMarks(tenon.Set(num, one), s...),
			"p": tenon.String("p"),
		}), root...)
	}
	ms := func(marks ...tenon.Mark) []tenon.Mark { return marks }
	doc := build(ms(b), ms(b), ms(a), ms(a), ms(b))
	// Each mark at each value, in the canonical order of paths and of
	// identifiers.
	var seen []string
	kept := tenon.RewriteMarks(doc, func(p tenon.Path, m tenon.Mark) tenon.MarkAction {
		seen = append(seen, p.String()+" "+m.MarkID())
		return tenon.KeepMark()
	})
	if want := []string{". b", ".l[1] a", ".s b", ".z a", ".z b"}; !slices.Equal(seen, want) {
		t.Errorf("RewriteMarks hands %v, want %v", seen, want)
	}
	if !tenon.Identical(kept, doc) {
		t.Errorf("keeping every mark gives %v", kept)
	}
	// Dropping and replacing.
	dropA := tenon.RewriteMarks(doc, func(_ tenon.Path, m tenon.Mark) tenon.MarkAction {
		if m == tenon.Mark(a) {
			return tenon.DropMark()
		}
		return tenon.KeepMark()
	})
	if want := build(ms(b), ms(b), nil, nil, ms(b)); !tenon.Identical(dropA, want) {
		t.Errorf("dropping a gives %v, want %v", dropA, want)
	}
	rootToC := tenon.RewriteMarks(doc, func(p tenon.Path, m tenon.Mark) tenon.MarkAction {
		if p.Len() == 0 {
			return tenon.ReplaceMark(c, a)
		}
		if p.Equal(at("s")) {
			return tenon.ReplaceMark()
		}
		return tenon.KeepMark()
	})
	if want := build(ms(a, c), ms(b), ms(a), ms(a), nil); !tenon.Identical(rootToC, want) {
		t.Errorf("replacing at the root gives %v, want %v", rootToC, want)
	}
	// A deep mark is handed at every value it reached, and kept on a value
	// it stays on what that value holds.
	marked := tenon.WithMarks(tenon.List(str, tenon.String("x"), tenon.String("y")), deep)
	at := func(where string) func(tenon.Path, tenon.Mark) tenon.MarkAction {
		return func(p tenon.Path, _ tenon.Mark) tenon.MarkAction {
			if p.String() == where {
				return tenon.DropMark()
			}
			return tenon.KeepMark()
		}
	}
	if got := tenon.RewriteMarks(marked, at(".")); !tenon.Identical(got, tenon.List(str, tenon.WithMarks(tenon.String("x"), deep), tenon.WithMarks(tenon.String("y"), deep))) {
		t.Errorf("dropping a deep mark at the root alone gives %v", got)
	}
	if got := tenon.RewriteMarks(marked, at(".[0]")); !tenon.Identical(got, marked) {
		t.Errorf("dropping a deep mark within a value keeping it gives %v", got)
	}
	everywhere := tenon.RewriteMarks(marked, func(tenon.Path, tenon.Mark) tenon.MarkAction { return tenon.ReplaceMark(a) })
	if want := tenon.WithMarks(tenon.List(str, tenon.WithMarks(tenon.String("x"), a), tenon.WithMarks(tenon.String("y"), a)), a); !tenon.Identical(everywhere, want) {
		t.Errorf("replacing a deep mark everywhere gives %v, want %v", everywhere, want)
	}
	mustPanicUsage(t, "ReplaceMark called with a nil Mark as mark 1", func() { tenon.ReplaceMark(a, nil) })
	// Actions are equal where they do the same with a mark.
	for _, tt := range []struct {
		x, y  tenon.MarkAction
		equal bool
	}{
		{tenon.KeepMark(), tenon.MarkAction{}, true},
		{tenon.DropMark(), tenon.ReplaceMark(), true},
		{tenon.ReplaceMark(a, b), tenon.ReplaceMark(b, a, b), true},
		{tenon.KeepMark(), tenon.DropMark(), false},
		{tenon.ReplaceMark(a), tenon.ReplaceMark(b), false},
		{tenon.ReplaceMark(a), tenon.DropMark(), false},
	} {
		if tt.x.Equal(tt.y) != tt.equal || tt.y.Equal(tt.x) != tt.equal {
			t.Errorf("%v.Equal(%v) is not %v", tt.x, tt.y, tt.equal)
		}
	}
}
