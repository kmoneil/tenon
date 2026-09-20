package tenon

import (
	"fmt"
	"testing"
)

// unionTestRun numbers the calls of the helper below, so that every call
// unifies object types that no earlier call interned.
var unionTestRun int

// TestObjectUnionsAreBuiltOnce holds unifying object types to one type built,
// however many types are unified: the union holds every attribute of every
// type, so building it again for every type after the first, as unifying two
// at a time does, builds and interns a type per type unified, all but the
// last of them waste. Types interned carry an id that nothing else hands out,
// so counting the ids is counting the types built.
func TestObjectUnionsAreBuiltOnce(t *testing.T) {
	built := func(n int) uint64 {
		unionTestRun++
		types := make([]Type, n)
		for i := range types {
			types[i] = Object(map[string]Type{fmt.Sprintf("r%d-a%04d", unionTestRun, i): NumberType()})
		}
		before := typeIDs.Load()
		u, ok := unifyTypes(types, Safe)
		switch {
		case !ok:
			t.Fatalf("%d object types of distinct attributes did not unify", n)
		case len(u.t.attrs) != n:
			t.Fatalf("the union of %d object types of distinct attributes holds %d attributes", n, len(u.t.attrs))
		}
		return typeIDs.Load() - before
	}
	// v0.1.0 built 15 types for 16 and 63 for 64.
	if small, large := built(16), built(64); small != large {
		t.Errorf("unifying 16 object types builds %d types and unifying 64 builds %d, where the union is one type either way", small, large)
	}
}
