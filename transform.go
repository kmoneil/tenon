package tenon

// Transform returns v with v and every value within it replaced by what fn
// answers for it (VA-028). fn is given each value with its path after the
// values within it have been replaced, siblings in the order Walk visits
// them, and each value as Walk hands it, marks included.
//
// A value holding members is rebuilt from them as they became, carrying its
// own marks again, a deep mark reaching what it now holds: an object or a
// tuple of whatever its members became; a list or a map of the type its
// members share, a pending member resolved to it where it can be; a set of
// the type its members share, its members merged where they became equal and
// their marks moved to the set, as a set's members carry none. A value whose
// members all stay as they were is not rebuilt. Members that share no type
// make an error value with code CodeConvertNoCommonType located at the value
// they were to be rebuilt into, and an error value fn answers is Transform's
// answer; neither panics. Transform panics on the zero Value, and where fn
// answers it.
func Transform(v Value, fn func(Path, Value) Value) Value {
	return TransformWith(v, nil, fn)
}

// TransformWith is Transform with a function given each value before what is
// within it, as well as after (VA-028). enter answers what stands in the
// value's place before its members are visited, and what to do next:
// WalkContinue visits the members of what it answered, WalkSkip passes over
// them, and WalkStop ends the transform, after which neither function is
// called, every value not yet visited stands as it is, and the values
// holding them are rebuilt as Transform rebuilds them. exit is as
// Transform's fn. Either may be nil, which leaves each value as it is.
func TransformWith(v Value, enter func(Path, Value) (Value, WalkAction), exit func(Path, Value) Value) Value {
	v.data()
	t := transformer{enter: enter, exit: exit}
	r, _ := t.value(v, Path{})
	return r
}

// transformer holds a transform's functions, and whether it has stopped.
type transformer struct {
	enter   func(Path, Value) (Value, WalkAction)
	exit    func(Path, Value) Value
	stopped bool
}

// value transforms v at p, and reports false where the answer is an error
// value, which ends the transform.
func (t *transformer) value(v Value, p Path) (Value, bool) {
	if t.stopped {
		return v, true
	}
	if t.enter != nil {
		r, action := t.enter(p, v)
		if r.IsZero() {
			usagePanic("TransformWith's enter function answered the zero Value at %s", p)
		}
		if r.IsError() {
			return r, false
		}
		v = r
		switch action {
		case WalkStop:
			t.stopped = true
			return v, true
		case WalkSkip:
			return t.leave(v, p)
		}
	}
	if v.HasMembers() {
		r, ok := t.members(v, p)
		if !ok {
			return r, false
		}
		v = r
	}
	return t.leave(v, p)
}

// leave hands v, at p, to the exit function, unless the transform has
// stopped.
func (t *transformer) leave(v Value, p Path) (Value, bool) {
	if t.stopped || t.exit == nil {
		return v, true
	}
	r := t.exit(p, v)
	if r.IsZero() {
		usagePanic("the transform's function answered the zero Value at %s", p)
	}
	return r, !r.IsError()
}

// members transforms the members of v, at p, and rebuilds v from them where
// one of them changed.
func (t *transformer) members(v Value, p Path) (Value, bool) {
	var steps []Step
	var vals []Value
	changed := false
	var failed Value
	membersOf(v, func(s Step, m Value) bool {
		r, ok := t.value(m, p.extend(s))
		if !ok {
			failed = r
			return false
		}
		changed = changed || r.n != m.n
		steps, vals = append(steps, s), append(vals, r)
		return true
	})
	if !failed.IsZero() {
		return failed, false
	}
	if !changed {
		return v, true
	}
	inner, own := Unmark(v)
	n := inner.data()
	var r Value
	switch h, held := n.held(); {
	case held && h.names != nil, !held && n.typ.t.kind == KindObject:
		attrs := make(map[string]Value, len(vals))
		for i, s := range steps {
			attrs[s.name] = vals[i]
		}
		r = Object(attrs)
	case held, n.typ.t.kind == KindTuple:
		r = Tuple(vals...)
	default:
		r = rebuildCollection(n.typ, steps, vals, p)
	}
	if r.IsError() {
		return r, false
	}
	return WithMarks(r, own...), true
}

// rebuildCollection rebuilds a list, a map or a set of type t, at p, from
// its members as they became, by the steps to them: of the type the members
// share, a pending member resolved to it, or of t's element type where every
// member is pending; or the failure that they share none.
func rebuildCollection(t Type, steps []Step, vals []Value, p Path) Value {
	elem, typed := t.ElementType(), false
	for _, m := range vals {
		n := m.data()
		switch {
		case n.state == statePending:
		case !typed:
			elem, typed = n.typ, true
		case !n.typ.Equal(elem):
			return noCommonMemberType(t, p, "values of "+elem.String()+" and of "+n.typ.String())
		}
	}
	for i, m := range vals {
		n := m.data()
		if n.state != statePending {
			continue
		}
		if !Satisfies(n.constraint(), elem) {
			return noCommonMemberType(t, p, "values of "+elem.String()+" and a pending value of "+n.constraint().String())
		}
		vals[i] = Resolve(m, elem)
	}
	switch t.t.kind {
	case KindList:
		return List(elem, vals...)
	case KindMap:
		entries := make(map[string]Value, len(vals))
		for i, s := range steps {
			entries[s.key.AsString()] = vals[i]
		}
		return Map(elem, entries)
	}
	var marks []Mark
	for i, m := range vals {
		var taken []Mark
		vals[i], taken = UnmarkDeep(m)
		marks = append(marks, taken...)
	}
	return WithMarks(Set(elem, vals...), marks...)
}

// noCommonMemberType is the failure to rebuild a value of type t, at p, from
// members that became what became says, which share no type.
func noCommonMemberType(t Type, p Path, became string) Value {
	return ErrorVal(Diagnostic{
		Code:    CodeConvertNoCommonType,
		Message: "Transform cannot rebuild the " + kindNoun(t.t.kind) + ": its members became " + became + ", which share no type",
		Path:    p,
	})
}
