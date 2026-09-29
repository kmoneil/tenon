package tenon

import "slices"

// A set holds one value per member, so a set holding members that are not
// known can be a given set of values only if its members can take those
// values, each its own. That is a matching between the values and the members,
// and it decides three questions: whether a set holding unknowns could be a
// known set (EQ-003), whether the values a listing names leave any set at all
// (UN-004), and, where a narrowing leaves room for nothing else, which set it
// leaves (UN-005).

// couldEqual reports whether the set u, which holds members that are not all
// known, could turn out to be the known set k. Every member of u must be able
// to be a member of k, since a member of u outside k would make the two sets
// differ, and every member of k must be one that some member of u can be, each
// its own, since one member is one value.
//
// A known member of u can be one value of k only, the one it equals, so those
// are paired by hash first and only the members that are not known are
// matched with the values left, where pairing every value with every member
// compared each known member with every value: 8,000 took 380 ms. Pairing a
// known member with the value it equals loses no matching: one that gave the
// value another member instead can swap the two, freeing that member, and the
// known member could be no other value.
func couldEqual(k, u *node) bool {
	values, held := k.data.([]Value), u.data.([]Value)
	if len(held) < len(values) {
		return false
	}
	byHash := make(map[uint64][]int, len(values))
	for i, v := range values {
		h := hashNode(v.n)
		byHash[h] = append(byHash[h], i)
	}
	paired := make([]bool, len(values))
	known := knownMembers(held)
	for _, m := range held[:known] {
		found := false
		for _, i := range byHash[hashNode(m.n)] {
			if eq, _ := equality(values[i].n, m.n); eq {
				paired[i], found = true, true
				break
			}
		}
		if !found {
			return false
		}
	}
	var left []Value
	for i, v := range values {
		if !paired[i] {
			left = append(left, v)
		}
	}
	rest := held[known:]
	edges, covered := valueEdges(left, rest)
	for i, some := range covered {
		// A member that could be none of the values left must still be able
		// to be one of those paired, as two members of u may turn out one.
		if !some && !slices.ContainsFunc(values, func(v Value) bool {
			eq, settled := equality(v.n, rest[i].n)
			return !settled || eq
		}) {
			return false
		}
	}
	return saturates(edges, len(rest))
}

// valueEdges returns, for each of these values, the members that could turn
// out to be it, and, for each member, whether it could be any of them.
func valueEdges(values, members []Value) (edges [][]int, covered []bool) {
	edges, covered = make([][]int, len(values)), make([]bool, len(members))
	for j, v := range values {
		for i, m := range members {
			if eq, settled := equality(v.n, m.n); !settled || eq {
				edges[j] = append(edges[j], i)
				covered[i] = true
			}
		}
	}
	return edges, covered
}

// saturates reports whether every left vertex can be given a right vertex of
// its own: a matching that covers the left side. edges[i] holds the right
// vertices that left vertex i may take, and rights is how many there are.
//
// It is Hopcroft and Karp's algorithm, which augments along the shortest paths
// it can find, all of them in one pass. Its work is the edges times the square
// root of the vertices, so the members of the sets being compared bound it,
// and it never costs more than reading every pair of them did.
func saturates(edges [][]int, rights int) bool {
	const unmatched = -1
	if len(edges) > rights {
		return false
	}
	left, right := make([]int, len(edges)), make([]int, rights)
	for i := range left {
		left[i] = unmatched
	}
	for j := range right {
		right[j] = unmatched
	}
	// layer holds how many edges from a free left vertex each left vertex is,
	// or unreached where this pass does not reach it.
	const unreached = -1
	layer := make([]int, len(edges))
	queue := make([]int, 0, len(edges))
	matched := 0
	for matched < len(edges) {
		// Lay out the left vertices by their distance from a free one, and
		// stop the pass as soon as no free right vertex is in reach.
		queue = queue[:0]
		for i := range left {
			if left[i] == unmatched {
				layer[i] = 0
				queue = append(queue, i)
			} else {
				layer[i] = unreached
			}
		}
		free := false
		for k := 0; k < len(queue); k++ {
			i := queue[k]
			for _, j := range edges[i] {
				switch taken := right[j]; {
				case taken == unmatched:
					free = true
				case layer[taken] == unreached:
					layer[taken] = layer[i] + 1
					queue = append(queue, taken)
				}
			}
		}
		if !free {
			return false
		}
		for i := range left {
			if left[i] == unmatched && augment(edges, left, right, layer, i) {
				matched++
			}
		}
	}
	return true
}

// augment walks from the free left vertex i along the layers, looking for a
// right vertex that no left vertex has taken. Where it finds one it hands each
// right vertex on the way to the left vertex before it, which leaves one more
// left vertex matched. A left vertex it fails from leads nowhere this pass,
// and is taken out of the layers so that no other walk tries it again.
func augment(edges [][]int, left, right, layer []int, i int) bool {
	const unmatched, unreached = -1, -1
	for _, j := range edges[i] {
		taken := right[j]
		if taken == unmatched || layer[taken] == layer[i]+1 && augment(edges, left, right, layer, taken) {
			left[i], right[j] = j, i
			return true
		}
	}
	layer[i] = unreached
	return false
}
