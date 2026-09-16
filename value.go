package tenon

import (
	"math/big"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"tenon/internal/decimal"
	"tenon/internal/uni"
)

// Value is a tenon value. A value is in exactly one of three states:
//
//   - an error value carries diagnostics describing what went wrong, and has
//     no type;
//   - a resolved value has a type;
//   - a pending value has no type yet, only a constraint on what its type
//     will be.
//
// Values are immutable, and every operation returns a new value.
//
// The zero Value is not a value: every method except String panics when
// called on it.
type Value struct {
	n *node
}

// state is the state of a value.
type state uint8

const (
	stateError   state = iota + 1 // carries diagnostics, and has no type
	stateKnown                    // has a type and content
	stateNull                     // has a type, and is null
	stateUnknown                  // has a type and a range of possible values
	statePending                  // has a constraint on its type, not a type
)

// resolved reports whether s is the state of a resolved value: one that has a
// type, whether or not its content is known.
func (s state) resolved() bool {
	return s == stateKnown || s == stateNull || s == stateUnknown
}

// isKnown reports whether n describes exactly one value: content that no
// member leaves open, or null, which is one value by itself.
func (n *node) isKnown() bool {
	switch n.state {
	case stateKnown:
		return !n.partial
	case stateNull:
		return true
	}
	return false
}

// node is the immutable description that a Value refers to.
type node struct {
	state state
	// partial is set on a collection or structural value that holds a member
	// which is not known. The range of such a value is every value of its type
	// whose members lie in its members' ranges, which is more than one, so the
	// value is not known however settled its own shape is.
	partial bool
	// null is the nullness fact of a pending value: whether it is known to be
	// null, known not to be, or neither yet. An unknown value keeps the same
	// fact in its range instead, where the other narrowings are.
	null nullness
	typ  Type // the type of a resolved value
	// data is the []Diagnostic of an error value, the Constraint of a pending
	// value, the *rangeData of an unknown value, or nil for a null value. For
	// a known value it is the content that the kind of its type calls for: a
	// bool, a decimal.Dec, a canonical string, the pointer that a capsule
	// encapsulates, the []Value elements of a list, set or tuple, the []Value
	// attributes of an object in its type's attribute order, or the
	// []mapEntry entries of a map, sorted by key.
	data any
}

var (
	trueValue  = Value{&node{state: stateKnown, typ: Type{boolType}, data: true}}
	falseValue = Value{&node{state: stateKnown, typ: Type{boolType}, data: false}}
)

// Bool returns the Bool value b.
func Bool(b bool) Value {
	if b {
		return trueValue
	}
	return falseValue
}

// NumberFromInt returns the Number value i.
func NumberFromInt(i int64) Value {
	return numberValue(decimal.FromInt64(i))
}

// NumberFromText returns the Number value that s denotes. The syntax is an
// optional "-", one or more ASCII digits, optionally a "." and one or more
// digits, and optionally an exponent: "e" or "E", an optional sign, and one or
// more digits. The number is exact. If s has another form, the result is an
// error value with code CodeNumberInvalidSyntax, and if the number is out of
// range, an error value with code CodeNumberOutOfRange.
func NumberFromText(s string) Value {
	d, err := decimal.Parse(s)
	switch err {
	case nil:
		return numberValue(d)
	case decimal.ErrOutOfRange:
		return errorValue(Diagnostic{Code: CodeNumberOutOfRange, Message: quoted(s) + " is outside the range of numbers"})
	}
	return errorValue(Diagnostic{Code: CodeNumberInvalidSyntax, Message: quoted(s) + " is not a number"})
}

func numberValue(d decimal.Dec) Value {
	return Value{&node{state: stateKnown, typ: Type{numberType}, data: d}}
}

// String returns the String value s, normalized to Unicode Normalization
// Form C. If s is not well-formed UTF-8, the result is an error value with code
// CodeStringInvalidUTF8: ill-formed bytes are never replaced or passed through.
func String(s string) Value {
	c, err := uni.Canonical(s)
	if err != nil {
		return errorValue(Diagnostic{
			Code:    CodeStringInvalidUTF8,
			Message: "the text is not well-formed UTF-8 at byte " + strconv.Itoa(invalidUTF8At(s)),
		})
	}
	return Value{&node{state: stateKnown, typ: Type{stringType}, data: c}}
}

// invalidUTF8At returns the offset of the first byte of s that does not begin
// well-formed UTF-8, or len(s) if there is none.
func invalidUTF8At(s string) int {
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			return i
		}
		i += size
	}
	return len(s)
}

// quoted quotes text for a diagnostic message, shortening it if it is long.
func quoted(s string) string { return shortened(s, strconv.Quote) }

// quotedASCII quotes text for a diagnostic message in ASCII, so that spellings
// that normalize alike stay distinguishable, shortening it if it is long.
func quotedASCII(s string) string { return shortened(s, strconv.QuoteToASCII) }

// shortened applies quote to s, or to a prefix of s followed by "..." if s is
// long.
func shortened(s string, quote func(string) string) string {
	const limit = 32
	if len(s) <= limit {
		return quote(s)
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return quote(s[:cut]) + "..."
}

// CapsuleVal returns the value of capsule type t that encapsulates p. It panics
// if t is not a capsule type of *E, or if p is nil.
func CapsuleVal[E any](t Type, p *E) Value {
	d := t.mustKind(KindCapsule, "CapsuleVal")
	if !d.capsule.accepts(p) {
		usagePanic("CapsuleVal called with a pointer that capsule type %s does not encapsulate", t)
	}
	if p == nil {
		usagePanic("CapsuleVal called with a nil pointer for capsule type %s", t)
	}
	return Value{&node{state: stateKnown, typ: t, data: p}}
}

// CapsuleValue returns the pointer that v encapsulates. It panics if v is not a
// value of a capsule type of *E.
func CapsuleValue[E any](v Value) *E {
	n := v.known(KindCapsule, "CapsuleValue")
	p, ok := n.data.(*E)
	if !ok {
		usagePanic("CapsuleValue called for a pointer type that capsule type %s does not encapsulate", n.typ)
	}
	return p
}

// Pending returns a pending value: a value whose type is not yet determined,
// and will satisfy c. Whether it will be null is not determined either;
// Narrow with Null or NotNull says so when the caller knows.
func Pending(c Constraint) Value {
	c.data()
	return Value{&node{state: statePending, data: c}}
}

// Resolve returns the value that a pending value takes once its type turns out
// to be t: an unknown value of t, narrowed by what the pending value already
// said. A pending value known to be null resolves to the null value of t, which
// is how a null read before its type is known keeps the one thing that was said
// about it.
//
// Resolve returns an error value if v is one. It panics if v is not a pending
// value, or if t does not satisfy the constraint that v carries, which is a
// mistake in the caller rather than in any data: the caller chose both.
func Resolve(v Value, t Type) Value {
	if e, ok := propagate(v); ok {
		return e
	}
	n := v.data()
	if n.state != statePending {
		usagePanic("Resolve called on %s, which is not a pending value", n.describe())
	}
	c := n.data.(Constraint)
	if !Satisfies(c, t) {
		usagePanic("Resolve called with type %s, which does not satisfy the constraint %s of the pending value", t, c)
	}
	switch n.null {
	case nullOnly:
		return NullVal(t)
	case nullNo:
		return Narrow(Unknown(t), NotNull())
	}
	return Unknown(t)
}

// Unknown returns the unknown value of type t: the value that could still be
// any value of t, null included. Narrow returns values that say more.
func Unknown(t Type) Value {
	t.data()
	return Value{&node{state: stateUnknown, typ: t, data: &rangeData{}}}
}

// NullVal returns the null value of type t. Null is a member of the domain of
// every type rather than a state of its own, so the null value of t is a known
// value whose range holds nothing but null.
//
// NullVal is the value; Null is the narrowing that produces it.
func NullVal(t Type) Value {
	t.data()
	return Value{&node{state: stateNull, typ: t}}
}

// errorValue returns an error value that carries diags, which must not be
// empty.
func errorValue(diags ...Diagnostic) Value {
	if len(diags) == 0 {
		usagePanic("an error value needs at least one diagnostic")
	}
	return Value{&node{state: stateError, data: slices.Clone(diags)}}
}

// data returns the description of v, panicking if v is the zero Value.
func (v Value) data() *node {
	if v.n == nil {
		usagePanic("use of the zero Value")
	}
	return v.n
}

// describe names what n describes, for panic messages.
func (n *node) describe() string {
	switch n.state {
	case stateError:
		return "an error value"
	case statePending:
		return "a pending value"
	case stateNull:
		return "the null value of type " + n.typ.String()
	case stateUnknown:
		return "an unknown value of type " + n.typ.String()
	}
	return "a value of type " + n.typ.String()
}

// noContent panics if the content of n cannot be read because it is null or
// unknown. method names the caller.
func (n *node) noContent(method string) {
	if n.state == stateNull || n.state == stateUnknown {
		usagePanic("%s called on %s, which has no content", method, n.describe())
	}
}

// known returns the description of v, panicking unless v is a known value of
// the given kind.
func (v Value) known(kind Kind, method string) *node {
	n := v.data()
	if !n.state.resolved() || n.typ.t.kind != kind {
		usagePanic("%s called on %s, not a value of kind %s", method, n.describe(), kind)
	}
	n.noContent(method)
	return n
}

// IsError reports whether v is an error value.
func (v Value) IsError() bool { return v.data().state == stateError }

// IsResolved reports whether v is a resolved value, which has a type, whether
// or not its content is known.
func (v Value) IsResolved() bool { return v.data().state.resolved() }

// IsKnown reports whether v is a resolved value whose range holds exactly one
// value. The null value of a type is known.
//
// A collection or structural value is known when every one of its members is.
// Its members can be read whether or not they are known, since they are there
// to read; it is the range that a member leaves open, not the content.
func (v Value) IsKnown() bool { return v.data().isKnown() }

// IsPending reports whether v is a pending value, whose type is not yet
// determined.
func (v Value) IsPending() bool { return v.data().state == statePending }

// Type returns the type of a resolved value. Error values and pending values
// have no type, and Type panics on them; test with IsResolved first.
func (v Value) Type() Type {
	n := v.data()
	if !n.state.resolved() {
		usagePanic("Type called on %s, which has no type", n.describe())
	}
	return n.typ
}

// Diagnostics returns the diagnostics of an error value, in order, in a new
// slice. It panics if v is not an error value.
func (v Value) Diagnostics() []Diagnostic {
	n := v.data()
	if n.state != stateError {
		usagePanic("Diagnostics called on %s, which is not an error value", n.describe())
	}
	return slices.Clone(n.data.([]Diagnostic))
}

// AsBool returns the content of a Bool value. It panics if v is not a Bool
// value.
func (v Value) AsBool() bool {
	return v.known(KindBool, "AsBool").data.(bool)
}

// AsString returns the content of a String value, which is in Normalization
// Form C. It panics if v is not a String value.
func (v Value) AsString() string {
	return v.known(KindString, "AsString").data.(string)
}

// AsBigRat returns the content of a Number value as a new exact rational
// number. It panics if v is not a Number value.
func (v Value) AsBigRat() *big.Rat {
	return v.known(KindNumber, "AsBigRat").data.(decimal.Dec).Rat()
}

// AsInt64 returns the content of a Number value and true if it is an integer
// that fits in an int64, and 0 and false otherwise. It panics if v is not a
// Number value.
func (v Value) AsInt64() (int64, bool) {
	return v.known(KindNumber, "AsInt64").data.(decimal.Dec).Int64()
}

// String describes v for messages, as in "text", list(number)[1, 2.5] or
// error(string.invalid_utf8: ...). It is not a format for parsing.
func (v Value) String() string {
	if v.n == nil {
		return "<zero Value>"
	}
	var b strings.Builder
	v.write(&b)
	return b.String()
}

func (v Value) write(b *strings.Builder) {
	n := v.n
	switch n.state {
	case stateError:
		b.WriteString("error(")
		for i, d := range n.data.([]Diagnostic) {
			if i > 0 {
				b.WriteString("; ")
			}
			b.WriteString(string(d.Code))
			b.WriteString(": ")
			b.WriteString(d.Message)
			if d.Path.Len() > 0 {
				b.WriteString(" at ")
				b.WriteString(d.Path.String())
			}
		}
		b.WriteByte(')')
		return
	case statePending:
		b.WriteString("pending(")
		n.data.(Constraint).write(b)
		switch n.null {
		case nullNo:
			b.WriteString(", not null")
		case nullOnly:
			b.WriteString(", null")
		}
		b.WriteByte(')')
		return
	case stateNull:
		b.WriteString("null")
		return
	case stateUnknown:
		b.WriteString("unknown(")
		Range{v}.write(b)
		b.WriteByte(')')
		return
	}
	switch n.typ.t.kind {
	case KindBool:
		b.WriteString(strconv.FormatBool(n.data.(bool)))
	case KindNumber:
		b.WriteString(n.data.(decimal.Dec).String())
	case KindString:
		b.WriteString(strconv.Quote(n.data.(string)))
	case KindCapsule:
		n.typ.write(b)
	default:
		n.writeContainer(b)
	}
}
