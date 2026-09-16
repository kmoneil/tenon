package uni_test

import (
	"strings"
	"testing"

	"tenon/internal/uni"
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
