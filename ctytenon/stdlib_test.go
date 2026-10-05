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
	"Absolute":             arithmetic(ctystdlib.AbsoluteFunc, stdlib.AbsoluteFunc),
	"Signum":               arithmetic(ctystdlib.SignumFunc, stdlib.SignumFunc),
	"Int":                  arithmetic(ctystdlib.IntFunc, stdlib.IntFunc),
	"Ceil":                 arithmetic(ctystdlib.CeilFunc, stdlib.CeilFunc),
	"Floor":                arithmetic(ctystdlib.FloorFunc, stdlib.FloorFunc),
	"Length": {
		cty: ctystdlib.LengthFunc,
		ten: stdlib.LengthFunc,
		cases: [][]cty.Value{
			{cty.ListVal([]cty.Value{cty.StringVal("a"), cty.UnknownVal(cty.String)})},
			{cty.SetVal([]cty.Value{cty.NumberIntVal(1), cty.UnknownVal(cty.Number)})},
			{cty.UnknownVal(cty.Tuple([]cty.Type{cty.String, cty.String}))},
			{cty.ListVal([]cty.Value{cty.StringVal("a")}).Mark("sensitive")},
		},
		random:      func(r *rand.Rand) []cty.Value { return []cty.Value{randomCtyValue(r, randomCtyType(r, 2))} },
		divergences: []divergence{superset("a string or an object", cty.String)},
	},
	"HasIndex": indexing(ctystdlib.HasIndexFunc, stdlib.HasIndexFunc),
	"Index":    indexing(ctystdlib.IndexFunc, stdlib.IndexFunc),
	"Element":  indexing(ctystdlib.ElementFunc, stdlib.ElementFunc),
	"Slice": {
		cty: ctystdlib.SliceFunc,
		ten: stdlib.SliceFunc,
		cases: [][]cty.Value{
			{abcList(), cty.NumberIntVal(1), cty.NumberIntVal(2)},
			{abcList(), cty.NumberIntVal(2), cty.NumberIntVal(1)},
			{cty.TupleVal([]cty.Value{cty.StringVal("a"), cty.NumberIntVal(1)}), cty.NumberIntVal(0), cty.NumberIntVal(1)},
			{cty.UnknownVal(cty.List(cty.String)), cty.NumberIntVal(0), cty.NumberIntVal(1)},
		},
		random: func(r *rand.Rand) []cty.Value {
			return []cty.Value{randomCtyValue(r, cty.List(randomCtyType(r, 1))), cty.NumberIntVal(int64(r.Intn(4))), cty.NumberIntVal(int64(r.Intn(5)))}
		},
	},
	"ReverseList": {
		cty: ctystdlib.ReverseListFunc,
		ten: stdlib.ReverseListFunc,
		cases: [][]cty.Value{
			{abcList()}, {cty.TupleVal([]cty.Value{cty.NumberIntVal(1), cty.StringVal("a")})},
			{cty.SetVal([]cty.Value{cty.NumberIntVal(3), cty.NumberIntVal(1), cty.NumberIntVal(2)})},
		},
		random: func(r *rand.Rand) []cty.Value { return []cty.Value{randomCtyValue(r, cty.List(randomCtyType(r, 1)))} },
	},
	"Concat": {
		cty: ctystdlib.ConcatFunc,
		ten: stdlib.ConcatFunc,
		cases: [][]cty.Value{
			{cty.ListVal([]cty.Value{cty.NumberIntVal(1)}), cty.ListVal([]cty.Value{cty.NumberIntVal(2)})},
			{cty.ListVal([]cty.Value{cty.NumberIntVal(1)}), cty.TupleVal([]cty.Value{cty.StringVal("a")})},
			{cty.ListVal([]cty.Value{cty.NumberIntVal(1), cty.UnknownVal(cty.Number)}), cty.ListVal([]cty.Value{cty.NumberIntVal(2)})},
		},
		random: func(r *rand.Rand) []cty.Value {
			typ := randomCtyType(r, 1)
			return []cty.Value{randomCtyValue(r, cty.List(typ)), randomCtyValue(r, cty.List(typ))}
		},
	},
	"Chunklist": {
		cty: ctystdlib.ChunklistFunc,
		ten: stdlib.ChunklistFunc,
		cases: [][]cty.Value{
			{cty.ListVal([]cty.Value{cty.NumberIntVal(1), cty.NumberIntVal(2), cty.NumberIntVal(3)}), cty.NumberIntVal(2)},
			{cty.ListVal([]cty.Value{cty.NumberIntVal(1), cty.NumberIntVal(2)}), cty.NumberIntVal(0)},
			{cty.ListVal([]cty.Value{cty.NumberIntVal(1), cty.UnknownVal(cty.Number)}), cty.NumberIntVal(1)},
		},
		random: func(r *rand.Rand) []cty.Value {
			return []cty.Value{randomCtyValue(r, cty.List(randomCtyType(r, 1))), cty.NumberIntVal(int64(r.Intn(4)))}
		},
	},
	"Flatten": {
		cty: ctystdlib.FlattenFunc,
		ten: stdlib.FlattenFunc,
		cases: [][]cty.Value{
			{cty.ListVal([]cty.Value{cty.ListVal([]cty.Value{cty.NumberIntVal(1), cty.NumberIntVal(2)}), cty.ListVal([]cty.Value{cty.NumberIntVal(3)})})},
			{cty.TupleVal([]cty.Value{cty.StringVal("a"), cty.TupleVal([]cty.Value{cty.StringVal("b")})})},
			{cty.ListVal([]cty.Value{cty.NullVal(cty.List(cty.Number)), cty.ListVal([]cty.Value{cty.NumberIntVal(1)})})},
		},
		random: func(r *rand.Rand) []cty.Value { return []cty.Value{randomCtyValue(r, cty.List(randomCtyType(r, 2)))} },
		divergences: []divergence{{
			why:   "a set of several members is flattened in tenon's canonical order (EQ-044), which go-cty's iteration does not follow, numbers included",
			match: func(args []cty.Value) bool { return holdsSetOfSeveral(args[0]) },
		}},
	},
	"Compact": {
		cty: ctystdlib.CompactFunc,
		ten: stdlib.CompactFunc,
		cases: [][]cty.Value{
			{cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal(""), cty.NullVal(cty.String), cty.StringVal("b")})},
			{cty.ListVal([]cty.Value{cty.StringVal("a"), cty.UnknownVal(cty.String)})},
		},
		random: func(r *rand.Rand) []cty.Value { return []cty.Value{randomCtyValue(r, cty.List(cty.String))} },
	},
	"Distinct": {
		cty: ctystdlib.DistinctFunc,
		ten: stdlib.DistinctFunc,
		cases: [][]cty.Value{
			{cty.ListVal([]cty.Value{cty.NumberIntVal(1), cty.NumberIntVal(2), cty.NumberIntVal(1)})},
			{cty.ListVal([]cty.Value{cty.NumberIntVal(1), cty.UnknownVal(cty.Number)})},
		},
		random: func(r *rand.Rand) []cty.Value {
			v := randomCtyValue(r, cty.List(randomCtyType(r, 1)))
			if v.IsKnown() && !v.IsNull() && v.LengthInt() > 0 && r.Intn(2) == 0 {
				// A repeated member, to be taken out.
				elems := v.AsValueSlice()
				v = cty.ListVal(append(elems, elems[0]))
			}
			return []cty.Value{v}
		},
	},
	"CoalesceList": {
		cty: ctystdlib.CoalesceListFunc,
		ten: stdlib.CoalesceListFunc,
		cases: [][]cty.Value{
			{cty.ListValEmpty(cty.Number), cty.ListVal([]cty.Value{cty.NumberIntVal(1)})},
			{cty.NullVal(cty.List(cty.Number)), cty.ListVal([]cty.Value{cty.NumberIntVal(1)})},
			{cty.ListVal([]cty.Value{cty.NumberIntVal(1)}), cty.UnknownVal(cty.List(cty.Number))},
		},
		random: func(r *rand.Rand) []cty.Value {
			typ := cty.List(randomCtyType(r, 1))
			return []cty.Value{randomCtyValue(r, typ), randomCtyValue(r, typ)}
		},
	},
	"Keys": {
		cty: ctystdlib.KeysFunc,
		ten: stdlib.KeysFunc,
		cases: [][]cty.Value{
			{cty.MapVal(map[string]cty.Value{"b": cty.NumberIntVal(2), "a": cty.NumberIntVal(1), "B": cty.NumberIntVal(3)})},
			{cty.UnknownVal(cty.Object(map[string]cty.Type{"b": cty.String, "a": cty.String}))},
		},
		random: func(r *rand.Rand) []cty.Value { return []cty.Value{randomCtyValue(r, randomMapOrObject(r))} },
	},
	"Values": {
		cty: ctystdlib.ValuesFunc,
		ten: stdlib.ValuesFunc,
		cases: [][]cty.Value{
			{cty.MapVal(map[string]cty.Value{"b": cty.NumberIntVal(2), "a": cty.NumberIntVal(1)})},
			{cty.ObjectVal(map[string]cty.Value{"b": cty.NumberIntVal(2), "a": cty.StringVal("x")})},
		},
		random: func(r *rand.Rand) []cty.Value { return []cty.Value{randomCtyValue(r, randomMapOrObject(r))} },
	},
	"Zipmap": {
		cty: ctystdlib.ZipmapFunc,
		ten: stdlib.ZipmapFunc,
		cases: [][]cty.Value{
			{cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}), cty.ListVal([]cty.Value{cty.NumberIntVal(1), cty.NumberIntVal(2)})},
			{cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}), cty.TupleVal([]cty.Value{cty.NumberIntVal(1), cty.StringVal("x")})},
			{cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("a")}), cty.ListVal([]cty.Value{cty.NumberIntVal(1), cty.NumberIntVal(2)})},
		},
		random: func(r *rand.Rand) []cty.Value {
			keys := randomCtyValue(r, cty.List(cty.String))
			n := 0
			if keys.IsKnown() && !keys.IsNull() {
				n = keys.LengthInt()
			}
			vals := make([]cty.Value, n)
			for i := range vals {
				vals[i] = cty.NumberIntVal(int64(i))
			}
			if n == 0 {
				return []cty.Value{keys, cty.ListValEmpty(cty.Number)}
			}
			return []cty.Value{keys, cty.ListVal(vals)}
		},
		divergences: []divergence{{
			why: "a key known to be null: tenon fails now, located at it, whatever the other keys turn out to be (LB-011), where cty answers unknown, and panics where every key is known",
			match: func(args []cty.Value) bool {
				keys, _ := args[0].UnmarkDeep()
				if !keys.IsKnown() || keys.IsNull() {
					return false
				}
				for it := keys.ElementIterator(); it.Next(); {
					if _, k := it.Element(); k.IsKnown() && k.IsNull() {
						return true
					}
				}
				return false
			},
		}},
	},
	"Merge": {
		cty: ctystdlib.MergeFunc,
		ten: stdlib.MergeFunc,
		cases: [][]cty.Value{
			{},
			{numberMap("a", 1), numberMap("b", 2)},
			{numberMap("a", 1), cty.MapVal(map[string]cty.Value{"a": cty.StringVal("x")})},
			{cty.ObjectVal(map[string]cty.Value{"a": cty.NumberIntVal(1)}), cty.ObjectVal(map[string]cty.Value{"a": cty.StringVal("x"), "b": cty.NumberIntVal(2)})},
			{cty.NullVal(cty.Object(map[string]cty.Type{"a": cty.Number}))},
			{cty.NullVal(cty.Map(cty.Number))},
			{cty.MapVal(map[string]cty.Value{"a": cty.UnknownVal(cty.Number)}), numberMap("b", 2)},
			{numberMap("\u00e9", 1), numberMap("e\u0301", 2)},
			{cty.MapVal(map[string]cty.Value{"k": cty.ObjectVal(map[string]cty.Value{"a": cty.True})}), cty.MapVal(map[string]cty.Value{"k": cty.ObjectVal(map[string]cty.Value{"b": cty.True})})},
			{cty.ObjectVal(map[string]cty.Value{"a": cty.NumberIntVal(1)}), cty.UnknownVal(cty.Object(map[string]cty.Type{"b": cty.Number}))},
			{cty.ObjectVal(map[string]cty.Value{"a": cty.NumberIntVal(1)}), cty.UnknownVal(cty.Object(map[string]cty.Type{"b": cty.Number})).RefineNotNull()},
			{cty.UnknownVal(cty.Map(cty.Number)), cty.ObjectVal(map[string]cty.Value{"a": cty.NumberIntVal(1)})},
			{cty.NullVal(cty.DynamicPseudoType), numberMap("a", 1)},
			{cty.NullVal(cty.Object(map[string]cty.Type{"a": cty.Number})).Mark("sensitive"), cty.ObjectVal(map[string]cty.Value{"b": cty.NumberIntVal(2)})},
			{cty.NullVal(cty.Map(cty.Number)).Mark("sensitive"), numberMap("b", 2)},
			{cty.UnknownVal(cty.Map(cty.Number)).Mark("sensitive"), numberMap("b", 2)},
			{cty.ListValEmpty(cty.String)},
		},
		random: func(r *rand.Rand) []cty.Value {
			args := make([]cty.Value, r.Intn(4))
			for i := range args {
				typ := cty.Map(cty.Number)
				if r.Intn(3) == 0 {
					typ = randomMapOrObject(r)
				}
				args[i] = randomCtyValue(r, typ)
				switch r.Intn(10) {
				case 0:
					args[i] = cty.NullVal(cty.DynamicPseudoType)
				case 1:
					args[i] = args[i].Mark("sensitive")
				}
			}
			return args
		},
		divergences: []divergence{{
			why: "an untyped null beside maps of one type: cty's answer is an object, since the null has DynamicPseudoType, and tenon's the map the others give, the null taking no part (LC-036, Appendix B row 34)",
			match: func(args []cty.Value) bool {
				var typ cty.Type
				untyped := false
				for _, a := range args {
					u, _ := a.Unmark()
					switch {
					case u.Type() == cty.DynamicPseudoType && u.IsNull():
						untyped = true
					case !u.Type().IsMapType():
						return false
					case typ == cty.NilType:
						typ = u.Type()
					case !u.Type().Equals(typ):
						return false
					}
				}
				return untyped && typ != cty.NilType
			},
		}, {
			why: "a marked null: cty skips a null argument and its marks, and tenon carries them, since whether it is null decides the answer (LC-035, Appendix B row 35)",
			match: func(args []cty.Value) bool {
				return slices.ContainsFunc(args, func(a cty.Value) bool {
					u, marks := a.Unmark()
					return len(marks) > 0 && u.IsNull()
				})
			},
		}},
	},
	"Contains": {
		cty: ctystdlib.ContainsFunc,
		ten: stdlib.ContainsFunc,
		cases: [][]cty.Value{
			{abcList(), cty.StringVal("b")},
			{numberList(1, 2), cty.StringVal("1")},
			{cty.ListVal([]cty.Value{cty.StringVal("1")}), cty.NumberIntVal(1)},
			{cty.ListVal([]cty.Value{cty.NumberIntVal(1), cty.UnknownVal(cty.Number)}), cty.NumberIntVal(1)},
			{cty.ListVal([]cty.Value{cty.NumberIntVal(1), cty.UnknownVal(cty.Number)}), cty.NumberIntVal(2)},
			{cty.ListVal([]cty.Value{cty.NumberIntVal(1), cty.UnknownVal(cty.Number)}), cty.StringVal("x")},
			{cty.ListVal([]cty.Value{cty.NullVal(cty.String)}), cty.NullVal(cty.String)},
			{cty.SetVal([]cty.Value{cty.NumberIntVal(1), cty.UnknownVal(cty.Number)}), cty.NumberIntVal(1)},
			{numberMap("a", 1), cty.StringVal("a")},
			{cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b").Mark("sensitive")}), cty.StringVal("a")},
			{cty.ListValEmpty(cty.Number), cty.NullVal(cty.DynamicPseudoType)},
			{cty.EmptyTupleVal, cty.NullVal(cty.DynamicPseudoType)},
			{cty.ListVal([]cty.Value{cty.NullVal(cty.String)}), cty.NullVal(cty.DynamicPseudoType)},
			{cty.ListValEmpty(cty.Number), cty.UnknownVal(cty.Number)},
			{cty.ListVal([]cty.Value{numberList(1)}), cty.TupleVal([]cty.Value{cty.NumberIntVal(1)})},
			{cty.TupleVal([]cty.Value{cty.True, cty.NullVal(cty.String)}), cty.NullVal(cty.Bool)},
			{cty.ListVal([]cty.Value{cty.SetVal([]cty.Value{cty.ListVal([]cty.Value{cty.UnknownVal(cty.Number)})})}), cty.SetVal([]cty.Value{numberList(1)})},
		},
		random: func(r *rand.Rand) []cty.Value {
			elem := randomCtyType(r, 1)
			l := randomCtyValue(r, []cty.Type{cty.List(elem), cty.Set(elem), cty.Tuple([]cty.Type{elem, cty.String})}[r.Intn(3)])
			needle := randomCtyValue(r, elem)
			if u, _ := l.UnmarkDeep(); u.IsKnown() && !u.IsNull() && u.LengthInt() > 0 && r.Intn(2) == 0 {
				needle = memberOf(r, u)
			}
			return []cty.Value{l, needle}
		},
		divergences: []divergence{{
			why: "a null looked for among nulls of another type: cty's nulls are equal whatever their types, and tenon's of different types are not (EQ-004, EQ-005, Appendix B row 39)",
			match: func(args []cty.Value) bool {
				l, _ := args[0].UnmarkDeep()
				v, _ := args[1].UnmarkDeep()
				if !v.IsNull() || v.Type() == cty.DynamicPseudoType || !l.IsKnown() || l.IsNull() {
					return false
				}
				for it := l.ElementIterator(); it.Next(); {
					if _, m := it.Element(); m.IsNull() && !m.Type().Equals(v.Type()) {
						return true
					}
				}
				return false
			},
		}, {
			why: "#229: a set holding a member with an unknown part: cty answers false, where the members may turn out equal",
			match: func(args []cty.Value) bool {
				return holdsPartlyUnknownMember(args[0]) || holdsPartlyUnknownMember(args[1])
			},
		}, {
			why: "a tuple or an object looked for among lists or maps: cty compares it as it is, never equal to a list or a map, and tenon converts it to the members' type under Safe first (LC-041, Appendix B row 37)",
			match: func(args []cty.Value) bool {
				l, _ := args[0].UnmarkDeep()
				v, _ := args[1].UnmarkDeep()
				if !l.Type().IsListType() && !l.Type().IsSetType() {
					return false
				}
				et := l.Type().ElementType()
				return (v.Type().IsTupleType() && (et.IsListType() || et.IsSetType())) || (v.Type().IsObjectType() && et.IsMapType())
			},
		}},
	},
	"SetHasElement": {
		cty: ctystdlib.SetHasElementFunc,
		ten: stdlib.SetHasElementFunc,
		cases: [][]cty.Value{
			{numberSet(1, 2), cty.NumberIntVal(1)},
			{numberSet(1, 2), cty.StringVal("1")},
			{cty.SetVal([]cty.Value{cty.NumberIntVal(1), cty.UnknownVal(cty.Number)}), cty.NumberIntVal(1)},
			{cty.SetVal([]cty.Value{cty.NumberIntVal(1), cty.UnknownVal(cty.Number)}), cty.NumberIntVal(5)},
			{numberList(1), cty.NumberIntVal(1)},
			{cty.SetVal([]cty.Value{numberList(1)}), cty.ListVal([]cty.Value{cty.UnknownVal(cty.Number)})},
		},
		random: func(r *rand.Rand) []cty.Value {
			elem := randomCtyType(r, 1)
			s := randomCtyValue(r, cty.Set(elem))
			needle := randomCtyValue(r, elem)
			if u, _ := s.UnmarkDeep(); u.IsKnown() && !u.IsNull() && u.LengthInt() > 0 && r.Intn(2) == 0 {
				needle = memberOf(r, u)
			}
			return []cty.Value{s, needle}
		},
		divergences: []divergence{{
			why: "a null element: cty refuses it, and tenon answers whether the set holds null, a member like any other (LC-043)",
			match: func(args []cty.Value) bool {
				v, _ := args[1].UnmarkDeep()
				return v.IsNull()
			},
		}, {
			why: "#229: a set holding a member with an unknown part beside a value it may equal: cty answers false, where tenon's answer is unknown",
			match: func(args []cty.Value) bool {
				v, _ := args[1].UnmarkDeep()
				return v.IsKnown() && !v.IsWhollyKnown() || holdsPartlyUnknownMember(args[0]) || holdsPartlyUnknownMember(args[1])
			},
		}},
	},
	"SetUnion":               setOperation(ctystdlib.SetUnionFunc, stdlib.SetUnionFunc, 3),
	"SetIntersection":        setOperation(ctystdlib.SetIntersectionFunc, stdlib.SetIntersectionFunc, 3),
	"SetSubtract":            setOperation(ctystdlib.SetSubtractFunc, stdlib.SetSubtractFunc, 2),
	"SetSymmetricDifference": setOperation(ctystdlib.SetSymmetricDifferenceFunc, stdlib.SetSymmetricDifferenceFunc, 3),
	"Lookup": {
		cty: ctystdlib.LookupFunc,
		ten: stdlib.LookupFunc,
		cases: [][]cty.Value{
			{cty.MapVal(map[string]cty.Value{"a": cty.NumberIntVal(1)}), cty.StringVal("a"), cty.NumberIntVal(0)},
			{cty.MapVal(map[string]cty.Value{"a": cty.NumberIntVal(1)}), cty.StringVal("c"), cty.NumberIntVal(0)},
			{cty.ObjectVal(map[string]cty.Value{"a": cty.NumberIntVal(1)}), cty.StringVal("c"), cty.True},
		},
		random: func(r *rand.Rand) []cty.Value {
			m := randomCtyValue(r, cty.Map(cty.Number))
			return []cty.Value{m, cty.StringVal([]string{"a", "b", "a name"}[r.Intn(3)]), cty.NumberIntVal(0)}
		},
	},
	"Log": transcendental(ctystdlib.LogFunc, stdlib.LogFunc),
	"Pow": transcendental(ctystdlib.PowFunc, stdlib.PowFunc),
	"Min": extremes(ctystdlib.MinFunc, stdlib.MinFunc),
	"Max": extremes(ctystdlib.MaxFunc, stdlib.MaxFunc),
	"ParseInt": {
		cty: ctystdlib.ParseIntFunc,
		ten: stdlib.ParseIntFunc,
		cases: [][]cty.Value{
			{cty.StringVal("ff"), cty.NumberIntVal(16)}, {cty.StringVal("+ff"), cty.NumberIntVal(16)},
			{cty.StringVal("Zz"), cty.NumberIntVal(62)}, {cty.StringVal("0xff"), cty.NumberIntVal(16)},
			{cty.StringVal("10"), cty.NumberIntVal(63)}, {cty.StringVal("10"), cty.MustParseNumberVal("16.0")},
			{cty.NumberIntVal(10), cty.NumberIntVal(16)}, {cty.UnknownVal(cty.String), cty.NumberIntVal(16)},
			{cty.StringVal("hunter2").Mark("sensitive"), cty.NumberIntVal(10)},
		},
		random: func(r *rand.Rand) []cty.Value {
			const alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ+-_ ."
			b := make([]byte, 1+r.Intn(12))
			for i := range b {
				b[i] = alphabet[r.Intn(len(alphabet))]
			}
			return []cty.Value{cty.StringVal(string(b)), cty.NumberIntVal(int64(1 + r.Intn(64)))}
		},
	},
	"Coalesce": {
		cty: ctystdlib.CoalesceFunc,
		ten: stdlib.CoalesceFunc,
		cases: [][]cty.Value{
			{cty.NullVal(cty.DynamicPseudoType), cty.NumberIntVal(1)},
			{cty.NullVal(cty.String), cty.NumberIntVal(1)},
			{cty.StringVal(""), cty.StringVal("a")},
			{cty.NumberIntVal(1), cty.UnknownVal(cty.Number)},
			{cty.UnknownVal(cty.Number), cty.NumberIntVal(1)},
			{cty.NullVal(cty.String), cty.NullVal(cty.String)},
			{cty.NullVal(cty.Number).Mark("sensitive"), cty.NumberIntVal(1)},
		},
		random: func(r *rand.Rand) []cty.Value {
			typ := randomCtyType(r, 2)
			args := make([]cty.Value, 1+r.Intn(3))
			for i := range args {
				args[i] = randomCtyValue(r, typ)
			}
			return args
		},
	},
	"MakeTo(number)": makeTo(cty.Number, tenon.Exactly(tenon.NumberType())),
	"MakeTo(string)": makeTo(cty.String, tenon.Exactly(tenon.StringType())),
	"MakeTo(bool)":   makeTo(cty.Bool, tenon.Exactly(tenon.BoolType())),
	"NotEqual":       equality(ctystdlib.NotEqualFunc, stdlib.NotEqualFunc),
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
	case "Signum":
		cp.divergences = []divergence{{
			why: "#218: go-cty v1.19.0's Signum refuses a number that is not a whole number within 64 bits (fixed on its main branch, unreleased)",
			match: func(args []cty.Value) bool {
				n, ok := numberOf(args[0])
				if !ok {
					return false
				}
				_, fits := n.AsInt64()
				return !fits
			},
		}}
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

// makeTo returns the counterpart of MakeToFunc for a type, called with
// strings, numbers and bools as a language's tostring, tonumber and tobool
// are.
func makeTo(typ cty.Type, c tenon.Constraint) counterpart {
	cp := counterpart{
		cty: ctystdlib.MakeToFunc(typ),
		ten: stdlib.MakeToFunc(c),
		cases: [][]cty.Value{
			{cty.StringVal("5")}, {cty.StringVal("true")}, {cty.NumberIntVal(5)}, {cty.True},
			{cty.NullVal(cty.String)}, {cty.UnknownVal(cty.String)}, {cty.StringVal("x")},
			{cty.StringVal("hunter2").Mark("sensitive")},
		},
		random: func(r *rand.Rand) []cty.Value {
			return []cty.Value{randomCtyValue(r, []cty.Type{cty.String, cty.Number, cty.Bool}[r.Intn(3)])}
		},
	}
	if typ == cty.Number {
		cp.cases = append(cp.cases, []cty.Value{cty.StringVal("1p4")}, []cty.Value{cty.StringVal("inf")}, []cty.Value{cty.StringVal("+1")})
		cp.divergences = []divergence{{
			why: "#223: cty reads \"inf\", \"1p4\" and \"+1\" as numbers, and tenon refuses them (NU-022, Appendix B row 8)",
			match: func(args []cty.Value) bool {
				a, _ := args[0].UnmarkDeep()
				if a.Type() != cty.String || !a.IsKnown() || a.IsNull() {
					return false
				}
				_, err := cty.ParseNumberVal(a.AsString())
				return err == nil && tenon.NumberFromText(a.AsString()).IsError()
			},
		}}
	}
	if typ == cty.String {
		cp.divergences = []divergence{{
			why: "a number of 10^21 or more in magnitude, or below 10^-20, converts to its canonical text, scientific past 20 places either way (NU-020), where cty writes every digit (Appendix B row 32)",
			match: func(args []cty.Value) bool {
				n, ok := numberOf(args[0])
				return ok && scientific(n)
			},
		}}
	}
	return cp
}

// scientific reports whether the canonical text of the number n is
// scientific: its magnitude is 10^21 or more, or not zero and below 10^-20.
func scientific(n tenon.Value) bool {
	n, _ = tenon.Unmark(n)
	if n.Equal(tenon.NumberFromInt(0)) {
		return false
	}
	if tenon.LessThan(n, tenon.NumberFromInt(0)).AsBool() {
		n = tenon.Sub(tenon.NumberFromInt(0), n)
	}
	return !tenon.LessThan(n, tenon.NumberFromText("1e21")).AsBool() || tenon.LessThan(n, tenon.NumberFromText("1e-20")).AsBool()
}

// holdsSetOfSeveral reports whether v is or holds, at any depth, a known
// set of two members or more.
func holdsSetOfSeveral(v cty.Value) bool {
	v, _ = v.UnmarkDeep()
	if !v.IsKnown() || v.IsNull() {
		return false
	}
	t := v.Type()
	if t.IsSetType() && v.LengthInt() > 1 {
		return true
	}
	if t.IsListType() || t.IsSetType() || t.IsTupleType() || t.IsMapType() || t.IsObjectType() {
		for it := v.ElementIterator(); it.Next(); {
			if _, e := it.Element(); holdsSetOfSeveral(e) {
				return true
			}
		}
	}
	return false
}

// randomMapOrObject returns a random map or object type.
func randomMapOrObject(r *rand.Rand) cty.Type {
	for {
		t := randomCtyType(r, 2)
		if t.IsMapType() || t.IsObjectType() {
			return t
		}
		if r.Intn(2) == 0 {
			return cty.Map(t)
		}
	}
}

// memberOf returns a member of the collection c, chosen at random.
func memberOf(r *rand.Rand, c cty.Value) cty.Value {
	it := c.ElementIterator()
	for range 1 + r.Intn(c.LengthInt()) {
		it.Next()
	}
	_, m := it.Element()
	return m
}

// numberList is the list of the numbers given.
func numberList(ns ...int64) cty.Value {
	vs := make([]cty.Value, len(ns))
	for i, n := range ns {
		vs[i] = cty.NumberIntVal(n)
	}
	return cty.ListVal(vs)
}

// numberSet is the set of the numbers given.
func numberSet(ns ...int64) cty.Value {
	vs := make([]cty.Value, len(ns))
	for i, n := range ns {
		vs[i] = cty.NumberIntVal(n)
	}
	return cty.SetVal(vs)
}

// setOperation is a counterpart of a set operation over n sets at most.
func setOperation(c function.Function, ten tenon.Function, n int) counterpart {
	return counterpart{
		cty: c,
		ten: ten,
		cases: [][]cty.Value{
			{numberSet(1, 2), numberSet(2, 3)},
			{numberSet(1), cty.SetVal([]cty.Value{cty.StringVal("1")})},
			{numberSet(1), cty.SetVal([]cty.Value{cty.StringVal("a")})},
			{numberSet(1), cty.SetVal([]cty.Value{cty.True})},
			{cty.SetVal([]cty.Value{cty.NumberIntVal(1), cty.UnknownVal(cty.Number)}), numberSet(2)},
			{cty.SetVal([]cty.Value{cty.NumberIntVal(1), cty.UnknownVal(cty.Number)}), cty.SetValEmpty(cty.Number)},
			{cty.SetValEmpty(cty.Number), cty.UnknownVal(cty.Set(cty.Number))},
			{cty.UnknownVal(cty.Set(cty.Number)), numberSet(1)},
			{numberSet(1, 2), numberList(2)},
			{cty.EmptyTupleVal, numberSet(1)},
			{numberSet(1).Mark("sensitive"), numberSet(2)},
		},
		random: func(r *rand.Rand) []cty.Value {
			elem := []cty.Type{cty.Number, cty.String, cty.Bool, cty.List(cty.Number)}[r.Intn(4)]
			args := make([]cty.Value, 1+r.Intn(n))
			for i := range args {
				args[i] = randomCtyValue(r, cty.Set(elem))
			}
			return args
		},
		divergences: []divergence{{
			why: "sets of numbers or strings beside sets of bools: go-cty's unification picks among the types given and finds none, where tenon's unsafe unification gives strings (CV-042, Appendix B row 38)",
			match: func(args []cty.Value) bool {
				var ets []cty.Type
				for _, a := range args {
					if t := a.Type(); t.IsSetType() || t.IsListType() {
						ets = append(ets, t.ElementType())
					}
				}
				return slices.Contains(ets, cty.Bool) && (slices.Contains(ets, cty.Number) || slices.Contains(ets, cty.String))
			},
		}},
	}
}

// numberMap is the map of one number at key k.
func numberMap(k string, n int64) cty.Value {
	return cty.MapVal(map[string]cty.Value{k: cty.NumberIntVal(n)})
}

// abcList is the list ["a", "b", "c"].
func abcList() cty.Value {
	return cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b"), cty.StringVal("c")})
}

// indexing returns the counterpart of HasIndex, Index or Element, called
// with collections of the kinds they take and keys of either kind.
func indexing(c function.Function, ten tenon.Function) counterpart {
	n := cty.NumberIntVal
	abc := cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b"), cty.StringVal("c")})
	cp := counterpart{
		cty: c,
		ten: ten,
		cases: [][]cty.Value{
			{abc, n(1)}, {abc, n(4)}, {abc, n(-1)}, {abc, cty.MustParseNumberVal("1.5")},
			{cty.ListValEmpty(cty.String), n(0)},
			{cty.TupleVal([]cty.Value{cty.StringVal("a"), n(1)}), n(1)},
			{cty.MapVal(map[string]cty.Value{"k": n(1)}), cty.StringVal("k")},
			{cty.ListVal([]cty.Value{cty.StringVal("a"), cty.UnknownVal(cty.String)}), n(0)},
			{cty.UnknownVal(cty.List(cty.String)), n(0)},
			{abc.Mark("sensitive"), n(0)},
		},
		random: func(r *rand.Rand) []cty.Value {
			var coll cty.Value
			switch r.Intn(3) {
			case 0:
				coll = randomCtyValue(r, cty.List(randomCtyType(r, 1)))
			case 1:
				coll = randomCtyValue(r, cty.Map(randomCtyType(r, 1)))
			default:
				coll = randomCtyValue(r, randomCtyType(r, 2))
			}
			key := cty.NumberIntVal(int64(r.Intn(6) - 2))
			if r.Intn(3) == 0 {
				key = cty.StringVal([]string{"a", "k", "a name", "0"}[r.Intn(4)])
			}
			return []cty.Value{coll, key}
		},
	}
	switch ten.Name() {
	case "HasIndex", "Index":
		cp.divergences = append(cp.divergences, superset("an object"))
	}
	if ten.Name() == "Index" {
		cp.divergences = append(cp.divergences, divergence{
			why: "a key no list could have, negative or not a whole number: tenon fails now, whatever the list turns out to be (LB-011), where cty answers unknown",
			match: func(args []cty.Value) bool {
				coll, _ := args[0].UnmarkDeep()
				k, ok := numberOf(args[1])
				return ok && !coll.IsKnown() && (fractionalNumber(k) || tenon.LessThan(k, tenon.NumberFromInt(0)).AsBool())
			},
		})
	}
	return cp
}

// superset is where tenon's function takes what go-cty's refuses, as the
// consumers' own length takes strings and objects and HCL's coll[key] indexes
// objects (D-298): a first argument that is an object, or of another of the
// types given.
func superset(what string, also ...cty.Type) divergence {
	return divergence{
		why: "tenon's takes " + what + ", as the consumers' own functions and HCL's coll[key] do, where go-cty's refuses (D-298)",
		match: func(args []cty.Value) bool {
			t := args[0].Type()
			return t.IsObjectType() || slices.Contains(also, t)
		},
	}
}

// fractionalNumber reports whether the known number n is not a whole number.
func fractionalNumber(n tenon.Value) bool {
	return !tenon.Mod(n, tenon.NumberFromInt(1)).Equal(tenon.NumberFromInt(0))
}

// transcendental returns the counterpart of Log or Pow. go-cty works both out
// through float64, so of known arguments its answer is a binary float good to
// about 16 digits, and its failures are infinities or panics (#219), where
// tenon's answer is correctly rounded to 96 and its failures are codes: what
// is compared is how each answers what is not known, null and marked.
func transcendental(c function.Function, ten tenon.Function) counterpart {
	n := cty.NumberIntVal
	unknown := cty.UnknownVal(cty.Number)
	cp := counterpart{
		cty: c,
		ten: ten,
		cases: [][]cty.Value{
			{n(1000), n(10)}, {n(2), cty.MustParseNumberVal("0.5")}, {n(0), n(10)}, {n(-1), n(2)},
			{unknown, n(10)}, {n(10), unknown}, {unknown.Mark("sensitive"), n(2)},
			{cty.NullVal(cty.Number), n(2)},
		},
		random: func(r *rand.Rand) []cty.Value {
			return []cty.Value{randomCtyValue(r, cty.Number), randomCtyValue(r, cty.Number)}
		},
		divergences: []divergence{{
			why: "both arguments known: go-cty works the result out through float64, good to about 16 digits, and fails with an infinity or a panic (#219), where tenon's is correctly rounded to 96 digits (LN-060, LN-061, Appendix B row 5)",
			match: func(args []cty.Value) bool {
				_, okA := numberOf(args[0])
				_, okB := numberOf(args[1])
				return okA && okB
			},
		}},
	}
	if ten.Name() == "Log" {
		cp.divergences = append(cp.divergences, divergence{
			why: "an argument known to lie outside Log's domain, a number or a base not greater than zero or a base of one: tenon fails now, whatever the other argument turns out to be (LB-011), where go-cty answers unknown",
			match: func(args []cty.Value) bool {
				x, okX := numberOf(args[0])
				b, okB := numberOf(args[1])
				zero := tenon.NumberFromInt(0)
				return okX && !tenon.LessThan(zero, x).AsBool() || okB && (!tenon.LessThan(zero, b).AsBool() || b.Equal(tenon.NumberFromInt(1)))
			},
		})
	}
	return cp
}

// extremes returns the counterpart of Min or Max, called with one number or
// more.
func extremes(c function.Function, ten tenon.Function) counterpart {
	n := cty.NumberIntVal
	return counterpart{
		cty: c,
		ten: ten,
		cases: [][]cty.Value{
			{n(3), cty.MustParseNumberVal("1.5"), n(2)},
			{n(2), cty.UnknownVal(cty.Number).Refine().NumberRangeLowerBound(n(5), true).NewValue()},
			{cty.UnknownVal(cty.Number), n(5)},
			{n(1)},
		},
		random: func(r *rand.Rand) []cty.Value {
			args := make([]cty.Value, 1+r.Intn(3))
			for i := range args {
				args[i] = randomCtyValue(r, cty.Number)
			}
			return args
		},
	}
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
			{cty.ObjectVal(map[string]cty.Value{"password": cty.StringVal("x").Mark("sensitive")}), cty.NullVal(cty.DynamicPseudoType)},
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
		}, {
			why: "an equality decided by null: cty's EqualFunc unmarks its operands deeply and marks the answer with all they hold, where tenon's reads no member and carries the operands' own marks (MK-003, Appendix B row 31)",
			match: func(args []cty.Value) bool {
				for i, a := range args {
					if a.IsKnown() && a.IsNull() && args[1-i].ContainsMarked() {
						return true
					}
				}
				return false
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
	var have []string
	for name, cp := range counterparts {
		// MakeToFunc is a factory, compared for each type it is made for.
		if base, _, made := strings.Cut(name, "("); made {
			if cp.ten.Name() != base || base != "MakeTo" {
				t.Errorf("counterparts[%q] holds %s", name, cp.ten.Name())
			}
			continue
		}
		if cp.ten.Name() != name {
			t.Errorf("counterparts[%q] holds %s", name, cp.ten.Name())
		}
		have = append(have, name)
	}
	if slices.Sort(have); !slices.Equal(declared, have) {
		t.Errorf("the stdlib package declares %v, and counterparts holds %v", declared, have)
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
		if !tenon.Identical(tr, want) && !identicalAsCty(b, tr, want) {
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

// identicalAsCty reports whether tenon's answer ans is want once it crosses
// to cty and back, as cty holds it: cty hands a container's marks to each
// member read out of it, so crossing gives the members those marks, where
// tenon's members keep their own (LB-020) and a member taken from an
// unmarked argument carries none.
func identicalAsCty(b ctytenon.Bridge, ans, want tenon.Value) bool {
	c, err := b.ToCty(ans)
	if err != nil {
		return false
	}
	back, err := b.FromCty(c)
	return err == nil && tenon.Identical(back, want)
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
