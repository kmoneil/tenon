// Package cbor writes and reads the subset of CBOR (RFC 8949) that tenon's
// encoding uses: integers, byte and text strings, arrays, maps, tags, and the
// simple values false, true and null. Everything is written in preferred
// serialization, the shortest form of every argument, with definite lengths.
//
// The reader never allocates for a length the input declares before checking
// that the input remaining could hold it, so a few bytes of input cannot make
// it allocate much more than their own size.
package cbor

import (
	"fmt"
	"unicode/utf8"
)

// The major types of a data item.
const (
	MajorUint   byte = 0
	MajorNeg    byte = 1
	MajorBytes  byte = 2
	MajorText   byte = 3
	MajorArray  byte = 4
	MajorMap    byte = 5
	MajorTag    byte = 6
	MajorSimple byte = 7
)

// The simple values the subset uses, as the argument of a major type 7 head.
const (
	SimpleFalse = 20
	SimpleTrue  = 21
	SimpleNull  = 22
)

// AppendHead appends the head of a data item of the given major type and
// argument, in the shortest form.
func AppendHead(b []byte, major byte, arg uint64) []byte {
	m := major << 5
	switch {
	case arg < 24:
		return append(b, m|byte(arg))
	case arg <= 0xff:
		return append(b, m|24, byte(arg))
	case arg <= 0xffff:
		return append(b, m|25, byte(arg>>8), byte(arg))
	case arg <= 0xffffffff:
		return append(b, m|26, byte(arg>>24), byte(arg>>16), byte(arg>>8), byte(arg))
	}
	return append(b, m|27, byte(arg>>56), byte(arg>>48), byte(arg>>40), byte(arg>>32),
		byte(arg>>24), byte(arg>>16), byte(arg>>8), byte(arg))
}

// AppendUint appends the unsigned integer n.
func AppendUint(b []byte, n uint64) []byte { return AppendHead(b, MajorUint, n) }

// AppendInt appends the integer n.
func AppendInt(b []byte, n int64) []byte {
	if n < 0 {
		return AppendHead(b, MajorNeg, uint64(-1-n))
	}
	return AppendHead(b, MajorUint, uint64(n))
}

// AppendText appends the text string s, which must be valid UTF-8.
func AppendText(b []byte, s string) []byte {
	return append(AppendHead(b, MajorText, uint64(len(s))), s...)
}

// AppendBytes appends the byte string p.
func AppendBytes(b []byte, p []byte) []byte {
	return append(AppendHead(b, MajorBytes, uint64(len(p))), p...)
}

// AppendArray appends the head of an array of n items.
func AppendArray(b []byte, n int) []byte { return AppendHead(b, MajorArray, uint64(n)) }

// AppendMap appends the head of a map of n pairs.
func AppendMap(b []byte, n int) []byte { return AppendHead(b, MajorMap, uint64(n)) }

// AppendTag appends the head of a tag.
func AppendTag(b []byte, tag uint64) []byte { return AppendHead(b, MajorTag, tag) }

// AppendBool appends false or true.
func AppendBool(b []byte, v bool) []byte {
	if v {
		return append(b, MajorSimple<<5|SimpleTrue)
	}
	return append(b, MajorSimple<<5|SimpleFalse)
}

// AppendNull appends null.
func AppendNull(b []byte) []byte { return append(b, MajorSimple<<5|SimpleNull) }

// Error describes input that the reader refuses, at the offset where it
// found the problem.
type Error struct {
	Offset int
	// Canonical is set where the input is well-formed CBOR that the subset
	// does not write, such as an indefinite length, rather than malformed.
	Canonical bool
	Message   string
}

func (e *Error) Error() string { return fmt.Sprintf("at byte %d: %s", e.Offset, e.Message) }

// Head is the head of a data item.
type Head struct {
	Major byte
	// Arg is the argument: the value of an integer, the length of a string,
	// array or map, the number of a tag, or the simple value.
	Arg uint64
}

// Reader reads data items from a byte slice.
type Reader struct {
	data []byte
	pos  int
}

// NewReader returns a reader of data.
func NewReader(data []byte) *Reader { return &Reader{data: data} }

// Offset returns the offset of the next byte to read.
func (r *Reader) Offset() int { return r.pos }

// Remaining returns how many bytes are left to read.
func (r *Reader) Remaining() int { return len(r.data) - r.pos }

// Consumed returns the bytes read from offset start to the current offset.
func (r *Reader) Consumed(start int) []byte { return r.data[start:r.pos] }

func (r *Reader) fail(at int, format string, args ...any) error {
	return &Error{Offset: at, Message: fmt.Sprintf(format, args...)}
}

// PeekHead returns the head of the next data item without reading it.
func (r *Reader) PeekHead() (Head, error) {
	pos := r.pos
	h, err := r.ReadHead()
	r.pos = pos
	return h, err
}

// ReadHead reads the head of the next data item. It refuses input that ends
// within the head, reserved additional information, indefinite lengths, and
// the major type 7 items the subset does not use: floating-point numbers and
// simple values other than false, true and null.
func (r *Reader) ReadHead() (Head, error) {
	start := r.pos
	if r.pos >= len(r.data) {
		return Head{}, r.fail(start, "the input ends where a data item was expected")
	}
	first := r.data[r.pos]
	r.pos++
	major, info := first>>5, first&0x1f
	var arg uint64
	switch {
	case info < 24:
		arg = uint64(info)
	case info <= 27:
		size := 1 << (info - 24)
		if r.Remaining() < size {
			r.pos = start
			return Head{}, r.fail(start, "the input ends within the head of a data item")
		}
		for _, c := range r.data[r.pos : r.pos+size] {
			arg = arg<<8 | uint64(c)
		}
		r.pos += size
	case info == 31:
		r.pos = start
		if major >= MajorBytes && major <= MajorMap {
			return Head{}, &Error{Offset: start, Canonical: true, Message: "an indefinite length, which the encoding does not use"}
		}
		return Head{}, r.fail(start, "additional information 31 on major type %d", major)
	default:
		r.pos = start
		return Head{}, r.fail(start, "reserved additional information %d", info)
	}
	if major == MajorSimple {
		if info >= 25 && info <= 27 {
			r.pos = start
			return Head{}, r.fail(start, "a floating-point number, which the encoding does not use")
		}
		// RFC 8949 3.3: f8 followed by a byte below 32 is not well-formed,
		// so a two-byte spelling of false, true or null is refused here, not
		// read as the value.
		if info == 24 && arg < 32 {
			r.pos = start
			return Head{}, r.fail(start, "a two-byte simple value below 32, which is not well-formed")
		}
		if arg != SimpleFalse && arg != SimpleTrue && arg != SimpleNull {
			r.pos = start
			return Head{}, r.fail(start, "the simple value %d, which the encoding does not use", arg)
		}
	}
	return Head{Major: major, Arg: arg}, nil
}

// expect reads a head of the given major type.
func (r *Reader) expect(major byte, what string) (Head, error) {
	start := r.pos
	h, err := r.ReadHead()
	if err != nil {
		return h, err
	}
	if h.Major != major {
		r.pos = start
		return h, r.fail(start, "expected %s", what)
	}
	return h, nil
}

// ReadUint reads an unsigned integer.
func (r *Reader) ReadUint() (uint64, error) {
	h, err := r.expect(MajorUint, "an unsigned integer")
	return h.Arg, err
}

// ReadInt reads an integer, returning whether it is negative and its
// argument: the integer is the argument, or -1 minus it where negative.
func (r *Reader) ReadInt() (negative bool, arg uint64, err error) {
	start := r.pos
	h, err := r.ReadHead()
	if err != nil {
		return false, 0, err
	}
	if h.Major != MajorUint && h.Major != MajorNeg {
		r.pos = start
		return false, 0, r.fail(start, "expected an integer")
	}
	return h.Major == MajorNeg, h.Arg, nil
}

// ReadText reads a text string, which must be valid UTF-8.
func (r *Reader) ReadText() (string, error) {
	start := r.pos
	h, err := r.expect(MajorText, "a text string")
	if err != nil {
		return "", err
	}
	if h.Arg > uint64(r.Remaining()) {
		r.pos = start
		return "", r.fail(start, "a text string of %d bytes, longer than the %d bytes left", h.Arg, r.Remaining())
	}
	s := string(r.data[r.pos : r.pos+int(h.Arg)])
	if !utf8.ValidString(s) {
		r.pos = start
		return "", r.fail(start, "a text string that is not valid UTF-8")
	}
	r.pos += int(h.Arg)
	return s, nil
}

// ReadBytes reads a byte string, returning a slice of the input.
func (r *Reader) ReadBytes() ([]byte, error) {
	start := r.pos
	h, err := r.expect(MajorBytes, "a byte string")
	if err != nil {
		return nil, err
	}
	if h.Arg > uint64(r.Remaining()) {
		r.pos = start
		return nil, r.fail(start, "a byte string of %d bytes, longer than the %d bytes left", h.Arg, r.Remaining())
	}
	p := r.data[r.pos : r.pos+int(h.Arg)]
	r.pos += int(h.Arg)
	return p, nil
}

// ReadArray reads the head of an array, returning its length, which is no
// more than the bytes left, since every item takes at least one.
func (r *Reader) ReadArray() (int, error) {
	start := r.pos
	h, err := r.expect(MajorArray, "an array")
	if err != nil {
		return 0, err
	}
	if h.Arg > uint64(r.Remaining()) {
		r.pos = start
		return 0, r.fail(start, "an array of %d items, more than the %d bytes left could hold", h.Arg, r.Remaining())
	}
	return int(h.Arg), nil
}

// ReadMap reads the head of a map, returning its number of pairs, which is no
// more than half the bytes left, since every pair takes at least two.
func (r *Reader) ReadMap() (int, error) {
	start := r.pos
	h, err := r.expect(MajorMap, "a map")
	if err != nil {
		return 0, err
	}
	if h.Arg > uint64(r.Remaining()/2) {
		r.pos = start
		return 0, r.fail(start, "a map of %d pairs, more than the %d bytes left could hold", h.Arg, r.Remaining())
	}
	return int(h.Arg), nil
}

// ReadTag reads the head of a tag, returning its number.
func (r *Reader) ReadTag() (uint64, error) {
	h, err := r.expect(MajorTag, "a tag")
	return h.Arg, err
}

// ReadBool reads false or true.
func (r *Reader) ReadBool() (bool, error) {
	start := r.pos
	h, err := r.expect(MajorSimple, "false or true")
	if err != nil {
		return false, err
	}
	if h.Arg == SimpleNull {
		r.pos = start
		return false, r.fail(start, "expected false or true")
	}
	return h.Arg == SimpleTrue, nil
}

// ReadNull reads null, reporting whether the next item was null; any other
// item, a head that cannot be read included, is left unread. It advances
// past the whole head it read, not one byte, so no byte of the head is read
// again as the next item.
func (r *Reader) ReadNull() bool {
	start := r.pos
	h, err := r.ReadHead()
	if err != nil || h.Major != MajorSimple || h.Arg != SimpleNull {
		r.pos = start
		return false
	}
	return true
}
