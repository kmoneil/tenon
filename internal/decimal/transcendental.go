package decimal

import (
	"math/big"
	"sync"
)

// This file computes logarithms and powers of numbers, which are irrational
// almost everywhere, correctly rounded: the result is the number of
// DivisionPrecision significant digits nearest the exact real value, ties to
// the even digit, as Div rounds a quotient. Nothing here uses binary floating
// point. Each function encloses the exact value in an interval of decimals
// computed at a working precision, every operation rounding its lower end
// down and its upper end up, so the interval holds the exact value however
// the errors fall; when both ends round to one number at DivisionPrecision
// digits, that number is the answer, and otherwise the working precision
// doubles and the enclosure is computed again (Ziv's strategy). The loop ends
// for every value that is not itself a rounding boundary, a midpoint; a
// logarithm never is one, and Pow finds the powers that might be before it
// computes.

// fl is a working number of the computations here: m × 10^e, its exponent
// unbounded, so that an intermediate below or above the window of a Dec is
// held all the same. The zero fl has a nil m and stands for zero.
type fl struct {
	m *big.Int
	e int64
}

// flInt returns v as a working number.
func flInt(v int64) fl { return fl{big.NewInt(v), 0} }

// flDec returns d as a working number, exactly.
func flDec(d Dec) fl {
	if d.Sign() == 0 {
		return fl{}
	}
	return fl{d.scaledCoefficient(0), d.exp}
}

func (a fl) sign() int {
	if a.m == nil {
		return 0
	}
	return a.m.Sign()
}

func (a fl) neg() fl {
	if a.sign() == 0 {
		return a
	}
	return fl{new(big.Int).Neg(a.m), a.e}
}

// top returns the exponent of a's leading digit. a is not zero.
func (a fl) top() int64 { return a.e + digitCount(new(big.Int).Abs(a.m)) - 1 }

// round returns a rounded to p significant digits, toward positive infinity
// where up is set and toward negative infinity where it is not.
func (a fl) round(p int64, up bool) fl {
	if a.sign() == 0 {
		return a
	}
	mag := new(big.Int).Abs(a.m)
	n := digitCount(mag)
	if n <= p {
		return a
	}
	k := n - p
	q, r := new(big.Int).QuoRem(mag, pow10(k), new(big.Int))
	// Toward positive infinity rounds a positive magnitude away from zero
	// and a negative one toward it.
	if r.Sign() != 0 && up == (a.m.Sign() > 0) {
		q.Add(q, bigOne)
	}
	if a.m.Sign() < 0 {
		q.Neg(q)
	}
	return fl{q, a.e + k}
}

// halfEven returns a rounded to p significant digits, half to even.
func (a fl) halfEven(p int64) fl {
	if a.sign() == 0 {
		return a
	}
	mag := new(big.Int).Abs(a.m)
	n := digitCount(mag)
	if n <= p {
		return a
	}
	k := n - p
	den := pow10(k)
	q, r := new(big.Int).QuoRem(mag, den, new(big.Int))
	if c := r.Lsh(r, 1).Cmp(den); c > 0 || c == 0 && q.Bit(0) == 1 {
		q.Add(q, bigOne)
	}
	if a.m.Sign() < 0 {
		q.Neg(q)
	}
	return fl{q, a.e + k}
}

// equal reports whether a and b are the same number.
func (a fl) equal(b fl) bool { return a.cmp(b) == 0 }

// cmp compares a and b.
func (a fl) cmp(b fl) int {
	switch sa, sb := a.sign(), b.sign(); {
	case sa != sb:
		if sa < sb {
			return -1
		}
		return 1
	case sa == 0:
		return 0
	}
	x, y := new(big.Int).Set(a.m), new(big.Int).Set(b.m)
	if a.e > b.e {
		x.Mul(x, pow10(a.e-b.e))
	} else if b.e > a.e {
		y.Mul(y, pow10(b.e-a.e))
	}
	return x.Cmp(y)
}

// exact returns a + b exactly. The operands are short, as every working
// number here is, so aligning them costs little.
func exactAdd(a, b fl) fl {
	switch {
	case a.sign() == 0:
		return b
	case b.sign() == 0:
		return a
	}
	if a.e < b.e {
		a, b = b, a
	}
	m := new(big.Int).Mul(a.m, pow10(a.e-b.e))
	return fl{m.Add(m, b.m), b.e}
}

// add returns a + b rounded to p digits in the direction up says. Where b
// lies below every digit of a that rounding to p digits can reach, it is
// replaced by half a unit of a digit below them, which rounds the same in
// either direction, so the sum is not carried out to b's last digit.
func add(a, b fl, p int64, up bool) fl {
	switch {
	case a.sign() == 0:
		return b.round(p, up)
	case b.sign() == 0:
		return a.round(p, up)
	}
	if a.top() < b.top() {
		a, b = b, a
	}
	floor := a.top() - p - 3
	if b.top() < floor && a.e >= floor {
		m := new(big.Int).Mul(a.m, pow10(a.e-floor+1))
		m.Add(m, big.NewInt(int64(5*b.sign())))
		return fl{m, floor - 1}.round(p, up)
	}
	return exactAdd(a, b).round(p, up)
}

// mul returns a × b rounded to p digits in the direction up says.
func mul(a, b fl, p int64, up bool) fl {
	if a.sign() == 0 || b.sign() == 0 {
		return fl{}
	}
	return fl{new(big.Int).Mul(a.m, b.m), a.e + b.e}.round(p, up)
}

// div returns a / b rounded to p digits in the direction up says. b is not
// zero.
func div(a, b fl, p int64, up bool) fl {
	if a.sign() == 0 {
		return fl{}
	}
	x, y := new(big.Int).Abs(a.m), new(big.Int).Abs(b.m)
	// Scale the dividend so the integer quotient has more than p digits;
	// a remainder then counts as half a unit of one digit further down.
	k := p + 2 - (digitCount(x) - digitCount(y))
	if k < 0 {
		k = 0
	}
	x.Mul(x, pow10(k))
	q, r := new(big.Int).QuoRem(x, y, new(big.Int))
	q.Mul(q, big.NewInt(10))
	if r.Sign() != 0 {
		q.Add(q, big.NewInt(5))
	}
	if a.m.Sign() != b.m.Sign() {
		q.Neg(q)
	}
	return fl{q, a.e - b.e - k - 1}.round(p, up)
}

// interval is a pair of working numbers enclosing a value: lo at most it,
// hi at least it.
type interval struct{ lo, hi fl }

// point returns the interval of the one value a, rounded outward to p digits.
func point(a fl, p int64) interval { return interval{a.round(p, false), a.round(p, true)} }

func (x interval) neg() interval { return interval{x.hi.neg(), x.lo.neg()} }

func (x interval) add(y interval, p int64) interval {
	return interval{add(x.lo, y.lo, p, false), add(x.hi, y.hi, p, true)}
}

// mul returns the product of x and y, whatever their signs.
func (x interval) mul(y interval, p int64) interval {
	los := []fl{mul(x.lo, y.lo, p, false), mul(x.lo, y.hi, p, false), mul(x.hi, y.lo, p, false), mul(x.hi, y.hi, p, false)}
	his := []fl{mul(x.lo, y.lo, p, true), mul(x.lo, y.hi, p, true), mul(x.hi, y.lo, p, true), mul(x.hi, y.hi, p, true)}
	lo, hi := los[0], his[0]
	for i := 1; i < 4; i++ {
		if los[i].cmp(lo) < 0 {
			lo = los[i]
		}
		if his[i].cmp(hi) > 0 {
			hi = his[i]
		}
	}
	return interval{lo, hi}
}

// scale returns x times the integer k.
func (x interval) scale(k int64, p int64) interval { return x.mul(point(flInt(k), p), p) }

// atanh returns an enclosure of atanh(s) for |s| <= 1/2, by its series
// s + s^3/3 + s^5/5 + ..., to p digits. For s of either sign the series is
// summed for |s|, whose terms are all positive: the partial sum is a lower
// bound, and adding the first term left out over 1 - s² bounds what is left.
func atanh(s fl, p int64) interval {
	if s.sign() < 0 {
		return atanh(s.neg(), p).neg()
	}
	if s.sign() == 0 {
		return interval{}
	}
	q := p + 5
	s2lo, s2hi := mul(s, s, q, false), mul(s, s, q, true)
	powLo, powHi := s.round(q, false), s.round(q, true) // s^(2k+1)
	sumLo, sumHi := powLo, powHi
	for k := int64(1); ; k++ {
		powLo, powHi = mul(powLo, s2lo, q, false), mul(powHi, s2hi, q, true)
		termLo := div(powLo, flInt(2*k+1), q, false)
		termHi := div(powHi, flInt(2*k+1), q, true)
		sumLo, sumHi = add(sumLo, termLo, q, false), add(sumHi, termHi, q, true)
		if termHi.sign() == 0 || termHi.top() < sumLo.top()-q-2 {
			// What is left is at most the next term over 1 - s², and the
			// next term is less than this one.
			oneMinus := add(flInt(1), s2hi.neg(), q, false)
			tail := div(termHi, oneMinus, q, true)
			return interval{sumLo.round(p, false), add(sumHi, tail, p, true)}
		}
	}
}

// constants holds ln 2 and ln 10 enclosed to the most digits asked so far.
var constants struct {
	sync.Mutex
	p         int64
	ln2, ln10 interval
}

// logConstants returns enclosures of ln 2 and ln 10 to p digits: ln 2 is
// 2 atanh(1/3), and ln 10 is 3 ln 2 + ln(5/4), ln(5/4) being 2 atanh(1/9).
func logConstants(p int64) (ln2, ln10 interval) {
	constants.Lock()
	defer constants.Unlock()
	if constants.p < p {
		q := p + 5
		third := interval{div(flInt(1), flInt(3), q, false), div(flInt(1), flInt(3), q, true)}
		ninth := interval{div(flInt(1), flInt(9), q, false), div(flInt(1), flInt(9), q, true)}
		ln2 := interval{atanh(third.lo, q).lo, atanh(third.hi, q).hi}.scale(2, q)
		ln54 := interval{atanh(ninth.lo, q).lo, atanh(ninth.hi, q).hi}.scale(2, q)
		constants.p, constants.ln2, constants.ln10 = p, ln2, ln2.scale(3, q).add(ln54, q)
	}
	return constants.ln2, constants.ln10
}

// lnNearOne returns an enclosure of ln(1 + d) to p digits, d being exact and
// 1 + d between 1/2 and 2: 2 atanh(d / (2 + d)), which increases with d.
// Taking d rather than 1 + d keeps the digits of a number next to 1.
func lnNearOne(d fl, p int64) interval {
	q := p + 5
	dlo, dhi := d.round(q, false), d.round(q, true)
	slo := div(dlo, add(flInt(2), dlo, q, true), q, false)
	shi := div(dhi, add(flInt(2), dhi, q, false), q, true)
	if dlo.sign() < 0 {
		slo = div(dlo, add(flInt(2), dlo, q, false), q, false)
	}
	if dhi.sign() < 0 {
		shi = div(dhi, add(flInt(2), dhi, q, true), q, true)
	}
	return interval{atanh(slo, q).lo, atanh(shi, q).hi}.scale(2, p)
}

// ln returns an enclosure of the natural logarithm of x, which is positive,
// to p digits.
func ln(x Dec, p int64) interval {
	one := FromInt64(1)
	half, _ := FromInt64Parts(5, -1)
	two := FromInt64(2)
	if x.Cmp(half) >= 0 && x.Cmp(two) <= 0 {
		d, _ := x.Sub(one)
		return lnNearOne(flDec(d), p)
	}
	// x = z × 2^j × 10^a with z between 3/4 and 3/2, all exact: dividing a
	// decimal by 2 is multiplying it by 5 and moving its point.
	a := x.adjustedExponent()
	z := flDec(x)
	z.e -= a // z is between 1 and 10
	j := int64(0)
	for z.cmp(fl{big.NewInt(15), -1}) >= 0 {
		z = fl{new(big.Int).Mul(z.m, big.NewInt(5)), z.e - 1}
		j++
	}
	d := exactAdd(z, flInt(-1))
	// The constants are needed to the digits of j and a beyond p, since
	// their errors grow with them.
	q := p + digits64(abs64(a)) + 5
	ln2, ln10 := logConstants(q)
	r := lnNearOne(d, q).add(ln2.scale(j, q), q).add(ln10.scale(a, q), q)
	return interval{r.lo.round(p, false), r.hi.round(p, true)}
}

// exp returns an enclosure of e^t for t in the interval given, to p digits.
func exp(t interval, p int64) interval {
	// e^t = 10^k × e^r, r = t - k ln 10 within about ln 10 / 2 of zero, so
	// that the series converges fast whatever t is.
	_, ln10 := logConstants(30)
	k := int64(0)
	if t.lo.sign() != 0 {
		guess := div(t.lo, ln10.lo, 25, false).halfEven(25)
		k = guess.integer()
	}
	q := p + digits64(abs64(k)) + 5
	_, ln10 = logConstants(q)
	r := t.add(ln10.scale(k, q).neg(), q)
	e := interval{expPoint(r.lo, q, false), expPoint(r.hi, q, true)}
	e.lo.e += k
	e.hi.e += k
	return interval{e.lo.round(p, false), e.hi.round(p, true)}
}

// expPoint returns e^v rounded to p digits, down or up as up says.
func expPoint(v fl, p int64, up bool) fl {
	if v.sign() < 0 {
		// e^v = 1 / e^-v; a bound on e^-v the other way bounds e^v this way.
		return div(flInt(1), expPoint(v.neg(), p+2, !up), p, up)
	}
	if v.sign() == 0 {
		return flInt(1)
	}
	// Halve v until it is below 1/100, sum e^u by its series, and square the
	// sum back as often: each squaring doubles a relative error, so the
	// working precision carries the digits it costs.
	halvings := int64(0)
	u := v
	hundredth := fl{bigOne, -2}
	for u.cmp(hundredth) >= 0 {
		u = fl{new(big.Int).Mul(u.m, big.NewInt(5)), u.e - 1}
		halvings++
	}
	q := p + halvings/3 + 6
	u = u.round(q, up)
	sum, term := flInt(1), flInt(1)
	for n := int64(1); ; n++ {
		term = div(mul(term, u, q, up), flInt(n), q, up)
		sum = add(sum, term, q, up)
		if term.sign() == 0 || term.top() < -q-2 {
			if up {
				// The terms left sum to less than twice the next, which is
				// less than this one, u being small.
				sum = add(sum, mul(term, flInt(2), q, true), q, true)
			}
			break
		}
	}
	for range halvings {
		sum = mul(sum, sum, q, up)
	}
	return sum.round(p, up)
}

// integer returns the integer a, which has no fractional digits, as an int64.
func (a fl) integer() int64 {
	if a.sign() == 0 {
		return 0
	}
	m := new(big.Int).Set(a.m)
	if a.e > 0 {
		m.Mul(m, pow10(a.e))
	} else if a.e < 0 {
		m.Quo(m, pow10(-a.e))
	}
	return m.Int64()
}

// adjustedExponent returns the exponent of the leading digit of d, which is
// not zero.
func (d Dec) adjustedExponent() int64 {
	return d.exp + digitCount(new(big.Int).Abs(d.scaledCoefficient(0))) - 1
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// maxWorkingPrecision bounds the working precision of correctlyRounded. No
// value that is not a rounding midpoint needs more than a few doublings past
// the first; reaching it means a midpoint got through, which is a defect,
// reported as ErrUnsettled for the caller to treat as one.
const maxWorkingPrecision = 1 << 16

// correctlyRounded returns the value that enclose encloses, rounded to
// DivisionPrecision significant digits half to even, ErrOutOfRange where
// that number lies outside the window, and ErrUnsettled where the working
// precision reached its bound without the ends settling.
func correctlyRounded(enclose func(p int64) interval) (Dec, error) {
	if d, err, ok := settle(enclose, DivisionPrecision+24, maxWorkingPrecision); ok {
		return d, err
	}
	return Dec{}, ErrUnsettled
}

// settle runs Ziv's loop from the working precision from, doubling it up to
// to, and reports whether the ends settled.
func settle(enclose func(p int64) interval, from, to int64) (Dec, error, bool) {
	for p := from; p <= to; p *= 2 {
		x := enclose(p)
		lo, hi := x.lo.halfEven(DivisionPrecision), x.hi.halfEven(DivisionPrecision)
		if lo.equal(hi) {
			d, err := lo.dec()
			return d, err, true
		}
	}
	return Dec{}, nil, false
}

// dec returns a as a Dec, or ErrOutOfRange where it lies outside the window.
func (a fl) dec() (Dec, error) {
	if a.sign() == 0 {
		return Dec{}, nil
	}
	if a.top() > MaxAdjustedExponent || a.top() < -MaxAdjustedExponent {
		return Dec{}, ErrOutOfRange
	}
	return FromParts(a.m, a.e)
}

// Ln returns the natural logarithm of x, which is positive, correctly rounded
// to DivisionPrecision significant digits, half to even.
func Ln(x Dec) (Dec, error) {
	return correctlyRounded(func(p int64) interval { return ln(x, p) })
}

// Exp returns e^x correctly rounded to DivisionPrecision significant digits,
// half to even, and ErrOutOfRange where it lies outside the window. x is
// read to at most a few million in magnitude: beyond that the result is out
// of range, and the caller decides so first.
func Exp(x Dec) (Dec, error) {
	t := flDec(x)
	return correctlyRounded(func(p int64) interval { return exp(point(t, p+10), p) })
}

// LogBase returns the logarithm of x to the base b, both positive and b not
// 1, correctly rounded to DivisionPrecision significant digits, half to even.
func LogBase(x, b Dec) (Dec, error) {
	return correctlyRounded(func(p int64) interval {
		q := p + 5
		num, den := ln(x, q), ln(b, q)
		return quotient(num, den, p)
	})
}

// quotient returns an enclosure of x / y, y holding no zero.
func quotient(x, y interval, p int64) interval {
	los := []fl{div(x.lo, y.lo, p, false), div(x.lo, y.hi, p, false), div(x.hi, y.lo, p, false), div(x.hi, y.hi, p, false)}
	his := []fl{div(x.lo, y.lo, p, true), div(x.lo, y.hi, p, true), div(x.hi, y.lo, p, true), div(x.hi, y.hi, p, true)}
	lo, hi := los[0], his[0]
	for i := 1; i < 4; i++ {
		if los[i].cmp(lo) < 0 {
			lo = los[i]
		}
		if his[i].cmp(hi) > 0 {
			hi = his[i]
		}
	}
	return interval{lo, hi}
}

// PowPositive returns x^y for x positive, correctly rounded to
// DivisionPrecision significant digits, half to even, and ErrOutOfRange where
// it lies outside the window: e^(y ln x).
//
// A result far outside the window is decided first, from y ln x enclosed to
// a few digits, so that pow(2, 1e400) costs what that does. And a result
// that is itself a rounding midpoint, a terminating decimal of
// DivisionPrecision + 1 digits ending in 5 as 5^138 is, would never settle:
// a power still unsettled after a few doublings is tested for an exact
// result, which is then rounded as it is.
func PowPositive(x, y Dec) (Dec, error) {
	ty := flDec(y)
	t := ln(x, 30).mul(point(ty, 40), 30)
	if limit := (fl{big.NewInt(2303000), 0}); t.lo.cmp(limit) > 0 || t.hi.cmp(limit.neg()) < 0 {
		// e^t passes 10^1000163 or falls below its reciprocal, outside the
		// window whatever the digits t has beyond these.
		return Dec{}, ErrOutOfRange
	}
	enclose := func(p int64) interval {
		// The exponent y ln x is read to p digits beyond its integer part,
		// whose digits grow the error of e^(y ln x).
		q := p + 12
		t := ln(x, q).mul(point(ty, q+10), q)
		return exp(t, p)
	}
	if d, err, ok := settle(enclose, DivisionPrecision+24, 1000); ok {
		return d, err
	}
	if r, ok := exactPower(x, y); ok {
		return r.halfEven(DivisionPrecision).dec()
	}
	return correctlyRounded(enclose)
}

// exactDigits bounds the coefficient of an exact power exactPower computes:
// a midpoint has DivisionPrecision + 1 digits, so a power of more cannot be
// one, and the loop settles it.
const exactDigits = 2 * DivisionPrecision

// exactPower returns x^y exactly where it is a terminating decimal of at
// most exactDigits significant digits, and false otherwise. With y = p/q in
// lowest terms, q dividing a power of ten since y terminates, x = m × 10^e
// (m not a multiple of 10) is a rational q-th power exactly where q divides
// e and m is a perfect q-th power: were q not to divide e, the powers of 2
// and of 5 in x could not both be multiples of q, m holding at most one of
// them.
func exactPower(x, y Dec) (fl, bool) {
	m, e := x.scaledCoefficient(0), x.exp
	pNum, yExp := y.scaledCoefficient(0), y.exp
	q := big.NewInt(1)
	if yExp >= 0 {
		pNum.Mul(pNum, pow10(yExp))
	} else {
		// pow10 hands out values it keeps, so q is a copy to divide.
		q = new(big.Int).Set(pow10(-yExp))
		g := new(big.Int).GCD(nil, nil, new(big.Int).Abs(pNum), q)
		pNum.Quo(pNum, g)
		q.Quo(q, g)
	}
	if !pNum.IsInt64() || !q.IsInt64() {
		return fl{}, false
	}
	p, qq := pNum.Int64(), q.Int64()
	if e%qq != 0 || digitCount(m)/qq*abs64(p) > exactDigits {
		return fl{}, false
	}
	r, ok := intRoot(m, qq)
	if !ok {
		return fl{}, false
	}
	// x^y = (r × 10^(e/q))^p.
	if digitCount(r)*abs64(p) > exactDigits+1 {
		return fl{}, false
	}
	rp := new(big.Int).Exp(r, big.NewInt(abs64(p)), nil)
	f := e / qq * p
	if p >= 0 {
		return fl{rp, f}, true
	}
	// A negative power terminates exactly where r^|p| divides a power of ten.
	quo, shift := exactQuotient(big.NewInt(1), rp)
	if quo == nil {
		return fl{}, false
	}
	return fl{quo, f - shift}, true
}

// intRoot returns the q-th root of m, positive, where it is an integer.
func intRoot(m *big.Int, q int64) (*big.Int, bool) {
	if q == 1 {
		return m, true
	}
	if int64(m.BitLen()) < q {
		return m, m.Cmp(bigOne) == 0
	}
	// Newton's iteration from above: r = ((q-1) r + m / r^(q-1)) / q
	// decreases to the floor of the root.
	r := new(big.Int).Lsh(bigOne, uint((int64(m.BitLen())+q-1)/q))
	qm1, qb := big.NewInt(q-1), big.NewInt(q)
	for {
		rq1 := new(big.Int).Exp(r, qm1, nil)
		next := new(big.Int).Quo(m, rq1)
		next.Add(next, new(big.Int).Mul(qm1, r))
		next.Quo(next, qb)
		if next.Cmp(r) >= 0 {
			break
		}
		r = next
	}
	return r, new(big.Int).Exp(r, qb, nil).Cmp(m) == 0
}
