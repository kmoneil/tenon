package tenon_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
)

// TestLookupsPairPanickingAccessors holds each accessor that panics on a name
// its value or type lacks to a Lookup form that answers whether there is one:
// the accessor for a name the program knows is there, and the Lookup for a
// name from data.
func TestLookupsPairPanickingAccessors(t *testing.T) {
	num := tenon.NumberType()
	one := tenon.NumberFromInt(1)
	obj := tenon.Object(map[string]tenon.Value{"caf\U000000e9": one})
	for _, name := range []string{"caf\U000000e9", "cafe\U00000301"} {
		if got, ok := obj.LookupAttribute(name); !ok || !got.Equal(one) {
			t.Errorf("LookupAttribute(%q) = %v, %t, want 1, true", name, got, ok)
		}
		if got, ok := obj.Type().LookupAttributeType(name); !ok || got != num {
			t.Errorf("LookupAttributeType(%q) = %v, %t, want number, true", name, got, ok)
		}
	}
	for _, name := range []string{"missing", "", "\xff"} {
		if got, ok := obj.LookupAttribute(name); ok || !got.IsZero() {
			t.Errorf("LookupAttribute(%q) = %v, %t, want the zero Value and false", name, got, ok)
		}
		if got, ok := obj.Type().LookupAttributeType(name); ok || !got.IsZero() {
			t.Errorf("LookupAttributeType(%q) = %v, %t, want the zero Type and false", name, got, ok)
		}
	}
	mustPanicUsage(t, `which has no attribute "missing"`, func() { obj.Attribute("missing") })
	mustPanicUsage(t, `has no attribute "missing"`, func() { obj.Type().AttributeType("missing") })
	mustPanicUsage(t, "not a value of kind Object", func() { one.LookupAttribute("a") })
	mustPanicUsage(t, "not Object", func() { num.LookupAttributeType("a") })
}

// TestPropagationNames holds Propagation to a String like Policy's.
func TestPropagationNames(t *testing.T) {
	for p, want := range map[tenon.Propagation]string{tenon.Propagate: "propagate", tenon.Isolate: "isolate", 7: "Propagation(7)"} {
		if got := p.String(); got != want {
			t.Errorf("Propagation(%d).String() = %q, want %q", uint8(p), got, want)
		}
	}
}

// TestStringOnEveryZeroValue holds the String method of every exported type
// that has one to answering for the type's zero value rather than panicking,
// so that printing a zero value while debugging shows what it is. The list is
// checked against the package's source, so a type that gains a String method
// is held to it too.
func TestStringOnEveryZeroValue(t *testing.T) {
	zeros := map[string]fmt.Stringer{
		"Change": tenon.Change{}, "ChangeKind": tenon.ChangeKind(0), "Changes": tenon.Changes(nil),
		"Constraint": tenon.Constraint{}, "ConstraintKind": tenon.ConstraintKind(0), "Kind": tenon.Kind(0),
		"Narrowing": tenon.Narrowing{}, "Path": tenon.Path{}, "Policy": tenon.Policy(0),
		"Propagation": tenon.Propagation(0), "Range": tenon.Range{}, "Step": tenon.Step{},
		"StepKind": tenon.StepKind(0), "Type": tenon.Type{}, "Value": tenon.Value{},
	}
	var names []string
	for name, zero := range zeros {
		names = append(names, name)
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("String on the zero %s panicked: %v", name, r)
				}
			}()
			_ = zero.String()
		}()
	}
	slices.Sort(names)
	if got := stringerTypes(t); !slices.Equal(got, names) {
		t.Errorf("the package's types with a String method are\n%q\nbut this test holds\n%q", got, names)
	}
}

// stringerTypes returns the exported types of the package that have a String
// method, sorted.
func stringerTypes(t *testing.T) []string {
	t.Helper()
	var names []string
	fset := token.NewFileSet()
	for _, path := range sourceFiles(t) {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range file.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Name.Name != "String" {
				continue
			}
			recv := fn.Recv.List[0].Type
			if star, ok := recv.(*ast.StarExpr); ok {
				recv = star.X
			}
			if id, ok := recv.(*ast.Ident); ok && ast.IsExported(id.Name) && !strings.HasPrefix(id.Name, "_") {
				names = append(names, id.Name)
			}
		}
	}
	slices.Sort(names)
	return names
}
