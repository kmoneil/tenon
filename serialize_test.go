package tenon_test

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/internal/conformance/values"
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

// TestConformance_SE032_NumbersAreWrittenAndReadWithoutBigIntegers holds
// writing and reading a number whose coefficient fits an int64, as nearly
// every number's does, to work that builds no big.Int for it. It reads what a
// number costs as the growth from a list of 100 to a list of 200, which leaves
// out what a document costs whatever it holds. Writing an integer took five
// allocations and reading any number nine more than writing it, since
// Deserialize writes what it read again to check that its input is canonical,
// and three of each were the path to the member, which the encoder builds
// only for a failure now. What is left is the value each number read is.
func TestConformance_SE032_NumbersAreWrittenAndReadWithoutBigIntegers(t *testing.T) {
	conformance.Covers(t, "SE-032", "NU-003")
	for _, tt := range []struct {
		name   string
		number func(i int) tenon.Value
	}{
		{"an integer", func(i int) tenon.Value { return n(int64(i)) }},
		{"a fraction", func(i int) tenon.Value { return tenon.NumberFromText(fmt.Sprintf("%d.25", i)) }},
	} {
		var written, read [2]float64
		for k, size := range []int{100, 200} {
			members := make([]tenon.Value, size)
			for i := range members {
				members[i] = tt.number(i)
			}
			list := tenon.ListVal(num, members...)
			data, failure, ok := tenon.Serialize(list)
			if !ok {
				t.Fatalf("%s: %v", tt.name, failure)
			}
			written[k] = testing.AllocsPerRun(50, func() { tenon.Serialize(list) })
			read[k] = testing.AllocsPerRun(50, func() { tenon.Deserialize(data, tenon.Decoders{}) })
		}
		// The output grows by doubling, which is a hundredth of an
		// allocation a number here, or two.
		const slack = 0.05
		if each := (written[1] - written[0]) / 100; each > slack {
			t.Errorf("writing %s costs %.2f allocations, want none (%v for 100, %v for 200)", tt.name, each, written[0], written[1])
		}
		if each := (read[1] - read[0]) / 100; each > 2+slack {
			t.Errorf("reading %s costs %.2f allocations, want at most the 2 of its value (%v for 100, %v for 200)", tt.name, each, read[0], read[1])
		}
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

// deepCount is a deep mark that counts how often it is asked whether it is
// deep. The count is shared, so two of them with one identifier are one mark.
type deepCount struct {
	id    string
	asked *int
}

func (m deepCount) MarkID() string                 { return m.id }
func (deepCount) Propagation() tenon.Propagation   { return tenon.Propagate }
func (deepCount) Redacting() bool                  { return false }
func (m deepCount) Deep() bool                     { *m.asked++; return true }
func (deepCount) MarkPayload() (tenon.Value, bool) { return tenon.Value{}, false }

// TestConformance_SE031_DeepMarksAreDecidedOncePerMarkSet holds the work of
// leaving a container's deep marks off the values it holds to the mark sets
// they carry rather than to the values themselves: the values a container
// holds share the set the deep marks were attached to them through, so the
// encoder asks about that set once. A mark counts what it is asked.
func TestConformance_SE031_DeepMarksAreDecidedOncePerMarkSet(t *testing.T) {
	conformance.Covers(t, "SE-031", "MK-008", "SE-003")
	asked := 0
	marks := make([]tenon.Mark, 4)
	read := tenon.Decoders{Marks: map[string]tenon.MarkDecoder{}}
	for i := range marks {
		m := deepCount{id: fmt.Sprintf("d%d", i), asked: &asked}
		marks[i] = m
		read.Marks[m.id] = func(tenon.Value, bool) (tenon.Mark, []tenon.Diagnostic) { return m, nil }
	}
	// asking returns how often serializing a list of that many numbers under
	// the marks asks a mark whether it is deep.
	asking := func(members int) int {
		held := make([]tenon.Value, members)
		for i := range held {
			held[i] = n(int64(i))
		}
		v := tenon.WithMarks(tenon.ListVal(num, held...), marks...)
		asked = 0
		b, failure, ok := tenon.Serialize(v)
		if !ok {
			t.Fatalf("Serialize(a list of %d members under %d deep marks) failed: %v", members, len(marks), failure)
		}
		count := asked
		if got, _, ok := tenon.Deserialize(b, read); !ok || !tenon.Identical(got, v) {
			t.Errorf("a list of %d members under %d deep marks came back as %v", members, len(marks), got)
		}
		return count
	}
	// v0.1.0 asked twice per member per mark: 40 questions of a list of four
	// and 2,056 of one of 256.
	if small, large := asking(4), asking(256); small != large {
		t.Errorf("serializing a list of 4 members under %d deep marks asks %d times whether a mark is deep, and one of 256 members asks %d: the question is not settled once per mark set",
			len(marks), small, large)
	}
	// Marks at many levels, one to a level: the question is settled once for
	// each mark, not once for each mark in every set that holds it, and a
	// value nested deeply holds the marks of the levels above it in every
	// set within it.
	const levels = 120
	nested := n(1)
	deepRead := tenon.Decoders{Marks: map[string]tenon.MarkDecoder{}}
	for i := range levels {
		m := deepCount{id: fmt.Sprintf("n%03d", i), asked: &asked}
		deepRead.Marks[m.id] = func(tenon.Value, bool) (tenon.Mark, []tenon.Diagnostic) { return m, nil }
		nested = tenon.WithMarks(tenon.ListVal(nested.Type(), nested), m)
	}
	nestedDoc, why, fine := tenon.Serialize(nested)
	if !fine {
		t.Fatalf("Serialize(a value nested %d levels, each marked) failed: %v", levels, why)
	}
	asked = 0
	if got, why, fine := tenon.Deserialize(nestedDoc, deepRead); !fine || !tenon.Identical(got, nested) {
		t.Fatalf("a value nested %d levels came back as %v, %v", levels, got, why)
	}
	// Once for every set that holds a mark is the square of the levels: 7,260
	// questions of 120 of them.
	if asked > 4*levels {
		t.Errorf("decoding a value nested %d levels asked %d times whether a mark is deep, want at most %d", levels, asked, 4*levels)
	}
	// What such a document costs to decode is held to a budget as well, since
	// every value holds the marks of the levels above it. Attaching each
	// level's mark to the values below it as the level was read merged every
	// value's marks once for each level above it: 17 KB for every byte of
	// this document while each merge built a set of the marks, and 3.7 KB
	// once merges took them in order. Giving each value its marks once, when
	// the value is read, takes 0.5 KB, and the budget is 1.
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	if _, _, fine := tenon.Deserialize(nestedDoc, deepRead); !fine {
		t.Fatalf("the nested document did not decode a second time")
	}
	runtime.ReadMemStats(&after)
	if grew, budget := after.TotalAlloc-before.TotalAlloc, uint64(1<<10)*uint64(len(nestedDoc)); grew > budget {
		t.Errorf("decoding a value nested %d levels, %d bytes of document, allocated %d bytes, more than the %d it may",
			levels, len(nestedDoc), grew, budget)
	}

	// What a mark set lists is settled against the marks the container
	// implies, not against the set alone: one value carrying both marks
	// lists the one its container does not imply, which is a different mark
	// under each of these two lists.
	held := tenon.WithMarks(n(1), marks[0], marks[1])
	pair := tenon.TupleVal(
		tenon.WithMarks(tenon.ListVal(num, held), marks[0]),
		tenon.WithMarks(tenon.ListVal(num, held), marks[1]),
	)
	b, failure, ok := tenon.Serialize(pair)
	if !ok {
		t.Fatalf("Serialize(%v) failed: %v", pair, failure)
	}
	if got, _, ok := tenon.Deserialize(b, read); !ok || !tenon.Identical(got, pair) {
		t.Errorf("two lists whose deep marks differ, both holding one value, came back as %v", got)
	}
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

// TestConformance_MK009_AMarkTypeMayDeclareAnEncoding pins the two sides of
// the choice: a mark whose type declares an encoding travels through a
// document, and a value carrying one whose type declares none is refused
// with serialize.unencodable_mark, as a capsule type without one is.
func TestConformance_MK009_AMarkTypeMayDeclareAnEncoding(t *testing.T) {
	conformance.Covers(t, "MK-009")
	wantSerializeFailure(t, "a mark that declares no encoding",
		tenon.WithMarks(n(1), stamp{id: "undeclared"}),
		wantDiag{tenon.CodeSerializeUnencodableMark, "."})
	v := tenon.WithMarks(n(1), note{id: "p", text: "kept"})
	b, failure, ok := tenon.Serialize(v)
	if !ok {
		t.Fatalf("Serialize(%v) failed: %v", v, failure)
	}
	if got, _, ok := tenon.Deserialize(b, decoders); !ok || !tenon.Identical(got, v) {
		t.Errorf("the declared mark came back as %v", got)
	}
}

// TestConformance_SE010_ItemsEncodeByState pins the item shapes: a resolved
// value is [0, type, content], a pending value [1, constraint, nullness],
// and an error value [2, [diagnostics]], each read back as itself.
func TestConformance_SE010_ItemsEncodeByState(t *testing.T) {
	conformance.Covers(t, "SE-010")
	for _, tt := range []struct {
		name string
		v    tenon.Value
		item string
	}{
		{"a resolved value", tenon.Bool(true), "83 00 01 f5"},
		{"a pending value", tenon.Pending(tenon.Any()), "83 01 81 02 00"},
		{"an error value", tenon.ErrorVal(tenon.Diagnostic{Code: "app.x", Message: "m"}), "82 02 81 83 65 6170702e78 61 6d 80"},
	} {
		wantEncoding(t, tt.name, tt.v, tt.item)
		if got, _, ok := tenon.Deserialize(fromHex(t, document+tt.item), decoders); !ok || !tenon.Identical(got, tt.v) {
			t.Errorf("%s came back as %v", tt.name, got)
		}
	}
}

// TestConformance_SE020_TypesEncodeByKind pins a row of each shape in the
// type table: a scalar by its number, a collection as [kind, element], a
// tuple as [7, [types]] and an object as [8, [[name, type]]] in name order,
// through the null value, whose content says nothing more.
func TestConformance_SE020_TypesEncodeByKind(t *testing.T) {
	conformance.Covers(t, "SE-020")
	for _, tt := range []struct {
		name string
		v    tenon.Value
		item string
	}{
		{"a scalar", tenon.NullVal(num), "83 00 02 f6"},
		{"a list of numbers", tenon.NullVal(tenon.List(num)), "83 00 82 04 02 f6"},
		{"a tuple", tenon.NullVal(tenon.Tuple(num, str)), "83 00 82 07 82 02 03 f6"},
		{"an object, attributes in name order", tenon.NullVal(tenon.Object(map[string]tenon.Type{"b": str, "a": num})),
			"83 00 82 08 82 82 6161 02 82 6162 03 f6"},
	} {
		wantEncoding(t, tt.name, tt.v, tt.item)
	}
}

// TestConformance_SE041_MarksEncodeWithAndWithoutAPayload pins the two mark
// encodings: [id] for a mark without a payload and [id, type, content] for
// one with, both read back as marks that equal what was written.
func TestConformance_SE041_MarksEncodeWithAndWithoutAPayload(t *testing.T) {
	conformance.Covers(t, "SE-041")
	one := tenon.WithMarks(n(1), markPlain)
	wantEncoding(t, "a mark without a payload", one, "83 00 02 da74656e02 82 01 81 81 616d")
	two := tenon.WithMarks(n(1), note{id: "p", text: "x"})
	wantEncoding(t, "a mark with a payload", two, "83 00 02 da74656e02 82 01 81 83 6170 03 6178")
	for _, v := range []tenon.Value{one, two} {
		b, _, _ := tenon.Serialize(v)
		if got, _, ok := tenon.Deserialize(b, decoders); !ok || !tenon.Identical(got, v) {
			t.Errorf("%v came back as %v", v, got)
		}
	}
}

// TestConformance_SE040_CapsuleIdentifiersAreText pins where an encoding
// identifier that is not valid UTF-8 is refused: at the declaration, as a
// usage error. The document format writes the identifier as CBOR text, so
// v0.1.0, which accepted it, wrote bytes that Deserialize refused as
// serialize.malformed, and what Serialize wrote did not decode (SE-003).
func TestConformance_SE040_CapsuleIdentifiersAreText(t *testing.T) {
	conformance.Covers(t, "SE-040", "SE-003", "ER-001")
	mustPanicUsage(t, "whose identifier is not valid UTF-8", func() {
		tenon.Capsule("x", tenon.CapsuleOps[celsius]{Encoding: &tenon.CapsuleEncoding[celsius]{
			ID:     "t/\xff",
			Type:   num,
			Encode: func(v *celsius) tenon.Value { return n(v.degrees) },
			Decode: func(tenon.Value) (*celsius, []tenon.Diagnostic) { return &celsius{}, nil },
		}})
	})
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
	// Marks whose payloads do not encode leave a placeholder where an encoding
	// would be, and every such placeholder is the same bytes. They are left
	// out of the SE-041 duplicate check, which is about encodings, so the
	// failure SE-042 already recorded is what comes back. Three of them, not
	// two: the second and later failures are identical diagnostics, which are
	// recorded once, so counting diagnostics would have missed them.
	held := pinned{&celsius{1}}
	wantSerializeFailure(t, "one mark whose payload does not encode", tenon.WithMarks(n(1), held),
		wantDiag{tenon.CodeSerializeUnencodableCapsule, "."})
	two := tenon.Add(tenon.WithMarks(n(1), held), tenon.WithMarks(n(2), pinned{&celsius{2}}))
	wantSerializeFailure(t, "two marks whose payloads do not encode", two,
		wantDiag{tenon.CodeSerializeUnencodableCapsule, "."})
	wantSerializeFailure(t, "three marks whose payloads do not encode",
		tenon.Add(two, tenon.WithMarks(n(3), pinned{&celsius{3}})),
		wantDiag{tenon.CodeSerializeUnencodableCapsule, "."})
	// A payload that does encode still holds the mark to SE-041.
	mustPanicUsage(t, "serialize alike", func() {
		tenon.Serialize(tenon.WithMarks(n(1), note{"p", "v"}, twinNote{"p", "v"}))
	})
	// A mark's payload is not a null, going out or coming in.
	mustPanicUsage(t, "other than a null", func() { tenon.Serialize(tenon.WithMarks(n(1), nullNote{})) })
	wantDecodeFailure(t, "a mark serialized with a null", document+"83 00 01 da74656e02 82 f5 81 83 6170 03 f6", tenon.CodeSerializeMalformed)
}

// unencodable is a capsule type that declares no encoding, so a value of it
// fails to serialize as data under [SE-042].
var unencodable = tenon.Capsule("unencodable", tenon.CapsuleOps[celsius]{})

// pinned is a mark whose payload is a value of that type. Two pinned marks
// holding different degrees are two marks, and neither payload encodes.
type pinned struct{ c *celsius }

func (pinned) MarkID() string                 { return "pinned" }
func (pinned) Propagation() tenon.Propagation { return tenon.Propagate }
func (pinned) Redacting() bool                { return false }
func (m pinned) MarkPayload() (tenon.Value, bool) {
	return tenon.CapsuleVal(unencodable, m.c), true
}

// TestConformance_SE050_AMemberCostsNoPathUnlessItFails holds the encoder and
// the projector to building the path to a member only for a failure they
// record there. Every member has a path, and nearly every member writes: a
// path built for each cost three allocations for an element or an entry, a
// node and the Number or String value its key is, and one for an attribute.
// A member's cost is read as the growth from 100 members to 200, which leaves
// out what the value costs whatever it holds.
func TestConformance_SE050_AMemberCostsNoPathUnlessItFails(t *testing.T) {
	conformance.Covers(t, "SE-050", "SE-061")
	numbers := func(size int) []tenon.Value {
		members := make([]tenon.Value, size)
		for i := range members {
			members[i] = n(int64(i))
		}
		return members
	}
	named := func(prefix string, size int) map[string]tenon.Value {
		entries := make(map[string]tenon.Value, size)
		for i, m := range numbers(size) {
			entries[prefix+strconv.Itoa(1000+i)] = m
		}
		return entries
	}
	for _, tt := range []struct {
		name  string
		build func(size int) tenon.Value
		// What a member may cost, written and projected: a set's member is
		// written apart to be sorted, and a projected number is its text.
		written, projected float64
	}{
		{"a list", func(size int) tenon.Value { return tenon.ListVal(num, numbers(size)...) }, 0, 2},
		{"a set", func(size int) tenon.Value { return tenon.SetVal(num, numbers(size)...) }, 1, 2},
		{"a map", func(size int) tenon.Value { return tenon.MapVal(num, named("k", size)) }, 0, 2},
		{"an object", func(size int) tenon.Value { return obj(named("a", size)) }, 0, 2},
	} {
		var written, projected [2]float64
		for k, size := range []int{100, 200} {
			v := tt.build(size)
			written[k] = testing.AllocsPerRun(50, func() { tenon.Serialize(v) })
			projected[k] = testing.AllocsPerRun(50, func() { tenon.ProjectJSON(v) })
		}
		// The output grows by doubling, which is a hundredth of an
		// allocation a member here, or two.
		const slack = 0.05
		if each := (written[1] - written[0]) / 100; each > tt.written+slack {
			t.Errorf("writing %s costs %.2f allocations a member, want at most %v (%v for 100, %v for 200)",
				tt.name, each, tt.written, written[0], written[1])
		}
		if each := (projected[1] - projected[0]) / 100; each > tt.projected+slack {
			t.Errorf("projecting %s costs %.2f allocations a member, want at most %v (%v for 100, %v for 200)",
				tt.name, each, tt.projected, projected[0], projected[1])
		}
	}
}

// TestConformance_SE050_FailuresUnderOneMemberShareItsPath holds a failure's
// cost to what it adds to the path above it. The path to a failure is made
// only when the failure is recorded, and failures under one member share the
// path to it, as a path built on the way down shared it: made anew for each,
// a hundred failures at the bottom of a hundred levels would cost ten thousand
// steps. Each failure here is an unknown, which does not project, and a value
// of a capsule type that declares no encoding, which does not serialize.
func TestConformance_SE050_FailuresUnderOneMemberShareItsPath(t *testing.T) {
	conformance.Covers(t, "SE-050", "SE-061")
	const levels = 100
	deep := func(bottom tenon.Value) tenon.Value {
		v := bottom
		for range levels {
			v = tenon.ListVal(v.Type(), v)
		}
		return v
	}
	for _, tt := range []struct {
		name   string
		member tenon.Value
		call   func(tenon.Value)
		// What a failure cost when the path was built on the way down: the
		// encoder also writes each diagnostic, path and all, to find one it
		// has recorded already.
		most float64
	}{
		{"projecting", tenon.Unknown(num), func(v tenon.Value) { tenon.ProjectJSON(v) }, 6},
		{"serializing", tenon.CapsuleVal(unencodable, &celsius{}), func(v tenon.Value) { tenon.Serialize(v) }, 13},
	} {
		var made [2]float64
		for k, failures := range []int{100, 200} {
			members := make([]tenon.Value, failures)
			for i := range members {
				members[i] = tt.member
			}
			v := deep(tenon.ListVal(tt.member.Type(), members...))
			made[k] = testing.AllocsPerRun(10, func() { tt.call(v) })
		}
		// A path of its own would cost a failure three allocations a level.
		if each := (made[1] - made[0]) / 100; each > tt.most+0.05 {
			t.Errorf("%s: a failure %d levels down costs %.2f allocations, want at most %v (%v for 100, %v for 200)",
				tt.name, levels, each, tt.most, made[0], made[1])
		}
	}
}

// TestConformance_SE050_FailuresAreLocatedAtEveryKindOfMember pins the path of
// a failure at each kind of member the encoder writes and the projector
// renders, alone and nested: an element of a list, a tuple or a set, the entry
// of a map and an attribute, and for the encoder the members a range records,
// a mark and its payload, and a capsule's payload. Both build a path only for
// a failure they record, and this holds that path to the one they would have
// built on the way down.
func TestConformance_SE050_FailuresAreLocatedAtEveryKindOfMember(t *testing.T) {
	conformance.Covers(t, "SE-050", "SE-042", "SE-061")
	c := func(d int64) tenon.Value { return tenon.CapsuleVal(unencodable, &celsius{d}) }
	const capsule, mark = tenon.CodeSerializeUnencodableCapsule, tenon.CodeSerializeUnencodableMark
	wrapper := tenon.Capsule("wrapper", tenon.CapsuleOps[celsius]{Encoding: &tenon.CapsuleEncoding[celsius]{
		ID: "t/wrapper", Type: tenon.List(unencodable),
		Encode: func(v *celsius) tenon.Value { return tenon.ListVal(unencodable, tenon.CapsuleVal(unencodable, v)) },
		Decode: func(tenon.Value) (*celsius, []tenon.Diagnostic) { return nil, nil },
	}})
	// Where a type names the capsule type, the type fails at the path of the
	// value it is the type of, which for these is the root.
	wantSerializeFailure(t, "a tuple", tenon.TupleVal(n(1), c(1)), wantDiag{capsule, "."}, wantDiag{capsule, ".[1]"})
	wantSerializeFailure(t, "a set", tenon.SetVal(unencodable, c(1), c(2)),
		wantDiag{capsule, "."}, wantDiag{capsule, ".[0]"}, wantDiag{capsule, ".[1]"})
	wantSerializeFailure(t, "a map", tenon.MapVal(unencodable, map[string]tenon.Value{"k": c(1)}),
		wantDiag{capsule, "."}, wantDiag{capsule, `.["k"]`})
	wantSerializeFailure(t, "a map in a list in an object", obj(map[string]tenon.Value{
		"a": tenon.ListVal(tenon.Map(unencodable), tenon.MapVal(unencodable, map[string]tenon.Value{"k": c(1)})),
	}), wantDiag{capsule, "."}, wantDiag{capsule, `.a[0]["k"]`})
	wantSerializeFailure(t, "a member a range records", obj(map[string]tenon.Value{
		"s": tenon.Narrow(tenon.Unknown(tenon.Set(unencodable)), tenon.Members(c(1))),
	}), wantDiag{capsule, "."}, wantDiag{capsule, ".s"})
	wantSerializeFailure(t, "a mark on an element", tenon.ListVal(num, n(1), tenon.WithMarks(n(2), stamp{id: "plain"})),
		wantDiag{mark, ".[1]"})
	// A container's marks are written after its members, and a mark that
	// fails there is located at the container, not at the member last
	// written.
	wantSerializeFailure(t, "a mark on a list", obj(map[string]tenon.Value{
		"a": tenon.WithMarks(tenon.ListVal(num, n(1), n(2)), stamp{id: "plain"}),
	}), wantDiag{mark, ".a"})
	wantSerializeFailure(t, "a mark's payload on an element", tenon.ListVal(num, n(1), tenon.WithMarks(n(2), pinned{&celsius{2}})),
		wantDiag{capsule, ".[1]"})
	wantSerializeFailure(t, "a capsule's payload", obj(map[string]tenon.Value{"x": tenon.CapsuleVal(wrapper, &celsius{1})}),
		wantDiag{capsule, ".x"}, wantDiag{capsule, ".x[0]"})

	unknown := tenon.Unknown(num)
	wantProjectionFailure(t, "a tuple", tenon.TupleVal(n(1), unknown), wantDiag{tenon.CodeSerializeNotKnown, ".[1]"})
	wantProjectionFailure(t, "a set", tenon.SetVal(num, n(1), unknown), wantDiag{tenon.CodeSerializeNotKnown, ".[1]"})
	wantProjectionFailure(t, "a map in a list in an object", obj(map[string]tenon.Value{
		"a": tenon.ListVal(tenon.Map(num), tenon.MapVal(num, map[string]tenon.Value{"k": unknown})),
	}), wantDiag{tenon.CodeSerializeNotKnown, `.a[0]["k"]`})
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

// deepNote is a deep mark serialized with a string payload.
type deepNote struct{ id, text string }

func (m deepNote) MarkID() string                   { return m.id }
func (deepNote) Propagation() tenon.Propagation     { return tenon.Propagate }
func (deepNote) Redacting() bool                    { return false }
func (deepNote) Deep() bool                         { return true }
func (m deepNote) MarkPayload() (tenon.Value, bool) { return tenon.String(m.text), true }

// BenchmarkDeepMarkEncoding measures serializing and deserializing a list of a
// thousand numbers under d distinct deep marks, at a count and four times it:
// the growth from one to the other is the reading, not the wall clock.
func BenchmarkDeepMarkEncoding(b *testing.B) {
	for _, d := range []int{100, 400} {
		marks := make([]tenon.Mark, d)
		read := tenon.Decoders{Marks: map[string]tenon.MarkDecoder{}}
		for i := range marks {
			m := deepNote{id: fmt.Sprintf("m%04d", i), text: fmt.Sprintf("payload %d", i)}
			marks[i] = m
			read.Marks[m.id] = func(tenon.Value, bool) (tenon.Mark, []tenon.Diagnostic) { return m, nil }
		}
		members := make([]tenon.Value, 1000)
		for i := range members {
			members[i] = tenon.NumberFromInt(int64(i))
		}
		v := tenon.WithMarks(tenon.ListVal(tenon.NumberType(), members...), marks...)
		encoded, failure, ok := tenon.Serialize(v)
		if !ok {
			b.Fatalf("Serialize(a list under %d deep marks) failed: %v", d, failure)
		}
		b.Run(fmt.Sprintf("serialize/%d", d), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, _, ok := tenon.Serialize(v); !ok {
					b.Fatal("the value did not serialize")
				}
			}
		})
		b.Run(fmt.Sprintf("deserialize/%d", d), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, _, ok := tenon.Deserialize(encoded, read); !ok {
					b.Fatal("the document did not decode")
				}
			}
		})
	}
}

// BenchmarkSerializeFailures measures serializing a list of capsule values of
// a type that declares no encoding, each a diagnostic at its own path, at a
// size and four times it: the growth from one to the other is the reading.
func BenchmarkSerializeFailures(b *testing.B) {
	opaque := tenon.Capsule("opaque", tenon.CapsuleOps[celsius]{})
	for _, size := range []int{5000, 20000} {
		members := make([]tenon.Value, size)
		for i := range members {
			members[i] = tenon.CapsuleVal(opaque, &celsius{int64(i)})
		}
		v := tenon.ListVal(opaque, members...)
		b.Run(fmt.Sprintf("%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, _, ok := tenon.Serialize(v); ok {
					b.Fatal("the list serialized")
				}
			}
		})
	}
}

// TestConformance_SE050_FailuresFollowTheEncoding pins the order of an
// encoding's failures where the value itself and its members each have some:
// the diagnostics come in the order the encoding meets what fails, a value's
// type, and the types within it, before its content, and within the content
// each member, its content then its marks, before the marks of the value
// holding them, which the encoding writes last.
func TestConformance_SE050_FailuresFollowTheEncoding(t *testing.T) {
	conformance.Covers(t, "SE-050")
	opaque := tenon.Capsule("opaque_in_se050", tenon.CapsuleOps[celsius]{})
	member := tenon.WithMarks(tenon.CapsuleVal(opaque, &celsius{1}), stamp{id: "inner"})
	list := tenon.WithMarks(tenon.ListVal(opaque, member), stamp{id: "outer"})
	wantSerializeFailure(t, "a type, a member and two marks that do not encode", list,
		wantDiag{tenon.CodeSerializeUnencodableCapsule, "."},
		wantDiag{tenon.CodeSerializeUnencodableCapsule, ".[0]"},
		wantDiag{tenon.CodeSerializeUnencodableMark, ".[0]"},
		wantDiag{tenon.CodeSerializeUnencodableMark, "."})
}
