package decimal

import (
	"math"
	"math/big"
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"

	"github.com/kmoneil/tenon/conformance"
)

// noErr returns a function that fails t on an error and otherwise checks and
// returns the number, for use as noErr(t)(a.Add(b)).
func noErr(t *testing.T) func(Dec, error) Dec {
	return func(d Dec, err error) Dec {
		t.Helper()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		checkCanonical(t, d)
		return d
	}
}

// toRat returns d as an exact rational number.
func toRat(d Dec) *big.Rat {
	r := new(big.Rat).SetInt(d.scaledCoefficient(0))
	switch {
	case d.exp > 0:
		r.Mul(r, ratPow10(d.exp))
	case d.exp < 0:
		r.Quo(r, ratPow10(-d.exp))
	}
	return r
}

// ratPow10 returns 10^n as a rational, for any integer n.
func ratPow10(n int64) *big.Rat {
	if n >= 0 {
		return new(big.Rat).SetInt(pow10(n))
	}
	return new(big.Rat).SetFrac(big.NewInt(1), pow10(-n))
}

// randomNumber returns a random non-zero number: usually one of up to 41
// digits with an exponent between -30 and 30, and sometimes one near the int64
// limits, where the fast paths overflow.
func randomNumber(t *testing.T, rng *rand.Rand) Dec {
	t.Helper()
	var b strings.Builder
	if rng.IntN(4) == 0 {
		base := []int64{math.MaxInt64, math.MinInt64, math.MaxInt64 / 7, math.MinInt64 / 7}[rng.IntN(4)]
		v := new(big.Int).Add(big.NewInt(base), big.NewInt(int64(rng.IntN(21)-10)))
		if v.Sign() == 0 {
			v.SetInt64(1)
		}
		b.WriteString(v.String())
		b.WriteByte('e')
		b.WriteString(strconv.Itoa(rng.IntN(7) - 3))
	} else {
		if rng.IntN(2) == 0 {
			b.WriteByte('-')
		}
		b.WriteByte(byte('1' + rng.IntN(9)))
		for range rng.IntN(41) {
			b.WriteByte(byte('0' + rng.IntN(10)))
		}
		b.WriteByte('e')
		b.WriteString(strconv.Itoa(rng.IntN(61) - 30))
	}
	return mustParse(t, b.String())
}

// terminates reports whether the rational r has a terminating decimal
// expansion.
func terminates(r *big.Rat) bool {
	den := new(big.Int).Set(r.Denom())
	for _, p := range []int64{2, 5} {
		prime, rem := big.NewInt(p), new(big.Int)
		for {
			q, _ := new(big.Int).QuoRem(den, prime, rem)
			if rem.Sign() != 0 {
				break
			}
			den = q
		}
	}
	return den.Cmp(big.NewInt(1)) == 0
}

// ratRounded returns the text of the non-zero rational r rounded to
// DivisionPrecision significant digits by big.Rat, or false if r is too large
// for that. Rat rounds halves away from zero, which never matters here, since
// the quotients checked do not terminate.
func ratRounded(r *big.Rat) (string, bool) {
	// |r| lies in [10^(guess-1), 10^(guess+1)), where guess is the difference
	// in digit counts of the numerator and denominator.
	num, den := new(big.Int).Abs(r.Num()), r.Denom()
	guess := int64(len(num.String()) - len(den.String()))
	abs := new(big.Rat).Abs(r)
	adj := guess - 1
	if abs.Cmp(ratPow10(guess)) >= 0 {
		adj = guess
	}
	places := DivisionPrecision - 1 - adj
	if places < 0 {
		return "", false
	}
	return r.FloatString(int(places)), true
}

func TestConformance_NU010_ExactArithmetic(t *testing.T) {
	conformance.Covers(t, "NU-010")
	get := noErr(t)
	n := func(s string) Dec { return mustParse(t, s) }
	maxI, minI, one := FromInt64(math.MaxInt64), FromInt64(math.MinInt64), FromInt64(1)
	for _, tt := range []struct {
		got  Dec
		want string
	}{
		{get(n("0.1").Add(n("0.2"))), "0.3"},
		{get(n("0.3").Sub(n("0.1"))), "0.2"},
		{get(n("1.1").Mul(n("1.1"))), "1.21"},
		{get(n("1e-5").Add(n("1e5"))), "100000.00001"},

		// Results that leave the int64 range promote, and return when they
		// fit again.
		{get(maxI.Add(one)), "9223372036854775808"},
		{get(minI.Sub(one)), "-9223372036854775809"},
		{get(maxI.Mul(FromInt64(2))), "18446744073709551614"},
		{get(minI.Mul(FromInt64(-1))), "9223372036854775808"},
		{minI.Neg(), "9223372036854775808"},
		{get(maxI.Mul(maxI)), "8.5070591730234615847396907784232501249e37"},
		{get(n("922337203685477580.7").Add(n("1e18"))), "1922337203685477580.7"},
		{get(get(maxI.Add(one)).Sub(one)), "9223372036854775807"},
	} {
		if got := tt.got.String(); got != tt.want {
			t.Errorf("got %s, want %s", got, tt.want)
		}
	}
	if back := get(get(maxI.Add(one)).Sub(one)); back.big != nil {
		t.Error("a result that fits in an int64 again kept a big coefficient")
	}

	rng := rand.New(rand.NewPCG(10, 10))
	for range conformance.Iterations(t, 2000) {
		a, b := randomNumber(t, rng), randomNumber(t, rng)
		ra, rb := toRat(a), toRat(b)
		for _, op := range []struct {
			name string
			got  Dec
			want *big.Rat
		}{
			{"+", get(a.Add(b)), new(big.Rat).Add(ra, rb)},
			{"-", get(a.Sub(b)), new(big.Rat).Sub(ra, rb)},
			{"*", get(a.Mul(b)), new(big.Rat).Mul(ra, rb)},
		} {
			if toRat(op.got).Cmp(op.want) != 0 {
				t.Fatalf("%s %s %s = %s, want %s", a, op.name, b, op.got, op.want.RatString())
			}
		}
	}
}

func TestConformance_NU011_ExactDivision(t *testing.T) {
	conformance.Covers(t, "NU-011")
	get := noErr(t)
	for _, tt := range []struct{ a, b, want string }{
		{"1", "4", "0.25"},
		{"1", "8", "0.125"},
		{"10", "4", "2.5"},
		{"1", "1024", "0.0009765625"},
		{"3", "1.5", "2"},
		{"-7", "0.25", "-28"},
		{"1e-10", "1e10", "1e-20"},
		{"123456789012345678901234567890", "0.00005", "2469135780246913578024691357800000"},
	} {
		got := get(mustParse(t, tt.a).Div(mustParse(t, tt.b)))
		if !got.Equal(mustParse(t, tt.want)) {
			t.Errorf("%s / %s = %s, want %s", tt.a, tt.b, got, tt.want)
		}
	}

	// A quotient with far more digits than the rounding precision is still
	// exact when it terminates.
	two200 := new(big.Int).Lsh(big.NewInt(1), 200)
	divisor, err := FromBigInt(two200)
	if err != nil {
		t.Fatal(err)
	}
	q := get(FromInt64(1).Div(divisor))
	if want := new(big.Rat).SetFrac(big.NewInt(1), two200); toRat(q).Cmp(want) != 0 {
		t.Errorf("1 / 2^200 = %s, not exact", q)
	}

	// A product divided by one of its factors gives back the other exactly.
	rng := rand.New(rand.NewPCG(11, 11))
	for range conformance.Iterations(t, 1000) {
		a, b := randomNumber(t, rng), randomNumber(t, rng)
		if back := get(get(a.Mul(b)).Div(b)); !back.Equal(a) {
			t.Fatalf("(%s * %s) / %s = %s", a, b, b, back)
		}
	}
}

func TestConformance_NU012_RoundedDivision(t *testing.T) {
	conformance.Covers(t, "NU-012")
	get := noErr(t)
	for _, g := range []struct{ a, b, want string }{
		{"1", "3", "0." + strings.Repeat("3", 96)},
		{"2", "3", "0." + strings.Repeat("6", 95) + "7"},
		{"1", "7", "0." + strings.Repeat("142857", 16)},
		{"-1", "3", "-0." + strings.Repeat("3", 96)},
		{"10", "3", "3." + strings.Repeat("3", 95)},
		{"1e50", "3", "3." + strings.Repeat("3", 95) + "e49"},
	} {
		a, b := mustParse(t, g.a), mustParse(t, g.b)
		got := get(a.Div(b))
		if got.String() != g.want {
			t.Errorf("%s / %s = %s, want %s", g.a, g.b, got, g.want)
		}
		// The golden texts agree with big.Rat.
		if text, ok := ratRounded(new(big.Rat).Quo(toRat(a), toRat(b))); !ok || !mustParse(t, text).Equal(got) {
			t.Errorf("%s / %s: big.Rat gives %s", g.a, g.b, text)
		}
	}

	rng := rand.New(rand.NewPCG(12, 12))
	checked := 0
	for range conformance.Iterations(t, 2000) {
		a, b := randomNumber(t, rng), randomNumber(t, rng)
		exact := new(big.Rat).Quo(toRat(a), toRat(b))
		got := get(a.Div(b))
		if terminates(exact) {
			if toRat(got).Cmp(exact) != 0 {
				t.Fatalf("%s / %s = %s, but the quotient terminates", a, b, got)
			}
			continue
		}
		if len(got.coefficientDigits()) > DivisionPrecision {
			t.Fatalf("%s / %s = %s has more than %d digits", a, b, got, DivisionPrecision)
		}
		if text, ok := ratRounded(exact); ok {
			checked++
			if !mustParse(t, text).Equal(got) {
				t.Fatalf("%s / %s = %s, but big.Rat rounds it to %s", a, b, got, text)
			}
		}
	}
	if checked < 500 {
		t.Errorf("only %d rounded quotients were checked against big.Rat", checked)
	}
}

func TestConformance_NU012_HalfToEven(t *testing.T) {
	conformance.Covers(t, "NU-012")
	// A quotient that does not terminate is never exactly halfway between two
	// candidates, so the tie rule is tested on the rounding routine itself,
	// with quotients that are.
	for _, tt := range []struct {
		x, y, prec int64
		want       string
	}{
		{25, 10, 1, "2"},
		{35, 10, 1, "4"},
		{15, 10, 1, "2"},
		{45, 10, 1, "4"},
		{26, 10, 1, "3"},
		{24, 10, 1, "2"},
		{125, 100, 2, "1.2"},
		{135, 100, 2, "1.4"},
		{985, 100, 2, "9.8"},
		{995, 100, 2, "10"},
		{25, 1000, 1, "0.02"},
		{5, 1000, 1, "0.005"},
		{1, 3, 5, "0.33333"},
	} {
		q, s := roundedQuotient(big.NewInt(tt.x), big.NewInt(tt.y), tt.prec)
		got, err := fromBig(q, -s)
		if err != nil || !got.Equal(mustParse(t, tt.want)) {
			t.Errorf("%d/%d to %d digits = %v, %v; want %s", tt.x, tt.y, tt.prec, got, err, tt.want)
		}
	}
}

func TestConformance_NU013_DivideByZero(t *testing.T) {
	conformance.Covers(t, "NU-013")
	for _, s := range []string{"0", "1", "-2.5", "1e999999", "123456789012345678901234567890"} {
		if d, err := mustParse(t, s).Div(Dec{}); err != ErrDivideByZero {
			t.Errorf("%s / 0 = %v, %v; want ErrDivideByZero", s, d, err)
		}
	}
}

func TestConformance_NU014_ModuloByZero(t *testing.T) {
	conformance.Covers(t, "NU-014")
	for _, s := range []string{"0", "1", "-2.5", "1e999999", "123456789012345678901234567890"} {
		if d, err := mustParse(t, s).Mod(Dec{}); err != ErrModuloByZero {
			t.Errorf("%s mod 0 = %v, %v; want ErrModuloByZero", s, d, err)
		}
	}
}

func TestConformance_NU015_RoundingIsObservable(t *testing.T) {
	conformance.Covers(t, "NU-015")
	get := noErr(t)
	third := get(FromInt64(1).Div(FromInt64(3)))
	if third.String() != "0."+strings.Repeat("3", 96) {
		t.Errorf("1/3 = %s", third)
	}
	if back := get(third.Mul(FromInt64(3))); back.Equal(FromInt64(1)) || back.String() != "0."+strings.Repeat("9", 96) {
		t.Errorf("(1/3) * 3 = %s, want 96 nines, which is not 1", back)
	}
	if sum, twoThirds := get(third.Add(third)), get(FromInt64(2).Div(FromInt64(3))); sum.Equal(twoThirds) {
		t.Errorf("1/3 + 1/3 = %s equals 2/3 = %s, though 2/3 rounds up", sum, twoThirds)
	}
}

func TestConformance_NU016_ArithmeticRange(t *testing.T) {
	conformance.Covers(t, "NU-016")
	n := func(s string) Dec { return mustParse(t, s) }
	for _, tt := range []struct {
		name string
		got  func() (Dec, error)
	}{
		{"9e999999 + 9e999999", func() (Dec, error) { return n("9e999999").Add(n("9e999999")) }},
		{"-9e999999 - 9e999999", func() (Dec, error) { return n("-9e999999").Sub(n("9e999999")) }},
		{"1e999999 * 10", func() (Dec, error) { return n("1e999999").Mul(n("10")) }},
		{"1e-999999 * 0.1", func() (Dec, error) { return n("1e-999999").Mul(n("0.1")) }},
		{"1e-999999 / 10", func() (Dec, error) { return n("1e-999999").Div(n("10")) }},
		{"1e999999 / 0.1", func() (Dec, error) { return n("1e999999").Div(n("0.1")) }},
		{"1e-999999 / 3", func() (Dec, error) { return n("1e-999999").Div(n("3")) }},
		// A result whose leading digit is in range can still have a last digit
		// below the window.
		{"2.5e-999998 * 0.1", func() (Dec, error) { return n("2.5e-999998").Mul(n("0.1")) }},
		{"1e-999999 * 1.5", func() (Dec, error) { return n("1e-999999").Mul(n("1.5")) }},
		{"3e-999999 / 2", func() (Dec, error) { return n("3e-999999").Div(n("2")) }},
		// The same below the window with coefficients too long for an int64:
		// an exact quotient, and one rounded to its precision.
		{"1.23456789012345678901e-999979 / 4", func() (Dec, error) { return n("1.23456789012345678901e-999979").Div(n("4")) }},
		{"1e-999950 / 3", func() (Dec, error) { return n("1e-999950").Div(n("3")) }},
		// Squaring a number whose digits span the window doubles the span, so a
		// few multiplications cannot grow a coefficient without limit.
		{"(1 + 1e-999999) squared", func() (Dec, error) {
			x, err := n("1").Add(n("1e-999999"))
			if err != nil {
				t.Fatalf("1 + 1e-999999: %v", err)
			}
			return x.Mul(x)
		}},
	} {
		if d, err := tt.got(); err != ErrOutOfRange {
			t.Errorf("%s = %v, %v; want ErrOutOfRange", tt.name, d, err)
		}
	}

	get := noErr(t)
	for _, tt := range []struct {
		got  Dec
		want string
	}{
		{get(n("1e500000").Mul(n("1e499999"))), "1e999999"},
		{get(n("1e999999").Sub(n("1e999999"))), "0"},
		{get(n("5e-999999").Add(n("5e-999999"))), "1e-999998"},
		{get(n("1e999999").Div(n("10"))), "1e999998"},
		{get(n("2.5e-999998").Mod(n("1e-999998"))), "5e-999999"},
		// Trailing zeros of a product raise its last digit back into the window.
		{get(n("5e-999999").Mul(n("0.2"))), "1e-999999"},
		{get(n("2.5e-999998").Mul(n("0.4"))), "1e-999998"},
		// And with coefficients too long for an int64: 2^70 times 5^70 is
		// 10^70, whose seventy zeros bring the last digit up into the window.
		{get(n("1180591620717411303424e-999999").Mul(n("8470329472543003390683225006796419620513916015625e-70"))), "1e-999999"},
		{get(n("3e-999998").Div(n("2"))), "1.5e-999998"},
		// A remainder never has a digit below the lower of its operands' last
		// digits, so it never leaves the window.
		{get(n("1.2e-999998").Mod(n("7e-999999"))), "5e-999999"},
	} {
		if !tt.got.Equal(n(tt.want)) {
			t.Errorf("got %s, want %s", tt.got, tt.want)
		}
	}
}

func TestConformance_NU017_Remainder(t *testing.T) {
	conformance.Covers(t, "NU-017")
	get := noErr(t)
	for _, tt := range []struct{ a, b, want string }{
		{"7", "2", "1"},
		{"-7", "2", "-1"},
		{"7", "-2", "1"},
		{"-7", "-2", "-1"},
		{"5.5", "2", "1.5"},
		{"-5.5", "2", "-1.5"},
		{"1", "0.3", "0.1"},
		{"0.3", "0.1", "0"},
		{"1e20", "3", "1"},
		{"0", "5", "0"},
		{"4", "8", "4"},
		{"-9223372036854775808", "-1", "0"},
		{"9223372036854775808", "10", "8"},
	} {
		got := get(mustParse(t, tt.a).Mod(mustParse(t, tt.b)))
		if !got.Equal(mustParse(t, tt.want)) {
			t.Errorf("%s mod %s = %s, want %s", tt.a, tt.b, got, tt.want)
		}
	}

	// The remainder is smaller than the divisor, is zero or has the sign of the
	// dividend, and leaves a whole multiple of the divisor. Those properties
	// determine it.
	rng := rand.New(rand.NewPCG(17, 17))
	for range conformance.Iterations(t, 2000) {
		a, b := randomNumber(t, rng), randomNumber(t, rng)
		r := get(a.Mod(b))
		ra, rb, rr := toRat(a), toRat(b), toRat(r)
		if new(big.Rat).Abs(rr).Cmp(new(big.Rat).Abs(rb)) >= 0 {
			t.Fatalf("%s mod %s = %s, which is not smaller than the divisor", a, b, r)
		}
		if r.Sign() != 0 && r.Sign() != a.Sign() {
			t.Fatalf("%s mod %s = %s, which has the wrong sign", a, b, r)
		}
		if !new(big.Rat).Quo(new(big.Rat).Sub(ra, rr), rb).IsInt() {
			t.Fatalf("%s mod %s = %s, which leaves no whole multiple of the divisor", a, b, r)
		}
	}
}

// TestDivBound holds the directed quotient to its promise: the quotient rounded
// toward one infinity or the other at DivisionPrecision digits, and itself
// where it terminates within them.
func TestDivBound(t *testing.T) {
	threes := strings.Repeat("3", DivisionPrecision)
	for _, tt := range []struct {
		x, y     string
		down, up string
	}{
		{"1", "4", "0.25", "0.25"},
		{"-1", "4", "-0.25", "-0.25"},
		{"1", "3", "0." + threes, "0." + threes[1:] + "4"},
		{"-1", "3", "-0." + threes[1:] + "4", "-0." + threes},
		{"1", "-3", "-0." + threes[1:] + "4", "-0." + threes},
		{"0", "7", "0", "0"},
		// A quotient that terminates beyond the precision is rounded as one
		// that does not terminate is, since Div keeps every one of its digits.
		{"9999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999.9", "3",
			threes[:DivisionPrecision-1] + "3" + strings.Repeat("0", 139-DivisionPrecision),
			threes[:DivisionPrecision-1] + "4" + strings.Repeat("0", 139-DivisionPrecision)},
	} {
		x, y := mustParse(t, tt.x), mustParse(t, tt.y)
		for _, dir := range []struct {
			up   bool
			want string
		}{{false, tt.down}, {true, tt.up}} {
			got, err := x.DivBound(y, dir.up)
			if err != nil {
				t.Fatalf("%s.DivBound(%s, %t): %v", tt.x, tt.y, dir.up, err)
			}
			if want := mustParse(t, dir.want); got.Cmp(want) != 0 || got.String() != want.String() {
				t.Errorf("%s.DivBound(%s, %t) = %s, want %s", tt.x, tt.y, dir.up, got, want)
			}
		}
	}
	if _, err := mustParse(t, "1").DivBound(Dec{}, true); err != ErrDivideByZero {
		t.Errorf("DivBound by zero gave %v", err)
	}

	// Whatever the quotient, Div lies between the two, and so does the exact
	// quotient: down × y and up × y bracket x.
	r := rand.New(rand.NewPCG(96, 0))
	texts := []string{"1", "3", "7", "-7", "0.1", "12345678901234567890", "1e-50", "2", "-9.5", "1e40", "999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999.7"}
	for range 2000 {
		x, y := mustParse(t, texts[r.IntN(len(texts))]), mustParse(t, texts[r.IntN(len(texts))])
		q, err := x.Div(y)
		if err != nil {
			continue
		}
		down, err1 := x.DivBound(y, false)
		up, err2 := x.DivBound(y, true)
		if err1 != nil || err2 != nil {
			t.Fatalf("%s / %s: %v, %v", x, y, err1, err2)
		}
		if down.Cmp(q) > 0 || up.Cmp(q) < 0 {
			t.Fatalf("%s / %s = %s, outside [%s, %s]", x, y, q, down, up)
		}
		lo, _ := down.Mul(y)
		hi, _ := up.Mul(y)
		if y.Sign() < 0 {
			lo, hi = hi, lo
		}
		if lo.Cmp(x) > 0 || hi.Cmp(x) < 0 {
			t.Fatalf("%s / %s is not between %s and %s", x, y, down, up)
		}
	}
}
