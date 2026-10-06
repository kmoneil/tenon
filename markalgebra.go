package tenon

import "slices"

// MergeLocatedMarks returns the entries of the lists as one list (MK-014):
// an entry for each path, its marks the union of the marks of the entries
// for that path, sorted by identifier, in the canonical order of paths. An
// entry with no marks adds nothing. The lists are left as they are.
//
// MergeLocatedMarks panics on a mark WithMarks would panic on.
func MergeLocatedMarks(lists ...[]LocatedMarks) []LocatedMarks {
	var all []LocatedMarks
	for _, list := range lists {
		for _, e := range list {
			if len(e.Marks) != 0 {
				checkMarks("MergeLocatedMarks", e.Marks)
				all = append(all, e)
			}
		}
	}
	return mergeEntries(all)
}

// CompactLocatedMarks returns the entries merged, as MergeLocatedMarks merges
// them, with each deep mark taken from the entries for the paths within a
// path whose entry carries it, and the entries left with no marks dropped
// (MK-014). Placing the entry for the outer path puts the deep mark on every
// value within, so WithLocatedMarks gives the same value from the entries
// compacted as from the entries: the located marks of a value marked by a
// deep mark compact to the outermost values carrying it, rather than naming
// every value within them.
//
// CompactLocatedMarks panics on a mark WithMarks would panic on.
func CompactLocatedMarks(lm []LocatedMarks) []LocatedMarks {
	merged := MergeLocatedMarks(lm)
	// within holds the entries, outermost first, whose paths the entry at
	// hand is within, each with every deep mark carried along the way.
	type outer struct {
		path Path
		deep []Mark
	}
	var within []outer
	out := merged[:0]
	for _, e := range merged {
		for len(within) > 0 && !e.Path.HasPrefix(within[len(within)-1].path) {
			within = within[:len(within)-1]
		}
		var carried []Mark
		if len(within) > 0 {
			carried = within[len(within)-1].deep
			e.Marks = slices.DeleteFunc(e.Marks, func(m Mark) bool {
				_, found := placeMark(carried, m)
				return found
			})
		}
		if deep := deepMarks(e.Marks); deep != nil {
			all, _ := mergeMarks(carried, deep)
			within = append(within, outer{path: e.Path, deep: all})
		}
		if len(e.Marks) != 0 {
			out = append(out, e)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// FilterLocatedMarks returns the entries with the marks keep reports true
// for, in their order, without those left with no marks (MK-014). The
// entries are left as they are.
func FilterLocatedMarks(lm []LocatedMarks, keep func(Mark) bool) []LocatedMarks {
	var out []LocatedMarks
	for _, e := range lm {
		var marks []Mark
		for _, m := range e.Marks {
			if keep(m) {
				marks = append(marks, m)
			}
		}
		if len(marks) != 0 {
			out = append(out, LocatedMarks{Path: e.Path, Marks: marks})
		}
	}
	return out
}

// SameMarks reports whether a and b have the same located marks (MK-015):
// the same paths to the values carrying marks, and the same marks at each.
// Equals ignores marks, and Identical compares them along with everything
// else; SameMarks compares the marks alone, wherever they are, so a value
// and the value it became when a host replaced what it holds have the same
// marks where they were put back alike. Two values holding no mark have
// the same marks, answered without a walk.
func SameMarks(a, b Value) bool {
	na, nb := a.data(), b.data()
	if !na.isMarked() || !nb.isMarked() {
		return na.isMarked() == nb.isMarked()
	}
	return slices.EqualFunc(MarkLocations(a), MarkLocations(b), func(x, y LocatedMarks) bool {
		return x.Path.Equal(y.Path) && sameMarkSet(x.Marks, y.Marks)
	})
}

// HasMarkDeep reports whether v or a value within it carries the mark
// (MK-015): HasMark asks of v alone. A set's marks are the set's, its
// members carrying none. A value holding no mark is answered without a walk,
// and otherwise only the values holding a mark are looked at. HasMarkDeep
// panics if the mark is nil.
func HasMarkDeep(v Value, m Mark) bool {
	n := v.data()
	if m == nil {
		usagePanic("HasMarkDeep called with a nil Mark")
	}
	return n.holdsMark(m)
}

// holdsMark reports whether n or a value within it carries m.
func (n *node) holdsMark(m Mark) bool {
	if n.marks.contains(m) {
		return true
	}
	if !n.markedWithin {
		return false
	}
	var members []Value
	switch data := n.data.(type) {
	case *pendingMembers:
		members = data.vals
	case []mapEntry:
		for _, e := range data {
			if e.val.n.holdsMark(m) {
				return true
			}
		}
	case []Value:
		members = data
	}
	for _, c := range members {
		if c.n.holdsMark(m) {
			return true
		}
	}
	return false
}

// MarkAction is what RewriteMarks does with a mark: KeepMark, the zero
// MarkAction, keeps it; DropMark drops it; ReplaceMark puts other marks in
// its place.
type MarkAction struct {
	replace bool
	with    []Mark
}

// KeepMark keeps the mark where it is.
func KeepMark() MarkAction { return MarkAction{} }

// DropMark takes the mark off.
func DropMark() MarkAction { return MarkAction{replace: true} }

// ReplaceMark takes the mark off and puts the marks given in its place,
// none of them dropping it. It panics on a mark WithMarks would panic on.
func ReplaceMark(with ...Mark) MarkAction {
	checkMarks("ReplaceMark", with)
	marks, _ := mergeMarks(nil, with)
	return MarkAction{replace: true, with: marks}
}

// Equal reports whether a and b do the same with a mark: both keep it, or
// both put the same marks in its place, none where they drop it.
func (a MarkAction) Equal(b MarkAction) bool {
	return a.replace == b.replace && sameMarkSet(a.with, b.with)
}

// RewriteMarks returns v with its marks rewritten by fn (MK-016). fn is
// handed each mark at each value carrying it, with that value's path, in
// the canonical order of paths and, at one value, of the marks'
// identifiers, as MarkLocations gives them, and answers what to do with it.
// The result is v with every mark taken off and the marks kept and the
// replacements put back where they were handed, as WithLocatedMarks puts
// them: a deep mark is handed at every value it reached, and where it is
// kept on a value it stays on everything that value holds, so dropping it
// takes dropping it there too. Where fn keeps every mark the result is v
// itself, with nothing rebuilt.
func RewriteMarks(v Value, fn func(Path, Mark) MarkAction) Value {
	located := MarkLocations(v)
	changed := false
	for i, e := range located {
		var marks []Mark
		for _, m := range e.Marks {
			a := fn(e.Path, m)
			if !a.replace {
				marks = append(marks, m)
				continue
			}
			changed = true
			marks = append(marks, a.with...)
		}
		located[i].Marks = marks
	}
	if !changed {
		return v
	}
	plain, _ := UnmarkDeep(v)
	rewritten, unplaced := WithLocatedMarks(plain, located)
	if unplaced != nil {
		internalPanic("RewriteMarks could not put back the marks at %s", unplaced[0].Path)
	}
	return rewritten
}
