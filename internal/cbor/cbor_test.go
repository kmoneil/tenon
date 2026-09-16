package cbor

import (
	"bytes"
	"encoding/hex"
	"errors"
	"math"
	"strings"
	"testing"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestAppendUsesTheShortestForm(t *testing.T) {
	for _, tt := range []struct {
		got  []byte
		want string
	}{
		{AppendHead(nil, MajorUint, 0), "00"},
		{AppendHead(nil, MajorUint, 23), "17"},
		{AppendHead(nil, MajorUint, 24), "18 18"},
		{AppendHead(nil, MajorUint, 255), "18 ff"},
		{AppendHead(nil, MajorUint, 256), "19 0100"},
		{AppendHead(nil, MajorUint, 65535), "19 ffff"},
		{AppendHead(nil, MajorUint, 65536), "1a 00010000"},
		{AppendHead(nil, MajorUint, math.MaxUint32), "1a ffffffff"},
		{AppendHead(nil, MajorUint, math.MaxUint32+1), "1b 0000000100000000"},
		{AppendHead(nil, MajorUint, math.MaxUint64), "1b ffffffffffffffff"},
		{AppendInt(nil, -1), "20"},
		{AppendInt(nil, -25), "38 18"},
		{AppendInt(nil, math.MinInt64), "3b 7fffffffffffffff"},
		{AppendText(nil, "ab"), "62 6162"},
		{AppendBytes(nil, []byte{1}), "41 01"},
		{AppendArray(nil, 2), "82"},
		{AppendMap(nil, 1), "a1"},
		{AppendTag(nil, 4), "c4"},
		{AppendTag(nil, 1952804352), "da 74656e00"},
		{AppendBool(nil, false), "f4"},
		{AppendBool(nil, true), "f5"},
		{AppendNull(nil), "f6"},
	} {
		if want := mustHex(t, tt.want); !bytes.Equal(tt.got, want) {
			t.Errorf("wrote %x, want %x", tt.got, want)
		}
	}
}

func TestReaderReadsWhatIsWritten(t *testing.T) {
	var b []byte
	b = AppendTag(b, 1952804352)
	b = AppendArray(b, 6)
	b = AppendHead(b, MajorUint, math.MaxUint64)
	b = AppendInt(b, -1000)
	b = AppendText(b, "caf\xc3\xa9")
	b = AppendBytes(b, []byte{0, 1, 2})
	b = AppendMap(b, 1)
	b = AppendUint(b, 3)
	b = AppendBool(b, true)
	b = AppendNull(b)

	r := NewReader(b)
	if tag, err := r.ReadTag(); err != nil || tag != 1952804352 {
		t.Fatalf("tag %d, %v", tag, err)
	}
	if n, err := r.ReadArray(); err != nil || n != 6 {
		t.Fatalf("array %d, %v", n, err)
	}
	if u, err := r.ReadUint(); err != nil || u != math.MaxUint64 {
		t.Errorf("uint %d, %v", u, err)
	}
	if neg, arg, err := r.ReadInt(); err != nil || !neg || arg != 999 {
		t.Errorf("int %t %d, %v", neg, arg, err)
	}
	if s, err := r.ReadText(); err != nil || s != "caf\xc3\xa9" {
		t.Errorf("text %q, %v", s, err)
	}
	if p, err := r.ReadBytes(); err != nil || !bytes.Equal(p, []byte{0, 1, 2}) {
		t.Errorf("bytes %x, %v", p, err)
	}
	if n, err := r.ReadMap(); err != nil || n != 1 {
		t.Errorf("map %d, %v", n, err)
	}
	if u, err := r.ReadUint(); err != nil || u != 3 {
		t.Errorf("key %d, %v", u, err)
	}
	if v, err := r.ReadBool(); err != nil || !v {
		t.Errorf("bool %t, %v", v, err)
	}
	if !r.ReadNull() || r.Remaining() != 0 {
		t.Errorf("null not read, %d bytes left", r.Remaining())
	}
}

func TestReaderRefusesWhatTheSubsetDoesNotWrite(t *testing.T) {
	for _, tt := range []struct {
		name      string
		input     string
		read      func(r *Reader) error
		canonical bool
		message   string
	}{
		{"nothing", "", func(r *Reader) error { _, err := r.ReadHead(); return err }, false, "ends where"},
		{"a truncated head", "19 01", func(r *Reader) error { _, err := r.ReadHead(); return err }, false, "ends within"},
		{"reserved information", "1c", func(r *Reader) error { _, err := r.ReadHead(); return err }, false, "reserved"},
		{"an indefinite array", "9f ff", func(r *Reader) error { _, err := r.ReadArray(); return err }, true, "indefinite"},
		{"an indefinite text", "7f ff", func(r *Reader) error { _, err := r.ReadText(); return err }, true, "indefinite"},
		{"a break", "ff", func(r *Reader) error { _, err := r.ReadHead(); return err }, false, "31"},
		{"a float", "f9 0000", func(r *Reader) error { _, err := r.ReadHead(); return err }, false, "floating-point"},
		{"undefined", "f7", func(r *Reader) error { _, err := r.ReadHead(); return err }, false, "simple value 23"},
		{"the wrong major type", "01", func(r *Reader) error { _, err := r.ReadText(); return err }, false, "expected a text string"},
		{"a long text", "63 6162", func(r *Reader) error { _, err := r.ReadText(); return err }, false, "longer than"},
		{"invalid UTF-8", "61 ff", func(r *Reader) error { _, err := r.ReadText(); return err }, false, "UTF-8"},
		{"a long byte string", "5a ffffffff 00", func(r *Reader) error { _, err := r.ReadBytes(); return err }, false, "longer than"},
		// A few bytes declaring billions of items are refused before
		// anything is allocated for them.
		{"a huge array", "9b 00000001 00000000 00", func(r *Reader) error { _, err := r.ReadArray(); return err }, false, "more than"},
		{"a huge map", "ba ffffffff 0000", func(r *Reader) error { _, err := r.ReadMap(); return err }, false, "more than"},
		{"null as a bool", "f6", func(r *Reader) error { _, err := r.ReadBool(); return err }, false, "false or true"},
	} {
		r := NewReader(mustHex(t, tt.input))
		err := tt.read(r)
		var e *Error
		if !errors.As(err, &e) {
			t.Errorf("%s: %v, want a refusal", tt.name, err)
			continue
		}
		if e.Canonical != tt.canonical || !strings.Contains(e.Message, tt.message) {
			t.Errorf("%s: %+v, want canonical %t and a message containing %q", tt.name, e, tt.canonical, tt.message)
		}
		if r.Offset() != 0 {
			t.Errorf("%s: the refusal left the reader at byte %d", tt.name, r.Offset())
		}
	}
}
