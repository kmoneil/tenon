package tenon_test

import (
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
)

func TestConformance_MK011_DiagnosticsWithholdRedactedContents(t *testing.T) {
	conformance.Covers(t, "MK-011")
	num, str := tenon.NumberType(), tenon.StringType()
	secret := stamp{id: "secret", redact: true}
	pii := stamp{id: "pii", policy: tenon.Isolate, redact: true}
	plain := stamp{id: "plain"}
	hunter, fortyTwo, fifty := tenon.String("hunter2"), tenon.NumberFromInt(42), tenon.NumberFromInt(50)
	// Crossed bounds leave a range that holds null null alone (UN-004), so
	// the ranges below exclude it to reach a contradiction.
	notNull := tenon.Narrow(tenon.Unknown(num), tenon.NotNull())

	// A division by zero over a secret numerator says nothing of the
	// numerator, and the error value carries the secret's mark, whether the
	// numerator is known or not known yet (UN-011).
	for _, numerator := range []tenon.Value{fortyTwo, tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(fortyTwo, true))} {
		div := tenon.Div(tenon.WithMarks(numerator, secret), tenon.NumberFromInt(0))
		if !div.IsError() || strings.Contains(div.String(), "42") || !tenon.HasMark(div, secret) {
			t.Errorf("dividing a secret by zero gave %v", div)
		}
	}

	// Where a message would render a value carrying a redacting mark, it shows
	// a placeholder naming the mark: for the value, for a value within it, for
	// what its range says, and for a bound taken from it, whether the bound
	// is in the narrowing given, in the range already, or in a narrowing given
	// earlier in the same call. An Isolate mark redacts as a Propagate mark
	// does, because what is withheld is the value that carries it.
	secretBound := tenon.NumberMax(tenon.WithMarks(fortyTwo, secret), true)
	for _, tt := range []struct {
		name string
		got  tenon.Value
		want string
	}{
		{
			"a value",
			tenon.Narrow(tenon.WithMarks(hunter, secret), tenon.StringPrefix("ab-")),
			`the value redacted("secret") does not satisfy prefix "ab-"`,
		},
		{
			"a value within a value",
			tenon.Narrow(tenon.List(num, tenon.NumberFromInt(7), tenon.WithMarks(fortyTwo, pii)), tenon.LengthMax(1)),
			`the value list(number)[7, redacted("pii")] does not satisfy length <= 1`,
		},
		{
			// The value carries the mark, so its type, which is part of what
			// the mark withholds, goes unnamed.
			"what a range says",
			tenon.Narrow(tenon.WithMarks(tenon.Narrow(notNull, tenon.NumberMax(fortyTwo, true)), secret),
				tenon.NumberMin(fifty, true)),
			`no value satisfies both redacted("secret") and >= 50`,
		},
		{
			"a bound given",
			tenon.Narrow(fifty, secretBound),
			`the value 50 does not satisfy <= redacted("secret")`,
		},
		{
			// The bound's redacting mark reached the value it narrowed.
			"a bound in the range",
			tenon.Narrow(tenon.Narrow(notNull, secretBound), tenon.NumberMin(fifty, true)),
			`no value satisfies both redacted("secret") and >= 50`,
		},
		{
			"a bound given earlier in the same call",
			tenon.Narrow(notNull, secretBound, tenon.NumberMin(fifty, true)),
			`no value of type number satisfies both redacted("secret") and >= 50`,
		},
		{
			// The bounds leave null alone, which NotNull then excludes; the
			// message names the bounds, as it does with NotNull given first.
			"bounds that leave only null, then NotNull",
			tenon.Narrow(tenon.Unknown(num), secretBound, tenon.NumberMin(fifty, true), tenon.NotNull()),
			`no value of type number satisfies both redacted("secret") and >= 50`,
		},
		{
			"a narrowing given earlier in the same call, which a set holding an unknown cannot meet beside this one",
			tenon.Narrow(tenon.WithMarks(tenon.Set(num, tenon.NumberFromInt(7), tenon.Unknown(num)), secret),
				tenon.LengthMin(2), tenon.LengthMax(1)),
			`the value redacted("secret") does not satisfy both redacted("secret") and length <= 1`,
		},
		{
			"a bound under an Isolate mark",
			tenon.Narrow(fifty, tenon.NumberMax(tenon.WithMarks(fortyTwo, pii), true)),
			`the value 50 does not satisfy <= redacted("pii")`,
		},
		{
			"a value under two redacting marks",
			tenon.Narrow(tenon.WithMarks(hunter, secret, pii), tenon.StringPrefix("ab-")),
			`the value redacted("pii", "secret") does not satisfy prefix "ab-"`,
		},
		// A mark that does not redact withholds nothing, and it does not show
		// either, since it cannot change a message (MK-005).
		{
			"a mark that does not redact",
			tenon.Narrow(tenon.WithMarks(hunter, plain), tenon.StringPrefix("ab-")),
			`the value "hunter2" does not satisfy prefix "ab-"`,
		},
		{
			"a mark that does not redact beside one that does",
			tenon.Narrow(tenon.WithMarks(hunter, secret, plain), tenon.StringPrefix("ab-")),
			`the value redacted("secret") does not satisfy prefix "ab-"`,
		},
		{
			"values within a value, one redacted and one not",
			tenon.Narrow(tenon.List(num, tenon.WithMarks(tenon.NumberFromInt(7), plain), tenon.WithMarks(fortyTwo, pii)), tenon.LengthMax(1)),
			`the value list(number)[7, redacted("pii")] does not satisfy length <= 1`,
		},
		{
			"a string that is not a number",
			tenon.Convert(tenon.WithMarks(hunter, plain), tenon.Exactly(num), tenon.Unsafe),
			`"hunter2" is not a number`,
		},
		{
			"a string that is not a number, redacted",
			tenon.Convert(tenon.WithMarks(hunter, secret, plain), tenon.Exactly(num), tenon.Unsafe),
			`redacted("secret") does not convert to exactly(number)`,
		},
	} {
		if !tt.got.IsError() {
			t.Errorf("%s: %v is not an error value", tt.name, tt.got)
			continue
		}
		if ds := tt.got.Diagnostics(); len(ds) != 1 || ds[0].Message != tt.want {
			t.Errorf("%s: the diagnostics are %v, want the message %s", tt.name, ds, tt.want)
		}
	}

	// A value is described the same way outside a diagnostic, so a message a
	// caller builds from String withholds it too. An error value shows its
	// diagnostics, which withheld what they had to when they were made, and
	// unmarking a value is how a caller shows it on purpose.
	sealed := tenon.WithMarks(tenon.Set(str, hunter), stamp{id: "secret", redact: true, deep: true})
	for _, tt := range []struct {
		name, got, want string
	}{
		{"a known value", tenon.WithMarks(hunter, secret).String(), `redacted("secret")`},
		{"a null value", tenon.WithMarks(tenon.Null(str), secret).String(), `redacted("secret")`},
		{
			"an unknown value",
			tenon.WithMarks(tenon.Narrow(tenon.Unknown(num), tenon.NumberMax(fortyTwo, true)), secret).String(),
			`redacted("secret")`,
		},
		{
			"the range of an unknown value",
			tenon.WithMarks(tenon.Narrow(tenon.Unknown(num), tenon.NumberMax(fortyTwo, true)), secret).Range().String(),
			`redacted("secret")`,
		},
		{
			"a pending value",
			tenon.WithMarks(tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly()), secret).String(),
			`redacted("secret")`,
		},
		{
			"a value within a value",
			tenon.List(str, tenon.String("shown"), tenon.WithMarks(hunter, secret)).String(),
			`list(string)["shown", redacted("secret")]`,
		},
		{
			"a value under two marks that share an identifier",
			tenon.WithMarks(hunter, secret, stamp{id: "secret", redact: true, deep: true}).String(),
			`redacted("secret")`,
		},
		{"a set under a deep redacting mark", sealed.String(), `redacted("secret")`},
		{"a member read out of that set", sealed.Elements()[0].String(), `redacted("secret")`},
		{"a narrowing with a secret bound", secretBound.String(), `<= redacted("secret")`},
		{
			"an error value",
			tenon.WithMarks(tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: "it failed"}), secret).String(),
			`marked(error(app.failed: "it failed"), "secret")`,
		},
		{"a value unmarked on purpose", unmarkedDeep(sealed).String(), `set(string)["hunter2"]`},
	} {
		if tt.got != tt.want {
			t.Errorf("%s reads %s, want %s", tt.name, tt.got, tt.want)
		}
	}
}

// TestConformance_MK011_RedactionWithholdsStructure holds a redacting mark to
// withholding a value's structure as well as its contents: the keys of a map
// and the names of an object's attributes, which a diagnostic's path, a
// message naming the value's type, or a result whose type is built from them
// would otherwise show.
func TestConformance_MK011_RedactionWithholdsStructure(t *testing.T) {
	conformance.Covers(t, "MK-011", "MK-005", "CV-050", "CV-033", "SE-050")
	num, str := tenon.NumberType(), tenon.StringType()
	secret := stamp{id: "secret", redact: true}
	leaks := func(what string, v tenon.Value) {
		t.Helper()
		for _, d := range errorsOf(v) {
			if strings.Contains(d.Message, "hunter2") || strings.Contains(d.Path.String(), "hunter2") {
				t.Errorf("%s: %s at %s shows what the mark withholds", what, d.Message, d.Path)
			}
		}
		if strings.Contains(v.String(), "hunter2") {
			t.Errorf("%s: %v shows what the mark withholds", what, v)
		}
	}
	vault := tenon.WithMarks(tenon.Map(str, map[string]tenon.Value{"hunter2": tenon.String("x"), "other": tenon.String("y")}), secret)

	// A failure within a redacted map is located at the map, once for each
	// code, named by the placeholder and the constraint converted to, which
	// a field that admits one type gives as Exactly of it (CV-026); the code
	// is kept (MK-005).
	for _, tt := range []struct {
		name  string
		v     tenon.Value
		c     tenon.Constraint
		p     tenon.Policy
		code  tenon.Code
		path  string
		inner string
	}{
		{"the map itself", vault, tenon.MapOf(tenon.Exactly(num)), tenon.Unsafe, tenon.CodeNumberInvalidSyntax, ".", "map_of(exactly(number))"},
		{"a map within an object", tenon.Object(map[string]tenon.Value{"vault": vault}), tenon.ObjectWith(map[string]tenon.Field{"vault": tenon.Required(tenon.MapOf(tenon.Exactly(num)))}, true),
			tenon.Unsafe, tenon.CodeNumberInvalidSyntax, ".vault", "exactly(map(number))"},
		{"under the safe policy", vault, tenon.MapOf(tenon.Exactly(num)), tenon.Safe, tenon.CodeConvertUnsafe, ".", "map_of(exactly(number))"},
	} {
		got := tenon.Convert(tt.v, tt.c, tt.p)
		ds := errorsOf(got)
		want := `redacted("secret") does not convert to ` + tt.inner
		if len(ds) != 1 || ds[0].Code != tt.code || ds[0].Path.String() != tt.path || ds[0].Message != want {
			t.Errorf("%s: %v, want %s: %q at %s", tt.name, got, tt.code, want, tt.path)
		}
		leaks(tt.name, got)
	}

	// A redacted object that does not convert names no attribute.
	record := tenon.WithMarks(tenon.Object(map[string]tenon.Value{"hunter2": tenon.NumberFromInt(1)}), secret)
	leaks("a redacted object converted to a number", tenon.Convert(record, tenon.Exactly(num), tenon.Unsafe))

	// A value holding a redacted one is named by its kind where a message
	// would name its type, since its type names the redacted value's
	// attributes: when it does not convert, when only the unsafe policy
	// would convert it, and when it converts to no member of a OneOf.
	for _, h := range []struct {
		kind   string
		holder tenon.Value
	}{
		{"a tuple", tenon.Tuple(record, tenon.Object(map[string]tenon.Value{"b": tenon.NumberFromInt(2)}))},
		{"an object", tenon.Object(map[string]tenon.Value{"outer": record})},
		{"a list", tenon.List(record.Type(), record)},
		{"a map", tenon.Map(record.Type(), map[string]tenon.Value{"k": record})},
	} {
		for _, tt := range []struct {
			to   string
			c    tenon.Constraint
			p    tenon.Policy
			code tenon.Code
			want string
		}{
			{"a number", tenon.Exactly(num), tenon.Unsafe, tenon.CodeConvertNoConversion, " does not convert to exactly(number)"},
			{"one of a number and a string", tenon.OneOf(tenon.Exactly(num), tenon.Exactly(str)), tenon.Unsafe, tenon.CodeConvertNoConversion,
				" does not convert to one_of([exactly(number), exactly(string)])"},
		} {
			got := tenon.Convert(h.holder, tt.c, tt.p)
			if ds := errorsOf(got); len(ds) != 1 || ds[0].Code != tt.code || ds[0].Message != h.kind+tt.want {
				t.Errorf("%s holding a redacted object converted to %s gave %v, want %s: %q", h.kind, tt.to, got, tt.code, h.kind+tt.want)
			}
			leaks(h.kind+" holding a redacted object converted to "+tt.to, got)
		}
	}
	safe := tenon.Convert(tenon.List(record.Type(), record), tenon.SetOf(tenon.Any()), tenon.Safe)
	if ds := errorsOf(safe); len(ds) != 1 || ds[0].Message != "a list converts to set_of(any) only unsafely, and the policy is safe" {
		t.Errorf("a list holding a redacted object converted to a set under the safe policy gave %v", safe)
	}
	leaks("a list holding a redacted object converted to a set under the safe policy", safe)

	// A narrowing that contradicts an unknown value carrying the mark leaves
	// its type unnamed, which would name the attributes the mark withholds.
	narrowed := tenon.Narrow(tenon.WithMarks(tenon.Unknown(record.Type()), secret), tenon.NullOnly(), tenon.NotNull())
	if ds := errorsOf(narrowed); len(ds) != 1 || ds[0].Code != tenon.CodeRangeContradiction || !strings.HasPrefix(ds[0].Message, "no value satisfies both") {
		t.Errorf("a redacted unknown object narrowed to nothing gave %v", narrowed)
	}
	leaks("a redacted unknown object narrowed to nothing", narrowed)

	// A list whose element type takes attribute names from a redacted map,
	// through the object it converts to, carries the mark, since its type and
	// the members given those attributes would show them. One whose element
	// type takes nothing from its redacted members is left as it is.
	mixed := tenon.List(tenon.MapType(num),
		tenon.WithMarks(tenon.Map(num, map[string]tenon.Value{"hunter2": tenon.NumberFromInt(1)}), secret),
		tenon.Map(num, map[string]tenon.Value{"b": tenon.NumberFromInt(2)}))
	objects := tenon.Convert(mixed, tenon.ListOf(tenon.ObjectWith(nil, false)), tenon.Unsafe)
	if objects.IsError() || !tenon.HasMark(objects, secret) {
		t.Errorf("a list taking attribute names from a redacted map converted to %v, not carrying its mark", objects)
	}
	leaks("a list taking attribute names from a redacted map", objects)
	numbers := tenon.Convert(tenon.List(num, tenon.WithMarks(tenon.NumberFromInt(1), secret), tenon.NumberFromInt(2)), tenon.ListOf(tenon.Any()), tenon.Safe)
	if _, marks := tenon.Unmark(numbers); len(marks) != 0 || numbers.String() != `list(number)[redacted("secret"), 2]` {
		t.Errorf("a list of numbers, one redacted, converted to %v", numbers)
	}

	// An operand is named by the placeholder: whether it is null, and the
	// constraint a pending one will satisfy, are what the mark withholds.
	null := tenon.Add(tenon.WithMarks(tenon.Null(num), secret), tenon.NumberFromInt(1))
	if ds := errorsOf(null); len(ds) != 1 || ds[0].Code != tenon.CodeOperationNullOperand || strings.Contains(ds[0].Message, "null") {
		t.Errorf("adding to a redacted null gave %v", null)
	}
	pending := tenon.Add(tenon.WithMarks(tenon.Pending(tenon.Exactly(str)), secret), tenon.NumberFromInt(1))
	if ds := errorsOf(pending); len(ds) != 1 || ds[0].Code != tenon.CodeOperationWrongType || strings.Contains(ds[0].Message, "string") {
		t.Errorf("adding to a redacted pending string gave %v", pending)
	}

	// Serialize locates what fails within a redacted value at the value.
	holder := tenon.Object(map[string]tenon.Value{"vault": tenon.WithMarks(
		tenon.Map(num, map[string]tenon.Value{"hunter2": tenon.WithMarks(tenon.NumberFromInt(1), stamp{id: "plain"})}), secret)})
	_, failure, ok := trySerialize(holder)
	if ds := errorsOf(failure); ok || len(ds) != 1 || ds[0].Code != tenon.CodeSerializeUnencodableMark || ds[0].Path.String() != ".vault" {
		t.Errorf("serializing a redacted map holding an unencodable mark gave %v", failure)
	}
	leaks("serializing a redacted map", failure)
}

// errorsOf returns the diagnostics of v, or none where v is no error value.
func errorsOf(v tenon.Value) []tenon.Diagnostic {
	if v.IsZero() || !v.IsError() {
		return nil
	}
	return v.Diagnostics()
}

// TestConformance_MK002_RedactingMarksAlwaysPropagate holds a redacting mark
// whose policy is Isolate to propagating as any redacting mark does, so that
// nothing derived from the value it withholds shows it: a narrowing taken
// from it, an operation over it, and a conversion of a set it is deep on. An
// Isolate mark that does not redact still stays where it was put.
func TestConformance_MK002_RedactingMarksAlwaysPropagate(t *testing.T) {
	conformance.Covers(t, "MK-002", "MK-003", "MK-011")
	num, str := tenon.NumberType(), tenon.StringType()
	pii := stamp{id: "pii", policy: tenon.Isolate, redact: true}
	deepPii := stamp{id: "pii", policy: tenon.Isolate, redact: true, deep: true}
	fortyTwo := tenon.WithMarks(tenon.NumberFromInt(42), pii)
	for _, tt := range []struct {
		name string
		got  tenon.Value
		mark tenon.Mark
	}{
		{"a narrowing taken from it", tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(fortyTwo, true)), pii},
		{"an operation over it", tenon.Add(fortyTwo, tenon.NumberFromInt(0)), pii},
		{"a set it is deep on, converted to a list", tenon.Convert(
			tenon.WithMarks(tenon.Set(str, tenon.String("hunter2")), deepPii), tenon.ListOf(tenon.Exactly(str)), tenon.Safe), deepPii},
	} {
		if !tenon.HasMark(tt.got, tt.mark) {
			t.Errorf("%s gave %v, not carrying the redacting mark", tt.name, tt.got)
		}
		if text := tt.got.String(); strings.Contains(text, "42") || strings.Contains(text, "hunter2") {
			t.Errorf("%s displays as %s, showing what the mark withholds", tt.name, text)
		}
	}
	quiet := stamp{id: "quiet", policy: tenon.Isolate}
	if sum := tenon.Add(tenon.WithMarks(tenon.NumberFromInt(42), quiet), tenon.NumberFromInt(0)); tenon.HasMark(sum, quiet) || sum.String() != "42" {
		t.Errorf("an Isolate mark that does not redact reached the sum: %v", sum)
	}
}

// TestConformance_ER001_PanicsNameRedactedValuesByTheirMarks holds a usage
// panic over a value carrying a redacting mark to what its display shows,
// since a panic's message reaches crash reports and logs: the value is named
// by its marks alone, never by its type, whose attribute names are its shape,
// and the reason for the panic is withheld where it would say whether the
// value is null, known or pending, what kind it is, or what it holds. Where
// the call refuses a marked value anyway, it says that, which holds of every
// redacted value alike. A value under a mark that does not redact is named by
// its type, as before.
func TestConformance_ER001_PanicsNameRedactedValuesByTheirMarks(t *testing.T) {
	conformance.Covers(t, "ER-001")
	secret := stamp{id: "secret", redact: true}
	objT := tenon.ObjectType(map[string]tenon.Type{"password": tenon.StringType()})
	num, str := tenon.NumberType(), tenon.StringType()
	obj := tenon.Object(map[string]tenon.Value{"password": tenon.String("hunter2")})
	red := tenon.WithMarks(obj, secret)
	redNull := tenon.WithMarks(tenon.Null(objT), secret)
	redUnknown := tenon.WithMarks(tenon.Unknown(objT), secret)
	redPending := tenon.WithMarks(tenon.Pending(tenon.Exactly(objT)), secret)
	redNum := tenon.WithMarks(tenon.NumberFromInt(42), secret)
	redList := tenon.WithMarks(tenon.List(num, tenon.NumberFromInt(42)), secret)
	one := tenon.NumberFromInt(1)
	const named, marked = `a value redacted by "secret"`, `a value redacted by "secret" that carries marks`
	for _, tt := range []struct {
		name, want string
		f          func()
	}{
		{"Hash of a known value", marked, func() { tenon.Hash(red) }},
		{"Hash of a null", marked, func() { tenon.Hash(redNull) }},
		{"Hash of an unknown value", marked, func() { tenon.Hash(redUnknown) }},
		{"CanonicalCompare", marked, func() { tenon.CanonicalCompare(redUnknown, one) }},
		{"Set", "element 0 is " + marked, func() { tenon.Set(objT, red) }},
		{"Set of another type", "element 0 is " + marked, func() { tenon.Set(str, red) }},
		{"Members", marked + " as member 0", func() { tenon.Members(redPending) }},
		{"a path's key", marked + " as a key", func() { tenon.Path{}.Index(redNull) }},
		{"Attribute", "Attribute cannot take " + named, func() { red.Attribute("nope") }},
		{"Attribute of a null", "Attribute cannot take " + named, func() { redNull.Attribute("password") }},
		{"AsString", "AsString cannot take " + named, func() { redNum.AsString() }},
		{"Len", "Len cannot take " + named, func() { redNum.Len() }},
		{"Index", "Index cannot take " + named, func() { redNum.Index(0) }},
		{"Index past the last element", "Index(5) cannot take " + named, func() { redList.Index(5) }},
		{"Elements", "Elements cannot take " + named, func() { redUnknown.Elements() }},
		{"Type", "Type cannot take " + named, func() { redPending.Type() }},
		{"Constraint", "Constraint cannot take " + named, func() { red.Constraint() }},
		{"Resolve", "Resolve cannot take " + named, func() { tenon.Resolve(red, objT) }},
		{"Resolve to a type that does not satisfy", "Resolve cannot take " + named, func() { tenon.Resolve(redPending, str) }},
		{"Range", "Range cannot take " + named, func() { redPending.Range() }},
		{"a bound", "NumberMin cannot take " + named, func() { tenon.NumberMin(red, true) }},
		{"Narrow", "Narrow cannot take " + named, func() { tenon.Narrow(red, tenon.NumberMin(one, true)) }},
		{"Narrow of a pending value", "Narrow cannot take " + named, func() { tenon.Narrow(redPending, tenon.NumberMin(one, true)) }},
		{"a list's element", "element 0 is " + named, func() { tenon.List(str, redNum) }},
		{"a list's pending element", "element 0 is " + named, func() { tenon.List(num, redPending) }},
		{"an operand", "the first operand is " + named, func() { tenon.Add(red, one) }},
		{"operands of two types", "the first operand is " + named, func() { tenon.LessThan(redNum, tenon.String("a")) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var msg string
			func() {
				defer func() { msg, _ = recover().(string) }()
				tt.f()
			}()
			if !strings.HasPrefix(msg, "tenon: usage: ") || !strings.Contains(msg, tt.want) {
				t.Fatalf("panicked with %q, want a usage error containing %q", msg, tt.want)
			}
			for _, withheld := range []string{"password", "hunter2", "42", "object", "of type", "null", "known", "pending", "kind", "content", "no range", "no type", "satisfy", "elements"} {
				if strings.Contains(msg, withheld) {
					t.Errorf("panicked with %q, which says %q of the redacted value", msg, withheld)
				}
			}
		})
	}
	mustPanicUsage(t, `Hash called on a value of type object({"password": string}) that carries marks`, func() {
		tenon.Hash(tenon.WithMarks(obj, stamp{id: "plain"}))
	})
	mustPanicUsage(t, `Attribute called on a value of type object({"password": string}), which has no attribute "nope"`, func() {
		tenon.WithMarks(obj, stamp{id: "plain"}).Attribute("nope")
	})
}
