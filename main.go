package main

import (
	"fmt"
	"log"
	"os"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/errors"
)

func main() {
	// Example CUE snippet:
	// This includes multiple top-level kinds (number, string, struct)
	// as well as enumerated "type" for the struct arms.
	cueSource := `
#S: number | string | {
	type!: "foo"
	x!:    int
} | {
	type!: "bar" | "baz"
	p?:    string
} | {
	type!: bool
}
`
	ctx := cuecontext.New()
	val := ctx.CompileString(cueSource)
	if err := val.Err(); err != nil {
		log.Fatal(errors.Details(err, nil))
	}
	root := val.LookupPath(cue.ParsePath("#S"))
	if root.Err() != nil {
		panic(root.Err())
	}

	op, disjuncts := root.Expr()
	if op != cue.OrOp {
		disjuncts = []cue.Value{root}
	}

	selected := make(intSet)
	for i := range len(disjuncts) {
		selected[i] = true
	}

	// Step 2: Build the decision tree from these arms
	tree := BuildDecisionTree(disjuncts, selected)

	// Step 3: Print the resulting tree
	fmt.Println("=== Decision Tree for #S ===")
	tree.Write(&indentWriter{
		w: os.Stdout,
	})
}
