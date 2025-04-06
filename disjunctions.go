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
		listLen, err := args[2].Len().Int64()
		if err != nil {
			break
		}
		n, err := args[1].Int64()
		if err == nil && (n == 0 || n == listLen) {
			// Exclude not and allOf
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

// mergeCompatible returns the given arms with all members
// that are "compatible" for data-representation purposes
// merged into a single value.
//
// Two arms a and b are considered compatible if one of the following
// definition recursively applies:
// - they both have identical atomc kind sets.
// - they are both lists where all members are compatible in each arm
// - they are both structs where any field in a either does not
// exist in b or is compable with the same field in b
//
// To avoid complicating the algoirthm, if there are multiple list
// or struct types, they must all be compatible or they're all
// left distinct.
//
// // TODO we _could_ do a better job and merge together
// only those structs that _are_ compatible, but that's harder.
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
//
// TODO when values have been merged, it would be nice
// to find out what the resulting data structure looks like.
func mergeCompatible(arms []cue.Value) ([]cue.Value, func(int) IntSet) {
	//	log.Printf("mergeCompatible %d{ ", len(arms))
	//	for i, v := range arms {
	//		log.Printf("\t%d. (%v)", i, v)
	//	}
	//	log.Printf("}")
	byKind := make(map[cue.Kind]mapSet[int])
	composites := make(map[cue.Kind][]cue.Value)
	for i, arm := range arms {
		k := arm.IncompleteKind()
		if allAtomsKind(k) {
			if byKind[k] == nil {
				byKind[k] = make(mapSet[int])
			}
			byKind[k][i] = true
		} else if k == cue.StructKind || k == cue.ListKind {
			composites[k] = append(composites[k], arm)
		}
	}
	for k, vs := range composites {
		if !compatible(vs) {
			continue
		}
		from := make(mapSet[int])
		for i, arm := range arms {
			if arm.Kind() == k {
				from[i] = true
			}
		}
		byKind[k] = from
	}
	// Build the final list by taking the first item
	// from any of the sets of compatible structs.
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
func compatible(arms []cue.Value) (_ok bool) {
	//	log.Printf("compatible (")
	//	for i, v := range arms {
	//		log.Printf("\t%d. (%v)", i, v)
	//	}
	//	log.Printf(") {")
	//	defer func() {
	//		log.Printf("} -> %v", _ok)
	//	}()
	if len(arms) <= 1 {
		return true
	}
	if !compatibleKinds(arms) {
		return false
	}
	switch k := arms[0].IncompleteKind(); k {
	case cue.StructKind:
		// We know that all arms are structs.
		for _, vals := range allFields(arms, intSetN(len(arms)), requiredLabel|optionalLabel|regularLabel) {
			if !compatibleKinds(vals) {
				return false
			}
		}
		return true
	case cue.ListKind:
		// We know that all arms are lists.
		types := make([]listType, len(arms))
		longest := 0
		for i, v := range arms {
			t, err := listTypeForValue(v)
			if err != nil {
				panic(fmt.Errorf("unexpected error getting list type: %v", err))
			}
			longest = max(longest, t.checkLen())
			types[i] = t
		}
		listArms := make([]cue.Value, len(arms))
		for i := range longest {
			for j, t := range types {
				listArms[j] = t.index(i)
			}
			if !compatible(listArms) {
				return false
			}
		}
		return true
	}
	return true
}

type listType struct {
	elems []cue.Value
	rest  cue.Value
}

func (t listType) checkLen() int {
	n := len(t.elems)
	if t.rest.Exists() {
		n++
	}
	return n
}

func (t listType) index(i int) cue.Value {
	if i < len(t.elems) {
		return t.elems[i]
	}
	if t.rest.Exists() {
		return t.rest
	}
	return cue.Value{}
}

func listTypeForValue(v cue.Value) (listType, error) {
	if v.Kind() != cue.ListKind {
		return listType{}, fmt.Errorf("listTypes called on non-list %v", v)
	}
	rest := v.LookupPath(cue.MakePath(cue.AnyIndex))
	lenv := v.Len()
	var n int64
	if rest.Exists() {
		// The length will be in the form int&>=5
		op, args := lenv.Expr()
		if op != cue.AndOp || len(args) != 2 {
			return listType{}, fmt.Errorf("list length has unexpected form; got %v want int&>=N", lenv)
		}
		op, args = args[1].Expr()
		if op != cue.GreaterThanEqualOp || len(args) != 1 {
			return listType{}, fmt.Errorf("list length has unexpected form (2); got %v want >=N", lenv)
		}
		var err error
		n, err = args[0].Int64()
		if err != nil {
			return listType{}, fmt.Errorf("cannot extract list length from %v: %v", v, err)
		}
	} else {
		var err error
		n, err = lenv.Int64()
		if err != nil {
			return listType{}, fmt.Errorf("cannot extract concrete list length from %v: %v", v, err)
		}
	}
	elems := make([]cue.Value, n)
	for i := range n {
		elems[i] = v.LookupPath(cue.MakePath(cue.Index(i)))
		if !elems[i].Exists() {
			return listType{}, fmt.Errorf("cannot get value at index %d in %v", i, v)
		}
	}
	return listType{
		elems: elems,
		rest:  rest,
	}, nil
}

func compatibleKinds(arms []cue.Value) bool {
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
		if vk != k {
			return false
		}
	}
	return true
}
