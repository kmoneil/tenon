package tenon_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"unicode/utf8"

	"github.com/kmoneil/tenon"
)

// FuzzString holds string construction to its rules for any input. Ill-formed
// UTF-8 is an error value with its code. Anything else is a String whose content
// is normalized and constructs the same value again; which equals the string
// written in any other normalization form; whose length is known and no more
// than its scalar values; whose display form writes every character it must
// escape as an escape; and which serializes and projects to JSON faithfully.
func FuzzString(f *testing.F) {
	for _, s := range []string{
		"", "a", "caf\U000000E9", "cafe\U00000301", "\xff", "a\xed\xa0\x80b", "\U0001F600",
		"\t\n\r\x00\x7f", "a\U0000200Bb", "\U000000A0\U00002028\U0000FEFF", "\U0001F1FA\U0001F1F8", `"\`,
		"\U00000378\U0010FFFF", "e\U00000301\U00000301\U00000323", "\U00001100\U00001161\U000011A8",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		v := tenon.String(s)
		if !utf8.ValidString(s) {
			if !v.IsError() || v.Diagnostics()[0].Code != tenon.CodeStringInvalidUTF8 {
				t.Fatalf("String(%q) = %v, want an error value with code %s", s, v, tenon.CodeStringInvalidUTF8)
			}
			return
		}
		if v.IsError() || v.Type() != tenon.StringType() {
			t.Fatalf("String(%q) = %v, want a String", s, v)
		}
		content := v.AsString()
		if again := tenon.String(content); !tenon.Identical(again, v) {
			t.Fatalf("String(%q) = %v, but its content constructs %v", s, v, again)
		}
		checkAgainstXText(t, s, v, content)
		if n, ok := tenon.Length(v).AsInt64(); !ok || n < 0 || n > int64(utf8.RuneCountInString(content)) || (n == 0) != (content == "") {
			t.Fatalf("Length(%v) = %v", v, tenon.Length(v))
		}
		checkDisplayedCategories(t, v)
		b, failure, ok := tenon.Serialize(v)
		if !ok {
			t.Fatalf("Serialize(%v) failed: %v", v, failure)
		}
		if back, failure, ok := tenon.Deserialize(b, tenon.Decoders{}); !ok || !tenon.Identical(back, v) {
			t.Fatalf("%v serializes as %x, which deserializes as %v, %v", v, b, back, failure)
		}
		projected, failure, ok := tenon.ProjectJSON(v)
		if !ok {
			t.Fatalf("ProjectJSON(%v) failed: %v", v, failure)
		}
		var decoded string
		if err := json.NewDecoder(bytes.NewReader(projected)).Decode(&decoded); err != nil || decoded != content {
			t.Fatalf("%v projects as %s, which decodes as %q, %v", v, projected, decoded, err)
		}
	})
}
