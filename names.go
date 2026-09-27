package tenon

import (
	"slices"
	"strconv"
	"strings"

	"github.com/kmoneil/tenon/internal/uni"
)

// CheckAttributeNames reports whether names can be the attribute names of one
// object: it returns nil, or an [*Error] whose diagnostics say what is wrong
// with them, as Object reports it (TY-018). A name that is empty fails with
// CodeObjectEmptyName, one that is not well-formed UTF-8 with
// CodeStringInvalidUTF8, and names that are the same name once normalized to
// Unicode Normalization Form C, identical names among them, with
// CodeObjectDuplicateName.
//
// Object gives an error value for names that cannot be attribute names, since
// they come from data as often as from the program. ObjectType, ObjectWith and
// Path.Attribute are given names the program writes, and panic on such a name;
// a program that builds them from names it did not write checks the names here
// first.
func CheckAttributeNames(names ...string) error {
	entries := make([]namedEntry[struct{}], len(names))
	for i, name := range names {
		entries[i] = namedEntry[struct{}]{original: name}
	}
	var diags []Diagnostic
	sorted, shared := checkNames(entries)
	for _, e := range sorted {
		if e.fault != "" {
			diags = append(diags, nameFault(e.fault, e.original))
		}
	}
	for _, group := range shared {
		diags = append(diags, sharedName(group))
	}
	if len(diags) == 0 {
		return nil
	}
	return asError(errorValue(diags...))
}

// namedEntry is a value given under a name that may not be an attribute name.
type namedEntry[V any] struct {
	key      string // the normalized name, or the name as given where it is not well-formed UTF-8
	original string // the name as given
	value    V
	fault    Code // what makes the name no attribute name, or "" where it is one
}

// checkNames normalizes the names of entries, notes what makes a name no
// attribute name, and returns the entries in the order TY-017 gives a map's
// entries: bytewise by key, and by the name as given between entries whose
// keys are one. It returns as well each run of two or more entries that share
// a key, which are one name given more than once.
func checkNames[V any](entries []namedEntry[V]) (sorted []namedEntry[V], shared [][]namedEntry[V]) {
	sorted = slices.Clone(entries)
	for i := range sorted {
		e := &sorted[i]
		normalized, err := uni.Canonical(e.original)
		switch {
		case err != nil:
			e.key, e.fault = e.original, CodeStringInvalidUTF8
		case normalized == "":
			e.key, e.fault = normalized, CodeObjectEmptyName
		default:
			e.key = normalized
		}
	}
	slices.SortFunc(sorted, func(a, b namedEntry[V]) int {
		if c := strings.Compare(a.key, b.key); c != 0 {
			return c
		}
		return strings.Compare(a.original, b.original)
	})
	for i := 0; i < len(sorted); {
		j := i + 1
		for j < len(sorted) && sorted[j].key == sorted[i].key {
			j++
		}
		// An empty name given twice, or one that is not well-formed UTF-8,
		// is reported for itself each time rather than as a name shared.
		if j-i > 1 && sorted[i].fault == "" {
			shared = append(shared, sorted[i:j])
		}
		i = j
	}
	return sorted, shared
}

// nameFault returns the diagnostic of a name that fault makes no attribute
// name.
func nameFault(fault Code, name string) Diagnostic {
	if fault == CodeObjectEmptyName {
		return Diagnostic{Code: fault, Message: "an attribute name must not be empty"}
	}
	return Diagnostic{Code: fault, Message: "attribute name " + quotedASCII(name) + " is not well-formed UTF-8 at byte " + strconv.Itoa(invalidUTF8At(name))}
}

// sharedName returns the diagnostic of entries that share one name.
func sharedName[V any](group []namedEntry[V]) Diagnostic {
	spellings := make([]string, len(group))
	for i, e := range group {
		spellings[i] = quotedASCII(e.original)
	}
	return Diagnostic{
		Code:    CodeObjectDuplicateName,
		Message: "attribute names " + strings.Join(spellings, " and ") + " are the same name after normalization",
	}
}
