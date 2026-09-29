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
	sorted, shared := checkNames(entries, attributeNames)
	for _, e := range sorted {
		if e.fault != "" {
			diags = append(diags, nameFault(attributeNames, e.fault, e.original))
		}
	}
	for _, group := range shared {
		diags = append(diags, sharedName(attributeNames, group))
	}
	if len(diags) == 0 {
		return nil
	}
	return asError(errorValue(diags...))
}

// A nameKind is what a name names: an attribute, whose name may not be empty,
// or a map's entry, whose key may be (TY-017, TY-018).
type nameKind uint8

const (
	attributeNames nameKind = iota
	mapKeys
)

// namedEntry is a value given under a name that may not be an attribute name.
type namedEntry[V any] struct {
	key      string // the normalized name, or the name as given where it is not well-formed UTF-8
	original string // the name as given
	value    V
	fault    Code // what makes the name no attribute name, or "" where it is one
}

// checkNames normalizes the names of entries, notes what makes a name no
// name of kind, and puts the entries in the order TY-017 gives a map's
// entries, in place: bytewise by key, and by the name as given between
// entries whose keys are one. It returns them, and each run of two or more
// entries that share a key, which are one name given more than once. It is
// the one statement of the rules for names, which Map, Object and
// CheckAttributeNames follow, and gotenon through them.
func checkNames[V any](entries []namedEntry[V], kind nameKind) (sorted []namedEntry[V], shared [][]namedEntry[V]) {
	sorted = entries
	for i := range sorted {
		e := &sorted[i]
		normalized, err := uni.Canonical(e.original)
		switch {
		case err != nil:
			e.key, e.fault = e.original, CodeStringInvalidUTF8
		case normalized == "" && kind == attributeNames:
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

// nameFault returns the diagnostic of a name that fault makes no name of kind.
func nameFault(kind nameKind, fault Code, name string) Diagnostic {
	if fault == CodeObjectEmptyName {
		return Diagnostic{Code: fault, Message: "an attribute name must not be empty"}
	}
	what := "attribute name "
	if kind == mapKeys {
		what = "map key "
	}
	return Diagnostic{Code: fault, Message: what + quotedASCII(name) + " is not well-formed UTF-8 at byte " + strconv.Itoa(invalidUTF8At(name))}
}

// sharedName returns the diagnostic of entries that share one name of kind.
func sharedName[V any](kind nameKind, group []namedEntry[V]) Diagnostic {
	spellings := make([]string, len(group))
	for i, e := range group {
		spellings[i] = quotedASCII(e.original)
	}
	if kind == mapKeys {
		return Diagnostic{
			Code:    CodeMapDuplicateKey,
			Message: "map keys " + strings.Join(spellings, " and ") + " are the same key after normalization",
		}
	}
	return Diagnostic{
		Code:    CodeObjectDuplicateName,
		Message: "attribute names " + strings.Join(spellings, " and ") + " are the same name after normalization",
	}
}
