package tenon

import (
	"strconv"
	"testing"
)

// TestAllFoldsInCanonicalOrder holds the fold a rule makes of a list, as a
// tuple against a tuple of another length makes, to the canonical order
// CV-045 names, whatever order the rule gathered the list in. Two OneOfs of 65
// objects and a number fail as soon as the number meets the first OneOf, but
// folded as given, the OneOfs would form their pairs first and pass the bound.
func TestAllFoldsInCanonicalOrder(t *testing.T) {
	singles := func(prefix string) Constraint {
		members := make([]Constraint, 65)
		for i := range members {
			members[i] = ObjectWith(map[string]Field{prefix + strconv.Itoa(i): Required(Exactly(NumberType()))}, false)
		}
		return canonical(OneOf(members...))
	}
	a, b, n := singles("a"), singles("b"), Exactly(NumberType())
	for _, list := range [][]Constraint{{a, b, n}, {n, a, b}, {b, n, a}} {
		un := newUnifier(Safe)
		for _, c := range list {
			un.left = saturatingAdd(un.left, saturatingMul(unifyBound, un.size(c)))
		}
		if _, ok := un.all(list); ok || un.over {
			t.Errorf("folding %d constraints in the order given: ok %t, refused %t; want a failure that is no refusal", len(list), ok, un.over)
		}
	}
}
