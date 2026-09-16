package vectors_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"math/rand"
	"os"
	"slices"
	"strings"
	"testing"

	"tenon"
	"tenon/conformance"
)

// The capsule type and marks the vectors use, as vectors.json describes them.
type degrees struct{ n int64 }

var degreesType = tenon.Capsule("degrees", tenon.CapsuleOps[degrees]{
	Equals: func(a, b *degrees) bool { return *a == *b },
	Hash:   func(v *degrees) uint64 { return uint64(v.n) },
	Encoding: &tenon.CapsuleEncoding[degrees]{
		ID:     "t/c",
		Type:   tenon.NumberType(),
		Encode: func(v *degrees) tenon.Value { return tenon.NumberFromInt(v.n) },
		Decode: func(v tenon.Value) (*degrees, []tenon.Diagnostic) {
			i, ok := v.AsInt64()
			if !ok {
				return nil, []tenon.Diagnostic{{Code: "vectors.not_whole", Message: "degrees are whole"}}
			}
			return &degrees{i}, nil
		},
	},
})

// mark is a mark that serializes as its identifier alone.
type mark struct {
	id   string
	deep bool
}

func (m mark) MarkID() string                 { return m.id }
func (mark) Propagation() tenon.Propagation   { return tenon.Propagate }
func (mark) Redacting() bool                  { return false }
func (m mark) Deep() bool                     { return m.deep }
func (mark) MarkPayload() (tenon.Value, bool) { return tenon.Value{}, false }

// secret is a redacting mark that serializes as its identifier alone.
type secret struct{}

func (secret) MarkID() string                   { return "r" }
func (secret) Propagation() tenon.Propagation   { return tenon.Propagate }
func (secret) Redacting() bool                  { return true }
func (secret) MarkPayload() (tenon.Value, bool) { return tenon.Value{}, false }

// note is a mark that serializes with a string.
type note struct{ text string }

func (note) MarkID() string                     { return "p" }
func (note) Propagation() tenon.Propagation     { return tenon.Propagate }
func (note) Redacting() bool                    { return false }
func (m note) MarkPayload() (tenon.Value, bool) { return tenon.String(m.text), true }

var (
	plain = mark{id: "m"}
	deep  = mark{id: "d", deep: true}
)

var decoders = tenon.Decoders{
	Capsules: []tenon.Type{degreesType},
	Marks: map[string]tenon.MarkDecoder{
		"m": func(tenon.Value, bool) (tenon.Mark, []tenon.Diagnostic) { return plain, nil },
		"d": func(tenon.Value, bool) (tenon.Mark, []tenon.Diagnostic) { return deep, nil },
		"r": func(tenon.Value, bool) (tenon.Mark, []tenon.Diagnostic) { return secret{}, nil },
		"p": func(payload tenon.Value, has bool) (tenon.Mark, []tenon.Diagnostic) {
			if !has || payload.Type() != tenon.StringType() {
				return nil, []tenon.Diagnostic{{Code: "vectors.bad_note", Message: "a note needs text"}}
			}
			return note{payload.AsString()}, nil
		},
	},
}

var (
	num, str, boo = tenon.NumberType(), tenon.StringType(), tenon.BoolType()
	n             = tenon.NumberFromInt
	s             = tenon.String
)

// shuffled returns xs in an order r chooses.
func shuffled[T any](r *rand.Rand, xs ...T) []T {
	out := slices.Clone(xs)
	r.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

// one returns one of xs, as r chooses.
func one[T any](r *rand.Rand, xs ...T) T { return xs[r.Intn(len(xs))] }

// spelled returns the number that each of the spellings denotes, spelled as
// r chooses.
func spelled(r *rand.Rand, spellings ...string) tenon.Value {
	return tenon.NumberFromText(one(r, spellings...))
}

// marked returns v carrying marks, attached in an order and a grouping r
// chooses.
func marked(r *rand.Rand, v tenon.Value, marks ...tenon.Mark) tenon.Value {
	for _, m := range shuffled(r, marks...) {
		v = tenon.WithMarks(v, m)
	}
	return v
}

// composed is "e" with an acute accent, which r writes composed or not.
func composed(r *rand.Rand) string { return one(r, "\U000000e9", "e\U00000301") }

// vector is a valid vector: a value, built as r chooses.
type vector struct {
	name  string
	build func(r *rand.Rand) tenon.Value
}

var valid = []vector{
	{"bool/false", func(*rand.Rand) tenon.Value { return tenon.Bool(false) }},
	{"bool/true", func(*rand.Rand) tenon.Value { return tenon.Bool(true) }},
	{"number/zero", func(r *rand.Rand) tenon.Value { return spelled(r, "0", "0.00", "-0", "0e7") }},
	{"number/23", func(r *rand.Rand) tenon.Value { return spelled(r, "23", "2.3e1", "230e-1") }},
	{"number/24", func(r *rand.Rand) tenon.Value { return spelled(r, "24", "24.0") }},
	{"number/-1", func(r *rand.Rand) tenon.Value { return spelled(r, "-1", "-1.000") }},
	{"number/-25", func(r *rand.Rand) tenon.Value { return spelled(r, "-25", "-2.5e1") }},
	{"number/1000", func(r *rand.Rand) tenon.Value { return spelled(r, "1000", "1e3", "10.00e2") }},
	{"number/1.5", func(r *rand.Rand) tenon.Value { return spelled(r, "1.5", "15e-1", "1.50") }},
	{"number/-0.25", func(r *rand.Rand) tenon.Value { return spelled(r, "-0.25", "-25e-2") }},
	{"number/2^64-1", func(*rand.Rand) tenon.Value { return tenon.NumberFromText("18446744073709551615") }},
	{"number/-2^64", func(*rand.Rand) tenon.Value { return tenon.NumberFromText("-18446744073709551616") }},
	{"number/2^64", func(*rand.Rand) tenon.Value { return tenon.NumberFromText("18446744073709551616") }},
	{"number/-2^64-1", func(*rand.Rand) tenon.Value { return tenon.NumberFromText("-18446744073709551617") }},
	{"number/1e30", func(r *rand.Rand) tenon.Value { return spelled(r, "1e30", "1"+strings.Repeat("0", 30)) }},
	{"number/1e-30", func(r *rand.Rand) tenon.Value { return spelled(r, "1e-30", "0."+strings.Repeat("0", 29)+"1") }},
	{"number/pi", func(*rand.Rand) tenon.Value {
		return tenon.NumberFromText("3.14159265358979323846264338327950288419716939937510")
	}},
	{"number/window top", func(r *rand.Rand) tenon.Value { return spelled(r, "1e999999", "0.1e1000000") }},
	{"number/window bottom", func(r *rand.Rand) tenon.Value { return spelled(r, "-1e-999999", "-10e-1000000") }},
	{"string/empty", func(*rand.Rand) tenon.Value { return s("") }},
	{"string/ascii", func(*rand.Rand) tenon.Value { return s("tenon") }},
	{"string/composed", func(r *rand.Rand) tenon.Value { return s("caf" + composed(r)) }},
	{"string/astral", func(*rand.Rand) tenon.Value { return s("\U0001F600") }},
	{"string/controls", func(*rand.Rand) tenon.Value { return s("\x00\t\n\x7f") }},
	{"string/24 bytes", func(*rand.Rand) tenon.Value { return s(strings.Repeat("a", 24)) }},
	{"string/300 bytes", func(*rand.Rand) tenon.Value { return s(strings.Repeat("ab", 150)) }},
	{"null/bool", func(*rand.Rand) tenon.Value { return tenon.NullVal(boo) }},
	{"null/number", func(*rand.Rand) tenon.Value { return tenon.NullVal(num) }},
	{"null/string", func(*rand.Rand) tenon.Value { return tenon.NullVal(str) }},
	{"null/list of numbers", func(*rand.Rand) tenon.Value { return tenon.NullVal(tenon.List(num)) }},
	{"null/capsule", func(*rand.Rand) tenon.Value { return tenon.NullVal(degreesType) }},
	{"list/empty", func(*rand.Rand) tenon.Value { return tenon.ListVal(num) }},
	{"list/numbers", func(*rand.Rand) tenon.Value { return tenon.ListVal(num, n(1), n(2), n(3)) }},
	{"list/nested", func(*rand.Rand) tenon.Value {
		return tenon.ListVal(tenon.List(num), tenon.ListVal(num, n(1)), tenon.ListVal(num))
	}},
	{"set/numbers", func(r *rand.Rand) tenon.Value { return tenon.SetVal(num, shuffled(r, n(-1), n(1), n(24), n(1000))...) }},
	{"set/strings given twice", func(r *rand.Rand) tenon.Value {
		return tenon.SetVal(str, shuffled(r, s("b"), s("a"), s("b"), s(composed(r)), s(composed(r)))...)
	}},
	{"set/unknown members", func(r *rand.Rand) tenon.Value {
		return tenon.SetVal(num, shuffled(r, n(1), tenon.Unknown(num), tenon.Unknown(num))...)
	}},
	{"set/capsules", func(r *rand.Rand) tenon.Value {
		return tenon.SetVal(degreesType, shuffled(r,
			tenon.CapsuleVal(degreesType, &degrees{30}),
			tenon.CapsuleVal(degreesType, &degrees{-5}),
			tenon.CapsuleVal(degreesType, &degrees{1}))...)
	}},
	{"map/empty", func(*rand.Rand) tenon.Value { return tenon.MapVal(num, nil) }},
	{"map/entries", func(*rand.Rand) tenon.Value {
		return tenon.MapVal(num, map[string]tenon.Value{"b": n(2), "a": n(1), "": n(0)})
	}},
	{"map/composed key", func(r *rand.Rand) tenon.Value {
		return tenon.MapVal(str, map[string]tenon.Value{composed(r): s("x")})
	}},
	{"tuple/empty", func(*rand.Rand) tenon.Value { return tenon.TupleVal() }},
	{"tuple/mixed", func(r *rand.Rand) tenon.Value {
		return tenon.TupleVal(tenon.Bool(true), s("x"), spelled(r, "1.5", "15e-1"))
	}},
	{"object/empty", func(*rand.Rand) tenon.Value { return tenon.ObjectVal(nil) }},
	{"object/attributes", func(r *rand.Rand) tenon.Value {
		return tenon.ObjectVal(map[string]tenon.Value{"b": n(1), "a": s("x"), composed(r): tenon.Bool(false)})
	}},
	{"unknown/number", func(*rand.Rand) tenon.Value { return tenon.Unknown(num) }},
	{"unknown/bounded number", func(r *rand.Rand) tenon.Value {
		return tenon.Narrow(tenon.Unknown(num), shuffled(r,
			tenon.NotNull(), tenon.NumberMin(n(1), true), tenon.NumberMax(tenon.NumberFromText("2.5"), false))...)
	}},
	{"unknown/string prefix", func(r *rand.Rand) tenon.Value {
		return tenon.Narrow(tenon.Unknown(str), shuffled(r, tenon.StringPrefix("cafe-"), tenon.LengthMax(10))...)
	}},
	{"unknown/list length", func(r *rand.Rand) tenon.Value {
		return tenon.Narrow(tenon.Unknown(tenon.List(str)), shuffled(r, tenon.LengthMin(1), tenon.LengthMax(3))...)
	}},
	{"unknown/set members", func(r *rand.Rand) tenon.Value {
		members := one(r,
			[]tenon.Narrowing{tenon.Members(n(1), n(2))},
			[]tenon.Narrowing{tenon.Members(n(2)), tenon.Members(n(1))})
		return tenon.Narrow(tenon.Unknown(tenon.Set(num)), shuffled(r, append(members, tenon.LengthMax(5))...)...)
	}},
	{"unknown/capsule", func(*rand.Rand) tenon.Value { return tenon.Narrow(tenon.Unknown(degreesType), tenon.NotNull()) }},
	{"pending/any", func(*rand.Rand) tenon.Value { return tenon.Pending(tenon.Any()) }},
	{"pending/null", func(*rand.Rand) tenon.Value { return tenon.Narrow(tenon.Pending(tenon.Any()), tenon.Null()) }},
	{"pending/not null", func(*rand.Rand) tenon.Value {
		return tenon.Narrow(tenon.Pending(tenon.Exactly(degreesType)), tenon.NotNull())
	}},
	{"pending/collections", func(*rand.Rand) tenon.Value {
		return tenon.Pending(tenon.TupleOf(tenon.ListOf(tenon.Any()), tenon.SetOf(tenon.Exactly(str)), tenon.MapOf(tenon.Exactly(boo))))
	}},
	{"pending/object", func(*rand.Rand) tenon.Value {
		return tenon.Pending(tenon.ObjectWith(map[string]tenon.Field{
			"a": tenon.Required(tenon.Exactly(num)),
			"b": tenon.Optional(tenon.OneOf(tenon.Exactly(str), tenon.Any())),
		}, true))
	}},
	{"error/one", func(*rand.Rand) tenon.Value {
		return tenon.ErrorVal(tenon.Diagnostic{Code: "vectors.failed", Message: "it failed"})
	}},
	{"error/paths", func(r *rand.Rand) tenon.Value {
		return tenon.ErrorVal(
			tenon.Diagnostic{Code: "vectors.failed", Message: "at a path", Path: tenon.Path{}.Attribute("a").Index(n(0)).Index(s("k"))},
			tenon.Diagnostic{Code: "vectors.other", Message: "at a number", Path: tenon.Path{}.Index(spelled(r, "2.5", "25e-1"))})
	}},
	{"marks/one", func(r *rand.Rand) tenon.Value { return marked(r, n(1), plain) }},
	{"marks/in order", func(r *rand.Rand) tenon.Value {
		return marked(r, tenon.Bool(true), plain, note{"x"}, note{"y"})
	}},
	{"marks/deep", func(r *rand.Rand) tenon.Value {
		first := n(1)
		if r.Intn(2) == 0 {
			first = tenon.WithMarks(first, deep)
		}
		return tenon.WithMarks(tenon.ListVal(num, first, n(2)), deep)
	}},
	{"marks/member", func(r *rand.Rand) tenon.Value {
		return tenon.ListVal(num, n(1), marked(r, n(2), plain, note{"x"}))
	}},
	{"marks/null and unknown", func(r *rand.Rand) tenon.Value {
		return tenon.TupleVal(marked(r, tenon.NullVal(num), plain), marked(r, tenon.Unknown(str), note{"u"}))
	}},
	{"marks/pending", func(r *rand.Rand) tenon.Value { return marked(r, tenon.Pending(tenon.Any()), plain) }},
	{"marks/error", func(r *rand.Rand) tenon.Value {
		return marked(r, tenon.ErrorVal(tenon.Diagnostic{Code: "vectors.failed", Message: "it failed"}), note{"e"}, plain)
	}},
	{"marks/set", func(r *rand.Rand) tenon.Value {
		return tenon.WithMarks(tenon.SetVal(num, shuffled(r, n(1), n(2))...), deep)
	}},
	{"marks/redacted", func(r *rand.Rand) tenon.Value {
		return tenon.ObjectVal(map[string]tenon.Value{"password": marked(r, s("hunter2"), secret{}, plain), "user": s("ann")})
	}},
	{"capsule/value", func(*rand.Rand) tenon.Value { return tenon.CapsuleVal(degreesType, &degrees{21}) }},
}

// invalidVector is input that encodes no value.
type invalidVector struct{ name, hex, code string }

// document heads every document.
const document = "da74656e008201"

var invalid = []invalidVector{
	{"integer in a longer form", document + "8300021801", "serialize.not_canonical"},
	{"set members out of order", document + "83008205028220 01", "serialize.not_canonical"},
	{"set member twice", document + "830082050282 0101", "serialize.not_canonical"},
	{"map keys out of order", document + "830082060282826162028261610 1", "serialize.not_canonical"},
	{"string not in normal form", document + "8300036365cc81", "serialize.not_canonical"},
	{"integer as a decimal fraction", document + "830002c4820001", "serialize.not_canonical"},
	{"coefficient a multiple of ten", document + "830002c482200a", "serialize.not_canonical"},
	{"range key out of order", document + "830002da74656e01a2018201f500f5", "serialize.not_canonical"},
	{"range that is one value", document + "8300820402da74656e01a200f50500", "serialize.not_canonical"},
	{"deep mark listed on a member", document + "8300820402da74656e028281da74656e02820181816164818161 64", "serialize.not_canonical"},
	{"marks out of order", document + "830001da74656e0282f58283617003617681616d", "serialize.not_canonical"},
	{"indefinite-length array", document + "83008204029f01ff", "serialize.not_canonical"},
	{"no document tag", "d9d9f78201830001f5", "serialize.malformed"},
	{"byte after the document", document + "830001f500", "serialize.malformed"},
	{"tuple of the wrong length", document + "830082078101 82f5f5", "serialize.malformed"},
	{"floating-point number", document + "830002f90000", "serialize.malformed"},
	{"map key twice", document + "83008206028282616101826161 02", "serialize.malformed"},
	{"range no value lies in", document + "830002da74656e01a2018205f5028201f5", "serialize.malformed"},
	{"number outside the window", document + "830002c4821a000f424001", "serialize.malformed"},
	{"error with no diagnostics", document + "820280", "serialize.malformed"},
	{"another format version", "da74656e008202830001f5", "serialize.unsupported_version"},
	{"deep nesting", document + "8300" + strings.Repeat("8204", 600) + "02f6", "serialize.too_large"},
	{"unknown capsule", document + "83008209637 82f79f6", "serialize.unknown_capsule"},
	{"unknown mark", document + "830001da74656e0282f58181617a", "serialize.unknown_mark"},
}

// file is the layout of vectors.json.
type file struct {
	Format   int          `json:"format"`
	About    string       `json:"about"`
	Capsules []entryNote  `json:"capsules"`
	Marks    []entryNote  `json:"marks"`
	Valid    []validOut   `json:"valid"`
	Invalid  []invalidOut `json:"invalid"`
}

type entryNote struct {
	ID    string `json:"id"`
	About string `json:"about"`
}

type validOut struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Hex   string `json:"hex"`
}

type invalidOut struct {
	Name string `json:"name"`
	Hex  string `json:"hex"`
	Code string `json:"code"`
}

// jsonLine returns v as JSON on one line, with <, > and & written as
// themselves, which the corpora's display forms are full of.
func jsonLine(t *testing.T, v any) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// render writes the file, one vector to a line so that a change reads as a
// change to that vector.
func render(t *testing.T, f file) []byte {
	var b bytes.Buffer
	line := func(v any) string { return jsonLine(t, v) }
	b.WriteString("{\n")
	b.WriteString(`  "format": ` + line(f.Format) + ",\n")
	b.WriteString(`  "about": ` + line(f.About) + ",\n")
	section := func(name string, items []string, last bool) {
		b.WriteString(`  "` + name + `": [` + "\n")
		for i, item := range items {
			b.WriteString("    " + item)
			if i < len(items)-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
		b.WriteString("  ]")
		if !last {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	var capsules, marks, valids, invalids []string
	for _, c := range f.Capsules {
		capsules = append(capsules, line(c))
	}
	for _, m := range f.Marks {
		marks = append(marks, line(m))
	}
	for _, v := range f.Valid {
		valids = append(valids, line(v))
	}
	for _, v := range f.Invalid {
		invalids = append(invalids, line(v))
	}
	section("capsules", capsules, false)
	section("marks", marks, false)
	section("valid", valids, false)
	section("invalid", invalids, true)
	b.WriteString("}\n")
	return b.Bytes()
}

func TestConformance_SE001_Vectors(t *testing.T) {
	conformance.Covers(t, "SE-001", "SE-002", "SE-003", "SE-031", "SE-033")
	f := file{
		Format: 1,
		About: "Each valid vector is the one encoding of the value named, whose display form (DI-010) is given in value: a decoder " +
			"accepts it, and encoding what it decodes gives it back. Each invalid vector is input that encodes no " +
			"value, and a decoder refuses it with the code given. The capsules and marks listed are what the vectors " +
			"use, and a decoder is supplied with them.",
		Capsules: []entryNote{{"t/c", "a whole number of degrees, serialized as that number"}},
		Marks: []entryNote{
			{"m", "a mark that propagates, serialized as its identifier alone"},
			{"d", "a deep mark that propagates, serialized as its identifier alone"},
			{"p", "a mark that propagates, serialized with a string, its text"},
			{"r", "a redacting mark that propagates, serialized as its identifier alone"},
		},
	}
	names := map[string]bool{}
	for _, vec := range valid {
		if names[vec.name] {
			t.Fatalf("two vectors are named %q", vec.name)
		}
		names[vec.name] = true
		v := vec.build(rand.New(rand.NewSource(0)))
		b, failure, ok := tenon.Serialize(v)
		if !ok {
			t.Fatalf("%s: %v does not serialize: %v", vec.name, v, failure)
		}
		// However the value is built, it has these bytes.
		for seed := int64(1); seed <= 16; seed++ {
			other := vec.build(rand.New(rand.NewSource(seed)))
			if ob, _, _ := tenon.Serialize(other); !bytes.Equal(ob, b) {
				t.Errorf("%s: built another way, %v encodes as %x, not %x", vec.name, other, ob, b)
			}
		}
		if got, failure, ok := tenon.Deserialize(b, decoders); !ok || !tenon.Identical(got, v) {
			t.Errorf("%s: %x decodes to %v, %v", vec.name, b, got, failure)
		}
		f.Valid = append(f.Valid, validOut{Name: vec.name, Value: v.String(), Hex: hex.EncodeToString(b)})
	}
	for _, vec := range invalid {
		data, err := hex.DecodeString(strings.ReplaceAll(vec.hex, " ", ""))
		if err != nil {
			t.Fatalf("%s: %v", vec.name, err)
		}
		_, failure, ok := tenon.Deserialize(data, decoders)
		if ok || failure.Diagnostics()[0].Code != tenon.Code(vec.code) {
			t.Errorf("%s: %x decodes with %v, want code %s", vec.name, data, failure, vec.code)
		}
		f.Invalid = append(f.Invalid, invalidOut{Name: vec.name, Hex: hex.EncodeToString(data), Code: vec.code})
	}

	want := render(t, f)
	conformance.Emit(t, "vectors.json", want)
	if os.Getenv("TENON_UPDATE_VECTORS") == "1" {
		if err := os.WriteFile("vectors.json", want, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile("vectors.json")
	if err != nil {
		t.Fatalf("reading the corpus: %v; set TENON_UPDATE_VECTORS=1 to write it", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("vectors.json is not what the vectors give; if the change is deliberate, set TENON_UPDATE_VECTORS=1 and review the diff")
	}
}
