package gotenon_test

import (
	"errors"
	"math"
	"math/big"
	"math/rand"
	"reflect"
	"testing"

	"tenon"
	"tenon/conformance"
	"tenon/gotenon"
)

// decoded decodes v into a T under p, failing t if decoding fails.
func decoded[T any](t *testing.T, v tenon.Value, p tenon.Policy) T {
	t.Helper()
	x, err := gotenon.Decode[T](v, p)
	if err != nil {
		t.Fatalf("Decode[%T](%v) failed: %v", x, v, err)
	}
	return x
}

// wantDecodeFailures fails t unless decoding v into a T fails with exactly
// these diagnostics.
func wantDecodeFailures[T any](t *testing.T, what string, v tenon.Value, p tenon.Policy, want ...wantDiag) {
	t.Helper()
	x, err := gotenon.Decode[T](v, p)
	var de *gotenon.DiagnosticError
	if !errors.As(err, &de) {
		t.Errorf("%s: Decode gave %v, %v, want a *DiagnosticError", what, x, err)
		return
	}
	wantErrors(t, what, de.Value, want...)
}

func TestConformance_GO002_DecodingIsConversion(t *testing.T) {
	conformance.Covers(t, "GO-002", "GO-014")
	// The policy is the conversion's.
	if got := decoded[int](t, s("42"), uns); got != 42 {
		t.Errorf("decoding \"42\" unsafely gave %d", got)
	}
	wantDecodeFailures[int](t, "a string, safely", s("42"), safe, wantDiag{tenon.CodeConvertUnsafe, ""})
	wantDecodeFailures[int](t, "not a number", s("x"), uns, wantDiag{tenon.CodeNumberInvalidSyntax, ""})
	if got := decoded[[]string](t, tenon.TupleVal(s("a"), s("b")), safe); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("a tuple decoded into %v", got)
	}
	// Conversion's diagnostics are located as they were.
	wantDecodeFailures[person](t, "a misspelled attribute", obj(map[string]tenon.Value{
		"name": s("x"), "age": n(1), "home": obj(map[string]tenon.Value{"stret": s("Main")}),
		"scores": tenon.MapVal(num, nil), "Nickname": s("x"),
	}), safe,
		wantDiag{tenon.CodeConvertMissingAttribute, ".home"},
		wantDiag{tenon.CodeConvertUnexpectedAttribute, ".home.stret"})

	// An object decodes into a Go map under either policy, and a map into a
	// struct under the unsafe policy only.
	for _, p := range []tenon.Policy{safe, uns} {
		if got := decoded[map[string]int](t, obj(map[string]tenon.Value{"a": n(1)}), p); got["a"] != 1 {
			t.Errorf("an object decoded into %v under %s", got, p)
		}
	}
	m := tenon.MapVal(str, map[string]tenon.Value{"street": s("Main")})
	if got := decoded[address](t, m, uns); got.Street != "Main" || got.Unit != nil {
		t.Errorf("a map decoded into %+v", got)
	}
	wantDecodeFailures[address](t, "a map into a struct, safely", m, safe, wantDiag{tenon.CodeConvertUnsafe, ""})
}

func TestConformance_GO032_NumbersDecode(t *testing.T) {
	conformance.Covers(t, "GO-032", "GO-050")
	if got := decoded[int8](t, n(-128), safe); got != -128 {
		t.Errorf("int8 %d", got)
	}
	if got := decoded[uint64](t, tenon.NumberFromText("18446744073709551615"), safe); got != math.MaxUint64 {
		t.Errorf("uint64 %d", got)
	}
	for _, tt := range []struct {
		name string
		v    tenon.Value
		fn   func(tenon.Value) error
	}{
		{"128 into an int8", n(128), func(v tenon.Value) error { _, err := gotenon.Decode[int8](v, safe); return err }},
		{"-1 into a uint", n(-1), func(v tenon.Value) error { _, err := gotenon.Decode[uint](v, safe); return err }},
		{"1.5 into an int", tenon.NumberFromText("1.5"), func(v tenon.Value) error { _, err := gotenon.Decode[int](v, safe); return err }},
		{"1e30 into an int64", tenon.NumberFromText("1e30"), func(v tenon.Value) error { _, err := gotenon.Decode[int64](v, safe); return err }},
		{"1e400 into a float64", tenon.NumberFromText("1e400"), func(v tenon.Value) error { _, err := gotenon.Decode[float64](v, safe); return err }},
		{"1e39 into a float32", tenon.NumberFromText("1e39"), func(v tenon.Value) error { _, err := gotenon.Decode[float32](v, safe); return err }},
		{"0.5 into a big.Int", tenon.NumberFromText("0.5"), func(v tenon.Value) error { _, err := gotenon.Decode[big.Int](v, safe); return err }},
	} {
		var de *gotenon.DiagnosticError
		if err := tt.fn(tt.v); !errors.As(err, &de) || de.Diagnostics()[0].Code != tenon.CodeDecodeOutOfRange {
			t.Errorf("%s: %v, want %s", tt.name, err, tenon.CodeDecodeOutOfRange)
		}
	}
	// A float is the nearest, ties to even, and a tiny number rounds to zero.
	if got := decoded[float64](t, tenon.NumberFromText("0.1"), safe); got != 0.1 {
		t.Errorf("0.1 decoded as %v", got)
	}
	if got := decoded[float64](t, tenon.NumberFromText("9007199254740993"), safe); got != 9007199254740992 {
		t.Errorf("2^53+1 decoded as %v, want the even neighbour", got)
	}
	if got := decoded[float64](t, tenon.NumberFromText("1e-400"), safe); got != 0 {
		t.Errorf("1e-400 decoded as %v", got)
	}
	// Big numbers are exact, and a big.Float holds a binary fraction exactly.
	huge := tenon.NumberFromText("1e100")
	if got := decoded[big.Int](t, huge, safe); got.Cmp(new(big.Int).Exp(big.NewInt(10), big.NewInt(100), nil)) != 0 {
		t.Errorf("1e100 decoded as %v", &got)
	}
	if got := decoded[big.Rat](t, tenon.NumberFromText("-0.125"), safe); got.Cmp(big.NewRat(-1, 8)) != 0 {
		t.Errorf("-0.125 decoded as %v", &got)
	}
	third := encoded(t, math.Nextafter(1, 2))
	if got := decoded[big.Float](t, third, safe); got.Cmp(big.NewFloat(math.Nextafter(1, 2))) != 0 {
		t.Errorf("the float after one decoded as %v", &got)
	}
}

func TestConformance_GO041_TheBoundaryRefusesWhatGoCannotHold(t *testing.T) {
	conformance.Covers(t, "GO-041", "GO-042", "GO-003")
	prop := stamp{id: "prop"}
	iso := stamp{id: "iso", policy: tenon.Isolate}
	input := obj(map[string]tenon.Value{
		"name": tenon.WithMarks(s("x"), prop), "age": tenon.Unknown(num), "tags": tenon.ListVal(str, tenon.WithMarks(s("t"), iso)),
		"home": obj(map[string]tenon.Value{"street": tenon.NullVal(str)}), "scores": tenon.NullVal(tenon.Map(num)),
		"Nickname": tenon.Narrow(tenon.Unknown(str), tenon.NotNull()),
	})
	wantDecodeFailures[person](t, "every part that fails, where it is", input, safe,
		wantDiag{tenon.CodeDecodeNotKnown, ".Nickname"},
		wantDiag{tenon.CodeDecodeNotKnown, ".age"},
		wantDiag{tenon.CodeDecodeNull, ".home.street"},
		wantDiag{tenon.CodeDecodeMarked, ".name"},
		wantDiag{tenon.CodeDecodeMarked, ".tags[0]"})

	wantDecodeFailures[int](t, "a pending value", tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NotNull()), safe,
		wantDiag{tenon.CodeDecodeNotKnown, ""})

	// A tenon.Value takes what it is given, error values included.
	failed := tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: "it failed"})
	if got := decoded[tenon.Value](t, failed, safe); !tenon.Identical(got, failed) {
		t.Errorf("an error value decoded into a tenon.Value as %v", got)
	}
	marked := tenon.WithMarks(tenon.Unknown(num), iso)
	if got := decoded[holder](t, obj(map[string]tenon.Value{"name": s("x"), "extra": marked}), safe); !tenon.Identical(got.Extra, marked) {
		t.Errorf("a tenon.Value field took %v", got.Extra)
	}
	// Anywhere else an error value gives its diagnostics.
	wantDecodeFailures[int](t, "an error value", failed, safe, wantDiag{"app.failed", ""})

	// Null is nil where Go has nil, and the zero value where optional.
	type nullable struct {
		P    *int           `tenon:"p"`
		S    []int          `tenon:"s"`
		M    map[string]int `tenon:"m"`
		O    int            `tenon:"o,optional"`
		Gone string         `tenon:"gone,optional"`
	}
	nullNum := tenon.NullVal(num)
	got := decoded[nullable](t, obj(map[string]tenon.Value{
		"p": nullNum, "s": tenon.NullVal(tenon.List(num)), "m": tenon.NullVal(tenon.Map(num)), "o": nullNum,
	}), safe)
	if got.P != nil || got.S != nil || got.M != nil || got.O != 0 || got.Gone != "" {
		t.Errorf("nulls decoded as %+v", got)
	}
	wantDecodeFailures[int](t, "a null int", nullNum, safe, wantDiag{tenon.CodeDecodeNull, ""})
	// A pending value known to be null is a null as well.
	pendingNull := tenon.Narrow(tenon.Pending(tenon.Any()), tenon.Null())
	if got := decoded[map[string]int](t, pendingNull, safe); got != nil {
		t.Errorf("a pending null decoded into %v", got)
	}
	wantDecodeFailures[int](t, "a pending null int", pendingNull, safe, wantDiag{tenon.CodeDecodeNull, ""})
}

func TestMarksAreRefusedWhereTheConversionPutsThem(t *testing.T) {
	conformance.Covers(t, "GO-041")
	iso := stamp{id: "iso", policy: tenon.Isolate}
	prop := stamp{id: "prop"}
	// A map converts to a struct under Unsafe, so a marked element is refused
	// as the attribute it becomes, and an object converts to a Go map, so a
	// marked attribute is refused as the element it becomes.
	type pair struct {
		A string `tenon:"a"`
		B string `tenon:"b"`
	}
	m := tenon.MapVal(str, map[string]tenon.Value{"a": tenon.WithMarks(s("x"), iso), "b": s("y")})
	wantDecodeFailures[pair](t, "a map with a marked element", m, uns, wantDiag{tenon.CodeDecodeMarked, ".a"})
	o := obj(map[string]tenon.Value{"a": tenon.WithMarks(s("x"), iso), "b b": tenon.WithMarks(s("y"), prop)})
	wantDecodeFailures[map[string]string](t, "an object with marked attributes", o, safe,
		wantDiag{tenon.CodeDecodeMarked, `["a"]`}, wantDiag{tenon.CodeDecodeMarked, `["b b"]`})

	// A member decoded by its own conversion is refused once, in member order
	// among the other failures.
	members := tenon.TupleVal(
		obj(map[string]tenon.Value{"name": tenon.WithMarks(s("a"), iso)}),
		obj(map[string]tenon.Value{"name": tenon.Unknown(str)}),
		tenon.WithMarks(obj(map[string]tenon.Value{"name": s("c")}), prop))
	wantDecodeFailures[[]holder](t, "a slice of holders", members, safe,
		wantDiag{tenon.CodeDecodeMarked, "[0].name"},
		wantDiag{tenon.CodeDecodeNotKnown, "[1].name"},
		wantDiag{tenon.CodeDecodeMarked, "[2]"})

	// What the conversion refuses is refused whole, marks and all, and so is
	// a list too long for its array.
	wantDecodeFailures[[]pair](t, "a list of the wrong type", tenon.ListVal(str, tenon.WithMarks(s("x"), iso)), safe,
		wantDiag{tenon.CodeConvertNoConversion, "[0]"})
	wantDecodeFailures[[2]string](t, "a long list", tenon.ListVal(str, s("a"), tenon.WithMarks(s("b"), iso), s("c")), safe,
		wantDiag{tenon.CodeDecodeLengthMismatch, ""})
}

func TestDecodingValuesOfManyTypes(t *testing.T) {
	conformance.Covers(t, "GO-012", "GO-013", "GO-022")
	// A slice of values takes a list, a set or a tuple, members as they are.
	members := []tenon.Value{n(1), s("x"), tenon.Unknown(boo)}
	if got := decoded[[]tenon.Value](t, tenon.TupleVal(members...), safe); len(got) != 3 || !tenon.Identical(got[2], members[2]) {
		t.Errorf("a tuple decoded into %v", got)
	}
	wantDecodeFailures[[]tenon.Value](t, "a map into a slice of values", tenon.MapVal(num, nil), safe,
		wantDiag{tenon.CodeConvertNoConversion, ""})
	// A slice of structs holding values converts each member on its own.
	holders := decoded[[]holder](t, tenon.TupleVal(
		obj(map[string]tenon.Value{"name": s("a")}),
		obj(map[string]tenon.Value{"name": s("b"), "extra": n(2)})), safe)
	if len(holders) != 2 || holders[0].Extra != (tenon.Value{}) || !tenon.Identical(holders[1].Extra, n(2)) {
		t.Errorf("holders decoded as %+v", holders)
	}
	wantDecodeFailures[[3]int](t, "a short array", tenon.ListVal(num, n(1)), safe, wantDiag{tenon.CodeDecodeLengthMismatch, ""})
	if got := decoded[map[string]tenon.Value](t, obj(map[string]tenon.Value{"a": n(1)}), safe); !tenon.Identical(got["a"], n(1)) {
		t.Errorf("an object decoded into %v", got)
	}
}

// genValue fills v, of a type holding what round trips, with random content.
func genValue(r *rand.Rand, v reflect.Value, depth int) {
	switch v.Type() {
	case reflect.TypeFor[tenon.Value]():
		pool := []tenon.Value{n(1), s("x"), tenon.Unknown(num), tenon.WithMarks(tenon.Bool(true), stamp{id: "m"}), tenon.NullVal(str)}
		v.Set(reflect.ValueOf(pool[r.Intn(len(pool))]))
		return
	case reflect.TypeFor[big.Int]():
		v.Set(reflect.ValueOf(*new(big.Int).Exp(big.NewInt(int64(r.Intn(40)-20)), big.NewInt(int64(r.Intn(30))), nil)))
		return
	case reflect.TypeFor[big.Rat]():
		v.Set(reflect.ValueOf(*big.NewRat(int64(r.Intn(1000)-500), int64(1)<<uint(r.Intn(20)))))
		return
	}
	switch v.Kind() {
	case reflect.Bool:
		v.SetBool(r.Intn(2) == 0)
	case reflect.Int, reflect.Int64:
		v.SetInt(r.Int63() - r.Int63())
	case reflect.Uint16:
		v.SetUint(uint64(r.Intn(65536)))
	case reflect.Float64:
		f := r.NormFloat64() * math.Pow(10, float64(r.Intn(40)-20))
		if f == 0 {
			f = 1
		}
		v.SetFloat(f)
	case reflect.String:
		v.SetString([]string{"", "a", "caf\U000000e9", "\U0001F600", "x y"}[r.Intn(5)])
	case reflect.Slice:
		if r.Intn(4) == 0 {
			return // nil
		}
		s := reflect.MakeSlice(v.Type(), r.Intn(3), r.Intn(3)+3)
		for i := range s.Len() {
			genValue(r, s.Index(i), depth-1)
		}
		v.Set(s)
	case reflect.Array:
		for i := range v.Len() {
			genValue(r, v.Index(i), depth-1)
		}
	case reflect.Map:
		if r.Intn(4) == 0 {
			return
		}
		m := reflect.MakeMap(v.Type())
		keys := []string{"", "k", "l"}
		if v.Type().Elem() == reflect.TypeFor[tenon.Value]() {
			keys = keys[1:] // such a map encodes as an object, which has no empty name
		}
		for _, k := range keys {
			if r.Intn(2) == 0 {
				e := reflect.New(v.Type().Elem()).Elem()
				genValue(r, e, depth-1)
				m.SetMapIndex(reflect.ValueOf(k), e)
			}
		}
		v.Set(m)
	case reflect.Pointer:
		if r.Intn(3) == 0 {
			return
		}
		p := reflect.New(v.Type().Elem())
		genValue(r, p.Elem(), depth-1)
		v.Set(p)
	case reflect.Struct:
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() {
				genValue(r, v.Field(i), depth-1)
			}
		}
	}
}

// roundTripped is a struct holding every kind of field that round trips.
type roundTripped struct {
	B      bool                   `tenon:"b"`
	I      int                    `tenon:"i"`
	U      uint16                 `tenon:"u"`
	F      float64                `tenon:"f"`
	S      string                 `tenon:"s"`
	List   []string               `tenon:"list"`
	Arr    [2]bool                `tenon:"arr"`
	Map    map[string]int64       `tenon:"map,optional"`
	Ptr    *int                   `tenon:"ptr,optional"`
	Nested *roundTrippedLeaf      `tenon:"nested"`
	Leaves []roundTrippedLeaf     `tenon:"leaves"`
	Values []tenon.Value          `tenon:"values"`
	ByName map[string]tenon.Value `tenon:"by_name"`
	Big    big.Int                `tenon:"big"`
	Rat    *big.Rat               `tenon:"rat"`
}

type roundTrippedLeaf struct {
	Name  string      `tenon:"name"`
	Extra tenon.Value `tenon:"extra,optional"`
}

// sameGo reports whether two Go values are equal as GO-004 compares them:
// numbers by value, tenon values by Identical, pointers by what they hold.
func sameGo(a, b reflect.Value) bool {
	switch a.Type() {
	case reflect.TypeFor[tenon.Value]():
		x, y := a.Interface().(tenon.Value), b.Interface().(tenon.Value)
		if x == (tenon.Value{}) || y == (tenon.Value{}) {
			return x == y
		}
		return tenon.Identical(x, y)
	case reflect.TypeFor[big.Int]():
		x, y := a.Interface().(big.Int), b.Interface().(big.Int)
		return x.Cmp(&y) == 0
	case reflect.TypeFor[big.Rat]():
		x, y := a.Interface().(big.Rat), b.Interface().(big.Rat)
		return x.Cmp(&y) == 0
	}
	switch a.Kind() {
	case reflect.Slice, reflect.Map, reflect.Pointer:
		if a.IsNil() || b.IsNil() {
			return a.IsNil() == b.IsNil()
		}
	}
	switch a.Kind() {
	case reflect.Slice, reflect.Array:
		if a.Len() != b.Len() {
			return false
		}
		for i := range a.Len() {
			if !sameGo(a.Index(i), b.Index(i)) {
				return false
			}
		}
		return true
	case reflect.Map:
		if a.Len() != b.Len() {
			return false
		}
		for _, k := range a.MapKeys() {
			if bv := b.MapIndex(k); !bv.IsValid() || !sameGo(a.MapIndex(k), bv) {
				return false
			}
		}
		return true
	case reflect.Pointer:
		return sameGo(a.Elem(), b.Elem())
	case reflect.Struct:
		for i := range a.NumField() {
			if a.Type().Field(i).IsExported() && !sameGo(a.Field(i), b.Field(i)) {
				return false
			}
		}
		return true
	}
	return a.Interface() == b.Interface()
}

func TestConformance_GO004_RoundTrip(t *testing.T) {
	conformance.Covers(t, "GO-004")
	r := rand.New(rand.NewSource(11))
	for i := 0; i < 3000; i++ {
		var x roundTripped
		genValue(r, reflect.ValueOf(&x).Elem(), 4)
		v, err := gotenon.Encode(x)
		if err != nil {
			t.Fatalf("Encode(%+v) failed: %v", x, err)
		}
		for _, p := range []tenon.Policy{safe, uns} {
			got, err := gotenon.Decode[roundTripped](v, p)
			if err != nil {
				t.Fatalf("Decode(Encode(%+v)) failed: %v\n%v", x, err, v)
			}
			if !sameGo(reflect.ValueOf(x), reflect.ValueOf(got)) {
				t.Fatalf("Decode(Encode(%+v)) = %+v\n%v", x, got, v)
			}
		}
	}
}
