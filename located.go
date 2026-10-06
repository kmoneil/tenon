package tenon

import (
	"cmp"
	"slices"
	"strings"
)

// LocatedMarks is marks together with the place in a value of the value that
// carries them: its path, and its marks, sorted by identifier (MK-012).
type LocatedMarks struct {
	Path  Path
	Marks []Mark
}

// MarkLocations returns the marks v holds with where they are (MK-012): an
// entry for v and for each value within it that carries marks, its path and
// its marks sorted by identifier, in the canonical order of their paths. A
// deep mark is reported at every value it reached, and the marks of a set,
// deep ones included, at the set, whose members carry none. A value that
// holds no mark gives nil, at once, without a walk.
//
// UnmarkDeep takes the marks off, and WithLocatedMarks puts them back: the
// value they are put back on is identical to v. The paths name the keys and
// attribute names of the values the marks are on, a redacting mark's
// included, as the value itself does; where they are written out, as a
// format that cannot carry marks needs, they say what those names are.
func MarkLocations(v Value) []LocatedMarks {
	n := v.data()
	if !n.isMarked() {
		return nil
	}
	var found []LocatedMarks
	locateMarks(n, Path{}, &found)
	return found
}

// locateMarks appends to found the located marks of n, at p, and of the
// values within it, in the canonical order of their paths.
func locateMarks(n *node, p Path, found *[]LocatedMarks) {
	if n.marks != nil {
		*found = append(*found, LocatedMarks{Path: p, Marks: slices.Clone(n.marks.all())})
	}
	if !n.markedWithin {
		return
	}
	switch data := n.data.(type) {
	case *pendingMembers:
		for i, m := range data.vals {
			if m.n.isMarked() {
				locateMarks(m.n, p.extend(heldStep(data, i)), found)
			}
		}
	case []mapEntry:
		for _, e := range data {
			if e.val.n.isMarked() {
				locateMarks(e.val.n, p.extend(indexStep(String(e.key))), found)
			}
		}
	case []Value:
		for i, m := range data {
			if !m.n.isMarked() {
				continue
			}
			s := indexStep(NumberFromInt(int64(i)))
			if n.typ.t.kind == KindObject {
				s = attributeStep(n.typ.t.attrs[i].name)
			}
			locateMarks(m.n, p.extend(s), found)
		}
	}
}

// heldStep returns the step to the member at i of the pending value holding
// p.
func heldStep(p *pendingMembers, i int) Step {
	if p.names != nil {
		return attributeStep(p.names[i])
	}
	return indexStep(NumberFromInt(int64(i)))
}

// WithLocatedMarks returns v with each entry's marks on the value its path
// reaches, as WithMarks puts them there, a deep mark reaching what that value
// holds, and the entries it could not place (MK-013). The entries for one
// path are united, and each value holding a marked one is rebuilt around it,
// keeping its own marks. A path stepping into a set's member puts its marks
// on the set, whose members carry none, where the rest of the path reaches a
// value within the member.
//
// An entry whose path reaches nothing, by a member that is not there, a step
// of the wrong kind, or a step from a null, a value not known yet, a pending
// value holding no members or an error value, is not placed: it is returned,
// one entry for each such path, its marks united and sorted by identifier, in
// the canonical order of their paths, for the caller to place elsewhere, as
// on the value the path stops at, or to fail on. An entry with no marks
// places nothing and is never returned.
//
// WithLocatedMarks follows each path once, and rebuilds only the values on
// the paths it places marks at, so it takes time in the size of what the
// paths reach and of the paths, never in their product. It panics on the
// zero Value, and on a mark WithMarks would panic on.
func WithLocatedMarks(v Value, marks []LocatedMarks) (Value, []LocatedMarks) {
	n := v.data()
	var entries []*placing
	for _, e := range marks {
		if len(e.Marks) == 0 {
			continue
		}
		checkMarks("WithLocatedMarks", e.Marks)
		entries = append(entries, &placing{path: e.Path, steps: e.Path.Steps(), marks: e.Marks})
	}
	if len(entries) == 0 {
		return v, nil
	}
	var pl placer
	placed := pl.place(n, entries, 0)
	return Value{n: placed}, pl.unplacedEntries()
}

// placing is an entry WithLocatedMarks places: its path, the path's steps,
// its marks, and the place, among the members of the value its path has
// reached, of the member its next step reaches.
type placing struct {
	path  Path
	steps []Step
	marks []Mark
	at    int
}

// placer gathers the entries WithLocatedMarks could not place.
type placer struct {
	unplaced []LocatedMarks
}

// leave records that the entry e could not be placed.
func (pl *placer) leave(e *placing) {
	pl.unplaced = append(pl.unplaced, LocatedMarks{Path: e.path, Marks: e.marks})
}

// place returns n with the marks of the entries, whose paths reached n after
// depth steps, placed on it and within it.
func (pl *placer) place(n *node, entries []*placing, depth int) *node {
	var own []Mark
	var below []*placing
	for _, e := range entries {
		if len(e.steps) == depth {
			own = append(own, e.marks...)
		} else {
			below = append(below, e)
		}
	}
	if len(below) != 0 {
		if n.state == stateKnown && n.typ.t.kind == KindSet {
			own = append(own, pl.intoSet(n, below, depth)...)
		} else {
			n = pl.placeBelow(n, below, depth)
		}
	}
	if len(own) == 0 {
		return n
	}
	return WithMarks(Value{n: n}, own...).n
}

// intoSet returns the marks of the entries, whose paths step from the set n
// after depth steps into one of its members, that the set takes: those whose
// paths reach a value within the member. The others are not placed.
func (pl *placer) intoSet(n *node, entries []*placing, depth int) []Mark {
	var marks []Mark
	for _, e := range entries {
		if reachesWithin(n, e.steps[depth:]) {
			marks = append(marks, e.marks...)
		} else {
			pl.leave(e)
		}
	}
	return marks
}

// reachesWithin reports whether the steps reach a value from n.
func reachesWithin(n *node, steps []Step) bool {
	for _, s := range steps {
		i, ok := memberPlace(n, s)
		if !ok {
			return false
		}
		n = memberNode(n, i)
	}
	return true
}

// placeBelow returns n with the marks of the entries, whose paths reached n
// after depth steps and go on into its members, placed within those members.
// The entries whose next step reaches no member of n are not placed.
func (pl *placer) placeBelow(n *node, entries []*placing, depth int) *node {
	var reached []*placing
	for _, e := range entries {
		at, ok := memberPlace(n, e.steps[depth])
		if !ok {
			pl.leave(e)
			continue
		}
		e.at = at
		reached = append(reached, e)
	}
	slices.SortStableFunc(reached, func(a, b *placing) int { return cmp.Compare(a.at, b.at) })
	var nn *node
	for start := 0; start < len(reached); {
		at, end := reached[start].at, start+1
		for end < len(reached) && reached[end].at == at {
			end++
		}
		old := memberNode(n, at)
		if placed := pl.place(old, reached[start:end], depth+1); placed != old {
			if nn == nil {
				nn = withOwnMembers(n)
			}
			putMember(nn, at, placed)
		}
		start = end
	}
	if nn == nil {
		return n
	}
	return nn
}

// withOwnMembers returns a copy of n, a value holding members, holding a
// copy of them that setMember can change, and holding a marked one.
func withOwnMembers(n *node) *node {
	nn := n.clone()
	switch data := n.data.(type) {
	case *pendingMembers:
		nn.data = &pendingMembers{c: data.c, vals: slices.Clone(data.vals), names: data.names}
	case []mapEntry:
		nn.data = slices.Clone(data)
	case []Value:
		nn.data = slices.Clone(data)
	}
	nn.markedWithin = true
	return nn
}

// putMember makes m the member of n at the place i, in n, which
// withOwnMembers made.
func putMember(n *node, i int, m *node) {
	switch data := n.data.(type) {
	case *pendingMembers:
		data.vals[i] = Value{n: m}
	case []mapEntry:
		data[i].val = Value{n: m}
	case []Value:
		data[i] = Value{n: m}
	}
}

// memberPlace returns the place among n's members of the member the step s
// reaches, and whether it reaches one: n is a known object, map, list, tuple
// or set, or a pending value holding members.
func memberPlace(n *node, s Step) (int, bool) {
	if p, ok := n.held(); ok {
		if p.names == nil {
			return sequencePlace(s, len(p.vals))
		}
		if s.kind != StepAttribute {
			return 0, false
		}
		return slices.BinarySearch(p.names, s.name)
	}
	if n.state != stateKnown {
		return 0, false
	}
	switch n.typ.t.kind {
	case KindObject:
		if s.kind != StepAttribute {
			return 0, false
		}
		return slices.BinarySearchFunc(n.typ.t.attrs, s.name, func(a attribute, name string) int {
			return strings.Compare(a.name, name)
		})
	case KindMap:
		if s.kind != StepIndex || s.key.n.typ.t.kind != KindString {
			return 0, false
		}
		return slices.BinarySearchFunc(n.data.([]mapEntry), s.key.n.data.(string), func(e mapEntry, key string) int {
			return strings.Compare(e.key, key)
		})
	case KindList, KindTuple, KindSet:
		return sequencePlace(s, len(n.data.([]Value)))
	}
	return 0, false
}

// sequencePlace returns the place the step s names among count members of a
// list, a tuple or a set, and whether it names one.
func sequencePlace(s Step, count int) (int, bool) {
	if s.kind != StepIndex || s.key.n.typ.t.kind != KindNumber {
		return 0, false
	}
	i, failed := sequenceIndex(s, "", int64(count), true)
	return int(i), failed == stepFailure{}
}

// memberNode returns the member of n at the place i, which memberPlace gave.
func memberNode(n *node, i int) *node {
	if p, ok := n.held(); ok {
		return p.vals[i].n
	}
	if entries, ok := n.data.([]mapEntry); ok {
		return entries[i].val.n
	}
	return n.data.([]Value)[i].n
}

// unplacedEntries returns the entries the placer could not place, one for
// each path, its marks united and sorted by identifier, in the canonical
// order of their paths.
func (pl *placer) unplacedEntries() []LocatedMarks {
	return mergeEntries(pl.unplaced)
}

// mergeEntries returns the entries, which it may reorder, merged into one
// for each path, its marks the union of theirs sorted by identifier, in the
// canonical order of their paths, or nil where there are none. Every entry
// has marks.
func mergeEntries(entries []LocatedMarks) []LocatedMarks {
	if len(entries) == 0 {
		return nil
	}
	slices.SortStableFunc(entries, func(a, b LocatedMarks) int { return ComparePaths(a.Path, b.Path) })
	var out []LocatedMarks
	for _, e := range entries {
		if last := len(out) - 1; last >= 0 && out[last].Path.Equal(e.Path) {
			out[last].Marks, _ = mergeMarks(out[last].Marks, e.Marks)
			continue
		}
		marks, _ := mergeMarks(nil, e.Marks)
		out = append(out, LocatedMarks{Path: e.Path, Marks: marks})
	}
	return out
}
