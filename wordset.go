package cuediscrim

import (
	"fmt"
	"iter"
	"math/bits"
)

type wordSetAPI struct{}

// check it implements setAPI.
var _ setAPI[wordSet, int] = wordSetAPI{}

func (wordSetAPI) clone(x wordSet) wordSet {
	return x
}

func (wordSetAPI) make() wordSet {
	return 0
}

func (wordSetAPI) of(xs ...int) wordSet {
	var s wordSet
	for _, x := range xs {
		checkWord(x)
		s |= 1 << x
	}
	return s
}
func (wordSetAPI) has(s wordSet, x int) bool {
	return s.Has(x)
}

func (wordSetAPI) values(s wordSet) iter.Seq[int] {
	return s.Values()
}

func (wordSetAPI) union(s1, s2 wordSet) wordSet {
	return s1.union(s2)
}

func (wordSetAPI) intersect(s1, s2 wordSet) wordSet {
	return s1.intersect(s2)
}

func (wordSetAPI) add(s *wordSet, x int) {
	s.add(x)
}

func (wordSetAPI) delete(s *wordSet, x int) {
	s.delete(x)
}

func (wordSetAPI) len(s wordSet) int {
	return s.len()
}

func (wordSetAPI) equal(s1, s2 wordSet) bool {
	return s1 == s2
}

func (wordSetAPI) asSet(s wordSet) Set[int] {
	return s
}

func wordSetN(n int) wordSet {
	checkWord(n - 1)
	return wordSet((1 << n) - 1)
}

func checkWord(x int) {
	if x < 0 || x >= 64 {
		panic(fmt.Errorf("cannot store out-of-bounds value %d in word set", x))
	}
}

type wordSet uint64

// Len implements Set.Len.
func (s wordSet) Len() int {
	return bits.OnesCount64(uint64(s))
}

// Has implements Set.Has.
func (s wordSet) Has(x int) bool {
	return (s & (1 << x)) != 0
}

// Values implements Set.Values.
func (s wordSet) Values() iter.Seq[int] {
	return func(yield func(int) bool) {
		for s != 0 {
			n := bits.TrailingZeros64(uint64(s))
			if !yield(n) {
				return
			}
			s &^= (1 << (n + 1)) - 1
		}
	}
}

func (s0 wordSet) union(s1 wordSet) wordSet {
	return s0 | s1
}

func (s0 wordSet) intersect(s1 wordSet) wordSet {
	return s0 & s1
}

func (s *wordSet) add(x int) {
	checkWord(x)
	*s |= 1 << x
}

func (s *wordSet) delete(x int) {
	*s &^= 1 << x
}

func (s wordSet) len() int {
	return bits.OnesCount64(uint64(s))
}
