package tenon

import (
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"tenon/internal/uni"
)

// mapEntry is one entry of a map value.
type mapEntry struct {
	key string // normalized
	val Value
}

// ListVal returns the list with element type elem and the given elements, in
// order. ListVal does not retain the slice. It panics if elem is the zero Type
// or an element is not a resolved value of type elem.
func ListVal(elem Type, elems ...Value) Value {
	return sequenceValue(List(elem), "ListVal", elems)
}

// SetVal returns the set with element type elem and the given members. It does
// not yet remove members that equal one another. SetVal does not retain the
// slice, and panics as ListVal does.
func SetVal(elem Type, elems ...Value) Value {
	return sequenceValue(Set(elem), "SetVal", elems)
}

// sequenceValue returns the value of list or set type t with the given
// elements.
func sequenceValue(t Type, fn string, elems []Value) Value {
	for i, e := range elems {
		requireResolved(fn, "element "+strconv.Itoa(i), e, t.t.elem)
	}
	return Value{&node{state: stateResolved, typ: t, data: slices.Clone(elems)}}
}

// TupleVal returns the tuple with the given elements, in order, whose type is
// the tuple type of the elements' types. TupleVal does not retain the slice. It
// panics if an element is not a resolved value.
func TupleVal(elems ...Value) Value {
	types := make([]Type, len(elems))
	for i, e := range elems {
		types[i] = resolvedType("TupleVal", "element "+strconv.Itoa(i), e)
	}
	return Value{&node{state: stateResolved, typ: Tuple(types...), data: slices.Clone(elems)}}
}

// ObjectVal returns the object with the given attributes, whose type is the
// object type of the attributes' types. Attribute names follow the rules of
// Object. ObjectVal does not retain the map. It panics if a name is empty or
// not valid UTF-8, if two names are the same name, or if an attribute is not a
// resolved value.
func ObjectVal(attrs map[string]Value) Value {
	entries := attributeEntries(attrs, "object attribute")
	types := make(map[string]Type, len(entries))
	vals := make([]Value, len(entries))
	for i, e := range entries {
		types[e.name] = resolvedType("ObjectVal", "attribute "+quoted(e.original), e.value)
		vals[i] = e.value
	}
	return Value{&node{state: stateResolved, typ: Object(types), data: vals}}
}

// MapVal returns the map with element type elem and the given entries. Keys are
// normalized to Unicode Normalization Form C. MapVal does not retain the map.
//
// If a key is not well-formed UTF-8, or keys are the same key after
// normalization, the result is an error value with a diagnostic for each
// problem: code CodeStringInvalidUTF8 for each such key, then code
// CodeMapDuplicateKey for each group of keys that normalize alike. MapVal panics
// if elem is the zero Type or an element is not a resolved value of type elem.
func MapVal(elem Type, entries map[string]Value) Value {
	t := Map(elem)
	type keyed struct {
		mapEntry
		original string
	}
	var diags []Diagnostic
	list := make([]keyed, 0, len(entries))
	for _, key := range slices.Sorted(maps.Keys(entries)) {
		val := entries[key]
		requireResolved("MapVal", "the element of key "+quotedASCII(key), val, elem)
		normalized, err := uni.Canonical(key)
		if err != nil {
			diags = append(diags, Diagnostic{
				Code:    CodeStringInvalidUTF8,
				Message: "map key " + quotedASCII(key) + " is not well-formed UTF-8 at byte " + strconv.Itoa(invalidUTF8At(key)),
			})
			continue
		}
		list = append(list, keyed{mapEntry{normalized, val}, key})
	}
	slices.SortStableFunc(list, func(a, b keyed) int { return strings.Compare(a.key, b.key) })
	out := make([]mapEntry, 0, len(list))
	for i := 0; i < len(list); {
		j := i + 1
		for j < len(list) && list[j].key == list[i].key {
			j++
		}
		if j-i > 1 {
			spellings := make([]string, j-i)
			for k, e := range list[i:j] {
				spellings[k] = quotedASCII(e.original)
			}
			diags = append(diags, Diagnostic{
				Code:    CodeMapDuplicateKey,
				Message: "map keys " + strings.Join(spellings, " and ") + " are the same key after normalization",
			})
		}
		out = append(out, list[i].mapEntry)
		i = j
	}
	if len(diags) > 0 {
		return errorValue(diags...)
	}
	return Value{&node{state: stateResolved, typ: t, data: out}}
}

// resolvedType returns the type of v, panicking if v is not a resolved value.
// fn and what name the caller and the member for the message.
func resolvedType(fn, what string, v Value) Type {
	n := v.data()
	if n.state != stateResolved {
		usagePanic("%s: %s is %s, not a resolved value", fn, what, n.describe())
	}
	return n.typ
}

// requireResolved panics unless v is a resolved value of type want.
func requireResolved(fn, what string, v Value, want Type) {
	if got := resolvedType(fn, what, v); got != want {
		usagePanic("%s: %s has type %s, not %s", fn, what, got, want)
	}
}

// Len returns the number of elements of a list, set or tuple, the number of
// entries of a map, or the number of attributes of an object. It panics for
// other values.
func (v Value) Len() int {
	n := v.data()
	if n.state == stateResolved {
		switch n.typ.t.kind {
		case KindList, KindSet, KindTuple, KindObject:
			return len(n.data.([]Value))
		case KindMap:
			return len(n.data.([]mapEntry))
		}
	}
	usagePanic("Len called on %s, not a collection or structural value", n.describe())
	return 0
}

// Index returns element i of a list or tuple. It panics if v is neither, or i
// is out of range.
func (v Value) Index(i int) Value {
	n := v.data()
	if n.state != stateResolved || (n.typ.t.kind != KindList && n.typ.t.kind != KindTuple) {
		usagePanic("Index called on %s, not a list or tuple value", n.describe())
	}
	elems := n.data.([]Value)
	if i < 0 || i >= len(elems) {
		usagePanic("Index(%d) called on a value with %d elements", i, len(elems))
	}
	return elems[i]
}

// Elements returns the elements of a list, set or tuple in order, in a new
// slice. It panics for other values.
func (v Value) Elements() []Value {
	n := v.data()
	if n.state == stateResolved {
		switch n.typ.t.kind {
		case KindList, KindSet, KindTuple:
			return slices.Clone(n.data.([]Value))
		}
	}
	usagePanic("Elements called on %s, not a list, set or tuple value", n.describe())
	return nil
}

// MapKeys returns the keys of a map value in sorted order, in a new slice. It
// panics if v is not a map value.
func (v Value) MapKeys() []string {
	entries := v.known(KindMap, "MapKeys").data.([]mapEntry)
	keys := make([]string, len(entries))
	for i, e := range entries {
		keys[i] = e.key
	}
	return keys
}

// MapElement returns the element of a map value with the given key, which is
// normalized before the lookup, and whether there is one. It panics if v is not
// a map value.
func (v Value) MapElement(key string) (Value, bool) {
	entries := v.known(KindMap, "MapElement").data.([]mapEntry)
	e, ok := findName(entries, key, func(e mapEntry) string { return e.key })
	return e.val, ok
}

// Attribute returns the attribute of an object value with the given name, which
// is normalized before the lookup. It panics if v is not an object value or has
// no such attribute.
func (v Value) Attribute(name string) Value {
	n := v.known(KindObject, "Attribute")
	if utf8.ValidString(name) {
		i, found := slices.BinarySearchFunc(n.typ.t.attrs, uni.NFC(name), func(a attribute, s string) int {
			return strings.Compare(a.name, s)
		})
		if found {
			return n.data.([]Value)[i]
		}
	}
	usagePanic("Attribute called on %s, which has no attribute %s", n.describe(), quoted(name))
	return Value{}
}

// writeContainer writes the content of a resolved collection or structural
// value.
func (n *node) writeContainer(b *strings.Builder) {
	switch n.typ.t.kind {
	case KindList, KindSet:
		n.typ.write(b)
		writeElements(b, n.data.([]Value))
	case KindTuple:
		writeElements(b, n.data.([]Value))
	case KindMap:
		n.typ.write(b)
		b.WriteByte('{')
		for i, e := range n.data.([]mapEntry) {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(strconv.Quote(e.key))
			b.WriteString(": ")
			e.val.write(b)
		}
		b.WriteByte('}')
	case KindObject:
		b.WriteByte('{')
		for i, val := range n.data.([]Value) {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(strconv.Quote(n.typ.t.attrs[i].name))
			b.WriteString(": ")
			val.write(b)
		}
		b.WriteByte('}')
	}
}

// writeElements writes elements in brackets, separated by commas.
func writeElements(b *strings.Builder, elems []Value) {
	b.WriteByte('[')
	for i, e := range elems {
		if i > 0 {
			b.WriteString(", ")
		}
		e.write(b)
	}
	b.WriteByte(']')
}
