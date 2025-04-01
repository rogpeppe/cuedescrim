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
	testName: "SeveralWithCompatibleStructs",
	cue:      `1 | 2 | "foo" | "bar" | =~"baz" | {x!: string} | {y!: string}`,
	want: `1 | "foo" | {
	x!: string
}`,
	rev: []Set[int]{
		setOf(0, 1),
		setOf(2, 3, 4),
		setOf(5, 6),
	},
}, {
	testName: "IncompatibleStructs",
	cue:      `{a!: int, b?: string} | {a!: int, b?: bool}`,
	want: `{
	a!: int
} | {
	a!: int
}`,
	rev: []Set[int]{
		setOf(0),
		setOf(1),
	},
}}

func TestMergeAtoms(t *testing.T) {
	for _, test := range mergeAtomsTests {
		t.Run(test.testName, func(t *testing.T) {
			ctx := cuecontext.New()
			val := ctx.CompileString(test.cue)
			qt.Assert(t, qt.IsNil(val.Err()))

			arms := Disjunctions(val)
			arms1, revFunc := mergeCompatible(arms)
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

var compatibleTests = []struct {
	name string
	cue  string
	want bool
}{
	{
		name: "SingleAtom",
		cue:  "1",
		want: true, // Only one arm, so trivially compatible.
	}, {
		name: "TwoAtomsSameKind",
		cue:  "1 | 2",
		want: true, // Both are int atoms.
	}, {
		name: "AtomTypes",
		cue:  "bool | int",
		want: false, // Different atom kinds.
	}, {
		name: "AtomAndStruct",
		cue:  "1 | {a!: int}",
		want: false, // Incompatible: an atom and a struct.
	}, {
		name: "StructsWithDifferentFields",
		cue:  "{a!: int} | {b!: string}",
		want: true, // They have compatible required fields.
	}, {
		name: "StructsWithMergeableFields",
		cue:  "{x!: int} | {x!: int, y?: string}",
		want: true,
	}, {
		name: "MixedStructAndAtomType",
		cue:  "string | {x!: bool}",
		want: false, // One is an atom kind, the other is a struct.
	},
}

func TestCompatible(t *testing.T) {
	for _, test := range compatibleTests {
		t.Run(test.name, func(t *testing.T) {
			ctx := cuecontext.New()
			val := ctx.CompileString(test.cue)
			qt.Assert(t, qt.IsNil(val.Err()))

			arms := Disjunctions(val)
			got := compatible(arms)
			qt.Assert(t, qt.Equals(got, test.want))
		})
	}
}
