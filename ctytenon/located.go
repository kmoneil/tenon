package ctytenon

import (
	"slices"

	"github.com/kmoneil/tenon"
	"github.com/zclconf/go-cty/cty"
)

// LocatedMarksFromCty returns the tenon located marks of marks, cty's marks
// with the paths of the values within v that carry them, as cty's
// UnmarkDeepWithPaths gives them and its MarkWithPaths takes them. Each path
// crosses as PathFromCty crosses it within v, and each mark as b.MarkFromCty
// maps it. cty hands a container's marks to every value read out of it, and
// FromCty crosses them so, onto the container and every value within it but
// the members of a set, which carry none; an entry's marks are located so
// too. The result is merged, as [tenon.MergeLocatedMarks] merges it.
//
// So [tenon.WithLocatedMarks] puts the result on FromCty's crossing of v
// unmarked as FromCty crosses v.MarkWithPaths(marks), and it returns the
// entries go-cty's MarkWithPaths drops silently: those whose paths reach
// nothing, or run through a null or a value not known yet.
//
// It fails as FromCty fails on v unmarked, as PathFromCty fails on a path,
// and with CodeUnmappedMark where a mark is one b.MarkFromCty does not map.
// It panics on cty.NilVal, which is not a value.
func (b Bridge) LocatedMarksFromCty(marks []cty.PathValueMarks, v cty.Value) ([]tenon.LocatedMarks, error) {
	if v.Type() == cty.NilType {
		usagePanic("LocatedMarksFromCty called with cty.NilVal, which is not a value")
	}
	plain, _ := v.UnmarkDeep()
	crossed, err := b.FromCty(plain)
	if err != nil {
		return nil, err
	}
	var located []tenon.LocatedMarks
	var f failures
	for _, e := range marks {
		p, err := b.PathFromCty(e.Path, plain)
		if err != nil {
			return nil, err
		}
		mapped, ok := b.marksFromCty(e.Marks, p, &f)
		if !ok || len(mapped) == 0 {
			continue
		}
		at := p.Apply(crossed)
		if at.IsError() {
			// What the path does not reach, placing the entry says.
			located = append(located, tenon.LocatedMarks{Path: p, Marks: mapped})
			continue
		}
		tenon.Walk(at, func(q tenon.Path, w tenon.Value) tenon.WalkAction {
			located = append(located, tenon.LocatedMarks{Path: continued(p, q), Marks: mapped})
			if w.IsResolved() && w.Type().Kind() == tenon.KindSet {
				return tenon.WalkSkip
			}
			return tenon.WalkContinue
		})
	}
	if len(f.list) > 0 {
		return nil, f.err()
	}
	return tenon.MergeLocatedMarks(located), nil
}

// continued returns the path q continuing from p.
func continued(p, q tenon.Path) tenon.Path {
	for _, s := range q.Steps() {
		if s.Kind() == tenon.StepAttribute {
			p = p.Attribute(s.Name())
		} else {
			p = p.Index(s.Key())
		}
	}
	return p
}

// LocatedMarksToCty returns cty's marks with the paths of the values within v
// that carry them, as cty's MarkWithPaths takes them, of the tenon located
// marks lm. Each path crosses as PathToCty crosses it within v, and each mark
// as b.MarkToCty maps it. cty hands a container's marks to every value read
// out of it, and ToCty leaves them off the values within it so; an entry
// leaves out the marks of the entries whose paths it is within, and an entry
// left with none is left out. The entries are one for each path, in the
// canonical order of their tenon paths.
//
// So go-cty's MarkWithPaths puts the result on ToCty's crossing of v
// unmarked as ToCty crosses what [tenon.WithLocatedMarks] makes of it.
//
// It fails as PathToCty fails on a path, and with CodeUnmappedMark where a
// mark is one b.MarkToCty does not map. It panics on the zero Value, which is
// not a value.
func (b Bridge) LocatedMarksToCty(lm []tenon.LocatedMarks, v tenon.Value) ([]cty.PathValueMarks, error) {
	if v.IsZero() {
		usagePanic("LocatedMarksToCty called with the zero Value, which is not a value")
	}
	// within holds the entries, outermost first, whose paths the entry at
	// hand is within: cty hands their marks to it.
	var within []tenon.LocatedMarks
	var out []cty.PathValueMarks
	var f failures
	for _, e := range tenon.MergeLocatedMarks(lm) {
		for len(within) > 0 && !e.Path.HasPrefix(within[len(within)-1].Path) {
			within = within[:len(within)-1]
		}
		own := slices.DeleteFunc(slices.Clone(e.Marks), func(m tenon.Mark) bool {
			return slices.ContainsFunc(within, func(o tenon.LocatedMarks) bool { return slices.Contains(o.Marks, m) })
		})
		within = append(within, e)
		if len(own) == 0 {
			continue
		}
		mapped, ok := b.marksToCty(own, e.Path, &f)
		if !ok {
			continue
		}
		p, err := b.PathToCty(e.Path, v)
		if err != nil {
			return nil, err
		}
		out = append(out, cty.PathValueMarks{Path: p, Marks: mapped})
	}
	if len(f.list) > 0 {
		return nil, f.err()
	}
	return out, nil
}

// PathSetFromCty returns the tenon paths of the paths in s, which locate
// values within v, each as PathFromCty crosses it, in the canonical order of
// paths ([tenon.ComparePaths]) and each once: what a tenon host holds in place
// of a set of paths. It fails as PathFromCty fails on a path, and panics on
// cty.NilVal, which is not a value.
func (b Bridge) PathSetFromCty(s cty.PathSet, v cty.Value) ([]tenon.Path, error) {
	if v.Type() == cty.NilType {
		usagePanic("PathSetFromCty called with cty.NilVal, which is not a value")
	}
	var paths []tenon.Path
	for _, p := range s.List() {
		t, err := b.PathFromCty(p, v)
		if err != nil {
			return nil, err
		}
		paths = append(paths, t)
	}
	slices.SortFunc(paths, tenon.ComparePaths)
	return slices.CompactFunc(paths, tenon.Path.Equal), nil
}

// PathSetToCty returns the cty path set of the tenon paths, which locate
// values within v, each as PathToCty crosses it. It fails as PathToCty fails
// on a path, and panics on the zero Value, which is not a value.
func (b Bridge) PathSetToCty(paths []tenon.Path, v tenon.Value) (cty.PathSet, error) {
	if v.IsZero() {
		usagePanic("PathSetToCty called with the zero Value, which is not a value")
	}
	crossed := make([]cty.Path, 0, len(paths))
	for _, p := range paths {
		c, err := b.PathToCty(p, v)
		if err != nil {
			return cty.PathSet{}, err
		}
		crossed = append(crossed, c)
	}
	return cty.NewPathSet(crossed...), nil
}
