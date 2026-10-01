// Package ctytenon carries values, types and type constraints between go-cty,
// the value system of HCL and Terraform, and tenon, so that a program built on
// go-cty can move to tenon one piece at a time, handing what it has to tenon
// and taking it back.
//
// A [Bridge] does the carrying. [Bridge.FromCty] gives the tenon value of a cty
// value, and [Bridge.ToCty] the cty value of a tenon value: a bool, string or
// number is itself, and a list, set, map, tuple or object is made of the
// values it holds, at any depth, nulls and unknown values among them. A number
// crosses to tenon as the decimal of the fewest digits that cty's parser reads
// as it, so that HCL's 0.1 is 0.1, and back as what cty's parser reads from
// its text. What a range says of an unknown value, in cty's refinements or
// tenon's narrowings, is left behind, which leaves the value allowing more,
// never less. A cty value whose type holds cty.DynamicPseudoType, as
// cty.DynamicVal and the untyped null do, crosses as a pending value, which
// crosses back as an unknown value or a null.
//
// [Bridge.TypeFromCty] gives the tenon type of a cty type, and
// [Bridge.TypeToCty] the cty type of a tenon type. A cty type that holds
// cty.DynamicPseudoType, or an object type with optional attributes, is a type
// constraint rather than a type, as the target of a conversion or the type of
// a Terraform variable is. [Bridge.ConstraintFromCty] gives the tenon
// constraint of a cty type constraint, and [Bridge.ConstraintToCty] the cty
// type constraint of a tenon constraint: cty.DynamicPseudoType is [tenon.Any],
// and a list, set, map or tuple type is ListOf, SetOf, MapOf or TupleOf the
// constraints of its parts. A OneOf does not cross, since no cty type
// constraint says one.
//
// The two differ over objects. cty's conversion to an object type accepts an
// object with attributes the type does not name, and drops them; tenon's
// conversion never drops an attribute. An object type crosses to tenon as an
// open [tenon.ObjectWith], which accepts every object cty's conversion does
// and keeps the attributes cty would drop, and a closed ObjectWith crosses to
// cty as an object type, which accepts more objects than it does. Whichever
// way a constraint crosses, every type it accepted is accepted still. The
// type of a value is not a conversion's target, and a value has exactly the
// attributes its type names, so there an object type is a closed ObjectWith.
//
// The zero Bridge maps no marks and pairs no capsule types, which only the
// program that made them can pair with another: a marked value, or a value of
// a capsule type, does not cross. A value that does not cross fails with a
// [*tenon.Error] whose diagnostics are located by the paths of the parts that
// fail, with the codes of tenon's where tenon has one, and with this
// package's, [CodeUnmappedMark], [CodeUnpairedCapsule] and [CodeOneOf],
// where it does not.
//
// The package is a module of its own, so that tenon never requires go-cty:
// a program that imports tenon alone does not download it.
package ctytenon
