package tenon_test

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"tenon"
	"tenon/conformance"
)

// unifyOK unifies cs under p, failing t if unification fails.
func unifyOK(t *testing.T, p tenon.Policy, cs ...tenon.Constraint) tenon.Constraint {
	t.Helper()
	u, failure, ok := tenon.Unify(p, cs...)
	if !ok {
		t.Fatalf("Unify(%s, %v) failed: %v", p, cs, failure)
	}
	return u
}

// wantUnified fails t unless cs unify under p to want.
func wantUnified(t *testing.T, p tenon.Policy, want tenon.Constraint, cs ...tenon.Constraint) {
	t.Helper()
	u, failure, ok := tenon.Unify(p, cs...)
	switch {
	case !ok:
		t.Errorf("Unify(%s, %v) failed: %v, want %v", p, cs, failure, want)
	case !u.Equal(want):
		t.Errorf("Unify(%s, %v) = %v, want %v", p, cs, u, want)
	}
}

// wantNoUnification fails t unless cs fail to unify under p.
func wantNoUnification(t *testing.T, p tenon.Policy, cs ...tenon.Constraint) {
	t.Helper()
	u, failure, ok := tenon.Unify(p, cs...)
	if ok {
		t.Errorf("Unify(%s, %v) = %v, want a failure", p, cs, u)
		return
	}
	if !failure.IsError() || failure.Diagnostics()[0].Code != tenon.CodeUnifyNoCommonConstraint {
		t.Errorf("Unify(%s, %v) failed with %v, want code %s", p, cs, failure, tenon.CodeUnifyNoCommonConstraint)
	}
}

// fields builds an ObjectWith constraint from name and field pairs.
func fields(closed bool, pairs ...any) tenon.Constraint {
	m := map[string]tenon.Field{}
	for i := 0; i < len(pairs); i += 2 {
		m[pairs[i].(string)] = pairs[i+1].(tenon.Field)
	}
	return tenon.ObjectWith(m, closed)
}

func TestConformance_CV040_Unification(t *testing.T) {
	conformance.Covers(t, "CV-040")
	wantUnified(t, safe, is(num), is(num), is(num))
	wantUnified(t, uns, is(str), is(num), is(str))
	wantNoUnification(t, safe, is(num), is(str))

	// Every value of each type given converts to the result.
	u := unifyOK(t, uns, is(tenon.Tuple(num, boo)), is(tenon.List(str)))
	for _, v := range []tenon.Value{tenon.TupleVal(n(1), tenon.Bool(true)), tenon.ListVal(str, s("x"))} {
		if r := tenon.Convert(v, u, uns); r.IsError() {
			t.Errorf("%v does not convert to the unification %v: %v", v, u, r)
		}
	}

	// What a constraint leaves open takes what the others give: a value that
	// was only known to be something converts, or fails to, once it is known.
	wantUnified(t, safe, is(num), tenon.Any(), is(num))
	if r := tenon.Convert(tenon.Bool(true), is(num), safe); !r.IsError() {
		t.Errorf("a bool converted to the number that Any unified with gave %v", r)
	}
	wantUnified(t, safe, fields(false, "a", tenon.Optional(is(num))), fields(false), fields(true, "a", tenon.Required(is(num))))

	// The failure is an error value naming what was unified.
	_, failure, _ := tenon.Unify(safe, is(str), is(num))
	if msg := failure.Diagnostics()[0].Message; !strings.Contains(msg, "exactly(number)") || !strings.Contains(msg, "exactly(string)") || !strings.Contains(msg, "safe") {
		t.Errorf("the failure says %q", msg)
	}
	mustPanicUsage(t, "neither Safe nor Unsafe", func() { tenon.Unify(0, is(num)) })
	mustPanicUsage(t, "use of the zero Constraint", func() { tenon.Unify(safe, is(num), tenon.Constraint{}) })
}

func TestConformance_CV042_UnificationRules(t *testing.T) {
	conformance.Covers(t, "CV-042")
	anyC, listNum := tenon.Any(), is(tenon.List(num))

	// 1. Any gives way to the other constraint.
	wantUnified(t, safe, tenon.ListOf(anyC), anyC, tenon.ListOf(anyC))
	wantUnified(t, safe, tenon.OneOf(), anyC, tenon.OneOf())

	// 2. OneOf distributes, leaving out members that do not unify.
	wantUnified(t, safe, is(num), tenon.OneOf(is(num), is(str)), is(num))
	wantUnified(t, uns, tenon.OneOf(is(num), is(str)), tenon.OneOf(is(num), is(boo)), is(num))
	wantUnified(t, safe, tenon.OneOf(is(num), is(str)), tenon.OneOf(is(num), is(str)), tenon.OneOf(is(str), is(num)))
	wantNoUnification(t, safe, tenon.OneOf(is(num), is(str)), is(boo))
	wantNoUnification(t, safe, tenon.OneOf(), is(num))

	// 3. Exactly of a structure unifies as that structure.
	wantUnified(t, safe, listNum, listNum, tenon.ListOf(anyC))
	wantUnified(t, safe, is(tenon.Map(num)), is(tenon.Object(map[string]tenon.Type{"a": num})), tenon.MapOf(is(num)))

	// 4. One type, and primitives as strings under the unsafe policy only.
	wantUnified(t, safe, is(boo), is(boo), is(boo))
	for _, pair := range [][2]tenon.Type{{num, str}, {boo, str}, {num, boo}} {
		wantUnified(t, uns, is(str), is(pair[0]), is(pair[1]))
		wantNoUnification(t, safe, is(pair[0]), is(pair[1]))
	}

	// 5. Collections of one kind, and a set with a list as a list.
	wantUnified(t, uns, is(tenon.Set(str)), tenon.SetOf(is(num)), tenon.SetOf(is(str)))
	wantUnified(t, safe, tenon.MapOf(anyC), tenon.MapOf(anyC), tenon.MapOf(anyC))
	wantUnified(t, safe, tenon.ListOf(tenon.OneOf(is(num), is(str))), tenon.SetOf(tenon.OneOf(is(num), is(str))), tenon.ListOf(anyC))
	wantNoUnification(t, safe, tenon.ListOf(anyC), tenon.MapOf(anyC))

	// 6. Tuples of one length position by position; otherwise a list.
	wantUnified(t, uns, is(tenon.Tuple(str, boo)), tenon.TupleOf(is(num), is(boo)), tenon.TupleOf(is(str), is(boo)))
	wantUnified(t, safe, is(tenon.List(num)), tenon.TupleOf(is(num)), tenon.TupleOf(is(num), is(num)))
	wantUnified(t, uns, is(tenon.List(str)), tenon.TupleOf(is(num), is(boo)), tenon.SetOf(is(str)))
	wantNoUnification(t, safe, tenon.TupleOf(is(num)), tenon.TupleOf(is(str)))

	// 7. Objects field by field, and an object with a map as a map.
	wantUnified(t, safe,
		fields(false, "a", tenon.Required(is(num)), "b", tenon.Optional(is(str)), "c", tenon.Optional(tenon.ListOf(anyC))),
		fields(true, "a", tenon.Required(is(num)), "b", tenon.Required(is(str))),
		fields(false, "a", tenon.Required(anyC), "c", tenon.Required(tenon.ListOf(anyC))))
	wantUnified(t, uns, is(tenon.Map(str)), fields(false, "a", tenon.Required(is(num))), tenon.MapOf(is(boo)))
	wantNoUnification(t, safe, fields(true, "a", tenon.Required(is(num))), fields(true, "a", tenon.Required(is(str))))

	// 8. Everything else fails, capsule types that are not one type included.
	one := tenon.Capsule("one", tenon.CapsuleOps[celsius]{})
	other := tenon.Capsule("one", tenon.CapsuleOps[celsius]{})
	wantUnified(t, uns, is(one), is(one), is(one))
	for _, pair := range [][2]tenon.Constraint{
		{is(one), is(other)},
		{is(num), tenon.ListOf(anyC)},
		{tenon.TupleOf(), tenon.MapOf(anyC)},
		{fields(false), tenon.ListOf(anyC)},
		{is(one), is(str)},
	} {
		wantNoUnification(t, uns, pair[0], pair[1])
	}
}

func TestConformance_CV043_CanonicalForm(t *testing.T) {
	conformance.Covers(t, "CV-043")
	anyC := tenon.Any()
	opt := tenon.Optional
	// A constraint that admits one type is Exactly of it, and one that admits
	// none is OneOf(), at any depth.
	wantUnified(t, safe, is(tenon.List(num)), tenon.ListOf(tenon.OneOf(is(num), tenon.OneOf())))
	wantUnified(t, safe, tenon.OneOf(), tenon.TupleOf(anyC, tenon.ListOf(tenon.OneOf())))
	wantUnified(t, safe, tenon.ListOf(tenon.OneOf(anyC, tenon.ListOf(anyC))), tenon.ListOf(tenon.OneOf(tenon.ListOf(anyC), anyC)))
	// An optional field no attribute can fill is left out.
	wantUnified(t, safe, fields(false, "b", opt(anyC)), fields(false, "a", opt(tenon.OneOf()), "b", opt(anyC)))
	// OneOf members are flattened, appear once, and one left alone stands for
	// the OneOf.
	wantUnified(t, safe, tenon.ListOf(anyC), tenon.OneOf(tenon.OneOf(tenon.ListOf(anyC)), tenon.ListOf(anyC)))

	// The order of members, each tie-break in turn.
	for _, tt := range []struct {
		name  string
		given []tenon.Constraint
		want  []tenon.Constraint
	}{
		{"kinds", []tenon.Constraint{fields(false), tenon.TupleOf(anyC), tenon.MapOf(anyC), tenon.SetOf(anyC), tenon.ListOf(anyC), anyC, is(num)},
			[]tenon.Constraint{is(num), anyC, tenon.ListOf(anyC), tenon.SetOf(anyC), tenon.MapOf(anyC), tenon.TupleOf(anyC), fields(false)}},
		{"types", []tenon.Constraint{is(tenon.List(num)), is(str), is(boo), is(num)},
			[]tenon.Constraint{is(boo), is(num), is(str), is(tenon.List(num))}},
		{"collection members", []tenon.Constraint{tenon.ListOf(tenon.TupleOf(anyC)), tenon.ListOf(anyC)},
			[]tenon.Constraint{tenon.ListOf(anyC), tenon.ListOf(tenon.TupleOf(anyC))}},
		{"tuple members in turn, shorter first", []tenon.Constraint{tenon.TupleOf(tenon.ListOf(anyC)), tenon.TupleOf(anyC, anyC), tenon.TupleOf(anyC)},
			[]tenon.Constraint{tenon.TupleOf(anyC), tenon.TupleOf(anyC, anyC), tenon.TupleOf(tenon.ListOf(anyC))}},
		{"field names", []tenon.Constraint{fields(false, "b", opt(anyC)), fields(false, "a", opt(anyC))},
			[]tenon.Constraint{fields(false, "a", opt(anyC)), fields(false, "b", opt(anyC))}},
		{"optional before required", []tenon.Constraint{fields(false, "a", tenon.Required(anyC)), fields(false, "a", opt(anyC))},
			[]tenon.Constraint{fields(false, "a", opt(anyC)), fields(false, "a", tenon.Required(anyC))}},
		{"field constraints", []tenon.Constraint{fields(false, "a", opt(tenon.ListOf(anyC))), fields(false, "a", opt(anyC))},
			[]tenon.Constraint{fields(false, "a", opt(anyC)), fields(false, "a", opt(tenon.ListOf(anyC)))}},
		{"fewer fields first", []tenon.Constraint{fields(false, "a", opt(anyC), "b", opt(anyC)), fields(false, "a", opt(anyC))},
			[]tenon.Constraint{fields(false, "a", opt(anyC)), fields(false, "a", opt(anyC), "b", opt(anyC))}},
		{"open before closed", []tenon.Constraint{fields(true, "a", opt(anyC)), fields(false, "a", opt(anyC))},
			[]tenon.Constraint{fields(false, "a", opt(anyC)), fields(true, "a", opt(anyC))}},
	} {
		got := unifyOK(t, safe, tenon.OneOf(tt.given...))
		if want := tenon.OneOf(tt.want...); !got.Equal(want) {
			t.Errorf("%s: %v, want %v", tt.name, got, want)
		}
	}
}

func TestConformance_UN020_PendingValuesCarryAConstraint(t *testing.T) {
	conformance.Covers(t, "UN-020", "UN-022")
	c := tenon.ListOf(tenon.OneOf(is(num), is(str)))
	v := tenon.WithMarks(tenon.Narrow(tenon.Pending(c), tenon.Null()), stamp{id: "m"})
	if got := v.Constraint(); !got.Equal(c) {
		t.Errorf("the constraint of %v is %v, want %v", v, got, c)
	}
	for _, other := range []tenon.Value{n(1), tenon.Unknown(num), tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: "it failed"})} {
		mustPanicUsage(t, "which is not a pending value", func() { other.Constraint() })
	}
}

// randomConstraint returns a constraint of every kind, nested up to depth.
func randomConstraint(r *rand.Rand, depth int, capsule tenon.Type) tenon.Constraint {
	leaves := []tenon.Constraint{
		tenon.Any(), is(num), is(str), is(boo), is(tenon.List(num)), is(tenon.Tuple(num, str)),
		is(tenon.Object(map[string]tenon.Type{"a": num})), is(tenon.Map(str)), is(capsule), tenon.OneOf(),
	}
	if depth == 0 || r.Intn(3) == 0 {
		return leaves[r.Intn(len(leaves))]
	}
	child := func() tenon.Constraint { return randomConstraint(r, depth-1, capsule) }
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

// related returns a constraint of much the same shape as c, so that the two
// often unify: parts of it left open, primitive types swapped, a list for a
// set, a tuple for a list, fields made optional, dropped or added, and closed
// objects opened.
func related(r *rand.Rand, c tenon.Constraint, capsule tenon.Type) tenon.Constraint {
	if r.Intn(6) == 0 {
		return tenon.Any()
	}
	child := func(m tenon.Constraint) tenon.Constraint { return related(r, m, capsule) }
	switch c.Kind() {
	case tenon.ConstraintExactly:
		if r.Intn(3) == 0 {
			return is([]tenon.Type{num, str, boo}[r.Intn(3)])
		}
		return c
	case tenon.ConstraintListOf, tenon.ConstraintSetOf:
		e := child(c.Element())
		switch r.Intn(3) {
		case 0:
			return tenon.ListOf(e)
		case 1:
			return tenon.SetOf(e)
		}
		return tenon.TupleOf(e)
	case tenon.ConstraintMapOf:
		if r.Intn(3) == 0 {
			return tenon.ObjectWith(map[string]tenon.Field{"a": tenon.Required(child(c.Element()))}, false)
		}
		return tenon.MapOf(child(c.Element()))
	case tenon.ConstraintTupleOf:
		members := c.Members()
		for i, m := range members {
			members[i] = child(m)
		}
		if r.Intn(4) == 0 && len(members) > 0 {
			return tenon.ListOf(members[0])
		}
		return tenon.TupleOf(members...)
	case tenon.ConstraintObjectWith:
		m := map[string]tenon.Field{}
		for _, name := range c.FieldNames() {
			f, _ := c.Field(name)
			if r.Intn(5) == 0 {
				continue
			}
			m[name] = tenon.Field{Constraint: child(f.Constraint), Required: f.Required && r.Intn(3) != 0}
		}
		if r.Intn(4) == 0 {
			m["c"] = tenon.Optional(is(num))
		}
		return tenon.ObjectWith(m, c.Closed() && r.Intn(2) == 0)
	case tenon.ConstraintOneOf:
		var members []tenon.Constraint
		for _, m := range c.Members() {
			if r.Intn(3) != 0 {
				members = append(members, child(m))
			}
		}
		return tenon.OneOf(members...)
	}
	return c
}

// unification is what one call to Unify gave.
type unification struct {
	c       tenon.Constraint
	failure tenon.Value
	ok      bool
}

// unify calls Unify, reporting a panic as a failure of the test.
func unify(t *testing.T, p tenon.Policy, cs ...tenon.Constraint) (u unification) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Unify(%s, %v) panicked: %v", p, cs, r)
		}
	}()
	u.c, u.failure, u.ok = tenon.Unify(p, cs...)
	return u
}

// same reports whether two unifications gave one answer.
func (u unification) same(v unification) bool {
	if u.ok != v.ok {
		return false
	}
	if u.ok {
		return u.c.Equal(v.c)
	}
	return tenon.Identical(u.failure, v.failure)
}

func (u unification) String() string {
	if u.ok {
		return u.c.String()
	}
	return u.failure.String()
}

func TestConformance_CV041_UnificationIsOrderIndependent(t *testing.T) {
	conformance.Covers(t, "CV-041")
	wantUnified(t, safe, tenon.Any())
	wantUnified(t, safe, tenon.OneOf(is(num), is(str)), tenon.OneOf(is(str), is(num)))

	r := rand.New(rand.NewSource(20260916))
	capsule := tenon.Capsule("cap", tenon.CapsuleOps[celsius]{})
	succeeded, failed := 0, 0
	for i := 0; i < 1500; i++ {
		base := randomConstraint(r, 3, capsule)
		cs := []tenon.Constraint{base, related(r, base, capsule), related(r, base, capsule)}
		if i%4 == 0 {
			cs[2] = randomConstraint(r, 3, capsule)
		}
		for _, p := range []tenon.Policy{safe, uns} {
			what := fmt.Sprintf("Unify(%s, %v)", p, cs)
			whole := unify(t, p, cs...)
			if whole.ok {
				succeeded++
				// A result is canonical already.
				if again := unify(t, p, whole.c); !again.same(whole) {
					t.Errorf("%s = %v, which unifies alone to %v", what, whole, again)
				}
			} else {
				failed++
			}
			// Any order of the constraints gives the same answer, a failure's
			// diagnostic included.
			for _, perm := range permutations(3) {
				if got := unify(t, p, cs[perm[0]], cs[perm[1]], cs[perm[2]]); !got.same(whole) {
					t.Errorf("%s = %v, but in the order %v it is %v", what, whole, perm, got)
				}
			}
			// Any grouping gives the same constraint, or fails as well.
			left, right := unify(t, p, cs[0], cs[1]), unify(t, p, cs[1], cs[2])
			for _, g := range []struct {
				first unification
				other tenon.Constraint
				name  string
			}{{left, cs[2], "(a, b), c"}, {right, cs[0], "a, (b, c)"}} {
				if !g.first.ok {
					if whole.ok {
						t.Errorf("%s = %v, but grouped as %s it fails: %v", what, whole, g.name, g.first)
					}
					continue
				}
				grouped := unify(t, p, g.first.c, g.other)
				if grouped.ok != whole.ok || grouped.ok && !grouped.c.Equal(whole.c) {
					t.Errorf("%s = %v, but grouped as %s it is %v", what, whole, g.name, grouped)
				}
			}
			// Any is the identity.
			if got := unify(t, p, cs[0], tenon.Any()); !got.same(unify(t, p, cs[0])) {
				t.Errorf("Unify(%s, %v, any) = %v", p, cs[0], got)
			}
		}
	}
	if succeeded < 500 || failed < 500 {
		t.Errorf("the generated sets unified %d times and failed %d times, too few of one to say much", succeeded, failed)
	}
}

// TestUnificationAgreesWithTypeUnification holds unification of the Exactly
// constraints of two types to the type unification that conversion uses,
// which differs only for objects whose attributes differ.
func TestUnificationAgreesWithTypeUnification(t *testing.T) {
	conformance.Covers(t, "CV-044")
	objA := obj(map[string]tenon.Value{"a": n(1)})
	objB := obj(map[string]tenon.Value{"b": s("x")})
	values := []tenon.Value{
		n(1), s("x"), tenon.Bool(true), tenon.ListVal(num, n(1)), tenon.SetVal(str, s("a")), tenon.TupleVal(n(1)),
		tenon.TupleVal(s("a"), n(2)), tenon.MapVal(num, nil), objA, objB,
	}
	for _, a := range values {
		for _, b := range values {
			for _, p := range []tenon.Policy{safe, uns} {
				u, _, ok := tenon.Unify(p, is(a.Type()), is(b.Type()))
				list := tenon.Convert(tenon.TupleVal(a, b), tenon.ListOf(tenon.Any()), p)
				differentObjects := a.Type().Kind() == tenon.KindObject && b.Type().Kind() == tenon.KindObject && a.Type() != b.Type()
				switch {
				case differentObjects:
					if !ok || u.Kind() != tenon.ConstraintObjectWith || list.IsError() {
						t.Errorf("%v and %v unify to %v and convert to %v", a.Type(), b.Type(), u, list)
					}
				case ok != !list.IsError():
					t.Errorf("under %s, %v and %v unify: %t, but a tuple of them converts to %v", p, a.Type(), b.Type(), ok, list)
				case ok && !u.Equal(is(list.Type().ElementType())):
					t.Errorf("under %s, %v and %v unify to %v, but the element type is %v", p, a.Type(), b.Type(), u, list.Type().ElementType())
				}
			}
		}
	}
}
