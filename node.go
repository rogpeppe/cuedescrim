package cuediscrim

import (
	"bytes"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"cuelang.org/go/cue"
)

// DecisionNode is the interface for all discriminators (internal nodes) and leaf nodes.
type DecisionNode interface {
	// Possible returns the set of arms that this decision node can match.
	Possible() intSet
	// Check returns the chosen arms for the given value.
	Check(v cue.Value) intSet
	write(w *indentWriter)
}

func NodeString(n DecisionNode) string {
	if n == nil {
		return "<nil>"
	}
	var buf strings.Builder
	w := &indentWriter{
		w: &buf,
	}
	n.write(w)
	return buf.String()
}

// AllNode holds the result of checking all its component nodes.
type AllNode struct {
	Nodes []DecisionNode
}

func (n *AllNode) Check(v cue.Value) intSet {
	return fold(iterMap(slices.Values(n.Nodes), func(n DecisionNode) intSet {
		return n.Check(v)
	}), intSet.intersect)
}

func (n *AllNode) Possible() intSet {
	return fold(iterMap(slices.Values(n.Nodes), DecisionNode.Possible), intSet.intersect)
}

// LeafNode represents a terminal node, which can contain one or more arms (if indistinguishable).
type LeafNode struct {
	// Arms holds the indexes of the disjunction that
	// are selected when this leaf is reached.
	// If fully discriminated, it’s usually 1 index.
	// If multiple arms remain indistinguishable, they’re all listed here.
	Arms intSet
}

func (l *LeafNode) write(w *indentWriter) {
	w.Printf("choose(%v)", setString(l.Arms))
}

func (l *LeafNode) Check(v cue.Value) intSet {
	return l.Arms
}

func (l *LeafNode) Possible() intSet {
	return l.Arms
}

// KindSwitchNode handles switching on the top-level CUE kind of a path.
type KindSwitchNode struct {
	Path     string
	Branches map[cue.Kind]DecisionNode
}

func (n *KindSwitchNode) Possible() intSet {
	return fold(iterMap(maps.Values(n.Branches), DecisionNode.Possible), intSet.union)
}

func (n *KindSwitchNode) Check(v cue.Value) intSet {
	f := lookupPath(v, n.Path)
	if sub, ok := n.Branches[f.Kind()]; ok {
		return sub.Check(v)
	}
	return nil
}

func lookupPath(v cue.Value, path string) cue.Value {
	if path == "." || path == "" {
		return v
	}
	// TODO this doesn't work when a field name contains a dot.
	parts := strings.Split(path, ".")
	sels := make([]cue.Selector, len(parts))
	for i, part := range parts {
		sels[i] = cue.Str(part)
	}
	return v.LookupPath(cue.MakePath(sels...))
}

func (k *KindSwitchNode) write(w *indentWriter) {
	w.Printf("switch kind(%v) {", k.Path)
	for _, kind := range slices.Sorted(maps.Keys(k.Branches)) {
		node := k.Branches[kind]
		w.Printf("case %v:", kind)
		w.Indent()
		node.write(w)
		w.Unindent()

	}
	w.Printf("}")
}

var tabs = strings.Repeat("\t", 1000)

func indentStr(n int) string {
	return tabs[:n]
}

// FieldPresenceNode tests for the presence of a particular field (e.g. `a`).
type FieldPresenceNode struct {
	Path       string
	Present    DecisionNode
	NotPresent DecisionNode
}

func (n *FieldPresenceNode) Possible() intSet {
	var s intSet
	if n.Present != nil {
		s = s.union(n.Present.Possible())
	}
	if n.NotPresent != nil {
		s = s.union(n.NotPresent.Possible())
	}
	return s
}

func (n *FieldPresenceNode) Check(v cue.Value) intSet {
	f := lookupPath(v, n.Path)
	if f.Exists() {
		if n.Present != nil {
			return n.Present.Check(v)
		}
	} else {
		if n.NotPresent != nil {
			return n.NotPresent.Check(v)
		}
	}
	return nil
}

func (f *FieldPresenceNode) write(w *indentWriter) {
	switch {
	case !isError(f.Present) && !isError(f.NotPresent):
		w.Printf("if present(%v) {", f.Path)
		w.Indent()
		f.Present.write(w)
		w.Unindent()
		w.Printf("} else {")
		w.Indent()
		f.NotPresent.write(w)
		w.Unindent()
	case !isError(f.Present):
		w.Printf("if present(%v) {", f.Path)
		w.Indent()
		f.Present.write(w)
		w.Unindent()
		w.Printf("}")
	default:
		w.Printf("if !present(%v) {", f.Path)
		w.Indent()
		f.NotPresent.write(w)
		w.Unindent()
		w.Printf("}")
	}
}

// ValueSwitchNode tests for specific enumerated (atomic) values in a field.
type ValueSwitchNode struct {
	Path     string
	Branches map[atom]DecisionNode // possible concrete values -> sub-node
	Default  DecisionNode
}

func (n *ValueSwitchNode) Possible() intSet {
	return fold(iterMap(maps.Values(n.Branches), DecisionNode.Possible), intSet.union)
}

func (n *ValueSwitchNode) Check(v cue.Value) intSet {
	f := lookupPath(v, n.Path)
	if !f.Exists() || !isAtomKind(f.Kind()) {
		return nil
	}
	if sub, ok := n.Branches[atomForValue(f)]; ok {
		return sub.Check(v)
	}
	return nil
}

func (n *ValueSwitchNode) write(w *indentWriter) {
	w.Printf("switch %s {", n.Path)
	for _, val := range slices.Sorted(maps.Keys(n.Branches)) {
		node := n.Branches[val]
		w.Printf("case %v:", val)
		w.Indent()
		node.write(w)
		w.Unindent()
	}
	w.Printf("default:")
	w.Indent()
	n.Default.write(w)
	w.Unindent()
	w.Printf("}")
}

func isError(n DecisionNode) bool {
	return n == nil || n == ErrorNode{}
}

type ErrorNode struct{}

func (ErrorNode) Possible() intSet {
	return nil
}

func (ErrorNode) Check(v cue.Value) intSet {
	return nil
}

func (ErrorNode) write(w *indentWriter) {
	w.Printf("error")
}

type indentWriter struct {
	w       io.Writer
	indent  int
	midline bool
}

// Write implements [io.Writer]. All lines written
// will be indented by the current indent level.
func (w *indentWriter) Write(buf []byte) (int, error) {
	if w == nil {
		return len(buf), nil
	}
	totalWritten := 0
	for line := range bytes.SplitAfterSeq(buf, []byte("\n")) {
		if len(line) == 0 {
			// After final newline
			continue
		}
		if !w.midline {
			for range w.indent {
				if _, err := io.WriteString(w.w, "\t"); err != nil {
					return totalWritten, err
				}
			}
			w.midline = true
		}

		n, err := w.w.Write(line)
		totalWritten += n
		if err != nil {
			return totalWritten, err
		}
		if line[len(line)-1] == '\n' {
			w.midline = false
		}
	}
	return totalWritten, nil
}

// Indent increments the current indent level.
func (w *indentWriter) Indent() {
	if w == nil {
		return
	}
	w.indent++
}

// Unindent decrements the current indent level.
func (w *indentWriter) Unindent() {
	if w == nil {
		return
	}
	w.indent--
}

// Printf is eqivalent to w.Write([]byte(fmt.Sprintf(f, a...))
// but it always ensures that there's a final newline.
func (w *indentWriter) Printf(f string, a ...any) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, f, a...)
	if !strings.HasSuffix(f, "\n") {
		fmt.Fprintf(w, "\n")
	}
}
