package cuediscrim

import (
	"fmt"
	"iter"
	"strings"
)

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

func stringerIter[T fmt.Stringer](it iter.Seq[T]) iter.Seq[string] {
	return func(yield func(string) bool) {
		for s := range it {
			if !yield(s.String()) {
				return
			}
		}
	}
}

func joinSeq(it iter.Seq[string], sep string) string {
	var buf strings.Builder
	first := true
	for s := range it {
		if !first {
			buf.WriteString(sep)
		} else {
			first = false
		}
		buf.WriteString(s)
	}
	return buf.String()
}

func mapHasKey[Map ~map[K]V, K comparable, V any](m Map, k K) bool {
	_, ok := m[k]
	return ok
}

func iterConcat[T any](iters ...iter.Seq[T]) iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, it := range iters {
			for x := range it {
				if !yield(x) {
					return
				}
			}
		}
	}
}
