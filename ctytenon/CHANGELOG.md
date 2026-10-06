# Changelog

This is the changelog of ctytenon, the bridge between go-cty and tenon, a
module of its own (`github.com/kmoneil/tenon/ctytenon`) whose versions are
tagged `ctytenon/vX.Y.Z`. tenon's own changes are in the repository's
`CHANGELOG.md`.

## 0.4.0 (2026-10-06)

Requires tenon 0.19.0, for its marks with their paths. A host that
stores or redacts marked values with go-cty can move that work to tenon:
the bridge crosses go-cty's marks with their paths, and its path sets,
to tenon's and back, each held to what crossing the marked value itself
gives, and a walkthrough takes sensitive values through Terraform's
state with both, byte for byte.

The minor version moves for the additions.

### Added

- `LocatedMarksFromCty` and `LocatedMarksToCty` cross marks with their
  paths, go-cty's `PathValueMarks` and tenon's `LocatedMarks`, given the
  value they lie within. go-cty hands a container's marks to every value
  read out of it, so an entry from go-cty is located at every value within
  its path, as `FromCty` puts the marks, and an entry to go-cty leaves off
  what a container's entry carries, as `ToCty` does. Putting either side's
  back on the other side's value gives what crossing the marked value
  gives, and the entries go-cty's `MarkWithPaths` drops silently, tenon's
  `WithLocatedMarks` hands back.
- `PathSetFromCty` and `PathSetToCty` cross a `cty.PathSet` as tenon paths
  sorted in their canonical order, each once, and back.
- A walkthrough, as a test, takes a resource's sensitive values through
  Terraform's state with go-cty and with tenon side by side, holding both
  to the same values, marks and bytes. The steps are: strip the marks with
  their places, make unknowns null, write the JSON and
  `sensitive_attributes`, read them back, show the next plan's changes
  redacted, and carry it all in tenon's own encoding. go-cty's
  `UnknownAsNull` is a recipe over tenon's `Transform` there, held to
  go-cty's answers; the repository's `MIGRATING.md` gives it.

## 0.3.0 (2026-10-05)

Requires tenon 0.16.0, whose standard library a host still evaluating
with cty calls in place through `FunctionToCty`, as the repository's HCL
proof does for HCL's operators. A tenon function declaring its result
never null crosses with cty's not-null refinement, and a cty function
crossed by `FunctionFromCty` answers arguments not known yet as cty does,
refined as it says.

The minor version moves for the change to what a function crossed by
`FunctionFromCty` admits.

### Changed

- A tenon function declaring its result never null crosses to cty with
  that refinement (`RefineResult`'s not null), as its unknown answers
  already carried it.
- A cty function crossed by `FunctionFromCty` admits unknown arguments on
  every parameter, so cty's own `Call` answers them as cty does: an
  argument the function does not allow unknown gives cty's unknown result,
  refined as the function's `RefineResult` says (not null, a length, a
  prefix), which crosses back as tenon's narrowing, and a list holding an
  unknown element, which cty counts as known, reaches the function as it
  does under cty. In 0.2, tenon's boundary answered such an argument with
  a bare unknown, saying less than cty does.

## 0.2.1 (2026-10-05)

Requires tenon 0.15.1, which fixes two defects that functions crossed
from cty reached through tenon 0.15.0's call: a dynamic null returned
from known arguments panicked, and a failure quoting a redacted argument
showed it. ctytenon 0.2.0 built against tenon 0.15.1 is fixed as well;
this release makes sure of it. A security advisory follows tenon's
release.

### Fixed

- `FunctionFromCty` panicked where the cty function returned a dynamic
  null from known arguments, as `jsondecode` does for `null`, `[null]` or
  `{"a":null}`: tenon 0.15.0's call took the pending null it crosses as
  for an unknown value. With tenon 0.15.1 the call answers with it, as
  `ParseJSON` reads a JSON null.
- A cty function crossed by `FunctionFromCty` whose failure quotes its
  argument, as `parseint` does, showed a value that a redacting mark
  withheld, since the cty function saw it unmarked. With tenon 0.15.1 the
  failure keeps its code and its message is withheld.

## 0.2.0 (2026-10-03)

Functions cross the bridge now, both ways, so a host still evaluating
with cty calls a tenon function in place, and one mid-migration calls
what it has through tenon. Requires tenon 0.15.0.

### Added

- Functions cross the bridge both ways. `Bridge.FunctionToCty(f, policy)`
  wraps a tenon function for a host still evaluating with cty, HCL's
  evaluator among them: the cty parameters carry the crossed constraints
  and grant cty's every allowance, so tenon's call boundary answers every
  argument state, a `cty.DynamicVal` crossing as a pending value; failures
  return as the `*tenon.Error` cty callers expect, and a defect of the
  function's author reaches a cty caller as cty's own `PanicError`
  convention. `Bridge.FunctionFromCty(f)` carries a cty function the other
  way, each allowance becoming the admission it means and the result
  deriving through the function's own `ReturnTypeForValues`; a function
  cty's `Unpredictable` wraps is crossed beneath the wrapper and declared
  with `tenon.Function.AsVolatile`.

## 0.1.0 (2026-10-02)

The first release. A program built on go-cty, the value system of HCL and
Terraform, can move to tenon one piece at a time: a `Bridge` carries values,
types, type constraints, paths and errors between the two, both ways, and
what is not known yet, and what is sensitive, crosses with them. It requires
tenon 0.13.0 and go-cty 1.19.0.

- Values cross as themselves, at any depth: a number as the decimal of the
  fewest digits that cty's parser reads as it, so that HCL's `0.1` is `0.1`,
  and back as what cty's parser reads from its text; a null as the null of
  its type; an unknown value as the unknown value of its type, its
  refinements as its range (not null, a number's bounds, a string's prefix,
  a collection's length), where the other side can say them.
- A value whose type holds `cty.DynamicPseudoType` is a pending value whose
  constraint is that type's. A known list, set or map of such a type keeps
  its length both ways, and a known tuple or object of one holds its members,
  as cty's tuple of `cty.DynamicVal` and a number holds the number, and
  crosses back `RawEquals`.
- Types cross as types, and a type holding `cty.DynamicPseudoType` as a
  constraint: `cty.DynamicPseudoType` is `tenon.Any()`, and an object type an
  open `ObjectWith` where it is a conversion's target.
- Marks cross as the `Bridge` maps them, `MarkFromCty` and `MarkToCty`, on
  the value that carries them and, as cty hands a container's marks to what
  it holds, on every value within. A mark it does not map fails the
  crossing (`cty.unmapped_mark`) rather than being left behind.
- Capsule values cross as values of the capsule type the `Bridge` pairs with
  their own (`PairCapsules`), holding the same pointer; an unpaired one
  fails (`cty.unpaired_capsule`).
- Paths cross given the value they lie within, and an error cty gave as
  diagnostics located by its paths (`ErrorFromCty`).
- What does not cross fails with a `*tenon.Error` whose diagnostics are
  located by the paths of the parts that fail, among them a pending value
  whose constraint holds a `OneOf` (`cty.one_of`), which no cty type says.
  A failure within a value
  carrying a redacting mark is located at that value, and names it by its
  placeholder alone.

The package documentation says what crosses, what does not, and where the
two differ: where one side can say what the other cannot, the crossing
allows more than its source, never less.
