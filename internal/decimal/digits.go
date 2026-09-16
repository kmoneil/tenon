package decimal

import (
	"math/big"
	"math/bits"
)

// log10Of2 is log10(2) in 64-bit fixed point, rounded down, so that log10(2)
// lies between log10Of2 and log10Of2+1 divided by 2^64.
const log10Of2 = 0x4D104D427DE7FBCC

// log10Pow2 returns floor(b × log10(2)) for b >= 0, which is one less than the
// number of digits of 2^b. It multiplies b by log10(2) rounded down and rounded
// up; where the two agree, which they do for every bit length a coefficient
// can have, that is the answer, and where an integer falls between them it is
// decided exactly.
func log10Pow2(b int64) int64 {
	lo, _ := bits.Mul64(uint64(b), log10Of2)
	hi, _ := bits.Mul64(uint64(b), log10Of2+1)
	if lo == hi {
		return int64(lo)
	}
	// 10^hi is at most 2^b exactly when it has at most b bits, since no power
	// of ten above one is a power of two.
	if pow10(int64(hi)).BitLen() <= int(b) {
		return int64(hi)
	}
	return int64(lo)
}

// digitBounds returns the fewest and the most decimal digits that the
// magnitude of c, which is not zero, can have for its bit length. They are
// equal, or one apart.
func digitBounds(c *big.Int) (lo, hi int64) {
	b := int64(c.BitLen())
	return log10Pow2(b-1) + 1, log10Pow2(b) + 1
}

// digitCount returns the number of decimal digits of the magnitude of c, which
// is not zero, without rendering c as text: its bit length leaves one count or
// two neighbouring ones, and a comparison with the power of ten between them
// decides.
func digitCount(c *big.Int) int64 {
	lo, hi := digitBounds(c)
	if lo == hi || c.CmpAbs(pow10(lo)) < 0 {
		return lo
	}
	return hi
}

// twos returns how many factors of two the coefficient of d holds, which is
// at least how many trailing decimal zeros any multiple of it can gain from it.
func (d Dec) twos() int64 {
	if d.big != nil {
		return int64(d.big.TrailingZeroBits())
	}
	return int64(bits.TrailingZeros64(uint64(d.small)))
}

// multipleOfTen reports whether c is a multiple of ten, without dividing it.
// It must be even, and a multiple of five: each word of c stands for a power
// of 2^64, or of 2^32 where words are that wide, and either leaves remainder
// one when divided by five, so c leaves the remainder its words' sum does.
func multipleOfTen(c *big.Int) bool {
	if c.TrailingZeroBits() == 0 {
		return false
	}
	var r uint64
	for _, w := range c.Bits() {
		r += uint64(w) % 5
	}
	return r%5 == 0
}
