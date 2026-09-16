// Package decimal implements the exact decimal numbers of tenon's Number type.
// A number is a coefficient times a power of ten, held without rounding.
package decimal

import (
	"math/big"
	"strconv"
	"strings"
)

// MaxAdjustedExponent bounds the places of the digits of every non-zero
// number, which lie in a window from 10^MaxAdjustedExponent down to
// 10^-MaxAdjustedExponent: the leading digit no higher, which bounds the
// adjusted exponent, the power of ten of that digit, and the last digit no
// lower. A coefficient therefore has fewer digits than twice
// MaxAdjustedExponent, however a number was made.
const MaxAdjustedExponent = 999999

// Error is an error reported by this package. Its values are constants, which
// callers compare errors against.
type Error uint8

const (
	// ErrSyntax reports text that is not the text of a number.
	ErrSyntax Error = iota + 1
	// ErrOutOfRange reports a number with a digit outside the window.
	ErrOutOfRange
	// ErrDivideByZero reports a division by zero.
	ErrDivideByZero
	// ErrModuloByZero reports a remainder with a zero divisor.
	ErrModuloByZero
)

func (e Error) Error() string {
	switch e {
	case ErrSyntax:
		return "decimal: invalid number syntax"
	case ErrOutOfRange:
		return "decimal: number out of range"
	case ErrDivideByZero:
		return "decimal: division by zero"
	case ErrModuloByZero:
		return "decimal: modulo by zero"
	}
	return "decimal: error " + strconv.Itoa(int(e))
}

// Dec is an exact decimal number: a coefficient times a power of ten. Every
// number has exactly one Dec form, so how a number was made is never
// observable:
//
//   - the coefficient has no trailing zero digit, and zero has exponent 0;
//   - the coefficient is held in small when it fits in an int64, and in big
//     only when it does not;
//   - every digit of a non-zero number lies within the window that
//     MaxAdjustedExponent bounds.
//
// The zero Dec is the number 0. A Dec is an immutable value.
type Dec struct {
	big   *big.Int // the coefficient, when it does not fit in an int64; never modified
	small int64    // the coefficient, when big is nil
	exp   int64    // the exponent
}

// FromInt64 returns the number v.
func FromInt64(v int64) Dec {
	d, _ := fromSmall(v, 0) // an int64 has at most 19 digits, so it is in range
	return d
}

// FromBigInt returns the number v, or ErrOutOfRange if v has more digits than
// the range allows. It does not retain v.
func FromBigInt(v *big.Int) (Dec, error) {
	if v.IsInt64() {
		return FromInt64(v.Int64()), nil
	}
	return fromBig(new(big.Int).Set(v), 0)
}

// fromSmall returns the Dec for c × 10^exp.
func fromSmall(c, exp int64) (Dec, error) {
	if c == 0 {
		return Dec{}, nil
	}
	for c%10 == 0 {
		c /= 10
		exp++
	}
	if !inRange(exp, digits64(c)) {
		return Dec{}, ErrOutOfRange
	}
	return Dec{small: c, exp: exp}, nil
}

// fromBig returns the Dec for c × 10^exp, taking ownership of c.
func fromBig(c *big.Int, exp int64) (Dec, error) {
	if c.IsInt64() {
		return fromSmall(c.Int64(), exp)
	}
	// The bit length of c brackets its digit count. Stripping trailing zeros
	// leaves the leading digit where it is, so the bracket rejects a number
	// whose leading digit is above the window, or whose every digit is below
	// it, before anything costly.
	lo, hi := digitBounds(c)
	if exp+lo-1 > MaxAdjustedExponent || exp+hi-1 < -MaxAdjustedExponent {
		return Dec{}, ErrOutOfRange
	}
	// A coefficient that is odd, or no multiple of ten, has no trailing zero to
	// strip, so exp is already its last digit's place. Most coefficients are
	// like that, and they need no conversion to text; the digits are counted
	// only when the leading digit could be at the top edge of the window.
	if !multipleOfTen(c) {
		if exp < -MaxAdjustedExponent || exp+hi-1 > MaxAdjustedExponent && exp+digitCount(c)-1 > MaxAdjustedExponent {
			return Dec{}, ErrOutOfRange
		}
		return Dec{big: c, exp: exp}, nil
	}
	digits := c.Text(10)
	if c.Sign() < 0 {
		digits = digits[1:]
	}
	trimmed := strings.TrimRight(digits, "0")
	zeros := len(digits) - len(trimmed)
	exp += int64(zeros)
	if !inRange(exp, int64(len(trimmed))) {
		return Dec{}, ErrOutOfRange
	}
	if zeros > 0 {
		c.Quo(c, pow10(int64(zeros)))
		if c.IsInt64() {
			return Dec{small: c.Int64(), exp: exp}, nil
		}
	}
	return Dec{big: c, exp: exp}, nil
}

// inRange reports whether a non-zero number whose coefficient has the given
// count of digits, and no trailing zero, and whose exponent is exp, has every
// digit within the window: the last digit at the 10^exp place, and the leading
// digit at the place of the adjusted exponent.
func inRange(exp, digits int64) bool {
	return exp >= -MaxAdjustedExponent && exp+digits-1 <= MaxAdjustedExponent
}

// digits64 returns the number of decimal digits of c, which is not zero.
func digits64(c int64) int64 {
	u := uint64(c)
	if c < 0 {
		u = -u
	}
	n := int64(1)
	for u >= 10 {
		u /= 10
		n++
	}
	return n
}

// pow10 returns a new big.Int holding 10^n.
func pow10(n int64) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(n), nil)
}

// expLimit is the magnitude at which a parsed exponent saturates. An exponent
// that large puts any number far out of range whatever its digits, since no
// text has anywhere near that many digits.
const expLimit = 1 << 62

// Parse returns the number that s denotes. s must consist of an optional "-",
// one or more ASCII digits, optionally a "." and one or more digits, and
// optionally an exponent: "e" or "E", an optional "+" or "-", and one or more
// digits. Parse returns ErrSyntax if s has any other form, and ErrOutOfRange
// if the number is out of range. The number is exact, never rounded.
func Parse(s string) (Dec, error) {
	rest := s
	neg := strings.HasPrefix(rest, "-")
	if neg {
		rest = rest[1:]
	}
	intPart, rest := leadingDigits(rest)
	if intPart == "" {
		return Dec{}, ErrSyntax
	}
	var frac string
	if strings.HasPrefix(rest, ".") {
		if frac, rest = leadingDigits(rest[1:]); frac == "" {
			return Dec{}, ErrSyntax
		}
	}
	var exp int64
	if rest != "" && (rest[0] == 'e' || rest[0] == 'E') {
		rest = rest[1:]
		expNeg := false
		if rest != "" && (rest[0] == '+' || rest[0] == '-') {
			expNeg = rest[0] == '-'
			rest = rest[1:]
		}
		var expDigits string
		if expDigits, rest = leadingDigits(rest); expDigits == "" {
			return Dec{}, ErrSyntax
		}
		if exp = saturatingInt(expDigits); expNeg {
			exp = -exp
		}
	}
	if rest != "" {
		return Dec{}, ErrSyntax
	}

	// The coefficient is the integer and fraction digits run together; each
	// fraction digit lowers the exponent by one.
	digits := strings.TrimLeft(intPart+frac, "0")
	trimmed := strings.TrimRight(digits, "0")
	if trimmed == "" {
		return Dec{}, nil
	}
	exp += int64(len(digits)-len(trimmed)) - int64(len(frac))
	if !inRange(exp, int64(len(trimmed))) {
		return Dec{}, ErrOutOfRange
	}
	if len(trimmed) <= 18 {
		c, _ := strconv.ParseInt(trimmed, 10, 64) // 18 digits always fit
		if neg {
			c = -c
		}
		return Dec{small: c, exp: exp}, nil
	}
	c, _ := new(big.Int).SetString(trimmed, 10) // trimmed holds only digits
	if neg {
		c.Neg(c)
	}
	if c.IsInt64() {
		return Dec{small: c.Int64(), exp: exp}, nil
	}
	return Dec{big: c, exp: exp}, nil
}

// leadingDigits splits s after its leading ASCII digits.
func leadingDigits(s string) (digits, rest string) {
	i := 0
	for i < len(s) && '0' <= s[i] && s[i] <= '9' {
		i++
	}
	return s[:i], s[i:]
}

// saturatingInt returns the value of a string of ASCII digits, or expLimit if
// that is less.
func saturatingInt(digits string) int64 {
	var n int64
	for i := 0; i < len(digits); i++ {
		if n > (expLimit-9)/10 {
			return expLimit
		}
		n = n*10 + int64(digits[i]-'0')
	}
	return n
}

// Sign returns -1, 0 or +1 as d is negative, zero or positive.
func (d Dec) Sign() int {
	if d.big != nil {
		return d.big.Sign()
	}
	switch {
	case d.small < 0:
		return -1
	case d.small > 0:
		return 1
	}
	return 0
}

// Equal reports whether d and e are the same number.
func (d Dec) Equal(e Dec) bool {
	if d.exp != e.exp || (d.big == nil) != (e.big == nil) {
		return false
	}
	if d.big == nil {
		return d.small == e.small
	}
	return d.big.Cmp(e.big) == 0
}

// coefficientDigits returns the decimal digits of the magnitude of the
// coefficient of d, which must not be zero.
func (d Dec) coefficientDigits() string {
	if d.big != nil {
		return strings.TrimPrefix(d.big.Text(10), "-")
	}
	u := uint64(d.small)
	if d.small < 0 {
		u = -u
	}
	return strconv.FormatUint(u, 10)
}

// String returns the canonical text of d: positional notation when the
// adjusted exponent is between -20 and 20, and scientific notation otherwise.
func (d Dec) String() string {
	if d.Sign() == 0 {
		return "0"
	}
	digits := d.coefficientDigits()
	n := int64(len(digits))
	adj := d.exp + n - 1
	var b strings.Builder
	if d.Sign() < 0 {
		b.WriteByte('-')
	}
	switch {
	case adj > 20 || adj < -20:
		b.WriteString(digits[:1])
		if n > 1 {
			b.WriteByte('.')
			b.WriteString(digits[1:])
		}
		b.WriteByte('e')
		b.WriteString(strconv.FormatInt(adj, 10))
	case d.exp >= 0:
		b.WriteString(digits)
		b.WriteString(strings.Repeat("0", int(d.exp)))
	case adj >= 0:
		b.WriteString(digits[:adj+1])
		b.WriteByte('.')
		b.WriteString(digits[adj+1:])
	default:
		b.WriteString("0.")
		b.WriteString(strings.Repeat("0", int(-adj-1)))
		b.WriteString(digits)
	}
	return b.String()
}
