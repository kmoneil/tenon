package tenon

import (
	"slices"
	"strconv"
	"strings"

	"tenon/internal/decimal"
	"tenon/internal/uni"
)

// Range describes the set of values that a resolved value may be. A known
// value has the singleton range holding only itself; an unknown value has a
// wider one, described by the narrowings known to hold of it. The range of a
// collection or structural value is given by its members, so it reads as the
// value does, with each member showing its own range.
//
// A range is never empty. A narrowing that would leave nothing possible
// produces an error value instead, because an empty range would describe a
// value that cannot exist.
//
// The zero Range is not a range.
type Range struct {
	v Value
}

// Range returns the range of a resolved value. Error values and pending values
// have no range, and Range panics on them; test with IsResolved first.
func (v Value) Range() Range {
	n := v.data()
	if !n.state.resolved() {
		usagePanic("Range called on %s, which has no range", n.describe())
	}
	return Range{v}
}

// Type returns the type of the values that r describes.
func (r Range) Type() Type { return r.v.Type() }

// AllowsNull reports whether null is one of the values that r describes.
func (r Range) AllowsNull() bool {
	n := r.v.data()
	switch n.state {
	case stateNull:
		return true
	case stateUnknown:
		return n.data.(*rangeData).null != nullNo
	}
	return false
}

// String describes r for messages, as in 5, "text" or
// string, not null, prefix "ab". The text is canonical: ranges recording the
// same narrowings render alike. It is not a format for parsing.
func (r Range) String() string {
	if r.v.n == nil {
		return "<zero Range>"
	}
	var b strings.Builder
	r.write(&b)
	return b.String()
}

func (r Range) write(b *strings.Builder) {
	n := r.v.n
	if n.state != stateUnknown {
		r.v.write(b) // a singleton range is the value itself
		return
	}
	n.typ.write(b)
	n.data.(*rangeData).write(b)
}

// nullness is what a range says about null.
type nullness uint8

const (
	nullMaybe nullness = iota // null is still possible
	nullNo                    // null is excluded
	nullOnly                  // null is all that is left
)

// nullOnly lives only while narrowings are being applied: a range that comes
// down to null is a range of one value, so it becomes the null value itself.

// bound is one end of the Number interval of a range.
type bound struct {
	v    decimal.Dec
	incl bool // the bound itself is in the range
	set  bool
}

// equal reports whether b and c are the same bound.
func (b bound) equal(c bound) bool {
	return b.set == c.set && b.incl == c.incl && (!b.set || b.v.Equal(c.v))
}

// tighten narrows b to the bound v, incl when that one is narrower than what b
// already says. lower says which end of the interval b is.
func (b *bound) tighten(v decimal.Dec, incl, lower bool) {
	if !b.set {
		*b = bound{v: v, incl: incl, set: true}
		return
	}
	switch c := v.Cmp(b.v); {
	case c == 0:
		b.incl = b.incl && incl
	case (c > 0) == lower:
		*b = bound{v: v, incl: incl, set: true}
	}
}

func (b bound) write(w *strings.Builder, lower bool) {
	switch {
	case lower && b.incl:
		w.WriteString(">= ")
	case lower:
		w.WriteString("> ")
	case b.incl:
		w.WriteString("<= ")
	default:
		w.WriteString("< ")
	}
	w.WriteString(b.v.String())
}

// holdsLower reports whether d is at or above b, which bounds nothing when it
// is unset.
func (b bound) holdsLower(d decimal.Dec) bool {
	if !b.set {
		return true
	}
	c := d.Cmp(b.v)
	return c > 0 || (c == 0 && b.incl)
}

// holdsUpper reports whether d is at or below b, which bounds nothing when it
// is unset.
func (b bound) holdsUpper(d decimal.Dec) bool {
	if !b.set {
		return true
	}
	c := d.Cmp(b.v)
	return c < 0 || (c == 0 && b.incl)
}

// crosses reports whether a lower bound lies above an upper bound, so that no
// number meets both. Either bound being unset leaves them uncrossed.
func crosses(lo, hi bound) bool {
	if !lo.set || !hi.set {
		return false
	}
	c := lo.v.Cmp(hi.v)
	return c > 0 || (c == 0 && !(lo.incl && hi.incl))
}

// text renders b as the narrowing that produced it.
func (b bound) text(lower bool) string {
	var w strings.Builder
	b.write(&w, lower)
	return w.String()
}

// lengthBound is a bound on the length of a string or collection.
type lengthBound struct {
	n   int64
	set bool
}

// rangeData is the immutable description of an unknown value's range: every
// narrowing known to hold of it. The zero rangeData is the whole domain of a
// type, null included.
//
// A range is canonical, so two ranges describe the same set exactly when their
// fields are equal. That holds because an absent narrowing has one spelling,
// and because a narrowing that another one implies is recorded as though it
// had been applied: a prefix of n grapheme clusters records a least length of
// n.
type rangeData struct {
	null  nullness
	lo    bound       // Number: the lower bound
	hi    bound       // Number: the upper bound
	pfx   string      // String: the required prefix, empty when there is none
	lenLo int64       // String and collections: the least length
	lenHi lengthBound // String and collections: the greatest length
	// members holds the values every set in the range has among its members,
	// in the order a set iterates them: values that are one member appear
	// once, and one whose range excludes nothing is not here, because it
	// promises no more than the least length already records.
	members []Value
}

// equal reports whether r and s describe the same set.
func (r *rangeData) equal(s *rangeData) bool {
	return r.null == s.null && r.lo.equal(s.lo) && r.hi.equal(s.hi) &&
		r.pfx == s.pfx && r.lenLo == s.lenLo && r.lenHi == s.lenHi &&
		len(r.members) == len(s.members) &&
		sameMembersFunc(r.members, s.members, Identical)
}

func (r *rangeData) write(b *strings.Builder) {
	if r.null == nullNo {
		b.WriteString(", not null")
	}
	if r.lo.set {
		b.WriteString(", ")
		r.lo.write(b, true)
	}
	if r.hi.set {
		b.WriteString(", ")
		r.hi.write(b, false)
	}
	if r.pfx != "" {
		b.WriteString(", prefix ")
		b.WriteString(strconv.Quote(r.pfx))
	}
	if len(r.members) > 0 {
		b.WriteString(", ")
		writeMembers(b, r.members)
	}
	if r.lenLo > 0 {
		b.WriteString(", length >= ")
		b.WriteString(strconv.FormatInt(r.lenLo, 10))
	}
	if r.lenHi.set {
		b.WriteString(", length <= ")
		b.WriteString(strconv.FormatInt(r.lenHi.n, 10))
	}
}

// writeMembers renders a member listing, as in members {1, 2}.
func writeMembers(b *strings.Builder, members []Value) {
	b.WriteString("members {")
	for i, m := range members {
		if i > 0 {
			b.WriteString(", ")
		}
		m.write(b)
	}
	b.WriteByte('}')
}

// narrowingKind identifies which row of the narrowings table a Narrowing is.
type narrowingKind uint8

const (
	narrowNotNull narrowingKind = iota + 1
	narrowNull
	narrowNumberMin
	narrowNumberMax
	narrowPrefix
	narrowLengthMin
	narrowLengthMax
	narrowMembers
)

// Narrowing is one fact about a value that rules out some of what it could
// otherwise be. Narrow applies narrowings to a value.
//
// The zero Narrowing is not a narrowing.
type Narrowing struct {
	kind    narrowingKind
	incl    bool
	num     decimal.Dec
	str     string
	n       int64
	members []Value
}

// NotNull returns the narrowing that excludes null. It applies to every type.
func NotNull() Narrowing { return Narrowing{kind: narrowNotNull} }

// Null returns the narrowing that leaves null as the only possibility. It
// applies to every type.
//
// Null is the narrowing; NullVal is the null value itself.
func Null() Narrowing { return Narrowing{kind: narrowNull} }

// NumberMin returns the narrowing that bounds a Number value from below by v,
// which is itself in the range when inclusive is true. It panics if v is not a
// known Number value.
func NumberMin(v Value, inclusive bool) Narrowing {
	return Narrowing{kind: narrowNumberMin, num: numberBound("NumberMin", v), incl: inclusive}
}

// NumberMax returns the narrowing that bounds a Number value from above by v,
// which is itself in the range when inclusive is true. It panics if v is not a
// known Number value.
func NumberMax(v Value, inclusive bool) Narrowing {
	return Narrowing{kind: narrowNumberMax, num: numberBound("NumberMax", v), incl: inclusive}
}

// StringPrefix returns the narrowing that requires a String value to begin with
// s. The prefix is normalized like the content of a string value, and then cut
// back to the part of s that text following s cannot change: a value built from
// "cafe" and a continuation beginning with a combining acute is "café", which
// does not begin with "cafe", so the narrowing keeps "caf". Nothing composes
// with a hyphen or a digit, so "v1-" is kept whole.
//
// The narrowing therefore holds of some values that s alone would exclude, and
// it holds of them whether the value is known yet or not.
//
// StringPrefix panics if s is not well-formed UTF-8. A narrowing carries no
// diagnostics, so, unlike String, it cannot report bad text as data: build a
// string value first when the text comes from outside the program.
func StringPrefix(s string) Narrowing {
	c, err := uni.Canonical(s)
	if err != nil {
		usagePanic("StringPrefix called with text that is not well-formed UTF-8, at byte %d", invalidUTF8At(s))
	}
	return Narrowing{kind: narrowPrefix, str: uni.StablePrefix(c)}
}

// LengthMin returns the narrowing that requires a String, list, set or map
// value to have length at least n. The length of a string is its count of
// extended grapheme clusters. It panics if n is negative.
func LengthMin(n int64) Narrowing {
	return Narrowing{kind: narrowLengthMin, n: lengthArg("LengthMin", n)}
}

// LengthMax returns the narrowing that requires a String, list, set or map
// value to have length at most n. The length of a string is its count of
// extended grapheme clusters. It panics if n is negative.
func LengthMax(n int64) Narrowing {
	return Narrowing{kind: narrowLengthMax, n: lengthArg("LengthMax", n)}
}

// Members returns the narrowing that requires a Set value to hold every
// listed value as a member. A listed value may be null, which a set can hold,
// and it may be unknown: the set then needs a member that the listed value's
// range allows.
//
// The record a range keeps of a listing is canonical. Listed values that
// could turn out to be one value promise one member between them, so the
// least length of the set rises to the count of listed values that are
// provably distinct, and a listed value whose range excludes nothing is kept
// only as that least length, since it promises no more than that a member
// exists.
//
// Members panics if a listed value is an error value or a pending value: a
// narrowing carries no diagnostics, and a value that has no type yet says
// nothing a member could be held to. It panics on a marked value too, for the
// reason SetVal gives: a set's members carry no marks. Unmark the value with
// UnmarkDeep, and reapply the marks to the set being narrowed.
func Members(vs ...Value) Narrowing {
	for i, v := range vs {
		n := v.data()
		if !n.state.resolved() {
			usagePanic("Members called with %s as member %d, not a resolved value", n.describe(), i)
		}
		if n.isMarked() {
			usagePanic("Members called with %s as member %d, and a set's members carry no marks; %s",
				n.describeMarked(), i, unmarkForSet)
		}
	}
	return Narrowing{kind: narrowMembers, members: slices.Clone(vs)}
}

// numberBound returns the content of a bound supplied to a number narrowing,
// panicking unless it is a known Number value. fn names the narrowing.
func numberBound(fn string, v Value) decimal.Dec {
	n := v.data()
	if n.state != stateKnown || n.typ.t.kind != KindNumber {
		usagePanic("%s called with %s as a bound, not a known Number value", fn, n.describe())
	}
	return n.data.(decimal.Dec)
}

// lengthArg returns n, panicking if it is negative. fn names the narrowing.
func lengthArg(fn string, n int64) int64 {
	if n < 0 {
		usagePanic("%s called with a negative length, %d", fn, n)
	}
	return n
}

// String describes nw for messages, as in not null, >= 5 or prefix "ab". It is
// not a format for parsing.
func (nw Narrowing) String() string {
	switch nw.kind {
	case narrowNotNull:
		return "not null"
	case narrowNull:
		return "null"
	case narrowNumberMin:
		return bound{v: nw.num, incl: nw.incl, set: true}.text(true)
	case narrowNumberMax:
		return bound{v: nw.num, incl: nw.incl, set: true}.text(false)
	case narrowPrefix:
		return "prefix " + strconv.Quote(nw.str)
	case narrowLengthMin:
		return "length >= " + strconv.FormatInt(nw.n, 10)
	case narrowLengthMax:
		return "length <= " + strconv.FormatInt(nw.n, 10)
	case narrowMembers:
		var b strings.Builder
		writeMembers(&b, nw.members)
		return b.String()
	}
	return "<zero Narrowing>"
}

// message renders nw for a diagnostic, shortening text that is long.
func (nw Narrowing) message() string {
	switch nw.kind {
	case narrowPrefix:
		return "prefix " + quoted(nw.str)
	case narrowMembers:
		return shortened(nw.String(), func(s string) string { return s })
	}
	return nw.String()
}

// appliesTo reports whether nw can narrow a value of type t.
func (nw Narrowing) appliesTo(t Type) bool {
	switch nw.kind {
	case narrowNotNull, narrowNull:
		return true
	case narrowNumberMin, narrowNumberMax:
		return t.t.kind == KindNumber
	case narrowPrefix:
		return t.t.kind == KindString
	case narrowMembers:
		return t.t.kind == KindSet
	}
	switch t.t.kind {
	case KindString, KindList, KindSet, KindMap:
		return true
	}
	return false
}

// holdsFor reports whether the known value n satisfies nw.
func (nw Narrowing) holdsFor(n *node) bool {
	if n.state == stateNull {
		// Null has no number, no prefix and no length, so every narrowing
		// that mentions one says nothing about it. Only NotNull rules it out.
		return nw.kind != narrowNotNull
	}
	switch nw.kind {
	case narrowNotNull:
		return true
	case narrowNull:
		return false
	case narrowNumberMin:
		return bound{v: nw.num, incl: nw.incl, set: true}.holdsLower(n.data.(decimal.Dec))
	case narrowNumberMax:
		return bound{v: nw.num, incl: nw.incl, set: true}.holdsUpper(n.data.(decimal.Dec))
	case narrowPrefix:
		return strings.HasPrefix(n.data.(string), nw.str)
	case narrowLengthMin:
		return n.length() >= nw.n
	case narrowLengthMax:
		return n.length() <= nw.n
	case narrowMembers:
		// The value is a range of one, so a listed value that could still
		// turn out to be a member leaves it standing; only one that provably
		// is not a member contradicts it.
		for _, m := range nw.members {
			if found, settled := membership(n, m); settled && !found {
				return false
			}
		}
		return true
	}
	return false
}

// length returns the length of a known String, list, set or map value.
func (n *node) length() int64 {
	switch n.typ.t.kind {
	case KindString:
		return int64(uni.GraphemeCount(n.data.(string)))
	case KindMap:
		return int64(len(n.data.([]mapEntry)))
	}
	return int64(len(n.data.([]Value)))
}

// Narrow returns v narrowed by ns, applied in order: the value that says
// everything v said and everything ns says. Narrowing is monotone, so the
// result describes no value that v did not, and narrowing a known value either
// returns it or contradicts it.
//
// A narrowing that brings a range down to a single value produces that value,
// known: an unknown that nothing more could ever say is not unknown. A set
// range recording as many members as its greatest length allows, all of them
// provably distinct and null excluded, has come down to the set holding those
// members, which is known when every member is.
//
// Apart from Null and NotNull, a narrowing says what a value is when it is not
// null, so a range that still holds null keeps it: NotNull alone excludes
// null, and narrowing the null value by a bound, a prefix or a length returns
// it unchanged. A narrowing that leaves nothing possible, such as an upper
// bound below a lower bound already in force, produces an error value with
// code CodeRangeContradiction rather than an empty range. Narrowing an error
// value returns an error value carrying its diagnostics.
//
// Narrow refines the value it is given rather than deriving a new one, so
// the result carries the marks of v, the Isolate ones included.
//
// Narrow panics on a pending value, which has no type to narrow against, and
// if a narrowing does not apply to the type of v, such as a length bound on a
// Number value.
func Narrow(v Value, ns ...Narrowing) Value {
	return carryMarks(v, narrowValue(v, ns))
}

// narrowValue is Narrow before marks are carried over.
func narrowValue(v Value, ns []Narrowing) Value {
	if e, ok := propagate(v); ok {
		return e
	}
	n := v.data()
	for _, nw := range ns {
		if nw.kind == 0 {
			usagePanic("Narrow called with the zero Narrowing")
		}
	}
	if n.state == statePending {
		return narrowPending(v, n, ns)
	}
	for _, nw := range ns {
		if !nw.appliesTo(n.typ) {
			usagePanic("Narrow called with %s, which does not apply to a value of type %s", nw, n.typ)
		}
		for i, m := range nw.members {
			if m.n.typ != n.typ.t.elem {
				usagePanic("Narrow called with %s as member %d of a Members narrowing, but the members of %s are of type %s",
					m.n.describe(), i, n.typ, n.typ.t.elem)
			}
		}
	}
	if n.state != stateUnknown {
		for _, nw := range ns {
			if !nw.holdsFor(n) {
				return contradiction("the value " + valueText(v) + " does not satisfy " + nw.message())
			}
		}
		return v
	}
	old := n.data.(*rangeData)
	r := *old
	for _, nw := range ns {
		clash, ok := r.apply(nw)
		if !ok {
			return contradiction("no value of type " + n.typ.String() + " satisfies both " + clash + " and " + nw.message())
		}
	}
	if sole, ok := r.singleton(n.typ); ok {
		return sole
	}
	if set, ok := r.memberSet(n.typ); ok {
		return set
	}
	if r.equal(old) {
		return v
	}
	return Value{&node{state: stateUnknown, typ: n.typ, data: &r}}
}

// singleton returns the one value that r describes, and whether it describes
// exactly one. A range that has come down to a single value is that value: an
// unknown that nothing more could ever say is a known value. t is the type of
// the value whose range r is.
func (r *rangeData) singleton(t Type) (Value, bool) {
	if r.null == nullOnly {
		return NullVal(t), true
	}
	if r.null != nullNo {
		// Null is still possible, so the range holds it and at least one more.
		return Value{}, false
	}
	switch t.t.kind {
	case KindNumber:
		// An empty range is never built, so bounds that meet are inclusive.
		if r.lo.set && r.hi.set && r.lo.v.Equal(r.hi.v) {
			return numberValue(r.lo.v), true
		}
	case KindString:
		if r.emptyOnly() {
			return String(""), true
		}
	case KindList:
		if r.emptyOnly() {
			return ListVal(t.t.elem), true
		}
	case KindSet:
		if r.emptyOnly() {
			return SetVal(t.t.elem), true
		}
	case KindMap:
		if r.emptyOnly() {
			return MapVal(t.t.elem, nil), true
		}
	case KindTuple, KindObject:
		return soleValue(t)
	}
	return Value{}, false
}

// emptyOnly reports whether the length bounds of r leave only length zero.
func (r *rangeData) emptyOnly() bool { return r.lenHi.set && r.lenHi.n == 0 }

// soleValue returns the only value of type t other than null, and whether t
// has only one: the empty tuple and the empty object have no room to differ,
// and neither do tuples and objects built from types with the same property.
// Every other kind has at least two values, so no narrowing but Null and none
// of the length bounds can pin one down.
func soleValue(t Type) (Value, bool) {
	d := t.t
	switch d.kind {
	case KindTuple:
		elems := make([]Value, len(d.elems))
		for i, e := range d.elems {
			v, ok := soleValue(e)
			if !ok {
				return Value{}, false
			}
			elems[i] = v
		}
		return TupleVal(elems...), true
	case KindObject:
		attrs := make(map[string]Value, len(d.attrs))
		for _, a := range d.attrs {
			v, ok := soleValue(a.typ)
			if !ok {
				return Value{}, false
			}
			attrs[a.name] = v
		}
		return ObjectVal(attrs), true
	}
	return Value{}, false
}

// narrowPending returns the pending value v narrowed by ns. A pending value has
// no type, so the only narrowings it can take are the two that say nothing
// about one: whether it will be null.
func narrowPending(v Value, n *node, ns []Narrowing) Value {
	null := n.null
	for _, nw := range ns {
		switch nw.kind {
		case narrowNotNull:
			if null == nullOnly {
				return contradiction("no pending value satisfies both " + Null().String() + " and " + nw.String())
			}
			null = nullNo
		case narrowNull:
			if null == nullNo {
				return contradiction("no pending value satisfies both " + NotNull().String() + " and " + nw.String())
			}
			null = nullOnly
		default:
			usagePanic("Narrow called with %s, which does not apply to a pending value, whose type is not determined", nw)
		}
	}
	if null == n.null {
		return v
	}
	return Value{&node{state: statePending, null: null, data: n.data}}
}

// contradiction returns the error value for a narrowing that leaves no value
// possible.
func contradiction(message string) Value {
	return errorValue(Diagnostic{Code: CodeRangeContradiction, Message: message})
}

// valueText renders v for a diagnostic message, shortening it if it is long.
func valueText(v Value) string {
	return shortened(v.String(), func(s string) string { return s })
}

// apply narrows r by nw. It reports whether anything is left, and names the
// narrowing already in force that nw contradicts when nothing is.
func (r *rangeData) apply(nw Narrowing) (string, bool) {
	switch nw.kind {
	case narrowNotNull:
		if r.null == nullOnly {
			return Null().String(), false
		}
		r.null = nullNo
		return "", true
	case narrowNull:
		if r.null == nullNo {
			return NotNull().String(), false
		}
		r.null = nullOnly
		return "", true
	}
	if r.null == nullOnly {
		// Null satisfies a narrowing that says what a value is when it is not
		// null, so there is nothing left for this one to say, and recording it
		// would give the same set two spellings.
		return "", true
	}
	switch nw.kind {
	case narrowNumberMin:
		r.lo.tighten(nw.num, nw.incl, true)
		if !r.numberOK() {
			return r.hi.text(false), false
		}
	case narrowNumberMax:
		r.hi.tighten(nw.num, nw.incl, false)
		if !r.numberOK() {
			return r.lo.text(true), false
		}
	case narrowPrefix:
		switch {
		case strings.HasPrefix(nw.str, r.pfx):
			r.pfx = nw.str
		case strings.HasPrefix(r.pfx, nw.str):
			// Already at least this narrow.
		default:
			return "prefix " + quoted(r.pfx), false
		}
		r.implyLength()
		if !r.lengthOK() {
			return r.lengthMaxText(), false
		}
	case narrowLengthMin:
		if nw.n > r.lenLo {
			r.lenLo = nw.n
		}
		if !r.lengthOK() {
			return r.lengthMaxText(), false
		}
	case narrowLengthMax:
		if !r.lenHi.set || nw.n < r.lenHi.n {
			r.lenHi = lengthBound{n: nw.n, set: true}
		}
		if !r.lengthOK() {
			return r.lengthMinText(), false
		}
	case narrowMembers:
		r.addMembers(nw.members)
		if !r.lengthOK() {
			return r.lengthMaxText(), false
		}
	}
	return "", true
}

// addMembers records vs among the members every set in the range holds. The
// record is canonical, however the listings arrive: values that are one
// member appear once, a value whose range excludes nothing is dropped, the
// rest are held in the order a set iterates them, and the least length rises
// to the count of members that are provably distinct, or to one, since any
// listing promises a member.
func (r *rangeData) addMembers(vs []Value) {
	if len(vs) == 0 {
		return
	}
	merged := slices.Clone(r.members)
	for _, v := range vs {
		if vacuousMember(v) || slices.ContainsFunc(merged, func(k Value) bool { return oneMember(k, v) }) {
			continue
		}
		merged = append(merged, v)
	}
	r.members = orderMembers(merged)
	lo := int64(provablyDistinct(r.members))
	if lo == 0 {
		lo = 1
	}
	if lo > r.lenLo {
		r.lenLo = lo
	}
}

// oneMember reports whether a set holding k needs no separate note that it
// holds v: equality settles that they are one value, or they are identical,
// which two unknowns with one range are. A set value keeps identical unknown
// members apart, since each may resolve its own way, but a range records
// requirements, and two identical ones require the same thing.
func oneMember(k, v Value) bool {
	if eq, settled := equality(k.n, v.n); settled && eq {
		return true
	}
	return Identical(k, v)
}

// vacuousMember reports whether a listed member's range excludes nothing, so
// that listing it says only that some member exists. Recording it would give
// one range two spellings: with such a member listed, and with the least
// length it implies.
func vacuousMember(v Value) bool {
	rd, ok := unknownRange(v.n)
	return ok && rd.equal(&rangeData{})
}

// memberSet returns the set that r has come down to, and whether it has come
// down to one: null excluded, as many members recorded as the greatest length
// allows, and every pair of them provably distinct, so that the sets r
// describes are exactly the sets holding one value from each member's range.
// A pair that could yet be one member would leave room for a set of fewer,
// other values, which no set holding the members describes.
func (r *rangeData) memberSet(t Type) (Value, bool) {
	if t.t.kind != KindSet || r.null != nullNo || len(r.members) == 0 ||
		!r.lenHi.set || r.lenHi.n != int64(len(r.members)) ||
		provablyDistinct(r.members) != len(r.members) {
		return Value{}, false
	}
	return SetVal(t.t.elem, r.members...), true
}

// implyLength records the least length that the prefix of r forces. The first
// clusters of the prefix cannot merge with anything a suffix adds, so a value
// in the range has at least as many clusters as the prefix has.
func (r *rangeData) implyLength() {
	if k := int64(uni.GraphemeCount(r.pfx)); k > r.lenLo {
		r.lenLo = k
	}
}

// numberOK reports whether some number meets both bounds of r.
func (r *rangeData) numberOK() bool { return !crosses(r.lo, r.hi) }

// lengthOK reports whether some length meets both length bounds of r.
func (r *rangeData) lengthOK() bool {
	return !r.lenHi.set || r.lenLo <= r.lenHi.n
}

// lengthMinText names the narrowing that forces the least length of r, which
// is the prefix when the prefix is what forced it.
func (r *rangeData) lengthMinText() string {
	if r.pfx != "" && int64(uni.GraphemeCount(r.pfx)) >= r.lenLo {
		return "prefix " + quoted(r.pfx)
	}
	if len(r.members) > 0 && int64(provablyDistinct(r.members)) >= r.lenLo {
		var b strings.Builder
		writeMembers(&b, r.members)
		return shortened(b.String(), func(s string) string { return s })
	}
	return "length >= " + strconv.FormatInt(r.lenLo, 10)
}

// lengthMaxText names the narrowing that forces the greatest length of r.
func (r *rangeData) lengthMaxText() string {
	return "length <= " + strconv.FormatInt(r.lenHi.n, 10)
}
