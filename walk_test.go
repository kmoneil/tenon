package tenon_test

import (
	"slices"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
)

// walked returns the paths and values Walk visits in v, taking every one.
func walked(v tenon.Value) ([]tenon.Path, []tenon.Value) {
	var ps []tenon.Path
	var vs []tenon.Value
	tenon.Walk(v, func(p tenon.Path, v tenon.Value) tenon.WalkAction {
		ps, vs = append(ps, p), append(vs, v)
		return tenon.WalkContinue
	})
	return ps, vs
}

func TestConformance_VA027_Walk(t *testing.T) {
	conformance.Covers(t, "VA-027")
	str, num := tenon.StringType(), tenon.NumberType()
	untyped := tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly())
	doc := tenon.Object(map[string]tenon.Value{
		"z":    tenon.String("last"),
		"a":    tenon.List(num, tenon.NumberFromInt(1), tenon.NumberFromInt(2)),
		"m":    tenon.Map(str, map[string]tenon.Value{"k2": tenon.String("2"), "k1": tenon.String("1")}),
		"s":    tenon.Set(num, tenon.NumberFromInt(30), tenon.NumberFromInt(10)),
		"held": tenon.Tuple(untyped, tenon.Object(map[string]tenon.Value{"x": tenon.Bool(true)})),
		"u":    tenon.Unknown(tenon.ListType(str)),
		"n":    tenon.Null(tenon.ListType(str)),
		"p":    tenon.Pending(tenon.Any()),
	})
	ps, vs := walked(doc)
	want := []string{".", ".a", ".a[0]", ".a[1]", ".held", ".held[0]", ".held[1]", ".held[1].x",
		".m", `.m["k1"]`, `.m["k2"]`, ".n", ".p", ".s", ".s[0]", ".s[1]", ".u", ".z"}
	got := make([]string, len(ps))
	for i, p := range ps {
		got[i] = p.String()
	}
	if !slices.Equal(got, want) {
		t.Errorf("Walk visits %v, want %v", got, want)
	}
	// The order is the canonical order of paths, and each value is the one
	// its path reaches, as stored.
	if !slices.IsSortedFunc(ps, tenon.ComparePaths) {
		t.Errorf("Walk's paths are not in the canonical order")
	}
	if !vs[14].Equal(tenon.NumberFromInt(10)) || !vs[15].Equal(tenon.NumberFromInt(30)) {
		t.Errorf("a set's members by place: %v, %v", vs[14], vs[15])
	}
	// Values as they are stored: a member keeps its own marks, and does not
	// take its container's, but those a deep mark put on it.
	own, outer := stamp{id: "own"}, stamp{id: "outer"}
	deep := stamp{id: "deep", deep: true}
	marked := tenon.WithMarks(tenon.List(str, tenon.WithMarks(tenon.String("x"), own)), outer)
	_, mvs := walked(marked)
	if !tenon.Identical(mvs[0], marked) || !tenon.Identical(mvs[1], tenon.WithMarks(tenon.String("x"), own)) {
		t.Errorf("Walk hands %v and %v", mvs[0], mvs[1])
	}
	_, dvs := walked(tenon.WithMarks(tenon.List(str, tenon.String("y")), deep))
	if !tenon.HasMark(dvs[1], deep) {
		t.Errorf("a member a deep mark reached is handed without it: %v", dvs[1])
	}
	// Skipping passes over what is within; stopping ends the walk.
	var skipped []string
	tenon.Walk(doc, func(p tenon.Path, _ tenon.Value) tenon.WalkAction {
		skipped = append(skipped, p.String())
		if p.Len() == 1 {
			return tenon.WalkSkip
		}
		return tenon.WalkContinue
	})
	if want := []string{".", ".a", ".held", ".m", ".n", ".p", ".s", ".u", ".z"}; !slices.Equal(skipped, want) {
		t.Errorf("skipping visits %v, want %v", skipped, want)
	}
	n := 0
	tenon.Walk(doc, func(tenon.Path, tenon.Value) tenon.WalkAction {
		n++
		if n == 3 {
			return tenon.WalkStop
		}
		return tenon.WalkContinue
	})
	if n != 3 {
		t.Errorf("stopping at the third visit made %d", n)
	}
	// All is the walk as an iterator, and breaking out ends it.
	var all []tenon.Path
	for p := range tenon.All(doc) {
		all = append(all, p)
		if len(all) == 4 {
			break
		}
	}
	if !slices.EqualFunc(all, ps[:4], tenon.Path.Equal) {
		t.Errorf("All gives %v, want %v", all, ps[:4])
	}
	// Every path handed out stays valid: kept from a walk eight deep, each
	// still reaches the value it was handed with.
	var nest func(int) tenon.Value
	nest = func(d int) tenon.Value {
		if d == 0 {
			return tenon.NumberFromInt(0)
		}
		inner := nest(d - 1)
		return tenon.Tuple(inner, inner)
	}
	deepDoc := nest(8)
	kept, keptVals := walked(deepDoc)
	if len(kept) != 511 {
		t.Fatalf("walked %d values, want 511", len(kept))
	}
	seen := map[string]bool{}
	for i, p := range kept {
		if !tenon.Identical(p.Apply(deepDoc), keptVals[i]) {
			t.Fatalf("the path %s kept from the walk no longer reaches the value it was handed with", p)
		}
		seen[p.String()] = true
	}
	if len(seen) != 511 {
		t.Errorf("the kept paths name %d places, want 511", len(seen))
	}
}
