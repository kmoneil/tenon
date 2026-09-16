package tenon_test

import (
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/conformance"
)

// sampleCapsule is one fixed capsule type for tests that list sample types.
var sampleCapsule = tenon.Capsule("sample", tenon.CapsuleOps[struct{}]{})

func TestConformance_TY040_CapsuleIdentity(t *testing.T) {
	conformance.Covers(t, "TY-040")
	type handle struct{ fd int }
	a := tenon.Capsule("handle", tenon.CapsuleOps[handle]{})
	b := tenon.Capsule("handle", tenon.CapsuleOps[handle]{})
	if a == b || a.Equals(b) {
		t.Errorf("two constructions of %v are the same type", a)
	}
	again := a
	if again != a || !again.Equals(a) {
		t.Errorf("%v is not the same type as itself", a)
	}
	if a.Kind() != tenon.KindCapsule || a.IsCollection() || a.IsStructural() || a.CapsuleName() != "handle" {
		t.Errorf("%v: kind %v, name %q", a, a.Kind(), a.CapsuleName())
	}
	if got := a.String(); got != `capsule("handle")` {
		t.Errorf("String() = %s", got)
	}

	// Types built from distinct capsule types are distinct too.
	if tenon.List(a) == tenon.List(b) || tenon.List(a) != tenon.List(again) {
		t.Error("list types do not follow the identity of their capsule element types")
	}
	if tenon.Object(map[string]tenon.Type{"h": a}) == tenon.Object(map[string]tenon.Type{"h": b}) {
		t.Error("object types do not follow the identity of their capsule attribute types")
	}

	// Exactly a capsule type is satisfied by that type alone.
	if !tenon.Satisfies(tenon.Exactly(a), a) || tenon.Satisfies(tenon.Exactly(a), b) || !tenon.Satisfies(tenon.Any(), b) {
		t.Error("capsule types satisfy constraints wrongly")
	}
	mustPanicUsage(t, "whose kind is String, not Capsule", func() { tenon.StringType().CapsuleName() })
}

func TestConformance_TY041_CapsuleEqualityNeedsHash(t *testing.T) {
	conformance.Covers(t, "TY-041")
	type point struct{ x, y int }
	equals := func(a, b *point) bool { return *a == *b }
	hash := func(p *point) uint64 { return uint64(p.x)<<32 | uint64(uint32(p.y)) }
	compare := func(a, b *point) int { return a.x - b.x }
	display := func(p *point) string { return "point" }

	mustPanicUsage(t, `capsule type "point" declares Equals but not Hash`, func() {
		tenon.Capsule("point", tenon.CapsuleOps[point]{Equals: equals})
	})
	mustPanicUsage(t, "declares Equals but not Hash", func() {
		tenon.Capsule("point", tenon.CapsuleOps[point]{Equals: equals, Compare: compare, Display: display})
	})

	// Any other choice of operations is accepted.
	for _, ops := range []tenon.CapsuleOps[point]{
		{},
		{Hash: hash},
		{Equals: equals, Hash: hash},
		{Compare: compare},
		{Display: display},
		{Equals: equals, Hash: hash, Compare: compare, Display: display},
	} {
		if typ := tenon.Capsule("point", ops); typ.Kind() != tenon.KindCapsule {
			t.Errorf("Capsule returned a %v type", typ.Kind())
		}
	}
}
