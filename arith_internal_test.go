package tenon

import (
	"math/rand"
	"testing"

	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/internal/decimal"
)

// TestOperationBoundsAreSound holds the bounds the arithmetic and the order
// read from their operands to the one thing [UN-007] forbids: a range that
// excludes a possible outcome. Operands are random ranges of numbers, bounded
// or not on either side, inclusive or not, and each is sampled at points it
// holds, its bounds among them where it holds them and points just inside
// where it does not. Every outcome the operation gives for the points must lie
// in the range it gives for the ranges, and an answer the ranges settle must
// be the answer every pair of points gives.
func TestOperationBoundsAreSound(t *testing.T) {
	r := rand.New(rand.NewSource(1337))
	arithmetic := []struct {
		name string
		op   func(a, b Value) Value
	}{{"Add", Add}, {"Sub", Sub}, {"Mul", Mul}, {"Div", Div}, {"Mod", Mod}}
	for i := range conformance.Iterations(t, 400) {
		a, as := randomRange(t, r)
		b, bs := randomRange(t, r)
		for _, o := range arithmetic {
			bounded := o.op(a, b)
			if bounded.IsError() {
				if a.IsKnown() && b.IsKnown() {
					continue // one pair of points, and an error for it
				}
				t.Fatalf("case %d: %s(%v, %v) = %v, where an operand is a range", i, o.name, a, b, bounded)
			}
			for _, x := range as {
				for _, y := range bs {
					exact := o.op(numberValue(x), numberValue(y))
					if exact.IsError() {
						// Division by zero and a result beyond the digit window
						// are errors, which appear once the value is known and
						// are no outcome a range holds.
						continue
					}
					if !holdsNumber(bounded, decOf(exact)) {
						t.Fatalf("case %d: %s(%v, %v) = %v, which excludes %s(%s, %s) = %v",
							i, o.name, a, b, bounded, o.name, x, y, exact)
					}
				}
			}
		}
		settled := LessThan(a, b)
		if !settled.IsKnown() {
			continue
		}
		for _, x := range as {
			for _, y := range bs {
				if exact := LessThan(numberValue(x), numberValue(y)); !Identical(exact, settled) {
					t.Fatalf("case %d: LessThan(%v, %v) = %v, but LessThan(%s, %s) = %v",
						i, a, b, settled, x, y, exact)
				}
			}
		}
	}
}

// randomRange returns a Number that is not null, known or a range, and points
// that it holds: its bounds where it holds them, points just inside them
// where it does not, points between them, and points far out along a side it
// leaves unbounded.
func randomRange(t *testing.T, r *rand.Rand) (Value, []decimal.Dec) {
	t.Helper()
	dec := func(s string) decimal.Dec {
		d, err := decimal.Parse(s)
		if err != nil {
			t.Fatalf("decimal.Parse(%q): %v", s, err)
		}
		return d
	}
	add := func(a, b decimal.Dec) decimal.Dec {
		d, err := a.Add(b)
		if err != nil {
			t.Fatalf("%s + %s: %v", a, b, err)
		}
		return d
	}
	endpoints := []string{"-5", "-2", "-1", "-0.5", "0", "0.25", "1", "2", "3", "7", "1e40"}
	pick := func() decimal.Dec { return dec(endpoints[r.Intn(len(endpoints))]) }
	var lo, hi bound
	if r.Intn(4) > 0 {
		lo = bound{v: pick(), incl: r.Intn(2) == 0, set: true}
	}
	if r.Intn(4) > 0 {
		hi = bound{v: pick(), incl: r.Intn(2) == 0, set: true}
	}
	if lo.set && hi.set {
		switch c := lo.v.Cmp(hi.v); {
		case c > 0:
			lo, hi = hi, lo
		case c == 0:
			lo.incl, hi.incl = true, true // a single value, which the range becomes
		}
	}
	ns := []Narrowing{NotNull()}
	if lo.set {
		ns = append(ns, NumberMin(numberValue(lo.v), lo.incl))
	}
	if hi.set {
		ns = append(ns, NumberMax(numberValue(hi.v), hi.incl))
	}
	v := Narrow(Unknown(NumberType()), ns...)

	var candidates []decimal.Dec
	// 1e-100 is a hundred digits in, beyond the 96 a quotient is rounded to,
	// so that a point the range holds can round onto a bound it excludes.
	near := []string{"1e-100", "0.001", "0.125", "1"}
	for _, d := range near {
		if lo.set {
			candidates = append(candidates, add(lo.v, dec(d)))
		}
		if hi.set {
			candidates = append(candidates, add(hi.v, dec("-"+d)))
		}
	}
	if lo.set {
		candidates = append(candidates, lo.v, add(lo.v, dec("1e30")))
	}
	if hi.set {
		candidates = append(candidates, hi.v, add(hi.v, dec("-1e30")))
	}
	if lo.set && hi.set {
		mid, _ := add(lo.v, hi.v).Div(dec("2"))
		candidates = append(candidates, mid)
	}
	for _, d := range []string{"0", "1", "-1", "3", "-3", "1e30", "-1e30"} {
		candidates = append(candidates, dec(d))
	}
	var points []decimal.Dec
	for _, c := range candidates {
		if within(c, lo, hi) {
			points = append(points, c)
		}
	}
	return v, points
}

// within reports whether d satisfies both bounds.
func within(d decimal.Dec, lo, hi bound) bool {
	if lo.set {
		if c := d.Cmp(lo.v); c < 0 || c == 0 && !lo.incl {
			return false
		}
	}
	if hi.set {
		if c := d.Cmp(hi.v); c > 0 || c == 0 && !hi.incl {
			return false
		}
	}
	return true
}

// holdsNumber reports whether the Number v, known or a range, holds d.
func holdsNumber(v Value, d decimal.Dec) bool {
	switch n := v.n; n.state {
	case stateKnown:
		return decOf(v).Equal(d)
	case stateUnknown:
		rd := n.data.(*rangeData)
		return within(d, rd.lo, rd.hi)
	}
	return false
}

// TestOrderByPrefixIsSound holds LessThan to what the prefixes of strings
// settle, and to nothing more, where the pitfall is normalization: a prefix
// as it was given need not begin every value, since what follows it may
// compose with its last character, and the order of a composed character is
// not the order of the letter it came from. Prefixes and the strings that
// follow them are drawn from letters, marks that compose with them, and
// punctuation that nothing composes with.
func TestOrderByPrefixIsSound(t *testing.T) {
	r := rand.New(rand.NewSource(1338))
	letters := []string{"a", "b", "c", "e", "z", "-", "."}
	marks := []string{"", "a", "-", "\U00000301", "\U00000327", "e\U00000301", "z"}
	word := func(from []string, n int) string {
		var b []byte
		for range n {
			b = append(b, from[r.Intn(len(from))]...)
		}
		return string(b)
	}
	// operand returns a String, a range of strings with a prefix or a known
	// one, and strings it holds.
	operand := func() (Value, []Value) {
		given := word(letters, 1+r.Intn(3))
		if r.Intn(3) == 0 {
			v := String(given + word(marks, r.Intn(2)))
			return v, []Value{v}
		}
		v := Narrow(Unknown(StringType()), NotNull(), StringPrefix(given))
		var held []Value
		for range 6 {
			held = append(held, String(given+word(marks, r.Intn(3))))
		}
		return v, held
	}
	for i := range conformance.Iterations(t, 2000) {
		a, as := operand()
		b, bs := operand()
		settled := LessThan(a, b)
		if !settled.IsKnown() {
			continue
		}
		for _, x := range as {
			for _, y := range bs {
				if exact := LessThan(x, y); !Identical(exact, settled) {
					t.Fatalf("case %d: LessThan(%v, %v) = %v, but LessThan(%v, %v) = %v", i, a, b, settled, x, y, exact)
				}
			}
		}
	}
}
