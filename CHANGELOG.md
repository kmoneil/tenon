# Changelog

## Unreleased

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
