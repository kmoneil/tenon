package tenon

import (
	"hash/maphash"
	"runtime"
	"unicode/utf8"
	"weak"
)

// CapsuleOps declares the optional operations of a capsule type whose values
// encapsulate pointers of type *E. A nil function is an operation that the
// type does not declare.
//
// A type that declares neither Equal nor Compare is ordered, where tenon
// orders its values, as in a set and by CanonicalCompare, by weak pointers to
// them, which keep nothing alive and which the Go runtime makes only for
// memory Go allocated. Where *E points to memory Go did not allocate, as C
// allocates or syscall.Mmap maps, ordering such values can end the process
// with a fatal error that no recover catches. A type whose values may point
// there declares Compare, or Equal and Hash, and is then ordered without
// weak pointers.
type CapsuleOps[E any] struct {
	// Equal reports whether two encapsulated values are equal. Without it, two
	// values are equal only when they encapsulate the same pointer. A capsule
	// type that declares Equal must also declare Hash. It must be an
	// equivalence relation: every value equal to itself, a equal to b exactly
	// when b is equal to a, and two values equal to a third equal to each
	// other. tenon may take a value to be equal to itself without asking.
	Equal func(a, b *E) bool

	// Hash returns a hash of an encapsulated value. Values that Equal reports
	// equal must have equal hashes.
	//
	// A type that declares Equal and neither Compare nor an Encoding is put in
	// order, where a set or CanonicalCompare needs one, by its hashes, and
	// values whose hashes collide by the order the run first met them in.
	// tenon keeps one value of each equality class whose hash collides with
	// another's for the rest of the run, so that the order holds: a hash that
	// seldom collides keeps next to nothing, and declaring Compare or an
	// Encoding keeps nothing at all.
	Hash func(v *E) uint64

	// Compare orders encapsulated values, returning a negative number, zero or
	// a positive number as a sorts before, together with, or after b. It must
	// be a total order that agrees with the type's equality. A type that
	// declares it keeps no value for the sake of its order, which one that
	// declares Equal without it may: see Hash.
	Compare func(a, b *E) int

	// Display returns the display form of an encapsulated value.
	Display func(v *E) string

	// ConvertTo declares conversions from the capsule type to other types.
	// Given a type, it returns the function that converts an encapsulated
	// value to a value of that type, and whether the conversion is safe; or a
	// nil function, where the capsule type declares no conversion to that
	// type. It must give the same answer for a type every time it is asked.
	// The function returns a known value of the type, or an error value where
	// the value it is given does not convert.
	ConvertTo func(t Type) (convert func(v *E) Value, safe bool)

	// ConvertFrom declares conversions to the capsule type from other types,
	// as ConvertTo does in the other direction. The function is given a known
	// value of the type, which is not null, and returns a known value of the
	// capsule type, or an error value.
	ConvertFrom func(t Type) (convert func(v Value) Value, safe bool)

	// Encoding declares how the type's values are serialized. A value that
	// holds a capsule value, or names a capsule type, of a type that declares
	// no encoding cannot be serialized. A type that declares an encoding must
	// declare Equal, and so Hash: a value read back is a new pointer, equal
	// to the one written only by the declared equality.
	Encoding *CapsuleEncoding[E]
}

// CapsuleEncoding declares how a capsule type's values are serialized: as
// values of another type, which are serialized as tenon values are.
type CapsuleEncoding[E any] struct {
	// ID identifies the capsule type in what is serialized. It must be the
	// same in every process that reads what another wrote, and no other
	// capsule type serialized alongside it may use it.
	ID string

	// Type is the type that the capsule type's values are serialized as.
	Type Type

	// Encode returns the value that an encapsulated value is serialized as: a
	// known, unmarked value of Type other than its null. Values that the
	// capsule type's equality reports equal must give identical values, and
	// values it reports unequal must not.
	Encode func(v *E) Value

	// Decode returns the encapsulated value that a value of Type was
	// serialized from, or an error saying why there is none: one that is a
	// *Error contributes its diagnostics, and any other its text, with code
	// CodeSerializeDecoderFailed, and Deserialize's error keeps it as a
	// cause. It is given a known, unmarked value of Type other than its null,
	// which need not be one that Encode ever returns: input is refused unless
	// decoding and encoding it again gives it back, so a value Decode takes
	// and Encode would write another way is refused as not canonical.
	Decode func(v Value) (*E, error)
}

// capsuleData is what a capsule type declares, with its operations adapted to
// encapsulated values held as any.
type capsuleData struct {
	name    string
	goType  string              // E as Go syntax names it, as in main.point
	accepts func(v any) bool    // whether v is a pointer of the encapsulated type
	equals  func(a, b any) bool // nil if not declared
	hash    func(v any) uint64  // nil if not declared
	compare func(a, b any) int  // nil if not declared
	display func(v any) string  // nil if not declared
	// convertTo and convertFrom return a declared conversion and whether it is
	// safe, or a nil function; each is nil if not declared.
	convertTo   func(t Type) (func(v any) Value, bool)
	convertFrom func(t Type) (func(v Value) Value, bool)
	// encoding is what an Encoding declares, or nil.
	encoding *capsuleEncoding
	// weakKey returns a weak pointer to an encapsulated value, comparable
	// and equal exactly where the pointers are, which keeps nothing alive;
	// forgetWhenCollected runs forget with key once the value is collected.
	// The canonical order numbers the values of a type with no equality by
	// these, so that numbering one does not keep it (capsuleOrder).
	weakKey             func(v any) any
	forgetWhenCollected func(v, key any)
}

// capsuleEncoding is a declared encoding with its conversions adapted to
// encapsulated values held as any.
type capsuleEncoding struct {
	id     string
	typ    Type
	encode func(v any) Value
	decode func(v Value) (any, error)
}

// encoded returns the value the type d serializes v as, which d's encoding
// must declare: what Encode returns, held to what Encode promises, a known,
// unmarked value of the encoding's type other than its null. The encoder and
// the canonical order, which breaks ties between colliding hashes by the
// encodings, both read it through here, so that an Encode that breaks its
// promise is a usage error wherever it is met rather than a nil dereference
// where the order meets it first.
func (d *capsuleData) encoded(v any) Value {
	payload := d.encoding.encode(v)
	if !isPayload(payload) || payload.n.typ != d.encoding.typ {
		usagePanic("capsule type %q serialized a value as %s, not a known, unmarked value of %s other than null",
			d.name, payload, d.encoding.typ)
	}
	return payload
}

// CapsuleType is a capsule type whose values encapsulate pointers of type *E:
// the handle that NewCapsule returns, through which a program builds the
// type's values and reads them back, the pointer type checked where the
// program is compiled. Type gives the capsule type itself, for use wherever a
// type is.
//
// A nil or zero CapsuleType handles no capsule type: every method but Equal
// and GoString panics when called on it.
type CapsuleType[E any] struct {
	t Type
}

// Equal reports whether c and d handle the same capsule type, a handle that
// NewCapsule did not make handling none. It is the method go-cmp's cmp.Equal
// calls, so a struct holding handles compares by the types they handle.
func (c *CapsuleType[E]) Equal(d *CapsuleType[E]) bool {
	return c.handled() == d.handled()
}

// GoString returns Go syntax that makes a capsule type of c's name and E,
// which the %#v verb prints, as in
// tenon.NewCapsule[main.point]("point", tenon.CapsuleOps[main.point]{}). It
// makes a new type, declaring no operations, which is another type than c's,
// as every capsule type is: Value.GoString says more.
func (c *CapsuleType[E]) GoString() string {
	switch {
	case c == nil:
		return "(*tenon.CapsuleType[" + goTypeName[E]() + "])(nil)"
	case c.t.t == nil:
		return "&tenon.CapsuleType[" + goTypeName[E]() + "]{}"
	}
	var w goWriter
	w.writeCapsule(c.t.t.capsule)
	return w.String()
}

// handled returns the capsule type c handles, the zero Type where it handles
// none.
func (c *CapsuleType[E]) handled() Type {
	if c == nil {
		return Type{}
	}
	return c.t
}

// NewCapsule returns a new capsule type, whose values carry pointers of type
// *E through tenon opaquely. Every call returns a distinct type, equal to no
// other type whatever its name and operations. The name describes the type in
// messages. A type whose values may point to memory Go did not allocate
// declares Compare, or Equal and Hash, as CapsuleOps says.
//
// NewCapsule panics if ops declares Equal but not Hash, or declares an
// encoding with no identifier, an identifier that is not valid UTF-8, the
// zero Type, without both Encode and Decode, or without Equal: a value read
// back from its encoding is a new pointer, which only a declared equality can
// find equal to the value written, as a round trip and a set's one encoding
// need.
func NewCapsule[E any](name string, ops CapsuleOps[E]) *CapsuleType[E] {
	if ops.Equal != nil && ops.Hash == nil {
		usagePanic("capsule type %q declares Equal but not Hash", name)
	}
	d := &capsuleData{name: name, goType: goTypeName[E]()}
	d.accepts = func(v any) bool {
		_, ok := v.(*E)
		return ok
	}
	d.weakKey = func(v any) any { return weak.Make(v.(*E)) }
	d.forgetWhenCollected = func(v, key any) { runtime.AddCleanup(v.(*E), forgetCapsule, key) }
	if f := ops.Equal; f != nil {
		d.equals = func(a, b any) bool { return f(a.(*E), b.(*E)) }
	}
	if f := ops.Hash; f != nil {
		d.hash = func(v any) uint64 { return f(v.(*E)) }
	}
	if f := ops.Compare; f != nil {
		d.compare = func(a, b any) int { return f(a.(*E), b.(*E)) }
	}
	if f := ops.Display; f != nil {
		d.display = func(v any) string { return f(v.(*E)) }
	}
	if enc := ops.Encoding; enc != nil {
		switch {
		case enc.ID == "":
			usagePanic("capsule type %q declares an encoding with no identifier", name)
		case !utf8.ValidString(enc.ID):
			// The document format writes the identifier as CBOR text, which
			// holds well-formed UTF-8, so what an invalid identifier would
			// serialize as could not be decoded.
			usagePanic("capsule type %q declares an encoding whose identifier is not valid UTF-8", name)
		case enc.Type.t == nil:
			usagePanic("capsule type %q declares an encoding with the zero Type", name)
		case enc.Encode == nil || enc.Decode == nil:
			usagePanic("capsule type %q declares an encoding without both Encode and Decode", name)
		case ops.Equal == nil:
			usagePanic("capsule type %q declares an encoding but not Equal; a value read back is a new pointer, equal to the one written only by a declared equality", name)
		}
		encode, decode := enc.Encode, enc.Decode
		d.encoding = &capsuleEncoding{
			id:     enc.ID,
			typ:    enc.Type,
			encode: func(v any) Value { return encode(v.(*E)) },
			decode: func(v Value) (any, error) {
				// A nil *E put into an any is not nil to the == the decoder
				// asks, so a Decode that returned neither a value nor an
				// error is handed on as a plain nil, which the decoder
				// refuses as a broken contract rather than encapsulating.
				p, err := decode(v)
				if p == nil {
					return nil, err
				}
				return p, err
			},
		}
	}
	if f := ops.ConvertTo; f != nil {
		d.convertTo = func(t Type) (func(any) Value, bool) {
			conv, safe := f(t)
			if conv == nil {
				return nil, false
			}
			return func(v any) Value { return conv(v.(*E)) }, safe
		}
	}
	// ConvertFrom already has the adapted signature, its values arriving as
	// Values, so it is taken as it is; capsuleConversion reads safe only
	// beside a non-nil conversion, so nothing normalizes the nil case.
	d.convertFrom = ops.ConvertFrom
	t := &typeData{id: newTypeID(), kind: KindCapsule, capsule: d}
	t.shape = shapeOf(t)
	return &CapsuleType[E]{t: Type{t: t}}
}

// Type returns the capsule type, for use wherever a type is: in a collection
// type, a constraint, a conversion or the Decoders given to Deserialize.
func (c *CapsuleType[E]) Type() Type { return c.made("Type") }

// Value returns the value of the capsule type that encapsulates p. It panics
// if p is nil.
func (c *CapsuleType[E]) Value(p *E) Value {
	t := c.made("Value")
	if p == nil {
		usagePanic("Value called with a nil pointer for capsule type %s", t)
	}
	return Value{n: &node{state: stateKnown, typ: t, data: p}}
}

// Of returns the pointer that v encapsulates, and true, where v is a known
// value of the capsule type, marked or not; and nil and false where v is
// anything else: a null or an unknown of the type, a value of another type, a
// pending value or an error value. It panics on the zero Value.
func (c *CapsuleType[E]) Of(v Value) (*E, bool) {
	t := c.made("Of")
	if n := v.data(); n.state == stateKnown && n.typ == t {
		return n.data.(*E), true
	}
	return nil, false
}

// made returns the capsule type c handles, panicking where c is not a handle
// NewCapsule made.
func (c *CapsuleType[E]) made(method string) Type {
	if c == nil || c.t.t == nil {
		usagePanic("%s called on a CapsuleType that NewCapsule did not make", method)
	}
	return c.t
}

// equal reports whether two values encapsulated by the capsule type are equal:
// by the declared equality if there is one, and otherwise by the identity of
// the encapsulated pointers.
func (d *capsuleData) equal(a, b any) bool {
	if d.equals != nil {
		return d.equals(a, b)
	}
	return a == b
}

// writeHash writes an encapsulated value into h: by the declared hash where
// there is one, and otherwise by the pointer itself, which is what equal
// compares when there is none.
func (d *capsuleData) writeHash(h *maphash.Hash, v any) {
	if d.hash != nil {
		writeUint(h, d.hash(v))
		return
	}
	maphash.WriteComparable(h, v)
}

// CapsuleName returns the name given to a capsule type. It panics if t is not a
// capsule type.
func (t Type) CapsuleName() string {
	return t.mustKind(KindCapsule, "CapsuleName").capsule.name
}

// capsuleConversion returns the conversion from t to s that a capsule type
// declares, where either is a capsule type: the source type's conversion to s,
// and failing that the target type's conversion from t. Exactly one of the two
// functions is set when there is one, and safe says whether it is safe.
func capsuleConversion(t, s Type) (to func(any) Value, from func(Value) Value, safe, ok bool) {
	if d := t.t.capsule; d != nil && d.convertTo != nil {
		if f, safe := d.convertTo(s); f != nil {
			return f, nil, safe, true
		}
	}
	if d := s.t.capsule; d != nil && d.convertFrom != nil {
		if f, safe := d.convertFrom(t); f != nil {
			return nil, f, safe, true
		}
	}
	return nil, nil, false, false
}
