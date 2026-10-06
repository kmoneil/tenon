package tenon

import "iter"

// WalkAction is what a visit tells Walk to do next.
type WalkAction uint8

const (
	// WalkContinue goes on to the values within the one visited, and then
	// past it. It is the zero WalkAction.
	WalkContinue WalkAction = iota
	// WalkSkip passes over the values within the one visited.
	WalkSkip
	// WalkStop ends the walk.
	WalkStop
)

// Walk visits v and every value within it, each with its path, in the
// canonical order of paths (VA-027, VA-026): a value before the values within
// it, an object's attributes by name, a map's elements by key, a list's and a
// tuple's elements and a set's members by place in its iteration order, and a
// pending value holding members as its tuple or object. Each value is handed
// as the value holding it stores it: its own marks, and those a deep mark put
// on it, but not the other marks of the values around it, which Path.Apply
// gathers. A null, a value not known yet and a pending value holding no
// members have nothing within them. What visit returns says whether to go on
// into the value, pass over what is within it, or stop. Every path handed out
// remains valid after visit returns (VA-021). Walk panics on the zero Value.
func Walk(v Value, visit func(Path, Value) WalkAction) {
	v.data()
	walk(v, Path{}, visit)
}

// All returns an iterator over v and every value within it, each with its
// path, in the order Walk visits them; breaking out of the loop ends the walk.
func All(v Value) iter.Seq2[Path, Value] {
	v.data()
	return func(yield func(Path, Value) bool) {
		walk(v, Path{}, func(p Path, v Value) WalkAction {
			if !yield(p, v) {
				return WalkStop
			}
			return WalkContinue
		})
	}
}

// walk visits v at p and what is within it, and reports whether the walk
// goes on.
func walk(v Value, p Path, visit func(Path, Value) WalkAction) bool {
	switch visit(p, v) {
	case WalkStop:
		return false
	case WalkSkip:
		return true
	}
	ok := true
	membersOf(v, func(s Step, m Value) bool {
		ok = walk(m, p.extend(s), visit)
		return ok
	})
	return ok
}

// membersOf calls f with each value within v and the step to it, in the
// canonical order of the steps, until f returns false: an object's
// attributes, a map's elements, a list's and a tuple's elements, a set's
// members as Elements gives them, and a held pending value's as its tuple's
// or object's. A value of another kind, null, not known yet or pending
// without members has none.
func membersOf(v Value, f func(Step, Value) bool) {
	n := v.data()
	if p, ok := n.held(); ok {
		for i, m := range p.vals {
			s := indexStep(NumberFromInt(int64(i)))
			if p.names != nil {
				s = attributeStep(p.names[i])
			}
			if !f(s, m) {
				return
			}
		}
		return
	}
	if n.state != stateKnown {
		return
	}
	switch n.typ.t.kind {
	case KindObject:
		for i, a := range n.typ.t.attrs {
			if !f(attributeStep(a.name), n.data.([]Value)[i]) {
				return
			}
		}
	case KindMap:
		for _, e := range n.data.([]mapEntry) {
			if !f(indexStep(String(e.key)), e.val) {
				return
			}
		}
	case KindList, KindTuple:
		for i, e := range n.data.([]Value) {
			if !f(indexStep(NumberFromInt(int64(i))), e) {
				return
			}
		}
	case KindSet:
		for i, m := range v.Elements() {
			if !f(indexStep(NumberFromInt(int64(i))), m) {
				return
			}
		}
	}
}
