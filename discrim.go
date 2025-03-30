package cuediscrim

import (
	"fmt"
	"iter"
	"maps"
	"os"

	"cuelang.org/go/cue"
)

var logger *indentWriter

func init() {
	if false {
		logger = &indentWriter{
			w: os.Stderr,
		}
	}
}

// Discriminate returns a decision tree that can be used
// to decide which arm of a disjunction should be chosen.
// If v is not a disjunction, it returns a decision node that
// just selects v itself.
func Discriminate(v cue.Value) DecisionNode {
	op, args := v.Expr()
	switch op {
	case cue.OrOp:
	case cue.CallOp:
		if fmt.Sprint(args[0]) != "matchN" {
			break
		}
		if args[2].Kind() != cue.ListKind {
			break
		}
		var arms []cue.Value
		if err := args[2].Decode(&arms); err != nil {
			panic(err)
		}
		args = arms
	}
	if args == nil {
		args = []cue.Value{v}
	}
	return discriminate(args, intSetN(len(args)))
}

func discriminate(arms []cue.Value, selected intSet) (_n DecisionNode) {
	logger.Printf("discriminate %v {", setString(selected))
	logger.Indent()
	defer func() {
		logger.Printf("} -> %T", _n)
	}()
	defer logger.Unindent()
	if len(selected) <= 1 {
		// Nothing to disambiguate.
		return &LeafNode{
			Arms: selected,
		}
	}
	// First try to discriminate based on the top level value only.
	// We're happy just to make some progress, so we'll consider
	// it "fully discriminated" if all the non-struct elements
	// are discriminated, assuming there are such elements.
	// If there aren't then we require all elements to be discriminated.
	needDiscrim := make(intSet)
	for i, v := range arms {
		if (v.IncompleteKind() & cue.StructKind) == 0 {
			needDiscrim[i] = true
		}
	}
	if len(needDiscrim) == 0 {
		needDiscrim = selected
	}
	byValue, byKind, full := discriminators(arms, selected, needDiscrim)
	if full {
		return buildDecisionFromDescriminators(".", arms, selected, byValue, byKind)
	}
	// First try to find a single discriminator that can be used to do all discrimination.
	for path, values := range allRequiredFields(arms, selected) {
		logger.Printf("----- PATH %s", path)
		byValue, byKind, full := discriminators(values, selected, selected)
		if full {
			logger.Printf("fully discriminated")
		}
		logger.Printf("values:")
		for v, group := range byValue {
			logger.Printf("	%v: %v", v, setString(group))
		}
		logger.Printf("kinds:")
		for k, group := range byKind {
			logger.Printf("	%v: %v", k, setString(group))
		}
		if full {
			return buildDecisionFromDescriminators(path, values, selected, byValue, byKind)
		}

	}
	// TODO try to find a discriminator that can distinguish between
	// all the the non-struct values. We'll use
	logger.Printf("no multiple discriminators available yet")
	return ErrorNode{}
}

func buildDecisionFromDescriminators(path string, values []cue.Value, selected intSet, byValue map[atom]intSet, byKind map[cue.Kind]intSet) DecisionNode {
	var kindSwitch DecisionNode
	if len(byKind) == 0 {
		kindSwitch = ErrorNode{}
	} else {
		// First build the kind switch.
		n := &KindSwitchNode{
			Path:     path,
			Branches: make(map[cue.Kind]DecisionNode, len(byKind)),
		}
		for k, group := range byKind {
			logger.Printf("kind %v: %v", k, setString(group))
			var branch DecisionNode
			switch {
			case k == cue.StructKind && len(group) > 1:
				// We need to disambiguate a struct.
				branch = discriminate(values, group)
			case group.equal(selected):
				// We've got nothing more to base a decision on,
				// so terminate.
				branch = &LeafNode{
					Arms: selected,
				}
			default:
				branch = discriminate(values, group)
			}
			n.Branches[k] = branch
		}
		kindSwitch = n
	}
	if len(byValue) == 0 {
		return kindSwitch
	}
	valSwitch := &ValueSwitchNode{
		Path:     path,
		Branches: make(map[atom]DecisionNode, len(byValue)),
		Default:  kindSwitch,
	}
	for val, group := range byValue {
		var branch DecisionNode
		if group.equal(selected) {
			// We've got nothing more to base a decision on,
			// so terminate.
			branch = &LeafNode{
				Arms: selected,
			}
		} else {
			logger.Printf("valSwitch %v", val)
			branch = discriminate(values, group)
		}
		valSwitch.Branches[val] = branch
	}
	return valSwitch
}

func buildStructDecisionTree(arms []cue.Value, selected intSet) DecisionNode {
	//	fields := make(map[string][]cue.Value)
	//	for i, arm := range arms {
	//		for field, v := range allRequiredFields(arm) {
	//			entry, ok := fields[field]
	//			if !ok {
	//				entry = make([]cue.Value, len(arms))
	//				fields[field] = entry
	//			}
	//			entry[i] = v
	//		}
	//	}
	//if we can look up
	// Note: values that aren't present will hold the zero value
	// which works out OK because the kind of the zero value is bottom.
	return ErrorNode{} // TODO

}

//	how are we going to discriminate?
//
//		go through required fields in the selected arms breadth firsth
//	logger.Printf("buildStruct %v", setString(selected))
//	return &LeafNode{}
//
//}
//
//{a!: int} | {a!: string} | {b!: int}
//
//
//a: (int string _)
//
//a: {
//	int -> {0, 2}
//	string -> {1, 2}
//	{_|_, null, bytes, [], {}} -> {2}
//}
//b: {
//	int -> {0, 1, 2}
//	* -> {0, 1}
//}
//
//we have a bunch of field -> set mappings
//what strategy should we choose for choosing a discriminator?
//
//choose a partition key that maximises the number of partitions.
//but what do we even mean by that?
//
//sort all the possible discriminators by:
//	number of non-overlapping sets (highest priority)
//	number of different sets
//
//choose the highest priority discriminator
//
//
//enumerate the paths of all required fields
//
//	for each required field, store the valueSet for its value in each arm
//
//
//
//b: {0, 1

// Try some discrimination on kind first.
//string, "foo", {...}

// logger.Printf("discriminate %v {", arms)
// defer logger.Printf("}")
//
//	if len(arms) == 0 {
//		// No arms => produce an "error" leaf node (or handle specially).
//		// We'll do a LeafNode with no arms, signifying error or no match.
//		return &LeafNode{}
//	}
//
//	if len(arms) == 1 {
//		// Exactly one arm => no further discrimination possible
//		return &LeafNode{Arms: intSet{0: true}}
//	}
//
// // Step 1: Group arms by top-level kind
// kindGroups := make(map[cue.Kind][]Arm)
//
//	for _, arm := range arms {
//		kind := arm.Value.IncompleteKind()
//		kindGroups[kind] = append(kindGroups[kind], arm)
//	}
//
//	if len(kindGroups) > 1 {
//		logger.Printf("multiple kinds {")
//		defer logger.Printf("}")
//		// We have multiple kinds among the arms => top-level KindSwitch
//		branches := make(map[cue.Kind]DecisionNode, len(kindGroups))
//		for k, group := range kindGroups {
//			branches[k] = discriminate(group)
//		}
//		return &KindSwitchNode{Branches: branches}
//	}
//
// // If we get here, all arms share the same kind. The interesting case is struct discrimination.
// theKind := arms[0].Value.IncompleteKind()
//
//	if theKind != cue.StructKind {
//		// They might be all number or all string, but still distinct constraints.
//		// For simplicity, we can't refine further => produce LeafNode with all indexes.
//		return leafWithAllArms(arms)
//	}
//
// // All arms are struct. Let's see if we can do a value-based switch on a field
// // that’s required in all arms. If not, try presence-based on a field that’s
// // required in only some arms.
//
//	if node := buildStructDiscriminator(arms); node != nil {
//		return node
//	}
//
// // can't refine further.
// return leafWithAllArms(arms)
//panic("TODO")
//}

// discriminators returns the possible discriminators between the selected elements
// of the given arm values. The first returned value discriminates based on exact
// value; the second discriminates based on kind.
//
// If it's possible to exactly discriminate using the types only, it'll return an empty
// value discriminator map.
//
// It also reports whether the returned discrimators will fully discriminate
// the elements of needDiscrim
func discriminators(arms0 []cue.Value, selected, needDiscrim intSet) (map[atom]intSet, map[cue.Kind]intSet, bool) {
	arms := make([]valueSet, len(arms0))
	for i := range selected {
		arms[i] = valueSetForValue(arms0[i])
	}
	byKind := kindDiscrim(arms, selected, valueSet.kinds)
	full := fullyDiscriminated(maps.Values(byKind), needDiscrim)
	if !hasConsts(arms) || full {
		return nil, byKind, full
	}
	byValue := valueDiscrim(arms, selected)
	byKind = kindDiscrim(arms, selected, func(v valueSet) cue.Kind {
		return v.types
	})
	if mapHasKey(byKind, cue.NullKind) {
		delete(byValue, "null")
	}
	if mapHasKey(byValue, "true") && mapHasKey(byValue, "false") {
		delete(byKind, cue.BoolKind)
	}
	return byValue, byKind, fullyDiscriminated(iterConcat(maps.Values(byValue), maps.Values(byKind)), needDiscrim)
}

func hasConsts(arms []valueSet) bool {
	for _, arm := range arms {
		if len(arm.consts) > 0 {
			return true
		}
	}
	return false
}

func kindDiscrim(arms []valueSet, selected intSet, armKind func(valueSet) cue.Kind) map[cue.Kind]intSet {
	m := make(map[cue.Kind]intSet)
	for i, arm := range arms {
		if !selected[i] {
			continue
		}
		for _, k := range allKinds {
			if (armKind(arm) & k) == 0 {
				continue
			}
			if m[k] == nil {
				m[k] = make(intSet)
			}
			m[k][i] = true
		}
	}
	return m
}

// valueDiscrim returns a map from const value to
// which arms are known to be selected for those
// values. It also returns a map from type to arm sets
// for values outside the known constants.
func valueDiscrim(arms []valueSet, selected intSet) map[atom]intSet {
	var byValue map[atom]intSet
	for i, arm := range arms {
		if !selected[i] {
			continue
		}
		for c := range arm.consts {
			if byValue == nil {
				byValue = make(map[atom]intSet)
			}
			if byValue[c] == nil {
				byValue[c] = make(intSet)
			}
			byValue[c][i] = true
		}
	}
	// Ensure that every value in byValue also includes
	// arms that don't have constants but do allow the
	// const.
	for c, group := range byValue {
		getm := copyMap(&group)
		kind := c.kind()
		for i, a := range arms {
			if (a.types & kind) != 0 {
				getm()[i] = true
			}
		}
		byValue[c] = group
	}
	return byValue
}

// fullyDiscriminated reports whether the iterator elements
// fully discriminate all the members of selected;
// that is, each member of the sequence must select
// at most one element and all the elements in selected
// must be present at least once.
func fullyDiscriminated(it iter.Seq[intSet], selected intSet) bool {
	found := make(intSet)
	for x := range it {
		n := 0
		for y := range x {
			if !selected[y] {
				continue
			}
			if !found[y] {
				found[y] = true
			}
			n++
		}
		if n > 1 {
			return false
		}
	}
	return len(found) == len(selected)
}

func allEqFunc[T any](it iter.Seq[T], eq func(T, T) bool) bool {
	first := true
	var firstItem T
	for x := range it {
		if first {
			firstItem = x
			first = false
		} else {
			if !eq(x, firstItem) {
				return false
			}
		}
	}
	return true
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

func intSetN(n int) intSet {
	if n == 0 {
		return nil
	}
	s := make(intSet)
	for i := range n {
		s[i] = true
	}
	return s
}
