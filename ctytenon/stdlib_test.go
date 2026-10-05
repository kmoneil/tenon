package ctytenon_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"math/rand"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/ctytenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/stdlib"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
	"github.com/zclconf/go-cty/cty/function"
	ctystdlib "github.com/zclconf/go-cty/cty/function/stdlib"
)

// The tests in this file hold tenon's standard library to go-cty's: each
// function is called with the same arguments on both sides, go-cty's after
// the conversion HCL makes before it calls, and where go-cty answers, tenon
// answers the same, unless a registered divergence says why tenon answers
// otherwise. A difference no divergence registers fails the run, and so
// does a divergence that no call exercises, so the registry says what is
// true and nothing more.

// counterpart is a library function and go-cty's.
type counterpart struct {
	cty function.Function
	ten tenon.Function
	// cases are arguments to call with, beside random ones of the
	// parameters' types.
	cases [][]cty.Value
	// random returns random arguments, where randomArgs's are not the
	// ones to compare with.
	random func(r *rand.Rand) []cty.Value
	// divergences are where tenon answers otherwise, and why.
	divergences []divergence
}

// divergence is a call whose answers differ, for a reason: an issue go-cty
// has, or a row of the specification's divergences from it.
type divergence struct {
	why   string
	match func(args []cty.Value) bool
}

// counterparts is every function of the library beside go-cty's.
var counterparts = map[string]counterpart{
	"AssertNotNull": {
		cty: ctystdlib.AssertNotNullFunc,
		ten: stdlib.AssertNotNullFunc,
		cases: [][]cty.Value{
			{cty.StringVal("a")},
			{cty.NullVal(cty.String)},
			{cty.NullVal(cty.DynamicPseudoType)},
			{cty.UnknownVal(cty.String)},
			{cty.DynamicVal},
			{cty.StringVal("a").Mark("sensitive")},
			{cty.ListVal([]cty.Value{cty.StringVal("a"), cty.UnknownVal(cty.String)})},
		},
		divergences: []divergence{{
			// cty's unknown answer holds no refinement where the argument's
			// type is not known yet; tenon's pending answer is not null all
			// the same, as the function declares (FN-024).
			why:   "a dynamic argument: cty answers DynamicVal, unrefined, and tenon a pending value not null",
			match: func(args []cty.Value) bool { return args[0].Type() == cty.DynamicPseudoType && !args[0].IsKnown() },
		}},
	},
	"Add":                  arithmetic(ctystdlib.AddFunc, stdlib.AddFunc),
	"Subtract":             arithmetic(ctystdlib.SubtractFunc, stdlib.SubtractFunc),
	"Multiply":             arithmetic(ctystdlib.MultiplyFunc, stdlib.MultiplyFunc),
	"Divide":               arithmetic(ctystdlib.DivideFunc, stdlib.DivideFunc),
	"Modulo":               arithmetic(ctystdlib.ModuloFunc, stdlib.ModuloFunc),
	"Negate":               arithmetic(ctystdlib.NegateFunc, stdlib.NegateFunc),
	"LessThan":             arithmetic(ctystdlib.LessThanFunc, stdlib.LessThanFunc),
	"LessThanOrEqualTo":    arithmetic(ctystdlib.LessThanOrEqualToFunc, stdlib.LessThanOrEqualToFunc),
	"GreaterThan":          arithmetic(ctystdlib.GreaterThanFunc, stdlib.GreaterThanFunc),
	"GreaterThanOrEqualTo": arithmetic(ctystdlib.GreaterThanOrEqualToFunc, stdlib.GreaterThanOrEqualToFunc),
	"Not":                  {cty: ctystdlib.NotFunc, ten: stdlib.NotFunc},
	"And":                  {cty: ctystdlib.AndFunc, ten: stdlib.AndFunc},
	"Or":                   {cty: ctystdlib.OrFunc, ten: stdlib.OrFunc},
	"Equal":                equality(ctystdlib.EqualFunc, stdlib.EqualFunc),
	"NotEqual":             equality(ctystdlib.NotEqualFunc, stdlib.NotEqualFunc),
}

// arithmetic returns the counterpart of a function of numbers, with the
// cases every one of them is called with.
func arithmetic(c function.Function, ten tenon.Function) counterpart {
	n := cty.NumberIntVal
	cases := [][]cty.Value{
		{n(7), n(2)}, {n(-7), n(2)}, {n(1), n(0)}, {n(0), n(0)},
		{cty.MustParseNumberVal("0.1"), cty.MustParseNumberVal("0.2")},
		{cty.MustParseNumberVal("1e200"), n(7)},
		{cty.UnknownVal(cty.Number), n(0)},
		{cty.UnknownVal(cty.Number).Refine().NumberRangeLowerBound(n(1), true).NewValue(), n(1)},
		{n(3).Mark("sensitive"), n(2)},
		{cty.NullVal(cty.Number), n(1)},
	}
	if len(c.Params()) == 1 {
		for i := range cases {
			cases[i] = cases[i][:1]
		}
	}
	cp := counterpart{cty: c, ten: ten, cases: cases}
	switch ten.Name() {
	case "Add", "Subtract", "Multiply", "Negate":
		cp.divergences = []divergence{binaryArithmetic(ten)}
	case "Modulo":
		cp.divergences = []divergence{{
			why:   "modulo by zero: cty answers the dividend, and tenon fails, known or not (NU-014, LB-011, Appendix B row 30)",
			match: func(args []cty.Value) bool { return isZero(args[1]) },
		}, binaryArithmetic(ten)}
	case "Divide":
		cp.divergences = []divergence{{
			why:   "division by zero: cty answers an infinity, and tenon fails (NU-013, Appendix B row 6)",
			match: func(args []cty.Value) bool { return isZero(args[1]) },
		}, binaryArithmetic(ten), {
			why: "a quotient that does not terminate: cty rounds it to 512 bits, and tenon to 96 significant digits (NU-012)",
			match: func(args []cty.Value) bool {
				a, okA := numberOf(args[0])
				b, okB := numberOf(args[1])
				return okA && okB && !tenon.Mul(tenon.Div(a, b), b).Equal(a)
			},
		}}
	}
	return cp
}

// binaryArithmetic is where cty's numbers, binary floating point of 512
// bits, round what tenon's hold exactly: an operand that is not an integer,
// or a value at or beyond 10^150, given or computed (Appendix B row 5).
func binaryArithmetic(ten tenon.Function) divergence {
	return divergence{
		why: "an operand that is not an integer, or a value of 10^150 or more: cty rounds it to 512 binary bits, and tenon holds it exactly (NU-010, Appendix B row 5)",
		match: func(args []cty.Value) bool {
			targs := make([]tenon.Value, len(args))
			for i, a := range args {
				n, ok := numberOf(a)
				if !ok {
					return false
				}
				if !isInteger(n) || isHuge(n) {
					return true
				}
				targs[i] = n
			}
			r := tenon.Call(ten, targs, tenon.Safe)
			return !r.IsError() && r.IsKnown() && isHuge(r)
		},
	}
}

// numberOf returns the number a cty value is, where it is a known number.
func numberOf(v cty.Value) (tenon.Value, bool) {
	v, _ = v.UnmarkDeep()
	if v.Type() != cty.Number || !v.IsKnown() || v.IsNull() {
		return tenon.Value{}, false
	}
	n, err := ctytenon.Bridge{}.FromCty(v)
	return n, err == nil
}

// isZero reports whether v is the number zero.
func isZero(v cty.Value) bool {
	n, ok := numberOf(v)
	return ok && n.Equal(tenon.NumberFromInt(0))
}

// isInteger reports whether the number n is an integer.
func isInteger(n tenon.Value) bool {
	return tenon.Mod(n, tenon.NumberFromInt(1)).Equal(tenon.NumberFromInt(0))
}

// huge is 10^150, below which cty's 512 bits hold every integer.
var huge = tenon.NumberFromText("1e150")

// isHuge reports whether the number n is 10^150 or more in magnitude.
func isHuge(n tenon.Value) bool {
	n, _ = tenon.Unmark(n)
	return !tenon.LessThan(n, huge).AsBool() || !tenon.LessThan(tenon.Sub(tenon.NumberFromInt(0), huge), n).AsBool()
}

// equality returns the counterpart of Equal or NotEqual, called with pairs
// of values alike, related, and of other types.
func equality(c function.Function, ten tenon.Function) counterpart {
	return counterpart{
		cty: c,
		ten: ten,
		cases: [][]cty.Value{
			{cty.NullVal(cty.DynamicPseudoType), cty.NullVal(cty.String)},
			{cty.NullVal(cty.DynamicPseudoType), cty.StringVal("x")},
			{cty.NullVal(cty.DynamicPseudoType), cty.NullVal(cty.DynamicPseudoType)},
			{cty.NumberIntVal(1), cty.StringVal("1")},
			{cty.MustParseNumberVal("1.50"), cty.MustParseNumberVal("1.5")},
			{cty.UnknownVal(cty.String), cty.NullVal(cty.DynamicPseudoType)},
		},
		random: func(r *rand.Rand) []cty.Value {
			v := randomCtyValue(r, randomCtyType(r, 3))
			return []cty.Value{v, pairOf(r, v)}
		},
		divergences: []divergence{{
			why: "#229: a set holding a member with an unknown part: cty answers false, even of the set and itself, where the members may turn out equal",
			match: func(args []cty.Value) bool {
				return holdsPartlyUnknownMember(args[0]) || holdsPartlyUnknownMember(args[1])
			},
		}},
	}
}

// TestLibraryHasCounterparts holds counterparts to the library: every
// function the stdlib package declares is compared with go-cty's, read from
// the package's source so that a function added without its counterpart
// fails here.
func TestLibraryHasCounterparts(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "stdlib", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	var declared []string
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			if gd, ok := decl.(*ast.GenDecl); ok && gd.Tok == token.VAR {
				for _, spec := range gd.Specs {
					for _, id := range spec.(*ast.ValueSpec).Names {
						if fn, ok := strings.CutSuffix(id.Name, "Func"); ok && id.IsExported() {
							declared = append(declared, fn)
						}
					}
				}
			}
		}
	}
	slices.Sort(declared)
	if have := slices.Sorted(maps.Keys(counterparts)); !slices.Equal(declared, have) {
		t.Errorf("the stdlib package declares %v, and counterparts holds %v", declared, have)
	}
	for name, cp := range counterparts {
		if cp.ten.Name() != name {
			t.Errorf("counterparts[%q] holds %s", name, cp.ten.Name())
		}
	}
}

func TestLibraryAgreesWithCty(t *testing.T) {
	b := terraform()
	r := rand.New(rand.NewSource(20261005))
	for _, name := range slices.Sorted(maps.Keys(counterparts)) {
		cp := counterparts[name]
		fired := make([]bool, len(cp.divergences))
		calls := slices.Clone(cp.cases)
		for range conformance.Iterations(t, 300) {
			if cp.random != nil {
				calls = append(calls, cp.random(r))
			} else {
				calls = append(calls, randomArgs(r, cp.cty))
			}
		}
		for _, args := range calls {
			what := fmt.Sprintf("%s(%#v)", name, args)
			if i := slices.IndexFunc(cp.divergences, func(d divergence) bool { return d.match(args) }); i >= 0 {
				fired[i] = true
				continue
			}
			agree(t, b, what, cp, args)
		}
		for i, d := range cp.divergences {
			if !fired[i] {
				t.Errorf("%s: no call exercises the divergence %q", name, d.why)
			}
		}
	}
}

// agree calls both functions with args and holds tenon's answer to go-cty's.
func agree(t *testing.T, b ctytenon.Bridge, what string, cp counterpart, args []cty.Value) {
	t.Helper()
	// HCL converts each argument to its parameter's type before it calls,
	// and reports a conversion's failure itself; such a call never reaches
	// either function.
	converted := make([]cty.Value, len(args))
	for i, a := range args {
		p, ok := paramOf(cp.cty, i)
		if !ok {
			return
		}
		c, err := convert.Convert(a, p.Type)
		if err != nil {
			return
		}
		converted[i] = c
	}
	cr, cerr, panicked := callCty(cp.cty, converted)
	targs := make([]tenon.Value, len(args))
	for i, a := range args {
		v, err := b.FromCty(a)
		if err != nil {
			t.Fatalf("%s: argument %d does not cross: %v", what, i, err)
		}
		targs[i] = v
	}
	tr := tenon.Call(cp.ten, targs, tenon.Unsafe)
	switch {
	case panicked:
		// go-cty panicked; tenon answered, since a call never panics
		// (FN-003), and its vectors say what the answer is.
	case cerr != nil:
		if !tr.IsError() {
			t.Errorf("%s: cty fails (%v), and tenon answers %v", what, cerr, tr)
		}
	case cr.IsWhollyKnown():
		want, err := b.FromCty(cr)
		if err != nil {
			t.Fatalf("%s: cty's answer %#v does not cross: %v", what, cr, err)
		}
		if !tenon.Identical(tr, want) {
			t.Errorf("%s: cty answers %#v, and tenon %v", what, cr, tr)
		}
	default:
		// cty's answer is not known; tenon's may say more, but not less of
		// what cty's refinements say, and must not fail.
		unmarked, _ := cr.UnmarkDeep()
		if tr.IsError() {
			t.Errorf("%s: cty answers %#v, and tenon fails: %v", what, cr, tr)
		} else if !unmarked.IsKnown() && unmarked.Type() != cty.DynamicPseudoType && !unmarked.Range().CouldBeNull() {
			if n := tenon.IsNull(tr); !n.IsKnown() || n.AsBool() {
				t.Errorf("%s: cty answers %#v, not null, and tenon %v, which may be", what, cr, tr)
			}
		}
	}
}

// paramOf returns the parameter that argument i of f is bound to.
func paramOf(f function.Function, i int) (function.Parameter, bool) {
	if ps := f.Params(); i < len(ps) {
		return ps[i], true
	}
	if vp := f.VarParam(); vp != nil {
		return *vp, true
	}
	return function.Parameter{}, false
}

// callCty calls f, reporting a panic rather than propagating it.
func callCty(f function.Function, args []cty.Value) (r cty.Value, err error, panicked bool) {
	defer func() {
		if recover() != nil {
			panicked = true
		}
	}()
	r, err = f.Call(args)
	return r, err, false
}

// randomArgs returns arguments for f's positional parameters, each a random
// value of the parameter's type, or of a random type where the parameter
// takes any.
func randomArgs(r *rand.Rand, f function.Function) []cty.Value {
	var args []cty.Value
	for _, p := range f.Params() {
		typ := p.Type
		if typ == cty.DynamicPseudoType {
			typ = randomCtyType(r, 2)
		}
		v := randomCtyValue(r, typ)
		if r.Intn(8) == 0 {
			v = v.Mark("sensitive")
		}
		args = append(args, v)
	}
	return args
}
