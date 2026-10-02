# Changelog

This is the changelog of ctytenon, the bridge between go-cty and tenon, a
module of its own (`github.com/kmoneil/tenon/ctytenon`) whose versions are
tagged `ctytenon/vX.Y.Z`. tenon's own changes are in the repository's
`CHANGELOG.md`.

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
