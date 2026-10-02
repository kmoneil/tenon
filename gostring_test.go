package tenon_test

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"math/rand"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/internal/conformance/values"
)

// tag is a mark whose GoString writes it as this package's source does, so
// the Go syntax of a value carrying it builds it here, as a mark type of a
// program's own may.
type tag string

func (m tag) MarkID() string               { return string(m) }
func (tag) Propagation() tenon.Propagation { return tenon.Propagate }
func (tag) Redacting() bool                { return false }
func (m tag) GoString() string             { return "tag(" + strconv.Quote(string(m)) + ")" }

// sealing is a deep mark that writes itself as tag does.
type sealing string

func (m sealing) MarkID() string               { return string(m) }
func (sealing) Propagation() tenon.Propagation { return tenon.Propagate }
func (sealing) Redacting() bool                { return false }
func (sealing) Deep() bool                     { return true }
func (m sealing) GoString() string             { return "sealing(" + strconv.Quote(string(m)) + ")" }

// veil is a redacting mark that writes itself as tag does.
type veil string

func (m veil) MarkID() string               { return string(m) }
func (veil) Propagation() tenon.Propagation { return tenon.Propagate }
func (veil) Redacting() bool                { return true }
func (m veil) GoString() string             { return "veil(" + strconv.Quote(string(m)) + ")" }

// failure is an error that writes itself as tag does.
type failure string

func (f failure) Error() string    { return string(f) }
func (f failure) GoString() string { return "failure(" + strconv.Quote(string(f)) + ")" }

// goStringCases pairs a Go expression with the text %#v prints for what it
// builds, which is the expression itself: the compiler holds each text to
// being Go that builds the value, and TestGoStringCasesAreTheirSource holds
// each expression and its text to being one.
var goStringCases = []struct {
	v    any
	want string
}{
	// Known values of each kind, nulls, unknown values with each fact, and
	// pending and error values.
	{tenon.Bool(true), `tenon.Bool(true)`},
	{tenon.NumberFromInt(-12), `tenon.NumberFromInt(-12)`},
	{tenon.NumberFromText("2.5"), `tenon.NumberFromText("2.5")`},
	{tenon.NumberFromText("1.2345678901234567890123456789e29"), `tenon.NumberFromText("1.2345678901234567890123456789e29")`},
	{tenon.String("a\tb \"c\""), `tenon.String("a\tb \"c\"")`},
	{tenon.Null(tenon.ListType(tenon.StringType())), `tenon.Null(tenon.ListType(tenon.StringType()))`},
	{tenon.Unknown(tenon.BoolType()), `tenon.Unknown(tenon.BoolType())`},
	{
		tenon.Narrow(tenon.Unknown(tenon.NumberType()), tenon.NotNull(), tenon.NumberMin(tenon.NumberFromInt(1), false), tenon.NumberMax(tenon.NumberFromText("9.5"), true)),
		`tenon.Narrow(tenon.Unknown(tenon.NumberType()), tenon.NotNull(), tenon.NumberMin(tenon.NumberFromInt(1), false), tenon.NumberMax(tenon.NumberFromText("9.5"), true))`,
	},
	{
		tenon.Narrow(tenon.Unknown(tenon.StringType()), tenon.StringPrefix("v1-"), tenon.LengthMin(3), tenon.LengthMax(8)),
		`tenon.Narrow(tenon.Unknown(tenon.StringType()), tenon.StringPrefix("v1-"), tenon.LengthMin(3), tenon.LengthMax(8))`,
	},
	{
		tenon.Narrow(tenon.Unknown(tenon.StringType()), tenon.StringPrefix("caf"+"x"), tenon.LengthMin(3)),
		`tenon.Narrow(tenon.Unknown(tenon.StringType()), tenon.StringPrefix("caf" + "x"), tenon.LengthMin(3))`,
	},
	{
		tenon.Narrow(tenon.Unknown(tenon.SetType(tenon.StringType())), tenon.LengthMin(2), tenon.Members(tenon.String("a"), tenon.String("b"))),
		`tenon.Narrow(tenon.Unknown(tenon.SetType(tenon.StringType())), tenon.LengthMin(2), tenon.Members(tenon.String("a"), tenon.String("b")))`,
	},
	{
		tenon.Narrow(tenon.Pending(tenon.ObjectWith(map[string]tenon.Field{"name": tenon.Required(tenon.Exactly(tenon.StringType())), "tags": tenon.Optional(tenon.ListOf(tenon.Any()))}, true)), tenon.NotNull()),
		`tenon.Narrow(tenon.Pending(tenon.ObjectWith(map[string]tenon.Field{"name":tenon.Required(tenon.Exactly(tenon.StringType())), "tags":tenon.Optional(tenon.ListOf(tenon.Any()))}, true)), tenon.NotNull())`,
	},
	{tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly()), `tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly())`},
	{
		tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: "it failed", Path: tenon.Path{}.Attribute("a").Index(tenon.String("k"))}, tenon.Diagnostic{Code: "app.other", Message: "and again"}),
		`tenon.ErrorVal(tenon.Diagnostic{Code:"app.failed", Message:"it failed", Path:tenon.Path{}.Attribute("a").Index(tenon.String("k"))}, tenon.Diagnostic{Code:"app.other", Message:"and again"})`,
	},
	{tenon.Value{}, `tenon.Value{}`},

	// Collections and structural values, empty and holding values not known.
	{
		tenon.List(tenon.NumberType(), tenon.NumberFromInt(1), tenon.Null(tenon.NumberType()), tenon.Unknown(tenon.NumberType())),
		`tenon.List(tenon.NumberType(), tenon.NumberFromInt(1), tenon.Null(tenon.NumberType()), tenon.Unknown(tenon.NumberType()))`,
	},
	{tenon.List(tenon.StringType()), `tenon.List(tenon.StringType())`},
	{tenon.Set(tenon.StringType(), tenon.String("a"), tenon.String("b")), `tenon.Set(tenon.StringType(), tenon.String("a"), tenon.String("b"))`},
	{
		tenon.Map(tenon.NumberType(), map[string]tenon.Value{"j": tenon.NumberFromInt(2), "k": tenon.NumberFromInt(1)}),
		`tenon.Map(tenon.NumberType(), map[string]tenon.Value{"j":tenon.NumberFromInt(2), "k":tenon.NumberFromInt(1)})`,
	},
	{tenon.Map(tenon.NumberType(), map[string]tenon.Value{}), `tenon.Map(tenon.NumberType(), map[string]tenon.Value{})`},
	{tenon.Tuple(), `tenon.Tuple()`},
	{tenon.Tuple(tenon.Bool(true), tenon.String("x")), `tenon.Tuple(tenon.Bool(true), tenon.String("x"))`},
	{tenon.Object(map[string]tenon.Value{}), `tenon.Object(map[string]tenon.Value{})`},
	{
		tenon.Object(map[string]tenon.Value{"a": tenon.NumberFromInt(1), "b": tenon.List(tenon.StringType(), tenon.String("x"))}),
		`tenon.Object(map[string]tenon.Value{"a":tenon.NumberFromInt(1), "b":tenon.List(tenon.StringType(), tenon.String("x"))})`,
	},

	// Marks, in order of their identifiers, on values in each state, on a
	// member, and deep, where a value within lists only the marks it carries
	// beyond the deep ones.
	{tenon.WithMarks(tenon.NumberFromInt(1), tag("audited"), tag("origin")), `tenon.WithMarks(tenon.NumberFromInt(1), tag("audited"), tag("origin"))`},
	{tenon.WithMarks(tenon.Pending(tenon.Any()), tag("origin")), `tenon.WithMarks(tenon.Pending(tenon.Any()), tag("origin"))`},
	{
		tenon.WithMarks(tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: "it failed"}), tag("origin")),
		`tenon.WithMarks(tenon.ErrorVal(tenon.Diagnostic{Code:"app.failed", Message:"it failed"}), tag("origin"))`,
	},
	{tenon.Tuple(tenon.WithMarks(tenon.Unknown(tenon.StringType()), tag("audited"))), `tenon.Tuple(tenon.WithMarks(tenon.Unknown(tenon.StringType()), tag("audited")))`},
	{
		tenon.WithMarks(tenon.List(tenon.ListType(tenon.NumberType()), tenon.List(tenon.NumberType(), tenon.WithMarks(tenon.NumberFromInt(1), tag("audited")), tenon.NumberFromInt(2))), sealing("sealed")),
		`tenon.WithMarks(tenon.List(tenon.ListType(tenon.NumberType()), tenon.List(tenon.NumberType(), tenon.WithMarks(tenon.NumberFromInt(1), tag("audited")), tenon.NumberFromInt(2))), sealing("sealed"))`,
	},
	{tenon.WithMarks(tenon.Set(tenon.StringType(), tenon.String("a")), sealing("sealed")), `tenon.WithMarks(tenon.Set(tenon.StringType(), tenon.String("a")), sealing("sealed"))`},
	{
		tenon.WithMarks(tenon.Tuple(tenon.WithMarks(tenon.Pending(tenon.Any()), tag("audited")), tenon.NumberFromInt(1)), sealing("sealed")),
		`tenon.WithMarks(tenon.Tuple(tenon.WithMarks(tenon.Pending(tenon.Any()), tag("audited")), tenon.NumberFromInt(1)), sealing("sealed"))`,
	},

	// Types, constraints, paths, steps, ranges and narrowings.
	{
		tenon.ObjectType(map[string]tenon.Type{"a": tenon.TupleType(tenon.BoolType(), tenon.SetType(tenon.NumberType())), "b": tenon.MapType(tenon.StringType())}),
		`tenon.ObjectType(map[string]tenon.Type{"a":tenon.TupleType(tenon.BoolType(), tenon.SetType(tenon.NumberType())), "b":tenon.MapType(tenon.StringType())})`,
	},
	{tenon.TupleType(), `tenon.TupleType()`},
	{tenon.ObjectType(map[string]tenon.Type{}), `tenon.ObjectType(map[string]tenon.Type{})`},
	{tenon.Type{}, `tenon.Type{}`},
	{
		tenon.OneOf(tenon.Exactly(tenon.NumberType()), tenon.TupleOf(tenon.Any(), tenon.SetOf(tenon.Any())), tenon.MapOf(tenon.Any())),
		`tenon.OneOf(tenon.Exactly(tenon.NumberType()), tenon.TupleOf(tenon.Any(), tenon.SetOf(tenon.Any())), tenon.MapOf(tenon.Any()))`,
	},
	{tenon.ObjectWith(map[string]tenon.Field{}, false), `tenon.ObjectWith(map[string]tenon.Field{}, false)`},
	{tenon.Constraint{}, `tenon.Constraint{}`},
	{tenon.Path{}, `tenon.Path{}`},
	{tenon.Path{}.Index(tenon.NumberFromInt(0)).Attribute("a name"), `tenon.Path{}.Index(tenon.NumberFromInt(0)).Attribute("a name")`},
	{tenon.Path{}.Attribute("a").Steps()[0], `tenon.Path{}.Attribute("a").Steps()[0]`},
	{tenon.Path{}.Index(tenon.String("k")).Steps()[0], `tenon.Path{}.Index(tenon.String("k")).Steps()[0]`},
	{tenon.Step{}, `tenon.Step{}`},
	{tenon.Narrow(tenon.Unknown(tenon.NumberType()), tenon.NotNull()).Range(), `tenon.Narrow(tenon.Unknown(tenon.NumberType()), tenon.NotNull()).Range()`},
	{tenon.Bool(true).Range(), `tenon.Bool(true).Range()`},
	{tenon.Range{}, `tenon.Range{}`},
	{tenon.NotNull(), `tenon.NotNull()`},
	{tenon.NullOnly(), `tenon.NullOnly()`},
	{tenon.NumberMin(tenon.WithMarks(tenon.NumberFromInt(5), tag("origin")), true), `tenon.NumberMin(tenon.WithMarks(tenon.NumberFromInt(5), tag("origin")), true)`},
	{tenon.NumberMax(tenon.NumberFromText("0.5"), false), `tenon.NumberMax(tenon.NumberFromText("0.5"), false)`},
	{tenon.StringPrefix("caf" + "x"), `tenon.StringPrefix("caf" + "x")`},
	{tenon.StringPrefix("v1-"), `tenon.StringPrefix("v1-")`},
	{tenon.LengthMin(0), `tenon.LengthMin(0)`},
	{tenon.LengthMax(4), `tenon.LengthMax(4)`},
	{tenon.Members(tenon.Null(tenon.StringType()), tenon.String("a")), `tenon.Members(tenon.Null(tenon.StringType()), tenon.String("a"))`},
	{tenon.Narrowing{}, `tenon.Narrowing{}`},

	// Errors, diagnostics, changes and capsules, which print as Go syntax
	// where a struct holds them.
	{
		tenon.NewError(tenon.ErrorVal(tenon.Diagnostic{Code: "decode.null", Message: "the value is null"}), failure("boom")),
		`tenon.NewError(tenon.ErrorVal(tenon.Diagnostic{Code:"decode.null", Message:"the value is null"}), failure("boom"))`,
	},
	{(*tenon.Error)(nil), `(*tenon.Error)(nil)`},
	{&tenon.Error{}, `&tenon.Error{}`},
	{
		tenon.Diagnostic{Code: "app.failed", Message: "it failed", Path: tenon.Path{}.Attribute("a")},
		`tenon.Diagnostic{Code:"app.failed", Message:"it failed", Path:tenon.Path{}.Attribute("a")}`,
	},
	{
		tenon.Change{Kind: tenon.ChangeAdded, InCollection: false, Path: tenon.Path{}.Attribute("b"), Old: tenon.Value{}, New: tenon.NumberFromInt(2), OldMarks: []tenon.Mark(nil), NewMarks: []tenon.Mark{tag("audited")}},
		`tenon.Change{Kind:tenon.ChangeAdded, InCollection:false, Path:tenon.Path{}.Attribute("b"), Old:tenon.Value{}, New:tenon.NumberFromInt(2), OldMarks:[]tenon.Mark(nil), NewMarks:[]tenon.Mark{tag("audited")}}`,
	},
	{tenon.NewCapsule[struct{ X int }]("pair", tenon.CapsuleOps[struct{ X int }]{}), `tenon.NewCapsule[struct { X int }]("pair", tenon.CapsuleOps[struct { X int }]{})`},
	{
		tenon.NewCapsule[struct{ X int }]("pair", tenon.CapsuleOps[struct{ X int }]{}).Value(&struct{ X int }{X: 1}),
		`tenon.NewCapsule[struct { X int }]("pair", tenon.CapsuleOps[struct { X int }]{}).Value(&struct { X int }{X:1})`,
	},
	{tenon.List(tenon.NewCapsule[int]("count", tenon.CapsuleOps[int]{}).Type()), `tenon.List(tenon.NewCapsule[int]("count", tenon.CapsuleOps[int]{}).Type())`},
	{(*tenon.CapsuleType[int])(nil), `(*tenon.CapsuleType[int])(nil)`},
	{&tenon.CapsuleType[int]{}, `&tenon.CapsuleType[int]{}`},

	// The kinds and policies, which print by their constants' names.
	{tenon.KindBool, `tenon.KindBool`},
	{tenon.KindNumber, `tenon.KindNumber`},
	{tenon.KindString, `tenon.KindString`},
	{tenon.KindList, `tenon.KindList`},
	{tenon.KindSet, `tenon.KindSet`},
	{tenon.KindMap, `tenon.KindMap`},
	{tenon.KindTuple, `tenon.KindTuple`},
	{tenon.KindObject, `tenon.KindObject`},
	{tenon.KindCapsule, `tenon.KindCapsule`},
	{tenon.Kind(0), `tenon.Kind(0)`},
	{tenon.ConstraintExactly, `tenon.ConstraintExactly`},
	{tenon.ConstraintAny, `tenon.ConstraintAny`},
	{tenon.ConstraintListOf, `tenon.ConstraintListOf`},
	{tenon.ConstraintSetOf, `tenon.ConstraintSetOf`},
	{tenon.ConstraintMapOf, `tenon.ConstraintMapOf`},
	{tenon.ConstraintObjectWith, `tenon.ConstraintObjectWith`},
	{tenon.ConstraintTupleOf, `tenon.ConstraintTupleOf`},
	{tenon.ConstraintOneOf, `tenon.ConstraintOneOf`},
	{tenon.ConstraintKind(9), `tenon.ConstraintKind(9)`},
	{tenon.StepAttribute, `tenon.StepAttribute`},
	{tenon.StepIndex, `tenon.StepIndex`},
	{tenon.StepKind(0), `tenon.StepKind(0)`},
	{tenon.ChangeReplaced, `tenon.ChangeReplaced`},
	{tenon.ChangeAdded, `tenon.ChangeAdded`},
	{tenon.ChangeRemoved, `tenon.ChangeRemoved`},
	{tenon.ChangeMemberAdded, `tenon.ChangeMemberAdded`},
	{tenon.ChangeMemberRemoved, `tenon.ChangeMemberRemoved`},
	{tenon.ChangeMarks, `tenon.ChangeMarks`},
	{tenon.ChangeKind(0), `tenon.ChangeKind(0)`},
	{tenon.Safe, `tenon.Safe`},
	{tenon.Unsafe, `tenon.Unsafe`},
	{tenon.Policy(0), `tenon.Policy(0)`},
	{tenon.Propagate, `tenon.Propagate`},
	{tenon.Isolate, `tenon.Isolate`},
	{tenon.Propagation(2), `tenon.Propagation(2)`},
}

func TestGoString(t *testing.T) {
	for _, c := range goStringCases {
		if got := fmt.Sprintf("%#v", c.v); got != c.want {
			t.Errorf("%%#v printed\n%s\nwant\n%s", got, c.want)
		}
	}
}

// TestGoStringCasesAreTheirSource reads goStringCases from this file and
// holds each expression to the text beside it, as Go reads both: so each text
// is the source of the value it is the text of, and the value it builds is
// the value it was printed from.
func TestGoStringCasesAreTheirSource(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "gostring_test.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var table *ast.CompositeLit
	ast.Inspect(file, func(n ast.Node) bool {
		if spec, ok := n.(*ast.ValueSpec); ok && spec.Names[0].Name == "goStringCases" {
			table = spec.Values[0].(*ast.CompositeLit)
		}
		return table == nil
	})
	if table == nil {
		t.Fatal("goStringCases not found")
	}
	if len(table.Elts) != len(goStringCases) {
		t.Fatalf("read %d cases, want %d", len(table.Elts), len(goStringCases))
	}
	printed := func(fset *token.FileSet, n ast.Node) string {
		var b bytes.Buffer
		if err := printer.Fprint(&b, fset, n); err != nil {
			t.Fatal(err)
		}
		return b.String()
	}
	for _, elt := range table.Elts {
		pair := elt.(*ast.CompositeLit).Elts
		want, err := strconv.Unquote(pair[1].(*ast.BasicLit).Value)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := parser.ParseExpr(want)
		if err != nil {
			t.Errorf("%s does not parse: %v", want, err)
			continue
		}
		if source, text := printed(fset, pair[0]), printed(token.NewFileSet(), parsed); source != text {
			t.Errorf("the case built by\n%s\nis given the text of\n%s", source, text)
		}
	}
}

// TestGoStringBuildsTheCorpus evaluates the Go syntax of every value of the
// corpus, and of every value within one, and holds it to building a value
// identical to the value it was printed from; and so for their types, their
// ranges, the constraints of pending values and the paths of diagnostics. A
// capsule value cannot be built again, its type being the one its handle
// makes, so its syntax is held to parsing alone.
func TestGoStringBuildsTheCorpus(t *testing.T) {
	all := values.All()
	known := goStringMarks(t, all)
	built := 0
	for _, v := range valuesWithin(all) {
		text := fmt.Sprintf("%#v", v)
		if text != v.GoString() {
			t.Errorf("%%#v printed %s, not GoString's %s", text, v.GoString())
		}
		if strings.Contains(text, "tenon.NewCapsule[") {
			if _, err := parser.ParseExpr(text); err != nil {
				t.Errorf("%s does not parse: %v", text, err)
			}
			continue
		}
		built++
		if got := evalGo(t, text, known); !tenon.Identical(got.(tenon.Value), v) {
			t.Errorf("%s built %s, not %s", text, got.(tenon.Value), v)
		}
		switch {
		case v.IsResolved():
			typ, r := v.Type(), v.Range()
			if got := evalGo(t, typ.GoString(), known); got.(tenon.Type) != typ {
				t.Errorf("%s built %s, not %s", typ.GoString(), got, typ)
			}
			if got := evalGo(t, r.GoString(), known); !got.(tenon.Range).Equal(r) {
				t.Errorf("%s built %s, not %s", r.GoString(), got, r)
			}
		case v.IsPending():
			c := v.Constraint()
			if got := evalGo(t, c.GoString(), known); !got.(tenon.Constraint).Equal(c) {
				t.Errorf("%s built %s, not %s", c.GoString(), got, c)
			}
		default:
			for _, d := range v.Diagnostics() {
				if got := evalGo(t, d.Path.GoString(), known); !got.(tenon.Path).Equal(d.Path) {
					t.Errorf("%s built %s, not %s", d.Path.GoString(), got, d.Path)
				}
			}
		}
	}
	if built < 150 {
		t.Errorf("built %d values, want the corpus's", built)
	}
}

// TestGoStringKeepsPrefixes narrows unknown strings by prefixes at random,
// over text that composes, and holds the Go syntax of the value and of the
// narrowing to building them again: StringPrefix cuts a character that what
// follows could change, so the syntax writes one after the prefix for it to
// cut.
func TestGoStringKeepsPrefixes(t *testing.T) {
	r := rand.New(rand.NewSource(20261001))
	pieces := []string{
		"a", "e", "c", "q", "x", "-", "1", ":", "/", " ",
		"\U00000301", "\U00000327", "\U00000338", "=", "<",
		"\U00001100", "\U00001161", "\U000011A8", "\U0000AC00",
		"\U00000B47", "\U00000B3E", "\U0001F1EB", "\U0001F1F7", "\U0000200D", "\U0001F468",
	}
	for range conformance.Iterations(t, 2000) {
		var b strings.Builder
		for range 1 + r.Intn(6) {
			b.WriteString(pieces[r.Intn(len(pieces))])
		}
		nw := tenon.StringPrefix(b.String())
		v := tenon.Narrow(tenon.Unknown(tenon.StringType()), nw)
		if got := evalGo(t, nw.GoString(), nil); !got.(tenon.Narrowing).Equal(nw) {
			t.Errorf("%q: %s built another narrowing", b.String(), nw.GoString())
		}
		if got := evalGo(t, v.GoString(), nil); !tenon.Identical(got.(tenon.Value), v) {
			t.Errorf("%q: %s built %s, not %s", b.String(), v.GoString(), got, v)
		}
	}
}

// TestGoStringWithholdsWhatRedactionWithholds holds a value carrying a
// redacting mark to the placeholder that names its redacting marks, alone or
// within another value, whatever it is: as its display form does, it says
// nothing of what it holds, its range, whether it is null, or its other marks.
// An error value says its diagnostics and every mark, as its display does.
func TestGoStringWithholdsWhatRedactionWithholds(t *testing.T) {
	const placeholder = `tenon.WithMarks(tenon.Value{} /* redacted */, veil("secret"))`
	for _, v := range valuesWithin(values.All()) {
		if v.IsError() {
			continue
		}
		if got := tenon.WithMarks(v, veil("secret"), tag("other")).GoString(); got != placeholder {
			t.Errorf("%s redacted: GoString is %s, want %s", v, got, placeholder)
		}
	}

	secret := tenon.String("hunter2")
	red := tenon.WithMarks(secret, veil("secret"))
	str := tenon.StringType()
	for _, c := range []struct {
		v    any
		want string
	}{
		{
			tenon.WithMarks(tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: "it failed"}), veil("secret"), tag("other")),
			`tenon.WithMarks(tenon.ErrorVal(tenon.Diagnostic{Code:"app.failed", Message:"it failed"}), tag("other"), veil("secret"))`,
		},
		{tenon.List(str, tenon.String("a"), red), `tenon.List(tenon.StringType(), tenon.String("a"), ` + placeholder + `)`},
		{tenon.Object(map[string]tenon.Value{"pw": red}), `tenon.Object(map[string]tenon.Value{"pw":` + placeholder + `})`},
		{red.Range(), placeholder + `.Range()`},
		{tenon.WithMarks(tenon.Narrow(tenon.Unknown(str), tenon.StringPrefix("hunter2-")), veil("secret")), placeholder},
		{tenon.NumberMin(tenon.WithMarks(tenon.NumberFromInt(42), veil("secret")), true), `tenon.NumberMin(` + placeholder + `, true)`},
		{tenon.Narrow(tenon.Unknown(tenon.NumberType()), tenon.NumberMin(tenon.WithMarks(tenon.NumberFromInt(42), veil("secret")), true)), placeholder},
		{tenon.Path{}.Attribute("pw"), `tenon.Path{}.Attribute("pw")`},
	} {
		got := fmt.Sprintf("%#v", c.v)
		if got != c.want {
			t.Errorf("%%#v printed %s, want %s", got, c.want)
		}
		if strings.Contains(got, "hunter2") || strings.Contains(got, "42") {
			t.Errorf("%s shows what a redacting mark withholds", got)
		}
		if _, err := parser.ParseExpr(got); err != nil {
			t.Errorf("%s does not parse: %v", got, err)
		}
	}
}

// TestGoStringGrowsWithTheValue holds Go syntax to growing with the value, in
// the shapes that would have it grow with the members times their type if it
// wrote each type in full where it is needed. Where the plain syntax would run
// long so, each type is named once in a variable, and the syntax builds the
// value all the same.
func TestGoStringGrowsWithTheValue(t *testing.T) {
	num := tenon.NumberType()
	deep := func(k int) tenon.Type {
		typ := num
		for range k {
			typ = tenon.ListType(typ)
		}
		return typ
	}
	times := func(k int, v tenon.Value) []tenon.Value {
		vs := make([]tenon.Value, k)
		for i := range vs {
			vs[i] = v
		}
		return vs
	}
	for _, shape := range []struct {
		name  string
		build func(k int) any
		// want is the Go syntax at k = 2, which is short enough to write in
		// full.
		want string
		// doubling says that the syntax, written in full, doubles with each
		// level: at k = 100 it would not end, so the shape runs only where
		// the others have shown that it is not written so.
		doubling bool
	}{
		{"null members", func(k int) any { return tenon.List(deep(k), times(k, tenon.Null(deep(k)))...) },
			`tenon.List(tenon.ListType(tenon.ListType(tenon.NumberType())), tenon.Null(tenon.ListType(tenon.ListType(tenon.NumberType()))), tenon.Null(tenon.ListType(tenon.ListType(tenon.NumberType()))))`, false},
		{"unknown members", func(k int) any {
			return tenon.Set(deep(k), times(k, tenon.Narrow(tenon.Unknown(deep(k)), tenon.NotNull()))...)
		}, `tenon.Set(tenon.ListType(tenon.ListType(tenon.NumberType())), tenon.Narrow(tenon.Unknown(tenon.ListType(tenon.ListType(tenon.NumberType()))), tenon.NotNull()), tenon.Narrow(tenon.Unknown(tenon.ListType(tenon.ListType(tenon.NumberType()))), tenon.NotNull()))`, false},
		{"a nest of lists", func(k int) any {
			v, typ := tenon.NumberFromInt(1), num
			for range k {
				v, typ = tenon.List(typ, v), tenon.ListType(typ)
			}
			return v
		}, `tenon.List(tenon.ListType(tenon.NumberType()), tenon.List(tenon.NumberType(), tenon.NumberFromInt(1)))`, false},
		{"objects within a map", func(k int) any {
			entries := make(map[string]tenon.Value, k)
			for i := range k {
				entries[fmt.Sprintf("k%04d", i)] = tenon.Object(map[string]tenon.Value{"a": tenon.Unknown(deep(k))})
			}
			return tenon.Map(tenon.ObjectType(map[string]tenon.Type{"a": deep(k)}), entries)
		}, `tenon.Map(tenon.ObjectType(map[string]tenon.Type{"a":tenon.ListType(tenon.ListType(tenon.NumberType()))}), map[string]tenon.Value{"k0000":tenon.Object(map[string]tenon.Value{"a":tenon.Unknown(tenon.ListType(tenon.ListType(tenon.NumberType())))}), "k0001":tenon.Object(map[string]tenon.Value{"a":tenon.Unknown(tenon.ListType(tenon.ListType(tenon.NumberType())))})})`, false},
		{"members a range lists", func(k int) any {
			members := make([]tenon.Value, k)
			for i := range members {
				members[i] = tenon.Narrow(tenon.Unknown(deep(k)), tenon.LengthMin(int64(i+1)))
			}
			return tenon.Members(members...)
		}, `tenon.Members(tenon.Narrow(tenon.Unknown(tenon.ListType(tenon.ListType(tenon.NumberType()))), tenon.LengthMin(1)), tenon.Narrow(tenon.Unknown(tenon.ListType(tenon.ListType(tenon.NumberType()))), tenon.LengthMin(2)))`, false},
		{"a type holding one type twice at each level", func(k int) any {
			typ := num
			for range k {
				typ = tenon.ObjectType(map[string]tenon.Type{"a": typ, "b": typ})
			}
			return typ
		}, `tenon.ObjectType(map[string]tenon.Type{"a":tenon.ObjectType(map[string]tenon.Type{"a":tenon.NumberType(), "b":tenon.NumberType()}), "b":tenon.ObjectType(map[string]tenon.Type{"a":tenon.NumberType(), "b":tenon.NumberType()})})`, true},
	} {
		if shape.doubling && t.Failed() {
			continue
		}
		if got := fmt.Sprintf("%#v", shape.build(2)); got != shape.want {
			t.Errorf("%s: %%#v printed\n%s\nwant\n%s", shape.name, got, shape.want)
		}
		var sizes, lengths [2]uint64
		for i, k := range []int{100, 400} {
			v := shape.build(k)
			var before, after runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&before)
			text := fmt.Sprintf("%#v", v)
			runtime.ReadMemStats(&after)
			sizes[i], lengths[i] = after.TotalAlloc-before.TotalAlloc, uint64(len(text))
			if !strings.HasPrefix(text, "func() tenon.") {
				t.Errorf("%s at k = %d: the syntax names no types: %.80s", shape.name, k, text)
			}
			if k == 100 {
				switch v := v.(type) {
				case tenon.Value:
					if got := evalGo(t, text, nil).(tenon.Value); !tenon.Identical(got, v) {
						t.Errorf("%s: the syntax at k = 100 built another value", shape.name)
					}
				case tenon.Narrowing:
					if got := evalGo(t, text, nil).(tenon.Narrowing); !got.Equal(v) {
						t.Errorf("%s: the syntax at k = 100 built another narrowing", shape.name)
					}
				case tenon.Type:
					if got := evalGo(t, text, nil).(tenon.Type); got != v {
						t.Errorf("%s: the syntax at k = 100 built another type", shape.name)
					}
				}
			}
		}
		if sizes[1] > 8*sizes[0] || lengths[1] > 8*lengths[0] {
			t.Errorf("%s: four times k allocated %d bytes, not %d, for %d bytes of syntax, not %d",
				shape.name, sizes[1], sizes[0], lengths[1], lengths[0])
		}
	}
}

// valuesWithin returns vs and every value within each, outermost first.
func valuesWithin(vs []tenon.Value) []tenon.Value {
	var out []tenon.Value
	var visit func(v tenon.Value)
	visit = func(v tenon.Value) {
		out = append(out, v)
		if !v.HasContent() {
			return
		}
		switch v.Type().Kind() {
		case tenon.KindList, tenon.KindSet, tenon.KindTuple:
			for _, e := range v.Elements() {
				visit(e)
			}
		case tenon.KindMap:
			for _, k := range v.MapKeys() {
				e, _ := v.LookupMapElement(k)
				visit(e)
			}
		case tenon.KindObject:
			for _, name := range v.Type().AttributeNames() {
				visit(v.Attribute(name))
			}
		}
	}
	for _, v := range vs {
		visit(v)
	}
	return out
}

// goStringMarks returns the marks the values in vs carry, at any depth, by the
// text %#v writes for each, which the Go syntax of a value carrying it holds.
func goStringMarks(t *testing.T, vs []tenon.Value) map[string]any {
	marks := map[string]any{}
	for _, v := range valuesWithin(vs) {
		_, ms := tenon.UnmarkDeep(v)
		for _, m := range ms {
			text := fmt.Sprintf("%#v", m)
			if held, ok := marks[text]; ok && held != m {
				t.Fatalf("two marks are written %s", text)
			}
			marks[text] = m
		}
	}
	return marks
}

// evalGo evaluates src, the Go syntax GoString writes, failing t where it does
// not parse or is not that syntax. known holds the marks and causes the
// syntax may hold, by the text %#v writes for each; tag, sealing, veil and
// failure, which write themselves as this file's source does, are known
// besides.
func evalGo(t *testing.T, src string, known map[string]any) any {
	t.Helper()
	expr, err := parser.ParseExpr(src)
	if err != nil {
		t.Fatalf("%s does not parse: %v", src, err)
	}
	e := &goEval{t: t, src: src, known: known, vars: map[string]any{}}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("evaluating %.300s: %v", src, r)
		}
	}()
	return e.eval(expr)
}

// goEval evaluates the Go that GoString writes: calls of the package's
// functions and of a path's and a value's methods, composite literals of the
// package's types and of maps, string and integer literals, the + of two
// strings, and a function literal called in place, which defines variables
// and returns.
type goEval struct {
	t     *testing.T
	src   string
	known map[string]any
	vars  map[string]any
}

// text returns the source of n.
func (e *goEval) text(n ast.Node) string { return e.src[n.Pos()-1 : n.End()-1] }

// fail ends the test, saying why the syntax could not be evaluated.
func (e *goEval) fail(format string, args ...any) any {
	e.t.Helper()
	e.t.Fatalf("evaluating %.300s: "+format, append([]any{e.src}, args...)...)
	return nil
}

func (e *goEval) eval(n ast.Expr) any {
	switch n := n.(type) {
	case *ast.ParenExpr:
		return e.eval(n.X)
	case *ast.BasicLit:
		switch n.Kind {
		case token.STRING:
			s, err := strconv.Unquote(n.Value)
			if err != nil {
				return e.fail("%v", err)
			}
			return s
		case token.INT:
			i, err := strconv.ParseInt(n.Value, 10, 64)
			if err != nil {
				return e.fail("%v", err)
			}
			return i
		}
	case *ast.UnaryExpr:
		if n.Op == token.SUB {
			return -e.eval(n.X).(int64)
		}
	case *ast.BinaryExpr:
		if n.Op == token.ADD {
			return e.eval(n.X).(string) + e.eval(n.Y).(string)
		}
	case *ast.Ident:
		switch n.Name {
		case "true":
			return true
		case "false":
			return false
		}
		if v, ok := e.vars[n.Name]; ok {
			return v
		}
	case *ast.IndexExpr:
		return e.eval(n.X).([]tenon.Step)[e.eval(n.Index).(int64)]
	case *ast.CompositeLit:
		return e.composite(n)
	case *ast.CallExpr:
		return e.call(n)
	}
	return e.fail("cannot evaluate %s", e.text(n))
}

func (e *goEval) composite(n *ast.CompositeLit) any {
	entries := func(each func(key string, value ast.Expr)) {
		for _, elt := range n.Elts {
			kv := elt.(*ast.KeyValueExpr)
			each(e.eval(kv.Key).(string), kv.Value)
		}
	}
	switch typ := e.text(n.Type); typ {
	case "tenon.Value":
		return tenon.Value{}
	case "tenon.Diagnostic":
		var d tenon.Diagnostic
		for _, elt := range n.Elts {
			kv := elt.(*ast.KeyValueExpr)
			switch kv.Key.(*ast.Ident).Name {
			case "Code":
				d.Code = tenon.Code(e.eval(kv.Value).(string))
			case "Message":
				d.Message = e.eval(kv.Value).(string)
			case "Path":
				d.Path = e.eval(kv.Value).(tenon.Path)
			}
		}
		return d
	case "tenon.Path":
		return tenon.Path{}
	case "map[string]tenon.Value":
		m := map[string]tenon.Value{}
		entries(func(k string, v ast.Expr) { m[k] = e.eval(v).(tenon.Value) })
		return m
	case "map[string]tenon.Type":
		m := map[string]tenon.Type{}
		entries(func(k string, v ast.Expr) { m[k] = e.eval(v).(tenon.Type) })
		return m
	case "map[string]tenon.Field":
		m := map[string]tenon.Field{}
		entries(func(k string, v ast.Expr) { m[k] = e.eval(v).(tenon.Field) })
		return m
	}
	return e.fail("cannot evaluate %s", e.text(n))
}

// lookup returns the mark or cause that n writes.
func (e *goEval) lookup(n ast.Expr) any {
	text := e.text(n)
	if v, ok := e.known[text]; ok {
		return v
	}
	if call, ok := n.(*ast.CallExpr); ok {
		if fn, ok := call.Fun.(*ast.Ident); ok && len(call.Args) == 1 {
			s := e.eval(call.Args[0]).(string)
			switch fn.Name {
			case "tag":
				return tag(s)
			case "sealing":
				return sealing(s)
			case "veil":
				return veil(s)
			case "failure":
				return failure(s)
			}
		}
	}
	return e.fail("no mark or cause is written %s", text)
}

func (e *goEval) call(n *ast.CallExpr) any {
	switch fn := n.Fun.(type) {
	case *ast.FuncLit:
		for _, s := range fn.Body.List {
			switch s := s.(type) {
			case *ast.AssignStmt:
				e.vars[s.Lhs[0].(*ast.Ident).Name] = e.eval(s.Rhs[0])
			case *ast.ReturnStmt:
				return e.eval(s.Results[0])
			}
		}
	case *ast.SelectorExpr:
		if pkg, ok := fn.X.(*ast.Ident); ok && pkg.Name == "tenon" {
			return e.function(fn.Sel.Name, n.Args)
		}
		switch recv := e.eval(fn.X).(type) {
		case tenon.Path:
			switch fn.Sel.Name {
			case "Attribute":
				return recv.Attribute(e.eval(n.Args[0]).(string))
			case "Index":
				return recv.Index(e.eval(n.Args[0]).(tenon.Value))
			case "Steps":
				return recv.Steps()
			}
		case tenon.Value:
			if fn.Sel.Name == "Range" {
				return recv.Range()
			}
		}
	}
	return e.fail("cannot evaluate %s", e.text(n))
}

func (e *goEval) function(name string, args []ast.Expr) any {
	arg := func(i int) any { return e.eval(args[i]) }
	vals := func(from int) []tenon.Value {
		var vs []tenon.Value
		for _, a := range args[from:] {
			vs = append(vs, e.eval(a).(tenon.Value))
		}
		return vs
	}
	types := func() []tenon.Type {
		var ts []tenon.Type
		for _, a := range args {
			ts = append(ts, e.eval(a).(tenon.Type))
		}
		return ts
	}
	constraints := func() []tenon.Constraint {
		var cs []tenon.Constraint
		for _, a := range args {
			cs = append(cs, e.eval(a).(tenon.Constraint))
		}
		return cs
	}
	switch name {
	case "Bool":
		return tenon.Bool(arg(0).(bool))
	case "NumberFromInt":
		return tenon.NumberFromInt(arg(0).(int64))
	case "NumberFromText":
		return tenon.NumberFromText(arg(0).(string))
	case "String":
		return tenon.String(arg(0).(string))
	case "List":
		return tenon.List(arg(0).(tenon.Type), vals(1)...)
	case "Set":
		return tenon.Set(arg(0).(tenon.Type), vals(1)...)
	case "Map":
		return tenon.Map(arg(0).(tenon.Type), arg(1).(map[string]tenon.Value))
	case "Tuple":
		return tenon.Tuple(vals(0)...)
	case "Object":
		return tenon.Object(arg(0).(map[string]tenon.Value))
	case "Null":
		return tenon.Null(arg(0).(tenon.Type))
	case "Unknown":
		return tenon.Unknown(arg(0).(tenon.Type))
	case "Pending":
		return tenon.Pending(arg(0).(tenon.Constraint))
	case "Narrow":
		var ns []tenon.Narrowing
		for _, a := range args[1:] {
			ns = append(ns, e.eval(a).(tenon.Narrowing))
		}
		return tenon.Narrow(arg(0).(tenon.Value), ns...)
	case "ErrorVal":
		var ds []tenon.Diagnostic
		for _, a := range args {
			ds = append(ds, e.eval(a).(tenon.Diagnostic))
		}
		return tenon.ErrorVal(ds...)
	case "WithMarks":
		var ms []tenon.Mark
		for _, a := range args[1:] {
			ms = append(ms, e.lookup(a).(tenon.Mark))
		}
		return tenon.WithMarks(arg(0).(tenon.Value), ms...)
	case "NewError":
		var causes []error
		for _, a := range args[1:] {
			causes = append(causes, e.lookup(a).(error))
		}
		return tenon.NewError(arg(0).(tenon.Value), causes...)
	case "BoolType":
		return tenon.BoolType()
	case "NumberType":
		return tenon.NumberType()
	case "StringType":
		return tenon.StringType()
	case "ListType":
		return tenon.ListType(arg(0).(tenon.Type))
	case "SetType":
		return tenon.SetType(arg(0).(tenon.Type))
	case "MapType":
		return tenon.MapType(arg(0).(tenon.Type))
	case "TupleType":
		return tenon.TupleType(types()...)
	case "ObjectType":
		return tenon.ObjectType(arg(0).(map[string]tenon.Type))
	case "Any":
		return tenon.Any()
	case "Exactly":
		return tenon.Exactly(arg(0).(tenon.Type))
	case "ListOf":
		return tenon.ListOf(arg(0).(tenon.Constraint))
	case "SetOf":
		return tenon.SetOf(arg(0).(tenon.Constraint))
	case "MapOf":
		return tenon.MapOf(arg(0).(tenon.Constraint))
	case "TupleOf":
		return tenon.TupleOf(constraints()...)
	case "OneOf":
		return tenon.OneOf(constraints()...)
	case "ObjectWith":
		return tenon.ObjectWith(arg(0).(map[string]tenon.Field), arg(1).(bool))
	case "Required":
		return tenon.Required(arg(0).(tenon.Constraint))
	case "Optional":
		return tenon.Optional(arg(0).(tenon.Constraint))
	case "NotNull":
		return tenon.NotNull()
	case "NullOnly":
		return tenon.NullOnly()
	case "NumberMin":
		return tenon.NumberMin(arg(0).(tenon.Value), arg(1).(bool))
	case "NumberMax":
		return tenon.NumberMax(arg(0).(tenon.Value), arg(1).(bool))
	case "StringPrefix":
		return tenon.StringPrefix(arg(0).(string))
	case "LengthMin":
		return tenon.LengthMin(arg(0).(int64))
	case "LengthMax":
		return tenon.LengthMax(arg(0).(int64))
	case "Members":
		return tenon.Members(vals(0)...)
	}
	return e.fail("tenon has no function %s that GoString writes", name)
}
