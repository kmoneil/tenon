//go:build unix

package tenon_test

import (
	"cmp"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
)

// mapped returns two points in memory Go did not allocate, mapped from the
// operating system as a memory-mapped file or C's allocator gives it, and
// unmapped once the test is done.
func mapped(t *testing.T) (*point, *point) {
	t.Helper()
	mem, err := syscall.Mmap(-1, 0, 4096, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_ANON|syscall.MAP_PRIVATE)
	if err != nil {
		t.Fatalf("mapping memory: %v", err)
	}
	t.Cleanup(func() { _ = syscall.Munmap(mem) })
	a, b := (*point)(unsafe.Pointer(&mem[0])), (*point)(unsafe.Pointer(&mem[64]))
	*a, *b = point{1, 0}, point{2, 0}
	return a, b
}

// TestConformance_TY042_CapsuleValuesOutsideTheGoHeap shows what CapsuleOps
// documents of a capsule type whose values point to memory Go did not
// allocate. A type declaring neither Equal nor Compare is ordered by weak
// pointers to its values (TY-042, EQ-045), which the runtime makes only for
// memory it allocated, so ordering two such values, as building a set does,
// ends the process with a fatal error that no recover catches: the set is
// built in a child process, this test run again. Holding the values in a
// list, which orders nothing, works, and so does ordering them where the
// type declares Compare, or Equal and Hash, the remedy the documentation
// gives.
func TestConformance_TY042_CapsuleValuesOutsideTheGoHeap(t *testing.T) {
	const child = "TENON_TEST_CAPSULE_SET_OVER_MAPPED_MEMORY"
	if os.Getenv(child) == "1" {
		a, b := mapped(t)
		opaque := tenon.NewCapsule("mapped", tenon.CapsuleOps[point]{})
		tenon.Set(opaque.Type(), opaque.Value(a), opaque.Value(b))
		return
	}
	conformance.Covers(t, "TY-042", "EQ-045", "TY-041")
	cmd := exec.Command(os.Args[0], "-test.run=^TestConformance_TY042_CapsuleValuesOutsideTheGoHeap$", "-test.count=1")
	cmd.Env = append(os.Environ(), child+"=1")
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "fatal error") {
		t.Errorf("a set of two values of a type declaring neither Equal nor Compare, over mapped memory, gave %v, want the process ended by a fatal error:\n%s", err, out)
	}

	a, b := mapped(t)
	opaque := tenon.NewCapsule("mapped_listed", tenon.CapsuleOps[point]{})
	if l := tenon.List(opaque.Type(), opaque.Value(a), opaque.Value(b)); l.Len() != 2 {
		t.Errorf("a list of the two values is %v", l)
	}
	for _, ops := range []tenon.CapsuleOps[point]{
		{Compare: func(p, q *point) int { return cmp.Compare(p.x, q.x) }},
		{Equal: func(p, q *point) bool { return *p == *q }, Hash: func(p *point) uint64 { return uint64(p.x) }},
	} {
		declared := tenon.NewCapsule("mapped_declared", ops)
		if s := tenon.Set(declared.Type(), declared.Value(a), declared.Value(b)); s.Len() != 2 {
			t.Errorf("a set of the two values of a type declaring an order or an equality is %v", s)
		}
	}
}
