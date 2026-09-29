package tenon

import (
	"bytes"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/kmoneil/tenon/internal/decimal"
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
//
// It keeps the members themselves, each with the step that locates it, and
// the error value it makes lists their diagnostics only when asked (hoisted),
// so that a diagnostic rising through many containers is not copied, with a
// path one step longer, at each of them.
type containerErrors struct {
	parts []hoistedPart
	marks propagating
}

// add records the diagnostics of an error member that step locates within the
// container, and its marks.
func (c *containerErrors) add(step Step, member Value) {
	c.parts = append(c.parts, hoistedPart{step: step, member: member.n})
	c.addMarks(member)
}

// addUnlocated records the diagnostics of an error member that the container
// cannot locate, leaving their paths as they are, and its marks.
func (c *containerErrors) addUnlocated(member Value) {
	c.parts = append(c.parts, hoistedPart{member: member.n})
	c.addMarks(member)
}

// addMarks records the Propagate marks of an error member, each shared layer
// once however many failing members share it.
func (c *containerErrors) addMarks(member Value) {
	c.marks.addSet(member.n.marks)
}

// addDiagnostic records d, a diagnostic of the container's own, as an
// unlocated member failing with d alone would be.
func (c *containerErrors) addDiagnostic(d Diagnostic) {
	c.parts = append(c.parts, hoistedPart{member: &node{state: stateError, data: []Diagnostic{d}}})
}

// value returns the error value for what was collected, and whether there was
// anything.
func (c *containerErrors) value() (Value, bool) {
	if len(c.parts) == 0 {
		return Value{}, false
	}
	return WithMarks(Value{n: &node{state: stateError, data: &hoisted{parts: c.parts}}}, c.marks.marks...), true
}

// hoisted is what an error value that a container's error members made holds
// in place of its diagnostics: those members, and any diagnostics of the
// container's own, in the order met, each member with the step that locates
// it. Locating each diagnostic as it rose would copy its path at every
// container it rose through, the square of its depth for each: a conversion
// of 2,000 failing leaves 200 levels deep allocated 6.5 GB. So the
// diagnostics are listed once, when first asked for, each path built from
// the outermost step in, the diagnostics beneath one step sharing it.
type hoisted struct {
	parts []hoistedPart
	once  sync.Once
	diags []Diagnostic
}

// hoistedPart is one part of a hoisted error: an error member and the step
// that locates it, the zero Step where nothing does. A diagnostic of the
// container's own is a member failing with it alone, unlocated.
type hoistedPart struct {
	step   Step
	member *node
}

// list returns the diagnostics, listing them on the first call.
func (h *hoisted) list() []Diagnostic {
	h.once.Do(func() {
		var f flattening
		f.parts(h.parts, nil)
		h.diags = f.out
	})
	return h.diags
}

// flattening lists the diagnostics of a hoisted error. Each path it builds is
// built once, one node for each path however many diagnostics sit on it, so
// that two diagnostics are the same exactly when their code, message and
// path node are: dropping the later of each such pair once, here, drops what
// dropping it at every container on the way up did, since the step a
// container puts before two paths leaves them the same or not as it found
// them.
type flattening struct {
	out   []Diagnostic
	nodes map[pathNodeKey]*pathNode
	seen  map[diagnosticAt]bool
}

// pathNodeKey identifies a path by the node of the path before its last step
// and that step's kind and text: a key is a String or Number value without
// marks, written as its kind and its string or canonical text.
type pathNodeKey struct {
	parent *pathNode
	kind   StepKind
	text   string
}

// diagnosticAt identifies a diagnostic by its code, its message and the one
// node flattening builds for its path.
type diagnosticAt struct {
	code    Code
	message string
	at      *pathNode
}

// parts lists the diagnostics of parts, located beneath at.
func (f *flattening) parts(parts []hoistedPart, at *pathNode) {
	for _, p := range parts {
		if p.step.kind == 0 {
			f.member(p.member, at)
		} else {
			f.member(p.member, f.node(at, p.step))
		}
	}
}

// member lists the diagnostics of the error member n, located beneath at.
func (f *flattening) member(n *node, at *pathNode) {
	if h, ok := n.data.(*hoisted); ok {
		f.parts(h.parts, at)
		return
	}
	for _, d := range n.data.([]Diagnostic) {
		f.add(d, at)
	}
}

// add lists d, its path located beneath at, unless it is listed already.
func (f *flattening) add(d Diagnostic, at *pathNode) {
	for _, s := range d.Path.Steps() {
		at = f.node(at, s)
	}
	key := diagnosticAt{d.Code, d.Message, at}
	if f.seen[key] {
		return
	}
	if f.seen == nil {
		f.seen = map[diagnosticAt]bool{}
	}
	f.seen[key] = true
	d.Path = Path{last: at}
	f.out = append(f.out, d)
}

// node returns the one node of the path at followed by s.
func (f *flattening) node(at *pathNode, s Step) *pathNode {
	key := pathNodeKey{parent: at, kind: s.kind, text: s.name}
	if s.kind == StepIndex {
		if s.key.n.typ.t.kind == KindString {
			key.text = "s" + s.key.n.data.(string)
		} else {
			key.text = "n" + s.key.n.data.(decimal.Dec).String()
		}
	}
	if n, ok := f.nodes[key]; ok {
		return n
	}
	n := Path{last: at}.extend(s).last
	if f.nodes == nil {
		f.nodes = map[pathNodeKey]*pathNode{}
	}
	f.nodes[key] = n
	return n
}

// diagnostics returns the diagnostics of the error node n. The result is read,
// never written to.
func (n *node) diagnostics() []Diagnostic {
	if h, ok := n.data.(*hoisted); ok {
		return h.list()
	}
	return n.data.([]Diagnostic)
}

// isError reports whether v is an error value, for constructors sorting their
// members.
func isError(v Value) bool { return v.data().state == stateError }

// List returns the list with element type elem and the given elements, in
// order. List does not retain the slice.
//
// If an element is an error value the result is an error value carrying the
// diagnostics of every such element, each located by its index, and the
// Propagate marks of those elements. List panics if elem is the zero Type,
// or an element is neither an error value nor a resolved value of type elem.
func List(elem Type, elems ...Value) Value {
	return sequenceValue(ListType(elem), "List", elems)
}

// Set returns the set with element type elem and the given members. Equality
// tells members apart: members it settles are one value are one member, of
// which the set keeps the first given, and members that are not known stay
// apart unless it settles that. A set holds its members in the order it
// iterates them. Set does not retain the slice, and treats error members and
// panics as List does.
//
// A member must not be marked: it must carry no mark and hold none, at any
// depth. Equality tells members apart without looking at marks, so of two
// members that differ only by their marks a set would keep one and lose the
// other's marks, and which it lost would depend on the order they were given
// in. Set panics on a marked member rather than choose. Take the marks off
// the members, and put them on the set:
//
//	var marks []tenon.Mark
//	for i, m := range members {
//		var taken []tenon.Mark
//		members[i], taken = tenon.UnmarkDeep(m)
//		marks = append(marks, taken...)
//	}
//	set := tenon.WithMarks(tenon.Set(elem, members...), marks...)
func Set(elem Type, elems ...Value) Value {
	return sequenceValue(SetType(elem), "Set", elems)
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
		// A redacted member of a set is refused for its marks, which says
		// nothing they withhold, and not asked its type first.
		marked := t.t.kind == KindSet && e.n.isMarked()
		if !marked || !e.n.withholds() {
			requireMember(fn, element(i), e, t.t.elem)
		}
		if marked {
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
	return Value{n: &node{state: stateKnown, partial: anyPartial(members), markedWithin: anyMarked(members), typ: t, data: members}}
}

// unmarkForSet is what a panic message tells a caller who gave a set a marked
// member: the way to do it that keeps the marks.
const unmarkForSet = "unmark it with UnmarkDeep and reapply the marks to the set"

// Tuple returns the tuple with the given elements, in order, whose type is
// the tuple type of the elements' types. Tuple does not retain the slice.
//
// If an element is an error value the result is an error value, as in List.
// Tuple panics if an element is neither an error value nor a resolved value.
func Tuple(elems ...Value) Value {
	var errs containerErrors
	types := make([]Type, len(elems))
	for i, e := range elems {
		if isError(e) {
			errs.add(indexStep(NumberFromInt(int64(i))), e)
			continue
		}
		types[i] = memberType("Tuple", element(i), e)
	}
	if v, ok := errs.value(); ok {
		return v
	}
	return Value{n: &node{state: stateKnown, partial: anyPartial(elems), markedWithin: anyMarked(elems), typ: TupleType(types...), data: slices.Clone(elems)}}
}

// Object returns the object with the given attributes, whose type is the
// object type of the attributes' types. Attribute names are normalized to
// Unicode Normalization Form C, as ObjectType normalizes them. Object does not
// retain the map.
//
// The names come from data as often as from the program, so a name that
// cannot be an attribute name is a failure in the data (TY-018). The result is
// an error value if a name is empty or not well-formed UTF-8, if names are the
// same name after normalization, or if an attribute is an error value, with a
// diagnostic for each problem, in the order of the names, normalized where
// they are well-formed: code CodeObjectEmptyName or CodeStringInvalidUTF8 for
// each such name, and the diagnostics of each error attribute, located by its
// name where the name can be one, then code CodeObjectDuplicateName for each
// name that attributes share, whatever their values are. The error value
// carries the Propagate marks of the error attributes. CheckAttributeNames
// reports the same of names alone. Object panics if an attribute is neither
// an error value nor a resolved value.
func Object(attrs map[string]Value) Value {
	given := make([]namedEntry[Value], 0, len(attrs))
	for name, v := range attrs {
		given = append(given, namedEntry[Value]{original: name, value: v})
	}
	entries, shared := checkNames(given, attributeNames)
	for _, e := range entries {
		if !isError(e.value) {
			memberType("Object", attributeNamed(e.original), e.value)
		}
	}
	var errs containerErrors
	for _, e := range entries {
		switch {
		case e.fault != "":
			errs.addDiagnostic(nameFault(attributeNames, e.fault, e.original))
			// No path step can name what is not an attribute name, so an
			// error attribute under it keeps the paths it came with.
			if isError(e.value) {
				errs.addUnlocated(e.value)
			}
		case isError(e.value):
			errs.add(attributeStep(e.key), e.value)
		}
	}
	for _, group := range shared {
		errs.addDiagnostic(sharedName(attributeNames, group))
	}
	if v, ok := errs.value(); ok {
		return v
	}
	types := make(map[string]Type, len(entries))
	vals := make([]Value, len(entries))
	for i, e := range entries {
		types[e.key] = e.value.n.typ
		vals[i] = e.value
	}
	return Value{n: &node{state: stateKnown, partial: anyPartial(vals), markedWithin: anyMarked(vals), typ: ObjectType(types), data: vals}}
}

// tupleOf returns the tuple of type t holding vals, which are resolved values
// of t's element types, none of them an error value: what Tuple returns for
// them, without finding t again.
func tupleOf(t Type, vals []Value) Value {
	for i, v := range vals {
		if isError(v) {
			internalPanic("tupleOf: element %d of %s is an error value", i, t)
		}
	}
	return Value{n: &node{state: stateKnown, partial: anyPartial(vals), markedWithin: anyMarked(vals), typ: t, data: slices.Clone(vals)}}
}

// objectOf returns the object value of type t holding vals, one per attribute
// of t in its order. It is for a caller that has the type and the values it
// asks for already, where Object takes a map, normalizes its names, orders
// them and interns the type they describe, all of which t settles. Since no
// container holds an error value, and nothing here would locate one, a caller
// that has not settled its values first is a mistake in this package.
func objectOf(t Type, vals []Value) Value {
	for i, v := range vals {
		if isError(v) {
			internalPanic("objectOf: attribute %q of %s is an error value", t.t.attrs[i].name, t)
		}
	}
	return Value{n: &node{state: stateKnown, partial: anyPartial(vals), markedWithin: anyMarked(vals), typ: t, data: vals}}
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
// ones that are not known, in the bytewise order of their encodings.
//
// A member's place has to follow from the member, since two sets with the same
// members are one value and one value iterates one way. A member's encoding is
// everything it says about itself, the same from one run to the next, and is
// all there is to go on for a member that is not known. It holds no type,
// which the set states once for all its members, where reading each member
// would spell the type out again for every one. Two members encode alike
// where they are identical, or where they are told apart only by a capsule
// value whose type declares no encoding, since what stands in for it is the
// same; compareAlike orders those by the capsule values.
func orderMembers(members []Value) []Value {
	slices.SortStableFunc(members, memberOrder())
	return members
}

// memberOrder returns the comparison orderMembers sorts by, each member that
// is not known encoded once however often it is compared.
func memberOrder() func(a, b Value) int {
	alike := notKnownOrder()
	return func(a, b Value) int {
		known, other := a.n.isKnown(), b.n.isKnown()
		switch {
		case known && other:
			return compareValues(a, b)
		case known:
			return -1
		case other:
			return 1
		}
		return alike(a.n, b.n)
	}
}

// notKnownOrder returns a comparison ordering members that are not known as
// a set iterates them: by their encodings, and where two encode alike
// because a capsule value stands in, by the capsule values (compareAlike).
// Each member is encoded once, the first time it is asked about, so a caller
// that never meets two such members builds no encoder. The diff orders a
// member removal and a member addition that read alike by the same
// comparison, so that the order follows from the members and Diff(b, a)
// mirrors Diff(a, b).
func notKnownOrder() func(a, b *node) int {
	// encoding is a member's encoding, and whether it stands in for a capsule
	// value anywhere, which the encoder records as a failure.
	type encoding struct {
		bytes    []byte
		standsIn bool
	}
	var e *encoder
	encoded := map[*node]encoding{}
	of := func(n *node) encoding {
		enc, ok := encoded[n]
		if !ok {
			if e == nil {
				e = newEncoder()
			}
			before := e.failures
			enc = encoding{e.content(nil, Value{n: n}, 0, nil), e.failures > before}
			encoded[n] = enc
		}
		return enc
	}
	return func(a, b *node) int {
		x, y := of(a), of(b)
		if c := bytes.Compare(x.bytes, y.bytes); c != 0 || !x.standsIn && !y.standsIn {
			return c
		}
		return compareAlike(a, b)
	}
}

// compareAlike orders two values that encode alike where an encoding stands in
// for a capsule value: by the first place, in the order the encodings take
// them, that holds a capsule value in either, in the canonical order, which
// is the order its type declares if it declares one. Their encodings being
// alike, what stands at such a place in the other is a capsule value or
// null, and every other place holds what the other does.
func compareAlike(a, b *node) int {
	if a == b {
		return 0
	}
	if a.isCapsuleValue() || b.isCapsuleValue() {
		return compareCanonical(a, b)
	}
	switch x := a.data.(type) {
	case []Value:
		if y, ok := b.data.([]Value); ok {
			for i := range min(len(x), len(y)) {
				if c := compareAlike(x[i].n, y[i].n); c != 0 {
					return c
				}
			}
		}
	case []mapEntry:
		if y, ok := b.data.([]mapEntry); ok {
			for i := range min(len(x), len(y)) {
				if c := compareAlike(x[i].val.n, y[i].val.n); c != 0 {
					return c
				}
			}
		}
	case *rangeData:
		if y, ok := b.data.(*rangeData); ok {
			for i := range min(len(x.members), len(y.members)) {
				if c := compareAlike(x.members[i].n, y.members[i].n); c != 0 {
					return c
				}
			}
		}
	}
	return 0
}

// isCapsuleValue reports whether n is a known value of a capsule type.
func (n *node) isCapsuleValue() bool {
	return n.state == stateKnown && n.typ.t.kind == KindCapsule
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

// Map returns the map with element type elem and the given entries. Keys are
// normalized to Unicode Normalization Form C. Map does not retain the map.
//
// The result is an error value if a key is not well-formed UTF-8, if keys are
// the same key after normalization, or if an element is an error value, with a
// diagnostic for each problem, in the order of the keys, normalized where they
// are well-formed: code CodeStringInvalidUTF8 for each such key and the
// diagnostics of each error element, then code CodeMapDuplicateKey for each
// group of keys that normalize alike, whatever their elements are. The error
// value carries the Propagate marks of the error elements. Map panics if
// elem is the zero Type, or an element is neither an error value nor a
// resolved value of type elem.
func Map(elem Type, entries map[string]Value) Value {
	t := MapType(elem)
	given := make([]namedEntry[Value], 0, len(entries))
	for _, key := range slices.Sorted(maps.Keys(entries)) {
		val := entries[key]
		if !isError(val) {
			requireMember("Map", mapElement(key), val, elem)
		}
		given = append(given, namedEntry[Value]{original: key, value: val})
	}
	// Each entry reports in the order of its key, and a key that entries
	// share is reported after them all, whatever their elements are (TY-017).
	sorted, shared := checkNames(given, mapKeys)
	var errs containerErrors
	for _, e := range sorted {
		switch {
		case e.fault != "":
			errs.addDiagnostic(nameFault(mapKeys, e.fault, e.original))
			// No path step can name a key that is not a string, so an
			// error element under it keeps the path it came with.
			if isError(e.value) {
				errs.addUnlocated(e.value)
			}
		case isError(e.value):
			errs.add(indexStep(String(e.key)), e.value)
		}
	}
	for _, group := range shared {
		errs.addDiagnostic(sharedName(mapKeys, group))
	}
	if v, ok := errs.value(); ok {
		return v
	}
	out := make([]mapEntry, len(sorted))
	partial, marked := false, false
	for i, e := range sorted {
		out[i] = mapEntry{e.key, e.value}
		partial = partial || !e.value.n.isKnown()
		marked = marked || e.value.n.isMarked()
	}
	return Value{n: &node{state: stateKnown, partial: partial, markedWithin: marked, typ: t, data: out}}
}

// memberName names a member of a container, for a panic message or as a step
// of a path. A constructor names each member it checks and nearly every check
// passes, so the name is written only when a message uses it: written for
// each member, it would be an allocation or two for each that nothing reads.
// The encoder and the projector make a path step of it only for a failure,
// for the same reason.
type memberName struct {
	kind  memberKind
	index int    // the index of an element of a list, set or tuple
	name  string // the name of an attribute, or the key of a map element
}

// memberKind is the way a memberName names its member.
type memberKind uint8

const (
	elementAt    memberKind = iota // an element, by its index
	attributeOf                    // an attribute of an object, by its name
	elementOfKey                   // an element of a map, by its key
)

// element names element i of a list, set or tuple.
func element(i int) memberName { return memberName{kind: elementAt, index: i} }

// attributeNamed names the attribute of an object with the name given.
func attributeNamed(name string) memberName { return memberName{kind: attributeOf, name: name} }

// mapElement names the element of a map under the key given.
func mapElement(key string) memberName { return memberName{kind: elementOfKey, name: key} }

// String writes the name as a message uses it: an attribute's name as a
// display form quotes it, and a key in ASCII, so that spellings that normalize
// alike stay distinguishable.
func (m memberName) String() string {
	switch m.kind {
	case attributeOf:
		return "attribute " + quoted(m.name)
	case elementOfKey:
		return "the element of key " + quotedASCII(m.name)
	}
	return "element " + strconv.Itoa(m.index)
}

// memberType returns the type of a member of a container, panicking if v is not
// a resolved value. A member that is null or unknown has a type all the same,
// and the container holds it: what a member leaves open is the container's
// range, which partial records. fn and what name the caller and the member for
// the message.
func memberType(fn string, what memberName, v Value) Type {
	n := v.data()
	if !n.state.resolved() && n.withholds() {
		usagePanic("%s: %s is %s, which it cannot take"+withheldReason, fn, what, n.describe())
	}
	if n.state == statePending {
		usagePanic("%s: %s is a pending value, which has no type; Resolve it to one first", fn, what)
	}
	if !n.state.resolved() {
		usagePanic("%s: %s is %s, not a resolved value", fn, what, n.describe())
	}
	return n.typ
}

// requireMember panics unless v is a resolved value of type want.
func requireMember(fn string, what memberName, v Value, want Type) {
	if got := memberType(fn, what, v); got != want {
		if n := v.data(); n.withholds() {
			usagePanic("%s: %s is %s, which it cannot take"+withheldReason, fn, what, n.describe())
		}
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
	if n.withholds() {
		usagePanic("Len cannot take %s"+withheldReason, n.describe())
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
		if n.withholds() {
			usagePanic("Index cannot take %s"+withheldReason, n.describe())
		}
		usagePanic("Index called on %s, not a list or tuple value", n.describe())
	}
	elems := n.data.([]Value)
	if i < 0 || i >= len(elems) {
		if n.withholds() {
			usagePanic("Index(%d) cannot take %s"+withheldReason, i, n.describe())
		}
		usagePanic("Index(%d) called on a value with %d elements", i, len(elems))
	}
	return elems[i]
}

// Elements returns the elements of a list, set or tuple in order, in a new
// slice: a set's in the order it iterates, its known members first, in the
// canonical order, and then the rest, in the order of their encodings. It
// panics for other values.
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
	if n.withholds() {
		usagePanic("Elements cannot take %s"+withheldReason, n.describe())
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

// LookupMapElement returns the element of a map value with the given key,
// which is normalized before the lookup, and whether there is one. It panics
// if v is not a map value.
func (v Value) LookupMapElement(key string) (Value, bool) {
	entries := v.known(KindMap, "LookupMapElement").data.([]mapEntry)
	e, ok := findName(entries, key, func(e mapEntry) string { return e.key })
	return e.val, ok
}

// Attribute returns the attribute of an object value with the given name, which
// is normalized before the lookup. It panics if v is not an object value or has
// no such attribute; LookupAttribute is for a name that may be absent.
func (v Value) Attribute(name string) Value {
	n := v.known(KindObject, "Attribute")
	if a, ok := n.attribute(name); ok {
		return a
	}
	if n.withholds() {
		usagePanic("Attribute cannot take %s"+withheldReason, n.describe())
	}
	usagePanic("Attribute called on %s, which has no attribute %s", n.describe(), quoted(name))
	return Value{}
}

// LookupAttribute returns the attribute of an object value with the given
// name, which is normalized before the lookup, and whether there is one. It
// panics if v is not an object value.
func (v Value) LookupAttribute(name string) (Value, bool) {
	return v.known(KindObject, "LookupAttribute").attribute(name)
}

// attribute returns the attribute of n, a known object value, with the given
// name, and whether there is one.
func (n *node) attribute(name string) (Value, bool) {
	if !utf8.ValidString(name) {
		return Value{}, false
	}
	i, found := slices.BinarySearchFunc(n.typ.t.attrs, uni.NFC(name), func(a attribute, s string) int {
		return strings.Compare(a.name, s)
	})
	if !found {
		return Value{}, false
	}
	return n.data.([]Value)[i], true
}

// writeContainer writes the content of a resolved collection or structural
// value. A list, set or map writes its type, which states its members' type,
// and they are written without theirs; a tuple or an object writes no type,
// and its members are written without theirs only where the display form
// around it states its own. A container is written without its type where
// the display form around it states it (DI-010).
func (n *node) writeContainer(b *textWriter) {
	defer b.within(n)()
	switch n.typ.t.kind {
	case KindList, KindSet:
		if !b.stated {
			n.typ.write(b)
		}
		writeElements(b, n.data.([]Value), true)
	case KindTuple:
		writeElements(b, n.data.([]Value), b.stated)
	case KindMap:
		if !b.stated {
			n.typ.write(b)
		}
		defer b.keepStated(true)()
		b.WriteByte('{')
		for i, e := range n.data.([]mapEntry) {
			if b.full() {
				return
			}
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
			if b.full() {
				return
			}
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

// writeElements writes elements in brackets, separated by commas, without
// their type where stated says the display form around them states it.
func writeElements(b *textWriter, elems []Value, stated bool) {
	defer b.keepStated(stated)()
	b.WriteByte('[')
	for i, e := range elems {
		if b.full() {
			return
		}
		if i > 0 {
			b.WriteString(", ")
		}
		e.write(b)
	}
	b.WriteByte(']')
}
