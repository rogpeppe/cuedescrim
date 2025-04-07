package cuediscrim

import (
	"fmt"
	"maps"
	"slices"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/token"
)

// DataTypeForValues returns a cue.Value that can be used to store
// instances of any of the given schemas.
// It is intended to be used on values that have been merged
// together as compatible.
func DataTypeForValues(arms []cue.Value) ast.Expr {
	if len(arms) == 0 {
		panic("no values")
	}
	k := cue.Kind(0)
	for _, arm := range arms {
		k |= arm.IncompleteKind()
	}
	if onesCount(k) > 1 {
		return syntaxForKind(k)
	}
	switch k {
	case cue.StructKind:
		return dataTypeForStruct(arms)
	case cue.ListKind:
		return dataTypeForList(arms)
	}
	return syntaxForKind(k)
}

func dataTypeForStruct(arms []cue.Value) ast.Expr {
	labelTypeOr := func(t1, t2 labelType) labelType {
		if t1 == t2 {
			return t1
		}
		return optionalLabel
	}
	type fieldInfo struct {
		labelType labelType
		values    []cue.Value
	}
	fields := make(map[string]*fieldInfo)
	for _, v := range arms {
		for label, fieldv := range structFields(v, requiredLabel|optionalLabel|regularLabel) {
			info := fields[label.name]
			if info == nil {
				info = &fieldInfo{
					labelType: label.labelType,
				}
				fields[label.name] = info
			}
			info.values = append(info.values, fieldv)
			info.labelType = labelTypeOr(label.labelType, info.labelType)
		}
	}
	lit := &ast.StructLit{}
	for _, name := range slices.Sorted(maps.Keys(fields)) {
		info := fields[name]
		f := &ast.Field{
			Label: &ast.Ident{
				Name: name,
			},
			Value: DataTypeForValues(info.values),
		}
		switch info.labelType {
		case optionalLabel:
			f.Constraint = token.OPTION
		case requiredLabel:
			f.Constraint = token.NOT
		}
		lit.Elts = append(lit.Elts, f)
	}
	return lit
}

func dataTypeForList(arms []cue.Value) ast.Expr {
	types, longest := listTypes(arms)
	hasEllipsis := false
	for _, t := range types {
		hasEllipsis = hasEllipsis || t.ellipsis.Exists()
	}
	lit := &ast.ListLit{
		Elts: make([]ast.Expr, longest+1),
	}
	for i := range longest + 1 {
		elem := DataTypeForValues(listValuesAt(types, i))
		if i < longest || !hasEllipsis {
			lit.Elts[i] = elem
		} else {
			lit.Elts[i] = &ast.Ellipsis{
				Type: elem,
			}
		}
	}
	return lit
}

var kindValues = map[cue.Kind]string{
	cue.NullKind:   "null",
	cue.BoolKind:   "bool",
	cue.IntKind:    "int",
	cue.FloatKind:  "float",
	cue.NumberKind: "number",
	cue.StringKind: "string",
	cue.BytesKind:  "bytes",
}

func syntaxForKind(k cue.Kind) ast.Expr {
	if (k & allKindsMask) == allKindsMask {
		return &ast.Ident{
			Name: "_",
		}
	}
	var args []ast.Expr
	for _, ak := range allKinds {
		if (k & ak) == 0 {
			continue
		}
		if ident, ok := kindValues[ak]; ok {
			args = append(args, &ast.Ident{Name: ident})
			continue
		}
		switch ak {
		case cue.StructKind:
			args = append(args, &ast.StructLit{
				Elts: []ast.Decl{
					&ast.Ellipsis{},
				},
			})
		case cue.ListKind:
			args = append(args, &ast.ListLit{
				Elts: []ast.Expr{
					&ast.Ellipsis{},
				},
			})
		default:
			// shouldn't happen?
			args = append(args, &ast.Ident{Name: "_"})
		}
	}
	return ast.NewBinExpr(token.OR, args...)
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
func mergeCompatible(arms []cue.Value) ([]cue.Value, func(int) IntSet) {
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
	case cue.ListKind:
		types, longest := listTypes(arms)
		for i := range longest {
			if !compatible(listValuesAt(types, i)) {
				return false
			}
		}
	}
	return true
}

func listValuesAt(types []listType, i int) []cue.Value {
	vs := make([]cue.Value, len(types))
	for j, t := range types {
		vs[j] = t.index(i)
	}
	return vs
}

type listType struct {
	elems    []cue.Value
	ellipsis cue.Value
}

func (t listType) checkLen() int {
	n := len(t.elems)
	if t.ellipsis.Exists() {
		n++
	}
	return n
}

func (t listType) index(i int) cue.Value {
	if i < len(t.elems) {
		return t.elems[i]
	}
	if t.ellipsis.Exists() {
		return t.ellipsis
	}
	return cue.Value{}
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

// listTypes returns the types of all the given list values,
// and also reports the maximum index that might differ
// across lists.
func listTypes(lists []cue.Value) ([]listType, int) {
	types := make([]listType, len(lists))
	longest := 0
	for i, v := range lists {
		t, err := listTypeForValue(v)
		if err != nil {
			panic(fmt.Errorf("unexpected error getting list type: %v", err))
		}
		longest = max(longest, t.checkLen())
		types[i] = t
	}
	return types, longest
}

func listTypeForValue(v cue.Value) (listType, error) {
	if v.Kind() != cue.ListKind {
		return listType{}, fmt.Errorf("listTypes called on non-list %v", v)
	}
	ellipsis := v.LookupPath(cue.MakePath(cue.AnyIndex))
	lenv := v.Len()
	var n int64
	if ellipsis.Exists() {
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
		elems:    elems,
		ellipsis: ellipsis,
	}, nil
}
