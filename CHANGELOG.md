# Changelog

## Unreleased

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
