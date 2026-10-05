package decimal

import (
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestTranscendentalFixtures holds Ln, Exp, LogBase and PowPositive to
// values an independent decimal implementation computed at 300 digits and
// rounded to 96 (testdata/transcendental.py, which says how).
func TestTranscendentalFixtures(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "transcendental.json"))
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Cases []struct {
			Op   string   `json:"op"`
			Args []string `json:"args"`
			Want string   `json:"want"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Cases) < 40 {
		t.Fatalf("the fixtures hold %d cases", len(f.Cases))
	}
	for _, c := range f.Cases {
		args := make([]Dec, len(c.Args))
		for i, a := range c.Args {
			args[i] = mustParse(t, a)
		}
		want := mustParse(t, c.Want)
		var got Dec
		start := time.Now()
		switch c.Op {
		case "ln":
			got, err = Ln(args[0])
		case "exp":
			got, err = Exp(args[0])
		case "log":
			got, err = LogBase(args[0], args[1])
		case "pow":
			got, err = PowPositive(args[0], args[1])
		default:
			t.Fatalf("the fixtures name the operation %q", c.Op)
		}
		if err != nil || !got.Equal(want) {
			t.Errorf("%s%v = %v, %v; want %s", c.Op, c.Args, got, err, c.Want)
		}
		if d := time.Since(start); d > 2*time.Second {
			t.Errorf("%s%v took %v", c.Op, c.Args, d)
		}
	}
}

// TestRootsBracketed checks square roots without an oracle: r = x^0.5 rounded
// to 96 digits lies within half a unit of its last digit of the root, so
// (r - u/2)^2 <= x <= (r + u/2)^2, in exact integer arithmetic.
func TestRootsBracketed(t *testing.T) {
	half := mustParse(t, "0.5")
	for _, s := range []string{"2", "3", "5", "10", "0.3", "123456789", "1e-7", "7e100"} {
		x := mustParse(t, s)
		r, err := PowPositive(x, half)
		if err != nil {
			t.Fatalf("PowPositive(%s, 0.5): %v", s, err)
		}
		rr := r.Rat()
		// u is a unit in the 96th digit of r.
		u := new(big.Rat).SetFrac(big.NewInt(1), big.NewInt(1))
		top := r.adjustedExponent()
		exp := top - 95
		if exp >= 0 {
			u.SetInt(pow10(exp))
		} else {
			u.SetFrac(big.NewInt(1), pow10(-exp))
		}
		halfU := new(big.Rat).Quo(u, big.NewRat(2, 1))
		lo := new(big.Rat).Sub(rr, halfU)
		hi := new(big.Rat).Add(rr, halfU)
		lo.Mul(lo, lo)
		hi.Mul(hi, hi)
		if xr := x.Rat(); lo.Cmp(xr) > 0 || hi.Cmp(xr) < 0 {
			t.Errorf("PowPositive(%s, 0.5) = %v, whose square brackets do not hold %s", s, r, s)
		}
	}
}

// TestExactCases checks results that are exact and short: they come out
// exactly, the enclosures settling on them.
func TestExactCases(t *testing.T) {
	for _, tt := range []struct {
		got  func() (Dec, error)
		want string
	}{
		{func() (Dec, error) { return Ln(FromInt64(1)) }, "0"},
		{func() (Dec, error) { return Exp(Dec{}) }, "1"},
		{func() (Dec, error) { return LogBase(mustParse(t, "1e-400"), FromInt64(10)) }, "-400"},
		{func() (Dec, error) { return LogBase(FromInt64(1), FromInt64(7)) }, "0"},
		{func() (Dec, error) { return PowPositive(FromInt64(4), mustParse(t, "0.5")) }, "2"},
		{func() (Dec, error) { return PowPositive(mustParse(t, "2.25"), mustParse(t, "0.5")) }, "1.5"},
		{func() (Dec, error) { return PowPositive(FromInt64(10), FromInt64(-400)) }, "1e-400"},
	} {
		got, err := tt.got()
		if err != nil || !got.Equal(mustParse(t, tt.want)) {
			t.Errorf("got %v, %v; want %s", got, err, tt.want)
		}
	}
}

// TestOutOfRange checks that a result outside the window is ErrOutOfRange.
func TestTranscendentalOutOfRange(t *testing.T) {
	if _, err := Exp(FromInt64(2303000)); err != ErrOutOfRange {
		t.Errorf("Exp(2303000) gave %v, want ErrOutOfRange", err)
	}
	if _, err := Exp(FromInt64(-2303000)); err != ErrOutOfRange {
		t.Errorf("Exp(-2303000) gave %v, want ErrOutOfRange", err)
	}
}

// TestNearOne checks that a logarithm of a number next to 1 keeps its
// digits: ln(1 + 10^-99999) is 10^-99999 less half its square, to 96
// digits 10^-99999 itself.
func TestNearOne(t *testing.T) {
	tiny, _ := FromInt64Parts(1, -99999)
	x, err := FromInt64(1).Add(tiny)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	got, err := Ln(x)
	if err != nil || !got.Equal(tiny) {
		t.Errorf("Ln(1 + 1e-99999) = %v, %v; want 1e-99999", got, err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("Ln(1 + 1e-99999) took %v", d)
	}
}

// TestPowMidpoints checks the powers that are rounding midpoints, exact
// decimals of 97 digits ending in 5, which the loop alone could never
// settle: each comes out as its exact value rounded half to even, worked
// out here in integer arithmetic.
func TestPowMidpoints(t *testing.T) {
	five138 := new(big.Int).Exp(big.NewInt(5), big.NewInt(138), nil)
	if digitCount(five138) != DivisionPrecision+1 {
		t.Fatalf("5^138 has %d digits", digitCount(five138))
	}
	want, err := (fl{five138, 0}).halfEven(DivisionPrecision).dec()
	if err != nil {
		t.Fatal(err)
	}
	five276, err := FromBigInt(new(big.Int).Mul(five138, five138))
	if err != nil {
		t.Fatal(err)
	}
	wantNeg, err := (fl{five138, -138}).halfEven(DivisionPrecision).dec()
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		x, y Dec
		want Dec
	}{
		{FromInt64(5), FromInt64(138), want},
		{FromInt64(25), FromInt64(69), want},
		{five276, mustParse(t, "0.5"), want},
		{FromInt64(2), FromInt64(-138), wantNeg},
	} {
		start := time.Now()
		got, err := PowPositive(tt.x, tt.y)
		if err != nil || !got.Equal(tt.want) {
			t.Errorf("PowPositive(%v, %v) = %v, %v; want %v", tt.x, tt.y, got, err, tt.want)
		}
		if d := time.Since(start); d > 2*time.Second {
			t.Errorf("PowPositive(%v, %v) took %v", tt.x, tt.y, d)
		}
	}
}

// TestPowFarOutOfRange checks that a power far outside the window is
// decided before it is computed.
func TestPowFarOutOfRange(t *testing.T) {
	for _, tt := range [][2]string{{"2", "1e400"}, {"1.05", "-1e9"}, {"10", "1000001"}} {
		start := time.Now()
		if _, err := PowPositive(mustParse(t, tt[0]), mustParse(t, tt[1])); err != ErrOutOfRange {
			t.Errorf("PowPositive(%s, %s) gave %v, want ErrOutOfRange", tt[0], tt[1], err)
		}
		if d := time.Since(start); d > 100*time.Millisecond {
			t.Errorf("PowPositive(%s, %s) took %v to refuse", tt[0], tt[1], d)
		}
	}
}

// TestPowersOfTenKept checks that the powers of ten pow10 keeps, which it
// hands out to every caller, are still powers of ten after the powers that
// take the exact path have run: a caller dividing one in place corrupts
// every number the package works out afterwards.
func TestPowersOfTenKept(t *testing.T) {
	// Only a midpoint reaches the exact path: 5^276 to the half and 5^552
	// to the quarter are both 5^138, of 97 digits ending in 5.
	five := big.NewInt(5)
	for _, tt := range []struct {
		n int64
		y string
	}{{276, "0.5"}, {552, "0.25"}} {
		x, err := FromBigInt(new(big.Int).Exp(five, big.NewInt(tt.n), nil))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := PowPositive(x, mustParse(t, tt.y)); err != nil {
			t.Fatalf("PowPositive(5^%d, %s): %v", tt.n, tt.y, err)
		}
	}
	ten := big.NewInt(10)
	want := big.NewInt(1)
	for n := int64(0); n < 40; n++ {
		if pow10(n).Cmp(want) != 0 {
			t.Fatalf("pow10(%d) is %v", n, pow10(n))
		}
		want = new(big.Int).Mul(want, ten)
	}
}
