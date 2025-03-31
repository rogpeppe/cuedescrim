package cuediscrim

import (
	"testing"

	"github.com/go-quicktest/qt"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/google/go-cmp/cmp"
)

func atoms(as ...string) mapSet[Atom] {
	s := make(mapSet[Atom])
	for _, a := range as {
		s[Atom{a}] = true
	}
	return s
}

var valueSetForValueTests = []struct {
	name string
	cue  string
	want valueSet
}{
	{
		name: "bool literal true",
		cue:  `true`,
		want: valueSet{
			consts: atoms(`true`),
		},
	},
	{
		name: "bool literal false",
		cue:  `false`,
		want: valueSet{
			consts: atoms(`false`),
		},
	},
	{
		name: "int literal 42",
		cue:  `42`,
		want: valueSet{
			consts: atoms(`42`),
		},
	},
	{
		name: "float literal 3.14",
		cue:  `3.14`,
		want: valueSet{
			consts: atoms(`3.14`),
		},
	},
	{
		name: "string literal hello",
		cue:  `"hello"`,
		want: valueSet{
			consts: atoms(`"hello"`),
		},
	},
	{
		name: "null literal",
		cue:  `null`,
		want: valueSet{
			types: cue.NullKind,
		},
	},
	{
		name: "bool type (non-concrete)",
		cue:  `bool`,
		want: valueSet{
			types: cue.BoolKind,
		},
	},
	{
		name: "or of int and float => number type",
		cue:  `int | float`,
		want: valueSet{
			types: cue.NumberKind, // merges intKind + floatKind => numberKind
		},
	},
	{
		name: "top",
		cue:  `_`,
		want: valueSet{
			types: cue.TopKind,
		},
	},
	{
		name: "string or object",
		cue:  `string | {a!: int}`,
		want: valueSet{
			types: cue.StringKind | cue.StructKind,
		},
	},
	{
		name: "or of 'hello' and 'world'",
		cue:  `"hello" | "world"`,
		want: valueSet{
			consts: atoms(`"hello"`, `"world"`),
		},
	},
	{
		name: "mixed or: 'foo' | bool",
		cue:  `"foo" | bool`,
		want: valueSet{
			types:  cue.BoolKind,
			consts: atoms(`"foo"`),
		},
	},
	{
		name: "non-atom struct => struct kind",
		cue:  `{}`,
		want: valueSet{
			types: cue.StructKind,
		},
	},
	{
		name: "mix of everything",
		cue:  `{foo!: int} | [] | "one" | "two" | 2 | number`,
		want: valueSet{
			types:  cue.ListKind | cue.NumberKind | cue.StructKind,
			consts: atoms(`"one"`, `"two"`),
		},
	},
	{
		name: "bottom",
		cue:  `_|_`,
		want: valueSet{
			types: cue.BottomKind,
		},
	},
}

func TestValueSetForZeroValue(t *testing.T) {
	qt.Assert(t, deepEquals(valueSetForValue(cue.Value{}), valueSet{
		types: cue.BottomKind,
	}))
}

func TestValueSetForValue(t *testing.T) {
	for _, test := range valueSetForValueTests {
		t.Run(test.name, func(t *testing.T) {
			qt.Assert(t, deepEquals(toVS(test.cue), test.want))
		})
	}
}

func TestValueSetOperations(t *testing.T) {
	// We'll define some basic test cases for union/intersect/without.
	tests := []struct {
		name string
		a, b string // input CUE expressions
		op   func(valueSet, valueSet) valueSet
		want valueSet
	}{
		{
			name: "union_of_true_and_false_=>_consts_with_both",
			a:    `true`,
			b:    `false`,
			op:   valueSet.union,
			want: valueSet{
				consts: atoms(`true`, `false`),
			},
		},
		{
			name: "intersect_of_'foo'_and_'foo'",
			a:    `"foo"`,
			b:    `"foo"`,
			op:   valueSet.intersect,
			want: valueSet{
				consts: atoms(`"foo"`),
			},
		},
		{
			name: "intersect_of_top_and_number",
			a:    `_`,
			b:    `number`,
			op:   valueSet.intersect,
			want: valueSet{
				types: cue.NumberKind,
			},
		},
		{
			name: "intersect_of_top_and_literals",
			a:    `_`,
			b:    `"a" | true`,
			op:   valueSet.intersect,
			want: valueSet{
				consts: atoms(`"a"`, "true"),
			},
		},
		{
			name: "intersect_of_'foo'_and_'bar'_=>_empty",
			a:    `"foo"`,
			b:    `"bar"`,
			op:   valueSet.intersect,
			want: valueSet{
				// no types, no consts
			},
		},
		{
			name: "union_of_bool_type_and_'true'",
			a:    `bool`,
			b:    `true`,
			op:   valueSet.union,
			want: valueSet{
				types: cue.BoolKind,
			},
		},
		{
			name: "intersect_of_bool_type_and_'true'_=>_'true'_only",
			a:    `bool`,
			b:    `true`,
			op:   valueSet.intersect,
			want: valueSet{
				consts: atoms(`true`),
			},
		},
		{
			name: "without_string-literal_from_union",
			a:    `string | number`,
			b:    `"hello"`,
			op:   valueSet.without,
			want: valueSet{
				types: cue.StringKind | cue.NumberKind,
			},
		},
		{
			name: "without_all",
			a:    `true | false`,
			b:    `bool`, // removing all bool from "true|false" => empty
			op:   valueSet.without,
			want: valueSet{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := toVS(tc.a)
			b := toVS(tc.b)
			got := tc.op(a, b)
			qt.Assert(t, deepEquals(got, tc.want))
		})
	}
}

func TestValueSetIsEmpty(t *testing.T) {
	t.Run("empty_isEmpty_=>_true", func(t *testing.T) {
		var empty valueSet
		qt.Assert(t, qt.IsTrue(empty.isEmpty()))
	})
	t.Run("string-literal_isEmpty_=>_false", func(t *testing.T) {
		foo := toVS(`"foo"`)
		qt.Assert(t, qt.IsFalse(foo.isEmpty()))
	})

	t.Run("bool_type_isEmpty_=>_false", func(t *testing.T) {
		bb := toVS(`bool`)
		qt.Assert(t, qt.IsFalse(bb.isEmpty()))
	})

	t.Run("without_entire_set_=>_empty", func(t *testing.T) {
		a := toVS(`true | false`)
		b := toVS(`bool`) // removing all bool
		qt.Assert(t, qt.IsTrue(a.without(b).isEmpty()))
	})
}

// deepEquals is a small helper that creates a checker
// for comparing two values by deep equality (including unexported fields).
func deepEquals[T any](got, want T) qt.Checker {
	return qt.CmpEquals(got, want,
		cmp.AllowUnexported(valueSet{}),
		cmp.Comparer(func(s1, s2 IntSet) bool {
			if s1 == nil {
				panic("nil s1")
			}
			if s2 == nil {
				panic("nil s2")
			}
			// TODO see issue https://github.com/google/go-cmp/issues/161
			// which makes this much less useful.
			if s1.Len() != s2.Len() {
				return false
			}
			for k := range s1.Values() {
				if !s2.Has(k) {
					return false
				}
			}
			return true
		}),
	)
}

// We'll define a helper for clarity.
func toVS(expr string) valueSet {
	v := cuecontext.New().CompileString(expr)
	if err := v.Err(); err != nil && expr != "_|_" {
		panic(err)
	}
	return valueSetForValue(v)
}
