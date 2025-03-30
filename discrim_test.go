package cuediscrim

import (
	"os"
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/go-quicktest/qt"
)

type dataTest struct {
	name string
	cue  string
	want intSet
}

var buildDecisionTreeTests = []struct {
	testName string
	cue      string
	want     string
	data     []dataTest
}{{
	testName: "SimpleKinds",
	cue:      `string | int`,
	want: `
switch kind(.) {
case int:
	choose({1})
case string:
	choose({0})
}
`,
	data: []dataTest{{
		name: "int",
		cue:  "123",
		want: setOf(1),
	}, {
		name: "string",
		cue:  `"foo"`,
		want: setOf(0),
	}, {
		name: "error",
		cue:  `true`,
		want: nil,
	}},
}, {
	testName: "SimpleValues",
	cue:      `"foo" | "bar" | true`,
	want: `
switch . {
case "bar":
	choose({1})
case "foo":
	choose({0})
case true:
	choose({2})
default:
	error
}
`,
	data: []dataTest{{
		name: "bar",
		cue:  `"bar"`,
		want: setOf(1),
	}, {
		name: "foo",
		cue:  `"foo"`,
		want: setOf(0),
	}, {
		name: "true",
		cue:  `true`,
		want: setOf(2),
	}, {
		name: "other",
		cue:  `{}`,
	}},
}, {
	testName: "ValuesAndTypes",
	cue:      `int | bool | (null | bytes) | "foo" | "bar"`,
	want: `
switch . {
case "bar":
	choose({4})
case "foo":
	choose({3})
default:
	switch kind(.) {
	case null:
		choose({2})
	case bool:
		choose({1})
	case int:
		choose({0})
	case bytes:
		choose({2})
	}
}
`,
	data: []dataTest{{
		name: "bar",
		cue:  `"bar"`,
		want: setOf(4),
	}, {
		name: "foo",
		cue:  `"foo"`,
		want: setOf(3),
	}, {
		name: "null",
		cue:  `null`,
		want: setOf(2),
	}, {
		name: "true",
		cue:  `true`,
		want: setOf(1),
	}, {
		name: "other",
		cue:  `1.2`,
	}},
}, {
	testName: "TwoStructs",
	cue: `
{
	type!: "foo"
	a?: int
} | {
	type!: "bar"
	b?: bool
}`,
	want: `
switch type {
case "bar":
	choose({1})
case "foo":
	choose({0})
default:
	error
}
`,
	data: []dataTest{{
		name: "withFoo",
		cue:  `{type: "foo", a: 3}`,
		want: setOf(0),
	}, {
		name: "withBar",
		cue:  `{type: "bar", b: false}`,
		want: setOf(1),
	}, {
		name: "withOther",
		cue:  `{type: "other"}`,
	}},
}, {
	testName: "StructsWithNestedDiscriminator",
	cue: `
{
	discrim!: kind!: "foo"
	a?: int
} | {
	discrim!: kind!: "bar"
	b?: bool
}`,
	want: `
switch discrim.kind {
case "bar":
	choose({1})
case "foo":
	choose({0})
default:
	error
}
`,
	data: []dataTest{{
		name: "withFoo",
		cue:  `{discrim: kind: "foo", a: 3}`,
		want: setOf(0),
	}, {
		name: "withBar",
		cue:  `{discrim: kind: "bar", a: 3}`,
		want: setOf(1),
	}, {
		name: "withOther",
		cue:  `{type: "other"}`,
	}},
}, {
	testName: "StructsWithSeveralPotentialDiscriminators",
	cue: `
{
	a!: int
	b!: string
	c!: "one"
} | {
	a!: >5
	b!: bool
	c!: "one"
}`,
	want: `
switch kind(b) {
case bool:
	choose({1})
case string:
	choose({0})
}
`,
}, {
	testName: "StructsWithOtherTypes",
	cue: `
{
	a!: int
	b!: string
	c!: "one"
} | {
	a!: >5
	b!: bool
	c!: "one"
} | string | null`,
	want: `
switch kind(.) {
case null:
	choose({3})
case string:
	choose({2})
case struct:
	switch kind(b) {
	case bool:
		choose({1})
	case string:
		choose({0})
	}
}
`,
}, {
	testName: "PairwiseDiscriminator",
	cue: `
{
	a!: "foo"
	b!: true
	c?: int
} | {
	a!: "foo"
	b!: false
	c?: string
} | {
	a!: "bar"
	b!: true
	d?: string
}
`,
	want: `
choose({0, 1, 2})
`}, {
	testName: "MatchN",
	cue:      `matchN(1, [true, false])`,
	want: `
switch . {
case false:
	choose({1})
case true:
	choose({0})
default:
	error
}
`,
}, {
	testName: "MultipleDisjointStructs",
	cue: `
{a!: int} | {b!: string} | {c!: bool}
`,
	want: `
allOf {
	notPresent(a) -> {1, 2}
	notPresent(b) -> {0, 2}
	notPresent(c) -> {0, 1}
}
`,
	data: []dataTest{{
		name: "hasA",
		cue:  `{a: 5}`,
		want: setOf(0),
	}, {
		name: "hasB",
		cue:  `{b: "ff"}`,
		want: setOf(1),
	}, {
		name: "hasC",
		cue:  `{b: "ff"}`,
		want: setOf(1),
	}, {
		name: "hasAB",
		cue:  `{a: 1, b: "x"}`,
		want: setOf(0, 1),
	}, {
		name: "hasAll",
		cue:  `{a: 1, b: "x", c: true}`,
		want: setOf(0, 1, 2),
	}, {
		name: "hasDifferentType",
		cue:  `{a: true}`,
		want: setOf(0),
	}},
}}

func TestBuildDecisionTree(t *testing.T) {
	if testing.Verbose() {
		LogTo(os.Stderr)
	}
	for _, test := range buildDecisionTreeTests {
		t.Run(test.testName, func(t *testing.T) {
			ctx := cuecontext.New()
			val := ctx.CompileString(test.cue)
			qt.Assert(t, qt.IsNil(val.Err()))

			tree := Discriminate(val)
			qt.Assert(t, qt.Equals(NodeString(tree), strings.TrimPrefix(test.want, "\n")))

			for _, dtest := range test.data {
				t.Run(dtest.name, func(t *testing.T) {
					data := ctx.CompileString(dtest.cue)
					got := tree.Check(data)
					qt.Assert(t, qt.DeepEquals(got, dtest.want))
				})
			}
		})
	}
}

func TestIndentWriter(t *testing.T) {
	var buf strings.Builder
	w := &indentWriter{
		w: &buf,
	}
	w.Printf("hello {")
	w.Indent()
	w.Printf("foo\nbar {")
	w.Indent()
	w.Write([]byte("some\ntext\nwritten"))
	w.Write([]byte(" directly\n"))
	w.Unindent()
	w.Printf("}")
	w.Unindent()
	w.Printf("} something")
	qt.Assert(t, qt.Equals(buf.String(), `
hello {
	foo
	bar {
		some
		text
		written directly
	}
} something
`[1:]))
}

func setOf[T comparable](xs ...T) set[T] {
	if len(xs) == 0 {
		return nil
	}
	s := make(set[T])
	for _, x := range xs {
		s[x] = true
	}
	return s
}
