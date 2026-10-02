// Package ctytenon carries values, types, type constraints, paths and errors
// between go-cty, the value system of HCL and Terraform, and tenon, so that a
// program built on go-cty can move to tenon one piece at a time, handing what
// it has to tenon and taking it back.
//
// A [Bridge] does the carrying, both ways: [Bridge.FromCty] and [Bridge.ToCty]
// for values, [Bridge.TypeFromCty] and [Bridge.TypeToCty] for types,
// [Bridge.ConstraintFromCty] and [Bridge.ConstraintToCty] for type
// constraints, [Bridge.PathFromCty] and [Bridge.PathToCty] for paths, and
// [Bridge.ErrorFromCty] for an error cty gave. The zero Bridge is ready to
// use; one that maps marks or pairs capsule types says how in its fields.
//
// # What crosses
//
//   - A bool, string or number is itself, and a list, set, map, tuple or
//     object is made of what it holds, at any depth, nulls and unknown values
//     among them.
//   - A number crosses to tenon as the decimal of the fewest digits that
//     cty's parser reads as it, so that HCL's 0.1 is 0.1, and back as what
//     cty's parser reads from its text: a decimal of up to 153 digits crosses
//     back as itself.
//   - What a range says of an unknown value crosses with it, where the other
//     side can say it: not null, a number's bounds, a string's prefix and a
//     collection's length.
//   - A cty value whose type holds cty.DynamicPseudoType, as cty.DynamicVal
//     and the untyped null do, crosses as a pending value, whose constraint is
//     that type's, and back as an unknown value or a null. A list, set or map
//     of such a type keeps its length both ways: an unknown one's refinements,
//     a known one's count, and a pending one's lengths.
//   - A type constraint crosses as a constraint: cty.DynamicPseudoType is
//     [tenon.Any], a list, set, map or tuple type is ListOf, SetOf, MapOf or
//     TupleOf the constraints of its parts, and an object type an open
//     [tenon.ObjectWith], as below.
//   - A mark crosses as the Bridge maps it, on the value that carries it, and,
//     since cty hands a container's marks to every value read out of it, on
//     every value within.
//   - A capsule value crosses as a value of the capsule type the Bridge pairs
//     with its own, holding the same pointer.
//   - A path crosses given the value it lies within: cty steps into a set by
//     the member, and tenon by the member's place in the set's order, which
//     only the set says.
//   - An error cty gave crosses as diagnostics located by its paths, and a
//     tenon error value as the [*tenon.Error] holding it.
//
// # What does not cross
//
// A value fails to cross with a [*tenon.Error] whose diagnostics are located
// by the paths of the parts that fail, with tenon's codes where tenon has one,
// and this package's where it does not:
//
//   - an infinity (tenon.CodeEncodeNotANumber), and a number outside tenon's
//     range (tenon.CodeNumberOutOfRange);
//   - text that is not UTF-8 (tenon.CodeStringInvalidUTF8), and attribute
//     names tenon refuses;
//   - a mark the Bridge does not map ([CodeUnmappedMark]), which fails the
//     crossing rather than being left behind, since a sensitive value without
//     its mark would show in tenon's display;
//   - a capsule type the Bridge pairs with none ([CodeUnpairedCapsule]);
//   - a pending value whose constraint holds a OneOf ([CodeOneOf]), which no
//     cty type says.
//
// A failure within a value carrying a redacting mark is located at that value,
// and names it by its placeholder, saying nothing of what it holds.
//
// # Where the two differ
//
// Some crossings give a value, type or constraint that allows more than its
// source, never less: every value the source allows crosses as one the result
// allows.
//
//   - cty's conversion to an object type drops the attributes the type does
//     not name, and tenon's never drops one. An object type crosses to tenon as
//     an open ObjectWith, which accepts every object cty's conversion does and
//     keeps what cty would drop, and a closed ObjectWith crosses to cty as an
//     object type. A value's type is not a conversion's target, and a value
//     has exactly the attributes its type names, so there an object type is a
//     closed ObjectWith.
//   - A known cty value whose type holds cty.DynamicPseudoType, as a list
//     holding cty.DynamicVal does, crosses as a pending value known not to be
//     null, and what it holds is left behind, its length aside: tenon's
//     containers hold only values whose types are settled.
//   - What one side's range says that the other's cannot is left behind: a
//     string's length and a set's listed members going to cty. A number bound
//     that excludes itself includes itself in cty, since a number just past
//     it can cross as the bound.
//   - A tenon mark on a container that tenon does not hand to the values
//     within, one that is not deep, comes back from cty on every one of them.
//
// The package is a module of its own, so that tenon never requires go-cty:
// a program that imports tenon alone does not download it.
package ctytenon
