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
