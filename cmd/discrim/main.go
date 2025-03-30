package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/errors"
	"cuelang.org/go/cue/load"
	"cuelang.org/go/cue/parser"

	"github.com/rogpeppe/cuediscrim"
)

var flagVerbose = flag.Bool("v", false, "vebose logging")

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: discrim <expr> [<cue-package>]\n")
		os.Exit(1)
	}
	flag.Parse()
	if flag.NArg() != 1 && flag.NArg() != 2 {
		flag.Usage()
	}
	if *flagVerbose {
		cuediscrim.LogTo(os.Stderr)
	}
	ctx := cuecontext.New()
	expr, err := parser.ParseExpr("expression", flag.Arg(0))
	if err != nil {
		log.Fatalf("cannot parse expression: %v", err)
	}

	scope := ctx.CompileString("_")
	if flag.NArg() > 1 {
		insts := load.Instances([]string{flag.Arg(1)}, nil)
		vs, err := ctx.BuildInstances(insts)
		if err != nil {
			log.Fatalf("cannot build instances: %v", errors.Details(err, nil))
		}
		scope = vs[0]
	}
	v := ctx.BuildExpr(expr, cue.Scope(scope), cue.InferBuiltins(true))
	fmt.Print(cuediscrim.NodeString(cuediscrim.Discriminate(v)))
}
