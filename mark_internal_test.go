package tenon

import (
	"fmt"
	"math/rand"
	"slices"
	"strconv"
	"testing"

	"github.com/kmoneil/tenon/conformance"
)

// probe is a Mark for the internal tests, deep or not, redacting or not.
type probe struct {
	id     string
	deep   bool
	redact bool
}

func (m probe) MarkID() string           { return m.id }
func (m probe) Propagation() Propagation { return Propagate }
func (m probe) Redacting() bool          { return m.redact }
func (m probe) Deep() bool               { return m.deep }

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
	// profile, and a mark-caused allocation on this path fails here. Reading
	// the members of an unmarked set costs the slice they come back in and
	// nothing more, though a marked set may have marks to apply to them.
	set := SetVal(str, String("a"), String("b"))
	for _, tt := range []struct {
		name string
		want float64
		f    func()
	}{
		{"Bool", 0, func() { Bool(true) }},
		{"NullVal", 1, func() { NullVal(str) }},
		{"Unknown", 2, func() { Unknown(str) }},
		{"NumberFromInt", 2, func() { NumberFromInt(42) }},
		{"Elements of an unmarked set", 1, func() { set.Elements() }},
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

// TestPlainWritesOutNoMark holds plain to its doc, including in a state no
// message renders today: an error value shows no marks at all, as a value
// that is not an error and carries no redacting mark does. What plain copies
// says truly whether it holds a marked value, and a value with no mark comes
// back as itself.
func TestPlainWritesOutNoMark(t *testing.T) {
	secret, origin := probe{id: "secret", redact: true}, probe{id: "origin"}
	e := ErrorVal(Diagnostic{Code: "app.x", Message: "m"})
	a, b := String("a"), String("b")
	for _, tt := range []struct {
		v    Value
		want string
	}{
		{WithMarks(e, secret, origin), `error(app.x: "m")`},
		{WithMarks(a, secret, origin), `redacted("secret")`},
		{WithMarks(a, origin), `"a"`},
		{WithMarks(ListVal(StringType(), WithMarks(a, origin)), origin), `list(string)["a"]`},
		{ListVal(StringType(), WithMarks(a, secret), WithMarks(b, origin)), `list(string)[redacted("secret"), "b"]`},
	} {
		p := Value{tt.v.n.plain()}
		if got := p.String(); got != tt.want {
			t.Errorf("%v reads %s, want %s", tt.v, got, tt.want)
		}
		checkMarkedWithin(t, p)
	}
	if a.n.plain() != a.n {
		t.Error("a value with no mark was copied")
	}
}

// TestDeepMarksAreAppliedAllTheWayDown holds WithMarks to what the code that
// attaches deep marks relies on to stop descending as soon as it can: a value
// carrying a deep mark has it on every value within it, except the members of
// a set, which carry no marks at all.
func TestDeepMarksAreAppliedAllTheWayDown(t *testing.T) {
	deep, other := probe{id: "deep", deep: true}, probe{id: "other", deep: true}
	shallow := probe{id: "shallow"}
	num, str := Type{numberType}, Type{stringType}
	one := NumberFromInt(1)
	set := SetVal(str, String("a"), String("b"))
	tree := ObjectVal(map[string]Value{
		"list":  ListVal(num, one, Unknown(num)),
		"map":   MapVal(num, map[string]Value{"k": WithMarks(one, shallow)}),
		"tuple": TupleVal(set, NullVal(str), WithMarks(one, other)),
		"sets":  ListVal(Set(str), set),
	})
	marked := WithMarks(tree, deep)
	stripped, _ := Unmark(marked)
	for _, v := range []Value{
		marked,
		stripped,
		WithMarks(stripped, deep, other),
		WithMarks(marked, other, shallow),
		WithMarks(set, deep).Elements()[0],
		ListVal(List(num), WithMarks(ListVal(num, one), deep)),
		WithMarks(SetVal(List(num), ListVal(num, one)), deep).Elements()[0],
		Narrow(WithMarks(Unknown(Tuple(Tuple(), Tuple())), deep), NotNull()),
		Narrow(WithMarks(Unknown(Set(num)), deep), NotNull(), Members(one), LengthMax(1)),
	} {
		checkDeepMarks(t, v)
		checkMarkedWithin(t, v)
	}
	// Values that take a mark together share one mark set rather than holding
	// a copy each.
	if elems := marked.Attribute("list").n.data.([]Value); elems[0].n.marks != elems[1].n.marks {
		t.Error("two members marked together hold two mark sets")
	}
}

// checkDeepMarks fails t unless every value within v carries the deep marks of
// the value holding it, and the members of every set within v carry no marks.
func checkDeepMarks(t *testing.T, v Value) {
	t.Helper()
	if v.n.state != stateKnown {
		return
	}
	var members []Value
	switch data := v.n.data.(type) {
	case []Value:
		members = data
	case []mapEntry:
		for _, e := range data {
			members = append(members, e.val)
		}
	}
	for _, m := range members {
		switch {
		case v.n.typ.t.kind == KindSet:
			if m.n.isMarked() {
				t.Errorf("the set %v holds the marked member %v", v, m)
			}
		default:
			for _, d := range v.n.markList() {
				if isDeep(d) && !slices.Contains(m.n.markList(), d) {
					t.Errorf("%v carries the deep mark %s and its member %v does not", v, d.MarkID(), m)
				}
			}
		}
		checkDeepMarks(t, m)
	}
}

// BenchmarkDeepMarks weighs applying a deep mark as it is attached, which is
// what WithMarks does, against a lazy design that records the mark on the
// value alone and applies it to each member as the member is read. The tree
// has 9331 values: objects of lists of objects, six wide and five deep above
// its numbers. The lazy reads here are the cheapest a lazy design could make
// them, so the comparison leans the lazy way.
//
// mark attaches the mark; path attaches it and reads one value at each depth;
// walk1 attaches it and reads every value once, as rendering or encoding the
// value would; walk3 reads every value three times.
func BenchmarkDeepMarks(b *testing.B) {
	tree := deepMarkTree(5, 6)
	deep := probe{id: "deep", deep: true}
	for _, s := range []struct {
		name string
		mark func(Value) Value
		read func(holder, member Value) Value
	}{
		{"eager", func(v Value) Value { return WithMarks(v, deep) }, func(_, member Value) Value { return member }},
		{"lazy", func(v Value) Value { return lazyMark(v, deep) }, lazyRead},
	} {
		b.Run(s.name+"/mark", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				s.mark(tree)
			}
		})
		b.Run(s.name+"/path", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				v := s.mark(tree)
				for m := treeMembers(v); len(m) > 0; m = treeMembers(v) {
					v = s.read(v, m[0])
				}
			}
		})
		for _, walks := range []int{1, 3} {
			b.Run(fmt.Sprintf("%s/walk%d", s.name, walks), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					v := s.mark(tree)
					for range walks {
						readTree(v, s.read)
					}
				}
			})
		}
	}
}

// deepMarkTree returns a tree of objects and lists, breadth wide at every level
// and depth levels deep above its numbers. Every value in it is a node of its
// own, as in a value decoded from a document.
func deepMarkTree(depth, breadth int) Value {
	if depth == 0 {
		return NumberFromInt(int64(breadth))
	}
	children := make([]Value, breadth)
	for i := range children {
		children[i] = deepMarkTree(depth-1, breadth)
	}
	if depth%2 == 0 {
		return ListVal(children[0].n.typ, children...)
	}
	attrs := make(map[string]Value, breadth)
	for i, c := range children {
		attrs["a"+strconv.Itoa(i)] = c
	}
	return ObjectVal(attrs)
}

// treeMembers returns the members of a list or object in the tree, and none
// for a number.
func treeMembers(v Value) []Value {
	members, _ := v.n.data.([]Value)
	return members
}

// readTree reads every value within v, depth first, through read.
func readTree(v Value, read func(holder, member Value) Value) {
	for _, m := range treeMembers(v) {
		readTree(read(v, m), read)
	}
}

// lazyMark is what attaching a mark costs a lazy design: a copy of the value
// alone. The tree carries no marks, so the mark is all its mark set holds.
func lazyMark(v Value, m Mark) Value {
	nn := *v.n
	nn.marks = &markSet{list: []Mark{m}}
	return Value{&nn}
}

// lazyRead is what reading a member costs a lazy design at best: a copy of the
// member carrying the holder's mark set, shared rather than merged. That is
// sound only because the tree's members carry no marks of their own and its
// one mark is deep, which a real lazy design could not assume.
func lazyRead(holder, member Value) Value {
	if holder.n.marks == nil {
		return member
	}
	nn := *member.n
	nn.marks = holder.n.marks
	return Value{&nn}
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

// TestMergeMarksIsTheScan holds mergeMarks and deepMarks, whichever way they
// look for a mark, to the answers a scan of the list gives, which they gave
// before a set took over from the scan past manyMarks. Marks are drawn from a
// pool small enough to repeat, both within the marks added and between them
// and the marks held, and the lists run from none to four times manyMarks.
func TestMergeMarksIsTheScan(t *testing.T) {
	r := rand.New(rand.NewSource(1342))
	pool := make([]Mark, 5*manyMarks)
	for i := range pool {
		pool[i] = namedMark{id: fmt.Sprintf("m%03d", i), deep: i%3 == 0}
	}
	pick := func(n int) []Mark {
		ms := make([]Mark, n)
		for i := range ms {
			ms[i] = pool[r.Intn(len(pool))]
		}
		return ms
	}
	for range conformance.Iterations(t, 1000) {
		held := scanMerge(nil, pick(r.Intn(4*manyMarks)))
		marks := pick(r.Intn(4 * manyMarks))
		got, grew := mergeMarks(held, marks)
		want := scanMerge(held, marks)
		if grew != (len(want) > len(held)) || !slices.Equal(got, want) {
			t.Fatalf("mergeMarks(%v, %v) = %v, %v; want %v", held, marks, got, grew, want)
		}
		if d, want := deepMarks(marks), scanDeep(marks); !slices.Equal(d, want) {
			t.Fatalf("deepMarks(%v) = %v, want %v", marks, d, want)
		}
	}
}

// scanMerge is mergeMarks as a scan of the list for every mark.
func scanMerge(held, marks []Mark) []Mark {
	merged := slices.Clone(held)
	for _, m := range marks {
		if !slices.Contains(merged, m) {
			merged = append(merged, m)
		}
	}
	sortMarks(merged)
	return merged
}

// scanDeep is deepMarks as a scan of the list for every mark.
func scanDeep(marks []Mark) []Mark {
	var deep []Mark
	for _, m := range marks {
		if isDeep(m) && !slices.Contains(deep, m) {
			deep = append(deep, m)
		}
	}
	sortMarks(deep)
	return deep
}

// namedMark is a mark told apart by its identifier, deep or not.
type namedMark struct {
	id   string
	deep bool
}

func (m namedMark) MarkID() string         { return m.id }
func (namedMark) Propagation() Propagation { return Propagate }
func (namedMark) Redacting() bool          { return false }
func (m namedMark) Deep() bool             { return m.deep }
