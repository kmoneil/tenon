package tenon

import (
	"runtime"
	"testing"
	"time"

	"github.com/kmoneil/tenon/internal/conformance"
)

// TestConformance_EQ032_HashesSurviveTypeCollection checks that a hash does not
// depend on which types happen to be alive. Composite types are interned, and
// one that nothing references is collected; building it again gives a type with
// a new id. A hash that began with the id therefore changed whenever the
// collector ran, so a table keyed by hash forgot its entries and two values
// Identical reports the same hashed differently within one run.
func TestConformance_EQ032_HashesSurviveTypeCollection(t *testing.T) {
	conformance.Covers(t, "EQ-030", "EQ-032")
	first, key := hashOfAnUnreferencedType()
	deadline := time.Now().Add(5 * time.Second)
	for {
		runtime.GC()
		registry.Lock()
		_, present := registry.types[key]
		registry.Unlock()
		if !present {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the registry entry of an unreferenced type was not removed")
		}
		time.Sleep(time.Millisecond)
	}
	// The type is gone, so building the value again interns a new one under a
	// new id. It is the same value, and hashes the same.
	second, _ := hashOfAnUnreferencedType()
	if first != second {
		t.Errorf("a value hashed %d, and %d once its type had been collected and built again", first, second)
	}
}

// hashOfAnUnreferencedType hashes a value of an object type that nothing else
// uses, and returns the hash with the type's registry key, keeping no
// reference to the type or the value.
//
//go:noinline
func hashOfAnUnreferencedType() (uint64, string) {
	v := ObjectVal(map[string]Value{"only-in-TestConformance_EQ032_HashesSurviveTypeCollection": NumberFromInt(1)})
	return Hash(v), v.Type().t.internKey()
}
