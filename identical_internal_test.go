package tenon

import (
	"math/rand"
	"slices"
	"testing"

	"github.com/kmoneil/tenon/internal/conformance"
)

// TestSameMultisetIsTheCount holds the walk that compares two sets' members to
// the count it replaced, over sets drawn with repeats from members of every
// sort a set orders: known ones, ones that are not known, and ones told apart
// only by a capsule value whose type declares no encoding, which encode alike
// and are ordered by the capsule values. The walk is sound only because a set
// holds its members in one order, which this holds it to as well.
func TestSameMultisetIsTheCount(t *testing.T) {
	r := rand.New(rand.NewSource(1618))
	ranked := Capsule("ranked", CapsuleOps[int]{
		Equals:  func(a, b *int) bool { return *a == *b },
		Hash:    func(v *int) uint64 { return uint64(*v) },
		Compare: func(a, b *int) int { return *a - *b },
	})
	num := Type{numberType}
	elem := Tuple(ranked, num)
	var pool []Value
	for k := range 4 {
		rank := func() Value { v := k; return CapsuleVal(ranked, &v) }
		pool = append(pool,
			TupleVal(rank(), NumberFromInt(int64(k))),
			TupleVal(rank(), Unknown(num)),
			TupleVal(rank(), Narrow(Unknown(num), NumberMin(NumberFromInt(int64(k)), true))),
		)
	}
	pick := func() []Value {
		out := make([]Value, r.Intn(12))
		for i := range out {
			out[i] = pool[r.Intn(len(pool))]
		}
		return out
	}
	alike := 0
	for range conformance.Iterations(t, 2000) {
		xs := pick()
		var ys []Value
		switch r.Intn(3) {
		case 0:
			ys = pick()
		default:
			// The same members in another order, one of them perhaps
			// replaced.
			ys = slices.Clone(xs)
			r.Shuffle(len(ys), func(i, j int) { ys[i], ys[j] = ys[j], ys[i] })
			if len(ys) > 0 && r.Intn(2) == 0 {
				ys[r.Intn(len(ys))] = pool[r.Intn(len(pool))]
			}
		}
		x, y := SetVal(elem, xs...).n.data.([]Value), SetVal(elem, ys...).n.data.([]Value)
		want := countedMultiset(x, y)
		if want {
			alike++
		}
		if got := sameMultiset(x, y); got != want {
			t.Fatalf("sameMultiset(%v, %v) = %t, where counting says %t", x, y, got, want)
		}
	}
	if alike < 200 {
		t.Errorf("only %d pairs held the same members", alike)
	}
}

// countedMultiset is sameMultiset as it was: the known members walked, and
// the rest each counted in both.
func countedMultiset(x, y []Value) bool {
	if len(x) != len(y) {
		return false
	}
	kx, ky := knownMembers(x), knownMembers(y)
	if kx != ky {
		return false
	}
	for i := range kx {
		if !Identical(x[i], y[i]) {
			return false
		}
	}
	x, y = x[kx:], y[ky:]
	count := func(members []Value, m Value) int {
		n := 0
		for _, k := range members {
			if Identical(k, m) {
				n++
			}
		}
		return n
	}
	for _, m := range x {
		if count(x, m) != count(y, m) {
			return false
		}
	}
	return true
}
