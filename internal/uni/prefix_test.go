package uni_test

import (
	"strings"
	"testing"

	"github.com/kmoneil/tenon/internal/uni"
)

func TestStablePrefix(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   string
		want string
	}{
		{"nothing at all", "", ""},
		{"a hyphen composes with nothing", "ab-", "ab-"},
		{"a digit composes with nothing", "v1", "v1"},
		{"a space composes with nothing", "one ", "one "},
		{"q is the one letter that composes with nothing", "seq", "seq"},
		{"a trailing letter a mark would change", "cafe", "caf"},
		{"a trailing letter, even without an accent in sight", "hello world", "hello worl"},
		{"a combining sequence keeps nothing", "e\U00000301", ""},
		{"a regional indicator pair composes with nothing", "\U0001F1E9\U0001F1EA", "\U0001F1E9\U0001F1EA"},
		{"text before a combining sequence", "v1.0-e\U00000301", "v1.0-"},
		// U+11382 is unassigned in Unicode 15.0.0, the version these tables
		// pin, so nothing composes with it here, whatever a later Unicode
		// says of it and whichever toolchain builds this.
		{"a code point a later Unicode gives a composition", "x\U00011382", "x\U00011382"},
		{"a Hangul syllable with no trailing consonant takes one", "\U0000AC00", ""},
		{"a Hangul leading consonant takes a vowel", "a\U00001100", "a"},
	} {
		if got := uni.StablePrefix(uni.NFC(tt.in)); got != tt.want {
			t.Errorf("%s: StablePrefix(%q) = %q, want %q", tt.name, tt.in, got, tt.want)
		}
	}
}

func TestStablePrefixSurvivesAnyContinuation(t *testing.T) {
	// The property the truncation exists for: however the text it was taken
	// from continues, the canonical form of the whole still begins with it.
	suffixes := []string{
		"", "x", " ", "\U00000301", "\U00000307", "\U0000030C", "\U00000323",
		"\U00000327", "\U0000200D", "\U0001F1EB", "e\U00000301", "\U00000301x",
	}
	for _, in := range []string{
		"cafe", "hello world", "seq", "ab-", "v1", "e\U00000301", "x\U00000301-",
		"\U0001F1E9\U0001F1EA", "\U0001F1E9", "\U0001F468\U0000200D", "",
	} {
		s := uni.NFC(in)
		p := uni.StablePrefix(s)
		for _, suffix := range suffixes {
			if joined := uni.NFC(s + suffix); !strings.HasPrefix(joined, p) {
				t.Errorf("StablePrefix(%q) = %q, but %q + %q normalizes to %q", in, p, in, suffix, joined)
			}
		}
	}
}

// A run of combining marks longer than thirty stays a run, since plain NFC
// caps none (ST-002), and the prefix kept from text ending in one still
// begins whatever follows: marks that sort before the run's, after it, or
// compose with its starter.
func TestStablePrefixHoldsPastThirtyMarks(t *testing.T) {
	for _, n := range []int{1, 29, 30, 31, 32, 40, 100} {
		s := uni.NFC("ab-e" + strings.Repeat("\U00000301", n))
		p := uni.StablePrefix(s)
		if p != "ab-" {
			t.Errorf("%d marks: StablePrefix(%q) = %q, want %q", n, s, p, "ab-")
		}
		for _, next := range []string{"\U00000323", "\U00000301", "\U00000327", "x", ""} {
			if got := uni.NFC(s + next); !strings.HasPrefix(got, p) {
				t.Errorf("%d marks, then %q: %q does not begin with %q", n, next, got, p)
			}
		}
	}
}
