package decimal

import (
	"cmp"
	"hash/maphash"
	"math"
	"math/big"
	"math/rand/v2"
	"testing"

	"github.com/kmoneil/tenon/internal/conformance"
)

// hashOf returns the hash under seed of the numbers written in turn.
func hashOf(seed maphash.Seed, ds ...Dec) uint64 {
	var h maphash.Hash
	h.SetSeed(seed)
	for _, d := range ds {
		d.WriteHash(&h)
	}
	return h.Sum64()
}

func TestCmp(t *testing.T) {
	// These numbers are in increasing order.
	ordered := []string{
		"-1e999999", "-123456789012345678901234567890", "-9223372036854775809", "-9223372036854775808",
		"-1000", "-999.999", "-1", "-0.5", "-1e-999999",
		"0",
		"1e-999999", "0.000001", "0.00001", "0.5", "1", "1.25", "1.5",
		"9223372036854775807", "9223372036854775808",
		"1e30", "1.0000000000000000000000000000001e30", "1e999999",
	}
	for i, x := range ordered {
		for j, y := range ordered {
			if got, want := mustParse(t, x).Cmp(mustParse(t, y)), cmp.Compare(i, j); got != want {
				t.Errorf("Cmp(%s, %s) = %d, want %d", x, y, got, want)
			}
		}
	}

	// Random pairs, some of them equal, agree with big.Rat.
	rng := rand.New(rand.NewPCG(24, 24))
	for range conformance.Iterations(t, 5000) {
		a, b := randomNumber(t, rng), randomNumber(t, rng)
		if rng.IntN(5) == 0 {
			b = mustParse(t, a.String())
		}
		want := toRat(a).Cmp(toRat(b))
		if got := a.Cmp(b); got != want || b.Cmp(a) != -want || (got == 0) != a.Equal(b) {
			t.Fatalf("Cmp(%s, %s) = %d and Cmp(%s, %s) = %d, but big.Rat gives %d", a, b, got, b, a, b.Cmp(a), want)
		}
	}
}

func TestHashFollowsTheNumber(t *testing.T) {
	seed := maphash.MakeSeed()
	get := noErr(t)
	n := func(s string) Dec { return mustParse(t, s) }
	two63 := get(FromBigInt(new(big.Int).Lsh(big.NewInt(1), 63)))
	ten30 := get(FromBigInt(pow10(30)))

	// Every construction of a number hashes alike.
	for _, group := range [][]Dec{
		{Dec{}, FromInt64(0), n("-0.000e7"), get(n("5").Sub(n("5")))},
		{FromInt64(1200), n("1.2e3"), n("0001200.00"), get(n("12").Mul(n("100")))},
		{FromInt64(math.MinInt64), n("-9.223372036854775808e18")},
		{two63, n("9223372036854775808"), get(FromInt64(math.MaxInt64).Add(FromInt64(1)))},
		{ten30, n("1e30"), get(n("1e15").Mul(n("1e15")))},
		{n("0.1"), get(FromInt64(1).Div(FromInt64(10)))},
	} {
		for _, d := range group[1:] {
			if hashOf(seed, d) != hashOf(seed, group[0]) {
				t.Errorf("%s and %s hash differently", d, group[0])
			}
		}
	}

	// Numbers reached by different paths hash alike, and distinct numbers
	// hash differently.
	rng := rand.New(rand.NewPCG(25, 25))
	seen := map[uint64]Dec{}
	for range conformance.Iterations(t, 10000) {
		a, b := randomNumber(t, rng), randomNumber(t, rng)
		if back := get(get(a.Add(b)).Sub(b)); hashOf(seed, back) != hashOf(seed, a) {
			t.Fatalf("%s + %s - %s hashes differently from %s", a, b, b, a)
		}
		h := hashOf(seed, a)
		if prev, ok := seen[h]; ok && !prev.Equal(a) {
			t.Fatalf("%s and %s have the same hash", prev, a)
		}
		seen[h] = a
	}

	// Sequences of numbers keep their boundaries.
	big1, big2 := n("123456789012345678901234567890"), n("-98765432109876543210987654321")
	for _, pair := range [][2][]Dec{
		{{n("1"), n("23")}, {n("12"), n("3")}},
		{{Dec{}, n("1")}, {n("1"), Dec{}}},
		{{big1, big2}, {big2, big1}},
		{{big1, Dec{}}, {Dec{}, big1}},
	} {
		if hashOf(seed, pair[0]...) == hashOf(seed, pair[1]...) {
			t.Errorf("the sequences %v and %v hash alike", pair[0], pair[1])
		}
	}
}
