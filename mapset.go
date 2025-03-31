package cuediscrim

import (
	"iter"
	"maps"
	"slices"
)

func intSetN(n int) mapSet[int] {
	if n == 0 {
		return nil
	}
	s := make(mapSet[int])
	for i := range n {
		s[i] = true
	}
	return s
}

type mapSetAPI[T comparable] struct{}

// check it implements setAPI.
var _ setAPI[mapSet[int], int] = mapSetAPI[int]{}

func (mapSetAPI[T]) make() mapSet[T] {
	return make(mapSet[T])
}

func (mapSetAPI[T]) clone(s mapSet[T]) mapSet[T] {
	return maps.Clone(s)
}

func (mapSetAPI[T]) asSet(s mapSet[T]) Set[T] {
	return s
}

func (mapSetAPI[T]) of(xs ...T) mapSet[T] {
	return mapSetOf(slices.Values(xs))
}

func (mapSetAPI[T]) has(s mapSet[T], x T) bool {
	return s[x]
}

func (mapSetAPI[T]) union(s1, s2 mapSet[T]) mapSet[T] {
	return s1.union(s2)
}

func (mapSetAPI[T]) intersect(s1, s2 mapSet[T]) mapSet[T] {
	return s1.intersect(s2)
}

func (mapSetAPI[T]) add(s *mapSet[T], x T) {
	if *s == nil {
		*s = make(mapSet[T])
	}
	(*s)[x] = true
}

func (mapSetAPI[T]) delete(s *mapSet[T], x T) {
	delete(*s, x)
}

func (mapSetAPI[T]) addSeq(s *mapSet[T], xs iter.Seq[T]) {
	s.addSeq(xs)
}

func (mapSetAPI[T]) values(s mapSet[T]) iter.Seq[T] {
	return maps.Keys(s)
}

func (mapSetAPI[T]) len(s mapSet[T]) int {
	return len(s)
}

func (mapSetAPI[T]) equal(s1, s2 mapSet[T]) bool {
	return maps.Equal(s1, s2)
}

func mapSetOf[T comparable](xs iter.Seq[T]) mapSet[T] {
	var s mapSet[T]
	for x := range xs {
		if s == nil {
			s = make(mapSet[T])
		}
		s[x] = true
	}
	return s
}

type mapSet[T comparable] map[T]bool

// Len implements Set.Len.
func (m mapSet[T]) Len() int {
	return len(m)
}

// Has implements Set.Has.
func (m mapSet[T]) Has(x T) bool {
	return m[x]
}

// Values implements Set.Values.
func (m mapSet[T]) Values() iter.Seq[T] {
	return maps.Keys(m)
}

func (s *mapSet[T]) addSeq(xs iter.Seq[T]) {
	if *s == nil {
		*s = make(mapSet[T])
	}
	for x := range xs {
		(*s)[x] = true
	}
}

func (m0 mapSet[T]) union(m1 mapSet[T]) mapSet[T] {
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

func (m0 mapSet[T]) Equal(m1 mapSet[T]) bool {
	return maps.Equal(m0, m1)
}

func (m0 mapSet[T]) intersect(m1 mapSet[T]) mapSet[T] {
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
