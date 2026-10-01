// Package ctytenon carries types and type constraints between go-cty, the
// value system of HCL and Terraform, and tenon, so that a program built on
// go-cty can move to tenon one piece at a time, handing what it has to tenon
// and taking it back.
//
// A [Bridge] does the carrying. [Bridge.TypeFromCty] gives the tenon type of a
// cty type, and [Bridge.TypeToCty] the cty type of a tenon type: Bool, Number
// and String are themselves, and lists, sets, maps, tuples and objects are
// made of the types they hold. A capsule type does not cross, since only the
// program that made it can pair it with another.
//
// A cty type that holds cty.DynamicPseudoType, or an object type with
// optional attributes, is a type constraint rather than a type, as the target
// of a conversion or the type of a Terraform variable is.
// [Bridge.ConstraintFromCty] gives the tenon constraint of a cty type
// constraint, and [Bridge.ConstraintToCty] the cty type constraint of a tenon
// constraint: cty.DynamicPseudoType is [tenon.Any], and a list, set, map or
// tuple type is ListOf, SetOf, MapOf or TupleOf the constraints of its parts.
// A OneOf does not cross, since no cty type constraint says one.
//
// The two differ over objects. cty's conversion to an object type accepts an
// object with attributes the type does not name, and drops them; tenon's
// conversion never drops an attribute. An object type crosses to tenon as an
// open [tenon.ObjectWith], which accepts every object cty's conversion does
// and keeps the attributes cty would drop, and a closed ObjectWith crosses to
// cty as an object type, which accepts more objects than it does. Whichever
// way a constraint crosses, every type it accepted is accepted still.
//
// The package is a module of its own, so that tenon never requires go-cty:
// a program that imports tenon alone does not download it.
package ctytenon
