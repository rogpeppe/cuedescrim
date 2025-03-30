package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/errors"
	"cuelang.org/go/cue/load"
	"cuelang.org/go/cue/parser"

	"github.com/rogpeppe/cuediscrim"
)

var (
	flagDebug   = flag.Bool("debug", false, "debug logging")
	flagPackage = flag.String("p", "", "package or CUE file to evaluate expression in")
	flagAll     = flag.Bool("a", false, "evaluate on all disjunctions/matchN fields in the package (default .)")
	flagVerbose = flag.Bool("v", false, "cause -a to show good discriminators too")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: discrim [-p package] [-a] [<expr>]\n")
		fmt.Fprintf(os.Stderr, `
By default print the decision tree for discriminating between arms
of the disjunction expression, which is evaluated in the context
of the given package if provided.

If the -a flag is provided, then the package is walked to find all disjunctions.
By default any disjunction that does not have a good discriminator is printed;
the -v flag causes all decision trees to be printed.
`)
		os.Exit(2)
	}
	flag.Parse()
	if flag.NArg() > 1 {
		flag.Usage()
	}
	if *flagDebug {
		cuediscrim.LogTo(os.Stderr)
	}
	ctx := cuecontext.New()
	var expr ast.Expr
	if flag.NArg() > 0 {
		var err error
		expr, err = parser.ParseExpr("expression", flag.Arg(0))
		if err != nil {
			log.Fatalf("cannot parse expression: %v", err)
		}
	}

	scope := ctx.CompileString("_")
	if *flagAll && *flagPackage == "" {
		*flagPackage = "."
	}
	if p := *flagPackage; p != "" {
		insts := load.Instances([]string{p}, nil)
		vs, err := ctx.BuildInstances(insts)
		if err != nil {
			log.Fatalf("cannot build instances: %v", errors.Details(err, nil))
		}
		scope = vs[0]
	}
	if *flagAll {
		if expr != nil {
			flag.Usage()
		}
		walkFields(scope)
		return
	}
	if expr == nil {
		flag.Usage()
	}
	v := ctx.BuildExpr(expr, cue.Scope(scope), cue.InferBuiltins(true))
	arms := cuediscrim.Disjunctions(v)
	if *flagDebug {
		for i, arm := range arms {
			fmt.Fprintf(os.Stderr, "%d: %v: %v\n", i, arm.Pos(), arm)
		}
	}
	fmt.Print(cuediscrim.NodeString(cuediscrim.Discriminate(arms)))
}

func walkFields(v cue.Value) {
	if v.IncompleteKind() != cue.StructKind {
		return
	}
	iter, err := v.Fields(cue.All())
	if err != nil {
		return
	}
	for iter.Next() {
		v := iter.Value()
		if isDisjunction(v) {
			n := cuediscrim.Discriminate(cuediscrim.Disjunctions(v))
			if *flagVerbose || !isPerfect(n) {
				fmt.Printf("%v: %v\n", v.Pos(), v.Path())
				fmt.Print(cuediscrim.NodeString(n))
				fmt.Println("")
			}

		}
		walkFields(v)
	}
}

func isDisjunction(v cue.Value) bool {
	op, args := v.Expr()
	switch op {
	case cue.OrOp:
		return true
	case cue.CallOp:
		if fmt.Sprint(args[0]) != "matchN" {
			return false
		}
		if n, _ := args[1].Int64(); n != 1 {
			return false
		}
		return true
	}
	return false
}

// isPerfect reports whether n is a "perfect" discriminator,
// in that any given value must result in a single arm chosen
// or an error.
func isPerfect(n cuediscrim.DecisionNode) bool {
	switch n := n.(type) {
	case nil:
		return true
	case *cuediscrim.LeafNode:
		return len(n.Arms) <= 1
	case *cuediscrim.KindSwitchNode:
		for _, n := range n.Branches {
			if !isPerfect(n) {
				return false
			}
		}
		return true
	case *cuediscrim.FieldAbsenceNode:
		return false
	case *cuediscrim.ValueSwitchNode:
		for _, n := range n.Branches {
			if !isPerfect(n) {
				return false
			}
		}
		return isPerfect(n.Default)
	case *cuediscrim.ErrorNode, cuediscrim.ErrorNode:
		return true
	}
	panic(fmt.Errorf("unexpected node type %#v", n))
}
