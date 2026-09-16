package decimal

import (
	"math/big"
	"math/rand/v2"
	"testing"
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
	for range 2000 {
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
