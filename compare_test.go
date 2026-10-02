package tenon_test

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance/values"
)

// TestHandlesAreNotComparable holds Value, Constraint and Path, and what holds
// them, to refusing ==, which would compare how each is held rather than what
// it says: String("a") built twice would be two map keys. Type stays
// comparable, since it is interned, and refusing == costs no space.
func TestHandlesAreNotComparable(t *testing.T) {
	for _, rt := range []reflect.Type{
		reflect.TypeFor[tenon.Value](),
		reflect.TypeFor[tenon.Constraint](),
		reflect.TypeFor[tenon.Path](),
		reflect.TypeFor[tenon.Step](),
		reflect.TypeFor[tenon.Diagnostic](),
		reflect.TypeFor[tenon.Change](),
	} {
		if rt.Comparable() {
			t.Errorf("%s is comparable", rt)
		}
	}
	if !reflect.TypeFor[tenon.Type]().Comparable() {
		t.Error("Type is not comparable")
	}
	pointer := reflect.TypeFor[*int]().Size()
	for _, rt := range []reflect.Type{
		reflect.TypeFor[tenon.Value](),
		reflect.TypeFor[tenon.Constraint](),
		reflect.TypeFor[tenon.Path](),
	} {
		if rt.Size() != pointer {
			t.Errorf("%s takes %d bytes, want %d", rt, rt.Size(), pointer)
		}
	}
}

// equalTypes holds each exported type that go-cmp's cmp.Equal would reach an
// unexported field in, by its name, as the type go-cmp is handed: every such
// type has the Equal method cmp.Equal calls in place of reading its fields,
// which it would panic on.
var equalTypes = map[string]reflect.Type{
	"Value":       reflect.TypeFor[tenon.Value](),
	"Type":        reflect.TypeFor[tenon.Type](),
	"Constraint":  reflect.TypeFor[tenon.Constraint](),
	"Path":        reflect.TypeFor[tenon.Path](),
	"Step":        reflect.TypeFor[tenon.Step](),
	"Diagnostic":  reflect.TypeFor[tenon.Diagnostic](),
	"Range":       reflect.TypeFor[tenon.Range](),
	"Narrowing":   reflect.TypeFor[tenon.Narrowing](),
	"Error":       reflect.TypeFor[*tenon.Error](),
	"CapsuleType": reflect.TypeFor[*tenon.CapsuleType[int]](),
	"Function":    reflect.TypeFor[tenon.Function](),
}

// TestEqualMethods holds each type in equalTypes to having an Equal method of
// the form go-cmp's cmp.Equal calls, (T) Equal(T) bool, so that a struct
// holding one compares by what it says without an option; and holds
// equalTypes to naming every exported struct type the package declares with an
// unexported field, read from its source, so that a type added later cannot
// leave go-cmp to panic on it.
func TestEqualMethods(t *testing.T) {
	var declared []string
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, e.Name(), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			g, ok := d.(*ast.GenDecl)
			if !ok || g.Tok != token.TYPE {
				continue
			}
			for _, spec := range g.Specs {
				ts := spec.(*ast.TypeSpec)
				st, ok := ts.Type.(*ast.StructType)
				if !ok || !ts.Name.IsExported() {
					continue
				}
				if slices.ContainsFunc(st.Fields.List, hidden) {
					declared = append(declared, ts.Name.Name)
				}
			}
		}
	}
	for _, name := range declared {
		if _, ok := equalTypes[name]; !ok {
			t.Errorf("%s holds an unexported field, and equalTypes does not name it, so nothing holds it to an Equal method", name)
		}
	}
	if len(declared) < 9 {
		t.Errorf("the source declares %d exported structs holding unexported fields, %v; fewer than the 9 there are means the reading missed some", len(declared), declared)
	}
	for _, rt := range equalTypes {
		m, ok := rt.MethodByName("Equal")
		if !ok {
			t.Errorf("%s has no Equal method", rt)
			continue
		}
		if ft := m.Type; ft.NumIn() != 2 || ft.In(1) != rt || ft.NumOut() != 1 || ft.Out(0) != reflect.TypeFor[bool]() {
			t.Errorf("%s.Equal is %s, want func(%s) bool", rt, ft, rt)
		}
	}
}

// hidden reports whether a struct field is unexported, or embeds an unexported
// type, which go-cmp cannot read without an option.
func hidden(f *ast.Field) bool {
	if len(f.Names) == 0 {
		typ := f.Type
		if star, ok := typ.(*ast.StarExpr); ok {
			typ = star.X
		}
		id, ok := typ.(*ast.Ident)
		return !ok || !id.IsExported()
	}
	return slices.ContainsFunc(f.Names, func(n *ast.Ident) bool { return !n.IsExported() })
}

// TestEqualSaysWhatItCompares holds the Equal methods of Range, Narrowing,
// *Error and *CapsuleType to what they say they compare: a range the value it
// is of, marks included; a narrowing its kind, bound, prefix, length or
// members, in order, and the marks it was taken from; an error its value and
// its causes, as errors.Is says; a handle the capsule type it handles. Each
// zero or nil one is equal only to itself, a nil handle to a zero one too.
func TestEqualSaysWhatItCompares(t *testing.T) {
	num := tenon.NumberType()
	five, six := tenon.NumberFromInt(5), tenon.NumberFromInt(6)
	m := stamp{id: "m"}
	atLeast := func(n tenon.Value) tenon.Value { return tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(n, true)) }
	for _, tt := range []struct {
		name string
		got  bool
		want bool
	}{
		{"ranges of identical values", atLeast(five).Range().Equal(atLeast(five).Range()), true},
		{"ranges of other bounds", atLeast(five).Range().Equal(atLeast(six).Range()), false},
		{"the range of a marked value and of its twin", tenon.WithMarks(five, m).Range().Equal(five.Range()), false},
		{"the zero Range and a range", tenon.Range{}.Equal(five.Range()), false},
		{"two zero Ranges", tenon.Range{}.Equal(tenon.Range{}), true},
		{"a bound written two ways", tenon.NumberMin(five, true).Equal(tenon.NumberMin(tenon.NumberFromText("5.0"), true)), true},
		{"a bound inclusive and not", tenon.NumberMin(five, true).Equal(tenon.NumberMin(five, false)), false},
		{"a least and a greatest bound", tenon.NumberMin(five, true).Equal(tenon.NumberMax(five, true)), false},
		{"a bound from a marked value", tenon.NumberMin(tenon.WithMarks(five, m), true).Equal(tenon.NumberMin(five, true)), false},
		{"members in one order", tenon.Members(five, six).Equal(tenon.Members(five, six)), true},
		{"members in another", tenon.Members(five, six).Equal(tenon.Members(six, five)), false},
		{"prefixes", tenon.StringPrefix("a").Equal(tenon.StringPrefix("a")), true},
		{"lengths", tenon.LengthMin(1).Equal(tenon.LengthMax(1)), false},
		{"the zero Narrowing and not null", tenon.Narrowing{}.Equal(tenon.NotNull()), false},
		{"two zero Narrowings", tenon.Narrowing{}.Equal(tenon.Narrowing{}), true},
	} {
		if tt.got != tt.want {
			t.Errorf("%s: Equal = %t, want %t", tt.name, tt.got, tt.want)
		}
	}

	failed := tenon.ErrorVal(tenon.Diagnostic{Code: "test.failed", Message: "it failed"})
	other := tenon.ErrorVal(tenon.Diagnostic{Code: "test.failed", Message: "it failed otherwise"})
	cause, another := errors.New("cause"), errors.New("another")
	var none *tenon.Error
	for _, tt := range []struct {
		name string
		a, b *tenon.Error
		want bool
	}{
		{"two nil errors", none, none, true},
		{"nil and an error", none, tenon.NewError(failed), false},
		{"errors holding identical values", tenon.NewError(failed), tenon.NewError(failed), true},
		{"errors holding other values", tenon.NewError(failed), tenon.NewError(other), false},
		{"the same cause", tenon.NewError(failed, cause), tenon.NewError(failed, cause), true},
		{"a cause wrapped the same way", tenon.NewError(failed, fmt.Errorf("w: %w", cause)), tenon.NewError(failed, fmt.Errorf("w: %w", cause)), false},
		{"another cause", tenon.NewError(failed, cause), tenon.NewError(failed, another), false},
		{"a cause and none", tenon.NewError(failed, cause), tenon.NewError(failed), false},
	} {
		if got := tt.a.Equal(tt.b); got != tt.want || tt.b.Equal(tt.a) != got {
			t.Errorf("%s: Equal = %t, want %t either way round", tt.name, got, tt.want)
		}
	}

	a := tenon.NewCapsule("a", tenon.CapsuleOps[int]{})
	b := tenon.NewCapsule("a", tenon.CapsuleOps[int]{})
	var nilHandle *tenon.CapsuleType[int]
	for _, tt := range []struct {
		name string
		got  bool
		want bool
	}{
		{"a handle and itself", a.Equal(a), true},
		{"two capsule types of one name", a.Equal(b), false},
		{"a nil handle and a zero one", nilHandle.Equal(&tenon.CapsuleType[int]{}), true},
		{"a nil handle and one NewCapsule made", nilHandle.Equal(a), false},
	} {
		if tt.got != tt.want {
			t.Errorf("%s: Equal = %t, want %t", tt.name, tt.got, tt.want)
		}
	}
}

// TestValueEqual holds Value.Equal to Identical, over the values of the
// conformance corpus, and to answering for the zero Value, which is equal
// only to itself.
func TestValueEqual(t *testing.T) {
	all := values.All()
	for _, v := range all {
		for _, w := range all {
			if got, want := v.Equal(w), tenon.Identical(v, w); got != want {
				t.Errorf("%v.Equal(%v) = %t, want %t as Identical", v, w, got, want)
			}
		}
	}
	one := tenon.NumberFromInt(1)
	if !one.Equal(tenon.NumberFromText("1.0")) {
		t.Error("1 and 1.0, built apart, are not Equal")
	}
	if one.Equal(tenon.WithMarks(one, bare("m"))) {
		t.Error("1 and a marked 1 are Equal, though Identical tells them apart")
	}
	var zero tenon.Value
	if !zero.IsZero() || one.IsZero() {
		t.Error("IsZero does not tell the zero Value from a value")
	}
	if zero.Equal(one) || one.Equal(zero) || !zero.Equal(tenon.Value{}) {
		t.Error("the zero Value is Equal to a value, or not to itself")
	}
}

// TestZeroHandles holds IsZero and Equal on Constraint, Path and Step to
// answering for the zero value, which is equal only to itself.
func TestZeroHandles(t *testing.T) {
	var c tenon.Constraint
	if !c.IsZero() || tenon.Any().IsZero() {
		t.Error("IsZero does not tell the zero Constraint from a constraint")
	}
	if c.Equal(tenon.Any()) || tenon.Any().Equal(c) || !c.Equal(tenon.Constraint{}) || !tenon.Any().Equal(tenon.Any()) {
		t.Error("Constraint.Equal is wrong about the zero Constraint")
	}

	var p tenon.Path
	a := p.Attribute("a")
	if !p.IsZero() || a.IsZero() || !a.Steps()[0].Equal(tenon.Path{}.Attribute("a").Steps()[0]) {
		t.Error("IsZero does not tell the empty path from a longer one")
	}
	if p.Equal(a) || a.Equal(p) || !p.Equal(tenon.Path{}) || !a.Equal(tenon.Path{}.Attribute("a")) {
		t.Error("Path.Equal is wrong about the empty path")
	}

	var s tenon.Step
	attr := a.Steps()[0]
	index := p.Index(tenon.NumberFromInt(0)).Steps()[0]
	if !s.IsZero() || attr.IsZero() || index.IsZero() {
		t.Error("IsZero does not tell the zero Step from a step")
	}
	for _, step := range []tenon.Step{attr, index} {
		if s.Equal(step) || step.Equal(s) || !step.Equal(step) {
			t.Errorf("Step.Equal is wrong about the zero Step and %s", step)
		}
	}
	if !s.Equal(tenon.Step{}) || attr.Equal(index) {
		t.Error("Step.Equal is wrong about the zero Step, or two steps of two kinds")
	}
}
