package tenon

import (
	"cmp"
	"fmt"
	"runtime"
	"testing"
	"time"
	"weak"

	"github.com/kmoneil/tenon/internal/conformance"
)

// capsulePoint is the encapsulated type of the capsule tests.
type capsulePoint struct{ x, y int }

func TestConformance_TY041_CapsuleOperations(t *testing.T) {
	conformance.Covers(t, "TY-041")
	d := NewCapsule("point", CapsuleOps[capsulePoint]{
		Equal:   func(a, b *capsulePoint) bool { return *a == *b },
		Hash:    func(p *capsulePoint) uint64 { return uint64(p.x)<<32 | uint64(uint32(p.y)) },
		Compare: func(a, b *capsulePoint) int { return cmp.Or(cmp.Compare(a.x, b.x), cmp.Compare(a.y, b.y)) },
		Display: func(p *capsulePoint) string { return fmt.Sprintf("(%d, %d)", p.x, p.y) },
	}).t.t.capsule

	// The declared operations are the ones the capsule type uses.
	p, twin, other := &capsulePoint{1, 2}, &capsulePoint{1, 2}, &capsulePoint{2, 1}
	if !d.equal(p, twin) || d.equal(p, other) {
		t.Error("the declared Equal is not used")
	}
	if d.hash(p) != d.hash(twin) || d.hash(p) == d.hash(other) {
		t.Error("the declared Hash is not used")
	}
	if d.compare(p, other) >= 0 || d.compare(other, p) <= 0 || d.compare(p, twin) != 0 {
		t.Error("the declared Compare is not used")
	}
	if got := d.display(other); got != "(2, 1)" {
		t.Errorf("display = %q, want %q", got, "(2, 1)")
	}

	bare := NewCapsule("bare", CapsuleOps[capsulePoint]{}).t.t.capsule
	if bare.equals != nil || bare.hash != nil || bare.compare != nil || bare.display != nil {
		t.Error("a capsule type that declares no operations has some")
	}
}

func TestConformance_TY042_CapsuleIdentityEquality(t *testing.T) {
	conformance.Covers(t, "TY-042")
	d := NewCapsule("point", CapsuleOps[capsulePoint]{}).t.t.capsule
	p, twin := &capsulePoint{1, 2}, &capsulePoint{1, 2}
	if !d.equal(p, p) {
		t.Error("an encapsulated pointer is not equal to itself")
	}
	if d.equal(p, twin) {
		t.Error("distinct pointers to equal values are equal, though the type declares no Equal")
	}
}

// The canonical order of capsule values keeps none of them alive. A type with
// no equality numbers its values by weak pointer, forgotten once each is
// collected; a type with an encoding orders values whose hashes collide by
// their encodings, and numbers nothing.
func TestCapsuleOrderRetainsNothing(t *testing.T) {
	// Large enough that the allocator does not batch them, which could keep
	// one alive beside another.
	type blob struct{ _ [256]byte }
	opaque := NewCapsule("blob", CapsuleOps[blob]{})
	const n = 200
	collected := make([]weak.Pointer[blob], 0, 2*n)
	func() {
		for range n {
			a, b := &blob{}, &blob{}
			collected = append(collected, weak.Make(a), weak.Make(b))
			if s := Set(opaque.Type(), opaque.Value(a), opaque.Value(b)); s.n.state != stateKnown {
				t.Fatalf("a set of two capsule values is %v", s)
			}
		}
	}()
	gone := 0
	for range 50 {
		runtime.GC()
		gone = 0
		for _, w := range collected {
			if w.Value() == nil {
				gone++
			}
		}
		if gone == len(collected) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if gone < len(collected)*9/10 {
		t.Errorf("%d of %d capsule values ordered into sets were collected", gone, len(collected))
	}

	// Values that collide in their declared hash, of a type with an
	// encoding: ordered by the encoding, the same in every run, and not
	// numbered.
	colliding := NewCapsule("colliding_encoded", CapsuleOps[capsulePoint]{
		Equal: func(a, b *capsulePoint) bool { return *a == *b },
		Hash:  func(*capsulePoint) uint64 { return 7 },
		Encoding: &CapsuleEncoding[capsulePoint]{
			ID: "t/colliding", Type: NumberType(),
			Encode: func(p *capsulePoint) Value { return NumberFromInt(int64(p.x)) },
			Decode: func(v Value) (*capsulePoint, error) { x, _ := v.AsInt64(); return &capsulePoint{x: int(x)}, nil },
		},
	})
	for _, order := range [][2]int{{2, 1}, {1, 2}} {
		s := Set(colliding.Type(), colliding.Value(&capsulePoint{x: order[0]}), colliding.Value(&capsulePoint{x: order[1]}))
		if first, _ := colliding.Of(s.Elements()[0]); first.x != 1 {
			t.Errorf("given %v, the set orders %d first, want the smaller encoding, 1", order, first.x)
		}
	}
	capsuleOrder.mu.Lock()
	numbered := 0
	for key := range capsuleOrder.byClass {
		if key.d == colliding.t.t.capsule {
			numbered += len(capsuleOrder.byClass[key])
		}
	}
	capsuleOrder.mu.Unlock()
	if numbered != 0 {
		t.Errorf("%d values of a type with an encoding were numbered", numbered)
	}
}
