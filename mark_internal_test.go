package tenon

import (
	"fmt"
	"math/rand"
	"slices"
	"strconv"
	"strings"
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
	// Marks that share an identifier and are not the same mark: they tie in
	// the order marks are held in, where the one held already comes first and
	// the one arriving goes behind it.
	for i := range 2 * manyMarks {
		pool = append(pool, probe{id: fmt.Sprintf("t%02d", i%4), deep: i%2 == 0, redact: i%3 == 0})
	}
	// A long run of marks that share one identifier, as a document can give a
	// value, which many marks merged at once take in one pass.
	for i := range 3 * manyMarks {
		pool = append(pool, runMark{i})
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
		// Marks sorted and distinct already, as an attachment's deep marks
		// are, merge in one pass to what mergeMarks gives them.
		distinct := scanMerge(nil, marks)
		want, wantGrew := mergeMarks(held, distinct)
		if got, grew := mergeDistinct(held, distinct); grew != wantGrew || !slices.Equal(got, want) {
			t.Fatalf("mergeDistinct(%v, %v) = %v, %v; want %v, %v", held, distinct, got, grew, want, wantGrew)
		}
		// Many marks that are all held already add nothing, however they
		// are ordered, and the list held comes back as it was.
		again := slices.Clone(held)
		slices.Reverse(again)
		if got, grew := mergeMarks(held, again); grew || !slices.Equal(got, held) {
			t.Fatalf("merging the %d marks held again gave %v, %v", len(held), got, grew)
		}
		if d, want := deepMarks(marks), scanDeep(marks); !slices.Equal(d, want) {
			t.Fatalf("deepMarks(%v) = %v, want %v", marks, d, want)
		}
	}
}

// runMark is one of many marks that share an identifier, told apart by n.
type runMark struct{ n int }

func (runMark) MarkID() string           { return "run" }
func (runMark) Propagation() Propagation { return Propagate }
func (runMark) Redacting() bool          { return false }

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

// TestSameMarkSetIsTheScan holds sameMarkSet, whichever way it looks for a
// mark, to the answer a scan of the other list gives, which is what it gave
// before a set took over from the scan past manyMarks. The lists run from
// none to four times manyMarks and are drawn from a pool small enough to
// repeat, marks sharing an identifier among them. Two of every three lists
// hold another's marks, shuffled or with one mark exchanged, since a pair of
// lists picked at random is nearly always unequal in its first mark and would
// leave the lookup untested.
func TestSameMarkSetIsTheScan(t *testing.T) {
	r := rand.New(rand.NewSource(1613))
	pool := make([]Mark, 6*manyMarks)
	for i := range pool {
		if i%3 == 0 {
			// Marks that share an identifier and are not the same mark, which
			// is what makes two values hold one set of marks in two orders.
			pool[i] = probe{id: fmt.Sprintf("t%02d", i%4), deep: i%2 == 0, redact: i%5 == 0}
			continue
		}
		pool[i] = namedMark{id: fmt.Sprintf("m%03d", i), deep: i%7 == 0}
	}
	pick := func(n int) []Mark {
		ms := make([]Mark, n)
		for i := range ms {
			ms[i] = pool[r.Intn(len(pool))]
		}
		return ms
	}
	for range conformance.Iterations(t, 1000) {
		x := pick(r.Intn(4 * manyMarks))
		y := slices.Clone(x)
		switch r.Intn(3) {
		case 0: // the same marks in another order
			r.Shuffle(len(y), func(i, j int) { y[i], y[j] = y[j], y[i] })
		case 1: // the same marks but one, which a scan finds last
			if len(y) > 0 {
				y[r.Intn(len(y))] = pool[r.Intn(len(pool))]
			}
		default: // marks of their own, of a length of their own
			y = pick(r.Intn(4 * manyMarks))
		}
		if got, want := sameMarkSet(x, y), scanSame(x, y); got != want {
			t.Fatalf("sameMarkSet(%v, %v) = %t, want %t", x, y, got, want)
		}
	}
}

// scanSame is sameMarkSet as a scan of the other list for every mark.
func scanSame(x, y []Mark) bool {
	if len(x) != len(y) {
		return false
	}
	for _, m := range x {
		if !slices.Contains(y, m) {
			return false
		}
	}
	return true
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

// keptMark is a mark whose policy is Isolate, which nothing gathers.
type keptMark struct{ id string }

func (m keptMark) MarkID() string         { return m.id }
func (keptMark) Propagation() Propagation { return Isolate }
func (keptMark) Redacting() bool          { return false }

// TestGatheringMarksIsTheScan holds the gathering of Propagate marks, which an
// operation, a container of error members and a narrowing's bounds each do,
// to the scan it replaces: every Propagate mark once, in the order met, and
// no Isolate mark. It holds marksAside to its scan as well. The lists are
// drawn with repeats from more marks than the count past which marks are
// looked up through a set, so both sides of it are taken, and a gathering
// past it holds its set.
func TestGatheringMarksIsTheScan(t *testing.T) {
	r := rand.New(rand.NewSource(1615))
	pool := make([]Mark, 4*manyMarks)
	for i := range pool {
		if i%5 == 0 {
			pool[i] = keptMark{id: fmt.Sprintf("k%03d", i)}
			continue
		}
		pool[i] = namedMark{id: fmt.Sprintf("m%03d", i)}
	}
	pick := func(n int) []Mark {
		ms := make([]Mark, n)
		for i := range ms {
			ms[i] = pool[r.Intn(len(pool))]
		}
		return ms
	}
	for range conformance.Iterations(t, 1000) {
		var g propagating
		var want []Mark
		for range 1 + r.Intn(4) {
			ms := pick(r.Intn(3 * manyMarks))
			g.add(ms)
			want = scanGather(want, ms)
		}
		if !slices.Equal(g.marks, want) {
			t.Fatalf("gathered %v, want %v", g.marks, want)
		}
		// A mark past the first beyond the count was looked for among more
		// than the count, which is where the set is made.
		if len(want) > manyMarks+1 && g.seen.set == nil {
			t.Fatalf("%d marks were gathered by scanning for each", len(want))
		}
		list, aside := pick(r.Intn(3*manyMarks)), pick(r.Intn(3*manyMarks))
		if got, want := marksAside(list, aside), scanAside(list, aside); !slices.Equal(got, want) {
			t.Fatalf("marksAside(%v, %v) = %v, want %v", list, aside, got, want)
		}
	}

	// A container's error members have their marks gathered the same way.
	var errs containerErrors
	for i := range 3 * manyMarks {
		failed := WithMarks(ErrorVal(Diagnostic{Code: "app.failed", Message: "it failed"}), namedMark{id: fmt.Sprintf("e%03d", i)})
		errs.add(indexStep(NumberFromInt(int64(i))), failed)
	}
	if len(errs.marks.marks) != 3*manyMarks || errs.marks.seen.set == nil {
		t.Errorf("the marks of %d error members were gathered as %d marks, with a set: %v", 3*manyMarks, len(errs.marks.marks), errs.marks.seen.set != nil)
	}
	// Every mark set aside leaves nothing to list, so the set that looks them
	// up is all marksAside allocates, where a scan for each would allocate
	// nothing.
	all := pool[1 : 3*manyMarks]
	if allocs := testing.AllocsPerRun(10, func() { marksAside(all, all) }); allocs == 0 {
		t.Errorf("marksAside looked for each of %d marks by scanning those set aside", len(all))
	}
}

// scanGather is the gathering of Propagate marks as a scan of those gathered
// for every mark.
func scanGather(gathered, ms []Mark) []Mark {
	for _, m := range ms {
		if m.Propagation() == Propagate && !slices.Contains(gathered, m) {
			gathered = append(gathered, m)
		}
	}
	return gathered
}

// scanAside is marksAside as a scan of aside for every mark.
func scanAside(list, aside []Mark) []Mark {
	if aside == nil {
		return list
	}
	var out []Mark
	for _, m := range list {
		if !slices.Contains(aside, m) {
			out = append(out, m)
		}
	}
	return out
}

// payloadMark is an encodable mark for the internal tests, deep or not, told
// apart from the others of its identifier by its payload, which says whether
// it is deep so that a decoder can make it again.
type payloadMark struct {
	id, text string
}

func (m payloadMark) MarkID() string             { return m.id }
func (payloadMark) Propagation() Propagation     { return Propagate }
func (payloadMark) Redacting() bool              { return false }
func (m payloadMark) Deep() bool                 { return strings.HasPrefix(m.text, "deep") }
func (m payloadMark) MarkPayload() (Value, bool) { return String(m.text), true }

// TestDecodedDeepMarksAreHeldAsAttached holds what the decoder gives every
// value within a value it reads to what attaching the marks level by level
// gives: the same marks, each value's sorted by identifier, and the same
// claims to hold a marked value. The order of marks that share an identifier
// is not held: an encoding lists a value's marks in the order of their
// encodings and leaves out a deep mark its container carries, so no reading
// can know the order they were attached in. Values of every kind are nested
// under marks of two identifiers, deep and not, drawn with repeats, so that
// ties are met, and deep marks a value carries that its container carries
// too.
func TestDecodedDeepMarksAreHeldAsAttached(t *testing.T) {
	r := rand.New(rand.NewSource(1608))
	read := Decoders{Marks: map[string]MarkDecoder{}}
	for _, id := range []string{"a", "b"} {
		read.Marks[id] = func(p Value, _ bool) (Mark, []Diagnostic) { return payloadMark{id, p.AsString()}, nil }
	}
	mark := func() Mark {
		kind := []string{"deep", "flat"}[r.Intn(2)]
		return payloadMark{[]string{"a", "b"}[r.Intn(2)], fmt.Sprintf("%s %d", kind, r.Intn(3))}
	}
	marked := func(v Value) Value {
		ms := make([]Mark, r.Intn(3))
		for i := range ms {
			ms[i] = mark()
		}
		return WithMarks(v, ms...)
	}
	num := Type{numberType}
	var build func(depth int) Value
	build = func(depth int) Value {
		if depth == 0 {
			switch r.Intn(3) {
			case 0:
				return marked(NumberFromInt(int64(r.Intn(4))))
			case 1:
				return marked(Unknown(num))
			}
			// A set keeps a deep mark on itself, and its members carry none.
			return marked(SetVal(num, NumberFromInt(1), NumberFromInt(2)))
		}
		inner := build(depth - 1)
		var v Value
		switch r.Intn(4) {
		case 0:
			// Two elements of one type, one of them marked again.
			v = ListVal(inner.Type(), inner, marked(inner))
		case 1:
			v = TupleVal(inner, NumberFromInt(7))
		case 2:
			v = ObjectVal(map[string]Value{"a": inner, "b": marked(String("x"))})
		default:
			v = MapVal(inner.Type(), map[string]Value{"k": inner})
		}
		return marked(v)
	}
	// sameThroughout reports whether two values hold the same marks at every
	// place, sorted by identifier.
	var sameThroughout func(a, b *node) bool
	sameThroughout = func(a, b *node) bool {
		x, y := a.markList(), b.markList()
		sorted := func(ms []Mark) bool {
			return slices.IsSortedFunc(ms, func(p, q Mark) int { return strings.Compare(p.MarkID(), q.MarkID()) })
		}
		if !sameMarkSet(x, y) || !sorted(x) || !sorted(y) || a.markedWithin != b.markedWithin {
			return false
		}
		switch x := a.data.(type) {
		case []Value:
			y, ok := b.data.([]Value)
			if !ok || len(x) != len(y) {
				return false
			}
			for i := range x {
				if !sameThroughout(x[i].n, y[i].n) {
					return false
				}
			}
		case []mapEntry:
			y, ok := b.data.([]mapEntry)
			if !ok || len(x) != len(y) {
				return false
			}
			for i := range x {
				if !sameThroughout(x[i].val.n, y[i].val.n) {
					return false
				}
			}
		}
		return true
	}
	for range conformance.Iterations(t, 200) {
		v := build(1 + r.Intn(5))
		b, failure, ok := Serialize(v)
		if !ok {
			t.Fatalf("Serialize(%v) failed: %v", v, failure)
		}
		got, failure, ok := Deserialize(b, read)
		if !ok {
			t.Fatalf("Deserialize(Serialize(%v)) failed: %v", v, failure)
		}
		if !sameThroughout(got.n, v.n) || !Identical(got, v) {
			t.Fatalf("%v came back as %v, holding its marks otherwise", v, got)
		}
	}
}

// TestMergesMakeTheirListOnce holds the merges that put marks among those a
// value holds to making the merged list once, at its length, and the gatherer
// of an operation's marks to making room for what it is given at once. T-1608a
// merged without building a set of the marks held and sorting them again, and
// T-1615 made room for every mark to come at once, where growing the list a
// mark at a time made it again and again. Since T-1608b and T-1617 what either
// saves is a constant factor, which no growth reads, so this holds the lists
// themselves. The marks held are a handful, which a merge scans rather than
// indexing: past that, mergeDistinct builds a set of them by design.
func TestMergesMakeTheirListOnce(t *testing.T) {
	held := make([]Mark, manyMarks)
	for i := range held {
		held[i] = namedMark{id: fmt.Sprintf("m%03d", i)}
	}
	// Three marks the held ones lack, sorted and distinct, as mergeDistinct
	// takes them: one before them, one among them and one after.
	fresh := []Mark{namedMark{id: "a"}, namedMark{id: "m005a"}, namedMark{id: "z"}}
	for _, merge := range []struct {
		name  string
		merge func(held, marks []Mark) ([]Mark, bool)
	}{{"mergeMarks", mergeMarks}, {"mergeDistinct", mergeDistinct}} {
		merged, grew := merge.merge(held, fresh)
		if !grew || len(merged) != len(held)+len(fresh) || cap(merged) != len(merged) {
			t.Errorf("%s gave %d marks in a list with room for %d, want %d in a list made to their length",
				merge.name, len(merged), cap(merged), len(held)+len(fresh))
		}
		if made := testing.AllocsPerRun(10, func() { merge.merge(held, fresh) }); made != 1 {
			t.Errorf("%s made %v allocations to merge %d marks into %d, want the one list", merge.name, made, len(fresh), len(held))
		}
	}
	// Gathering twice as many marks makes no more allocations: the one list,
	// with its room made at once. What the race detector's build adds is the
	// same for both.
	gathering := func(ms []Mark) float64 {
		return testing.AllocsPerRun(10, func() {
			var g propagating
			g.add(ms)
		})
	}
	if half, all := gathering(held[:len(held)/2]), gathering(held); all != half {
		t.Errorf("gathering %d marks made %v allocations where gathering %d made %v, want as many for both", len(held), all, len(held)/2, half)
	}
}
