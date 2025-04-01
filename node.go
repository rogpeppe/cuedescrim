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
	Possible() IntSet
	// Check returns the chosen arms for the given value.
	Check(v cue.Value) IntSet
	write(w *indentWriter)
}

// NodeString returns a string representation of a node,
// showing pseudo-code about the decisions that can be taken.
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

// LeafNode represents a terminal node, which can contain one or more arms (if indistinguishable).
type LeafNode struct {
	// Arms holds the indexes of the disjunction that
	// are selected when this leaf is reached.
	// If fully discriminated, it’s usually 1 index.
	// If multiple arms remain indistinguishable, they’re all listed here.
	Arms IntSet
}

func (l *LeafNode) write(w *indentWriter) {
	w.Printf("choose(%v)", setString(l.Arms))
}

func (l *LeafNode) Check(v cue.Value) IntSet {
	return l.Arms
}

func (l *LeafNode) Possible() IntSet {
	return l.Arms
}

// KindSwitchNode handles switching on the top-level CUE kind of a path.
type KindSwitchNode struct {
	Path     string
	Branches map[cue.Kind]DecisionNode
}

func (n *KindSwitchNode) Possible() IntSet {
	return fold(iterMap(maps.Values(n.Branches), DecisionNode.Possible), union[int])
}

func (n *KindSwitchNode) Check(v cue.Value) IntSet {
	f := lookupPath(v, n.Path)
	if sub, ok := n.Branches[f.Kind()]; ok {
		return sub.Check(v)
	}
	return wordSet(0)
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

// FieldAbsenceNode tests for the absence of a set of paths
// and uses the resulting information to infer the selected arms.
type FieldAbsenceNode struct {
	// Branches maps paths to the set of arms selected
	// if the field at that path is known not to exist.
	Branches map[string]IntSet
}

func (n *FieldAbsenceNode) Possible() IntSet {
	s := make(mapSet[int])
	for _, s1 := range n.Branches {
		s.addSeq(s1.Values())
	}
	return s
}

func (n *FieldAbsenceNode) Check(v cue.Value) IntSet {
	first := true
	var s IntSet = wordSet(0)
	for path, group := range n.Branches {
		if lookupPath(v, path).Exists() {
			continue
		}
		if first {
			s = group
			first = false
		} else {
			s = intersect(s, group)
		}
	}
	if first {
		// No non-existence test failed. Could be anything.
		return n.Possible()
	}
	return s
}

func (n *FieldAbsenceNode) write(w *indentWriter) {
	w.Printf("allOf {")
	w.Indent()
	for _, path := range slices.Sorted(maps.Keys(n.Branches)) {
		group := n.Branches[path]
		w.Printf("notPresent(%v) -> %s", path, setString(group))
	}
	w.Unindent()
	w.Printf("}")
}

// ValueSwitchNode tests for specific enumerated (atomic) values in a field.
type ValueSwitchNode struct {
	Path     string
	Branches map[Atom]DecisionNode // possible concrete values -> sub-node
	Default  DecisionNode
}

func (n *ValueSwitchNode) Possible() IntSet {
	return fold(iterMap(maps.Values(n.Branches), DecisionNode.Possible), union[int])
}

func (n *ValueSwitchNode) Check(v cue.Value) IntSet {
	f := lookupPath(v, n.Path)
	if f.Exists() && isAtomKind(f.Kind()) {
		if sub, ok := n.Branches[atomForValue(f)]; ok {
			return sub.Check(v)
		}
	}
	if n.Default != nil {
		return n.Default.Check(v)
	}
	return wordSet(0)
}

func (n *ValueSwitchNode) write(w *indentWriter) {
	w.Printf("switch %s {", n.Path)
	for _, val := range slices.SortedFunc(maps.Keys(n.Branches), Atom.compare) {
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

// isPerfect reports whether n is a "perfect" discriminator,
// in that any given value must result in a single arm chosen
// or an error.
// If noAtoms is true, it's still considered "perfect" if all the chosen
// arms are of the same atom type (it uses arms to determine that)
func isPerfect(n DecisionNode, noAtoms bool, arms []cue.Value) bool {
	switch n := n.(type) {
	case nil:
		return true
	case *LeafNode:
		if n.Arms.Len() <= 1 {
			return true
		}
		if !noAtoms {
			return false
		}
		var k cue.Kind
		for i := range n.Arms.Values() {
			v := arms[i]
			vk := v.Kind()
			if !isAtomKind(vk) {
				return false
			}
			if k != 0 && k != vk {
				return false
			}
			k = vk
		}
		// If all the arms have the same atom kind: we're still OK.
		return true
	case *KindSwitchNode:
		for _, n := range n.Branches {
			if !isPerfect(n, noAtoms, arms) {
				return false
			}
		}
		return true
	case *FieldAbsenceNode:
		return false
	case *ValueSwitchNode:
		for _, n := range n.Branches {
			if !isPerfect(n, noAtoms, arms) {
				return false
			}
		}
		return isPerfect(n.Default, noAtoms, arms)
	case *ErrorNode, ErrorNode:
		return true
	}
	panic(fmt.Errorf("unexpected node type %#v", n))
}

type ErrorNode struct{}

func (ErrorNode) Possible() IntSet {
	return nil
}

func (ErrorNode) Check(v cue.Value) IntSet {
	return wordSet(0)
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
