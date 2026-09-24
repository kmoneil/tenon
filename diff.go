package tenon

import (
	"slices"
	"strings"
)

// ChangeKind is what a Change says happened at its path.
type ChangeKind uint8

const (
	// ChangeReplaced is a part of the first value replaced by the part of the
	// second at the same path: Old and New are the two parts.
	ChangeReplaced ChangeKind = iota + 1
	// ChangeAdded is a part that only the second value has: New is the part.
	ChangeAdded
	// ChangeRemoved is a part that only the first value has: Old is the part.
	ChangeRemoved
	// ChangeMemberAdded is a member that the set at the path holds in the
	// second value and not in the first: New is the member.
	ChangeMemberAdded
	// ChangeMemberRemoved is a member that the set at the path holds in the
	// first value and not in the second: Old is the member.
	ChangeMemberRemoved
	// ChangeMarks is a part whose marks differ between the two values, whose
	// changes within follow it: OldMarks and NewMarks are the two parts' marks.
	ChangeMarks
)

// String names the kind, as in "replaced" or "member added".
func (k ChangeKind) String() string {
	switch k {
	case ChangeReplaced:
		return "replaced"
	case ChangeAdded:
		return "added"
	case ChangeRemoved:
		return "removed"
	case ChangeMemberAdded:
		return "member added"
	case ChangeMemberRemoved:
		return "member removed"
	case ChangeMarks:
		return "marks changed"
	}
	return "<zero ChangeKind>"
}

// Change is one change in a diff: what happened at a path, and the parts or
// marks it is about. Which of Old, New, OldMarks and NewMarks it carries
// depends on its kind.
type Change struct {
	Kind               ChangeKind
	Path               Path
	Old, New           Value
	OldMarks, NewMarks []Mark
}

// String returns the display form of the change (DI-037), as in
// ~ .name: "web" -> "api", + .ports[2]: 8443 or ~ .tags: marks [] -> ["audited"].
func (c Change) String() string {
	var b strings.Builder
	c.write(&b)
	return b.String()
}

func (c Change) write(b *strings.Builder) {
	switch c.Kind {
	case ChangeReplaced, ChangeMarks:
		b.WriteString("~ ")
	case ChangeAdded, ChangeMemberAdded:
		b.WriteString("+ ")
	case ChangeRemoved, ChangeMemberRemoved:
		b.WriteString("- ")
	default:
		b.WriteString("<zero Change>")
		return
	}
	c.Path.write(b)
	b.WriteString(": ")
	switch c.Kind {
	case ChangeReplaced:
		c.Old.write(b)
		b.WriteString(" -> ")
		c.New.write(b)
	case ChangeAdded:
		c.New.write(b)
	case ChangeRemoved:
		c.Old.write(b)
	case ChangeMemberAdded:
		b.WriteString("member ")
		c.New.write(b)
	case ChangeMemberRemoved:
		b.WriteString("member ")
		c.Old.write(b)
	case ChangeMarks:
		b.WriteString("marks [")
		writeIdentifiers(b, c.OldMarks)
		b.WriteString("] -> [")
		writeIdentifiers(b, c.NewMarks)
		b.WriteByte(']')
	}
}

// Changes is the diff of one value against another, its changes in order.
type Changes []Change

// String returns the display form of the diff (DI-037): each change's display
// form followed by a line feed, and no text for no changes.
func (cs Changes) String() string {
	var b strings.Builder
	for _, c := range cs {
		c.write(&b)
		b.WriteByte('\n')
	}
	return b.String()
}

// Diff returns the changes that say where and how b differs from a (DI-030).
// It is empty exactly when a and b are identical.
//
// Diff compares a and b from the top, and looks within two parts at a path
// only where both are lists, sets or maps of one element type, tuples, or
// objects, neither is null, unknown, pending or an error value, and neither
// carries a redacting mark. Two other parts that are not identical are a mark
// change where only their marks differ and neither carries a redacting mark,
// and a replacement otherwise. Within two parts it looks within, list
// and tuple elements compare by index, map entries and object attributes by
// name, and set members by membership, and a part only one side has is an
// addition or a removal. Where the marks of two such parts differ, the diff has
// a mark change for them first. A deep mark is counted once, where it is
// attached: the values within compare as if they did not carry it.
//
// The changes are in the order a walk of both values meets them: elements in
// index order, entries and attributes in name order, set members in the order
// a set holding both sets' members iterates. Diff(b, a) is Diff(a, b) with
// each change turned the other way.
//
// Diff panics only on the zero Value, which is not a value.
func Diff(a, b Value) Changes {
	a.data()
	b.data()
	var d differ
	d.compare(a, b, Path{}, nil, nil)
	return d.changes
}

// differ collects the changes of a diff.
type differ struct {
	changes Changes
}

func (d *differ) add(c Change) { d.changes = append(d.changes, c) }

// compare adds the changes between the parts a and b at p. asideA and asideB are
// the deep marks of the parts that hold a and b, which a and b and every value
// within them compare without.
func (d *differ) compare(a, b Value, p Path, asideA, asideB []Mark) {
	na, nb := a.n, b.n
	ownA, ownB := marksAside(na.markList(), asideA), marksAside(nb.markList(), asideB)
	sameOwn := sameMarkSet(ownA, ownB)
	if !enterable(na, nb) {
		switch {
		case !restIdenticalAside(na, nb, asideA, asideB):
			d.add(Change{Kind: ChangeReplaced, Path: p, Old: a, New: b})
		case sameOwn:
		case na.redactingMarks() != nil || nb.redactingMarks() != nil:
			d.add(Change{Kind: ChangeReplaced, Path: p, Old: a, New: b})
		default:
			d.add(Change{Kind: ChangeMarks, Path: p, OldMarks: ownA, NewMarks: ownB})
		}
		return
	}
	if na == nb && sameMarkSet(asideA, asideB) {
		return
	}
	if !sameOwn {
		d.add(Change{Kind: ChangeMarks, Path: p, OldMarks: ownA, NewMarks: ownB})
	}
	innerA, innerB := deepOf(na.markList()), deepOf(nb.markList())
	switch na.typ.t.kind {
	case KindList, KindTuple:
		x, y := na.data.([]Value), nb.data.([]Value)
		for i := range max(len(x), len(y)) {
			at := p.extend(indexStep(NumberFromInt(int64(i))))
			switch {
			case i >= len(x):
				d.add(Change{Kind: ChangeAdded, Path: at, New: y[i]})
			case i >= len(y):
				d.add(Change{Kind: ChangeRemoved, Path: at, Old: x[i]})
			default:
				d.compare(x[i], y[i], at, innerA, innerB)
			}
		}
	case KindMap:
		x, y := na.data.([]mapEntry), nb.data.([]mapEntry)
		key := func(e mapEntry) string { return e.key }
		at := func(name string) Path { return p.extend(indexStep(stringValue(name))) }
		mergeByName(x, y, key, func(i, j int) {
			switch {
			case j < 0:
				d.add(Change{Kind: ChangeRemoved, Path: at(x[i].key), Old: x[i].val})
			case i < 0:
				d.add(Change{Kind: ChangeAdded, Path: at(y[j].key), New: y[j].val})
			default:
				d.compare(x[i].val, y[j].val, at(x[i].key), innerA, innerB)
			}
		})
	case KindObject:
		x, y := na.typ.t.attrs, nb.typ.t.attrs
		vx, vy := na.data.([]Value), nb.data.([]Value)
		name := func(at attribute) string { return at.name }
		at := func(name string) Path { return p.extend(attributeStep(name)) }
		mergeByName(x, y, name, func(i, j int) {
			switch {
			case j < 0:
				d.add(Change{Kind: ChangeRemoved, Path: at(x[i].name), Old: vx[i]})
			case i < 0:
				d.add(Change{Kind: ChangeAdded, Path: at(y[j].name), New: vy[j]})
			default:
				d.compare(vx[i], vy[j], at(x[i].name), innerA, innerB)
			}
		})
	case KindSet:
		d.members(na.data.([]Value), nb.data.([]Value), p)
	}
}

// members adds the member changes between two sets at p, whose members are as
// the sets hold them, in iteration order: known members first, in canonical
// order, and then the rest, in the order of their encodings.
func (d *differ) members(x, y []Value, p Path) {
	firstUnknown := func(members []Value) int {
		if i := slices.IndexFunc(members, func(m Value) bool { return !m.n.isKnown() }); i >= 0 {
			return i
		}
		return len(members)
	}
	kx, ky := firstUnknown(x), firstUnknown(y)
	// Known members are distinct and in canonical order, which a merge pairs.
	for i, j := 0, 0; i < kx || j < ky; {
		c := 0
		switch {
		case j == ky:
			c = -1
		case i == kx:
			c = 1
		default:
			c = compareValues(x[i], y[j])
		}
		switch {
		case c < 0:
			d.add(Change{Kind: ChangeMemberRemoved, Path: p, Old: x[i]})
			i++
		case c > 0:
			d.add(Change{Kind: ChangeMemberAdded, Path: p, New: y[j]})
			j++
		default:
			if !Identical(x[i], y[j]) {
				d.add(Change{Kind: ChangeMemberRemoved, Path: p, Old: x[i]})
				d.add(Change{Kind: ChangeMemberAdded, Path: p, New: y[j]})
			}
			i++
			j++
		}
	}
	// The rest may repeat, and pair with identical members one for one.
	rest := y[ky:]
	paired := make([]bool, len(rest))
	var removed, added []Value
	for _, m := range x[kx:] {
		found := false
		for k, o := range rest {
			if !paired[k] && Identical(m, o) {
				paired[k], found = true, true
				break
			}
		}
		if !found {
			removed = append(removed, m)
		}
	}
	for k, m := range rest {
		if !paired[k] {
			added = append(added, m)
		}
	}
	// Removals and additions interleave by display form, a removal first
	// where the two read alike.
	for len(removed) > 0 || len(added) > 0 {
		if len(added) == 0 || len(removed) > 0 && removed[0].String() <= added[0].String() {
			d.add(Change{Kind: ChangeMemberRemoved, Path: p, Old: removed[0]})
			removed = removed[1:]
		} else {
			d.add(Change{Kind: ChangeMemberAdded, Path: p, New: added[0]})
			added = added[1:]
		}
	}
}

// mergeByName walks two lists sorted by name together, calling each with the
// index of an item in x and in y of one name, or -1 for the side without it.
func mergeByName[T any](x, y []T, name func(T) string, each func(i, j int)) {
	i, j := 0, 0
	for i < len(x) || j < len(y) {
		switch {
		case j == len(y) || i < len(x) && name(x[i]) < name(y[j]):
			each(i, -1)
			i++
		case i == len(x) || name(y[j]) < name(x[i]):
			each(-1, j)
			j++
		default:
			each(i, j)
			i++
			j++
		}
	}
}

// enterable reports whether the diff looks within two parts: known containers
// of one kind, and of one type where the kind is a collection, neither
// carrying a redacting mark.
func enterable(a, b *node) bool {
	if a.state != stateKnown || b.state != stateKnown || a.typ.t.kind != b.typ.t.kind {
		return false
	}
	if a.redactingMarks() != nil || b.redactingMarks() != nil {
		return false
	}
	switch a.typ.t.kind {
	case KindList, KindSet, KindMap:
		return a.typ == b.typ
	case KindTuple, KindObject:
		return true
	}
	return false
}

// restIdenticalAside reports whether a and b are identical but for the marks
// they carry themselves, once the marks in asideA are set aside from every value
// within a, and those in asideB from every value within b.
func restIdenticalAside(a, b *node, asideA, asideB []Mark) bool {
	if a.state != b.state {
		return false
	}
	if a.state == stateKnown && (asideA != nil || asideB != nil) {
		if a.typ != b.typ {
			return false
		}
		switch a.typ.t.kind {
		case KindList, KindTuple, KindObject:
			return slices.EqualFunc(a.data.([]Value), b.data.([]Value), func(x, y Value) bool {
				return identicalAside(x.n, y.n, asideA, asideB)
			})
		case KindMap:
			return slices.EqualFunc(a.data.([]mapEntry), b.data.([]mapEntry), func(x, y mapEntry) bool {
				return x.key == y.key && identicalAside(x.val.n, y.val.n, asideA, asideB)
			})
		}
	}
	// Nothing within is left to set marks aside from: a set's members carry
	// no marks where it holds them, and other values hold no values at all.
	return Identical(withMarksOf(a, b), Value{b})
}

// identicalAside reports whether a and b are identical once the marks in
// asideA are set aside from a and every value within it, and those in asideB
// from b.
func identicalAside(a, b *node, asideA, asideB []Mark) bool {
	if asideA == nil && asideB == nil {
		return Identical(Value{a}, Value{b})
	}
	return sameMarkSet(marksAside(a.markList(), asideA), marksAside(b.markList(), asideB)) &&
		restIdenticalAside(a, b, asideA, asideB)
}

// withMarksOf returns a as a value carrying the marks of b, for comparing the
// rest of a and b once their marks are known to agree.
func withMarksOf(a, b *node) Value {
	if a.marks == b.marks {
		return Value{a}
	}
	c := *a
	c.marks = b.marks
	return Value{&c}
}

// marksAside returns the marks of list other than those in aside, which are
// looked up by markLookup: a value under a part carrying many deep marks
// carries them all, and scanning aside for each would cost the square of
// them.
func marksAside(list, aside []Mark) []Mark {
	if aside == nil {
		return list
	}
	var out []Mark
	var set markLookup
	for _, m := range list {
		if !set.holds(aside, m) {
			out = append(out, m)
		}
	}
	return out
}

// deepOf returns the deep marks among ms, or nil if there are none.
func deepOf(ms []Mark) []Mark {
	var out []Mark
	for _, m := range ms {
		if isDeep(m) {
			out = append(out, m)
		}
	}
	return out
}
