package tenon

import (
	"hash/maphash"
	"slices"

	"tenon/internal/decimal"
)

// hashSeed is drawn once per process, so a hash means nothing outside the run
// that produced it. Anything that writes one down, or compares one against a
// hash from another run, finds out rather than appearing to work.
var hashSeed = maphash.MakeSeed()

// Hash returns a hash of a known value. Values that Identical reports the same
// hash the same, so a hash can stand in for a value in a table, and values that
// differ usually hash differently, which is all a hash owes.
//
// The hash is stable within a process and not between processes. It must not be
// written down, serialized, or compared against one from another run.
//
// Hash panics on a value that is not known, and on null, which has no hash.
func Hash(v Value) uint64 {
	n := v.data()
	if n.state == stateNull {
		usagePanic("Hash called on %s, and null has no hash", n.describe())
	}
	if !n.isKnown() {
		usagePanic("Hash called on %s, which is not a known value", n.describe())
	}
	return hashNode(n)
}

// hashNode returns the hash of a known value or of a member of one, which may
// be null even where the value that holds it is not.
func hashNode(n *node) uint64 {
	var h maphash.Hash
	h.SetSeed(hashSeed)
	writeHash(&h, n)
	return h.Sum64()
}

// writeHash writes a known value into h. The type goes first, so values of
// different types rarely collide, and everything of unknown length says how
// long it is, so a sequence of values cannot be mistaken for another sequence.
func writeHash(h *maphash.Hash, n *node) {
	writeUint(h, n.typ.t.id)
	if n.state == stateNull {
		h.WriteByte(0)
		return
	}
	h.WriteByte(1)
	switch n.typ.t.kind {
	case KindBool:
		if n.data.(bool) {
			h.WriteByte(1)
		} else {
			h.WriteByte(0)
		}
	case KindNumber:
		// The canonical form, which every way of writing one number shares.
		n.data.(decimal.Dec).WriteHash(h)
	case KindString:
		s := n.data.(string)
		writeUint(h, uint64(len(s)))
		h.WriteString(s)
	case KindCapsule:
		n.typ.t.capsule.writeHash(h, n.data)
	case KindSet:
		writeSetHash(h, n.data.([]Value))
	case KindMap:
		entries := n.data.([]mapEntry)
		writeUint(h, uint64(len(entries)))
		for _, e := range entries {
			// Entries are kept in key order, so this walk is the same walk
			// every time, whatever order the map was built in.
			writeUint(h, uint64(len(e.key)))
			h.WriteString(e.key)
			writeHash(h, e.val.n)
		}
	default:
		// A list, a tuple or an object: members in order, and for the last two
		// the type fixes what that order is.
		elems := n.data.([]Value)
		writeUint(h, uint64(len(elems)))
		for _, e := range elems {
			writeHash(h, e.n)
		}
	}
}

// writeSetHash writes a set into h. A set is its members, so the order they
// were given in and the number of times each was given must make no difference:
// the member hashes are sorted, and those that are equal are written once.
func writeSetHash(h *maphash.Hash, members []Value) {
	hashes := make([]uint64, len(members))
	for i, m := range members {
		hashes[i] = hashNode(m.n)
	}
	slices.Sort(hashes)
	hashes = slices.Compact(hashes)
	writeUint(h, uint64(len(hashes)))
	for _, x := range hashes {
		writeUint(h, x)
	}
}

// writeUint writes a number that delimits or tags what follows it.
func writeUint(h *maphash.Hash, x uint64) { maphash.WriteComparable(h, x) }
