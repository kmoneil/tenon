package tenon

import (
	"cmp"
	"fmt"
	"testing"

	"github.com/kmoneil/tenon/conformance"
)

// capsulePoint is the encapsulated type of the capsule tests.
type capsulePoint struct{ x, y int }

func TestConformance_TY041_CapsuleOperations(t *testing.T) {
	conformance.Covers(t, "TY-041")
	d := Capsule("point", CapsuleOps[capsulePoint]{
		Equals:  func(a, b *capsulePoint) bool { return *a == *b },
		Hash:    func(p *capsulePoint) uint64 { return uint64(p.x)<<32 | uint64(uint32(p.y)) },
		Compare: func(a, b *capsulePoint) int { return cmp.Or(cmp.Compare(a.x, b.x), cmp.Compare(a.y, b.y)) },
		Display: func(p *capsulePoint) string { return fmt.Sprintf("(%d, %d)", p.x, p.y) },
	}).t.capsule

	// The declared operations are the ones the capsule type uses.
	p, twin, other := &capsulePoint{1, 2}, &capsulePoint{1, 2}, &capsulePoint{2, 1}
	if !d.equal(p, twin) || d.equal(p, other) {
		t.Error("the declared Equals is not used")
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

	bare := Capsule("bare", CapsuleOps[capsulePoint]{}).t.capsule
	if bare.equals != nil || bare.hash != nil || bare.compare != nil || bare.display != nil {
		t.Error("a capsule type that declares no operations has some")
	}
}

func TestConformance_TY042_CapsuleIdentityEquality(t *testing.T) {
	conformance.Covers(t, "TY-042")
	d := Capsule("point", CapsuleOps[capsulePoint]{}).t.capsule
	p, twin := &capsulePoint{1, 2}, &capsulePoint{1, 2}
	if !d.equal(p, p) {
		t.Error("an encapsulated pointer is not equal to itself")
	}
	if d.equal(p, twin) {
		t.Error("distinct pointers to equal values are equal, though the type declares no Equals")
	}
}
