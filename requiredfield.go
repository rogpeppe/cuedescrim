package main

import (
	"iter"

	"cuelang.org/go/cue"
)

// allRequiredFields returns an iterator over the paths of all the required fields
// in the selected elements of values, in breadth-first order with non-structs produced earlier
// than structs.
// This includes the root values, which are also "required" at the root path.
func allRequiredFields(values []cue.Value, selected intSet) iter.Seq2[string, []cue.Value] {
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
				if !selected[i] {
					continue
				}
				for name, v := range requiredFields(v) {
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

			// First produce produce any field that has a non-struct value.
		outer:
			for oi := range ordered {
				name, values := orderedNames[oi], ordered[oi]
				for _, v := range values {
					if v.IncompleteKind() != cue.StructKind {
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

// requiredFields returns an iterator over the names of all the required fields
// in v and their values.
func requiredFields(v cue.Value) iter.Seq2[string, cue.Value] {
	return func(yield func(string, cue.Value) bool) {
		if v.IncompleteKind() != cue.StructKind {
			return
		}
		iter, err := v.Fields(cue.Optional(true))
		if err != nil {
			return
		}
		const requiredRegular = cue.StringLabel | cue.RequiredConstraint
		for iter.Next() {
			if (iter.FieldType() & requiredRegular) == requiredRegular {
				if !yield(iter.Selector().Unquoted(), iter.Value()) {
					break
				}
			}
		}
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
