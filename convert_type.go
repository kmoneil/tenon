package tenon

import (
	"cmp"
	"slices"
	"strconv"
)

// keys says what a conversion of a type knows of the keys of the maps that a
// value of the type holds, which is what settles the type a map converts to as
// an object.
type keys uint8

const (
	// keysNone is a null value's, or an empty container's: no map within it has
	// keys, so a map converted to an object holds the required attributes.
	keysNone keys = iota + 1
	// keysUnknown is an unknown value's: the maps within it have keys, and
	// nothing says which.
	keysUnknown
)

// typeOutcome is what converting a type to a constraint gives, whatever value
// of the type is converted: the type of the result; or pending, where the
// result's type would follow from keys that are not in hand; or a failure.
// Where the type leaves a part open (CV-021), open is the failure that the
// conversion gives if nothing above it settles that part.
type typeOutcome struct {
	typ     Type
	pending bool
	fail    *failure
	open    *failure
}

// kindOpen is the kind of openType, which no type outside a conversion has.
const kindOpen = KindCapsule + 1

// openType is the part of a type that a conversion leaves open (CV-021): the
// element type of a collection converted where there is nothing to unify, as
// for an empty tuple converted to ListOf(Any). It unifies with any type to
// that type (CV-044), so the members beside it settle it, and a member whose
// type holds it is built at the type they settle (fillOpen). A conversion
// whose type still holds it at the top fails (openFailure): no value is ever
// made of it, and no type a program sees holds it.
var openType = func() Type {
	d := &typeData{id: newTypeID(), kind: kindOpen, open: true}
	d.shape = shapeOf(d)
	return Type{d}
}()

// unsettledElement returns the failure of a conversion to a collection whose
// element type nothing settles: it has no members, or none that settle a type,
// and c gives none.
func unsettledElement(c Constraint) *failure {
	return &failure{CodeConvertNoCommonType,
		"nothing settles an element type for " + c.String() + ": there are no members, and the constraint admits more than one type"}
}

// fillOpen returns p, the type a conversion gave, with each part it leaves
// open taking the type that t has in its place, t being a type that
// unification made from p and others (CV-021). A type with no part open is
// returned as it is.
func fillOpen(p, t Type) Type {
	switch {
	case !p.t.open:
		return p
	case p == openType:
		return t
	}
	from := p.t
	switch from.kind {
	case KindList, KindSet, KindMap:
		return collection(from.kind, fillOpen(from.elem, t.t.elem))
	case KindTuple:
		elems := make([]Type, len(from.elems))
		for i, e := range from.elems {
			elems[i] = fillOpen(e, placeOf(t, i, ""))
		}
		return TupleType(elems...)
	}
	attrs := make(map[string]Type, len(from.attrs))
	for _, a := range from.attrs {
		attrs[a.name] = fillOpen(a.typ, placeOf(t, 0, a.name))
	}
	return ObjectType(attrs)
}

// placeOf returns the part of t that holds the member at i of a container
// whose type unification made t from, or the attribute name, where the
// container is an object or a map: the element type at i of a tuple, the type
// of the attribute name of an object, and the element type of anything else.
func placeOf(t Type, i int, name string) Type {
	switch d := t.t; d.kind {
	case KindTuple:
		return d.elems[i]
	case KindObject:
		a, _ := d.attribute(name)
		return a
	}
	return t.t.elem
}

// firstOpen returns the open failure of the first of outs whose type leaves
// open a part that elem, their unification, leaves open too, and otherwise the
// failure of the conversion to c whose element type is elem, which nothing
// settled at all.
func firstOpen(outs []typeOutcome, elem Type, c Constraint) *failure {
	for _, o := range outs {
		if o.typ.t != nil && o.typ.t.open && fillOpen(o.typ, elem).t.open {
			return o.open
		}
	}
	return unsettledElement(c)
}

// failure says why a conversion fails: a diagnostic code and a message.
type failure struct {
	code    Code
	message string
}

func (f *failure) diagnostic() Diagnostic {
	return Diagnostic{Code: f.code, Message: f.message}
}

// typeConvert returns what converting a value of type t to c under the policy
// p gives, for values whose map keys k describes. It is what decides whether a
// conversion exists, and the result type of a null, an unknown or a pending
// value. A constraint that gives a type (CV-027) settles the result type,
// whatever keys it would otherwise have taken.
func typeConvert(t Type, c Constraint, p Policy, k keys) typeOutcome {
	out := typeConvertKind(t, c, p, k)
	if out.pending {
		if s, ok := resultType(c); ok {
			return typeOutcome{typ: s}
		}
	}
	return out
}

// typeConvertKind converts t to c by the kind of c. A constraint that admits
// exactly one type converts as Exactly of that type, however it is written
// (CV-026), so that is decided first, once, and the kind of c decides the
// rest. One that structural builds is converted to as it stands, since it
// converts as Exactly of its type already, unless t is a capsule type, which
// converts by what the capsule types declare.
func typeConvertKind(t Type, c Constraint, p Policy, k keys) typeOutcome {
	if fits(c, t) {
		return typeOutcome{typ: t}
	}
	if t.t.kind != KindCapsule && isStructural(c) {
		return typeConvertStructure(t, c, p, k)
	}
	if s, ok := soleType(c); ok {
		return typeConvertExactly(t, s, p, k)
	}
	if c.c.kind == ConstraintOneOf {
		return typeConvertOneOf(t, c, p, k)
	}
	if t.t.kind == KindCapsule {
		// A capsule type converts only to a type that it, or the type it
		// converts to, declares.
		return failed(noConversion(typeText(t), c))
	}
	return typeConvertStructure(t, c, p, k)
}

// typeConvertExactly converts t to the type s, as converting to Exactly(s)
// does (CV-020).
func typeConvertExactly(t, s Type, p Policy, k keys) typeOutcome {
	switch {
	case t.t.kind == KindCapsule || s.t.kind == KindCapsule:
		return capsuleTypeConvert(t, s, typeText(t), p)
	case isPrimitive(s.t.kind):
		return primitiveTypeConvert(t, s, typeText(t), p)
	}
	// The structure of s admits s alone, so converting to it does not ask
	// for its one type again.
	return typeConvertStructure(t, structural(s), p, k)
}

// typeConvertStructure converts t to a ListOf, SetOf, MapOf, TupleOf or
// ObjectWith constraint.
func typeConvertStructure(t Type, c Constraint, p Policy, k keys) typeOutcome {
	switch c.c.kind {
	case ConstraintListOf, ConstraintSetOf, ConstraintMapOf:
		return collectionTypeConvert(t, c, p, k)
	case ConstraintTupleOf:
		return tupleTypeConvert(t, c, p, k)
	case ConstraintObjectWith:
		return objectTypeConvert(t, c, p, k)
	}
	return failed(noConversion(typeText(t), c))
}

// structural returns the constraint that names a list, set, map, tuple or
// object type by its structure, which converts as Exactly of the type does.
// Every member of a container converted to Exactly of a type asks for it, so
// it is built once for the type and kept there.
func structural(t Type) Constraint {
	d := t.t
	if c := d.structure.Load(); c != nil {
		return Constraint{c: c}
	}
	var c Constraint
	switch d.kind {
	case KindList:
		c = ListOf(Exactly(d.elem))
	case KindSet:
		c = SetOf(Exactly(d.elem))
	case KindMap:
		c = MapOf(Exactly(d.elem))
	case KindTuple:
		members := make([]Constraint, len(d.elems))
		for i, e := range d.elems {
			members[i] = Exactly(e)
		}
		c = TupleOf(members...)
	case KindObject:
		fields := make(map[string]Field, len(d.attrs))
		for _, a := range d.attrs {
			fields[a.name] = Required(Exactly(a.typ))
		}
		c = ObjectWith(fields, true)
	default:
		internalPanic("structural called on %s", t)
	}
	d.structure.CompareAndSwap(nil, c.c)
	return Constraint{c: d.structure.Load()}
}

// isStructural reports whether c is a constraint that structural builds: the
// list, set or map of an Exactly element, the tuple of Exactly members, or
// the closed object whose every field is required and Exactly. Converting to
// such a constraint is converting to Exactly of the one type it admits
// already, so nothing needs to ask for that type.
func isStructural(c Constraint) bool {
	d := c.c
	switch d.kind {
	case ConstraintListOf, ConstraintSetOf, ConstraintMapOf:
		return d.elem.c.kind == ConstraintExactly
	case ConstraintTupleOf:
		for _, m := range d.members {
			if m.c.kind != ConstraintExactly {
				return false
			}
		}
		return true
	case ConstraintObjectWith:
		if !d.closed {
			return false
		}
		for _, f := range d.fields {
			if !f.Required || f.Constraint.c.kind != ConstraintExactly {
				return false
			}
		}
		return true
	}
	return false
}

func failed(f *failure) typeOutcome { return typeOutcome{fail: f} }

// isPrimitive reports whether k is the kind of a Bool, Number or String type.
func isPrimitive(k Kind) bool { return k == KindBool || k == KindNumber || k == KindString }

// primitiveTypeConvert converts t to the distinct primitive type s. String
// converts to and from Number and Bool, unsafely, and nothing else converts to
// a primitive type. from is what a failure names t by (noConversion).
func primitiveTypeConvert(t, s Type, from string, p Policy) typeOutcome {
	if !isPrimitive(t.t.kind) || t.t.kind != KindString && s.t.kind != KindString {
		return failed(noConversion(from, Exactly(s)))
	}
	if p == Safe {
		return failed(unsafeConversion(from, Exactly(s)))
	}
	return typeOutcome{typ: s}
}

// collectionTypeConvert converts t to a ListOf, SetOf or MapOf constraint.
func collectionTypeConvert(t Type, c Constraint, p Policy, k keys) typeOutcome {
	d, from := c.c, t.t
	var members []Type
	unsafe := false
	switch {
	case d.kind == ConstraintMapOf && from.kind == KindMap:
		members = []Type{from.elem}
	case d.kind == ConstraintMapOf && from.kind == KindObject:
		for _, a := range from.attrs {
			members = append(members, a.typ)
		}
	case d.kind != ConstraintMapOf && (from.kind == KindList || from.kind == KindSet):
		members = []Type{from.elem}
		unsafe = d.kind == ConstraintSetOf && from.kind == KindList
	case d.kind != ConstraintMapOf && from.kind == KindTuple:
		members = from.elems
		unsafe = d.kind == ConstraintSetOf
	default:
		return failed(noConversion(typeText(t), c))
	}
	results := make([]Type, 0, len(members)+1)
	var least []Type
	var opens []typeOutcome
	pending := false
	for _, m := range members {
		out := typeConvert(m, d.elem, p, k)
		switch {
		case out.fail != nil:
			return out
		case out.open != nil:
			opens = append(opens, out)
			results = append(results, out.typ)
		case out.pending:
			pending = true
			// The type the member would have with no keys in hand is the
			// least it can have. Where even that does not convert, the member
			// says nothing about the element type: keys it has yet to see can
			// still give it attributes that convert, so the failure is not
			// one every value shares, and the rest decide.
			if none := typeConvert(m, d.elem, p, keysNone); none.fail == nil {
				least = append(least, none.typ)
			}
		default:
			results = append(results, out.typ)
		}
	}
	if unsafe && p == Safe {
		return failed(unsafeConversion(typeText(t), c))
	}
	if pending {
		return pendingElements(results, least, d.elem, p, false)
	}
	elem, f := elementType(results, d.elem, p, false, nil)
	if f != nil {
		return failed(f)
	}
	var out typeOutcome
	switch d.kind {
	case ConstraintListOf:
		out.typ = ListType(elem)
	case ConstraintSetOf:
		out.typ = SetType(elem)
	default:
		out.typ = MapType(elem)
	}
	if elem.t.open {
		out.open = firstOpen(opens, elem, d.elem)
	}
	return out
}

// elementType returns the element type of a collection whose members convert
// to these types under the element constraint c: their unification, with the
// type c names where it names one, which must satisfy c. Where there is
// nothing to unify, the element type is left open (openType), and where every
// type that has a part leaves it open, the element type leaves it open too:
// the levels above settle it, and judge what they settle by their own
// constraints, which hold c in its place. withhold leaves the types out of a
// message, where a map's keys that a redacting mark withholds may have named
// their attributes. memo remembers what a conversion of values asks, and is
// nil for a conversion of types.
func elementType(types []Type, c Constraint, p Policy, withhold bool, memo *convertMemo) (Type, *failure) {
	if s, ok := memo.resultType(c); ok {
		types = append(types[:len(types):len(types)], s)
	}
	if len(types) == 0 {
		return openType, nil
	}
	elem, ok := unifyTypes(types, p)
	switch {
	case !ok && withhold:
		return Type{}, &failure{CodeConvertNoCommonType, "the members have no common type"}
	case !ok:
		return Type{}, &failure{CodeConvertNoCommonType, "the members have no common type: " + typeList(types)}
	case elem.t.open:
		return elem, nil
	case !memo.satisfies(c, elem) && withhold:
		return Type{}, &failure{CodeConvertNoCommonType, "the members' common type does not satisfy " + c.String()}
	case !memo.satisfies(c, elem):
		return Type{}, &failure{CodeConvertNoCommonType,
			"the members' common type " + typeText(elem) + " does not satisfy " + c.String()}
	}
	return elem, nil
}

// pendingElements settles what a collection gives where some members convert
// to types that keys not in hand would settle. Such a member's type holds at
// least the attributes that it would hold with no keys, which are least, and
// types that fail to unify still fail with more attributes, so where the least
// of them fail with the settled ones every value fails. Otherwise the result
// is pending.
func pendingElements(settled, least []Type, c Constraint, p Policy, withhold bool) typeOutcome {
	types := append(append([]Type{}, settled...), least...)
	if s, ok := resultType(c); ok {
		types = append(types, s)
	}
	if len(types) == 0 {
		// Nothing is left to unify: every member waits on keys not in hand,
		// and the constraint names no type either. Nothing can fail here.
		return typeOutcome{pending: true}
	}
	if _, ok := unifyTypes(types, p); !ok {
		f := &failure{CodeConvertNoCommonType, "the members have no common type"}
		if !withhold {
			f.message += ": " + typeList(types)
		}
		return failed(f)
	}
	return typeOutcome{pending: true}
}

// typeList renders types for a message, each once, in order, writing no more
// of them than the message keeps.
func typeList(types []Type) string {
	return shortText(func(w *textWriter) {
		seen := map[Type]bool{}
		for _, t := range types {
			if w.full() {
				return
			}
			if seen[t] {
				continue
			}
			if len(seen) > 0 {
				w.WriteString(", ")
			}
			seen[t] = true
			t.write(w)
		}
	})
}

// tupleTypeConvert converts t to a TupleOf constraint.
func tupleTypeConvert(t Type, c Constraint, p Policy, k keys) typeOutcome {
	d, from := c.c, t.t
	unsafe := false
	switch from.kind {
	case KindTuple:
		if len(from.elems) != len(d.members) {
			return failed(&failure{CodeConvertNoConversion, "a tuple of " + count(len(from.elems), "element") +
				" does not convert to " + c.String() + ", which has " + count(len(d.members), "member")})
		}
	case KindList, KindSet:
		unsafe = true
	default:
		return failed(noConversion(typeText(t), c))
	}
	elems := make([]Type, len(d.members))
	pending := false
	var open *failure
	for i, m := range d.members {
		member := from.elem
		if from.kind == KindTuple {
			member = from.elems[i]
		}
		out := typeConvert(member, m, p, k)
		switch {
		case out.fail != nil:
			return out
		case out.pending:
			pending = true
		default:
			elems[i] = out.typ
			open = cmp.Or(open, out.open)
		}
	}
	if unsafe && p == Safe {
		return failed(unsafeConversion(typeText(t), c))
	}
	if pending {
		return typeOutcome{pending: true}
	}
	return typeOutcome{typ: TupleType(elems...), open: open}
}

// objectTypeConvert converts t to an ObjectWith constraint.
func objectTypeConvert(t Type, c Constraint, p Policy, k keys) typeOutcome {
	d, from := c.c, t.t
	switch from.kind {
	case KindObject:
	case KindMap:
		return mapObjectTypeConvert(t, c, p, k)
	default:
		return failed(noConversion(typeText(t), c))
	}
	attrs := map[string]Type{}
	pending := false
	var open *failure
	fields := d.fields
	for _, a := range from.attrs {
		for len(fields) > 0 && fields[0].name < a.name {
			if fields[0].Required {
				return failed(missingAttribute(fields[0].name))
			}
			addNull(attrs, fields[0])
			fields = fields[1:]
		}
		if len(fields) == 0 || fields[0].name != a.name {
			if d.closed {
				return failed(unexpectedAttribute(a.name))
			}
			attrs[a.name] = a.typ
			continue
		}
		out := typeConvert(a.typ, fields[0].Constraint, p, k)
		switch {
		case out.fail != nil:
			return out
		case out.pending:
			pending = true
		default:
			attrs[a.name] = out.typ
			open = cmp.Or(open, out.open)
		}
		fields = fields[1:]
	}
	for _, f := range fields {
		if f.Required {
			return failed(missingAttribute(f.name))
		}
		addNull(attrs, f)
	}
	if pending {
		return typeOutcome{pending: true}
	}
	return typeOutcome{typ: ObjectType(attrs), open: open}
}

// addNull adds to attrs the attribute that an absent optional field f adds,
// where its constraint gives a type: the null of that type is present in its
// place.
func addNull(attrs map[string]Type, f field) {
	if t, ok := resultType(f.Constraint); ok {
		attrs[f.name] = t
	}
}

// mapObjectTypeConvert converts a map type to an ObjectWith constraint. The
// attributes come from the keys. A map with none holds the required fields and
// the optional ones a conversion adds as null. An unknown map's type is
// pending where its keys could still decide which attributes are there: under
// an open constraint, or for an optional field that adds nothing when absent.
func mapObjectTypeConvert(t Type, c Constraint, p Policy, k keys) typeOutcome {
	d := c.c
	attrs := map[string]Type{}
	pending := k == keysUnknown && !d.closed
	var open *failure
	for _, f := range d.fields {
		if !f.Required {
			if _, ok := resultType(f.Constraint); !ok && k == keysUnknown && !admitsNone(f.Constraint) {
				pending = true
			}
			addNull(attrs, f)
			continue
		}
		out := typeConvert(t.t.elem, f.Constraint, p, k)
		switch {
		case out.fail != nil:
			return out
		case out.pending:
			pending = true
		default:
			attrs[f.name] = out.typ
			open = cmp.Or(open, out.open)
		}
	}
	if p == Safe {
		return failed(unsafeConversion(typeText(t), c))
	}
	if pending {
		return typeOutcome{pending: true}
	}
	return typeOutcome{typ: ObjectType(attrs), open: open}
}

// typeConvertOneOf converts t to the first member of a OneOf constraint to
// which a conversion from t exists under the policy.
func typeConvertOneOf(t Type, c Constraint, p Policy, k keys) typeOutcome {
	m, f := oneOfMember(t, typeText(t), c, p)
	if f != nil {
		return failed(f)
	}
	return typeConvert(t, m, p, k)
}

// oneOfMember returns the first member of the OneOf constraint c to which a
// conversion from t exists under the policy, or the failure when none does,
// which names t by from (noConversion).
func oneOfMember(t Type, from string, c Constraint, p Policy) (Constraint, *failure) {
	for _, m := range c.c.members {
		if typeConvert(t, m, p, keysNone).fail == nil {
			return m, nil
		}
	}
	if p == Safe {
		for _, m := range c.c.members {
			if typeConvert(t, m, Unsafe, keysNone).fail == nil {
				return Constraint{}, unsafeConversion(from, c)
			}
		}
	}
	return Constraint{}, noConversion(from, c)
}

// capsuleTypeConvert converts between t and the type s where either is a
// capsule type, by the conversions a capsule type declares: the source type's
// conversion to s, and failing that the target type's conversion from t. from
// is what a failure names t by (noConversion).
func capsuleTypeConvert(t, s Type, from string, p Policy) typeOutcome {
	_, _, safe, ok := capsuleConversion(t, s)
	switch {
	case !ok:
		return failed(noConversion(from, Exactly(s)))
	case !safe && p == Safe:
		return failed(unsafeConversion(from, Exactly(s)))
	}
	return typeOutcome{typ: s}
}

// noConversion returns the failure of a conversion that does not exist, from
// what the message names the source by: a type as typeText renders it, or a
// value's type as typeName does.
func noConversion(from string, c Constraint) *failure {
	return &failure{CodeConvertNoConversion, from + " does not convert to " + c.String()}
}

// unsafeConversion returns the failure of a conversion that exists only under
// the unsafe policy, applied under the safe one.
func unsafeConversion(from string, c Constraint) *failure {
	return &failure{CodeConvertUnsafe, from + " converts to " + c.String() + " only unsafely, and the policy is safe"}
}

// missingAttribute returns the failure of an object that lacks an attribute
// it must have.
func missingAttribute(name string) *failure {
	return &failure{CodeConvertMissingAttribute, "attribute " + quoted(name) + " is required, and absent"}
}

// unexpectedAttribute returns the failure of an object or map with an
// attribute or key that the constraint does not allow.
func unexpectedAttribute(name string) *failure {
	return &failure{CodeConvertUnexpectedAttribute, "attribute " + quoted(name) + " is not one the constraint allows"}
}

// count renders n things, as in "1 element" or "3 elements".
func count(n int, thing string) string {
	if n == 1 {
		return "1 " + thing
	}
	return strconv.Itoa(n) + " " + thing + "s"
}

// unifyTypes returns the type that every one of types converts to under the
// policy, by the rules of type unification, and whether there is one. It is
// commutative and associative, so the order of types does not matter.
func unifyTypes(types []Type, p Policy) (Type, bool) {
	if len(types) > 1 && allObjects(types) {
		return unifyObjectTypes(types, p)
	}
	// The object types among others are unified in one pass too, and their
	// union then with the rest, which the order not mattering allows: folded
	// in among a map, 4,000 objects of distinct attributes built the union
	// again for each, 1.5 s and 1.9 GB.
	if objects := slices.DeleteFunc(slices.Clone(types), func(t Type) bool { return t.t.kind != KindObject }); len(objects) > 1 {
		union, ok := unifyObjectTypes(objects, p)
		if !ok {
			return Type{}, false
		}
		rest := slices.DeleteFunc(slices.Clone(types), func(t Type) bool { return t.t.kind == KindObject })
		types = append(rest, union)
	}
	u := types[0]
	for _, t := range types[1:] {
		var ok bool
		if u, ok = unifyTwo(u, t, p); !ok {
			return Type{}, false
		}
	}
	return u, true
}

// allObjects reports whether every one of types is an object type.
func allObjects(types []Type) bool {
	for _, t := range types {
		if t.t.kind != KindObject {
			return false
		}
	}
	return true
}

// unifyObjectTypes unifies object types in one pass: the union holds every
// attribute of every type, each the unification of the types that hold that
// attribute, which is what unifying two at a time gives and what unification
// being associative promises.
//
// Folding builds the union again at every step, so n objects of distinct
// attributes build and intern n object types, the last of them the answer and
// the rest of them waste: the work and the memory grow as the square of the
// number of types where the union grows with it.
func unifyObjectTypes(types []Type, p Policy) (Type, bool) {
	names := make([]string, 0, len(types[0].t.attrs))
	held := map[string][]Type{}
	for _, t := range types {
		for _, attr := range t.t.attrs {
			if _, seen := held[attr.name]; !seen {
				names = append(names, attr.name)
			}
			held[attr.name] = append(held[attr.name], attr.typ)
		}
	}
	attrs := make(map[string]Type, len(names))
	// In the order the names were met, so that nothing turns on the order a
	// Go map iterates in.
	for _, name := range names {
		u, ok := unifyTypes(held[name], p)
		if !ok {
			return Type{}, false
		}
		attrs[name] = u
	}
	return ObjectType(attrs), true
}

// unifyTwo unifies two types. A part left open unifies as Any does, with
// anything to that thing (CV-044).
func unifyTwo(a, b Type, p Policy) (Type, bool) {
	switch {
	case a == b, b == openType:
		return a, true
	case a == openType:
		return b, true
	}
	if a.t.kind > b.t.kind {
		a, b = b, a
	}
	ka, kb := a.t.kind, b.t.kind
	switch {
	case isPrimitive(ka) && isPrimitive(kb):
		// Two distinct primitive types meet only as text, and only unsafely.
		if p == Unsafe {
			return Type{stringType}, true
		}
	case ka == kb && (ka == KindList || ka == KindSet || ka == KindMap):
		if elem, ok := unifyTwo(a.t.elem, b.t.elem, p); ok {
			return collection(ka, elem), true
		}
	case ka == KindList && kb == KindSet:
		if elem, ok := unifyTwo(a.t.elem, b.t.elem, p); ok {
			return ListType(elem), true
		}
	case (ka == KindList || ka == KindSet) && kb == KindTuple:
		if elem, ok := unifyTypes(append([]Type{a.t.elem}, b.t.elems...), p); ok {
			return ListType(elem), true
		}
	case ka == KindTuple && kb == KindTuple:
		return unifyTuples(a, b, p)
	case ka == KindMap && kb == KindObject:
		members := []Type{a.t.elem}
		for _, attr := range b.t.attrs {
			members = append(members, attr.typ)
		}
		if elem, ok := unifyTypes(members, p); ok {
			return MapType(elem), true
		}
	case ka == KindObject && kb == KindObject:
		return unifyObjects(a, b, p)
	}
	return Type{}, false
}

// unifyTuples unifies two tuple types: position by position where they are of
// one length, and otherwise as a list of every element type of both.
func unifyTuples(a, b Type, p Policy) (Type, bool) {
	x, y := a.t.elems, b.t.elems
	if len(x) != len(y) {
		all := append(append([]Type{}, x...), y...)
		if elem, ok := unifyTypes(all, p); ok {
			return ListType(elem), true
		}
		return Type{}, false
	}
	elems := make([]Type, len(x))
	for i := range x {
		var ok bool
		if elems[i], ok = unifyTwo(x[i], y[i], p); !ok {
			return Type{}, false
		}
	}
	return TupleType(elems...), true
}

// unifyObjects unifies two object types to the object type holding every
// attribute of either, each attribute of the types unified where both hold
// it.
func unifyObjects(a, b Type, p Policy) (Type, bool) {
	attrs := map[string]Type{}
	for _, attr := range a.t.attrs {
		attrs[attr.name] = attr.typ
	}
	for _, attr := range b.t.attrs {
		t, ok := attrs[attr.name]
		if !ok {
			attrs[attr.name] = attr.typ
			continue
		}
		if attrs[attr.name], ok = unifyTwo(t, attr.typ, p); !ok {
			return Type{}, false
		}
	}
	return ObjectType(attrs), true
}
