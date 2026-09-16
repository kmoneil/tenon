package tenon

import (
	"reflect"
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
// mark, Isolate included.
type Mark interface {
	// MarkID returns the stable identifier of the mark, used where the mark
	// must be named without its value: redaction placeholders and encodings.
	MarkID() string
	// Propagation returns how the mark moves through operations.
	Propagation() Propagation
	// Redacting reports whether diagnostics must hide the contents of a
	// value carrying the mark.
	Redacting() bool
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
// it does not have.
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
// nothing new to attach the result is v itself.
//
// WithMarks panics if a mark is nil or of a type that is not comparable,
// since Go equality is what tells marks apart.
func WithMarks(v Value, marks ...Mark) Value {
	n := v.data()
	held := n.markList()
	merged, grew := held, false
	for i, m := range marks {
		if m == nil {
			usagePanic("WithMarks called with a nil Mark as mark %d", i)
		}
		if !reflect.TypeOf(m).Comparable() {
			usagePanic("WithMarks called with a mark of type %T, which is not comparable and so cannot be told from other marks", m)
		}
		if slices.Contains(merged, m) {
			continue
		}
		if !grew {
			merged, grew = slices.Clone(held), true
		}
		merged = append(merged, m)
	}
	if !grew {
		return v
	}
	sortMarks(merged)
	nn := *n
	nn.marks = &markSet{list: merged}
	return Value{&nn}
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
// too. A value that carries no mark comes back as itself, with no marks.
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

// propagated returns the marks that the result of an operation over these
// operands carries: the union of the operands' Propagate marks.
func propagated(args []Value) []Mark {
	var ms []Mark
	for _, a := range args {
		for _, m := range a.data().markList() {
			if m.Propagation() == Propagate && !slices.Contains(ms, m) {
				ms = append(ms, m)
			}
		}
	}
	return ms
}

// carryMarks returns r carrying every mark of v. A narrowing or a resolution
// refines the value it was given rather than deriving a new one, so the
// marks stay, the Isolate ones included. An error result is returned as it
// is: what marks an error value carries is settled where its diagnostics
// are built, not here.
func carryMarks(v, r Value) Value {
	if v.n.marks == nil || r.n == v.n || r.n.state == stateError {
		return r
	}
	return WithMarks(r, v.n.marks.list...)
}
