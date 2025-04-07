package cuediscrim

import (
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"github.com/go-quicktest/qt"
)

var dataTypeForValuesTests = []struct {
	name string
	cue  string
	want string
}{{
	name: "SingleAtom",
	cue:  "1",
	want: "int",
}, {
	name: "SeveralAtoms",
	cue:  `"foo" | "bar" | "baz"`,
	want: "string",
}, {
	name: "Lists",
	cue:  `[int, ... string] | [int] | [int, "foo"]`,
	want: `[int, ...string]`,
}, {
	name: "ListsAllFixedLength",
	cue:  `[int, string] | [int&>3, =~"foo"]`,
	want: `[int, string]`,
}, {
	name: "ListsMultipleEllipses",
	cue:  `[int, ... int] | [int, int, int] | [int, int, ...int]`,
	want: `[int, ...int]`,
}, {
	name: "Structs",
	cue:  `{a!: int, b!: string} | {a!: 5, c?: bool}`,
	want: `{
	a!: int
	b!: string
	c?: bool
}`,
}}

func TestDataTypeForValues(t *testing.T) {
	for _, test := range dataTypeForValuesTests {
		t.Run(test.name, func(t *testing.T) {
			ctx := cuecontext.New()
			val := ctx.CompileString(test.cue)
			qt.Assert(t, qt.IsNil(val.Err()))

			arms := Disjunctions(val)
			expr := DataTypeForValues(arms)
			data, err := format.Node(expr)
			qt.Assert(t, qt.IsNil(err))
			qt.Assert(t, qt.Equals(string(data), strings.TrimPrefix(test.want, "\n")))
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
