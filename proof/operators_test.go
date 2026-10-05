package proof_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/ctytenon"
	"github.com/kmoneil/tenon/stdlib"
	"github.com/zclconf/go-cty/cty"
)

// operators are HCL's operations, each beside the library function that
// takes its place.
var operators = []struct {
	op *hclsyntax.Operation
	f  tenon.Function
}{
	{hclsyntax.OpLogicalOr, stdlib.OrFunc},
	{hclsyntax.OpLogicalAnd, stdlib.AndFunc},
	{hclsyntax.OpLogicalNot, stdlib.NotFunc},
	{hclsyntax.OpEqual, stdlib.EqualFunc},
	{hclsyntax.OpNotEqual, stdlib.NotEqualFunc},
	{hclsyntax.OpGreaterThan, stdlib.GreaterThanFunc},
	{hclsyntax.OpGreaterThanOrEqual, stdlib.GreaterThanOrEqualToFunc},
	{hclsyntax.OpLessThan, stdlib.LessThanFunc},
	{hclsyntax.OpLessThanOrEqual, stdlib.LessThanOrEqualToFunc},
	{hclsyntax.OpAdd, stdlib.AddFunc},
	{hclsyntax.OpSubtract, stdlib.SubtractFunc},
	{hclsyntax.OpMultiply, stdlib.MultiplyFunc},
	{hclsyntax.OpDivide, stdlib.DivideFunc},
	{hclsyntax.OpModulo, stdlib.ModuloFunc},
	{hclsyntax.OpNegate, stdlib.NegateFunc},
}

// swapIn puts the library's functions in place of HCL's operations until
// the test ends.
func swapIn(t *testing.T) {
	t.Helper()
	b := ctytenon.Bridge{MarkFromCty: markFromCty, MarkToCty: markToCty}
	for _, o := range operators {
		f, err := b.FunctionToCty(o.f, tenon.Unsafe)
		if err != nil {
			t.Fatalf("%s does not cross: %v", o.f.Name(), err)
		}
		old := o.op.Impl
		o.op.Impl = f
		t.Cleanup(func() { o.op.Impl = old })
	}
}

// sensitive is the tenon mark Terraform's "sensitive" crosses as.
type sensitive struct{}

func (sensitive) MarkID() string                 { return "sensitive" }
func (sensitive) Propagation() tenon.Propagation { return tenon.Propagate }
func (sensitive) Redacting() bool                { return true }

func markFromCty(m any) (tenon.Mark, bool) {
	if m == "sensitive" {
		return sensitive{}, true
	}
	return nil, false
}

func markToCty(m tenon.Mark) (any, bool) {
	if m == (sensitive{}) {
		return "sensitive", true
	}
	return nil, false
}

// variables are what the expressions read.
var variables = map[string]cty.Value{
	"unknown":   cty.UnknownVal(cty.Number),
	"positive":  cty.UnknownVal(cty.Number).Refine().NumberRangeLowerBound(cty.NumberIntVal(1), true).NewValue(),
	"nothing":   cty.NullVal(cty.String),
	"big":       cty.MustParseNumberVal("1e200"),
	"secret":    cty.NumberIntVal(3).Mark("sensitive"),
	"maybenull": cty.UnknownVal(cty.String),
}

// evaluate evaluates src, giving its value's Go syntax, or its first error,
// or the panic HCL's evaluation let through.
func evaluate(t *testing.T, src string) (out string) {
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
	v, diags := expr.Value(&hcl.EvalContext{Variables: variables})
	if diags.HasErrors() {
		return "error: " + diags[0].Detail
	}
	return v.GoString()
}

// proofs are expressions, what HCL as it is answers (stock, a part of its
// answer where the whole is long), what it answers with tenon's operators
// (tenon, the whole answer), and why the two differ, where they do.
var proofs = []struct {
	src, stock, tenon, why string
}{
	{`1 + 2`, `cty.NumberIntVal(3)`, `cty.NumberIntVal(3)`, ""},
	{`0.1 + 0.2`, `cty.MustParseNumberVal("0.3")`, `cty.MustParseNumberVal("0.3")`, ""},
	{`1 / 3`, `0.33333333333333333333333333333333333333333333333333333333333333333333333333333333333333333333333333333`,
		`cty.MustParseNumberVal("0.` + strings.Repeat("3", 96) + `")`,
		"a quotient that does not terminate is 96 significant digits (NU-012), where cty's is 512 binary bits"},
	{`1 / 0`, `cty.NumberFloatVal(+Inf)`, `error: Error during operation: number.divide_by_zero: a number cannot be divided by zero.`,
		"division by zero fails (NU-013, Appendix B row 6)"},
	{`5 % 0`, `cty.NumberIntVal(5)`, `error: Error during operation: number.modulo_by_zero: a number has no remainder modulo zero.`,
		"modulo by zero fails (NU-014, Appendix B row 30)"},
	{`(1/0) % 2`, `panic in function implementation`, `error: Error during operation: number.divide_by_zero: a number cannot be divided by zero.`,
		"go-cty #90: cty's modulo of an infinity panics, which HCL reports with a goroutine's stack; tenon has no infinity"},
	{`1e200 % 7`, `cty.NumberIntVal(0)`, `cty.NumberIntVal(2)`, "a remainder is exact (NU-017), where cty's comes from 512 binary bits"},
	{`0.9 % 0.3`, `0.29999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999`, `cty.NumberIntVal(0)`,
		"a remainder is exact (NU-017), where cty's comes from 512 binary bits"},
	{`big + 1 - big`, `cty.NumberIntVal(0)`, `cty.NumberIntVal(0)`, ""},
	{`-positive`, `cty.UnknownVal(cty.Number).RefineNotNull()`,
		`cty.UnknownVal(cty.Number).Refine().NotNull().NumberUpperBound(cty.NumberIntVal(-1), true).NewValue()`,
		"an unknown operand's bounds carry into the answer (UN-007), where cty drops them"},
	{`positive + 1`, `cty.UnknownVal(cty.Number).RefineNotNull()`,
		`cty.UnknownVal(cty.Number).Refine().NotNull().NumberLowerBound(cty.NumberIntVal(2), true).NewValue()`,
		"an unknown operand's bounds carry into the answer (UN-007), where cty drops them"},
	{`positive > 0`, `cty.True`, `cty.True`, ""},
	{`"10" > "9"`, `cty.True`, `cty.True`, ""},
	{`"1p4" + 0`, `cty.NumberIntVal(16)`, `cty.NumberIntVal(16)`, ""},
	{`nothing == null`, `cty.True`, `cty.True`, ""},
	{`null == null`, `cty.True`, `cty.True`, ""},
	{`[null] == [null]`, `cty.True`, `cty.True`, ""},
	{`{a = null} != {a = null}`, `cty.False`, `cty.False`, ""},
	{`maybenull == null`, `cty.UnknownVal(cty.Bool).RefineNotNull()`, `cty.UnknownVal(cty.Bool).RefineNotNull()`, ""},
	{`1 == "1"`, `cty.False`, `cty.False`, ""},
	{`secret + 1`, `cty.NumberIntVal(4).Mark("sensitive")`, `cty.NumberIntVal(4).Mark("sensitive")`, ""},
	{`true && false`, `cty.False`, `cty.False`, ""},
	{`!true`, `cty.False`, `cty.False`, ""},
	{`null && true`, `cty.False`, `cty.False`, ""},
	{`unknown > 1`, `cty.UnknownVal(cty.Bool).RefineNotNull()`, `cty.UnknownVal(cty.Bool).RefineNotNull()`, ""},
	{`unknown == maybenull`, `cty.UnknownVal(cty.Bool).RefineNotNull()`, `cty.False`,
		"a number and a string are never equal (EQ-005), known or not, where cty and HCL's specification answer unknown (Appendix B row 53)"},
	{`[unknown, 1] == [1, 2]`, `cty.UnknownVal(cty.Bool).RefineNotNull()`, `cty.False`,
		"a pair of elements known unequal decides (EQ-003), where cty answers unknown at the first element not known (Appendix B row 53)"},
	{`unknown * 0`, `cty.UnknownVal(cty.Number).RefineNotNull()`, `cty.NumberIntVal(0)`,
		"every number times zero is zero, tenon having no infinity (NU-002, UN-007), where cty and HCL's specification answer unknown (Appendix B row 54)"},
	{`[nothing] == [null]`, `cty.False`, `cty.True`,
		"an untyped null takes the other side's type first, in a tuple as at the top (LN-011), where cty compares a string with no type (Appendix B row 56)"},
	{`"<${-0}>"`, `cty.StringVal("<-0>")`, `cty.StringVal("<0>")`,
		"tenon has one zero (NU-001), where cty keeps math/big's signed zero (Appendix B row 55)"},
	{`"inf" + 0`, `cty.NumberFloatVal(+Inf)`, `error: Error during operation: encode.not_a_number: +Inf is not a number.`,
		"HCL converts the string with go-cty, to an infinity, which has no tenon number to cross to (NU-002, Appendix B rows 6 and 8)"},
}

// TestOperators evaluates each expression with HCL as it is and with tenon's
// operators in place, and holds each answer to what it should be: the same,
// or different for the reason given.
func TestOperators(t *testing.T) {
	stock := make([]string, len(proofs))
	for i, p := range proofs {
		stock[i] = evaluate(t, p.src)
	}
	swapIn(t)
	for i, p := range proofs {
		got := evaluate(t, p.src)
		switch {
		case !strings.Contains(stock[i], p.stock):
			t.Errorf("%s: HCL as it is answers %s, not %s; go-cty has changed", p.src, stock[i], p.stock)
		case got != p.tenon:
			t.Errorf("%s: with tenon's operators, HCL answers %s, want %s", p.src, got, p.tenon)
		case (got == stock[i]) != (p.why == ""):
			t.Errorf("%s: answers %s and %s; a difference, and only a difference, gives its reason", p.src, stock[i], got)
		}
	}
}
