package tenon

import (
	"cmp"
	"maps"
	"slices"
	"strconv"

	"github.com/kmoneil/tenon/internal/decimal"
)

// converter converts values under one policy.
type converter struct {
	policy Policy
	// carried is what the conversion keeps to carry members' marks, shared
	// by every converter of one conversion.
	carried *carrying
	// memo is what the conversion remembers of the constraints and values it
	// meets, shared by every converter of one conversion.
	memo *convertMemo
}

// convertTop converts the operand of a conversion. The framework has settled
// an error operand already, and puts the operand's Propagate marks on the
// result, so the result carries no marks of the operand's own.
func convertTop(v Value, cv conversion) Value {
	return converter{policy: cv.policy, carried: &carrying{}, memo: &convertMemo{}}.value(v, cv.target)
}

// convertMemo is what one conversion remembers of what it meets. A conversion
// asks of each member it converts whether the member's type fits the member's
// constraint, and what that constraint admits and gives, and it asks of each
// container whether a value within it carries a redacting mark. Each answer
// walked the rest of the constraint, or the marked values beneath the
// container, and asking again at every level cost the value's size times its
// depth. Remembered, each constraint, type and marked value is walked once.
type convertMemo struct {
	fit       map[typeAsked]bool
	sat       map[typeAsked]bool
	canon     *canonMemo
	given     map[*constraintData]soleResult
	redacting map[*node]bool
	isolating map[*node]bool
}

// typeAsked is a constraint and a type, as fits and Satisfies are asked of
// them.
type typeAsked struct {
	c *constraintData
	t *typeData
}

// fits is fits, remembered where m is not nil.
func (m *convertMemo) fits(c Constraint, t Type) bool {
	if m == nil {
		return fits(c, t)
	}
	k := typeAsked{c.c, t.t}
	if r, ok := m.fit[k]; ok {
		return r
	}
	r := fits(c, t)
	if m.fit == nil {
		m.fit = map[typeAsked]bool{}
	}
	m.fit[k] = r
	return r
}

// satisfies is Satisfies, remembered where m is not nil.
func (m *convertMemo) satisfies(c Constraint, t Type) bool {
	if m == nil {
		return Satisfies(c, t)
	}
	k := typeAsked{c.c, t.t}
	if r, ok := m.sat[k]; ok {
		return r
	}
	r := Satisfies(c, t)
	if m.sat == nil {
		m.sat = map[typeAsked]bool{}
	}
	m.sat[k] = r
	return r
}

// soleType is soleType, remembered where m is not nil.
func (m *convertMemo) soleType(c Constraint) (Type, bool) {
	if m == nil {
		return soleType(c)
	}
	if m.canon == nil {
		m.canon = newCanonMemo()
	}
	return m.canon.soleType(c)
}

// resultType is resultType, remembered where m is not nil.
func (m *convertMemo) resultType(c Constraint) (Type, bool) {
	if m == nil {
		return resultType(c)
	}
	if r, ok := m.given[c.c]; ok {
		return r.t, r.ok
	}
	t, ok := resultType(c)
	if m.given == nil {
		m.given = map[*constraintData]soleResult{}
	}
	m.given[c.c] = soleResult{t, ok}
	return t, ok
}

// value converts v, in whatever state, to c. The result carries none of v's
// own marks: whoever asked for the conversion puts on the marks it calls for.
func (x converter) value(v Value, c Constraint) Value {
	d := x.draft(v, c)
	switch {
	case d.done.n != nil:
		return d.done
	case d.typ.t.open:
		// Nothing is above the value to settle what its conversion leaves
		// open (CV-021).
		return x.openFailure(d, v.n, d.typ)
	}
	return x.build(d, d.typ)
}

// draft is a conversion worked out and not yet built. A conversion to a
// collection unifies its members' types into the collection's element type,
// and builds each member at that type; built as it was converted, a member was
// built again at every level above it whose element type grew, which cost the
// value's size times its depth. So a container's conversion is worked out
// first, to its type and its members' drafts, and built once, at the type the
// levels above it settle (build). The draft of any other conversion holds its
// result, which building fits to the type it is built at, as a member of a
// collection is fitted to the collection's element type.
type draft struct {
	// done is the result of a conversion that builds no container here,
	// fails, or is pending.
	done Value
	// typ is the type a container's conversion gives, where done is zero,
	// and parts are what it is built from, or the type a deferred one gives
	// (parts.later). Most drafts are of members that build no container, so
	// what only a container has is held apart.
	typ Type
	*parts
}

// parts are what a container's conversion is built from.
type parts struct {
	// members are the drafts of its members, in the order it holds them, and
	// names are the keys of a map's members or the names of an object's.
	members []memberDraft
	names   []string
	// marks are what a collection takes from its members: the redacting marks
	// of those that name its element type's attributes (CV-033).
	marks []Mark
	// withheld is what redactedStructure finds within the container once
	// built: the redacting marks of the values within it whose types name
	// attributes.
	withheld []Mark

	// What a conversion whose type leaves a part open (CV-021) has besides.
	// open is the failure it gives where nothing above settles that part, set
	// where it left the part open itself, holding no member that did. later
	// makes its value at the type the levels above settle, where it builds
	// no container: a null or unknown value, or a set holding members that
	// are not known.
	// redactedTo is the constraint that a value carrying a redacting mark
	// was converted to, which a failure found within it names (MK-011).
	open       *failure
	later      func(Type) Value
	redactedTo Constraint
	// src is the container a set is made of, which says whether it is made
	// at its own type before a level above widens it (madeAtItsOwnType).
	src *node
}

// deferred returns the draft of a conversion that builds no container and
// leaves a part of its type t open: later makes its value at t once the
// levels above have settled that part, and open is its failure where they do
// not.
func deferred(t Type, open *failure, later func(Type) Value) draft {
	return draft{typ: t, parts: &parts{open: open, later: later}}
}

// memberDraft is the draft of a member's conversion, with the member, whose
// Propagate marks what it converts to carries (CV-033). from is nil for what a
// conversion puts in an object without converting it: an attribute carried
// across unchanged, and the null of an absent optional one.
type memberDraft struct {
	draft
	from *node
}

// finished returns the draft of a conversion whose result is r.
func finished(r Value) draft { return draft{done: r} }

// typeOf returns the type that d's conversion gives.
func (d draft) typeOf() Type {
	if d.done.n != nil {
		return d.done.n.typ
	}
	return d.typ
}

// draft works out the conversion of v to c, building no container.
//
// Where v carries a redacting mark, what fails within it fails at v, since a
// path within it and a message naming its type or what it holds would show
// its structure: each code the failures have, once, located at v, with a
// message naming v by the placeholder and c (MK-011, CV-050).
func (x converter) draft(v Value, c Constraint) draft {
	ms := v.n.redactingMarks()
	d := x.draftOf(v, c)
	switch {
	case ms == nil:
		return d
	case d.done.n == nil:
		if d.typ.t.open {
			// What its conversion leaves open fails within it, where nothing
			// settles it, and is moved to it then (openFailure).
			d.redactedTo = c
		}
		return d
	case d.done.n.state != stateError:
		return d
	}
	return finished(redactedFailure(d.done, ms, c))
}

// redactedFailure returns the failure r of a value carrying the redacting
// marks ms, converted to c, as draft moves it to that value: each code r has,
// once, with a message naming the value by the placeholder and c.
func redactedFailure(r Value, ms []Mark, c Constraint) Value {
	message := redactedText(ms) + " does not convert to " + c.String()
	var diags []Diagnostic
	for _, dg := range r.n.diagnostics() {
		if !slices.ContainsFunc(diags, func(e Diagnostic) bool { return e.Code == dg.Code }) {
			diags = append(diags, Diagnostic{Code: dg.Code, Message: message})
		}
	}
	e := errorValue(diags...)
	e.n.marks = r.n.marks
	return e
}

// draftOf is draft for a value whose failures need not be moved.
func (x converter) draftOf(v Value, c Constraint) draft {
	if v.n.state == stateKnown {
		return x.known(v, c)
	}
	return x.unsettled(v, c)
}

// unsettled works out the conversion of a pending, null or unknown value.
func (x converter) unsettled(v Value, c Constraint) draft {
	n := v.n
	if n.state == statePending {
		return finished(x.pending(v, c))
	}
	if x.memo.fits(c, n.typ) {
		u := withoutMarks(v)
		return finished(u)
	}
	k := keysUnknown
	if n.state == stateNull {
		k = keysNone
	}
	out := typeConvert(n.typ, c, x.policy, k)
	switch {
	case out.fail != nil:
		return finished(errorValue(out.fail.diagnostic()))
	case out.pending:
		return finished(pendingValue(c, n.data.(*rangeData).null))
	case out.open != nil:
		return deferred(out.typ, out.open, func(t Type) Value { return resolvedAs(n, t) })
	}
	return finished(resolvedAs(n, out.typ))
}

// resolvedAs returns the null or unknown value of type t that the null or
// unknown value n converts to, an unknown value keeping its nullness and the
// lengths it is narrowed to.
func resolvedAs(n *node, t Type) Value {
	if n.state == stateNull {
		return Null(t)
	}
	rd := n.data.(*rangeData)
	return narrowedUnknown(t, rd.null, lengthNarrowings(n.typ, t, rd))
}

// build builds what d works out as a value of type t: the type d gives, or
// one that the levels above it settled, which unification made from d's type
// and others'. A container is built at t, each member at the part of t that
// holds it, so that nothing is built at a type a level above would widen, and
// the result of any other conversion is fitted to t.
func (x converter) build(d draft, t Type) Value {
	if d.done.n == nil && d.src != nil && t.t.kind == KindSet {
		if own := fillOpen(d.typ, t); own != t && x.madeAtItsOwnType(d, own.t.elem) {
			return x.fit(x.build(d, own), t)
		}
	}
	switch {
	case d.done.n != nil:
		return x.fit(d.done, t)
	case d.later != nil:
		return x.fit(d.later(fillOpen(d.typ, t)), t)
	case t.t.kind != d.typ.t.kind, t.t.kind == KindTuple && len(t.t.elems) != len(d.members):
		// A union of another shape, as a list is of a tuple: built at the
		// type the conversion gives, its open parts taking what t has in
		// their places, then fitted.
		return x.fit(x.build(d, fillOpen(d.typ, t)), t)
	case t.t.kind == KindObject:
		return x.buildObject(d, t)
	}
	vals := make([]Value, len(d.members))
	for i, md := range d.members {
		part := t.t.elem
		if t.t.kind == KindTuple {
			part = t.t.elems[i]
		}
		vals[i] = x.buildMember(md, part)
	}
	// The type is in hand, so the list, set or tuple is made of it, not of
	// a type found again for it.
	var r Value
	switch t.t.kind {
	case KindList:
		r = sequenceValue(t, "List", vals)
	case KindSet:
		r = setOf(t, vals)
	case KindMap:
		entries := make(map[string]Value, len(vals))
		for i, v := range vals {
			entries[d.names[i]] = v
		}
		r = Map(t.t.elem, entries)
	default:
		r = tupleOf(t, vals)
	}
	if d.marks != nil {
		r = WithMarks(r, d.marks...)
	}
	return r
}

// leavesOpen reports whether what md works out leaves a part of its type open
// (CV-021), for the levels above it to settle.
func leavesOpen(md memberDraft) bool { return md.done.n == nil && md.typ.t.open }

// openFailure returns the error value of the conversion that d works out for
// the value n, where t, the type at n's place in the result, is still open and
// nothing is above to settle it (CV-021): a diagnostic located at each
// innermost value within n whose conversion leaves a part of t open, as the
// failure of a member is located (CV-050), and moved to a value carrying a
// redacting mark where it is within one.
func (x converter) openFailure(d draft, n *node, t Type) Value {
	var r Value
	if d.open != nil {
		r = failedReading(n, d.open.diagnostic())
	} else {
		h := held{names: d.names, kind: n.typ.t.kind}
		var errs containerErrors
		for i, md := range d.members {
			name := ""
			if d.names != nil {
				name = d.names[i]
			}
			if part := placeOf(t, i, name); leavesOpen(md) && part.t.open {
				errs.add(h.step(i), x.carry(x.openFailure(md.draft, md.from, part), md.from))
			}
		}
		var found bool
		if r, found = errs.value(); !found {
			internalPanic("openFailure found nothing open within %s", n.describe())
		}
	}
	if ms := n.redactingMarks(); ms != nil {
		r = redactedFailure(r, ms, d.redactedTo)
	}
	return r
}

// buildObject builds the object that d works out as a value of the object type
// t, which holds each of its attributes and may hold more: an attribute it
// lacks it holds as the null of its type, as fitting it to a union does. The
// names of d's attributes and of t's are in one order, so the two are walked
// together.
func (x converter) buildObject(d draft, t Type) Value {
	attrs := t.t.attrs
	vals := make([]Value, len(attrs))
	own := 0
	for i, a := range attrs {
		for own < len(d.names) && d.names[own] < a.name {
			own++
		}
		if own < len(d.names) && d.names[own] == a.name {
			vals[i] = x.buildMember(d.members[own], a.typ)
			own++
			continue
		}
		vals[i] = Null(a.typ)
	}
	return objectOf(t, vals)
}

// buildMember builds what the member md converts to as a value of type t,
// carrying the member's Propagate marks (CV-033). What builds no container here
// is carried and then fitted, as a member converted and then fitted to a
// collection's element type is.
func (x converter) buildMember(md memberDraft, t Type) Value {
	if r := md.done; r.n != nil {
		if md.from != nil {
			r = x.carry(r, md.from)
		}
		return x.fit(r, t)
	}
	r := x.build(md.draft, t)
	if md.from != nil {
		r = x.carry(r, md.from)
	}
	return r
}

// withheldWithin returns what redactedStructure finds in what the members
// drafts convert to, without building them.
func withheldWithin(drafts []memberDraft) []Mark {
	var marks []Mark
	for _, md := range drafts {
		marks, _ = mergeMarks(marks, md.withheldIn())
	}
	return marks
}

// withheldIn returns what redactedStructure finds in what md converts to: the
// redacting marks it carries, where its type names attributes, and otherwise
// those of the values within it. What it carries are its member's, which
// carrying puts on it, and those its own result carries.
func (md memberDraft) withheldIn() []Mark {
	var own []Mark
	if md.from != nil {
		own = md.from.redactingMarks()
	}
	if r := md.done; r.n != nil {
		if r.n.state == stateError {
			return nil
		}
		own, _ = mergeMarks(own, r.n.redactingMarks())
	} else {
		own, _ = mergeMarks(own, md.marks)
	}
	switch {
	case own != nil && namesAttributes(md.typeOf()):
		return own
	case own != nil:
		return nil
	case md.done.n != nil:
		return redactedStructure([]Value{md.done})
	}
	return md.withheld
}

// carrying carries the marks of a conversion's members to what they convert
// to (CV-033) in proportion to the members, not to the marks each holds. A
// member under a container's k deep marks holds them as a layer its siblings
// share (attachment), and giving each converted member its k marks anew,
// then attaching the deep ones within it again, cost k by k: a list of 2,000
// members under 2,000 deep marks allocated 1.6 GB to convert. So what a
// member carries becomes its Propagate part, which shares the member's
// layers, and the deep marks within it are attached by one attachment per
// layer, which stops at every value holding that layer already, as every
// value converted from within the member does.
type carrying struct {
	parts       map[*markSet]*markSet    // the Propagate part of each set met
	attachments map[*markSet]*attachment // the attachment of each layer met
}

// part returns the Propagate marks of s as a set sharing the layers of s's
// own part, or nil where there are none. The part of a set holding only
// Propagate marks is the set itself.
func (c *carrying) part(s *markSet) *markSet {
	if s == nil {
		return nil
	}
	if p, ok := c.parts[s]; ok {
		return p
	}
	outer := c.part(s.outer)
	list := s.list
	if i := slices.IndexFunc(list, isolates); i >= 0 {
		list = slices.Clone(list[:i])
		for _, m := range s.list[i+1:] {
			if !isolates(m) {
				list = append(list, m)
			}
		}
	}
	p := s
	switch {
	case len(list) == 0:
		p = outer
	case len(list) != len(s.list) || outer != s.outer:
		p = &markSet{list: list, outer: outer, layer: s.layer}
	}
	if c.parts == nil {
		c.parts = map[*markSet]*markSet{}
	}
	c.parts[s] = p
	return p
}

// isolates reports whether m is a mark that no operation carries.
func isolates(m Mark) bool { return !propagates(m) }

// attachment returns the attachment of layer, the same for every member it
// is asked for, so that what one member's attaching learns serves the rest.
func (c *carrying) attachment(layer *markSet) *attachment {
	a, ok := c.attachments[layer]
	if !ok {
		a = &attachment{layer: layer}
		if c.attachments == nil {
			c.attachments = map[*markSet]*attachment{}
		}
		c.attachments[layer] = a
	}
	return a
}

// carry returns r, what a member or a fitted value converted to, carrying
// the Propagate marks that from carries: what WithMarks(r,
// propagateMarks(from)...) returns. r carries no marks of its own where the
// conversion made it, and then it takes from's Propagate part, sharing its
// layers, and the deep marks among them are attached within it by the
// layer's attachment. The deep marks from holds are those of its layers,
// where its own list holds none: a layer holds deep marks alone, and from
// holds its own list before its outer layers. Otherwise, and where r carries
// marks, as an error value can, r is given the marks one by one.
func (x converter) carry(r Value, from *node) Value {
	s := from.marks
	if s == nil {
		return r
	}
	if r.n.marks == nil && (s.layer || !slices.ContainsFunc(s.list, isDeep)) {
		p := x.carried.part(s)
		if p == nil {
			return r
		}
		nn := r.n.clone()
		nn.marks = p
		deep := p
		if !s.layer {
			deep = x.carried.part(s.outer)
		}
		if deep != nil {
			x.carried.attachment(deep).within(nn)
		}
		return Value{n: nn}
	}
	if ms := propagateMarks(from); ms != nil {
		return WithMarks(r, ms...)
	}
	return r
}

// propagateMarks returns the Propagate marks that n carries, or nil.
func propagateMarks(n *node) []Mark {
	var ms []Mark
	for _, m := range n.markList() {
		if propagates(m) {
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
		return Narrow(v, NullOnly())
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
		// A pending value is held by no container, so nothing settles what
		// its conversion leaves open.
		out.fail = cmp.Or(out.fail, out.open)
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
		u := withoutMarks(v)
		return u
	}
	return pendingValue(c, n.null)
}

// known works out the conversion of a known value, whose content is in hand
// though a member of it may not be known. A constraint that admits exactly one
// type converts as Exactly of that type, however it is written (CV-026), so
// that is decided first, once, as typeConvertKind decides it, and the kind of
// c decides the rest.
func (x converter) known(v Value, c Constraint) draft {
	n := v.n
	if x.memo.fits(c, n.typ) {
		u := withoutMarks(v)
		return finished(u)
	}
	if n.typ.t.kind != KindCapsule && isStructural(c) {
		return x.structure(v, c)
	}
	if s, ok := x.memo.soleType(c); ok {
		return x.exactly(v, s)
	}
	if c.c.kind == ConstraintOneOf {
		m, f := oneOfMember(n.typ, x.typeName(n), c, x.policy)
		if f != nil {
			return finished(errorValue(f.diagnostic()))
		}
		d := x.known(v, m)
		if r := d.done; r.n != nil && r.n.state == statePending {
			// The value converts to what the target says, whichever member
			// of it applied.
			return finished(WithMarks(pendingValue(c, r.n.null), r.n.markList()...))
		}
		return d
	}
	if n.typ.t.kind == KindCapsule {
		// A capsule type converts only to a type that it, or the type it
		// converts to, declares.
		return finished(errorValue(noConversion(x.typeName(n), c).diagnostic()))
	}
	return x.structure(v, c)
}

// exactly works out the conversion of a known value to the type s, as
// converting to Exactly(s) does (CV-020).
func (x converter) exactly(v Value, s Type) draft {
	switch {
	case v.n.typ.t.kind == KindCapsule || s.t.kind == KindCapsule:
		return finished(x.capsule(v, s))
	case isPrimitive(s.t.kind):
		return finished(x.primitive(v, s))
	}
	// The structure of s admits s alone, so converting to it does not ask
	// for its one type again.
	return x.structure(v, structural(s))
}

// structure works out the conversion of a known value to a ListOf, SetOf,
// MapOf, TupleOf or ObjectWith constraint.
func (x converter) structure(v Value, c Constraint) draft {
	switch c.c.kind {
	case ConstraintListOf, ConstraintSetOf, ConstraintMapOf:
		return x.collection(v, c)
	case ConstraintTupleOf:
		return x.tuple(v, c)
	case ConstraintObjectWith:
		return x.object(v, c)
	}
	return finished(errorValue(noConversion(x.typeName(v.n), c).diagnostic()))
}

// primitive converts a known value to the primitive type s.
func (x converter) primitive(v Value, s Type) Value {
	n := v.n
	if out := primitiveTypeConvert(n.typ, s, x.typeName(n), x.policy); out.fail != nil {
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
		return errorValue(Diagnostic{Code: CodeBoolInvalidSyntax, Message: valueText(v) + ` is neither "true" nor "false"`})
	}
	d, err := decimal.Parse(text)
	if err == nil {
		return numberValue(d)
	}
	switch code := numberCode(err.(decimal.Error)); code {
	case CodeNumberInvalidSyntax:
		return errorValue(Diagnostic{Code: code, Message: valueText(v) + " is not a number"})
	case CodeNumberOutOfRange:
		return errorValue(Diagnostic{Code: code, Message: valueText(v) + " is outside the range of numbers"})
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
	if out := capsuleTypeConvert(n.typ, s, x.typeName(n), x.policy); out.fail != nil {
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
// with the name each has where it has one. The step that locates a member is
// made only for a member that fails (step), as most do not: making one for
// every member, a Number for each index, cost more than the members did.
type held struct {
	vals  []Value
	names []string // map keys or attribute names; nil for other containers
	kind  Kind     // the kind of the container
}

// step returns the step that locates the member at i.
func (h held) step(i int) Step {
	switch h.kind {
	case KindMap:
		return indexStep(String(h.names[i]))
	case KindObject:
		return attributeStep(h.names[i])
	}
	return indexStep(NumberFromInt(int64(i)))
}

// members returns what the known container n holds. A set's members come out
// as Elements gives them, carrying the set's deep marks.
func members(n *node) held {
	h := held{kind: n.typ.t.kind}
	switch h.kind {
	case KindMap:
		entries := n.data.([]mapEntry)
		h.vals = make([]Value, len(entries))
		h.names = make([]string, len(entries))
		for i, e := range entries {
			h.vals[i], h.names[i] = e.val, e.key
		}
	case KindObject:
		h.vals = n.data.([]Value)
		h.names = make([]string, len(n.typ.t.attrs))
		for i, a := range n.typ.t.attrs {
			h.names[i] = a.name
		}
	case KindSet:
		h.vals = n.retrievedMembers()
	default:
		h.vals = n.data.([]Value)
	}
	return h
}

// draftMembers works out the conversion of each member to the constraint that
// at gives for its position, reporting the error value that the failed ones
// make, located by their steps and carrying their marks, and whether any
// converts to a pending value.
func (x converter) draftMembers(h held, at func(i int) Constraint) ([]memberDraft, Value, bool, bool) {
	out := make([]memberDraft, len(h.vals))
	var errs containerErrors
	pending := false
	for i, m := range h.vals {
		d := x.draft(m, at(i))
		out[i] = memberDraft{draft: d, from: m.n}
		switch r := d.done; {
		case r.n == nil:
		case r.n.state == stateError:
			errs.add(h.step(i), x.carry(r, m.n))
		case r.n.state == statePending:
			pending = true
		}
	}
	e, failed := errs.value()
	return out, e, failed, pending
}

// collection works out the conversion of a known list, set, tuple, map or
// object to a ListOf, SetOf or MapOf constraint.
func (x converter) collection(v Value, c Constraint) draft {
	n, d := v.n, c.c
	from := n.typ.t.kind
	unsafe := false
	switch {
	case d.kind == ConstraintMapOf && (from == KindMap || from == KindObject):
	case d.kind != ConstraintMapOf && (from == KindList || from == KindSet || from == KindTuple):
		unsafe = d.kind == ConstraintSetOf && from != KindSet
	default:
		return finished(errorValue(noConversion(x.typeName(n), c).diagnostic()))
	}
	if unsafe && x.policy == Safe {
		// The policy refuses the conversion whatever the members hold, so it
		// fails before any is read, carrying none of their marks (CV-051,
		// CV-033).
		return finished(errorValue(unsafeConversion(x.typeName(n), c).diagnostic()))
	}
	h := members(n)
	drafts, e, failed, pending := x.draftMembers(h, func(int) Constraint { return d.elem })
	if failed {
		return finished(e)
	}
	withhold := x.typeWithheld(n)
	types := make([]Type, 0, len(drafts)+2)
	var least []Type
	for i, md := range drafts {
		if r := md.done; r.n != nil && r.n.state == statePending {
			// As in collectionTypeConvert: a member whose no-keys conversion
			// fails settles no element type, and is left out rather than
			// contributing the zero Type.
			if none := typeConvert(h.vals[i].n.typ, d.elem, x.policy, keysNone); none.fail == nil {
				least = append(least, none.typ)
			}
			continue
		}
		types = append(types, md.typeOf())
	}
	var nullOpen *failure
	if from == KindList || from == KindSet || from == KindMap {
		out := typeConvert(n.typ.t.elem, d.elem, x.policy, keysNone)
		if out.fail != nil {
			return finished(failedReading(n, out.fail.diagnostic()))
		}
		types = append(types, out.typ)
		nullOpen = out.open
	}
	if pending {
		if out := pendingElements(types, least, d.elem, x.policy, withhold); out.fail != nil {
			return finished(failedReading(n, out.fail.diagnostic()))
		}
		return finished(pendingContainer(c, n))
	}
	elem, f := elementType(types, d.elem, x.policy, withhold, x.memo)
	if f != nil {
		return finished(failedReading(n, f.diagnostic()))
	}
	r := draft{parts: &parts{members: drafts, names: h.names}}
	if elem.t.open && !slices.ContainsFunc(drafts, leavesOpen) {
		// No member leaves the element type open, so the collection does
		// itself: it has no members, and nothing else settles a type
		// (CV-021).
		r.open = cmp.Or(nullOpen, unsettledElement(d.elem))
	}
	if from == KindSet && n.partial && d.kind == ConstraintListOf {
		// A set holding members that are not known has no settled order and
		// no settled count, so the list it becomes is not known either.
		low, high := setLengthBounds(n)
		unknownList := func(t Type) Value {
			return Narrow(Unknown(t), NotNull(), LengthMin(int64(low)), LengthMax(int64(high)))
		}
		if !elem.t.open {
			return finished(unknownList(ListType(elem)))
		}
		r.typ, r.later = ListType(elem), unknownList
		return r
	}
	switch d.kind {
	case ConstraintListOf:
		r.typ = ListType(elem)
	case ConstraintSetOf:
		r.typ = SetType(elem)
	default:
		r.typ = MapType(elem)
	}
	if x.holdsRedacting(n) {
		r.withheld = withheldWithin(drafts)
	}
	// An element type the members settle holds the attribute names of their
	// object types, and every member is given those attributes, so one taken
	// from a redacted member shows its structure in the result's type and
	// in its siblings: the result carries that member's redacting marks
	// (CV-033). A constraint that settles the type takes nothing from them.
	if _, fixed := x.memo.resultType(d.elem); withhold && !fixed {
		r.marks = r.withheld
	}
	if d.kind == ConstraintSetOf {
		r.src = n
		// A set gathers the marks of the values within its members, the
		// redacting ones among them (CV-033). The levels above read what a
		// member carries before it is built (withheldIn), so they are given
		// those now.
		if x.holdsRedacting(n) {
			r.marks, _ = mergeMarks(r.marks, redactingWithin(n))
		}
	}
	return r
}

// madeAtItsOwnType reports whether the set that d works out, whose element
// type its members settle as elem, is made at that type before the level
// above widens it, rather than at the wider type. A set is made of its members
// at its element type and widened then, keeping the marks it gathered
// (CV-033) and the members it kept (EQ-041), whatever its members would be at
// the wider type. Made at the wider type, it differs only where a value within
// what a member converts to carries an Isolate mark, which a value rebuilt at
// a wider type does not carry, and where a member that is not known could be
// one the set holds already, over an element type whose values can be
// counted. Elsewhere it is made once, at the wider type: made at its own type
// first, it would be made again for every set holding it that widens.
func (x converter) madeAtItsOwnType(d draft, elem Type) bool {
	if x.isolatingIn(d.members) {
		return true
	}
	c, ok := memberCount(elem)
	return d.src.partial && ok && c <= maxDomainSet
}

// isolatingIn reports whether a value within what these members convert to
// carries an Isolate mark, short of a set, which keeps the marks it gathers
// however it is widened, and of what builds no container, which holds no
// value. A member's own Isolate marks its conversion does not carry (CV-033),
// but a value carried across unchanged keeps every mark it has.
func (x converter) isolatingIn(mds []memberDraft) bool {
	return slices.ContainsFunc(mds, func(md memberDraft) bool {
		switch r := md.done.n; {
		case r != nil:
			return r.marks != nil && r.marks.holdsIsolating() || x.holdsIsolating(r)
		case md.later != nil || md.src != nil:
			return false
		}
		return x.isolatingIn(md.members)
	})
}

// redactedStructure returns the redacting marks of the values among members,
// at any depth, that carry one and whose type names attributes: an object
// type, or one holding an object type. Such a value's attribute names are its
// structure, which its redacting marks withhold (MK-011).
func redactedStructure(members []Value) []Mark {
	var marks []Mark
	var walk func(n *node)
	walk = func(n *node) {
		if ms := n.redactingMarks(); ms != nil && n.state != stateError {
			if namesAttributes(n.typ) {
				marks, _ = mergeMarks(marks, ms)
			}
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

// namesAttributes reports whether t is an object type or holds one.
func namesAttributes(t Type) bool {
	if t.t == nil {
		return false
	}
	switch d := t.t; d.kind {
	case KindObject:
		return true
	case KindList, KindSet, KindMap:
		return namesAttributes(d.elem)
	case KindTuple:
		return slices.ContainsFunc(d.elems, namesAttributes)
	}
	return false
}

// setOf returns the set of type st holding these members, which give their
// marks, at every depth, to the set, since a set's members carry none. The
// members are unmarked by one taking, as UnmarkDeep unmarks one value: members
// under a container's deep marks share the layer that holds them, which is
// taken once rather than once for each member, so a list of k members under k
// deep marks costs k and not k by k.
func setOf(st Type, members []Value) Value {
	var t taking
	unmarked := make([]Value, len(members))
	for i, m := range members {
		unmarked[i] = Value{n: m.n.unmarkDeep(&t)}
	}
	sortMarks(t.marks)
	return WithMarks(sequenceValue(st, "Set", unmarked), t.marks...)
}

// tuple works out the conversion of a known tuple, list or set to a TupleOf
// constraint.
func (x converter) tuple(v Value, c Constraint) draft {
	n, d := v.n, c.c
	from := n.typ.t.kind
	switch from {
	case KindTuple:
		if len(n.typ.t.elems) != len(d.members) {
			return finished(errorValue(tupleTypeConvert(n.typ, c, x.policy, keysNone).fail.diagnostic()))
		}
	case KindList, KindSet:
		want := len(d.members)
		if from == KindSet && n.partial {
			return x.partialSetTuple(v, c)
		}
		if got := len(n.data.([]Value)); got != want {
			return finished(errorValue(Diagnostic{Code: CodeConvertLengthMismatch,
				Message: "a " + kindNoun(from) + " of " + count(got, "member") + " does not convert to " + c.String() + ", which has " + count(want, "member")}))
		}
	default:
		return finished(errorValue(noConversion(x.typeName(n), c).diagnostic()))
	}
	if from != KindTuple && x.policy == Safe {
		// As for a list to a set (collection).
		return finished(errorValue(unsafeConversion(x.typeName(n), c).diagnostic()))
	}
	drafts, e, failed, pending := x.draftMembers(members(n), func(i int) Constraint { return d.members[i] })
	switch {
	case failed:
		return finished(e)
	case pending:
		return finished(pendingContainer(c, n))
	}
	types := make([]Type, len(drafts))
	for i, md := range drafts {
		types[i] = md.typeOf()
	}
	r := draft{typ: TupleType(types...), parts: &parts{members: drafts}}
	if x.holdsRedacting(n) {
		r.withheld = withheldWithin(drafts)
	}
	return r
}

// partialSetTuple converts a set holding members that are not known to a
// TupleOf constraint. Its members have no settled order, so the tuple is not
// known, and where the number of members it could have rules out the tuple's
// length the conversion fails already. Its known members convert all the
// same, to whichever position they take, which the members not known leave
// open: one that fails at every position alike fails every outcome, and so
// fails now (CV-031, UN-011), as it does converted to a list.
func (x converter) partialSetTuple(v Value, c Constraint) draft {
	n, d := v.n, c.c
	low, high := setLengthBounds(n)
	if want := len(d.members); want < low || want > high {
		return finished(errorValue(Diagnostic{Code: CodeConvertLengthMismatch,
			Message: "a set of " + strconv.Itoa(low) + " to " + count(high, "member") + " does not convert to " + c.String() + ", which has " + count(want, "member")}))
	}
	out := typeConvert(n.typ, c, x.policy, keysUnknown)
	switch {
	case out.fail != nil:
		return finished(errorValue(out.fail.diagnostic()))
	case out.pending:
		return finished(Narrow(Pending(c), NotNull()))
	}
	h := members(n)
	var errs containerErrors
	for i, m := range h.vals[:knownMembers(h.vals)] {
		if r, ok := x.failsEverywhere(m, d.members); ok {
			errs.add(h.step(i), x.carry(r, m.n))
		}
	}
	if e, failed := errs.value(); failed {
		return finished(e)
	}
	unknownTuple := func(t Type) Value { return Narrow(Unknown(t), NotNull()) }
	if out.open != nil {
		return deferred(out.typ, out.open, unknownTuple)
	}
	return finished(unknownTuple(out.typ))
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

// object works out the conversion of a known object or map to an ObjectWith
// constraint.
func (x converter) object(v Value, c Constraint) draft {
	n, d := v.n, c.c
	from := n.typ.t.kind
	if from != KindObject && from != KindMap {
		return finished(errorValue(noConversion(x.typeName(n), c).diagnostic()))
	}
	if from == KindMap && x.policy == Safe {
		// As for a list to a set (collection).
		return finished(errorValue(unsafeConversion(x.typeName(n), c).diagnostic()))
	}
	h := members(n)
	var errs containerErrors
	attrs := make(map[string]memberDraft, len(h.vals))
	pending := false
	fields := d.fields
	missing := func(name string) {
		errs.addDiagnostic(missingAttribute(name).diagnostic())
	}
	absent := func(f field) {
		if r, ok := x.absentValue(f); ok {
			attrs[f.name] = memberDraft{draft: finished(r)}
		}
	}
	for i, name := range h.names {
		for len(fields) > 0 && fields[0].name < name {
			if fields[0].Required {
				missing(fields[0].name)
			}
			absent(fields[0])
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
			attrs[name] = memberDraft{draft: finished(m)}
		default:
			md := memberDraft{draft: x.draft(m, fields[0].Constraint), from: m.n}
			switch r := md.done; {
			case r.n == nil:
			case r.n.state == stateError:
				errs.add(h.step(i), x.carry(r, m.n))
			case r.n.state == statePending:
				pending = true
			}
			attrs[name] = md
			fields = fields[1:]
		}
	}
	for _, f := range fields {
		if f.Required {
			missing(f.name)
		}
		absent(f)
	}
	if e, failed := errs.value(); failed {
		return finished(e)
	}
	if pending {
		return finished(pendingContainer(c, n))
	}
	r := draft{parts: &parts{names: slices.Sorted(maps.Keys(attrs))}}
	r.members = make([]memberDraft, len(r.names))
	types := make(map[string]Type, len(r.names))
	for i, name := range r.names {
		r.members[i] = attrs[name]
		types[name] = r.members[i].typeOf()
	}
	r.typ = ObjectType(types)
	if x.holdsRedacting(n) {
		r.withheld = withheldWithin(r.members)
	}
	return r
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
		r = Null(e)
	case stateUnknown:
		rd := n.data.(*rangeData)
		r = narrowedUnknown(e, rd.null, lengthNarrowings(n.typ, e, rd))
	default:
		r = x.fitKnown(m, e)
	}
	return x.carry(r, n)
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
		switch {
		case to.kind == KindList:
			return List(to.elem, fitted...)
		case to.kind == KindSet && n.typ.t.kind == KindSet:
			// Every mark stays on the set, Isolate ones too: they include
			// those its members gave it at every depth (CV-033), which members
			// fitted again, carrying none, do not give back.
			return WithMarks(Set(to.elem, fitted...), n.markList()...)
		case to.kind == KindSet:
			return Set(to.elem, fitted...)
		}
		entries := make(map[string]Value, len(fitted))
		for i, v := range fitted {
			entries[h.names[i]] = v
		}
		return Map(to.elem, entries)
	case KindTuple:
		for i, v := range h.vals {
			fitted[i] = x.fit(v, to.elems[i])
		}
		return Tuple(fitted...)
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
			vals[i] = Null(a.typ)
		}
		return objectOf(e, vals)
	}
	internalPanic("fit called with %s for %s", e, n.describe())
	return Value{}
}

// failedReading returns the error value of a conversion of the known container
// n that failed once it had read the values within n: it holds none of them,
// so it carries their Propagate marks, as pendingContainer does (CV-033).
func failedReading(n *node, d Diagnostic) Value {
	return WithMarks(errorValue(d), heldMarks(n)...)
}

// failsEverywhere converts m to each of cs, and returns the failure where every
// one fails alike, with the same diagnostics, and false where any succeeds or
// two fail otherwise: which of them m converts to is not settled yet.
func (x converter) failsEverywhere(m Value, cs []Constraint) (Value, bool) {
	var first Value
	for _, c := range cs {
		r := x.draft(m, c).done
		if r.n == nil || r.n.state != stateError {
			return Value{}, false
		}
		if first.n == nil {
			first = r
		} else if !slices.EqualFunc(first.n.diagnostics(), r.n.diagnostics(), Diagnostic.Equal) {
			return Value{}, false
		}
	}
	return first, first.n != nil
}

// pendingContainer returns the pending value that a known container converts
// to when a member of it converts to a pending value, which no container can
// hold. The conversion read the values within the container, and the result
// holds none of them, so it carries their Propagate marks.
func pendingContainer(c Constraint, n *node) Value {
	return WithMarks(Narrow(Pending(c), NotNull()), heldMarks(n)...)
}

// heldMarks returns the Propagate marks that the values within n carry, at any
// depth, each once, in the order met. They are gathered as UnmarkDeep gathers
// marks: a layer of deep marks that members share is read once, and a mark is
// looked up in a set once there are many, so k members under k deep marks,
// or k members each carrying a mark of its own, cost k and not k by k.
func heldMarks(n *node) []Mark { return marksWithin(n, propagates) }

// redactingWithin returns the redacting marks that the values within n carry,
// at any depth, each once: what a set made of n's members gathers of them.
func redactingWithin(n *node) []Mark { return marksWithin(n, Mark.Redacting) }

// marksWithin returns the marks for which keep is true that the values within
// n carry, at any depth, each once, in the order met, as heldMarks says.
func marksWithin(n *node, keep func(Mark) bool) []Mark {
	t := taking{keep: keep}
	var walk func(n *node)
	walk = func(n *node) {
		if !n.markedWithin {
			return
		}
		visit := func(m *node) {
			t.add(m.marks)
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
	return t.marks
}

// typeWithheld reports whether a message leaves out the type of n: where n
// carries a redacting mark, whose type is part of what the mark withholds, or
// holds a value that does, whose type shows in n's (MK-011).
func (x converter) typeWithheld(n *node) bool {
	return n.redactingMarks() != nil || x.holdsRedacting(n)
}

// typeName names the type of n, a value being converted, for a diagnostic
// message: by the placeholder where n carries a redacting mark, by its kind
// alone where it holds a value that carries one, and otherwise as typeText
// renders it.
func (x converter) typeName(n *node) string {
	switch {
	case n.redactingMarks() != nil:
		return redactedText(n.redactingMarks())
	case !x.holdsRedacting(n):
		return typeText(n.typ)
	case n.typ.t.kind == KindObject:
		return "an object"
	}
	return "a " + kindNoun(n.typ.t.kind)
}

// holdsIsolating reports whether a value within n, at any depth, carries an
// Isolate mark. Each set a conversion widens asks it of the values its members
// keep (isolatingIn), so each value's answer is remembered for the
// conversion.
func (x converter) holdsIsolating(n *node) bool {
	if !n.markedWithin {
		return false
	}
	if r, ok := x.memo.isolating[n]; ok {
		return r
	}
	holds := func(m *node) bool { return m.marks != nil && m.marks.holdsIsolating() || x.holdsIsolating(m) }
	r := false
	switch data := n.data.(type) {
	case []Value:
		r = slices.ContainsFunc(data, func(m Value) bool { return holds(m.n) })
	case []mapEntry:
		r = slices.ContainsFunc(data, func(e mapEntry) bool { return holds(e.val.n) })
	}
	if x.memo.isolating == nil {
		x.memo.isolating = map[*node]bool{}
	}
	x.memo.isolating[n] = r
	return r
}

// holdsRedacting reports whether a value within n, at any depth, carries a
// redacting mark. Each container a conversion meets asks it of the values
// beneath it, so each value's answer is remembered for the conversion.
func (x converter) holdsRedacting(n *node) bool {
	if !n.markedWithin {
		return false
	}
	if r, ok := x.memo.redacting[n]; ok {
		return r
	}
	holds := func(m *node) bool { return m.redactingMarks() != nil || x.holdsRedacting(m) }
	r := false
	switch data := n.data.(type) {
	case []Value:
		r = slices.ContainsFunc(data, func(m Value) bool { return holds(m.n) })
	case []mapEntry:
		r = slices.ContainsFunc(data, func(e mapEntry) bool { return holds(e.val.n) })
	}
	if x.memo.redacting == nil {
		x.memo.redacting = map[*node]bool{}
	}
	x.memo.redacting[n] = r
	return r
}

// absentValue returns the attribute that an absent optional field f adds,
// where its constraint gives a type: the null of that type.
func (x converter) absentValue(f field) (Value, bool) {
	if t, ok := x.memo.resultType(f.Constraint); ok {
		return Null(t), true
	}
	return Value{}, false
}
