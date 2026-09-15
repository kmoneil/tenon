package tenon

import (
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"tenon/internal/uni"
)

// Kind identifies the kind of a Type.
type Kind uint8

// The kinds of type, declared in the canonical order of kinds.
const (
	KindBool Kind = iota + 1
	KindNumber
	KindString
	KindList
	KindSet
	KindMap
	KindTuple
	KindObject
	KindCapsule
)

var kindNames = [...]string{
	KindBool:    "Bool",
	KindNumber:  "Number",
	KindString:  "String",
	KindList:    "List",
	KindSet:     "Set",
	KindMap:     "Map",
	KindTuple:   "Tuple",
	KindObject:  "Object",
	KindCapsule: "Capsule",
}

// String returns the name of the kind, such as "List".
func (k Kind) String() string {
	if k >= KindBool && k <= KindCapsule {
		return kindNames[k]
	}
	return "Kind(" + strconv.Itoa(int(k)) + ")"
}

// Type describes what a value is. A type is always fully concrete, and it is
// immutable.
//
// The zero Type is not a type: every method except String panics when called
// on it. Compare types with Equals.
type Type struct {
	t *typeData
}

// typeData is the immutable description that a Type refers to.
type typeData struct {
	kind  Kind
	elem  Type        // the element type of a List, Set or Map
	attrs []attribute // the attributes of an Object, sorted by name
	elems []Type      // the element types of a Tuple
}

// attribute is one attribute of an object type.
type attribute struct {
	name string
	typ  Type
}

var (
	boolType   = &typeData{kind: KindBool}
	numberType = &typeData{kind: KindNumber}
	stringType = &typeData{kind: KindString}
)

// BoolType returns the type Bool.
func BoolType() Type { return Type{boolType} }

// NumberType returns the type Number.
func NumberType() Type { return Type{numberType} }

// StringType returns the type String.
func StringType() Type { return Type{stringType} }

// List returns the type of lists whose elements have type elem.
func List(elem Type) Type { return collection(KindList, elem) }

// Set returns the type of sets whose elements have type elem.
func Set(elem Type) Type { return collection(KindSet, elem) }

// Map returns the type of maps whose elements have type elem. The keys of a
// map are always strings, normalized to Unicode Normalization Form C when the
// map is constructed.
func Map(elem Type) Type { return collection(KindMap, elem) }

func collection(kind Kind, elem Type) Type {
	if elem.t == nil {
		usagePanic("the element type of a %s type is the zero Type", kind)
	}
	return Type{&typeData{kind: kind, elem: elem}}
}

// Object returns the object type with the given attributes. Attribute names
// are normalized to Unicode Normalization Form C, and two names are the same
// name when they are identical after normalization. Object does not retain
// the map.
//
// Object panics if a name is empty or not valid UTF-8, if two names in the map
// are the same name, or if an attribute type is the zero Type.
func Object(attrs map[string]Type) Type {
	type named struct {
		original string
		attribute
	}
	list := make([]named, 0, len(attrs))
	for _, name := range slices.Sorted(maps.Keys(attrs)) {
		typ := attrs[name]
		if typ.t == nil {
			usagePanic("the type of object attribute %q is the zero Type", name)
		}
		list = append(list, named{name, attribute{attributeName(name), typ}})
	}
	slices.SortStableFunc(list, func(a, b named) int { return strings.Compare(a.name, b.name) })
	out := make([]attribute, len(list))
	for i, n := range list {
		if i > 0 && n.name == list[i-1].name {
			usagePanic("object attribute names %q and %q are the same name after normalization", list[i-1].original, n.original)
		}
		out[i] = n.attribute
	}
	return Type{&typeData{kind: KindObject, attrs: out}}
}

// attributeName returns the normalized form of an object attribute name,
// panicking if name cannot be one.
func attributeName(name string) string {
	if name == "" {
		usagePanic("an object attribute name must not be empty")
	}
	if !utf8.ValidString(name) {
		usagePanic("object attribute name %q is not valid UTF-8", name)
	}
	return uni.NFC(name)
}

// Tuple returns the tuple type whose elements have the given types, in order.
// Tuple does not retain the slice.
func Tuple(elems ...Type) Type {
	for i, e := range elems {
		if e.t == nil {
			usagePanic("the type of tuple element %d is the zero Type", i)
		}
	}
	return Type{&typeData{kind: KindTuple, elems: slices.Clone(elems)}}
}

// data returns the description of t, panicking if t is the zero Type.
func (t Type) data() *typeData {
	if t.t == nil {
		usagePanic("use of the zero Type")
	}
	return t.t
}

// mustKind returns the description of t, panicking if t is not of the given
// kind.
func (t Type) mustKind(kind Kind, method string) *typeData {
	d := t.data()
	if d.kind != kind {
		usagePanic("%s called on %s, whose kind is %s, not %s", method, t, d.kind, kind)
	}
	return d
}

// Kind returns the kind of t.
func (t Type) Kind() Kind { return t.data().kind }

// IsCollection reports whether t is a collection type: a List, Set or Map,
// whose elements all have the same type.
func (t Type) IsCollection() bool {
	switch t.Kind() {
	case KindList, KindSet, KindMap:
		return true
	}
	return false
}

// IsStructural reports whether t is a structural type: an Object or Tuple,
// whose members may have differing types.
func (t Type) IsStructural() bool {
	k := t.Kind()
	return k == KindObject || k == KindTuple
}

// ElementType returns the element type of a collection type. It panics if t
// is not a collection type.
func (t Type) ElementType() Type {
	if !t.IsCollection() {
		usagePanic("ElementType called on %s, whose kind is %s, not a collection kind", t, t.t.kind)
	}
	return t.t.elem
}

// AttributeNames returns the attribute names of an object type in sorted
// order, in a new slice. It panics if t is not an object type.
func (t Type) AttributeNames() []string {
	d := t.mustKind(KindObject, "AttributeNames")
	names := make([]string, len(d.attrs))
	for i, a := range d.attrs {
		names[i] = a.name
	}
	return names
}

// HasAttribute reports whether an object type has an attribute of the given
// name, which is normalized before the lookup. It panics if t is not an
// object type.
func (t Type) HasAttribute(name string) bool {
	_, ok := t.mustKind(KindObject, "HasAttribute").attribute(name)
	return ok
}

// AttributeType returns the type of the named attribute of an object type.
// The name is normalized before the lookup. AttributeType panics if t is not
// an object type or has no such attribute.
func (t Type) AttributeType(name string) Type {
	typ, ok := t.mustKind(KindObject, "AttributeType").attribute(name)
	if !ok {
		usagePanic("%s has no attribute %q", t, name)
	}
	return typ
}

// attribute looks up the attribute of an object type with the given name,
// after normalizing it.
func (d *typeData) attribute(name string) (Type, bool) {
	if !utf8.ValidString(name) {
		return Type{}, false
	}
	i, found := slices.BinarySearchFunc(d.attrs, uni.NFC(name), func(a attribute, name string) int {
		return strings.Compare(a.name, name)
	})
	if !found {
		return Type{}, false
	}
	return d.attrs[i].typ, true
}

// TupleLength returns the number of elements of a tuple type. It panics if t
// is not a tuple type.
func (t Type) TupleLength() int {
	return len(t.mustKind(KindTuple, "TupleLength").elems)
}

// TupleElementType returns the type of element i of a tuple type. It panics if
// t is not a tuple type or i is out of range.
func (t Type) TupleElementType(i int) Type {
	d := t.mustKind(KindTuple, "TupleElementType")
	if i < 0 || i >= len(d.elems) {
		usagePanic("TupleElementType(%d) called on %s, which has %d elements", i, t, len(d.elems))
	}
	return d.elems[i]
}

// TupleElementTypes returns the element types of a tuple type in order, in a
// new slice. It panics if t is not a tuple type.
func (t Type) TupleElementTypes() []Type {
	return slices.Clone(t.mustKind(KindTuple, "TupleElementTypes").elems)
}

// Equals reports whether t and u are the same type. Types of the same
// structure are the same type.
func (t Type) Equals(u Type) bool {
	a, b := t.data(), u.data()
	if a == b {
		return true
	}
	if a.kind != b.kind {
		return false
	}
	switch a.kind {
	case KindList, KindSet, KindMap:
		return a.elem.Equals(b.elem)
	case KindObject:
		return slices.EqualFunc(a.attrs, b.attrs, func(x, y attribute) bool {
			return x.name == y.name && x.typ.Equals(y.typ)
		})
	case KindTuple:
		return slices.EqualFunc(a.elems, b.elems, Type.Equals)
	}
	// The primitive types are singletons, so a == b has decided them.
	return false
}

// String describes t for messages, as in
// object({"name": string, "tags": list(string)}). It is not a format for
// parsing.
func (t Type) String() string {
	if t.t == nil {
		return "<zero Type>"
	}
	var b strings.Builder
	t.write(&b)
	return b.String()
}

func (t Type) write(b *strings.Builder) {
	d := t.t
	switch d.kind {
	case KindBool, KindNumber, KindString:
		b.WriteString(strings.ToLower(d.kind.String()))
	case KindList, KindSet, KindMap:
		b.WriteString(strings.ToLower(d.kind.String()))
		b.WriteByte('(')
		d.elem.write(b)
		b.WriteByte(')')
	case KindObject:
		b.WriteString("object({")
		for i, a := range d.attrs {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(strconv.Quote(a.name))
			b.WriteString(": ")
			a.typ.write(b)
		}
		b.WriteString("})")
	case KindTuple:
		b.WriteString("tuple([")
		for i, e := range d.elems {
			if i > 0 {
				b.WriteString(", ")
			}
			e.write(b)
		}
		b.WriteString("])")
	}
}
