package tenon

import (
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/kmoneil/tenon/internal/uni"
)

// mapEntry is one entry of a map value.
type mapEntry struct {
	key string // normalized
	val Value
}

// containerErrors collects the diagnostics of the error members of a container
// under construction, each located within the container, with exact duplicates
// dropped, and the Propagate marks of those members, which survive into the
// error value that takes the container's place. Members that are not errors
// keep their marks to themselves: building a container is not an operation
// over its members, and the error value says nothing of them.
type containerErrors struct {
	diags []Diagnostic
	marks []Mark
	seen  diagnosticLookup
}

// add records the diagnostics of an error member that step locates within the
// container, and its marks.
func (c *containerErrors) add(step Step, member Value) {
	for _, d := range member.n.data.([]Diagnostic) {
		d.Path = d.Path.prepend(step)
		c.addDiagnostic(d)
	}
	c.addMarks(member)
}

// addUnlocated records the diagnostics of an error member that the container
// cannot locate, leaving their paths as they are, and its marks.
func (c *containerErrors) addUnlocated(member Value) {
	for _, d := range member.n.data.([]Diagnostic) {
		c.addDiagnostic(d)
	}
	c.addMarks(member)
}

// addMarks records the Propagate marks of an error member.
func (c *containerErrors) addMarks(member Value) {
	for _, m := range member.n.markList() {
		if m.Propagation() == Propagate && !slices.Contains(c.marks, m) {
			c.marks = append(c.marks, m)
		}
	}
}

// addDiagnostic records d unless it is already there.
func (c *containerErrors) addDiagnostic(d Diagnostic) {
	if !c.seen.holds(c.diags, d) {
		c.diags = append(c.diags, d)
	}
}

// value returns the error value for what was collected, and whether there was
// anything.
func (c *containerErrors) value() (Value, bool) {
	if len(c.diags) == 0 {
		return Value{}, false
	}
	return WithMarks(errorValue(c.diags...), c.marks...), true
}

// isError reports whether v is an error value, for constructors sorting their
// members.
func isError(v Value) bool { return v.data().state == stateError }

// ListVal returns the list with element type elem and the given elements, in
// order. ListVal does not retain the slice.
//
// If an element is an error value the result is an error value carrying the
// diagnostics of every such element, each located by its index, and the
// Propagate marks of those elements. ListVal panics if elem is the zero Type,
// or an element is neither an error value nor a resolved value of type elem.
func ListVal(elem Type, elems ...Value) Value {
	return sequenceValue(List(elem), "ListVal", elems)
}

// SetVal returns the set with element type elem and the given members. Equality
// tells members apart: members it settles are one value are one member, of
// which the set keeps the first given, and members that are not known stay
// apart unless it settles that. A set holds its members in the order it
// iterates them. SetVal does not retain the slice, and treats error members and
// panics as ListVal does.
//
// A member must not be marked: it must carry no mark and hold none, at any
// depth. Equality tells members apart without looking at marks, so of two
// members that differ only by their marks a set would keep one and lose the
// other's marks, and which it lost would depend on the order they were given
// in. SetVal panics on a marked member rather than choose. Take the marks off
// the members, and put them on the set:
//
//	var marks []tenon.Mark
//	for i, m := range members {
//		var taken []tenon.Mark
//		members[i], taken = tenon.UnmarkDeep(m)
//		marks = append(marks, taken...)
//	}
//	set := tenon.WithMarks(tenon.SetVal(elem, members...), marks...)
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
		requireMember(fn, "element "+strconv.Itoa(i), e, t.t.elem)
		if t.t.kind == KindSet && e.n.isMarked() {
			usagePanic("%s: element %d is %s, and a set's members carry no marks; %s",
				fn, i, e.n.describeMarked(), unmarkForSet)
		}
	}
	if v, ok := errs.value(); ok {
		return v
	}
	members := slices.Clone(elems)
	if t.t.kind == KindSet {
		members = withNothingLeftToBe(t, orderMembers(distinctMembers(members)))
	}
	return Value{&node{state: stateKnown, partial: anyPartial(members), markedWithin: anyMarked(members), typ: t, data: members}}
}

// unmarkForSet is what a panic message tells a caller who gave a set a marked
// member: the way to do it that keeps the marks.
const unmarkForSet = "unmark it with UnmarkDeep and reapply the marks to the set"

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
		types[i] = memberType("TupleVal", "element "+strconv.Itoa(i), e)
	}
	if v, ok := errs.value(); ok {
		return v
	}
	return Value{&node{state: stateKnown, partial: anyPartial(elems), markedWithin: anyMarked(elems), typ: Tuple(types...), data: slices.Clone(elems)}}
}

// ObjectVal returns the object with the given attributes, whose type is the
// object type of the attributes' types. Attribute names follow the rules of
// Object. ObjectVal does not retain the map.
//
// If an attribute is an error value the result is an error value carrying the
// diagnostics of every such attribute, each located by its name, and the
// Propagate marks of those attributes. ObjectVal panics if a name is empty or
// not valid UTF-8, if two names are the same name, or if an attribute is
// neither an error value nor a resolved value.
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
		types[e.name] = memberType("ObjectVal", "attribute "+quoted(e.original), e.value)
		vals[i] = e.value
	}
	if v, ok := errs.value(); ok {
		return v
	}
	return Value{&node{state: stateKnown, partial: anyPartial(vals), markedWithin: anyMarked(vals), typ: Object(types), data: vals}}
}

// objectOf returns the object value of type t holding vals, one per attribute
// of t in its order. It is for a caller that has the type and the values it
// asks for already, where ObjectVal takes a map, normalizes its names, orders
// them and interns the type they describe, all of which t settles. Since no
// container holds an error value, and nothing here would locate one, a caller
// that has not settled its values first is a mistake in this package.
func objectOf(t Type, vals []Value) Value {
	for i, v := range vals {
		if isError(v) {
			internalPanic("objectOf: attribute %q of %s is an error value", t.t.attrs[i].name, t)
		}
	}
	return Value{&node{state: stateKnown, partial: anyPartial(vals), markedWithin: anyMarked(vals), typ: t, data: vals}}
}

// distinctMembers returns the members of a set: members that equality reports
// the same are one member, and the first of them is the one kept. Known members
// are looked up by hash, among the known members kept.
//
// A member that is not known is kept without comparing it with anything.
// [EQ-041] joins it to another member only where Equals settles the two equal,
// and equality settles two values equal only where both are known nulls or both
// are known, which a member that is not known is neither. Comparing it with
// every member kept, as this once did, could never find its double, and cost
// the square of the members: 133 ms to decode a set of 4,000 unknowns.
// TestConformance_EQ041_NoMemberThatIsNotKnownIsSettledEqual holds equality to
// that, so that a change to it that would make this wrong fails there.
func distinctMembers(members []Value) []Value {
	var kept []Value
	var buckets map[uint64][]Value
	for _, m := range members {
		if !m.n.isKnown() {
			kept = append(kept, m)
			continue
		}
		if buckets == nil {
			buckets = map[uint64][]Value{}
		}
		h := hashNode(m.n)
		if sameAsSome(buckets[h], m) {
			continue
		}
		buckets[h] = append(buckets[h], m)
		kept = append(kept, m)
	}
	return kept
}

// orderMembers puts the members of a set in the order it iterates in, which is
// the order it holds them in: the known ones in canonical order, and then the
// ones that are not known, ordered by how they read.
//
// A member's place has to follow from the member, since two sets with the same
// members are one value and one value iterates one way. Reading a value is a
// rendering of everything it says about itself, which is the same from one run
// to the next, and is all there is to go on for a member that is not known.
// Where it tells two of them apart no further, which takes a capsule value
// inside one of them whose type declares no order, they stay as they came.
func orderMembers(members []Value) []Value {
	reading := map[*node]string{}
	for _, m := range members {
		if !m.n.isKnown() {
			reading[m.n] = m.String()
		}
	}
	slices.SortStableFunc(members, func(a, b Value) int {
		known, other := a.n.isKnown(), b.n.isKnown()
		switch {
		case known && other:
			return compareValues(a, b)
		case known:
			return -1
		case other:
			return 1
		}
		return strings.Compare(reading[a.n], reading[b.n])
	})
	return members
}

// withNothingLeftToBe returns these members of a set of type t, in the order
// they are held, without those that are not known and could only be values the
// set holds already. Such a member has no value of its own left to be, so the
// set does not keep it (EQ-041), and where none is left the set is known, its
// range holding the one set (VA-003). Only an element type whose values can be
// counted and built is asked about, since deciding it means naming them.
func withNothingLeftToBe(t Type, members []Value) []Value {
	c := setCeiling(t)
	if !c.set || c.n > maxDomainSet {
		return members
	}
	known := knownMembers(members)
	if known == len(members) {
		return members
	}
	// Which values the set does not hold already is worked out once, since
	// every member that is not known is asked the same thing about them.
	missing := valuesMissing(t.t.elem, members[:known])
	kept, dropped := members[:known:known], false
	for _, m := range members[known:] {
		if couldOnlyBe(m, missing) {
			dropped = true
			continue
		}
		kept = append(kept, m)
	}
	if !dropped {
		return members
	}
	return kept
}

// valuesMissing returns the values a member of type elem can be that these
// members, which are known, do not hold. They are looked up among the members
// by hash, as a set's own construction tells its members apart, rather than
// each being compared with every member.
func valuesMissing(elem Type, known []Value) []Value {
	values := memberValues(elem)
	if len(known) == 0 {
		// A set with no known member holds none of them, so the values the
		// type keeps are the answer, and nothing is built for this set.
		return values
	}
	held := indexMembers(known)
	var missing []Value
	for _, v := range values {
		if found, _ := held.membership(v); !found {
			missing = append(missing, v)
		}
	}
	return missing
}

// couldOnlyBe reports whether every value m could turn out to be is one the
// set holds already, which is so exactly when m could be none of the values
// missing from it.
func couldOnlyBe(m Value, missing []Value) bool {
	for _, v := range missing {
		if eq, settled := equality(v.n, m.n); !settled || eq {
			return false
		}
	}
	return true
}

// sameAsSome reports whether equality settles that m is one of these members.
func sameAsSome(members []Value, m Value) bool {
	return slices.ContainsFunc(members, func(k Value) bool {
		eq, settled := equality(k.n, m.n)
		return settled && eq
	})
}

// anyPartial reports whether a container holding these members has a range
// wider than one value, which it has as soon as one member does.
func anyPartial(vals []Value) bool {
	for _, v := range vals {
		if !v.n.isKnown() {
			return true
		}
	}
	return false
}

// anyMarked reports whether a container holding these members holds a marked
// value, which it does as soon as one member is marked.
func anyMarked(vals []Value) bool {
	for _, v := range vals {
		if v.n.isMarked() {
			return true
		}
	}
	return false
}

// MapVal returns the map with element type elem and the given entries. Keys are
// normalized to Unicode Normalization Form C. MapVal does not retain the map.
//
// The result is an error value if a key is not well-formed UTF-8, if keys are
// the same key after normalization, or if an element is an error value, with a
// diagnostic for each problem, in the order of the keys, normalized where they
// are well-formed: code CodeStringInvalidUTF8 for each such key and the
// diagnostics of each error element, then code CodeMapDuplicateKey for each
// group of keys that normalize alike, whatever their elements are. The error
// value carries the Propagate marks of the error elements. MapVal panics if
// elem is the zero Type, or an element is neither an error value nor a
// resolved value of type elem.
func MapVal(elem Type, entries map[string]Value) Value {
	t := Map(elem)
	// keyed is an entry beside its key as given. Its key is the normalized
	// form, or the key as given where that is not well-formed UTF-8, which no
	// normalized key can equal.
	type keyed struct {
		mapEntry
		original string
		invalid  bool
	}
	list := make([]keyed, 0, len(entries))
	for _, key := range slices.Sorted(maps.Keys(entries)) {
		val := entries[key]
		if !isError(val) {
			requireMember("MapVal", "the element of key "+quotedASCII(key), val, elem)
		}
		normalized, err := uni.Canonical(key)
		if err != nil {
			list = append(list, keyed{mapEntry{key, val}, key, true})
			continue
		}
		list = append(list, keyed{mapEntry{normalized, val}, key, false})
	}

	// Bytewise order is string order for the normalized keys, and places the
	// others among them; entries that share a key follow their keys as given
	// (TY-017). Each entry reports in that order, and a key that entries share
	// is reported after them all, whatever their elements are.
	slices.SortFunc(list, func(a, b keyed) int {
		if c := strings.Compare(a.key, b.key); c != 0 {
			return c
		}
		return strings.Compare(a.original, b.original)
	})
	var errs containerErrors
	var shared [][]keyed
	out := make([]mapEntry, 0, len(list))
	for i := 0; i < len(list); {
		j := i + 1
		for j < len(list) && list[j].key == list[i].key {
			j++
		}
		for _, e := range list[i:j] {
			switch {
			case e.invalid:
				errs.addDiagnostic(Diagnostic{
					Code:    CodeStringInvalidUTF8,
					Message: "map key " + quotedASCII(e.original) + " is not well-formed UTF-8 at byte " + strconv.Itoa(invalidUTF8At(e.original)),
				})
				// No path step can name a key that is not a string, so an
				// error element under it keeps the path it came with.
				if isError(e.val) {
					errs.addUnlocated(e.val)
				}
			case isError(e.val):
				errs.add(indexStep(String(e.key)), e.val)
			}
		}
		if j-i > 1 {
			shared = append(shared, list[i:j])
		}
		out = append(out, list[i].mapEntry)
		i = j
	}
	for _, group := range shared {
		spellings := make([]string, len(group))
		for k, e := range group {
			spellings[k] = quotedASCII(e.original)
		}
		errs.addDiagnostic(Diagnostic{
			Code:    CodeMapDuplicateKey,
			Message: "map keys " + strings.Join(spellings, " and ") + " are the same key after normalization",
		})
	}
	if v, ok := errs.value(); ok {
		return v
	}
	partial, marked := false, false
	for _, e := range out {
		partial = partial || !e.val.n.isKnown()
		marked = marked || e.val.n.isMarked()
	}
	return Value{&node{state: stateKnown, partial: partial, markedWithin: marked, typ: t, data: out}}
}

// memberType returns the type of a member of a container, panicking if v is not
// a resolved value. A member that is null or unknown has a type all the same,
// and the container holds it: what a member leaves open is the container's
// range, which partial records. fn and what name the caller and the member for
// the message.
func memberType(fn, what string, v Value) Type {
	n := v.data()
	if n.state == statePending {
		usagePanic("%s: %s is a pending value, which has no type; Resolve it to one first", fn, what)
	}
	if !n.state.resolved() {
		usagePanic("%s: %s is %s, not a resolved value", fn, what, n.describe())
	}
	return n.typ
}

// requireMember panics unless v is a resolved value of type want.
func requireMember(fn, what string, v Value, want Type) {
	if got := memberType(fn, what, v); got != want {
		usagePanic("%s: %s has type %s, not %s", fn, what, got, want)
	}
}

// Len returns the number of elements of a list, set or tuple, the number of
// entries of a map, or the number of attributes of an object. It panics for
// other values.
func (v Value) Len() int {
	n := v.data()
	n.noContent("Len")
	if n.state == stateKnown {
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
	n.noContent("Index")
	if n.state != stateKnown || (n.typ.t.kind != KindList && n.typ.t.kind != KindTuple) {
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
//
// A set's members carry no marks where the set holds them, so a deep mark on
// the set is attached to each member as Elements returns it, and a member
// comes out as it would out of a list carrying the mark.
func (v Value) Elements() []Value {
	n := v.data()
	n.noContent("Elements")
	if n.state == stateKnown {
		switch n.typ.t.kind {
		case KindList, KindTuple:
			return slices.Clone(n.data.([]Value))
		case KindSet:
			return n.retrievedMembers()
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
			writeQuoted(b, e.key)
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
			writeQuoted(b, n.typ.t.attrs[i].name)
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
