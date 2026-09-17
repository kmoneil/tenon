// Package uni is tenon's Unicode layer. Every Unicode algorithm that tenon
// applies to text uses the data of one version of the Unicode Standard,
// UnicodeVersion.
package uni

import (
	"strconv"
	"unicode/utf8"
)

// UnicodeVersion is the version of the Unicode Standard whose data tenon's
// Unicode operations use: normalization and grapheme cluster segmentation.
// Changing it can change which strings are equal and how long they are, so it
// is a breaking change.
const UnicodeVersion = "15.0.0"

// Error is an error reported by this package. Its values are constants, which
// callers compare errors against.
type Error uint8

// ErrInvalidUTF8 reports text that is not well-formed UTF-8.
const ErrInvalidUTF8 Error = 1

func (e Error) Error() string {
	if e == ErrInvalidUTF8 {
		return "uni: invalid UTF-8"
	}
	return "uni: error " + strconv.Itoa(int(e))
}

// Canonical returns the canonical form of s as the content of a string value:
// s normalized to Unicode Normalization Form C. Two strings are the same string
// exactly when their canonical forms are identical. Canonical returns
// ErrInvalidUTF8 and no text if s is not well-formed UTF-8: ill-formed bytes
// are never replaced and never passed through.
func Canonical(s string) (string, error) {
	if !utf8.ValidString(s) {
		return "", ErrInvalidUTF8
	}
	return nfc(s), nil
}

// NFC returns s in Unicode Normalization Form C. The caller must ensure that s
// is well-formed UTF-8.
func NFC(s string) string {
	return nfc(s)
}
