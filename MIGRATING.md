# Moving a function library from go-cty to tenon

Package `stdlib` is go-cty's `cty/function/stdlib`, function for function, as
tenon functions. Each is a variable named as go-cty names it, so a host's table
moves by its import path:

```go
import "github.com/kmoneil/tenon/stdlib"

functions := map[string]tenon.Function{
	"upper":      stdlib.UpperFunc,
	"jsonencode": stdlib.JSONEncodeFunc,
}
```

A host still evaluating with go-cty, HCL's evaluator among them, puts these in
its table crossed by ctytenon, one at a time if it likes:

```go
f, err := ctytenon.Bridge{}.FunctionToCty(stdlib.UpperFunc, tenon.Unsafe)
functions["upper"] = f // a map[string]function.Function, as hcl.EvalContext holds
```

The `proof` module in this repository does that for every function and HCL
operator, and holds each answer beside go-cty's: [proof/doc.go](proof/doc.go)
says what crossing gives and does not, and its corpus names every difference
below in an expression. What follows is each function and how its answers
differ from go-cty v1.19's. Where the table says nothing, the answers are the
same. Throughout:

- **Numbers are exact.** A number is a decimal, never a 512-bit float: no
  infinity, division and modulo by zero fail, a quotient that does not end is
  96 significant digits, and a string reads as a number by tenon's grammar
  (`"1p4"`, `"Inf"` and `"+5"` are none).
- **Failures have codes.** A failure is an error value with a code such as
  `function.invalid_argument`, located at the argument and the member within
  it, never Go's text or a panic's stack; and what the known arguments settle
  fails now, beside arguments not known yet, where go-cty answers unknown.
- **Unknown answers say what is known.** An answer not known yet keeps what
  the arguments settle, a number's bounds, a string's prefix, a list's length,
  and is never null where the function cannot answer null.
- **Text is Unicode 15.0.0's, on every toolchain.** Lengths, substrings,
  searches and trims go by grapheme cluster, and case by Unicode's full
  mappings; patterns have Go 1.26's syntax with Unicode 15.0.0's classes.
- **Bounds come before the work.** A function whose answer would outgrow its
  arguments, by 64 times their size and 64 KiB, fails with
  `function.too_large` before making any of it.

The rules named are the specification's; [CONFORMANCE.md](CONFORMANCE.md)
lists them.

## Operators

| go-cty | tenon | What differs |
| --- | --- | --- |
| `AddFunc`, `SubtractFunc`, `MultiplyFunc` | the same names | Exact; an unknown operand's bounds carry into the answer, and an unknown times zero is zero (`LN-001`) |
| `DivideFunc`, `ModuloFunc` | the same names | By zero fails, known or not, where go-cty answers an infinity or the dividend; a quotient is 96 significant digits; a remainder is exact (`LN-001`, `NU-012` to `NU-014`) |
| `NegateFunc` | `NegateFunc` | Exact; one zero |
| `LessThanFunc` and the other orderings | the same names | Exact on numbers of any size (`LN-010`) |
| `EqualFunc`, `NotEqualFunc` | the same names | A language's untyped `null` takes the other side's type first, at any depth, so `[n] == [null]` is true; operands of two types, or known to differ in a member, are unequal though not known (`LN-011`, `EQ-003`, `EQ-005`) |
| `NotFunc`, `AndFunc`, `OrFunc` | the same names | None |

## Numbers

| go-cty | tenon | What differs |
| --- | --- | --- |
| `AbsoluteFunc`, `SignumFunc` | the same names | Exact; an unknown's answer keeps its bounds (`LN-030`, `LN-031`) |
| `CeilFunc`, `FloorFunc`, `IntFunc` | the same names | Exact for numbers of any size |
| `LogFunc`, `PowFunc` | the same names | Correctly rounded to 96 significant digits, where go-cty goes through a float64; an argument outside the domain fails with `number.domain` (`LN-060`, `LN-061`) |
| `MinFunc`, `MaxFunc` | the same names | Exact |
| `ParseIntFunc` | `ParseIntFunc` | Exact for any number of digits; a bad base or digit fails at it (`LN-050`) |

## General

| go-cty | tenon | What differs |
| --- | --- | --- |
| `AssertNotNullFunc` | `AssertNotNullFunc` | A value not known yet answers not null (`LN-083`) |
| `CoalesceFunc` | `CoalesceFunc` | None |
| `MakeToFunc(t)` | `MakeToFunc(c)` | Takes a constraint; a number converts to its canonical text, `1e30` not 31 digits (`LN-085`, `NU-020`) |

## Collections and sets

| go-cty | tenon | What differs |
| --- | --- | --- |
| `LengthFunc` | `LengthFunc` | Also takes a string, counting grapheme clusters, and an object, counting attributes (`LC-001`) |
| `IndexFunc`, `ElementFunc`, `HasIndexFunc` | the same names | A key no list could have fails now (`LC-003`) |
| `SliceFunc`, `ReverseListFunc`, `ConcatFunc`, `ChunklistFunc` | the same names | None |
| `FlattenFunc` | `FlattenFunc` | A set's members come in the canonical order (`EQ-044`) |
| `CompactFunc`, `DistinctFunc`, `CoalesceListFunc`, `KeysFunc`, `ValuesFunc`, `LookupFunc` | the same names | None |
| `ZipmapFunc` | `ZipmapFunc` | A key known to be null fails now |
| `MergeFunc` | `MergeFunc` | An untyped null beside maps of one type takes their type; every argument's marks reach the answer, a null's too (`LC-035`) |
| `ContainsFunc`, `SetHasElementFunc` | the same names | The value is converted to the members' type first; nulls of two types are not equal; a set may hold null (`LC-040`, `LC-043`) |
| `SetUnionFunc` and the other set operations | the same names | Members in the canonical order; numbers and bools unify to strings under the unsafe policy |
| `RangeFunc` | `RangeFunc` | Every element exact, where go-cty's steps drift; a step of zero fails however it is written (`LC-050` to `LC-053`) |
| `SetProductFunc` | `SetProductFunc` | HCL's `[]` is the empty tuple; a product past 1,048,576 tuples fails (`LC-054`, `LC-056`) |

## Text

| go-cty | tenon | What differs |
| --- | --- | --- |
| `UpperFunc`, `LowerFunc`, `TitleFunc` | the same names | Unicode's full case mapping: `upper("straße")` is `"STRASSE"`, `lower("ΟΔΟΣ")` ends in the final sigma (`LS-005` to `LS-007`) |
| `StrlenFunc`, `ReverseFunc`, `SubstrFunc` | the same names | Grapheme clusters of Unicode 15.0.0, whatever the toolchain; a length of zero is empty whatever the offset (`LS-008` to `LS-011`) |
| `SplitFunc`, `ReplaceFunc`, `TrimPrefixFunc`, `TrimSuffixFunc`, `TrimFunc`, `TrimSpaceFunc` | the same names | Match and cut only where clusters begin and end; an empty separator splits clusters; `Replace`'s answer is bounded (`LS-012` to `LS-020`) |
| `ChompFunc` | `ChompFunc` | None |
| `IndentFunc` | `IndentFunc` | A count that is not a whole number not less than zero fails at it, where go-cty panics; the answer is bounded (`LS-022`) |
| `JoinFunc`, `SortFunc` | the same names | A null element fails at it, beside parts not known yet (`LS-024` to `LS-027`) |
| `FormatFunc` | `FormatFunc` | Numbers from their exact value, ties half to even; `%v` of a number its canonical text, `1000000` not `1e+06`; widths count clusters, at most 10,000; seven small fixes where go-cty disagrees with Go and its documentation; a bad format fails with `format.invalid_syntax`; the answer bounded (`LF-001` to `LF-017`, `LF-022`) |
| `FormatListFunc` | `FormatListFunc` | The format is checked even where a list is empty; a failure is located at the member (`LF-018` to `LF-021`) |
| `RegexFunc`, `RegexAllFunc` | the same names | Go 1.26's syntax and Unicode 15.0.0's classes on every toolchain; a group named twice is refused; the answer bounded (`LR-001` to `LR-009`) |
| `RegexReplaceFunc` | `RegexReplaceFunc` | A reference to a group the pattern does not have fails, `$1x` naming the group `1x` (write `${1}x`); the answer bounded (`LR-010` to `LR-014`) |

## Encodings and time

| go-cty | tenon | What differs |
| --- | --- | --- |
| `JSONEncodeFunc` | `JSONEncodeFunc` | go-cty's escapes and positional numbers, but exact: `1/3` is its 96 digits; one zero; sets in the canonical order; an unknown's prefix as far as the known parts go; the answer bounded (`LE-001` to `LE-003`) |
| `JSONDecodeFunc` | `JSONDecodeFunc` | RFC 8259 strictly: trailing text, a name given twice, a number past the range and a lone surrogate fail; numbers exact; `null` a language's untyped null (`LE-004`, `LE-005`) |
| `CSVDecodeFunc` | `CSVDecodeFunc` | A leading byte order mark is passed over; an empty header name fails; failures name the line and the column (`LE-006` to `LE-011`) |
| `FormatDateFunc` | `FormatDateFunc` | A small `t` and `z` are read, as RFC 3339 allows; the format is read whole, where go-cty drops text past 64 KiB; literal text no quotation mark closes fails (`LT-001` to `LT-006`) |
| `TimeAddFunc` | `TimeAddFunc` | Exact: the fraction of a second is kept, `500ms` no longer lost, and a duration has no 292-year limit; an answer outside the years 0000 to 9999 fails with `time.out_of_range` (`LT-007` to `LT-011`) |

## Not carried

go-cty's `Bytes`, `BytesVal`, `BytesLen` and `BytesSlice` are not carried: no
expression of HCL, Terraform, OpenTofu or Packer can make a Bytes value. A host
that holds them pairs go-cty's capsule type with one of tenon's through
`ctytenon.PairCapsules` and crosses go-cty's functions with `FunctionFromCty`.
