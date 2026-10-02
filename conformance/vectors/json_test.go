package vectors_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
)

// esc returns JSON's escape of the code point written in hex, made here so
// that nothing reads an escape in this file's source as its character.
func esc(code string) string { return `\` + "u" + code }

// jsonVector is a JSON text read with a constraint under a policy.
type jsonVector struct {
	name   string
	text   string
	c      tenon.Constraint
	policy tenon.Policy
}

var (
	anyC       = tenon.Any()
	numberList = tenon.ListOf(tenon.Exactly(tenon.NumberType()))
	service    = tenon.ObjectWith(map[string]tenon.Field{
		"name": tenon.Required(tenon.Exactly(tenon.StringType())),
		"port": tenon.Optional(tenon.Exactly(tenon.NumberType())),
		"tags": tenon.Optional(tenon.SetOf(tenon.Exactly(tenon.StringType()))),
	}, true)
)

// jsonVectors are the texts json.json records: what each reads as, or how it
// fails.
var jsonVectors = []jsonVector{
	{"implied/number exactly", `123456789012345678901234567890.5`, anyC, tenon.Safe},
	{"implied/number forms", `[0, -0, 1.50, 1E+2, 2.5e-30, 1e999999]`, anyC, tenon.Safe},
	{"implied/string escapes", `"a\"b\\c\/d\b\f\n\r\t"`, anyC, tenon.Safe},
	{"implied/string code points", `"` + esc("00e9") + "e" + esc("0301") + esc("D83D") + esc("DE00") + "\U00004e16" + `"`, anyC, tenon.Safe},
	{"implied/structures", ` {"b": [1, true, false], "a": {}, "c": [] } `, anyC, tenon.Safe},
	{"implied/null", `null`, anyC, tenon.Safe},
	{"implied/null within", `{"a": [1, null]}`, anyC, tenon.Safe},
	{"constraint/strings to numbers", `["8080", 1]`, numberList, tenon.Unsafe},
	{"constraint/object to map", `{"b": 2, "a": 1}`, tenon.MapOf(anyC), tenon.Safe},
	{"constraint/null beside a number", `{"a": null, "b": 1}`, tenon.MapOf(anyC), tenon.Safe},
	{"constraint/null beside strings, a level down", `[[null], ["x"]]`, tenon.ListOf(tenon.ListOf(anyC)), tenon.Safe},
	{"constraint/object with optional fields", `{"name": "web", "tags": ["b", "a", "b"]}`, service, tenon.Unsafe},
	{"constraint/a type", `[1, "x"]`, tenon.Exactly(tenon.TupleType(tenon.NumberType(), tenon.StringType())), tenon.Safe},
	{"constraint/null alone", `[null]`, tenon.ListOf(anyC), tenon.Safe},
	{"refused/leading zero", `007`, anyC, tenon.Safe},
	{"refused/trailing comma", `[1,]`, anyC, tenon.Safe},
	{"refused/second value", `1 2`, anyC, tenon.Safe},
	{"refused/byte order mark", "\xef\xbb\xbf1", anyC, tenon.Safe},
	{"refused/comment", `[1 /* one */]`, anyC, tenon.Safe},
	{"refused/control character", "\"a\tb\"", anyC, tenon.Safe},
	{"refused/unterminated", `{"a": [1`, anyC, tenon.Safe},
	{"refused/too deep", strings.Repeat("[", 513) + strings.Repeat("]", 513), anyC, tenon.Safe},
	{"refused/ill-formed UTF-8", "[\"a\", \"\xff\"]", anyC, tenon.Safe},
	{"refused/lone surrogate", `{"a": "` + esc("d800") + `x"}`, anyC, tenon.Safe},
	{"refused/out of range", `[1e1000000]`, anyC, tenon.Safe},
	{"refused/name given twice", `{"a": 1, "a": 2}`, tenon.MapOf(anyC), tenon.Safe},
	{"refused/one name spelled twice", `{"x": {"e` + esc("0301") + `": 1, "` + esc("00e9") + `": 2}}`, anyC, tenon.Safe},
	{"refused/empty name", `{"": 1}`, tenon.MapOf(anyC), tenon.Safe},
	{"refused/reading failures together", `[1e1000000, "` + esc("dfff") + `", {"": true}]`, numberList, tenon.Safe},
	{"refused/conversion", `["8080", "x"]`, numberList, tenon.Safe},
}

// jsonFile is the layout of json.json.
type jsonFile struct {
	Format int       `json:"format"`
	About  string    `json:"about"`
	Read   []jsonOut `json:"read"`
}

type jsonOut struct {
	Name       string        `json:"name"`
	Text       string        `json:"text,omitempty"`
	TextHex    string        `json:"textHex"`
	Constraint string        `json:"constraint"`
	Hex        string        `json:"constraintHex"`
	Policy     string        `json:"policy"`
	Value      string        `json:"value,omitempty"`
	ValueHex   string        `json:"valueHex,omitempty"`
	Failures   []jsonFailure `json:"failures,omitempty"`
}

type jsonFailure struct {
	Code string `json:"code"`
	Path string `json:"path"`
}

// TestConformance_JS021_JSONVectors writes json.json: each text read with its
// constraint under its policy, and what it reads as, its display form and its
// encoding, or the failures it gives, each code with its path.
func TestConformance_JS021_JSONVectors(t *testing.T) {
	conformance.Covers(t, "JS-001", "JS-002", "JS-003", "JS-010", "JS-011", "JS-020", "JS-021", "JS-022")
	f := jsonFile{
		Format: 1,
		About: "Each entry is JSON text (textHex, and text where it is well-formed UTF-8) read with a constraint under a " +
			"policy (§11). The constraint is given as the encoding of the pending value of it (constraintHex, SE-010) " +
			"and as its display form. A text that reads gives value, its display form, and valueHex, its encoding; one " +
			"that does not gives the failures, each a code and the display form of its path, in order.",
	}
	names := map[string]bool{}
	for _, vec := range jsonVectors {
		if names[vec.name] {
			t.Fatalf("two vectors are named %q", vec.name)
		}
		names[vec.name] = true
		pending, err := tenon.Serialize(tenon.Pending(vec.c))
		if err != nil {
			t.Fatalf("%s: the constraint %v does not encode: %v", vec.name, vec.c, err)
		}
		out := jsonOut{Name: vec.name, TextHex: hex.EncodeToString([]byte(vec.text)), Constraint: vec.c.String(),
			Hex: hex.EncodeToString(pending), Policy: vec.policy.String()}
		if utf8.ValidString(vec.text) {
			out.Text = vec.text
		}
		v, err := tenon.ParseJSON([]byte(vec.text), vec.c, vec.policy)
		var failed *tenon.Error
		switch {
		case err == nil:
			b, err := tenon.Serialize(v)
			if err != nil {
				t.Fatalf("%s: %v does not encode: %v", vec.name, v, err)
			}
			out.Value, out.ValueHex = v.String(), hex.EncodeToString(b)
		case errors.As(err, &failed):
			for _, d := range failed.Diagnostics() {
				out.Failures = append(out.Failures, jsonFailure{Code: string(d.Code), Path: d.Path.String()})
			}
		default:
			t.Fatalf("%s: ParseJSON failed with %T", vec.name, err)
		}
		if strings.HasPrefix(vec.name, "refused/") != (out.Failures != nil) {
			t.Errorf("%s: %v, %v; the name says whether it is refused", vec.name, v, err)
		}
		f.Read = append(f.Read, out)
	}

	var b bytes.Buffer
	b.WriteString("{\n")
	b.WriteString(`  "format": ` + jsonLine(t, f.Format) + ",\n")
	b.WriteString(`  "about": ` + jsonLine(t, f.About) + ",\n")
	b.WriteString(`  "read": [` + "\n")
	for i, out := range f.Read {
		b.WriteString("    " + jsonLine(t, out))
		if i < len(f.Read)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("  ]\n}\n")
	want := b.Bytes()
	conformance.Emit(t, "json.json", want)
	if os.Getenv("TENON_UPDATE_VECTORS") == "1" {
		if err := os.WriteFile("json.json", want, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile("json.json")
	if err != nil {
		t.Fatalf("reading the corpus: %v; set TENON_UPDATE_VECTORS=1 to write it", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("json.json is not what the vectors give; if the change is deliberate, set TENON_UPDATE_VECTORS=1 and review the diff")
	}
}
