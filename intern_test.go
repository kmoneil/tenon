package tenon_test

import (
	"sync"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/conformance"
)

// nested builds the same deeply structured type afresh on every call.
func nested() tenon.Type {
	str, num := tenon.StringType(), tenon.NumberType()
	return tenon.Map(tenon.Object(map[string]tenon.Type{
		"id":     num,
		"labels": tenon.Map(str),
		"points": tenon.List(tenon.Tuple(num, num)),
		"owner":  tenon.Object(map[string]tenon.Type{"name": str, "groups": tenon.Set(str)}),
	}))
}

func TestConformance_TY014_ObjectAttributeOrder(t *testing.T) {
	conformance.Covers(t, "TY-014")
	first := map[string]tenon.Type{}
	first["name"] = tenon.StringType()
	first["tags"] = tenon.List(tenon.StringType())
	second := map[string]tenon.Type{}
	second["tags"] = tenon.List(tenon.StringType())
	second["name"] = tenon.StringType()
	a, b := tenon.Object(first), tenon.Object(second)
	if a != b || !a.Equals(b) {
		t.Errorf("%v and %v are different types", a, b)
	}

	// A different name or attribute type makes a different type.
	for _, other := range []tenon.Type{
		tenon.Object(map[string]tenon.Type{"name": tenon.StringType()}),
		tenon.Object(map[string]tenon.Type{"name": tenon.StringType(), "tags": tenon.Set(tenon.StringType())}),
		tenon.Object(map[string]tenon.Type{"name": tenon.StringType(), "tag": tenon.List(tenon.StringType())}),
	} {
		if a == other || a.Equals(other) {
			t.Errorf("%v and %v are the same type", a, other)
		}
	}
}

func TestConformance_TY015_TupleOrder(t *testing.T) {
	conformance.Covers(t, "TY-015")
	str, num := tenon.StringType(), tenon.NumberType()
	a := tenon.Tuple(str, tenon.List(num))
	b := tenon.Tuple(tenon.StringType(), tenon.List(tenon.NumberType()))
	if a != b || !a.Equals(b) {
		t.Errorf("%v and %v are different types", a, b)
	}
	for _, p := range [][2]tenon.Type{
		{tenon.Tuple(str, num), tenon.Tuple(num, str)},
		{tenon.Tuple(str), tenon.Tuple(str, str)},
		{tenon.Tuple(), tenon.Tuple(str)},
		{tenon.Tuple(str, num), tenon.Tuple(str, tenon.List(num))},
	} {
		if p[0] == p[1] || p[0].Equals(p[1]) {
			t.Errorf("%v and %v are the same type", p[0], p[1])
		}
	}
}

func TestConformance_TY020_StructuralEquality(t *testing.T) {
	conformance.Covers(t, "TY-020")
	if a, b := nested(), nested(); a != b || !a.Equals(b) {
		t.Errorf("two constructions of %v are different types", a)
	}

	// Types of equal structure are one map key.
	counts := map[tenon.Type]int{}
	for range 3 {
		counts[nested()]++
		counts[tenon.List(tenon.StringType())]++
	}
	if len(counts) != 2 || counts[nested()] != 3 {
		t.Errorf("counts = %v, want 3 for each of two types", counts)
	}
}

func TestConformance_TY021_DeterministicEquality(t *testing.T) {
	conformance.Covers(t, "TY-021")
	names := []string{"alpha", "beta", "gamma", "delta", "epsilon", "caf\u00e9"}
	attrType := func(i int) tenon.Type {
		switch i % 3 {
		case 0:
			return tenon.List(tenon.NumberType())
		case 1:
			return tenon.Tuple(tenon.StringType(), tenon.BoolType())
		}
		return tenon.Object(map[string]tenon.Type{"n": tenon.NumberType()})
	}
	// build constructs the attribute types and fills the map in the given
	// order.
	build := func(order []int) tenon.Type {
		attrs := make(map[string]tenon.Type, len(order))
		for _, i := range order {
			attrs[names[i]] = attrType(i)
		}
		return tenon.Object(attrs)
	}

	want := build([]int{0, 1, 2, 3, 4, 5})
	// Every one of the 720 construction orders, not a sample of them.
	var enumerate func(order, rest []int)
	enumerate = func(order, rest []int) {
		if len(rest) == 0 {
			if got := build(order); got != want {
				t.Fatalf("construction order %v gave %v, which is not the interned %v", order, got, want)
			}
			return
		}
		for i, pick := range rest {
			remaining := make([]int, 0, len(rest)-1)
			remaining = append(remaining, rest[:i]...)
			remaining = append(remaining, rest[i+1:]...)
			enumerate(append(order, pick), remaining)
		}
	}
	enumerate(nil, []int{0, 1, 2, 3, 4, 5})

	// Equality is decided at once, however deep the types.
	deep, deeper := tenon.StringType(), tenon.StringType()
	for range 10000 {
		deep, deeper = tenon.List(deep), tenon.List(deeper)
	}
	if deep != deeper || !deep.Equals(deeper) || deep.Equals(tenon.List(deep)) {
		t.Error("deeply nested list types compared wrongly")
	}
}

func TestTypeInterningConcurrent(t *testing.T) {
	const workers = 16
	results := make([]tenon.Type, workers)
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			for range 200 {
				results[w] = nested()
			}
		})
	}
	wg.Wait()
	for w, got := range results {
		if got != results[0] {
			t.Errorf("worker %d built %v, which is not the type that worker 0 built", w, got)
		}
	}
}
