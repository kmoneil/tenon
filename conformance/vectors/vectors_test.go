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

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
)

// The capsule type and marks the vectors use, as vectors.json describes them.
type degrees struct{ n int64 }

var degreesType = tenon.NewCapsule("degrees", tenon.CapsuleOps[degrees]{
	Equal: func(a, b *degrees) bool { return *a == *b },
	Hash:  func(v *degrees) uint64 { return uint64(v.n) },
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
	Capsules: []tenon.Type{degreesType.Type()},
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
	// Two code points that Unicode 15.0.0 leaves apart and later versions
	// compose into one. An implementation normalizing by another version
	// encodes this differently, and refuses this vector as not canonical.
	{"string/composed later", func(*rand.Rand) tenon.Value { return s("\U00011382\U000113C9") }},
	// A run of more than thirty non-starters, which the Stream-Safe Text
	// Process would break up with U+034F and plain UAX #15 normalization
	// leaves alone.
	{"string/long run of marks", func(*rand.Rand) tenon.Value {
		return s("a" + strings.Repeat("́", 31))
	}},
	{"string/controls", func(*rand.Rand) tenon.Value { return s("\x00\t\n\x7f") }},
	{"string/24 bytes", func(*rand.Rand) tenon.Value { return s(strings.Repeat("a", 24)) }},
	{"string/300 bytes", func(*rand.Rand) tenon.Value { return s(strings.Repeat("ab", 150)) }},
	{"null/bool", func(*rand.Rand) tenon.Value { return tenon.Null(boo) }},
	{"null/number", func(*rand.Rand) tenon.Value { return tenon.Null(num) }},
	{"null/string", func(*rand.Rand) tenon.Value { return tenon.Null(str) }},
	{"null/list of numbers", func(*rand.Rand) tenon.Value { return tenon.Null(tenon.ListType(num)) }},
	{"null/capsule", func(*rand.Rand) tenon.Value { return tenon.Null(degreesType.Type()) }},
	{"list/empty", func(*rand.Rand) tenon.Value { return tenon.List(num) }},
	{"list/numbers", func(*rand.Rand) tenon.Value { return tenon.List(num, n(1), n(2), n(3)) }},
	{"list/nested", func(*rand.Rand) tenon.Value {
		return tenon.List(tenon.ListType(num), tenon.List(num, n(1)), tenon.List(num))
	}},
	{"set/numbers", func(r *rand.Rand) tenon.Value { return tenon.Set(num, shuffled(r, n(-1), n(1), n(24), n(1000))...) }},
	{"set/strings given twice", func(r *rand.Rand) tenon.Value {
		return tenon.Set(str, shuffled(r, s("b"), s("a"), s("b"), s(composed(r)), s(composed(r)))...)
	}},
	{"set/unknown members", func(r *rand.Rand) tenon.Value {
		return tenon.Set(num, shuffled(r, n(1), tenon.Unknown(num), tenon.Unknown(num))...)
	}},
	{"set/capsules", func(r *rand.Rand) tenon.Value {
		return tenon.Set(degreesType.Type(), shuffled(r,
			degreesType.Value(&degrees{30}),
			degreesType.Value(&degrees{-5}),
			degreesType.Value(&degrees{1}))...)
	}},
	{"map/empty", func(*rand.Rand) tenon.Value { return tenon.Map(num, nil) }},
	{"map/entries", func(*rand.Rand) tenon.Value {
		return tenon.Map(num, map[string]tenon.Value{"b": n(2), "a": n(1), "": n(0)})
	}},
	{"map/composed key", func(r *rand.Rand) tenon.Value {
		return tenon.Map(str, map[string]tenon.Value{composed(r): s("x")})
	}},
	{"tuple/empty", func(*rand.Rand) tenon.Value { return tenon.Tuple() }},
	{"tuple/mixed", func(r *rand.Rand) tenon.Value {
		return tenon.Tuple(tenon.Bool(true), s("x"), spelled(r, "1.5", "15e-1"))
	}},
	{"object/empty", func(*rand.Rand) tenon.Value { return tenon.Object(nil) }},
	{"object/attributes", func(r *rand.Rand) tenon.Value {
		return tenon.Object(map[string]tenon.Value{"b": n(1), "a": s("x"), composed(r): tenon.Bool(false)})
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
		return tenon.Narrow(tenon.Unknown(tenon.ListType(str)), shuffled(r, tenon.LengthMin(1), tenon.LengthMax(3))...)
	}},
	{"unknown/set members", func(r *rand.Rand) tenon.Value {
		members := one(r,
			[]tenon.Narrowing{tenon.Members(n(1), n(2))},
			[]tenon.Narrowing{tenon.Members(n(2)), tenon.Members(n(1))})
		return tenon.Narrow(tenon.Unknown(tenon.SetType(num)), shuffled(r, append(members, tenon.LengthMax(5))...)...)
	}},
	{"unknown/capsule", func(*rand.Rand) tenon.Value { return tenon.Narrow(tenon.Unknown(degreesType.Type()), tenon.NotNull()) }},
	{"pending/any", func(*rand.Rand) tenon.Value { return tenon.Pending(tenon.Any()) }},
	{"pending/null", func(*rand.Rand) tenon.Value { return tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly()) }},
	{"pending/not null", func(*rand.Rand) tenon.Value {
		return tenon.Narrow(tenon.Pending(tenon.Exactly(degreesType.Type())), tenon.NotNull())
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
		return tenon.WithMarks(tenon.List(num, first, n(2)), deep)
	}},
	{"marks/member", func(r *rand.Rand) tenon.Value {
		return tenon.List(num, n(1), marked(r, n(2), plain, note{"x"}))
	}},
	{"marks/null and unknown", func(r *rand.Rand) tenon.Value {
		return tenon.Tuple(marked(r, tenon.Null(num), plain), marked(r, tenon.Unknown(str), note{"u"}))
	}},
	{"marks/pending", func(r *rand.Rand) tenon.Value { return marked(r, tenon.Pending(tenon.Any()), plain) }},
	{"marks/error", func(r *rand.Rand) tenon.Value {
		return marked(r, tenon.ErrorVal(tenon.Diagnostic{Code: "vectors.failed", Message: "it failed"}), note{"e"}, plain)
	}},
	{"marks/set", func(r *rand.Rand) tenon.Value {
		return tenon.WithMarks(tenon.Set(num, shuffled(r, n(1), n(2))...), deep)
	}},
	{"marks/set holding an unknown", func(r *rand.Rand) tenon.Value {
		return tenon.WithMarks(tenon.Set(num, shuffled(r, n(1), tenon.Unknown(num))...), deep)
	}},
	{"marks/redacted", func(r *rand.Rand) tenon.Value {
		return tenon.Object(map[string]tenon.Value{"password": marked(r, s("hunter2"), secret{}, plain), "user": s("ann")})
	}},
	{"capsule/value", func(*rand.Rand) tenon.Value { return degreesType.Value(&degrees{21}) }},
	// The deepest a document nests: the item, 510 list types and a number
	// are 512 levels, and a mark on the value adds none.
	{"nesting/512 levels", func(*rand.Rand) tenon.Value { return tenon.Null(lists(510)) }},
	{"nesting/marked at 512 levels", func(r *rand.Rand) tenon.Value { return marked(r, tenon.Null(lists(510)), plain) }},
	// What a mark is on is at the mark's level: 510 tuples around a number,
	// the outermost marked, reach 512 levels through their content, and a
	// marked pending value of 510 list constraints around Any through its
	// constraint. Tuples, since a tuple displays without its type, where each
	// list in a nest would spell its own.
	{"nesting/marked content at 512 levels", func(r *rand.Rand) tenon.Value { return marked(r, nested(510), plain) }},
	{"nesting/marked pending at 512 levels", func(r *rand.Rand) tenon.Value {
		return marked(r, tenon.Pending(listsOf(510)), plain)
	}},
}

// lists returns k list types around Number.
func lists(k int) tenon.Type {
	t := num
	for range k {
		t = tenon.ListType(t)
	}
	return t
}

// nested returns k tuples around the number 1, each holding the next.
func nested(k int) tenon.Value {
	v := n(1)
	for range k {
		v = tenon.Tuple(v)
	}
	return v
}

// listsOf returns k list constraints around Any.
func listsOf(k int) tenon.Constraint {
	c := tenon.Any()
	for range k {
		c = tenon.ListOf(c)
	}
	return c
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
	// Input holding two faults fails with the first the reading meets: one
	// the reading stops at, where it is written, and one found by comparing
	// the input with the value's encoding only once it is read through.
	{"an indefinite length, then a malformed value", document + "8300820402 9f f816 ff", "serialize.not_canonical"},
	{"a malformed value, then an indefinite length", document + "8300820401 82 f816 9f ff", "serialize.malformed"},
	{"members out of order, then a malformed value", document + "8300820502 83 01 20 f816", "serialize.malformed"},
	{"members out of order, then an indefinite length", document + "8300820502 83 01 20 9f ff", "serialize.not_canonical"},
	{"an integer in a longer form, then a malformed value", document + "8300820402 82 1801 f816", "serialize.malformed"},
	// A mantissa that is a multiple of ten stops the reading whatever its
	// size, zero among them; a map key the same as one before it once
	// normalized, and a narrowing that does not apply to its type, stop it
	// at the key; and an indefinite length stops it even where range key 0
	// expects true.
	{"a mantissa a multiple of ten, then a malformed value", document + "8300820402 82 c482200a f816", "serialize.not_canonical"},
	{"a mantissa of zero, then a malformed value", document + "8300820402 82 c4820000 f816", "serialize.not_canonical"},
	{"a map key twice once normalized, then a bare bignum", document + "8300820602 82 8262c3a901 826365cc81 c249010000000000000000", "serialize.malformed"},
	{"a narrowing that does not apply, then a bare bignum", document + "830001 da74656e01 a1 01 82 c249010000000000000000 f5", "serialize.malformed"},
	{"range key 0 holding an indefinite length", document + "830002 da74656e01 a1 00 9fff", "serialize.not_canonical"},
	{"two-byte simple value", document + "830001f816", "serialize.malformed"},
	{"no document tag", "d9d9f78201830001f5", "serialize.malformed"},
	{"byte after the document", document + "830001f500", "serialize.malformed"},
	{"tuple of the wrong length", document + "830082078101 82f5f5", "serialize.malformed"},
	{"floating-point number", document + "830002f90000", "serialize.malformed"},
	{"map key twice", document + "83008206028282616101826161 02", "serialize.malformed"},
	{"range no value lies in", document + "830002da74656e01a300f5018205f5028201f5", "serialize.malformed"},
	{"range only null lies in", document + "830002da74656e01a2018205f5028201f5", "serialize.not_canonical"},
	{"number as a bare bignum", document + "830002c249010000000000000000", "serialize.not_canonical"},
	{"decimal fraction whose mantissa is a multiple of ten", document + "830002c48200c249056bc75e2d63100000", "serialize.not_canonical"},
	{"number outside the window", document + "830002c4821a000f424001", "serialize.malformed"},
	{"error with no diagnostics", document + "820280", "serialize.malformed"},
	{"another format version", "da74656e008202830001f5", "serialize.unsupported_version"},
	// The envelope's first element is the version, read first, so a later
	// version is refused as one whatever else its envelope holds.
	{"another format version of another shape", "da74656e00830200f6", "serialize.unsupported_version"},
	{"an envelope with no version", "da74656e0080", "serialize.malformed"},
	{"version 1 of another shape", "da74656e00830183000 1f5f6", "serialize.malformed"},
	{"deep nesting", document + "8300" + strings.Repeat("8204", 600) + "02f6", "serialize.too_large"},
	// One level past the deepest, 513: the item, 511 list types and a number;
	// the same marked, which adds no level; and a mark whose payload's type
	// reaches it, that type sitting two levels below the item, under the
	// content the mark is on.
	{"513 levels", document + "8300" + strings.Repeat("8204", 511) + "02f6", "serialize.too_large"},
	{"marked at 513 levels", document + "8300" + strings.Repeat("8204", 511) + "02da74656e0282f68181616d", "serialize.too_large"},
	{"mark payload at 513 levels", document + "830002da74656e02820181836170" + strings.Repeat("8204", 510) + "0280", "serialize.too_large"},
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
		b, err := tenon.Serialize(v)
		if err != nil {
			t.Fatalf("%s: %v does not serialize: %v", vec.name, v, err)
		}
		// However the value is built, it has these bytes.
		for seed := int64(1); seed <= 16; seed++ {
			other := vec.build(rand.New(rand.NewSource(seed)))
			if ob, _ := tenon.Serialize(other); !bytes.Equal(ob, b) {
				t.Errorf("%s: built another way, %v encodes as %x, not %x", vec.name, other, ob, b)
			}
		}
		if got, err := tenon.Deserialize(b, decoders); err != nil || !tenon.Identical(got, v) {
			t.Errorf("%s: %x decodes to %v, %v", vec.name, b, got, err)
		}
		f.Valid = append(f.Valid, validOut{Name: vec.name, Value: v.String(), Hex: hex.EncodeToString(b)})
	}
	for _, vec := range invalid {
		data, err := hex.DecodeString(strings.ReplaceAll(vec.hex, " ", ""))
		if err != nil {
			t.Fatalf("%s: %v", vec.name, err)
		}
		_, err = tenon.Deserialize(data, decoders)
		if te, ok := err.(*tenon.Error); !ok || te.Diagnostics()[0].Code != tenon.Code(vec.code) {
			t.Errorf("%s: %x decodes with %v, want code %s", vec.name, data, err, vec.code)
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
