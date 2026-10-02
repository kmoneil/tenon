package ctytenon

import (
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/kmoneil/tenon"
	"github.com/zclconf/go-cty/cty"
)

// PathFromCty returns the tenon path of the cty path p, which locates a value
// within v. An attribute step is an attribute step, and an index step one of
// the same key, a string or a number, but for a step into a set: cty takes a
// member of a set by the member itself, and tenon by its place in the set's
// order, so the step is the place of the member's crossing in the order of
// the set's crossing, and v must hold the set for it to be found.
//
// It fails where a step has no tenon form: an attribute name tenon refuses,
// a key that is neither a known string nor a known number, and a step into a
// set that v does not hold, or holds without the member, or that does not
// cross. It panics on cty.NilVal, which is not a value.
func (b Bridge) PathFromCty(p cty.Path, v cty.Value) (tenon.Path, error) {
	if v.Type() == cty.NilType {
		usagePanic("PathFromCty called with cty.NilVal, which is not a value")
	}
	t, n, err := b.pathFromCty(p, v)
	if err != nil {
		return tenon.Path{}, fmt.Errorf("ctytenon: step %d of %#v: %w", n, p, err)
	}
	return t, nil
}

// pathFromCty returns the tenon path of p within v, or as many of its first
// steps as cross, how many those are, and why the next does not.
func (b Bridge) pathFromCty(p cty.Path, v cty.Value) (tenon.Path, int, error) {
	var t tenon.Path
	at := v
	for i, step := range p {
		at, _ = at.Unmark()
		switch s := step.(type) {
		case cty.GetAttrStep:
			if err := tenon.CheckAttributeNames(s.Name); err != nil {
				return t, i, err
			}
			t = t.Attribute(s.Name)
		case cty.IndexStep:
			key, _ := s.Key.Unmark()
			if at.Type().IsSetType() {
				place, err := b.placeInSet(at, key)
				if err != nil {
					return t, i, err
				}
				t = t.Index(tenon.NumberFromInt(int64(place)))
				// The member is its own key, which cty's Apply cannot
				// return where the set holds unknown members.
				at = key
				continue
			}
			k, err := keyFromCty(key)
			if err != nil {
				return t, i, err
			}
			t = t.Index(k)
		default:
			return t, i, fmt.Errorf("%#v is no step ctytenon knows", step)
		}
		at = next(at, step)
	}
	return t, len(p), nil
}

// keyFromCty returns the tenon key of key, a cty index step's.
func keyFromCty(key cty.Value) (tenon.Value, error) {
	switch {
	case !key.IsKnown() || key.IsNull():
		return tenon.Value{}, fmt.Errorf("the key %#v is not a known string or number", key)
	case key.Type() == cty.String:
		if s := key.AsString(); utf8.ValidString(s) {
			return tenon.String(s), nil
		}
		return tenon.Value{}, fmt.Errorf("the key %#v is not valid UTF-8", key)
	case key.Type() == cty.Number:
		if n := numberFromCty(key.AsBigFloat()); !n.IsError() {
			return n, nil
		}
		return tenon.Value{}, fmt.Errorf("the key %#v is no tenon number", key)
	}
	return tenon.Value{}, fmt.Errorf("the key %#v is not a known string or number", key)
}

// placeInSet returns the place of the crossing of member, which the cty set
// set holds, in the order of the set's crossing.
func (b Bridge) placeInSet(set, member cty.Value) (int, error) {
	if !set.IsKnown() || set.IsNull() {
		return 0, errors.New("the step is into a set the value does not hold")
	}
	ts, err := b.FromCty(set)
	if err != nil {
		return 0, fmt.Errorf("the set the step is into does not cross: %w", err)
	}
	tm, err := b.FromCty(member)
	if err != nil {
		return 0, fmt.Errorf("the member the step is to does not cross: %w", err)
	}
	place := 0
	for e := range ts.ElementsSeq() {
		if e, _ := tenon.Unmark(e); e.Equal(tm) {
			return place, nil
		}
		place++
	}
	return 0, fmt.Errorf("the set does not hold %#v", member)
}

// next returns the value the step from at leads to, cty.NilVal where at does
// not say: where it is not a known value holding what the step names.
func next(at cty.Value, step cty.PathStep) cty.Value {
	if at.Type() == cty.NilType || !at.IsKnown() || at.IsNull() {
		return cty.NilVal
	}
	v, err := step.Apply(at)
	if err != nil {
		return cty.NilVal
	}
	return v
}

// PathToCty returns the cty path of the tenon path p, which locates a value
// within v, as PathFromCty gives the tenon path of a cty one: a step into a
// set, which tenon takes by the member's place in the set's order, is a step
// to the member's crossing, and v must hold the set for it to be found.
//
// It fails where a step into a set has no cty form: the set is not one v
// holds, or has no member at that place, or the member does not cross. It
// panics on the zero Value, which is not a value.
func (b Bridge) PathToCty(p tenon.Path, v tenon.Value) (cty.Path, error) {
	if v.IsZero() {
		usagePanic("PathToCty called with the zero Value, which is not a value")
	}
	var c cty.Path
	at := v
	for i, s := range p.Steps() {
		if !at.IsZero() {
			at, _ = tenon.Unmark(at)
		}
		if s.Kind() == tenon.StepAttribute {
			c = append(c, cty.GetAttrStep{Name: s.Name()})
			at = tenonNext(at, s)
			continue
		}
		if !at.IsZero() && at.IsResolved() && at.Type().Kind() == tenon.KindSet {
			member, err := b.memberAt(at, s.Key())
			if err != nil {
				return nil, fmt.Errorf("ctytenon: step %d of %s: %w", i, p, err)
			}
			c = append(c, cty.IndexStep{Key: member})
		} else if key := s.Key(); key.Type() == tenon.StringType() {
			c = append(c, cty.IndexStep{Key: cty.StringVal(key.AsString())})
		} else {
			c = append(c, cty.IndexStep{Key: numberToCty(key)})
		}
		at = tenonNext(at, s)
	}
	return c, nil
}

// memberAt returns the crossing of the member of set at the place key, a
// tenon index step's.
func (b Bridge) memberAt(set, key tenon.Value) (cty.Value, error) {
	member, ok := memberOf(set, key)
	if !ok {
		return cty.NilVal, fmt.Errorf("the set holds no member at %s", key)
	}
	m, err := b.ToCty(member)
	if err != nil {
		return cty.NilVal, fmt.Errorf("the member the step is to does not cross: %w", err)
	}
	m, _ = m.Unmark()
	return m, nil
}

// memberOf returns the member of set, a tenon set, at the place key, and
// whether it holds one there.
func memberOf(set, key tenon.Value) (tenon.Value, bool) {
	place, ok := key.AsInt64()
	if !set.HasContent() || !ok || place < 0 {
		return tenon.Value{}, false
	}
	for e := range set.ElementsSeq() {
		if place == 0 {
			return e, true
		}
		place--
	}
	return tenon.Value{}, false
}

// tenonNext returns the value the step s from at leads to, the zero Value
// where at does not say: where it is not a value holding what s names.
func tenonNext(at tenon.Value, s tenon.Step) tenon.Value {
	if at.IsZero() || !at.HasContent() {
		return tenon.Value{}
	}
	switch k := at.Type().Kind(); {
	case s.Kind() == tenon.StepAttribute && k == tenon.KindObject:
		if v, ok := at.LookupAttribute(s.Name()); ok {
			return v
		}
	case k == tenon.KindMap && s.Key().Type() == tenon.StringType():
		if v, ok := at.LookupMapElement(s.Key().AsString()); ok {
			return v
		}
	case k == tenon.KindSet:
		if v, ok := memberOf(at, s.Key()); ok {
			return v
		}
	case k == tenon.KindList || k == tenon.KindTuple:
		if i, ok := s.Key().AsInt64(); ok && i >= 0 && i < int64(at.Len()) {
			return at.Index(int(i))
		}
	}
	return tenon.Value{}
}

// ErrorFromCty returns err, an error cty gave of the value v, as the
// [*tenon.Error] that says the same: a diagnostic with code CodeCtyError and
// err's message, located by err's path where err is a cty.PathError, as
// PathFromCty gives it, as far as its steps cross. Where a value on the path
// carries a cty mark that crosses as a redacting mark, or one the Bridge does
// not map, the diagnostic is located at that value, and its message names
// the value by a placeholder rather than saying what cty said of what it
// holds. An error that joins others gives a diagnostic for each. A nil err
// gives nil.
func (b Bridge) ErrorFromCty(err error, v cty.Value) error {
	if err == nil {
		return nil
	}
	var diags []tenon.Diagnostic
	for _, e := range joined(err) {
		d := tenon.Diagnostic{Code: CodeCtyError, Message: e.Error()}
		var pe cty.PathError
		if errors.As(e, &pe) {
			d.Path, d.Message = b.locate(pe.Path, v, d.Message)
		}
		if d.Message == "" {
			d.Message = "cty gave an error with no message"
		}
		diags = append(diags, d)
	}
	return tenon.NewError(tenon.ErrorVal(diags...), err)
}

// joined returns the errors err joins, or err alone.
func joined(err error) []error {
	if j, ok := err.(interface{ Unwrap() []error }); ok {
		var out []error
		for _, e := range j.Unwrap() {
			out = append(out, joined(e)...)
		}
		return out
	}
	return []error{err}
}

// locate returns where in v, as a tenon path, a diagnostic of message at the
// cty path p lies, and what it says: as far along p as its steps cross, and,
// where a value on the way carries a cty mark that is redacting in tenon or
// that the Bridge does not map, that value, named by a placeholder.
func (b Bridge) locate(p cty.Path, v cty.Value, message string) (tenon.Path, string) {
	for i := range len(p) + 1 {
		at, err := p[:i].Apply(v)
		if err != nil {
			break
		}
		if _, marks := at.Unmark(); len(marks) > 0 {
			if placeholder, hidden := b.hides(marks); hidden {
				t, _, _ := b.pathFromCty(p[:i], v)
				return t, placeholder + " holds what cty refused"
			}
		}
	}
	t, _, _ := b.pathFromCty(p, v)
	return t, message
}

// hides reports whether a value carrying marks, cty's, must not have what it
// holds said, and the placeholder that names it: where a mark crosses as a
// redacting one, or the Bridge maps none, which could be one.
func (b Bridge) hides(marks cty.ValueMarks) (string, bool) {
	var redacting []tenon.Mark
	unmapped := false
	for _, m := range sortedMarks(marks) {
		var t tenon.Mark
		ok := false
		if b.MarkFromCty != nil {
			t, ok = b.MarkFromCty(m)
		}
		switch {
		case !ok:
			unmapped = true
		case t.Redacting():
			redacting = append(redacting, t)
		}
	}
	switch {
	case len(redacting) > 0:
		return tenon.WithMarks(tenon.Null(tenon.BoolType()), redacting...).String(), true
	case unmapped:
		return "a value carrying the cty marks " + fmtMarks(marks), true
	}
	return "", false
}

// fmtMarks returns cty's marks as Go syntax, in order.
func fmtMarks(marks cty.ValueMarks) string {
	s := ""
	for i, m := range sortedMarks(marks) {
		if i > 0 {
			s += ", "
		}
		s += fmt.Sprintf("%#v", m)
	}
	return s
}
