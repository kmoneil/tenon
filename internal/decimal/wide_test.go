package decimal

import (
	"math/big"
	"testing"
)

// BenchmarkWideCoefficients measures arithmetic on numbers whose digits span
// much of the window, which a few bytes of input can ask for: a sum of numbers
// of very different magnitude, comparing two such sums, making a number from
// such a coefficient, and squaring one, which the window refuses.
func BenchmarkWideCoefficients(b *testing.B) {
	one := FromInt64(1)
	tiny, _ := Parse("1e-999999")
	wide, _ := one.Add(tiny)
	wider, _ := one.Add(Dec{small: 2, exp: -999999})
	odd := new(big.Int).Add(pow10(999999), big.NewInt(1))
	b.Run("add", func(b *testing.B) {
		for b.Loop() {
			_, _ = one.Add(tiny)
		}
	})
	b.Run("cmp", func(b *testing.B) {
		for b.Loop() {
			_ = wide.Cmp(wider)
		}
	})
	b.Run("fromBig", func(b *testing.B) {
		for b.Loop() {
			_, _ = fromBig(new(big.Int).Set(odd), -999999)
		}
	})
	b.Run("square", func(b *testing.B) {
		for b.Loop() {
			_, _ = wide.Mul(wide)
		}
	})
}
