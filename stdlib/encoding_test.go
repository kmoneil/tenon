package stdlib_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/stdlib"
)

// encodes checks that JSONEncode of v answers the text want.
func encodes(t *testing.T, v tenon.Value, want string) {
	t.Helper()
	if got := call(stdlib.JSONEncodeFunc, v); !got.Equal(tenon.String(want)) {
		t.Errorf("JSONEncode(%v) = %v, want %q", v, got, want)
	}
}

func TestConformance_LE001_JSONEncode(t *testing.T) {
	conformance.Covers(t, "LE-001")
	str, num := tenon.StringType(), tenon.NumberType()
	untyped := tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly())
	// go-cty's escapes, positional numbers, no whitespace.
	encodes(t, tenon.String("a<b>&c\U00002028\U00002029\"\\\n\x01"), `"a\u003cb\u003e\u0026c\u2028\u2029\"\\\n\u0001"`)
	encodes(t, tenon.NumberFromText("1e21"), "1000000000000000000000")
	encodes(t, tenon.NumberFromText("-1.5e-7"), "-0.00000015")
	encodes(t, tenon.NumberFromText("120.50"), "120.5")
	encodes(t, tenon.Bool(false), "false")
	encodes(t, tenon.List(num, tenon.NumberFromInt(1), tenon.NumberFromInt(2)), "[1,2]")
	encodes(t, tenon.Set(str, tenon.String("b"), tenon.String("a")), `["a","b"]`)
	encodes(t, tenon.Map(num, map[string]tenon.Value{"z": tenon.NumberFromInt(1), "a": tenon.Null(num)}), `{"a":null,"z":1}`)
	encodes(t, tenon.Object(map[string]tenon.Value{"name": tenon.String("web"), "ports": tenon.Tuple(tenon.NumberFromInt(80))}), `{"name":"web","ports":[80]}`)
	// A quotient is its 96 significant digits, exactly.
	third := call(stdlib.DivideFunc, tenon.NumberFromInt(1), tenon.NumberFromInt(3))
	encodes(t, third, "0."+strings.Repeat("3", 96))
	// Nulls, and what waits on untyped nulls, are known.
	encodes(t, tenon.Null(str), "null")
	encodes(t, untyped, "null")
	encodes(t, tenon.Tuple(untyped, tenon.NumberFromInt(1)), "[null,1]")
	encodes(t, tenon.Object(map[string]tenon.Value{"a": untyped}), `{"a":null}`)
	// A capsule is its display form, and one with none fails within the
	// value.
	opaque := tenon.NewCapsule("opaque", tenon.CapsuleOps[int]{})
	seven := 7
	failsWith(t, "JSONEncode([1, an opaque capsule])", call(stdlib.JSONEncodeFunc, tenon.Tuple(tenon.NumberFromInt(1), opaque.Value(&seven))), tenon.CodeSerializeUnencodableCapsule, at(0).Index(tenon.NumberFromInt(1)))
	shown := tenon.NewCapsule("shown", tenon.CapsuleOps[int]{Display: func(i *int) string { return "<" + strconv.Itoa(*i) + ">" }})
	encodes(t, shown.Value(&seven), `"\u003c7\u003e"`)
	// Marks are left to the boundary.
	secretText := tenon.WithMarks(tenon.String("x"), secret{})
	if got := call(stdlib.JSONEncodeFunc, tenon.List(str, secretText)); !got.Equal(tenon.WithMarks(tenon.String(`["x"]`), secret{})) {
		t.Errorf("JSONEncode of a marked member = %v, want its text carrying the mark", got)
	}
}

func TestConformance_LE002_JSONEncodeBound(t *testing.T) {
	conformance.Covers(t, "LE-002")
	// 1e999999 is nine bytes in its canonical text and a million written
	// out.
	failsWith(t, "JSONEncode(1e999999)", call(stdlib.JSONEncodeFunc, tenon.NumberFromText("1e999999")), tenon.CodeFunctionTooLarge, at(0))
	unknown := tenon.Narrow(tenon.Unknown(tenon.NumberType()), tenon.NotNull())
	failsWith(t, "JSONEncode([1e999999, unknown])", call(stdlib.JSONEncodeFunc, tenon.List(tenon.NumberType(), tenon.NumberFromText("1e999999"), unknown)), tenon.CodeFunctionTooLarge, at(0))
	// 1e60000 is 60,001 digits, within 64 times "1e+60000" and 65,536;
	// 1e70000's 70,001 are not.
	if got := call(stdlib.JSONEncodeFunc, tenon.NumberFromText("1e60000")); got.IsError() || len(got.AsString()) != 60001 {
		t.Errorf("JSONEncode(1e60000) = %v, want 60,001 digits", got)
	}
	failsWith(t, "JSONEncode(1e70000)", call(stdlib.JSONEncodeFunc, tenon.NumberFromText("1e70000")), tenon.CodeFunctionTooLarge, at(0))
	// Strings escape at most six bytes for one, well within the bound.
	if got := call(stdlib.JSONEncodeFunc, tenon.String(strings.Repeat("\x01", 100000))); got.IsError() {
		t.Errorf("JSONEncode of 100,000 controls = %v", got)
	}
}

func TestConformance_LE003_JSONEncodeNotKnown(t *testing.T) {
	conformance.Covers(t, "LE-003")
	str, num := tenon.StringType(), tenon.NumberType()
	notNull := func(v tenon.Value) tenon.Value { return tenon.Narrow(v, tenon.NotNull()) }
	for _, tt := range []struct {
		v      tenon.Value
		prefix string
	}{
		{notNull(tenon.Unknown(str)), `"`},
		{tenon.Unknown(str), ""},
		{notNull(tenon.Unknown(num)), ""},
		{notNull(tenon.Unknown(tenon.ListType(num))), "["},
		{notNull(tenon.Unknown(tenon.MapType(num))), "{"},
		{tenon.List(num, tenon.NumberFromInt(1), notNull(tenon.Unknown(num)), tenon.NumberFromInt(3)), "[1,"},
		{tenon.List(str, tenon.String("a"), notNull(tenon.Unknown(str))), `["a","`},
		{tenon.List(str, tenon.String("a"), tenon.Unknown(str)), `["a",`},
		{tenon.Set(num, tenon.NumberFromInt(1), notNull(tenon.Unknown(num))), "["},
		{tenon.Map(num, map[string]tenon.Value{"a": tenon.NumberFromInt(1), "b": notNull(tenon.Unknown(num))}), `{"a":1,"b":`},
		{tenon.Tuple(tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly()), tenon.Pending(tenon.Any())), "[null,"},
		{notNull(tenon.Pending(tenon.ListOf(tenon.Any()))), "["},
		{tenon.Pending(tenon.Any()), ""},
	} {
		got := call(stdlib.JSONEncodeFunc, tt.v)
		if got.IsKnown() || !notNullValue(got) || got.Range().StringPrefix() != tt.prefix {
			t.Errorf("JSONEncode(%v) = %v, want the unknown string, not null, beginning %q", tt.v, got, tt.prefix)
		}
	}
}

// notNullValue reports whether v is known not to be null.
func notNullValue(v tenon.Value) bool { return notNull(v) }

func TestConformance_LE004_JSONDecode(t *testing.T) {
	conformance.Covers(t, "LE-004")
	untyped := tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly())
	for _, tt := range []struct {
		text string
		want tenon.Value
	}{
		{` {"a": [1, "x", true], "b": 1.50} `, tenon.Object(map[string]tenon.Value{
			"a": tenon.Tuple(tenon.NumberFromInt(1), tenon.String("x"), tenon.Bool(true)),
			"b": tenon.NumberFromText("1.5"),
		})},
		{`123456789012345678901234567890`, tenon.NumberFromText("123456789012345678901234567890")},
		{`-0`, tenon.NumberFromInt(0)},
		{`"\u00e9"`, tenon.String("\U000000E9")},
		{`null`, untyped},
		{`[null, 1]`, tenon.Tuple(untyped, tenon.NumberFromInt(1))},
		{`{"a": null}`, tenon.Object(map[string]tenon.Value{"a": untyped})},
	} {
		if got := call(stdlib.JSONDecodeFunc, tenon.String(tt.text)); !tenon.Identical(got, tt.want) {
			t.Errorf("JSONDecode(%s) = %v, want %v", tt.text, got, tt.want)
		}
	}
	// ParseJSON's failures, located at the argument.
	for _, tt := range []struct {
		text string
		code tenon.Code
	}{
		{`1]garbage{{{`, tenon.CodeJSONInvalidSyntax},
		{`{}}`, tenon.CodeJSONInvalidSyntax},
		{`.5`, tenon.CodeJSONInvalidSyntax},
		{`1e1000000000`, tenon.CodeNumberOutOfRange},
		{`"\ud800"`, tenon.CodeStringInvalidUTF8},
		{strings.Repeat("[", 513) + strings.Repeat("]", 513), tenon.CodeJSONTooDeep},
	} {
		got := call(stdlib.JSONDecodeFunc, tenon.String(tt.text))
		if !got.IsError() || got.Diagnostics()[0].Code != tt.code || got.Diagnostics()[0].Path.Len() == 0 || !got.Diagnostics()[0].Path.Steps()[0].Key().Equal(tenon.NumberFromInt(0)) {
			t.Errorf("JSONDecode(%.20s) = %v, want %s at the argument", tt.text, got, tt.code)
		}
	}
	failsWith(t, "JSONDecode of a name given twice", call(stdlib.JSONDecodeFunc, tenon.String(`{"a":1,"a":2}`)), tenon.CodeObjectDuplicateName, at(0))
	// The derivation settles the type, and fails as the text does.
	if c, err := tenon.ResultConstraint(stdlib.JSONDecodeFunc, []tenon.Value{tenon.String(`[1, "a"]`)}, tenon.Safe); err != nil || !c.Equal(tenon.Exactly(tenon.TupleType(tenon.NumberType(), tenon.StringType()))) {
		t.Errorf("JSONDecode's derivation of [1, \"a\"] = %v, %v", c, err)
	}
	if _, err := tenon.ResultConstraint(stdlib.JSONDecodeFunc, []tenon.Value{tenon.String(`[1,`)}, tenon.Safe); err == nil {
		t.Errorf("JSONDecode's derivation of [1, does not fail")
	}
}

func TestConformance_LE005_JSONDecodeNotKnown(t *testing.T) {
	conformance.Covers(t, "LE-005")
	text := func(prefix string) tenon.Value {
		return tenon.Narrow(tenon.Unknown(tenon.StringType()), tenon.StringPrefix(prefix))
	}
	for _, tt := range []struct {
		prefix string
		kind   tenon.Kind
		begins string
	}{
		{` "abc `, tenon.KindString, "abc "},
		{`"a\n`, tenon.KindString, ""},
		{"tru", tenon.KindBool, ""},
		{"fal", tenon.KindBool, ""},
		{"-1", tenon.KindNumber, ""},
		{"12", tenon.KindNumber, ""},
	} {
		got := call(stdlib.JSONDecodeFunc, text(tt.prefix))
		if got.IsKnown() || got.IsPending() || got.Type().Kind() != tt.kind || !notNull(got) {
			t.Errorf("JSONDecode(unknown %q) = %v, want an unknown %v, not null", tt.prefix, got, tt.kind)
			continue
		}
		if tt.kind == tenon.KindString && got.Range().StringPrefix() != tt.begins {
			t.Errorf("JSONDecode(unknown %q) = %v, want it beginning %q", tt.prefix, got, tt.begins)
		}
	}
	if got := call(stdlib.JSONDecodeFunc, text("{ ")); !got.IsPending() || got.Constraint().Kind() != tenon.ConstraintObjectWith || !notNull(got) {
		t.Errorf("JSONDecode(unknown \"{\") = %v, want a pending object, not null", got)
	}
	if got := call(stdlib.JSONDecodeFunc, text("[ ")); !got.IsPending() || !notNull(got) {
		t.Errorf("JSONDecode(unknown \"[\") = %v, want a pending value, not null", got)
	}
	if got := call(stdlib.JSONDecodeFunc, text("nul")); !tenon.Identical(got, tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly())) {
		t.Errorf("JSONDecode(unknown \"nul\") = %v, want the untyped null", got)
	}
	if got := call(stdlib.JSONDecodeFunc, tenon.Unknown(tenon.StringType())); !got.IsPending() || notNull(got) {
		t.Errorf("JSONDecode(unknown) = %v, want a pending value that may be null", got)
	}
	failsWith(t, "JSONDecode(unknown \".5\")", call(stdlib.JSONDecodeFunc, text(".5")), tenon.CodeJSONInvalidSyntax, at(0))
	failsWith(t, "JSONDecode(unknown \"xy\")", call(stdlib.JSONDecodeFunc, text("xy")), tenon.CodeJSONInvalidSyntax, at(0))
}
