package values

import (
	"math/rand"

	"github.com/kmoneil/tenon"
)

// RandomConstraint returns a constraint of every kind, nested up to depth,
// with capsule among the types it may hold. Unify's, Convert's and
// ctytenon's property tests draw on it, so that what one is held to over
// these constraints the others are too.
func RandomConstraint(r *rand.Rand, depth int, capsule tenon.Type) tenon.Constraint {
	num, str := tenon.NumberType(), tenon.StringType()
	leaves := []tenon.Constraint{
		tenon.Any(), tenon.Exactly(num), tenon.Exactly(str), tenon.Exactly(tenon.BoolType()),
		tenon.Exactly(tenon.ListType(num)), tenon.Exactly(tenon.TupleType(num, str)),
		tenon.Exactly(tenon.ObjectType(map[string]tenon.Type{"a": num})), tenon.Exactly(tenon.MapType(str)),
		tenon.Exactly(capsule), tenon.OneOf(),
	}
	if depth == 0 || r.Intn(3) == 0 {
		return leaves[r.Intn(len(leaves))]
	}
	child := func() tenon.Constraint { return RandomConstraint(r, depth-1, capsule) }
	children := func(max int) []tenon.Constraint {
		out := make([]tenon.Constraint, r.Intn(max+1))
		for i := range out {
			out[i] = child()
		}
		return out
	}
	switch r.Intn(6) {
	case 0:
		return tenon.ListOf(child())
	case 1:
		return tenon.SetOf(child())
	case 2:
		return tenon.MapOf(child())
	case 3:
		return tenon.TupleOf(children(2)...)
	case 4:
		m := map[string]tenon.Field{}
		for _, name := range []string{"a", "b"} {
			if r.Intn(2) == 0 {
				m[name] = tenon.Field{Constraint: child(), Required: r.Intn(2) == 0}
			}
		}
		return tenon.ObjectWith(m, r.Intn(2) == 0)
	}
	return tenon.OneOf(children(3)...)
}
