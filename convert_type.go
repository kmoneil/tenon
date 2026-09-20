package tenon

import "strconv"

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
type typeOutcome struct {
	typ     Type
	pending bool
	fail    *failure
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
		return failed(noConversion(t, c))
	}
	return typeConvertStructure(t, c, p, k)
}

// typeConvertExactly converts t to the type s, as converting to Exactly(s)
// does (CV-020).
func typeConvertExactly(t, s Type, p Policy, k keys) typeOutcome {
	switch {
	case t.t.kind == KindCapsule || s.t.kind == KindCapsule:
		return capsuleTypeConvert(t, s, p)
	case isPrimitive(s.t.kind):
		return primitiveTypeConvert(t, s, p)
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
	return failed(noConversion(t, c))
}

// structural returns the constraint that names a list, set, map, tuple or
// object type by its structure, which converts as Exactly of the type does.
// Every member of a container converted to Exactly of a type asks for it, so
// it is built once for the type and kept there.
func structural(t Type) Constraint {
	d := t.t
	if c := d.structure.Load(); c != nil {
		return Constraint{c}
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
	return Constraint{d.structure.Load()}
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
// a primitive type.
func primitiveTypeConvert(t, s Type, p Policy) typeOutcome {
	if !isPrimitive(t.t.kind) || t.t.kind != KindString && s.t.kind != KindString {
		return failed(noConversion(t, Exactly(s)))
	}
	if p == Safe {
		return failed(unsafeConversion(t, Exactly(s)))
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
		return failed(noConversion(t, c))
	}
	results := make([]Type, 0, len(members)+1)
	var least []Type
	pending := false
	for _, m := range members {
		out := typeConvert(m, d.elem, p, k)
		switch {
		case out.fail != nil:
			return out
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
		return failed(unsafeConversion(t, c))
	}
	if pending {
		return pendingElements(results, least, d.elem, p, false)
	}
	elem, f := elementType(results, d.elem, p, false)
	if f != nil {
		return failed(f)
	}
	switch d.kind {
	case ConstraintListOf:
		return typeOutcome{typ: List(elem)}
	case ConstraintSetOf:
		return typeOutcome{typ: Set(elem)}
	}
	return typeOutcome{typ: Map(elem)}
}

// elementType returns the element type of a collection whose members convert
// to these types under the element constraint c: their unification, with the
// type c names where it names one, which must satisfy c. withhold leaves the
// types out of a message, where a map's keys that a redacting mark withholds
// may have named their attributes.
func elementType(types []Type, c Constraint, p Policy, withhold bool) (Type, *failure) {
	if s, ok := resultType(c); ok {
		types = append(types[:len(types):len(types)], s)
	}
	if len(types) == 0 {
		return Type{}, &failure{CodeConvertNoCommonType,
			"nothing settles an element type for " + c.String() + ": there are no members, and the constraint admits more than one type"}
	}
	elem, ok := unifyTypes(types, p)
	switch {
	case !ok && withhold:
		return Type{}, &failure{CodeConvertNoCommonType, "the members have no common type"}
	case !ok:
		return Type{}, &failure{CodeConvertNoCommonType, "the members have no common type: " + typeList(types)}
	case !Satisfies(c, elem) && withhold:
		return Type{}, &failure{CodeConvertNoCommonType, "the members' common type does not satisfy " + c.String()}
	case !Satisfies(c, elem):
		return Type{}, &failure{CodeConvertNoCommonType,
			"the members' common type " + elem.String() + " does not satisfy " + c.String()}
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

// typeList renders types for a message, each once, in order.
func typeList(types []Type) string {
	var seen []Type
	text := ""
	for _, t := range types {
		dup := false
		for _, s := range seen {
			dup = dup || s == t
		}
		if dup {
			continue
		}
		if len(seen) > 0 {
			text += ", "
		}
		seen = append(seen, t)
		text += t.String()
	}
	return shortened(text, func(s string) string { return s })
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
		return failed(noConversion(t, c))
	}
	elems := make([]Type, len(d.members))
	pending := false
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
		}
	}
	if unsafe && p == Safe {
		return failed(unsafeConversion(t, c))
	}
	if pending {
		return typeOutcome{pending: true}
	}
	return typeOutcome{typ: Tuple(elems...)}
}

// objectTypeConvert converts t to an ObjectWith constraint.
func objectTypeConvert(t Type, c Constraint, p Policy, k keys) typeOutcome {
	d, from := c.c, t.t
	switch from.kind {
	case KindObject:
	case KindMap:
		return mapObjectTypeConvert(t, c, p, k)
	default:
		return failed(noConversion(t, c))
	}
	attrs := map[string]Type{}
	pending := false
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
	return typeOutcome{typ: Object(attrs)}
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
		}
	}
	if p == Safe {
		return failed(unsafeConversion(t, c))
	}
	if pending {
		return typeOutcome{pending: true}
	}
	return typeOutcome{typ: Object(attrs)}
}

// typeConvertOneOf converts t to the first member of a OneOf constraint to
// which a conversion from t exists under the policy.
func typeConvertOneOf(t Type, c Constraint, p Policy, k keys) typeOutcome {
	m, f := oneOfMember(t, c, p)
	if f != nil {
		return failed(f)
	}
	return typeConvert(t, m, p, k)
}

// oneOfMember returns the first member of the OneOf constraint c to which a
// conversion from t exists under the policy, or the failure when none does.
func oneOfMember(t Type, c Constraint, p Policy) (Constraint, *failure) {
	for _, m := range c.c.members {
		if typeConvert(t, m, p, keysNone).fail == nil {
			return m, nil
		}
	}
	if p == Safe {
		for _, m := range c.c.members {
			if typeConvert(t, m, Unsafe, keysNone).fail == nil {
				return Constraint{}, unsafeConversion(t, c)
			}
		}
	}
	return Constraint{}, noConversion(t, c)
}

// capsuleTypeConvert converts between t and the type s where either is a
// capsule type, by the conversions a capsule type declares: the source type's
// conversion to s, and failing that the target type's conversion from t.
func capsuleTypeConvert(t, s Type, p Policy) typeOutcome {
	_, _, safe, ok := capsuleConversion(t, s)
	switch {
	case !ok:
		return failed(noConversion(t, Exactly(s)))
	case !safe && p == Safe:
		return failed(unsafeConversion(t, Exactly(s)))
	}
	return typeOutcome{typ: s}
}

// noConversion returns the failure of a conversion that does not exist.
func noConversion(t Type, c Constraint) *failure {
	return &failure{CodeConvertNoConversion, t.String() + " does not convert to " + c.String()}
}

// unsafeConversion returns the failure of a conversion that exists only under
// the unsafe policy, applied under the safe one.
func unsafeConversion(t Type, c Constraint) *failure {
	return &failure{CodeConvertUnsafe, t.String() + " converts to " + c.String() + " only unsafely, and the policy is safe"}
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
	return Object(attrs), true
}

// unifyTwo unifies two types.
func unifyTwo(a, b Type, p Policy) (Type, bool) {
	if a == b {
		return a, true
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
			return List(elem), true
		}
	case (ka == KindList || ka == KindSet) && kb == KindTuple:
		if elem, ok := unifyTypes(append([]Type{a.t.elem}, b.t.elems...), p); ok {
			return List(elem), true
		}
	case ka == KindTuple && kb == KindTuple:
		return unifyTuples(a, b, p)
	case ka == KindMap && kb == KindObject:
		members := []Type{a.t.elem}
		for _, attr := range b.t.attrs {
			members = append(members, attr.typ)
		}
		if elem, ok := unifyTypes(members, p); ok {
			return Map(elem), true
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
			return List(elem), true
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
	return Tuple(elems...), true
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
	return Object(attrs), true
}
