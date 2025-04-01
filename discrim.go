package cuediscrim

import (
	"io"
	"iter"
	"maps"

	"cuelang.org/go/cue"
)

type options struct {
	logger          *indentWriter
	mergeCompatible bool
}

// LogTo causes debug information to be written to w.
func LogTo(w io.Writer) Option {
	return func(opts *options) {
		opts.logger = &indentWriter{
			w: w,
		}
	}
}

func MergeCompatible(enable bool) Option {
	return func(opts *options) {
		opts.mergeCompatible = enable
	}
}

type Option func(*options)

// Discriminate returns a decision tree that can be used
// to decide between the given values, assuming they're
// all arms of a disjunction. See [Disjunctions] for a way
// to split a value into its component disjunctions recursively.
//
// It also reports whether the discriminator is "perfect"
// for discriminating between the arms. That decision
// is influenced by the MergeAtoms and MergeCompatibleStructs
// options.
func Discriminate(arms []cue.Value, optArgs ...Option) (DecisionNode, bool) {
	var opts options
	for _, f := range optArgs {
		f(&opts)
	}
	origArms := arms
	var rev func(int) IntSet
	if opts.mergeCompatible {
		arms, rev = mergeCompatible(arms)
	}
	var n DecisionNode
	if len(arms) <= 64 {
		d := &discriminator[wordSet]{
			options: opts,
			sets:    wordSetAPI{},
			rev:     rev,
		}
		n = d.discriminate(arms, wordSetN(len(arms)))
	} else {
		d := &discriminator[mapSet[int]]{
			options: opts,
			sets:    mapSetAPI[int]{},
			rev:     rev,
		}
		n = d.discriminate(arms, intSetN(len(arms)))
	}

	return n, isPerfect(n, opts.mergeCompatible, origArms)
}

type discriminator[Set any] struct {
	sets setAPI[Set, int]
	rev  func(int) IntSet
	options
}

func (d *discriminator[Set]) discriminate(arms []cue.Value, selected Set) (_n DecisionNode) {
	d.logger.Printf("discriminate %v {", d.setString(selected))
	d.logger.Indent()
	defer func() {
		d.logger.Printf("} -> %T", _n)
	}()
	defer d.logger.Unindent()
	if d.sets.len(selected) <= 1 {
		// Nothing to disambiguate.
		return d.newLeaf(selected)
	}
	// First try to discriminate based on the top level value only.
	// We're happy just to make some progress, so we'll consider
	// it "fully discriminated" if all the non-struct elements
	// are discriminated, assuming there are such elements.
	// If there aren't then we require all elements to be discriminated.
	needDiscrim := d.sets.make()
	for i, v := range arms {
		if (v.IncompleteKind() & cue.StructKind) == 0 {
			d.sets.add(&needDiscrim, i)
		}
	}
	if d.sets.len(needDiscrim) == 0 {
		needDiscrim = selected
	}
	byValue, byKind, full := d.discriminators(arms, selected, needDiscrim)
	if full {
		return d.buildDecisionFromDescriminators(".", arms, selected, byValue, byKind)
	}
	// First try to find a single discriminator that can be used to do all discrimination.
	for path, values := range allFields(arms, d.sets.asSet(selected), requiredLabel) {
		d.logger.Printf("----- PATH %s", path)
		byValue, byKind, full := d.discriminators(values, selected, selected)
		if full {
			d.logger.Printf("fully discriminated")
		}
		d.logger.Printf("values:")
		for v, group := range byValue {
			d.logger.Printf("	%v: %v", v, d.setString(group))
		}
		d.logger.Printf("kinds:")
		for k, group := range byKind {
			d.logger.Printf("	%v: %v", k, d.setString(group))
		}
		if full {
			return d.buildDecisionFromDescriminators(path, values, selected, byValue, byKind)
		}
	}
	d.logger.Printf("no pure discriminator found; trying existence checks; selected %s", d.setString(selected))

	// We haven't found any pure single discriminator.
	// Now try to narrow things down by checking for field absence.
	//
	// Note that in general, testing for existence isn't useful because all the discrimination
	// is based on requirements, and extra fields are generally allowed.
	// So by testing for non-existence we can narrow things down
	// one arm at a time.
	possible := selected
	branches := make(map[string]IntSet)
	for path, values := range allFields(arms, d.sets.asSet(selected), requiredLabel) {
		group := d.existenceDiscriminator(values, selected)
		d.logger.Printf("----- PATH %s %s; possible %s", path, d.setString(group), d.setString(possible))

		if d.sets.len(group) != d.sets.len(selected)-1 {
			continue
		}
		d.logger.Printf("it's possible!")
		// we're deselecting exactly one member, but
		// we want to be sure that we're removing something new.
		removed := false
		for i := range d.sets.values(possible) {
			if !d.sets.has(group, i) {
				removed = true
				break
			}
		}
		if !removed {
			d.logger.Printf("nothing removed")
			continue
		}
		possible = d.sets.intersect(possible, group)
		branches[path] = d.sets.asSet(group)
		if d.sets.len(possible) == 0 {
			break
		}
	}
	if d.sets.len(possible) > 0 {
		// We haven't been able to form a discriminator.
		// TODO better than this.
		return d.newLeaf(selected)
	}
	return &FieldAbsenceNode{
		Branches: branches,
	}
}

func (d *discriminator[Set]) buildDecisionFromDescriminators(path string, values []cue.Value, selected Set, byValue map[Atom]Set, byKind map[cue.Kind]Set) DecisionNode {
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
			d.logger.Printf("kind %v: %v", k, d.setString(group))
			var branch DecisionNode
			switch {
			case k == cue.StructKind && d.sets.len(group) > 1:
				// We need to disambiguate a struct.
				branch = d.discriminate(values, group)
			case d.sets.equal(group, selected):
				// We've got nothing more to base a decision on,
				// so terminate.
				branch = d.newLeaf(selected)
			default:
				branch = d.discriminate(values, group)
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
		Branches: make(map[Atom]DecisionNode, len(byValue)),
		Default:  kindSwitch,
	}
	for val, group := range byValue {
		var branch DecisionNode
		if d.sets.equal(group, selected) {
			// We've got nothing more to base a decision on,
			// so terminate.
			branch = d.newLeaf(selected)
		} else {
			d.logger.Printf("valSwitch %v", val)
			branch = d.discriminate(values, group)
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
func (d *discriminator[Set]) discriminators(arms0 []cue.Value, selected, needDiscrim Set) (map[Atom]Set, map[cue.Kind]Set, bool) {
	arms := make([]valueSet, len(arms0))
	for i := range d.sets.values(selected) {
		arms[i] = valueSetForValue(arms0[i])
	}
	byKind := d.kindDiscrim(arms, selected, valueSet.kinds)
	full := d.fullyDiscriminated(maps.Values(byKind), needDiscrim)
	if !hasConsts(arms) || full {
		return nil, byKind, full
	}
	byValue := d.valueDiscrim(arms, selected)
	byKind = d.kindDiscrim(arms, selected, func(v valueSet) cue.Kind {
		return v.types
	})
	if mapHasKey(byKind, cue.NullKind) {
		delete(byValue, Atom{"null"})
	}
	if mapHasKey(byValue, Atom{"true"}) && mapHasKey(byValue, Atom{"false"}) {
		delete(byKind, cue.BoolKind)
	}
	return byValue, byKind, d.fullyDiscriminated(iterConcat(maps.Values(byValue), maps.Values(byKind)), needDiscrim)
}

// existenceDiscriminator returns the subset of selected that checking for non-existence
// will select.
func (d *discriminator[Set]) existenceDiscriminator(arms []cue.Value, selected Set) Set {
	discrim := d.sets.make()
	for i, v := range arms {
		if d.sets.has(selected, i) && !v.Exists() {
			// Note: because we're only inspecting required fields,
			// when v exists, we know it's required.
			// As a corollary, when it doesn't exist, we know
			// that checking for existence won't rule it out.
			d.sets.add(&discrim, i)
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

func (d *discriminator[Set]) kindDiscrim(arms []valueSet, selected Set, armKind func(valueSet) cue.Kind) map[cue.Kind]Set {
	m := make(map[cue.Kind]Set)
	for i, arm := range arms {
		if !d.sets.has(selected, i) {
			continue
		}
		for _, k := range allKinds {
			if (armKind(arm) & k) == 0 {
				continue
			}
			s := m[k]
			d.sets.add(&s, i)
			m[k] = s
		}
	}
	return m
}

// valueDiscrim returns a map from const value to
// which arms are known to be selected for those
// values. It also returns a map from type to arm sets
// for values outside the known constants.
func (d *discriminator[Set]) valueDiscrim(arms []valueSet, selected Set) map[Atom]Set {
	var byValue map[Atom]Set
	for i, arm := range arms {
		if !d.sets.has(selected, i) {
			continue
		}
		for c := range arm.consts {
			if byValue == nil {
				byValue = make(map[Atom]Set)
			}
			s := byValue[c]
			d.sets.add(&s, i)
			byValue[c] = s
		}
	}
	// Ensure that every value in byValue also includes
	// arms that don't have constants but do allow the
	// const.
	for c, group := range byValue {
		changed := false
		kind := c.kind()
		for i, a := range arms {
			if (a.types&kind) != 0 && !d.sets.has(group, i) {
				if !changed {
					group = d.sets.clone(group)
					changed = true
				}
				d.sets.add(&group, i)
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
func (d *discriminator[Set]) fullyDiscriminated(it iter.Seq[Set], selected Set) bool {
	found := d.sets.make()
	for x := range it {
		n := 0
		for y := range d.sets.values(x) {
			if !d.sets.has(selected, y) {
				continue
			}
			d.sets.add(&found, y)
			n++
		}
		if n > 1 {
			return false
		}
	}
	return d.sets.len(found) == d.sets.len(selected)
}

func (d *discriminator[Set]) setString(s Set) string {
	return setString(d.asExternalSet(s))
}

func (d *discriminator[Set]) newLeaf(s Set) DecisionNode {
	return &LeafNode{
		Arms: d.asExternalSet(s),
	}
}

func (d *discriminator[Set]) asExternalSet(s Set) IntSet {
	return revSet(d.sets.asSet(s), d.rev)
}
