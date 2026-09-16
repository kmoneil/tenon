package tenon_test

import (
	"strings"
	"testing"
	"unicode"

	"tenon"
	"tenon/conformance"
	"tenon/conformance/values"
	"tenon/internal/uni"
)

// esc returns the display escape of the code point written in hex, as in
// esc("200B").
func esc(hex string) string { return `\` + "u{" + hex + "}" }

// wantDisplay fails t unless got displays as want.
func wantDisplay(t *testing.T, what string, got interface{ String() string }, want string) {
	t.Helper()
	if s := got.String(); s != want {
		t.Errorf("%s displays as\n  %s\nwant\n  %s", what, s, want)
	}
}

// spot is what the display tests encapsulate.
type spot struct{ x, y int }

func TestConformance_DI010_EveryValueHasADisplayForm(t *testing.T) {
	conformance.Covers(t, "DI-010")
	num, str := tenon.NumberType(), tenon.StringType()
	one := tenon.NumberFromInt(1)
	for _, tt := range []struct {
		name string
		v    tenon.Value
		want string
	}{
		{"an error value", tenon.ErrorVal(
			tenon.Diagnostic{Code: tenon.CodeNumberDivideByZero, Message: "division by zero", Path: tenon.Path{}.Attribute("a").Index(tenon.NumberFromInt(0))},
			tenon.Diagnostic{Code: "app.failed", Message: "it failed"}),
			`error(number.divide_by_zero: "division by zero" at .a[0]; app.failed: "it failed")`},
		{"a pending value", tenon.Pending(tenon.Any()), `pending(any)`},
		{"a pending value known not to be null", tenon.Narrow(tenon.Pending(tenon.ListOf(tenon.Exactly(num))), tenon.NotNull()), `pending(list_of(exactly(number)), not null)`},
		{"a pending value known to be null", tenon.Narrow(tenon.Pending(tenon.Any()), tenon.Null()), `pending(any, null)`},
		{"a null", tenon.NullVal(tenon.Map(num)), `null(map(number))`},
		{"an unknown value", tenon.Unknown(str), `unknown(string)`},
		{"an unknown value with facts", tenon.Narrow(tenon.Unknown(num), tenon.NotNull(), tenon.NumberMin(one, false)), `unknown(number, not null, > 1)`},
		{"false", tenon.Bool(false), `false`},
		{"a number", tenon.NumberFromText("-12.50e-1"), `-1.25`},
		{"a large number", tenon.NumberFromText("1e21"), `1e21`},
		{"a string", tenon.String(`say "hi"`), `"say \"hi\""`},
		{"a list", tenon.ListVal(num, one, tenon.Unknown(num)), `list(number)[1, unknown(number)]`},
		{"an empty list", tenon.ListVal(str), `list(string)[]`},
		{"a set", tenon.SetVal(num, tenon.NumberFromInt(2), one), `set(number)[1, 2]`},
		{"a map", tenon.MapVal(num, map[string]tenon.Value{"b": one, "a": tenon.NullVal(num)}), `map(number){"a": null(number), "b": 1}`},
		{"a tuple", tenon.TupleVal(one, tenon.String("x"), tenon.NullVal(str)), `[1, "x", null(string)]`},
		{"an empty tuple", tenon.TupleVal(), `[]`},
		{"an object", tenon.ObjectVal(map[string]tenon.Value{"name": tenon.String("x"), "list": tenon.ListVal(num)}), `{"list": list(number)[], "name": "x"}`},
		{"an empty object", tenon.ObjectVal(nil), `{}`},
		{"a capsule value", tenon.CapsuleVal(tenon.Capsule("spot", tenon.CapsuleOps[spot]{}), &spot{1, 2}), `capsule("spot")`},
		{"a marked value", tenon.WithMarks(one, stamp{id: "m"}), `marked(1, "m")`},
		{"a redacted value", tenon.WithMarks(one, stamp{id: "s", redact: true}), `redacted("s")`},
	} {
		wantDisplay(t, tt.name, tt.v, tt.want)
	}
}

func TestConformance_DI011_DisplayTellsValuesApart(t *testing.T) {
	conformance.Covers(t, "DI-011")
	// Over the generator's values, two display alike exactly when they are
	// identical, other than capsule values, which display by what their type
	// declares.
	all := values.All()
	texts := make([]string, len(all))
	for i, v := range all {
		texts[i] = v.String()
	}
	for i, a := range all {
		for j, b := range all {
			if strings.Contains(texts[i], "capsule(") || strings.Contains(texts[j], "capsule(") {
				continue
			}
			if same := texts[i] == texts[j]; same != tenon.Identical(a, b) {
				t.Errorf("%s and %s: same display %t, identical %t", texts[i], texts[j], same, !same)
			}
		}
	}

	// Pairs that differ only in what a looser display would leave out.
	num, str := tenon.NumberType(), tenon.StringType()
	one := tenon.NumberFromInt(1)
	for _, pair := range [][2]tenon.Value{
		{one, tenon.WithMarks(one, stamp{id: "m"})},
		{tenon.NullVal(num), tenon.NullVal(str)},
		{tenon.TupleVal(), tenon.ListVal(num)},
		{tenon.ListVal(num), tenon.ListVal(str)},
		{tenon.ListVal(num, one), tenon.SetVal(num, one)},
		{tenon.ObjectVal(map[string]tenon.Value{"a": one}), tenon.MapVal(num, map[string]tenon.Value{"a": one})},
		{tenon.Unknown(num), tenon.Narrow(tenon.Unknown(num), tenon.NotNull())},
		{tenon.Pending(tenon.Any()), tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NotNull())},
		{tenon.String("a b"), tenon.String("a\U000000A0b")},
		{tenon.String("ab"), tenon.String("a\U0000200Bb")},
		{
			tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: "x at .y"}),
			tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: "x", Path: tenon.Path{}.Attribute("y")}),
		},
		{
			tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: "x", Path: tenon.Path{}.Attribute("a b")}),
			tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: "x", Path: tenon.Path{}.Index(tenon.String("a b"))}),
		},
	} {
		if pair[0].String() == pair[1].String() {
			t.Errorf("%v and %v display alike", pair[0], pair[1])
		}
	}

	// The exceptions: a redacting mark withholds what differs, and marks and
	// capsules are shown by what they declare.
	secret := stamp{id: "s", redact: true}
	opaque := tenon.Capsule("spot", tenon.CapsuleOps[spot]{})
	for _, pair := range [][2]tenon.Value{
		{tenon.WithMarks(one, secret), tenon.WithMarks(tenon.String("x"), secret)},
		{tenon.WithMarks(one, stamp{id: "m"}), tenon.WithMarks(one, stamp{id: "m", policy: tenon.Isolate})},
		{tenon.CapsuleVal(opaque, &spot{1, 2}), tenon.CapsuleVal(opaque, &spot{1, 2})},
	} {
		if tenon.Identical(pair[0], pair[1]) || pair[0].String() != pair[1].String() {
			t.Errorf("%v and %v: expected an exception to injectivity", pair[0], pair[1])
		}
	}
}

func TestConformance_DI012_QuotedText(t *testing.T) {
	conformance.Covers(t, "DI-012")
	// The categories are those of the pinned Unicode version.
	if unicode.Version != uni.UnicodeVersion {
		t.Fatalf("Go's Unicode tables are version %s, and the display form needs %s", unicode.Version, uni.UnicodeVersion)
	}
	for _, tt := range []struct {
		name, in, want string
	}{
		{"quotation mark and reverse solidus", `a"b\c`, `"a\"b\\c"`},
		{"tab, line feed and carriage return", "\t\n\r", `"\t\n\r"`},
		{"other controls", "\x00\x1f\x7f\U00000085", `"` + esc("0000") + esc("001F") + esc("007F") + esc("0085") + `"`},
		{"spaces other than the space", " \U000000A0\U00003000", `" ` + esc("00A0") + esc("3000") + `"`},
		{"separators", "\U00002028\U00002029", `"` + esc("2028") + esc("2029") + `"`},
		{"format characters", "\U000000AD\U0000200B\U0000202E\U0000FEFF\U000E0001", `"` + esc("00AD") + esc("200B") + esc("202E") + esc("FEFF") + esc("E0001") + `"`},
		{"private use", "\U0000E000\U000F0000", `"` + esc("E000") + esc("F0000") + `"`},
		{"noncharacters", "\U0000FDD0\U0000FFFF\U0010FFFF", `"` + esc("FDD0") + esc("FFFF") + esc("10FFFF") + `"`},
		{"unassigned in Unicode 15.0", "\U00000378\U00002FFC", `"` + esc("0378") + esc("2FFC") + `"`},
		{"everything else as itself", "caf\U000000E9 \U0001F600 x\U00000301 \U00004E2D \U000000A2 \U00000627", "\"caf\U000000E9 \U0001F600 x\U00000301 \U00004E2D \U000000A2 \U00000627\""},
	} {
		wantDisplay(t, tt.name, tenon.String(tt.in), tt.want)
	}

	// Every text of a display form is quoted alike: keys, attribute names,
	// mark identifiers, capsule names and display forms, and messages.
	zw := "a\U0000200Bb"
	quoted := `"a` + esc("200B") + `b"`
	num := tenon.NumberType()
	capsule := tenon.Capsule(zw, tenon.CapsuleOps[spot]{Display: func(*spot) string { return zw }})
	for _, tt := range []struct {
		name string
		got  interface{ String() string }
		want string
	}{
		{"a map key", tenon.MapVal(num, map[string]tenon.Value{zw: tenon.NumberFromInt(1)}), `map(number){` + quoted + `: 1}`},
		{"an attribute name", tenon.Object(map[string]tenon.Type{zw: num}), `object({` + quoted + `: number})`},
		{"a mark identifier", tenon.WithMarks(tenon.Bool(true), stamp{id: zw}), `marked(true, ` + quoted + `)`},
		{"a capsule", tenon.CapsuleVal(capsule, &spot{}), `capsule(` + quoted + `, ` + quoted + `)`},
		{"a message", tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: zw}), `error(app.failed: ` + quoted + `)`},
	} {
		wantDisplay(t, tt.name, tt.got, tt.want)
	}
}

func TestConformance_DI013_PathDisplay(t *testing.T) {
	conformance.Covers(t, "DI-013")
	root := tenon.Path{}
	for _, tt := range []struct {
		name string
		p    tenon.Path
		want string
	}{
		{"the empty path", root, `.`},
		{"an identifier", root.Attribute("name"), `.name`},
		{"identifiers with underscores and digits", root.Attribute("_a1").Attribute("b_2"), `._a1.b_2`},
		{"a name that is no identifier", root.Attribute("a b"), `."a b"`},
		{"a name beginning with a digit", root.Attribute("9lives"), `."9lives"`},
		{"a name beyond ASCII", root.Attribute("caf\U000000E9"), ".\"caf\U000000E9\""},
		{"an index first", root.Index(tenon.NumberFromInt(0)), `.[0]`},
		{"a fractional index", root.Index(tenon.NumberFromText("2.5")), `.[2.5]`},
		{"a key first", root.Index(tenon.String("k")), `.["k"]`},
		{"steps after an index", root.Index(tenon.NumberFromInt(1)).Attribute("a").Index(tenon.String("a b")).Attribute("c d"), `.[1].a["a b"]."c d"`},
		{"a key needing escapes", root.Index(tenon.String("a\U0000200B")), `.["a` + esc("200B") + `"]`},
	} {
		wantDisplay(t, tt.name, tt.p, tt.want)
	}
	// An attribute and a key of one name are different steps, and display so.
	if a, k := root.Attribute("a b").String(), root.Index(tenon.String("a b")).String(); a == k {
		t.Errorf("an attribute and a key both display as %s", a)
	}
}

func TestConformance_DI014_TypeAndConstraintDisplay(t *testing.T) {
	conformance.Covers(t, "DI-014")
	num, str, boo := tenon.NumberType(), tenon.StringType(), tenon.BoolType()
	for _, tt := range []struct {
		name string
		got  interface{ String() string }
		want string
	}{
		{"bool", boo, `bool`},
		{"a list", tenon.List(num), `list(number)`},
		{"a set of maps", tenon.Set(tenon.Map(str)), `set(map(string))`},
		{"a tuple", tenon.Tuple(num, str), `tuple([number, string])`},
		{"the empty tuple", tenon.Tuple(), `tuple([])`},
		{"an object", tenon.Object(map[string]tenon.Type{"b": num, "a b": str, "B": boo}), `object({"B": bool, "a b": string, "b": number})`},
		{"the empty object", tenon.Object(nil), `object({})`},
		{"a capsule type", tenon.Capsule("spot", tenon.CapsuleOps[spot]{}), `capsule("spot")`},
		{"any", tenon.Any(), `any`},
		{"exactly", tenon.Exactly(num), `exactly(number)`},
		{"collections", tenon.MapOf(tenon.SetOf(tenon.ListOf(tenon.Any()))), `map_of(set_of(list_of(any)))`},
		{"a tuple constraint", tenon.TupleOf(tenon.Exactly(num), tenon.Any()), `tuple_of([exactly(number), any])`},
		{"one of, in order", tenon.OneOf(tenon.Exactly(str), tenon.Exactly(num)), `one_of([exactly(string), exactly(number)])`},
		{"a closed object", tenon.ObjectWith(map[string]tenon.Field{
			"name": {Constraint: tenon.Exactly(str), Required: true},
			"tags": {Constraint: tenon.ListOf(tenon.Any())},
		}, true), `object_with({"name": exactly(string), "tags"?: list_of(any)}, closed)`},
		{"an open object", tenon.ObjectWith(nil, false), `object_with({}, open)`},
	} {
		wantDisplay(t, tt.name, tt.got, tt.want)
	}
}

func TestConformance_DI015_MarksAndRedaction(t *testing.T) {
	conformance.Covers(t, "DI-015")
	num, str := tenon.NumberType(), tenon.StringType()
	one := tenon.NumberFromInt(1)
	secret, other := stamp{id: "s", redact: true}, stamp{id: "r", redact: true}
	deep, secretDeep := stamp{id: "d", deep: true}, stamp{id: "s", redact: true, deep: true}
	for _, tt := range []struct {
		name string
		v    tenon.Value
		want string
	}{
		{"marks in string order", tenon.WithMarks(one, stamp{id: "b"}, stamp{id: "a"}, stamp{id: "B"}), `marked(1, "B", "a", "b")`},
		{"one identifier once", tenon.WithMarks(one, stamp{id: "m"}, stamp{id: "m", policy: tenon.Isolate}), `marked(1, "m")`},
		{"a marked null", tenon.WithMarks(tenon.NullVal(num), stamp{id: "m"}), `marked(null(number), "m")`},
		{"a marked unknown", tenon.WithMarks(tenon.Narrow(tenon.Unknown(num), tenon.NotNull()), stamp{id: "m"}), `marked(unknown(number, not null), "m")`},
		{"a marked pending value", tenon.WithMarks(tenon.Pending(tenon.Any()), stamp{id: "m"}), `marked(pending(any), "m")`},
		{"a marked member", tenon.ListVal(num, one, tenon.WithMarks(one, stamp{id: "m"})), `list(number)[1, marked(1, "m")]`},
		{"a deep mark on a list", tenon.WithMarks(tenon.ListVal(num, one), deep), `marked(list(number)[marked(1, "d")], "d")`},
		{"a deep mark on a set", tenon.WithMarks(tenon.SetVal(num, one), deep), `marked(set(number)[1], "d")`},
		// A redacting mark withholds everything about the value but itself.
		{"a redacted known value", tenon.WithMarks(one, secret), `redacted("s")`},
		{"a redacted null", tenon.WithMarks(tenon.NullVal(str), secret), `redacted("s")`},
		{"a redacted unknown", tenon.WithMarks(tenon.Narrow(tenon.Unknown(str), tenon.StringPrefix("ab")), secret), `redacted("s")`},
		{"a redacted pending value", tenon.WithMarks(tenon.Pending(tenon.Any()), secret), `redacted("s")`},
		{"other marks withheld too", tenon.WithMarks(one, secret, other, stamp{id: "m"}), `redacted("r", "s")`},
		{"a redacted member", tenon.ObjectVal(map[string]tenon.Value{"a": one, "b": tenon.WithMarks(tenon.String("x"), secret)}), `{"a": 1, "b": redacted("s")}`},
		{"a deep redacting mark", tenon.WithMarks(tenon.SetVal(str, tenon.String("x")), secretDeep), `redacted("s")`},
		// An error value shows its diagnostics, which withheld what they had
		// to, and all its marks.
		{"a redacted error", tenon.WithMarks(tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: "it failed"}), secret, stamp{id: "m"}),
			`marked(error(app.failed: "it failed"), "m", "s")`},
	} {
		wantDisplay(t, tt.name, tt.v, tt.want)
	}
	// A set member read out of a deeply marked set carries the mark, and the
	// set shows it once, on the set.
	set := tenon.WithMarks(tenon.SetVal(num, one), deep)
	wantDisplay(t, "a member read out", set.Elements()[0], `marked(1, "d")`)
}

func TestConformance_DI016_CapsuleDisplay(t *testing.T) {
	conformance.Covers(t, "DI-016")
	plain := tenon.Capsule("spot", tenon.CapsuleOps[spot]{})
	shown := tenon.Capsule("shown spot", tenon.CapsuleOps[spot]{
		Display: func(p *spot) string { return strings.Repeat("*", p.x) + "\n" + strings.Repeat("*", p.y) },
	})
	wantDisplay(t, "a capsule declaring no display form", tenon.CapsuleVal(plain, &spot{1, 2}), `capsule("spot")`)
	wantDisplay(t, "a capsule declaring one", tenon.CapsuleVal(shown, &spot{1, 2}), `capsule("shown spot", "*\n**")`)
	wantDisplay(t, "a capsule within a list", tenon.ListVal(shown, tenon.CapsuleVal(shown, &spot{0, 1})), `list(capsule("shown spot"))[capsule("shown spot", "\n*")]`)
	wantDisplay(t, "a null capsule", tenon.NullVal(shown), `null(capsule("shown spot"))`)
}

func TestConformance_DI017_OrderWithinADisplayForm(t *testing.T) {
	conformance.Covers(t, "DI-017")
	num, str := tenon.NumberType(), tenon.StringType()
	n := tenon.NumberFromInt
	for _, tt := range []struct {
		name string
		v    tenon.Value
		want string
	}{
		{"list elements in order", tenon.ListVal(num, n(3), n(1), n(2)), `list(number)[3, 1, 2]`},
		{"tuple elements in order", tenon.TupleVal(tenon.String("b"), n(1)), `["b", 1]`},
		{"map keys in string order", tenon.MapVal(num, map[string]tenon.Value{"b": n(1), "B": n(2), "\U000000E9": n(3), "a": n(4)}),
			"map(number){\"B\": 2, \"a\": 4, \"b\": 1, \"\U000000E9\": 3}"},
		{"attributes in string order", tenon.ObjectVal(map[string]tenon.Value{"z": n(1), "Z": n(2), "_": n(3)}), `{"Z": 2, "_": 3, "z": 1}`},
		{"set members in iteration order", tenon.SetVal(num, tenon.Unknown(num), n(10), tenon.NullVal(num), n(2)), `set(number)[null(number), 2, 10, unknown(number)]`},
		{"number facts", tenon.Narrow(tenon.Unknown(num), tenon.NumberMax(n(9), true), tenon.NumberMin(n(1), false), tenon.NotNull()), `unknown(number, not null, > 1, <= 9)`},
		{"string facts", tenon.Narrow(tenon.Unknown(str), tenon.LengthMax(5), tenon.StringPrefix("ab-"), tenon.NotNull(), tenon.LengthMin(3)),
			`unknown(string, not null, prefix "ab-", length >= 3, length <= 5)`},
		{"members last", tenon.Narrow(tenon.Unknown(tenon.Set(num)), tenon.Members(n(2), tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(n(5), true)), n(1)), tenon.LengthMax(4)),
			`unknown(set(number), length >= 3, length <= 4, members {1, 2, unknown(number, >= 5)})`},
	} {
		wantDisplay(t, tt.name, tt.v, tt.want)
	}
}
