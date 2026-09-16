package tenon

import (
	"slices"
	"strconv"
	"strings"
)

// ConstraintKind identifies the kind of a Constraint.
type ConstraintKind uint8

// The kinds of constraint.
const (
	ConstraintExactly ConstraintKind = iota + 1
	ConstraintAny
	ConstraintListOf
	ConstraintSetOf
	ConstraintMapOf
	ConstraintObjectWith
	ConstraintTupleOf
	ConstraintOneOf
)

var constraintKindNames = [...]string{
	ConstraintExactly:    "Exactly",
	ConstraintAny:        "Any",
	ConstraintListOf:     "ListOf",
	ConstraintSetOf:      "SetOf",
	ConstraintMapOf:      "MapOf",
	ConstraintObjectWith: "ObjectWith",
	ConstraintTupleOf:    "TupleOf",
	ConstraintOneOf:      "OneOf",
}

// String returns the name of the kind, such as "ListOf".
func (k ConstraintKind) String() string {
	if k >= ConstraintExactly && k <= ConstraintOneOf {
		return constraintKindNames[k]
	}
	return "ConstraintKind(" + strconv.Itoa(int(k)) + ")"
}

// Constraint describes which types are acceptable, as the target of a
// conversion, a parameter declaration or a schema does. Unlike a type, a
// constraint may leave parts unspecified. Constraints are immutable.
//
// The zero Constraint is not a constraint: every method except String panics
// when called on it.
type Constraint struct {
	c *constraintData
}

// constraintData is the immutable description that a Constraint refers to.
type constraintData struct {
	kind    ConstraintKind // the kind of the constraint
	typ     Type           // the type of Exactly
	elem    Constraint     // the element constraint of ListOf, SetOf and MapOf
	fields  []field        // the fields of ObjectWith, sorted by name
	closed  bool           // whether ObjectWith is closed
	members []Constraint   // the members of TupleOf and OneOf, in order
}

// Field constrains one attribute of an object: the constraint its type must
// satisfy, and whether the attribute must be present.
type Field struct {
	Constraint Constraint
	Required   bool
}

// field is a field of an ObjectWith constraint, with its normalized name.
type field struct {
	name string
	Field
}

var anyConstraint = &constraintData{kind: ConstraintAny}

// Required returns the field of an attribute that must be present and whose
// type must satisfy c.
func Required(c Constraint) Field { return Field{Constraint: c, Required: true} }

// Optional returns the field of an attribute that may be absent, and whose
// type must satisfy c when it is present.
func Optional(c Constraint) Field { return Field{Constraint: c} }

// Exactly returns the constraint that t satisfies and no other type does.
func Exactly(t Type) Constraint {
	if t.t == nil {
		usagePanic("Exactly of the zero Type")
	}
	return Constraint{&constraintData{kind: ConstraintExactly, typ: t}}
}

// Any returns the constraint that every type satisfies.
func Any() Constraint { return Constraint{anyConstraint} }

// ListOf returns the constraint satisfied by the list types whose element type
// satisfies elem.
func ListOf(elem Constraint) Constraint { return elementConstraint(ConstraintListOf, elem) }

// SetOf returns the constraint satisfied by the set types whose element type
// satisfies elem.
func SetOf(elem Constraint) Constraint { return elementConstraint(ConstraintSetOf, elem) }

// MapOf returns the constraint satisfied by the map types whose element type
// satisfies elem.
func MapOf(elem Constraint) Constraint { return elementConstraint(ConstraintMapOf, elem) }

func elementConstraint(kind ConstraintKind, elem Constraint) Constraint {
	if elem.c == nil {
		usagePanic("the element constraint of %s is the zero Constraint", kind)
	}
	return Constraint{&constraintData{kind: kind, elem: elem}}
}

// ObjectWith returns the constraint satisfied by the object types whose
// attributes match fields: every required field names an attribute, and every
// attribute that a field names has a type satisfying the field's constraint.
// A closed constraint also rejects object types with attributes that no field
// names; an open one accepts them, whatever their types.
//
// Field names follow the rules for attribute names in Object. ObjectWith
// panics in the same cases as Object, or if a field's constraint is the zero
// Constraint. It does not retain the map.
func ObjectWith(fields map[string]Field, closed bool) Constraint {
	entries := attributeEntries(fields, "object field")
	list := make([]field, len(entries))
	for i, e := range entries {
		if e.value.Constraint.c == nil {
			usagePanic("the constraint of object field %q is the zero Constraint", e.original)
		}
		list[i] = field{e.name, e.value}
	}
	return Constraint{&constraintData{kind: ConstraintObjectWith, fields: list, closed: closed}}
}

// TupleOf returns the constraint satisfied by the tuple types with one element
// per member, where the type of each element satisfies the member in its
// position. TupleOf does not retain the slice.
func TupleOf(members ...Constraint) Constraint {
	return membersConstraint(ConstraintTupleOf, members)
}

// OneOf returns the constraint satisfied by the types that satisfy at least
// one of members, so OneOf with no members is satisfied by no type. OneOf does
// not retain the slice.
func OneOf(members ...Constraint) Constraint {
	return membersConstraint(ConstraintOneOf, members)
}

func membersConstraint(kind ConstraintKind, members []Constraint) Constraint {
	for i, m := range members {
		if m.c == nil {
			usagePanic("member %d of %s is the zero Constraint", i, kind)
		}
	}
	return Constraint{&constraintData{kind: kind, members: slices.Clone(members)}}
}

// Satisfies reports whether t satisfies c. It is defined for every constraint
// and type, and always terminates. It panics only if c or t is a zero value,
// which is neither a constraint nor a type.
func Satisfies(c Constraint, t Type) bool {
	cd, td := c.data(), t.data()
	switch cd.kind {
	case ConstraintAny:
		return true
	case ConstraintExactly:
		return cd.typ.t == td
	case ConstraintListOf:
		return td.kind == KindList && Satisfies(cd.elem, td.elem)
	case ConstraintSetOf:
		return td.kind == KindSet && Satisfies(cd.elem, td.elem)
	case ConstraintMapOf:
		return td.kind == KindMap && Satisfies(cd.elem, td.elem)
	case ConstraintObjectWith:
		return td.kind == KindObject && satisfiesFields(cd, td.attrs)
	case ConstraintTupleOf:
		return td.kind == KindTuple && slices.EqualFunc(cd.members, td.elems, Satisfies)
	case ConstraintOneOf:
		return slices.ContainsFunc(cd.members, func(m Constraint) bool { return Satisfies(m, t) })
	}
	return false
}

// satisfiesFields reports whether attrs, the attributes of an object type,
// match the ObjectWith constraint d. Both lists are sorted by name, so a single
// merging pass compares them.
func satisfiesFields(d *constraintData, attrs []attribute) bool {
	fields := d.fields
	for len(attrs) > 0 || len(fields) > 0 {
		switch {
		case len(fields) == 0 || (len(attrs) > 0 && attrs[0].name < fields[0].name):
			// An attribute that no field names.
			if d.closed {
				return false
			}
			attrs = attrs[1:]
		case len(attrs) == 0 || fields[0].name < attrs[0].name:
			// A field that names no attribute.
			if fields[0].Required {
				return false
			}
			fields = fields[1:]
		default:
			if !Satisfies(fields[0].Constraint, attrs[0].typ) {
				return false
			}
			attrs, fields = attrs[1:], fields[1:]
		}
	}
	return true
}

// data returns the description of c, panicking if c is the zero Constraint.
func (c Constraint) data() *constraintData {
	if c.c == nil {
		usagePanic("use of the zero Constraint")
	}
	return c.c
}

// mustKind returns the description of c, panicking unless c has one of the
// given kinds.
func (c Constraint) mustKind(method string, kinds ...ConstraintKind) *constraintData {
	d := c.data()
	if !slices.Contains(kinds, d.kind) {
		usagePanic("%s called on %s, whose kind is %s", method, c, d.kind)
	}
	return d
}

// Equal reports whether c and d are the same constraint: of one kind, built
// from the same types, members and fields, in the same order where order
// counts. Unify writes its results canonically, so two of them that constrain
// alike are Equal. Equal panics if either is the zero Constraint.
func (c Constraint) Equal(d Constraint) bool {
	c.data()
	d.data()
	return c.equal(d)
}

// Kind returns the kind of c.
func (c Constraint) Kind() ConstraintKind { return c.data().kind }

// Type returns the type of an Exactly constraint. It panics for other kinds.
func (c Constraint) Type() Type { return c.mustKind("Type", ConstraintExactly).typ }

// Element returns the element constraint of a ListOf, SetOf or MapOf
// constraint. It panics for other kinds.
func (c Constraint) Element() Constraint {
	return c.mustKind("Element", ConstraintListOf, ConstraintSetOf, ConstraintMapOf).elem
}

// FieldNames returns the field names of an ObjectWith constraint in sorted
// order, in a new slice. It panics for other kinds.
func (c Constraint) FieldNames() []string {
	d := c.mustKind("FieldNames", ConstraintObjectWith)
	names := make([]string, len(d.fields))
	for i, f := range d.fields {
		names[i] = f.name
	}
	return names
}

// Field returns the field of an ObjectWith constraint with the given name,
// which is normalized before the lookup, and whether there is one. It panics
// for other kinds.
func (c Constraint) Field(name string) (Field, bool) {
	f, ok := findName(c.mustKind("Field", ConstraintObjectWith).fields, name, func(f field) string { return f.name })
	return f.Field, ok
}

// Closed reports whether an ObjectWith constraint is closed. It panics for
// other kinds.
func (c Constraint) Closed() bool { return c.mustKind("Closed", ConstraintObjectWith).closed }

// Members returns the members of a TupleOf or OneOf constraint in order, in a
// new slice. It panics for other kinds.
func (c Constraint) Members() []Constraint {
	return slices.Clone(c.mustKind("Members", ConstraintTupleOf, ConstraintOneOf).members)
}

// String describes c for messages, as in
// object_with({"name": exactly(string), "tags"?: list_of(any)}, closed), where
// ? marks an optional field. It is not a format for parsing.
func (c Constraint) String() string {
	if c.c == nil {
		return "<zero Constraint>"
	}
	var b strings.Builder
	c.write(&b)
	return b.String()
}

func (c Constraint) write(b *strings.Builder) {
	d := c.c
	switch d.kind {
	case ConstraintAny:
		b.WriteString("any")
	case ConstraintExactly:
		b.WriteString("exactly(")
		d.typ.write(b)
		b.WriteByte(')')
	case ConstraintListOf, ConstraintSetOf, ConstraintMapOf:
		switch d.kind {
		case ConstraintListOf:
			b.WriteString("list_of(")
		case ConstraintSetOf:
			b.WriteString("set_of(")
		default:
			b.WriteString("map_of(")
		}
		d.elem.write(b)
		b.WriteByte(')')
	case ConstraintObjectWith:
		b.WriteString("object_with({")
		for i, f := range d.fields {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(strconv.Quote(f.name))
			if !f.Required {
				b.WriteByte('?')
			}
			b.WriteString(": ")
			f.Constraint.write(b)
		}
		if d.closed {
			b.WriteString("}, closed)")
		} else {
			b.WriteString("}, open)")
		}
	case ConstraintTupleOf, ConstraintOneOf:
		if d.kind == ConstraintTupleOf {
			b.WriteString("tuple_of([")
		} else {
			b.WriteString("one_of([")
		}
		for i, m := range d.members {
			if i > 0 {
				b.WriteString(", ")
			}
			m.write(b)
		}
		b.WriteString("])")
	}
}
