package tenon

import (
	"strconv"
	"strings"

	"tenon/internal/decimal"
)

// StepKind identifies the kind of a path step.
type StepKind uint8

// The kinds of path step.
const (
	StepAttribute StepKind = iota + 1
	StepIndex
)

// String returns the name of the kind, "Attribute" or "Index".
func (k StepKind) String() string {
	switch k {
	case StepAttribute:
		return "Attribute"
	case StepIndex:
		return "Index"
	}
	return "StepKind(" + strconv.Itoa(int(k)) + ")"
}

// Step is one step of a path: an attribute of an object, or an index into a
// list, tuple, map or set.
type Step struct {
	kind StepKind
	name string // the attribute name, normalized
	key  Value  // the index key
}

// Kind returns the kind of s. It panics on the zero Step, which is not a step.
func (s Step) Kind() StepKind {
	if s.kind == 0 {
		usagePanic("use of the zero Step")
	}
	return s.kind
}

// Name returns the attribute name of an attribute step. It panics for other
// steps.
func (s Step) Name() string {
	if s.kind != StepAttribute {
		usagePanic("Name called on %s, which is not an attribute step", s)
	}
	return s.name
}

// Key returns the key of an index step. It panics for other steps.
func (s Step) Key() Value {
	if s.kind != StepIndex {
		usagePanic("Key called on %s, which is not an index step", s)
	}
	return s.key
}

// String describes the step, as in .name, ["a name"] or [0].
func (s Step) String() string {
	switch s.kind {
	case StepAttribute:
		if isIdentifier(s.name) {
			return "." + s.name
		}
		return "[" + strconv.Quote(s.name) + "]"
	case StepIndex:
		return "[" + s.key.String() + "]"
	}
	return "<zero Step>"
}

// equal reports whether s and t are the same step.
func (s Step) equal(t Step) bool {
	if s.kind != t.kind {
		return false
	}
	if s.kind == StepAttribute {
		return s.name == t.name
	}
	return sameKey(s.key, t.key)
}

// sameKey reports whether two index keys, which are Number or String values,
// are the same value. Value equality proper belongs to Equals.
func sameKey(a, b Value) bool {
	if a.n.typ != b.n.typ {
		return false
	}
	if a.n.typ.t.kind == KindString {
		return a.n.data.(string) == b.n.data.(string)
	}
	return a.n.data.(decimal.Dec).Equal(b.n.data.(decimal.Dec))
}

// isIdentifier reports whether name reads as a plain identifier, which a path
// can show without quoting.
func isIdentifier(name string) bool {
	for i, r := range name {
		switch {
		case r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return name != ""
}

// Path locates a value within a containing value: the steps that lead from the
// containing value down to it. The zero Path is the empty path, which locates
// the containing value itself.
//
// Paths are immutable. Extending a path returns a new path and leaves the
// original as it was, so a path stays valid for as long as its holder keeps
// it, however the walk that produced it goes on.
type Path struct {
	last *pathNode
}

// pathNode is the last step of a path together with everything before it.
// Paths extended from one prefix share that prefix's nodes, which never
// change.
type pathNode struct {
	parent *pathNode
	step   Step
	depth  int
}

// Attribute returns p followed by a step to the named attribute. The name is
// normalized, and panics on the same names as Object.
func (p Path) Attribute(name string) Path {
	return p.extend(Step{kind: StepAttribute, name: attributeName(name, "object attribute")})
}

// Index returns p followed by a step to the element with the given key, which
// must be a Number or String value.
func (p Path) Index(key Value) Path {
	n := key.data()
	if n.state != stateResolved || (n.typ.t.kind != KindNumber && n.typ.t.kind != KindString) {
		usagePanic("Index called with %s as a key; a path indexes by a Number or String value", n.describe())
	}
	return p.extend(Step{kind: StepIndex, key: key})
}

// extend returns p followed by s.
func (p Path) extend(s Step) Path {
	depth := 1
	if p.last != nil {
		depth = p.last.depth + 1
	}
	return Path{&pathNode{parent: p.last, step: s, depth: depth}}
}

// Len returns the number of steps in p.
func (p Path) Len() int {
	if p.last == nil {
		return 0
	}
	return p.last.depth
}

// Steps returns the steps of p, outermost first, in a new slice.
func (p Path) Steps() []Step {
	steps := make([]Step, p.Len())
	for n := p.last; n != nil; n = n.parent {
		steps[n.depth-1] = n.step
	}
	return steps
}

// Equal reports whether p and q are the same sequence of steps.
func (p Path) Equal(q Path) bool {
	if p.Len() != q.Len() {
		return false
	}
	for a, b := p.last, q.last; a != nil; a, b = a.parent, b.parent {
		if a == b {
			return true // the rest is a shared prefix
		}
		if !a.step.equal(b.step) {
			return false
		}
	}
	return true
}

// String describes p, as in .name[0]["key"]. The empty path is "".
func (p Path) String() string {
	var b strings.Builder
	for _, s := range p.Steps() {
		b.WriteString(s.String())
	}
	return b.String()
}

// prepend returns the path of what p locates, seen from one step further out:
// s followed by the steps of p.
func (p Path) prepend(s Step) Path {
	out := Path{}.extend(s)
	for _, step := range p.Steps() {
		out = out.extend(step)
	}
	return out
}

// attributeStep returns a step to the attribute of the given normalized name.
func attributeStep(name string) Step {
	return Step{kind: StepAttribute, name: name}
}

// indexStep returns a step to the element with the given key, which must be a
// Number or String value.
func indexStep(key Value) Step {
	return Step{kind: StepIndex, key: key}
}
