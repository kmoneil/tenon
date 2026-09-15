package decimal

import (
	"math"
	"math/big"
	"math/bits"
)

// DivisionPrecision is the number of significant digits to which a quotient
// that does not terminate is rounded.
const DivisionPrecision = 96

// pow10Small holds 10^0 through 10^18, the powers of ten that fit in an int64.
var pow10Small = func() (p [19]int64) {
	p[0] = 1
	for i := 1; i < len(p); i++ {
		p[i] = p[i-1] * 10
	}
	return p
}()

// Neg returns -d.
func (d Dec) Neg() Dec {
	switch {
	case d.big != nil:
		n := new(big.Int).Neg(d.big)
		if n.IsInt64() {
			return Dec{small: n.Int64(), exp: d.exp}
		}
		return Dec{big: n, exp: d.exp}
	case d.small == math.MinInt64:
		return Dec{big: new(big.Int).Neg(big.NewInt(d.small)), exp: d.exp}
	}
	return Dec{small: -d.small, exp: d.exp}
}

// Add returns d + e, exactly. It returns ErrOutOfRange if the sum is out of
// range.
func (d Dec) Add(e Dec) (Dec, error) {
	switch {
	case e.Sign() == 0:
		return d, nil
	case d.Sign() == 0:
		return e, nil
	}
	exp := min(d.exp, e.exp)
	if d.big == nil && e.big == nil {
		if x, ok := scaleSmall(d.small, d.exp-exp); ok {
			if y, ok := scaleSmall(e.small, e.exp-exp); ok {
				if sum, ok := addInt64(x, y); ok {
					return fromSmall(sum, exp)
				}
			}
		}
	}
	x := d.scaledCoefficient(d.exp - exp)
	return fromBig(x.Add(x, e.scaledCoefficient(e.exp-exp)), exp)
}

// Sub returns d - e, exactly. It returns ErrOutOfRange if the difference is out
// of range.
func (d Dec) Sub(e Dec) (Dec, error) { return d.Add(e.Neg()) }

// Mul returns d × e, exactly. It returns ErrOutOfRange if the product is out of
// range.
func (d Dec) Mul(e Dec) (Dec, error) {
	if d.Sign() == 0 || e.Sign() == 0 {
		return Dec{}, nil
	}
	if d.big == nil && e.big == nil {
		if p, ok := mulInt64(d.small, e.small); ok {
			return fromSmall(p, d.exp+e.exp)
		}
	}
	x := d.scaledCoefficient(0)
	return fromBig(x.Mul(x, e.scaledCoefficient(0)), d.exp+e.exp)
}

// Div returns d / e: exactly when the quotient terminates, and otherwise
// rounded to DivisionPrecision significant digits, half to even. It returns
// ErrDivideByZero if e is zero, and ErrOutOfRange if the quotient is out of
// range.
func (d Dec) Div(e Dec) (Dec, error) {
	if e.Sign() == 0 {
		return Dec{}, ErrDivideByZero
	}
	if d.Sign() == 0 {
		return Dec{}, nil
	}
	x, y := d.scaledCoefficient(0), e.scaledCoefficient(0)
	neg := x.Sign() != y.Sign()
	x.Abs(x)
	y.Abs(y)
	q, shift := exactQuotient(x, y)
	if q == nil {
		q, shift = roundedQuotient(x, y, DivisionPrecision)
	}
	if neg {
		q.Neg(q)
	}
	return fromBig(q, d.exp-e.exp-shift)
}

// Mod returns the remainder of d divided by e: d - e × q, where q is the exact
// quotient d/e truncated toward zero. The remainder is zero or has the sign of
// d, and is smaller in magnitude than e. Mod returns ErrModuloByZero if e is
// zero, and ErrOutOfRange if the remainder is out of range.
func (d Dec) Mod(e Dec) (Dec, error) {
	if e.Sign() == 0 {
		return Dec{}, ErrModuloByZero
	}
	if d.Sign() == 0 {
		return Dec{}, nil
	}
	// With both operands scaled to the smaller exponent, the remainder of the
	// coefficients is the remainder of the numbers.
	exp := min(d.exp, e.exp)
	if d.big == nil && e.big == nil {
		if x, ok := scaleSmall(d.small, d.exp-exp); ok {
			if y, ok := scaleSmall(e.small, e.exp-exp); ok {
				return fromSmall(x%y, exp)
			}
		}
	}
	x := d.scaledCoefficient(d.exp - exp)
	return fromBig(x.Rem(x, e.scaledCoefficient(e.exp-exp)), exp)
}

// exactQuotient returns q and k with x/y = q × 10^-k when x/y terminates, and
// a nil q when it does not. x and y are positive.
//
// x/y terminates exactly when its denominator in lowest terms is 2^a × 5^b,
// that is, when y divides x × 10^max(a, b). Both a and b are less than the bit
// length of y, so y divides x × 10^bitlen(y) exactly when x/y terminates.
func exactQuotient(x, y *big.Int) (*big.Int, int64) {
	k := int64(y.BitLen())
	num := new(big.Int).Mul(x, pow10(k))
	q, r := new(big.Int).QuoRem(num, y, new(big.Int))
	if r.Sign() != 0 {
		return nil, 0
	}
	return q, k
}

// roundedQuotient returns q and s with q × 10^-s equal to x/y rounded to prec
// significant digits, half to even. x and y are positive.
func roundedQuotient(x, y *big.Int, prec int64) (*big.Int, int64) {
	lo, hi := pow10(prec-1), pow10(prec)
	// Guess the shift that gives the quotient prec digits; the loop corrects
	// the guess until the integer quotient lies in [lo, hi).
	s := prec - (digitsLowerBound(x) - digitsLowerBound(y))
	q, r := new(big.Int), new(big.Int)
	for {
		num, den := x, y
		if s >= 0 {
			num = new(big.Int).Mul(x, pow10(s))
		} else {
			den = new(big.Int).Mul(y, pow10(-s))
		}
		q.QuoRem(num, den, r)
		switch {
		case q.Cmp(hi) >= 0:
			s--
		case q.Cmp(lo) < 0:
			s++
		default:
			if c := r.Lsh(r, 1).Cmp(den); c > 0 || (c == 0 && q.Bit(0) == 1) {
				q.Add(q, big.NewInt(1))
			}
			return q, s
		}
	}
}

// digitsLowerBound returns a lower bound, within a few digits for any
// practical size, on the number of decimal digits of v, which is positive.
func digitsLowerBound(v *big.Int) int64 {
	return int64(v.BitLen()-1)*1233>>12 + 1
}

// scaledCoefficient returns the coefficient of d times 10^k, as a new big.Int.
func (d Dec) scaledCoefficient(k int64) *big.Int {
	var c *big.Int
	if d.big != nil {
		c = new(big.Int).Set(d.big)
	} else {
		c = big.NewInt(d.small)
	}
	if k > 0 {
		c.Mul(c, pow10(k))
	}
	return c
}

// scaleSmall returns c × 10^k, for k >= 0, and whether it fits in an int64.
func scaleSmall(c, k int64) (int64, bool) {
	if k == 0 {
		return c, true
	}
	if k >= int64(len(pow10Small)) {
		return 0, false
	}
	return mulInt64(c, pow10Small[k])
}

// addInt64 returns x + y and whether the sum fits in an int64.
func addInt64(x, y int64) (int64, bool) {
	s := x + y
	return s, (s > x) == (y > 0)
}

// mulInt64 returns x × y and whether the product fits in an int64.
func mulInt64(x, y int64) (int64, bool) {
	hi, lo := bits.Mul64(absUint64(x), absUint64(y))
	neg := (x < 0) != (y < 0)
	limit := uint64(math.MaxInt64)
	if neg {
		limit++
	}
	if hi != 0 || lo > limit {
		return 0, false
	}
	if neg {
		return int64(-lo), true
	}
	return int64(lo), true
}

// absUint64 returns the magnitude of x.
func absUint64(x int64) uint64 {
	u := uint64(x)
	if x < 0 {
		u = -u
	}
	return u
}
