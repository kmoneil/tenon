package tenon_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/conformance"
)

// codePattern matches a diagnostic code: parts of lowercase letters, digits
// and underscores, joined by dots.
var codePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

// specCodes are the diagnostic codes that the specification names, in sorted
// order. The registry holds these and nothing else.
var specCodes = []string{
	"bool.invalid_syntax",
	"convert.length_mismatch",
	"convert.missing_attribute",
	"convert.no_common_type",
	"convert.no_conversion",
	"convert.unexpected_attribute",
	"convert.unsafe",
	"decode.length_mismatch",
	"decode.marked",
	"decode.not_known",
	"decode.null",
	"decode.out_of_range",
	"decode.unmarshal_failed",
	"encode.inexact",
	"encode.marshal_failed",
	"encode.not_a_number",
	"encode.untyped_nil",
	"map.duplicate_key",
	"number.divide_by_zero",
	"number.invalid_syntax",
	"number.modulo_by_zero",
	"number.out_of_range",
	"number.too_long",
	"operation.null_operand",
	"operation.wrong_type",
	"range.contradiction",
	"serialize.malformed",
	"serialize.not_canonical",
	"serialize.not_known",
	"serialize.redacted",
	"serialize.too_large",
	"serialize.unencodable_capsule",
	"serialize.unencodable_mark",
	"serialize.unknown_capsule",
	"serialize.unknown_mark",
	"serialize.unsupported_version",
	"string.invalid_utf8",
	"unify.no_common_constraint",
}

func TestConformance_ER007_DiagnosticCodes(t *testing.T) {
	conformance.Covers(t, "ER-007")
	for _, c := range specCodes {
		if !codePattern.MatchString(c) {
			t.Errorf("%q is not an area and a name joined by a dot", c)
		}
	}

	// The codes that tenon reports come from the registry.
	num := tenon.NumberType()
	one := tenon.NumberFromInt(1)
	for _, v := range []tenon.Value{
		tenon.String("\xff"),
		tenon.NumberFromText("x"),
		tenon.NumberFromText("1e1000000"),
		tenon.MapVal(num, map[string]tenon.Value{"caf\u00e9": one, "cafe\u0301": one, "a\xff": one}),
	} {
		for _, d := range v.Diagnostics() {
			if !slices.Contains(specCodes, string(d.Code)) {
				t.Errorf("%v carries the code %q, which is not in the registry", v, d.Code)
			}
		}
	}

	// Codes are defined in codes.go and spelled out nowhere else, gotenon
	// included, which sits at the boundary and mints nothing of its own.
	// Test files are left out: they hold file names, such as that of
	// usage.go, which read like codes.
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	gotenonFiles, err := filepath.Glob(filepath.Join("gotenon", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	scanned := 0
	for _, name := range append(files, gotenonFiles...) {
		if name == "codes.go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		scanned++
		for _, lit := range stringLiterals(t, name) {
			if codePattern.MatchString(lit) {
				t.Errorf("%s spells out the code %q; use the constant from codes.go", name, lit)
			}
		}
	}
	// A glob that stopped matching would pass as cleanly.
	if scanned < 20 {
		t.Errorf("only %d files were scanned; the packages hold more", scanned)
	}
}

func TestConformance_DI001_CodesHaveOneForm(t *testing.T) {
	conformance.Covers(t, "DI-001")
	// A code is namespaced, whoever mints it.
	for _, c := range []tenon.Code{"", "nodot", ".name", "area.", "Area.name", "area.Name", "area name", "area..name", "1area.name", "_area.name", "area.na-me"} {
		mustPanicUsage(t, "not an area and a name", func() {
			tenon.ErrorVal(tenon.Diagnostic{Code: c, Message: "a message"})
		})
	}
	for _, c := range []tenon.Code{"myapp.unknown_setting", "a.b", "app9.sub_area.name_2"} {
		if v := tenon.ErrorVal(tenon.Diagnostic{Code: c, Message: "a message"}); !v.IsError() {
			t.Errorf("the code %q was refused", c)
		}
	}
}

func TestConformance_DI002_TheRegistryListsEveryCode(t *testing.T) {
	conformance.Covers(t, "DI-002")
	// specCodes mirrors the specification's appendix of codes, which is
	// generated from codes.go and checked against the rules that name them, so
	// a code that codes.go gains or loses has to be accounted for here too.
	if got := stringLiterals(t, "codes.go"); !slices.Equal(got, specCodes) {
		t.Errorf("codes.go defines\n%q\nwant\n%q", got, specCodes)
	}
	// The committed record holds every code ever declared, `make codes`
	// keeping it, and rulecheck refuses a code that disappears or returns
	// from withdrawal; here the record's codes that are not withdrawn must
	// be exactly the registry's.
	var record struct {
		Format int    `json:"format"`
		About  string `json:"about"`
		Codes  []struct {
			Code      string `json:"code"`
			Withdrawn bool   `json:"withdrawn"`
		} `json:"codes"`
	}
	data, err := os.ReadFile(filepath.Join("conformance", "codes.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &record); err != nil || record.Format != 1 {
		t.Fatalf("conformance/codes.json is not a code record: %v", err)
	}
	var current []string
	for _, c := range record.Codes {
		if !c.Withdrawn {
			current = append(current, c.Code)
		}
	}
	slices.Sort(current)
	if !slices.Equal(current, specCodes) {
		t.Errorf("conformance/codes.json records\n%q\nwant\n%q", current, specCodes)
	}
	// Each condition is reported with its code.
	num := tenon.NumberType()
	for _, tt := range []struct {
		v    tenon.Value
		code tenon.Code
	}{
		{tenon.Div(tenon.NumberFromInt(1), tenon.NumberFromInt(0)), tenon.CodeNumberDivideByZero},
		{tenon.Mod(tenon.NumberFromInt(1), tenon.NumberFromInt(0)), tenon.CodeNumberModuloByZero},
		{tenon.NumberFromText("x"), tenon.CodeNumberInvalidSyntax},
		{tenon.NumberFromText("1e1000000"), tenon.CodeNumberOutOfRange},
		{tenon.String("\xff"), tenon.CodeStringInvalidUTF8},
		{tenon.Add(tenon.NullVal(num), tenon.NumberFromInt(1)), tenon.CodeOperationNullOperand},
		{tenon.Narrow(tenon.Unknown(num), tenon.NotNull(), tenon.Null()), tenon.CodeRangeContradiction},
		{tenon.Convert(tenon.String("x"), tenon.Exactly(num), tenon.Unsafe), tenon.CodeNumberInvalidSyntax},
		{tenon.Convert(tenon.String("5"), tenon.Exactly(num), tenon.Safe), tenon.CodeConvertUnsafe},
	} {
		if !tt.v.IsError() || tt.v.Diagnostics()[0].Code != tt.code {
			t.Errorf("%v: want the code %s", tt.v, tt.code)
		}
	}
}

func TestConformance_DI003_MessagesAreForPeople(t *testing.T) {
	conformance.Covers(t, "DI-003")
	code := tenon.CodeNumberDivideByZero
	mustPanicUsage(t, "needs a message", func() { tenon.ErrorVal(tenon.Diagnostic{Code: code}) })
	for _, m := range []string{"\xff", "a\xed\xa0\x80b"} { // a stray byte, and a surrogate
		mustPanicUsage(t, "not valid UTF-8", func() { tenon.ErrorVal(tenon.Diagnostic{Code: code, Message: m}) })
	}
	// A message in any script, not normalized, is a message.
	if v := tenon.ErrorVal(tenon.Diagnostic{Code: code, Message: "cafe\u0301 \u2260 caf\u00e9"}); !v.IsError() {
		t.Error("a message of Unicode text was refused")
	}
}

// stringLiterals returns the string literals of a Go file, sorted.
func stringLiterals(t *testing.T, path string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var lits []string
	ast.Inspect(file, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			if s, err := strconv.Unquote(lit.Value); err == nil {
				lits = append(lits, s)
			}
		}
		return true
	})
	slices.Sort(lits)
	return lits
}
