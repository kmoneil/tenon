package tenon

import (
	"slices"
	"strings"
)

// Mark is caller-defined metadata attached to a value: a sensitivity label,
// a provenance note, anything that must travel with a value without being
// part of it. Marks never change what a value is or what an operation
// returns; they change only what is attached to the result.
//
// A mark is any comparable value implementing this interface. Marks are told
// apart by Go equality, so a mark is usually a small struct or a pointer,
// and attaching one that is not comparable is a usage panic. The identifier
// names the mark where its value cannot appear, such as a redaction
// placeholder or a serialized form.
//
// How a mark moves is the mark's choice. A Propagate mark appears on the
// result of every operation that consumes the marked value; an Isolate mark
// stays on the value it was attached to. Narrowing and resolving refine the
// value they are given rather than deriving a new one, so both keep every
// mark, Isolate included. A mark that implements DeepMark can also reach
// down, to every value within the one it is attached to.
type Mark interface {
	// MarkID returns the stable identifier of the mark, used where the mark
	// must be named without its value: redaction placeholders and encodings.
	MarkID() string
	// Propagation returns how the mark moves through operations.
	Propagation() Propagation
	// Redacting reports whether the contents of a value carrying the mark
	// are withheld wherever the value is described: in the messages of
	// diagnostics, and in String, which puts a placeholder naming the mark
	// in their place.
	Redacting() bool
}

// DeepMark is a Mark that can declare itself deep. A deep mark attached to a
// collection or structural value is attached to every value within it as
// well, at any depth, so a mark put on a whole document is on each part of it
// that is read out. A mark type declares itself deep by implementing DeepMark
// with a Deep method that reports true, and Deep must give the same answer
// every time it is asked. A mark that does not implement DeepMark is not deep.
//
// WithMarks applies a deep mark when it attaches it, rather than leaving it to
// be looked up later: once WithMarks returns, the values within carry the mark
// in their own right, and taking it off the outer value with Unmark leaves it
// on them. The members of a set are the exception, because they carry no
// marks (see SetVal): a deep mark on a set stays on the set, and Elements
// attaches it to each member as it returns the member.
type DeepMark interface {
	Mark
	// Deep reports whether the mark is attached to every value within the
	// value it is attached to.
	Deep() bool
}

// EncodableMark is a Mark that declares how it is serialized: as its identifier
// alone, or with a value. A value carrying a mark that does not implement
// EncodableMark cannot be serialized.
type EncodableMark interface {
	Mark
	// MarkPayload returns the value the mark is serialized with and true, or
	// false where the identifier alone serializes it. The value must be known,
	// unmarked and not a null, and marks that are not equal must not serialize
	// alike.
	MarkPayload() (Value, bool)
}

// Propagation says how a mark moves through operations.
type Propagation uint8

const (
	// Propagate puts the mark on the result of any operation that consumes
	// the marked value. It is the zero value: a mark propagates unless it
	// says otherwise.
	Propagate Propagation = iota
	// Isolate keeps the mark on the value it is attached to; results
	// derived from that value do not carry it.
	Isolate
)

// markSet is the immutable set of marks on a value, held sorted by
// identifier, attachment order breaking ties. It is nil on an unmarked
// value, which therefore pays a nil pointer and nothing else for the marks
// it does not have. Nothing changes a mark set once it is made, so values
// that carry the same marks may share one.
type markSet struct {
	list []Mark
}

// markList returns the marks on n, nil when there are none.
func (n *node) markList() []Mark {
	if n.marks == nil {
		return nil
	}
	return n.marks.list
}

// WithMarks returns v carrying the given marks beside those it already
// carries. A mark that is already on v is not attached twice, and with
// nothing new to attach the result is v itself. A deep mark (DeepMark) is
// attached to every value within v as well, except the members of a set,
// which Elements marks as it returns them.
//
// WithMarks panics if a mark is nil or of a type that is not comparable,
// since Go equality is what tells marks apart.
func WithMarks(v Value, marks ...Mark) Value {
	n := v.data()
	for i, m := range marks {
		if m == nil {
			usagePanic("WithMarks called with a nil Mark as mark %d", i)
		}
		if !comparableMark(m) {
			usagePanic("WithMarks called with a mark of type %T, which is not comparable and so cannot be told from other marks", m)
		}
	}
	merged, grew := mergeMarks(n.markList(), marks)
	if !grew {
		// A deep mark v carries already is on everything within v too, since
		// it was attached to those values when it was attached to v.
		return v
	}
	nn := *n
	nn.marks = &markSet{list: merged}
	if deep := deepMarks(marks); deep != nil {
		(&attachment{deep: deep}).within(&nn)
	}
	return Value{&nn}
}

// comparableMark reports whether m is of a type Go equality can compare, which
// telling marks apart needs. Comparing a value of a type that is not comparable
// panics, and the panic is the answer.
func comparableMark(m Mark) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	_ = m == m
	return true
}

// mergeMarks returns held with marks added, each once, sorted by identifier,
// and whether any of them was not held already. held itself is left as it is.
//
// held is sorted already, so each mark is looked for among the marks sharing
// its identifier and put where it belongs, rather than the whole list being
// searched and then sorted again. A value can carry thousands of marks, and
// one more arrives whenever a deep mark reaches it, so both of those cost
// more than the merge itself.
func mergeMarks(held, marks []Mark) ([]Mark, bool) {
	merged, grew := held, false
	for i, m := range marks {
		at, found := placeMark(merged, m)
		if found {
			continue
		}
		if !grew {
			// Room for every mark still to come, in one list, where
			// growing it a mark at a time would make it again and again.
			merged, grew = make([]Mark, len(held), len(held)+len(marks)-i), true
			copy(merged, held)
		}
		merged = slices.Insert(merged, at, m)
	}
	return merged, grew
}

// placeMark returns where m belongs in a list of marks sorted by identifier,
// which is after the marks that share its identifier, so that marks arriving
// later sit behind those held already, and whether the list holds m already.
func placeMark(list []Mark, m Mark) (int, bool) {
	id := m.MarkID()
	at, _ := slices.BinarySearchFunc(list, id, func(h Mark, id string) int {
		return strings.Compare(h.MarkID(), id)
	})
	for ; at < len(list) && list[at].MarkID() == id; at++ {
		if list[at] == m {
			return at, true
		}
	}
	return at, false
}

// manyMarks is the most marks a markLookup scans for one mark. A value carries
// a handful of marks, among which a scan is quickest and allocates nothing; but
// a document can put thousands on one value, and scanning them for each mark
// costs the square of them: 4,000 marks took 36 ms to decode.
const manyMarks = 16

// markLookup says whether a mark is among the marks a list holds so far, by
// scanning the list while it is short and by a set of them once it is not. The
// list only grows while a markLookup is in use, so the set, once made, needs
// only the marks the list gains after it.
type markLookup struct {
	set map[Mark]struct{}
	n   int // how many of the list's marks the set holds
}

// holds reports whether m is in list, which holds the marks the previous
// calls saw and perhaps more at its end.
func (l *markLookup) holds(list []Mark, m Mark) bool {
	if l.set == nil {
		if len(list) <= manyMarks {
			return slices.Contains(list, m)
		}
		l.set = make(map[Mark]struct{}, 2*len(list))
	}
	for _, h := range list[l.n:] {
		l.set[h] = struct{}{}
	}
	l.n = len(list)
	_, ok := l.set[m]
	return ok
}

// sameMarkSet reports whether two lists hold the same marks. Marks are a set:
// what is there matters, the order they were attached in does not.
//
// A value holds its marks in one order, so two values carrying the same marks
// hold them alike wherever no two of those marks share an identifier, and the
// walk settles them a mark at a time. A mark that does not line up is looked
// for among the other list's marks, by a scan while there are few and by a
// set of them once there are many, as markLookup is used wherever marks are
// looked up: a value can carry thousands, one arriving whenever a deep mark
// reaches it, and scanning for each costs the square of them. The walk is a
// shortcut for a mark that is certainly there, not a premise, so no answer
// here depends on the order the marks are held in.
func sameMarkSet(x, y []Mark) bool {
	if len(x) != len(y) {
		return false
	}
	var seen markLookup
	for i, m := range x {
		if m == y[i] {
			continue
		}
		if !seen.holds(y, m) {
			return false
		}
	}
	return true
}

// isDeep reports whether m is a deep mark.
func isDeep(m Mark) bool {
	d, ok := m.(DeepMark)
	return ok && d.Deep()
}

// deepMarks returns the deep marks among marks, each once, sorted by
// identifier, or nil when there are none.
func deepMarks(marks []Mark) []Mark {
	var deep []Mark
	var seen markLookup
	for _, m := range marks {
		if isDeep(m) && !seen.holds(deep, m) {
			deep = append(deep, m)
		}
	}
	sortMarks(deep)
	return deep
}

// attachment attaches deep marks to everything within a value. It relies on
// what it keeps: a value carrying a deep mark has it on every value within
// it, except the members of a set, which carry no marks. Attaching a mark to
// a value that carries it already can therefore stop there.
//
// Values that held the same marks before the attachment hold the same marks
// after it, so they share one mark set rather than each holding a copy. Most
// of the values within a value being marked held no marks, and all of those
// share one.
type attachment struct {
	deep     []Mark                // the marks to attach, each once, sorted
	unmarked *markSet              // what a value that held no marks holds after
	sets     map[*markSet]*markSet // what a value that held some holds after
}

// within attaches the deep marks to every value n holds, at any depth, other
// than the members of a set. n is a copy that nothing shares yet, and within
// replaces its content when a member changes.
func (a *attachment) within(n *node) {
	if n.state != stateKnown {
		return
	}
	switch data := n.data.(type) {
	case []Value:
		if n.typ.t.kind == KindSet {
			// The marks stay on the set, and Elements attaches them to each
			// member it returns.
			return
		}
		var members []Value
		for i, m := range data {
			if r := a.attach(m.n); r != m.n {
				if members == nil {
					members = slices.Clone(data)
				}
				members[i] = Value{r}
			}
		}
		if members != nil {
			n.data, n.markedWithin = members, true
		}
	case []mapEntry:
		var entries []mapEntry
		for i, e := range data {
			if r := a.attach(e.val.n); r != e.val.n {
				if entries == nil {
					entries = slices.Clone(data)
				}
				entries[i].val = Value{r}
			}
		}
		if entries != nil {
			n.data, n.markedWithin = entries, true
		}
	}
}

// attach returns n carrying the deep marks, with everything within it
// carrying them too, or n itself when it carries them already.
func (a *attachment) attach(n *node) *node {
	marks, grew := a.merged(n.marks)
	if !grew {
		return n
	}
	nn := *n
	nn.marks = marks
	a.within(&nn)
	return &nn
}

// merged returns the mark set that a value holding held holds once the deep
// marks are attached to it, and whether that adds any.
func (a *attachment) merged(held *markSet) (*markSet, bool) {
	if held == nil {
		if a.unmarked == nil {
			a.unmarked = &markSet{list: a.deep}
		}
		return a.unmarked, true
	}
	if set, ok := a.sets[held]; ok {
		return set, set != held
	}
	list, grew := mergeMarks(held.list, a.deep)
	set := held
	if grew {
		set = &markSet{list: list}
	}
	if a.sets == nil {
		a.sets = map[*markSet]*markSet{}
	}
	a.sets[held] = set
	return set, grew
}

// retrievedMembers returns the members of a known set as a caller retrieves
// them: in a new slice, each carrying the set's deep marks, which the set
// keeps on itself because its members carry no marks in storage.
func (n *node) retrievedMembers() []Value {
	members := slices.Clone(n.data.([]Value))
	if deep := deepMarks(n.markList()); deep != nil {
		a := attachment{deep: deep}
		for i, m := range members {
			members[i] = Value{a.attach(m.n)}
		}
	}
	return members
}

// sortMarks sorts marks by identifier, leaving marks that share an identifier
// in the order they had.
func sortMarks(ms []Mark) {
	slices.SortStableFunc(ms, func(a, b Mark) int {
		return strings.Compare(a.MarkID(), b.MarkID())
	})
}

// Unmark returns v without the marks it carries, and those marks, sorted by
// identifier. The values v holds keep their own marks, which UnmarkDeep takes
// too. Those include a deep mark attached to v, which the values within v
// carry in their own right, but not a deep mark attached to a set, which its
// members carry only as Elements returns them. A value that carries no mark
// comes back as itself, with no marks.
func Unmark(v Value) (Value, []Mark) {
	n := v.data()
	if n.marks == nil {
		return v, nil
	}
	nn := *n
	nn.marks = nil
	return Value{&nn}, slices.Clone(n.marks.list)
}

// UnmarkDeep returns v without a mark anywhere in it: without its own marks,
// and with every value it holds, at any depth, unmarked too. It returns the
// marks it took, each once, sorted by identifier. A value that carries no mark
// and holds none comes back as itself, with no marks.
//
// A marked value, one that carries a mark or holds one, has no hash, no place
// in the canonical order and no place in a set. UnmarkDeep is the first half of
// what to do instead; the second is reapplying the marks it returns, which are
// the caller's to place. SetVal shows the usual place: the set.
func UnmarkDeep(v Value) (Value, []Mark) {
	n := v.data()
	if !n.isMarked() {
		return v, nil
	}
	var taken []Mark
	u := n.unmarkDeep(&taken)
	sortMarks(taken)
	return Value{u}, taken
}

// unmarkDeep returns n with no mark at any depth, adding each mark it takes to
// taken unless one equal to it is there already. Whatever holds no mark is
// shared rather than copied.
func (n *node) unmarkDeep(taken *[]Mark) *node {
	if !n.isMarked() {
		return n
	}
	for _, m := range n.markList() {
		if !slices.Contains(*taken, m) {
			*taken = append(*taken, m)
		}
	}
	nn := *n
	nn.marks = nil
	if n.markedWithin {
		nn.markedWithin = false
		switch data := n.data.(type) {
		case []Value:
			members := make([]Value, len(data))
			for i, m := range data {
				members[i] = Value{m.n.unmarkDeep(taken)}
			}
			nn.data = members
		case []mapEntry:
			entries := make([]mapEntry, len(data))
			for i, e := range data {
				entries[i] = mapEntry{key: e.key, val: Value{e.val.n.unmarkDeep(taken)}}
			}
			nn.data = entries
		}
	}
	return &nn
}

// isMarked reports whether n carries a mark or holds, at any depth, a value
// that does. Hashing, the canonical order and set membership are defined only
// for values that are not marked.
func (n *node) isMarked() bool { return n.marks != nil || n.markedWithin }

// describeMarked names a marked value and says where its marks are, for a
// panic message, as in "a value of type number that carries marks" or "a value
// of type list(number) that holds a marked value at [0]".
func (n *node) describeMarked() string {
	if n.marks != nil {
		return n.describe() + " that carries marks"
	}
	return n.describe() + " that holds a marked value at " + n.markPath().String()
}

// markPath returns the path from n to the first value within it that carries
// a mark, taking members in the order they are held. n must hold one.
func (n *node) markPath() Path {
	var p Path
	for n.marks == nil {
		step, member := n.markedMember()
		p, n = p.extend(step), member
	}
	return p
}

// markedMember returns the first member of n that is marked, and the step
// that locates it. n must hold one.
func (n *node) markedMember() (Step, *node) {
	switch data := n.data.(type) {
	case []mapEntry:
		for _, e := range data {
			if e.val.n.isMarked() {
				return indexStep(String(e.key)), e.val.n
			}
		}
	case []Value:
		for i, m := range data {
			if !m.n.isMarked() {
				continue
			}
			if n.typ.t.kind == KindObject {
				return attributeStep(n.typ.t.attrs[i].name), m.n
			}
			return indexStep(NumberFromInt(int64(i))), m.n
		}
	}
	internalPanic("%s says it holds a marked member and holds none", n.describe())
	return Step{}, nil
}

// HasMark reports whether v carries the mark.
func HasMark(v Value, m Mark) bool {
	return slices.Contains(v.data().markList(), m)
}

// propagated returns the marks that the result of the operation over these
// operands carries: the union of the operands' Propagate marks, and of the
// Propagate marks of the values within an operand that the operation reads,
// which it consumes along with the operand.
func (o *op) propagated(args []Value) []Mark {
	var g propagating
	for i, a := range args {
		g.gather(a.data(), o.operands[i].within)
	}
	return g.marks
}

// propagating gathers the Propagate marks of what is consumed, each once, in
// the order they are met: the operands of an operation and what it reads
// within them, the error members of a container, the bounds of a narrowing. A
// mark is looked for among those gathered by markLookup, by a scan while there
// are few and through a set once there are many: a value can carry thousands,
// and scanning for each would cost the square of them.
type propagating struct {
	marks []Mark
	seen  markLookup
}

// add gathers the Propagate marks among ms not gathered already. It makes
// room for all of them at once, which a value carrying thousands needs: grown
// a mark at a time, the list would be made again and again.
func (g *propagating) add(ms []Mark) {
	g.marks = slices.Grow(g.marks, len(ms))
	for _, m := range ms {
		if m.Propagation() == Propagate && !g.seen.holds(g.marks, m) {
			g.marks = append(g.marks, m)
		}
	}
}

// gather gathers the marks of n, and where within says that what is consumed
// reads the values within n, theirs as well, at any depth.
func (g *propagating) gather(n *node, within bool) {
	g.add(n.markList())
	if !within || !n.markedWithin {
		return
	}
	switch data := n.data.(type) {
	case []Value:
		for _, member := range data {
			g.gather(member.n, true)
		}
	case []mapEntry:
		for _, e := range data {
			g.gather(e.val.n, true)
		}
	}
}

// carryMarks returns r carrying every mark of v. A narrowing or a resolution
// refines the value it was given rather than deriving a new one, so the
// marks stay, the Isolate ones included, whether the result is a value or an
// error.
func carryMarks(v, r Value) Value {
	if v.n.marks == nil || r.n == v.n {
		return r
	}
	return WithMarks(r, v.n.marks.list...)
}
