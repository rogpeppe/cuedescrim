package cuediscrim

import (
	"fmt"
	"io"
	"iter"
	"maps"

	"cuelang.org/go/cue"
)

var logger *indentWriter

func LogTo(w io.Writer) {
	logger = &indentWriter{
		w: w,
	}
}

// Discriminate returns a decision tree that can be used
// to decide which arm of a disjunction should be chosen.
// If v is not a disjunction, it returns a decision node that
// just selects v itself.
func Discriminate(arms []cue.Value) DecisionNode {
	return discriminate(arms, intSetN(len(arms)))
}

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
	logger.Printf("no pure discriminator found; trying existence checks; selected %s", setString(selected))

	// We haven't found any pure single discriminator.
	// Now try to narrow things down by checking for field absence.
	//
	// Note that in general, testing for existence isn't useful because all the discrimination
	// is based on requirements, and extra fields are generally allowed.
	// So by testing for non-existence we can narrow things down
	// one arm at a time.
	possible := selected
	branches := make(map[string]intSet)
	for path, values := range allRequiredFields(arms, selected) {
		group := existenceDiscriminator(values, selected)
		logger.Printf("----- PATH %s %s; possible %s", path, setString(group), setString(possible))

		if len(group) != len(selected)-1 {
			continue
		}
		logger.Printf("it's possible!")
		// we're deselecting exactly one member, but
		// we want to be sure that we're removing something new.
		removed := false
		for i := range possible {
			if !group[i] {
				removed = true
				break
			}
		}
		if !removed {
			logger.Printf("nothing removed")
			continue
		}
		possible = possible.intersect(group)
		branches[path] = group
		if len(possible) == 0 {
			break
		}
	}
	if len(possible) > 0 {
		// We haven't been able to form a discriminator.
		// TODO better than this.
		return &LeafNode{
			Arms: selected,
		}
	}
	return &FieldAbsenceNode{
		Branches: branches,
	}
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
			case group.Equal(selected):
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
		if group.Equal(selected) {
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

// existenceDiscriminator returns the subset of selected that checking for non-existence
// will select.
func existenceDiscriminator(arms []cue.Value, selected intSet) intSet {
	discrim := make(intSet)
	for i, v := range arms {
		if selected[i] && !v.Exists() {
			// Note: because we're only inspecting required fields,
			// when v exists, we know it's required.
			// As a corollary, when it doesn't exist, we know
			// that checking for existence won't rule it out.
			discrim[i] = true
		}
	}
	return discrim
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
