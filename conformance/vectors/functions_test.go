package vectors_test

import (
	"bytes"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/stdlib"
)

// library is the standard library's functions, by the names the
// specification gives them.
var library = map[string]tenon.Function{
	"AssertNotNull": stdlib.AssertNotNullFunc,
}

// callVector is a call of a library function with arguments under a policy.
type callVector struct {
	name     string
	function string
	args     []tenon.Value
	policy   tenon.Policy
}

// callVectors are the calls functions.json records: what each answers, or
// how it fails. A name begins with the function's and, for a call that
// fails, "refused/" after it.
var callVectors = []callVector{
	{"AssertNotNull/a number", "AssertNotNull", []tenon.Value{n(1)}, tenon.Safe},
	{"AssertNotNull/marks", "AssertNotNull", []tenon.Value{tenon.WithMarks(s("hunter2"), secret{}, plain)}, tenon.Safe},
	{"AssertNotNull/a tuple holding a null", "AssertNotNull", []tenon.Value{tenon.Tuple(tenon.Null(str), n(1))}, tenon.Safe},
	{"AssertNotNull/unknown", "AssertNotNull", []tenon.Value{tenon.Unknown(str)}, tenon.Safe},
	{"AssertNotNull/unknown with a prefix", "AssertNotNull", []tenon.Value{tenon.Narrow(tenon.Unknown(str), tenon.StringPrefix("abc"))}, tenon.Safe},
	{"AssertNotNull/pending", "AssertNotNull", []tenon.Value{tenon.Pending(tenon.ListOf(tenon.Any()))}, tenon.Safe},
	{"AssertNotNull/refused/null", "AssertNotNull", []tenon.Value{tenon.Null(str)}, tenon.Safe},
	{"AssertNotNull/refused/pending null", "AssertNotNull", []tenon.Value{tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly())}, tenon.Safe},
	{"AssertNotNull/refused/redacted null", "AssertNotNull", []tenon.Value{tenon.WithMarks(tenon.Null(str), secret{})}, tenon.Safe},
	{"AssertNotNull/refused/arity", "AssertNotNull", []tenon.Value{n(1), n(2)}, tenon.Safe},
}

// callFile is the layout of functions.json.
type callFile struct {
	Format int       `json:"format"`
	About  string    `json:"about"`
	Calls  []callOut `json:"calls"`
}

type callOut struct {
	Name     string        `json:"name"`
	Function string        `json:"function"`
	Args     []callArg     `json:"args"`
	Policy   string        `json:"policy"`
	Value    string        `json:"value,omitempty"`
	ValueHex string        `json:"valueHex,omitempty"`
	Failures []jsonFailure `json:"failures,omitempty"`
}

type callArg struct {
	Value string `json:"value"`
	Hex   string `json:"hex"`
}

// TestConformance_LB001_FunctionVectors writes functions.json: each call of a
// library function, its arguments' display forms and encodings, and what it
// answers, its display form and its encoding, or the failures it gives, each
// code with its path.
func TestConformance_LB001_FunctionVectors(t *testing.T) {
	conformance.Covers(t, "LB-001", "LN-083")
	f := callFile{
		Format: 1,
		About: "Each entry is a call of a function of the standard library (§13 to §20), named as the specification " +
			"names it, with arguments, each given as its display form and its encoding (args), under a policy. A call " +
			"that answers gives value, its display form, and valueHex, its encoding; one that fails gives the " +
			"failures, each a code and the display form of its path, in order. The marks the arguments carry are " +
			"those vectors.json lists.",
	}
	names := map[string]bool{}
	called := map[string]bool{}
	for _, vec := range callVectors {
		if names[vec.name] {
			t.Fatalf("two vectors are named %q", vec.name)
		}
		names[vec.name] = true
		fn, ok := library[vec.function]
		if !ok || !strings.HasPrefix(vec.name, vec.function+"/") {
			t.Fatalf("%s: no library function %q, or the name does not begin with it", vec.name, vec.function)
		}
		called[vec.function] = true
		out := callOut{Name: vec.name, Function: vec.function, Policy: vec.policy.String()}
		for _, a := range vec.args {
			b, err := tenon.Serialize(a)
			if err != nil {
				t.Fatalf("%s: the argument %v does not encode: %v", vec.name, a, err)
			}
			out.Args = append(out.Args, callArg{Value: a.String(), Hex: hex.EncodeToString(b)})
		}
		r := tenon.Call(fn, vec.args, vec.policy)
		if r.IsError() {
			for _, d := range r.Diagnostics() {
				out.Failures = append(out.Failures, jsonFailure{Code: string(d.Code), Path: d.Path.String()})
			}
		} else {
			b, err := tenon.Serialize(r)
			if err != nil {
				t.Fatalf("%s: %v does not encode: %v", vec.name, r, err)
			}
			out.Value, out.ValueHex = r.String(), hex.EncodeToString(b)
		}
		if strings.Contains(vec.name, "/refused/") != (out.Failures != nil) {
			t.Errorf("%s: %v; the name says whether it is refused", vec.name, r)
		}
		f.Calls = append(f.Calls, out)
	}
	for name := range library {
		if !called[name] {
			t.Errorf("no vector calls %s", name)
		}
	}

	var b bytes.Buffer
	b.WriteString("{\n")
	b.WriteString(`  "format": ` + jsonLine(t, f.Format) + ",\n")
	b.WriteString(`  "about": ` + jsonLine(t, f.About) + ",\n")
	b.WriteString(`  "calls": [` + "\n")
	for i, out := range f.Calls {
		b.WriteString("    " + jsonLine(t, out))
		if i < len(f.Calls)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("  ]\n}\n")
	want := b.Bytes()
	conformance.Emit(t, "functions.json", want)
	if os.Getenv("TENON_UPDATE_VECTORS") == "1" {
		if err := os.WriteFile("functions.json", want, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile("functions.json")
	if err != nil {
		t.Fatalf("reading the corpus: %v; set TENON_UPDATE_VECTORS=1 to write it", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("functions.json is not what the vectors give; if the change is deliberate, set TENON_UPDATE_VECTORS=1 and review the diff")
	}
}
