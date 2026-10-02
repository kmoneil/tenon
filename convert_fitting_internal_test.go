package tenon

import (
	"slices"
	"sync"
)

// The conversion as it was before a container's members were built once: a
// container converts its members, unifies their types, and fits each member
// to the union, so a member is built again at every level above it whose
// element type grows, the value's size times its depth. It is kept as the
// reference the conversion is held to (ConvertBothWays): the same result,
// whatever the value, the constraint and the policy. It shares everything
// but the containers' conversions, which are its own copies.
//
// It makes what a conversion leaves open (CV-021) as values of the types that
// leave it open, which the levels above fit to the type they settle, as they
// fit any member: the conversion instead defers such a member and builds it
// once at that type. A value whose type is still open at the top is no
// result, and where nothing settles it the reference stops there, without
// locating the failure (ConvertBothWays).

// fittingValue converts v, in whatever state, to c. The result carries none of v's
// own marks: whoever asked for the conversion puts on the marks it calls for.
//
// Where v carries a redacting mark, what fails within it fails at v, since a
// path within it and a message naming its type or what it holds would show
// its structure: each code the failures have, once, located at v, with a
// message naming v by the placeholder and c (MK-011, CV-050).
func (x converter) fittingValue(v Value, c Constraint) Value {
	ms := v.n.redactingMarks()
	r := x.fittingValueOf(v, c)
	if ms == nil || r.n.state != stateError {
		return r
	}
	message := redactedText(ms) + " does not convert to " + c.String()
	var diags []Diagnostic
	for _, d := range r.n.diagnostics() {
		if !slices.ContainsFunc(diags, func(e Diagnostic) bool { return e.Code == d.Code }) {
			diags = append(diags, Diagnostic{Code: d.Code, Message: message})
		}
	}
	return Value{n: &node{state: stateError, data: diags, marks: r.n.marks}}
}

// fittingValueOf is value for a value whose failures need not be moved.
func (x converter) fittingValueOf(v Value, c Constraint) Value {
	n := v.n
	switch n.state {
	case statePending:
		// A pending tuple or object holding its members converts as the
		// resolved value of the one type its constraint admits, as the tuple
		// or object it will be to a structure, and otherwise by its
		// constraint, carrying its members' marks where the result holds
		// none of them (CV-032, CV-033).
		if _, ok := n.held(); ok {
			if s, ok := x.memo.soleType(n.constraint()); ok {
				return x.fittingKnown(Resolve(withoutMarks(v), s), c)
			}
			if s, ok := x.heldStructure(c); ok {
				return x.fittingStructure(v, s)
			}
			r := x.pending(v, c)
			if _, kept := r.n.held(); !kept && r.n.state != stateError {
				r = WithMarks(r, heldMarks(n)...)
			}
			return r
		}
		return x.pending(v, c)
	case stateNull, stateUnknown:
		if x.memo.fits(c, n.typ) {
			u := withoutMarks(v)
			return u
		}
		k := keysUnknown
		if n.state == stateNull {
			k = keysNone
		}
		out := typeConvert(n.typ, c, x.policy, k)
		switch {
		case out.fail != nil:
			return errorValue(out.fail.diagnostic())
		case n.state == stateNull:
			return Null(out.typ)
		}
		rd := n.data.(*rangeData)
		if out.pending {
			return pendingValue(c, rd.null)
		}
		return narrowedUnknown(out.typ, rd.null, lengthNarrowings(n.typ, out.typ, rd))
	}
	return x.fittingKnown(v, c)
}

// fittingMember converts a member of a container to c. Converting a member is a
// conversion in its own right, so the result carries the member's Propagate
// marks, as an error result does.
func (x converter) fittingMember(m Value, c Constraint) Value {
	r := x.fittingValue(m, c)
	carried := x.carry(r, m.n)
	if marks, ok := x.brought(r.n); ok {
		x.bring(carried.n, marks)
	}
	return carried
}

// brought holds, for each conversion the reference makes, what a collection
// it made found within its members as it converted them, before fitting them
// to its element type: the redacting marks of the values whose types, as
// their own conversions gave them, name attributes, which a level above takes
// from within that collection (CV-033) however the fitting widened the types
// within it. It is kept apart, by the conversion's memo, since the reference
// adds nothing to the conversion's types.
var brought = struct {
	sync.Mutex
	of map[*convertMemo]map[*node][]Mark
}{of: map[*convertMemo]map[*node][]Mark{}}

// brought returns what the collection n found within its members, and
// whether n is a collection the reference made.
func (x converter) brought(n *node) ([]Mark, bool) {
	brought.Lock()
	defer brought.Unlock()
	marks, ok := brought.of[x.memo][n]
	return marks, ok
}

// bring records what the collection n found within its members.
func (x converter) bring(n *node, marks []Mark) {
	brought.Lock()
	defer brought.Unlock()
	if brought.of[x.memo] == nil {
		brought.of[x.memo] = map[*node][]Mark{}
	}
	brought.of[x.memo][n] = marks
}

// forgetBrought drops what the conversion with memo m recorded.
func forgetBrought(m *convertMemo) {
	brought.Lock()
	defer brought.Unlock()
	delete(brought.of, m)
}

// broughtStructure is redactedStructure as the reference reads it: the
// redacting marks of the values among members, at any depth, that carry one
// and whose type names attributes, taking what a collection within found as
// it made it in place of the types its fitting widened.
func (x converter) broughtStructure(members []Value) []Mark {
	var marks []Mark
	var walk func(n *node)
	walk = func(n *node) {
		if ms := n.redactingMarks(); ms != nil && n.state != stateError {
			if namesAttributes(n.typ) {
				marks, _ = mergeMarks(marks, ms)
			}
			return
		}
		if within, ok := x.brought(n); ok {
			marks, _ = mergeMarks(marks, within)
			return
		}
		if !n.markedWithin {
			return
		}
		switch data := n.data.(type) {
		case []Value:
			for _, m := range data {
				walk(m.n)
			}
		case []mapEntry:
			for _, e := range data {
				walk(e.val.n)
			}
		}
	}
	for _, m := range members {
		walk(m.n)
	}
	return marks
}

// fittingKnown converts a known value, whose content is in hand though a member of it
// may not be known. A constraint that admits exactly one type converts as
// Exactly of that type, however it is written (CV-026), so that is decided
// first, once, as typeConvertKind decides it, and the kind of c decides the
// rest.
func (x converter) fittingKnown(v Value, c Constraint) Value {
	n := v.n
	if x.memo.fits(c, n.typ) {
		u := withoutMarks(v)
		return u
	}
	if n.typ.t.kind != KindCapsule && isStructural(c) {
		return x.fittingStructure(v, c)
	}
	if s, ok := x.memo.soleType(c); ok {
		return x.fittingExactly(v, s)
	}
	if c.c.kind == ConstraintOneOf {
		m, f := oneOfMember(n.typ, x.typeName(n), c, x.policy)
		if f != nil {
			return errorValue(f.diagnostic())
		}
		r := x.fittingKnown(v, m)
		if r.n.state == statePending {
			// The value converts to what the target says, whichever member
			// of it applied.
			return WithMarks(pendingValue(c, r.n.null), r.n.markList()...)
		}
		return r
	}
	if n.typ.t.kind == KindCapsule {
		// A capsule type converts only to a type that it, or the type it
		// converts to, declares.
		return errorValue(noConversion(x.typeName(n), c).diagnostic())
	}
	return x.fittingStructure(v, c)
}

// fittingExactly converts a known value to the type s, as converting to Exactly(s)
// does (CV-020).
func (x converter) fittingExactly(v Value, s Type) Value {
	switch {
	case v.n.typ.t.kind == KindCapsule || s.t.kind == KindCapsule:
		return x.capsule(v, s)
	case isPrimitive(s.t.kind):
		return x.primitive(v, s)
	}
	// The structure of s admits s alone, so converting to it does not ask
	// for its one type again.
	return x.fittingStructure(v, structural(s))
}

// fittingStructure converts a known value to a ListOf, SetOf, MapOf, TupleOf or
// ObjectWith constraint.
func (x converter) fittingStructure(v Value, c Constraint) Value {
	switch c.c.kind {
	case ConstraintListOf, ConstraintSetOf, ConstraintMapOf:
		return x.fittingCollection(v, c)
	case ConstraintTupleOf:
		return x.fittingTuple(v, c)
	case ConstraintObjectWith:
		return x.fittingObject(v, c)
	}
	return errorValue(noConversion(x.typeName(v.n), c).diagnostic())
}

// fittingMembers converts each member to the constraint that at gives for its
// position, reporting the error value that the failed ones make, located by
// their steps, and whether any converted to a pending value.
func (x converter) fittingMembers(h held, at func(i int) Constraint) ([]Value, Value, bool, bool) {
	out := make([]Value, len(h.vals))
	var errs containerErrors
	pending := false
	for i, m := range h.vals {
		r := x.fittingMember(m, at(i))
		switch r.n.state {
		case stateError:
			errs.add(h.step(i), r)
		case statePending:
			pending = true
		}
		out[i] = r
	}
	e, failed := errs.value()
	return out, e, failed, pending
}

// fittingCollection converts a known list, set, tuple, map or object to a ListOf,
// SetOf or MapOf constraint.
func (x converter) fittingCollection(v Value, c Constraint) Value {
	n, d := v.n, c.c
	from := sourceKind(n)
	unsafe := false
	switch {
	case d.kind == ConstraintMapOf && (from == KindMap || from == KindObject):
	case d.kind != ConstraintMapOf && (from == KindList || from == KindSet || from == KindTuple):
		unsafe = d.kind == ConstraintSetOf && from != KindSet
	default:
		return errorValue(noConversion(x.typeName(n), c).diagnostic())
	}
	if unsafe && x.policy == Safe {
		return errorValue(unsafeConversion(x.typeName(n), c).diagnostic())
	}
	h := members(n)
	converted, e, failed, pending := x.fittingMembers(h, func(int) Constraint { return d.elem })
	if failed {
		return e
	}
	withhold := x.typeWithheld(n)
	types := make([]Type, 0, len(converted)+2)
	var least []Type
	for i, r := range converted {
		if r.n.state == statePending {
			// As in collectionTypeConvert: a member whose no-keys conversion
			// fails settles no element type, and is left out rather than
			// contributing the zero Type; so is a member that is pending
			// itself, which has no type to convert.
			if src := h.vals[i].n; src.state != statePending {
				if none := typeConvert(src.typ, d.elem, x.policy, keysNone); none.fail == nil {
					least = append(least, none.typ)
				}
			}
			continue
		}
		types = append(types, r.n.typ)
	}
	if from == KindList || from == KindSet || from == KindMap {
		out := typeConvert(n.typ.t.elem, d.elem, x.policy, keysNone)
		if out.fail != nil {
			return failedReading(n, out.fail.diagnostic())
		}
		types = append(types, out.typ)
	}
	if pending {
		if out := pendingElements(types, least, d.elem, x.policy, withhold); out.fail != nil {
			return failedReading(n, out.fail.diagnostic())
		}
		return pendingCollection(c, n)
	}
	elem, f := elementType(types, d.elem, x.policy, withhold, x.memo)
	if f != nil {
		return failedReading(n, f.diagnostic())
	}
	if from == KindSet && n.partial && d.kind == ConstraintListOf {
		// A set holding members that are not known has no settled order and
		// no settled count, so the list it becomes is not known either.
		low, high := setLengthBounds(n)
		return Narrow(Unknown(ListType(elem)), NotNull(), LengthMin(int64(low)), LengthMax(int64(high)))
	}
	// An element type the members settle holds the attribute names of their
	// object types, and every member is given those attributes, so one taken
	// from a redacted member shows its structure in the result's type and
	// in its siblings: the result carries that member's redacting marks
	// (CV-033). A constraint that settles the type takes nothing from them.
	within := x.broughtStructure(converted)
	var derived []Mark
	if _, fixed := x.memo.resultType(d.elem); withhold && !fixed {
		derived = within
	}
	for i, r := range converted {
		converted[i] = x.fit(r, elem)
	}
	var result Value
	switch d.kind {
	case ConstraintListOf:
		result = List(elem, converted...)
	case ConstraintSetOf:
		result = setOf(SetType(elem), converted)
	default:
		entries := make(map[string]Value, len(converted))
		for i, r := range converted {
			entries[h.names[i]] = r
		}
		result = Map(elem, entries)
	}
	if derived != nil {
		result = WithMarks(result, derived...)
	}
	x.bring(result.n, within)
	return result
}

// fittingTuple converts a known tuple, list or set to a TupleOf constraint.
func (x converter) fittingTuple(v Value, c Constraint) Value {
	n, d := v.n, c.c
	from := sourceKind(n)
	switch from {
	case KindTuple:
		if got, want := len(members(n).vals), len(d.members); got != want {
			return errorValue(Diagnostic{Code: CodeConvertNoConversion,
				Message: "a tuple of " + count(got, "element") + " does not convert to " + c.String() + ", which has " + count(want, "member")})
		}
	case KindList, KindSet:
		want := len(d.members)
		if from == KindSet && n.partial {
			d := x.partialSetTuple(v, c)
			if d.done.n != nil {
				return d.done
			}
			// The unknown tuple of the type it leaves open, which a level
			// above fits to the type it settles.
			return d.later(d.typ)
		}
		if got := len(n.data.([]Value)); got != want {
			return errorValue(Diagnostic{Code: CodeConvertLengthMismatch,
				Message: "a " + kindNoun(from) + " of " + count(got, "member") + " does not convert to " + c.String() + ", which has " + count(want, "member")})
		}
	default:
		return errorValue(noConversion(x.typeName(n), c).diagnostic())
	}
	if from != KindTuple && x.policy == Safe {
		return errorValue(unsafeConversion(x.typeName(n), c).diagnostic())
	}
	converted, e, failed, _ := x.fittingMembers(members(n), func(i int) Constraint { return d.members[i] })
	if failed {
		return e
	}
	// A member converted to a pending value makes it the pending tuple
	// holding them (CV-031).
	return Tuple(converted...)
}

// fittingObject converts a known object or map to an ObjectWith constraint.
func (x converter) fittingObject(v Value, c Constraint) Value {
	n, d := v.n, c.c
	from := sourceKind(n)
	if from != KindObject && from != KindMap {
		return errorValue(noConversion(x.typeName(n), c).diagnostic())
	}
	if from == KindMap && x.policy == Safe {
		return errorValue(unsafeConversion(x.typeName(n), c).diagnostic())
	}
	h := members(n)
	var errs containerErrors
	attrs := make(map[string]Value, len(h.vals))
	fields := d.fields
	missing := func(name string) {
		errs.addDiagnostic(missingAttribute(name).diagnostic())
	}
	for i, name := range h.names {
		for len(fields) > 0 && fields[0].name < name {
			if fields[0].Required {
				missing(fields[0].name)
			}
			if r, ok := x.absentValue(fields[0]); ok {
				attrs[fields[0].name] = r
			}
			fields = fields[1:]
		}
		m := h.vals[i]
		switch {
		case name == "":
			errs.add(h.step(i), errorValue(Diagnostic{Code: CodeObjectEmptyName,
				Message: "the map key " + quoted(name) + " cannot be an attribute name"}))
		case len(fields) == 0 || fields[0].name != name:
			if d.closed {
				f := unexpectedAttribute(name)
				if from == KindMap {
					f.message = "key " + quoted(name) + " is not an attribute the constraint allows"
				}
				errs.add(h.step(i), errorValue(f.diagnostic()))
				continue
			}
			// Carried across unchanged, marks and all.
			attrs[name] = m
		default:
			r := x.fittingMember(m, fields[0].Constraint)
			if r.n.state == stateError {
				errs.add(h.step(i), r)
			}
			attrs[name] = r
			fields = fields[1:]
		}
	}
	for _, f := range fields {
		if f.Required {
			missing(f.name)
		}
		if r, ok := x.absentValue(f); ok {
			attrs[f.name] = r
		}
	}
	if e, failed := errs.value(); failed {
		return e
	}
	// A member converted to a pending value makes it the pending object
	// holding them (CV-031).
	return Object(attrs)
}
