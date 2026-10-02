package ctytenon_test

import (
	"errors"
	"math/rand"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/ctytenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
)

// config is a value paths are taken within: an object holding a list of
// objects holding a set, and a map.
var config = cty.ObjectVal(map[string]cty.Value{
	"servers": cty.ListVal([]cty.Value{cty.ObjectVal(map[string]cty.Value{
		"name": cty.StringVal("web"),
		"tags": cty.SetVal([]cty.Value{cty.StringVal("x"), cty.StringVal("y")}),
	})}),
	"env": cty.MapVal(map[string]cty.Value{"region": cty.StringVal("us")}),
})

// TestPaths holds paths of each kind of step to crossing as themselves, both
// ways, a step into a set as the member cty takes and the place tenon does.
func TestPaths(t *testing.T) {
	var b ctytenon.Bridge
	tconfig, err := b.FromCty(config)
	if err != nil {
		t.Fatal(err)
	}
	// "y" is the second member of the set's crossing.
	for _, c := range []struct {
		name  string
		cty   cty.Path
		tenon tenon.Path
	}{
		{"the empty path", nil, tenon.Path{}},
		{"attributes and an index", cty.GetAttrPath("servers").IndexInt(0).GetAttr("name"), pathOf("servers", 0, "name")},
		{"a map key", cty.GetAttrPath("env").IndexString("region"), pathOf("env", tenon.String("region"))},
		{"a step into a set", cty.GetAttrPath("servers").IndexInt(0).GetAttr("tags").Index(cty.StringVal("y")), pathOf("servers", 0, "tags", 1)},
		{"past what the value holds", cty.GetAttrPath("absent").IndexInt(3), pathOf("absent", 3)},
	} {
		got, err := b.PathFromCty(c.cty, config)
		if err != nil || !got.Equal(c.tenon) {
			t.Errorf("%s: PathFromCty(%#v) = %v, %v; want %v", c.name, c.cty, got, err, c.tenon)
		}
		back, err := b.PathToCty(c.tenon, tconfig)
		if err != nil || !back.Equals(c.cty) {
			t.Errorf("%s: PathToCty(%v) = %#v, %v; want %#v", c.name, c.tenon, back, err, c.cty)
		}
	}
}

// TestPathsThatDoNotCross holds each step with no form on the other side to
// failing with an error that names it.
func TestPathsThatDoNotCross(t *testing.T) {
	var b ctytenon.Bridge
	for _, c := range []struct {
		name string
		cty  cty.Path
		want string
	}{
		{"an empty attribute name", cty.GetAttrPath(""), "ctytenon: step 0 of cty.Path{cty.GetAttrStep{Name:\"\"}}: object.empty_name"},
		{"an unknown key", cty.GetAttrPath("servers").Index(cty.UnknownVal(cty.Number)), "step 1 of"},
		{"a bool key", cty.GetAttrPath("env").Index(cty.True), "is not a known string or number"},
		{"a member the set does not hold", cty.GetAttrPath("servers").IndexInt(0).GetAttr("tags").Index(cty.StringVal("z")), `the set does not hold cty.StringVal("z")`},
	} {
		got, err := b.PathFromCty(c.cty, config)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: PathFromCty(%#v) = %v, %v; want an error containing %q", c.name, c.cty, got, err, c.want)
		}
	}
	tconfig, _ := b.FromCty(config)
	got, err := b.PathToCty(pathOf("servers", 0, "tags", 2), tconfig)
	if want := "ctytenon: step 3 of .servers[0].tags[2]: the set holds no member at 2"; err == nil || err.Error() != want {
		t.Errorf("PathToCty past a set's last member = %#v, %v; want %q", got, err, want)
	}
}

// TestPathsRoundTrip carries random paths within random cty values, into
// sets among them, to tenon and back, which gives each as it was.
func TestPathsRoundTrip(t *testing.T) {
	var b ctytenon.Bridge
	r := rand.New(rand.NewSource(20261011))
	intoSets := 0
	for range conformance.Iterations(t, 2000) {
		v := randomCtyValue(r, randomCtyType(r, 3))
		var paths []cty.Path
		cty.Walk(v, func(p cty.Path, _ cty.Value) (bool, error) {
			paths = append(paths, p.Copy())
			return true, nil
		})
		p := paths[r.Intn(len(paths))]
		tv, err := b.FromCty(v)
		if err != nil {
			t.Fatalf("FromCty(%#v): %v", v, err)
		}
		tp, err := b.PathFromCty(p, v)
		if err != nil {
			t.Fatalf("PathFromCty(%#v) within %#v: %v", p, v, err)
		}
		back, err := b.PathToCty(tp, tv)
		if err != nil {
			t.Fatalf("%#v within %#v crossed as %v and failed back: %v", p, v, tp, err)
		}
		// A set member is its own key, and tenon can know more of it than
		// cty said (TestValuesRoundTripFromCty), but it is the same member.
		if again, err := b.PathFromCty(back, v); !back.Equals(p) && (err != nil || !again.Equal(tp)) {
			t.Fatalf("%#v within %#v crossed as %v and back as %#v", p, v, tp, back)
		}
		if strings.Contains(fmtPathKinds(v, p), "set") {
			intoSets++
		}
	}
	if intoSets < 50 {
		t.Errorf("took %d paths into sets, want many", intoSets)
	}
}

// fmtPathKinds returns the kinds of the values p steps out of within v.
func fmtPathKinds(v cty.Value, p cty.Path) string {
	var kinds []string
	for i := range p {
		at, err := p[:i].Apply(v)
		if err != nil {
			break
		}
		if at.Type().IsSetType() {
			kinds = append(kinds, "set")
		}
	}
	return strings.Join(kinds, " ")
}

// TestErrorsFromCty holds errors cty gives, its conversion's among them, to
// crossing as diagnostics located by their paths, and a path through a value
// whose marks hide what it holds to stopping there.
func TestErrorsFromCty(t *testing.T) {
	var b ctytenon.Bridge
	_, convErr := convert.Convert(config, cty.Object(map[string]cty.Type{
		"servers": cty.List(cty.Object(map[string]cty.Type{"name": cty.Number, "tags": cty.Set(cty.String)})),
		"env":     cty.Map(cty.String),
	}))
	pathErr := cty.GetAttrPath("env").IndexString("region").NewErrorf("not a valid region")
	login := cty.ObjectVal(map[string]cty.Value{"password": cty.ObjectVal(map[string]cty.Value{"hash": cty.StringVal("x")}).Mark("sensitive")})
	secretErr := cty.GetAttrPath("password").GetAttr("hash").NewErrorf("the hash is x")
	for _, c := range []struct {
		name   string
		bridge ctytenon.Bridge
		err    error
		v      cty.Value
		want   []tenon.Diagnostic
	}{
		{"cty's conversion", b, convErr, config, []tenon.Diagnostic{{Code: ctytenon.CodeCtyError, Path: pathOf("servers", 0, "name"), Message: "a number is required"}}},
		{"a path error", b, pathErr, config, []tenon.Diagnostic{{Code: ctytenon.CodeCtyError, Path: pathOf("env", tenon.String("region")), Message: "not a valid region"}}},
		{"no path", b, errors.New("it failed"), config, []tenon.Diagnostic{{Code: ctytenon.CodeCtyError, Message: "it failed"}}},
		{
			"joined errors",
			b,
			errors.Join(pathErr, cty.GetAttrPath("servers").NewErrorf("too few")),
			config,
			[]tenon.Diagnostic{
				{Code: ctytenon.CodeCtyError, Path: pathOf("env", tenon.String("region")), Message: "not a valid region"},
				{Code: ctytenon.CodeCtyError, Path: pathOf("servers"), Message: "too few"},
			},
		},
		{"within a sensitive value", marking, secretErr, login, []tenon.Diagnostic{{Code: ctytenon.CodeCtyError, Path: pathOf("password"), Message: `redacted("sensitive") holds what cty refused`}}},
		{"within a mark the Bridge does not map", b, secretErr, login, []tenon.Diagnostic{{Code: ctytenon.CodeCtyError, Path: pathOf("password"), Message: `a value carrying the cty marks "sensitive" holds what cty refused`}}},
	} {
		err := c.bridge.ErrorFromCty(c.err, c.v)
		wantFailure(t, c.name+": ErrorFromCty", "", err, c.want)
		// cty's PathError is not comparable, which errors.Is needs.
		if causes := asError(t, err).Unwrap(); len(causes) != 1 || causes[0].Error() != c.err.Error() {
			t.Errorf("%s: ErrorFromCty(%v) = %v, caused by %v; want it caused by the error", c.name, c.err, err, causes)
		}
	}
	if err := b.ErrorFromCty(nil, config); err != nil {
		t.Errorf("ErrorFromCty(nil) = %v, want nil", err)
	}
}

// TestPathUsageErrors holds the zero values of either side, which are not
// values, to usage panics.
func TestPathUsageErrors(t *testing.T) {
	var b ctytenon.Bridge
	for _, c := range []struct {
		want string
		f    func()
	}{
		{"tenon: usage: PathFromCty called with cty.NilVal, which is not a value", func() { b.PathFromCty(nil, cty.NilVal) }},
		{"tenon: usage: PathToCty called with the zero Value, which is not a value", func() { b.PathToCty(tenon.Path{}, tenon.Value{}) }},
	} {
		func() {
			defer func() {
				if r := recover(); r != c.want {
					t.Errorf("recovered %v, want %q", r, c.want)
				}
			}()
			c.f()
		}()
	}
}
