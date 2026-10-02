package tenon_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/internal/conformance/values"
)

// randomJSON returns random JSON text nested at most depth deep, every form
// of value among it: numbers written each way JSON allows, strings holding
// every escape and characters well beyond ASCII, arrays, and objects whose
// names are distinct after normalization, with whitespace scattered between
// tokens. It holds no duplicate name, nothing ill-formed and no number
// beyond tenon's range, so that encoding/json reads it as tenon does.
func randomJSON(r *rand.Rand, depth int) string {
	var b strings.Builder
	space := func() {
		if r.Intn(4) == 0 {
			b.WriteString([]string{" ", "\n", "\t", "\r\n  "}[r.Intn(4)])
		}
	}
	var value func(depth int)
	value = func(depth int) {
		space()
		k := r.Intn(9)
		if depth == 0 && k >= 7 {
			k = r.Intn(7)
		}
		switch k {
		case 0, 1:
			b.WriteString([]string{"0", "-0", "7", "-12", "1.50", "1e2", "1E+2", "-3.25e-7", "0.1",
				"123456789012345678901234567890", "2.5E-30", "1e999999", "-0.000"}[r.Intn(13)])
		case 2, 3:
			b.WriteString(jsonString(r))
		case 4:
			b.WriteString([]string{"true", "false"}[r.Intn(2)])
		case 5:
			b.WriteString("null")
		case 6:
			b.WriteString([]string{"[]", "{}"}[r.Intn(2)])
		case 7:
			b.WriteString("[")
			for i := range 1 + r.Intn(4) {
				if i > 0 {
					space()
					b.WriteString(",")
				}
				value(depth - 1)
			}
			space()
			b.WriteString("]")
		default:
			b.WriteString("{")
			names := []string{`"a"`, `"b"`, `"name"`, `"x y"`, "", `"z\n"`}
			names[4] = `"` + `\` + "u00e9" + `"` // an escape, made here so that no tool reads it as its character
			r.Shuffle(len(names), func(i, j int) { names[i], names[j] = names[j], names[i] })
			for i := range 1 + r.Intn(4) {
				if i > 0 {
					space()
					b.WriteString(",")
				}
				space()
				b.WriteString(names[i])
				space()
				b.WriteString(":")
				value(depth - 1)
			}
			space()
			b.WriteString("}")
		}
		space()
	}
	value(depth)
	return b.String()
}

// jsonString returns a random JSON string: plain characters, characters well
// beyond ASCII written as themselves and as escapes, a pair escaping a
// character beyond the Basic Multilingual Plane, a combining mark that
// normalization joins to the letter before it, and the short escapes.
func jsonString(r *rand.Rand) string {
	var b strings.Builder
	b.WriteString(`"`)
	for range r.Intn(6) {
		switch r.Intn(7) {
		case 0:
			b.WriteString([]string{"a", "Z", "0", " ", "-"}[r.Intn(5)])
		case 1:
			b.WriteString([]string{"\U000000e9", "\U00004e16", "\U0001f600", "\U00000301"}[r.Intn(4)])
		case 2:
			c := []rune{0x00e9, 0x4e16, 0x0041, 0x0301, 0x0007}[r.Intn(5)]
			b.WriteString(fmt.Sprintf(`\`+"u%04x", c))
		case 3:
			hi, lo := utf16.EncodeRune(0x1f600 + rune(r.Intn(16)))
			b.WriteString(fmt.Sprintf(`\`+"u%04X"+`\`+"u%04x", hi, lo))
		case 4:
			b.WriteString([]string{`\"`, `\\`, `\/`, `\b`, `\f`, `\n`, `\r`, `\t`}[r.Intn(8)])
		case 5:
			b.WriteString("e")
			b.WriteString(fmt.Sprintf(`\`+"u%04x", 0x0301))
		default:
			b.WriteString("x")
		}
	}
	b.WriteString(`"`)
	return b.String()
}

// implied returns what JSON implies of x, encoding/json's reading with
// UseNumber: the reference ParseJSON's reading with Any is held to (JS-020).
func implied(t *testing.T, x any) tenon.Value {
	t.Helper()
	switch x := x.(type) {
	case nil:
		return pendingNull()
	case bool:
		return tenon.Bool(x)
	case json.Number:
		return tenon.NumberFromText(string(x))
	case string:
		return tenon.String(x)
	case []any:
		vals := make([]tenon.Value, len(x))
		for i, m := range x {
			vals[i] = implied(t, m)
		}
		return tenon.Tuple(vals...)
	case map[string]any:
		attrs := make(map[string]tenon.Value, len(x))
		for name, m := range x {
			attrs[name] = implied(t, m)
		}
		return tenon.Object(attrs)
	}
	t.Fatalf("encoding/json gave %T", x)
	return tenon.Value{}
}

// decoded returns encoding/json's reading of doc with UseNumber.
func decoded(t *testing.T, doc string) any {
	t.Helper()
	d := json.NewDecoder(bytes.NewReader([]byte(doc)))
	d.UseNumber()
	var x any
	if err := d.Decode(&x); err != nil {
		t.Fatalf("encoding/json refuses %q: %v", doc, err)
	}
	return x
}

// TestConformance_JS020_RandomTextReadsAsEncodingJSONReadsIt holds the reader
// to reading random JSON text, every form of number, string and escape among
// it, exactly as encoding/json reads it, made into what JSON implies: the
// same numbers to the last digit, the same characters, normalized, and the
// same structure.
func TestConformance_JS020_RandomTextReadsAsEncodingJSONReadsIt(t *testing.T) {
	conformance.Covers(t, "JS-020", "JS-010", "JS-011", "JS-002")
	r := rand.New(rand.NewSource(20261002))
	for range conformance.Iterations(t, 2000) {
		doc := randomJSON(r, 3)
		want := implied(t, decoded(t, doc))
		if got := parsed(t, doc, tenon.Any(), tenon.Safe); !tenon.Identical(got, want) {
			t.Fatalf("ParseJSON(%q) = %v, where encoding/json reads %v", doc, got, want)
		}
	}
}

// TestConformance_JS021_RandomTextUnderAConstraintIsConverted holds reading
// random JSON text with a random constraint, under either policy, to
// converting what encoding/json reads, made into what JSON implies, to that
// constraint: identical, failures and all.
func TestConformance_JS021_RandomTextUnderAConstraintIsConverted(t *testing.T) {
	conformance.Covers(t, "JS-021", "CV-021", "CV-032")
	r := rand.New(rand.NewSource(20261003))
	converted := 0
	for range conformance.Iterations(t, 2000) {
		doc := randomJSON(r, 3)
		c := values.RandomConstraint(r, 3, degrees.Type())
		p := []tenon.Policy{tenon.Safe, tenon.Unsafe}[r.Intn(2)]
		want := tenon.Convert(implied(t, decoded(t, doc)), c, p)
		got := parsed(t, doc, c, p)
		if !tenon.Identical(got, want) {
			t.Fatalf("ParseJSON(%q, %v, %v) = %v, where converting what encoding/json reads gives %v", doc, c, p, got, want)
		}
		if !got.IsError() {
			converted++
		}
	}
	if converted < 200 {
		t.Errorf("%d documents read into their constraint; want many", converted)
	}
}

// holdsEmptyKey reports whether v is, or holds, a map with an empty key,
// whose projection names the empty string, which no object can (JS-020).
func holdsEmptyKey(v tenon.Value) bool {
	if !v.IsKnown() || v.IsNull() {
		return false
	}
	switch v.Type().Kind() {
	case tenon.KindMap:
		for key, e := range v.MapEntries() {
			if key == "" || holdsEmptyKey(e) {
				return true
			}
		}
	case tenon.KindList, tenon.KindSet, tenon.KindTuple:
		for e := range v.ElementsSeq() {
			if holdsEmptyKey(e) {
				return true
			}
		}
	case tenon.KindObject:
		for _, e := range v.Attributes() {
			if holdsEmptyKey(e) {
				return true
			}
		}
	}
	return false
}

// TestConformance_JS001_RandomValuesReadBack holds random values of random
// types, nulls among their parts, to reading back as themselves from their
// projection with their own type: under the unsafe policy, and under the
// safe one where they hold no set. A value is left out where it holds a
// capsule, a mark, a value not known, or a map with an empty key.
func TestConformance_JS001_RandomValuesReadBack(t *testing.T) {
	conformance.Covers(t, "JS-001", "SE-060", "SE-062")
	g := generator{rand.New(rand.NewSource(20261004))}
	read := 0
	for range conformance.Iterations(t, 3000) {
		typ := g.typ(3)
		if holdsCapsule(typ) {
			continue
		}
		v, _ := tenon.UnmarkDeep(g.value(typ, 3, false))
		if !v.IsKnown() || holdsEmptyKey(v) {
			continue
		}
		text, err := tenon.ProjectJSON(v)
		if err != nil {
			t.Fatalf("ProjectJSON(%v) failed: %v", v, err)
		}
		read++
		for _, p := range []tenon.Policy{tenon.Unsafe, tenon.Safe} {
			if p == tenon.Safe && holdsSet(typ) {
				continue
			}
			if got := parsed(t, string(text), tenon.Exactly(typ), p); !tenon.Identical(got, v) {
				t.Fatalf("%v projected as %s reads back under the %v policy as %v", v, text, p, got)
			}
		}
	}
	if read < 500 {
		t.Errorf("read back %d values; want many", read)
	}
}
