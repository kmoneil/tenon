package tenon

import (
	"math/rand"
	"testing"
)

// TestSaturatesMatchesAnExhaustiveSearch checks the matching over small random
// graphs against trying every assignment: it must say yes exactly when the
// left vertices can each be given a right vertex of their own.
func TestSaturatesMatchesAnExhaustiveSearch(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	yes, no := 0, 0
	for range 5000 {
		left, right := 1+r.Intn(5), 1+r.Intn(5)
		edges := make([][]int, left)
		for i := range edges {
			for j := range right {
				if r.Intn(3) > 0 {
					edges[i] = append(edges[i], j)
				}
			}
		}
		want := everyLeftVertexFits(edges, 0, 0)
		if want {
			yes++
		} else {
			no++
		}
		if got := saturates(edges, right); got != want {
			t.Fatalf("saturates(%v, %d) is %t, want %t", edges, right, got, want)
		}
	}
	if yes < 100 || no < 100 {
		t.Errorf("%d graphs matched and %d did not; want some of each", yes, no)
	}
}

// everyLeftVertexFits reports whether the left vertices from i on can each be
// given a right vertex of their own, none of them one that used holds.
func everyLeftVertexFits(edges [][]int, i int, used uint) bool {
	if i == len(edges) {
		return true
	}
	for _, j := range edges[i] {
		if used&(1<<uint(j)) == 0 && everyLeftVertexFits(edges, i+1, used|1<<uint(j)) {
			return true
		}
	}
	return false
}

// TestSaturatesTakesTheLongWayRound covers a graph whose greedy matching has
// to be undone: the first left vertex must give up the only right vertex the
// last one can take.
func TestSaturatesTakesTheLongWayRound(t *testing.T) {
	// 0 may take either right vertex, 1 only the first.
	if !saturates([][]int{{0, 1}, {0}}, 2) {
		t.Error("a matching that needs an augmenting path was not found")
	}
	if saturates([][]int{{0}, {0}}, 2) {
		t.Error("two left vertices were given one right vertex between them")
	}
	if !saturates(nil, 0) {
		t.Error("nothing to match did not match")
	}
	if saturates([][]int{nil}, 1) {
		t.Error("a left vertex with no edge was matched")
	}
}
