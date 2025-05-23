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
