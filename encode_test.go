package tenon_test

import (
	"errors"
	"math"
	"math/big"
	"testing"

	"tenon"
	"tenon/conformance"
)

// encoded encodes x, failing t if encoding fails.
func encoded[T any](t *testing.T, x T) tenon.Value {
	t.Helper()
	v, err := tenon.Encode(x)
	if err != nil {
		t.Fatalf("Encode(%v) failed: %v", x, err)
	}
	return v
}

// wantEncodeFailure fails t unless encoding x fails with exactly these
// diagnostics.
func wantEncodeFailure[T any](t *testing.T, what string, x T, want ...wantDiag) {
	t.Helper()
	_, err := tenon.Encode(x)
	var de *tenon.DiagnosticError
	if !errors.As(err, &de) {
		t.Errorf("%s: Encode gave %v, want a *DiagnosticError", what, err)
		return
	}
	wantErrors(t, what, de.Value, want...)
}

type address struct {
	Street string `tenon:"street"`
	Unit   *int   `tenon:"unit,optional"`
}

type person struct {
	Name     string            `tenon:"name"`
	Age      int               `tenon:"age"`
	Tags     []string          `tenon:"tags,optional"`
	Home     address           `tenon:"home"`
	Scores   map[string]uint16 `tenon:"scores"`
	Nickname string            // named by the field's own name
	secret   string
	Skipped  bool `tenon:"-"`
}

func TestConformance_GO010_TheMapping(t *testing.T) {
	conformance.Covers(t, "GO-010", "GO-001", "GO-020")
	unit := 4
	p := person{
		Name: "Ada", Age: 36, Tags: []string{"a", "b"},
		Home:     address{Street: "Main", Unit: &unit},
		Scores:   map[string]uint16{"x": 1},
		Nickname: "ada", secret: "hidden", Skipped: true,
	}
	want := obj(map[string]tenon.Value{
		"name": s("Ada"), "age": n(36), "tags": tenon.ListVal(str, s("a"), s("b")),
		"home":     obj(map[string]tenon.Value{"street": s("Main"), "unit": n(4)}),
		"scores":   tenon.MapVal(num, map[string]tenon.Value{"x": n(1)}),
		"Nickname": s("ada"),
	})
	wantValue(t, "a person", encoded(t, p), want)

	for _, tt := range []struct {
		name string
		got  tenon.Value
		want tenon.Value
	}{
		{"bool", encoded(t, true), tenon.Bool(true)},
		{"int8", encoded(t, int8(-8)), n(-8)},
		{"uint64", encoded(t, uint64(math.MaxUint64)), tenon.NumberFromText("18446744073709551615")},
		{"an array", encoded(t, [2]bool{true, false}), tenon.ListVal(boo, tenon.Bool(true), tenon.Bool(false))},
		{"a named type", encoded(t, celsiusDegrees(21)), n(21)},
		{"a map with named keys", encoded(t, map[key]int{"k": 1}), tenon.MapVal(num, map[string]tenon.Value{"k": n(1)})},
		{"a tenon.Value", encoded(t, tenon.Unknown(str)), tenon.Unknown(str)},
		{"a pointer", encoded(t, &unit), n(4)},
	} {
		wantValue(t, tt.name, tt.got, tt.want)
	}
}

type celsiusDegrees float64

type key string

type selfish struct {
	Next *selfish `tenon:"next"`
}

type embeds struct {
	Address `tenon:"addr"`
	address
	Name string `tenon:"name"`
}

type embedsUntagged struct {
	Address
}

type Address struct {
	Street string
}

func TestConformance_GO011_UnsupportedTypes(t *testing.T) {
	conformance.Covers(t, "GO-011", "GO-021")
	mustPanicUsage(t, "of kind interface", func() { tenon.Encode[any](1) })
	mustPanicUsage(t, "of kind chan", func() { tenon.Encode(make(chan int)) })
	mustPanicUsage(t, "of kind func", func() { tenon.Encode(func() {}) })
	mustPanicUsage(t, "of kind complex128", func() { tenon.Encode(complex(1, 2)) })
	mustPanicUsage(t, "pointer to tenon.Value", func() { tenon.Encode(&tenon.Value{}) })
	mustPanicUsage(t, "keys of kind int", func() { tenon.Encode(map[int]string{}) })
	mustPanicUsage(t, "holds itself", func() { tenon.Encode(selfish{}) })

	// Tags are checked when the type is first met.
	mustPanicUsage(t, `whose option "omitempty" is not optional`, func() {
		tenon.Encode(struct {
			A int `tenon:"a,omitempty"`
		}{})
	})
	mustPanicUsage(t, "skips it and so takes no options", func() {
		tenon.Encode(struct {
			A int `tenon:"-,optional"`
		}{})
	})
	mustPanicUsage(t, "both map to the attribute", func() {
		tenon.Encode(struct {
			A int `tenon:"caf\U000000e9"`
			B int `tenon:"cafe\U00000301"`
		}{})
	})
	mustPanicUsage(t, "must be named by its tenon tag", func() { tenon.Encode(embedsUntagged{}) })
	// An embedded field named by its tag is a field of its own type, and an
	// unexported one is not mapped.
	wantValue(t, "an embedded field", encoded(t, embeds{Address: Address{Street: "Main"}, address: address{Street: "hidden"}, Name: "x"}),
		obj(map[string]tenon.Value{"name": s("x"), "addr": obj(map[string]tenon.Value{"Street": s("Main")})}))
}

type holder struct {
	Name  string      `tenon:"name"`
	Extra tenon.Value `tenon:"extra,optional"`
}

func TestConformance_GO012_ValuesOfManyTypes(t *testing.T) {
	conformance.Covers(t, "GO-012", "GO-013", "GO-022")
	values := []tenon.Value{n(1), s("x"), tenon.Unknown(boo)}
	wantValue(t, "a slice of values", encoded(t, values), tenon.TupleVal(values...))
	wantValue(t, "a map of values", encoded(t, map[string]tenon.Value{"a": n(1), "b": s("x")}),
		obj(map[string]tenon.Value{"a": n(1), "b": s("x")}))
	wantEncodeFailure(t, "an empty key", map[string]tenon.Value{"": n(1)},
		wantDiag{tenon.CodeConvertUnexpectedAttribute, `[""]`})
	wantEncodeFailure(t, "keys that are one", map[string]tenon.Value{"caf\U000000e9": n(1), "cafe\U00000301": n(2)},
		wantDiag{tenon.CodeMapDuplicateKey, ""})

	// Nil encodes as the null of the type, and empty as empty.
	var nilInts []int
	var nilMap map[string]bool
	var nilPtr *address
	var nilValues []tenon.Value
	var nilHolder *holder
	for _, tt := range []struct {
		name      string
		got, want tenon.Value
	}{
		{"a nil slice", encoded(t, nilInts), tenon.NullVal(tenon.List(num))},
		{"an empty slice", encoded(t, []int{}), tenon.ListVal(num)},
		{"a nil map", encoded(t, nilMap), tenon.NullVal(tenon.Map(boo))},
		{"a nil pointer", encoded(t, nilPtr), tenon.NullVal(tenon.Object(map[string]tenon.Type{"street": str, "unit": num}))},
		{"a nil slice of values", encoded(t, nilValues), tenon.NullVal(tenon.Tuple())},
		{"a nil pointer to a type of no type", encoded(t, nilHolder), tenon.Narrow(tenon.Pending(tenon.Any()), tenon.Null())},
	} {
		wantValue(t, tt.name, tt.got, tt.want)
	}

	// An optional tenon.Value holding the zero Value is left out, and a
	// required one is a defect in the program.
	wantValue(t, "an absent extra", encoded(t, holder{Name: "x"}), obj(map[string]tenon.Value{"name": s("x")}))
	wantValue(t, "an extra", encoded(t, holder{Name: "x", Extra: n(1)}), obj(map[string]tenon.Value{"name": s("x"), "extra": n(1)}))
	mustPanicUsage(t, "holding the zero Value", func() {
		tenon.Encode(struct {
			V tenon.Value `tenon:"v"`
		}{})
	})
}

func TestConformance_GO030_NumbersEncodeExactly(t *testing.T) {
	conformance.Covers(t, "GO-030", "GO-031", "GO-033", "GO-003", "GO-050")
	for _, tt := range []struct {
		name string
		got  tenon.Value
		want string
	}{
		{"0.1 as a float64", encoded(t, 0.1), "0.1000000000000000055511151231257827021181583404541015625"},
		{"0.1 as a float32", encoded(t, float32(0.1)), "0.100000001490116119384765625"},
		{"a large float64", encoded(t, 1e300), "1000000000000000052504760255204420248704468581108159154915854115511802457988908195786371375080447864043704443832883878176942523235360430575644792184786706982848387200926575803737830233794788090059368953234970799945081119038967640880074652742780142494579258788820056842838115669472196386865459400540160"},
		{"the least subnormal", encoded(t, math.SmallestNonzeroFloat64), "4.940656458412465441765687928682213723650598026143247644255856825006755072702087518652998363616359923797965646954457177309266567103559397963987747960107818781263007131903114045278458171678489821036887186360569987307230500063874091535649843873124733972731696151400317153853980741262385655911710266585566867681870395603106249319452715914924553293054565444011274801297099995419319894090804165633245247571478690147267801593552386115501348035264934720193790268107107491703332226844753335720832431936092382893458368060106011506169809753078342277318329247904982524730776375927247874656084778203734469699533647017972677717585125660551199131504891101451037862738167250955837389733598993664809941164205702637090279242767544565229087538682506419718265533447265625e-324"},
		{"negative zero", encoded(t, math.Copysign(0, -1)), "0"},
		{"a big integer", encoded(t, *new(big.Int).Exp(big.NewInt(10), big.NewInt(50), nil)), "1e50"},
		{"a big float", encoded(t, *new(big.Float).SetMantExp(big.NewFloat(3), -3)), "0.375"},
		{"a big rational", encoded(t, *big.NewRat(1, 8)), "0.125"},
		{"a rational over fifths", encoded(t, *big.NewRat(-3, 25)), "-0.12"},
	} {
		if !tt.got.IsKnown() || tt.got.Type() != num {
			t.Errorf("%s: %v is not a known number", tt.name, tt.got)
			continue
		}
		if want := tenon.NumberFromText(tt.want); !tenon.Equals(tt.got, want).AsBool() {
			t.Errorf("%s: %v, want %s", tt.name, tt.got, tt.want)
		}
	}

	type measured struct {
		A float64   `tenon:"a"`
		B []float32 `tenon:"b"`
		C big.Float `tenon:"c"`
		D big.Rat   `tenon:"d"`
		E string    `tenon:"e"`
		F big.Int   `tenon:"f"`
	}
	bad := measured{
		A: math.NaN(), B: []float32{1, float32(math.Inf(-1))},
		C: *new(big.Float).SetInf(false), D: *big.NewRat(1, 3), E: "\xff",
		F: *new(big.Int).Exp(big.NewInt(10), big.NewInt(1000001), nil),
	}
	// Every part that fails is reported, where it is, and nothing panics.
	wantEncodeFailure(t, "numbers that are not", bad,
		wantDiag{tenon.CodeEncodeNotANumber, ".a"},
		wantDiag{tenon.CodeEncodeNotANumber, ".b[1]"},
		wantDiag{tenon.CodeEncodeNotANumber, ".c"},
		wantDiag{tenon.CodeEncodeInexact, ".d"},
		wantDiag{tenon.CodeStringInvalidUTF8, ".e"},
		wantDiag{tenon.CodeNumberOutOfRange, ".f"})
	_, err := tenon.Encode(bad)
	if msg := err.Error(); msg == "" || !errors.As(err, new(*tenon.DiagnosticError)) {
		t.Errorf("the error reads %q", msg)
	}
	wantValue(t, "a string in normal form", encoded(t, "cafe\U00000301"), s("caf\U000000e9"))
}

func TestConformance_GO043_EncodingGivesKnownValues(t *testing.T) {
	conformance.Covers(t, "GO-043")
	unit := 2
	for _, x := range []any{
		person{Name: "x", Home: address{Unit: &unit}},
		[]map[string][2]float64{{"a": {0.5, -1}}},
		map[string]*address{"a": nil},
	} {
		var v tenon.Value
		switch x := x.(type) {
		case person:
			v = encoded(t, x)
		case []map[string][2]float64:
			v = encoded(t, x)
		case map[string]*address:
			v = encoded(t, x)
		}
		if _, marks := tenon.UnmarkDeep(v); !v.IsKnown() || marks != nil {
			t.Errorf("Encode(%v) = %v, not a known, unmarked value", x, v)
		}
	}
	// What a tenon.Value holds is its own.
	marked := tenon.WithMarks(tenon.Unknown(num), stamp{id: "m"})
	if v := encoded(t, holder{Name: "x", Extra: marked}); tenon.Identical(v, obj(map[string]tenon.Value{"name": s("x"), "extra": marked})) == false {
		t.Errorf("a tenon.Value field encoded as %v", v)
	}
}
