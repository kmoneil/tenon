package ctytenon_test

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"slices"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/ctytenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

// classified is the tenon mark Terraform's sensitive mark crosses as in the
// walkthrough: redacting, as sensitivity is; deep, since everything within a
// sensitive value is sensitive, which cty says by handing a container's marks
// to what is read out of it; and encodable by its identifier, so that tenon's
// own encoding carries it.
type classified struct{}

func (classified) MarkID() string                            { return "sensitive" }
func (classified) Propagation() tenon.Propagation            { return tenon.Propagate }
func (classified) Redacting() bool                           { return true }
func (classified) Deep() bool                                { return true }
func (classified) MarkPayload() (tenon.Value, bool)          { return tenon.Value{}, false }
func decodeClassified(tenon.Value, bool) (tenon.Mark, error) { return classified{}, nil }

// walkthrough is the Bridge of the walkthrough: Terraform's sensitive mark,
// and no other, crosses as classified and back.
var walkthrough = ctytenon.Bridge{
	MarkFromCty: func(m any) (tenon.Mark, bool) { return classified{}, m == "sensitive" },
	MarkToCty:   func(m tenon.Mark) (any, bool) { return "sensitive", m == classified{} },
}

// unknownAsNull is go-cty's UnknownAsNull as a recipe over tenon's
// Transform: each value not known yet becomes the null of its type, keeping
// its marks, and each pending value holding no members the pending value
// known to be null. It breaks what go-cty's breaks: a value known not to be
// null is null, and the members of a set that differed only by what was not
// known are one member. A known list, set or map whose element type is not
// known yet, which crosses from cty as a pending value, becomes null too,
// where go-cty's keeps the known collection.
func unknownAsNull(v tenon.Value) tenon.Value {
	return tenon.Transform(v, func(_ tenon.Path, v tenon.Value) tenon.Value {
		u, marks := tenon.Unmark(v)
		switch {
		case u.IsPending() && !u.HasMembers():
			return tenon.WithMarks(tenon.Narrow(tenon.Pending(u.Constraint()), tenon.NullOnly()), marks...)
		case u.IsResolved() && !u.HasContent() && !u.IsNull():
			return tenon.WithMarks(tenon.Null(u.Type()), marks...)
		}
		return v
	})
}

// TestUnknownAsNullRecipe holds the recipe to go-cty's UnknownAsNull on
// random marked values whose types are known.
func TestUnknownAsNullRecipe(t *testing.T) {
	r := rand.New(rand.NewSource(20261006))
	n := 0
	for n < conformance.Iterations(t, 1000) {
		ty := randomCtyType(r, 3)
		if ty.HasDynamicTypes() {
			continue
		}
		n++
		v := markAtRandom(r, randomCtyValue(r, ty), nil)
		want, err := marking.FromCty(cty.UnknownAsNull(v))
		if err != nil {
			t.Fatalf("FromCty(UnknownAsNull(%#v)): %v", v, err)
		}
		tv, err := marking.FromCty(v)
		if err != nil {
			t.Fatalf("FromCty(%#v): %v", v, err)
		}
		if got := unknownAsNull(tv); !tenon.Identical(got, want) {
			t.Fatalf("the recipe gives %v of %v, where go-cty gives %v", got, tv, want)
		}
	}
}

// stateStep is a step of a path as Terraform's state, version 4, writes it
// in sensitive_attributes.
type stateStep struct {
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}

// ctyStatePaths writes paths as Terraform's state does (marshalPaths in
// internal/states/statefile/version4.go): an attribute step its name, an
// index step its key as cty's JSON of a value of no type known in advance.
func ctyStatePaths(t *testing.T, paths []cty.Path) []byte {
	t.Helper()
	out := [][]stateStep{}
	for _, p := range paths {
		steps := []stateStep{}
		for _, s := range p {
			switch s := s.(type) {
			case cty.GetAttrStep:
				name, _ := json.Marshal(s.Name)
				steps = append(steps, stateStep{Type: "get_attr", Value: name})
			case cty.IndexStep:
				key, err := ctyjson.Marshal(s.Key, cty.DynamicPseudoType)
				if err != nil {
					t.Fatal(err)
				}
				steps = append(steps, stateStep{Type: "index", Value: key})
			}
		}
		out = append(out, steps)
	}
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// ctyReadStatePaths reads what ctyStatePaths writes, as Terraform's
// unmarshalPaths does.
func ctyReadStatePaths(t *testing.T, data []byte) []cty.Path {
	t.Helper()
	var in [][]stateStep
	if err := json.Unmarshal(data, &in); err != nil {
		t.Fatal(err)
	}
	var paths []cty.Path
	for _, steps := range in {
		var p cty.Path
		for _, s := range steps {
			if s.Type == "get_attr" {
				var name string
				if err := json.Unmarshal(s.Value, &name); err != nil {
					t.Fatal(err)
				}
				p = p.GetAttr(name)
				continue
			}
			key, err := ctyjson.Unmarshal(s.Value, cty.DynamicPseudoType)
			if err != nil {
				t.Fatal(err)
			}
			p = p.Index(key)
		}
		paths = append(paths, p)
	}
	return paths
}

// tenonStatePaths writes tenon paths in the shape of Terraform's state, with
// tenon's own JSON for a key.
func tenonStatePaths(t *testing.T, paths []tenon.Path) []byte {
	t.Helper()
	out := [][]stateStep{}
	for _, p := range paths {
		steps := []stateStep{}
		for _, s := range p.Steps() {
			if s.Kind() == tenon.StepAttribute {
				name, _ := json.Marshal(s.Name())
				steps = append(steps, stateStep{Type: "get_attr", Value: name})
				continue
			}
			key, err := tenon.ProjectJSON(s.Key())
			if err != nil {
				t.Fatal(err)
			}
			typ := "number"
			if s.Key().Type() == tenon.StringType() {
				typ = "string"
			}
			steps = append(steps, stateStep{Type: "index", Value: json.RawMessage(`{"value":` + string(key) + `,"type":"` + typ + `"}`)})
		}
		out = append(out, steps)
	}
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// tenonReadStatePaths reads what tenonStatePaths writes, keys by tenon's
// JSON reading, exact.
func tenonReadStatePaths(t *testing.T, data []byte) []tenon.Path {
	t.Helper()
	var in [][]stateStep
	if err := json.Unmarshal(data, &in); err != nil {
		t.Fatal(err)
	}
	var paths []tenon.Path
	for _, steps := range in {
		var p tenon.Path
		for _, s := range steps {
			if s.Type == "get_attr" {
				var name string
				if err := json.Unmarshal(s.Value, &name); err != nil {
					t.Fatal(err)
				}
				p = p.Attribute(name)
				continue
			}
			key, err := tenon.ParseJSON(s.Value, tenon.Any(), tenon.Safe)
			if err != nil {
				t.Fatal(err)
			}
			p = p.Index(key.Attribute("value"))
		}
		paths = append(paths, p)
	}
	return paths
}

// TestSensitiveValuesWalkthrough follows a resource's sensitive values the
// way Terraform keeps them in its state, done with go-cty as Terraform does
// it and with tenon beside it, each step holding both to the same values,
// marks and text:
//
//  1. the marks are taken off with their places;
//  2. what is not known yet becomes null, since the state holds no unknown;
//  3. the value is written as JSON and the places as sensitive_attributes;
//  4. both are read back and the marks put back where they were;
//  5. the next plan's changes are shown, withholding what is sensitive;
//  6. and tenon's own encoding carries the marks with no stripping at all.
func TestSensitiveValuesWalkthrough(t *testing.T) {
	b := walkthrough
	rule := func(port int64, token string) cty.Value {
		return cty.ObjectVal(map[string]cty.Value{"port": cty.NumberIntVal(port), "token": cty.StringVal(token).Mark("sensitive")})
	}
	resource := func(password, token string, tags map[string]cty.Value) cty.Value {
		return cty.ObjectVal(map[string]cty.Value{
			"id":       cty.UnknownVal(cty.String),
			"name":     cty.StringVal("web"),
			"password": cty.StringVal(password).Mark("sensitive"),
			"credentials": cty.ObjectVal(map[string]cty.Value{
				"user": cty.StringVal("admin"), "key": cty.StringVal("k3y-0f-th3-r3alm"),
			}).Mark("sensitive"),
			"tags":  cty.MapVal(tags),
			"rules": cty.ListVal([]cty.Value{rule(443, token), rule(80, "t0k3n-80")}),
			"ports": cty.SetVal([]cty.Value{cty.NumberIntVal(443), cty.NumberIntVal(80)}).Mark("sensitive"),
		})
	}
	planned := resource("hunter2", "t0k3n-443", map[string]cty.Value{
		"env": cty.StringVal("prod"), "db": cty.StringVal("pg://u:pw@db").Mark("sensitive"),
	})
	ty := planned.Type()
	tPlanned, err := b.FromCty(planned)
	if err != nil {
		t.Fatal(err)
	}

	// 1. The marks taken off with their places: go-cty's
	// UnmarkDeepWithPaths, the paths sorted by their text as Terraform sorts
	// them; tenon's MarkLocations, compacted, the mark being deep, and
	// UnmarkDeep. The places are the same.
	cPlain, cMarks := planned.UnmarkDeepWithPaths()
	var cPaths []cty.Path
	for _, e := range cMarks {
		cPaths = append(cPaths, e.Path)
	}
	slices.SortFunc(cPaths, func(a, b cty.Path) int { return strings.Compare(pathText(a), pathText(b)) })
	tLocated := tenon.CompactLocatedMarks(tenon.MarkLocations(tPlanned))
	tPlain, _ := tenon.UnmarkDeep(tPlanned)
	var tPaths []tenon.Path
	for _, e := range tLocated {
		tPaths = append(tPaths, e.Path)
	}
	crossed, err := b.PathSetToCty(tPaths, tPlain)
	if err != nil || !crossed.Equal(cty.NewPathSet(cPaths...)) {
		t.Fatalf("tenon's places %v cross as %#v, where go-cty's are %#v (%v)", tPaths, crossed.List(), cPaths, err)
	}

	// 2. Unknowns as nulls: go-cty's UnknownAsNull and tenon's recipe.
	cState := cty.UnknownAsNull(cPlain)
	tState := unknownAsNull(tPlain)
	if again, err := b.FromCty(cState); err != nil || !tenon.Identical(again, tState) {
		t.Fatalf("the state's value is %v in tenon, where go-cty's crosses as %v (%v)", tState, again, err)
	}

	// 3. The state written: the same JSON, and the same
	// sensitive_attributes. tenon writes its places in the canonical order
	// of paths, which is Terraform's order of their text here.
	cJSON, err := ctyjson.Marshal(cState, ty)
	if err != nil {
		t.Fatal(err)
	}
	tJSON, err := tenon.ProjectJSON(tState)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(cJSON, tJSON) {
		t.Fatalf("tenon writes the value as\n%s\nwhere go-cty writes\n%s", tJSON, cJSON)
	}
	cAttrs, tAttrs := ctyStatePaths(t, cPaths), tenonStatePaths(t, tPaths)
	if !bytes.Equal(cAttrs, tAttrs) {
		t.Fatalf("tenon writes sensitive_attributes as\n%s\nwhere go-cty writes\n%s", tAttrs, cAttrs)
	}
	for _, secret := range []string{"hunter2", "k3y-0f-th3-r3alm", "t0k3n-443", "pg://u:pw@db"} {
		if !bytes.Contains(tJSON, []byte(secret)) || bytes.Contains(tAttrs, []byte(secret)) {
			t.Errorf("the state holds %q in its value, or names it in its places", secret)
		}
	}

	// 4. The state read back: the value of its type, and the marks put
	// back. Both give the planned value with its unknowns null, marked as
	// it was.
	cRead, err := ctyjson.Unmarshal(cJSON, ty)
	if err != nil {
		t.Fatal(err)
	}
	var cBackMarks []cty.PathValueMarks
	for _, p := range ctyReadStatePaths(t, cAttrs) {
		cBackMarks = append(cBackMarks, cty.PathValueMarks{Path: p, Marks: cty.NewValueMarks("sensitive")})
	}
	cBack := cRead.MarkWithPaths(cBackMarks)
	tType, err := b.TypeFromCty(ty)
	if err != nil {
		t.Fatal(err)
	}
	// A set is written as an array, whose elements reading it as a set
	// would merge were any two equal: tenon's policy says the reading may.
	tRead, err := tenon.ParseJSON(tJSON, tenon.Exactly(tType), tenon.Unsafe)
	if err != nil {
		t.Fatal(err)
	}
	var tBackMarks []tenon.LocatedMarks
	for _, p := range tenonReadStatePaths(t, tAttrs) {
		tBackMarks = append(tBackMarks, tenon.LocatedMarks{Path: p, Marks: []tenon.Mark{classified{}}})
	}
	tBack, left := tenon.WithLocatedMarks(tRead, tBackMarks)
	if left != nil {
		t.Fatalf("reading the state back leaves %v unplaced", left)
	}
	if want := unknownAsNull(tPlanned); !tenon.Identical(tBack, want) {
		t.Fatalf("tenon reads the state back as %v, want %v", tBack, want)
	}
	if again, err := b.FromCty(cBack); err != nil || !tenon.Identical(again, tBack) {
		t.Fatalf("go-cty reads the state back as %v in tenon, where tenon reads %v (%v)", again, tBack, err)
	}

	// 5. The next plan rotates the password and a rule's token and adds a
	// tag: the changes say where, and show nothing sensitive, old or new.
	next, err := b.FromCty(cty.UnknownAsNull(resource("correct-horse", "t0k3n-r0tated", map[string]cty.Value{
		"env": cty.StringVal("prod"), "db": cty.StringVal("pg://u:pw@db").Mark("sensitive"), "owner": cty.StringVal("ops"),
	})))
	if err != nil {
		t.Fatal(err)
	}
	var shown []string
	for _, c := range tenon.Diff(tBack, next) {
		shown = append(shown, c.String())
	}
	want := []string{
		`~ .password: redacted("sensitive") -> redacted("sensitive")`,
		`~ .rules[0].token: redacted("sensitive") -> redacted("sensitive")`,
		`+ .tags["owner"]: "ops"`,
	}
	if !slices.Equal(shown, want) {
		t.Errorf("the plan's changes are\n%s\nwant\n%s", strings.Join(shown, "\n"), strings.Join(want, "\n"))
	}

	// 6. tenon's own encoding carries the marks, and the unknowns, with no
	// stripping, no side channel and nothing nulled.
	data, err := tenon.Serialize(tPlanned)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := tenon.Deserialize(data, tenon.Decoders{Marks: map[string]tenon.MarkDecoder{"sensitive": decodeClassified}})
	if err != nil || !tenon.Identical(decoded, tPlanned) {
		t.Errorf("tenon's encoding gives back %v, want %v (%v)", decoded, tPlanned, err)
	}
}

// pathText is a cty path's text as Terraform's format.CtyPath writes it,
// which Terraform sorts sensitive paths by.
func pathText(p cty.Path) string {
	var b strings.Builder
	for _, s := range p {
		switch s := s.(type) {
		case cty.GetAttrStep:
			b.WriteString("." + s.Name)
		case cty.IndexStep:
			key, _ := ctyjson.Marshal(s.Key, s.Key.Type())
			b.WriteString("[" + string(key) + "]")
		}
	}
	return b.String()
}
