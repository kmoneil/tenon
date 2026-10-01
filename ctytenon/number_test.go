package ctytenon

import (
	"math/big"
	"math/rand"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/zclconf/go-cty/cty"
)

// parsed returns the number cty's parser reads from s.
func parsed(t *testing.T, s string) *big.Float {
	t.Helper()
	v, err := cty.ParseNumberVal(s)
	if err != nil {
		t.Fatalf("cty.ParseNumberVal(%q): %v", s, err)
	}
	return v.AsBigFloat()
}

// show returns f in binary, as 0x.8p-2999998: writing it in decimal works
// out every digit of its exact value, which takes minutes at the window's
// edges.
func show(f *big.Float) string { return f.Text('p', 0) }

// binary returns m × 2^e held in prec bits.
func binary(m int64, e int, prec uint) *big.Float {
	f := new(big.Float).SetPrec(prec).SetInt64(m)
	return f.SetMantExp(f, e)
}

// TestNumbersFromCty holds cty numbers to crossing as the decimal of the
// fewest digits cty's parser reads as them, as their exact value where they
// are held in more than 512 bits, and to failing where tenon has no such
// number.
func TestNumbersFromCty(t *testing.T) {
	third := new(big.Float).SetPrec(512).Quo(new(big.Float).SetPrec(512).SetInt64(1), new(big.Float).SetPrec(512).SetInt64(3))
	fineThird := new(big.Float).SetPrec(1000).Quo(new(big.Float).SetPrec(1000).SetInt64(1), new(big.Float).SetPrec(1000).SetInt64(3))
	for _, c := range []struct {
		name string
		f    *big.Float
		want string
	}{
		{"HCL's 0.1", parsed(t, "0.1"), "0.1"},
		{"a negative fraction", parsed(t, "-2.5e-7"), "-2.5e-7"},
		{"an integer", parsed(t, "-42"), "-42"},
		{"zero", new(big.Float), "0"},
		{"negative zero", new(big.Float).Neg(new(big.Float)), "0"},
		// A 53-bit 0.1 is the binary fraction nearest 0.1, which 512 bits
		// tell from 0.1 and from every shorter decimal.
		{"a 53-bit 0.1", binary(3602879701896397, -55, 53), "0.1000000000000000055511151231257827021181583404541015625"},
		// Go's own shortest formatting gives it at 512 bits too.
		{"1/3 in 512 bits", third, third.Text('g', -1)},
		{"the window's last place", parsed(t, "1e-999999"), "1e-999999"},
		{"the window's first place", parsed(t, "9.87654321e999999"), "9.87654321e999999"},
		// cty reads 1e1000000 as the integer of a million digits just below
		// it, whose shortest decimal is 1e1000000, outside the window: it
		// crosses exactly.
		{"cty's 1e1000000", parsed(t, "1e1000000"), ""},
		// 2^-3000000 has its last digit at the 10^-3000000 place, outside the
		// window, but the decimal of 156 digits that cty reads as it has not.
		{"a binary fraction finer than the window", binary(1, -2999999, 1), ""},
		// Held in 1,000 bits, a third crosses exactly.
		{"1/3 in 1,000 bits", fineThird, ""},
	} {
		got := numberFromCty(c.f)
		if got.IsError() {
			t.Errorf("%s: numberFromCty(%s) = %v", c.name, show(c.f), got)
			continue
		}
		if c.want != "" {
			if want := tenon.NumberFromText(c.want); !got.Equal(want) {
				t.Errorf("%s: numberFromCty(%s) = %v, want %v", c.name, show(c.f), got, want)
			}
		}
		if c.f.MinPrec() > ctyPrecision {
			if got.AsBigRat().Cmp(mustRat(c.f)) != 0 {
				t.Errorf("%s: numberFromCty(%s) = %v, which is not its exact value", c.name, show(c.f), got)
			}
			continue
		}
		if back := numberToCty(got).AsBigFloat(); back.Cmp(c.f) != 0 {
			t.Errorf("%s: %s crossed as %v and back as %s", c.name, show(c.f), got, show(back))
		}
	}
}

// mustRat returns the exact value of f.
func mustRat(f *big.Float) *big.Rat {
	r, _ := f.Rat(nil)
	return r
}

// TestNumbersTenonHasNot holds the cty numbers tenon has no number for to
// failing with the code that says why.
func TestNumbersTenonHasNot(t *testing.T) {
	for _, c := range []struct {
		name string
		f    *big.Float
		code tenon.Code
		msg  string
	}{
		{"an infinity", new(big.Float).SetInf(false), tenon.CodeEncodeNotANumber, "+Inf is not a number"},
		{"a negative infinity", new(big.Float).SetInf(true), tenon.CodeEncodeNotANumber, "-Inf is not a number"},
		{"a place above the window", parsed(t, "2e1000000"), tenon.CodeNumberOutOfRange, ""},
		{"a place below the window", parsed(t, "1e-1000000"), tenon.CodeNumberOutOfRange, ""},
		// Within a factor of two of the window, the binary exponent does not
		// decide, and tenon's own reading of the decimal does.
		{"half the window's last place", parsed(t, "5e-1000000"), tenon.CodeNumberOutOfRange, ""},
		{"far above the window", binary(1, 1<<30, 1), tenon.CodeNumberOutOfRange, ""},
		{"far below the window", binary(1, -1<<30, 1), tenon.CodeNumberOutOfRange, ""},
		// Held in 601 bits, it crosses exactly or not at all, and its last
		// digit lies at the 10^-1000600 place.
		{"held exactly, below the window", heldExactly(601, -1000600), tenon.CodeNumberOutOfRange, ""},
	} {
		got := numberFromCty(c.f)
		if !got.IsError() {
			t.Errorf("%s: numberFromCty = %v; want an error with code %s", c.name, got, c.code)
			continue
		}
		if d := got.Diagnostics(); len(d) != 1 || d[0].Code != c.code || c.msg != "" && d[0].Message != c.msg {
			t.Errorf("%s: numberFromCty = %v; want an error with code %s", c.name, got, c.code)
		}
	}
}

// heldExactly returns (2^(bits-1) + 1) × 2^e, which needs bits bits.
func heldExactly(bits uint, e int) *big.Float {
	m := new(big.Int).Lsh(big.NewInt(1), bits-1)
	f := new(big.Float).SetPrec(bits).SetInt(m.Add(m, big.NewInt(1)))
	return f.SetMantExp(f, e-f.MantExp(nil)+int(bits))
}

// TestNumbersFromHCL holds numbers as HCL writes them to crossing as
// themselves and back as the number cty's parser read.
func TestNumbersFromHCL(t *testing.T) {
	for _, s := range []string{"0.1", "0.2", "3.14159", "1e-7", "100", "1.5e300", "123456789012345678901234567890.5", "0.000001", "2.718281828459045235360287471352662497757"} {
		f := parsed(t, s)
		got := numberFromCty(f)
		if want := tenon.NumberFromText(s); !got.Equal(want) {
			t.Errorf("HCL's %s crossed as %v", s, got)
		}
		if back := numberToCty(got); !back.RawEquals(cty.NumberVal(f)) {
			t.Errorf("HCL's %s crossed back as %#v", s, back)
		}
	}
}

// TestCtyIssue220 holds the bench module's #220 case, a big.Float of 64 bits
// holding 2^128, to crossing as its exact integer, where cty's own JSON and
// msgpack write it as rounded text, and back as the same number.
func TestCtyIssue220(t *testing.T) {
	f, _, err := big.ParseFloat("340282366920938463463374607431768211457", 10, 64, big.ToNearestEven)
	if err != nil {
		t.Fatal(err)
	}
	got := numberFromCty(f)
	if i, ok := got.AsBigInt(); !ok || i.Cmp(new(big.Int).Lsh(big.NewInt(1), 128)) != 0 {
		t.Errorf("#220's number crossed as %v, want 2^128", got)
	}
	if back := numberToCty(got); !back.RawEquals(cty.NumberVal(f)) {
		t.Errorf("#220's number crossed back as %#v", back)
	}
}

// TestNumbersRoundTripFromCty carries random cty numbers, of the kinds cty's
// parser, its integer constructors and a 53-bit binary fraction make and of
// every precision up to 512 bits, to tenon and back, which gives the same
// number. Where Go's own shortest formatting is quick enough to ask, the
// decimal is the one it gives at 512 bits.
func TestNumbersRoundTripFromCty(t *testing.T) {
	r := rand.New(rand.NewSource(20261003))
	asked := 0
	for range conformance.Iterations(t, 3000) {
		f := randomCtyNumber(r)
		got := numberFromCty(f)
		if got.IsError() {
			t.Fatalf("numberFromCty(%s) = %v", show(f), got)
		}
		if back := numberToCty(got).AsBigFloat(); back.Cmp(f) != 0 {
			t.Fatalf("%s crossed as %v and back as %s", show(f), got, show(back))
		}
		if e := f.MantExp(nil); e > -2000 && e < 2000 {
			g := new(big.Float).Copy(f).SetPrec(ctyPrecision)
			if want := tenon.NumberFromText(g.Text('g', -1)); !got.Equal(want) {
				t.Fatalf("%s crossed as %v, but the shortest decimal Go gives at 512 bits is %v", show(f), got, want)
			}
			asked++
		}
	}
	if asked < 1000 {
		t.Errorf("asked Go's shortest formatting of %d numbers, want many", asked)
	}
}

// randomCtyNumber returns a random finite cty number inside tenon's window.
func randomCtyNumber(r *rand.Rand) *big.Float {
	exponent := func(limit int) int {
		if r.Intn(3) == 0 {
			return r.Intn(2*limit+1) - limit
		}
		return r.Intn(81) - 40
	}
	sign := int64(1 - 2*r.Intn(2))
	switch r.Intn(4) {
	case 0:
		// A decimal as HCL writes it, read by cty's parser.
		v, _ := cty.ParseNumberVal(randomDigits(r, 1+r.Intn(60)) + "e" + strconv.Itoa(exponent(999000)))
		if sign < 0 {
			return new(big.Float).Neg(v.AsBigFloat())
		}
		return v.AsBigFloat()
	case 1:
		return new(big.Float).SetInt64(sign * r.Int63())
	case 2:
		// A float64's value, held in 53 bits.
		return binary(sign*(1<<52|r.Int63n(1<<52)), exponent(1000), 53)
	}
	// A binary fraction of up to 512 bits.
	bits := 1 + r.Intn(ctyPrecision)
	m := new(big.Int).Rand(r, new(big.Int).Lsh(big.NewInt(1), uint(bits)))
	m.SetBit(m, bits-1, 1)
	f := new(big.Float).SetPrec(ctyPrecision).SetInt(m)
	if sign < 0 {
		f.Neg(f)
	}
	return f.SetMantExp(f, exponent(3321000)-f.MantExp(nil))
}

// randomDigits returns n random decimal digits, the first not zero.
func randomDigits(r *rand.Rand, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte('0' + r.Intn(10))
	}
	b[0] = byte('1' + r.Intn(9))
	return string(b)
}

// TestNumbersRoundTripFromTenon carries random tenon numbers to cty and back.
// One of up to 153 digits, which 512 bits tell apart from every other, comes
// back identical; one of more comes back as the decimal of fewest digits that
// cty reads as the same 512 bits, which crosses to cty as those bits again.
func TestNumbersRoundTripFromTenon(t *testing.T) {
	r := rand.New(rand.NewSource(20261004))
	for range conformance.Iterations(t, 3000) {
		n := 1 + r.Intn(153)
		if r.Intn(4) == 0 {
			n = 154 + r.Intn(400)
		}
		last := r.Intn(81) - 40
		if r.Intn(3) == 0 {
			last = r.Intn(1999999-n) - 999999
		}
		text := randomDigits(r, n) + "e" + strconv.Itoa(last)
		if r.Intn(2) == 0 {
			text = "-" + text
		}
		v := tenon.NumberFromText(text)
		if v.IsError() {
			t.Fatalf("NumberFromText(%s) = %v", text, v)
		}
		c := numberToCty(v)
		back := numberFromCty(c.AsBigFloat())
		if n <= 153 {
			if !back.Equal(v) {
				t.Fatalf("%s crossed as %#v and back as %v", text, c, back)
			}
			continue
		}
		if again := numberToCty(back); again.AsBigFloat().Cmp(c.AsBigFloat()) != 0 {
			t.Fatalf("%s crossed as %#v, back as %v, and again as %#v", text, c, back, again)
		}
	}
}

// TestLongNumbersToCty holds a number whose text is longer than cty's parser
// is handed to rounding to 512 bits from its exact value, which is the
// number cty's parser reads from the text where it rounds correctly.
func TestLongNumbersToCty(t *testing.T) {
	for _, digits := range []int{maxParsedText - 2, maxParsedText, maxParsedText + 2, 20000} {
		i, _ := new(big.Int).SetString("1"+strings.Repeat("3", digits-1), 10)
		v := tenon.Mul(tenon.NumberFromBigInt(i), tenon.NumberFromText("1e-"+strconv.Itoa(digits-1)))
		got := numberToCty(v).AsBigFloat()
		if want := parsed(t, v.String()); got.Cmp(want) != 0 {
			t.Errorf("%d digits, %d bytes of text, crossed as %s, but cty's parser reads %s", digits, len(v.String()), show(got), show(want))
		}
	}
}

// TestNumbersAtTheEdgesCostLittle holds crossing numbers at the edges of
// tenon's window, and beyond them, to an allocation budget. Formatting such a
// number with big.Float.Text, the obvious way to its shortest decimal, works
// out every digit of its exact value first, allocating hundreds of megabytes
// for 1e-999999 over minutes; reading a million digits with cty's parser
// takes half a second.
func TestNumbersAtTheEdgesCostLittle(t *testing.T) {
	million := tenon.Add(tenon.NumberFromInt(1), tenon.NumberFromText("1e-999999"))
	for _, c := range []struct {
		name   string
		call   func()
		budget uint64
	}{
		{"1e-999999 to tenon", func() { numberFromCty(parsed(t, "1e-999999")) }, 1 << 20},
		{"9.87654321e999999 to tenon", func() { numberFromCty(parsed(t, "9.87654321e999999")) }, 1 << 20},
		{"a 512-bit fraction at the window's last place to tenon", func() { numberFromCty(parsed(t, "1.2345678901234567890123456789e-999990")) }, 1 << 20},
		{"2^-3000000 to tenon", func() { numberFromCty(binary(1, -2999999, 1)) }, 1 << 20},
		{"2^-(2^30) to tenon", func() { numberFromCty(binary(1, -1<<30, 1)) }, 64 << 10},
		{"1e-999999 to cty", func() { numberToCty(tenon.NumberFromText("1e-999999")) }, 1 << 20},
		{"1 + 1e-999999 to cty", func() { numberToCty(million) }, 64 << 20},
		{"cty's 1e1000000 to tenon", func() { numberFromCty(parsed(t, "1e1000000")) }, 64 << 20},
	} {
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		c.call()
		runtime.ReadMemStats(&after)
		if grew := after.TotalAlloc - before.TotalAlloc; grew > c.budget {
			t.Errorf("%s allocated %d bytes, more than the %d it may", c.name, grew, c.budget)
		}
	}
}
