package cuediscrim

import (
	"fmt"

	"cuelang.org/go/cue"
)

// Disjunctions splits v into its component disjunctions,
// including disjunctions in subexpressions.
// Any matchN operator with an argument of 1 also counts as a disjunction.
func Disjunctions(v cue.Value) []cue.Value {
	return appendDisjunctions(nil, v)
}

func appendDisjunctions(dst []cue.Value, v cue.Value) []cue.Value {
	op, args := v.Eval().Expr()
	switch op {
	case cue.OrOp:
		for _, v := range args {
			dst = appendDisjunctions(dst, v)
		}
		return dst
	case cue.CallOp:
		if fmt.Sprint(args[0]) != "matchN" {
			break
		}
		if n, _ := args[1].Int64(); n != 1 {
			break
		}
		iter, err := args[2].List()
		if err != nil {
			break
		}
		for iter.Next() {
			dst = appendDisjunctions(dst, iter.Value())
		}
		return dst
	}
	return append(dst, v)
}

// mergeAtoms returns the given arms with all members
// that are atom types merged into a single entry
// for each type.
// It also returns a function that can be used to map indexes in the
// returned slice to indexes (possibly multiple) in the original.
//
// This makes it possible to use [Discriminate] to do better
// when there's an imperfect discriminator involving,
// for example, a set of strings and regexps, e.g.
//
//	"a" | "b" | =~"^\d+$"
func mergeAtoms(arms []cue.Value) ([]cue.Value, func(int) IntSet) {
	byKind := make(map[cue.Kind]mapSet[int])
	for i, arm := range arms {
		if k := arm.IncompleteKind(); isAtomKind(k) {
			if byKind[k] == nil {
				byKind[k] = make(mapSet[int])
			}
			byKind[k][i] = true
		}
	}
	done := make(mapSet[cue.Kind])
	arms1 := make([]cue.Value, 0, len(arms))
	revMap := make([]mapSet[int], 0, len(arms))
	for i, arm := range arms {
		k := arm.IncompleteKind()
		from := byKind[k]
		if len(from) == 1 || !isAtomKind(k) || !done[k] {
			if len(from) == 0 {
				// It's a struct.
				from = mapSet[int]{i: true}
			}
			arms1 = append(arms1, arm)
			revMap = append(revMap, from)
			done[k] = true
		}
	}
	return arms1, func(i int) IntSet {
		if i < 0 || i >= len(revMap) {
			return mapSet[int](nil)
		}
		return revMap[i]
	}
}
