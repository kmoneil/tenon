package proof_test

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/stdlib"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	ctystdlib "github.com/zclconf/go-cty/cty/function/stdlib"
)

// library is every function of tenon's library beside go-cty's, by the
// name a language built on HCL gives it, Terraform's where it has one.
var library = []struct {
	name string
	cty  function.Function
	ten  tenon.Function
}{
	{"abs", ctystdlib.AbsoluteFunc, stdlib.AbsoluteFunc},
	{"assertnotnull", ctystdlib.AssertNotNullFunc, stdlib.AssertNotNullFunc},
	{"ceil", ctystdlib.CeilFunc, stdlib.CeilFunc},
	{"chomp", ctystdlib.ChompFunc, stdlib.ChompFunc},
	{"chunklist", ctystdlib.ChunklistFunc, stdlib.ChunklistFunc},
	{"coalesce", ctystdlib.CoalesceFunc, stdlib.CoalesceFunc},
	{"coalescelist", ctystdlib.CoalesceListFunc, stdlib.CoalesceListFunc},
	{"compact", ctystdlib.CompactFunc, stdlib.CompactFunc},
	{"concat", ctystdlib.ConcatFunc, stdlib.ConcatFunc},
	{"contains", ctystdlib.ContainsFunc, stdlib.ContainsFunc},
	{"csvdecode", ctystdlib.CSVDecodeFunc, stdlib.CSVDecodeFunc},
	{"distinct", ctystdlib.DistinctFunc, stdlib.DistinctFunc},
	{"element", ctystdlib.ElementFunc, stdlib.ElementFunc},
	{"flatten", ctystdlib.FlattenFunc, stdlib.FlattenFunc},
	{"floor", ctystdlib.FloorFunc, stdlib.FloorFunc},
	{"format", ctystdlib.FormatFunc, stdlib.FormatFunc},
	{"formatdate", ctystdlib.FormatDateFunc, stdlib.FormatDateFunc},
	{"formatlist", ctystdlib.FormatListFunc, stdlib.FormatListFunc},
	{"hasindex", ctystdlib.HasIndexFunc, stdlib.HasIndexFunc},
	{"indent", ctystdlib.IndentFunc, stdlib.IndentFunc},
	{"index", ctystdlib.IndexFunc, stdlib.IndexFunc},
	{"int", ctystdlib.IntFunc, stdlib.IntFunc},
	{"join", ctystdlib.JoinFunc, stdlib.JoinFunc},
	{"jsondecode", ctystdlib.JSONDecodeFunc, stdlib.JSONDecodeFunc},
	{"jsonencode", ctystdlib.JSONEncodeFunc, stdlib.JSONEncodeFunc},
	{"keys", ctystdlib.KeysFunc, stdlib.KeysFunc},
	{"length", ctystdlib.LengthFunc, stdlib.LengthFunc},
	{"log", ctystdlib.LogFunc, stdlib.LogFunc},
	{"lookup", ctystdlib.LookupFunc, stdlib.LookupFunc},
	{"lower", ctystdlib.LowerFunc, stdlib.LowerFunc},
	{"max", ctystdlib.MaxFunc, stdlib.MaxFunc},
	{"merge", ctystdlib.MergeFunc, stdlib.MergeFunc},
	{"min", ctystdlib.MinFunc, stdlib.MinFunc},
	{"parseint", ctystdlib.ParseIntFunc, stdlib.ParseIntFunc},
	{"pow", ctystdlib.PowFunc, stdlib.PowFunc},
	{"range", ctystdlib.RangeFunc, stdlib.RangeFunc},
	{"regex", ctystdlib.RegexFunc, stdlib.RegexFunc},
	{"regexall", ctystdlib.RegexAllFunc, stdlib.RegexAllFunc},
	{"regexreplace", ctystdlib.RegexReplaceFunc, stdlib.RegexReplaceFunc},
	{"replace", ctystdlib.ReplaceFunc, stdlib.ReplaceFunc},
	{"reverse", ctystdlib.ReverseListFunc, stdlib.ReverseListFunc},
	{"sethaselement", ctystdlib.SetHasElementFunc, stdlib.SetHasElementFunc},
	{"setintersection", ctystdlib.SetIntersectionFunc, stdlib.SetIntersectionFunc},
	{"setproduct", ctystdlib.SetProductFunc, stdlib.SetProductFunc},
	{"setsubtract", ctystdlib.SetSubtractFunc, stdlib.SetSubtractFunc},
	{"setsymmetricdifference", ctystdlib.SetSymmetricDifferenceFunc, stdlib.SetSymmetricDifferenceFunc},
	{"setunion", ctystdlib.SetUnionFunc, stdlib.SetUnionFunc},
	{"signum", ctystdlib.SignumFunc, stdlib.SignumFunc},
	{"slice", ctystdlib.SliceFunc, stdlib.SliceFunc},
	{"sort", ctystdlib.SortFunc, stdlib.SortFunc},
	{"split", ctystdlib.SplitFunc, stdlib.SplitFunc},
	{"strlen", ctystdlib.StrlenFunc, stdlib.StrlenFunc},
	{"strrev", ctystdlib.ReverseFunc, stdlib.ReverseFunc},
	{"substr", ctystdlib.SubstrFunc, stdlib.SubstrFunc},
	{"timeadd", ctystdlib.TimeAddFunc, stdlib.TimeAddFunc},
	{"title", ctystdlib.TitleFunc, stdlib.TitleFunc},
	{"tonumber", ctystdlib.MakeToFunc(cty.Number), stdlib.MakeToFunc(tenon.Exactly(tenon.NumberType()))},
	{"tostring", ctystdlib.MakeToFunc(cty.String), stdlib.MakeToFunc(tenon.Exactly(tenon.StringType()))},
	{"toset", ctystdlib.MakeToFunc(cty.Set(cty.DynamicPseudoType)), stdlib.MakeToFunc(tenon.SetOf(tenon.Any()))},
	{"trim", ctystdlib.TrimFunc, stdlib.TrimFunc},
	{"trimprefix", ctystdlib.TrimPrefixFunc, stdlib.TrimPrefixFunc},
	{"trimspace", ctystdlib.TrimSpaceFunc, stdlib.TrimSpaceFunc},
	{"trimsuffix", ctystdlib.TrimSuffixFunc, stdlib.TrimSuffixFunc},
	{"upper", ctystdlib.UpperFunc, stdlib.UpperFunc},
	{"values", ctystdlib.ValuesFunc, stdlib.ValuesFunc},
	{"zipmap", ctystdlib.ZipmapFunc, stdlib.ZipmapFunc},
}

// tables returns HCL's function table of go-cty's functions, and of
// tenon's crossed by ctytenon, as a host built on HCL would set either.
func tables(t *testing.T) (stock, swapped map[string]function.Function) {
	t.Helper()
	b := bridge()
	stock, swapped = map[string]function.Function{}, map[string]function.Function{}
	for _, f := range library {
		crossed, err := b.FunctionToCty(f.ten, tenon.Unsafe)
		if err != nil {
			t.Fatalf("%s does not cross: %v", f.name, err)
		}
		stock[f.name], swapped[f.name] = f.cty, crossed
	}
	return stock, swapped
}

// evaluateWith evaluates src with the function table, giving its value's
// Go syntax, or its first error, or the panic HCL's evaluation let through.
func evaluateWith(t *testing.T, src string, functions map[string]function.Function) (out string) {
	t.Helper()
	expr, diags := hclsyntax.ParseExpression([]byte(src), "proof.hcl", hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatalf("%s does not parse: %v", src, diags)
	}
	defer func() {
		if r := recover(); r != nil {
			out = fmt.Sprintf("panic: %v", r)
		}
	}()
	v, diags := expr.Value(&hcl.EvalContext{Variables: variables, Functions: functions})
	if diags.HasErrors() {
		return "error: " + diags[0].Detail
	}
	return v.GoString()
}

// TestFunctions evaluates each expression of the corpus with HCL's
// operators and go-cty's functions as they are, and again with tenon's
// operators and functions in their place, and holds each answer to what it
// should be: the same, or different for the reason given. Every function
// of the library is called by at least one.
func TestFunctions(t *testing.T) {
	stock, swapped := tables(t)
	answers := make([][2]string, len(corpus))
	for i, c := range corpus {
		answers[i][0] = evaluateWith(t, c.src, stock)
	}
	swapIn(t)
	for i, c := range corpus {
		answers[i][1] = evaluateWith(t, c.src, swapped)
	}
	if os.Getenv("PROOF_RECORD") != "" {
		for i, c := range corpus {
			fmt.Printf("\t{%s, %s, %s, %s},\n", strconv.Quote(c.src), strconv.Quote(answers[i][0]), strconv.Quote(answers[i][1]), strconv.Quote(c.why))
		}
		return
	}
	for i, c := range corpus {
		got, was := answers[i][1], answers[i][0]
		switch {
		case !strings.Contains(was, c.stock):
			t.Errorf("%s: HCL with go-cty's functions answers %s, not %s; go-cty has changed", c.src, was, c.stock)
		case got != c.tenon:
			t.Errorf("%s: with tenon's functions, HCL answers %s, want %s", c.src, got, c.tenon)
		case (got == was) != (c.why == ""):
			t.Errorf("%s: answers %s and %s; a difference, and only a difference, gives its reason", c.src, was, got)
		}
	}
	for _, f := range library {
		if !slices.ContainsFunc(corpus, func(c entry) bool { return strings.Contains(c.src, f.name+"(") }) {
			t.Errorf("no expression of the corpus calls %s", f.name)
		}
	}
}

// entry is an expression of the corpus: what HCL answers with go-cty's
// functions (stock, a part of its answer where the whole is long), what it
// answers with tenon's (tenon, the whole answer), and why the two differ,
// where they do.
type entry struct {
	src, stock, tenon, why string
}
