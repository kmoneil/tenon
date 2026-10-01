package ctytenon

import (
	"errors"
	"math/big"
	"strconv"
	"strings"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/gotenon"
	"github.com/zclconf/go-cty/cty"
)

// ctyPrecision is the precision of the numbers cty's parser makes, rounding
// to nearest even: the way cty reads a number from text, as HCL's are.
const ctyPrecision = 512

// maxDigits is how many digits shortest tries before giving up. 156 tell
// apart every two numbers held in 512 bits; the rest are a margin.
const maxDigits = 160

// workPrecision is the precision leadingDigits works in. Its result, about
// 10^165, is off by less than 10^-64, so every digit it gives is the number's
// own but where the number lies that close to a place it rounds at, and
// shortest asks cty's parser of every candidate anyway.
const workPrecision = 768

// The binary exponents, as MantExp gives them, beyond which a number has a
// digit outside tenon's window (NU-016), whatever decimal stands for it:
// 2^3321929 exceeds 10^1000000, and a number below 2^-3321926 is less than
// half of 10^-999999, as is every decimal cty's parser reads as it.
const (
	maxBinaryExponent = 3321929
	minBinaryExponent = -3321925
)

// maxParsedText is the length of text beyond which numberToCty does not hand
// a number's text to cty's parser, which reads digits in time the square of
// their number. It is the length tenon itself reads (NU-024).
const maxParsedText = 10000

// numberFromCty returns the tenon number of f, a cty number. A number held in
// 512 bits or fewer, as every number cty's parser makes is, is the decimal of
// the fewest digits that cty's parser reads as f, so that HCL's 0.1 is 0.1
// and crosses back as the number it was. A number held in more bits is f's
// exact value, which a binary fraction always has, and so is one that no
// decimal of up to maxDigits digits is read as, or whose decimal lies outside
// tenon's window where f does not: cty reads 1e1000000 as the integer just
// below it. Either zero is zero.
//
// An infinity is an error value with code CodeEncodeNotANumber, and a number
// with a digit outside tenon's window one with code CodeNumberOutOfRange,
// decided from f's binary exponent before any work in proportion to it.
func numberFromCty(f *big.Float) tenon.Value {
	switch {
	case f.IsInf():
		return tenon.ErrorVal(tenon.Diagnostic{Code: tenon.CodeEncodeNotANumber, Message: f.String() + " is not a number"})
	case f.Sign() == 0:
		return tenon.NumberFromInt(0)
	}
	exp := f.MantExp(nil)
	if exp > maxBinaryExponent || exp < minBinaryExponent {
		return tenon.ErrorVal(tenon.Diagnostic{Code: tenon.CodeNumberOutOfRange, Message: "the number is outside the range of numbers"})
	}
	if f.MinPrec() <= ctyPrecision {
		// An integer below 2^508 has at most 153 digits, and no other
		// decimal of so few is read as the same 512 bits.
		if f.IsInt() && exp <= 508 {
			i, _ := f.Int(nil)
			return tenon.NumberFromBigInt(i)
		}
		if digits, e, ok := shortest(f); ok {
			if n := tenon.NumberFromText(digits + "e" + strconv.Itoa(e)); !n.IsError() {
				return n
			}
		}
	}
	v, err := gotenon.Encode(f)
	if e := (*tenon.Error)(nil); errors.As(err, &e) {
		return e.Value()
	}
	return v
}

// shortest returns the digits and exponent, digits × 10^exp, of the decimal of
// the fewest digits that cty's parser reads as f, which is finite and not
// zero, and false where no decimal of up to maxDigits digits is read so. Of
// two such decimals of as many digits, it gives the nearer to f.
//
// Fewer digits are read as f wherever more are, so it searches the count by
// halves, asking cty's parser of the nearest decimal of each count tried.
func shortest(f *big.Float) (string, int, bool) {
	abs := new(big.Float).Abs(f)
	digits, exp, more := leadingDigits(abs)
	reads := func(n int) bool {
		d, e := roundDigits(digits, exp, more, n)
		v, err := cty.ParseNumberVal(d + "e" + strconv.Itoa(e))
		return err == nil && v.AsBigFloat().Cmp(abs) == 0
	}
	if !reads(maxDigits) {
		return "", 0, false
	}
	lo, hi := 1, maxDigits
	for lo < hi {
		if mid := (lo + hi) / 2; reads(mid) {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	d, e := roundDigits(digits, exp, more, lo)
	if f.Sign() < 0 {
		d = "-" + d
	}
	return d, e, true
}

// leadingDigits returns the first maxDigits+1 digits of f, which is
// positive, as digits × 10^exp, and whether any digit after them is not zero:
// one digit more than shortest keeps, so that each count it tries rounds on
// a digit.
func leadingDigits(f *big.Float) (digits string, exp int, more bool) {
	// f lies in [2^(e2-1), 2^e2), so the place of its first digit is the
	// floor of (e2-1)×log10(2) or one above it; asking for a few digits more
	// covers both, and the rounding of log10(2) too.
	e2 := int64(f.MantExp(nil))
	first := floorDiv((e2-1)*301029995664, 1000000000000)
	exp = int(first) - maxDigits - 3
	scaled := new(big.Float).SetPrec(workPrecision)
	if exp <= 0 {
		scaled.Mul(f, pow10(-exp))
	} else {
		scaled.Quo(f, pow10(exp))
	}
	i, acc := scaled.Int(nil)
	s := i.String()
	keep := maxDigits + 1
	more = acc != big.Exact || strings.TrimRight(s[keep:], "0") != ""
	return s[:keep], exp + len(s) - keep, more
}

// roundDigits returns digits × 10^exp, which more says is followed by digits
// not zero, rounded to n digits to nearest even, as digits and an exponent,
// without the trailing zeros.
func roundDigits(digits string, exp int, more bool, n int) (string, int) {
	head, tail := []byte(digits[:n]), digits[n:]
	exp += len(tail)
	up := tail[0] > '5' || tail[0] == '5' && (more || strings.TrimRight(tail[1:], "0") != "" || (head[n-1]-'0')%2 == 1)
	if up {
		i := n - 1
		for ; i >= 0 && head[i] == '9'; i-- {
			head[i] = '0'
		}
		if i < 0 {
			head = append([]byte{'1'}, head[:n-1]...)
			exp++
		} else {
			head[i]++
		}
	}
	s := strings.TrimRight(string(head), "0")
	return s, exp + len(head) - len(s)
}

// pow10 returns 10^n at workPrecision.
func pow10(n int) *big.Float {
	z := new(big.Float).SetPrec(workPrecision).SetInt64(1)
	b := new(big.Float).SetPrec(workPrecision).SetInt64(10)
	for ; n > 0; n >>= 1 {
		if n&1 == 1 {
			z.Mul(z, b)
		}
		b.Mul(b, b)
	}
	return z
}

// floorDiv returns a/b rounded toward negative infinity, b being positive.
func floorDiv(a, b int64) int64 {
	q := a / b
	if a%b < 0 {
		q--
	}
	return q
}

// numberToCty returns the cty number of v, a known Number: the number cty's
// parser reads from v's text, as it reads one of HCL's. Where the text is
// longer than maxParsedText, which that parser would read in time the square
// of its length, it is v's exact value rounded to 512 bits to nearest even:
// the number cty's parser gives wherever it rounds correctly.
func numberToCty(v tenon.Value) cty.Value {
	if text := v.String(); len(text) <= maxParsedText {
		if n, err := cty.ParseNumberVal(text); err == nil {
			return n
		}
	}
	return cty.NumberVal(new(big.Float).SetPrec(ctyPrecision).SetMode(big.ToNearestEven).SetRat(v.AsBigRat()))
}
