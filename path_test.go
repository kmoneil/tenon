package tenon_test

import (
	"slices"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
)

func TestConformance_VA020_PathSteps(t *testing.T) {
	conformance.Covers(t, "VA-020")
	var root tenon.Path
	if root.Len() != 0 || len(root.Steps()) != 0 || root.String() != "." {
		t.Errorf("the zero Path is not the empty path: %d steps, %q", root.Len(), root.String())
	}

	key := tenon.String("k")
	p := root.Attribute("name").Index(tenon.NumberFromInt(2)).Index(key)
	if p.Len() != 3 {
		t.Errorf("Len() = %d, want 3", p.Len())
	}
	steps := p.Steps()
	if kinds := []tenon.StepKind{steps[0].Kind(), steps[1].Kind(), steps[2].Kind()}; !slices.Equal(kinds, []tenon.StepKind{tenon.StepAttribute, tenon.StepIndex, tenon.StepIndex}) {
		t.Errorf("step kinds are %v", kinds)
	}
	if steps[0].Name() != "name" || steps[2].Key() != key {
		t.Errorf("steps do not carry what they were built from: %v", p)
	}
	if got, want := p.String(), `.name[2]["k"]`; got != want {
		t.Errorf("String() = %s, want %s", got, want)
	}

	// Attribute names follow the rules for object attributes, and a name that
	// is not an identifier is shown quoted.
	composed, decomposed := "caf\u00e9", "cafe\u0301"
	if got, want := root.Attribute(decomposed).String(), ".\""+composed+"\""; got != want {
		t.Errorf("String() = %s, want %s", got, want)
	}
	if got := root.Attribute("has space").String(); got != `."has space"` {
		t.Errorf("String() = %s", got)
	}
	mustPanicUsage(t, "must not be empty", func() { root.Attribute("") })
	mustPanicUsage(t, "not valid UTF-8", func() { root.Attribute("a\xff") })

	// A path indexes by a Number or String value.
	mustPanicUsage(t, "a value of type bool", func() { root.Index(tenon.Bool(true)) })
	mustPanicUsage(t, "an error value", func() { root.Index(tenon.String("\xff")) })
	mustPanicUsage(t, "a pending value", func() { root.Index(tenon.Pending(tenon.Any())) })

	// Step accessors are for the kind of step they name.
	mustPanicUsage(t, "not an attribute step", func() { steps[1].Name() })
	mustPanicUsage(t, "not an index step", func() { steps[0].Key() })
	mustPanicUsage(t, "use of the zero Step", func() { tenon.Step{}.Kind() })
}

func TestConformance_VA021_PathsStayValid(t *testing.T) {
	conformance.Covers(t, "VA-021")
	// Walk a nested structure, capturing a path at every value, and extending
	// the same prefixes again both deeper and sideways. Nothing a walk does
	// afterwards may change a path that was captured earlier.
	type captured struct {
		path tenon.Path
		want string
	}
	var seen []captured
	capture := func(p tenon.Path) {
		seen = append(seen, captured{p, p.String()})
	}

	var root tenon.Path
	capture(root)
	for _, name := range []string{"alpha", "beta", "gamma"} {
		p := root.Attribute(name)
		capture(p)
		for i := range 3 {
			q := p.Index(tenon.NumberFromInt(int64(i)))
			capture(q)
			capture(q.Attribute("leaf"))
			capture(q.Index(tenon.String("key")))
		}
		root = p // walk deeper, reusing the prefix the captured paths share
	}

	for _, c := range seen {
		if got := c.path.String(); got != c.want {
			t.Errorf("a captured path changed from %s to %s", c.want, got)
		}
	}

	// The steps a path hands out are the caller's own.
	p := tenon.Path{}.Attribute("a").Attribute("b")
	steps := p.Steps()
	steps[0] = steps[1]
	if got := p.String(); got != ".a.b" {
		t.Errorf("changing the slice from Steps changed the path to %s", got)
	}
}

func TestPathEqual(t *testing.T) {
	var root tenon.Path
	a := root.Attribute("a")
	for _, pair := range [][2]tenon.Path{
		{root, tenon.Path{}},
		{a, root.Attribute("a")},
		{a.Index(tenon.NumberFromInt(1)), root.Attribute("a").Index(tenon.NumberFromText("1.0"))},
		{a.Index(tenon.String("k")), a.Index(tenon.String("k"))},
	} {
		if !pair[0].Equal(pair[1]) || !pair[1].Equal(pair[0]) {
			t.Errorf("%s and %s are not equal paths", pair[0], pair[1])
		}
	}
	for _, pair := range [][2]tenon.Path{
		{root, a},
		{a, root.Attribute("b")},
		{a, a.Attribute("a")},
		{a.Index(tenon.NumberFromInt(1)), a.Index(tenon.NumberFromInt(2))},
		{a.Index(tenon.String("1")), a.Index(tenon.NumberFromInt(1))},
	} {
		if pair[0].Equal(pair[1]) || pair[1].Equal(pair[0]) {
			t.Errorf("%s and %s are equal paths", pair[0], pair[1])
		}
	}
}
