package ctytenon_test

import (
	"math/rand"
	"slices"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/ctytenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/zclconf/go-cty/cty"
)

// sameLocated reports whether two lists of located marks are the same: the
// same paths, in order, and the same marks at each.
func sameLocated(a, b []tenon.LocatedMarks) bool {
	return slices.EqualFunc(a, b, func(x, y tenon.LocatedMarks) bool {
		return x.Path.Equal(y.Path) && slices.Equal(x.Marks, y.Marks)
	})
}

// TestLocatedMarksCross marks random cty values at random places and takes
// their marks off with their paths, which cross to tenon as the located
// marks of the value FromCty crosses, and back to cty as the paths of the
// value ToCty crosses: placing each side's on the other side's value
// unmarked gives the marked value.
func TestLocatedMarksCross(t *testing.T) {
	r := rand.New(rand.NewSource(20261006))
	for range conformance.Iterations(t, 1000) {
		cv := markAtRandom(r, randomCtyValue(r, randomCtyType(r, 3)), nil)
		plain, cPaths := cv.UnmarkDeepWithPaths()
		tv, err := marking.FromCty(cv)
		if err != nil {
			t.Fatalf("FromCty(%#v): %v", cv, err)
		}
		tPlain, err := marking.FromCty(plain)
		if err != nil {
			t.Fatalf("FromCty(%#v): %v", plain, err)
		}
		lm, err := marking.LocatedMarksFromCty(cPaths, plain)
		if err != nil {
			t.Fatalf("LocatedMarksFromCty(%#v): %v", cPaths, err)
		}
		if !sameLocated(lm, tenon.MarkLocations(tv)) {
			t.Fatalf("%#v's marks cross as %v, where FromCty's value has %v", cv, lm, tenon.MarkLocations(tv))
		}
		if placed, left := tenon.WithLocatedMarks(tPlain, lm); !tenon.Identical(placed, tv) || left != nil {
			t.Fatalf("%#v's marks placed in tenon give %v, leaving %v", cv, placed, left)
		}
		back, err := marking.LocatedMarksToCty(tenon.MarkLocations(tv), tPlain)
		if err != nil {
			t.Fatalf("LocatedMarksToCty(%v): %v", tenon.MarkLocations(tv), err)
		}
		cPlain, err := marking.ToCty(tPlain)
		if err != nil {
			t.Fatalf("ToCty(%v): %v", tPlain, err)
		}
		want, err := marking.ToCty(tv)
		if err != nil {
			t.Fatalf("ToCty(%v): %v", tv, err)
		}
		if got := cPlain.MarkWithPaths(back); !got.RawEquals(want) {
			t.Fatalf("%v's marks placed in cty give %#v, want %#v", tv, got, want)
		}
	}
}

// TestLocatedMarksCtyDrops holds LocatedMarksFromCty to keeping the entries
// go-cty's MarkWithPaths drops silently, which placing them in tenon hands
// back.
func TestLocatedMarksCtyDrops(t *testing.T) {
	str := cty.String
	v := cty.ObjectVal(map[string]cty.Value{
		"a": cty.StringVal("x"),
		"u": cty.UnknownVal(cty.Object(map[string]cty.Type{"x": str})),
		"n": cty.NullVal(cty.List(str)),
		"l": cty.ListVal([]cty.Value{cty.StringVal("y")}),
	})
	marks := []cty.PathValueMarks{
		{Path: cty.GetAttrPath("a"), Marks: cty.NewValueMarks("m")},
		{Path: cty.GetAttrPath("nope"), Marks: cty.NewValueMarks("m")},
		{Path: cty.GetAttrPath("u").GetAttr("x"), Marks: cty.NewValueMarks("m")},
		{Path: cty.GetAttrPath("n").IndexInt(0), Marks: cty.NewValueMarks("m")},
		{Path: cty.GetAttrPath("l").IndexInt(3), Marks: cty.NewValueMarks("m")},
	}
	// go-cty marks .a and nothing else, and says nothing of the rest.
	if got, want := v.MarkWithPaths(marks), cty.ObjectVal(map[string]cty.Value{
		"a": cty.StringVal("x").Mark("m"), "u": v.GetAttr("u"), "n": v.GetAttr("n"), "l": v.GetAttr("l"),
	}); !got.RawEquals(want) {
		t.Errorf("go-cty's MarkWithPaths gives %#v", got)
	}
	lm, err := marking.LocatedMarksFromCty(marks, v)
	if err != nil {
		t.Fatal(err)
	}
	tv, _ := marking.FromCty(v)
	placed, left := tenon.WithLocatedMarks(tv, lm)
	want, _ := marking.FromCty(v.MarkWithPaths(marks))
	if !tenon.Identical(placed, want) {
		t.Errorf("placing in tenon gives %v, want %v", placed, want)
	}
	var paths []string
	for _, e := range left {
		paths = append(paths, e.Path.String())
	}
	if !slices.Equal(paths, []string{".l[3]", ".n[0]", ".nope", ".u.x"}) {
		t.Errorf("placing in tenon leaves %v", paths)
	}
	// A mark the Bridge does not map fails, located at its path.
	_, err = marking.LocatedMarksFromCty([]cty.PathValueMarks{{Path: cty.GetAttrPath("a"), Marks: cty.NewValueMarks("refused")}}, v)
	if d := asError(t, err).Diagnostics(); len(d) != 1 || d[0].Code != ctytenon.CodeUnmappedMark || !d[0].Path.Equal(pathOf("a")) {
		t.Errorf("an unmapped cty mark fails with %v", d)
	}
	_, err = marking.LocatedMarksToCty([]tenon.LocatedMarks{{Path: pathOf("a"), Marks: []tenon.Mark{label("audited")}}}, tv)
	if d := asError(t, err).Diagnostics(); len(d) != 1 || d[0].Code != ctytenon.CodeUnmappedMark || !d[0].Path.Equal(pathOf("a")) {
		t.Errorf("an unmapped tenon mark fails with %v", d)
	}
}

// TestPathSetsCross holds a cty path set to crossing as its paths sorted in
// tenon's canonical order, each once, a step into a set by the member's
// place, and back as the same set.
func TestPathSetsCross(t *testing.T) {
	v := cty.ObjectVal(map[string]cty.Value{
		"a": cty.ListVal([]cty.Value{cty.StringVal("x"), cty.StringVal("y")}),
		"s": cty.SetVal([]cty.Value{cty.StringVal("p"), cty.StringVal("q")}),
		"b": cty.StringVal("z"),
	})
	set := cty.NewPathSet(
		cty.GetAttrPath("b"),
		cty.GetAttrPath("a").IndexInt(1),
		cty.GetAttrPath("s").Index(cty.StringVal("q")),
		cty.GetAttrPath("a").IndexInt(0),
		cty.GetAttrPath("a").Index(cty.MustParseNumberVal("1.0")),
	)
	paths, err := marking.PathSetFromCty(set, v)
	if err != nil {
		t.Fatal(err)
	}
	var shown []string
	for _, p := range paths {
		shown = append(shown, p.String())
	}
	if !slices.Equal(shown, []string{".a[0]", ".a[1]", ".b", ".s[1]"}) {
		t.Errorf("the path set crosses as %v", shown)
	}
	tv, _ := marking.FromCty(v)
	back, err := marking.PathSetToCty(paths, tv)
	if err != nil || !back.Equal(set) {
		t.Errorf("the paths cross back as %#v, %v", back.List(), err)
	}
}
