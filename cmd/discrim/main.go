package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/load"
	"cuelang.org/go/cue/parser"

	"github.com/rogpeppe/cuediscrim"
)

var (
	flagAll                   = flag.Bool("a", false, "show information on all disjuncts, not just imperfect ones")
	flagVerbose               = flag.Bool("v", false, "print more info")
	flagExpr                  = flag.String("e", "", "expression to print info on")
	flagContinue              = flag.Bool("continue-on-error", false, "continue on error")
	flagMergeCompatible       = flag.Bool("m", false, "merge compatible data types if a perfect discriminator cannot be found")
	flagMergeCompatibleAlways = flag.Bool("M", false, "merge compatible types even when the discriminator is perfect")
	flagTypes                 = flag.Bool("t", false, "when types have been merged, show the merged result")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: discrim [package...]\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
By default, discrim searches for and prints information on discriminators
that are not "perfect" in the named packages.

If an expression is provided with -e, the discriminator for just that
expression will be printed, evaluated in the context of the specified
package specified.
`)
		os.Exit(2)
	}
	flag.Parse()
	ctx := cuecontext.New()

	var expr ast.Expr
	if *flagExpr != "" {
		var err error
		expr, err = parser.ParseExpr("expression", *flagExpr)
		if err != nil {
			log.Fatalf("cannot parse expression: %v", err)
		}
	}

	insts := load.Instances(flag.Args(), nil)
	if len(insts) != 1 && expr != nil {
		log.Fatalf("-e requires exactly one package to be specifed")
	}
	if expr != nil {
		scope := ctx.BuildInstance(insts[0]) // Ignore error.
		var logTo io.Writer
		if *flagVerbose {
			logTo = os.Stdout
		}
		v := ctx.BuildExpr(expr, cue.Scope(scope), cue.InferBuiltins(true))
		if err := v.Err(); err != nil {
			log.Fatalf("cannot build expression: %v", err)
		}
		arms := cuediscrim.Disjunctions(v)
		if *flagVerbose {
			printArms(arms)
		}
		d, groups, isPerfect := discriminate(arms, logTo)
		if *flagTypes || *flagVerbose {
			printMergedTypes(arms, groups)
		}
		if !isPerfect {
			fmt.Printf("discriminator is imperfect\n")
		}
		fmt.Print(cuediscrim.NodeString(d))
		return
	}
	for _, inst := range insts {
		pkg := ctx.BuildInstance(inst)
		if err := pkg.Err(); err != nil {
			fmt.Fprintf(os.Stderr, "cannot build instance: %v\n", err)
			if !*flagContinue {
				os.Exit(1)
			}
			continue
		}
		new(walker).walkFields(pkg)
	}
}

func discriminate(arms []cue.Value, verboseWriter io.Writer) (cuediscrim.DecisionNode, []cuediscrim.IntSet, bool) {
	merge := *flagMergeCompatibleAlways

	n, groups, isPerfect := cuediscrim.Discriminate(arms, cuediscrim.LogTo(verboseWriter), cuediscrim.MergeCompatible(merge))
	if isPerfect || !*flagMergeCompatible {
		return n, groups, isPerfect
	}
	return cuediscrim.Discriminate(arms, cuediscrim.LogTo(verboseWriter), cuediscrim.MergeCompatible(true))
}

func printMergedTypes(arms []cue.Value, groups []cuediscrim.IntSet) {
	for _, g := range groups {
		if g.Len() < 2 {
			continue
		}
		var vs []cue.Value
		for i := range g.Values() {
			vs = append(vs, arms[i])
		}
		expr := cuediscrim.DataTypeForValues(vs)
		data, err := format.Node(expr)
		if err != nil {
			panic(err)
		}
		fmt.Printf("merged %s into %s\n", cuediscrim.SetString(g), data)
	}
}

type walker struct {
	printed bool
}

func (w *walker) walkFields(v cue.Value) {
	if (v.IncompleteKind() & cue.StructKind) == 0 {
		return
	}
	iter, err := v.Fields(cue.All())
	if err != nil {
		return
	}
	for iter.Next() {
		v := iter.Value()
		if arms := cuediscrim.Disjunctions(v); len(arms) > 1 {
			n, groups, isPerfect := discriminate(arms, nil)
			if *flagAll || !isPerfect {
				if w.printed {
					fmt.Printf("\n")
				}
				w.printed = true
				fmt.Printf("%v: %v\n", v.Pos(), v.Path())
				if *flagVerbose {
					printArms(arms)
					// Run again so that we get the debug info.
					// TODO avoid duplicating the work when *flagAll is specified
					// so we know we're printing debug info in advance.
					n, groups, _ = discriminate(arms, os.Stdout)
				}
				if *flagTypes || *flagVerbose {
					printMergedTypes(arms, groups)
				}
				fmt.Print(cuediscrim.NodeString(n))
			}

		}
		w.walkFields(v)
	}
}

func printArms(arms []cue.Value) {
	for i, arm := range arms {
		fmt.Printf("%d: %v: %v\n", i, arm.Pos(), arm)
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
