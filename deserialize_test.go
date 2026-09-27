package tenon_test

import (
	"bytes"
	"fmt"
	"math/big"
	"math/rand"
	"runtime"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/cbor"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/internal/conformance/values"
)

// The marks the round-trip generator attaches, and the decoders that read
// them back.
var (
	markPlain    = signal{id: "m"}
	markDeep     = signal{id: "d", deep: true}
	markIsolated = signal{id: "i", policy: tenon.Isolate}
)

// decoders reads everything the round-trip generator writes.
var decoders = tenon.Decoders{
	Capsules: []tenon.Type{degrees.Type()},
	Marks: map[string]tenon.MarkDecoder{
		"m": func(tenon.Value, bool) (tenon.Mark, []tenon.Diagnostic) { return markPlain, nil },
		"d": func(tenon.Value, bool) (tenon.Mark, []tenon.Diagnostic) { return markDeep, nil },
		"i": func(tenon.Value, bool) (tenon.Mark, []tenon.Diagnostic) { return markIsolated, nil },
		"p": func(payload tenon.Value, has bool) (tenon.Mark, []tenon.Diagnostic) {
			if !has || payload.Type() != tenon.StringType() {
				return nil, []tenon.Diagnostic{{Code: "app.bad_note", Message: "a note needs text"}}
			}
			return note{"p", payload.AsString()}, nil
		},
	},
}

// generator makes random values of every state, kind and markedness that
// serialization can hold.
type generator struct{ r *rand.Rand }

func (g generator) typ(depth int) tenon.Type {
	if depth == 0 || g.r.Intn(3) == 0 {
		return []tenon.Type{boo, num, str, degrees.Type()}[g.r.Intn(4)]
	}
	switch g.r.Intn(5) {
	case 0:
		return tenon.ListType(g.typ(depth - 1))
	case 1:
		return tenon.SetType(g.typ(depth - 1))
	case 2:
		return tenon.MapType(g.typ(depth - 1))
	case 3:
		elems := make([]tenon.Type, g.r.Intn(3))
		for i := range elems {
			elems[i] = g.typ(depth - 1)
		}
		return tenon.TupleType(elems...)
	}
	attrs := map[string]tenon.Type{}
	for _, name := range []string{"a", "b", "c\U00000301"} {
		if g.r.Intn(2) == 0 {
			attrs[name] = g.typ(depth - 1)
		}
	}
	return tenon.ObjectType(attrs)
}

func (g generator) number() tenon.Value {
	texts := []string{"0", "1", "-1", "23", "24", "-25", "1000", "1.5", "-0.001", "1e30", "-7e-40",
		"18446744073709551615", "18446744073709551616", "-18446744073709551617", "3.14159265358979323846264338327950288"}
	return tenon.NumberFromText(texts[g.r.Intn(len(texts))])
}

// value returns a value of type t: null, unknown or known, marked or not,
// with members of its own where it is a container.
func (g generator) value(t tenon.Type, depth int, inSet bool) tenon.Value {
	var v tenon.Value
	switch g.r.Intn(10) {
	case 0:
		v = tenon.Null(t)
	case 1, 2:
		v = g.unknown(t)
	default:
		v = g.known(t, depth)
	}
	if !inSet && g.r.Intn(5) == 0 {
		pool := []tenon.Mark{markPlain, markDeep, markIsolated, note{"p", "x"}, note{"p", "y"}}
		v = tenon.WithMarks(v, pool[g.r.Intn(len(pool))], pool[g.r.Intn(len(pool))])
	}
	if inSet {
		v, _ = tenon.UnmarkDeep(v)
	}
	return v
}

func (g generator) unknown(t tenon.Type) tenon.Value {
	var ns []tenon.Narrowing
	if g.r.Intn(2) == 0 {
		ns = append(ns, tenon.NotNull())
	}
	switch t.Kind() {
	case tenon.KindNumber:
		if g.r.Intn(2) == 0 {
			ns = append(ns, tenon.NumberMin(g.number(), g.r.Intn(2) == 0))
		}
		if g.r.Intn(2) == 0 {
			ns = append(ns, tenon.NumberMax(g.number(), g.r.Intn(2) == 0))
		}
	case tenon.KindString:
		if g.r.Intn(2) == 0 {
			ns = append(ns, tenon.StringPrefix([]string{"ab", "cafe", "x-"}[g.r.Intn(3)]))
		}
	case tenon.KindList, tenon.KindSet, tenon.KindMap:
		if g.r.Intn(2) == 0 {
			ns = append(ns, tenon.LengthMin(int64(g.r.Intn(3))))
		}
		if g.r.Intn(2) == 0 {
			ns = append(ns, tenon.LengthMax(int64(g.r.Intn(4))))
		}
		if t.Kind() == tenon.KindSet && g.r.Intn(2) == 0 {
			ns = append(ns, tenon.Members(g.value(t.ElementType(), 0, true)))
		}
	}
	if v := tenon.Narrow(tenon.Unknown(t), ns...); !v.IsError() {
		return v
	}
	return tenon.Unknown(t)
}

func (g generator) known(t tenon.Type, depth int) tenon.Value {
	switch t.Kind() {
	case tenon.KindBool:
		return tenon.Bool(g.r.Intn(2) == 0)
	case tenon.KindNumber:
		return g.number()
	case tenon.KindString:
		return s([]string{"", "a", "e\U00000301", "\U000000e9", "hello, world", "\x00\U0001F600"}[g.r.Intn(6)])
	case tenon.KindCapsule:
		return degrees.Value(&celsius{int64(g.r.Intn(5))})
	case tenon.KindList, tenon.KindSet:
		members := make([]tenon.Value, g.r.Intn(4))
		for i := range members {
			members[i] = g.value(t.ElementType(), depth-1, t.Kind() == tenon.KindSet)
		}
		if t.Kind() == tenon.KindSet {
			return tenon.Set(t.ElementType(), members...)
		}
		return tenon.List(t.ElementType(), members...)
	case tenon.KindMap:
		entries := map[string]tenon.Value{}
		for _, key := range []string{"", "k", "j\U00000301"} {
			if g.r.Intn(2) == 0 {
				entries[key] = g.value(t.ElementType(), depth-1, false)
			}
		}
		return tenon.Map(t.ElementType(), entries)
	case tenon.KindTuple:
		elems := make([]tenon.Value, t.TupleLength())
		for i := range elems {
			elems[i] = g.value(t.TupleElementType(i), depth-1, false)
		}
		return tenon.Tuple(elems...)
	}
	attrs := map[string]tenon.Value{}
	for _, name := range t.AttributeNames() {
		attrs[name] = g.value(t.AttributeType(name), depth-1, false)
	}
	return tenon.Object(attrs)
}

// top returns a value to serialize: now and then a pending or an error value,
// and otherwise a resolved one.
func (g generator) top() tenon.Value {
	var v tenon.Value
	switch g.r.Intn(12) {
	case 0:
		v = tenon.Pending(randomConstraint(g.r, 2, degrees.Type()))
		v = tenon.Narrow(v, []tenon.Narrowing{tenon.NotNull(), tenon.NullOnly(), tenon.NotNull()}[g.r.Intn(3)])
	case 1:
		p := tenon.Path{}.Attribute("a").Index(g.number()).Index(s("k"))
		v = tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: "it failed \U0001F600", Path: p},
			tenon.Diagnostic{Code: "app.other", Message: "again"})
	default:
		return g.value(g.typ(3), 3, false)
	}
	if g.r.Intn(3) == 0 {
		v = tenon.WithMarks(v, markPlain, note{"p", "z"})
	}
	return v
}

func TestConformance_SE003_RoundTrip(t *testing.T) {
	conformance.Covers(t, "SE-003", "SE-002", "SE-043")
	g := generator{rand.New(rand.NewSource(20260916))}
	var emitted bytes.Buffer
	defer func() { conformance.Emit(t, "encodings.txt", emitted.Bytes()) }()
	for range conformance.Iterations(t, 10000) {
		v := g.top()
		b, failure, ok := trySerialize(v)
		if !ok {
			t.Fatalf("Serialize(%v) failed: %v", v, failure)
		}
		fmt.Fprintf(&emitted, "%x\n", b)
		got, failure, ok := tryDeserialize(b, decoders)
		switch {
		case !ok:
			t.Fatalf("Deserialize(Serialize(%v)) failed: %v\n%x", v, failure, b)
		case !tenon.Identical(got, v):
			t.Fatalf("Deserialize(Serialize(%v)) = %v", v, got)
		}
	}
	// The generator's own values that serialize come back too.
	for _, v := range values.All() {
		b, _, ok := trySerialize(v)
		if !ok {
			continue
		}
		if got, failure, ok := tryDeserialize(b, decoders); !ok || !tenon.Identical(got, v) {
			t.Errorf("Deserialize(Serialize(%v)) = %v, %v", v, got, failure)
		}
	}
}

// wantDecodeFailure fails t unless the document holding item fails to decode
// with a diagnostic of code.
func wantDecodeFailure(t *testing.T, what, input string, code tenon.Code) {
	t.Helper()
	got, failure, ok := tryDeserialize(fromHex(t, input), decoders)
	if ok {
		t.Errorf("%s: decoded %v", what, got)
		return
	}
	if d := failure.Diagnostics(); len(d) != 1 || d[0].Code != code {
		t.Errorf("%s: %v, want code %s", what, failure, code)
	}
}

func TestConformance_SE002_OnlyTheEncodingDecodes(t *testing.T) {
	conformance.Covers(t, "SE-002", "SE-051")
	for _, tt := range []struct{ name, item string }{
		{"an integer in a longer form", "83 00 02 18 01"},
		{"set members out of order", "83 00 82 05 02 82 20 01"},
		{"a set member twice", "83 00 82 05 02 82 01 01"},
		{"map keys out of order", "83 00 82 06 02 82 82 6162 02 82 6161 01"},
		{"a string not in normal form", "83 00 03 63 65cc81"},
		{"an integer as a decimal fraction", "83 00 02 c4 82 00 01"},
		{"a coefficient that is a multiple of ten", "83 00 02 c4 82 20 0a"},
		{"a bignum that fits an integer", "83 00 02 c4 82 01 c2 41 01"},
		{"a least length of zero", "83 00 03 da74656e01 a1 04 00"},
		{"range keys out of order", "83 00 02 da74656e01 a2 01 82 01 f5 00 f5"},
		{"a range that is one value", "83 00 82 04 02 da74656e01 a2 00 f5 05 00"},
		{"a set range bounded by what its element type bounds", "83 00 82 05 01 da74656e01 a1 05 03"},
		{"a set of every bool holding an unknown bool", "83 00 82 05 01 84 da74656e01 a0 f4 f5 f6"},
		{"a deep mark listed on a member", "83 00 82 04 02 da74656e02 82 81 da74656e02 82 01 81 81 6164 81 81 6164"},
		{"marks out of order", "83 00 01 da74656e02 82 f5 82 83 6170 03 6176 81 616d"},
		{"an indefinite-length array", "83 00 82 04 02 9f 01 ff"},
		{"object attributes out of order", "83 00 82 08 82 82 6162 02 82 6161 02 82 01 02"},
		// Crossed bounds on a range that holds null leave null alone, which
		// is the null value and encodes as one (UN-004).
		{"a range only null lies in", "83 00 02 da74656e01 a2 01 82 05 f5 02 82 01 f5"},
		// 2^64, an integer beyond CBOR's, is the decimal fraction [0, 2^64],
		// never a bare bignum; and 10^20's mantissa as a bignum is a multiple
		// of ten, which the canonical [20, 1] is not.
		{"a bignum standing for a number", "83 00 02 c2 49 010000000000000000"},
		{"a decimal fraction whose bignum mantissa is a multiple of ten", "83 00 02 c4 82 00 c2 49 056bc75e2d63100000"},
	} {
		wantDecodeFailure(t, tt.name, document+tt.item, tenon.CodeSerializeNotCanonical)
	}
	for _, tt := range []struct{ name, input string }{
		{"no input", ""},
		{"another tag", "d9d9f7 82 01 83 00 01 f5"},
		{"a byte after the document", document + "83 00 01 f5 00"},
		{"an item of no kind", document + "83 03 01 f5"},
		{"a tuple of the wrong length", document + "83 00 82 07 81 01 82 f5 f5"},
		{"a float", document + "83 00 02 f9 0000"},
		{"a key twice", document + "83 00 82 06 02 82 82 6161 01 82 6161 02"},
		{"an empty attribute name", document + "83 00 82 08 81 82 60 01 81 f5"},
		{"a range no value lies in", document + "83 00 02 da74656e01 a3 00 f5 01 82 05 f5 02 82 01 f5"},
		{"a number outside the window", document + "83 00 02 c4 82 1a 000f4240 01"},
		{"a prefix on a number", document + "83 00 02 da74656e01 a1 03 6161"},
		{"a number for a bool", document + "83 00 01 01"},
		{"an error with no diagnostics", document + "82 02 80"},
		{"a code that is not one", document + "82 02 81 83 62 7878 61 78 80"},
		{"a truncated document", document + "83 00 82 04"},
		{"a marked resolved item", document + "da74656e02 82 83 00 01 f5 81 81 616d"},
	} {
		wantDecodeFailure(t, tt.name, tt.input, tenon.CodeSerializeMalformed)
	}
	wantDecodeFailure(t, "version 2", "da74656e00 82 02 83 00 01 f5", tenon.CodeSerializeUnsupportedVersion)
}

// TestConformance_SE051_TheCodesOfSerialization produces each code the
// section's table names, one row a code, the two the JSON projection mints
// included.
func TestConformance_SE051_TheCodesOfSerialization(t *testing.T) {
	conformance.Covers(t, "SE-051")
	wantDecodeFailure(t, "serialize.malformed", document+"83 03 01 f5", tenon.CodeSerializeMalformed)
	wantDecodeFailure(t, "serialize.not_canonical", document+"83 00 02 18 01", tenon.CodeSerializeNotCanonical)
	wantDecodeFailure(t, "serialize.unsupported_version", "da74656e00 82 02 83 00 01 f5", tenon.CodeSerializeUnsupportedVersion)
	wantDecodeFailure(t, "serialize.too_large", document+"83 00 "+strings.Repeat("82 04 ", 600)+"02 f6", tenon.CodeSerializeTooLarge)
	wantDecodeFailure(t, "serialize.unknown_capsule", document+"83 00 82 09 63 782f79 f6", tenon.CodeSerializeUnknownCapsule)
	wantDecodeFailure(t, "serialize.unknown_mark", document+"83 00 01 da74656e02 82 f5 81 81 617a", tenon.CodeSerializeUnknownMark)
	opaque := tenon.NewCapsule("opaque", tenon.CapsuleOps[int]{})
	held := 1
	wantSerializeFailure(t, "serialize.unencodable_capsule", opaque.Value(&held),
		wantDiag{tenon.CodeSerializeUnencodableCapsule, "."})
	wantSerializeFailure(t, "serialize.unencodable_mark", tenon.WithMarks(tenon.NumberFromInt(1), stamp{id: "x"}),
		wantDiag{tenon.CodeSerializeUnencodableMark, "."})
	if _, failure, ok := tryProjectJSON(tenon.Unknown(tenon.NumberType())); ok || failure.Diagnostics()[0].Code != tenon.CodeSerializeNotKnown {
		t.Errorf("projecting an unknown gave %v, want serialize.not_known", failure)
	}
	redacted := tenon.WithMarks(tenon.NumberFromInt(1), stamp{id: "secret", redact: true})
	if _, failure, ok := tryProjectJSON(redacted); ok || failure.Diagnostics()[0].Code != tenon.CodeSerializeRedacted {
		t.Errorf("projecting a redacted value gave %v, want serialize.redacted", failure)
	}
}

// TestConformance_SE002_TwoByteSimpleValues holds documents spelling false,
// true or null in two bytes to serialize.malformed: RFC 8949 3.3 says f8
// followed by a byte below 32 is not well-formed, so such input is not CBOR,
// not a longer spelling of the value. v0.1.0 read f8 16 as null and its
// second byte again as the next item, and reported what it then made of the
// input as serialize.not_canonical.
func TestConformance_SE002_TwoByteSimpleValues(t *testing.T) {
	conformance.Covers(t, "SE-002", "SE-051")
	for _, tt := range []struct{ name, input string }{
		{"false in two bytes", document + "83 00 01 f8 14"},
		{"true in two bytes", document + "83 00 01 f8 15"},
		{"null in two bytes", document + "83 00 01 f8 16"},
		{"null in two bytes in a list of numbers", document + "83 00 82 04 02 82 f8 16 01"},
	} {
		wantDecodeFailure(t, tt.name, tt.input, tenon.CodeSerializeMalformed)
	}
}

// TestConformance_SE031_AMarkListedAgainIsRefusedWhereItFirstIs holds the
// refusal of a document that lists a deep mark on a value whose container
// carries it already to name the first value listing it again: the byte where
// the input departs from the encoding of the value it describes. A list lists
// the deep mark of the list holding it again, beside a plain mark, and a
// number two levels within it lists the mark again as well, which the input
// holds ahead of the list's marks. The number's listing repeats a mark it
// carries already, since a deep mark reaches every value within the value
// carrying it (MK-008), past a list that carries the mark itself.
func TestConformance_SE031_AMarkListedAgainIsRefusedWhereItFirstIs(t *testing.T) {
	conformance.Covers(t, "SE-031", "SE-002", "MK-008")
	// A list of lists of lists of numbers carrying the deep mark, holding a
	// list carrying it again and the plain mark, which holds a list holding
	// the number, which carries it again too.
	before := document + "83 00 82 04 82 04 82 04 02 da74656e02 82 81 da74656e02 82 81 81"
	input := before + "da74656e02 82 01 81 81 6164 82 81 6164 81 616d 81 81 6164"
	_, failure, ok := tryDeserialize(fromHex(t, input), decoders)
	if ok {
		t.Fatal("a document listing a deep mark again decoded")
	}
	want := fmt.Sprintf("differs from byte %d", len(fromHex(t, before)))
	if d := failure.Diagnostics(); len(d) != 1 || d[0].Code != tenon.CodeSerializeNotCanonical || !strings.Contains(d[0].Message, want) {
		t.Errorf("%v, want %s saying it %s", failure, tenon.CodeSerializeNotCanonical, want)
	}
}

func TestConformance_SE043_DecodersAreSupplied(t *testing.T) {
	conformance.Covers(t, "SE-043", "SE-051")
	wantDecodeFailure(t, "an unknown capsule", document+"83 00 82 09 63 782f79 f6", tenon.CodeSerializeUnknownCapsule)
	wantDecodeFailure(t, "an unknown mark", document+"83 00 01 da74656e02 82 f5 81 81 617a", tenon.CodeSerializeUnknownMark)
	// A decoder that refuses gives its own diagnostics.
	wantDecodeFailure(t, "a note without text", document+"83 00 01 da74656e02 82 f5 81 81 6170", "app.bad_note")
	refusing := tenon.NewCapsule("refusing", tenon.CapsuleOps[celsius]{Equal: celsiusEqual, Hash: celsiusHash, Encoding: &tenon.CapsuleEncoding[celsius]{
		ID: "t/refusing", Type: num,
		Encode: func(v *celsius) tenon.Value { return n(v.degrees) },
		Decode: func(tenon.Value) (*celsius, []tenon.Diagnostic) {
			return nil, []tenon.Diagnostic{{Code: "app.refused", Message: "no"}}
		},
	}})
	b, _, _ := trySerialize(refusing.Value(&celsius{1}))
	if _, failure, ok := tryDeserialize(b, tenon.Decoders{Capsules: []tenon.Type{refusing.Type()}}); ok || failure.Diagnostics()[0].Code != "app.refused" {
		t.Errorf("a refusing capsule decoder gave %v", failure)
	}
	// What the caller supplies must itself be sound.
	opaque := tenon.NewCapsule("opaque", tenon.CapsuleOps[celsius]{}).Type()
	mustPanicUsage(t, "declares no encoding", func() { tryDeserialize(nil, tenon.Decoders{Capsules: []tenon.Type{opaque}}) })
	mustPanicUsage(t, "returned the mark", func() {
		tryDeserialize(fromHex(t, document+"83 00 01 da74656e02 82 f5 81 81 616d"), tenon.Decoders{Marks: map[string]tenon.MarkDecoder{
			"m": func(tenon.Value, bool) (tenon.Mark, []tenon.Diagnostic) { return markDeep, nil },
		}})
	})
}

// counting is a capsule type that encodes as a number and counts how often
// two of its values are compared, which only two known values of it ever are.
var (
	countingCompared int
	counting         = tenon.NewCapsule("counting", tenon.CapsuleOps[int64]{
		Equal: func(a, b *int64) bool { countingCompared++; return *a == *b },
		Hash:  func(v *int64) uint64 { return uint64(*v) },
		Encoding: &tenon.CapsuleEncoding[int64]{
			ID:     "t/counting",
			Type:   tenon.NumberType(),
			Encode: func(v *int64) tenon.Value { return tenon.NumberFromInt(*v) },
			Decode: func(v tenon.Value) (*int64, []tenon.Diagnostic) {
				i, _ := v.AsInt64()
				return &i, nil
			},
		},
	})
)

// TestConformance_SE005_DecodingWorkIsBounded holds decoding to the promise
// that its work grows no faster than n log n in the length of its input, and
// that nesting is the only thing it refuses for size. Three shapes are
// quadratic done the obvious way, and each is decoded here at a size where
// that took seconds: a run of combining marks out of canonical order, a set
// whose members are not known, and a value carrying thousands of marks. The
// set is held to a count rather than a time: its members hold values of a
// capsule type that counts its comparisons, which comparing every pair of
// members would make millions of.
func TestConformance_SE005_DecodingWorkIsBounded(t *testing.T) {
	conformance.Covers(t, "SE-005", "SE-051", "ST-002", "EQ-041")
	// 512 levels decode and 513 do not: the item, 510 list types, a number.
	levels := func(k int) string { return document + "83 00 " + strings.Repeat("82 04 ", k) + "02 f6" }
	if _, failure, ok := tryDeserialize(fromHex(t, levels(510)), decoders); !ok {
		t.Errorf("512 levels were refused: %v", failure)
	}
	wantDecodeFailure(t, "513 levels", levels(511), tenon.CodeSerializeTooLarge)
	// A mark's payload nests below the content the mark is on: its type is
	// two levels below the item, so 509 list types around a number reach 512
	// and 510 reach 513.
	payloads := tenon.Decoders{Marks: map[string]tenon.MarkDecoder{
		"p": func(p tenon.Value, _ bool) (tenon.Mark, []tenon.Diagnostic) { return holding{&p}, nil },
	}}
	payload := func(k int) string {
		return document + "830002da74656e02820181836170" + strings.Repeat("82 04 ", k) + "02 80"
	}
	if _, failure, ok := tryDeserialize(fromHex(t, payload(509)), payloads); !ok {
		t.Errorf("a payload reaching 512 levels was refused: %v", failure)
	}
	if _, failure, _ := tryDeserialize(fromHex(t, payload(510)), payloads); !failure.IsError() || failure.Diagnostics()[0].Code != tenon.CodeSerializeTooLarge {
		t.Errorf("a payload reaching 513 levels gave %v, want %s", failure, tenon.CodeSerializeTooLarge)
	}

	// A run of 40,000 marks out of canonical order, 80 KB, is refused as not
	// canonical, which took 6.2 seconds when the run was sorted by insertion.
	var run strings.Builder
	run.WriteString("a")
	for i := range 40_000 {
		run.WriteString([]string{"\U00000316", "\U00000301"}[i%2])
	}
	text := run.String()
	doc := fromHex(t, document+"83 00 03")
	doc = append(doc, 0x7a, byte(len(text)>>24), byte(len(text)>>16), byte(len(text)>>8), byte(len(text)))
	doc = append(doc, text...)
	if _, failure, ok := tryDeserialize(doc, decoders); ok || failure.Diagnostics()[0].Code != tenon.CodeSerializeNotCanonical {
		t.Errorf("the run out of order decoded as %v, %v", ok, failure)
	}

	// A set of 2,000 members that are not known, each a tuple holding a
	// counting value and an unknown number: no two are compared.
	num := tenon.NumberType()
	elem := tenon.TupleType(counting.Type(), num)
	members := make([]tenon.Value, 2000)
	for i := range members {
		v := int64(i)
		members[i] = tenon.Tuple(counting.Value(&v), tenon.Unknown(num))
	}
	withCounting := tenon.Decoders{Capsules: []tenon.Type{counting.Type()}}
	countingCompared = 0
	set := tenon.Set(elem, members...)
	b, failure, ok := trySerialize(set)
	if !ok {
		t.Fatalf("Serialize(the set) failed: %v", failure)
	}
	got, failure, ok := tryDeserialize(b, withCounting)
	compared := countingCompared
	if !ok || !tenon.Identical(got, set) {
		t.Fatalf("the set came back as %v, %v", got, failure)
	}
	if compared > 4*len(members) {
		t.Errorf("building, encoding and decoding a set of %d members that are not known compared them %d times, where every pair is %d",
			len(members), compared, len(members)*(len(members)-1)/2)
	}
	// A range listing the same 2,000 values, and a range listing them as all
	// its members, null excluded. The least length a listing of values that
	// are not known implies is one, and such a listing is left a range, so
	// neither compares every pair to count the values provably distinct.
	for _, listing := range []tenon.Value{
		tenon.Narrow(tenon.Unknown(tenon.SetType(elem)), tenon.Members(members...)),
		tenon.Narrow(tenon.Unknown(tenon.SetType(elem)), tenon.NotNull(), tenon.LengthMax(int64(len(members))), tenon.Members(members...)),
	} {
		b, failure, ok := trySerialize(listing)
		if !ok {
			t.Fatalf("Serialize(the listing) failed: %v", failure)
		}
		countingCompared = 0
		got, failure, ok := tryDeserialize(b, withCounting)
		compared := countingCompared
		if !ok || !tenon.Identical(got, listing) {
			t.Fatalf("the listing came back as %v, %v", got, failure)
		}
		if compared > 4*len(members) {
			t.Errorf("decoding %v compared its %d listed values %d times, where every pair is %d",
				listing.Type(), len(members), compared, len(members)*(len(members)-1)/2)
		}
	}
	// A range listing two sets that each hold 1,000 of those members, which a
	// listing tells apart by whether they are one: neither the least length
	// of each set nor the pairs of their members are counted to find out.
	half := len(members) / 2
	sets := tenon.Narrow(tenon.Unknown(tenon.SetType(tenon.SetType(elem))),
		tenon.Members(tenon.Set(elem, members[:half]...), tenon.Set(elem, members[half:]...)))
	b, failure, ok = trySerialize(sets)
	if !ok {
		t.Fatalf("Serialize(the listing of two sets) failed: %v", failure)
	}
	countingCompared = 0
	got, failure, ok = tryDeserialize(b, withCounting)
	compared = countingCompared
	if !ok || !tenon.Identical(got, sets) {
		t.Fatalf("the listing of two sets came back as %v, %v", got, failure)
	}
	if compared > 4*len(members) {
		t.Errorf("decoding a listing of two sets of %d members that are not known compared them %d times, where every pair of one is %d",
			half, compared, half*(half-1)/2)
	}

	// A value carrying 4,000 marks decodes, which took 36 ms and grew with
	// the square of the marks.
	marks := make([]tenon.Mark, 4000)
	read := tenon.Decoders{Marks: map[string]tenon.MarkDecoder{}}
	for i := range marks {
		m := note{id: fmt.Sprintf("m%05d", i), text: "x"}
		marks[i] = m
		read.Marks[m.id] = func(tenon.Value, bool) (tenon.Mark, []tenon.Diagnostic) { return m, nil }
	}
	marked := tenon.WithMarks(tenon.NumberFromInt(1), marks...)
	b, _, _ = trySerialize(marked)
	if got, failure, ok := tryDeserialize(b, read); !ok || !tenon.Identical(got, marked) {
		t.Errorf("the value with 4,000 marks came back as %v, %v", got, failure)
	}

	// 4,000 marks sharing one identifier, told apart by their payloads, which
	// one mark decoder reads. Each was looked for among all those sharing its
	// identifier, asking each for it, which is eight million times in all;
	// sorting them asks about twenty-six times a mark.
	shared := make([]tenon.Mark, 4000)
	for i := range shared {
		shared[i] = tallied{fmt.Sprintf("t%05d", i)}
	}
	one := tenon.WithMarks(tenon.NumberFromInt(1), shared...)
	b, _, _ = trySerialize(one)
	talliedIDs = 0
	got, failure, ok = tryDeserialize(b, tallyDecoders)
	if asked := talliedIDs; asked > 64*len(shared) {
		t.Errorf("decoding %d marks that share an identifier asked for it %d times, more than 64 a mark", len(shared), asked)
	}
	if !ok || !tenon.Identical(got, one) {
		t.Errorf("the value with %d marks sharing an identifier came back as %v, %v", len(shared), got, failure)
	}
}

// TestConformance_SE005_NestedSetsCostWhatTheyHold holds a set nested in sets
// to be hashed, encoded and decoded once however many sets hold it. A capsule
// type counts how often its values are hashed: a list of 1,000 of them under
// 200 sets is hashed 1,000 times to build and 1,000 to decode, where every
// set rehashed all it held, 200,000 times each. And a list 400 sets down
// encodes in under twice what one 100 sets down does, where every set copied
// all it held into the next: 2.5 MB for a 7 KB document.
func TestConformance_SE005_NestedSetsCostWhatTheyHold(t *testing.T) {
	conformance.Covers(t, "SE-005", "EQ-030")
	hashed := 0
	counted := tenon.NewCapsule("counted", tenon.CapsuleOps[int64]{
		Equal: func(a, b *int64) bool { return *a == *b },
		Hash:  func(v *int64) uint64 { hashed++; return uint64(*v) },
		Encoding: &tenon.CapsuleEncoding[int64]{
			ID:     "tenon.test/counted",
			Type:   tenon.NumberType(),
			Encode: func(v *int64) tenon.Value { return tenon.NumberFromInt(*v) },
			Decode: func(v tenon.Value) (*int64, []tenon.Diagnostic) {
				x, _ := v.AsInt64()
				return &x, nil
			},
		},
	})
	const size, depth = 1000, 200
	members := make([]tenon.Value, size)
	for i := range members {
		x := int64(i)
		members[i] = counted.Value(&x)
	}
	nest := func(v tenon.Value, levels int) tenon.Value {
		for range levels {
			v = tenon.Set(v.Type(), v)
		}
		return v
	}
	hashed = 0
	v := nest(tenon.List(counted.Type(), members...), depth)
	if hashed > 2*size {
		t.Errorf("building %d sets around %d values hashed them %d times", depth, size, hashed)
	}
	doc, failure, ok := trySerialize(v)
	if !ok {
		t.Fatalf("Serialize failed: %v", failure)
	}
	hashed = 0
	back, failure, ok := tryDeserialize(doc, tenon.Decoders{Capsules: []tenon.Type{counted.Type()}})
	if !ok || !tenon.Identical(back, v) {
		t.Fatalf("the nested sets came back as %.80s (%v)", back, failure)
	}
	if hashed > 2*size {
		t.Errorf("decoding %d sets around %d values hashed them %d times", depth, size, hashed)
	}

	num := tenon.NumberType()
	leaves := make([]tenon.Value, 20000)
	for i := range leaves {
		leaves[i] = tenon.NumberFromInt(int64(i))
	}
	list := tenon.List(num, leaves...)
	var sizes [2]uint64
	for i, levels := range []int{100, 400} {
		deep := nest(list, levels)
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		if _, failure, ok := trySerialize(deep); !ok {
			t.Fatalf("Serialize failed: %v", failure)
		}
		runtime.ReadMemStats(&after)
		sizes[i] = after.TotalAlloc - before.TotalAlloc
	}
	if grew := float64(sizes[1]) / float64(sizes[0]); grew > 2 {
		t.Errorf("encoding a list four times as many sets down allocated %.1f times as much (%d bytes, then %d)", grew, sizes[0], sizes[1])
	}
}

// TestConformance_SE005_DeepMarksNestedLevelUponLevel holds decoding a nest of
// lists, each level carrying a deep mark of its own, to work in proportion to
// the marks it gives: every value holds the deep marks of every level above
// it, so a nest of d levels holds about d*d/2 of them, and a document four
// times as deep a little over sixteen times as many. Attaching each level's
// mark to everything below it as the level was read merged every value's
// marks once for each level above it, the cube of d. The marks share one
// identifier and are told apart by their payloads, which one mark decoder is
// enough to read. Allocated bytes are the reading.
// A container's deep marks and the marks its members carry of their own meet
// in every member, and a member holds its own marks beside one layer of the
// container's that every member shares, so k deep marks over k members each
// carrying one of its own cost k, not k by k: decoding, encoding the result,
// attaching the marks in memory, and taking them off again all grow with the
// document. The four times larger case allocates about four times as much.
func TestConformance_SE005_DeepMarksBesideMembersOwn(t *testing.T) {
	conformance.Covers(t, "SE-005", "MK-008", "SE-031")
	read := tenon.Decoders{Marks: map[string]tenon.MarkDecoder{
		"level": func(p tenon.Value, _ bool) (tenon.Mark, []tenon.Diagnostic) {
			return deepNote{"level", p.AsString()}, nil
		},
		"m": func(tenon.Value, bool) (tenon.Mark, []tenon.Diagnostic) { return markPlain, nil },
	}}
	allocations := func(f func()) uint64 {
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		f()
		runtime.ReadMemStats(&after)
		return after.TotalAlloc - before.TotalAlloc
	}
	var steps [4][2]uint64 // attach, encode, decode, unmark, at k and 4k
	for i, k := range []int{500, 2000} {
		members := make([]tenon.Value, k)
		for j := range members {
			members[j] = tenon.WithMarks(n(int64(j)), markPlain)
		}
		deep := make([]tenon.Mark, k)
		for j := range deep {
			deep[j] = deepNote{"level", fmt.Sprintf("%d", j)}
		}
		list := tenon.List(num, members...)
		var v, back tenon.Value
		var doc []byte
		steps[0][i] = allocations(func() { v = tenon.WithMarks(list, deep...) })
		steps[1][i] = allocations(func() {
			var failure tenon.Value
			var ok bool
			if doc, failure, ok = trySerialize(v); !ok {
				t.Fatalf("Serialize failed: %v", failure)
			}
		})
		steps[2][i] = allocations(func() {
			var failure tenon.Value
			var ok bool
			if back, failure, ok = tryDeserialize(doc, read); !ok {
				t.Fatalf("Deserialize failed: %v", failure)
			}
		})
		if !tenon.Identical(back, v) {
			t.Fatalf("%d members under %d deep marks came back other than they went", k, k)
		}
		if !tenon.HasMark(back.Elements()[k-1], deep[0]) || !tenon.HasMark(back.Elements()[k-1], markPlain) {
			t.Fatalf("a member read back lacks a mark it carried")
		}
		steps[3][i] = allocations(func() {
			if _, taken := tenon.UnmarkDeep(back); len(taken) != k+1 {
				t.Fatalf("UnmarkDeep took %d marks, want %d", len(taken), k+1)
			}
		})
	}
	for i, name := range []string{"attaching", "encoding", "decoding", "unmarking"} {
		if grew := float64(steps[i][1]) / float64(steps[i][0]); grew > 8 {
			t.Errorf("%s four times as many allocated %.1f times as much (%d bytes, then %d)", name, grew, steps[i][0], steps[i][1])
		}
	}
}

// A decimal fraction whose mantissa is a multiple of ten is no number's
// encoding, and is refused before its digits are worked out: telling the
// trailing zeros of a mantissa of 200,000 digits by its text allocates many
// times the document, where refusing it costs about the document.
func TestConformance_SE005_NonCanonicalMantissasAreRefusedFirst(t *testing.T) {
	conformance.Covers(t, "SE-005", "SE-032", "SE-002")
	c := new(big.Int).Exp(big.NewInt(10), big.NewInt(200000), nil)
	doc := cbor.AppendTag(nil, 1952804352)
	doc = cbor.AppendArray(doc, 2)
	doc = cbor.AppendUint(doc, 1)
	doc = cbor.AppendArray(doc, 3)
	doc = cbor.AppendUint(doc, 0)
	doc = cbor.AppendUint(doc, 2)
	doc = cbor.AppendTag(doc, 4)
	doc = cbor.AppendArray(doc, 2)
	doc = cbor.AppendUint(doc, 0)
	doc = cbor.AppendTag(doc, 2)
	doc = cbor.AppendBytes(doc, c.Bytes())
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	_, failure, ok := tryDeserialize(doc, tenon.Decoders{})
	runtime.ReadMemStats(&after)
	if ok || failure.Diagnostics()[0].Code != tenon.CodeSerializeNotCanonical {
		t.Fatalf("a mantissa of 10^200000 decoded as %v", failure)
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 4*uint64(len(doc)) {
		t.Errorf("refusing a %d-byte document allocated %d bytes", len(doc), allocated)
	}
}

func TestConformance_SE005_DeepMarksNestedLevelUponLevel(t *testing.T) {
	conformance.Covers(t, "SE-005", "MK-008", "SE-031")
	read := tenon.Decoders{Marks: map[string]tenon.MarkDecoder{
		"level": func(p tenon.Value, _ bool) (tenon.Mark, []tenon.Diagnostic) {
			return deepNote{"level", p.AsString()}, nil
		},
	}}
	var allocated [2]uint64
	for k, levels := range []int{60, 240} {
		nest := n(0)
		for level := range levels {
			nest = tenon.WithMarks(tenon.List(nest.Type(), nest), deepNote{"level", fmt.Sprintf("level %d", level)})
		}
		nests := make([]tenon.Value, 16)
		for i := range nests {
			nests[i] = nest
		}
		v := tenon.List(nest.Type(), nests...)
		doc, failure, ok := trySerialize(v)
		if !ok {
			t.Fatalf("Serialize(16 nests of %d levels) failed: %v", levels, failure)
		}
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		got, failure, ok := tryDeserialize(doc, read)
		runtime.ReadMemStats(&after)
		if !ok || !tenon.Identical(got, v) {
			t.Fatalf("16 nests of %d levels came back as %v, %v", levels, got, failure)
		}
		allocated[k] = after.TotalAlloc - before.TotalAlloc
	}
	if grew := float64(allocated[1]) / float64(allocated[0]); grew > 20 {
		t.Errorf("nests four times as deep allocated %.1f times as much (%d bytes, then %d), where the marks they hold are a little over sixteen times as many",
			grew, allocated[0], allocated[1])
	}
}

// tallied is a mark whose identifier every one of them shares, told apart by
// its payload, and which counts how often it is asked for its identifier:
// looking a mark up among those that share its identifier asks each of them.
type tallied struct{ text string }

var talliedIDs int

func (tallied) MarkID() string                     { talliedIDs++; return "tallied" }
func (tallied) Propagation() tenon.Propagation     { return tenon.Propagate }
func (tallied) Redacting() bool                    { return false }
func (m tallied) MarkPayload() (tenon.Value, bool) { return tenon.String(m.text), true }

// tallyDecoders reads tallied marks.
var tallyDecoders = tenon.Decoders{Marks: map[string]tenon.MarkDecoder{
	"tallied": func(payload tenon.Value, _ bool) (tenon.Mark, []tenon.Diagnostic) {
		return tallied{payload.AsString()}, nil
	},
}}

// TestConformance_SE005_ObjectsCostWhatTheyHold holds decoding a document of
// many objects of one type to work that grows with the document. A document
// states an object type once and then holds values of it, each of which can
// be a couple of bytes, so a decoder that reads the type again for every
// object, by rendering it or by gathering its attributes afresh, does work
// that grows with the square of what it is given.
func TestConformance_SE005_ObjectsCostWhatTheyHold(t *testing.T) {
	conformance.Covers(t, "SE-005", "SE-003", "EQ-044")
	// A type is as long as its text, whether that is one long name or many
	// short ones, so both shapes are decoded at a size and four times it. A
	// decoder that reads the type again for every object does sixteen times
	// the work for four times the document. Allocated bytes are the reading,
	// being the same on every run where a wall clock is not.
	shapes := []struct {
		name  string
		build func(size int) tenon.Value
	}{
		{"objects of a type naming one long attribute", func(size int) tenon.Value {
			name := strings.Repeat("a", 8*size)
			objects := make([]tenon.Value, size)
			for i := range objects {
				objects[i] = obj(map[string]tenon.Value{name: tenon.Null(num)})
			}
			return tenon.List(tenon.ObjectType(map[string]tenon.Type{name: num}), objects...)
		}},
		{"objects holding a null of a type of many attributes", func(size int) tenon.Value {
			attrs := map[string]tenon.Type{}
			for i := range size / 2 {
				attrs[fmt.Sprintf("x%06d", i)] = num
			}
			inner := tenon.ObjectType(attrs)
			objects := make([]tenon.Value, size)
			for i := range objects {
				objects[i] = obj(map[string]tenon.Value{"a": tenon.Null(inner)})
			}
			return tenon.List(tenon.ObjectType(map[string]tenon.Type{"a": inner}), objects...)
		}},
		// A set orders its members that are not known by what they are,
		// which reading them type and all spells the type out for each.
		{"unknown members of a set of a type naming one long attribute", func(size int) tenon.Value {
			elem := tenon.ObjectType(map[string]tenon.Type{strings.Repeat("a", 8*size): num})
			members := make([]tenon.Value, size)
			for i := range members {
				members[i] = tenon.Unknown(elem)
			}
			return tenon.Set(elem, members...)
		}},
	}
	for _, shape := range shapes {
		var allocated []uint64
		for _, size := range []int{250, 1000} {
			doc, failure, ok := trySerialize(shape.build(size))
			if !ok {
				t.Fatalf("Serialize(%d %s) failed: %v", size, shape.name, failure)
			}
			var before, after runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&before)
			got, failure, ok := tryDeserialize(doc, decoders)
			runtime.ReadMemStats(&after)
			if !ok {
				t.Fatalf("a document of %d %s came back as %v", size, shape.name, failure)
			}
			if got.Len() != size {
				t.Fatalf("a document of %d %s decoded to %d", size, shape.name, got.Len())
			}
			allocated = append(allocated, after.TotalAlloc-before.TotalAlloc)
		}
		if grew := float64(allocated[1]) / float64(allocated[0]); grew > 5 {
			t.Errorf("four times a document of %s allocated %.1f times as much (%d bytes, then %d)",
				shape.name, grew, allocated[0], allocated[1])
		}
	}

	// A document of ordinary objects costs what its content holds, rather
	// than what its type says: the decoder has the type already.
	small := make([]tenon.Value, 100)
	for i := range small {
		small[i] = obj(map[string]tenon.Value{"name": s("x"), "port": n(int64(i)), "on": tenon.Bool(true)})
	}
	doc, failure, ok := trySerialize(tenon.List(small[0].Type(), small...))
	if !ok {
		t.Fatalf("Serialize(100 objects) failed: %v", failure)
	}
	allocs := testing.AllocsPerRun(10, func() {
		if _, _, ok := tryDeserialize(doc, decoders); !ok {
			t.Fatal("the document did not decode")
		}
	})
	if budget := float64(30 * len(small)); allocs > budget {
		t.Errorf("decoding %d objects of three attributes made %.0f allocations, more than %.0f", len(small), allocs, budget)
	}

	// The content of an object is an array of its attributes, and a document
	// saying otherwise fails where it says it, naming the kind of type it is
	// not and no attribute: a message quotes nothing the document holds.
	short := document + "83 00 82 08 82 82 6161 02 82 6162 02 81 f6"
	wantDecodeFailure(t, "an object content of one item too few", short, tenon.CodeSerializeMalformed)
	want := `at byte 20: the content of an object is an array of 1 items, not 2`
	if _, failure, _ := tryDeserialize(fromHex(t, short), decoders); failure.Diagnostics()[0].Message != want {
		t.Errorf("a short object content said %q, want %q", failure.Diagnostics()[0].Message, want)
	}
}

func TestConformance_SE005_DecodingIsBounded(t *testing.T) {
	conformance.Covers(t, "SE-005", "SE-051")
	// Nesting beyond the bound is refused, not followed down the stack.
	deep := document + "83 00 " + strings.Repeat("82 04 ", 600) + "02 f6"
	wantDecodeFailure(t, "a deep type", deep, tenon.CodeSerializeTooLarge)

	// A few bytes declaring billions of members allocate nothing for them.
	for _, input := range []string{
		document + "83 00 82 04 02 9b 00000000ffffffff",
		document + "83 00 03 7a ffffffff",
		document + "83 00 82 06 02 9a ffffffff",
		document + "83 00 02 c4 82 00 c2 5b 00000000ffffffff",
	} {
		data := fromHex(t, input)
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		for range 100 {
			if _, _, ok := tryDeserialize(data, decoders); ok {
				t.Fatalf("%s decoded", input)
			}
		}
		runtime.ReadMemStats(&after)
		if grew := after.TotalAlloc - before.TotalAlloc; grew > 1<<20 {
			t.Errorf("decoding %s 100 times allocated %d bytes", input, grew)
		}
	}

	// Mutated documents decode to nothing or to a value whose encoding they
	// are, and never panic.
	g := generator{rand.New(rand.NewSource(7))}
	r := rand.New(rand.NewSource(8))
	for range conformance.Iterations(t, 400) {
		b, _, _ := trySerialize(g.top())
		for j := 0; j < 25; j++ {
			m := mutate(r, b)
			func() {
				defer func() {
					if p := recover(); p != nil {
						t.Fatalf("Deserialize(%x) panicked: %v", m, p)
					}
				}()
				if v, _, ok := tryDeserialize(m, decoders); ok {
					if again, _, _ := trySerialize(v); !bytes.Equal(again, m) {
						t.Fatalf("Deserialize(%x) = %v, which encodes as %x", m, v, again)
					}
				}
			}()
		}
	}
}

// mutate returns a copy of b with a byte changed, dropped or added, or cut
// short.
func mutate(r *rand.Rand, b []byte) []byte {
	m := bytes.Clone(b)
	i := r.Intn(len(m))
	switch r.Intn(4) {
	case 0:
		m[i] ^= byte(1 << r.Intn(8))
	case 1:
		m = append(m[:i], m[i+1:]...)
	case 2:
		m = append(m[:i], append([]byte{byte(r.Intn(256))}, m[i:]...)...)
	default:
		m = m[:i]
	}
	return m
}

// FuzzDeserialize holds the decoder to never panicking, and to accepting only
// what it would write. The seed corpus runs with the tests; `make fuzz` runs
// the fuzzer itself.
func FuzzDeserialize(f *testing.F) {
	g := generator{rand.New(rand.NewSource(1))}
	for range 64 {
		b, _, _ := trySerialize(g.top())
		f.Add(b)
	}
	for _, v := range values.All() {
		if b, _, ok := trySerialize(v); ok {
			f.Add(b)
		}
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		v, _, ok := tryDeserialize(data, decoders)
		if !ok {
			return
		}
		if again, failure, ok := trySerialize(v); !ok || !bytes.Equal(again, data) {
			t.Fatalf("Deserialize(%x) = %v, which serializes as %x, %v", data, v, again, failure)
		}
	})
}

// BenchmarkSetListings measures the work of recording, counting and decoding
// the known members of a set, at a size and four times it: the growth from one
// to the other is the reading, not the wall clock.
func BenchmarkSetListings(b *testing.B) {
	num := tenon.NumberType()
	for _, size := range []int{2000, 8000} {
		members := make([]tenon.Value, size)
		for i := range members {
			members[i] = tenon.NumberFromInt(int64(i))
		}
		listing := tenon.Members(members...)
		held := tenon.Set(num, append(members, tenon.Unknown(num))...)
		recorded := tenon.Narrow(tenon.Unknown(tenon.SetType(num)), listing)
		encoded, failure, ok := trySerialize(recorded)
		if !ok {
			b.Fatalf("Serialize(%v) failed: %v", recorded, failure)
		}
		b.Run(fmt.Sprintf("record/%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				tenon.Narrow(tenon.Unknown(tenon.SetType(num)), listing)
			}
		})
		b.Run(fmt.Sprintf("length/%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				tenon.Length(held)
			}
		})
		b.Run(fmt.Sprintf("decode/%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, _, ok := tryDeserialize(encoded, decoders); !ok {
					b.Fatal("the document did not decode")
				}
			}
		})
	}
}

// BenchmarkSetOfUnknowns measures building and decoding a set whose members
// are none of them known, at a size and four times it: the growth from one to
// the other is the reading, not the wall clock.
func BenchmarkSetOfUnknowns(b *testing.B) {
	num := tenon.NumberType()
	for _, size := range []int{2000, 8000} {
		members := make([]tenon.Value, size)
		for i := range members {
			members[i] = tenon.Narrow(tenon.Unknown(num), tenon.NotNull(), tenon.NumberMin(tenon.NumberFromInt(int64(i)), true))
		}
		set := tenon.Set(num, members...)
		encoded, failure, ok := trySerialize(set)
		if !ok {
			b.Fatalf("Serialize(a set of %d unknowns) failed: %v", size, failure)
		}
		b.Run(fmt.Sprintf("build/%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				tenon.Set(num, members...)
			}
		})
		b.Run(fmt.Sprintf("decode/%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, _, ok := tryDeserialize(encoded, decoders); !ok {
					b.Fatal("the document did not decode")
				}
			}
		})
	}
}

// BenchmarkManyMarks measures decoding a value that carries many marks, at a
// count and four times it: the growth from one to the other is the reading.
func BenchmarkManyMarks(b *testing.B) {
	for _, d := range []int{1000, 4000} {
		marks := make([]tenon.Mark, d)
		read := tenon.Decoders{Marks: map[string]tenon.MarkDecoder{}}
		for i := range marks {
			m := note{id: fmt.Sprintf("m%05d", i), text: "x"}
			marks[i] = m
			read.Marks[m.id] = func(tenon.Value, bool) (tenon.Mark, []tenon.Diagnostic) { return m, nil }
		}
		encoded, failure, ok := trySerialize(tenon.WithMarks(tenon.NumberFromInt(1), marks...))
		if !ok {
			b.Fatalf("Serialize(a value with %d marks) failed: %v", d, failure)
		}
		b.Run(fmt.Sprintf("decode/%d", d), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, _, ok := tryDeserialize(encoded, read); !ok {
					b.Fatal("the document did not decode")
				}
			}
		})
	}
}

// BenchmarkMarkDocuments measures decoding a value carrying many marks whose
// order in a document is not the order a value holds them in, at a count and
// four times it: marks that share one identifier, told apart by their
// payloads, which a value holds in the order they came, and marks whose
// identifiers are of two lengths, which a document lists shorter first and a
// value holds in the order of their text, so that the two interleave.
func BenchmarkMarkDocuments(b *testing.B) {
	interleaved := func(m int) (tenon.Value, tenon.Decoders) {
		marks := make([]tenon.Mark, 0, m)
		read := tenon.Decoders{Marks: map[string]tenon.MarkDecoder{}}
		for i := range m / 2 {
			for _, id := range []string{fmt.Sprintf("m%06d", i), fmt.Sprintf("m%06d_", i)} {
				mark := signal{id: id}
				marks = append(marks, mark)
				read.Marks[id] = func(tenon.Value, bool) (tenon.Mark, []tenon.Diagnostic) { return mark, nil }
			}
		}
		return tenon.WithMarks(tenon.NumberFromInt(1), marks...), read
	}
	shared := func(m int) (tenon.Value, tenon.Decoders) {
		marks := make([]tenon.Mark, m)
		for i := range marks {
			marks[i] = tallied{fmt.Sprintf("t%06d", i)}
		}
		return tenon.WithMarks(tenon.NumberFromInt(1), marks...), tallyDecoders
	}
	for _, shape := range []struct {
		name  string
		sizes []int
		build func(m int) (tenon.Value, tenon.Decoders)
	}{
		{"shared", []int{1000, 4000}, shared},
		{"interleaved", []int{16000, 64000}, interleaved},
	} {
		for _, m := range shape.sizes {
			v, read := shape.build(m)
			data, failure, ok := trySerialize(v)
			if !ok {
				b.Fatalf("%s/%d: %v", shape.name, m, failure)
			}
			if got, failure, ok := tryDeserialize(data, read); !ok || !tenon.Identical(got, v) {
				b.Fatalf("%s/%d came back as %v, %v", shape.name, m, got, failure)
			}
			b.Run(fmt.Sprintf("%s/%d", shape.name, m), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					tryDeserialize(data, read)
				}
			})
		}
	}
}

// BenchmarkObjectDocuments measures decoding a document of many objects of
// one type, at a size and four times it: the growth from one to the other is
// the reading, not the wall clock. The type's text is eight bytes per object,
// so a decoder that reads the type again for every object grows with the
// square of the document.
func BenchmarkObjectDocuments(b *testing.B) {
	for _, size := range []int{250, 1000} {
		name := strings.Repeat("a", 8*size)
		objects := make([]tenon.Value, size)
		for i := range objects {
			objects[i] = obj(map[string]tenon.Value{name: tenon.Null(num)})
		}
		doc, failure, ok := trySerialize(tenon.List(tenon.ObjectType(map[string]tenon.Type{name: num}), objects...))
		if !ok {
			b.Fatalf("Serialize(%d objects) failed: %v", size, failure)
		}
		b.Run(fmt.Sprintf("named/%d", size), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(doc)))
			for b.Loop() {
				if _, _, ok := tryDeserialize(doc, decoders); !ok {
					b.Fatal("the document did not decode")
				}
			}
		})
	}
	// An ordinary document, whose objects hold more than its type says.
	for _, size := range []int{250, 1000} {
		objects := make([]tenon.Value, size)
		for i := range objects {
			objects[i] = obj(map[string]tenon.Value{"name": s("x"), "port": n(int64(i)), "on": tenon.Bool(true)})
		}
		doc, failure, ok := trySerialize(tenon.List(objects[0].Type(), objects...))
		if !ok {
			b.Fatalf("Serialize(%d objects) failed: %v", size, failure)
		}
		b.Run(fmt.Sprintf("plain/%d", size), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(doc)))
			for b.Loop() {
				if _, _, ok := tryDeserialize(doc, decoders); !ok {
					b.Fatal("the document did not decode")
				}
			}
		})
	}
}

// TestConformance_SE043_DecoderContracts holds the decoder hooks to their
// contracts. Input that is wrong is data: a path step that is not
// well-formed text reports the text failure, where dropping ReadText's
// error once read it as an empty name. A caller that is wrong is a usage
// error, refused up front where possible: a nil mark decoder before any
// byte is read, a type in Capsules that is not a capsule type, and a mark
// decoder returning a mark whose type declares no encoding, which could
// never serialize again and once surfaced as serialize.not_canonical.
func TestConformance_SE043_DecoderContracts(t *testing.T) {
	conformance.Covers(t, "SE-043", "ER-001")
	input := document + "82 02 81 83 65 6170702e78 61 6d 81 61ff"
	got, failure, ok := tryDeserialize(fromHex(t, input), decoders)
	if ok {
		t.Fatalf("an ill-formed path step decoded: %v", got)
	}
	if d := failure.Diagnostics(); len(d) != 1 || d[0].Code != tenon.CodeSerializeMalformed ||
		!strings.Contains(d[0].Message, "UTF-8") {
		t.Errorf("an ill-formed path step reports %v, want serialize.malformed naming UTF-8", failure)
	}

	mustPanicUsage(t, `the decoder for the mark "m" is nil`, func() {
		tryDeserialize(nil, tenon.Decoders{Marks: map[string]tenon.MarkDecoder{"m": nil}})
	})
	mustPanicUsage(t, "not Capsule", func() {
		tryDeserialize(nil, tenon.Decoders{Capsules: []tenon.Type{tenon.NumberType()}})
	})
	marked := document + "83 00 01 da74656e02 82 f5 81 81 616d"
	plain := tenon.Decoders{Marks: map[string]tenon.MarkDecoder{
		"m": func(tenon.Value, bool) (tenon.Mark, []tenon.Diagnostic) { return stamp{id: "m"}, nil },
	}}
	mustPanicUsage(t, "declares no encoding, which cannot serialize again", func() {
		tryDeserialize(fromHex(t, marked), plain)
	})
}

// A value nesting deeper than a document may has no encoding: Serialize
// refuses it where the decoder would, so whatever it writes reads back. The
// bound is the decoder's: the item is the first level, and each type,
// constraint and content another holds is one deeper.
func TestConformance_SE003_OnlyValuesWithinTheBoundHaveAnEncoding(t *testing.T) {
	conformance.Covers(t, "SE-003", "SE-005", "SE-050")
	lists := func(n int) tenon.Type {
		typ := tenon.NumberType()
		for range n {
			typ = tenon.ListType(typ)
		}
		return typ
	}
	// The item, then 510 list types around a number: 512 levels.
	within := tenon.Null(lists(510))
	data, fail, ok := trySerialize(within)
	if !ok {
		t.Fatalf("a null of 510 list types did not serialize: %v", fail)
	}
	if back, fail, ok := tryDeserialize(data, tenon.Decoders{}); !ok || !tenon.Identical(back, within) {
		t.Fatalf("a null of 510 list types read back as %v, %v", back, fail)
	}
	_, fail, ok = trySerialize(tenon.Null(lists(511)))
	if ok {
		t.Fatal("a null of 511 list types serialized, which no decoder reads")
	}
	if ds := fail.Diagnostics(); len(ds) != 1 || ds[0].Code != tenon.CodeSerializeTooLarge || ds[0].Path.Len() != 0 {
		t.Errorf("a null of 511 list types failed with %v, want one %s at .", fail, tenon.CodeSerializeTooLarge)
	}
	// Values nested list in list: whatever Serialize writes decodes, and what
	// it refuses, it refuses as too large.
	nested := func(d int) tenon.Value {
		v := tenon.NumberFromInt(1)
		for range d {
			v = tenon.List(v.Type(), v)
		}
		return v
	}
	written, refused := 0, 0
	for d := 505; d <= 515; d++ {
		v := nested(d)
		data, fail, ok := trySerialize(v)
		if !ok {
			refused++
			if ds := fail.Diagnostics(); len(ds) != 1 || ds[0].Code != tenon.CodeSerializeTooLarge {
				t.Errorf("%d lists deep: %v, want %s", d, fail, tenon.CodeSerializeTooLarge)
			}
			continue
		}
		written++
		if back, fail, ok := tryDeserialize(data, tenon.Decoders{}); !ok || !tenon.Identical(back, v) {
			t.Errorf("%d lists deep serialized, and read back as %v, %v", d, back, fail)
		}
	}
	if written == 0 || refused == 0 {
		t.Errorf("of the depths tried, %d serialized and %d were refused; want some of each", written, refused)
	}
}

// holding is a mark serialized with whatever value it holds. It holds the
// value by pointer, since a struct holding a Value is not comparable.
type holding struct{ v *tenon.Value }

func (holding) MarkID() string                     { return "p" }
func (holding) Propagation() tenon.Propagation     { return tenon.Propagate }
func (holding) Redacting() bool                    { return false }
func (m holding) MarkPayload() (tenon.Value, bool) { return *m.v, true }

// TestConformance_SE050_DecodeMessagesQuoteNothing holds a decoding failure's
// message to saying what is wrong and where, by byte offset, and quoting
// nothing the document holds: a document is refused before the marks that
// follow its content are read, so it cannot know what a redacting mark would
// withhold (MK-011). Each document below carries "hunter2" in what it gets
// wrong: a map key twice, an attribute name twice, a prefix on a number, a
// listing on a number, and a diagnostic code that is none.
func TestConformance_SE050_DecodeMessagesQuoteNothing(t *testing.T) {
	conformance.Covers(t, "SE-050", "MK-011")
	const hunter2 = "67 68756e74657232"
	for _, tt := range []struct{ name, hex string }{
		{"a map key twice", document + "830082060282 82" + hunter2 + "01 82" + hunter2 + "02"},
		{"an attribute name twice", document + "8300 8208 82 82" + hunter2 + "02 82" + hunter2 + "02 82 01 02"},
		{"a prefix on a number", document + "830002da74656e01 a1 03" + hunter2},
		{"a listing on a number", document + "830002da74656e01 a1 06 81" + hunter2},
		{"a diagnostic code that is none", document + "8202 81 83 67 48756e74657232 616d 80"},
	} {
		_, failure, ok := tryDeserialize(fromHex(t, tt.hex), decoders)
		if ok {
			t.Errorf("%s decoded", tt.name)
			continue
		}
		for _, d := range failure.Diagnostics() {
			if strings.Contains(strings.ToLower(d.Message), "hunter2") || !strings.HasPrefix(d.Message, "at byte ") {
				t.Errorf("%s: %s: %q quotes the document or says no offset", tt.name, d.Code, d.Message)
			}
		}
	}
}
