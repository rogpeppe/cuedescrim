package cuediscrim

import (
	"fmt"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/go-quicktest/qt"
)

var allRequiredFieldsTests = []struct {
	testName   string
	labelTypes labelType
	cue        string
	want       string
}{{
	testName:   "SimpleStruct",
	labelTypes: requiredLabel,
	cue:        "a!: int, b!: string",
	want: `
a: [int]
b: [string]
`,
}, {
	testName:   "IncludeOtherFields",
	labelTypes: requiredLabel | optionalLabel | regularLabel,
	cue:        "a!: int, b?: string, c: 5",
	want: `
a: [int]
b: [string]
c: [5]
`,
}, {
	testName:   "NestedStruct",
	labelTypes: requiredLabel,
	cue: `
a!: int
b!: x!: string
b!: y!: "foo"
c!: null
`,
	want: `
a: [int]
c: [null]
b: [{
	x!: string
	y!: "foo"
}]
b.x: [string]
b.y: ["foo"]
`,
}, {
	testName:   "JustAtoms",
	labelTypes: requiredLabel,
	cue:        `1 | 2`,
	want:       ``,
}, {
	testName:   "Structs",
	labelTypes: requiredLabel,
	cue: `
{a!: "x", b!: bool, c?: string} |
{a!: "y", d!: bool}
`,
	want: `
a: ["x", "y"]
b: [bool, _|_]
d: [_|_, bool]
`,
}, {
	testName:   "StructsWithNonStructs",
	labelTypes: requiredLabel,
	cue: `
>5 | null | "foo" | "bar" | {
	type!: "t1"
	a!: bool
} | {
	type!: "t2"
	b!: int
}
`,
	want: `
type: [_|_, _|_, _|_, _|_, "t1", "t2"]
a: [_|_, _|_, _|_, _|_, bool, _|_]
b: [_|_, _|_, _|_, _|_, _|_, int]
`,
}, {
	testName:   "WithOptional",
	labelTypes: requiredLabel,
	cue: `
	discrim!: kind!: "foo"
	a?: int
`,
	want: `
discrim: [{
	kind!: "foo"
}]
discrim.kind: ["foo"]
`,
}}

func TestAllFields(t *testing.T) {
	ctx := cuecontext.New()
	for _, test := range allRequiredFieldsTests {
		t.Run(test.testName, func(t *testing.T) {
			v := ctx.CompileString(test.cue)
			var buf strings.Builder
			w := &indentWriter{
				w: &buf,
			}
			arms := disjunctionArms(v)
			for path, values := range allFields(arms, intSetN(len(arms)), test.labelTypes) {
				fmt.Fprintf(w, "%s: [", path)
				for i, v := range values {
					if i > 0 {
						fmt.Fprintf(w, ", ")
					}
					if v.Exists() {
						fmt.Fprintf(w, "%v", v)
					} else {
						fmt.Fprintf(w, "_|_")
					}
				}
				w.Printf("]")
			}
			qt.Assert(t, qt.Equals(buf.String(), strings.TrimPrefix(test.want, "\n")))
		})
	}
}

func disjunctionArms(v cue.Value) []cue.Value {
	op, args := v.Expr()
	if op != cue.OrOp {
		return []cue.Value{v}
	}
	return args
}
