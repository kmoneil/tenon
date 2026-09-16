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
	slices.SortStableFunc(merged, func(a, b Mark) int {
		return strings.Compare(a.MarkID(), b.MarkID())
	})
	nn := *n
	nn.marks = &markSet{list: merged}
	return Value{&nn}
}

// Unmark returns v without its marks, and the marks it carried, sorted by
// identifier. An unmarked value comes back as itself, with no marks.
func Unmark(v Value) (Value, []Mark) {
	n := v.data()
	if n.marks == nil {
		return v, nil
	}
	nn := *n
	nn.marks = nil
	return Value{&nn}, slices.Clone(n.marks.list)
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
