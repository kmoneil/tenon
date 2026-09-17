package tenon

import (
	"encoding/binary"
	"runtime"
	"sync"
	"sync/atomic"
	"weak"
)

// Composite types are interned. Constructing a type with the same structure as
// a live type returns that type, so type equality is a pointer comparison. The
// registry holds types weakly: a type that nothing references any more can be
// garbage collected, and its registry entry is then removed.

// registry maps the interning key of each interned type to the type.
var registry = struct {
	sync.Mutex
	types map[string]weak.Pointer[typeData]
}{types: make(map[string]weak.Pointer[typeData])}

// typeIDs counts the ids handed out after those of the primitive types, which
// are 1 to 3.
var typeIDs atomic.Uint64

// newTypeID returns an id that no other type in the process has had.
func newTypeID() uint64 { return 3 + typeIDs.Add(1) }

// intern returns the canonical type for the new description d: the live type
// with the same structure if there is one, and otherwise d itself, which
// becomes canonical.
func intern(d *typeData) Type {
	key := d.internKey()
	registry.Lock()
	defer registry.Unlock()
	if p, ok := registry.types[key]; ok {
		if live := p.Value(); live != nil {
			return Type{live}
		}
	}
	d.id = newTypeID()
	// Set before the type is published, and never written again, so every
	// reader of a shared type sees a shape that was there all along.
	d.shape = shapeOf(d)
	registry.types[key] = weak.Make(d)
	runtime.AddCleanup(d, forgetType, key)
	return Type{d}
}

// forgetType removes the registry entry for key after the type it held has
// been garbage collected, unless a new type has taken the entry since.
func forgetType(key string) {
	registry.Lock()
	defer registry.Unlock()
	if p, ok := registry.types[key]; ok && p.Value() == nil {
		delete(registry.types, key)
	}
}

// internKey returns a string identifying the structure of d. Component types
// appear by id. Ids are never reused, and the components of a live type are
// live, so two live types have the same key exactly when they have the same
// structure.
func (d *typeData) internKey() string {
	buf := []byte{byte(d.kind)}
	switch d.kind {
	case KindList, KindSet, KindMap:
		buf = binary.AppendUvarint(buf, d.elem.t.id)
	case KindTuple:
		buf = binary.AppendUvarint(buf, uint64(len(d.elems)))
		for _, e := range d.elems {
			buf = binary.AppendUvarint(buf, e.t.id)
		}
	case KindObject:
		buf = binary.AppendUvarint(buf, uint64(len(d.attrs)))
		for _, a := range d.attrs {
			buf = binary.AppendUvarint(buf, uint64(len(a.name)))
			buf = append(buf, a.name...)
			buf = binary.AppendUvarint(buf, a.typ.t.id)
		}
	}
	return string(buf)
}
