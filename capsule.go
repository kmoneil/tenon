package tenon

// CapsuleOps declares the optional operations of a capsule type whose values
// encapsulate pointers of type *E. A nil function is an operation that the
// type does not declare.
type CapsuleOps[E any] struct {
	// Equals reports whether two encapsulated values are equal. Without it, two
	// values are equal only when they encapsulate the same pointer. A capsule
	// type that declares Equals must also declare Hash.
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
	return Type{&typeData{id: newTypeID(), kind: KindCapsule, capsule: d}}
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

// CapsuleName returns the name given to a capsule type. It panics if t is not a
// capsule type.
func (t Type) CapsuleName() string {
	return t.mustKind(KindCapsule, "CapsuleName").capsule.name
}
