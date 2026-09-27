package decimal

import (
	"math/big"
	"math/rand/v2"
	"testing"

	"github.com/kmoneil/tenon/internal/conformance"
)

// TestLog10Pow2 holds the fixed-point floor of b times log10(2) to the exact
// answer, which is one less than the number of digits of 2^b.
func TestLog10Pow2(t *testing.T) {
	check := func(b int64) {
		t.Helper()
		want := int64(len(new(big.Int).Lsh(big.NewInt(1), uint(b)).Text(10))) - 1
		if got := log10Pow2(b); got != want {
			t.Fatalf("log10Pow2(%d) = %d, want %d", b, got, want)
		}
	}
	for b := int64(0); b <= 5000; b++ {
		check(b)
	}
	rng := rand.New(rand.NewPCG(7, 7))
	for range 40 {
		check(5000 + rng.Int64N(200000))
	}
}

// TestDigitCountWithoutText holds the digit count taken from a coefficient's
// bit length to the length of its decimal text, at and around every kind of
// boundary: powers of ten, powers of two, and random numbers of every size.
func TestDigitCountWithoutText(t *testing.T) {
	want := func(c *big.Int) int64 { return int64(len(new(big.Int).Abs(c).Text(10))) }
	var cases []*big.Int
	one := big.NewInt(1)
	for k := int64(1); k <= 700; k++ {
		p := pow10(k)
		cases = append(cases, new(big.Int).Sub(p, one), p, new(big.Int).Add(p, one))
	}
	for b := uint(1); b <= 2400; b++ {
		p := new(big.Int).Lsh(one, b)
		cases = append(cases, new(big.Int).Sub(p, one), p, new(big.Int).Add(p, one))
	}
	rng := rand.New(rand.NewPCG(8, 8))
	for range conformance.Iterations(t, 2000) {
		bitLen := 1 + rng.IntN(4000)
		buf := make([]byte, (bitLen+7)/8)
		for i := range buf {
			buf[i] = byte(rng.Uint32())
		}
		c := new(big.Int).SetBytes(buf)
		c.Rsh(c, uint(len(buf)*8-bitLen))
		if c.Sign() == 0 {
			continue
		}
		cases = append(cases, c)
	}
	ten := big.NewInt(10)
	for _, c := range cases {
		for _, v := range []*big.Int{c, new(big.Int).Neg(c), new(big.Int).Mul(c, ten)} {
			if got, want := multipleOfTen(v), new(big.Int).Rem(v, ten).Sign() == 0; got != want {
				t.Fatalf("multipleOfTen of a %d-bit coefficient is %t, want %t", v.BitLen(), got, want)
			}
			lo, hi := digitBounds(v)
			exact := digitCount(v)
			if w := want(v); exact != w || lo > w || hi < w || hi-lo > 1 {
				t.Fatalf("%d-bit coefficient: bounds [%d, %d], count %d, want %d", v.BitLen(), lo, hi, exact, w)
			}
		}
	}
}

// TestTrailingZerosWithoutText holds trailingZeros to the count text gives,
// for every count up to past a few powers of two, on either sign, beside
// coefficients whose trailing zero bits outnumber their zeros.
func TestTrailingZerosWithoutText(t *testing.T) {
	r := rand.New(rand.NewPCG(20260927, 1))
	textCount := func(c *big.Int) (string, int64) {
		s := new(big.Int).Abs(c).Text(10)
		n := int64(0)
		for len(s) > 1 && s[len(s)-1] == '0' {
			s, n = s[:len(s)-1], n+1
		}
		return s, n
	}
	for zeros := int64(0); zeros <= 70; zeros++ {
		for range 20 {
			c := new(big.Int).SetUint64(r.Uint64() | 1)
			c.Mul(c, new(big.Int).Exp(big.NewInt(r.Int64N(90)+2), big.NewInt(r.Int64N(40)), nil))
			for c.Bit(0) == 0 && new(big.Int).Rem(c, big.NewInt(10)).Sign() == 0 {
				c.Quo(c, big.NewInt(10))
			}
			if new(big.Int).Rem(c, big.NewInt(10)).Sign() == 0 {
				continue
			}
			c.Mul(c, new(big.Int).Exp(big.NewInt(10), big.NewInt(zeros), nil))
			if r.IntN(2) == 0 {
				c.Neg(c)
			}
			wantDigits, wantZeros := textCount(c)
			sign := c.Sign()
			got, n := trailingZeros(new(big.Int).Set(c))
			if n != wantZeros || new(big.Int).Abs(got).Text(10) != wantDigits || got.Sign() != sign {
				t.Fatalf("trailingZeros(%s) = %s, %d; want %s, %d", c, got, n, wantDigits, wantZeros)
			}
		}
	}
}

// TestPowersOfTenAreKeptAndShared holds pow10 to making each power it keeps
// once, and to the value it keeps staying 10^n after the arithmetic that
// reads it.
func TestPowersOfTenAreKeptAndShared(t *testing.T) {
	for _, n := range []int64{0, 1, 7, 64, pow10Kept - 1} {
		if a, b := pow10(n), pow10(n); a != b {
			t.Errorf("10^%d was made twice", n)
		}
	}
	if a, b := pow10(pow10Kept), pow10(pow10Kept); a == b {
		t.Errorf("10^%d, past the powers kept, was kept", pow10Kept)
	}
	x, _ := Parse("1.2345678901234567890123456789e-5")
	y, _ := Parse("98765432109876543210.123")
	for range 3 {
		x.Cmp(y)
		y.Div(x)
		x.Mul(y)
	}
	for n := range int64(200) {
		if got, want := pow10(n), new(big.Int).Exp(big.NewInt(10), big.NewInt(n), nil); got.Cmp(want) != 0 {
			t.Fatalf("the kept 10^%d reads %s", n, got)
		}
	}
}
