package tenon_test

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/conformance"
	"github.com/kmoneil/tenon/conformance/values"
)

// document is the head of every document: the document tag, a two-element
// array, and format version 1.
const document = "da74656e00 82 01"

// fromHex decodes hex written with spaces for reading.
func fromHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// wantEncoding fails t unless v serializes to the document holding item, given
// in hex.
func wantEncoding(t *testing.T, what string, v tenon.Value, item string) {
	t.Helper()
	got, failure, ok := tenon.Serialize(v)
	if !ok {
		t.Errorf("%s: Serialize(%v) failed: %v", what, v, failure)
		return
	}
	if want := fromHex(t, document+item); !bytes.Equal(got, want) {
		t.Errorf("%s: Serialize(%v) = %x, want %x", what, v, got, want)
	}
}

// wantSerializeFailure fails t unless v fails to serialize with exactly these
// diagnostics.
func wantSerializeFailure(t *testing.T, what string, v tenon.Value, want ...wantDiag) {
	t.Helper()
	_, failure, ok := tenon.Serialize(v)
	if ok {
		t.Errorf("%s: Serialize(%v) succeeded", what, v)
		return
	}
	wantErrors(t, what, failure, want...)
}

// signal is an encodable mark serialized as its identifier alone.
type signal struct {
	id     string
	policy tenon.Propagation
	deep   bool
}

func (m signal) MarkID() string                 { return m.id }
func (m signal) Propagation() tenon.Propagation { return m.policy }
func (signal) Redacting() bool                  { return false }
func (m signal) Deep() bool                     { return m.deep }
func (signal) MarkPayload() (tenon.Value, bool) { return tenon.Value{}, false }

// note is an encodable mark serialized with a string.
type note struct{ id, text string }

func (m note) MarkID() string                   { return m.id }
func (note) Propagation() tenon.Propagation     { return tenon.Propagate }
func (note) Redacting() bool                    { return false }
func (m note) MarkPayload() (tenon.Value, bool) { return tenon.String(m.text), true }

// degrees is a capsule type serialized as a number.
var degrees = tenon.Capsule("degrees", tenon.CapsuleOps[celsius]{
	Equals: func(a, b *celsius) bool { return *a == *b },
	Hash:   func(v *celsius) uint64 { return uint64(v.degrees) },
	Encoding: &tenon.CapsuleEncoding[celsius]{
		ID:     "t/c",
		Type:   tenon.NumberType(),
		Encode: func(v *celsius) tenon.Value { return tenon.NumberFromInt(v.degrees) },
		Decode: func(v tenon.Value) (*celsius, []tenon.Diagnostic) {
			i, _ := v.AsInt64()
			return &celsius{i}, nil
		},
	},
})

func TestConformance_SE004_Documents(t *testing.T) {
	conformance.Covers(t, "SE-004", "SE-010")
	got, _, _ := tenon.Serialize(tenon.Bool(true))
	if want := fromHex(t, "da74656e00 82 01 83 00 01 f5"); !bytes.Equal(got, want) {
		t.Errorf("Serialize(true) = %x, want %x", got, want)
	}
	if !bytes.HasPrefix(got, []byte{0xda, 't', 'e', 'n'}) {
		t.Errorf("a document begins %x, not the tag spelling ten", got[:4])
	}
	mustPanicUsage(t, "use of the zero Value", func() { tenon.Serialize(tenon.Value{}) })
}

func TestConformance_SE030_ContentByType(t *testing.T) {
	conformance.Covers(t, "SE-030", "SE-010", "SE-020")
	for _, tt := range []struct {
		name string
		v    tenon.Value
		item string
	}{
		{"false", tenon.Bool(false), "83 00 01 f4"},
		{"a string", s("a\xc3\xa9"), "83 00 03 63 61c3a9"},
		{"a list", tenon.ListVal(num, n(1), n(2)), "83 00 82 04 02 82 01 02"},
		{"an empty list of strings", tenon.ListVal(str), "83 00 82 04 03 80"},
		{"a map, keys in order", tenon.MapVal(num, map[string]tenon.Value{"b": n(2), "a": n(1)}),
			"83 00 82 06 02 82 82 6161 01 82 6162 02"},
		{"a tuple", tenon.TupleVal(tenon.Bool(true), s("x")), "83 00 82 07 82 01 03 82 f5 6178"},
		{"an object, in attribute order", obj(map[string]tenon.Value{"b": n(1), "a": s("x")}),
			"83 00 82 08 82 82 6161 03 82 6162 02 82 6178 01"},
		{"a null list", tenon.NullVal(tenon.List(num)), "83 00 82 04 02 f6"},
		{"a list holding a null", tenon.ListVal(num, tenon.NullVal(num)), "83 00 82 04 02 81 f6"},
		{"a capsule value", tenon.CapsuleVal(degrees, &celsius{21}), "83 00 82 09 63 742f63 82 02 15"},
		{"a nested type", tenon.NullVal(tenon.Set(tenon.Map(tenon.Tuple()))), "83 00 82 05 82 06 82 07 80 f6"},
	} {
		wantEncoding(t, tt.name, tt.v, tt.item)
	}
}

func TestConformance_SE032_Numbers(t *testing.T) {
	conformance.Covers(t, "SE-032")
	for _, tt := range []struct {
		text string
		item string
	}{
		{"0", "00"},
		{"23", "17"},
		{"-5", "24"},
		{"1000", "19 03e8"},
		{"1.000", "01"},
		{"1.5", "c4 82 20 0f"},
		{"-0.25", "c4 82 21 38 18"},
		{"1e30", "c4 82 18 1e 01"},
		{"18446744073709551615", "1b ffffffffffffffff"},
		{"-18446744073709551616", "3b ffffffffffffffff"},
		{"18446744073709551616", "c4 82 00 c2 49 010000000000000000"},
		{"-18446744073709551617", "c4 82 00 c3 49 010000000000000000"},
		{"123456789012345678901.5", "c4 82 20 c2 49 42ed123b0bd8203a17"},
	} {
		wantEncoding(t, tt.text, tenon.NumberFromText(tt.text), "83 00 02 "+tt.item)
	}
}

func TestConformance_SE033_SetMembersInEncodingOrder(t *testing.T) {
	conformance.Covers(t, "SE-033")
	// 1 is 01 and -1 is 20: the order of the bytes, not of the numbers.
	wantEncoding(t, "numbers", tenon.SetVal(num, n(-1), n(1)), "83 00 82 05 02 82 01 20")
	wantEncoding(t, "given the other way", tenon.SetVal(num, n(1), n(-1)), "83 00 82 05 02 82 01 20")
	// An unknown member, and one held twice, order by their bytes too.
	u := tenon.Unknown(num)
	wantEncoding(t, "unknowns", tenon.SetVal(num, u, n(1), u), "83 00 82 05 02 83 01 da74656e01a0 da74656e01a0")
}

func TestConformance_SE034_Ranges(t *testing.T) {
	conformance.Covers(t, "SE-034")
	unknownNum := tenon.Unknown(num)
	for _, tt := range []struct {
		name string
		v    tenon.Value
		item string
	}{
		{"nothing known", unknownNum, "83 00 02 da74656e01 a0"},
		{"not null, at least 1", tenon.Narrow(unknownNum, tenon.NotNull(), tenon.NumberMin(n(1), true)),
			"83 00 02 da74656e01 a2 00 f5 01 82 01 f5"},
		{"below 2.5", tenon.Narrow(unknownNum, tenon.NumberMax(tenon.NumberFromText("2.5"), false)),
			"83 00 02 da74656e01 a1 02 82 c4 82 20 18 19 f4"},
		{"a prefix and the length it implies", tenon.Narrow(tenon.Unknown(str), tenon.StringPrefix("ab-")),
			"83 00 03 da74656e01 a2 03 63 61622d 04 03"},
		{"at most two", tenon.Narrow(tenon.Unknown(tenon.List(str)), tenon.LengthMax(2)),
			"83 00 82 04 03 da74656e01 a1 05 02"},
		{"a listed member", tenon.Narrow(tenon.Unknown(tenon.Set(num)), tenon.Members(n(1))),
			"83 00 82 05 02 da74656e01 a2 04 01 06 81 01"},
		{"an unknown member of a list", tenon.ListVal(num, unknownNum), "83 00 82 04 02 81 da74656e01 a0"},
	} {
		wantEncoding(t, tt.name, tt.v, tt.item)
	}
}

func TestConformance_SE021_PendingValuesAndConstraints(t *testing.T) {
	conformance.Covers(t, "SE-021", "SE-010")
	for _, tt := range []struct {
		name string
		v    tenon.Value
		item string
	}{
		{"any", tenon.Pending(tenon.Any()), "83 01 81 02 00"},
		{"a list of any, not null", tenon.Narrow(tenon.Pending(tenon.ListOf(tenon.Any())), tenon.NotNull()), "83 01 82 03 81 02 01"},
		{"null", tenon.Narrow(tenon.Pending(is(num)), tenon.Null()), "83 01 82 01 02 02"},
		{"an object with a field", tenon.Pending(fields(true, "a", tenon.Required(is(num)))), "83 01 83 06 81 83 6161 f5 82 01 02 f5 00"},
		{"one of, in the order given", tenon.Pending(tenon.OneOf(is(str), is(num))), "83 01 82 08 82 82 01 03 82 01 02 00"},
		{"a tuple of sets and maps", tenon.Pending(tenon.TupleOf(tenon.SetOf(tenon.Any()), tenon.MapOf(is(boo)))), "83 01 82 07 82 82 04 81 02 82 05 82 01 01 00"},
	} {
		wantEncoding(t, tt.name, tt.v, tt.item)
	}
}

func TestConformance_SE011_Diagnostics(t *testing.T) {
	conformance.Covers(t, "SE-011", "SE-010")
	p := tenon.Path{}.Attribute("a").Index(n(0)).Index(s("k"))
	failed := tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: "x", Path: p})
	wantEncoding(t, "an error", failed, "82 02 81 83 6a 6170702e6661696c6564 61 78 83 6161 00 81 616b")
	// A mark on a key is no part of the diagnostic.
	marked := tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: "x", Path: tenon.Path{}.Attribute("a").Index(tenon.WithMarks(n(0), signal{id: "m"})).Index(s("k"))})
	wantEncoding(t, "an error with a marked key", marked, "82 02 81 83 6a 6170702e6661696c6564 61 78 83 6161 00 81 616b")
}

func TestConformance_SE031_Marks(t *testing.T) {
	conformance.Covers(t, "SE-031", "SE-041", "SE-010")
	m, deep := signal{id: "m"}, signal{id: "d", deep: true}
	wantEncoding(t, "a marked value", tenon.WithMarks(tenon.Bool(true), m), "83 00 01 da74656e02 82 f5 81 81 616d")
	wantEncoding(t, "a mark with a payload", tenon.WithMarks(tenon.Bool(true), note{"p", "v"}), "83 00 01 da74656e02 82 f5 81 83 6170 03 6176")
	wantEncoding(t, "marks in the order of their bytes", tenon.WithMarks(tenon.Bool(true), note{"p", "v"}, m),
		"83 00 01 da74656e02 82 f5 82 81 616d 83 6170 03 6176")
	wantEncoding(t, "a marked pending value", tenon.WithMarks(tenon.Pending(tenon.Any()), m), "da74656e02 82 83 01 81 02 00 81 81 616d")
	wantEncoding(t, "a marked member", tenon.ListVal(num, tenon.WithMarks(n(1), m)), "83 00 82 04 02 81 da74656e02 82 01 81 81 616d")
	// A deep mark is listed where it was attached, not again on what holds it.
	wantEncoding(t, "a deep mark", tenon.WithMarks(tenon.ListVal(num, n(1)), deep), "83 00 82 04 02 da74656e02 82 81 01 81 81 6164")
	wantEncoding(t, "a deep mark attached twice", tenon.WithMarks(tenon.ListVal(num, tenon.WithMarks(n(1), deep)), deep),
		"83 00 82 04 02 da74656e02 82 81 01 81 81 6164")
	// A member of a set carries no marks; the set does.
	wantEncoding(t, "a deep mark on a set", tenon.WithMarks(tenon.SetVal(num, n(1)), deep), "83 00 82 05 02 da74656e02 82 81 01 81 81 6164")
}

func TestConformance_SE040_Capsules(t *testing.T) {
	conformance.Covers(t, "SE-040", "SE-042", "SE-050")
	wantEncoding(t, "a capsule type in a constraint", tenon.Pending(is(degrees)), "83 01 82 01 82 09 63 742f63 00")
	// Values the type reports equal encode alike.
	a, _, _ := tenon.Serialize(tenon.CapsuleVal(degrees, &celsius{5}))
	b, _, _ := tenon.Serialize(tenon.CapsuleVal(degrees, &celsius{5}))
	if !bytes.Equal(a, b) {
		t.Errorf("equal capsule values encode as %x and %x", a, b)
	}

	opaque := tenon.Capsule("opaque", tenon.CapsuleOps[celsius]{})
	wantSerializeFailure(t, "a list of undeclared capsules", tenon.ListVal(opaque, tenon.CapsuleVal(opaque, &celsius{}), tenon.CapsuleVal(opaque, &celsius{})),
		wantDiag{tenon.CodeSerializeUnencodableCapsule, "."},
		wantDiag{tenon.CodeSerializeUnencodableCapsule, ".[0]"},
		wantDiag{tenon.CodeSerializeUnencodableCapsule, ".[1]"})
	wantSerializeFailure(t, "a pending value naming one", tenon.Pending(tenon.ListOf(is(opaque))),
		wantDiag{tenon.CodeSerializeUnencodableCapsule, "."})
	twin := tenon.Capsule("degrees", tenon.CapsuleOps[celsius]{Encoding: &tenon.CapsuleEncoding[celsius]{
		ID: "t/c", Type: num,
		Encode: func(v *celsius) tenon.Value { return n(v.degrees) },
		Decode: func(tenon.Value) (*celsius, []tenon.Diagnostic) { return &celsius{}, nil },
	}})
	wantSerializeFailure(t, "two types of one identifier", obj(map[string]tenon.Value{
		"a": tenon.CapsuleVal(degrees, &celsius{1}), "b": tenon.CapsuleVal(twin, &celsius{1}),
	}), wantDiag{tenon.CodeSerializeUnencodableCapsule, "."})

	mustPanicUsage(t, "declares an encoding with no identifier", func() {
		tenon.Capsule("x", tenon.CapsuleOps[celsius]{Encoding: &tenon.CapsuleEncoding[celsius]{Type: num}})
	})
	liar := tenon.Capsule("liar", tenon.CapsuleOps[celsius]{Encoding: &tenon.CapsuleEncoding[celsius]{
		ID: "t/liar", Type: num,
		Encode: func(*celsius) tenon.Value { return s("not a number") },
		Decode: func(tenon.Value) (*celsius, []tenon.Diagnostic) { return nil, nil },
	}})
	mustPanicUsage(t, `capsule type "liar" serialized a value as "not a number"`, func() {
		tenon.Serialize(tenon.CapsuleVal(liar, &celsius{}))
	})
	// A null is no encoding of a capsule value, which a decoder could not tell
	// from a value it never gets.
	nothing := tenon.Capsule("nothing", tenon.CapsuleOps[celsius]{Encoding: &tenon.CapsuleEncoding[celsius]{
		ID: "t/nothing", Type: num,
		Encode: func(*celsius) tenon.Value { return tenon.NullVal(num) },
		Decode: func(tenon.Value) (*celsius, []tenon.Diagnostic) { return nil, nil },
	}})
	mustPanicUsage(t, "other than null", func() { tenon.Serialize(tenon.CapsuleVal(nothing, &celsius{})) })
	wantDecodeFailure(t, "a capsule value serialized as a null", document+"83 00 82 09 63 742f63 82 02 f6", tenon.CodeSerializeMalformed)
}

func TestConformance_SE042_UnencodableMarks(t *testing.T) {
	conformance.Covers(t, "SE-042", "SE-050", "SE-051", "MK-009")
	plain := stamp{id: "plain"}
	wantSerializeFailure(t, "marks without encodings, located", obj(map[string]tenon.Value{
		"a": tenon.WithMarks(n(1), plain),
		"b": tenon.ListVal(num, n(2), tenon.WithMarks(n(3), stamp{id: "other"})),
	}),
		wantDiag{tenon.CodeSerializeUnencodableMark, ".a"},
		wantDiag{tenon.CodeSerializeUnencodableMark, ".b[1]"})
	wantSerializeFailure(t, "on an error value", tenon.WithMarks(tenon.ErrorVal(tenon.Diagnostic{Code: "app.x", Message: "x"}), plain),
		wantDiag{tenon.CodeSerializeUnencodableMark, "."})
	mustPanicUsage(t, "serialize alike", func() {
		tenon.Serialize(tenon.WithMarks(n(1), note{"p", "v"}, twinNote{"p", "v"}))
	})
	// A mark's payload is not a null, going out or coming in.
	mustPanicUsage(t, "other than a null", func() { tenon.Serialize(tenon.WithMarks(n(1), nullNote{})) })
	wantDecodeFailure(t, "a mark serialized with a null", document+"83 00 01 da74656e02 82 f5 81 83 6170 03 f6", tenon.CodeSerializeMalformed)
}

// nullNote is a mark whose payload is a null, which breaks the contract of an
// encodable mark.
type nullNote struct{}

func (nullNote) MarkID() string                   { return "p" }
func (nullNote) Propagation() tenon.Propagation   { return tenon.Propagate }
func (nullNote) Redacting() bool                  { return false }
func (nullNote) MarkPayload() (tenon.Value, bool) { return tenon.NullVal(tenon.StringType()), true }

// twinNote is a mark unequal to a note that serializes as one does, which
// breaks the contract of an encodable mark.
type twinNote struct{ id, text string }

func (m twinNote) MarkID() string                   { return m.id }
func (twinNote) Propagation() tenon.Propagation     { return tenon.Propagate }
func (twinNote) Redacting() bool                    { return false }
func (m twinNote) MarkPayload() (tenon.Value, bool) { return tenon.String(m.text), true }

// TestConformance_SE001_OneValueOneEncoding holds the generator's values to
// the rule that identical values, however they were built, encode alike, and
// values that are not identical do not.
func TestConformance_SE001_OneValueOneEncoding(t *testing.T) {
	conformance.Covers(t, "SE-001", "SE-002")
	var encodable []tenon.Value
	var encodings [][]byte
	for _, v := range values.All() {
		if b, _, ok := tenon.Serialize(v); ok {
			encodable = append(encodable, v)
			encodings = append(encodings, b)
		}
	}
	if len(encodable) < 60 {
		t.Fatalf("only %d generated values serialize", len(encodable))
	}
	for i, a := range encodable {
		for j, b := range encodable {
			if same := bytes.Equal(encodings[i], encodings[j]); same != tenon.Identical(a, b) {
				t.Errorf("%v and %v: identical %t, but their encodings equal: %t", a, b, !same, same)
			}
		}
	}
	// One value built different ways.
	m, other := signal{id: "m"}, note{"p", "v"}
	for _, pair := range [][2]tenon.Value{
		{tenon.WithMarks(tenon.WithMarks(n(1), m), other), tenon.WithMarks(n(1), other, m)},
		{obj(map[string]tenon.Value{"x": s("e\U00000301")}), obj(map[string]tenon.Value{"x": s("\U000000e9")})},
		{tenon.SetVal(str, s("b"), s("a"), s("b")), tenon.SetVal(str, s("a"), s("b"))},
		{tenon.Narrow(tenon.Unknown(str), tenon.StringPrefix("ab"), tenon.LengthMin(1)), tenon.Narrow(tenon.Unknown(str), tenon.StringPrefix("ab"))},
	} {
		a, _, _ := tenon.Serialize(pair[0])
		b, _, _ := tenon.Serialize(pair[1])
		if !bytes.Equal(a, b) {
			t.Errorf("%v and %v encode as %x and %x", pair[0], pair[1], a, b)
		}
	}
}
