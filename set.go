package cuediscrim

import (
	"cmp"
	"fmt"
	"iter"
	"slices"
	"strings"
)

type setAPI[S any, T comparable] interface {
	clone(S) S
	make() S
	of(...T) S
	has(S, T) bool
	values(S) iter.Seq[T]
	union(S, S) S
	intersect(S, S) S
	add(*S, T)
	delete(*S, T)
	len(S) int
	equal(S, S) bool
	asSet(S) Set[T]
}

// Set holds a set of distinct values.
type Set[T comparable] interface {
	Values() iter.Seq[T]
	Has(T) bool
	Len() int
}

// IntSet is used to hold a set of possible discrimination choices.
type IntSet = Set[int]

func union[T comparable](s1, s2 Set[T]) Set[T] {
	if s1.Len() == 0 {
		return s2
	}
	if s2.Len() == 0 {
		return s1
	}
	s1m, ok1 := s1.(mapSet[T])
	s2m, ok2 := s2.(mapSet[T])
	if ok1 && ok2 {
		return s1m.union(s2m)
	}
	var m mapSet[T]
	m.addSeq(s1.Values())
	m.addSeq(s2.Values())
	return m
}

func intersect[T comparable](s0, s1 Set[T]) Set[T] {
	if s0.Len() == 0 {
		return s1
	}
	if s1.Len() == 0 {
		return s1
	}
	s2 := make(mapSet[T])
	for x := range s0.Values() {
		if s1.Has(x) {
			s2[x] = true
		}
	}
	return s2
}

type singleInt int

func (i singleInt) Values() iter.Seq[int] {
	return func(yield func(int) bool) {
		yield(int(i))
	}
}

func (i singleInt) Has(x int) bool {
	return int(i) == x
}

func (i singleInt) Len() int {
	return 1
}

func SetString[T cmp.Ordered](s Set[T]) string {
	var buf strings.Builder
	buf.WriteString("{")
	first := true
	for _, x := range slices.Sorted(s.Values()) {
		if !first {
			buf.WriteString(", ")
		}
		fmt.Fprintf(&buf, "%#v", x)
		first = false
	}
	buf.WriteString("}")
	return buf.String()
}

func revSet[T comparable](s Set[T], rev func(T) Set[T]) Set[T] {
	if rev == nil {
		return s
	}
	return &revSetImpl[T]{
		orig: s,
		rev:  rev,
	}
}

type revSetImpl[T comparable] struct {
	orig Set[T]
	new  mapSet[T]
	rev  func(T) Set[T]
}

func (s *revSetImpl[T]) Values() iter.Seq[T] {
	return func(yield func(T) bool) {
		for x := range s.orig.Values() {
			for y := range s.rev(x).Values() {
				if !yield(y) {
					return
				}
			}
		}
	}
}

func (s *revSetImpl[T]) Has(x T) bool {
	s.init()
	return s.new.Has(x)
}

func (s *revSetImpl[T]) Len() int {
	s.init()
	return len(s.new)
}

func (s *revSetImpl[T]) init() {
	if s.new == nil {
		s.new = mapSetOf(s.orig.Values())
	}
}
