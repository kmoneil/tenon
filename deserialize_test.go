package tenon_test

import (
	"bytes"
	"fmt"
	"math/rand"
	"runtime"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/conformance"
	"github.com/kmoneil/tenon/conformance/values"
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
	Capsules: []tenon.Type{degrees},
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
		return []tenon.Type{boo, num, str, degrees}[g.r.Intn(4)]
	}
	switch g.r.Intn(5) {
	case 0:
		return tenon.List(g.typ(depth - 1))
	case 1:
		return tenon.Set(g.typ(depth - 1))
	case 2:
		return tenon.Map(g.typ(depth - 1))
	case 3:
		elems := make([]tenon.Type, g.r.Intn(3))
		for i := range elems {
			elems[i] = g.typ(depth - 1)
		}
		return tenon.Tuple(elems...)
	}
	attrs := map[string]tenon.Type{}
	for _, name := range []string{"a", "b", "c\U00000301"} {
		if g.r.Intn(2) == 0 {
			attrs[name] = g.typ(depth - 1)
		}
	}
	return tenon.Object(attrs)
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
		v = tenon.NullVal(t)
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
		return tenon.CapsuleVal(degrees, &celsius{int64(g.r.Intn(5))})
	case tenon.KindList, tenon.KindSet:
		members := make([]tenon.Value, g.r.Intn(4))
		for i := range members {
			members[i] = g.value(t.ElementType(), depth-1, t.Kind() == tenon.KindSet)
		}
		if t.Kind() == tenon.KindSet {
			return tenon.SetVal(t.ElementType(), members...)
		}
		return tenon.ListVal(t.ElementType(), members...)
	case tenon.KindMap:
		entries := map[string]tenon.Value{}
		for _, key := range []string{"", "k", "j\U00000301"} {
			if g.r.Intn(2) == 0 {
				entries[key] = g.value(t.ElementType(), depth-1, false)
			}
		}
		return tenon.MapVal(t.ElementType(), entries)
	case tenon.KindTuple:
		elems := make([]tenon.Value, t.TupleLength())
		for i := range elems {
			elems[i] = g.value(t.TupleElementType(i), depth-1, false)
		}
		return tenon.TupleVal(elems...)
	}
	attrs := map[string]tenon.Value{}
	for _, name := range t.AttributeNames() {
		attrs[name] = g.value(t.AttributeType(name), depth-1, false)
	}
	return tenon.ObjectVal(attrs)
}

// top returns a value to serialize: now and then a pending or an error value,
// and otherwise a resolved one.
func (g generator) top() tenon.Value {
	var v tenon.Value
	switch g.r.Intn(12) {
	case 0:
		v = tenon.Pending(randomConstraint(g.r, 2, degrees))
		v = tenon.Narrow(v, []tenon.Narrowing{tenon.NotNull(), tenon.Null(), tenon.NotNull()}[g.r.Intn(3)])
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
		b, failure, ok := tenon.Serialize(v)
		if !ok {
			t.Fatalf("Serialize(%v) failed: %v", v, failure)
		}
		fmt.Fprintf(&emitted, "%x\n", b)
		got, failure, ok := tenon.Deserialize(b, decoders)
		switch {
		case !ok:
			t.Fatalf("Deserialize(Serialize(%v)) failed: %v\n%x", v, failure, b)
		case !tenon.Identical(got, v):
			t.Fatalf("Deserialize(Serialize(%v)) = %v", v, got)
		}
	}
	// The generator's own values that serialize come back too.
	for _, v := range values.All() {
		b, _, ok := tenon.Serialize(v)
		if !ok {
			continue
		}
		if got, failure, ok := tenon.Deserialize(b, decoders); !ok || !tenon.Identical(got, v) {
			t.Errorf("Deserialize(Serialize(%v)) = %v, %v", v, got, failure)
		}
	}
}

// wantDecodeFailure fails t unless the document holding item fails to decode
// with a diagnostic of code.
func wantDecodeFailure(t *testing.T, what, input string, code tenon.Code) {
	t.Helper()
	got, failure, ok := tenon.Deserialize(fromHex(t, input), decoders)
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
		{"a range no value lies in", document + "83 00 02 da74656e01 a2 01 82 05 f5 02 82 01 f5"},
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

func TestConformance_SE043_DecodersAreSupplied(t *testing.T) {
	conformance.Covers(t, "SE-043", "SE-051")
	wantDecodeFailure(t, "an unknown capsule", document+"83 00 82 09 63 782f79 f6", tenon.CodeSerializeUnknownCapsule)
	wantDecodeFailure(t, "an unknown mark", document+"83 00 01 da74656e02 82 f5 81 81 617a", tenon.CodeSerializeUnknownMark)
	// A decoder that refuses gives its own diagnostics.
	wantDecodeFailure(t, "a note without text", document+"83 00 01 da74656e02 82 f5 81 81 6170", "app.bad_note")
	refusing := tenon.Capsule("refusing", tenon.CapsuleOps[celsius]{Encoding: &tenon.CapsuleEncoding[celsius]{
		ID: "t/refusing", Type: num,
		Encode: func(v *celsius) tenon.Value { return n(v.degrees) },
		Decode: func(tenon.Value) (*celsius, []tenon.Diagnostic) {
			return nil, []tenon.Diagnostic{{Code: "app.refused", Message: "no"}}
		},
	}})
	b, _, _ := tenon.Serialize(tenon.CapsuleVal(refusing, &celsius{1}))
	if _, failure, ok := tenon.Deserialize(b, tenon.Decoders{Capsules: []tenon.Type{refusing}}); ok || failure.Diagnostics()[0].Code != "app.refused" {
		t.Errorf("a refusing capsule decoder gave %v", failure)
	}
	// What the caller supplies must itself be sound.
	opaque := tenon.Capsule("opaque", tenon.CapsuleOps[celsius]{})
	mustPanicUsage(t, "declares no encoding", func() { tenon.Deserialize(nil, tenon.Decoders{Capsules: []tenon.Type{opaque}}) })
	mustPanicUsage(t, "returned the mark", func() {
		tenon.Deserialize(fromHex(t, document+"83 00 01 da74656e02 82 f5 81 81 616d"), tenon.Decoders{Marks: map[string]tenon.MarkDecoder{
			"m": func(tenon.Value, bool) (tenon.Mark, []tenon.Diagnostic) { return markDeep, nil },
		}})
	})
}

// counting is a capsule type that encodes as a number and counts how often
// two of its values are compared, which only two known values of it ever are.
var (
	countingCompared int
	counting         = tenon.Capsule("counting", tenon.CapsuleOps[int64]{
		Equals: func(a, b *int64) bool { countingCompared++; return *a == *b },
		Hash:   func(v *int64) uint64 { return uint64(*v) },
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
	if _, failure, ok := tenon.Deserialize(fromHex(t, levels(510)), decoders); !ok {
		t.Errorf("512 levels were refused: %v", failure)
	}
	wantDecodeFailure(t, "513 levels", levels(511), tenon.CodeSerializeTooLarge)

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
	if _, failure, ok := tenon.Deserialize(doc, decoders); ok || failure.Diagnostics()[0].Code != tenon.CodeSerializeNotCanonical {
		t.Errorf("the run out of order decoded as %v, %v", ok, failure)
	}

	// A set of 2,000 members that are not known, each a tuple holding a
	// counting value and an unknown number: no two are compared.
	num := tenon.NumberType()
	elem := tenon.Tuple(counting, num)
	members := make([]tenon.Value, 2000)
	for i := range members {
		v := int64(i)
		members[i] = tenon.TupleVal(tenon.CapsuleVal(counting, &v), tenon.Unknown(num))
	}
	withCounting := tenon.Decoders{Capsules: []tenon.Type{counting}}
	countingCompared = 0
	set := tenon.SetVal(elem, members...)
	b, failure, ok := tenon.Serialize(set)
	if !ok {
		t.Fatalf("Serialize(the set) failed: %v", failure)
	}
	got, failure, ok := tenon.Deserialize(b, withCounting)
	compared := countingCompared
	if !ok || !tenon.Identical(got, set) {
		t.Fatalf("the set came back as %v, %v", got, failure)
	}
	if compared > 4*len(members) {
		t.Errorf("building, encoding and decoding a set of %d members that are not known compared them %d times, where every pair is %d",
			len(members), compared, len(members)*(len(members)-1)/2)
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
	b, _, _ = tenon.Serialize(marked)
	if got, failure, ok := tenon.Deserialize(b, read); !ok || !tenon.Identical(got, marked) {
		t.Errorf("the value with 4,000 marks came back as %v, %v", got, failure)
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
			if _, _, ok := tenon.Deserialize(data, decoders); ok {
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
		b, _, _ := tenon.Serialize(g.top())
		for j := 0; j < 25; j++ {
			m := mutate(r, b)
			func() {
				defer func() {
					if p := recover(); p != nil {
						t.Fatalf("Deserialize(%x) panicked: %v", m, p)
					}
				}()
				if v, _, ok := tenon.Deserialize(m, decoders); ok {
					if again, _, _ := tenon.Serialize(v); !bytes.Equal(again, m) {
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
		b, _, _ := tenon.Serialize(g.top())
		f.Add(b)
	}
	for _, v := range values.All() {
		if b, _, ok := tenon.Serialize(v); ok {
			f.Add(b)
		}
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		v, _, ok := tenon.Deserialize(data, decoders)
		if !ok {
			return
		}
		if again, failure, ok := tenon.Serialize(v); !ok || !bytes.Equal(again, data) {
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
		held := tenon.SetVal(num, append(members, tenon.Unknown(num))...)
		recorded := tenon.Narrow(tenon.Unknown(tenon.Set(num)), listing)
		encoded, failure, ok := tenon.Serialize(recorded)
		if !ok {
			b.Fatalf("Serialize(%v) failed: %v", recorded, failure)
		}
		b.Run(fmt.Sprintf("record/%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				tenon.Narrow(tenon.Unknown(tenon.Set(num)), listing)
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
				if _, _, ok := tenon.Deserialize(encoded, decoders); !ok {
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
		set := tenon.SetVal(num, members...)
		encoded, failure, ok := tenon.Serialize(set)
		if !ok {
			b.Fatalf("Serialize(a set of %d unknowns) failed: %v", size, failure)
		}
		b.Run(fmt.Sprintf("build/%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				tenon.SetVal(num, members...)
			}
		})
		b.Run(fmt.Sprintf("decode/%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, _, ok := tenon.Deserialize(encoded, decoders); !ok {
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
		encoded, failure, ok := tenon.Serialize(tenon.WithMarks(tenon.NumberFromInt(1), marks...))
		if !ok {
			b.Fatalf("Serialize(a value with %d marks) failed: %v", d, failure)
		}
		b.Run(fmt.Sprintf("decode/%d", d), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, _, ok := tenon.Deserialize(encoded, read); !ok {
					b.Fatal("the document did not decode")
				}
			}
		})
	}
}
