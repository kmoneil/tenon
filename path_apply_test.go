package tenon_test

import (
	"errors"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
)

// at returns the path of the steps given: a string an attribute, an int an
// index by number, and a Value an index by that key.
func at(steps ...any) tenon.Path {
	var p tenon.Path
	for _, s := range steps {
		switch s := s.(type) {
		case string:
			p = p.Attribute(s)
		case int:
			p = p.Index(tenon.NumberFromInt(int64(s)))
		case tenon.Value:
			p = p.Index(s)
		}
	}
	return p
}

// appliesAs checks that applying p to v answers want.
func appliesAs(t *testing.T, p tenon.Path, v, want tenon.Value) {
	t.Helper()
	if got := p.Apply(v); !tenon.Identical(got, want) {
		t.Errorf("%s applied to %v = %v, want %v", p, v, got, want)
	}
}

// applyFails checks that applying p to v fails with code, located at loc.
func applyFails(t *testing.T, p tenon.Path, v tenon.Value, code tenon.Code, loc tenon.Path) {
	t.Helper()
	got := p.Apply(v)
	if !got.IsError() || got.Diagnostics()[0].Code != code || !got.Diagnostics()[0].Path.Equal(loc) {
		t.Errorf("%s applied to %v = %v, want %s at %s", p, v, got, code, loc)
	}
}

func TestConformance_VA020_SetMembersByPlace(t *testing.T) {
	conformance.Covers(t, "VA-020")
	num := tenon.NumberType()
	set := tenon.Set(num, tenon.NumberFromInt(30), tenon.NumberFromInt(10), tenon.NumberFromInt(20))
	// A set's member by its place in the iteration order: the canonical one.
	for i, want := range []int64{10, 20, 30} {
		appliesAs(t, at(i), set, tenon.NumberFromInt(want))
	}
	// Members not known yet come after the known ones.
	partial := tenon.Set(num, tenon.Unknown(num), tenon.NumberFromInt(5))
	appliesAs(t, at(0), partial, tenon.NumberFromInt(5))
	appliesAs(t, at(1), partial, tenon.Unknown(num))
	applyFails(t, at(2), partial, tenon.CodePathNoMember, at(2))
}

func TestConformance_VA022_Apply(t *testing.T) {
	conformance.Covers(t, "VA-022")
	str, num := tenon.StringType(), tenon.NumberType()
	doc := tenon.Object(map[string]tenon.Value{
		"name":  tenon.String("web"),
		"ports": tenon.List(num, tenon.NumberFromInt(80), tenon.NumberFromInt(443)),
		"env":   tenon.Map(str, map[string]tenon.Value{"LOG": tenon.String("debug")}),
		"pair":  tenon.Tuple(tenon.String("a"), tenon.NumberFromInt(1)),
		"tags":  tenon.Set(str, tenon.String("b"), tenon.String("a")),
	})
	appliesAs(t, tenon.Path{}, doc, doc)
	appliesAs(t, at("name"), doc, tenon.String("web"))
	appliesAs(t, at("ports", 1), doc, tenon.NumberFromInt(443))
	appliesAs(t, at("env", tenon.String("LOG")), doc, tenon.String("debug"))
	appliesAs(t, at("pair", 1), doc, tenon.NumberFromInt(1))
	appliesAs(t, at("tags", 0), doc, tenon.String("a"))
	// A pending value holding members reads as its tuple or object.
	untyped := tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly())
	held := tenon.Object(map[string]tenon.Value{"a": tenon.Tuple(untyped, tenon.NumberFromInt(7))})
	appliesAs(t, at("a", 1), held, tenon.NumberFromInt(7))
	appliesAs(t, at("a", 0), held, untyped)
	// The answer carries its own marks, and the Propagate and redacting
	// marks of what it was read out of, not their Isolate ones.
	prop, iso, red := stamp{id: "prop"}, stamp{id: "iso", policy: tenon.Isolate}, stamp{id: "red", redact: true}
	own := stamp{id: "own", policy: tenon.Isolate}
	marked := tenon.WithMarks(tenon.Object(map[string]tenon.Value{
		"inner": tenon.WithMarks(tenon.List(str, tenon.WithMarks(tenon.String("x"), own)), red),
	}), prop, iso)
	got := at("inner", 0).Apply(marked)
	if want := tenon.WithMarks(tenon.String("x"), own, prop, red); !tenon.Identical(got, want) {
		t.Errorf("the marks of a member read out = %v, want %v", got, want)
	}
}

func TestConformance_VA023_ApplyFailures(t *testing.T) {
	conformance.Covers(t, "VA-023")
	str, num := tenon.StringType(), tenon.NumberType()
	doc := tenon.Object(map[string]tenon.Value{
		"l": tenon.List(num, tenon.NumberFromInt(1)),
		"m": tenon.Map(str, map[string]tenon.Value{"k": tenon.String("v")}),
		"n": tenon.Null(tenon.ObjectType(map[string]tenon.Type{"x": str})),
		"s": tenon.String("text"),
	})
	for _, tt := range []struct {
		p    tenon.Path
		code tenon.Code
	}{
		{at("missing"), tenon.CodePathNoMember},
		{at("l", 1), tenon.CodePathNoMember},
		{at("l", -1), tenon.CodePathNoMember},
		{at("l", tenon.NumberFromText("0.5")), tenon.CodePathNoMember},
		{at("l", tenon.NumberFromText("1e30")), tenon.CodePathNoMember},
		{at("m", tenon.String("other")), tenon.CodePathNoMember},
		{at("l", "x"), tenon.CodeOperationWrongType},
		{at("l", tenon.String("0")), tenon.CodeOperationWrongType},
		{at("m", 0), tenon.CodeOperationWrongType},
		{at("m", "k"), tenon.CodeOperationWrongType},
		{at(tenon.String("l")), tenon.CodeOperationWrongType},
		{at("s", 0), tenon.CodeOperationWrongType},
		{at("n", "x"), tenon.CodeOperationNullOperand},
	} {
		applyFails(t, tt.p, doc, tt.code, tt.p)
	}
	// Located at the step that fails, not past it.
	applyFails(t, at("missing", "deeper", 0), doc, tenon.CodePathNoMember, at("missing"))
	// An error value met on the way is the answer.
	bad := tenon.ErrorVal(tenon.Diagnostic{Code: tenon.CodeFunctionInvalidArgument, Message: "bad"})
	if got := at("a").Apply(bad); !got.IsError() || got.Diagnostics()[0].Code != tenon.CodeFunctionInvalidArgument {
		t.Errorf("a path applied to an error value = %v, want the error value", got)
	}
	// Within a redacted value, the failure is located at it and says nothing
	// of what it holds.
	red := stamp{id: "red", redact: true}
	secret := tenon.Object(map[string]tenon.Value{
		"cred": tenon.WithMarks(tenon.Object(map[string]tenon.Value{"user": tenon.String("u")}), red),
	})
	got := at("cred", "password").Apply(secret)
	if !got.IsError() || got.Diagnostics()[0].Code != tenon.CodePathNoMember || !got.Diagnostics()[0].Path.Equal(at("cred")) {
		t.Errorf("a missing attribute within a redacted value = %v, want %s at .cred", got, tenon.CodePathNoMember)
	} else if msg := got.Diagnostics()[0].Message; strings.Contains(msg, "password") || strings.Contains(msg, "user") {
		t.Errorf("the failure within a redacted value says %q", msg)
	}
	if !tenon.HasMark(got, red) {
		t.Errorf("the failure within a redacted value does not carry its mark: %v", got)
	}
}

func TestConformance_VA024_ApplyNotKnown(t *testing.T) {
	conformance.Covers(t, "VA-024")
	str, num := tenon.StringType(), tenon.NumberType()
	objType := tenon.ObjectType(map[string]tenon.Type{"a": str, "l": tenon.ListType(num)})
	u := tenon.Unknown(objType)
	appliesAs(t, at("a"), u, tenon.Unknown(str))
	appliesAs(t, at("l", 3), u, tenon.Unknown(num))
	applyFails(t, at("nope"), u, tenon.CodePathNoMember, at("nope"))
	applyFails(t, at("l", "x"), u, tenon.CodeOperationWrongType, at("l", "x"))
	// The lengths a list's range records decide now.
	short := tenon.Narrow(tenon.Unknown(tenon.ListType(str)), tenon.LengthMax(2))
	appliesAs(t, at(1), short, tenon.Unknown(str))
	applyFails(t, at(2), short, tenon.CodePathNoMember, at(2))
	applyFails(t, at(-1), short, tenon.CodePathNoMember, at(-1))
	tup := tenon.Unknown(tenon.TupleType(str, num))
	appliesAs(t, at(1), tup, tenon.Unknown(num))
	applyFails(t, at(2), tup, tenon.CodePathNoMember, at(2))
	appliesAs(t, at(tenon.String("k")), tenon.Unknown(tenon.MapType(num)), tenon.Unknown(num))
	// Marks of a container not known yet reach the answer, a list's and a
	// map's as an object's.
	prop := stamp{id: "prop"}
	appliesAs(t, at(0), tenon.WithMarks(tenon.Unknown(tenon.ListType(str)), prop), tenon.WithMarks(tenon.Unknown(str), prop))
	appliesAs(t, at(tenon.String("k")), tenon.WithMarks(tenon.Unknown(tenon.MapType(str)), prop), tenon.WithMarks(tenon.Unknown(str), prop))
	// A pending value answers from its constraint.
	appliesAs(t, at("x", 0), tenon.Pending(tenon.Any()), tenon.Pending(tenon.Any()))
	appliesAs(t, at(4), tenon.Pending(tenon.ListOf(tenon.Exactly(str))), tenon.Unknown(str))
	appliesAs(t, at("a"), tenon.Pending(tenon.ObjectWith(map[string]tenon.Field{"a": tenon.Required(tenon.MapOf(tenon.Any()))}, true)), tenon.Pending(tenon.MapOf(tenon.Any())))
	closed := tenon.Pending(tenon.ObjectWith(map[string]tenon.Field{"a": tenon.Required(tenon.Exactly(str))}, true))
	applyFails(t, at("b"), closed, tenon.CodePathNoMember, at("b"))
	applyFails(t, at(0), tenon.Pending(tenon.MapOf(tenon.Any())), tenon.CodeOperationWrongType, at(0))
	applyFails(t, at(tenon.NumberFromText("0.5")), tenon.Pending(tenon.Any()), tenon.CodePathNoMember, at(tenon.NumberFromText("0.5")))
	appliesAs(t, at(1), tenon.Pending(tenon.OneOf(tenon.ListOf(tenon.Exactly(num)), tenon.Exactly(str))), tenon.Unknown(num))
}

func TestConformance_VA025_Lookup(t *testing.T) {
	conformance.Covers(t, "VA-025")
	str := tenon.StringType()
	doc := tenon.Object(map[string]tenon.Value{
		"a": tenon.String("x"),
		"n": tenon.Null(tenon.ObjectType(map[string]tenon.Type{"b": str})),
	})
	if v, ok := at("a").Lookup(doc); !ok || !v.Equal(tenon.String("x")) {
		t.Errorf("Lookup(.a) = %v, %v", v, ok)
	}
	for _, p := range []tenon.Path{at("missing"), at("n", "b"), at("a", 0)} {
		if v, ok := p.Lookup(doc); ok || !v.IsZero() {
			t.Errorf("Lookup(%s) = %v, %v, want nothing", p, v, ok)
		}
	}
	bad := tenon.ErrorVal(tenon.Diagnostic{Code: tenon.CodeFunctionInvalidArgument, Message: "bad"})
	if _, ok := (tenon.Path{}).Lookup(bad); ok {
		t.Errorf("Lookup in an error value reaches a value")
	}
}

func TestConformance_VA026_PathOrder(t *testing.T) {
	conformance.Covers(t, "VA-026")
	want := []tenon.Path{
		{},
		at("a"),
		at("a", "b"),
		at("a", 2),
		at("a", 10),
		at("a", tenon.String("10")),
		at("a", tenon.String("9")),
		at("b"),
		at("\U000000E9"),
		at(0),
		at(tenon.NumberFromText("0.5")),
		at(tenon.String("")),
	}
	rng := rand.New(rand.NewPCG(20261006, 1))
	for range 50 {
		got := slices.Clone(want)
		rng.Shuffle(len(got), func(i, j int) { got[i], got[j] = got[j], got[i] })
		slices.SortFunc(got, tenon.ComparePaths)
		if !slices.EqualFunc(got, want, tenon.Path.Equal) {
			t.Fatalf("sorted paths are %v, want %v", got, want)
		}
	}
	if tenon.ComparePaths(at("a", 1), at("a", 1)) != 0 {
		t.Errorf("a path does not compare equal to itself")
	}
	// Prefixes.
	p := at("a", 2, "b")
	for _, q := range []tenon.Path{{}, at("a"), at("a", 2), p} {
		if !p.HasPrefix(q) {
			t.Errorf("%s.HasPrefix(%s) = false", p, q)
		}
	}
	for _, q := range []tenon.Path{at("b"), at("a", 3), at("a", 2, "b", "c")} {
		if p.HasPrefix(q) {
			t.Errorf("%s.HasPrefix(%s) = true", p, q)
		}
	}
	if !p.Parent().Equal(at("a", 2)) || !(tenon.Path{}).Parent().Equal(tenon.Path{}) {
		t.Errorf("Parent of %s = %s", p, p.Parent())
	}
	if s, ok := p.Last(); !ok || s.Name() != "b" {
		t.Errorf("Last of %s = %v, %v", p, s, ok)
	}
	if _, ok := (tenon.Path{}).Last(); ok {
		t.Errorf("the empty path has a last step")
	}
}

func TestConformance_DI038_PathReadsBack(t *testing.T) {
	conformance.Covers(t, "DI-038")
	for _, p := range []tenon.Path{
		{},
		at("name"),
		at(0, "x"),
		at("a b", tenon.String("k\"\\\t\n\r"), tenon.NumberFromText("-1.5e-7"), tenon.String("\U00002028\U0000200B")),
		at("_x9", 10, tenon.String(""), "\U000000E9"),
	} {
		got, err := tenon.ParsePath(p.String())
		if err != nil || !got.Equal(p) {
			t.Errorf("ParsePath(%q) = %v, %v, want %v", p.String(), got, err, p)
		}
	}
	// Random paths read back.
	rng := rand.New(rand.NewPCG(20261006, 2))
	names := []string{"a", "B_1", "x y", "\"", "\\", "\U000000E9", "1a", "\U0001F600"}
	for range 2000 {
		var p tenon.Path
		for range rng.IntN(5) {
			switch rng.IntN(3) {
			case 0:
				p = p.Attribute(names[rng.IntN(len(names))])
			case 1:
				p = p.Index(tenon.NumberFromInt(int64(rng.IntN(1000) - 500)))
			default:
				p = p.Index(tenon.String(names[rng.IntN(len(names))]))
			}
		}
		if got, err := tenon.ParsePath(p.String()); err != nil || !got.Equal(p) {
			t.Fatalf("ParsePath(%q) = %v, %v", p.String(), got, err)
		}
	}
	for _, text := range []string{"", "a", ".a.", "..a", ".[", ".[1", ".[x]", `.["a]`, `."a`, `.["\q"]`, `.["\u{D800}"]`, `.["\u{12}"]`, ".[1]]", ".a b", `."" `, `.""`} {
		_, err := tenon.ParsePath(text)
		var e *tenon.Error
		if err == nil || !errors.As(err, &e) || e.Diagnostics()[0].Code != tenon.CodePathInvalidSyntax {
			t.Errorf("ParsePath(%q) = %v, want %s", text, err, tenon.CodePathInvalidSyntax)
		}
	}
}
