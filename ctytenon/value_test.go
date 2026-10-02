package ctytenon_test

import (
	"errors"
	"maps"
	"math/big"
	"math/rand"
	"reflect"
	"slices"
	"strconv"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/ctytenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/internal/conformance/values"
	"github.com/zclconf/go-cty/cty"
)

var (
	str = tenon.StringType()
	num = tenon.NumberType()
	boo = tenon.BoolType()
)

func n(i int64) tenon.Value { return tenon.NumberFromInt(i) }

func posInf() *big.Float { return new(big.Float).SetInf(false) }

// digits returns n random decimal digits, the first not zero.
func digits(r *rand.Rand, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte('0' + r.Intn(10))
	}
	b[0] = byte('1' + r.Intn(9))
	return string(b)
}

// TestValues holds a value of each kind, and nulls, unknown and pending
// values, to crossing as itself, both ways.
func TestValues(t *testing.T) {
	var b ctytenon.Bridge
	point := map[string]cty.Type{"a": cty.String}
	for _, c := range []struct {
		name  string
		cty   cty.Value
		tenon tenon.Value
	}{
		{"a bool", cty.True, tenon.Bool(true)},
		{"a string", cty.StringVal("caf\U000000e9"), tenon.String("caf\U000000e9")},
		{"HCL's 0.1", cty.MustParseNumberVal("0.1"), tenon.NumberFromText("0.1")},
		{"an integer", cty.NumberIntVal(-7), n(-7)},
		{"a list", cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}), tenon.List(str, tenon.String("a"), tenon.String("b"))},
		{"an empty list", cty.ListValEmpty(cty.Number), tenon.List(num)},
		{"a set", cty.SetVal([]cty.Value{cty.NumberIntVal(1), cty.NumberIntVal(2)}), tenon.Set(num, n(1), n(2))},
		{"an empty set", cty.SetValEmpty(cty.Bool), tenon.Set(boo)},
		{"a map", cty.MapVal(map[string]cty.Value{"a": cty.NumberIntVal(1), "b c": cty.NumberIntVal(2)}), tenon.Map(num, map[string]tenon.Value{"a": n(1), "b c": n(2)})},
		{"an empty map", cty.MapValEmpty(cty.String), tenon.Map(str, nil)},
		{"a tuple", cty.TupleVal([]cty.Value{cty.StringVal("x"), cty.NumberIntVal(1)}), tenon.Tuple(tenon.String("x"), n(1))},
		{"the empty tuple", cty.EmptyTupleVal, tenon.Tuple()},
		{
			"an object",
			cty.ObjectVal(map[string]cty.Value{"name": cty.StringVal("web"), "ports": cty.ListVal([]cty.Value{cty.NumberIntVal(80)})}),
			tenon.Object(map[string]tenon.Value{"name": tenon.String("web"), "ports": tenon.List(num, n(80))}),
		},
		{"the empty object", cty.EmptyObjectVal, tenon.Object(nil)},
		{"a null", cty.NullVal(cty.String), tenon.Null(str)},
		{"a null list of objects", cty.NullVal(cty.List(cty.Object(point))), tenon.Null(tenon.ListType(tenon.ObjectType(map[string]tenon.Type{"a": str})))},
		{"an unknown number", cty.UnknownVal(cty.Number), tenon.Unknown(num)},
		{"a list holding an unknown", cty.ListVal([]cty.Value{cty.UnknownVal(cty.String), cty.StringVal("a")}), tenon.List(str, tenon.Unknown(str), tenon.String("a"))},
		{"a null within an object", cty.ObjectVal(map[string]cty.Value{"a": cty.NullVal(cty.Number)}), tenon.Object(map[string]tenon.Value{"a": tenon.Null(num)})},
		// A value whose type holds cty.DynamicPseudoType is pending.
		{"DynamicVal", cty.DynamicVal, tenon.Pending(tenon.Any())},
		{"the untyped null", cty.NullVal(cty.DynamicPseudoType), tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly())},
		{"an unknown list of some type", cty.UnknownVal(cty.List(cty.DynamicPseudoType)), tenon.Pending(tenon.ListOf(tenon.Any()))},
		{
			"an unknown object of exactly its attributes",
			cty.UnknownVal(cty.Object(map[string]cty.Type{"a": cty.DynamicPseudoType, "b": cty.String})),
			tenon.Pending(tenon.ObjectWith(map[string]tenon.Field{"a": tenon.Required(tenon.Any()), "b": tenon.Required(tenon.Exactly(str))}, true)),
		},
		{
			"a null tuple of some type and an object",
			cty.NullVal(cty.Tuple([]cty.Type{cty.DynamicPseudoType, cty.Object(point)})),
			tenon.Narrow(tenon.Pending(tenon.TupleOf(tenon.Any(), tenon.Exactly(tenon.ObjectType(map[string]tenon.Type{"a": str})))), tenon.NullOnly()),
		},
	} {
		got, err := b.FromCty(c.cty)
		if err != nil || !got.Equal(c.tenon) {
			t.Errorf("%s: FromCty(%#v) = %v, %v; want %v", c.name, c.cty, got, err, c.tenon)
		}
		back, err := b.ToCty(c.tenon)
		if err != nil || !back.RawEquals(c.cty) {
			t.Errorf("%s: ToCty(%v) = %#v, %v; want %#v", c.name, c.tenon, back, err, c.cty)
		}
	}
}

// TestValuesThatWiden holds the values that cross as one allowing more than
// they do to doing so: what a known value whose type holds
// cty.DynamicPseudoType holds, which tenon has no known value for, and what
// cty cannot say of a pending value.
func TestValuesThatWiden(t *testing.T) {
	var b ctytenon.Bridge
	for _, c := range []struct {
		name  string
		cty   cty.Value
		tenon tenon.Value
	}{
		{"a list holding DynamicVal", cty.ListVal([]cty.Value{cty.DynamicVal}), tenon.Narrow(tenon.Pending(tenon.ListOf(tenon.Any())), tenon.NotNull())},
		{
			"a tuple holding DynamicVal and a number",
			cty.TupleVal([]cty.Value{cty.DynamicVal, cty.NumberIntVal(1)}),
			tenon.Narrow(tenon.Pending(tenon.TupleOf(tenon.Any(), tenon.Exactly(num))), tenon.NotNull()),
		},
		{
			"a null of an object type with optional attributes",
			cty.NullVal(cty.ObjectWithOptionalAttrs(map[string]cty.Type{"a": cty.String}, []string{"a"})),
			tenon.Null(tenon.ObjectType(map[string]tenon.Type{"a": str})),
		},
	} {
		if got, err := b.FromCty(c.cty); err != nil || !got.Equal(c.tenon) {
			t.Errorf("%s: FromCty(%#v) = %v, %v; want %v", c.name, c.cty, got, err, c.tenon)
		}
	}
	for _, c := range []struct {
		name  string
		tenon tenon.Value
		cty   cty.Value
	}{
		{"a pending value not null", tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NotNull()), cty.DynamicVal},
		{"an open object", tenon.Pending(tenon.ObjectWith(map[string]tenon.Field{"a": tenon.Required(tenon.Any())}, false)), cty.DynamicVal},
		{"an optional field", tenon.Pending(tenon.ObjectWith(map[string]tenon.Field{"a": tenon.Optional(tenon.Exactly(str))}, true)), cty.DynamicVal},
		{"a list of open objects", tenon.Pending(tenon.ListOf(tenon.ObjectWith(nil, false))), cty.UnknownVal(cty.List(cty.DynamicPseudoType))},
		{"a pending value of one type", tenon.Pending(tenon.Exactly(str)), cty.UnknownVal(cty.String)},
	} {
		if got, err := b.ToCty(c.tenon); err != nil || !got.RawEquals(c.cty) {
			t.Errorf("%s: ToCty(%v) = %#v, %v; want %#v", c.name, c.tenon, got, err, c.cty)
		}
	}
}

type label string

func (l label) MarkID() string               { return string(l) }
func (label) Propagation() tenon.Propagation { return tenon.Propagate }
func (label) Redacting() bool                { return false }
func (l label) GoString() string             { return "label(" + strconv.Quote(string(l)) + ")" }
func pathOf(steps ...any) tenon.Path         { return path(tenon.Path{}, steps...) }
func path(p tenon.Path, steps ...any) tenon.Path {
	for _, s := range steps {
		switch s := s.(type) {
		case string:
			p = p.Attribute(s)
		case int:
			p = p.Index(n(int64(s)))
		case tenon.Value:
			p = p.Index(s)
		}
	}
	return p
}

// TestWhatDoesNotCross holds each value that cannot cross to failing with a
// diagnostic of its code located at the part that fails, every part that
// fails reported.
func TestWhatDoesNotCross(t *testing.T) {
	var b ctytenon.Bridge
	thing := cty.Capsule("thing", reflect.TypeOf(0))
	one := 1
	for _, c := range []struct {
		name string
		cty  cty.Value
		want []tenon.Diagnostic
	}{
		{"a string not UTF-8", cty.StringVal("\xff"), []tenon.Diagnostic{{Code: tenon.CodeStringInvalidUTF8}}},
		{
			"two failures within",
			cty.ObjectVal(map[string]cty.Value{
				"a": cty.ListVal([]cty.Value{cty.NumberIntVal(1), cty.NumberVal(posInf())}),
				"b": cty.MapVal(map[string]cty.Value{"k": cty.MustParseNumberVal("1e1000001")}),
			}),
			[]tenon.Diagnostic{
				{Code: tenon.CodeEncodeNotANumber, Path: pathOf("a", 1)},
				{Code: tenon.CodeNumberOutOfRange, Path: pathOf("b", tenon.String("k"))},
			},
		},
		{"a marked attribute", cty.ObjectVal(map[string]cty.Value{"password": cty.StringVal("x").Mark("sensitive")}), []tenon.Diagnostic{{Code: ctytenon.CodeUnmappedMark, Path: pathOf("password")}}},
		{"a marked set", cty.SetVal([]cty.Value{cty.StringVal("x")}).Mark("sensitive"), []tenon.Diagnostic{{Code: ctytenon.CodeUnmappedMark}}},
		{"an empty name within", cty.ObjectVal(map[string]cty.Value{"x": cty.ObjectVal(map[string]cty.Value{"": cty.True})}), []tenon.Diagnostic{{Code: tenon.CodeObjectEmptyName, Path: pathOf("x")}}},
		{"a capsule within", cty.ObjectVal(map[string]cty.Value{"thing": cty.CapsuleVal(thing, &one), "name": cty.StringVal("x")}), []tenon.Diagnostic{{Code: ctytenon.CodeUnpairedCapsule, Path: pathOf("thing")}}},
		{"an empty list of capsules", cty.ListValEmpty(thing), []tenon.Diagnostic{{Code: ctytenon.CodeUnpairedCapsule}}},
		{"a map key not UTF-8", cty.MapVal(map[string]cty.Value{"\xfe": cty.True}), []tenon.Diagnostic{{Code: tenon.CodeStringInvalidUTF8}}},
		{"a failure within a value of some type", cty.TupleVal([]cty.Value{cty.DynamicVal, cty.StringVal("\xff")}), []tenon.Diagnostic{{Code: tenon.CodeStringInvalidUTF8, Path: pathOf(1)}}},
	} {
		got, err := b.FromCty(c.cty)
		wantFailure(t, c.name+": FromCty", got, err, c.want)
	}

	type thingT struct{}
	capsule := tenon.NewCapsule("thing", tenon.CapsuleOps[thingT]{})
	for _, c := range []struct {
		name  string
		tenon tenon.Value
		want  []tenon.Diagnostic
	}{
		{"a marked element", tenon.List(str, tenon.String("a"), tenon.WithMarks(tenon.String("b"), label("audited"))), []tenon.Diagnostic{{Code: ctytenon.CodeUnmappedMark, Path: pathOf(1), Message: `the value carries the marks "audited", which the Bridge maps to no cty mark`}}},
		{"a OneOf", tenon.Pending(tenon.OneOf(tenon.Exactly(str), tenon.Exactly(num))), []tenon.Diagnostic{{Code: ctytenon.CodeOneOf}}},
		{"a capsule within", tenon.Object(map[string]tenon.Value{"c": capsule.Value(&thingT{}), "d": tenon.Bool(true)}), []tenon.Diagnostic{{Code: ctytenon.CodeUnpairedCapsule, Path: pathOf("c")}}},
		{"an empty map of capsules", tenon.Map(capsule.Type(), nil), []tenon.Diagnostic{{Code: ctytenon.CodeUnpairedCapsule}}},
	} {
		got, err := b.ToCty(c.tenon)
		wantFailure(t, c.name+": ToCty", got, err, c.want)
	}

	// An error value crosses as the error holding it.
	e := tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: "it failed", Path: pathOf("a")})
	if got, err := b.ToCty(e); err == nil || !asError(t, err).Value().Equal(e) {
		t.Errorf("ToCty of an error value = %#v, %v; want the error holding it", got, err)
	}
}

// wantFailure fails t unless err is a *tenon.Error holding diagnostics of the
// codes and paths want gives, and messages too where want gives them.
func wantFailure[V any](t *testing.T, name string, got V, err error, want []tenon.Diagnostic) {
	t.Helper()
	if err == nil {
		t.Errorf("%s = %v; want it to fail", name, got)
		return
	}
	diags := asError(t, err).Diagnostics()
	if len(diags) != len(want) {
		t.Errorf("%s failed with %v; want %d diagnostics", name, err, len(want))
		return
	}
	for i, d := range diags {
		if w := want[i]; d.Code != w.Code || !d.Path.Equal(w.Path) || w.Message != "" && d.Message != w.Message {
			t.Errorf("%s failed with %s at %v: %q; want %s at %v %q", name, d.Code, d.Path, d.Message, w.Code, w.Path, w.Message)
		}
	}
}

func asError(t *testing.T, err error) *tenon.Error {
	t.Helper()
	var e *tenon.Error
	if !errors.As(err, &e) {
		t.Fatalf("%v is not a *tenon.Error", err)
	}
	return e
}

// TestValueUsageErrors holds the zero values of either side, which are not
// values, to usage panics.
func TestValueUsageErrors(t *testing.T) {
	var b ctytenon.Bridge
	for _, c := range []struct {
		want string
		f    func()
	}{
		{"tenon: usage: FromCty called with cty.NilVal, which is not a value", func() { b.FromCty(cty.NilVal) }},
		{"tenon: usage: ToCty called with the zero Value, which is not a value", func() { b.ToCty(tenon.Value{}) }},
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

// plain reports whether v crosses to cty and back as itself: it is neither an
// error value nor pending, carries no mark and holds no capsule, and every
// unknown value within it has its type's whole range.
func plain(v tenon.Value) bool {
	if _, marks := tenon.Unmark(v); len(marks) > 0 || v.IsError() || v.IsPending() || holdsCapsule(v.Type()) {
		return false
	}
	if !v.HasContent() {
		return v.IsNull() || v.Range().Equal(tenon.Unknown(v.Type()).Range())
	}
	switch v.Type().Kind() {
	case tenon.KindList, tenon.KindSet, tenon.KindTuple:
		for e := range v.ElementsSeq() {
			if !plain(e) {
				return false
			}
		}
	case tenon.KindMap:
		for _, e := range v.MapEntries() {
			if !plain(e) {
				return false
			}
		}
	case tenon.KindObject:
		for _, e := range v.Attributes() {
			if !plain(e) {
				return false
			}
		}
	}
	return true
}

// TestCorpusValues carries every value of tenon's corpus, and every value
// within one, to cty and back. A plain value comes back identical; any other
// is refused for a mark, a capsule or a OneOf, or crosses back allowing what
// it allowed and more: a value of the same type, or of a type its constraint
// allows.
func TestCorpusValues(t *testing.T) {
	var b ctytenon.Bridge
	same, other := 0, 0
	for _, v := range within(values.All()) {
		c, err := b.ToCty(v)
		if v.IsError() {
			if !asError(t, err).Value().Equal(v) {
				t.Errorf("ToCty(%v) = %#v, %v; want the error holding it", v, c, err)
			}
			continue
		}
		if err != nil {
			for _, d := range asError(t, err).Diagnostics() {
				if d.Code != ctytenon.CodeUnmappedMark && d.Code != ctytenon.CodeUnpairedCapsule && d.Code != ctytenon.CodeOneOf {
					t.Errorf("ToCty(%v) failed with %v", v, err)
				}
			}
			continue
		}
		back, err := b.FromCty(c)
		switch {
		case err != nil:
			t.Errorf("%v crossed as %#v and failed back: %v", v, c, err)
		case plain(v):
			if !back.Equal(v) {
				t.Errorf("%v crossed as %#v and back as %v", v, c, back)
			}
			same++
		default:
			// A pending value of a constraint one type satisfies comes back
			// as the unknown value of that type.
			if !v.IsPending() && back.Type() != v.Type() || v.IsPending() && !back.IsPending() && !tenon.Satisfies(v.Constraint(), back.Type()) {
				t.Errorf("%v crossed as %#v and back as %v", v, c, back)
			}
			other++
		}
	}
	if same < 100 || other < 5 {
		t.Errorf("%d corpus values crossed back identical and %d otherwise; want many of each", same, other)
	}
}

// TestValuesRoundTripFromTenon carries random plain tenon values to cty and
// back, which gives each identical.
func TestValuesRoundTripFromTenon(t *testing.T) {
	var b ctytenon.Bridge
	r := rand.New(rand.NewSource(20261005))
	for range conformance.Iterations(t, 2000) {
		v := randomTenonValue(r, randomTenonType(r, 3))
		c, err := b.ToCty(v)
		if err != nil {
			t.Fatalf("ToCty(%v): %v", v, err)
		}
		if back, err := b.FromCty(c); err != nil || !back.Equal(v) {
			t.Fatalf("%v crossed as %#v and back as %v, %v", v, c, back, err)
		}
	}
}

// TestValuesRoundTripFromCty carries random cty values, unknown and null
// parts, and values whose type holds cty.DynamicPseudoType, among them, to
// tenon and back, which gives each RawEquals, but where tenon is the more
// decided of the two. cty numbers that its parser and its integer
// constructors make compare by value.
func TestValuesRoundTripFromCty(t *testing.T) {
	var b ctytenon.Bridge
	r := rand.New(rand.NewSource(20261006))
	for range conformance.Iterations(t, 2000) {
		var v cty.Value
		if r.Intn(5) == 0 {
			// A value's type has no optional attributes, which are a
			// conversion's.
			ct := randomCtyConstraint(r, 3).WithoutOptionalAttributesDeep()
			v = []cty.Value{cty.UnknownVal(ct), cty.NullVal(ct)}[r.Intn(2)]
		} else {
			v = randomCtyValue(r, randomCtyType(r, 3))
		}
		tv, err := b.FromCty(v)
		if err != nil {
			t.Fatalf("FromCty(%#v): %v", v, err)
		}
		back, err := b.ToCty(tv)
		if err != nil {
			t.Fatalf("%#v crossed as %v and failed back: %v", v, tv, err)
		}
		if back.RawEquals(v) {
			continue
		}
		// tenon can know more than cty said: that an unknown member of a set
		// is one the set holds already, where its type allows no other, as
		// the unknown empty object is {} or null; or the most members a set
		// of a type of few values can have. What comes back says the same to
		// tenon.
		if again, err := b.FromCty(back); err != nil || !again.Equal(tv) {
			t.Fatalf("%#v crossed as %v and back as %#v", v, tv, back)
		}
	}
}

// randomTenonValue returns a random value of type typ, nulls and unknown
// values among its parts.
func randomTenonValue(r *rand.Rand, typ tenon.Type) tenon.Value {
	switch r.Intn(12) {
	case 0:
		return tenon.Null(typ)
	case 1:
		return tenon.Unknown(typ)
	}
	switch typ.Kind() {
	case tenon.KindBool:
		return tenon.Bool(r.Intn(2) == 0)
	case tenon.KindNumber:
		return tenon.NumberFromText(digits(r, 1+r.Intn(40)) + "e" + strconv.Itoa(r.Intn(81)-40))
	case tenon.KindString:
		return tenon.String(names[r.Intn(len(names))])
	case tenon.KindList, tenon.KindSet:
		elems := make([]tenon.Value, r.Intn(4))
		for i := range elems {
			elems[i] = randomTenonValue(r, typ.ElementType())
		}
		if typ.Kind() == tenon.KindList {
			return tenon.List(typ.ElementType(), elems...)
		}
		return tenon.Set(typ.ElementType(), elems...)
	case tenon.KindMap:
		entries := map[string]tenon.Value{}
		for range r.Intn(4) {
			entries[names[r.Intn(len(names))]] = randomTenonValue(r, typ.ElementType())
		}
		return tenon.Map(typ.ElementType(), entries)
	case tenon.KindTuple:
		var elems []tenon.Value
		for _, e := range typ.TupleElementTypes() {
			elems = append(elems, randomTenonValue(r, e))
		}
		return tenon.Tuple(elems...)
	}
	attrs := map[string]tenon.Value{}
	for _, name := range typ.AttributeNames() {
		attrs[name] = randomTenonValue(r, typ.AttributeType(name))
	}
	return tenon.Object(attrs)
}

// randomCtyValue returns a random cty value of type typ, nulls and unknown
// values among its parts.
func randomCtyValue(r *rand.Rand, typ cty.Type) cty.Value {
	switch r.Intn(12) {
	case 0:
		return cty.NullVal(typ)
	case 1:
		return cty.UnknownVal(typ)
	}
	elems := func(e cty.Type) []cty.Value {
		out := make([]cty.Value, r.Intn(4))
		for i := range out {
			out[i] = randomCtyValue(r, e)
		}
		return out
	}
	switch {
	case typ == cty.Bool:
		return cty.BoolVal(r.Intn(2) == 0)
	case typ == cty.Number:
		if r.Intn(2) == 0 {
			return cty.NumberIntVal(r.Int63() - r.Int63())
		}
		return cty.MustParseNumberVal(digits(r, 1+r.Intn(40)) + "e" + strconv.Itoa(r.Intn(81)-40))
	case typ == cty.String:
		return cty.StringVal(names[r.Intn(len(names))])
	case typ.IsListType():
		if e := elems(typ.ElementType()); len(e) > 0 {
			return cty.ListVal(e)
		}
		return cty.ListValEmpty(typ.ElementType())
	case typ.IsSetType():
		if e := elems(typ.ElementType()); len(e) > 0 {
			return cty.SetVal(e)
		}
		return cty.SetValEmpty(typ.ElementType())
	case typ.IsMapType():
		entries := map[string]cty.Value{}
		for range r.Intn(4) {
			entries[names[r.Intn(len(names))]] = randomCtyValue(r, typ.ElementType())
		}
		if len(entries) > 0 {
			return cty.MapVal(entries)
		}
		return cty.MapValEmpty(typ.ElementType())
	case typ.IsTupleType():
		var out []cty.Value
		for _, e := range typ.TupleElementTypes() {
			out = append(out, randomCtyValue(r, e))
		}
		return cty.TupleVal(out)
	}
	attrs := map[string]cty.Value{}
	// In order of name, so that the values a seed gives do not depend on
	// Go's map order.
	for _, name := range slices.Sorted(maps.Keys(typ.AttributeTypes())) {
		attrs[name] = randomCtyValue(r, typ.AttributeType(name))
	}
	return cty.ObjectVal(attrs)
}
