// Package ctytenon carries types between go-cty, the value system of HCL and
// Terraform, and tenon, so that a program built on go-cty can move to tenon
// one piece at a time, handing what it has to tenon and taking it back.
//
// A [Bridge] does the carrying. [Bridge.TypeFromCty] gives the tenon type of a
// cty type, and [Bridge.TypeToCty] the cty type of a tenon type: Bool, Number
// and String are themselves, and lists, sets, maps, tuples and objects are
// made of the types they hold. A cty type that holds cty.DynamicPseudoType,
// or an object type with optional attributes, is a type constraint rather
// than a type, and does not cross as one; nor does a capsule type, which only
// the program that made it can pair with another.
//
// The package is a module of its own, so that tenon never requires go-cty:
// a program that imports tenon alone does not download it.
package ctytenon
