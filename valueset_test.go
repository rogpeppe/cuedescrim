package main

import (
	"testing"

	"github.com/go-quicktest/qt"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/google/go-cmp/cmp"
)

var valueSetForValueTests = []struct {
	name string
	cue  string
	want valueSet
}{
	{
		name: "bool literal true",
		cue:  `true`,
		want: valueSet{
			consts: set[atom]{`true`: true},
		},
	},
	{
		name: "bool literal false",
		cue:  `false`,
		want: valueSet{
			consts: set[atom]{`false`: true},
		},
	},
	{
		name: "int literal 42",
		cue:  `42`,
		want: valueSet{
			consts: set[atom]{`42`: true},
		},
	},
	{
		name: "float literal 3.14",
		cue:  `3.14`,
		want: valueSet{
			consts: set[atom]{`3.14`: true},
		},
	},
	{
		name: "string literal hello",
		cue:  `"hello"`,
		want: valueSet{
			consts: set[atom]{`"hello"`: true},
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
			consts: set[atom]{`"hello"`: true, `"world"`: true},
		},
	},
	{
		name: "mixed or: 'foo' | bool",
		cue:  `"foo" | bool`,
		want: valueSet{
			types:  cue.BoolKind,
			consts: set[atom]{`"foo"`: true},
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
			consts: set[atom]{`"one"`: true, `"two"`: true},
		},
	},
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
				consts: set[atom]{`true`: true, `false`: true},
			},
		},
		{
			name: "intersect_of_'foo'_and_'foo'",
			a:    `"foo"`,
			b:    `"foo"`,
			op:   valueSet.intersect,
			want: valueSet{
				consts: set[atom]{`"foo"`: true},
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
				consts: set[atom]{`"a"`: true, "true": true},
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
				consts: set[atom]{`true`: true},
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
	return qt.CmpEquals(got, want, cmp.AllowUnexported(valueSet{}))
}

// We'll define a helper for clarity.
func toVS(expr string) valueSet {
	v := cuecontext.New().CompileString(expr)
	if err := v.Err(); err != nil {
		panic(err)
	}
	return valueSetForValue(v)
}
