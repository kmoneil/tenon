package decimal

import "math/big"

// FromRat returns the number r, exactly, or ErrInexact if r is not a
// terminating decimal, one whose denominator has no prime factor but two and
// five, and ErrOutOfRange if it has a digit outside the window. Whether r
// terminates is decided before its size, and neither costs the division of a
// denominator too large for the window. It does not retain r.
func FromRat(r *big.Rat) (Dec, error) {
	c, exp, exact, ok := ratParts(r)
	switch {
	case !exact:
		return Dec{}, ErrInexact
	case !ok:
		return Dec{}, ErrOutOfRange
	}
	return FromParts(c, exp)
}

// maxBinaryPlaces bounds how many binary places a number in the window can
// need: 2^3321929 exceeds 10^1000000.
const maxBinaryPlaces = 3321929

// maxDecimalPlaces is how many decimal places a number in the window can have:
// a digit below the 10^-MaxAdjustedExponent place lies outside it. A rational
// whose denominator is 2^a × 5^b has max(a, b) of them.
const maxDecimalPlaces = MaxAdjustedExponent

// ratParts returns r as a coefficient and an exponent, whether r is a
// terminating decimal, one whose denominator has no prime factor but two and
// five, and false for a denominator too large to be worth dividing.
func ratParts(r *big.Rat) (c *big.Int, exp int64, exact, ok bool) {
	// A denominator of more bits than the window has decimal places, however
	// it factors, puts the last digit below the window. Whether the number
	// terminates is still established first, from the bits alone, so that a
	// rational that does not is inexact at any size: the part above the
	// trailing zeros is one exactly when the bit length is one past them,
	// and it holds fives exactly when the denominator does, five being odd.
	// A denominator this large is not copied, let alone factored; one that
	// is a power of two, or holds a five, is refused for its places, as one
	// whose count passes the window below is, and anything else has a
	// factor besides two and five whatever its fives would count to.
	if den := r.Denom(); int64(den.BitLen()) > maxBinaryPlaces+1 {
		if den.BitLen() == int(den.TrailingZeroBits())+1 || remByFive(den) == 0 {
			return nil, 0, true, false
		}
		return nil, 0, false, true
	}
	den := new(big.Int).Set(r.Denom())
	twos := int64(den.TrailingZeroBits())
	den.Rsh(den, uint(twos))
	fives, rest := valuation(den, 5, maxDecimalPlaces)
	// A terminating decimal with 2^a × 5^b beneath it has max(a, b) places,
	// so past the window the last digit is below it whatever is left over,
	// and the counting stops rather than dividing the rest of the way out.
	if twos > maxDecimalPlaces || fives > maxDecimalPlaces {
		return nil, 0, true, false
	}
	if rest.Cmp(big.NewInt(1)) != 0 {
		return nil, 0, false, true
	}
	n := max(twos, fives)
	c = new(big.Int).Set(r.Num())
	c.Lsh(c, uint(n-twos))
	c.Mul(c, new(big.Int).Exp(big.NewInt(5), big.NewInt(n-fives), nil))
	return c, -n, true, true
}

// remByFive returns den modulo five without dividing: a word holds 2^32 or
// 2^64 values, either of which is one more than a multiple of five, so den is
// its words' sum modulo five, as a decimal number is its digits' sum modulo
// three. One pass, nothing allocated, which the refusal of an enormous
// denominator is held to.
func remByFive(den *big.Int) uint64 {
	var sum uint64
	for _, w := range den.Bits() {
		sum += uint64(w % 5)
	}
	return sum % 5
}

// valuation returns how many times p divides d, and what is left of d once
// they are taken out, stopping once the count passes limit, where what is
// left is what it had reached. It divides by p, then by p squared, then by
// that squared, while each divides, and then takes the smaller powers out on
// the way down, so a denominator of p^n costs the logarithm of n divisions
// rather than n of them.
func valuation(d *big.Int, p, limit int64) (int64, *big.Int) {
	rest := new(big.Int).Set(d)
	powers := []*big.Int{big.NewInt(p)}
	steps := []int64{1}
	q, rem := new(big.Int), new(big.Int)
	var n int64
	for i := 0; n <= limit; i++ {
		if q.QuoRem(rest, powers[i], rem); rem.Sign() != 0 {
			break
		}
		rest.Set(q)
		n += steps[i]
		if i+1 == len(powers) {
			powers = append(powers, new(big.Int).Mul(powers[i], powers[i]))
			steps = append(steps, 2*steps[i])
		}
	}
	// Each power above the one that stopped it is too large to divide what is
	// left, and each below it divides at most once, the powers being squares.
	for i := len(powers) - 2; i >= 0 && n <= limit; i-- {
		if q.QuoRem(rest, powers[i], rem); rem.Sign() == 0 {
			rest.Set(q)
			n += steps[i]
		}
	}
	return n, rest
}
