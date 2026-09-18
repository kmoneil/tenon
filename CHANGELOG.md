# Changelog

## Unreleased

### Fixed

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

### Changed

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
