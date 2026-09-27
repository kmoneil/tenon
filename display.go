package tenon

import (
	"slices"
	"strconv"
	"strings"

	"github.com/kmoneil/tenon/internal/uni"
)

// writeQuoted writes s quoted as a display form quotes text (DI-012): in
// quotation marks, with quotation mark and reverse solidus escaped, tab, line
// feed and carriage return by their short escapes, and every other character
// that is a control, format, private-use, unassigned or separator character,
// the space aside, by its code point.
func writeQuoted(b *textWriter, s string) {
	b.WriteByte('"')
	for _, r := range s {
		if b.full() {
			return
		}
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
// other (C), unassigned included. The categories are those of
// uni.UnicodeVersion, which internal/uni holds rather than reading from the
// toolchain.
func escapedInDisplay(r rune) bool {
	switch {
	case r == ' ':
		return false
	case r < 0x80:
		return r < 0x20 || r == 0x7F
	}
	return uni.IsSeparatorOrOther(r)
}

// quotedText returns s quoted as a display form quotes text.
func quotedText(s string) string {
	var b textWriter
	writeQuoted(&b, s)
	return b.String()
}

// writeIdentifiers writes the identifiers of marks as a display form lists
// them: quoted, each once, in string order, separated by commas.
func writeIdentifiers(b *textWriter, marks []Mark) {
	ids := make([]string, len(marks))
	for i, m := range marks {
		ids[i] = m.MarkID()
	}
	slices.Sort(ids)
	for i, id := range slices.Compact(ids) {
		if b.full() {
			return
		}
		if i > 0 {
			b.WriteString(", ")
		}
		writeQuoted(b, id)
	}
}

// textWriter builds a display form, or the start of one. A message shows a
// value, a type or a listing cut to its first bytes (shortened), and a
// display form can be far larger than what it shows: a list of k nulls
// spells their type out k times, and each of k members under k deep marks
// names k marks. So a writer given a limit takes nothing once it holds more
// than that, and the writers that walk the parts of a value, a type, a
// constraint or a range stop at it (full): a message costs its limit, not
// the display form of what it quotes.
type textWriter struct {
	strings.Builder
	// limit is how many bytes the text will be cut to; a writer holding more
	// takes no more. Zero is no limit.
	limit int
	// plain says to write a value, and the values it holds, with no mark but
	// a redacting one, as a message shows a value (MK-005): the placeholder
	// of a value that is not an error and carries a redacting mark, and
	// every other value without its marks.
	plain bool
}

// keepPlain sets whether w writes values plain, and returns what restores the
// setting it had.
func (w *textWriter) keepPlain(plain bool) func() {
	was := w.plain
	w.plain = plain
	return func() { w.plain = was }
}

// shortLimit is how many bytes shortened keeps of a long text.
const shortLimit = 32

// full reports whether w holds more than its limit, past which it takes no
// more.
func (w *textWriter) full() bool { return w.limit > 0 && w.Len() > w.limit }

// WriteString writes s, or as much of it as takes w one byte past its limit.
func (w *textWriter) WriteString(s string) (int, error) {
	if w.limit > 0 {
		room := w.limit + 1 - w.Len()
		if room <= 0 {
			return 0, nil
		}
		if len(s) > room {
			s = s[:room]
		}
	}
	return w.Builder.WriteString(s)
}

// WriteByte writes c unless w is full.
func (w *textWriter) WriteByte(c byte) error {
	if w.full() {
		return nil
	}
	return w.Builder.WriteByte(c)
}

// WriteRune writes r unless w is full.
func (w *textWriter) WriteRune(r rune) (int, error) {
	if w.full() {
		return 0, nil
	}
	return w.Builder.WriteRune(r)
}

// shortText returns what write writes, shortened, writing no more of it than
// shortened keeps.
func shortText(write func(w *textWriter)) string {
	w := textWriter{limit: shortLimit}
	write(&w)
	return shortened(w.String(), func(s string) string { return s })
}
