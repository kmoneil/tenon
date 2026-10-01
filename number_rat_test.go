package tenon_test

import (
	"math/big"
	"math/rand"
	"runtime"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/internal/conformance/values"
)

// TestConformance_GO030_NumberFromBigRat holds NumberFromBigRat to GO-030's
// encoding of a big.Rat: the Number of exactly its value where it terminates,
// encode.inexact where it does not, and number.out_of_range outside the
// window.
func TestConformance_GO030_NumberFromBigRat(t *testing.T) {
	conformance.Covers(t, "GO-030")
	one := big.NewInt(1)
	pow := func(base, k int64) *big.Int { return new(big.Int).Exp(big.NewInt(base), big.NewInt(k), nil) }
	for _, tt := range []struct {
		r    *big.Rat
		want string
	}{
		{big.NewRat(1, 8), "0.125"},
		{big.NewRat(-3, 25), "-0.12"},
		{big.NewRat(3, 400), "0.0075"},
		{big.NewRat(7, 1), "7"},
		{big.NewRat(0, 5), "0"},
		{big.NewRat(-12000, 10), "-1200"},
		{new(big.Rat).SetFrac(one, pow(5, 999_999)), ""}, // at the window's edge
	} {
		got := tenon.NumberFromBigRat(tt.r)
		if got.IsError() || !got.IsKnown() {
			t.Errorf("NumberFromBigRat(%v) is %v, want a number", shortRat(tt.r), got)
			continue
		}
		if tt.want != "" && !tenon.Identical(got, tenon.NumberFromText(tt.want)) {
			t.Errorf("NumberFromBigRat(%v) is %v, want %s", tt.r, got, tt.want)
		}
	}

	for _, tt := range []struct {
		name    string
		r       *big.Rat
		code    tenon.Code
		message string
	}{
		{"a third", big.NewRat(1, 3), tenon.CodeEncodeInexact, "the rational 1/3 is not a terminating decimal"},
		{"a seventh of a tenth", big.NewRat(-1, 70), tenon.CodeEncodeInexact, "the rational -1/70 is not a terminating decimal"},
		{"one place below the window", new(big.Rat).SetFrac(one, pow(5, 1_000_001)), tenon.CodeNumberOutOfRange,
			"the rational is outside the range of numbers"},
		{"above the window", new(big.Rat).SetInt(pow(10, 1_000_000)), tenon.CodeNumberOutOfRange,
			"the rational is outside the range of numbers"},
		{"a huge rational that does not terminate", new(big.Rat).SetFrac(one, pow(3, 2_100_000)), tenon.CodeEncodeInexact,
			"the rational is not a terminating decimal"},
	} {
		got := tenon.NumberFromBigRat(tt.r)
		if !got.IsError() {
			t.Errorf("%s: NumberFromBigRat gives %v, want an error value", tt.name, got)
			continue
		}
		if d := got.Diagnostics(); len(d) != 1 || d[0].Code != tt.code || d[0].Message != tt.message {
			t.Errorf("%s: NumberFromBigRat gives %v, want %s: %q", tt.name, d, tt.code, tt.message)
		}
	}

	// A rational over twos and fives is the decimal of as many places as the
	// greater count, which big.Rat writes exactly at that precision.
	r := rand.New(rand.NewSource(20261001))
	for range conformance.Iterations(t, 2000) {
		twos, fives := r.Int63n(40), r.Int63n(40)
		num := new(big.Int).Rand(r, pow(10, 30))
		if r.Intn(2) == 0 {
			num.Neg(num)
		}
		rat := new(big.Rat).SetFrac(num, new(big.Int).Mul(pow(2, twos), pow(5, fives)))
		want := tenon.NumberFromText(rat.FloatString(int(max(twos, fives))))
		if got := tenon.NumberFromBigRat(rat); !tenon.Identical(got, want) {
			t.Fatalf("NumberFromBigRat(%v) is %v, want %v", rat, got, want)
		}
	}

	// Every number of the corpus comes back from its rational as itself.
	for _, v := range values.All() {
		if !v.HasContent() || v.Type() != tenon.NumberType() {
			continue
		}
		bare, _ := tenon.Unmark(v)
		if got := tenon.NumberFromBigRat(v.AsBigRat()); !tenon.Identical(got, bare) {
			t.Errorf("NumberFromBigRat(%v.AsBigRat()) is %v", v, got)
		}
	}

	// The value does not hold on to the rational it was made from.
	held := big.NewRat(1, 4)
	quarter := tenon.NumberFromBigRat(held)
	held.SetInt64(9)
	if !tenon.Identical(quarter, tenon.NumberFromText("0.25")) {
		t.Errorf("changing the rational after NumberFromBigRat changed the number to %v", quarter)
	}

	// Refusing a denominator too large to factor costs what its bits say,
	// not a division by it.
	huge := new(big.Rat).SetFrac(one, pow(3, 2_100_000))
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	tenon.NumberFromBigRat(huge)
	runtime.ReadMemStats(&after)
	if grew := after.TotalAlloc - before.TotalAlloc; grew > 64<<10 {
		t.Errorf("refusing a huge rational that does not terminate allocated %d bytes, more than the 65,536 it may", grew)
	}

	mustPanicUsage(t, "NumberFromBigRat called with a nil *big.Rat", func() { tenon.NumberFromBigRat(nil) })
}

// shortRat names r for a message without writing a denominator of a million
// digits.
func shortRat(r *big.Rat) string {
	if r.Denom().BitLen() > 256 || r.Num().BitLen() > 256 {
		return "a rational of " + big.NewInt(int64(r.Denom().BitLen())).String() + " bits"
	}
	return r.String()
}

// TestValueIsNull holds the program's null test to the IsNull operation: true
// exactly where the operation's answer is known true.
func TestValueIsNull(t *testing.T) {
	str := tenon.StringType()
	secret := stamp{id: "secret", redact: true}
	for _, tt := range []struct {
		v    tenon.Value
		want bool
	}{
		{tenon.Null(str), true},
		{tenon.WithMarks(tenon.Null(str), secret), true},
		{tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly()), true},
		{tenon.String(""), false},
		{tenon.List(str, tenon.Null(str)), false},
		{tenon.Unknown(str), false}, // it may yet be null, and is not yet
		{tenon.Narrow(tenon.Unknown(str), tenon.NotNull()), false},
		{tenon.Pending(tenon.Any()), false},
		{tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NotNull()), false},
		{tenon.String("\xff"), false},
	} {
		if got := tt.v.IsNull(); got != tt.want {
			t.Errorf("%v: IsNull is %t, want %t", tt.v, got, tt.want)
		}
	}
	checked := 0
	for _, v := range values.All() {
		answer := tenon.IsNull(v)
		want := !answer.IsError() && answer.IsKnown() && answer.AsBool()
		if got := v.IsNull(); got != want {
			t.Errorf("%v: IsNull is %t, where the operation answers %v", v, got, answer)
		}
		checked++
	}
	if checked < 100 {
		t.Errorf("checked %d values, want the corpus's", checked)
	}
	mustPanicUsage(t, "use of the zero Value", func() { tenon.Value{}.IsNull() })
}
