package cuediscrim

import (
	"iter"

	"cuelang.org/go/cue"
)

// allFields returns an iterator over the paths of all the required fields
// in the selected elements of values, in breadth-first order with non-structs produced earlier
// than structs.
// This includes the root values, which are also "required" at the root path.
// It only includes string labels that have any bits set in labelTypes.
func allFields(values []cue.Value, selected Set[int], labelTypes labelType) iter.Seq2[string, []cue.Value] {
	return func(yield func(string, []cue.Value) bool) {
		var q queue[pathValues]
		q.push(pathValues{
			path: ".",
			// Note: this might include elements not in selected, but
			// those are ignored so it doesn't matter.
			values: values,
		})
		for {
			x, ok := q.pop()
			if !ok {
				return
			}
			var ordered [][]cue.Value
			var orderedNames []string
			byName := make(map[string]int)
			for i, v := range x.values {
				if !selected.Has(i) {
					continue
				}
				for label, v := range structFields(v, labelTypes) {
					name := label.name
					var entry []cue.Value
					if i, ok := byName[name]; ok {
						entry = ordered[i]
					} else {
						entry = make([]cue.Value, len(x.values))
						byName[name] = len(ordered)
						ordered = append(ordered, entry)
						orderedNames = append(orderedNames, name)
					}
					entry[i] = v
				}
			}

			// First produce any field that has a non-struct value.
		outer:
			for oi := range ordered {
				name, values := orderedNames[oi], ordered[oi]
				for _, v := range values {
					if v.Exists() && v.IncompleteKind() != cue.StructKind {
						if !yield(pathConcat(x.path, name), values) {
							return
						}
						ordered[oi] = nil
						continue outer
					}
				}
			}
			// Then all remaining fields and queue up the deeper fields.
			for i := range ordered {
				name, values := orderedNames[i], ordered[i]
				if values == nil {
					// Already produced.
					continue
				}
				path := pathConcat(x.path, name)
				if !yield(path, values) {
					return
				}
				q.push(pathValues{path, values})
			}
		}
	}
}

func pathConcat(p1, p2 string) string {
	if p1 == "" || p1 == "." {
		return p2
	}
	return p1 + "." + p2
}

type pathValues struct {
	path   string
	values []cue.Value
}

// structFields returns an iterator over the names of all the fields in v
// that match any of the given label types, and their values.
func structFields(v cue.Value, labelTypes labelType) iter.Seq2[label, cue.Value] {
	return func(yield func(label, cue.Value) bool) {
		if !v.Exists() {
			return
		}
		iter, err := v.Fields(cue.Optional(true))
		if err != nil {
			return
		}
		for iter.Next() {
			if labelTypes.match(iter.FieldType()) {
				lab := label{
					name:      iter.Selector().Unquoted(),
					labelType: labelTypeForSelectorType(iter.FieldType()),
				}
				if !yield(lab, iter.Value()) {
					break
				}
			}
		}
	}
}

type label struct {
	name      string
	labelType labelType
}

type labelType int

const (
	requiredLabel labelType = 1 << iota
	optionalLabel
	regularLabel
)

func (t labelType) match(selt cue.SelectorType) bool {
	return (t & labelTypeForSelectorType(selt)) != 0
}

func labelTypeForSelectorType(selt cue.SelectorType) labelType {
	if (selt & cue.StringLabel) == 0 {
		return 0
	}
	switch selt & (cue.OptionalConstraint | cue.RequiredConstraint) {
	case 0:
		return regularLabel
	case cue.OptionalConstraint:
		return optionalLabel
	case cue.RequiredConstraint:
		return requiredLabel
	default:
		panic("unreachable")
	}
}

type queue[T any] []T

func (q *queue[T]) push(x T) {
	*q = append(*q, x)
}

func (q *queue[T]) pop() (T, bool) {
	if len(*q) == 0 {
		return *new(T), false
	}
	x := (*q)[0]
	*q = (*q)[1:]
	return x, true
}
