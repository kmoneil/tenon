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
	"Coalesce":             stdlib.CoalesceFunc,
	"MakeTo":               stdlib.MakeToFunc(tenon.Exactly(tenon.NumberType())),
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
	// to is the constraint MakeTo makes its function for, and is zero for
	// every other function.
	to tenon.Constraint
}

// callVectors are the calls functions.json records: what each answers, or
// how it fails. A name begins with the function's and, for a call that
// fails, "refused/" after it.
var callVectors = []callVector{
	{"AssertNotNull/a number", "AssertNotNull", []tenon.Value{n(1)}, tenon.Safe, tenon.Constraint{}},
	{"AssertNotNull/marks", "AssertNotNull", []tenon.Value{tenon.WithMarks(s("hunter2"), secret{}, plain)}, tenon.Safe, tenon.Constraint{}},
	{"AssertNotNull/a tuple holding a null", "AssertNotNull", []tenon.Value{tenon.Tuple(tenon.Null(str), n(1))}, tenon.Safe, tenon.Constraint{}},
	{"AssertNotNull/unknown", "AssertNotNull", []tenon.Value{tenon.Unknown(str)}, tenon.Safe, tenon.Constraint{}},
	{"AssertNotNull/unknown with a prefix", "AssertNotNull", []tenon.Value{tenon.Narrow(tenon.Unknown(str), tenon.StringPrefix("abc"))}, tenon.Safe, tenon.Constraint{}},
	{"AssertNotNull/pending", "AssertNotNull", []tenon.Value{tenon.Pending(tenon.ListOf(tenon.Any()))}, tenon.Safe, tenon.Constraint{}},
	{"AssertNotNull/refused/null", "AssertNotNull", []tenon.Value{tenon.Null(str)}, tenon.Safe, tenon.Constraint{}},
	{"AssertNotNull/refused/pending null", "AssertNotNull", []tenon.Value{tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly())}, tenon.Safe, tenon.Constraint{}},
	{"AssertNotNull/refused/redacted null", "AssertNotNull", []tenon.Value{tenon.WithMarks(tenon.Null(str), secret{})}, tenon.Safe, tenon.Constraint{}},
	{"AssertNotNull/refused/arity", "AssertNotNull", []tenon.Value{n(1), n(2)}, tenon.Safe, tenon.Constraint{}},
	{"Add/exact", "Add", []tenon.Value{tenon.NumberFromText("0.1"), tenon.NumberFromText("0.2")}, tenon.Safe, tenon.Constraint{}},
	{"Add/large", "Add", []tenon.Value{tenon.NumberFromText("1e200"), n(1)}, tenon.Safe, tenon.Constraint{}},
	{"Add/a range", "Add", []tenon.Value{tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(n(1), true)), n(1)}, tenon.Safe, tenon.Constraint{}},
	{"Add/a string under Unsafe", "Add", []tenon.Value{s("2"), n(3)}, tenon.Unsafe, tenon.Constraint{}},
	{"Add/marked", "Add", []tenon.Value{tenon.WithMarks(n(2), plain), n(3)}, tenon.Safe, tenon.Constraint{}},
	{"Add/refused/null", "Add", []tenon.Value{tenon.Null(num), n(1)}, tenon.Safe, tenon.Constraint{}},
	{"Add/refused/a string under Safe", "Add", []tenon.Value{s("2"), n(3)}, tenon.Safe, tenon.Constraint{}},
	{"Subtract/exact", "Subtract", []tenon.Value{tenon.NumberFromText("0.3"), tenon.NumberFromText("0.1")}, tenon.Safe, tenon.Constraint{}},
	{"Multiply/exact", "Multiply", []tenon.Value{tenon.NumberFromText("1.1"), tenon.NumberFromText("1.1")}, tenon.Safe, tenon.Constraint{}},
	{"Divide/terminating", "Divide", []tenon.Value{n(1), n(8)}, tenon.Safe, tenon.Constraint{}},
	{"Divide/rounded", "Divide", []tenon.Value{n(2), n(3)}, tenon.Safe, tenon.Constraint{}},
	{"Divide/refused/by zero", "Divide", []tenon.Value{n(1), n(0)}, tenon.Safe, tenon.Constraint{}},
	{"Divide/refused/an unknown by zero", "Divide", []tenon.Value{tenon.Unknown(num), n(0)}, tenon.Safe, tenon.Constraint{}},
	{"Modulo/the dividend's sign", "Modulo", []tenon.Value{n(-7), n(3)}, tenon.Safe, tenon.Constraint{}},
	{"Modulo/decimal", "Modulo", []tenon.Value{tenon.NumberFromText("0.9"), tenon.NumberFromText("0.3")}, tenon.Safe, tenon.Constraint{}},
	{"Modulo/large", "Modulo", []tenon.Value{tenon.NumberFromText("1e200"), n(7)}, tenon.Safe, tenon.Constraint{}},
	{"Modulo/refused/by zero", "Modulo", []tenon.Value{n(1), n(0)}, tenon.Safe, tenon.Constraint{}},
	{"Negate/a number", "Negate", []tenon.Value{tenon.NumberFromText("-2.5")}, tenon.Safe, tenon.Constraint{}},
	{"Negate/a range", "Negate", []tenon.Value{tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(n(1), true))}, tenon.Safe, tenon.Constraint{}},
	{"LessThan/numbers", "LessThan", []tenon.Value{n(1), tenon.NumberFromText("1.5")}, tenon.Safe, tenon.Constraint{}},
	{"LessThanOrEqualTo/equal", "LessThanOrEqualTo", []tenon.Value{tenon.NumberFromText("1.50"), tenon.NumberFromText("1.5")}, tenon.Safe, tenon.Constraint{}},
	{"GreaterThan/strings as numbers", "GreaterThan", []tenon.Value{s("10"), s("9")}, tenon.Unsafe, tenon.Constraint{}},
	{"GreaterThan/settled by a range", "GreaterThan", []tenon.Value{tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(n(5), true)), n(1)}, tenon.Safe, tenon.Constraint{}},
	{"GreaterThanOrEqualTo/numbers", "GreaterThanOrEqualTo", []tenon.Value{n(2), n(3)}, tenon.Safe, tenon.Constraint{}},
	{"Equal/an untyped null and a typed one", "Equal", []tenon.Value{tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly()), tenon.Null(str)}, tenon.Safe, tenon.Constraint{}},
	{"Equal/an untyped null and a value", "Equal", []tenon.Value{tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly()), s("x")}, tenon.Safe, tenon.Constraint{}},
	{"Equal/two untyped nulls", "Equal", []tenon.Value{tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly()), tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly())}, tenon.Safe, tenon.Constraint{}},
	{"Equal/types differ", "Equal", []tenon.Value{n(1), s("1")}, tenon.Safe, tenon.Constraint{}},
	{"Equal/numbers spelled differently", "Equal", []tenon.Value{tenon.NumberFromText("1.50"), tenon.NumberFromText("1.5")}, tenon.Safe, tenon.Constraint{}},
	{"NotEqual/an untyped null and a value", "NotEqual", []tenon.Value{tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly()), s("x")}, tenon.Safe, tenon.Constraint{}},
	{"Not/true", "Not", []tenon.Value{tenon.Bool(true)}, tenon.Safe, tenon.Constraint{}},
	{"And/false decides", "And", []tenon.Value{tenon.Bool(false), tenon.Unknown(boo)}, tenon.Safe, tenon.Constraint{}},
	{"Or/true decides", "Or", []tenon.Value{tenon.Unknown(boo), tenon.Bool(true)}, tenon.Safe, tenon.Constraint{}},
	{"Or/refused/null", "Or", []tenon.Value{tenon.Bool(true), tenon.Null(boo)}, tenon.Safe, tenon.Constraint{}},
	{"Coalesce/the first not null", "Coalesce", []tenon.Value{tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly()), tenon.Null(num), n(1), n(2)}, tenon.Safe, tenon.Constraint{}},
	{"Coalesce/one not known yet", "Coalesce", []tenon.Value{tenon.Unknown(num), n(1)}, tenon.Safe, tenon.Constraint{}},
	{"Coalesce/unified under Unsafe", "Coalesce", []tenon.Value{n(1), s("a")}, tenon.Unsafe, tenon.Constraint{}},
	{"Coalesce/marks of what is read", "Coalesce", []tenon.Value{tenon.WithMarks(tenon.Null(num), plain), n(1), tenon.WithMarks(n(2), secret{})}, tenon.Safe, tenon.Constraint{}},
	{"Coalesce/refused/every argument null", "Coalesce", []tenon.Value{tenon.Null(str), tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly())}, tenon.Safe, tenon.Constraint{}},
	{"Coalesce/refused/not unified under Safe", "Coalesce", []tenon.Value{n(1), s("a")}, tenon.Safe, tenon.Constraint{}},
	{"MakeTo/a string to a number", "MakeTo", []tenon.Value{s("5")}, tenon.Safe, tenon.Exactly(num)},
	{"MakeTo/a tuple to a list", "MakeTo", []tenon.Value{tenon.Tuple(n(1), s("a"))}, tenon.Safe, tenon.ListOf(tenon.Any())},
	{"MakeTo/null", "MakeTo", []tenon.Value{tenon.Null(str)}, tenon.Safe, tenon.Exactly(num)},
	{"MakeTo/refused/not a number", "MakeTo", []tenon.Value{s("inf")}, tenon.Safe, tenon.Exactly(num)},
	{"MakeTo/refused/a member", "MakeTo", []tenon.Value{tenon.Tuple(s("1"), s("x"))}, tenon.Safe, tenon.ListOf(tenon.Exactly(num))},
}

// callFile is the layout of functions.json.
type callFile struct {
	Format int       `json:"format"`
	About  string    `json:"about"`
	Calls  []callOut `json:"calls"`
}

type callOut struct {
	Name          string        `json:"name"`
	Function      string        `json:"function"`
	Constraint    string        `json:"constraint,omitempty"`
	ConstraintHex string        `json:"constraintHex,omitempty"`
	Args          []callArg     `json:"args"`
	Policy        string        `json:"policy"`
	Value         string        `json:"value,omitempty"`
	ValueHex      string        `json:"valueHex,omitempty"`
	Failures      []jsonFailure `json:"failures,omitempty"`
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
	conformance.Covers(t, "LB-001", "LN-001", "LN-002", "LN-010", "LN-011", "LN-020", "LN-080", "LN-083", "LN-085")
	f := callFile{
		Format: 1,
		About: "Each entry is a call of a function of the standard library (§13 to §20), named as the specification " +
			"names it, with arguments, each given as its display form and its encoding (args), under a policy. A call " +
			"that answers gives value, its display form, and valueHex, its encoding; one that fails gives the " +
			"failures, each a code and the display form of its path, in order. The marks the arguments carry are " +
			"those vectors.json lists. A call of a function MakeTo makes gives the constraint it was made for, as " +
			"its display form and as the encoding of the pending value of it (constraint, constraintHex).",
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
		if vec.function == "MakeTo" {
			fn = stdlib.MakeToFunc(vec.to)
			pending, err := tenon.Serialize(tenon.Pending(vec.to))
			if err != nil {
				t.Fatalf("%s: the constraint %v does not encode: %v", vec.name, vec.to, err)
			}
			out.Constraint, out.ConstraintHex = vec.to.String(), hex.EncodeToString(pending)
		}
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
