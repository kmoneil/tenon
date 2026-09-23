// Package fixture holds benchmarks for the growth command's tests: one pair
// whose allocation grows with its size and one whose allocation grows as the
// square of it.
package fixture

import (
	"strconv"
	"testing"
)

// sink keeps what the benchmarks allocate from being optimized away.
var sink []byte

// BenchmarkLinear allocates its size.
func BenchmarkLinear(b *testing.B) {
	for _, n := range []int{1000, 4000} {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			for b.Loop() {
				sink = make([]byte, n)
			}
		})
	}
}

// BenchmarkSquare allocates the square of its size.
func BenchmarkSquare(b *testing.B) {
	for _, n := range []int{100, 400} {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			for b.Loop() {
				sink = make([]byte, n*n)
			}
		})
	}
}
