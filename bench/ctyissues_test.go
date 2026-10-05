package bench

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"math/big"
	"strconv"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/gotenon"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
	"github.com/zclconf/go-cty/cty/gocty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
	"github.com/zclconf/go-cty/cty/msgpack"
)

// The tests named TestCtyIssueN probe tenon with the case go-cty's open issue
// N reports, https://github.com/zclconf/go-cty/issues/N. Each asserts what
// go-cty v1.19.0, the version this module requires, does with the case, so
// that an upgrade that fixes the issue fails the test and says so, and then
// what tenon does with its counterpart.

// openCtyIssues is go-cty's open issues, as listed on 2026-10-05: those of
// 2026-10-01, unchanged since 2026-09-14, and #229 and #230, opened on
// 2026-10-02. TestEveryOpenCtyIssueIsProbed holds the file to it: each has a
// TestCtyIssueN or a reason in unprobed.
var openCtyIssues = []int{17, 90, 148, 211, 215, 216, 217, 219, 220, 221, 222, 223, 224, 225, 226, 227, 228, 229, 230}

// unprobed gives the reason for each open issue that reports nothing tenon
// could get wrong.
var unprobed = map[int]string{
	215: "a thank-you for the README's line about a language's reflection API, reporting no defect",
	217: "Substr's offsets: tenon defines no string functions, leaving them to the language built on it",
}

func TestEveryOpenCtyIssueIsProbed(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "ctyissues_test.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	probed := map[int]bool{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		name, ok := strings.CutPrefix(fn.Name.Name, "TestCtyIssue")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
		if err != nil {
			t.Fatalf("%s names no issue", fn.Name.Name)
		}
		probed[n] = true
	}
	open := map[int]bool{}
	for _, n := range openCtyIssues {
		open[n] = true
		if probed[n] == (unprobed[n] != "") {
			t.Errorf("go-cty #%d: probed %t, and a reason given for not probing it %t", n, probed[n], unprobed[n] != "")
		}
	}
	for n := range probed {
		if !open[n] {
			t.Errorf("go-cty #%d is probed but not listed open", n)
		}
	}
}

// ctyPanics runs f and returns what it panicked with, or nil.
func ctyPanics(f func()) (r any) {
	defer func() { r = recover() }()
	f()
	return nil
}

// answers calls f n times and counts each answer it gives.
func answers(n int, f func() string) map[string]int {
	seen := map[string]int{}
	for range n {
		seen[f()]++
	}
	return seen
}

// ctyAnswer describes a cty.Bool for answers.
func ctyAnswer(v cty.Value) string {
	if !v.IsKnown() {
		return "unknown"
	}
	return strconv.FormatBool(v.True())
}

// failsWith reports whether v is an error value whose first diagnostic has
// the code c.
func failsWith(v tenon.Value, c tenon.Code) bool {
	return v.IsError() && v.Diagnostics()[0].Code == c
}

// TestCtyIssue17_MapsAndObjectsInGo: gocty decodes an object only into a
// struct and a map only into a Go map. gotenon decodes as conversion does: an
// object into a Go map under either policy, and a map into a struct under the
// unsafe policy, which a map's keys need, since nothing says they are the
// struct's.
func TestCtyIssue17_MapsAndObjectsInGo(t *testing.T) {
	type port struct {
		Port int `cty:"port" tenon:"port"`
	}
	var m map[string]int
	if err := gocty.FromCtyValue(cty.ObjectVal(map[string]cty.Value{"port": cty.NumberIntVal(1)}), &m); err == nil {
		t.Error("cty decoded an object into a Go map; #17 is fixed")
	}
	var p port
	if err := gocty.FromCtyValue(cty.MapVal(map[string]cty.Value{"port": cty.NumberIntVal(1)}), &p); err == nil {
		t.Error("cty decoded a map into a struct; #17 is fixed")
	}

	num := tenon.NumberType()
	got, err := gotenon.Decode[map[string]int](tenon.Object(map[string]tenon.Value{"port": tenon.NumberFromInt(1)}), tenon.Safe)
	if err != nil || got["port"] != 1 {
		t.Errorf("tenon decoded an object into %v, %v", got, err)
	}
	fromMap := tenon.Map(num, map[string]tenon.Value{"port": tenon.NumberFromInt(1)})
	if p, err := gotenon.Decode[port](fromMap, tenon.Unsafe); err != nil || p.Port != 1 {
		t.Errorf("tenon decoded a map into %+v, %v", p, err)
	}
	if _, err := gotenon.Decode[port](fromMap, tenon.Safe); err == nil {
		t.Error("tenon decoded a map into a struct under the safe policy")
	}
}

// TestCtyIssue90_ModuloOfAnInfinity: Modulo panicked on an infinity. At
// v1.19.0 it no longer panics, and answers an infinity. tenon's numbers are
// finite: no infinity can be made, and a remainder modulo zero is an error.
func TestCtyIssue90_ModuloOfAnInfinity(t *testing.T) {
	var got cty.Value
	var err error
	if r := ctyPanics(func() {
		got, err = stdlib.ModuloFunc.Call([]cty.Value{cty.PositiveInfinity, cty.NumberIntVal(1)})
	}); r != nil || err != nil || !got.AsBigFloat().IsInf() {
		t.Errorf("cty's Modulo of an infinity gave %#v, %v, panicking with %v; it answered an infinity at v1.19.0", got, err, r)
	}

	for _, text := range []string{"Infinity", "inf", "+Inf"} {
		if v := tenon.NumberFromText(text); !failsWith(v, tenon.CodeNumberInvalidSyntax) {
			t.Errorf("tenon made %s of %q", v, text)
		}
	}
	if _, err := gotenon.Encode(math.Inf(1)); !hasCode(err, tenon.CodeEncodeNotANumber) {
		t.Errorf("tenon encoded an infinity, failing with %v", err)
	}
	if v := tenon.Mod(tenon.NumberFromInt(1), tenon.NumberFromInt(0)); !failsWith(v, tenon.CodeNumberModuloByZero) {
		t.Errorf("tenon's 1 mod 0 is %s", v)
	}
}

// hasCode reports whether err is a *tenon.Error whose first diagnostic has the
// code c.
func hasCode(err error, c tenon.Code) bool {
	var te *tenon.Error
	return errors.As(err, &te) && len(te.Diagnostics()) > 0 && te.Diagnostics()[0].Code == c
}

// userRecord is a Go type whose attribute names are another library's, which
// it decodes by a method of its own.
type userRecord struct {
	Name string `json:"user_name"`
}

func (u *userRecord) UnmarshalValue(v tenon.Value, p tenon.Policy) error {
	name, err := gotenon.Decode[string](v.Attribute("user_name"), p)
	u.Name = name
	return err
}

// TestCtyIssue148_AnotherTagKey: gocty reads only the cty tag, and the issue
// asks for another key. gotenon's key is tenon, and a type naming its
// attributes otherwise decodes by its own UnmarshalValue, since another
// key's grammar, as json's omitempty, would be misread.
func TestCtyIssue148_AnotherTagKey(t *testing.T) {
	var u userRecord
	if err := gocty.FromCtyValue(cty.ObjectVal(map[string]cty.Value{"user_name": cty.StringVal("ada")}), &u); err == nil {
		t.Error("cty decoded by a json tag; #148 is done")
	}

	got, err := gotenon.Decode[userRecord](tenon.Object(map[string]tenon.Value{"user_name": tenon.String("ada")}), tenon.Safe)
	if err != nil || got.Name != "ada" {
		t.Errorf("tenon decoded %+v, %v", got, err)
	}
}

// TestCtyIssue211_MismatchMessages: where a map does not conform to an object
// type, cty's message says only "object required". A tenon conversion that
// fails says which types, where and why.
func TestCtyIssue211_MismatchMessages(t *testing.T) {
	obj := cty.Object(map[string]cty.Type{"a": cty.String})
	if got := convert.MismatchMessage(cty.Map(cty.String), obj); got != "object required" {
		t.Errorf("cty's message is %q; #211 is fixed", got)
	}

	str, num := tenon.StringType(), tenon.NumberType()
	toObject := tenon.Convert(tenon.Map(str, map[string]tenon.Value{"a": tenon.String("x")}),
		tenon.Exactly(tenon.ObjectType(map[string]tenon.Type{"a": num})), tenon.Safe)
	if d := toObject.Diagnostics(); !failsWith(toObject, tenon.CodeConvertUnsafe) ||
		d[0].Message != `map(string) converts to object_with({"a": exactly(number)}, closed) only unsafely, and the policy is safe` {
		t.Errorf("tenon's map to an object failed with %s", toObject)
	}
	toMap := tenon.Convert(tenon.Object(map[string]tenon.Value{"a": tenon.String("x"), "b": tenon.NumberFromInt(1)}),
		tenon.Exactly(tenon.MapType(num)), tenon.Safe)
	if d := toMap.Diagnostics(); !failsWith(toMap, tenon.CodeConvertUnsafe) ||
		d[0].Message != "string converts to exactly(number) only unsafely, and the policy is safe" || d[0].Path.String() != ".a" {
		t.Errorf("tenon's object to a map failed with %s", toMap)
	}
}

// TestCtyIssue216_ConversionGivesTheTarget: converting a set holding an
// unknown to a list of another element type gave cty a list of the set's
// element type. tenon's conversion gives a value of the target, in every
// state.
func TestCtyIssue216_ConversionGivesTheTarget(t *testing.T) {
	in := cty.SetVal([]cty.Value{cty.NumberIntVal(5), cty.UnknownVal(cty.Number)})
	if got, err := convert.Convert(in, cty.List(cty.String)); err != nil || !got.Type().Equals(cty.List(cty.Number)) {
		t.Errorf("cty converted to %#v, %v; #216 is fixed", got.Type(), err)
	}

	num := tenon.NumberType()
	target := tenon.ListType(tenon.StringType())
	for _, v := range []tenon.Value{
		tenon.Set(num, tenon.NumberFromInt(5), tenon.Unknown(num)),
		tenon.Set(num, tenon.Unknown(num)),
		tenon.Unknown(tenon.SetType(num)),
		tenon.Null(tenon.SetType(num)),
	} {
		got := tenon.Convert(v, tenon.Exactly(target), tenon.Unsafe)
		if !got.IsResolved() || got.Type() != target {
			t.Errorf("tenon converted %s to %s", v, got)
		}
	}
}

// TestCtyIssue219_NaN: cty's Log and Pow panic on a NaN result, which the
// function package reports as a panic in its implementation. tenon has no
// NaN: a NaN from Go fails to encode, with a code of its own.
func TestCtyIssue219_NaN(t *testing.T) {
	_, err := stdlib.LogFunc.Call([]cty.Value{cty.NumberIntVal(-1), cty.NumberIntVal(10)})
	if err == nil || !strings.Contains(err.Error(), "panic in function implementation") {
		t.Errorf("cty's log(-1, 10) failed with %v; #219 is fixed", err)
	}

	if _, err := gotenon.Encode(math.NaN()); !hasCode(err, tenon.CodeEncodeNotANumber) {
		t.Errorf("tenon encoded NaN, failing with %v", err)
	}
	if v := tenon.NumberFromText("NaN"); !failsWith(v, tenon.CodeNumberInvalidSyntax) {
		t.Errorf("tenon made %s of NaN", v)
	}
}

// TestCtyIssue220_ExactLargeIntegers: cty writes the number of a
// low-precision big.Float as rounded decimal text, so JSON and msgpack give
// back another number. tenon takes the big.Float's exact value, and its
// encoding and JSON projection keep every digit.
func TestCtyIssue220_ExactLargeIntegers(t *testing.T) {
	f, _, err := big.ParseFloat("340282366920938463463374607431768211457", 10, 64, big.ToNearestEven)
	if err != nil {
		t.Fatal(err)
	}
	v := cty.NumberVal(f)
	j, err := ctyjson.Marshal(v, cty.Number)
	if err != nil {
		t.Fatal(err)
	}
	if back, err := ctyjson.Unmarshal(j, cty.Number); err != nil || back.RawEquals(v) {
		t.Errorf("cty's JSON gave back %#v, %v from %s; #220 is fixed", back, err, j)
	}
	packed, err := msgpack.Marshal(v, cty.Number)
	if err != nil {
		t.Fatal(err)
	}
	if back, err := msgpack.Unmarshal(packed, cty.Number); err != nil || back.RawEquals(v) {
		t.Errorf("cty's msgpack gave back %#v, %v; #220 is fixed", back, err)
	}

	exact := new(big.Int).Lsh(big.NewInt(1), 128) // what f holds, its 64 bits rounding the 1 off
	n, err := gotenon.Encode(f)
	if err != nil {
		t.Fatal(err)
	}
	if i, ok := n.AsBigInt(); !ok || i.Cmp(exact) != 0 {
		t.Errorf("tenon encoded %s, not %s", n, exact)
	}
	data, err := tenon.Serialize(n)
	if err != nil {
		t.Fatal(err)
	}
	if back, err := tenon.Deserialize(data, tenon.Decoders{}); err != nil || !tenon.Identical(back, n) {
		t.Errorf("tenon's encoding gave back %s, %v", back, err)
	}
	projected, err := tenon.ProjectJSON(n)
	if err != nil {
		t.Fatal(err)
	}
	if back := tenon.NumberFromText(string(projected)); !tenon.Identical(back, n) {
		t.Errorf("tenon projected %s, which reads back as %s", projected, back)
	}
}

// TestCtyIssue221_ContainsAnUntypedNull: cty's contains([], null) is unknown,
// though nothing could make it true. tenon's untyped null, a pending value
// known to be null, is in no set that holds no null, and may be in one that
// holds the null of a type it may turn out to have.
func TestCtyIssue221_ContainsAnUntypedNull(t *testing.T) {
	got, err := stdlib.ContainsFunc.Call([]cty.Value{cty.ListValEmpty(cty.Number), cty.NullVal(cty.DynamicPseudoType)})
	if err != nil || got.IsKnown() {
		t.Errorf("cty's contains([], null) is %#v, %v; #221 is fixed", got, err)
	}

	num := tenon.NumberType()
	untyped := tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly())
	for _, c := range []struct {
		set  tenon.Value
		want string
	}{
		{tenon.Set(num), "false"},
		{tenon.Set(num, tenon.NumberFromInt(1)), "false"},
		{tenon.Set(num, tenon.Null(num)), "unknown(bool, not null)"},
	} {
		if got := tenon.Contains(c.set, untyped); got.String() != c.want {
			t.Errorf("tenon's Contains(%s, untyped null) is %s, want %s", c.set, got, c.want)
		}
	}
}

// TestCtyIssue222_EqualsWithASharedUnknown: two objects sharing an unknown
// attribute and differing in a known one compare false or unknown in cty,
// whichever attribute Go's map order reaches first. tenon answers false, every
// time.
func TestCtyIssue222_EqualsWithASharedUnknown(t *testing.T) {
	u := cty.UnknownVal(cty.Number)
	a := cty.ObjectVal(map[string]cty.Value{"a": u, "b": cty.NumberIntVal(1)})
	b := cty.ObjectVal(map[string]cty.Value{"a": u, "b": cty.NumberIntVal(2)})
	if seen := answers(1000, func() string { return ctyAnswer(a.Equals(b)) }); len(seen) < 2 {
		t.Errorf("cty answered %v; #222 is fixed", seen)
	}

	num := tenon.NumberType()
	seen := answers(1000, func() string {
		a := tenon.Object(map[string]tenon.Value{"a": tenon.Unknown(num), "b": tenon.NumberFromInt(1)})
		b := tenon.Object(map[string]tenon.Value{"a": tenon.Unknown(num), "b": tenon.NumberFromInt(2)})
		return tenon.Equals(a, b).String()
	})
	if len(seen) != 1 || seen["false"] != 1000 {
		t.Errorf("tenon answered %v", seen)
	}
}

// TestCtyIssue226_EqualsWithAnUnknownAndADifference: an object or a map
// holding an unknown on one side and a definite difference elsewhere compares
// false or unknown in cty, by map order. tenon answers false, every time.
func TestCtyIssue226_EqualsWithAnUnknownAndADifference(t *testing.T) {
	left := map[string]cty.Value{"a": cty.UnknownVal(cty.String), "b": cty.StringVal("z")}
	right := map[string]cty.Value{"a": cty.StringVal("x"), "b": cty.StringVal("y")}
	for name, pair := range map[string][2]cty.Value{
		"object": {cty.ObjectVal(left), cty.ObjectVal(right)},
		"map":    {cty.MapVal(left), cty.MapVal(right)},
	} {
		if seen := answers(1000, func() string { return ctyAnswer(pair[0].Equals(pair[1])) }); len(seen) < 2 {
			t.Errorf("cty's %s answered %v; #226 is fixed", name, seen)
		}
	}

	str := tenon.StringType()
	tLeft := func() map[string]tenon.Value {
		return map[string]tenon.Value{"a": tenon.Unknown(str), "b": tenon.String("z")}
	}
	tRight := func() map[string]tenon.Value {
		return map[string]tenon.Value{"a": tenon.String("x"), "b": tenon.String("y")}
	}
	for name, build := range map[string]func(map[string]tenon.Value) tenon.Value{
		"object": tenon.Object,
		"map":    func(m map[string]tenon.Value) tenon.Value { return tenon.Map(str, m) },
	} {
		seen := answers(1000, func() string { return tenon.Equals(build(tLeft()), build(tRight())).String() })
		if len(seen) != 1 || seen["false"] != 1000 {
			t.Errorf("tenon's %s answered %v", name, seen)
		}
	}
}

// TestCtyIssue223_NumberText: cty parses a binary exponent, an infinity and a
// leading plus sign as numbers. tenon reads decimal text alone, and a string
// converts to a number only where it is that.
func TestCtyIssue223_NumberText(t *testing.T) {
	for text, want := range map[string]string{"1p4": "16", "inf": "+Inf", "+5": "5"} {
		v, err := cty.ParseNumberVal(text)
		if err != nil || v.AsBigFloat().Text('g', -1) != want {
			t.Errorf("cty read %q as %#v, %v; #223 is fixed", text, v, err)
		}
	}

	for _, text := range []string{"1p4", "inf", "Inf", "+Inf", "+5", "0x10", "Infinity", "NaN", "1_000", " 1", "1."} {
		if v := tenon.NumberFromText(text); !failsWith(v, tenon.CodeNumberInvalidSyntax) {
			t.Errorf("tenon read %q as %s", text, v)
		}
		if v := tenon.Convert(tenon.String(text), tenon.Exactly(tenon.NumberType()), tenon.Unsafe); !v.IsError() {
			t.Errorf("tenon converted %q to %s", text, v)
		}
	}
}

// TestCtyIssue224_EveryOperandChecked: a cty function given DynamicVal first
// returns DynamicVal without checking the arguments after it, so a null that
// is an error already goes unreported. A tenon operation checks every
// operand, wherever a pending one stands.
func TestCtyIssue224_EveryOperandChecked(t *testing.T) {
	f := function.New(&function.Spec{
		Params: []function.Parameter{{Name: "a", Type: cty.String}, {Name: "b", Type: cty.String}},
		Type:   function.StaticReturnType(cty.String),
		Impl:   func(args []cty.Value, _ cty.Type) (cty.Value, error) { return args[0], nil },
	})
	if _, err := f.Call([]cty.Value{cty.DynamicVal, cty.NullVal(cty.String)}); err != nil {
		t.Errorf("cty checked the null after DynamicVal: %v; #224 is fixed", err)
	}
	if _, err := f.Call([]cty.Value{cty.NullVal(cty.String), cty.DynamicVal}); err == nil {
		t.Error("cty did not check the null before DynamicVal")
	}

	pending, null := tenon.Pending(tenon.Any()), tenon.Null(tenon.NumberType())
	for _, args := range [][2]tenon.Value{{pending, null}, {null, pending}} {
		if v := tenon.Add(args[0], args[1]); !failsWith(v, tenon.CodeOperationNullOperand) {
			t.Errorf("tenon's Add(%s, %s) is %s", args[0], args[1], v)
		}
	}
	for _, args := range [][2]tenon.Value{{pending, tenon.String("x")}, {tenon.String("x"), pending}} {
		r := ctyPanics(func() { tenon.Add(args[0], args[1]) })
		if msg, _ := r.(string); !strings.Contains(msg, "of type string, which does not satisfy exactly(number)") {
			t.Errorf("tenon's Add(%s, %s) panicked with %v", args[0], args[1], r)
		}
	}
}

// TestCtyIssue225_NamesThatNormalizeAlike: cty folds two keys that are one
// after NFC into one, keeping whichever Go's map order reaches last. tenon
// refuses them, the same way every time: a panic where the program wrote the
// names, an error value or error where they came from data, naming both
// spellings.
func TestCtyIssue225_NamesThatNormalizeAlike(t *testing.T) {
	decomposed, composed := "e\U00000301", "\U000000e9"
	if seen := answers(200, func() string {
		return cty.Object(map[string]cty.Type{decomposed: cty.String, composed: cty.Number}).GoString()
	}); len(seen) < 2 {
		t.Errorf("cty kept %v; #225 is fixed", seen)
	}

	str := tenon.StringType()
	spellings := strconv.QuoteToASCII(decomposed) + " and " + strconv.QuoteToASCII(composed)
	for range 100 {
		r := ctyPanics(func() { tenon.ObjectType(map[string]tenon.Type{decomposed: str, composed: tenon.NumberType()}) })
		if want := "tenon: usage: object attribute names " + spellings + " are the same name after normalization"; r != want {
			t.Fatalf("tenon's ObjectType panicked with %v, want %s", r, want)
		}
		object := tenon.Object(map[string]tenon.Value{decomposed: tenon.String("x"), composed: tenon.NumberFromInt(1)})
		if !failsWith(object, tenon.CodeObjectDuplicateName) || !strings.Contains(object.Diagnostics()[0].Message, spellings) {
			t.Fatalf("tenon's Object gave %s", object)
		}
		m := tenon.Map(str, map[string]tenon.Value{decomposed: tenon.String("x"), composed: tenon.String("y")})
		if !failsWith(m, tenon.CodeMapDuplicateKey) || !strings.Contains(m.Diagnostics()[0].Message, spellings) {
			t.Fatalf("tenon's Map gave %s", m)
		}
		if err := tenon.CheckAttributeNames(decomposed, composed); !strings.Contains(fmt.Sprint(err), spellings) {
			t.Fatalf("tenon's CheckAttributeNames gave %v", err)
		}
	}
}

// TestCtyIssue227_CountsThatOverflow: cty's setproduct multiplies its
// arguments' lengths unchecked, and 64 lists of two give an empty set. tenon
// counts the values a type holds, a product over a tuple's elements, for the
// greatest length of a set of it: exactly where an int64 holds the count, and
// as no bound where it does not.
func TestCtyIssue227_CountsThatOverflow(t *testing.T) {
	pair := cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")})
	pairs := func(n int) []cty.Value {
		args := make([]cty.Value, n)
		for i := range args {
			args[i] = pair
		}
		return args
	}
	if got, err := stdlib.SetProductFunc.Call(pairs(64)); err != nil || got.LengthInt() != 0 {
		t.Errorf("cty's product of 64 pairs is %d long, %v; #227 is fixed", got.LengthInt(), err)
	}
	if _, err := stdlib.SetProductFunc.Call(pairs(63)); err == nil || !strings.Contains(err.Error(), "panic in function implementation") {
		t.Errorf("cty's product of 63 pairs failed with %v; #227 is fixed", err)
	}

	limit := new(big.Int).SetInt64(math.MaxInt64)
	for n := 1; n <= 64; n++ {
		elems := make([]tenon.Type, n)
		for i := range elems {
			elems[i] = tenon.BoolType()
		}
		set := tenon.Unknown(tenon.SetType(tenon.TupleType(elems...)))
		// A member is null or a tuple of n bools, each null, true or false.
		count := new(big.Int).Exp(big.NewInt(3), big.NewInt(int64(n)), nil)
		count.Add(count, big.NewInt(1))
		got, ok := set.Range().LengthMax()
		switch {
		case count.Cmp(limit) <= 0 && (!ok || got != count.Int64()):
			t.Errorf("tenon bounds a set of %d-tuples at %d, %t, want %s", n, got, ok, count)
		case count.Cmp(limit) > 0 && ok:
			t.Errorf("tenon bounds a set of %d-tuples at %d, where %s values do not fit", n, got, count)
		}
	}
}

// TestCtyIssue228_EmptyNumberRanges: cty accepts an unknown number above 3
// and below 3, and panics on one above 3 and at most 3, both of which nothing
// satisfies. tenon answers the two alike: the bounds hold of a number, so
// what is left is null, and a value narrowed not to be null as well is a
// contradiction.
func TestCtyIssue228_EmptyNumberRanges(t *testing.T) {
	refine := func(maxInclusive bool) (v cty.Value, r any) {
		r = ctyPanics(func() {
			v = cty.UnknownVal(cty.Number).Refine().
				NumberRangeLowerBound(cty.NumberIntVal(3), false).
				NumberRangeUpperBound(cty.NumberIntVal(3), maxInclusive).
				NewValue()
		})
		return v, r
	}
	if v, r := refine(false); r != nil || v.IsKnown() {
		t.Errorf("cty refined 3 < x < 3 to %#v, panicking with %v; #228 is fixed", v, r)
	}
	if _, r := refine(true); r == nil {
		t.Error("cty refined 3 < x <= 3 without a panic; #228 is fixed")
	}

	num, three := tenon.NumberType(), tenon.NumberFromInt(3)
	for _, inclusive := range []bool{false, true} {
		bounds := []tenon.Narrowing{tenon.NumberMin(three, false), tenon.NumberMax(three, inclusive)}
		if v := tenon.Narrow(tenon.Unknown(num), bounds...); !tenon.Identical(v, tenon.Null(num)) {
			t.Errorf("tenon narrowed 3 < x (<= %t) 3 to %s", inclusive, v)
		}
		if v := tenon.Narrow(tenon.Unknown(num), append(bounds, tenon.NotNull())...); !failsWith(v, tenon.CodeRangeContradiction) {
			t.Errorf("tenon narrowed 3 < x (<= %t) 3, not null, to %s", inclusive, v)
		}
	}
}

// TestCtyIssue229_ASetEqualsItself: cty answers false where a set holds a
// member with an unknown part, compared even with itself, which can be no
// other value. tenon answers unknown: the member may turn out to be any list
// of one bool, and two such sets are equal or not as their members turn out.
func TestCtyIssue229_ASetEqualsItself(t *testing.T) {
	s := cty.SetVal([]cty.Value{cty.ListVal([]cty.Value{cty.UnknownVal(cty.Bool)})})
	if got := s.Equals(s); !got.RawEquals(cty.False) {
		t.Errorf("cty answers %#v; #229 is fixed", got)
	}
	ts := tenon.Set(tenon.ListType(tenon.BoolType()), tenon.List(tenon.BoolType(), tenon.Unknown(tenon.BoolType())))
	if got := tenon.Equals(ts, ts); got.IsKnown() {
		t.Errorf("tenon answers %v, want unknown", got)
	}
}

// TestCtyIssue230_AShortTuple: cty's JSON reader panics on an array shorter
// than the tuple it reads it as, at the top level. tenon's fails with the
// failure to convert, located where the tuple is short, its message naming
// both lengths.
func TestCtyIssue230_AShortTuple(t *testing.T) {
	ty := cty.Tuple([]cty.Type{cty.Number, cty.Number})
	func() {
		defer func() {
			if recover() == nil {
				t.Errorf("cty read a short tuple without panicking; #230 is fixed")
			}
		}()
		_, _ = ctyjson.Unmarshal([]byte(`[1]`), ty)
	}()
	pair := tenon.Exactly(tenon.TupleType(tenon.NumberType(), tenon.NumberType()))
	for text, at := range map[string]tenon.Path{
		`[1]`:        {},
		`{"a": [1]}`: tenon.Path{}.Attribute("a"),
	} {
		c := pair
		if text != `[1]` {
			c = tenon.ObjectWith(map[string]tenon.Field{"a": tenon.Required(pair)}, true)
		}
		_, err := tenon.ParseJSON([]byte(text), c, tenon.Safe)
		var failed *tenon.Error
		if !errors.As(err, &failed) {
			t.Errorf("tenon read %s with %v", text, err)
			continue
		}
		if d := failed.Diagnostics(); len(d) != 1 || d[0].Code != tenon.CodeConvertNoConversion || !d[0].Path.Equal(at) {
			t.Errorf("tenon read %s failing with %+v, want %s at %s", text, d, tenon.CodeConvertNoConversion, at)
		}
	}
}
