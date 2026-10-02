package tenon_test

import (
	"encoding/json"
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/internal/conformance/values"
)

// parsed reads doc with c under p, giving the value, or the error value its
// *Error holds.
func parsed(t *testing.T, doc string, c tenon.Constraint, p tenon.Policy) tenon.Value {
	t.Helper()
	v, err := tenon.ParseJSON([]byte(doc), c, p)
	if err == nil {
		return v
	}
	var e *tenon.Error
	if !errors.As(err, &e) {
		t.Fatalf("ParseJSON(%q) failed with %T, want a *tenon.Error", doc, err)
	}
	return e.Value()
}

// pendingNull is what a JSON null is read as with no type given.
func pendingNull() tenon.Value { return tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly()) }

// jsonEsc returns doc with each ^u written as JSON's escape, a reverse solidus
// and u, which the documents below write that way so that nothing reads an
// escape in this file's source as the character it names.
func jsonEsc(doc string) string { return strings.ReplaceAll(doc, "^u", `\`+"u") }

// holdsCapsule reports whether t is, or holds, a capsule type.
func holdsCapsule(t tenon.Type) bool {
	switch t.Kind() {
	case tenon.KindCapsule:
		return true
	case tenon.KindList, tenon.KindSet, tenon.KindMap:
		return holdsCapsule(t.ElementType())
	case tenon.KindTuple:
		for _, e := range t.TupleElementTypes() {
			if holdsCapsule(e) {
				return true
			}
		}
	case tenon.KindObject:
		for _, name := range t.AttributeNames() {
			if holdsCapsule(t.AttributeType(name)) {
				return true
			}
		}
	}
	return false
}

// holdsSet reports whether t is, or holds, a set type.
func holdsSet(t tenon.Type) bool {
	switch t.Kind() {
	case tenon.KindSet:
		return true
	case tenon.KindList, tenon.KindMap:
		return holdsSet(t.ElementType())
	case tenon.KindTuple:
		for _, e := range t.TupleElementTypes() {
			if holdsSet(e) {
				return true
			}
		}
	case tenon.KindObject:
		for _, name := range t.AttributeNames() {
			if holdsSet(t.AttributeType(name)) {
				return true
			}
		}
	}
	return false
}

// TestConformance_JS001_AProjectionReadsBack holds every known value of the
// generator that holds no capsule, carries no mark and holds no map with an
// empty key to reading back, with its own type, as itself from its JSON
// projection: under the unsafe policy, and under the safe one where it holds
// no set, which a JSON array becomes only unsafely.
func TestConformance_JS001_AProjectionReadsBack(t *testing.T) {
	conformance.Covers(t, "JS-001", "SE-060")
	read := 0
	for _, v := range values.All() {
		if !v.IsKnown() || holdsCapsule(v.Type()) || holdsEmptyKey(v) {
			continue
		}
		if plain, _ := tenon.UnmarkDeep(v); !tenon.Identical(plain, v) {
			continue
		}
		text, err := tenon.ProjectJSON(v)
		if err != nil {
			t.Errorf("ProjectJSON(%v) failed: %v", v, err)
			continue
		}
		read++
		for _, p := range []tenon.Policy{tenon.Unsafe, tenon.Safe} {
			if p == tenon.Safe && holdsSet(v.Type()) {
				continue
			}
			if got := parsed(t, string(text), tenon.Exactly(v.Type()), p); !tenon.Identical(got, v) {
				t.Errorf("%v projected as %s reads back under the %v policy as %v", v, text, p, got)
			}
		}
	}
	if read < 40 {
		t.Errorf("read back %d values; want many", read)
	}
	// A map's empty key is projected as a name, which no object can have.
	m := tenon.Map(tenon.NumberType(), map[string]tenon.Value{"": n(1)})
	text, err := tenon.ProjectJSON(m)
	if err != nil {
		t.Fatal(err)
	}
	wantErrors(t, "a map with an empty key", parsed(t, string(text), tenon.Exactly(m.Type()), tenon.Unsafe),
		wantDiag{tenon.CodeObjectEmptyName, "."})
}

// TestConformance_JS002_TheTextIsJSONStrictly holds the reader to RFC 8259's
// grammar: what it allows reads, and what it does not, tenon's own number
// syntax among it, fails as json.invalid_syntax.
func TestConformance_JS002_TheTextIsJSONStrictly(t *testing.T) {
	conformance.Covers(t, "JS-002", "JS-003")
	for _, doc := range []string{
		`0`, `-0`, `1.5e+10`, `1E-2`, `""`, `"\/"`, ` [ ] `, "\t{ }\r\n", `true`, `false`, `null`,
		`[1,[2,[3]],{"a":{}}]`, jsonEsc(`"^u00e9"`),
	} {
		if v := parsed(t, doc, tenon.Any(), tenon.Safe); v.IsError() {
			t.Errorf("%q: %v, want it read", doc, v)
		}
	}
	for _, doc := range []string{
		``, ` `, `007`, `01`, `-`, `+1`, `.5`, `1.`, `1e`, `0x10`, `NaN`, `Infinity`, `[1,]`, `{"a":1,}`,
		`{a:1}`, `{'a':1}`, `'a'`, `[1 2]`, `{"a" 1}`, `{"a":}`, `[`, `{`, `"abc`, `"\x"`, `"\u12"`,
		`tru`, `nul`, `True`, `// c` + "\n1", `/* c */ 1`, "\xef\xbb\xbf1", `1 2`, `[] []`, "\"a\x01\"",
		"\"\t\"",
	} {
		v := parsed(t, doc, tenon.Any(), tenon.Unsafe)
		if !v.IsError() || len(v.Diagnostics()) != 1 || v.Diagnostics()[0].Code != tenon.CodeJSONInvalidSyntax {
			t.Errorf("%q: %v, want one json.invalid_syntax", doc, v)
		}
	}
}

// TestConformance_JS003_WhereTheReadingStops holds a failure of the text to
// one diagnostic, located at the member being read and naming the byte
// offset, after which nothing is read: a failure later in the text is not
// reported. Arrays and objects nest 512 levels deep and no deeper.
func TestConformance_JS003_WhereTheReadingStops(t *testing.T) {
	conformance.Covers(t, "JS-003")
	wantErrors(t, "a missing value", parsed(t, `{"a": [1, 2, ]}`, tenon.Any(), tenon.Unsafe),
		wantDiag{tenon.CodeJSONInvalidSyntax, ".a[2]"})
	if v := parsed(t, `{"a": [1, 2, ]}`, tenon.Any(), tenon.Unsafe); !strings.Contains(v.Diagnostics()[0].Message, "at byte 13") {
		t.Errorf("%v, want the message to name byte 13", v)
	}
	wantErrors(t, "a failure before it, and one after", parsed(t, `[1e1000000, }, "\ud800"]`, tenon.Any(), tenon.Unsafe),
		wantDiag{tenon.CodeJSONInvalidSyntax, ".[1]"})

	for _, open := range []string{"[", `{"a":`} {
		close := map[string]string{"[": "]", `{"a":`: "}"}[open]
		deep := strings.Repeat(open, 512) + "1" + strings.Repeat(close, 512)
		if v := parsed(t, deep, tenon.Any(), tenon.Unsafe); v.IsError() {
			t.Errorf("512 levels of %s: %v, want it read", open, v)
		}
		v := parsed(t, strings.Repeat(open, 513)+"1"+strings.Repeat(close, 513), tenon.Any(), tenon.Unsafe)
		if !v.IsError() || len(v.Diagnostics()) != 1 || v.Diagnostics()[0].Code != tenon.CodeJSONTooDeep || v.Diagnostics()[0].Path.Len() != 512 {
			t.Errorf("513 levels of %s: %v, want one json.too_deep 512 steps down", open, v)
		}
	}
}

// TestConformance_JS004_ReadingGrowsWithTheText holds reading to never
// panicking, which FuzzParseJSON holds over every input, and to work in
// proportion to the text however it is shaped: wide, deep, long strings,
// many names and many failures, each at a size and four times it.
func TestConformance_JS004_ReadingGrowsWithTheText(t *testing.T) {
	conformance.Covers(t, "JS-004")
	shapes := map[string]func(n int) string{
		"wide": func(n int) string {
			return "[" + strings.TrimSuffix(strings.Repeat(`1.25,"x",true,null,`, n), ",") + "]"
		},
		"deep": func(n int) string {
			n /= 32
			return strings.Repeat(`{"a":[1,`, n) + "2" + strings.Repeat("]}", n)
		},
		"long strings": func(n int) string { return `["` + strings.Repeat(jsonEsc(`e^u0301\n`), n) + `"]` },
		"many names": func(n int) string {
			var b strings.Builder
			b.WriteString("{")
			for i := range n {
				if i > 0 {
					b.WriteString(",")
				}
				b.WriteString(`"name`)
				b.WriteString(strings.Repeat("x", i%7))
				b.WriteString(string(rune('a' + i%26)))
				b.WriteString(`":1`)
			}
			b.WriteString("}")
			return b.String()
		},
		"many failures": func(n int) string { return "[" + strings.TrimSuffix(strings.Repeat(`1e1000000,`, n), ",") + "]" },
	}
	allocated := func(doc []byte) uint64 {
		const rounds = 3
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		for range rounds {
			_, _ = tenon.ParseJSON(doc, tenon.Any(), tenon.Unsafe)
		}
		runtime.ReadMemStats(&after)
		return after.TotalAlloc - before.TotalAlloc
	}
	for name, shape := range shapes {
		a, b := allocated([]byte(shape(2000))), allocated([]byte(shape(8000)))
		if grew := float64(b) / float64(a); grew > 5 {
			t.Errorf("%s: four times the text allocated %.2f times the bytes (%d, then %d)", name, grew, a, b)
		}
	}
}

// TestConformance_JS010_StringsAreTheirCharacters holds a string to the
// characters it writes, its escapes and escaped pairs decoded and the result
// normalized, and to failing where it is not well-formed: no replacement
// character stands in.
func TestConformance_JS010_StringsAreTheirCharacters(t *testing.T) {
	conformance.Covers(t, "JS-010", "ST-002", "ST-004")
	for doc, want := range map[string]string{
		`"a\"b\\c\/d\b\f\n\r\t"`:  "a\"b\\c/d\b\f\n\r\t",
		jsonEsc(`"^u00e9^u0041"`): "\U000000e9A",
		jsonEsc(`"e^u0301"`):      "\U000000e9",
		jsonEsc(`"^ud83d^ude00"`): "\U0001f600",
		jsonEsc(`"^uD83D^uDE00"`): "\U0001f600",
		"\"caf\xc3\xa9\"":         "caf\U000000e9",
	} {
		wantValue(t, doc, parsed(t, doc, tenon.Any(), tenon.Safe), tenon.String(want))
	}
	for doc, at := range map[string]string{
		`"\ud800"`:                ".",
		`"\ude00x"`:               ".",
		jsonEsc(`"^ud800^u0041"`): ".",
		`["a", "\udbff"]`:         ".[1]",
		"[\"a\", \"\xff\"]":       ".[1]",
		"{\"\xff\": 1}":           ".",
		`{"a": {"\ud800": 1}}`:    ".a",
	} {
		wantErrors(t, doc, parsed(t, doc, tenon.Any(), tenon.Safe), wantDiag{tenon.CodeStringInvalidUTF8, at})
	}
}

// TestConformance_JS011_NumbersAreExact holds a number to the one its text
// writes, exactly, however long, and to failing where tenon's range or its
// limit on a number's text does not hold it, located at the number.
func TestConformance_JS011_NumbersAreExact(t *testing.T) {
	conformance.Covers(t, "JS-011", "NU-016", "NU-024")
	for doc, want := range map[string]string{
		`123456789012345678901234567890123456789`: "123456789012345678901234567890123456789",
		`0.1`:       "0.1",
		`1.50`:      "1.5",
		`-0`:        "0",
		`1E+2`:      "100",
		`1e-999999`: "1e-999999",
	} {
		wantValue(t, doc, parsed(t, doc, tenon.Any(), tenon.Safe), tenon.NumberFromText(want))
	}
	wantErrors(t, "beyond the range", parsed(t, `{"a": [1, 1e1000000]}`, tenon.Any(), tenon.Safe),
		wantDiag{tenon.CodeNumberOutOfRange, ".a[1]"})
	wantErrors(t, "too long", parsed(t, "["+strings.Repeat("1", 10001)+"]", tenon.Any(), tenon.Safe),
		wantDiag{tenon.CodeNumberTooLong, ".[0]"})
}

// TestConformance_JS020_TheTypesJSONImplies holds a document read with Any to
// what JSON implies: arrays as tuples, objects as objects, null as the pending
// null, a container holding one the pending tuple or object holding it; and
// an object's names to Object's rules, a name given twice failing as two
// spellings of one name do, at the object, whatever it would become.
func TestConformance_JS020_TheTypesJSONImplies(t *testing.T) {
	conformance.Covers(t, "JS-020", "TY-018", "UN-024", "UN-025")
	for _, tt := range []struct {
		doc  string
		want tenon.Value
	}{
		{`[1, "a", true]`, tenon.Tuple(n(1), s("a"), tenon.Bool(true))},
		{`[]`, tenon.Tuple()},
		{`{}`, obj(nil)},
		{`{"b": 1, "a": [false]}`, obj(map[string]tenon.Value{"a": tenon.Tuple(tenon.Bool(false)), "b": n(1)})},
		{`null`, pendingNull()},
		{`[1, null]`, tenon.Tuple(n(1), pendingNull())},
		{`{"a": {"b": null}}`, obj(map[string]tenon.Value{"a": obj(map[string]tenon.Value{"b": pendingNull()})})},
	} {
		wantValue(t, tt.doc, parsed(t, tt.doc, tenon.Any(), tenon.Safe), tt.want)
	}
	for _, c := range []tenon.Constraint{tenon.Any(), tenon.MapOf(tenon.Any())} {
		wantErrors(t, "a name given twice", parsed(t, `{"a": 1, "b": 2, "a": 3}`, c, tenon.Unsafe),
			wantDiag{tenon.CodeObjectDuplicateName, "."})
		wantErrors(t, "two spellings of one name", parsed(t, jsonEsc(`{"x": {"e^u0301": 1, "^u00e9": 2}}`), c, tenon.Unsafe),
			wantDiag{tenon.CodeObjectDuplicateName, ".x"})
		wantErrors(t, "an empty name", parsed(t, `{"": 1}`, c, tenon.Unsafe),
			wantDiag{tenon.CodeObjectEmptyName, "."})
	}
}

// TestConformance_JS021_UnderAConstraintTheReadingIsConverted holds reading
// with a constraint to converting what JSON implies to it under the policy,
// identical, failures and all: a string read into a number under one policy
// and not the other, an array into a list or a set, an object into a map, and
// a null into the null of its type or of its siblings'.
func TestConformance_JS021_UnderAConstraintTheReadingIsConverted(t *testing.T) {
	conformance.Covers(t, "JS-021", "CV-021", "CV-032")
	num := tenon.NumberType()
	for _, tt := range []struct {
		doc  string
		c    tenon.Constraint
		p    tenon.Policy
		want tenon.Value
	}{
		{`["8080"]`, tenon.ListOf(is(num)), tenon.Unsafe, tenon.List(num, n(8080))},
		{`{"a": 1, "b": 2}`, tenon.MapOf(is(num)), tenon.Safe, tenon.Map(num, map[string]tenon.Value{"a": n(1), "b": n(2)})},
		{`[2, 1, 2]`, tenon.SetOf(tenon.Any()), tenon.Unsafe, tenon.Set(num, n(1), n(2))},
		{`{"a": null, "b": 1}`, tenon.MapOf(tenon.Any()), tenon.Safe, tenon.Map(num, map[string]tenon.Value{"a": tenon.Null(num), "b": n(1)})},
		{`null`, is(num), tenon.Safe, tenon.Null(num)},
		{`{"port": null}`, tenon.ObjectWith(map[string]tenon.Field{"port": tenon.Required(is(num))}, true), tenon.Safe,
			obj(map[string]tenon.Value{"port": tenon.Null(num)})},
	} {
		wantValue(t, tt.doc, parsed(t, tt.doc, tt.c, tt.p), tt.want)
	}
	for _, tt := range []struct {
		doc string
		c   tenon.Constraint
		p   tenon.Policy
	}{
		{`["8080"]`, tenon.ListOf(is(num)), tenon.Safe},
		{`[1, 2]`, tenon.SetOf(is(num)), tenon.Safe},
		{`{"a": "x"}`, tenon.ObjectWith(map[string]tenon.Field{"b": tenon.Required(tenon.Any())}, true), tenon.Unsafe},
	} {
		var implied tenon.Value
		if v, err := tenon.ParseJSON([]byte(tt.doc), tenon.Any(), tt.p); err == nil {
			implied = v
		}
		wantValue(t, tt.doc, parsed(t, tt.doc, tt.c, tt.p), tenon.Convert(implied, tt.c, tt.p))
	}
}

// TestConformance_JS022_ReadingFailuresComeTogether holds the failures of
// reading the text to coming together, every one, in the order of the text,
// an object's names where it ends, and alone: a part that reads is not
// converted, so a conversion's failure beside them is not reported.
func TestConformance_JS022_ReadingFailuresComeTogether(t *testing.T) {
	conformance.Covers(t, "JS-022")
	doc := `[1e1000000, "\ud800", {"": 1, "b": "x"}, {"z": [1e1000000], "a": 1, "a": 2}]`
	wantErrors(t, "four failures", parsed(t, doc, tenon.ListOf(tenon.Exactly(tenon.NumberType())), tenon.Safe),
		wantDiag{tenon.CodeNumberOutOfRange, ".[0]"},
		wantDiag{tenon.CodeStringInvalidUTF8, ".[1]"},
		wantDiag{tenon.CodeObjectEmptyName, ".[2]"},
		wantDiag{tenon.CodeNumberOutOfRange, ".[3].z[0]"},
		wantDiag{tenon.CodeObjectDuplicateName, ".[3]"})
}

// FuzzParseJSON holds the reader to never panicking, and to reading JSON
// exactly as encoding/json judges it valid: whatever it reads, encoding/json
// accepts, and whatever encoding/json accepts it fails with nothing but its
// own refusals (JS-010, JS-011, JS-020), and json.too_deep only where the
// text nests more than 512 levels. The seed corpus runs with the tests;
// `make fuzz` runs the fuzzer itself.
func FuzzParseJSON(f *testing.F) {
	for _, doc := range []string{
		`{"a": [1, "x", true, null], "b": {"c": 1.5e2}}`, `[]`, jsonEsc(`"^ud83d^ude00"`), jsonEsc(`"^ud800"`), `{"a":1,"a":2}`,
		`007`, `[1,]`, "\"\xff\"", `1e1000000`, `{"": 1}`, strings.Repeat("[", 600) + strings.Repeat("]", 600),
	} {
		f.Add([]byte(doc))
	}
	for _, v := range values.All() {
		if text, err := tenon.ProjectJSON(v); err == nil {
			f.Add(text)
		}
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		_, err := tenon.ParseJSON(data, tenon.Any(), tenon.Unsafe)
		valid := json.Valid(data)
		if err == nil {
			if !valid {
				t.Fatalf("ParseJSON(%q) read text encoding/json refuses", data)
			}
			return
		}
		var e *tenon.Error
		if !errors.As(err, &e) {
			t.Fatalf("ParseJSON(%q) failed with %T", data, err)
		}
		for _, d := range e.Value().Diagnostics() {
			switch d.Code {
			case tenon.CodeJSONInvalidSyntax:
				if valid {
					t.Fatalf("ParseJSON(%q) refuses as not JSON text encoding/json accepts: %v", data, d)
				}
			case tenon.CodeJSONTooDeep:
				if strings.Count(string(data), "[")+strings.Count(string(data), "{") <= 512 {
					t.Fatalf("ParseJSON(%q) refuses as too deep text that cannot nest so deep: %v", data, d)
				}
			case tenon.CodeStringInvalidUTF8, tenon.CodeNumberOutOfRange, tenon.CodeNumberTooLong,
				tenon.CodeObjectEmptyName, tenon.CodeObjectDuplicateName:
				if !valid {
					t.Fatalf("ParseJSON(%q) read on through text encoding/json refuses, failing with %v", data, d)
				}
			default:
				t.Fatalf("ParseJSON(%q) failed with %v, which reading with Any never gives", data, d)
			}
		}
	})
}
