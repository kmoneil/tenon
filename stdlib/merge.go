package stdlib

import (
	"slices"

	"github.com/kmoneil/tenon"
)

// mergeShapes is the most arguments of Merge that may be null whose answers
// the derivation lists shape by shape, each such argument adding its
// attributes or none: past it, the attributes they add are optional.
const mergeShapes = 4

// mergeArg is what one argument of Merge, unmarked, says of the answer.
type mergeArg struct {
	c tenon.Value
	// typ is the argument's type, the zero Type where it is pending.
	typ tenon.Type
	// object is whether the argument is an object whatever it settles to,
	// which makes the answer one.
	object bool
	// adds is how the argument adds its keys, which entries reads where
	// they are wanted.
	adds mergeAdds
	// maybe is whether the argument adds its entries or none: an object not
	// known yet that may be null.
	maybe bool
	// open is whether the argument may add keys not known yet, each holding
	// what elem admits: a map not known yet, or a pending value not known
	// to be null that holds no members.
	open bool
	elem tenon.Constraint
	read []mergeEntry
	done bool
}

// mergeAdds is how an argument of Merge adds its keys: none, as a null
// does; its members, a map's entries or an object's attributes, a pending
// object's among them; or the attributes an object's type names, each not
// known yet.
type mergeAdds int

const (
	addsNothing mergeAdds = iota
	addsMembers
	addsTypeAttributes
)

// mergeEntry is a key an argument of Merge adds, its value and the
// constraint the value's type satisfies.
type mergeEntry struct {
	name string
	v    tenon.Value
	c    tenon.Constraint
}

// entries returns the keys the argument adds, in its order, read once.
func (a *mergeArg) entries() []mergeEntry {
	if a.done {
		return a.read
	}
	a.done = true
	switch {
	case a.adds == addsTypeAttributes:
		for _, name := range a.typ.AttributeNames() {
			at := a.typ.AttributeType(name)
			a.read = append(a.read, mergeEntry{name, tenon.Unknown(at), tenon.Exactly(at)})
		}
	case a.adds == addsMembers && a.object:
		for name, v := range a.c.Attributes() {
			a.read = append(a.read, mergeEntry{name, v, typeOf(v)})
		}
	case a.adds == addsMembers:
		ec := tenon.Exactly(a.typ.ElementType())
		a.read = make([]mergeEntry, 0, a.c.Len())
		for k, v := range a.c.MapEntries() {
			a.read = append(a.read, mergeEntry{k, v, ec})
		}
	}
	return a.read
}

// mergeArgOf reads the unmarked argument c of Merge.
func mergeArgOf(c tenon.Value) mergeArg {
	if c.IsPending() {
		switch {
		case c.IsNull():
			// A null with no type yet, as a language's untyped null, adds
			// nothing and has no type for the others to agree with.
			return mergeArg{c: c}
		case isObject(c):
			return mergeArg{c: c, object: true, adds: addsMembers}
		}
		return mergeArg{c: c, object: !slices.Contains(kindsOf(c.Constraint()), tenon.KindMap), open: true, elem: tenon.Any()}
	}
	t := c.Type()
	a := mergeArg{c: c, typ: t, object: t.Kind() == tenon.KindObject}
	switch {
	case c.IsNull():
	case c.HasMembers():
		a.adds = addsMembers
	case a.object:
		// An object not known yet has the attributes its type names.
		n := tenon.IsNull(c)
		a.adds, a.maybe = addsTypeAttributes, !n.IsKnown()
	default:
		a.open, a.elem = true, tenon.Exactly(t.ElementType())
	}
	return a
}

// mergeResult returns the constraint of Merge's answer: a map of the type
// every argument with a type has where they are all one map type, and
// otherwise an object of the keys they add.
func mergeResult(as []mergeArg) tenon.Constraint {
	var mapType tenon.Type
	object, settled := false, true
	for _, a := range as {
		switch {
		case a.object:
			object = true
		case !a.typ.IsZero() && mapType.IsZero():
			mapType = a.typ
		case !a.typ.IsZero() && !a.typ.Equal(mapType):
			object = true
		case a.typ.IsZero() && a.open:
			settled = false
		}
	}
	switch {
	case object, settled && mapType.IsZero():
		return mergeObject(as)
	case settled:
		return tenon.Exactly(mapType)
	case mapType.IsZero():
		// A pending argument may settle to a map, of any type where no
		// other has one.
		return tenon.OneOf(tenon.MapOf(tenon.Any()), mergeObject(as))
	}
	return tenon.OneOf(tenon.Exactly(mapType), mergeObject(as))
}

// mergeObject returns the constraint of Merge's answer where it is an
// object: open where an argument may add keys not known yet, and otherwise
// each shape the arguments that may be null give, up to mergeShapes of
// them, past which the attributes they add are optional.
func mergeObject(as []mergeArg) tenon.Constraint {
	var maybe []int
	open := false
	for i, a := range as {
		if a.maybe {
			maybe = append(maybe, i)
		}
		open = open || a.open
	}
	if open || len(maybe) > mergeShapes {
		return mergeFields(as, nil, open)
	}
	var shapes []tenon.Constraint
	for world := range 1 << len(maybe) {
		null := map[int]bool{}
		for j, i := range maybe {
			null[i] = world&(1<<j) == 0
		}
		shapes = with(shapes, mergeFields(as, null, false))
	}
	return oneOf(shapes)
}

// mergeFields returns the object the arguments give, an argument that may
// be null taken as null where null says so, as not null where null says it
// is not, and, where null is nil, as adding its attributes or none: each
// attribute required where an argument adds it for certain, and holding
// what the last argument that adds it for certain holds, or what a later one
// that may add it does, or, where none adds it for certain, any that may.
func mergeFields(as []mergeArg, null map[int]bool, open bool) tenon.Constraint {
	type field struct {
		cs []tenon.Constraint
		// certain is the last argument adding the attribute for certain,
		// and -1 where none does.
		certain int
	}
	fields := map[string]*field{}
	// opens holds, for each constraint the keys not known yet may hold,
	// the last argument that may add them.
	type opening struct {
		at   int
		elem tenon.Constraint
	}
	var opens []opening
	for i := range as {
		a := &as[i]
		if a.open {
			opens = slices.DeleteFunc(opens, func(o opening) bool { return o.elem.Equal(a.elem) })
			opens = append(opens, opening{i, a.elem})
		}
		if a.maybe && null[i] {
			continue
		}
		maybe := a.maybe && null == nil
		for _, e := range a.entries() {
			switch f := fields[e.name]; {
			case f == nil && maybe:
				fields[e.name] = &field{cs: []tenon.Constraint{e.c}, certain: -1}
			case f == nil, !maybe:
				fields[e.name] = &field{cs: []tenon.Constraint{e.c}, certain: i}
			default:
				f.cs = with(f.cs, e.c)
			}
		}
	}
	exact := !open
	types := map[string]tenon.Type{}
	out := map[string]tenon.Field{}
	for name, f := range fields {
		for _, o := range opens {
			if o.at > f.certain {
				f.cs = with(f.cs, o.elem)
			}
		}
		c := oneOf(f.cs)
		if f.certain >= 0 {
			out[name] = tenon.Required(c)
		} else {
			out[name] = tenon.Optional(c)
			exact = false
		}
		if c.Kind() == tenon.ConstraintExactly {
			types[name] = c.Type()
		} else {
			exact = false
		}
	}
	if exact {
		return tenon.Exactly(tenon.ObjectType(types))
	}
	return tenon.ObjectWith(out, !open)
}

// with returns cs with c added where no member of cs is equal to it.
func with(cs []tenon.Constraint, c tenon.Constraint) []tenon.Constraint {
	if slices.ContainsFunc(cs, c.Equal) {
		return cs
	}
	return append(cs, c)
}

// oneOf returns the constraint one of cs satisfies: the one, Any where one
// is Any, and the OneOf of them otherwise.
func oneOf(cs []tenon.Constraint) tenon.Constraint {
	switch {
	case len(cs) == 1:
		return cs[0]
	case slices.ContainsFunc(cs, func(c tenon.Constraint) bool { return c.Kind() == tenon.ConstraintAny }):
		return tenon.Any()
	}
	return tenon.OneOf(cs...)
}

// mergeMap returns Merge's answer of the map type t: the entries of each
// argument, a later key taking an earlier one, or, where a map is not known
// yet, the unknown map of the lengths they allow, since a key the others
// add is never taken away.
func mergeMap(t tenon.Type, as []mergeArg) tenon.Value {
	entries := map[string]tenon.Value{}
	var open []tenon.Value
	for i := range as {
		if as[i].open {
			open = append(open, as[i].c)
		}
		for _, e := range as[i].entries() {
			entries[e.name] = e.v
		}
	}
	if len(open) == 0 {
		return tenon.Map(t.ElementType(), entries)
	}
	lo, hi, bounded := int64(len(entries)), int64(len(entries)), true
	for _, c := range open {
		l, h, ok := lengthOf(c)
		if n := tenon.IsNull(c); n.IsKnown() && !n.AsBool() {
			lo = max(lo, l)
		}
		hi += h
		bounded = bounded && ok
	}
	ns := []tenon.Narrowing{tenon.NotNull(), tenon.LengthMin(lo)}
	if bounded {
		ns = append(ns, tenon.LengthMax(hi))
	}
	return tenon.Narrow(tenon.Unknown(t), ns...)
}

// MergeFunc merges maps and objects in order, an entry of a later argument
// taking the place of one with the same key: an answer of a map where
// every argument with a type is a map of one type, and otherwise an object
// of every key the arguments add, each attribute of the type of the last
// argument adding it. A null adds nothing, and one with no type yet, as a
// language's untyped null, has no type for the others to agree with. An
// object not known yet adds its attributes, not known yet, beside the
// others; one that may be null adds them or none, the answer pending among
// the shapes that gives, and a map not known yet adds keys not known yet.
// Every argument's own marks reach the answer, a null argument's included,
// and the members keep theirs.
var MergeFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "Merge",
	Description: "Merges the entries of the given maps, or the attributes of the given objects, into one map or object, a later key taking the place of an earlier one.",
	VarParam:    &tenon.Param{Name: "maps", Description: "The maps and objects, in order.", Constraint: tenon.Any(), AllowNull: true, AllowUnknown: true, AllowPending: true, AllowMarked: true},
	ResultOf: func(args []tenon.Value, _ tenon.Policy) (tenon.Constraint, error) {
		as := make([]mergeArg, len(args))
		for i, a := range args {
			c, _ := tenon.Unmark(a)
			if !mayBe(c, tenon.KindMap, tenon.KindObject) {
				return tenon.Constraint{}, wrongKind("Merge", i, a, "maps and objects")
			}
			as[i] = mergeArgOf(c)
		}
		return mergeResult(as), nil
	},
	NotNull: true,
	Impl: func(args []tenon.Value, rc tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		marks := propagating(args...)
		as := make([]mergeArg, len(args))
		for i, a := range args {
			c, _ := tenon.Unmark(a)
			as[i] = mergeArgOf(c)
		}
		exact := rc.Kind() == tenon.ConstraintExactly
		if exact && rc.Type().Kind() == tenon.KindMap {
			return tenon.WithMarks(mergeMap(rc.Type(), as), marks...), nil
		}
		entries := map[string]tenon.Value{}
		for i := range as {
			a := &as[i]
			if a.open || a.maybe && !exact {
				return tenon.WithMarks(unknownOf(rc), marks...), nil
			}
			for _, e := range a.entries() {
				if a.maybe {
					// It adds this attribute or leaves the one before, of
					// the same type, since the answer's type is settled,
					// and which it is is not known yet.
					v := tenon.Unknown(rc.Type().AttributeType(e.name))
					if before, ok := entries[e.name]; ok {
						v = marked(v, before)
					}
					e.v = v
				}
				entries[e.name] = e.v
			}
		}
		return tenon.WithMarks(tenon.Object(entries), marks...), nil
	},
})
