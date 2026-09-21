package gotenon_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/conformance"
	"github.com/kmoneil/tenon/gotenon"
)

// encoded encodes x, failing t if encoding fails.
func encoded[T any](t *testing.T, x T) tenon.Value {
	t.Helper()
	v, err := gotenon.Encode(x)
	if err != nil {
		t.Fatalf("Encode(%v) failed: %v", x, err)
	}
	return v
}

// wantEncodeFailure fails t unless encoding x fails with exactly these
// diagnostics.
func wantEncodeFailure[T any](t *testing.T, what string, x T, want ...wantDiag) {
	t.Helper()
	_, err := gotenon.Encode(x)
	var de *gotenon.DiagnosticError
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
	// An interface encodes as what it holds [GO-015], and only decoding into
	// one is a usage error: nothing in a value says which Go type it takes.
	mustPanicUsage(t, "which says nothing of the Go type it would take", func() {
		gotenon.Decode[any](n(1), safe)
	})
	mustPanicUsage(t, "which says nothing of the Go type it would take", func() {
		gotenon.Decode[map[string]any](obj(map[string]tenon.Value{"a": n(1)}), safe)
	})
	mustPanicUsage(t, "of kind chan", func() { gotenon.Encode(make(chan int)) })
	mustPanicUsage(t, "of kind func", func() { gotenon.Encode(func() {}) })
	mustPanicUsage(t, "of kind complex128", func() { gotenon.Encode(complex(1, 2)) })
	mustPanicUsage(t, "pointer to tenon.Value", func() { gotenon.Encode(&tenon.Value{}) })
	mustPanicUsage(t, "keys of kind int", func() { gotenon.Encode(map[int]string{}) })
	mustPanicUsage(t, "holds itself", func() { gotenon.Encode(selfish{}) })

	// Tags are checked when the type is first met.
	mustPanicUsage(t, `whose option "omitempty" is not optional`, func() {
		gotenon.Encode(struct {
			A int `tenon:"a,omitempty"`
		}{})
	})
	mustPanicUsage(t, "skips it and so takes no options", func() {
		gotenon.Encode(struct {
			A int `tenon:"-,optional"`
		}{})
	})
	mustPanicUsage(t, "both map to the attribute", func() {
		gotenon.Encode(struct {
			A int `tenon:"caf\U000000e9"`
			B int `tenon:"cafe\U00000301"`
		}{})
	})
	mustPanicUsage(t, "must be named by its tenon tag", func() { gotenon.Encode(embedsUntagged{}) })
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
		wantDiag{tenon.CodeConvertUnexpectedAttribute, `.[""]`})
	wantEncodeFailure(t, "keys that are one", map[string]tenon.Value{"caf\U000000e9": n(1), "cafe\U00000301": n(2)},
		wantDiag{tenon.CodeMapDuplicateKey, "."})

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
		{"a nil pointer to a type of no type", encoded(t, nilHolder), tenon.NullVal(tenon.Object(map[string]tenon.Type{"name": str}))},
		{"a nil pointer to a slice of values", encoded(t, &nilValues), tenon.NullVal(tenon.Tuple())},
	} {
		wantValue(t, tt.name, tt.got, tt.want)
	}

	// An optional tenon.Value holding the zero Value is left out, and a
	// required one is a defect in the program.
	wantValue(t, "an absent extra", encoded(t, holder{Name: "x"}), obj(map[string]tenon.Value{"name": s("x")}))
	wantValue(t, "an extra", encoded(t, holder{Name: "x", Extra: n(1)}), obj(map[string]tenon.Value{"name": s("x"), "extra": n(1)}))
	mustPanicUsage(t, "holding the zero Value", func() {
		gotenon.Encode(struct {
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
	_, err := gotenon.Encode(bad)
	if msg := err.Error(); msg == "" || !errors.As(err, new(*gotenon.DiagnosticError)) {
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

// TestConformance_GO015_InterfacesEncodeWhatTheyHold holds an interface to the
// value it holds, which is how data whose types are not known at compile time
// reaches a Go program: encoding/json gives map[string]any, and a container
// whose members' types need not agree encodes as [GO-012] says.
func TestConformance_GO015_InterfacesEncodeWhatTheyHold(t *testing.T) {
	conformance.Covers(t, "GO-015", "GO-010", "GO-012")
	wantValue(t, "an interface holding a number", encoded(t, any(1)), n(1))
	wantValue(t, "an interface holding a string", encoded(t, any("x")), s("x"))
	// A map of them is an object and a slice of them a tuple [GO-012].
	wantValue(t, "a map of anything", encoded(t, map[string]any{"a": 1, "b": "x", "c": true}),
		obj(map[string]tenon.Value{"a": n(1), "b": s("x"), "c": tenon.Bool(true)}))
	wantValue(t, "a slice of anything", encoded(t, []any{1, "x"}), tenon.TupleVal(n(1), s("x")))
	// What it holds may itself hold anything, at any depth.
	wantValue(t, "anything within anything", encoded(t, map[string]any{"a": []any{map[string]any{"b": 1}}}),
		obj(map[string]tenon.Value{"a": tenon.TupleVal(obj(map[string]tenon.Value{"b": n(1)}))}))
	// A field of interface type carries what the struct's own types cannot.
	wantValue(t, "a struct field", encoded(t, struct {
		Extra any `tenon:"extra"`
	}{Extra: "x"}), obj(map[string]tenon.Value{"extra": s("x")}))
	// An interface that is not empty encodes what it holds just the same.
	var held fmt.Stringer = stringer{Name: "x"}
	wantValue(t, "an interface with methods", encoded(t, held), obj(map[string]tenon.Value{"Name": s("x")}))
	// A tenon.Value held in an interface is that value.
	wantValue(t, "a value held in an interface", encoded(t, any(tenon.Unknown(num))), tenon.Unknown(num))

	// A nil interface holds no value, and no type follows from nothing.
	_, err := gotenon.Encode(map[string]any{"a": nil})
	var failed *gotenon.DiagnosticError
	if !errors.As(err, &failed) {
		t.Fatalf("a nil interface encoded: %v", err)
	}
	// The map encodes as an object [GO-012], so the failure is located at an
	// attribute of one and not at a key of a map.
	wantErrors(t, "a nil interface", failed.Value, wantDiag{tenon.CodeEncodeUntypedNil, ".a"})
	_, err = gotenon.Encode([]any{1, nil})
	if !errors.As(err, &failed) {
		t.Fatalf("a nil interface in a slice encoded: %v", err)
	}
	wantErrors(t, "a nil interface in a slice", failed.Value, wantDiag{tenon.CodeEncodeUntypedNil, ".[1]"})
	// What it holds must itself map, as any other Go value must [GO-011].
	mustPanicUsage(t, "of kind chan", func() { gotenon.Encode(any(make(chan int))) })

	// An interface is the only way a Go value holds itself, a type that holds
	// itself having no mapping at all, and a value that does is a usage error
	// rather than a stack that runs out.
	cyclicMap := map[string]any{"name": "web"}
	cyclicMap["self"] = cyclicMap
	mustPanicUsage(t, "holds itself", func() { gotenon.Encode(cyclicMap) })
	cyclicSlice := make([]any, 1)
	cyclicSlice[0] = cyclicSlice
	mustPanicUsage(t, "holds itself", func() { gotenon.Encode(cyclicSlice) })
	ring := &ringed{}
	ring.Held = ring
	mustPanicUsage(t, "holds itself", func() { gotenon.Encode(ring) })

	// Holding the same thing twice is not holding itself.
	shared := map[string]any{"a": 1}
	inner := obj(map[string]tenon.Value{"a": n(1)})
	wantValue(t, "the same map in two places", encoded(t, map[string]any{"x": shared, "y": shared}),
		obj(map[string]tenon.Value{"x": inner, "y": inner}))
	// Nor is a slice holding what it held before it grew: the members it
	// holds are not the members the one within it holds.
	grown := []any{"x"}
	grown = append(grown, grown)
	wantValue(t, "a slice holding what it held", encoded(t, grown),
		tenon.TupleVal(s("x"), tenon.TupleVal(s("x"))))
}

// ringed is a struct that can hold anything, which is how a Go value comes to
// hold itself without a Go type that holds itself.
type ringed struct {
	Held any `tenon:"held"`
}

// stringer is a type behind an interface with a method, to show that an
// interface encodes what it holds whether or not it is the empty one.
type stringer struct {
	Name string
}

func (stringer) String() string { return "x" }

// TestConformance_GO034_JSONNumbers holds a json.Number to the number its text
// spells, which is the only exact way a JSON document's numbers reach a value.
func TestConformance_GO034_JSONNumbers(t *testing.T) {
	conformance.Covers(t, "GO-034", "GO-010", "GO-004")
	wantValue(t, "an integer", encoded(t, json.Number("8080")), n(8080))
	wantValue(t, "a fraction", encoded(t, json.Number("0.1")), tenon.NumberFromText("0.1"))
	wantValue(t, "an exponent", encoded(t, json.Number("1e3")), n(1000))
	wantValue(t, "a negative", encoded(t, json.Number("-2.50")), tenon.NumberFromText("-2.5"))

	// The document's own distinction survives: a number is a Number and a
	// string is a String, so a schema of numbers is met under the safe policy.
	var fields map[string]any
	decoder := json.NewDecoder(strings.NewReader(`{"port": 8080, "name": "8080"}`))
	decoder.UseNumber()
	if err := decoder.Decode(&fields); err != nil {
		t.Fatal(err)
	}
	wantValue(t, "a document", encoded(t, fields), obj(map[string]tenon.Value{"port": n(8080), "name": s("8080")}))

	// Decoding gives the canonical text of the number, so a round trip keeps
	// the number rather than the spelling [GO-004].
	back, err := gotenon.Decode[json.Number](encoded(t, json.Number("1e3")), safe)
	if err != nil || back != json.Number("1000") {
		t.Errorf("1e3 came back as %q, %v", back, err)
	}
	if got, err := gotenon.Decode[json.Number](n(8080), safe); err != nil || got != json.Number("8080") {
		t.Errorf("8080 came back as %q, %v", got, err)
	}

	// Text that spells no number is data that is wrong, not a panic.
	_, err = gotenon.Encode(json.Number("http"))
	var failed *gotenon.DiagnosticError
	if !errors.As(err, &failed) {
		t.Fatalf(`json.Number("http") encoded: %v`, err)
	}
	wantErrors(t, "text that is no number", failed.Value, wantDiag{tenon.CodeNumberInvalidSyntax, "."})

	// Text longer than parsing reads is refused before it is read [NU-024],
	// at the path of the number in the document.
	_, err = gotenon.Encode(map[string]any{"n": json.Number(strings.Repeat("7", 10_001))})
	if !errors.As(err, &failed) {
		t.Fatalf("a json.Number of 10,001 digits encoded: %v", err)
	}
	wantErrors(t, "text longer than parsing reads", failed.Value, wantDiag{tenon.CodeNumberTooLong, ".n"})
}

// TestConformance_GO030_NumbersOfManyDigits holds Go's big numbers to their
// exact values however many digits they have: they are made from their
// coefficients, not read back from text, so the limit on how much number text
// is read does not reach them.
func TestConformance_GO030_NumbersOfManyDigits(t *testing.T) {
	conformance.Covers(t, "GO-030", "NU-024")
	huge := new(big.Int).Exp(big.NewInt(7), big.NewInt(20_000), nil) // 16,902 digits
	got := encoded(t, huge)
	if back, ok := got.AsBigInt(); !ok || back.Cmp(huge) != 0 {
		t.Errorf("7^20000 encoded as a number that is not 7^20000")
	}
	// 1 / 2^20000 is a terminating decimal of 20,000 places.
	tiny := new(big.Rat).SetFrac(big.NewInt(1), new(big.Int).Lsh(big.NewInt(1), 20_000))
	if got := encoded(t, *tiny); got.AsBigRat().Cmp(tiny) != 0 {
		t.Errorf("1/2^20000 encoded as a different number")
	}
	want := new(big.Rat).Add(big.NewRat(1, 1), tiny)
	f := new(big.Float).SetPrec(40_000).SetRat(want) // 20,001 bits, exact at this precision
	if r, _ := f.Rat(nil); r.Cmp(want) != 0 {
		t.Fatal("the big.Float does not hold 1 + 2^-20000")
	}
	if got := encoded(t, *f); got.AsBigRat().Cmp(want) != 0 {
		t.Errorf("1 + 2^-20000 as a big.Float encoded as a different number")
	}
}
