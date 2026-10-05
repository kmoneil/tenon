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
	"AssertNotNull":        stdlib.AssertNotNullFunc,
	"Add":                  stdlib.AddFunc,
	"Subtract":             stdlib.SubtractFunc,
	"Multiply":             stdlib.MultiplyFunc,
	"Divide":               stdlib.DivideFunc,
	"Modulo":               stdlib.ModuloFunc,
	"Negate":               stdlib.NegateFunc,
	"LessThan":             stdlib.LessThanFunc,
	"LessThanOrEqualTo":    stdlib.LessThanOrEqualToFunc,
	"GreaterThan":          stdlib.GreaterThanFunc,
	"GreaterThanOrEqualTo": stdlib.GreaterThanOrEqualToFunc,
	"Equal":                stdlib.EqualFunc,
	"NotEqual":             stdlib.NotEqualFunc,
	"Not":                  stdlib.NotFunc,
	"And":                  stdlib.AndFunc,
	"Or":                   stdlib.OrFunc,
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
	{"Add/exact", "Add", []tenon.Value{tenon.NumberFromText("0.1"), tenon.NumberFromText("0.2")}, tenon.Safe},
	{"Add/large", "Add", []tenon.Value{tenon.NumberFromText("1e200"), n(1)}, tenon.Safe},
	{"Add/a range", "Add", []tenon.Value{tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(n(1), true)), n(1)}, tenon.Safe},
	{"Add/a string under Unsafe", "Add", []tenon.Value{s("2"), n(3)}, tenon.Unsafe},
	{"Add/marked", "Add", []tenon.Value{tenon.WithMarks(n(2), plain), n(3)}, tenon.Safe},
	{"Add/refused/null", "Add", []tenon.Value{tenon.Null(num), n(1)}, tenon.Safe},
	{"Add/refused/a string under Safe", "Add", []tenon.Value{s("2"), n(3)}, tenon.Safe},
	{"Subtract/exact", "Subtract", []tenon.Value{tenon.NumberFromText("0.3"), tenon.NumberFromText("0.1")}, tenon.Safe},
	{"Multiply/exact", "Multiply", []tenon.Value{tenon.NumberFromText("1.1"), tenon.NumberFromText("1.1")}, tenon.Safe},
	{"Divide/terminating", "Divide", []tenon.Value{n(1), n(8)}, tenon.Safe},
	{"Divide/rounded", "Divide", []tenon.Value{n(2), n(3)}, tenon.Safe},
	{"Divide/refused/by zero", "Divide", []tenon.Value{n(1), n(0)}, tenon.Safe},
	{"Divide/refused/an unknown by zero", "Divide", []tenon.Value{tenon.Unknown(num), n(0)}, tenon.Safe},
	{"Modulo/the dividend's sign", "Modulo", []tenon.Value{n(-7), n(3)}, tenon.Safe},
	{"Modulo/decimal", "Modulo", []tenon.Value{tenon.NumberFromText("0.9"), tenon.NumberFromText("0.3")}, tenon.Safe},
	{"Modulo/large", "Modulo", []tenon.Value{tenon.NumberFromText("1e200"), n(7)}, tenon.Safe},
	{"Modulo/refused/by zero", "Modulo", []tenon.Value{n(1), n(0)}, tenon.Safe},
	{"Negate/a number", "Negate", []tenon.Value{tenon.NumberFromText("-2.5")}, tenon.Safe},
	{"Negate/a range", "Negate", []tenon.Value{tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(n(1), true))}, tenon.Safe},
	{"LessThan/numbers", "LessThan", []tenon.Value{n(1), tenon.NumberFromText("1.5")}, tenon.Safe},
	{"LessThanOrEqualTo/equal", "LessThanOrEqualTo", []tenon.Value{tenon.NumberFromText("1.50"), tenon.NumberFromText("1.5")}, tenon.Safe},
	{"GreaterThan/strings as numbers", "GreaterThan", []tenon.Value{s("10"), s("9")}, tenon.Unsafe},
	{"GreaterThan/settled by a range", "GreaterThan", []tenon.Value{tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(n(5), true)), n(1)}, tenon.Safe},
	{"GreaterThanOrEqualTo/numbers", "GreaterThanOrEqualTo", []tenon.Value{n(2), n(3)}, tenon.Safe},
	{"Equal/an untyped null and a typed one", "Equal", []tenon.Value{tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly()), tenon.Null(str)}, tenon.Safe},
	{"Equal/an untyped null and a value", "Equal", []tenon.Value{tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly()), s("x")}, tenon.Safe},
	{"Equal/two untyped nulls", "Equal", []tenon.Value{tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly()), tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly())}, tenon.Safe},
	{"Equal/types differ", "Equal", []tenon.Value{n(1), s("1")}, tenon.Safe},
	{"Equal/numbers spelled differently", "Equal", []tenon.Value{tenon.NumberFromText("1.50"), tenon.NumberFromText("1.5")}, tenon.Safe},
	{"NotEqual/an untyped null and a value", "NotEqual", []tenon.Value{tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly()), s("x")}, tenon.Safe},
	{"Not/true", "Not", []tenon.Value{tenon.Bool(true)}, tenon.Safe},
	{"And/false decides", "And", []tenon.Value{tenon.Bool(false), tenon.Unknown(boo)}, tenon.Safe},
	{"Or/true decides", "Or", []tenon.Value{tenon.Unknown(boo), tenon.Bool(true)}, tenon.Safe},
	{"Or/refused/null", "Or", []tenon.Value{tenon.Bool(true), tenon.Null(boo)}, tenon.Safe},
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
	conformance.Covers(t, "LB-001", "LN-001", "LN-002", "LN-010", "LN-011", "LN-020", "LN-083")
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
