package tenon

import "hash/maphash"

// CapsuleOps declares the optional operations of a capsule type whose values
// encapsulate pointers of type *E. A nil function is an operation that the
// type does not declare.
type CapsuleOps[E any] struct {
	// Equals reports whether two encapsulated values are equal. Without it, two
	// values are equal only when they encapsulate the same pointer. A capsule
	// type that declares Equals must also declare Hash. It must be an
	// equivalence relation: every value equal to itself, a equal to b exactly
	// when b is equal to a, and two values equal to a third equal to each
	// other. tenon may take a value to be equal to itself without asking.
	Equals func(a, b *E) bool

	// Hash returns a hash of an encapsulated value. Values that Equals reports
	// equal must have equal hashes.
	Hash func(v *E) uint64

	// Compare orders encapsulated values, returning a negative number, zero or
	// a positive number as a sorts before, together with, or after b. It must
	// be a total order that agrees with the type's equality.
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
	// no encoding cannot be serialized.
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
	// known, unmarked value of Type other than its null. Values that the capsule type's equality
	// reports equal must give identical values, and values it reports unequal
	// must not.
	Encode func(v *E) Value

	// Decode returns the encapsulated value that a value of Type was
	// serialized from, or the diagnostics that say why there is none. It is
	// given a known, unmarked value of Type other than its null, which need not
	// be one that Encode ever returns: input is refused unless decoding and
	// encoding it again gives it back, so a value Decode takes and Encode
	// would write another way is refused as not canonical.
	Decode func(v Value) (*E, []Diagnostic)
}

// capsuleData is what a capsule type declares, with its operations adapted to
// encapsulated values held as any.
type capsuleData struct {
	name    string
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
}

// capsuleEncoding is a declared encoding with its conversions adapted to
// encapsulated values held as any.
type capsuleEncoding struct {
	id     string
	typ    Type
	encode func(v any) Value
	decode func(v Value) (any, []Diagnostic)
}

// Capsule returns a new capsule type, whose values carry pointers of type *E
// through tenon opaquely. Every call returns a distinct type, equal to no other
// type whatever its name and operations. The name describes the type in
// messages.
//
// Capsule panics if ops declares Equals but not Hash.
func Capsule[E any](name string, ops CapsuleOps[E]) Type {
	if ops.Equals != nil && ops.Hash == nil {
		usagePanic("capsule type %q declares Equals but not Hash", name)
	}
	d := &capsuleData{name: name}
	d.accepts = func(v any) bool {
		_, ok := v.(*E)
		return ok
	}
	if f := ops.Equals; f != nil {
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
		case enc.Type.t == nil:
			usagePanic("capsule type %q declares an encoding with the zero Type", name)
		case enc.Encode == nil || enc.Decode == nil:
			usagePanic("capsule type %q declares an encoding without both Encode and Decode", name)
		}
		encode, decode := enc.Encode, enc.Decode
		d.encoding = &capsuleEncoding{
			id:     enc.ID,
			typ:    enc.Type,
			encode: func(v any) Value { return encode(v.(*E)) },
			decode: func(v Value) (any, []Diagnostic) {
				p, diags := decode(v)
				if p == nil {
					return nil, diags
				}
				return p, diags
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
	if f := ops.ConvertFrom; f != nil {
		d.convertFrom = func(t Type) (func(Value) Value, bool) {
			conv, safe := f(t)
			if conv == nil {
				return nil, false
			}
			return conv, safe
		}
	}
	t := &typeData{id: newTypeID(), kind: KindCapsule, capsule: d}
	t.shape = shapeOf(t)
	return Type{t}
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
