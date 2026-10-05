# Changelog

## Unreleased

### Added

- The library's text functions begin: `UpperFunc`, `LowerFunc` and
  `TitleFunc`, on Unicode's default case conversion of the version tenon
  states, whatever the Go toolchain's: full mappings with no language's
  tailoring, so `upper("straße")` is `"STRASSE"` and `upper("ﬁsh")` is
  `"FISH"`, and a capital sigma ending a word lowercases to the final
  sigma, so `lower("ΟΔΟΣ")` is `"οδος"`, where go-cty's answers `"STRAßE"`,
  `"ﬁSH"` and `"οδοσ"` on Go's simple mappings. `Title` keeps go-cty's
  rule, which the specification now states: the first code point and each
  after an ASCII punctuation mark or space, or other white space, take
  their titlecase, so `foo.example.com` is still `Foo.Example.Com`, and
  only the mapping is the full one (`"ßtraße"` is `"Sstraße"`). A string
  not known yet answers with the case of its recorded prefix, where
  go-cty's drops the prefix. The specification gains §15's first rules
  (`LS-001` to `LS-007`): cut positions, case conversion, white space,
  string results, and the three functions.
- `StrlenFunc`, `ReverseFunc` and `SubstrFunc`, by extended grapheme
  cluster of the Unicode version tenon states, where go-cty's follow the
  go-textseg the toolchain selects: below Go 1.27 it joins a ZWJ after a
  regional indicator into one cluster, against UAX #29, and above it a
  Devanagari conjunct, by a later Unicode. `Substr` takes nothing for a
  length of zero whatever the offset, where go-cty's takes the rest for a
  negative offset (#217), and takes offsets and lengths of any magnitude,
  a fraction failing located at it; a string not known yet answers from
  the clusters its recorded prefix settles, known where they hold the
  whole part taken. `Reverse` reverses clusters, and the specification
  says plainly that clusters meeting anew may compose, so reversing twice
  need not give a string back. The specification says so (`LS-008` to
  `LS-011`).
- `SplitFunc`, `ReplaceFunc`, `TrimPrefixFunc` and `TrimSuffixFunc`, which
  match and cut only at cut positions: the boundaries of extended grapheme
  clusters and the position inside a CR LF, so no ASCII answer changes,
  but trimming one regional indicator from two flags no longer makes a
  third, and an `e` no longer matches the start of `é`. An empty separator
  splits a string into its clusters, and an empty search inserts the
  replacement between them, where go-cty's work by code points. `Replace`
  refuses an answer of more than 64 times the size of its arguments and
  64 KiB with `function.too_large`, before making any of it. A string not
  known yet answers from its recorded prefix: `Split` with at least as many
  parts as the separators it settles, the others with the text it
  settles, matched and trimmed. The specification says so (`LS-012` to
  `LS-018`).
- `TrimFunc`, `TrimSpaceFunc`, `ChompFunc` and `IndentFunc`. `Trim` and
  `TrimSpace` remove runs that end and begin at cut positions, so a space
  carrying a combining mark stays and no cluster is split; `TrimSpace`
  reads White_Space from the Unicode version tenon states. `Indent` takes
  a whole number not less than zero, failing with
  `function.invalid_argument` at it otherwise, where go-cty's panics with
  a stack trace in its message, and refuses an answer of more than 64
  times the string and 64 KiB with `function.too_large` before making any
  of it, where go-cty's may exhaust the host's memory; a string with no
  line feed is its own answer whatever the number. A string not known yet
  answers from its recorded prefix. The specification says so (`LS-019`
  to `LS-023`).
- `JoinFunc` and `SortFunc`. A null element fails at it, located by its
  list and index, even where another part is not known yet, where
  go-cty's answers unknown. `Join` without a list fails as the call's
  arity, and a string not known yet among its lists answers with the text
  known from the start, a separator before a list not known yet left out
  since it may be empty. `Sort` orders by scalar value, as go-cty's byte
  order does, and a list holding an element not known yet answers a list
  as long of strings not known yet and not null; a list of one element is
  itself. The specification says so (`LS-024` to `LS-027`).
- `FormatFunc`, on go-cty's verb language, which the specification now
  states whole. Numbers are formatted from the exact decimal: `%d`, `%x`
  and the other integer verbs write integers of any magnitude exactly, and
  `%e`, `%f` and `%g` round ties half to even on the exact value, so
  `%.2f` of 2.675 is `"2.68"` where go-cty's rounds the binary float to
  `"2.67"`. `%v` of a number is its canonical text, as `"${n}"` and
  `tostring` write it, where go-cty's is Go's `%g` (`"1e+06"` for a
  million). Widths and precisions count grapheme clusters by the Unicode
  version tenon states, and pass 10,000 only with `function.too_large`,
  where go-cty's wrap past an `int` or allocate whatever is asked. The
  small fixes: `%.0s` is empty, `-` overrides `0`, zeros come after a
  number's sign, `%t` pads, `%v` of a string takes a precision as `%s`
  does, and `%#o` of 0 is `0`. A format that is none fails with the new
  code `format.invalid_syntax`; what the known arguments settle fails
  now, and an argument not known yet leaves the answer beginning with the
  text before its verb. The specification says so (`LF-001` to
  `LF-017`).
- `FormatListFunc`: Format once for each member of the lists, sets and
  tuples among its arguments, the others repeated. The format and the
  arguments its verbs read are checked even where a list is empty, where
  go-cty's accepts any format then; a failure in one element is located
  at the argument and the member; an argument not known yet leaves the
  answer the unknown list of the length the others settle, where go-cty's
  answer has none; and an answer past 64 times its arguments and 64 KiB
  fails with `function.too_large` before it is made. The specification
  says so (`LF-018` to `LF-021`).
- `RegexFunc` and `RegexAllFunc`, on a pattern language the specification
  now states and holds: Go 1.26's `regexp/syntax` with its Perl flags,
  whatever the Go toolchain's, so `(?<name>)` and loose property names
  mean one thing everywhere, and Unicode classes and case folding of the
  Unicode version tenon states, so `\p{Greek}` and `(?i)k` match alike on
  every toolchain and a script of a later Unicode is no class. Answers
  have go-cty's shapes: the match, a tuple for unnamed groups, an object
  for named ones, a group that took no part null. A pattern naming a group
  twice fails with the new code `regex.duplicate_group`, where go-cty's
  keeps the later capture and loses the earlier; one mixing named and
  unnamed groups with `regex.mixed_groups`; one that is none with
  `regex.invalid_syntax`; each located at the pattern and failing now,
  though the string is not known yet. `Regex` with no match fails with
  `regex.no_match` at the string. An answer of more than 64 times the
  size of the arguments and 64 KiB, each capture counting its bytes and
  one more, fails with `function.too_large` before it is made, where
  go-cty's makes a list of a string for each of a pattern's groups at
  each position of the string, whatever their number. The specification
  says so (`LR-001` to `LR-009`).
- `RegexReplaceFunc`, replacing each match as `RegexAll` finds them. The
  replacement is read as Go's `Regexp.Expand` reads a template, `$1`,
  `${1}`, `$name`, `${name}` and `$$`, the letters and digits a name runs
  over being those of the Unicode version tenon states, where go-cty's
  follow the toolchain: on Go 1.27 a letter of a later Unicode after `$1`
  continues the name, and the group is lost. A reference to a group the
  pattern does not have fails with the new code `regex.missing_group` at
  the replacement, where go-cty's writes nothing for it, so `$1x`, which
  names a group `1x`, no longer drops group 1 silently, and the message
  says to write `${1}x`. A pattern that is none fails located at it, and
  with a string not known yet fails now, where go-cty's answers unknown.
  An answer of more than 64 times the size of the arguments and 64 KiB
  fails with `function.too_large` before it is made, where go-cty's makes
  whatever is asked: the empty pattern over a megabyte, with a
  replacement of a thousand bytes, makes a gigabyte. A replacement not known yet leaves the answer the string
  where nothing matches, and beginning with the text before the first
  match otherwise. The specification says so (`LR-010` to `LR-014`).

### Fixed

- `EqualFunc` and `NotEqualFunc` settled two untyped nulls only where
  they were the operands themselves, so `[null] == [null]`,
  `[1, null] == [1, null]` and `{a = null} == {a = null}` answered
  unknown, though `null == null` is true and nothing in either operand
  is open but its type; through HCL's evaluator with tenon's operators, an
  expression with no variables evaluated to an unknown, which HCL's
  specification rules out (#208). Two tuples or objects whose types wait
  on such nulls are compared member by member now, by the same rules, and
  `LN-011` says so.

### Changed

- `Convert` gives a value back as it is, without the conversion's
  machinery, where it can say at once that the conversion would: a known
  value carrying no mark and holding neither a marked member nor one not
  known, converted to `Any()`, or a string, number or bool converted to
  `Exactly` its own type. Every call converts its arguments so, and
  library functions convert again inside: formatting a line for each of
  200 services makes 7,200 allocations where it made 27,201, in under a
  third of the time, and merging their environments 5,600 where it made
  9,600. The answers are the same values.

## 0.16.0 (2026-10-05)

tenon has a standard library now. Package `stdlib` is go-cty's
`cty/function/stdlib` as tenon functions, function for function, beginning
with its 53 functions over values: the operators, the functions over
numbers, the general ones, and those over collections and sets. Each
answers as the specification states: exactly, where go-cty's rounds
through binary floats; from what is known, where an argument is not known
yet; and with a code, located at the argument, where go-cty's answers with
a Go error's text or a panic. It implements version 0.14.0 of the tenon
specification, which adds the library's part, §13 to §20 (§13, §14 and
§18 normative, the rest outlines), and `FN-024`: 292 rules.

The minor version moves for the addition and for one change to the
function system: `FunctionSpec.Impl` and `FunctionSpec.ResultOf` are given
the call's policy.

**Upgrading from 0.15.1.** A function written for 0.15 adds the policy
parameter to its `Impl`, and to its `ResultOf` where it has one; the
compiler finds each. `Equals` with an operand known to be null carries the
operands' own marks alone, where it carried every mark within the other
operand. Nothing else changes for an existing program: documents decode as
they did, and values encode to the same bytes.

**What `CONFORMANCE.md` states.** 292 of 292.

### Added

- The standard library: package `stdlib` (`github.com/kmoneil/tenon/stdlib`),
  go-cty's `cty/function/stdlib` as tenon functions, each a variable named
  as go-cty names it, so a host's table of functions moves by its import
  path: `AssertNotNullFunc` and the functions below. go-cty's functions
  over text, encodings and time are not here yet. The
  specification gains a part for the library, §13 to §20, each section an
  optional feature area: §13 states what every library function shares
  (`LB-001` to `LB-031`), with the codes `function.invalid_argument` and
  `function.too_large`, and §14 begins with `AssertNotNull` (`LN-083`).
  `conformance/vectors/functions.json` holds calls of the library's
  functions, their arguments and what each answers or how it fails, for
  testing another implementation of it.
- The library's operators: `AddFunc`, `SubtractFunc`, `MultiplyFunc`,
  `DivideFunc`, `ModuloFunc`, `NegateFunc`, `LessThanFunc`,
  `LessThanOrEqualToFunc`, `GreaterThanFunc`, `GreaterThanOrEqualToFunc`,
  `EqualFunc`, `NotEqualFunc`, `NotFunc`, `AndFunc` and `OrFunc`, each
  tenon's operation of its name offered as a function: exact arithmetic,
  division and modulo by zero failing, an unknown operand's bounds carried
  into the answer. `EqualFunc` settles a language's untyped `null` against
  the other operand's type first, so `x == null` is true of a null of any
  type. The specification says so (`LN-001`, `LN-002`, `LN-010`, `LN-011`,
  `LN-020`).
- `CoalesceFunc`, the first argument that is not null, converted to the
  type the arguments unify to under the call's policy: a null is passed
  over, one not known yet leaves the answer unknown and not null, and every
  argument null fails with `function.invalid_argument`. Its answer carries
  the marks of what it read, not of an argument after the one it chose.
  `MakeToFunc(c)`, a language's `tostring`, `tonumber` and `tolist`: the
  argument converted to `c` under the unsafe policy, a failure located
  within the argument and withholding what a redacting mark requires,
  where go-cty's quotes a sensitive argument. The specification says so
  (`LN-080`, `LN-085`).
- `AbsoluteFunc`, `SignumFunc`, `IntFunc`, `CeilFunc`, `FloorFunc`,
  `MinFunc` and `MaxFunc`, exact on every number, a number not known yet
  answered with the range its own gives (`Signum` of a positive range is
  known 1, `min(2, x)` is 2 where `x` is at least 5), where go-cty drops
  the range and its `Signum` refuses a fraction (#218). `ParseIntFunc`
  reads an integer in a base from 2 to 62, go-cty's alphabet, with no
  prefix, separator or space, refusing text over 10,000 bytes before
  reading it, and a base outside 2 to 62 with `function.invalid_argument`
  at it; a redacted text is named by its placeholder where go-cty's
  `parseint` quotes it. The specification says so (`LN-030` to `LN-033`,
  `LN-050`).
- `LogFunc` and `PowFunc`, correctly rounded to 96 significant digits, half
  to even, as a quotient is: `log(1000, 10)` is 3 and `pow(10, 23)` is
  10^23, where go-cty answers `2.9999999999999996` and
  `99999999999999991611392`, and `pow(2, 0.5)` is the 96-digit root. A
  number or a base outside `Log`'s domain fails with the new code
  `number.domain`, at once whatever the other argument is; zero to a
  negative power fails with `number.divide_by_zero`, a negative number to
  a power that is not an integer with `number.domain`, and a result far
  outside the range a number holds with `number.out_of_range` before any
  of it is computed. A power whose exact value is a rounding midpoint, as
  5^138 is, rounds half to even. No binary floating point is used: the
  result is enclosed in decimals computed until both ends round alike.
  The specification says so (`LN-060`, `LN-061`).
- The collection functions begin: `LengthFunc`, `HasIndexFunc`, `IndexFunc`
  and `ElementFunc`, each reading a collection as it stands: a list holding
  a member not known yet is read for the members it has
  (`length([unknown])` is 1, `element([a, unknown], 0)` is `a`), a list not
  known yet answers from its lengths, and a tuple or an object from its
  type. `Length` also counts a string's grapheme clusters and an object's
  attributes, as the consumers' own `length` does, and `HasIndex` and
  `Index` take objects, as HCL's `coll[key]` does. `Element` wraps an index
  of any magnitude by floor modulus. A key naming no member fails with
  `function.invalid_argument` at the key, now where a list's lengths
  already rule it out; only the collection's own marks and those of the
  member read reach the answer. The specification says so (§18, `LC-001`
  to `LC-004`).
- `SliceFunc`, `ReverseListFunc`, `ConcatFunc` and `ChunklistFunc`. Indexes
  and sizes are whole numbers of any magnitude, each failure located at its
  argument and decided now where a list's recorded lengths already rule it
  out; an unknown answer carries the length its arguments give
  (`slice(x, 0, 2)` has 2 elements, `concat` sums its arguments' lengths,
  `chunklist` counts its chunks). `ReverseList` of a set holding a member
  not known yet answers within the set's length range, where go-cty's
  claims every listed member. `Concat` unifies its lists under the call's
  policy, so `concat([1], ["a"])` is `["1", "a"]` under Unsafe and the tuple
  `[1, "a"]` under Safe. `Chunklist` of a language's `[]` is `[]`. The
  specification says so (`LC-010` to `LC-013`).
- `FlattenFunc`, `CompactFunc`, `DistinctFunc` and `CoalesceListFunc`.
  `Flatten` keeps a null sequence as a leaf, as go-cty does, takes a set's
  members in the canonical order, and answers pending where a nested
  sequence is not known yet. `Compact` keeps a member not known yet where
  its range rules out null and the empty string. `Distinct` tells known
  members apart by their hashes, so its work grows with the list rather
  than with its pairs (go-cty's takes 2.8 s for 4,000 members), and with a
  member not known yet answers within the lengths that leaves.
  `CoalesceList` takes a language's `null` (#221's class) and carries the
  marks of the arguments it examined, not of one after its choice. The
  specification says so (`LC-020` to `LC-023`).
- `KeysFunc`, `ValuesFunc`, `ZipmapFunc` and `LookupFunc`. `Keys` and
  `Values` read in the canonical order of strings, an object's keys known
  from its type even where it is not. `Zipmap` refuses a null key at it,
  where go-cty's panics, and lists of different lengths now where they
  already differ; each key's marks reach the map. `Lookup` reads its
  default only where the key is missing, so a default not known yet leaves
  a found member known, and the default is optional and may be null, as
  the consumers' own `lookup` has it; a map holding a member not known yet
  still answers a known key. The specification says so (`LC-030` to
  `LC-033`).
- `MergeFunc`. Its answer is a map where every argument with a type is a
  map of one type, and otherwise an object of every key, each attribute of
  the type of the last argument adding it, as go-cty's. Where go-cty's
  differs: a language's untyped null takes no part, so it no longer turns
  a merge of maps into an object; a null argument's marks reach the
  answer; an object not known yet that may be null gives one of the two
  shapes it may leave, where go-cty's type assumes it is not null; one
  known not null leaves the other attributes known; and a map not known
  yet leaves an object's known attributes, where go-cty's answer has no
  type at all. The specification says so (`LC-034` to `LC-037`).
- `ContainsFunc`, `SetHasElementFunc`, `SetUnionFunc`,
  `SetIntersectionFunc`, `SetSubtractFunc` and
  `SetSymmetricDifferenceFunc`. `Contains` converts the value it looks for
  to the members' type under the safe policy first, so a tuple is found
  among lists and an untyped null among nulls (go-cty's answers the null
  with an unknown of no type, #221), while a number is still not found
  among strings; an empty collection holds nothing, whatever the value.
  `SetHasElement` may look for null. The set operations unify their
  element types under the call's policy, take the empty tuple as the empty
  set, and answer from what is known: `SetSubtract` of the empty set is
  the first set, members not known yet and all; an intersection, a
  difference or a union with a member or a set not known yet is the
  unknown set listing the members known to be in it, within the lengths
  the arguments allow, where go-cty's is wholly unknown. The specification
  says so (`LC-040` to `LC-049`).
- `RangeFunc` and `SetProductFunc`, the library's first bounded results
  (`LB-031`). `Range` computes each element exactly, so `range(0, 1, 0.1)`
  is ten tenths and `range(0, 0.05, 0.01)` five hundredths, where go-cty's
  binary steps drift to wrong elements and counts. It refuses a step of
  zero however it is written (go-cty's catches only its own zero value),
  and more than 1,024 elements with `function.too_large` before making
  any, as soon as the known arguments settle it. `SetProduct` refuses more
  than 1,048,576 tuples with `function.too_large` at the argument that
  passes the bound, before making any, where go-cty's product of 64 lists
  of two wraps to an empty answer and one of 63 panics (go-cty #227). Its
  unknown answers keep the lengths the arguments allow, an empty one
  included, and the empty tuple as an argument answers the empty tuple.
  The specification says so (`LC-050` to `LC-057`).
- A function may declare its result never null (`FunctionSpec.NotNull`,
  read back with `Function.NotNull`). Every answer the call makes that is
  not a known value then says so, the unknown answer of an argument not
  yet known among them, as `unknown(number, not null)`, so a host knows
  before evaluating what a null check of the result will say; an
  implementation returning null breaks the declaration, a usage panic.
  go-cty states the same in a `RefineResult` callback. The specification
  says so (`FN-024`, a new rule; `FN-001`, `FN-002`).

### Changed

- `FunctionSpec.Impl` and `FunctionSpec.ResultOf` are given the call's
  policy, as a third and a second argument, so a conversion a function
  makes between its own arguments, as one unifying their types does,
  follows the policy the call is made under, as the arguments' own
  conversions do. A function written for 0.15 adds the parameter:
  `func(args []tenon.Value, result tenon.Constraint, p tenon.Policy)
  (tenon.Value, error)`, and `func(args []tenon.Value, p tenon.Policy)
  (tenon.Constraint, error)`. The specification says so (`FN-011`,
  `FN-020`).

### Fixed

- `Equals` with an operand known to be null, a null of a type or a pending
  value known to be null, carried every mark the other operand held within
  it, though its answer is decided by nullness alone: `x == null` came back
  sensitive where `x` merely held a sensitive value. It now reads no value
  within either operand and carries the operands' own marks alone. go-cty
  fixed the same in `Value.Equals` in v1.18.0, but its `EqualFunc`, which
  HCL's `==` calls, still unmarks deeply first; tenon's `EqualFunc`
  follows `Equals`. The specification says so (`MK-003`).

## 0.15.1 (2026-10-05)

Two fixes to the function system that 0.15.0 introduced. A function may
return a null whose type is still open from known arguments, as a JSON
`null` read with `Any` is, where `Call` panicked; and where the call
removed a redacting mark from an argument, the implementation's failure
no longer shows what the mark withheld. It implements version 0.13.1 of
the tenon specification, which amends `UN-008`, `FN-021` and `FN-023`:
227 rules. A security advisory follows this release.

**Upgrade if you call functions on values a redacting mark guards, or on
JSON from outside.** In 0.15.0, a function whose failure quoted its
argument showed a redacted value in the diagnostic's message
(`function.failed: cannot parse "hunter2"`, from a function of one's own,
from cty's `parseint` crossed by ctytenon, or from `Convert` run inside the
implementation), and a function returning a JSON null from known
arguments, as cty's `jsondecode("null")` crossed by ctytenon does, panicked
the host.

The patch version moves: nothing that existed changes but these two
answers.

**Upgrading from 0.15.0.** Nothing changes for a program but the two
cases: the compiler finds nothing, documents decode as they did, and
values encode to the same bytes. A function's failure on a redacted
argument now reads `<name> failed on redacted("...")`, its code kept and
located at the call, so a host that matched the old text matches the code
instead.

**What `CONFORMANCE.md` states.** 227 of 227.

### Fixed

- A function's implementation may return a value whose only open part is
  its type, from known arguments: a pending value known to be null, as
  `ParseJSON` reads a JSON `null` with `Any`, and a tuple or object
  holding only such values and known ones. `Call` took such a value for an
  unknown one and panicked, calling it the function author's defect,
  though nothing else can say `null` before its type is known. A function
  that decodes JSON, or one crossed from cty that returns a dynamic null
  as `jsondecode("null")` does, now answers with it. From 0.15.0. The
  specification says so (`UN-008`, `FN-021`).
- A function whose implementation failed with a message quoting its
  argument showed what a redacting mark withheld: the call unmarks an
  argument whose parameter does not admit marks, so the implementation
  could not know, and a failure such as `cannot parse "hunter2"` reached
  the diagnostic, as cty's `parseint` and `tonumber` put it there when
  crossed. Where the call removed a redacting mark from an argument, the
  failures of the implementation and of the derivation now keep their
  codes and have their messages withheld, located at the call:
  `Parse failed on redacted("secret"), for a reason its redacting marks
  withhold`. A usage panic for a broken contract describes the result by
  the redacting marks too. A function that would quote its argument
  admits marks and withholds what they require itself. From 0.15.0. The
  specification says so (`FN-021`, `FN-023`).

## 0.15.0 (2026-10-03)

tenon has functions now. A `Function` is defined once, from a
specification whose parameters are constraints, and called as every
operation is called: `Call` converts each argument to its parameter's
constraint under the policy, answers the states the implementation does
not admit, reports every failing argument as a diagnostic located by
its zero-based index, and holds the implementation to its contract:
known arguments give a known result or an error, unless volatility is
declared. ctytenon 0.2.0 crosses functions both ways, so an unmodified
cty host calls a tenon function in place. It implements version 0.13.0
of the tenon specification, which adds §12, functions (`FN-001` to
`FN-030`), and amends `UN-008` for declared volatility: 227 rules.

The minor version moves for the addition; nothing that existed changes.

**Upgrading from 0.14.0.** Nothing changes for an existing program: the
compiler finds nothing, documents decode as they did, and values encode
to the same bytes.

**What `CONFORMANCE.md` states.** 227 of 227.

### Added

- Functions: a `Function` is an operation a caller defines once, from a
  `FunctionSpec`, and a host assembles into the table an expression
  language evaluates with. Each parameter is a `Constraint`, and
  `Call(f, args, policy)` converts every argument to its parameter's
  constraint under the policy before the implementation sees it. The call
  boundary answers what the implementation does not admit, by the rules
  operations already follow: an error argument fails the call with every
  failing argument's diagnostics, each located by its zero-based index
  (`function.arity`, `function.failed`, `operation.null_operand` and the
  conversion's own codes); an unknown or pending argument answers with the
  unknown of the function's result; marked arguments are unmarked and the
  result carries the marks that propagate. The admissions `AllowNull`,
  `AllowUnknown`, `AllowPending` and `AllowMarked` widen what the
  implementation sees without changing what any state means. A result is a
  constraint or derives from the arguments (`ResultOf`), a host asks it
  before evaluating with `ResultConstraint`, and a specification that
  declares `Volatile`, or a function declared so with
  `Function.AsVolatile`, answers known arguments with the unknown, as a
  timestamp must. The specification gains section 12, Functions (`FN-001`
  to `FN-030`), and the operand matrix calls a function in every state and
  every position.

## 0.14.0 (2026-10-02)

A program reads JSON with tenon itself now. `ParseJSON` reads a document
into a value of the constraint it gives, in one pass, in under half the time
that encoding/json and gotenon took together, and refuses what encoding/json
lets through: a name given twice, and text that is not well-formed UTF-8.
With it, a null whose type nothing gives takes the type the members beside
it settle, as go-cty and Terraform read `{"a": null, "b": 1}` for a
`map(any)`. It implements version 0.12.0 of the tenon specification, which
adds §11, reading JSON (`JS-001` to `JS-022`), and amends `CV-021`, `CV-031`
and `SE-060`: 211 rules.

The minor version moves for the addition, and because a conversion's result
changes: a collection whose pending-null members the other members settle is
now that collection, where it was a pending one.

**Upgrading from 0.13.0.** Documents 0.13.0 wrote decode as they did, and
values encode to the same bytes. The compiler finds nothing. These it does
not find:

- Converting a list, set or map one of whose members converts to a pending
  value known to be null gives the collection of the type the other members
  settle, holding that type's null, where it gave the pending collection of
  its length.
- A name given twice as it is written, as `CheckAttributeNames` can be
  given it, is reported as `attribute name "a" is given 2 times`, where the
  message called the two one name after normalization.

**What `CONFORMANCE.md` states.** 211 of 211.

### Added

- `ParseJSON(data, c, p)` reads JSON text into a value that `c` admits, in
  one pass over the text, where a program took two: `encoding/json` into
  `any`, then gotenon, which it reads in under half the time and with half
  the allocations. What the text says is read as JSON implies, a number
  exactly from its text, a string normalized, an array as a tuple and an
  object as an object, and then converted to `c` under `p` as `Convert`
  converts it, so that `ParseJSON(data, ListOf(Exactly(NumberType())),
  Unsafe)` reads `["8080"]` as a list of numbers, as go-cty does, and
  `Safe` refuses it.
  - A `null` is the null of the type `c` gives there, or of the type the
    members beside it in a list, set or map settle, and otherwise the
    pending null: `{"a": null, "b": 1}` read with `MapOf(Any())` is a map of
    numbers holding a null.
  - The text is RFC 8259's grammar, strictly: no byte order mark, comment,
    trailing comma or leading zero, and nothing after the value. A failure
    of the text stops the reading with `json.invalid_syntax`, naming the
    byte offset, and arrays and objects nested more than 512 levels deep
    with `json.too_deep`.
  - What encoding/json lets through is refused: a string that is not
    well-formed UTF-8, or an escaped surrogate no pair completes, fails as
    `string.invalid_utf8` rather than becoming U+FFFD, and two members of
    one object with one name, as written or after normalization, fail as
    `object.duplicate_name` rather than the last winning, which lets two
    readers of one document see two values. An empty name fails as
    `object.empty_name`, whatever the object becomes.
  - Every failure of reading the text is reported, each at its path, in
    the order of the text.

  The specification says so in a new section, §11 (`JS-001` to `JS-022`):
  211 rules, each covered. The conformance vectors gain `json.json`, texts
  read with their constraints and what each reads as or fails with, for
  another implementation to test against.

### Changed

- A member that converts to a pending value known to be null, a null whose
  type nothing gives, now takes the type the other members of its list, set
  or map settle, and becomes that type's null, as an empty member takes its
  siblings' element type. Converting `{"a": null, "b": 1}`, its null the
  pending null, to `MapOf(Any())` gave a pending map of two entries; it now
  gives `map(number){"a": null, "b": 1}`, as go-cty and Terraform read it.
  It holds a level down, `[[null], [1]]` to `ListOf(ListOf(Any()))` being a
  list of lists of numbers, and a Propagate mark on the null stays on it.
  Where nothing settles a type, or a member converts to another pending
  value, the collection is pending, as before. The specification says so
  (`CV-021`, `CV-031`).
- A name given twice as it is written, which a JSON object can hold and
  `CheckAttributeNames` can be given, is reported as given that many times,
  `attribute name "a" is given 2 times`, and a map key as `map key "a" is
  given 2 times`, where the message called the spellings one name after
  normalization.

## 0.13.0 (2026-10-02)

A value can now hold what is known of it before its type is. A pending
list, set or map can be narrowed by its length, and a tuple or object one of
whose members is pending is the pending value holding its members, where
`Tuple` and `Object` panicked: as go-cty's tuple of a dynamic value and a
number holds the number, so does tenon's. A conversion keeps both where it
meets them. A member that settles no element type, as an empty list beside
lists of strings, takes the type its siblings settle, as go-cty does, where
the conversion failed. And a set that a conversion makes inside a collection
that widens it is the set at its own type, widened, which closes a leak of a
redacted value's attribute names. It implements version 0.11.0 of the tenon
specification, which adds `UN-025` and amends `VA-001`, `UN-023`, `UN-024`,
`EQ-010`, `CV-021`, `CV-031`, `CV-032`, `CV-033`, `CV-044`, `SE-010`,
`GO-041`, `DI-010`, `DI-032` and `MK-008`: 202 rules.

**Upgrade if a redacting mark guards a value you convert into sets.** From
0.10.0 to 0.12.0, converting a value that held a redacted value inside a
member of a set, to a collection whose element type widens the set's, gave
a collection without the redacting mark: its display form and `%#v` stated
its type, and with it the attribute names and the shape of what the
redacted value held. Converting `[[{"a0": [s, []]}]]`, where `s` is a
string carrying a redacting mark `"secret"`, to
`ListOf(SetOf(ObjectWith(nil, false)))` displayed as
`list(set(object({"a0": tuple([string, tuple([])])})))[redacted("secret")]`.
It is now `redacted("secret")` whole, as it was in 0.9.0.

The minor version moves because results change: conversions that failed
succeed, one that gave a bare pending value gives the pending tuple or
object holding what its members convert to, two panics now give values,
and documents can hold two things earlier versions cannot read.

**Upgrading from 0.12.0.** Documents 0.12.0 wrote decode as they did, and
values encode to the same bytes. The compiler finds nothing. These it does
not find:

- `Tuple` and `Object` given a pending member return the pending value
  holding the members, and `Narrow` takes `LengthMin` and `LengthMax` on a
  pending value whose constraint admits only lists, sets and maps, where
  each panicked.
- A pending value may hold members, which `Len`, `Index`, `Elements`,
  `ElementsSeq`, `Attribute`, `LookupAttribute` and `Attributes` read. A
  walker that steps into a value where `HasContent` is true does not reach
  them; one that asks the new `HasMembers` does. `Type` still panics on
  it, as on any pending value.
- Converting a container one of whose members converts to a pending value
  gives the pending tuple or object holding the converted members, or a
  pending list, set or map of their number, where it gave a bare pending
  value.
- A conversion that failed with `convert.no_common_type` because a member,
  as an empty list, settled no element type now succeeds where its
  siblings settle one.
- A document holding a pending value narrowed by its length, or a pending
  tuple or object holding members, is refused by 0.12.0 and earlier as
  `serialize.malformed`.

**What `CONFORMANCE.md` states.** 202 of 202.

### Added

- A tuple or an object one of whose members is pending is now the pending
  value holding its members, where `Tuple` and `Object` panicked.
  `Tuple(Pending(Any()), NumberFromInt(1))` is a pending tuple whose second
  element reads as `1`, as go-cty's tuple of a dynamic value and a number
  does.
  - Its constraint is `TupleOf`, or a closed `ObjectWith` of required fields,
    of each member's constraint, and it is not null.
  - `Len`, `Index`, `Elements`, `ElementsSeq`, `Attribute`,
    `LookupAttribute` and `Attributes` read its members as they read a
    tuple's or an object's, and the new `HasMembers` says whether a value
    has members to read.
  - `Resolve` resolves each pending member to its part of a tuple or object
    type.
  - It displays as the tuple or object it will be, and `%#v` writes the
    call that makes it.
  - It encodes as an item of its own kind, its members each a whole item,
    which earlier versions refuse as `serialize.malformed`.
  - `Equals`, and `Contains` through it, compare it member by member, and
    a deep mark on it reaches its members.
  - Converting a tuple or object one of whose members converts to a
    pending value, as an unknown map does to an open `ObjectWith`, gives
    the pending tuple or object holding what the members convert to, where
    it gave a bare pending value and lost them. A list, set or map
    converted so gives the pending collection with the number of members
    as its length.
  - A pending tuple or object converts member by member as the tuple or
    object it will be, to a tuple, object, list, set or map; as the
    resolved value of its one type where its constraint admits one; and
    whole to `Any`.
  - `Diff` walks into it as into the tuple or object it will be, and
    gotenon, converting it member by member, decodes its resolved members
    and refuses each pending one with `decode.not_known` where it is.

  A list, set or map of a pending member still panics, its element type
  being given. The specification says so (`UN-025`), a new rule: 202 rules,
  each covered.
- A pending value whose constraint admits only lists, sets and maps can be
  narrowed by its length. `Narrow(Pending(ListOf(Any())), LengthMin(2))` is
  a pending list of at least two members: a configuration language can know
  how many items a list holds before it knows their type, as go-cty does
  for a `list(any)` whose elements are not known yet.
  - `Length` of such a value is the unknown number within those lengths.
  - `Resolve` narrows the unknown value by them.
  - `Equals` is known `false` against a value whose length they exclude.
  - The display form, `%#v` and the encoding carry them.

  A pending value whose constraint admits a string, a tuple or an object
  still takes no length, since a string counts characters and the others'
  lengths are in their types. Bounds that leave no length leave a value that
  may be null as the pending null, and are a contradiction where it cannot
  be null. A document holding such a value has a pending item with a fourth
  element, its lengths, which earlier versions refuse as
  `serialize.malformed`. Converting such a value keeps its lengths as
  converting an unknown collection does: both into a list or a map, and
  into a set the greatest, with a least of one. The specification says so
  (`UN-024`, `SE-010`, `CV-032`).

### Changed

- A member that settles no element type of its own takes, in that part, the
  type the members beside it settle. Converting `[["a"], []]` to
  `ListOf(ListOf(Any()))`, as HCL writes a value for a Terraform variable
  of type `list(list(any))`, failed with `convert.no_common_type` at `[1]`,
  since each member's type was settled alone and the empty tuple settles
  none. It now gives a list of two lists of strings, the second empty, as
  go-cty does. The same holds at any depth, for null and unknown members,
  within tuples and objects, and converting to sets and maps: the part a
  member leaves open unifies as `Any` does, so an empty member converts as
  a member of the same type with members in it would. Where nothing
  settles the part, the conversion still fails with
  `convert.no_common_type`, at each empty value that left it open. A
  `OneOf` whose first member leaves a part open now takes that member,
  where it went on to a later one. The specification says so (`CV-021`,
  `CV-044`).

### Fixed

- A set that a conversion makes inside a collection whose element type
  widens the set's own is now the set it is at its own element type,
  widened. Made directly at the wider type, it could differ in three ways:
  - It lost an `Isolate` mark on a value inside a member that the wider
    type rebuilt.
  - It kept a member that is not known and could only be a value the set
    already held, so it was not known where on its own it was. Converting
    `[[unknown, [], null], [[], [1]]]`, the first three of type
    `tuple([])`, to `ListOf(SetOf(Any()))` gave a first set holding an
    unknown list; it is now the known set of the empty list and null, as
    converting the first member alone gives.
  - A redacting mark it gathered from inside a member did not reach the
    collection holding it, whose type shows the set's attribute names, so
    the collection's display form and `%#v` showed them, from 0.10.0 (see
    above).

  A set is made at its own type first only where that changes it, so the
  conversion still works in proportion to its result. The specification
  says so (`CV-021`, `CV-033`, `EQ-041`).

## 0.12.0 (2026-10-01)

The 1.0 audit's additions to the API, each of them additive: a range reads
back what it says, iterators range over a container without copying it, a
program asks whether a value is null as a `bool` and makes a number of a
`big.Rat`, and `%#v` prints the Go that builds a value, so that a failing
test shows what it compared rather than tenon's internals. It implements
version 0.10.0 of the tenon specification, as 0.11.0 did: 201 rules.

Beside the library, the bench module now holds a test for each of
go-cty's open issues whose defect tenon could share, asserting what go-cty
v1.19.0 does with the issue's case and what tenon does with its
counterpart. One of them found the message fixed below.

The minor version moves for the additions. Values, their encodings and
the results of every operation are as they were.

**Upgrading from 0.11.0.** The compiler finds nothing. Three messages and
texts change:

- `%#v` prints a `Value`, `Type`, `Constraint`, `Path`, `Step`, `Range`,
  `Narrowing`, `*Error` and `*CapsuleType` as Go syntax, and the kinds and
  policies by their constants' names, where it printed internals or a
  number. A test comparing `%#v`'s text needs the new text.
- The panic of `ObjectType` and `ObjectWith` for two spellings of one name
  names them in ASCII.
- gotenon's message for a `big.Rat` outside the range of numbers names the
  rational, as its message for an inexact one did.

**What `CONFORMANCE.md` states.** 201 of 201, as before.

### Added

- A `Range` reads back what it says. `NumberMin` and `NumberMax` give a
  bound and whether it is inclusive, `StringPrefix` the prefix, `LengthMin`
  and `LengthMax` the lengths, and `Members` the members a set is known to
  hold, each method named for the narrowing it reads and returning what
  that narrowing takes. The range of a known value answers for the value,
  so a known number is its own bound; lengths are the ones `Length` gives,
  so a set of `bool` is no longer than 3 whatever its range records; a
  bound carries the `Propagate` marks of its value, and a set's members its
  deep marks, as `Elements` gives them.
- Iterators over a container's members, which read them where the value
  holds them rather than copying them first: `ElementsSeq` beside
  `Elements`, giving a set's members with its deep marks as `Elements`
  does, `MapEntries` for a map's keys and elements and `Attributes` for an
  object's names and attributes, both in sorted order. Each panics where
  the read beside it does, as it is called.
- `Value.IsNull`, whether a value is null, as a `bool` for the program:
  true for the null value of a type and a pending value known to be null,
  where the `IsNull` operation's answer is known true, and false for a
  value that may yet turn out null.
- `NumberFromBigRat`, a `big.Rat` as a Number, exactly. A rational that is
  not a terminating decimal gives an error value with code
  `encode.inexact`, and one outside the range of numbers
  `number.out_of_range`, as encoding a `big.Rat` with gotenon does, which
  now calls it. gotenon's message for a rational outside the range names
  the rational.
- `GoString`, the Go syntax that builds a value, which `%#v` prints in
  place of the value's internals, and so testify where an assertion fails:
  `tenon.List(tenon.NumberType(), tenon.NumberFromInt(1))`, which can be
  pasted back into the test as the value it expects. `Type`, `Constraint`,
  `Path`, `Step`, `Range`, `Narrowing`, `*Error` and `*CapsuleType` have
  one too, and the kinds and policies print by their constants' names, as
  in `tenon.KindList`, so a `Diagnostic`, a `Change` or a `Field` prints
  as Go as well. The syntax builds an identical value, but that a capsule
  type is written as a new one of its name, a mark as `%#v` writes it, and
  a value carrying a redacting mark as `tenon.Value{}` with its redacting
  marks, withholding all else as its display form does. Where writing each
  type in full would make the syntax grow with the members times their
  type, it names each type once, in a function literal called in place.

### Fixed

- `ObjectType` and `ObjectWith`, given two names that are one name after
  normalization, panic naming the two spellings in ASCII, as
  `CheckAttributeNames` and the error values of `Object` and `Map` name
  them. The panic quoted them as they are, so an `e` with a combining
  acute and a precomposed `é` printed as the same name twice.

## 0.11.0 (2026-09-29)

The rest of the architecture review of 0.9.0: the fixes that waited on
answers to its questions, and two more found along the way. Above all,
`String` and the text of a diff did work out of proportion to their input,
spelling out a member's type for every member. With them, tenon requires no
other module and holds the Unicode data for a string's length itself;
decoders a program supplies fail with an `error`; `Deserialize` refuses
what a part may not hold where its tag says so; a panic names a redacted
value by its marks alone; `Contains`, `Div` and `Mod` answer as soon as the
type or the divisor decides; `Convert` refuses a container by its policy
before reading its members; gotenon reports a Go map's name failures as
`Map` does, and refuses a marshaler a struct would take on from a field it
embeds; and `encoding/xml` and go-cmp see values as they are. It implements
version 0.10.0 of the tenon specification, which amends `NU-024`,
`UN-011`, `EQ-042`, `EQ-043`, `CV-031`, `CV-033`, `CV-051`, `SE-043`,
`SE-051`, `GO-004`, `GO-020`, `GO-022`, `GO-041`, `GO-043`, `GO-044`,
`DI-010`, `DI-030`, `DI-035` and `DI-037`, clarifies `GO-012`, and adds the
code `serialize.decoder_failed`: 201 rules, as before.

**Upgrade if you log, display or diff values from parties you do not
trust.** In 0.10.0 a value's display form, which `String`, `LogValue`,
`MarshalText` and `fmt` give, spelled out a member's type for every null or
unknown member and every nested collection, and a diff's text spelled out
each changed part's: a document of 101 KB holding 100,000 nulls of a type
509 levels deep displayed in 307 MB, and the diff of two lists of 4,000
such members, of 5 KB and 25 KB, in 24 MB. `Diff` itself did work of that
size to order the members of two sets that are not known. They now display
in 603 KB and 107 KB. A security advisory follows this release.

The minor version moves because results change and two signatures break:
`MarkDecoder` and `CapsuleEncoding.Decode` return an `error`, display forms
and diffs read differently, and `Contains`, `Div`, `Mod`, `Narrow`,
`Convert`, `gotenon.Encode` and `Deserialize` answer otherwise in a few
cases.

**Upgrading from 0.10.0.** Documents 0.10.0 wrote decode as they did, and
values encode to the same bytes. The compiler finds two changes:

- A `MarkDecoder` returns `(Mark, error)`, and a `CapsuleEncoding.Decode`
  `(*E, error)`: return `tenon.NewError(tenon.ErrorVal(d))` where one
  returned `[]tenon.Diagnostic{d}`, or an error of your own, which fails
  with `serialize.decoder_failed` and stays the cause of `Deserialize`'s
  error.
- A `tenon.Change` written as a composite literal without field names
  needs the new field, `InCollection`; one with names compiles as it did.

These it does not find:

- A member of a list, set or map displays without its type, as in
  `list(number)[1, null, unknown]`; a diff's change within one shows its
  parts without theirs; and a set's member changes carry the set's deep
  marks, as `+ .: member marked(2, "d")`. A test comparing `String` or a
  diff's text with text written for 0.10.0 needs the new text, and
  `encoding/xml` writes a value as its display form, where it wrote
  nothing.
- `Contains` answers false where the value looked for cannot be of the
  members' type, and `Div` and `Mod` fail where the divisor can be no
  number but zero, where each answered unknown.
- Narrowing a set that holds members that are not known is decided by its
  length alone, so some narrowings it refused leave the set as it was.
- Under `Safe`, a container that converts only unsafely fails as a whole,
  at its own path, before its members are read.
- `gotenon.Encode` panics on a pending value below the top of what it is
  given, and gotenon on a struct that would take on a marshaler from a
  field it embeds; `Encode` reports a Go map's name failures in `Map`'s
  order and at its paths.
- `Deserialize` fails a few inputs with another code, or at another byte,
  where a mark, an unknown value or a null stands where its tag forbids it.
- In a diff, two members of a set of objects or tuples that are not known
  can come in the other order.

**What `CONFORMANCE.md` states.** 201 of 201, and no rule more widely than
its test exercises.

### Fixed

- tenon requires no other module, and grapheme segmentation, which gives a
  string its length, is its own, generated from Unicode 15.0.0's data as its
  normalization is. It came from `github.com/rivo/uniseg` v0.4.7, which Go
  treats as a minimum: a program requiring a later uniseg, itself or through
  another module, would raise it, and a uniseg of a later Unicode would
  count some strings' lengths, and so encode some unknown strings narrowed by
  a prefix, otherwise than the Unicode 15.0.0 that tenon states does. Nothing
  a program requires moves the version now, and `golang.org/x/text`, which
  only tenon's tests used, leaves your module graph too. Normalization and
  segmentation are held to Unicode's own conformance tests for the version
  on every toolchain.
- `Deserialize` refuses a mark within a set member or a recorded member, a
  mark or an unknown value within the payload of a capsule value or a mark,
  and a resolved item within a marked item where the tag, or the item's
  kind, that says so is written. It read the whole part first, so input
  holding a later fault within the part failed with that one, often with
  another code: a set member carrying a mark that no decoder was supplied
  for failed with `serialize.unknown_mark`, though the mark being there at
  all is the first fault, `serialize.malformed`. These refusals, and that
  of a payload that is null, now name the byte where the mark, the unknown
  value or the null is written.
- A usage panic names a value carrying a redacting mark by its marks alone,
  as `a value redacted by "secret"`, as its display does. `Hash`, `Set`,
  `Attribute` and the rest named it by its type, whose attribute names are
  the value's shape: ``Hash called on a value of type object({"password":
  string}) that carries marks``. A panic's message reaches crash reports and
  logs. Where the reason for a panic would say whether such a value is null,
  known or pending, what kind it is or what it holds, the message withholds
  it and says to unmark the value to see it; where the call refuses marked
  values anyway, as `Hash` and `Set` do, it says that. `SECURITY.md` says
  so, and that a collection's declared type is its own: a list of objects
  shows its element type beside a redacted member as beside any other.

### Added

- `Value.MarshalText`, which gives a value's display form, so that
  `encoding/xml` and the YAML and TOML libraries write a value as itself,
  where `encoding/xml` wrote every value as nothing. What a redacting mark
  withholds stays withheld, and the zero Value fails rather than being
  written. `encoding/json` still writes a value's JSON projection.
- `Equal` on `Range`, `Narrowing`, `*Error` and `*CapsuleType`, the method
  that go-cmp's `cmp.Equal` calls, so that a struct holding one compares by
  what it says, where go-cmp panicked on their unexported fields. Every
  exported type go-cmp would reach an unexported field in now has one.
- `Change.InCollection`, which says that a change lies within a list, set or
  map that `Diff` looked within, whose element type fixes its parts' type in
  both values. A change's display form leaves that type out where it is set,
  so a change built or copied by hand sets it to display as `Diff`'s does.

### Changed

- `MarkDecoder` and `CapsuleEncoding.Decode` return an `error` in place of
  `[]Diagnostic`, as gotenon's `MarshalValue` and `UnmarshalValue` do, so that
  every hook a program supplies fails one way. An error that is a
  `*tenon.Error` contributes its diagnostics, and any other error its text,
  with the new code `serialize.decoder_failed`; `Deserialize`'s error keeps
  it as a cause, which `errors.Is` and `errors.As` find. This breaks code that
  supplies decoders, before 1.0 freezes the signatures: a decoder that
  returned `[]tenon.Diagnostic{d}` returns `tenon.NewError(tenon.ErrorVal(d))`,
  or an error of its own.
- The limit on number text, 10,000, counts bytes of its UTF-8 encoding, as
  parsing always has, and the `number.too_long` message, `NumberFromText`'s
  documentation and `SECURITY.md` now say so, where they said characters:
  3,334 euro signs are fewer than 10,000 characters and more than 10,000
  bytes, and are refused unread. Only a count of bytes can refuse text before
  reading any of it. The specification says so (`NU-024`).
- `CapsuleOps.Hash` and `Compare` say that a capsule type declaring `Equal`
  and neither `Compare` nor an `Encoding` keeps one value of each equality
  class whose hash collides with another's for the rest of the run, which its
  canonical order needs, and that declaring `Compare` or an `Encoding` keeps
  nothing.
- `gotenon.Encode` reports what fails in a Go map as `tenon.Map` and
  `tenon.Object` do, in one order and at one set of paths: each key in turn
  gives its own failure and its member's, and then comes one failure for each
  key that keys share once normalized. It reported the keys' failures first,
  and an empty key at `.[""]` where `Object` reports it at `.`, and it did
  not encode a member under a key that failed, whose own failure was lost.
  `Map`, `Object` and `CheckAttributeNames` check names by one implementation,
  which `gotenon.Encode` builds through.
- `gotenon.Encode` panics on a pending value below the top of what it is
  given, whether a `tenon.Value` or a `MarshalValue` method gives it, naming
  the path where it is: `Encode: the pending value at ".items[0].v" has no
  type`. It panicked from the root's constructors, which name an attribute
  but no path. A pending value that is the whole of what `Encode` is given is
  kept, as before. `SECURITY.md` and gotenon's documentation now state that
  decoding into `big.Int`, `big.Rat` or `big.Float` costs work set by a
  number's magnitude rather than its length, and advise decoding numbers
  from outside into `tenon.Value` or a fixed-size type.
- Under `Safe`, a container that converts only unsafely, a list or a tuple
  to a set, a list or a set to a tuple, or a map to an object, fails as a
  whole with `convert.unsafe` before any member is read, as the policy
  refuses it whatever the members hold. Its members were converted first,
  and a member that failed was reported instead: `list(string)["a",
  "abc"]` converted to `set_of(number)` failed at `.[0]` and `.[1]`, and now
  fails at `.`. A container that fails once its members are read, as members
  of no common type do, carries their `Propagate` marks, holding none of
  them. A set holding members that are not known, converted to a tuple,
  converts its known members, as it does converted to a list, so one that
  fails at every position alike fails the conversion: `set(string)["x",
  unknown]` converted to `tuple_of(number, number)` was an unknown tuple.
  The specification says so (`CV-031`, `CV-033`, `CV-051`).
- `Contains` answers false where the value looked for cannot be of the type
  of the set's members, however little of the set is known:
  `Contains(unknown set(bool), 1)` was unknown, and is false, as it is for a
  known set and as `Equals` answers of values of two types. `Div` and `Mod`
  fail with `number.divide_by_zero` and `number.modulo_by_zero` where the
  divisor can be no number but zero, as for a divisor known to be zero:
  `Div(5, u)`, `u` unknown and bounded by `>= 0, <= 0`, was an unknown
  number, though every outcome but a null divides by zero. The specification
  says both (`EQ-043`, `UN-011`).
- Narrowing a set that holds members that are not known is decided by its
  length alone, as the specification now states: the narrowings contradict
  it exactly where the least length they and its members give is above the
  greatest, and otherwise the set is left as it was. It was decided more
  finely, by matching the values a listing asks for to the members that
  could be them and by the values its element type holds, in ways the
  specification did not state and another implementation could not
  reproduce. Some narrowings it refused it now leaves the set as it was,
  such as `Members` of 2 and 3 asked of `set(number)[1, unknown]`,
  and none makes such a set known: narrowed to one member, it stays as it
  was. No set that satisfies the narrowings is ruled out, as before.
- `Diff` gives a set's member changes their member as the set gives it when
  read, carrying the set's deep marks, as `Elements` gives it and as a
  list's changes carry their elements: with a deep mark `d` on the set, a
  member added reads `+ .: member marked(2, "d")`, where it read
  `+ .: member 2`. The members are still paired and ordered as the sets
  hold them, so only what the changes carry and show changes.
- gotenon refuses, with a usage panic naming the method and the field, a
  struct that may have a `ValueMarshaler`, `ValueUnmarshaler` or text
  marshaler method from a field it embeds, in the direction the method
  concerns. Such a struct took on the embedded field's method and crossed
  as that field alone: ``struct{ time.Time `tenon:"at"`; Name string }``
  encoded as a timestamp and dropped `Name`, and ``struct{ big.Int }`` as a
  string. Name the field, and the struct's methods are its own.
- A value's display form shows a member without the type its container's
  type states: a null member as `null`, an unknown one as `unknown` and its
  facts, and a list, set or map within a collection as its brackets, as a
  tuple always displayed. `list(number)[1, unknown(number), null(number)]`
  now reads `list(number)[1, unknown, null]`, and
  `list(list(number))[list(number)[1]]` reads `list(list(number))[[1]]`. The
  members of a tuple or object within a collection, and the members an
  unknown set's range lists, display the same way. Spelling the type for
  every member made a display form grow with the members times their type:
  a list of 4,000 nulls of a type 500 levels deep encodes in 5 KB and
  displayed in 12 MB, and now displays in 27 KB. `String`, `LogValue`,
  `MarshalText` and the messages that quote a value show the new form, and
  the conformance vectors' display forms change with it. The specification
  says so (`DI-010`).
- A diff shows the parts of a change within a list, set or map without
  their type, as a value's display form shows members:
  `~ .items[0]: null -> unknown` where it read
  `~ .items[0]: null(list(number)) -> unknown(list(number))`. The type is the
  collection's element type in both values; a part above the first such
  collection keeps its type, in which the two values can differ. Spelled out
  for every change, the type made a diff's text grow with the changed
  members times their type: two lists of 4,000 members of a type 500 levels
  deep encode in 5 KB and 25 KB, and their diff displayed in 24 MB, and now
  displays in 107 KB. `Diff` read the members of two sets that are not known
  with their type to order them, work of the same size, and now reads them
  as members are shown, which can order two members of a set of objects or
  tuples the other way. The conformance corpus's diffs change with it. The
  specification says so (`DI-030`, `DI-035`, `DI-037`).

## 0.10.0 (2026-09-28)

An architecture review of 0.9.0 found places where tenon did not keep the
promises `SECURITY.md` makes, and this release keeps them: a diagnostic could
name a type holding a redacted value's attribute names, and `Convert`,
`Unify` and `gotenon.Decode` did work out of proportion to their input on
shapes a document can take. With them come the review's other fixes: `Diff`
raced with the functions that keep a value's hash, `Deserialize` refused
documents `Serialize` wrote near the depth bound, a deep mark attached again
copied the whole value, and gotenon decoded a slice of a type that only
encodes itself member by member. It implements version 0.9.0 of the tenon
specification, which amends `GO-010` and `CV-050`, rewords the rationales of
`CV-042` and `EQ-012`, and checks Appendix B's divergences from go-cty
against go-cty 1.19: 201 rules, as before.

**Upgrade if a redacting mark guards anything you log, display or return, or
if you convert, unify or decode values from parties you do not trust.** In
0.9.0 a conversion failing for a value that held a redacted object, and a
narrowing that left nothing of a redacted unknown value, named in its message
a type holding the object's attribute names. And converting nested tuples
whose element type grows at every level, or a value whose leaves carry
marks, unifying object constraints one level down, and naming a long type
once for each of many failing members did work growing with the square of
the input or faster: 4,000 object constraints as the elements of lists
unified in 1.7 GB, and a document of 112 KB decoded through
`gotenon.Decode` allocated 642 MB. A security advisory follows this release.

The minor version moves because results change: `Type.Equals`, deprecated
since 0.6.0, is gone, two misuses of marks panic where they passed quietly,
gotenon decodes a few Go types differently, and `Deserialize` gives another
code for a few inputs.

**Upgrading from 0.9.0.** Documents 0.9.0 wrote decode as they did, and
values encode to the same bytes. The compiler finds one change: use
`Type.Equal` where you used `Type.Equals`, which answered alike. Four it does
not find:

- `HasMark` panics on a nil mark, where it answered false for a value
  carrying no marks.
- `WithMarks`, and `Deserialize` for what a mark decoder returns, panic on a
  mark whose `Propagation` is neither `Propagate` nor `Isolate`, which was
  carried as an `Isolate` mark is.
- gotenon decodes a slice, array, map or pointer holding a type that
  implements `ValueMarshaler` and not `ValueUnmarshaler` as it would without
  the method: `ConstraintFor` gives the `list_of` or `map_of` of the type's
  constraint, where it gave `any`, and a map of such a type refuses a null
  list, which it decoded as a nil map.
- `Deserialize` fails a few inputs holding two faults with the code of the
  first, where it gave the second's, and range key 0 holding an indefinite
  length with `serialize.not_canonical`, where it gave
  `serialize.malformed`.

**What `CONFORMANCE.md` states.** 201 of 201, and no rule more widely than its
test exercises.

### Fixed

- `Diff` no longer races with `Hash`, `Length` and the other functions that
  work out a value's hash, or a set's count of distinct members, and keep it:
  `Diff` copied a value without the atomic reads the rest of the package uses
  for those, so a program reading one value from several goroutines, as the
  package documentation allows, could see the race detector report inside
  tenon. The fields are atomic types now, and `go vet` keeps them from being
  copied any other way.
- `Deserialize` reads back every document `Serialize` writes near the depth
  bound. It counted a marked value one level deeper than `Serialize` does, and
  than the specification says, where a mark adds no level, so 510 nested lists
  with the outer one marked, or 256 marked at every level, serialized and were
  then refused as `serialize.too_large`. A marked value held directly in
  another marked value is refused where the inner one begins, with the code
  it was refused with before.
- A diagnostic's message no longer names a type that shows what a redacting
  mark withholds: a narrowing that leaves nothing of a redacted unknown value
  names no type, where it named its attributes, and a conversion that fails
  for a value holding a redacted one names its kind, "a tuple", where it
  named a type holding the redacted value's attribute names, whether the
  value did not convert, would convert only unsafely, or converted to no
  member of a `OneOf`.
- A message names a type by at most its first 32 bytes, as it names a value:
  a conversion failing for each of many members, a projection failing for
  each unknown member, and `gotenon.Decode` failing for each part it cannot
  decode each repeated a long type whole, so their messages grew with the
  members times the type.
- `Convert` asks what it needs of a constraint, a type or a marked value once
  in a conversion, where it asked again at every level: whether a member's
  type fits its constraint, what that constraint admits and gives, whether a
  collection's element type satisfies it, and whether a container holds a
  redacted value. A comb of values 400 deep with marked leaves converted in
  606 ms and now in 33 ms, and it now grows with the values rather than with
  the values times the depth. A member's path step is made only when the
  member fails, where every member made one, a number for each index.
- `Convert` builds each member of a container once, at the element type the
  levels above it settle, where it built each member at its own collection's
  element type and built it again at every level above whose element type
  grew. Tuples nested 80 deep around 2,000 objects, each level adding an
  attribute every object is given, converted to nested lists in 144 ms and
  263 MB, and now in 9 ms and 13 MB, in proportion to the result.
- `Unify` unifies constraints one level down, as the elements of lists,
  sets or maps, in tuples, or in an object's fields, as it unifies them at
  the top: each one more touches only its own parts, where it was paired
  with the union of all before it written out. 4,000 `ObjectWith`
  constraints of distinct fields, each the element of a `ListOf`, unified
  in 1.7 GB, and now in 6 MB, as many as at the top.
- A set that a conversion builds keeps the marks its members give it,
  `Isolate` ones included, when the collection holding it widens its element
  type, as it did when nothing widened it: building it again at the wider
  type lost them.
- A deep mark attached to a value stops at the values within that carry it
  already, as it did in 0.6.0: unmarking a value and marking it again, or
  putting a marked value's elements in another container and marking that,
  copied every value within and gave each one more layer of marks every
  time, which every read of their marks then walked. Unmarking a list of 100
  lists of 100 numbers and marking it again made 10,313 allocations and
  makes 8, and after 64 times a number held 65 layers of marks and holds 1.
- gotenon decodes a slice, an array, a map or a pointer holding a Go type
  that implements `ValueMarshaler` and not `ValueUnmarshaler` as it would if
  the type did not implement it, since the method concerns encoding alone:
  `ConstraintFor` of such a slice was `any` where it is the `list_of` of the
  type's constraint, and decoding took each member on its own, so that a map
  of the type decoded a null list as a nil map, where a map of the same type
  without the method refuses it, as it now does.
- `Deserialize` fails input holding two faults with the first, as the
  specification says, in four more places, which each judged their fault
  only once more had been read. A decimal fraction whose mantissa is a
  multiple of ten, zero among them, stops the reading whatever the
  mantissa's size, where only a bignum mantissa did. A map key that is the
  same as one before it once normalized, and a range's narrowing that does
  not apply to its type, are refused at the key, where they were refused
  once the map or the range had been read. Input with one fault fails with
  the code it did, but for range key 0 holding an indefinite length, which
  fails with `serialize.not_canonical`, as an indefinite length does
  anywhere else, where it failed with `serialize.malformed`. Five vectors
  in `conformance/vectors/vectors.json` pin these.
- A capsule type whose encoding's `Encode` returns what it promises not to,
  an error value or the zero `Value` among them, panics with a usage error
  wherever tenon reads it, as `Serialize` did: `Set` and `CanonicalCompare`,
  which order values whose declared hashes collide by their encodings,
  failed with a nil pointer dereference. `CONFORMANCE.md` now says that
  such values order by their encodings, as they have since 0.7.0.
- `CapsuleOps` and `NewCapsule` state what ordering a capsule type's values
  asks of their memory. A type declaring neither `Equal` nor `Compare` is
  ordered, in a set and by `CanonicalCompare`, by weak pointers to its
  values, which the Go runtime makes only for memory Go allocated: where the
  pointers lead into memory C allocates or `syscall.Mmap` maps, ordering them
  can end the process with a fatal error that no recover catches. Such a
  type declares `Compare`, or `Equal` and `Hash`, and is then ordered
  without them.
- A usage panic names the call that was misused: `Deserialize` refuses a
  mark decoder's mark that cannot be told from other marks naming the
  decoder, where it named `WithMarks`, which its caller never called, and
  `LookupField` asked of a constraint of another kind names `LookupField`,
  where it named `Field`.

### Added

- `BENCHMARKS.md` says what tenon costs beside `encoding/json` and go-cty:
  parsing a configuration document, converting it to a schema, encoding and
  decoding it, comparing two copies, reading a nested value and diffing two
  versions, at a kilobyte, 32 kilobytes and a megabyte, each figure the
  median of ten runs. `make bench` measures them again, in the `bench`
  module, which has its own `go.mod` so that go-cty never becomes a
  dependency of tenon's, and writes the file and the README's Performance
  summary.
- The README says where tenon differs from go-cty, each difference checked
  against go-cty 1.19, how to take a document from JSON into a Go struct with
  gotenon, and what 1.0 will hold stable; its redaction example is whole, as
  `ExampleWithMarks`, and the quick start is gotenon's `Example_quickStart`.
  The table of `make` targets moves to `CONTRIBUTING.md`.

### Changed

- `WithMarks`, and `Deserialize` for what a mark decoder returns, refuse a
  mark whose `Propagation` is neither `Propagate` nor `Isolate` with a usage
  panic. Such a mark was carried as an `Isolate` mark is, a policy it did
  not declare, and a policy a later version adds would be taken for
  `Isolate` by this one.
- `HasMark` panics with a usage error when the mark is nil, whether or not
  the value carries marks: it answered false for a value carrying none, and
  failed with a nil pointer dereference for one carrying some.

### Removed

- `Type.Equals`, deprecated since 0.6.0: use `Type.Equal`, which answers as
  it did. The package-level `Equals`, the language's equality of values, is
  unchanged.

## 0.9.0 (2026-09-27)

This release is the API that 1.0 keeps. The pre-1.0 audit left questions open
about tenon's API, its encoding and its specification, and each answer is
here: one naming rule, results of `(T, error)` with one error type, values
that `==` cannot compare, a typed capsule handle, names from data that fail
as data, gotenon's runtime forms and its reading of text marshalers, and the
forms `log/slog` and `encoding/json` render values in. With them come the
last of the audit's fixes, to redaction above all. It implements version
0.8.0 of the tenon specification, which adds `UN-010`, `UN-011`, `TY-018`,
`GO-044` and `DI-018`, amends `EQ-042`, `EQ-044`, `MK-002`, `MK-005`,
`MK-011`, `CV-023`, `CV-033`, `CV-050`, `SE-002`, `SE-004`, `SE-050`,
`GO-001`, `GO-004`, `GO-010`, `GO-011`, `GO-012`, `GO-015`, `GO-020`,
`GO-021`, `GO-040`, `GO-050` and `DI-015`, and closes the provisional numbers
and the canonical order in Appendix C: 201 rules.

**Upgrade if a redacting mark guards anything you log, display or return.**
In 0.8.0 a redacted map's keys and an object's attribute names reached
diagnostics' paths and messages, and a redacting mark whose policy was
`Isolate` stayed behind on its value, so what was derived from the value
showed what it withheld. Each is fixed below.

The minor version moves because nearly every program's source changes: the
constructors are renamed, four functions return an error, and `==` no longer
compiles on values. The deprecated `Type.Equals` goes at 1.0.

**Upgrading from 0.8.0.** Documents 0.8.0 wrote decode as they did, and
values encode to the same bytes. The compiler finds almost everything to
change:

- Rename by the table under Changed: the type constructors to `ListType`,
  `SetType`, `MapType`, `TupleType` and `ObjectType` first, then the value
  constructors to `List`, `Set`, `Map`, `Tuple`, `Object` and `Null`, and the
  narrowing `Null()` to `NullOnly()`.
- `Serialize`, `Deserialize`, `ProjectJSON` and `Unify` return `(T, error)`;
  read a failure's diagnostics from the `*tenon.Error` that `errors.As`
  finds. `Unify` takes the constraints first and the policy last.
- Compare values with `Value.Equal` or `Identical` and test for the zero
  value with `IsZero`; key a map by `Serialize`'s bytes.
- Make capsule types with `NewCapsule`, and build and read their values
  through the handle it returns.
- In gotenon, look for a `*tenon.Error` where you looked for a
  `*gotenon.DiagnosticError`, and give each `UnmarshalValue` method the
  policy parameter.

Five changes the compiler does not find:

- An `UnmarshalValue` method left with one parameter compiles, and no longer
  implements `ValueUnmarshaler`, so its type decodes by its kind. Assert
  `var _ gotenon.ValueUnmarshaler = (*T)(nil)` beside each.
- gotenon encodes `time.Time`, `netip.Addr` and any other type that marshals
  itself to text as the string of its text, where it encoded `{}`, and a
  struct whose state is all unexported, marshaling itself neither way,
  panics as a usage error.
- `Div` and `Mod` by a known zero fail now over a dividend not known yet,
  where they gave an unknown number, and `Contains` answers `true` for a
  member recorded in the range of a set that may still be null.
- A name that cannot be an attribute name fails with the new
  `CodeObjectEmptyName` or `CodeObjectDuplicateName` wherever it is met,
  where a conversion or `gotenon.Encode` failed with
  `CodeConvertUnexpectedAttribute` or `CodeMapDuplicateKey`.
- A mark whose type holds a `Value` is no longer comparable, and `WithMarks`
  panics on it; keep what the mark says as Go data, and build its payload
  from that.

**What `CONFORMANCE.md` states.** 201 of 201, and no rule more widely than its
test exercises.

### Fixed

- A redacting mark withholds a value's structure as well as its contents, as
  `SECURITY.md` promises: the keys of a map and the attribute names of an
  object no longer reach a diagnostic's path, a message, or a type. A
  conversion that fails within a redacted value fails at that value, once for
  each code, its message naming the value by the placeholder and the
  constraint converted to, where a redacted map `{"hunter2-key": "x"}`
  converted to numbers failed at `.["hunter2-key"]`. A list whose element type
  takes attribute names from a redacted member carries the member's redacting
  marks, where converting it showed them in its type and in the members given
  those attributes. An operation names a redacted operand by the placeholder,
  where it said the operand was null or named its constraint, though the code
  still says why. `Serialize` locates what fails within a redacted value at
  the value. A diagnostic's code is unchanged in every case.
- A redacting mark propagates whatever its `Propagation` says, as the rest of
  what `SECURITY.md` promises requires: a redacting mark whose policy is
  `Isolate` stayed on its value, so what was derived from the value showed
  what it withheld. A narrowing taken from a redacted 42 displayed `>= 42`,
  adding 0 to it gave 42 unmarked, and a set it was deep on converted to a
  list of its members in clear. Each now carries the mark. An `Isolate` mark
  that does not redact stays where it is put, as before.
- `Deserialize`'s failure messages quote nothing the document holds: they say
  what is wrong and the byte offset where. A document is refused before the
  marks that follow its content are read, so a message quoting it could show
  what a redacting mark would withhold: a map key or attribute name that
  appears twice, a range's narrowing, a diagnostic code, or the type a
  document gives. A capsule type's or a mark's identifier is still named,
  since it says which decoder to supply.
- A type that implements only one of gotenon's marshaler interfaces is mapped
  by its kind only in the other direction, and only when a value goes that
  way. `gotenon.Encode` panicked on a `map[int]string`, or a tree holding
  itself, that implemented `MarshalValue`, where the method encodes it; it
  now encodes it, and decoding into such a type is the usage error. A type
  that implements only `UnmarshalValue` decodes whatever its kind, and
  encoding from it is the usage error. A nil pointer to a type that encodes
  itself encodes as the null decoding reads back as nil, as before.

### Added

- Values, types, paths and constraints log through `log/slog` as their
  display forms, which withhold what a redacting mark withholds, where slog
  wrote their representations. Types, paths and constraints implement
  `encoding.TextMarshaler` with their display forms, so `encoding/json`
  writes them as strings, and a value implements `json.Marshaler` with its
  JSON projection, failing with a `*tenon.Error` where the projection fails:
  a secret, or a value not known yet. The zero `Value` fails rather than
  panics; a field that may hold it is tagged `omitzero`. gotenon encodes a
  type, path or constraint held in a Go value as the text of its display
  form, by the text marshaling it now honours.

### Changed

- The API is named by one rule: a function that makes a type ends in `Type`,
  as `BoolType`, `NumberType` and `StringType` did, and one that makes a value
  is bare, as `Bool`, `String` and the operations were. `List`, `Set`, `Map`,
  `Tuple` and `Object` make values, and `Null` makes the null of a type:

  | 0.8.0 | Now |
  | ----- | --- |
  | `ListVal`, `SetVal`, `MapVal`, `TupleVal`, `ObjectVal` | `List`, `Set`, `Map`, `Tuple`, `Object` |
  | `List`, `Set`, `Map`, `Tuple`, `Object` (types) | `ListType`, `SetType`, `MapType`, `TupleType`, `ObjectType` |
  | `NullVal(t)` | `Null(t)` |
  | `Null()` (the narrowing) | `NullOnly()` |
  | `Value.MapElement` | `Value.LookupMapElement` |
  | `Constraint.Field` | `Constraint.LookupField` |
  | `Unify(p, cs...)` | `Unify(cs, p)` |

  A call left under an old name fails to compile, the names that changed
  meaning among them, since a value is no type. An accessor that panics on a
  name its value or type lacks has a `Lookup` form that answers whether there
  is one, for a name from data: `LookupAttribute` beside `Attribute` and
  `LookupAttributeType` beside `AttributeType`; the lookups that already
  answered so take the name. `Unify` takes the policy last, as `Convert`
  does. `Propagation` has a `String` method, and the `String` method of every
  exported type answers for its zero value.

- gotenon honours `encoding.TextMarshaler` and `encoding.TextUnmarshaler`: a
  type that marshals itself to text, and not by `MarshalValue`, encodes as
  the `String` of its text, and one whose pointer unmarshals itself from text
  decodes from a string, so `time.Time` and `netip.Addr` cross as their text
  where they crossed as `{}` and came back as zero. `ValueMarshaler` and
  `ValueUnmarshaler` come first, each direction maps on its own, and the big
  numbers map as numbers still. A struct whose state is all in unexported
  fields, and which marshals itself neither to a value nor to text, is a
  usage error, where it encoded as an empty object and lost its state.

- `gotenon.ValueUnmarshaler`'s method is `UnmarshalValue(v tenon.Value, p
  tenon.Policy) error`: it is told the policy the decoding was given, so what
  it converts it converts as the rest of the decoding does. Add the parameter
  to each `UnmarshalValue` method; one left with the old signature no longer
  implements the interface, and its type decodes by its kind.
- `gotenon.DecodeInto(v, dst, p)` decodes into what a pointer points to, and
  `gotenon.ConstraintFor` and `gotenon.TypeFor` give a `reflect.Type`'s
  mapping, for a program that has a Go type only when it runs. `DecodeInto`
  leaves `*dst` as it was where decoding fails.
- gotenon's documentation says what decoding into a struct always did: the
  object is closed, so an attribute no field names fails, and names match
  only exactly, case included.

- `Object` gives an error value for attribute names that cannot be ones, as
  `Map` does for keys, where it panicked: names come from data as often
  as from the program, and `SECURITY.md` promises that data never panics. An
  empty name fails with the new `CodeObjectEmptyName`, a name that is not
  well-formed UTF-8 with `CodeStringInvalidUTF8`, and names that are one once
  normalized with the new `CodeObjectDuplicateName`, in the order a map's keys
  report theirs. The new `CheckAttributeNames` reports the same of names
  alone, so a program can check names from data before building an
  `ObjectType`, an `ObjectWith` constraint or a `Path.Attribute` step, which
  still panic on such a name. An empty map key that no attribute can be named fails
  with `CodeObjectEmptyName` as well where a conversion to an object or
  `gotenon.Encode` of a map meets it, where it failed with
  `CodeConvertUnexpectedAttribute`, and keys of a map that `gotenon.Encode`
  makes an object of fail with `CodeObjectDuplicateName` where they failed
  with `CodeMapDuplicateKey`: one fault, one code, wherever it is met.

- A capsule type is made by `NewCapsule`, which returns a typed handle, a
  `*CapsuleType[E]`: `Type` gives the capsule type, `Value` builds a value
  from a `*E`, and `Of` reads back the `*E` a value encapsulates, with false
  for anything but a known value of the type. The pointer type is checked
  where the program is compiled, where `CapsuleVal` and `CapsuleValue`
  asserted it where it ran and panicked. `Capsule`, `CapsuleVal` and
  `CapsuleValue` are removed, and `CapsuleOps.Equals` is `CapsuleOps.Equal`,
  the name the rest of the API uses. Write `h := tenon.NewCapsule(name, ops)`
  and `h.Type()` where a program wrote `t := tenon.Capsule(name, ops)`,
  `h.Value(p)` for `tenon.CapsuleVal(t, p)`, and `p, ok := h.Of(v)` for
  `tenon.CapsuleValue[E](v)`.

- `Serialize`, `Deserialize`, `ProjectJSON` and `Unify` return a result and
  an `error`, where they returned the result, an error value and a bool of
  which only some could be used; gotenon's `Encode` and `Decode` fail with
  the same error. It is a `*tenon.Error`, which holds the error value
  (`Value`) and its diagnostics (`Diagnostics`), renders them in `Error`
  without ever panicking, and leads `errors.Is` and `errors.As` to the Go
  errors that caused it (`Unwrap`); `NewError` builds one.
  `gotenon.DiagnosticError` is gone. A marshaler or unmarshaler returns a
  `*tenon.Error` to contribute diagnostics, and whatever error it returns is
  kept as a cause of the failure, where only its text was kept, so
  `errors.Is` finds it. Write `b, err := tenon.Serialize(v)` where a program
  wrote `b, failure, ok := tenon.Serialize(v)`, and look for a
  `*tenon.Error`, calling `Value()`, where it looked for a
  `*gotenon.DiagnosticError` and read its field `Value`. A marshaler
  returning a `*tenon.Error` that holds no error value fails the encoding as
  any other error does, with its text, where it was a usage panic.

- `Value`, `Constraint` and `Path` cannot be compared with `==` or used as
  map keys, and neither can `Step`, `Diagnostic` and `Change`, which hold
  them. `==` compared how two were held rather than what they said, so
  `String("a")` built twice was two map keys. Compare values with
  `Value.Equal`, which is `Identical`, and steps with `Step.Equal`, each in
  the form go-cmp's `cmp.Equal` calls; test for the zero value with `IsZero`,
  which `Value`, `Type`, `Constraint`, `Path` and `Step` now have, where a
  program wrote `v == tenon.Value{}`. `Type.Equal` and `Constraint.Equal` no
  longer panic on a zero value, which is equal only to itself. `Type` is
  still comparable. A mark's type must be comparable, so a mark cannot hold a
  `Value`: it keeps what it says as Go data and builds its payload from that.

- An operation that its known operands alone make fail fails now: `Div` and
  `Mod` by a known zero give `CodeNumberDivideByZero` and
  `CodeNumberModuloByZero` where the dividend is unknown or pending, where
  they gave an unknown number that could only ever become that error. A plan
  that can never apply is refused before anything is applied.
- `Contains` answers `true` for a member recorded in an unknown set's range
  even while the set could still turn out null, where it answered unknown
  until the set was narrowed `NotNull`: a null set would make the answer an
  error, not `false`. `LessThan`, `Length` and `And` already answered this
  way, and the specification now says so for every operation.

- A document's envelope is an array whose first element is the format
  version, which `Deserialize` reads before anything else, so a document of a
  later version is refused as `CodeSerializeUnsupportedVersion` whatever its
  envelope holds, where one of another length was called malformed. Version
  1 documents are unchanged, byte for byte.
- `SECURITY.md` counts `Convert`, `Diff` and `Unify` among the entry points
  where work out of proportion to the input is a vulnerability, with the
  bounds `Convert` and `Unify` keep.

- A deep mark displays once, on the value it was attached to, and not again on
  each value within it, which carries it too: `String` gives
  `marked(list(number)[1, 2], "d")` where it gave
  `marked(list(number)[marked(1, "d"), marked(2, "d")], "d")`. A member lists
  only the marks it carries beyond its container's. The display form of k
  members under k deep marks listed k identifiers on each, growing with the
  square of the value, which `SECURITY.md` bounds `String` against; it now
  grows with the value. Display forms of deep-marked values change, the
  vector corpus's among them; encodings do not.

- `SECURITY.md` and `gotenon.Encode` say what an input's length is where a Go
  value shares its parts: the tree the value describes, a slice, map or
  pointer reached from two places counted at each, as `encoding/json` counts
  it. `Encode` encodes such a part wherever it is reached, so a value built by
  sharing one part many times costs what it spells out; a program that builds
  what it encodes from input bounds that tree.
- The specification says which code input holding more than one fault fails
  with: the first a decoder meets, reading from the first byte, as
  `Deserialize` does. The reading stops at a malformed value, a nesting too
  deep, an unknown identifier and an indefinite length, among others, where
  each is written, and finds the rest, as an integer in a longer form than it
  needs, by comparing the input with the value's encoding once it is read
  through. The vector corpus holds inputs with two faults, in both orders.
- The specification defines the least length of a set holding unknown members
  as tenon counts it: the members taken in iteration order, each counted where
  it is provably distinct from every member counted before it, and it fixes
  that order for the members that are not known as the order of their
  encodings, where it was left to implementations. So every implementation
  gives one length, and one contradiction, for one set. `Length` and
  `Elements` say so.

## 0.8.0 (2026-09-27)

This release carries the rest of what the pre-1.0 audit found that needed no
decision: three corrections to gotenon, work kept in proportion to its input
across marks, paths, messages, ranges, sets, diffs, conversion, unification
and the encoding, and documentation brought back to what the code does. It
implements version 0.7.0 of the tenon specification, which amends `GO-011`,
`GO-012`, `GO-013`, `GO-032`, `GO-040`, `GO-043`, `GO-050`, `SE-005` and
`CV-040`, and scopes `CV-045`'s rationale: 196 rules, as before.

**Upgrade if you decode documents from parties you do not trust.** In 0.7.0
a set nested in sets was hashed again by every set holding it, so decoding a
document whose bulk sits many sets down did work growing with its depth
times its size, up to 512 times what the document's size alone calls for: a
list 400 sets down took 15 ms to decode where it now takes 0.6 ms. Upgrade as
well if you convert, unify or diff values built from data: several shapes did
work growing with the square of their size, and each is below.

The minor version moves because results change. `gotenon.Encode` fails where
it met an error value, which it returned with a nil error. `gotenon.Decode`
fails on a set holding members that are not known decoded into a slice or
array, which it decoded as though its members were settled, and panics on a
type holding an interface whatever the value, where a null let one through.
A container's hash differs from 0.7.0's, as hashes may between any two runs.

**Upgrading from 0.7.0.** Documents 0.7.0 wrote decode as they did, and
values encode to the same bytes. A program that passes error values through
`gotenon.Encode` now sees them as failures, with their diagnostics located
where the part was.

**What `CONFORMANCE.md` states.** 196 of 196, and no rule more widely than its
test exercises.

### Fixed

- `gotenon.Encode` fails where a `tenon.Value` it encodes, or the value a
  `MarshalValue` method returns, is an error value, with that value's own
  diagnostics located within the part, where it returned the error value with
  a nil error: `Encode(S{V: tenon.Div(one, zero)})` succeeded.
- `gotenon.Decode` fails with `CodeDecodeNotKnown`, at the set, where a set
  holding members that are not known is decoded into a slice or array of
  `tenon.Value`, or of a type that decodes by an unmarshaler. Such a set has
  no settled number of members and no settled order, which is why a slice of
  numbers already refused it; `set(number)[1, u, u]`, of one to three members,
  decoded as three, and into `[2]tenon.Value` failed with a length mismatch.
- Converting a container whose members sit under its deep marks does work in
  proportion to the members, not to the members times the marks: each
  converted member shares the layer of marks its siblings share, where it was
  given every mark anew. 2,000 marked numbers under 2,000 deep marks took
  1.6 s and 2.7 GB to convert to a set, and take 3 ms and 3.6 MB; 4,000
  failing members under as many deep marks took 3.6 s and 5.9 GB, and take
  5 ms and 8 MB. A container that converts to a pending value gathers its
  members' marks the same way.
- Failures deep in a value are located once. A container's error value keeps
  its failing members and builds each diagnostic's path when the diagnostics
  are first read, the steps above them shared, where every level extended
  every path beneath it: 2,000 failing members 200 levels down took 1.9 s and
  6.5 GB to convert, and take 1.3 ms and 2.8 MB, their diagnostics listed.
  The diagnostics, their order and the duplicates dropped are as they were.
- A message that quotes a value writes no more of it than it shows, where it
  wrote the whole display form to keep its first 32 bytes: a narrowing
  contradicted by a list of 1,000 nulls of an object type of 1,000 attributes,
  which spells the type out 1,000 times, allocated 90 MB for its message, and
  now allocates 408 bytes at any size. Lists of types in messages are cut the
  same way.
- Sets holding members that are not known, and ranges that list the members
  a set must hold, are compared, narrowed and diffed in proportion to their
  members, where each member was looked for among all the others: at 8,000
  members, comparing two ranges listing them took 350 ms and now takes
  0.05 ms; a set of known members beside an unknown one compared with a known
  set, 380 ms and now 0.75 ms; such a set narrowed by `NotNull`, a known set
  narrowed by a listing of its members, and a known set compared with a range
  listing it, about 190 ms each and now about 1 ms. A range narrowed by 4,000
  listings of one member each re-sorted its listing each time, 203 ms and
  68 MB, and now merges each in, 0.8 ms and 0.14 MB. Diffing two sets whose
  members are not known pairs identical members in one pass and writes each
  changed member's display form once.
- A value is hashed once, however many sets hold it: a value's hash is kept,
  and a container's is made from its members'. `Deserialize` and building a
  set rehashed everything a nested set held at every level that held it, and
  `Serialize` copied it into every enclosing set: a list 400 sets down took
  15 ms to decode and 2.5 MB to encode as a 7 KB document, and now takes
  0.6 ms and 80 KB. `Serialize` writes a set's members where they go, moving
  them only when they are out of order.
- `Unify` works in proportion to the constraints it is given where the bound
  on pairs does not reach: 4,000 `ObjectWith` constraints of distinct fields
  took 414 ms and 2.4 GB, and take 3 ms and 5 MB, their fields gathered as
  they go; and a unification writes each constraint canonically once, where
  OneOfs nested 400 lists deep allocated 6.7 MB, now 0.5 MB. Converting a list
  of 4,000 objects of distinct attributes beside a map took 1.5 s and 1.9 GB
  to find the element type, and takes 3 ms and 4.6 MB.
- Numbers with long coefficients compare without making the same power of
  ten again for every comparison, and trailing zeros are counted by dividing
  rather than by writing the number out as text.

### Changed

- Documentation that had drifted from what the code does is corrected. Of
  a range's listing, `Members` and `Narrow` describe the least length as
  the known values listed, and only a listing of known values as becoming a
  set. `Length` gives the element type's bound on a set's length. `Unify`
  says a value satisfying only a `OneOf` member it leaves out does not
  convert to the result. `Contains`, `Serialize` and `Deserialize` list the
  panics they have, and `Serialize`, `Deserialize`, `ProjectJSON` and `Unify`
  say the result they return beside a failure, or beside success, is a zero
  value not to be used. `Type.Equal` says it panics on the zero Type. The
  examples of a prefix narrowing and of a division's error show what the code
  gives. gotenon says `Encode` takes an interface, where only `Decode`
  panics on one.
- gotenon's `CodeDecodeLengthMismatch` message names the kind decoded, a set
  or a tuple as well as a list, and counts one member as one.
- The vector corpus pins the depth bound: a value 512 levels deep, alone and
  marked, since marks add no level, and 513 levels refused, alone, marked,
  and in a mark's payload.

- `gotenon.Decode` panics on a type that is an interface or holds one, as its
  documentation says, before it looks at the value. It panicked only once a
  known value reached the interface, so a null, an empty collection or an
  absent optional field let such a type through until the data changed. A
  type that decodes by an unmarshaler may still hold an interface.

## 0.7.0 (2026-09-27)

An audit of the whole library ahead of 1.0 found a few things that should not
wait for it, and this release fixes them: one shape of document that breaks
the bound SECURITY.md sets on decoding, an encoding that depended on the Go
toolchain, capsule values kept alive for the life of the process, values
`Serialize` wrote that `Deserialize` then refused, and `Diff` results that
could alter the values they compared. It implements version 0.6.0 of the tenon
specification, which amends `SE-003`, `SE-040` and `SE-051`, and restates
`SE-005`'s rationale: 196 rules, as before.

**Upgrade if you decode documents from parties you do not trust.** In 0.6.0 a
list carrying k deep marks, whose k members each carry a mark of their own,
decoded in time and memory growing with k by k: 85 KB of it took 354 ms and
256 MB. It is a canonical document, which `Serialize` writes, and here it
decodes in 4 ms and 6 MB. A security advisory follows this release. Upgrade as
well if you build with more than one Go toolchain and encode `StringPrefix`
narrowings: 0.6.0 built with Go 1.27 recorded a shorter prefix for some texts
than built with Go 1.26, so the same value had two encodings.

The minor version moves because results change. `StringPrefix` keeps more of
its text in a few places, so those narrowings encode differently. `Capsule`
panics on a type that declares an `Encoding` without `Equals`. `Serialize`
refuses a value nesting more than 512 levels deep. A slice, array or map of a
type that decodes by an unmarshaler decodes from members of differing types,
where it failed. Each is below.

**Upgrading from 0.6.0.** Documents 0.6.0 wrote decode as they did, a recorded
prefix read as recorded, and 0.6.0 reads the longer prefixes 0.7.0 records. A
capsule type that declares `Encoding` must declare `Equals` and `Hash` too,
the equality its encoding already implies.

**What `CONFORMANCE.md` states.** 196 of 196, and no rule more widely than its
test exercises.

### Fixed

- Decoding does work in proportion to the document where a container's deep
  marks meet members that carry marks of their own. Each such member held a
  merged copy of every deep mark beside its own, so a document of k members
  under k deep marks, which `Serialize` writes and `Deserialize` accepts, cost
  k by k: 85 KB of it allocated 256 MB and took 354 ms, which SECURITY.md
  promises cannot happen. Members now hold their own marks beside one shared
  list of the deep marks they inherit, and the same document decodes in 4 ms
  and 6 MB; attaching deep marks with `WithMarks`, encoding such a value and
  `UnmarkDeep` grow with it likewise. What a value carries, and how it
  displays, encodes and compares, are unchanged.
- `StringPrefix` records the same prefix whichever Go toolchain builds tenon.
  It found the part of its text that text following it cannot change by
  `golang.org/x/text`'s normalization boundary, whose Unicode data follows the
  toolchain, so built with Go 1.27 it recorded `prefix "x"` for
  `"x\U00011382"` where Go 1.26 recorded the whole text, and the narrowed
  value encoded differently. That boundary also assumes text no longer than
  thirty combining marks per run, which tenon does not limit, and past thirty
  it recorded a prefix that a value beginning with it could fail. It is now
  computed from tenon's own Unicode 15.0.0 data by plain normalization: the
  same everywhere, sound for any run, and keeping more text in a few places
  where the old boundary was needlessly cautious, such as a macron (U+00AF)
  that nothing composes with.
- Ordering capsule values no longer keeps them alive. Where a capsule type
  declares no `Compare`, the canonical order numbered its values for the rest
  of the process, so a long-running program that put capsule values into
  sets, or decoded documents holding them, kept every one it had ever
  ordered. Values of a type with no equality are now numbered by weak pointer
  and forgotten once collected, and a type with an encoding orders values
  whose hashes collide by their encodings, numbering none.
- `Deserialize` refuses a number written as a bare bignum, or as a decimal
  fraction whose bignum mantissa is a multiple of ten, as
  `serialize.not_canonical` before working out its digits. Neither is any
  number's encoding, and stripping the zeros of such a mantissa by its text
  cost many times the document: 826 KB took 286 ms where its canonical twin
  takes half a millisecond. The vector corpus gains both.
- `Serialize` refuses a value nesting more than 512 levels deep, with
  `CodeSerializeTooLarge` at the first part past the bound, where it wrote a
  document that `Deserialize` then refused as too large: data written that
  could not be read back. It counts levels as the decoder does, so whatever it
  writes reads back.
- `Diff`'s mark changes hold their own copies of the marks. `OldMarks` and
  `NewMarks` were the diffed values' own storage where no deep mark was set
  aside, so writing to them changed a value that is immutable: it no longer
  displayed, was no longer identical to its twin, and failed to serialize,
  and another goroutine reading it raced with the write.

### Changed

- `Capsule` panics where `CapsuleOps` declares an `Encoding` without `Equals`
  (and so `Hash`). Such a type's values were equal only by pointer, so a value
  read back from its encoding was never identical to the one written, and a
  set of two equal capsule values encoded two identical members, which the
  one-encoding rule forbids. Declare the equality the encoding already
  implies.
- A slice, array or map whose elements decode by an unmarshaler, directly or
  through a pointer, decodes as a slice or map of `tenon.Value` does: from a
  list, set or tuple, or a map or object, each member by its own conversion.
  Members whose types differ now decode into `[]T`, `[]*T`, `[N]T`, `[][]T`
  and `map[string]T` under either policy, each method given its own, where
  under Safe they failed with `convert.no_common_type`. Encoding is unchanged.

## 0.6.0 (2026-09-26)

Every question the v0.1.0 audit left open is answered, and this release
carries the answers and the fixes they unblocked, two of which close ways a
redacting mark could be lost. It implements version 0.5.0 of the tenon
specification, which amends `UN-002`, `UN-004`, `UN-005`, `EQ-010`, `VA-020`,
`MK-006`, `CV-002`, `CV-024`, `CV-041`, `GO-041` and `GO-042`, and adds
`CV-045`: 196 rules.

**Upgrade if you mark values as redacting and build paths, or decode them with
gotenon.** In 0.5.0 a key under a redacting mark, given to `Path.Index`, read in
clear after one `Serialize` and `Deserialize`, and `gotenon.Decode` dropped an
Isolate mark, a redacting one included, from a struct field, slice element or
map element that it decoded into a `tenon.Value` or by an unmarshaler.
`Path.Index` now refuses a marked key, and `Decode` hands such members over as
they are. Upgrade as well if you `Unify` constraints you did not write: ten
`OneOf`s of three object types each, 1.5 KB written out, took 392 MiB, and are
now refused in under a millisecond.

The minor version moves because results change. Narrowings that leave a value
that may be null no other value give the null value, where they gave
`range.contradiction`. `Path.Index` panics on a marked key. `Unify` refuses
past its bound with the new code `unify.too_large`. A pointer to a type that
decodes by an unmarshaler decodes by it, where it failed. The Go packages
`conformance`, `conformance/values` and `conformance/matrix` leave the public
API for `internal/`, and `Type.Equals` is deprecated for `Type.Equal`. Each is
below.

**Upgrading from 0.5.0.** A document recording a range that holds nothing but
null, which 0.5.0 refused as malformed, is refused as not canonical, since
that value is the null; the vector corpus renames its entry and adds one for a
range that excludes null. Replace `Type.Equals` with `Type.Equal` before 1.0
removes it. A program that imported the conformance packages, which only
tenon's own tests did, keeps a copy of what it used.

**What `CONFORMANCE.md` states.** 196 of 196, and no rule more widely than its
test exercises: the narrower tests of `UN-004`, `UN-005` and `DI-011`, noted
since 0.4.0, are settled.

### Added

- `Type.Equal` reports whether two types are the same type, the name
  `Constraint`, `Path` and `Diagnostic` already give their comparisons.
  go-cmp calls an `Equal` method where a type has one, so `cmp.Equal` and
  `cmp.Diff` now compare structs holding a `Type`, where they panicked on
  its unexported field.

### Changed

- The specification now says which rule decides a conversion to a `OneOf`
  that a value already fits: a value whose type satisfies some member, and
  holds each attribute that member would add, converts to itself, whichever
  place the member has, and the first member a conversion exists to decides
  only otherwise. `{"a": 1}` converted to `OneOf(withB, onlyA)`, where `withB`
  would add an optional `b`, stays `{"a": 1}`. Nothing behaves differently.
- `gotenon.Decode` decodes a pointer, at any depth, to a type whose pointer
  implements `ValueUnmarshaler` by that method, as it decodes the type
  itself: `*T`, a `*T` field and `[]*T` receive unknown, pending and marked
  values as `T` does, where they failed with `decode.not_known` or
  `decode.marked`. As `encoding/json` treats a pointer to an `Unmarshaler`,
  a null that carries no mark leaves the pointer nil, and a marked null goes
  to the method, so its mark is not dropped.
- `Unify` is bounded by the size of what it is given. Each member of one
  `OneOf` unifies with each member of another, so constraints that are each a
  `OneOf` of object types with different attributes multiplied: ten of three
  objects each, 1.5 KB written out, unified to 3^10 members in 392 MiB.
  `Unify` now weighs each pair of members it forms by their sizes, and where
  the pairs would weigh more in all than 64 times the size of its
  constraints, it forms no more and returns an error value with the new code
  `CodeUnifyTooLarge` (`unify.too_large`); that case is refused in under a
  millisecond. Unions that stay small pass however many there are: a
  thousand "number or string" constraints, an object of eleven such fields
  with itself, two tagged unions of 60 variants each. Constraints are
  unified in canonical order, so a refusal does not depend on the order they
  are given in, though unifying in stages can differ from one call near the
  bound. `Convert`'s doc now says that objects of distinct attributes
  converted to one collection each gain the others' attributes, n by n.
- `Path.Index` panics on a key that carries marks, as `SetVal`, `Hash` and
  `Members` panic on marked values. A mark on a key showed in the path's
  display, went unnoticed by `Identical`, and dropped out of the encoding,
  so a key under a redacting mark read in clear after `Serialize` and
  `Deserialize`: `.[redacted("secret")]` came back as `.["k"]`. tenon's own
  paths index by fresh keys and are unaffected; a program indexing by a
  marked key unmarks it first, deciding what a diagnostic may show. This
  settles what `CONFORMANCE.md` overstated for `DI-011` since 0.4.0, the
  last rule it overstated.
- The specification now says what `Identical` and the encoding compare for
  an unknown set: what its range records. A listing keeps a listed
  requirement the others imply, so `Members(1, u)`, with `u` any number at
  least 0, and `Members(1)` allow the same sets and are two ranges, not
  identical and encoded differently, while every operation answers alike
  for them. Nothing behaves differently; finding implied requirements would
  compare listed values pairwise, which bounded decoding rules out.
- Narrowings that leave a value that may be null no other value leave it
  null. `Narrow(Unknown(NumberType()), NumberMin(5), NumberMax(3))` is
  `null(number)`, where it was `range.contradiction`: bounds, prefixes,
  lengths and listings say nothing about null, so null satisfies them, and
  narrowing the null value by them already returned it. The unknown and
  the null it may turn out to be now answer alike, one narrowing at a time
  or all at once, and a plan no longer reports a contradiction the
  configuration it plans does not meet. The same holds for a set longer
  than its element type has values, such as a set of bools of length 4. With
  `NotNull` in force, before those narrowings or after, they are still a
  contradiction, with the same message either way. A document recording
  such a range is refused as `serialize.not_canonical`, since the value is
  the null, where it was `serialize.malformed`; the vector corpus renames
  that entry "range only null lies in" and adds "range no value lies in"
  for a range that excludes null. This settles what `CONFORMANCE.md`
  overstated for `UN-004` and `UN-005` since 0.4.0; `DI-011` still waits.
- The module's Go directive is `go 1.26.0`, the floor `golang.org/x/text`
  v0.42.0 sets, where it named the toolchain's patch release and demanded
  more of consumers than anything in tenon needs.
- `gotenon`'s boundary messages stay readable and its contracts panic as
  usage errors. A message that embeds a value cuts it to 32 bytes at a
  character boundary: decoding the widest in-window number into an int64
  reported about two megabytes of digits. A marshaler or unmarshaler
  returning a `*DiagnosticError` holding no error value is named a broken
  contract where rendering it once panicked inside `Error`. The kind
  switches of decoding and encoding end in a panicking arm naming gotenon
  as the defect's home, and a message naming a tenon type renders the
  type once per call however many parts name it.
- `Narrow` judges every narrowing before it answers anything: a narrowing
  that could never apply panics as a usage error wherever it stands, where
  a contradiction among the narrowings before it once answered first, and
  an error operand no longer swallows the zero Narrowing. `WithMarks`
  refuses a mark that does not equal itself, one holding a NaN, which
  would vanish from every lookup that stored it; a mark of a type that is
  not comparable was already refused, and the message now says which
  defect it met.
- `Deserialize` refuses a nil mark decoder up front as a usage error, and
  panics on a mark decoder returning a mark whose type declares no
  encoding, a broken contract that once surfaced as
  `serialize.not_canonical` after the round trip failed. A path step that
  is not well-formed text reports the text failure as
  `serialize.malformed`, where it was read as an empty name. Deserialize's
  doc now names every panic.
- `Capsule` refuses an encoding identifier that is not valid UTF-8, as a
  usage error at the declaration, as serializing a mark whose identifier is
  not valid UTF-8 is refused. Through 0.5.0 the declaration was accepted,
  `Serialize` wrote the bytes as CBOR text, and `Deserialize` refused what it
  wrote as `serialize.malformed`.

### Deprecated

- `Type.Equals`, for `Type.Equal`. It answers as `Equal` does and is
  removed at 1.0. The package-level `Equals`, the language's equality of
  values, is unchanged.

### Removed

- The Go packages `conformance`, `conformance/values` and
  `conformance/matrix` move to `internal/conformance`, out of the public
  API. They exist for tenon's own tests (the coverage harness, the corpus of
  values and the operand matrix), and a public package is a promise kept
  past 1.0. What other implementations test against stays where it was:
  `conformance/vectors` with its encoding and diff corpora, and the rule and
  code records `conformance/rules.json` and `conformance/codes.json`.

### Fixed

- `gotenon.Decode` gives a `tenon.Value` and an unmarshaler within a
  container the member as it was handed, before the container is
  converted. A conversion carries only Propagate marks, so an Isolate mark
  on a struct field, a slice element or a map element was dropped before the
  field's `tenon.Value` or the `UnmarshalValue` method saw it, a redacting
  one included; and under Unsafe a tuple of a number and a string decoded
  into `[]T` gave each method the string its member converted to. Both now
  receive the member as it is, as the specification says.
- Division refuses a terminating quotient whose last digit lies below the
  digit window before computing it: `1 / 2^3000000` is refused from the
  denominator's twos and fives where it was refused after ten million
  digits were built, 755 milliseconds at 0.4.0. A quotient that rounds is
  left to the rounding path, since it can round into the window however
  deep its denominator, and a divisor inside the window divides as before.
- A partly known set's least length is counted once for the value's
  lifetime: the count follows from the members, which never change, so
  asking `Length`, `Equals` or a conversion again reads the count already
  made, where each ask counted every pair of members that are not known.
  Narrowing such a set by a listing of its own members costs one comparison
  a listed value, where the members and the listed values were counted
  pairwise: 3,004,181 comparisons at 1,000 members, 2,228 now. Counting a
  member identical to the one beside it asks nothing more, and equal
  encodings hold identical members side by side. What remains pairwise is
  the first count of a value's members, whose answer EQ-042 promises
  exactly, and membership between two such sets under `Equals`.
- `Diff` of two sets whose changed members read alike mirrors. A member
  removal and a member addition whose display forms tie are ordered by the
  members themselves, with the comparison a set uses for members that encode
  alike, so `Diff(b, a)` is `Diff(a, b)` with the sides exchanged, in the
  same order. Through 0.5.0 each direction put its own removal first, and
  the two disagreed.
- `gotenon.Encode` of a rational that does not terminate fails with
  `encode.inexact` however large its denominator. Through 0.5.0 a
  denominator of more bits than the digit window has places was refused as
  `number.out_of_range` without being factored, so `1/3^2100000` was called
  out of range where `1/3` is inexact. Whether the number terminates is now
  established from the denominator's bits alone, at no allocation, before
  its size refuses it; only a denominator whose fives hide another factor is
  still refused for its places. The inexact message writes the rational only
  while it is small, since turning megabytes of bits into digits costs more
  than a refusal may.
- `gotenon.Encode` reports a value's failures in member order: a struct's
  attributes by name, and a map's keys by their normalized spelling. Through
  0.5.0 a struct's failures came in the order its fields are declared, and a
  map key that normalization moves (an `e` with a combining acute normalizes
  to `é`, which follows `f` where its raw spelling precedes it) was reported
  on the wrong side of its neighbors. Only the order of diagnostics moves;
  the values encoding gives are as they were.
- `Deserialize` refuses a two-byte simple value (`f8` followed by a byte
  below 32) as `serialize.malformed` at its own byte, since RFC 8949 §3.3
  says such a sequence is not well-formed CBOR. Through 0.5.0 the reader
  accepted `f8 14` and `f8 15` as false and true, and `f8 16` shifted it one
  byte, so the second byte was read again as the next item; every such
  document was still refused, but as `serialize.not_canonical` at whatever
  byte the re-encoding first differed. No document that decoded before is
  refused now.
- `ProjectJSON` of a capsule value whose declared display form returns text
  that is not well-formed UTF-8 writes each ill-formed byte as U+FFFD, as the
  display form writes it, so the projection is the JSON text it promises.
  Through 0.5.0 the bytes were copied into the output as they were.

## 0.5.0 (2026-09-24)

Bounded work, kept. 0.4.0 promised that `Deserialize` does work that grows no
faster than n log n in the length of what it is given, however that is shaped,
and four shapes of document broke the promise; `gotenon.Encode`, which
SECURITY.md holds to the same standard, took the square of a JSON array of
nulls and of a rational's length. This release fixes each of them, and the
amplifications inside the bound that the same audit measured, and holds every
fix to a test that fails if the fix is undone. It implements version 0.4.0 of
the tenon specification, which amends `TY-041`, `UN-002` and `UN-004`: 195
rules, as before.

**Upgrade if you decode documents or encode Go values from input you do not
trust.** In 0.4.0 a document of 40 KB holding objects of a type that names one
long attribute took 1.75 seconds to decode; one of 79 KB listing two sets whose
members are partly known, which is canonical and decodes, made 256 million
comparisons and took 2.46 seconds; a set of 4,000 unknown members over a long
type held 234 MB live while it decoded; and `gotenon.Encode` of the 32,000
nulls `encoding/json` reads from 160 KB of JSON took 8 seconds. Each costs time
in proportion to its input here.

The minor version moves because two changes alter what a program sees. A set
holds, and iterates, its members that are not known in the order of their
encodings, which is the order `Elements`, the display form and `Diff` give
them; and the least length a `Members` narrowing implies for a set that is not
known counts only the distinct known values it lists. Both are under Changed
below.

**Upgrading from 0.4.0.** Documents 0.4.0 wrote decode as they did. A range
listing values that are not known records a least length of one where 0.4.0
recorded more, so the document 0.5.0 writes for it is not the encoding 0.4.0
writes, and 0.4.0 refuses it as not canonical: upgrade whatever reads such
documents before whatever writes them. `CapsuleOps.Equals` must be an
equivalence relation, as tenon already assumed of it, and tenon may take a
capsule value to equal itself without asking.

**What `CONFORMANCE.md` still overstates.** As in 0.4.0: it reports 195 of
195, and every rule does have a passing test, but for `UN-004` and `UN-005`
(what a narrowing leaves when only null remains) and `DI-011` (path keys that
carry marks) the test exercises a narrower case than the rule states. Both
wait on a decision about what the rule should say.

### Changed

- A set holds, and iterates, its members that are not known in the order of
  their encodings, where it ordered them by their display forms. A display
  form spells out the member's type, so a document stating one large element
  type and listing many unknown members of a few bytes each cost the type's
  text for every member: 54 kilobytes of such a document decoded in 1.2
  seconds and allocated 620 megabytes, and decodes in 1.8 milliseconds and
  2.8 megabytes. The order `Elements`, the display form and `Diff` give those
  members changes; known members, and every encoding, do not. Members told
  apart only by a capsule value whose type declares no encoding follow the
  order of those capsule values, which is their type's order where it
  declares one.

- The least length a `Members` narrowing implies for a set that is not known
  is the number of distinct known values it lists, or one where it lists only
  values that are not known. It was the number of listed values that are
  provably distinct, which compares every pair of the values that are not
  known: decoding a range listing 8,000 of them made 32 million comparisons
  and makes 16,000. The range is wider where such values are provably
  distinct, and three things follow. A range listing them records a least
  length of one where it recorded more, so a document this version writes for
  it is not the encoding 0.4.0 writes, and 0.4.0 refuses it as not canonical;
  documents 0.4.0 wrote still decode. A listing of as many such values as the
  greatest length allows stays a range, where it became the set holding them.
  And a narrowing those values could satisfy in no set within its lengths,
  only because they are provably distinct, is no longer refused as a
  contradiction. A set value narrowed by a listing is judged as before.

### Fixed

- Collecting the diagnostics of a value that fails in many places takes time
  in proportion to them, where it took the square of them. Building a list of
  20,000 error members took 1.0 second and takes 13 milliseconds; an operation
  over two error values carrying 20,000 diagnostics between them took 6.5
  seconds and takes 18 milliseconds; and `gotenon.Encode` of a `[]any` holding
  20,000 nils, which is what an array of nulls read by `encoding/json` gives,
  took 3.1 seconds and takes 18 milliseconds. Each diagnostic was compared
  with every diagnostic recorded, so that one arriving twice is recorded once;
  past a handful, each is now looked up by a key that two diagnostics share
  exactly when they are equal, as the encoder has done since 0.4.0. `Convert`
  and `gotenon.Decode` collect their failures the same way and are bounded
  with them. A value that fails in a handful of places, as nearly every one
  does, allocates nothing more than before.

- Decoding a document of many objects of one type takes time and memory in
  proportion to the document, where it took the square of it. The decoder
  rendered the object type into a message that only a malformed document
  would have used, and then gathered the attributes into a map and worked the
  type out again, once for every object; it now builds the object from the
  type it has already decoded, and renders that type only where the content
  is not the array it should be. A 10 KB document of 1,000 objects whose type
  names one long attribute decoded in 64 milliseconds and decodes in 0.4, and
  an ordinary document of 1,000 objects of three attributes decodes 2.8 times
  faster and allocates 2.6 times less.

- Comparing two sets of known members takes time in proportion to them, where
  it took the square of them. A set holds its members in the canonical order,
  which ties exactly the ones that are equal and holds each of them once, so
  two sets with the same members hold them in the same order and one walk
  decides it, as the canonical comparison of two sets already did; membership
  each way compared every pair. `Equals` of two sets of 8,000 members took
  399 milliseconds and takes 77 microseconds, and `Identical` took 1.6 seconds
  and takes 0.2. A set asked about many values at once, which is what deciding
  two sets that hold members which are not known comes down to, now indexes
  its known members by hash once rather than scanning them for each: a
  document listing two such sets of 400 members made 2.6 million comparisons
  of the values inside them and makes 4,572.

- A Go `big.Float` or `big.Rat` outside tenon's digit window is refused from
  the places it has rather than by working them out, and one inside it costs
  what the arithmetic costs rather than the square of it. `gotenon.Encode` of
  a float a place below the window was refused after 17 milliseconds and
  2.3 megabytes of working, and is refused in 39 microseconds and 904 bytes;
  a rational of 1,430,000 places took 10 seconds and takes 0.24; and one of
  400,000 places, which is inside the window and encodes, took 0.71 seconds
  and takes 0.03. The fives under a rational were divided out one at a time,
  which costs the square of the denominator; they are now counted by squaring,
  and the denominator's size decides the far cases before it is even copied.

- A set over an element type holding few values costs what a set over any
  other element type costs. The values such a type holds, up to 256 of them,
  were built again for every set that held a member which was not known, and
  each such member was then asked about every one of them and every value the
  set held already, which is about 30,000 comparisons for one member. The
  values are now built once for the type and kept on it, and what the set does
  not hold already is worked out once for the set. A document of 1,000 sets
  over a tuple of five Bools, each holding one unknown member, decoded in
  599 milliseconds and allocated 533 megabytes; it decodes in 5 milliseconds
  and allocates 4.3, which is what the same document over a tuple of five
  Numbers costs. A member that is not known beside the 243 values of such a
  type cost 0.37 milliseconds to decide and costs 0.6 microseconds.

- Decoding a value nested level upon level under deep marks, which a document
  can be, takes time and memory in proportion to the marks the value holds,
  where it took the cube of the levels. The decoder attached each level's deep
  mark to everything below it as the level was read, so every value's marks
  were merged again for each level above it, and each merge built a set of the
  marks held to look one up and sorted the whole list again. It now reads each
  value with the marks listed on it and, once the value is read, gives every
  value within it the deep marks above it in one pass, merging each value's
  marks once and taking them as they are. A document of 83 kilobytes holding
  sixteen nests of 240 levels, each level carrying a deep mark of its own,
  decoded in 2.3 seconds and allocated 4.1 gigabytes, and decodes in 65
  milliseconds and allocates 53 megabytes; a list of 10,000 numbers under 250
  levels of deep marks decoded in 309 milliseconds and decodes in 7.3.
  Serializing asks each mark once whether it is deep rather than once for
  every set of marks holding it. A value decodes carrying the marks it did,
  in the order it did.

- Comparing two values that carry many marks takes time in proportion to the
  marks, where it took the square of them: `Identical` of two values carrying
  16,000 marks took 0.91 seconds and takes 0.18 milliseconds. Every mark of
  one value was looked for among the other's by scanning them. A value holds
  its marks in one order, so the two lists are walked together, and a mark
  that does not line up, which is one sharing its identifier with another, is
  looked up through a set of them past sixteen, as attaching a mark has been
  since 0.4.0. Two values carrying 4,000 marks in runs of eight that share an
  identifier, so that none of them line up, were compared in 38 milliseconds
  and are compared in 0.41. `Diff` compares the marks of every value it walks
  and takes the same bound with it. A value carrying a handful of marks, as
  nearly every value does, allocates nothing more than before.

- `Equals` settles a known value compared with itself without comparing what
  it holds, and a part that two values share the same way, as `Identical`
  already did. A plan engine compares a prior configuration with a planned
  one that shares every part that did not change, and paid for the shared
  parts all the same: a configuration of 4,000 resources compared with
  itself, or with a planned one sharing them, took about 100 microseconds and
  takes about 70 nanoseconds, whatever its size. A value that is not known is
  not known to equal itself, so the answer for one is what it was, and every
  result carries the marks it carried. `CapsuleOps.Equals` now states what
  the package already assumed of it: a capsule type's equality must be an
  equivalence relation, and tenon may take a value to be equal to itself
  without asking it.

- Building a container no longer names every member it checks. `ListVal`,
  `SetVal`, `TupleVal`, `ObjectVal` and `MapVal` wrote a name for each member,
  as in element 3 or attribute "name", for the panic they would give were the
  member wrong, which it nearly never is: a list of 100 numbers made 106
  allocations, 100 of them names, and makes 6, at any length. At 100 members,
  a tuple makes 12 allocations where it made 112, a set 125 where it made 225,
  an object 41 where it made 241, and a map 18 where it made 218. A name is
  written only when a constructor panics, and every message reads as before.

- Writing and reading a number builds no big integer for it unless it needs
  one: where its digits, without trailing zeros, fit in 64 bits, as nearly
  every number's do. Writing an integer took two allocations for that and
  reading any number four, and `Deserialize` pays for both, since it writes
  the value it read again to check that its input is canonical. A list of 100
  small integers took 508 allocations to serialize and 1,119 to deserialize,
  which this change alone brings to 309 and 523, and the next to 10 and 224.
  Every encoding is what it was.

- Writing a value builds the path to a member only where the member fails to
  write. `Serialize` built the path to every member it wrote, for a
  diagnostic nearly no member needs: three allocations for an element of a
  list, a set or a tuple, or an entry of a map, and one for an attribute.
  `Deserialize` paid it again, since it writes what it read to check that
  its input is canonical, and `ProjectJSON` paid it too. A list of 100 small
  numbers serializes in 10 allocations where it took 309, deserializes in 224
  where it took 523, two for each number, which is the value it is, and
  projects in 107 where it took 406. Every diagnostic is located where it
  was, and failures under one member still share the path to it.

- An operation, a container built from error values, a narrowing by bounds,
  and `Diff` take the marks of what they consume in time proportional to the
  marks, where each looked for every mark among those it had taken already,
  which cost the square of them. At 4,000 marks an operation over two
  operands carrying them took 38 milliseconds and takes 1.3, a list of 4,000
  marked error values took 42 and takes 2.9, and a diff under 4,000 deep
  marks took 84 and takes 1.3. A value carrying a handful of marks, as nearly
  every value does, allocates less than before.

- Decoding a range that lists sets holding many members that are not known
  takes time in proportion to them, where it took the square of them: two
  listed sets of 1,000 such members, 22 kilobytes, made a million
  comparisons of those members, and make one. The listing asked whether two
  listed sets are one value by counting each set's least length, which
  compares every pair of the members that are not known, although only known
  values are ever settled equal; and `Identical` counted each such member's
  repeats in both sets. A set holds those members in an order that puts
  identical ones together, so `Identical` walks two sets' members side by
  side: two sets of the same 400 such members, given in two orders, are
  compared in 400 comparisons where they took 319,200.

## 0.4.0 (2026-09-21)

Bounded work on input from outside. `Deserialize` now promises that its work
grows no faster than n log n in the length of what it is given, however that
is shaped, and number text is read only up to 10,000 characters. It implements
version 0.3.0 of the tenon specification, which amends `SE-005` to make that
promise and adds `NU-024`: 195 rules where 0.3.0's had 194.

**Upgrade if you build strings or decode documents from input you do not
trust.** 0.3.0 and earlier normalize a long run of combining marks in time
that grows with the square of the run: a string of 200 KB takes 70 seconds to
build, and a document of 80 KB holding such a run takes 6.2 seconds to decode
before it is refused. `String`, map keys, attribute names, `Deserialize` and
`gotenon.Encode` of text all reach it. Three other shapes cost the square of
their size in 0.3.0 and no longer do: a set's members that are not known, the
marks on one value, and the failures the encoder reports.

The minor version moves because one change refuses what 0.3.0 accepted:
number text longer than 10,000 characters, through `NumberFromText`,
conversion from a String or a `json.Number`, now gives an error value with the
new code `number.too_long`. A number of more digits is still a number:
`NumberFromBigInt`, which is new, makes one from a `*big.Int`, and `gotenon`
makes Go's big numbers that way, where it read them back from text.

**Upgrading from 0.3.0.** Documents 0.3.0 wrote decode as they did, and
nothing that decoded then is refused now: nesting deeper than 512 levels was
refused before and is the only refusal for size. No value, answer or encoding
changes, except that number text past 10,000 characters is refused.

**What `CONFORMANCE.md` still overstates.** It reports 195 of 195, and every
rule does have a passing test, but for `UN-004` and `UN-005` (what a narrowing
leaves when only null remains) and `DI-011` (path keys that carry marks) the
test exercises a narrower case than the rule states, as the audit found. Both
wait on a decision about what the rule should say.

### Added

- `NumberFromBigInt` makes a Number from a `*big.Int` exactly, without
  rendering it as text, as `AsBigInt` reads one back. It is how a number of
  more digits than `NumberFromText` reads is made, and `gotenon` now makes Go's
  `big.Int`, `big.Float` and `big.Rat` values this way, from their coefficients,
  where it rendered them as text and read the text back.

### Changed

- `Deserialize` promises what it costs: its work grows no faster than n log n
  in the length of the input, however the input is shaped (`[SE-005]`), so a
  caller taking documents from outside bounds the cost by bounding their
  length. The specification allowed a decoder to bound what it decodes, and
  three shapes a document could take made decoding quadratic; they are fixed
  below, and nothing is refused for its size but nesting deeper than 512
  levels, as before. `SECURITY.md` says so.

- Number text longer than 10,000 characters is refused before it is read,
  with the new code `number.too_long`, by `NumberFromText`, by conversion from
  a String, and by `gotenon.Encode` of a `json.Number` (`[NU-024]`). Reading
  decimal digits costs the square of their number: 10,000 characters parse in
  about a tenth of a millisecond, and a million, still a number in range, took
  more than a second, so a few megabytes of digits in a JSON document cost
  seconds. The message gives the length of the text rather than the text. A
  number of more digits is still a number: arithmetic makes one, `Deserialize`
  reads one from its coefficient's bytes, and `NumberFromBigInt` makes one.

### Fixed

- Normalizing text that holds a long run of combining marks takes time in
  proportion to the run, where it took the square of it. tenon normalizes by
  plain UAX #15, without the Stream-Safe Text Process, so nothing caps a run,
  and canonical ordering sorted each one by insertion: a string of 100,000
  marks took 70 seconds to build, and a document of 80 KB holding such a run
  out of order took 6.2 seconds to decode before it was refused as not
  canonical. They take 15 and 11 milliseconds. Every string tenon builds is
  normalized, so this reached `String`, map keys, attribute names, decoding,
  and `gotenon.Encode` of text from outside, such as a JSON document. The
  order a run is sorted into is the same.

- Building or decoding a set whose members are not known takes time in
  proportion to the members, where it took the square of them: a set of 8,000
  unknowns took half a second to decode and decodes in 16 milliseconds. Each
  such member was compared with every member already kept, looking for one
  `Equals` settles equal to it, and `Equals` never settles a value that is not
  known equal to anything, so no comparison could succeed. Which members a set
  keeps is unchanged.

- Attaching many marks to one value, which decoding does with the marks a
  document lists, takes time in proportion to the marks, where it took the
  square of them: a value carrying 4,000 marks took 40 milliseconds to decode
  and takes 2.4. Each mark was looked for in the list of marks so far; past
  sixteen they are looked up in a set. A value carrying a handful of marks, as
  nearly every value does, allocates nothing more than before.

- Serializing a value with many parts that cannot be serialized takes time in
  proportion to them, where it took the square of them: a list of 20,000
  capsule values of a type that declares no encoding took 2.6 seconds to
  report them and takes 16 milliseconds. Each failure was compared with every
  failure recorded, so that one arriving twice is recorded once; each is now
  looked up by its encoding. `Deserialize` encodes what it decoded, so this
  reached decoding too.

## 0.3.0 (2026-09-21)

Most of what the 0.1.0 audit found that did not wait on a decision: answers
the rules did not allow, and work that grew faster than its input. Beside
those, documentation a consumer can start from, and data whose types a Go
program does not know. It implements version 0.2.0 of the tenon
specification, which amends
`TY-017`, `UN-005`, `EQ-041` and `EQ-042`, and adds `GO-015` and `GO-034`,
amending `GO-010` and `GO-011` with them: 194 rules where 0.1.0 had 192.

The minor version moves rather than the patch because two changes break a
0.2.0 user. `gotenon` encodes a `json.Number` as the Number its text spells,
where 0.2.0 gave a String, and a few encodings 0.2.0 wrote no longer decode.

**Upgrading from 0.2.0.** Documents 0.2.0 wrote still decode, with two
exceptions, both over sets whose element type holds at most 256 values, as a
set of bools does: a range recording a length bound the element type already
sets, and a set holding a member that is not known where its other members
leave it nothing to be. Both now fail with `serialize.not_canonical`, since
the value such bytes describe is spelled differently now. `gotenon` decodes a
`json.Number` from a Number, as it encodes one. Where an operand is not
known, `Mul`, `Div`, `Mod` and `LessThan` now give bounds and answers where
0.2.0 gave a bare unknown, and a factor of zero gives a known zero, so the
values they compute, and the encodings of those values, move; nothing moves
where every operand is known. One diagnostic code is added,
`encode.untyped_nil`, and some inputs are reported under a different code
than 0.2.0 gave: `operation.wrong_type` for a pending operand of a type the
operation rejects, where 0.2.0 answered an unknown, `convert.unexpected_attribute`
where a constraint admits one type and `Exactly` of that type reports it,
where 0.2.0 gave `convert.no_conversion`, and `map.duplicate_key` for keys that
normalize alike beside an error element. Encoding a value of an interface type
no longer panics; one that holds itself is refused as a usage error.

**What `CONFORMANCE.md` still overstates.** It reports 194 of 194, and every
rule does have a passing test, but for these the test exercises a narrower
case than the rule states, as the audit found. This release closes `UN-023`,
`CV-026`, `MK-005` and `TY-017`. Still outstanding: `UN-004` and `UN-005`
(what a narrowing leaves when only null remains) and `DI-011` (path keys that
carry marks), each waiting on a decision about what the rule should say.

### Added

- `gotenon.Encode` takes data whose types a Go program does not know. A value
  of an interface type encodes as what it holds, so the `map[string]any` that
  `encoding/json` gives encodes as an object of whatever the document held, a
  slice of anything as a tuple, at any depth. It was a usage panic, which made
  the commonest input to a library for data whose types are not known at
  compile time the one thing it refused.

  A `json.Number` is now the number its text spells rather than text, in both
  directions. Read a document with `json.Decoder.UseNumber` and its numbers
  arrive exactly, keeping the distinction the document drew between `8080` and
  `"8080"`, so a schema asking for numbers is met under the safe policy; read
  it without, and each number is the `float64` nearest to it, which is a
  different number and says so.

  An interface is also the only way a Go value comes to hold itself, since a
  Go type that holds itself has no mapping at all, and a value that does is a
  usage error rather than a stack that runs out. Holding the same value in
  two places is not holding itself.

  Two things such data cannot carry by itself, and both are reported rather
  than guessed. A JSON null says null without saying null of what, so encoding
  a nil interface fails with the new code `encode.untyped_nil`, located by its
  path, and the schema the caller converts to says which null it meant.
  Decoding back into an interface is a usage error: nothing in a value says
  which Go type it would take, so decode into `tenon.Value`, which holds any
  value.

- Documentation a consumer can start from. The package doc is a tour of the
  package in thirteen sections, each naming the API to look at next, where it
  was a definition and a note about Unicode. Nineteen examples in the root
  package and four in `gotenon` show the API a consumer meets first and build
  the use cases the README claims: a validation layer over untrusted JSON, a
  plugin protocol across a process boundary, a plan engine that shows a
  secret changing without showing the secret, and the value layer of a
  configuration language. The README opens with a program and its output
  rather than three claims. Each of these is held to the code by a test: the
  package doc's links must name symbols that exist, the README's code must
  be a line-for-line copy of an example the suite runs, and every example's
  output is the output it gave, so documentation that drifts fails the gate
  rather than the reader.

### Fixed

- `gotenon.Encode` locates a failure in a map that encodes as an object, one
  whose members' types need not agree, by the attribute of that object rather
  than by an index of the map, so the path names a place the value it
  describes has.

- An operation given a pending operand that can only be of a type it rejects
  gives an error value with code `operation.wrong_type`, however the operand's
  constraint is written, as `[UN-023]` requires. 0.2.0 decided this only where
  the operation accepted one type or one of several, or the constraint was
  written `Exactly`, and answered an unknown elsewhere: the length of a
  pending `Exactly(NumberType())` or `TupleOf()` was an unknown number,
  `Contains` over a pending list an unknown Bool, and `LessThan` of a pending
  `OneOf(Exactly(NumberType()))` and a string an unknown Bool. Nothing in
  those answers said the data was wrong, and resolving the operand turned the
  next call into a usage panic (`[ER-001]`), so a type error in a
  configuration reached its caller as a panic rather than as an error value.
  Whether constraints have a type in common is now decided for every
  constraint, part by part, rather than only for one written `Exactly`.

- `Equals` answers what the constraints of pending operands settle, however
  they are written. Two operands whose constraints share no type are unequal
  (`[EQ-005]`), as a pending list and a pending set are, and a pending value
  known to be null equals the null of the one type its constraint admits, as
  one written `TupleOf()` does the null of the empty tuple. 0.2.0 answered an
  unknown Bool for both unless a constraint was written `Exactly`.

- `Convert` gives an error value with code `operation.wrong_type` for a
  pending value converted to a constraint that no type satisfies, such as
  `OneOf()` or `ListOf(OneOf())`, where a value in any other state already
  failed, with `convert.no_conversion`. 0.2.0 gave a pending value carrying
  that constraint, which could never be resolved. Such a pending value is now
  refused by every operation, since none can apply to it whatever it turns
  out to be.

- A mark that does not redact no longer changes a diagnostic's message, as
  `[MK-005]` requires. A message that rendered a value rendered its marks too,
  at any depth: `Convert` of a marked string that does not parse gave
  `marked("a", "origin") is not a number`, and `Narrow` of a marked known
  value that a narrowing rules out gave `the value marked(1, "origin") does
  not satisfy >= 5`, so the error value differed from the one the same call
  gave unmarked. Only a redacting mark changes a message now, by the
  placeholder that stands in for what it withholds (`[MK-011]`).

- Converting to a constraint that admits exactly one type converts as
  `Exactly` of that type does, however the constraint is written, as
  `[CV-026]` requires. 0.2.0 did so only for a `OneOf` or a capsule value. A
  closed `ObjectWith` that names an optional field no type can fill, such as
  `"z"?: one_of([])`, admits one type, and 0.2.0 reported an attribute of
  that name as `convert.no_conversion`, where `Exactly` of the type reports
  `convert.unexpected_attribute`, as it now does for an object or a map in
  any state, alone or within a collection. A message that names the target
  names it as `Exactly` of the type does, whatever the spelling.

- `MapVal` reports keys that normalize alike with `map.duplicate_key`
  whatever their elements are, as `[TY-017]` requires, and gives its
  diagnostics in the order of the keys, normalized (`[ER-008]`). 0.2.0 looked
  for such keys only among the elements that were not errors, so an error
  element under U+00E9 beside a number under e followed by U+0301 gave no
  `map.duplicate_key`; and it hoisted error elements in the order of the keys
  as given, so two spellings of one map holding error elements listed the
  same diagnostics in different orders, and were not `Identical`. Each entry
  now reports in the order of its key, a key that is not well-formed UTF-8
  placed by its bytes, and a shared key follows them all, as `[TY-017]` now
  states.

- `Narrow` allows a set holding members that are not known every length it
  can have, from the count of those that are provably distinct to the count
  of all of them, as `Length` reports it (`[EQ-042]`). 0.2.0 held such a set
  to the count of members it holds, so `LengthMax(1)` on a set of two unknown
  numbers was a `range.contradiction`, although the two could be one member.
  The narrowings of one call are now decided together, since such a set has
  no range to record what each says: `LengthMin(2)` beside `LengthMax(1)`
  contradicts it, as does a listing of more values than it could hold, such
  as `Members` of 1, 2 and 3 on a set of two unknowns, which 0.2.0 let
  through. A narrowing that leaves the set no more members than its known
  ones gives the set of those, known, as `[UN-005]` requires: `LengthMax(1)`
  on the set of 1 and an unknown number gives the set of 1.

- A set is no longer than the values its element type holds, null among them,
  and `Narrow` and `Length` both say so where that type holds few of them: a
  set of bools has at most three members, one for each bool and one for null.
  0.2.0 let a range of such a set ask for more, so
  `Narrow(Unknown(Set(BoolType())), LengthMin(4))` was an unknown set no value
  could ever be; it is now a `range.contradiction` (`[UN-001]`, `[UN-004]`).
  A narrowing that leaves such a set every one of those values, null excluded,
  gives the set holding them, known, as `[UN-005]` requires, where there are
  at most 256 of them; above that the range stands, since the values of a type
  can be far more numerous than the text of the type. A length bound that the
  element type already sets is not recorded, so one range describes one set of
  values: `LengthMax(3)` on an unknown set of bools leaves the range as it was.
  An encoding written by 0.2.0 that records such a bound no longer decodes,
  since the range it describes is not the one the bytes spell
  (`serialize.not_canonical`).

- A set holding members that are not known is held to what its members can be,
  one member apiece, since one member is one value. `Equals` of a known set and
  such a set is false where no way of resolving the members gives that set, as
  it is for `{1, 2, 3, 4}` against a set of three members between 1 and 2 and
  one between 3 and 4, whose one member between 3 and 4 would have to be both 3
  and 4 (`[EQ-003]`). Narrowing such a set by `Members` is a
  `range.contradiction` where the listed values the set does not hold cannot
  each be given a member of their own (`[UN-004]`), and where the lengths leave
  it no more members than the values it must hold, the result is the known set
  of those (`[UN-005]`): `Members` of 1 and 2 on a set of two unknown numbers
  is the set of 1 and 2, and a set of three unknown bools narrowed to a length
  of three is the set of every bool. 0.2.0 answered an unknown Bool and left
  the set as it was in each of those.

- A set does not keep a member that is not known where every value that member
  could be is already a member of it, as `[EQ-041]` now says: the set of
  `false`, `true`, null and an unknown bool is the set of the three, known, and
  so is the set of `false`, `true` and a bool known only not to be null. 0.2.0
  kept the member, so the value reported itself not known although its range
  held one set (`[VA-003]`), `Equals` could not tell it from the set of the
  others (`[EQ-002]`), and one value had two encodings (`[SE-001]`). Which
  values a member could be is decided where the element type holds at most 256
  of them, as `[UN-005]` counts them, and every member is kept where it holds
  more. An encoding written by 0.2.0 that holds such a member no longer
  decodes, the members it spells not being the ones the set holds
  (`serialize.not_canonical`).

### Changed

- `Mul`, `Div`, `Mod` and `LessThan` answer what their operands' ranges
  settle, as `Add`, `Sub` and `Equals` already did. Twice a number at least 2
  is a number at least 4, a quotient is bounded where the divisor keeps away
  from zero, a remainder lies between zero and its dividend and is smaller
  than its divisor can be, and a number of 1024 or more is known not to come
  before 80. `LessThan` of strings reads their prefixes, as far as
  normalization leaves them standing. A factor of zero makes a product known
  zero however little is known of the other. 0.2.0 answered each of these
  with a bare unknown, which the rules allow and which told a plan nothing.

  A quotient's bounds are rounded outward. Division is exact where a quotient
  terminates, at any length, and rounded to 96 digits where it does not, which
  is not monotone: `(1e40 - 1e-100)/3` terminates in 140 threes and is greater
  than `1e40/3` rounded to 96 of them, although it is the smaller quotient. A
  bound on a range of quotients is therefore rounded down or up at 96 digits,
  and is the quotient itself where that terminates within them.

- Converting to `Exactly` of a list, set, map, tuple or object type builds
  the constraint it converts by once for the type, not again for every
  member, which makes converting a list of objects to one about a fifth
  faster.

- Recording the members a range lists, and counting how many of a set's
  members are provably distinct, take the time the order a set holds its
  members in already gives them, rather than comparing every pair of known
  members. A range listing 16,000 known numbers, a document of 47,746 bytes,
  decodes in six milliseconds where 0.2.0 took three and a half seconds, and
  `Length` of a set of 8,000 known members beside one that is not known takes
  69 microseconds where it took 300 milliseconds. Neither allocates any more
  than before. Members that are not known are still compared with one another,
  which is what telling those apart takes.

- Serializing a value that carries deep marks settles what each mark set
  under it lists for itself once, against the marks its container implies
  held as a set, rather than testing every mark on every value against them
  one by one and computing the deep marks of every known value it meets,
  containers and numbers alike. A list of 1,000 numbers under 400 distinct
  deep marks with payloads, a document of 10,634 bytes, serializes in 236
  microseconds where 0.2.0 took 0.72 seconds and deserializes in 1.4
  milliseconds where it took 0.68 seconds, in a sixtieth and a thirtieth of
  the memory. Deserializing pays what serializing pays, since it encodes the
  value it decoded to check that the input is that value's encoding.

- `ProjectJSON` records the diagnostic of each member that does not project
  as it meets it, rather than scanning the diagnostics it has recorded for
  one equal to it first. It visits each path once and fails at most once
  there, so no two of them can be equal; a container under construction,
  which can meet one diagnostic on many members, still looks. Projecting a
  list of 20,000 unknowns takes 5.8 milliseconds where 0.2.0 took 2.78
  seconds, and 5,000 take 1.3 milliseconds where they took 165. The
  projection itself, in bytes and in diagnostics, is what it was, and so is
  the memory it takes.

- Unifying object types, which converting a collection to `ListOf`, `SetOf`
  or `MapOf` does over its members, builds the union in one pass: it holds
  every attribute of every type, each the unification of the types that hold
  that attribute. Unifying two at a time built the union again for every type
  after the first, and fitting each member to it gathered another map per
  member and interned the type they all share again. Converting a tuple of
  2,000 objects of distinct attributes to a list of anything takes 162
  milliseconds and 216 MiB where 0.2.0 took 2.6 seconds and 2.2 GiB, and
  8,000 nulls of such object types take 5.9 milliseconds and 6 MiB where they
  took 10 seconds and 7.8 GiB. The objects still grow as the square of their
  number, since each of them gains every attribute of all of them, but the
  memory now grows with that output rather than faster than it.

- `conformance/matrix` reports a `UN-023` violation where an operation answers
  a pending operand that can only be of a type it rejects without an
  `operation.wrong_type` diagnostic. For each operand position it builds
  pending operands whose constraints name a type and a kind of type (a list,
  set, map, tuple or object of anything), one it accepts and one it rejects
  of each. It had built the one naming a type and asserted nothing of it,
  which is how the defects above passed the gate.

## 0.2.0 (2026-09-18)

A fix release for the six critical findings of an architecture audit of
0.1.0: two panics on data, three classes of wrong answer, and a Unicode
version that followed whichever Go toolchain a consumer built with. It
implements version 0.1.0 of the tenon specification, unchanged: no rule was
amended, added or withdrawn, and the count stays at 192.

The minor version moves rather than the patch because value identity moved
with it. A string of more than thirty non-starters is a different value than
0.1.0 built, and `Equals`, `Length`, `Contains`, `Narrow` and `Convert`
answer differently where an operand could still be null. Encodings of those
values move with them. No diagnostic code changed, and no panic was added:
two were removed.

**Upgrading from 0.1.0.** Documents 0.1.0 wrote still decode, and a Go 1.27
build now reads a Go 1.26 build's documents and writes the same bytes, which
0.1.0 did not. Two things do not carry over: a value built from text holding
a run of more than thirty non-starters, which 0.1.0 gave a U+034F it should
not have, and the recorded length of a listing of members that could each be
null, which 0.1.0 wrote as `length >= 2` and this release would derive as
`length >= 1`. The document keeps what it recorded either way.

**What `CONFORMANCE.md` still overstates.** It reports 192 of 192, and every
rule does have a passing test, but for these the test exercises a narrower
case than the rule states, as the audit found. This release closes `EQ-003`,
`EQ-030`, `EQ-032`, `EQ-042`, `EQ-043`, `EQ-045`, `DI-012`, `DI-031`,
`ER-002`, `ST-003` and `UN-007`. Still outstanding: `UN-004` and `UN-005`
(what a narrowing leaves when only null remains), `UN-023` (`Length` and
`Contains` on pending operands of a type they reject), `CV-026` and `MK-005`
(conversion results moving with marks that do not redact and with how a
one-type constraint is written), `DI-011` (path keys that carry marks), and
`TY-017` (where a map's key diagnostics sit). Each has a fix planned for a
later release, some waiting on a decision about what the rule should say.

### Fixed

- `Equals` no longer settles two operands unequal while both could still be
  null. A narrowing other than `Null` and `NotNull` says nothing about null
  (`[UN-002]`), so two ranges holding no other value in common still hold null
  in common, and two nulls of one type are equal (`[EQ-004]`). 0.1.0 compared
  what such ranges said about their other values and answered a known `false`,
  overstating `[EQ-003]`, `[EQ-042]`, `[EQ-043]` and `[UN-007]`; the answer is
  now unknown until null is ruled out on one side.

  Answers that rested on it move with it. Where two members of a set could
  each be null, the length of the set is now a range rather than the member
  count, membership is unknown rather than known `false`, a listing of the two
  no longer contradicts a `LengthMax` of one, and converting such a set to a
  tuple no longer fails as `convert.length_mismatch`. Ruling null out on
  either side settles all of them again, as before.

  A document 0.1.0 wrote is still read as it was written. The length of a
  listed set is recorded in the encoding, so a 0.1.0 document listing two
  members that could each be null decodes with the `length >= 2` it recorded,
  and re-encodes to the same bytes, although this release would derive
  `length >= 1` for the same listing. The length of a set value is not
  recorded but computed from its members, so such a set decodes unchanged and
  answers `>= 1, <= 2` where 0.1.0 answered `2`.

- `Convert` no longer panics with a nil pointer dereference on data
  (`[ER-002]`). A container member that converts to a pending value
  contributes the type it would have with no keys in hand, as the least type
  it can have. Where that no-keys conversion itself failed, 0.1.0 dropped the
  failure and unified the zero `Type` that came with it. The member now
  contributes nothing instead, and a collection with nothing left to unify is
  pending (`[CV-031]`), since keys it has yet to see can still give it
  attributes that convert. Reaching the panic took a container holding a
  member whose own members convert to an object with a required field
  admitting more than one type, such as a tuple of maps under
  `ListOf(ListOf(ObjectWith(...)))`.

- `Serialize` no longer panics when two marks on one value have payloads that
  fail to encode. A payload that does not encode leaves a placeholder where
  its bytes would be, and every such placeholder is alike, so the `[SE-041]`
  check for two unequal marks sharing one encoding blamed the mark type for a
  failure `[SE-042]` had already recorded as data. Such marks are now left out
  of that check, which is about encodings, and serializing gives
  `serialize.unencodable_capsule` as it does for a single mark. Two unequal
  marks whose payloads do encode alike still panic as a usage error.

- `Hash` is stable within a run, as `[EQ-032]` requires. Composite types are
  interned, and a type nothing references is collected; building it again gives
  a type with a new id. Every hash began with that id, so a value's hash
  changed whenever the collector happened to run: a table keyed by hash forgot
  its entries, and two values `Identical` reports the same could hash
  differently in one run, against `[EQ-030]`. A value now hashes by the
  structure of its type, which does not change, rather than by its id. Hashes
  remain meaningless outside the run that produced them.

- The Unicode version tenon states no longer follows the Go toolchain a
  consumer builds with (`[ST-003]`). `golang.org/x/text` ships Unicode 15.0.0
  behind a `!go1.27` build tag and 17.0.0 behind `go1.27`, and the display form
  read Go's own general categories, so a build with Go 1.27 or later
  normalized and escaped by 17.0.0 while grapheme cluster lengths stayed at
  15.0.0. Twenty sequences compose in 17.0.0 that do not in 15.0.0, so one
  build produced encodings another refused as `serialize.not_canonical`.
  `internal/uni` now holds the 15.0.0 canonical combining classes, canonical
  decompositions, primary composites and general-category ranges, generated by
  `tools/unigen` and committed. Two builds of one version of tenon agree on
  every string whatever toolchain made them.

- Normalization is plain UAX #15 (`[ST-002]`). `golang.org/x/text` also applies
  the Stream-Safe Text Process, which inserts U+034F into a run of more than
  thirty non-starters; a string that gained a code point on the way in is not
  the string that was given, and `[ST-002]` makes two strings equal exactly
  when their normalized forms are identical. A string holding such a run now
  normalizes without the inserted joiner, so it may differ from the value
  0.1.0 built from the same text.

- The canonical order places capsule values a type reports equal together, as
  `[EQ-045]` requires. Where a capsule type declares equality and a hash but no
  order, values whose hashes collide are told apart by a number this run gives
  them. That number was per pointer, so two equal values reached through
  different pointers got two numbers, and an unequal value could sort between
  them. The number now belongs to the equality class the type reports, so a
  value that is one value sorts as one value: a set holding such values
  iterates one way whatever order it was built in (`[EQ-044]`), two identical
  such sets compare equal and their diff is empty (`[DI-031]`), and one value
  has one encoding (`[SE-001]`). A type declaring no equality is unchanged:
  every pointer is its own class.

### Added

- `conformance/values.Colliding`, a capsule type declaring equality and a hash
  that is the same for every value, with values of it in `values.All`. No test
  reached a hash collision before, which is why the ordering defect above
  passed the gate.

- Two encoding vectors in `conformance/vectors`: a pair of code points that
  Unicode 15.0.0 leaves apart and later versions compose, and a run of
  thirty-one non-starters. An implementation normalizing by another version,
  or applying the Stream-Safe Text Process, encodes one of them differently.

## 0.1.0 (2026-09-16)

The first release: a reference implementation of version 0.1.0 of the tenon
specification, satisfying all 192 of its normative rules, as
`CONFORMANCE.md` reports. The module is `github.com/kmoneil/tenon`, licensed
under the Apache License, Version 2.0.

### Types and values

- Types (`Bool`, `Number`, `String`, lists, sets, maps, tuples, objects and
  capsule types) kept apart from constraints (`Any`, `Exactly`, `ListOf`,
  `SetOf`, `MapOf`, `TupleOf`, `ObjectWith`, `OneOf`).
- Values that are known, null, unknown with a range of what they may yet
  turn out to be, pending a type, or error values whose diagnostics carry a
  code and a path.
- Exact decimal numbers in a window of two million digits, with no rounding
  outside division, no infinities and no NaN.
- Strings held in Normalization Form C under Unicode 15.0.0, with lengths in
  grapheme clusters, and ill-formed UTF-8 refused as data.
- Marks: typed metadata that propagates through operations or stays where
  it is put, deep marks, and redacting marks that diagnostics, display and
  diff withhold.

### Operations

- `Equals`, which answers unknown where it cannot know, beside `Identical`,
  a canonical total order, and hashing.
- Conversion under a safe or an unsafe policy, and unification that is
  commutative, associative and total.

### Serialization

- One canonical CBOR encoding for every value, unknown values, ranges and
  marks included, which decoders hold their input to, and a strict JSON
  projection.

### Go interoperability

- Package `gotenon`: `Encode` and `Decode` between Go values and tenon
  values, decoding being conversion under a policy, with struct tags and the
  `ValueMarshaler` and `ValueUnmarshaler` interfaces.

### Diagnostics, display and diff

- A registry of stable diagnostic codes.
- A display form that shows everything `Identical` compares, so two values
  that differ never read alike, other than through redaction.
- `Diff`, a structural diff that is empty exactly when two values are
  identical and never shows a redacted part.

### Conformance

- Conformance tests name the rules they cover, and `make check` fails on an
  uncovered rule or a stale `CONFORMANCE.md`.
- Encoding vectors and diff fixtures for testing other implementations, in
  `conformance/vectors`.
- Fuzz targets for number parsing, string construction and decoding, with
  their corpora; property tests at twenty times their cases in
  `make check-slow`; and `make determinism`, which runs the tests twice and
  compares their canonical output.

### Known issues

- The Unicode version is not held inside the module. Normalization and display
  escaping read tables that follow the toolchain a consumer builds with, so a
  build with Go 1.27 or later applies Unicode 17.0.0 to both, while grapheme
  cluster lengths stay at 15.0.0: one build mixes two versions, against
  `[ST-003]`. Some sequences normalize differently between them, so a value
  built under one version can serialize to bytes a build under the other
  refuses as `serialize.not_canonical`. Build with Go 1.26 for the version
  this release states. Fixed in the next release.
