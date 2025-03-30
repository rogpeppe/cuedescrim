package cuediscrim

import (
	"cmp"
	"fmt"
	"iter"
	"maps"
	"slices"
	"strings"

	"cuelang.org/go/cue"
)

type intSet = set[int]

// valueSetForValue returns a discrimination set for the value v.
func valueSetForValue(v cue.Value) valueSet {
	if v.IncompleteKind() == cue.NullKind {
		// Special case: if the kind is null, treat it
		// as a type rather than an atom so that
		// type-based discrimination will be used by preference.
		return valueSet{
			types: cue.NullKind,
		}
	}
	if s := atomForValue(v); s != "" {
		return valueSet{
			consts: set[atom]{s: true},
		}
	}
	op, args := v.Expr()
	if op != cue.OrOp {
		return valueSet{
			types: v.IncompleteKind(),
		}
	}
	s := valueSetForValue(args[0])
	for _, arg := range args[1:] {
		s = s.union(valueSetForValue(arg))
	}
	return s
}

var allKinds = []cue.Kind{
	cue.BottomKind, // Note: bottom is used to represent a missing field.
	cue.NullKind,
	cue.BoolKind,
	cue.IntKind,
	cue.FloatKind,
	cue.StringKind,
	cue.BytesKind,
	cue.ListKind,
	cue.StructKind,
}

// valueSet holds a set of possible discriminating values for a field.
// The actual CUE that it represents can be considered to be
// a disjunction of two disjunctions:
//
//	(type0 | type1 | ...) | (const0 | const1 | ...)
//
// It's kept normalized such that there are no members
// of consts that overlap with members of types.
type valueSet struct {
	// types holds the set of possible types the value can take.
	types cue.Kind
	// consts holds the set of possible const expressions that the value can take.
	// If a member is also a member of Types, it's redundant.
	consts set[atom]
}

func (s valueSet) String() string {
	var buf strings.Builder
	buf.WriteString("(")
	add := func(s string) {
		if buf.Len() > 1 {
			buf.WriteString(" | ")
		}
		buf.WriteString(s)
	}
	for _, k := range allKinds {
		if (s.types & k) != 0 {
			add(k.String())
		}
	}
	for _, c := range slices.Sorted(maps.Keys(s.consts)) {
		add(string(c))
	}
	buf.WriteString(")")
	return buf.String()
}

func (s0 valueSet) intersect(s1 valueSet) valueSet {
	// By the usual CUE algebra:
	// (s0.types | s0.consts) & (s1.types | s1.consts)
	// =>
	// s0.types & s1.types) |
	//	(s0.types & s1.consts) |
	//	(s1.types & s0.consts) |
	//	(s0.consts & s1.consts)

	s2 := valueSet{
		types:  s0.types & s1.types,
		consts: make(set[atom]),
	}
	for c := range s1.consts {
		if (s0.types & c.kind()) != 0 {
			s2.consts[c] = true
		}
	}
	for c := range s0.consts {
		if (s1.types & c.kind()) != 0 {
			s2.consts[c] = true
		}
	}
	for c := range s0.consts {
		if s1.consts[c] {
			s2.consts[c] = true
		}
	}
	return s2.normalize()
}

func (s0 valueSet) holdsAtom(v atom) bool {
	if (s0.types & v.kind()) != 0 {
		return true
	}
	return s0.consts[v]
}

// kinds returns all the possible kinds for values in the set.
func (s valueSet) kinds() cue.Kind {
	k := s.types
	for c := range s.consts {
		k |= c.kind()
	}
	return k
}

func (s0 valueSet) without(s1 valueSet) valueSet {
	s2 := valueSet{
		types:  s0.types &^ s1.types,
		consts: maps.Clone(s0.consts),
	}
	for c := range s2.consts {
		if s1.holdsAtom(c) {
			delete(s2.consts, c)
		}
	}
	return s2.normalize()
}

func (s0 valueSet) union(s1 valueSet) valueSet {
	return valueSet{
		types:  s0.types | s1.types,
		consts: s0.consts.union(s1.consts),
	}.normalize()
}

func (s valueSet) isEmpty() bool {
	return s.types == 0 && len(s.consts) == 0
}

func (s valueSet) normalize() valueSet {
	getm := copyMap(&s.consts)
	for c := range s.consts {
		if (s.types & c.kind()) != 0 {
			delete(getm(), c)
		}
	}
	if len(s.consts) == 0 {
		s.consts = nil
	}
	return s
}

type set[T comparable] map[T]bool

func setString[T cmp.Ordered](s set[T]) string {
	var buf strings.Builder
	buf.WriteString("{")
	first := true
	for _, x := range slices.Sorted(maps.Keys(s)) {
		if !first {
			buf.WriteString(", ")
		}
		fmt.Fprintf(&buf, "%#v", x)
		first = false
	}
	buf.WriteString("}")
	return buf.String()
}

func (m0 set[T]) union(m1 set[T]) set[T] {
	if len(m0) == 0 {
		return m1
	}
	if len(m1) == 0 {
		return m0
	}
	m2 := maps.Clone(m0)
	maps.Copy(m2, m1)
	return m2
}

func (m0 set[T]) Equal(m1 set[T]) bool {
	return maps.Equal(m0, m1)
}

func (m0 set[T]) intersect(m1 set[T]) set[T] {
	if len(m0) == 0 {
		return m0
	}
	if len(m1) == 0 {
		return m1
	}
	var m2 map[T]bool
	getm := copyMap(&m2)
	for x := range m0 {
		if m1[x] {
			getm()[x] = true
		}
	}
	return m2
}

func (m0 set[T]) without(m1 set[T]) set[T] {
	if len(m1) == 0 {
		return m0
	}
	m2 := maps.Clone(m0)
	for x := range m1 {
		delete(m2, x)
	}
	return m2
}

func copyMap[Map ~map[K]V, K comparable, V any](m *Map) func() Map {
	written := false
	return func() Map {
		if written {
			return *m
		}
		if *m == nil {
			*m = make(Map)
		} else {
			*m = maps.Clone(*m)
		}
		written = true
		return *m
	}
}

type atom string

func (s atom) kind() cue.Kind {
	if s == "" {
		return 0
	}
	switch s[0] {
	case '"':
		return cue.StringKind
	case '\'':
		return cue.BytesKind
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '.':
		return cue.NumberKind
	case 'n':
		return cue.NullKind
	case 'f', 't':
		return cue.BoolKind
	}
	panic(fmt.Errorf("unknown kind for atom %q", s))
}

func atomForValue(v cue.Value) atom {
	if !isAtomKind(v.IncompleteKind()) || v.Validate(cue.Concrete(true)) != nil {
		return ""
	}
	// TODO it's probably not guaranteed that the value is actually canonical.
	// For example, a string might be represented differently depending
	// on its representation in the original source. We should make
	// sure it's canonical.
	return atom(fmt.Sprint(v))
}

func isAtomKind(k cue.Kind) bool {
	switch k {
	case cue.NullKind,
		cue.BoolKind,
		cue.IntKind,
		cue.FloatKind,
		cue.StringKind,
		cue.BytesKind,
		cue.NumberKind:
		return true
	}
	return false
}

func fold[T any](it iter.Seq[T], op func(T, T) T) T {
	first := true
	var tot T
	for x := range it {
		if first {
			tot = x
			first = false
		} else {
			tot = op(tot, x)
		}
	}
	return tot
}

func iterMap[T1, T2 any](it iter.Seq[T1], f func(T1) T2) iter.Seq[T2] {
	return func(yield func(T2) bool) {
		for t := range it {
			if !yield(f(t)) {
				return
			}
		}
	}
}
