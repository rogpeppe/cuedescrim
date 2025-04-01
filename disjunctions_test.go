package cuediscrim

import (
	"fmt"
	"slices"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/go-quicktest/qt"
)

var mergeAtomsTests = []struct {
	testName string
	cue      string
	want     string
	rev      []Set[int]
}{{
	testName: "Single",
	cue:      "{}",
	want:     "{}",
	rev:      []Set[int]{setOf(0)},
}, {
	testName: "TwoDifferent",
	cue:      "1 | null",
	want:     "1 | null",
	rev:      []Set[int]{setOf(0), setOf(1)},
}, {
	testName: "Several",
	cue:      `1 | 2 | "foo" | "bar" | =~"baz" | {x!: string} | {y!: string}`,
	want: `1 | "foo" | {
	x!: string
} | {
	y!: string
}`,
	rev: []Set[int]{
		setOf(0, 1),
		setOf(2, 3, 4),
		setOf(5),
		setOf(6),
	},
}}

func TestMergeAtoms(t *testing.T) {
	for _, test := range mergeAtomsTests {
		t.Run(test.testName, func(t *testing.T) {
			ctx := cuecontext.New()
			val := ctx.CompileString(test.cue)
			qt.Assert(t, qt.IsNil(val.Err()))

			arms := Disjunctions(val)
			arms1, revFunc := mergeAtoms(arms)
			got := joinSeq(
				iterMap(slices.Values(arms1), func(v cue.Value) string {
					return fmt.Sprint(v)
				}),
				" | ",
			)
			qt.Assert(t, qt.Equals(got, test.want))
			rev := make([]Set[int], len(arms1))
			for i := range arms1 {
				rev[i] = revFunc(i)
			}
			qt.Assert(t, deepEquals(rev, test.rev))
		})
	}
}
