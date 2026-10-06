package tenon_test

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
)

// shown renders located marks as their paths, each followed by the
// identifiers of its marks.
func shown(lm []tenon.LocatedMarks) []string {
	var out []string
	for _, e := range lm {
		s := e.Path.String()
		for _, m := range e.Marks {
			s += " " + m.MarkID()
		}
		out = append(out, s)
	}
	return out
}

// entry is the located marks of the path p.
func entry(p tenon.Path, marks ...tenon.Mark) tenon.LocatedMarks {
	return tenon.LocatedMarks{Path: p, Marks: marks}
}

// roundTrips checks that taking every mark off v and placing its located
// marks back gives v.
func roundTrips(t *testing.T, v tenon.Value) {
	t.Helper()
	plain, _ := tenon.UnmarkDeep(v)
	back, unplaced := tenon.WithLocatedMarks(plain, tenon.MarkLocations(v))
	if !tenon.Identical(back, v) || unplaced != nil {
		t.Errorf("%v placed back on itself unmarked gives %v, leaving %v", v, back, shown(unplaced))
	}
}

// randomMarked returns a random value at most depth deep, with marks from
// the list on values within it at random.
func randomMarked(r *rand.Rand, depth int, marks []tenon.Mark) tenon.Value {
	str, num := tenon.StringType(), tenon.NumberType()
	mark := func(v tenon.Value) tenon.Value {
		if r.IntN(3) == 0 {
			return tenon.WithMarks(v, marks[r.IntN(len(marks))])
		}
		return v
	}
	text := func() tenon.Value {
		switch r.IntN(4) {
		case 0:
			return tenon.Unknown(str)
		case 1:
			return tenon.Null(str)
		}
		return tenon.String(string(rune('a' + r.IntN(26))))
	}
	choice := r.IntN(7)
	if depth == 0 {
		choice = 0
	}
	var v tenon.Value
	switch choice {
	case 0:
		if r.IntN(5) == 0 {
			v = tenon.Pending(tenon.Any())
		} else {
			v = text()
		}
	case 1:
		attrs := map[string]tenon.Value{}
		for i := range r.IntN(4) {
			attrs[string(rune('a'+i))] = randomMarked(r, depth-1, marks)
		}
		v = tenon.Object(attrs)
	case 2:
		var elems []tenon.Value
		for range r.IntN(4) {
			elems = append(elems, randomMarked(r, depth-1, marks))
		}
		v = tenon.Tuple(elems...)
	case 3:
		var elems []tenon.Value
		for range r.IntN(4) {
			elems = append(elems, mark(text()))
		}
		v = tenon.List(str, elems...)
	case 4:
		entries := map[string]tenon.Value{}
		for range r.IntN(4) {
			entries[string(rune('k'+r.IntN(4)))] = mark(tenon.NumberFromInt(int64(r.IntN(9))))
		}
		v = tenon.Map(num, entries)
	case 5:
		var elems []tenon.Value
		for range r.IntN(4) {
			elems = append(elems, tenon.NumberFromInt(int64(r.IntN(9))))
		}
		v = tenon.Set(num, elems...)
	default:
		v = tenon.Tuple(tenon.Pending(tenon.Any()), randomMarked(r, depth-1, marks))
	}
	return mark(v)
}

func TestConformance_MK012_MarkLocations(t *testing.T) {
	conformance.Covers(t, "MK-012")
	str, num := tenon.StringType(), tenon.NumberType()
	one, two := tenon.NumberFromInt(1), tenon.NumberFromInt(2)
	a, b := stamp{id: "a"}, stamp{id: "b"}
	iso := stamp{id: "iso", policy: tenon.Isolate}
	deep := stamp{id: "deep", deep: true}
	// A value that holds no mark has none, at once.
	elems := make([]tenon.Value, 10000)
	for i := range elems {
		elems[i] = tenon.NumberFromInt(int64(i))
	}
	big := tenon.List(num, elems...)
	if got := tenon.MarkLocations(big); got != nil {
		t.Errorf("an unmarked value has located marks %v", shown(got))
	}
	if allocs := testing.AllocsPerRun(10, func() { tenon.MarkLocations(big) }); allocs != 0 {
		t.Errorf("finding no marks in an unmarked list allocates %v times", allocs)
	}
	// One entry for each value that carries marks, in the canonical order of
	// their paths, its marks sorted by identifier.
	doc := tenon.WithMarks(tenon.Object(map[string]tenon.Value{
		"z":     tenon.WithMarks(tenon.String("z"), b, a),
		"l":     tenon.List(str, tenon.String("x"), tenon.WithMarks(tenon.String("y"), iso)),
		"m":     tenon.Map(str, map[string]tenon.Value{"k": tenon.WithMarks(tenon.Unknown(str), a), "j": tenon.String("j")}),
		"s":     tenon.WithMarks(tenon.Set(num, one, two), b),
		"h":     tenon.Tuple(tenon.Pending(tenon.Any()), tenon.WithMarks(tenon.Null(str), a)),
		"plain": tenon.String("p"),
	}), b)
	got := tenon.MarkLocations(doc)
	want := []string{". b", ".h[1] a", ".l[1] iso", `.m["k"] a`, ".s b", ".z a b"}
	if !slices.Equal(shown(got), want) {
		t.Errorf("MarkLocations gives %v, want %v", shown(got), want)
	}
	if !slices.IsSortedFunc(got, func(x, y tenon.LocatedMarks) int { return tenon.ComparePaths(x.Path, y.Path) }) {
		t.Errorf("located marks are not in the canonical order of their paths")
	}
	// The entries are the caller's to change.
	got[0].Marks[0] = a
	if again := tenon.MarkLocations(doc); !slices.Equal(shown(again), want) {
		t.Errorf("changing an entry changed the value's marks: %v", shown(again))
	}
	// A deep mark is located at every value it reached; a set's marks, deep
	// ones included, at the set.
	for _, tt := range []struct {
		v    tenon.Value
		want []string
	}{
		{tenon.WithMarks(tenon.List(str, tenon.String("x"), tenon.String("y")), deep), []string{". deep", ".[0] deep", ".[1] deep"}},
		{tenon.WithMarks(tenon.Set(num, one, two), deep), []string{". deep"}},
		{tenon.WithMarks(tenon.Tuple(tenon.Pending(tenon.Any()), tenon.WithMarks(one, a)), deep), []string{". deep", ".[0] deep", ".[1] a deep"}},
		{tenon.WithMarks(tenon.Object(map[string]tenon.Value{"o": tenon.Object(map[string]tenon.Value{"i": one})}), deep), []string{". deep", ".o deep", ".o.i deep"}},
	} {
		if got := shown(tenon.MarkLocations(tt.v)); !slices.Equal(got, tt.want) {
			t.Errorf("MarkLocations(%v) = %v, want %v", tt.v, got, tt.want)
		}
		roundTrips(t, tt.v)
	}
	// Taking every mark off and placing the located marks back gives the
	// value back.
	roundTrips(t, doc)
	marks := []tenon.Mark{a, b, iso, deep, stamp{id: "deep-iso", deep: true, policy: tenon.Isolate}}
	rng := rand.New(rand.NewPCG(20261006, 12))
	for range conformance.Iterations(t, 2000) {
		roundTrips(t, randomMarked(rng, 4, marks))
	}
}

func TestConformance_MK013_WithLocatedMarks(t *testing.T) {
	conformance.Covers(t, "MK-013")
	str, num := tenon.StringType(), tenon.NumberType()
	one, two := tenon.NumberFromInt(1), tenon.NumberFromInt(2)
	a, b := stamp{id: "a"}, stamp{id: "b"}
	deep := stamp{id: "deep", deep: true}
	plain := func(l, h, s tenon.Value) tenon.Value {
		return tenon.Object(map[string]tenon.Value{
			"z": tenon.String("z"),
			"l": l,
			"m": tenon.Map(str, map[string]tenon.Value{"k": tenon.Unknown(str), "j": tenon.String("j")}),
			"s": s,
			"h": h,
			"u": tenon.Unknown(tenon.ObjectType(map[string]tenon.Type{"x": str})),
			"n": tenon.Null(tenon.ListType(str)),
			"p": tenon.Pending(tenon.Any()),
		})
	}
	list := tenon.List(str, tenon.String("x"), tenon.String("y"))
	held := tenon.Tuple(tenon.Pending(tenon.Any()), tenon.Null(str))
	set := tenon.Set(num, one, two)
	doc := plain(list, held, set)
	// Each entry's marks on the value its path reaches, the entries for one
	// path united, a set's member's on the set.
	placed, unplaced := tenon.WithLocatedMarks(doc, []tenon.LocatedMarks{
		entry(at("z"), b), entry(at("z"), a, b),
		entry(at("l", 1), a),
		entry(at("m", tenon.String("k")), a),
		entry(at("h", 1), b),
		entry(at("s", 1), a),
		entry(at("u"), b),
		entry(tenon.Path{}, b),
	})
	want := tenon.WithMarks(plain(
		tenon.List(str, tenon.String("x"), tenon.WithMarks(tenon.String("y"), a)),
		tenon.Tuple(tenon.Pending(tenon.Any()), tenon.WithMarks(tenon.Null(str), b)),
		tenon.WithMarks(set, a),
	), b)
	want = tenon.Object(map[string]tenon.Value{
		"z": tenon.WithMarks(tenon.String("z"), a, b), "l": want.Attribute("l"),
		"m": tenon.Map(str, map[string]tenon.Value{"k": tenon.WithMarks(tenon.Unknown(str), a), "j": tenon.String("j")}),
		"s": want.Attribute("s"), "h": want.Attribute("h"),
		"u": tenon.WithMarks(tenon.Unknown(tenon.ObjectType(map[string]tenon.Type{"x": str})), b),
		"n": tenon.Null(tenon.ListType(str)), "p": tenon.Pending(tenon.Any()),
	})
	want = tenon.WithMarks(want, b)
	if !tenon.Identical(placed, want) || unplaced != nil {
		t.Errorf("WithLocatedMarks gives %v, leaving %v; want %v", placed, shown(unplaced), want)
	}
	// A deep mark reaches what the value it is placed on holds, and a value
	// holding a marked one keeps its own marks.
	if got, _ := tenon.WithLocatedMarks(list, []tenon.LocatedMarks{entry(tenon.Path{}, deep)}); !tenon.Identical(got, tenon.WithMarks(list, deep)) {
		t.Errorf("a deep mark placed at the root gives %v", got)
	}
	ownMarked := tenon.WithMarks(tenon.Object(map[string]tenon.Value{"l": tenon.WithMarks(list, b)}), a)
	got, _ := tenon.WithLocatedMarks(ownMarked, []tenon.LocatedMarks{entry(at("l", 0), deep)})
	wantOwn := tenon.WithMarks(tenon.Object(map[string]tenon.Value{
		"l": tenon.WithMarks(tenon.List(str, tenon.WithMarks(tenon.String("x"), deep), tenon.String("y")), b),
	}), a)
	if !tenon.Identical(got, wantOwn) {
		t.Errorf("placing within a marked value gives %v, want %v", got, wantOwn)
	}
	// A set's member's path places on the set only where the rest of it
	// reaches a value within the member.
	objects := tenon.Set(tenon.ObjectType(map[string]tenon.Type{"x": str}), tenon.Object(map[string]tenon.Value{"x": tenon.String("1")}))
	if got, left := tenon.WithLocatedMarks(objects, []tenon.LocatedMarks{entry(at(0, "x"), a), entry(at(0, "y"), b)}); !tenon.Identical(got, tenon.WithMarks(objects, a)) || !slices.Equal(shown(left), []string{".[0].y b"}) {
		t.Errorf("placing into a set's member gives %v, leaving %v", got, shown(left))
	}
	// An entry whose path reaches nothing is answered, one for each path,
	// its marks united, in the canonical order, and never an entry with no
	// marks.
	entries := []tenon.LocatedMarks{
		entry(at("nope"), a),
		entry(at("l", 7), a),
		entry(at("l", "x"), a),
		entry(at("l", tenon.NumberFromText("0.5")), a),
		entry(at("l", -1), a),
		entry(at("m", 0), a),
		entry(at("u", "x"), a),
		entry(at("n", 0), a),
		entry(at("p", 0), a),
		entry(at("h", 0, "x"), a),
		entry(at("z", "x"), b),
		entry(at("z", "x"), a),
		entry(at("s", 2), a),
		entry(at("s", 0, "x"), a),
		entry(at("never")),
	}
	placed, unplaced = tenon.WithLocatedMarks(doc, entries)
	if !tenon.Identical(placed, doc) {
		t.Errorf("entries placing nothing changed the value: %v", placed)
	}
	wantLeft := []string{".h[0].x a", ".l.x a", ".l[-1] a", ".l[0.5] a", ".l[7] a", `.m[0] a`, ".n[0] a", ".nope a",
		".p[0] a", ".s[0].x a", ".s[2] a", ".u.x a", ".z.x a b"}
	if !slices.Equal(shown(unplaced), wantLeft) {
		t.Errorf("WithLocatedMarks leaves %v, want %v", shown(unplaced), wantLeft)
	}
	// In whatever order the entries come.
	rng := rand.New(rand.NewPCG(20261006, 13))
	for range 20 {
		rng.Shuffle(len(entries), func(i, j int) { entries[i], entries[j] = entries[j], entries[i] })
		if _, left := tenon.WithLocatedMarks(doc, entries); !slices.Equal(shown(left), wantLeft) {
			t.Fatalf("shuffled entries leave %v", shown(left))
		}
	}
	// An error value takes marks at its own path, and nothing within.
	failure := tenon.ErrorVal(tenon.Diagnostic{Code: tenon.CodeOperationWrongType, Message: "refused"})
	if got, left := tenon.WithLocatedMarks(failure, []tenon.LocatedMarks{entry(tenon.Path{}, a), entry(at("x"), b)}); !tenon.Identical(got, tenon.WithMarks(failure, a)) || !slices.Equal(shown(left), []string{".x b"}) {
		t.Errorf("placing on an error value gives %v, leaving %v", got, shown(left))
	}
	mustPanicUsage(t, "WithLocatedMarks called with a nil Mark as mark 1", func() {
		tenon.WithLocatedMarks(doc, []tenon.LocatedMarks{entry(at("nope"), a, nil)})
	})
	// The time is in the sum of the value's size and the entries, not their
	// product: four times the entries on four times the values allocate
	// about four times as much.
	allocs := func(n int) float64 {
		objs := make([]tenon.Value, n)
		lm := make([]tenon.LocatedMarks, n)
		for i := range objs {
			objs[i] = tenon.Object(map[string]tenon.Value{"k": tenon.String(strings.Repeat("v", i%7))})
			lm[i] = entry(at(i, "k"), a)
		}
		v := tenon.List(tenon.ObjectType(map[string]tenon.Type{"k": str}), objs...)
		return testing.AllocsPerRun(2, func() { tenon.WithLocatedMarks(v, lm) })
	}
	if small, large := allocs(2000), allocs(8000); large > 5*small {
		t.Errorf("placing 8,000 entries allocates %v times, 2,000 %v times", large, small)
	}
}
