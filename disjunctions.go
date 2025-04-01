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
//
// It will also merge all struct types into a single type
// if they're compatible; if not, it will leave them
// all as-is.
// TODO we _could_ do a better job and merge together
// only those structs that _are_ compatible, but that's harder
//
// It also returns a function that can be used to map indexes in the
// returned slice to indexes (possibly multiple) in the original.
//
// This makes it possible to use [Discriminate] to do better
// when there's an imperfect discriminator involving,
// for example, a set of strings and regexps, e.g.
//
//	"a" | "b" | =~"^\d+$"
//
// Note that the result of this will contain an arbitrary value
// from the original slice for a set of merged items,
// but because we're only merging at the top level and
// the core discrimination algorithm will use
// type as a primary distinguishing feature, that won't
// make any different to the results.
func mergeCompatible(arms []cue.Value) ([]cue.Value, func(int) IntSet) {
	byKind := make(map[cue.Kind]mapSet[int])
	structs := make([]cue.Value, 0, len(arms))
	for i, arm := range arms {
		k := arm.IncompleteKind()
		if k == cue.StructKind {
			structs = append(structs, arm)
		}
		if isAtomKind(k) {
			if byKind[k] == nil {
				byKind[k] = make(mapSet[int])
			}
			byKind[k][i] = true
		}
	}
	if len(structs) > 1 && compatible(structs) {
		from := make(mapSet[int])
		for i, arm := range arms {
			if arm.Kind() == cue.StructKind {
				from[i] = true
			}
		}
		byKind[cue.StructKind] = from
	}
	done := make(mapSet[cue.Kind])
	arms1 := make([]cue.Value, 0, len(arms))
	revMap := make([]mapSet[int], 0, len(arms))
	for i, arm := range arms {
		k := arm.IncompleteKind()
		from := byKind[k]
		if len(from) <= 1 || !done[k] {
			if len(from) == 0 {
				// It's a non-mergeable item.
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

// compatible reports whether all the given values
// should be considered "compatible"; that is:
// - the kinds of each value should either be atoms and the same or
// - the kinds of each value should all be structs and every
// field should be compatible (recursively) across all structs
// where it's defined.
// TODO we should probably allow identical list types too.
func compatible(arms []cue.Value) bool {
	if !compatibleKinds(arms) {
		return false
	}
	if len(arms) > 0 && arms[0].Kind() != cue.StructKind {
		return true
	}
	if len(arms) <= 1 {
		return true
	}
	// We know that all arms are structs.
	for _, vals := range allFields(arms, intSetN(len(arms)), requiredLabel|optionalLabel|regularLabel) {
		if !compatibleKinds(vals) {
			return false
		}
	}
	return true
}

func compatibleKinds(arms []cue.Value) (_r bool) {
	if len(arms) <= 1 {
		return true
	}
	known := false
	var k cue.Kind
	for _, v := range arms {
		if !v.Exists() {
			continue
		}
		vk := v.IncompleteKind()
		if !known {
			k = vk
			known = true
			continue
		}
		if vk != k || !isAtomKind(vk) && vk != cue.StructKind {
			return false
		}
	}
	return true
}
