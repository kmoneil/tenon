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

// containerErrors collects the diagnostics of the error members of a container
// under construction, each located within the container, with exact duplicates
// dropped.
type containerErrors struct {
	diags []Diagnostic
}

// add records the diagnostics of an error member that step locates within the
// container.
func (c *containerErrors) add(step Step, member Value) {
	for _, d := range member.n.data.([]Diagnostic) {
		d.Path = d.Path.prepend(step)
		c.addDiagnostic(d)
	}
}

// addUnlocated records the diagnostics of an error member that the container
// cannot locate, leaving their paths as they are.
func (c *containerErrors) addUnlocated(member Value) {
	for _, d := range member.n.data.([]Diagnostic) {
		c.addDiagnostic(d)
	}
}

// addDiagnostic records d unless it is already there.
func (c *containerErrors) addDiagnostic(d Diagnostic) {
	if !slices.ContainsFunc(c.diags, d.Equal) {
		c.diags = append(c.diags, d)
	}
}

// value returns the error value for what was collected, and whether there was
// anything.
func (c *containerErrors) value() (Value, bool) {
	if len(c.diags) == 0 {
		return Value{}, false
	}
	return errorValue(c.diags...), true
}

// isError reports whether v is an error value, for constructors sorting their
// members.
func isError(v Value) bool { return v.data().state == stateError }

// ListVal returns the list with element type elem and the given elements, in
// order. ListVal does not retain the slice.
//
// If an element is an error value the result is an error value carrying the
// diagnostics of every such element, each located by its index. ListVal panics
// if elem is the zero Type, or an element is neither an error value nor a
// resolved value of type elem.
func ListVal(elem Type, elems ...Value) Value {
	return sequenceValue(List(elem), "ListVal", elems)
}

// SetVal returns the set with element type elem and the given members. It does
// not yet remove members that equal one another. SetVal does not retain the
// slice, and treats error members and panics as ListVal does.
func SetVal(elem Type, elems ...Value) Value {
	return sequenceValue(Set(elem), "SetVal", elems)
}

// sequenceValue returns the value of list or set type t with the given
// elements.
func sequenceValue(t Type, fn string, elems []Value) Value {
	var errs containerErrors
	for i, e := range elems {
		if isError(e) {
			errs.add(indexStep(NumberFromInt(int64(i))), e)
			continue
		}
		requireResolved(fn, "element "+strconv.Itoa(i), e, t.t.elem)
	}
	if v, ok := errs.value(); ok {
		return v
	}
	return Value{&node{state: stateResolved, typ: t, data: slices.Clone(elems)}}
}

// TupleVal returns the tuple with the given elements, in order, whose type is
// the tuple type of the elements' types. TupleVal does not retain the slice.
//
// If an element is an error value the result is an error value, as in ListVal.
// TupleVal panics if an element is neither an error value nor a resolved value.
func TupleVal(elems ...Value) Value {
	var errs containerErrors
	types := make([]Type, len(elems))
	for i, e := range elems {
		if isError(e) {
			errs.add(indexStep(NumberFromInt(int64(i))), e)
			continue
		}
		types[i] = resolvedType("TupleVal", "element "+strconv.Itoa(i), e)
	}
	if v, ok := errs.value(); ok {
		return v
	}
	return Value{&node{state: stateResolved, typ: Tuple(types...), data: slices.Clone(elems)}}
}

// ObjectVal returns the object with the given attributes, whose type is the
// object type of the attributes' types. Attribute names follow the rules of
// Object. ObjectVal does not retain the map.
//
// If an attribute is an error value the result is an error value carrying the
// diagnostics of every such attribute, each located by its name. ObjectVal
// panics if a name is empty or not valid UTF-8, if two names are the same name,
// or if an attribute is neither an error value nor a resolved value.
func ObjectVal(attrs map[string]Value) Value {
	entries := attributeEntries(attrs, "object attribute")
	var errs containerErrors
	types := make(map[string]Type, len(entries))
	vals := make([]Value, len(entries))
	for i, e := range entries {
		if isError(e.value) {
			errs.add(attributeStep(e.name), e.value)
			continue
		}
		types[e.name] = resolvedType("ObjectVal", "attribute "+quoted(e.original), e.value)
		vals[i] = e.value
	}
	if v, ok := errs.value(); ok {
		return v
	}
	return Value{&node{state: stateResolved, typ: Object(types), data: vals}}
}

// MapVal returns the map with element type elem and the given entries. Keys are
// normalized to Unicode Normalization Form C. MapVal does not retain the map.
//
// The result is an error value if a key is not well-formed UTF-8, if keys are
// the same key after normalization, or if an element is an error value, with a
// diagnostic for each problem: code CodeStringInvalidUTF8 for each such key and
// the diagnostics of each error element, in key order, then code
// CodeMapDuplicateKey for each group of keys that normalize alike. MapVal
// panics if elem is the zero Type, or an element is neither an error value nor
// a resolved value of type elem.
func MapVal(elem Type, entries map[string]Value) Value {
	t := Map(elem)
	type keyed struct {
		mapEntry
		original string
	}
	var errs containerErrors
	list := make([]keyed, 0, len(entries))
	for _, key := range slices.Sorted(maps.Keys(entries)) {
		val := entries[key]
		normalized, err := uni.Canonical(key)
		if err != nil {
			errs.addDiagnostic(Diagnostic{
				Code:    CodeStringInvalidUTF8,
				Message: "map key " + quotedASCII(key) + " is not well-formed UTF-8 at byte " + strconv.Itoa(invalidUTF8At(key)),
			})
			// No path step can name a key that is not a string, so an error
			// element under it keeps the path it came with.
			if isError(val) {
				errs.addUnlocated(val)
			} else {
				requireResolved("MapVal", "the element of key "+quotedASCII(key), val, elem)
			}
			continue
		}
		if isError(val) {
			errs.add(indexStep(String(normalized)), val)
			continue
		}
		requireResolved("MapVal", "the element of key "+quotedASCII(key), val, elem)
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
			errs.addDiagnostic(Diagnostic{
				Code:    CodeMapDuplicateKey,
				Message: "map keys " + strings.Join(spellings, " and ") + " are the same key after normalization",
			})
		}
		out = append(out, list[i].mapEntry)
		i = j
	}
	if v, ok := errs.value(); ok {
		return v
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
