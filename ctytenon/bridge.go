package ctytenon

import "github.com/kmoneil/tenon"

// Bridge carries values, types and type constraints between go-cty and tenon.
// The zero Bridge is ready to use; it maps no marks and pairs no capsule
// types.
type Bridge struct {
	// MarkFromCty gives the tenon mark a cty mark crosses to tenon as, and
	// false for a cty mark that does not cross, which fails the crossing of a
	// value carrying it with CodeUnmappedMark rather than leaving the mark
	// behind. A nil MarkFromCty maps no mark.
	//
	// cty hands a container's marks to every value read out of it, so the
	// marks a list, map, tuple or object carries cross on it and on every
	// value within it. The members of a set carry no marks in tenon, as in
	// cty, and a set's marks stay on the set; a deep mark (tenon.DeepMark)
	// is one tenon hands to the members as they are read.
	MarkFromCty func(mark any) (tenon.Mark, bool)

	// MarkToCty gives the cty mark a tenon mark crosses to cty as, and false
	// for a tenon mark that does not cross, which fails the crossing of a
	// value carrying it with CodeUnmappedMark. A nil MarkToCty maps no mark.
	// The cty mark must be comparable, as cty requires of every mark.
	//
	// cty hands a container's marks to every value read out of it, so a value
	// within a container carries in cty only the marks its containers do not:
	// a mark on a list that tenon does not hand to its elements, one that is
	// not deep, is on them once the list crosses back.
	MarkToCty func(mark tenon.Mark) (any, bool)
}
