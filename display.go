package tenon

import (
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// writeQuoted writes s quoted as a display form quotes text (DI-012): in
// quotation marks, with quotation mark and reverse solidus escaped, tab, line
// feed and carriage return by their short escapes, and every other character
// that is a control, format, private-use, unassigned or separator character,
// the space aside, by its code point.
func writeQuoted(b *strings.Builder, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case escapedInDisplay(r):
			b.WriteString(`\u{`)
			hex := strings.ToUpper(strconv.FormatInt(int64(r), 16))
			b.WriteString(strings.Repeat("0", max(4-len(hex), 0)))
			b.WriteString(hex)
			b.WriteByte('}')
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
}

// escapedInDisplay reports whether a display form writes r by its code point:
// whether its general category is a separator (Z) other than the space, or
// other (C), unassigned included. The categories are Go's, whose Unicode
// version a test holds to uni.UnicodeVersion.
func escapedInDisplay(r rune) bool {
	switch {
	case r == ' ':
		return false
	case r < 0x80:
		return r < 0x20 || r == 0x7F
	}
	return !unicode.In(r, unicode.L, unicode.M, unicode.N, unicode.P, unicode.S)
}

// quotedText returns s quoted as a display form quotes text.
func quotedText(s string) string {
	var b strings.Builder
	writeQuoted(&b, s)
	return b.String()
}

// writeIdentifiers writes the identifiers of marks as a display form lists
// them: quoted, each once, in string order, separated by commas.
func writeIdentifiers(b *strings.Builder, marks []Mark) {
	ids := make([]string, len(marks))
	for i, m := range marks {
		ids[i] = m.MarkID()
	}
	slices.Sort(ids)
	for i, id := range slices.Compact(ids) {
		if i > 0 {
			b.WriteString(", ")
		}
		writeQuoted(b, id)
	}
}
