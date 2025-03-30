package cuediscrim

import (
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/go-quicktest/qt"
)

var buildDecisionTreeTests = []struct {
	testName string
	cue      string
	want     string
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
error
`,
}}

func TestBuildDecisionTree(t *testing.T) {
	for _, test := range buildDecisionTreeTests {
		t.Run(test.testName, func(t *testing.T) {
			ctx := cuecontext.New()
			val := ctx.CompileString(test.cue)
			qt.Assert(t, qt.IsNil(val.Err()))

			tree := Discriminate(val)
			qt.Assert(t, qt.Equals(NodeString(tree), strings.TrimPrefix(test.want, "\n")))
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
