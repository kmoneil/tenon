// Package uni is tenon's Unicode layer. Every Unicode algorithm that tenon
// applies to text uses the data of one version of the Unicode Standard,
// UnicodeVersion.
package uni

import "golang.org/x/text/unicode/norm"

// UnicodeVersion is the version of the Unicode Standard whose data tenon's
// Unicode operations use. Normalization depends on it, so changing it can
// change which strings are equal: it is a breaking change.
const UnicodeVersion = "15.0.0"

// NFC returns s in Unicode Normalization Form C. The caller must ensure that s
// is well-formed UTF-8.
func NFC(s string) string {
	return norm.NFC.String(s)
}
