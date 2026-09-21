// Package tenon is a dynamic type and value system for applications that must
// represent user-supplied data whose types are not known at compile time, such
// as the value layer of a configuration language.
//
// A program that holds data it did not declare has three problems that Go's
// own types do not answer: part of the data is not known yet, part of it must
// not be shown, and part of it is wrong. A tenon value answers each of them in
// the value itself.
//
//   - An operation over a value that is not known yet gives a value that is
//     not known, carrying what it can still turn out to be, rather than a zero
//     value or a guess.
//   - A value carrying a redacting mark keeps its contents out of messages,
//     display forms and projections, and the mark travels into whatever is
//     derived from it.
//   - Data that is wrong becomes an error value carrying a diagnostic for each
//     problem, located by its path, which operations carry forward instead of
//     panicking.
//
// However a value was built, two values that say the same thing are one value:
// [Identical] reports them identical, [Hash] gives them one hash, and
// [Serialize] gives them one encoding.
//
// This package implements tenon specification version 0.1.0. Every rule of it
// has a conformance test, which CONFORMANCE.md reports.
//
// # Values and their states
//
// A [Value] is in exactly one of three states.
//
//   - A resolved value has a [Type]. It is known, built by [Bool], [String],
//     [NumberFromInt], [NumberFromText], [ListVal], [SetVal], [MapVal],
//     [TupleVal], [ObjectVal] or [CapsuleVal]; or it is null, the absence of a
//     value at a type, built by [NullVal]; or it is unknown, a value of a type
//     whose content is not settled yet, built by [Unknown].
//   - A pending value, built by [Pending], has no type yet, only a
//     [Constraint] on what its type will be. [Resolve] settles it.
//   - An error value, built by [ErrorVal], has no type and carries
//     diagnostics.
//
// [Value.IsResolved], [Value.IsKnown], [Value.IsPending] and [Value.IsError]
// ask which state a value is in; [Value.Type] and [Value.Constraint] say what
// the first two hold. [Value.HasContent] reports whether there is content to
// read, which a null, an unknown, a pending value and an error value have not.
// Content is read with [Value.Len], [Value.Index], [Value.Elements],
// [Value.Attribute], [Value.MapKeys], [Value.MapElement] and the accessors
// [Value.AsBool], [Value.AsString], [Value.AsInt64], [Value.AsBigInt] and
// [Value.AsBigRat]. [Value.String] is the display form, meant for people.
//
// # Types and constraints
//
// A [Type] is what a resolved value is: [BoolType], [NumberType],
// [StringType], [List], [Set], [Map], [Tuple], [Object], and [Capsule] for a
// Go type carried through unchanged. Types are interned, so two types are the
// same type exactly when they are ==, and a type can be a map key.
//
// A [Constraint] is what a type must satisfy: [Any], [Exactly], [ListOf],
// [SetOf], [MapOf], [TupleOf], [ObjectWith] with [Required] and [Optional]
// fields, and [OneOf] for a choice between them. [Satisfies] asks whether a
// type meets a constraint, and [Unify] gives the one constraint that several
// of them come to. A pending value carries a constraint in place of a type,
// which is how a program holds a value whose shape depends on data it has not
// read yet.
//
// # Unknown values and ranges
//
// An unknown value is a promise about a value that is not in hand. What is
// known about it is its [Range]: whether it may be null, bounds on a number, a
// prefix of a string, bounds on a length, the members a collection holds.
// [Narrow] records more, through [NumberMin], [NumberMax], [StringPrefix],
// [LengthMin], [LengthMax], [Members], [Null] and [NotNull].
//
// Narrowing is monotone: the result says everything the value said and
// everything the narrowing says. A narrowing that brings a range down to one
// value gives that value, known, since an unknown that nothing more could ever
// say is not unknown. A narrowing that leaves nothing gives an error value,
// since no value has an empty range.
//
// Operations read ranges and write them: [Add], [Sub], [Mul], [Div] and
// [Mod] of unknown numbers give an unknown number bounded by what the
// operands' bounds allow, and [Equals] and [LessThan] answer where the ranges
// settle the question, however little else is known: no number of 1024 or
// more comes before 80.
//
// # Operations, errors and paths
//
// The operations are [Add], [Sub], [Mul], [Div], [Mod] over numbers, which are
// exact and never rounded; [And], [Or], [Not] over booleans; [Equals],
// [LessThan], [IsNull] and [Contains] for comparison and membership; and
// [Length], which counts a string in grapheme clusters and a collection in
// members.
//
// An operation given an error value gives an error value, so a failure is
// carried to where it is handled rather than checked at every step. A
// [Diagnostic] holds a [Code] for programs, a message for people and a [Path]
// locating the problem within the value that carries it, built of a [Step] per
// attribute or index. [Value.Diagnostics] reads them back. The codes are
// constants, from [CodeBoolInvalidSyntax] to [CodeSerializeUnknownMark], so a
// caller switches on them rather than matching text.
//
// Passing a value of the wrong type to an operation is not a diagnostic but a
// panic: it is a mistake in the program, not in the data.
//
// # Marks and secrets
//
// A [Mark] is a label attached to a value that travels with it. [WithMarks]
// attaches marks, [HasMark] asks, and [Unmark] and [UnmarkDeep] take them off
// deliberately. A mark says how far it travels: [Propagate], the default,
// reaches whatever is derived from the value, and [Isolate] stays where it was
// put. A mark whose Redacting method reports true withholds the value's
// contents from display forms, diagnostic messages and [ProjectJSON]. A mark
// that also implements [DeepMark] marks everything within the value it is
// attached to, and one that implements [EncodableMark] survives serialization.
//
// # Conversion
//
// [Convert] converts a value to a constraint under a [Policy]. [Safe] applies
// only the conversions that change how a value is held without changing what
// kind of thing it is, as a tuple of strings becomes a list of strings.
// [Unsafe] applies as well those that change what a value is, as a number
// becomes text, and those that can fail or lose something, as a list becomes a
// set and loses its order. Whether text may stand where a number is expected
// is a question about a language rather than about values, so tenon never
// converts on its own: the caller chooses the policy, conversion by
// conversion.
//
// A conversion that cannot be made gives an error value carrying a diagnostic
// for each part that could not be converted.
//
// # Equality, order and hashing
//
// [Equals] is the operation over values, and answers an unknown boolean where
// what is known does not settle the question. [Identical] is the Go-level
// question of whether two values say exactly the same thing, marks, ranges and
// diagnostics included; it is total, and never unknown. [Hash] agrees with it,
// and [CanonicalCompare] puts values in a total order that does not depend on
// how they were built.
//
// # Serialization, JSON and diffs
//
// [Serialize] encodes a value as a CBOR document, unknown values, ranges,
// marks and diagnostics included, and [Deserialize] reads one back, given
// [Decoders] for the capsule types and marks it may hold. One value has one
// encoding: the bytes can be compared, hashed or used as a key in place of the
// value.
//
// [ProjectJSON] renders a value as JSON for a consumer that speaks JSON and
// nothing else. The projection is one-way and lossy, and it refuses what JSON
// cannot say: a value that is not known, or one a redacting mark withholds.
//
// [Diff] reports what changed between two values as [Changes], each a [Change]
// locating what happened by its path, which is what a plan engine shows.
//
// # Go values
//
// Package [github.com/kmoneil/tenon/gotenon] maps Go values to tenon values
// and back: Encode makes a value from a Go value, and Decode fills a Go value
// from one, converting under a policy the caller chooses. Struct fields are
// named by their tenon tag.
//
// Data whose types a program does not know is what this package is for, and in
// Go that data arrives as any: the map[string]any that encoding/json gives
// encodes by what each value holds, so a document becomes a value without a Go
// type written for it.
//
// A Go type that tenon should carry through unchanged, rather than map, is a
// [Capsule] type: it keeps its identity, and declares how it is compared,
// hashed, displayed and encoded.
//
// # Immutability and concurrent use
//
// Values, types, constraints, paths and diagnostics are immutable. Every
// operation returns a new value, and nothing a caller holds is written to
// again, so they may be read from any number of goroutines at once without
// synchronization.
//
// Operations a caller supplies, which are the operations of a capsule type,
// the methods of a mark and the decoders given to [Deserialize], may be called
// from any goroutine that uses the value carrying them, and must not depend on
// being called from one.
//
// # Unicode
//
// tenon holds strings in Normalization Form C and measures their length in
// grapheme clusters, both under Unicode 15.0.0, and its display form escapes
// text by the same version. Which Unicode version is in use decides which
// strings are equal and how long they are, so changing it is a breaking
// change, and the version is held inside this module: it does not follow the
// Go toolchain a consumer builds with, and two builds of one version of tenon
// agree on every string whatever toolchain made them.
//
// Normalization is plain UAX #15. In particular tenon does not apply the
// Stream-Safe Text Process, which inserts U+034F into a run of more than
// thirty non-starters: a string that gained a code point on the way in would
// not be the string that was given.
package tenon
