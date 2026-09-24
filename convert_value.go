package tenon

import (
	"slices"
	"strconv"

	"github.com/kmoneil/tenon/internal/decimal"
)

// converter converts values under one policy.
type converter struct {
	policy Policy
	// withheld holds the redacting marks of the values that hold the one
	// being converted. What a value holds is part of what those marks
	// withhold, so a message about it shows a placeholder instead.
	withheld []Mark
}

// within returns the converter for the members of n.
func (x converter) within(n *node) converter {
	if ms := n.redactingMarks(); ms != nil {
		x.withheld, _ = mergeMarks(x.withheld, ms)
	}
	return x
}

// text renders v for a message, as valueText does, and as a placeholder where
// a value holding v carries a redacting mark.
func (x converter) text(v Value) string {
	if x.withheld == nil {
		return valueText(v)
	}
	ms, _ := mergeMarks(x.withheld, v.n.redactingMarks())
	return redactedText(ms)
}

// keyText renders a map key of the map n for a message, as a placeholder
// where the map, or a value holding it, carries a redacting mark.
func (x converter) keyText(n *node, key string) string {
	if ms, _ := mergeMarks(x.withheld, n.redactingMarks()); ms != nil {
		return redactedText(ms)
	}
	return quoted(key)
}

// convertTop converts the operand of a conversion. The framework has settled
// an error operand already, and puts the operand's Propagate marks on the
// result, so the result carries no marks of the operand's own.
func convertTop(v Value, cv conversion) Value {
	return converter{policy: cv.policy}.value(v, cv.target)
}

// value converts v, in whatever state, to c. The result carries none of v's
// own marks: whoever asked for the conversion puts on the marks it calls for.
func (x converter) value(v Value, c Constraint) Value {
	n := v.n
	switch n.state {
	case statePending:
		return x.pending(v, c)
	case stateNull, stateUnknown:
		if fits(c, n.typ) {
			u, _ := Unmark(v)
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
			return NullVal(out.typ)
		}
		rd := n.data.(*rangeData)
		if out.pending {
			return pendingValue(c, rd.null)
		}
		return narrowedUnknown(out.typ, rd.null, lengthNarrowings(n.typ, out.typ, rd))
	}
	return x.known(v, c)
}

// member converts a member of a container to c. Converting a member is a
// conversion in its own right, so the result carries the member's Propagate
// marks, as an error result does.
func (x converter) member(m Value, c Constraint) Value {
	r := x.value(m, c)
	if ms := propagateMarks(m.n); ms != nil {
		r = WithMarks(r, ms...)
	}
	return r
}

// propagateMarks returns the Propagate marks that n carries, or nil.
func propagateMarks(n *node) []Mark {
	var ms []Mark
	for _, m := range n.markList() {
		if m.Propagation() == Propagate {
			ms = append(ms, m)
		}
	}
	return ms
}

// pendingValue returns the pending value with constraint c and the nullness
// fact null.
func pendingValue(c Constraint, null nullness) Value {
	return withNullness(Pending(c), null)
}

// narrowedUnknown returns the unknown value of type t with the nullness fact
// null, narrowed by ns.
func narrowedUnknown(t Type, null nullness, ns []Narrowing) Value {
	return Narrow(withNullness(Unknown(t), null), ns...)
}

// withNullness narrows v, an unknown or a pending value that could still be
// null, by the nullness fact null.
func withNullness(v Value, null nullness) Value {
	switch null {
	case nullNo:
		return Narrow(v, NotNull())
	case nullOnly:
		return Narrow(v, Null())
	}
	return v
}

// lengthNarrowings returns the length bounds that converting an unknown value
// of type from, with range rd, to type to keeps: all of them where the
// conversion keeps every member, the upper one where members may merge in a
// set, and the length a tuple or object type fixes where its members become a
// collection's.
func lengthNarrowings(from, to Type, rd *rangeData) []Narrowing {
	fk, tk := from.t.kind, to.t.kind
	var ns []Narrowing
	switch {
	case fk == KindTuple && tk == KindList:
		n := int64(len(from.t.elems))
		return []Narrowing{LengthMin(n), LengthMax(n)}
	case fk == KindObject && tk == KindMap:
		n := int64(len(from.t.attrs))
		return []Narrowing{LengthMin(n), LengthMax(n)}
	case fk == tk && (fk == KindList || fk == KindMap), fk == KindSet && tk == KindList:
		if rd.lenLo > 0 {
			ns = append(ns, LengthMin(rd.lenLo))
		}
	case tk == KindSet && (fk == KindList || fk == KindSet):
		if rd.lenLo > 0 {
			ns = append(ns, LengthMin(1))
		}
	default:
		return nil
	}
	if rd.lenHi.set {
		ns = append(ns, LengthMax(rd.lenHi.n))
	}
	return ns
}

// pending converts a pending value to c: as the one type its constraint
// admits would convert, where it admits one, and otherwise to the one type c
// admits, or to a pending value. Where c admits no type, nothing converts to
// it, and the answer is an error value whatever the value turns out to be.
func (x converter) pending(v Value, c Constraint) Value {
	n := v.n
	pc := n.data.(Constraint)
	if s, ok := soleType(pc); ok {
		k := keysUnknown
		if n.null == nullOnly {
			k = keysNone
		}
		out := typeConvert(s, c, x.policy, k)
		switch {
		case out.fail != nil:
			return errorValue(Diagnostic{
				Code: CodeOperationWrongType,
				Message: "the operand of Convert is pending with constraint " + pc.String() +
					", and the type it allows does not convert: " + out.fail.message,
			})
		case out.pending:
			return pendingValue(c, n.null)
		}
		return narrowedUnknown(out.typ, n.null, nil)
	}
	if admitsNone(c) {
		// Nothing converts to a constraint that no type satisfies, so no type
		// the value could take does, and a pending value carrying c could
		// never be resolved.
		return errorValue(Diagnostic{
			Code:    CodeOperationWrongType,
			Message: "the operand of Convert is pending with constraint " + pc.String() + ", and no type converts to " + c.String(),
		})
	}
	if t, ok := resultType(c); ok {
		return narrowedUnknown(t, n.null, nil)
	}
	if c.c.kind == ConstraintAny {
		// Whatever type the value takes satisfies Any, so its own constraint
		// says more than Any would.
		u, _ := Unmark(v)
		return u
	}
	return pendingValue(c, n.null)
}

// known converts a known value, whose content is in hand though a member of it
// may not be known. A constraint that admits exactly one type converts as
// Exactly of that type, however it is written (CV-026), so that is decided
// first, once, as typeConvertKind decides it, and the kind of c decides the
// rest.
func (x converter) known(v Value, c Constraint) Value {
	n := v.n
	if fits(c, n.typ) {
		u, _ := Unmark(v)
		return u
	}
	if n.typ.t.kind != KindCapsule && isStructural(c) {
		return x.structure(v, c)
	}
	if s, ok := soleType(c); ok {
		return x.exactly(v, s)
	}
	if c.c.kind == ConstraintOneOf {
		m, f := oneOfMember(n.typ, c, x.policy)
		if f != nil {
			return errorValue(f.diagnostic())
		}
		r := x.known(v, m)
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
		return errorValue(noConversion(n.typ, c).diagnostic())
	}
	return x.structure(v, c)
}

// exactly converts a known value to the type s, as converting to Exactly(s)
// does (CV-020).
func (x converter) exactly(v Value, s Type) Value {
	switch {
	case v.n.typ.t.kind == KindCapsule || s.t.kind == KindCapsule:
		return x.capsule(v, s)
	case isPrimitive(s.t.kind):
		return x.primitive(v, s)
	}
	// The structure of s admits s alone, so converting to it does not ask
	// for its one type again.
	return x.structure(v, structural(s))
}

// structure converts a known value to a ListOf, SetOf, MapOf, TupleOf or
// ObjectWith constraint.
func (x converter) structure(v Value, c Constraint) Value {
	switch c.c.kind {
	case ConstraintListOf, ConstraintSetOf, ConstraintMapOf:
		return x.collection(v, c)
	case ConstraintTupleOf:
		return x.tuple(v, c)
	case ConstraintObjectWith:
		return x.object(v, c)
	}
	return errorValue(noConversion(v.n.typ, c).diagnostic())
}

// primitive converts a known value to the primitive type s.
func (x converter) primitive(v Value, s Type) Value {
	n := v.n
	if out := primitiveTypeConvert(n.typ, s, x.policy); out.fail != nil {
		return errorValue(out.fail.diagnostic())
	}
	switch n.typ.t.kind {
	case KindNumber:
		return String(n.data.(decimal.Dec).String())
	case KindBool:
		return String(strconv.FormatBool(n.data.(bool)))
	}
	text := n.data.(string)
	if s.t.kind == KindBool {
		switch text {
		case "true":
			return Bool(true)
		case "false":
			return Bool(false)
		}
		return errorValue(Diagnostic{Code: CodeBoolInvalidSyntax, Message: x.text(v) + ` is neither "true" nor "false"`})
	}
	d, err := decimal.Parse(text)
	if err == nil {
		return numberValue(d)
	}
	switch code := numberCode(err.(decimal.Error)); code {
	case CodeNumberInvalidSyntax:
		return errorValue(Diagnostic{Code: code, Message: x.text(v) + " is not a number"})
	case CodeNumberOutOfRange:
		return errorValue(Diagnostic{Code: code, Message: x.text(v) + " is outside the range of numbers"})
	case CodeNumberTooLong:
		return errorValue(tooLong(len(text)))
	}
	internalPanic("Parse reported %v, which it does not report", err)
	return Value{}
}

// capsule converts a known value between its type and the type s, where
// either is a capsule type, by the conversion the capsule type declares.
func (x converter) capsule(v Value, s Type) Value {
	n := v.n
	if out := capsuleTypeConvert(n.typ, s, x.policy); out.fail != nil {
		return errorValue(out.fail.diagnostic())
	}
	// The declared conversion reads what the value holds, and the capsule
	// value it makes holds none of it, so the result carries the Propagate
	// marks of the values within.
	held := heldMarks(n)
	if n.partial {
		// A container holding a member that is not known is not a value the
		// declared function can take; what it will convert to is not known
		// either.
		return WithMarks(Narrow(Unknown(s), NotNull()), held...)
	}
	to, from, _, _ := capsuleConversion(n.typ, s)
	var r Value
	if to != nil {
		r = to(n.data)
	} else {
		r = from(v)
	}
	if r.n == nil || r.n.state != stateError && (!r.n.isKnown() || r.n.typ != s) {
		usagePanic("a conversion that capsule type %s declares from %s to %s returned %s, not a known value of %s or an error value",
			capsuleName(n.typ, s), n.typ, s, r, s)
	}
	return WithMarks(r, held...)
}

// capsuleName names the capsule type that declared a conversion between t
// and s.
func capsuleName(t, s Type) string {
	if t.t.kind == KindCapsule {
		return quotedText(t.t.capsule.name)
	}
	return quotedText(s.t.capsule.name)
}

// held is the members of a container, in the order a conversion takes them,
// with the step that locates each and the name each has where it has one.
type held struct {
	vals  []Value
	steps []Step
	names []string // map keys or attribute names; nil for other containers
}

// members returns what the known container n holds. A set's members come out
// as Elements gives them, carrying the set's deep marks.
func members(n *node) held {
	var h held
	switch n.typ.t.kind {
	case KindMap:
		for _, e := range n.data.([]mapEntry) {
			h.vals = append(h.vals, e.val)
			h.steps = append(h.steps, indexStep(String(e.key)))
			h.names = append(h.names, e.key)
		}
		return h
	case KindObject:
		h.vals = n.data.([]Value)
		for _, a := range n.typ.t.attrs {
			h.steps = append(h.steps, attributeStep(a.name))
			h.names = append(h.names, a.name)
		}
		return h
	case KindSet:
		h.vals = n.retrievedMembers()
	default:
		h.vals = n.data.([]Value)
	}
	for i := range h.vals {
		h.steps = append(h.steps, indexStep(NumberFromInt(int64(i))))
	}
	return h
}

// convertMembers converts each member to the constraint that at gives for its
// position, reporting the error value that the failed ones make, located by
// their steps, and whether any converted to a pending value.
func (x converter) convertMembers(h held, at func(i int) Constraint) ([]Value, Value, bool, bool) {
	out := make([]Value, len(h.vals))
	var errs containerErrors
	pending := false
	for i, m := range h.vals {
		r := x.member(m, at(i))
		switch r.n.state {
		case stateError:
			errs.add(h.steps[i], r)
		case statePending:
			pending = true
		}
		out[i] = r
	}
	e, failed := errs.value()
	return out, e, failed, pending
}

// collection converts a known list, set, tuple, map or object to a ListOf,
// SetOf or MapOf constraint.
func (x converter) collection(v Value, c Constraint) Value {
	n, d := v.n, c.c
	from := n.typ.t.kind
	unsafe := false
	switch {
	case d.kind == ConstraintMapOf && (from == KindMap || from == KindObject):
	case d.kind != ConstraintMapOf && (from == KindList || from == KindSet || from == KindTuple):
		unsafe = d.kind == ConstraintSetOf && from != KindSet
	default:
		return errorValue(noConversion(n.typ, c).diagnostic())
	}
	h := members(n)
	converted, e, failed, pending := x.within(n).convertMembers(h, func(int) Constraint { return d.elem })
	switch {
	case failed:
		return e
	case unsafe && x.policy == Safe:
		return errorValue(unsafeConversion(n.typ, c).diagnostic())
	}
	withhold := x.within(n).withheld != nil || holdsRedacting(n)
	types := make([]Type, 0, len(converted)+2)
	var least []Type
	for i, r := range converted {
		if r.n.state == statePending {
			// As in collectionTypeConvert: a member whose no-keys conversion
			// fails settles no element type, and is left out rather than
			// contributing the zero Type.
			if none := typeConvert(h.vals[i].n.typ, d.elem, x.policy, keysNone); none.fail == nil {
				least = append(least, none.typ)
			}
			continue
		}
		types = append(types, r.n.typ)
	}
	if from == KindList || from == KindSet || from == KindMap {
		out := typeConvert(n.typ.t.elem, d.elem, x.policy, keysNone)
		if out.fail != nil {
			return errorValue(out.fail.diagnostic())
		}
		types = append(types, out.typ)
	}
	if pending {
		if out := pendingElements(types, least, d.elem, x.policy, withhold); out.fail != nil {
			return errorValue(out.fail.diagnostic())
		}
		return pendingContainer(c, n)
	}
	elem, f := elementType(types, d.elem, x.policy, withhold)
	if f != nil {
		return errorValue(f.diagnostic())
	}
	if from == KindSet && n.partial && d.kind == ConstraintListOf {
		// A set holding members that are not known has no settled order and
		// no settled count, so the list it becomes is not known either.
		low, high := setLengthBounds(n)
		return Narrow(Unknown(List(elem)), NotNull(), LengthMin(int64(low)), LengthMax(int64(high)))
	}
	for i, r := range converted {
		converted[i] = x.fit(r, elem)
	}
	switch d.kind {
	case ConstraintListOf:
		return ListVal(elem, converted...)
	case ConstraintSetOf:
		return setOf(elem, converted)
	}
	entries := make(map[string]Value, len(converted))
	for i, r := range converted {
		entries[h.names[i]] = r
	}
	return MapVal(elem, entries)
}

// setOf returns the set of these members, which give their marks, at every
// depth, to the set, since a set's members carry none.
func setOf(elem Type, members []Value) Value {
	var marks []Mark
	unmarked := make([]Value, len(members))
	for i, m := range members {
		var taken []Mark
		unmarked[i], taken = UnmarkDeep(m)
		marks = append(marks, taken...)
	}
	return WithMarks(SetVal(elem, unmarked...), marks...)
}

// tuple converts a known tuple, list or set to a TupleOf constraint.
func (x converter) tuple(v Value, c Constraint) Value {
	n, d := v.n, c.c
	from := n.typ.t.kind
	switch from {
	case KindTuple:
		if len(n.typ.t.elems) != len(d.members) {
			return errorValue(tupleTypeConvert(n.typ, c, x.policy, keysNone).fail.diagnostic())
		}
	case KindList, KindSet:
		want := len(d.members)
		if from == KindSet && n.partial {
			return x.partialSetTuple(v, c)
		}
		if got := len(n.data.([]Value)); got != want {
			message := "a " + kindNoun(from) + " of " + count(got, "member")
			if x.within(n).withheld != nil {
				message = "the " + kindNoun(from)
			}
			return errorValue(Diagnostic{Code: CodeConvertLengthMismatch,
				Message: message + " does not convert to " + c.String() + ", which has " + count(want, "member")})
		}
	default:
		return errorValue(noConversion(n.typ, c).diagnostic())
	}
	converted, e, failed, pending := x.within(n).convertMembers(members(n), func(i int) Constraint { return d.members[i] })
	switch {
	case failed:
		return e
	case from != KindTuple && x.policy == Safe:
		return errorValue(unsafeConversion(n.typ, c).diagnostic())
	case pending:
		return pendingContainer(c, n)
	}
	return TupleVal(converted...)
}

// partialSetTuple converts a set holding members that are not known to a
// TupleOf constraint. Its members have no settled order, so the tuple is not
// known, and where the number of members it could have rules out the tuple's
// length the conversion fails already.
func (x converter) partialSetTuple(v Value, c Constraint) Value {
	n, d := v.n, c.c
	low, high := setLengthBounds(n)
	if want := len(d.members); want < low || want > high {
		message := "a set of " + strconv.Itoa(low) + " to " + count(high, "member")
		if x.within(n).withheld != nil {
			message = "the set"
		}
		return errorValue(Diagnostic{Code: CodeConvertLengthMismatch,
			Message: message + " does not convert to " + c.String() + ", which has " + count(want, "member")})
	}
	out := typeConvert(n.typ, c, x.policy, keysUnknown)
	switch {
	case out.fail != nil:
		return errorValue(out.fail.diagnostic())
	case out.pending:
		return Narrow(Pending(c), NotNull())
	}
	return Narrow(Unknown(out.typ), NotNull())
}

// kindNoun names a kind of container in a message, as in "list".
func kindNoun(k Kind) string {
	switch k {
	case KindList:
		return "list"
	case KindSet:
		return "set"
	case KindMap:
		return "map"
	case KindTuple:
		return "tuple"
	}
	return "object"
}

// object converts a known object or map to an ObjectWith constraint.
func (x converter) object(v Value, c Constraint) Value {
	n, d := v.n, c.c
	from := n.typ.t.kind
	if from != KindObject && from != KindMap {
		return errorValue(noConversion(n.typ, c).diagnostic())
	}
	h := members(n)
	inner := x.within(n)
	var errs containerErrors
	attrs := make(map[string]Value, len(h.vals))
	pending := false
	fields := d.fields
	missing := func(name string) {
		errs.addDiagnostic(missingAttribute(name).diagnostic())
	}
	for i, name := range h.names {
		for len(fields) > 0 && fields[0].name < name {
			if fields[0].Required {
				missing(fields[0].name)
			}
			addNullValue(attrs, fields[0])
			fields = fields[1:]
		}
		m := h.vals[i]
		switch {
		case name == "":
			errs.add(h.steps[i], errorValue(Diagnostic{Code: CodeConvertUnexpectedAttribute,
				Message: "the map key " + x.keyText(n, name) + " cannot be an attribute name"}))
		case len(fields) == 0 || fields[0].name != name:
			if d.closed {
				f := unexpectedAttribute(name)
				if from == KindMap {
					f.message = "key " + x.keyText(n, name) + " is not an attribute the constraint allows"
				}
				errs.add(h.steps[i], errorValue(f.diagnostic()))
				continue
			}
			// Carried across unchanged, marks and all.
			attrs[name] = m
		default:
			r := inner.member(m, fields[0].Constraint)
			switch r.n.state {
			case stateError:
				errs.add(h.steps[i], r)
			case statePending:
				pending = true
			}
			attrs[name] = r
			fields = fields[1:]
		}
	}
	for _, f := range fields {
		if f.Required {
			missing(f.name)
		}
		addNullValue(attrs, f)
	}
	if e, failed := errs.value(); failed {
		return e
	}
	switch {
	case from == KindMap && x.policy == Safe:
		return errorValue(unsafeConversion(n.typ, c).diagnostic())
	case pending:
		return pendingContainer(c, n)
	}
	return ObjectVal(attrs)
}

// fit returns m, a member already converted to a collection's element
// constraint, as a value of the element type e, which unification made from
// the member's type and others. Where the types differ, a primitive becomes a
// string, a structure takes the shape e gives it, and an object gains the null
// of each attribute of e that it lacks. What is rebuilt is converted, and
// carries its Propagate marks, as a member converted does; what already has
// its type is carried across with every mark.
func (x converter) fit(m Value, e Type) Value {
	n := m.n
	if n.typ == e {
		return m
	}
	var r Value
	switch n.state {
	case stateNull:
		r = NullVal(e)
	case stateUnknown:
		rd := n.data.(*rangeData)
		r = narrowedUnknown(e, rd.null, lengthNarrowings(n.typ, e, rd))
	default:
		r = x.fitKnown(m, e)
	}
	if ms := propagateMarks(n); ms != nil {
		r = WithMarks(r, ms...)
	}
	return r
}

// fitKnown is fit for a known value.
func (x converter) fitKnown(m Value, e Type) Value {
	n, to := m.n, e.t
	if isPrimitive(to.kind) {
		return x.primitive(m, e)
	}
	h := members(n)
	fitted := make([]Value, len(h.vals))
	switch to.kind {
	case KindList, KindSet, KindMap:
		if n.typ.t.kind == KindSet && n.partial && to.kind == KindList {
			low, high := setLengthBounds(n)
			return Narrow(Unknown(e), NotNull(), LengthMin(int64(low)), LengthMax(int64(high)))
		}
		vals := h.vals
		if n.typ.t.kind == KindSet && to.kind == KindSet {
			// The set's own marks, deep ones included, stay on the set, whose
			// members carry none.
			vals = n.data.([]Value)
		}
		for i, v := range vals {
			fitted[i] = x.fit(v, to.elem)
		}
		switch to.kind {
		case KindList:
			return ListVal(to.elem, fitted...)
		case KindSet:
			return SetVal(to.elem, fitted...)
		}
		entries := make(map[string]Value, len(fitted))
		for i, v := range fitted {
			entries[h.names[i]] = v
		}
		return MapVal(to.elem, entries)
	case KindTuple:
		for i, v := range h.vals {
			fitted[i] = x.fit(v, to.elems[i])
		}
		return TupleVal(fitted...)
	case KindObject:
		// The type names the attributes and their order, and the value's own
		// attributes are in that order too, both being sorted by name, so
		// the two are walked together and the value is built by the type.
		// Gathering a map of every attribute instead, and interning the type
		// it describes, would cost each of n objects fitted to a type of n
		// attributes another copy of a type they all share.
		vals := make([]Value, len(to.attrs))
		own := 0
		for i, a := range to.attrs {
			for own < len(h.names) && h.names[own] < a.name {
				own++
			}
			if own < len(h.names) && h.names[own] == a.name {
				vals[i] = x.fit(h.vals[own], a.typ)
				own++
				continue
			}
			vals[i] = NullVal(a.typ)
		}
		return objectOf(e, vals)
	}
	internalPanic("fit called with %s for %s", e, n.describe())
	return Value{}
}

// pendingContainer returns the pending value that a known container converts
// to when a member of it converts to a pending value, which no container can
// hold. The conversion read the values within the container, and the result
// holds none of them, so it carries their Propagate marks.
func pendingContainer(c Constraint, n *node) Value {
	return WithMarks(Narrow(Pending(c), NotNull()), heldMarks(n)...)
}

// heldMarks returns the Propagate marks that the values within n carry, at any
// depth, each once, in the order met.
func heldMarks(n *node) []Mark {
	var ms []Mark
	var walk func(n *node)
	walk = func(n *node) {
		if !n.markedWithin {
			return
		}
		visit := func(m *node) {
			for _, mark := range propagateMarks(m) {
				if !slices.Contains(ms, mark) {
					ms = append(ms, mark)
				}
			}
			walk(m)
		}
		switch data := n.data.(type) {
		case []Value:
			for _, m := range data {
				visit(m.n)
			}
		case []mapEntry:
			for _, e := range data {
				visit(e.val.n)
			}
		}
	}
	walk(n)
	return ms
}

// holdsRedacting reports whether a value within n, at any depth, carries a
// redacting mark.
func holdsRedacting(n *node) bool {
	if !n.markedWithin {
		return false
	}
	switch data := n.data.(type) {
	case []Value:
		for _, m := range data {
			if m.n.redactingMarks() != nil || holdsRedacting(m.n) {
				return true
			}
		}
	case []mapEntry:
		for _, e := range data {
			if e.val.n.redactingMarks() != nil || holdsRedacting(e.val.n) {
				return true
			}
		}
	}
	return false
}

// addNullValue adds to attrs the attribute that an absent optional field f
// adds, where its constraint gives a type: the null of that type.
func addNullValue(attrs map[string]Value, f field) {
	if t, ok := resultType(f.Constraint); ok {
		attrs[f.name] = NullVal(t)
	}
}
