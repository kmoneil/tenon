package tenon

import (
	"runtime"
	"testing"
	"time"
)

// TestInternedTypesAreCollected checks that the registry does not keep types
// alive: once nothing references a type, the garbage collector reclaims it and
// its registry entry goes away.
func TestInternedTypesAreCollected(t *testing.T) {
	key := internUnreferencedType()
	deadline := time.Now().Add(5 * time.Second)
	for {
		runtime.GC()
		registry.Lock()
		_, present := registry.types[key]
		registry.Unlock()
		if !present {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the registry entry of an unreferenced type was not removed")
		}
		time.Sleep(time.Millisecond)
	}
}

// internUnreferencedType interns a type that nothing else uses and returns its
// registry key, keeping no reference to the type.
//
//go:noinline
func internUnreferencedType() string {
	typ := Object(map[string]Type{"only-in-TestInternedTypesAreCollected": StringType()})
	return typ.t.internKey()
}
