package tenon

import (
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
)

// Mark is caller-defined metadata attached to a value: a sensitivity label,
// a provenance note, anything that must travel with a value without being
// part of it. Marks never change what a value is or what an operation
// returns; they change only what is attached to the result.
//
// A mark is any comparable value implementing this interface. Marks are told
// apart by Go equality, so a mark is usually a small struct or a pointer,
// and attaching one that is not comparable is a usage panic. A struct holding
// a [Value], a [Constraint] or a [Path] is not comparable, so a mark keeps
// what it says as Go data and builds its payload from that. The identifier
// names the mark where its value cannot appear, such as a redaction
// placeholder or a serialized form.
//
// How a mark moves is the mark's choice. A Propagate mark appears on the
// result of every operation that consumes the marked value; an Isolate mark
// stays on the value it was attached to. Narrowing and resolving refine the
// value they are given rather than deriving a new one, so both keep every
// mark, Isolate included. A mark that implements DeepMark can also reach
// down, to every value within the one it is attached to.
type Mark interface {
	// MarkID returns the stable identifier of the mark, used where the mark
	// must be named without its value: redaction placeholders and encodings.
	MarkID() string
	// Propagation returns how the mark moves through operations: Propagate
	// or Isolate, and attaching a mark that returns anything else is a
	// usage panic. A redacting mark propagates whatever this says, since
	// what it withholds must not show in anything derived from the value.
	Propagation() Propagation
	// Redacting reports whether the contents of a value carrying the mark
	// are withheld wherever the value is described: in the messages of
	// diagnostics, and in String, which puts a placeholder naming the mark
	// in their place. Its contents include its structure, the keys of a map
	// and an object's attribute names: a diagnostic arising within the value
	// is located at it, and a collection whose element type takes attribute
	// names from it carries the mark.
	Redacting() bool
}

// DeepMark is a Mark that can declare itself deep. A deep mark attached to a
// collection or structural value is attached to every value within it as
// well, at any depth, so a mark put on a whole document is on each part of it
// that is read out. A mark type declares itself deep by implementing DeepMark
// with a Deep method that reports true, and Deep must give the same answer
// every time it is asked. A mark that does not implement DeepMark is not deep.
//
// WithMarks applies a deep mark when it attaches it, rather than leaving it to
// be looked up later: once WithMarks returns, the values within carry the mark
// in their own right, and taking it off the outer value with Unmark leaves it
// on them. The members of a set are the exception, because they carry no
// marks (see Set): a deep mark on a set stays on the set, and Elements
// attaches it to each member as it returns the member.
type DeepMark interface {
	Mark
	// Deep reports whether the mark is attached to every value within the
	// value it is attached to.
	Deep() bool
}

// EncodableMark is a Mark that declares how it is serialized: as its identifier
// alone, or with a value. A value carrying a mark that does not implement
// EncodableMark cannot be serialized.
type EncodableMark interface {
	Mark
	// MarkPayload returns the value the mark is serialized with and true, or
	// false where the identifier alone serializes it. The value must be known,
	// unmarked and not a null, and marks that are not equal must not serialize
	// alike.
	MarkPayload() (Value, bool)
}

// Propagation says how a mark moves through operations.
type Propagation uint8

const (
	// Propagate puts the mark on the result of any operation that consumes
	// the marked value. It is the zero value: a mark propagates unless it
	// says otherwise.
	Propagate Propagation = iota
	// Isolate keeps the mark on the value it is attached to; results
	// derived from that value do not carry it. A redacting mark propagates
	// even so: a result that did not carry it would show what it withholds.
	Isolate
)

// knownPolicy reports whether p is one of the propagation policies, which a
// mark must declare: a mark declaring another would be carried as Isolate
// is, which is no policy it said, and one a later version adds would be
// taken for Isolate by this one.
func knownPolicy(p Propagation) bool { return p == Propagate || p == Isolate }

// String returns the name of the propagation policy, "propagate" or
// "isolate".
func (p Propagation) String() string {
	switch p {
	case Propagate:
		return "propagate"
	case Isolate:
		return "isolate"
	}
	return "Propagation(" + strconv.Itoa(int(p)) + ")"
}

// markSet is the immutable set of marks on a value. It is nil on an unmarked
// value, which therefore pays a nil pointer and nothing else for the marks it
// does not have. Nothing changes a mark set once it is made, so values that
// carry the same marks may share one.
//
// A set is held in layers. list is the set's own layer, sorted by identifier,
// attachment order breaking ties. outer is the layer of marks the value
// inherits, the deep marks of the values above it, which every value
// inheriting them shares rather than each holding a copy: a list whose
// members carry marks of their own beside k deep marks would otherwise hold k
// marks per member. The marks a value carries are its own layer's, then its
// outer layers', each once (all). An outer layer is itself a mark set, and
// layer says a set is one, which values share and which keeps its merged list
// once asked for it.
type markSet struct {
	list  []Mark
	outer *markSet
	layer bool
	full  atomic.Pointer[[]Mark]
	// redacting is a layer's redacting marks, kept as full is, since each
	// value sharing the layer asks for them.
	redacting atomic.Pointer[[]Mark]
}

// all returns every mark s holds, each once, sorted by identifier: among marks
// that share one, those of its own layer first, then each outer layer's in
// turn. The result is read, never written to: it may be the set's own list.
// Only a layer keeps the list it merges, since values share a layer, where a
// value's own set is asked for its marks rarely and keeping each would hold
// again what the layers save.
func (s *markSet) all() []Mark {
	if s.outer == nil {
		return s.list
	}
	if s.layer {
		if p := s.full.Load(); p != nil {
			return *p
		}
	}
	merged, _ := mergeDistinct(s.list, s.outer.all())
	if s.layer {
		s.full.Store(&merged)
	}
	return merged
}

// redactingMarks returns the redacting marks among all, in its order, or nil:
// those of s's own list merged with its outer layers', which a layer keeps,
// rather than all merged and then filtered, which would cost every value
// sharing a layer the layer's marks each time.
func (s *markSet) redactingMarks() []Mark {
	if s.layer {
		if p := s.redacting.Load(); p != nil {
			return *p
		}
	}
	merged := redactingOf(s.list)
	if s.outer != nil {
		if outer := s.outer.redactingMarks(); merged == nil {
			merged = outer
		} else {
			merged, _ = mergeDistinct(merged, outer)
		}
	}
	if s.layer {
		s.redacting.Store(&merged)
	}
	return merged
}

// withoutMarks returns v without the marks it carries, as Unmark does, where
// the marks themselves are not wanted: listing them costs a value sharing a
// layer the layer's marks.
func withoutMarks(v Value) Value {
	if v.n.marks == nil {
		return v
	}
	nn := v.n.clone()
	nn.marks = nil
	return Value{n: nn}
}

// contains reports whether s holds m, looking in each layer in turn.
func (s *markSet) contains(m Mark) bool {
	for ; s != nil; s = s.outer {
		if _, found := placeMark(s.list, m); found {
			return true
		}
	}
	return false
}

// markList returns the marks on n, nil when there are none. The result is
// read, never written to.
func (n *node) markList() []Mark {
	if n.marks == nil {
		return nil
	}
	return n.marks.all()
}

// WithMarks returns v carrying the given marks beside those it already
// carries. A mark that is already on v is not attached twice, and with
// nothing new to attach the result is v itself. A deep mark (DeepMark) is
// attached to every value within v as well, except the members of a set,
// which Elements marks as it returns them.
//
// WithMarks panics if a mark is nil or of a type that is not comparable,
// since Go equality is what tells marks apart, and if its propagation policy
// is neither Propagate nor Isolate.
func WithMarks(v Value, marks ...Mark) Value {
	n := v.data()
	for i, m := range marks {
		if m == nil {
			usagePanic("WithMarks called with a nil Mark as mark %d", i)
		}
		switch comparable, self := comparableMark(m); {
		case !comparable:
			usagePanic("WithMarks called with a mark of type %T, which is not comparable and so cannot be told from other marks", m)
		case !self:
			usagePanic("WithMarks called with a mark of type %T holding a value that does not equal itself, so it cannot be told from other marks", m)
		}
		if p := m.Propagation(); !knownPolicy(p) {
			usagePanic("WithMarks called with a mark of type %T whose propagation policy is %s, neither Propagate nor Isolate", m, p)
		}
	}
	merged, grew := mergeMarks(n.markList(), marks)
	if !grew {
		// A deep mark v carries already is on everything within v too, since
		// it was attached to those values when it was attached to v.
		return v
	}
	nn := n.clone()
	nn.marks = &markSet{list: merged}
	if deep := deepMarks(marks); deep != nil {
		newAttachment(deep, nil).within(nn)
	}
	return Value{n: nn}
}

// comparableMark reports whether Go equality can compare m at all, and
// whether it finds m equal to itself, both of which telling marks apart
// needs. Comparing a value of a type that is not comparable panics, and the
// panic is the answer; a mark holding a NaN compares and still fails, the
// one way a Go value is unequal to itself, and would vanish from every
// lookup that stored it.
func comparableMark(m Mark) (comparable, self bool) {
	defer func() {
		if recover() != nil {
			comparable, self = false, false
		}
	}()
	return true, m == m
}

// mergeMarks returns held with marks added, each once, sorted by identifier,
// and whether any of them was not held already. held itself is left as it is.
//
// held is sorted already, so a few marks are each looked for among the marks
// sharing its identifier and put where it belongs, rather than the whole list
// being searched and then sorted again. A value can carry thousands of marks,
// and one more arrives whenever a deep mark reaches it, so both of those cost
// more than the merge itself. Many marks at once, as a document gives a value,
// are merged in one pass instead (mergeMany).
func mergeMarks(held, marks []Mark) ([]Mark, bool) {
	if len(marks) > manyMarks {
		return mergeMany(held, marks)
	}
	merged, grew := held, false
	for i, m := range marks {
		at, found := placeMark(merged, m)
		if found {
			continue
		}
		if !grew {
			// Room for every mark still to come, in one list, where
			// growing it a mark at a time would make it again and again.
			merged, grew = make([]Mark, len(held), len(held)+len(marks)-i), true
			copy(merged, held)
		}
		merged = slices.Insert(merged, at, m)
	}
	return merged, grew
}

// mergeMany is mergeMarks for many marks. Looking each up among the marks
// sharing its identifier costs the square of them where many share one, and
// putting each in its place moves the rest of the list along, which costs the
// square of them where they interleave with those held. So the marks not
// held already are found through markLookup, sorted by identifier with the
// order they came in breaking ties, and merged with held in one pass, where a
// held mark comes first among those sharing an identifier, as it does when
// the marks are put in place one at a time.
func mergeMany(held, marks []Mark) ([]Mark, bool) {
	all := make([]Mark, len(held), len(held)+len(marks))
	copy(all, held)
	var seen markLookup
	for _, m := range marks {
		if !seen.holds(all, m) {
			all = append(all, m)
		}
	}
	fresh := all[len(held):]
	if len(fresh) == 0 {
		return held, false
	}
	sortMarks(fresh)
	merged := make([]Mark, 0, len(all))
	i, j := 0, 0
	for i < len(held) && j < len(fresh) {
		if fresh[j].MarkID() < held[i].MarkID() {
			merged = append(merged, fresh[j])
			j++
			continue
		}
		merged = append(merged, held[i])
		i++
	}
	merged = append(merged, held[i:]...)
	return append(merged, fresh[j:]...), true
}

// mergeDistinct is mergeMarks for marks that are sorted and distinct already,
// as the deep marks an attachment gives are: each is looked for among held
// alone, by markLookup, and the two lists are merged in one pass, a held mark
// first among those sharing an identifier. It builds a set only where held is
// long, where mergeMarks, given many marks, builds one of them all.
func mergeDistinct(held, marks []Mark) ([]Mark, bool) {
	var seen markLookup
	var merged []Mark
	i := 0
	for _, m := range marks {
		if seen.holds(held, m) {
			continue
		}
		if merged == nil {
			merged = make([]Mark, 0, len(held)+len(marks))
		}
		for ; i < len(held) && held[i].MarkID() <= m.MarkID(); i++ {
			merged = append(merged, held[i])
		}
		merged = append(merged, m)
	}
	if merged == nil {
		return held, false
	}
	return append(merged, held[i:]...), true
}

// placeMark returns where m belongs in a list of marks sorted by identifier,
// which is after the marks that share its identifier, so that marks arriving
// later sit behind those held already, and whether the list holds m already.
func placeMark(list []Mark, m Mark) (int, bool) {
	id := m.MarkID()
	at, _ := slices.BinarySearchFunc(list, id, func(h Mark, id string) int {
		return strings.Compare(h.MarkID(), id)
	})
	for ; at < len(list) && list[at].MarkID() == id; at++ {
		if list[at] == m {
			return at, true
		}
	}
	return at, false
}

// manyMarks is the most marks a markLookup scans for one mark. A value carries
// a handful of marks, among which a scan is quickest and allocates nothing; but
// a document can put thousands on one value, and scanning them for each mark
// costs the square of them: 4,000 marks took 36 ms to decode.
const manyMarks = 16

// markLookup says whether a mark is among the marks a list holds so far, by
// scanning the list while it is short and by a set of them once it is not. The
// list only grows while a markLookup is in use, so the set, once made, needs
// only the marks the list gains after it.
type markLookup struct {
	set map[Mark]struct{}
	n   int // how many of the list's marks the set holds
}

// holds reports whether m is in list, which holds the marks the previous
// calls saw and perhaps more at its end.
func (l *markLookup) holds(list []Mark, m Mark) bool {
	if l.set == nil {
		if len(list) <= manyMarks {
			return slices.Contains(list, m)
		}
		l.set = make(map[Mark]struct{}, 2*len(list))
	}
	for _, h := range list[l.n:] {
		l.set[h] = struct{}{}
	}
	l.n = len(list)
	_, ok := l.set[m]
	return ok
}

// sameMarkSet reports whether two lists hold the same marks. Marks are a set:
// what is there matters, the order they were attached in does not.
//
// A value holds its marks in one order, so two values carrying the same marks
// hold them alike wherever no two of those marks share an identifier, and the
// walk settles them a mark at a time. A mark that does not line up is looked
// for among the other list's marks, by a scan while there are few and by a
// set of them once there are many, as markLookup is used wherever marks are
// looked up: a value can carry thousands, one arriving whenever a deep mark
// reaches it, and scanning for each costs the square of them. The walk is a
// shortcut for a mark that is certainly there, not a premise, so no answer
// here depends on the order the marks are held in.
func sameMarkSet(x, y []Mark) bool {
	if len(x) != len(y) {
		return false
	}
	var seen markLookup
	for i, m := range x {
		if m == y[i] {
			continue
		}
		if !seen.holds(y, m) {
			return false
		}
	}
	return true
}

// isDeep reports whether m is a deep mark.
func isDeep(m Mark) bool {
	d, ok := m.(DeepMark)
	return ok && d.Deep()
}

// deepMarks returns the deep marks among marks, each once, sorted by
// identifier, or nil when there are none.
func deepMarks(marks []Mark) []Mark {
	var deep []Mark
	var seen markLookup
	for _, m := range marks {
		if isDeep(m) && !seen.holds(deep, m) {
			deep = append(deep, m)
		}
	}
	sortMarks(deep)
	return deep
}

// attachment attaches deep marks to everything within a value. It relies on
// what it keeps: a value carrying a deep mark has it on every value within
// it, except the members of a set, which carry no marks. Attaching a mark to
// a value that carries it already can therefore stop there. A value the
// decoder is reading does not keep it until it is settled, so settleDeep
// walks that value itself and takes from an attachment only the mark sets.
//
// The marks attached are one layer, which every value given them shares as
// its outer layer, after any it had: a value that held no marks holds the
// layer itself, and one that held some holds its own layer before it. Values
// that held the same set before the attachment hold the same set after it.
//
// A value whose own layer, or one of its outer layers, holds every mark
// attached carries them already, and the attachment stops there. A long list
// of marks is asked that once (held), since a list is shared: by the values
// of one container, and by every copy of an outer layer an attachment makes.
// Unasked, a mark attached again copied every value it reached and gave it
// one more layer each time: unmarking a value and marking it again, or
// putting its members in another container and marking that, copied the
// whole value however often it was done.
type attachment struct {
	layer  *markSet              // the marks to attach, and the layers out from them
	sets   map[*markSet]*markSet // what a value that held a set holds after
	chains map[*markSet]*markSet // what an outer layer becomes with layer beyond it
	held   map[listKey]bool      // whether a list holds every mark attached
}

// listKey is a list of marks as the lists that share it know it: where it
// starts and how long it is. Nothing changes a list of marks once it is made,
// so what a list holds follows from these.
type listKey struct {
	first *Mark
	n     int
}

// newAttachment returns an attachment of the deep marks deep, sorted and
// distinct, with outer, the layers of a value further out, beyond them.
func newAttachment(deep []Mark, outer *markSet) *attachment {
	return &attachment{layer: &markSet{list: deep, outer: outer, layer: true}}
}

// within attaches the deep marks to every value n holds, at any depth, other
// than the members of a set. n is a copy that nothing shares yet, and within
// replaces its content when a member changes.
func (a *attachment) within(n *node) {
	if n.state != stateKnown || n.typ.t.kind == KindSet {
		// A set's marks stay on the set, and Elements attaches them to each
		// member it returns.
		return
	}
	if data := a.replacedMembers(n, false); data != nil {
		n.data, n.markedWithin = data, true
	}
}

// replacedMembers returns the members of n, a known list, tuple, object or
// map, each given a's marks, by attach or, settling a value read, by
// settleDeep, and nil where every member stays as it was: the members are
// copied only where one of them changes, so that a value whose members all
// stay shares them. The two are told apart by a flag rather than handed in
// as a function, which would make every member's call an indirect one:
// attaching a deep mark to 9,331 values took about 8% longer so.
func (a *attachment) replacedMembers(n *node, settling bool) any {
	replace := func(m *node) *node {
		if settling {
			return settleDeep(m, a)
		}
		return a.attach(m)
	}
	switch data := n.data.(type) {
	case []Value:
		var members []Value
		for i, m := range data {
			if r := replace(m.n); r != m.n {
				if members == nil {
					members = slices.Clone(data)
				}
				members[i] = Value{n: r}
			}
		}
		if members != nil {
			return members
		}
	case []mapEntry:
		var entries []mapEntry
		for i, e := range data {
			if r := replace(e.val.n); r != e.val.n {
				if entries == nil {
					entries = slices.Clone(data)
				}
				entries[i].val = Value{n: r}
			}
		}
		if entries != nil {
			return entries
		}
	}
	return nil
}

// attach returns n carrying the deep marks, with everything within it
// carrying them too, or n itself when it carries them already.
func (a *attachment) attach(n *node) *node {
	if n.marks != nil && a.holds(n.marks.list) {
		return n
	}
	marks, grew := a.merged(n.marks)
	if !grew {
		return n
	}
	nn := n.clone()
	nn.marks = marks
	a.within(nn)
	return nn
}

// holds reports whether list holds every mark attached, those of the
// attached layer and of the layers out from it. A list shorter than those
// marks cannot. A short list is looked through each time it is asked, which
// costs less than keeping the answer, and a long one once, however many mark
// sets share it.
func (a *attachment) holds(list []Mark) bool {
	marks := a.layer.all()
	switch {
	case len(list) < len(marks):
		return false
	case len(list) <= manyMarks:
		return holdsMarks(list, marks)
	}
	key := listKey{&list[0], len(list)}
	if held, asked := a.held[key]; asked {
		return held
	}
	held := holdsMarks(list, marks)
	if a.held == nil {
		a.held = map[listKey]bool{}
	}
	a.held[key] = held
	return held
}

// holdsMarks reports whether list, sorted by identifier, holds every mark of
// marks, which are distinct: a few are each looked for among the marks
// sharing its identifier, as mergeMarks places them, and many through a
// markLookup, which scanning for each would cost the square of.
func holdsMarks(list, marks []Mark) bool {
	if len(marks) <= manyMarks {
		for _, m := range marks {
			if _, found := placeMark(list, m); !found {
				return false
			}
		}
		return true
	}
	var seen markLookup
	for _, m := range marks {
		if !seen.holds(list, m) {
			return false
		}
	}
	return true
}

// merged returns the mark set that a value holding held holds once the deep
// marks are attached to it, and whether that is another set. The value keeps
// its own layer and gains the attached one beyond its outer layers, sharing
// both with every value that held what it held, unless one of its outer
// layers holds the marks already (beyond). Whether its own layer does is
// asked by attach, not here: the decoder settles each value it reads through
// merged, and a value read lists none of the deep marks above it, which a
// document may not list again (SE-031). A mark its own layer holds beside
// others it lacks is held twice, and carried once (all).
func (a *attachment) merged(held *markSet) (*markSet, bool) {
	if held == nil {
		return a.layer, true
	}
	if set, ok := a.sets[held]; ok {
		return set, set != held
	}
	set := held
	if outer := a.beyond(held.outer); outer != held.outer {
		set = &markSet{list: held.list, outer: outer, layer: held.layer}
	}
	if a.sets == nil {
		a.sets = map[*markSet]*markSet{}
	}
	a.sets[held] = set
	return set, set != held
}

// beyond returns the layers outer with the attached layer beyond them, which
// is outer itself where the attached layer is among them already, or a layer
// among them holds every mark attached. Each layer is asked once, however
// many chains it is in.
func (a *attachment) beyond(outer *markSet) *markSet {
	if outer == nil {
		return a.layer
	}
	if outer == a.layer {
		return outer
	}
	if c, ok := a.chains[outer]; ok {
		return c
	}
	c := outer
	if !a.holds(outer.list) {
		if rest := a.beyond(outer.outer); rest != outer.outer {
			c = &markSet{list: outer.list, outer: rest, layer: true}
		}
	}
	if a.chains == nil {
		a.chains = map[*markSet]*markSet{}
	}
	a.chains[outer] = c
	return c
}

// withOwnMarks returns v carrying marks on itself alone, as WithMarks does but
// for a deep mark, which it leaves for settleDeep to give the values within v.
// The decoder reads a value this way, part by part, and settles it once read.
// The marks are the decoder's, which it has held to what WithMarks asks of a
// mark as each mark decoder returned it (mark).
func withOwnMarks(v Value, marks []Mark) Value {
	merged, grew := mergeMarks(v.n.markList(), marks)
	if !grew {
		return v
	}
	nn := v.n.clone()
	nn.marks = &markSet{list: merged}
	return Value{n: nn}
}

// settleDeep gives n, and every value within it but a set's members, the deep
// marks the values above it carry: those a attaches, nil where n is the value
// read. The decoder gives each part of a value only the marks listed on it,
// and makes this one pass when the value is read, outside in, so that each
// value's marks are merged once. Attached level by level as each level is
// read, a value's marks were merged once for every deep mark above it, which
// for a nest of d levels comes to the cube of d.
//
// Among marks sharing an identifier, a value holds its own first, then those
// of the value nearest above it, and so on out, which is the order attaching
// them level by level gives. It walks every value rather than stopping at one
// that holds the marks already, as attach does: until this pass nothing has
// given the values within a value its deep marks, so holding them says
// nothing of those values.
func settleDeep(n *node, a *attachment) *node {
	own := n.markList()
	out := n
	if a != nil {
		if marks, grew := a.merged(n.marks); grew {
			nn := n.clone()
			nn.marks = marks
			out = nn
		}
	}
	// What the values within n are given: n's own deep marks, then those n
	// was given, one layer that they all share.
	below := a
	if deep := deepMarks(own); deep != nil {
		var outer *markSet
		if a != nil {
			outer = a.layer
		}
		below = newAttachment(deep, outer)
	}
	if n.state != stateKnown || n.typ.t.kind == KindSet || below == nil && !n.markedWithin {
		// A set keeps its deep marks, and Elements gives them to a member as
		// it returns it. With no deep mark to give, only a value holding a
		// marked value can hold one that has deep marks of its own to give.
		return out
	}
	if data := below.replacedMembers(n, true); data != nil {
		if out == n {
			out = n.clone()
		}
		out.data, out.markedWithin = data, true
	}
	return out
}

// retrievedMembers returns the members of a known set as a caller retrieves
// them: in a new slice, each carrying the set's deep marks, which the set
// keeps on itself because its members carry no marks in storage.
func (n *node) retrievedMembers() []Value {
	members := slices.Clone(n.data.([]Value))
	if deep := deepMarks(n.markList()); deep != nil {
		a := newAttachment(deep, nil)
		for i, m := range members {
			members[i] = Value{n: a.attach(m.n)}
		}
	}
	return members
}

// sortMarks sorts marks by identifier, leaving marks that share an identifier
// in the order they had.
func sortMarks(ms []Mark) {
	slices.SortStableFunc(ms, func(a, b Mark) int {
		return strings.Compare(a.MarkID(), b.MarkID())
	})
}

// Unmark returns v without the marks it carries, and those marks, sorted by
// identifier. The values v holds keep their own marks, which UnmarkDeep takes
// too. Those include a deep mark attached to v, which the values within v
// carry in their own right, but not a deep mark attached to a set, which its
// members carry only as Elements returns them. A value that carries no mark
// comes back as itself, with no marks.
func Unmark(v Value) (Value, []Mark) {
	n := v.data()
	if n.marks == nil {
		return v, nil
	}
	nn := n.clone()
	nn.marks = nil
	return Value{n: nn}, slices.Clone(n.marks.all())
}

// UnmarkDeep returns v without a mark anywhere in it: without its own marks,
// and with every value it holds, at any depth, unmarked too. It returns the
// marks it took, each once, sorted by identifier. A value that carries no mark
// and holds none comes back as itself, with no marks.
//
// A marked value, one that carries a mark or holds one, has no hash, no place
// in the canonical order and no place in a set. UnmarkDeep is the first half of
// what to do instead; the second is reapplying the marks it returns, which are
// the caller's to place. Set shows the usual place: the set.
func UnmarkDeep(v Value) (Value, []Mark) {
	n := v.data()
	if !n.isMarked() {
		return v, nil
	}
	var t taking
	u := n.unmarkDeep(&t)
	sortMarks(t.marks)
	return Value{n: u}, t.marks
}

// taking gathers the marks UnmarkDeep takes, each once, looked up through a
// markLookup, and each shared layer once however many values share it: a
// document whose members carry marks of their own beside k deep marks gives
// every member the same layer of k.
type taking struct {
	marks  []Mark
	seen   markLookup
	layers map[*markSet]bool
	// keep, where set, says which marks to take; the others are passed over.
	keep func(Mark) bool
}

// add takes the marks of s, layer by layer, stopping at a layer taken already,
// whose outer layers were taken with it.
func (t *taking) add(s *markSet) {
	for ; s != nil; s = s.outer {
		if s.layer {
			if t.layers[s] {
				return
			}
			if t.layers == nil {
				t.layers = map[*markSet]bool{}
			}
			t.layers[s] = true
		}
		for _, m := range s.list {
			if (t.keep == nil || t.keep(m)) && !t.seen.holds(t.marks, m) {
				t.marks = append(t.marks, m)
			}
		}
	}
}

// unmarkDeep returns n with no mark at any depth, adding each mark it takes to
// t. Whatever holds no mark is shared rather than copied.
func (n *node) unmarkDeep(t *taking) *node {
	if !n.isMarked() {
		return n
	}
	t.add(n.marks)
	nn := n.clone()
	nn.marks = nil
	if n.markedWithin {
		nn.markedWithin = false
		switch data := n.data.(type) {
		case []Value:
			members := make([]Value, len(data))
			for i, m := range data {
				members[i] = Value{n: m.n.unmarkDeep(t)}
			}
			nn.data = members
		case []mapEntry:
			entries := make([]mapEntry, len(data))
			for i, e := range data {
				entries[i] = mapEntry{key: e.key, val: Value{n: e.val.n.unmarkDeep(t)}}
			}
			nn.data = entries
		}
	}
	return nn
}

// isMarked reports whether n carries a mark or holds, at any depth, a value
// that does. Hashing, the canonical order and set membership are defined only
// for values that are not marked.
func (n *node) isMarked() bool { return n.marks != nil || n.markedWithin }

// describeMarked names a marked value and says where its marks are, for a
// panic message, as in "a value of type number that carries marks" or "a value
// of type list(number) that holds a marked value at [0]".
func (n *node) describeMarked() string {
	if n.marks != nil {
		return n.describe() + " that carries marks"
	}
	return n.describe() + " that holds a marked value at " + n.markPath().String()
}

// markPath returns the path from n to the first value within it that carries
// a mark, taking members in the order they are held. n must hold one.
func (n *node) markPath() Path {
	var p Path
	for n.marks == nil {
		step, member := n.markedMember()
		p, n = p.extend(step), member
	}
	return p
}

// markedMember returns the first member of n that is marked, and the step
// that locates it. n must hold one.
func (n *node) markedMember() (Step, *node) {
	switch data := n.data.(type) {
	case []mapEntry:
		for _, e := range data {
			if e.val.n.isMarked() {
				return indexStep(String(e.key)), e.val.n
			}
		}
	case []Value:
		for i, m := range data {
			if !m.n.isMarked() {
				continue
			}
			if n.typ.t.kind == KindObject {
				return attributeStep(n.typ.t.attrs[i].name), m.n
			}
			return indexStep(NumberFromInt(int64(i))), m.n
		}
	}
	internalPanic("%s says it holds a marked member and holds none", n.describe())
	return Step{}, nil
}

// HasMark reports whether v carries the mark. It panics if the mark is nil,
// which no value carries, whether or not v carries marks.
func HasMark(v Value, m Mark) bool {
	n := v.data()
	if m == nil {
		usagePanic("HasMark called with a nil Mark")
	}
	return n.marks.contains(m)
}

// propagated returns the marks that the result of the operation over these
// operands carries: the union of the operands' Propagate marks, and of the
// Propagate marks of the values within an operand that the operation reads,
// which it consumes along with the operand.
func (o *op) propagated(args []Value) []Mark {
	var g propagating
	for i, a := range args {
		g.gather(a.data(), o.operands[i].within)
	}
	return g.marks
}

// propagates reports whether m reaches what is derived from the value it is
// on: a mark whose policy is Propagate does, and so does a redacting mark,
// whatever its policy, since what it withholds must not show in anything
// derived from the value (MK-002, MK-011). An Isolate redacting mark
// propagates as any redacting mark does.
func propagates(m Mark) bool { return m.Propagation() == Propagate || m.Redacting() }

// propagating gathers the Propagate marks of what is consumed, each once, in
// the order they are met: the operands of an operation and what it reads
// within them, the error members of a container, the bounds of a narrowing. A
// mark is looked for among those gathered by markLookup, by a scan while there
// are few and through a set once there are many: a value can carry thousands,
// and scanning for each would cost the square of them.
type propagating struct {
	marks  []Mark
	seen   markLookup
	layers map[*markSet]bool // the shared layers gathered already
}

// addSet gathers the Propagate marks of s, layer by layer, and each shared
// layer once however many of the values consumed share it.
func (g *propagating) addSet(s *markSet) {
	for ; s != nil; s = s.outer {
		if s.layer {
			if g.layers[s] {
				return
			}
			if g.layers == nil {
				g.layers = map[*markSet]bool{}
			}
			g.layers[s] = true
		}
		g.add(s.list)
	}
}

// add gathers the Propagate marks among ms not gathered already. It makes
// room for all of them at once, which a value carrying thousands needs: grown
// a mark at a time, the list would be made again and again.
func (g *propagating) add(ms []Mark) {
	g.marks = slices.Grow(g.marks, len(ms))
	for _, m := range ms {
		if propagates(m) && !g.seen.holds(g.marks, m) {
			g.marks = append(g.marks, m)
		}
	}
}

// gather gathers the marks of n, and where within says that what is consumed
// reads the values within n, theirs as well, at any depth.
func (g *propagating) gather(n *node, within bool) {
	g.addSet(n.marks)
	if !within || !n.markedWithin {
		return
	}
	switch data := n.data.(type) {
	case []Value:
		for _, member := range data {
			g.gather(member.n, true)
		}
	case []mapEntry:
		for _, e := range data {
			g.gather(e.val.n, true)
		}
	}
}

// carryMarks returns r carrying every mark of v. A narrowing or a resolution
// refines the value it was given rather than deriving a new one, so the
// marks stay, the Isolate ones included, whether the result is a value or an
// error.
func carryMarks(v, r Value) Value {
	if v.n.marks == nil || r.n == v.n {
		return r
	}
	return WithMarks(r, v.n.marks.all()...)
}

// impliedMarks is what a container carrying deep marks implies on the values
// it holds: they carry those marks because the container does, so [SE-031]
// does not list them again, nor does a display form (DI-015). The marks a value lists for itself follow from
// the mark set it holds, so they are decided once per set rather than once
// per value: a container's members commonly share one set, the one the deep
// marks were attached to them through.
type impliedMarks struct {
	deep   map[Mark]bool       // the container's deep marks
	own    map[*markSet][]Mark // what a value holding that set lists for itself
	layers map[*markSet]bool   // whether the container implies every mark of an outer layer
}

// impliesLayer reports whether the container implies every mark of the outer
// layer l and the layers beyond it, asked once per layer: the members of a
// container share the layer of the deep marks they inherit from it.
func (im *impliedMarks) impliesLayer(l *markSet) bool {
	if implied, ok := im.layers[l]; ok {
		return implied
	}
	implied := true
	for _, m := range l.all() {
		if !im.deep[m] {
			implied = false
			break
		}
	}
	if im.layers == nil {
		im.layers = map[*markSet]bool{}
	}
	im.layers[l] = implied
	return implied
}

// listed returns the marks a value holding held lists for itself, which are
// those the container does not imply on it. The result is read, never
// appended to: it is often held's own list, which the mark set shares.
func (im *impliedMarks) listed(held *markSet) []Mark {
	if own, ok := im.own[held]; ok {
		return own
	}
	// Where the container implies every mark the set's outer layers hold, as
	// it does of the layer its members inherit from it, only the set's own
	// layer lists anything.
	list := held.list
	if held.outer != nil && !im.impliesLayer(held.outer) {
		list = held.all()
	}
	own := list
	for i, m := range list {
		if !im.deep[m] {
			continue
		}
		own = slices.Clone(list[:i:i])
		for _, m := range list[i+1:] {
			if !im.deep[m] {
				own = append(own, m)
			}
		}
		break
	}
	if im.own == nil {
		im.own = map[*markSet][]Mark{}
	}
	im.own[held] = own
	return own
}

// implications holds what the mark sets met on containers imply on the values
// they hold, for writing each deep mark once: the encoding (SE-031) and the
// display form (DI-015) both list a container's deep marks on the container
// alone. Containers that carry the same marks share one answer.
type implications struct {
	implied map[*markSet]*impliedMarks
	// deepMarks holds whether each mark met is deep, since a mark declares
	// that once and for all and the marks of an enclosing value are in the
	// mark set of every value under it.
	deepMarks map[Mark]bool
}

// deep reports whether m is a deep mark, asking the mark once however many
// mark sets hold it: a value nested deeply carries a set at every level, and
// the marks of the levels above are in each of them.
func (c *implications) deep(m Mark) bool {
	if kept, asked := c.deepMarks[m]; asked {
		return kept
	}
	if c.deepMarks == nil {
		c.deepMarks = map[Mark]bool{}
	}
	d := isDeep(m)
	c.deepMarks[m] = d
	return d
}

// implies returns what a container carrying the marks in ms implies on the
// values it holds, or nil where it implies nothing. Containers that carry the
// same marks share one answer, which holds what the values under them list.
func (c *implications) implies(ms *markSet) *impliedMarks {
	if ms == nil {
		return nil
	}
	if im, ok := c.implied[ms]; ok {
		return im
	}
	var im *impliedMarks
	for _, m := range ms.all() {
		if !c.deep(m) {
			continue
		}
		if im == nil {
			im = &impliedMarks{deep: map[Mark]bool{}}
		}
		im.deep[m] = true
	}
	if c.implied == nil {
		c.implied = map[*markSet]*impliedMarks{}
	}
	c.implied[ms] = im
	return im
}
