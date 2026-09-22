package gotenon

import (
	"math/big"
	"math/rand"
	"testing"
)

// TestValuationIsTheDivision holds the ladder that counts how often a prime
// divides a number to dividing it out one at a time, which is what it stands
// in for, and holds what it leaves to what that leaves.
func TestValuationIsTheDivision(t *testing.T) {
	// divide takes p out of d one at a time.
	divide := func(d *big.Int, p int64) (int64, *big.Int) {
		rest, div := new(big.Int).Set(d), big.NewInt(p)
		q, rem := new(big.Int), new(big.Int)
		var n int64
		for {
			if q.QuoRem(rest, div, rem); rem.Sign() != 0 {
				return n, rest
			}
			rest.Set(q)
			n++
		}
	}
	r := rand.New(rand.NewSource(20260922))
	for _, p := range []int64{2, 5, 7} {
		for _, times := range []int64{0, 1, 2, 3, 5, 6, 7, 8, 15, 16, 17, 64, 100} {
			for range 5 {
				// p^times, times a number that p does not divide.
				other := big.NewInt(r.Int63n(1000) + 1)
				for other.Sign() == 0 || new(big.Int).Mod(other, big.NewInt(p)).Sign() == 0 {
					other.Add(other, big.NewInt(1))
				}
				d := new(big.Int).Mul(new(big.Int).Exp(big.NewInt(p), big.NewInt(times), nil), other)
				wantN, wantRest := divide(d, p)
				gotN, gotRest := valuation(d, p, maxDecimalPlaces)
				if gotN != wantN || gotRest.Cmp(wantRest) != 0 {
					t.Fatalf("valuation(%d^%d × %v, %d) = %d, %v; dividing gives %d, %v",
						p, times, other, p, gotN, gotRest, wantN, wantRest)
				}
				if d.Cmp(new(big.Int).Mul(new(big.Int).Exp(big.NewInt(p), big.NewInt(gotN), nil), gotRest)) != 0 {
					t.Fatalf("%d^%d × %v is not %d^%d × %v", p, times, other, p, gotN, gotRest)
				}
			}
		}
	}
	// Past the limit the counting stops, and says so, rather than taking the
	// rest of the divisions: what is left is then whatever it had reached.
	big5 := new(big.Int).Exp(big.NewInt(5), big.NewInt(4096), nil)
	if n, _ := valuation(big5, 5, 100); n <= 100 {
		t.Errorf("counting 5^4096 with a limit of 100 gave %d, want more than the limit", n)
	}
	if n, rest := valuation(big5, 5, 1<<20); n != 4096 || rest.Cmp(big.NewInt(1)) != 0 {
		t.Errorf("counting 5^4096 gave %d, %v", n, rest)
	}
}
