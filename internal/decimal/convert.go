package decimal

import "math/big"

// Rat returns d as a new exact rational number.
func (d Dec) Rat() *big.Rat {
	r := new(big.Rat).SetInt(d.scaledCoefficient(0))
	switch {
	case d.exp > 0:
		r.Mul(r, new(big.Rat).SetInt(pow10(d.exp)))
	case d.exp < 0:
		r.Quo(r, new(big.Rat).SetInt(pow10(-d.exp)))
	}
	return r
}

// Int64 returns d and true if d is an integer that fits in an int64, and 0 and
// false otherwise.
func (d Dec) Int64() (int64, bool) {
	if d.big != nil || d.exp < 0 {
		return 0, false
	}
	return scaleSmall(d.small, d.exp)
}

// BigInt returns d as a new big.Int and true if d is an integer, and nil and
// false otherwise. A fraction is refused without arithmetic, however small.
func (d Dec) BigInt() (*big.Int, bool) {
	if d.exp < 0 {
		return nil, false
	}
	return d.scaledCoefficient(d.exp), true
}

// Parts returns the coefficient and exponent of d, which is coefficient ×
// 10^exp. The coefficient is in small where big is nil, and has no trailing
// zero unless d is zero, whose parts are all zero. big must not be modified.
func (d Dec) Parts() (small int64, big *big.Int, exp int64) {
	return d.small, d.big, d.exp
}

// FromParts returns the number c × 10^exp, or ErrOutOfRange if it has a digit
// outside the window. It does not retain c. An exponent far outside the window
// is refused without arithmetic, so no input can make it compute a large
// power of ten.
func FromParts(c *big.Int, exp int64) (Dec, error) {
	if c.Sign() == 0 {
		return Dec{}, nil
	}
	// Stripping trailing zeros only raises the exponent, so a coefficient's
	// own exponent above the window puts its leading digit above it too; and
	// no coefficient has more trailing zeros than a third of its bits.
	if exp > MaxAdjustedExponent || exp < -MaxAdjustedExponent-int64(c.BitLen()/3+1) {
		return Dec{}, ErrOutOfRange
	}
	return fromBig(new(big.Int).Set(c), exp)
}

// FromInt64Parts returns the number c × 10^exp, as FromParts does, for a
// coefficient that is an int64: the same Dec or the same error, without a
// big.Int.
func FromInt64Parts(c, exp int64) (Dec, error) {
	if c == 0 {
		return Dec{}, nil
	}
	// An exponent above the window puts the leading digit above it, and one
	// further below the window than an int64 has trailing zeros, eighteen,
	// leaves a digit below it however many are stripped. Both are refused
	// before the exponent takes part in a sum, which near the top of an int64
	// would overflow and could read as in range.
	if exp > MaxAdjustedExponent || exp < -MaxAdjustedExponent-18 {
		return Dec{}, ErrOutOfRange
	}
	return fromSmall(c, exp)
}
