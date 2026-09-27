package tenon_test

import (
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
)

// sampleCapsule is one fixed capsule type for tests that list sample types.
var sampleCapsule = tenon.NewCapsule("sample", tenon.CapsuleOps[struct{}]{})

func TestConformance_TY040_CapsuleIdentity(t *testing.T) {
	conformance.Covers(t, "TY-040")
	type handle struct{}
	handleType := tenon.NewCapsule("handle", tenon.CapsuleOps[handle]{})
	a := handleType.Type()
	b := tenon.NewCapsule("handle", tenon.CapsuleOps[handle]{}).Type()
	if a == b || a.Equal(b) {
		t.Errorf("two constructions of %v are the same type", a)
	}
	again := handleType.Type()
	if again != a || !again.Equal(a) {
		t.Errorf("%v is not the same type as itself", a)
	}
	if a.Kind() != tenon.KindCapsule || a.IsCollection() || a.IsStructural() || a.CapsuleName() != "handle" {
		t.Errorf("%v: kind %v, name %q", a, a.Kind(), a.CapsuleName())
	}
	if got := a.String(); got != `capsule("handle")` {
		t.Errorf("String() = %s", got)
	}

	// Types built from distinct capsule types are distinct too.
	if tenon.ListType(a) == tenon.ListType(b) || tenon.ListType(a) != tenon.ListType(again) {
		t.Error("list types do not follow the identity of their capsule element types")
	}
	if tenon.ObjectType(map[string]tenon.Type{"h": a}) == tenon.ObjectType(map[string]tenon.Type{"h": b}) {
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

	mustPanicUsage(t, `capsule type "point" declares Equal but not Hash`, func() {
		tenon.NewCapsule("point", tenon.CapsuleOps[point]{Equal: equals})
	})
	mustPanicUsage(t, "declares Equal but not Hash", func() {
		tenon.NewCapsule("point", tenon.CapsuleOps[point]{Equal: equals, Compare: compare, Display: display})
	})
	// An encoding needs an equality: a value read back is a new pointer,
	// which only a declared equality finds equal to the one written.
	mustPanicUsage(t, `capsule type "point" declares an encoding but not Equal`, func() {
		tenon.NewCapsule("point", tenon.CapsuleOps[point]{Hash: hash, Encoding: &tenon.CapsuleEncoding[point]{
			ID: "t/point", Type: tenon.NumberType(),
			Encode: func(p *point) tenon.Value { return tenon.NumberFromInt(int64(p.x)) },
			Decode: func(tenon.Value) (*point, []tenon.Diagnostic) { return &point{}, nil },
		}})
	})

	// Any other choice of operations is accepted.
	for _, ops := range []tenon.CapsuleOps[point]{
		{},
		{Hash: hash},
		{Equal: equals, Hash: hash},
		{Compare: compare},
		{Display: display},
		{Equal: equals, Hash: hash, Compare: compare, Display: display},
	} {
		if typ := tenon.NewCapsule("point", ops).Type(); typ.Kind() != tenon.KindCapsule {
			t.Errorf("NewCapsule returned a %v type", typ.Kind())
		}
	}
}
