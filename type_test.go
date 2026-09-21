package tenon_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"math/big"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/conformance"
)

// mustPanicUsage runs f and fails t unless f panics with a usage error whose
// message contains want.
func mustPanicUsage(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		t.Helper()
		r := recover()
		msg, ok := r.(string)
		if !ok || !strings.HasPrefix(msg, "tenon: usage: ") || !strings.Contains(msg, want) {
			t.Errorf("recovered %#v, want a usage error panic containing %q", r, want)
		}
	}()
	f()
}

func TestConformance_TY010_TypeKinds(t *testing.T) {
	conformance.Covers(t, "TY-010")
	str := tenon.StringType()
	tests := []struct {
		typ  tenon.Type
		kind tenon.Kind
		name string
	}{
		{tenon.BoolType(), tenon.KindBool, "Bool"},
		{tenon.NumberType(), tenon.KindNumber, "Number"},
		{str, tenon.KindString, "String"},
		{tenon.List(str), tenon.KindList, "List"},
		{tenon.Set(str), tenon.KindSet, "Set"},
		{tenon.Map(str), tenon.KindMap, "Map"},
		{tenon.Object(map[string]tenon.Type{"a": str}), tenon.KindObject, "Object"},
		{tenon.Tuple(str), tenon.KindTuple, "Tuple"},
		{sampleCapsule, tenon.KindCapsule, "Capsule"},
	}
	for _, tt := range tests {
		if got := tt.typ.Kind(); got != tt.kind || got.String() != tt.name {
			t.Errorf("%v: Kind() = %v, want %s", tt.typ, got, tt.name)
		}
	}

	// There are no other kinds.
	var names []string
	for k := range tenon.Kind(32) {
		if name := k.String(); !strings.HasPrefix(name, "Kind(") {
			names = append(names, name)
		}
	}
	want := []string{"Bool", "Number", "String", "List", "Set", "Map", "Tuple", "Object", "Capsule"}
	if !slices.Equal(names, want) {
		t.Errorf("named kinds = %q, want %q", names, want)
	}
}

func TestConformance_TY011_CollectionTypes(t *testing.T) {
	conformance.Covers(t, "TY-011")
	elem := tenon.List(tenon.NumberType())
	for _, typ := range []tenon.Type{tenon.List(elem), tenon.Set(elem), tenon.Map(elem)} {
		if !typ.IsCollection() || typ.IsStructural() {
			t.Errorf("%v: IsCollection() = %t, IsStructural() = %t; want a collection type", typ, typ.IsCollection(), typ.IsStructural())
		}
		if got := typ.ElementType(); !got.Equals(elem) {
			t.Errorf("%v: ElementType() = %v, want %v", typ, got, elem)
		}
	}
	for _, typ := range []tenon.Type{tenon.BoolType(), tenon.StringType(), tenon.Object(nil), tenon.Tuple(elem)} {
		if typ.IsCollection() {
			t.Errorf("%v: IsCollection() = true", typ)
		}
		mustPanicUsage(t, "not a collection kind", func() { typ.ElementType() })
	}
}

func TestConformance_TY012_StructuralTypes(t *testing.T) {
	conformance.Covers(t, "TY-012")
	str, num, tags := tenon.StringType(), tenon.NumberType(), tenon.Set(tenon.StringType())
	obj := tenon.Object(map[string]tenon.Type{"name": str, "count": num, "tags": tags})
	tup := tenon.Tuple(str, num, tags)
	for _, typ := range []tenon.Type{obj, tup} {
		if !typ.IsStructural() || typ.IsCollection() {
			t.Errorf("%v: IsStructural() = %t, IsCollection() = %t; want a structural type", typ, typ.IsStructural(), typ.IsCollection())
		}
	}

	// Members have differing types.
	for name, want := range map[string]tenon.Type{"name": str, "count": num, "tags": tags} {
		if got := obj.AttributeType(name); !got.Equals(want) {
			t.Errorf("AttributeType(%q) = %v, want %v", name, got, want)
		}
	}
	if got, want := obj.AttributeNames(), []string{"count", "name", "tags"}; !slices.Equal(got, want) {
		t.Errorf("AttributeNames() = %q, want %q", got, want)
	}
	if got := tup.TupleLength(); got != 3 {
		t.Errorf("TupleLength() = %d, want 3", got)
	}
	for i, want := range []tenon.Type{str, num, tags} {
		if got := tup.TupleElementType(i); !got.Equals(want) {
			t.Errorf("TupleElementType(%d) = %v, want %v", i, got, want)
		}
	}

	mustPanicUsage(t, "whose kind is Tuple, not Object", func() { tup.AttributeNames() })
	mustPanicUsage(t, "whose kind is Object, not Tuple", func() { obj.TupleLength() })
	mustPanicUsage(t, "whose kind is List, not Object", func() { tenon.List(str).HasAttribute("name") })
	mustPanicUsage(t, `has no attribute "missing"`, func() { obj.AttributeType("missing") })
	mustPanicUsage(t, "which has 3 elements", func() { tup.TupleElementType(3) })
}

func TestConformance_TY013_AttributeNames(t *testing.T) {
	conformance.Covers(t, "TY-013")
	num := tenon.NumberType()
	composed, decomposed := "caf\u00e9", "cafe\u0301"

	// A name that is not in NFC and its NFC form construct equal types, and
	// the name is kept in its normalized form.
	fromDecomposed := tenon.Object(map[string]tenon.Type{decomposed: num})
	fromComposed := tenon.Object(map[string]tenon.Type{composed: num})
	if !fromDecomposed.Equals(fromComposed) {
		t.Errorf("%v and %v are not equal", fromDecomposed, fromComposed)
	}
	if got, want := fromDecomposed.AttributeNames(), []string{composed}; !slices.Equal(got, want) {
		t.Errorf("AttributeNames() = %+q, want %+q", got, want)
	}

	// Lookups normalize the name they are given.
	if !fromComposed.HasAttribute(decomposed) || !fromComposed.AttributeType(decomposed).Equals(num) {
		t.Errorf("%v: looking up %+q failed", fromComposed, decomposed)
	}
	if fromComposed.HasAttribute("cafe") || fromComposed.HasAttribute("\xff") {
		t.Errorf("%v: found an attribute that it does not have", fromComposed)
	}

	// Names are non-empty strings of Unicode scalar values, and one object
	// type cannot have two attributes with the same name.
	mustPanicUsage(t, "must not be empty", func() { tenon.Object(map[string]tenon.Type{"": num}) })
	mustPanicUsage(t, "not valid UTF-8", func() { tenon.Object(map[string]tenon.Type{"a\xff": num}) })
	mustPanicUsage(t, "not valid UTF-8", func() { tenon.Object(map[string]tenon.Type{"\xed\xa0\x80": num}) }) // a surrogate
	mustPanicUsage(t, "the same name after normalization", func() {
		tenon.Object(map[string]tenon.Type{composed: num, decomposed: tenon.StringType()})
	})
}

func TestConformance_TY022_TypesImmutable(t *testing.T) {
	conformance.Covers(t, "TY-022")
	str, num := tenon.StringType(), tenon.NumberType()

	attrs := map[string]tenon.Type{"a": str}
	obj := tenon.Object(attrs)
	attrs["a"], attrs["b"] = num, num
	if want := tenon.Object(map[string]tenon.Type{"a": str}); !obj.Equals(want) {
		t.Errorf("changing the map passed to Object changed the type to %v", obj)
	}
	names := obj.AttributeNames()
	names[0] = "z"
	if !obj.HasAttribute("a") || obj.HasAttribute("z") {
		t.Errorf("changing the slice from AttributeNames changed the type to %v", obj)
	}

	elems := []tenon.Type{str, num}
	tup := tenon.Tuple(elems...)
	elems[0] = num
	if !tup.TupleElementType(0).Equals(str) {
		t.Errorf("changing the slice passed to Tuple changed the type to %v", tup)
	}
	got := tup.TupleElementTypes()
	got[1] = str
	if !tup.TupleElementType(1).Equals(num) {
		t.Errorf("changing the slice from TupleElementTypes changed the type to %v", tup)
	}

	// Type has no exported fields through which it could be changed.
	for f := range reflect.TypeFor[tenon.Type]().Fields() {
		if f.IsExported() {
			t.Errorf("tenon.Type has an exported field %s", f.Name)
		}
	}
}

func TestTypeEquals(t *testing.T) {
	str, num := tenon.StringType(), tenon.NumberType()
	object := func(attrs map[string]tenon.Type) tenon.Type { return tenon.Object(attrs) }
	equal := [][2]tenon.Type{
		{str, tenon.StringType()},
		{tenon.List(tenon.Map(num)), tenon.List(tenon.Map(num))},
		{object(map[string]tenon.Type{"a": str, "b": num}), object(map[string]tenon.Type{"b": num, "a": str})},
		{tenon.Tuple(str, num), tenon.Tuple(str, num)},
		{tenon.Tuple(), tenon.Tuple()},
		{object(nil), object(map[string]tenon.Type{})},
	}
	unequal := [][2]tenon.Type{
		{str, num},
		{tenon.List(str), tenon.Set(str)},
		{tenon.List(str), tenon.List(num)},
		{object(map[string]tenon.Type{"a": str}), object(map[string]tenon.Type{"a": num})},
		{object(map[string]tenon.Type{"a": str}), object(map[string]tenon.Type{"a": str, "b": str})},
		{tenon.Tuple(str, num), tenon.Tuple(num, str)},
		{tenon.Tuple(str), tenon.Tuple(str, str)},
		{tenon.Tuple(), object(nil)},
	}
	for _, p := range equal {
		if !p[0].Equals(p[1]) || !p[1].Equals(p[0]) || p[0] != p[1] {
			t.Errorf("%v and %v are not equal", p[0], p[1])
		}
	}
	for _, p := range unequal {
		if p[0].Equals(p[1]) || p[1].Equals(p[0]) || p[0] == p[1] {
			t.Errorf("%v and %v are equal", p[0], p[1])
		}
	}
}

func TestTypeString(t *testing.T) {
	str, num := tenon.StringType(), tenon.NumberType()
	tests := []struct {
		typ  tenon.Type
		want string
	}{
		{tenon.BoolType(), "bool"},
		{tenon.Map(tenon.Set(num)), "map(set(number))"},
		{tenon.Tuple(), "tuple([])"},
		{tenon.Object(nil), "object({})"},
		{
			tenon.Object(map[string]tenon.Type{"b": tenon.Tuple(tenon.BoolType(), num), "a": tenon.List(str)}),
			`object({"a": list(string), "b": tuple([bool, number])})`,
		},
		{tenon.Type{}, "<zero Type>"},
	}
	for _, tt := range tests {
		if got := tt.typ.String(); got != tt.want {
			t.Errorf("String() = %s, want %s", got, tt.want)
		}
	}
}

func TestZeroType(t *testing.T) {
	var zero tenon.Type
	str := tenon.StringType()
	mustPanicUsage(t, "zero Type", func() { zero.Kind() })
	mustPanicUsage(t, "zero Type", func() { zero.Equals(str) })
	mustPanicUsage(t, "zero Type", func() { str.Equals(zero) })
	mustPanicUsage(t, "zero Type", func() { tenon.List(zero) })
	mustPanicUsage(t, "zero Type", func() { tenon.Tuple(str, zero) })
	mustPanicUsage(t, "zero Type", func() { tenon.Object(map[string]tenon.Type{"a": zero}) })
}

// exportedFuncs returns the names of the package's exported functions whose
// results include the named type, sorted. Methods are left out: they hand back
// what a function made in the first place.
func exportedFuncs(t *testing.T, result string) []string {
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
			if !ok || fn.Recv != nil || !fn.Name.IsExported() || fn.Type.Results == nil {
				continue
			}
			for _, r := range fn.Type.Results.List {
				if id, ok := r.Type.(*ast.Ident); ok && id.Name == result {
					names = append(names, fn.Name.Name)
					break
				}
			}
		}
	}
	slices.Sort(names)
	return names
}

// sourceFiles returns the package's own Go files, leaving out its tests.
func sourceFiles(t *testing.T) []string {
	t.Helper()
	all, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, path := range all {
		if !strings.HasSuffix(path, "_test.go") {
			files = append(files, path)
		}
	}
	return files
}

// unmarked returns just the value half of Unmark.
func unmarked(v tenon.Value) tenon.Value {
	u, _ := tenon.Unmark(v)
	return u
}

// unmarkedDeep returns just the value half of UnmarkDeep.
func unmarkedDeep(v tenon.Value) tenon.Value {
	u, _ := tenon.UnmarkDeep(v)
	return u
}

// concrete walks ty and fails unless every type in it, at any depth, is one of
// the kinds a value can have.
func concrete(t *testing.T, name string, ty tenon.Type) {
	t.Helper()
	switch k := ty.Kind(); k {
	case tenon.KindBool, tenon.KindNumber, tenon.KindString, tenon.KindCapsule:
	case tenon.KindList, tenon.KindSet, tenon.KindMap:
		concrete(t, name, ty.ElementType())
	case tenon.KindObject:
		for _, a := range ty.AttributeNames() {
			concrete(t, name, ty.AttributeType(a))
		}
	case tenon.KindTuple:
		for _, e := range ty.TupleElementTypes() {
			concrete(t, name, e)
		}
	default:
		t.Errorf("%s: %v has kind %v, which is not a kind a value can have", name, ty, k)
	}
}

func TestConformance_TY001_EveryValueHasOneConcreteType(t *testing.T) {
	conformance.Covers(t, "TY-001")
	type thing struct{}
	held := tenon.Capsule("thing", tenon.CapsuleOps[thing]{})
	str, num, bl := tenon.StringType(), tenon.NumberType(), tenon.BoolType()
	one, tr := tenon.NumberFromInt(1), tenon.Bool(true)
	typed := map[string]tenon.Value{
		"Bool":             tr,
		"NumberFromInt":    one,
		"NumberFromBigInt": tenon.NumberFromBigInt(new(big.Int).Lsh(big.NewInt(1), 100)),
		"NumberFromText":   tenon.NumberFromText("1.5"),
		"String":           tenon.String("x"),
		"CapsuleVal":       tenon.CapsuleVal(held, &thing{}),
		"NullVal":          tenon.NullVal(str),
		"Unknown":          tenon.Unknown(str),
		"ListVal":          tenon.ListVal(str, tenon.String("a")),
		"SetVal":           tenon.SetVal(str),
		"MapVal":           tenon.MapVal(str, map[string]tenon.Value{"k": tenon.String("v")}),
		"TupleVal":         tenon.TupleVal(one, tr),
		"ObjectVal":        tenon.ObjectVal(map[string]tenon.Value{"a": one}),
		"Narrow":           tenon.Narrow(tenon.Unknown(num), tenon.NotNull()),
		"Resolve":          tenon.Resolve(tenon.Pending(tenon.Any()), str),
		"And":              tenon.And(tr, tenon.Bool(false)),
		"Equals":           tenon.Equals(one, one),
		"LessThan":         tenon.LessThan(one, one),
		"Length":           tenon.Length(tenon.ListVal(str, tenon.String("a"))),
		"Contains":         tenon.Contains(tenon.SetVal(str), one),
		"Or":               tenon.Or(tr, tenon.Bool(false)),
		"Not":              tenon.Not(tr),
		"IsNull":           tenon.IsNull(one),
		"Add":              tenon.Add(one, one),
		"Sub":              tenon.Sub(one, one),
		"Mul":              tenon.Mul(one, one),
		"Div":              tenon.Div(one, one),
		"Mod":              tenon.Mod(one, one),
		"Convert":          tenon.Convert(one, tenon.Exactly(str), tenon.Unsafe),
		"WithMarks":        tenon.WithMarks(one, stamp{id: "m"}),
		"Unmark":           unmarked(tenon.WithMarks(one, stamp{id: "m"})),
		"UnmarkDeep":       unmarkedDeep(tenon.ListVal(num, tenon.WithMarks(one, stamp{id: "m"}))),
	}
	want := map[string]tenon.Type{
		"Bool": bl, "NumberFromInt": num, "NumberFromBigInt": num, "NumberFromText": num, "String": str,
		"CapsuleVal": held, "NullVal": str, "Unknown": str,
		"ListVal": tenon.List(str), "SetVal": tenon.Set(str), "MapVal": tenon.Map(str),
		"TupleVal": tenon.Tuple(num, bl), "ObjectVal": tenon.Object(map[string]tenon.Type{"a": num}),
		"Narrow": num, "Resolve": str, "And": bl, "Or": bl, "Not": bl, "IsNull": bl,
		"Equals": bl, "LessThan": bl, "Length": num, "Contains": bl,
		"Add": num, "Sub": num, "Mul": num, "Div": num, "Mod": num, "Convert": str,
		"WithMarks": num, "Unmark": num, "UnmarkDeep": tenon.List(num),
	}
	// A value with no type is the other half of the rule.
	untyped := map[string]tenon.Value{
		"ErrorVal":    tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: "it failed"}),
		"Pending":     tenon.Pending(tenon.Any()),
		"Unify":       unifyFailure(),
		"Serialize":   serializeFailure(),
		"Deserialize": deserializeFailure(),
		"ProjectJSON": projectFailure(),
	}
	// Between them these are every function that makes a value, so one added
	// later has to be accounted for here before this test passes again.
	var covered []string
	for name := range typed {
		covered = append(covered, name)
	}
	for name := range untyped {
		covered = append(covered, name)
	}
	slices.Sort(covered)
	if funcs := exportedFuncs(t, "Value"); !slices.Equal(covered, funcs) {
		t.Errorf("this test covers\n%q\nbut the package makes values with\n%q", covered, funcs)
	}
	for name, v := range typed {
		if !v.IsResolved() {
			t.Errorf("%s: %v is neither resolved nor covered as an error or pending value", name, v)
			continue
		}
		got := v.Type()
		if got != v.Type() {
			t.Errorf("%s: the type of %v is not the same each time it is asked for", name, v)
		}
		if got != want[name] {
			t.Errorf("%s: the type of %v is %v, want %v", name, v, got, want[name])
		}
		concrete(t, name, got)
	}
	for name, v := range untyped {
		if v.IsResolved() {
			t.Errorf("%s: %v is resolved, so it is covered by the wrong half of this test", name, v)
		}
		mustPanicUsage(t, "which has no type", func() { v.Type() })
	}
}

func TestConformance_TY002_NoWildcardInATypeAtAnyDepth(t *testing.T) {
	conformance.Covers(t, "TY-002")
	// The package makes types in nine ways, one for each kind, and each takes
	// types and names. There is no tenth that takes a constraint or a
	// placeholder, and adding one would have to start here.
	want := []string{"BoolType", "Capsule", "List", "Map", "NumberType", "Object", "Set", "StringType", "Tuple"}
	if got := exportedFuncs(t, "Type"); !slices.Equal(got, want) {
		t.Errorf("the package makes types with\n%q\nwant\n%q", got, want)
	}
	// Nesting the kinds as deeply as they go finds nothing that is not a kind.
	str, bl := tenon.StringType(), tenon.BoolType()
	deep := tenon.Object(map[string]tenon.Type{
		"a": tenon.List(tenon.Map(tenon.Set(tenon.Tuple(str, tenon.List(bl))))),
	})
	concrete(t, "a deeply nested type", deep)
	// Optionality is an acceptance test, and stays on that side: a constraint
	// can leave an attribute out, and the two object types it then answers for
	// are different types, each of them concrete.
	c := tenon.ObjectWith(map[string]tenon.Field{"a": tenon.Optional(tenon.Exactly(str))}, false)
	with, without := tenon.Object(map[string]tenon.Type{"a": str}), tenon.Object(nil)
	if !tenon.Satisfies(c, with) || !tenon.Satisfies(c, without) {
		t.Error("an optional field did not accept the object with and the object without")
	}
	if with == without {
		t.Error("the two objects are one type, so optionality reached into the type")
	}
	concrete(t, "an object with the attribute", with)
	concrete(t, "an object without it", without)
}

func TestConformance_TY003_AcceptanceIsExpressedAsConstraints(t *testing.T) {
	conformance.Covers(t, "TY-003")
	str := tenon.StringType()
	// A conversion target is a constraint: a value converts to a type the
	// constraint accepts, an optional attribute absent from it included.
	target := tenon.ObjectWith(map[string]tenon.Field{"tags": tenon.Optional(tenon.ListOf(tenon.Exactly(str)))}, true)
	want := tenon.ObjectVal(map[string]tenon.Value{"tags": tenon.NullVal(tenon.List(str))})
	if got := tenon.Convert(tenon.ObjectVal(nil), target, tenon.Safe); !tenon.Identical(got, want) {
		t.Errorf("converting an empty object to %v gave %v, want %v", target, got, want)
	}
	// A schema is a constraint, which is what lets it accept a set of types
	// rather than naming one, and an optional attribute be expressible at all.
	schema := tenon.ObjectWith(map[string]tenon.Field{
		"name": tenon.Required(tenon.Exactly(str)),
		"tags": tenon.Optional(tenon.ListOf(tenon.Exactly(str))),
	}, false)
	for _, tt := range []struct {
		ty   tenon.Type
		want bool
	}{
		{tenon.Object(map[string]tenon.Type{"name": str}), true},
		{tenon.Object(map[string]tenon.Type{"name": str, "tags": tenon.List(str)}), true},
		{tenon.Object(map[string]tenon.Type{"name": str, "tags": tenon.Set(str)}), false},
		{tenon.Object(nil), false},
	} {
		if got := tenon.Satisfies(schema, tt.ty); got != tt.want {
			t.Errorf("%v satisfies the schema: %t, want %t", tt.ty, got, tt.want)
		}
	}
	// What an operation accepts is a parameter declaration, so every operation
	// states it as a constraint. An operation that takes parameters, as a
	// conversion does, binds what depends on them for each choice instead.
	for _, lit := range operationLiterals(t) {
		fields := []string{"name", "operands", "result", "known"}
		if lit.keys["bind"] {
			fields = []string{"name", "operands", "bind", "samples"}
		}
		for _, field := range fields {
			if !lit.keys[field] {
				t.Errorf("%s: an operation literal does not set %s", lit.where, field)
			}
		}
	}
}

// operationLiteral is one &op{...} in the package's source.
type operationLiteral struct {
	where      string
	keys       map[string]bool
	registered bool // the literal is the argument of register
}

// operationLiterals returns every operation the package defines.
func operationLiterals(t *testing.T) []operationLiteral {
	t.Helper()
	var lits []operationLiteral
	fset := token.NewFileSet()
	for _, path := range sourceFiles(t) {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		// register(&op{...}) is visited before the literal inside it.
		registered := map[token.Pos]bool{}
		ast.Inspect(file, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "register" && len(call.Args) == 1 {
					if ref, ok := call.Args[0].(*ast.UnaryExpr); ok && ref.Op == token.AND {
						registered[ref.X.Pos()] = true
					}
				}
				return true
			}
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			if id, ok := lit.Type.(*ast.Ident); !ok || id.Name != "op" {
				return true
			}
			keys := map[string]bool{}
			for _, e := range lit.Elts {
				if kv, ok := e.(*ast.KeyValueExpr); ok {
					if id, ok := kv.Key.(*ast.Ident); ok {
						keys[id.Name] = true
					}
				}
			}
			lits = append(lits, operationLiteral{
				where:      fset.Position(lit.Pos()).String(),
				keys:       keys,
				registered: registered[lit.Pos()],
			})
			return true
		})
	}
	if len(lits) == 0 {
		t.Fatal("found no operations to check")
	}
	return lits
}

// unifyFailure returns the error value of a unification that fails.
func unifyFailure() tenon.Value {
	_, failure, _ := tenon.Unify(tenon.Safe, tenon.Exactly(tenon.NumberType()), tenon.Exactly(tenon.BoolType()))
	return failure
}

// serializeFailure returns the error value of a value that cannot be
// serialized.
func serializeFailure() tenon.Value {
	_, failure, _ := tenon.Serialize(tenon.WithMarks(tenon.Bool(true), stamp{id: "plain"}))
	return failure
}

// deserializeFailure returns the error value of input that is not a document.
func deserializeFailure() tenon.Value {
	_, failure, _ := tenon.Deserialize(nil, tenon.Decoders{})
	return failure
}

// projectFailure returns the error value of a value that cannot be projected.
func projectFailure() tenon.Value {
	_, failure, _ := tenon.ProjectJSON(tenon.Unknown(tenon.NumberType()))
	return failure
}
