package cuediscrim

import (
	"testing"

	"github.com/go-quicktest/qt"
)

func TestIntSets(t *testing.T) {
	t.Run("mapSet", func(t *testing.T) {
		testIntSet(t, mapSetAPI[int]{})
	})
	t.Run("word", func(t *testing.T) {
		testIntSet(t, wordSetAPI{})
	})
}

func testIntSet[S any](t *testing.T, sets setAPI[S, int]) {
	t.Run("binop", func(t *testing.T) {
		testIntSetBinop(t, sets)
	})
	t.Run("has", func(t *testing.T) {
		s := sets.of(1, 3, 6)
		qt.Assert(t, qt.IsTrue(sets.has(s, 1)))
		qt.Assert(t, qt.IsFalse(sets.has(s, 0)))
	})
}

func testIntSetBinop[S any](t *testing.T, sets setAPI[S, int]) {
	tests := []struct {
		testName string
		a, b     S
		op       func(S, S) S
		want     S
	}{{
		testName: "intersect",
		a:        sets.of(1, 2),
		b:        sets.of(2, 3),
		op:       sets.intersect,
		want:     sets.of(2),
	}, {
		testName: "union",
		a:        sets.of(1, 2, 6),
		b:        sets.of(2, 3, 5, 7),
		op:       sets.union,
		want:     sets.of(1, 2, 3, 5, 6, 7),
	}}
	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			got := test.op(test.a, test.b)
			qt.Assert(t, qt.DeepEquals(got, test.want))
		})
	}
}
