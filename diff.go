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
// depends on its kind. OldMarks and NewMarks are slices of the change's own,
// which the caller may keep or change without touching the values diffed.
type Change struct {
	Kind ChangeKind
	// InCollection says that the change lies within a list, set or map that
	// Diff looked within, as a member change lies within its set. Both values
	// hold that collection with one element type, so Old and New are of one
	// type, which the change's display form leaves out (DI-037).
	InCollection       bool
	Path               Path
	Old, New           Value
	OldMarks, NewMarks []Mark
}

// String returns the display form of the change (DI-037), as in
//
//	~ .name: "web" -> "api"
//	+ .ports[2]: 8443
//	~ .tags: marks [] -> ["audited"]
//	~ .ports[0]: null -> 80
//
// A change within a list, set or map, as the last is, shows its parts without
// their type, which the collection's element type fixes in both values.
func (c Change) String() string {
	var b textWriter
	c.write(&b)
	return b.String()
}

func (c Change) write(b *textWriter) {
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
	defer b.keepStated(c.InCollection)()
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
	var b textWriter
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
// attached: the values within compare as if they did not carry it. A change
// within a list, set or map, which both values hold with one element type,
// says so in InCollection, and its display form leaves its parts' type out.
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
	d.compare(a, b, Path{}, nil, nil, false)
	return d.changes
}

// marksChanged returns the change of a part's marks from ownA to ownB, at p,
// within a collection where in says so. The lists are copied: they may be
// the parts' own storage, which a value never lets out.
func marksChanged(p Path, ownA, ownB []Mark, in bool) Change {
	return Change{Kind: ChangeMarks, Path: p, OldMarks: slices.Clone(ownA), NewMarks: slices.Clone(ownB), InCollection: in}
}

// differ collects the changes of a diff.
type differ struct {
	changes Changes
}

func (d *differ) add(c Change) { d.changes = append(d.changes, c) }

// compare adds the changes between the parts a and b at p. asideA and asideB
// are the deep marks of the parts that hold a and b, which a and b and every
// value within them compare without. in says that p lies within a list, set
// or map that both values hold with one element type, which fixes the type
// of a and b and of every part within them.
func (d *differ) compare(a, b Value, p Path, asideA, asideB []Mark, in bool) {
	na, nb := a.n, b.n
	ownA, ownB := marksAside(na.markList(), asideA), marksAside(nb.markList(), asideB)
	sameOwn := sameMarkSet(ownA, ownB)
	if !enterable(na, nb) {
		switch {
		case !restIdenticalAside(na, nb, asideA, asideB):
			d.add(Change{Kind: ChangeReplaced, Path: p, Old: a, New: b, InCollection: in})
		case sameOwn:
		case na.redactingMarks() != nil || nb.redactingMarks() != nil:
			d.add(Change{Kind: ChangeReplaced, Path: p, Old: a, New: b, InCollection: in})
		default:
			d.add(marksChanged(p, ownA, ownB, in))
		}
		return
	}
	if na == nb && sameMarkSet(asideA, asideB) {
		return
	}
	if !sameOwn {
		d.add(marksChanged(p, ownA, ownB, in))
	}
	innerA, innerB := deepOf(na.markList()), deepOf(nb.markList())
	switch na.typ.t.kind {
	case KindList, KindTuple:
		// A list's elements are of its element type in both values; a tuple's
		// are fixed only where the tuple's own type is.
		within := in || na.typ.t.kind == KindList
		x, y := na.data.([]Value), nb.data.([]Value)
		for i := range max(len(x), len(y)) {
			at := p.extend(indexStep(NumberFromInt(int64(i))))
			switch {
			case i >= len(x):
				d.add(Change{Kind: ChangeAdded, Path: at, New: y[i], InCollection: within})
			case i >= len(y):
				d.add(Change{Kind: ChangeRemoved, Path: at, Old: x[i], InCollection: within})
			default:
				d.compare(x[i], y[i], at, innerA, innerB, within)
			}
		}
	case KindMap:
		x, y := na.data.([]mapEntry), nb.data.([]mapEntry)
		key := func(e mapEntry) string { return e.key }
		at := func(name string) Path { return p.extend(indexStep(stringValue(name))) }
		mergeByName(x, y, key, func(i, j int) {
			switch {
			case j < 0:
				d.add(Change{Kind: ChangeRemoved, Path: at(x[i].key), Old: x[i].val, InCollection: true})
			case i < 0:
				d.add(Change{Kind: ChangeAdded, Path: at(y[j].key), New: y[j].val, InCollection: true})
			default:
				d.compare(x[i].val, y[j].val, at(x[i].key), innerA, innerB, true)
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
				d.add(Change{Kind: ChangeRemoved, Path: at(x[i].name), Old: vx[i], InCollection: in})
			case i < 0:
				d.add(Change{Kind: ChangeAdded, Path: at(y[j].name), New: vy[j], InCollection: in})
			default:
				d.compare(vx[i], vy[j], at(x[i].name), innerA, innerB, in)
			}
		})
	case KindSet:
		d.members(na, nb, p)
	}
}

// members adds the member changes between the sets na and nb at p. Their
// members are as the sets hold them, in iteration order: known members first,
// in canonical order, and then the rest, in the order of their encodings.
// Each change carries its member as its set gives it when read, the set's
// deep marks on it (DI-030), and the members are paired and ordered as the
// sets hold them, without those marks (DI-034, DI-035), which each side's set
// may hold differently.
func (d *differ) members(na, nb *node, p Path) {
	x, y := na.data.([]Value), nb.data.([]Value)
	oldAs, newAs := givenBy(na), givenBy(nb)
	removed := func(m Value) { d.add(Change{Kind: ChangeMemberRemoved, Path: p, Old: oldAs(m), InCollection: true}) }
	added := func(m Value) { d.add(Change{Kind: ChangeMemberAdded, Path: p, New: newAs(m), InCollection: true}) }
	kx, ky := knownMembers(x), knownMembers(y)
	knownMemberChanges(x[:kx], y[:ky], removed, added)
	gone, come := unpairedMembers(x[kx:], y[ky:])
	interleave(gone, come, removed, added)
}

// givenBy returns what gives a member of the set n as the set gives it when
// read: carrying the set's deep marks, attached through one attachment for
// all of its members, as Elements attaches them (MK-008).
func givenBy(n *node) func(Value) Value {
	deep := deepMarks(n.markList())
	if deep == nil {
		return func(m Value) Value { return m }
	}
	a := newAttachment(deep, nil)
	return func(m Value) Value { return Value{n: a.attach(m.n)} }
}

// knownMemberChanges reports the changes between the known members of two sets,
// which are distinct and in canonical order, so that a merge pairs them: a
// member of x alone is removed, a member of y alone added, and two that the
// order ties but that are not identical are one removed and one added.
func knownMemberChanges(x, y []Value, removed, added func(Value)) {
	for i, j := 0, 0; i < len(x) || j < len(y); {
		c := 0
		switch {
		case j == len(y):
			c = -1
		case i == len(x):
			c = 1
		default:
			c = compareValues(x[i], y[j])
		}
		switch {
		case c < 0:
			removed(x[i])
			i++
		case c > 0:
			added(y[j])
			j++
		default:
			if !Identical(x[i], y[j]) {
				removed(x[i])
				added(y[j])
			}
			i++
			j++
		}
	}
}

// unpairedMembers returns the members of x and of y, members of two sets that
// are not known, that no identical member of the other set pairs with. They
// are in the order a set holds them, which follows from the members, so
// identical ones tie there, and they pair one for one in a merge: within a
// run of members that tie, each pairs with an identical one not yet paired,
// where each looked through all the other's for one, the square of them
// (16,000 took 1.4 s).
func unpairedMembers(x, y []Value) (removed, added []Value) {
	alike := notKnownOrder()
	for i, j := 0, 0; i < len(x) || j < len(y); {
		c := 0
		switch {
		case j == len(y):
			c = -1
		case i == len(x):
			c = 1
		default:
			c = alike(x[i].n, y[j].n)
		}
		if c < 0 {
			removed = append(removed, x[i])
			i++
			continue
		}
		if c > 0 {
			added = append(added, y[j])
			j++
			continue
		}
		ei, ej := i+1, j+1
		for ei < len(x) && alike(x[i].n, x[ei].n) == 0 {
			ei++
		}
		for ej < len(y) && alike(y[j].n, y[ej].n) == 0 {
			ej++
		}
		paired := make([]bool, ej-j)
		for _, m := range x[i:ei] {
			found := false
			for k, o := range y[j:ej] {
				if !paired[k] && Identical(m, o) {
					paired[k], found = true, true
					break
				}
			}
			if !found {
				removed = append(removed, m)
			}
		}
		for k, o := range y[j:ej] {
			if !paired[k] {
				added = append(added, o)
			}
		}
		i, j = ei, ej
	}
	return removed, added
}

// interleave reports the removals and additions of members that are not
// known in the order of their display forms as the sets hold them, without
// their type (DI-010), each written once however long it waits its turn: the
// type is the sets' element type, and writing it for each member would cost
// the members times its length. A removal and an addition that
// read alike are ordered by the members themselves, as a set holding the
// members of both sets orders members that encode alike (DI-035, EQ-044): the
// key follows from the member and not from the side it came from, so
// Diff(b, a) mirrors Diff(a, b), where putting the removal first put a
// different member first each way. A removal still leads where even that
// comparison ties, which only members told apart by what EQ-045 leaves
// unordered reach.
func interleave(gone, come []Value, removed, added func(Value)) {
	alike := notKnownOrder()
	texts := func(ms []Value) []string {
		out := make([]string, len(ms))
		for i, m := range ms {
			b := textWriter{stated: true}
			m.write(&b)
			out[i] = b.String()
		}
		return out
	}
	goneText, comeText := texts(gone), texts(come)
	for i, j := 0, 0; i < len(gone) || j < len(come); {
		c := 0
		switch {
		case j == len(come):
			c = -1
		case i == len(gone):
			c = 1
		default:
			if c = strings.Compare(goneText[i], comeText[j]); c == 0 {
				c = alike(gone[i].n, come[j].n)
			}
		}
		if c <= 0 {
			removed(gone[i])
			i++
		} else {
			added(come[j])
			j++
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
// they carry themselves, once the marks in asideA are set aside from every
// value within a, and those in asideB from every value within b.
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
	return Identical(withMarksOf(a, b), Value{n: b})
}

// identicalAside reports whether a and b are identical once the marks in
// asideA are set aside from a and every value within it, and those in asideB
// from b.
func identicalAside(a, b *node, asideA, asideB []Mark) bool {
	if asideA == nil && asideB == nil {
		return Identical(Value{n: a}, Value{n: b})
	}
	return sameMarkSet(marksAside(a.markList(), asideA), marksAside(b.markList(), asideB)) &&
		restIdenticalAside(a, b, asideA, asideB)
}

// withMarksOf returns a as a value carrying the marks of b, for comparing the
// rest of a and b once their marks are known to agree.
func withMarksOf(a, b *node) Value {
	if a.marks == b.marks {
		return Value{n: a}
	}
	c := a.clone()
	c.marks = b.marks
	return Value{n: c}
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
