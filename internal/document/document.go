// Package document stores an immutable, indexed JSON document.
package document

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/mellowforge/jsontrans-go/internal/budget"
	"github.com/mellowforge/jsontrans-go/internal/jsonvalue"
)

type Options struct{ Limits *budget.Limits }
type Stats struct {
	Bytes, Depth, MaxCollectionItems, Nodes int
	Binary64Overflow                        bool
}
type member struct {
	name string
	node int
}

// Arrays and objects use an offset/count into their respective arenas. Strings
// use itemsStart for a decoded-string index, or -1 for a view into owned raw JSON.
// Scalar nodes carry no slices, strings, or maps.
type entry struct {
	kind       byte
	start, end int
	itemsStart int
	itemsLen   int
	number     float64
	overflow   bool
}
type Document struct {
	raw      string
	nodes    []entry
	children []int
	members  []member
	strings  []string
	lookups  map[int]map[string]int
	stats    Stats
}
type Node struct {
	doc *Document
	id  int
}

func (d *Document) JSON() string {
	if d == nil {
		return ""
	}
	return d.raw
}
func (d *Document) Root() Node {
	if d == nil || len(d.nodes) == 0 {
		return Node{}
	}
	return Node{d, 0}
}
func (d *Document) Stats() Stats {
	if d == nil {
		return Stats{}
	}
	return d.stats
}
func (d *Document) CheckLimits(ctx context.Context, l budget.Limits) error {
	if ctx == nil {
		return fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if d == nil || len(d.nodes) == 0 {
		return fmt.Errorf("%w: unprepared document", jsonvalue.ErrInvalid)
	}
	limits, err := budget.Resolve(budget.DefaultLimits(), &l)
	if err != nil {
		return err
	}
	if d.stats.Bytes > limits.MaxInputBytes || d.stats.Depth > limits.MaxDepth || d.stats.MaxCollectionItems > limits.MaxCollectionItems {
		return fmt.Errorf("%w: prepared input dimensions", budget.ErrLimit)
	}
	return nil
}
func (n Node) data() *entry {
	if n.doc == nil || n.id < 0 || n.id >= len(n.doc.nodes) {
		return nil
	}
	return &n.doc.nodes[n.id]
}
func (n Node) Kind() string {
	e := n.data()
	if e == nil {
		return ""
	}
	switch e.kind {
	case 'o':
		return "object"
	case 'a':
		return "array"
	case 's':
		return "string"
	case 'n':
		return "number"
	case 'b':
		return "boolean"
	case 'z':
		return "null"
	}
	return ""
}
func (n Node) JSON() string {
	e := n.data()
	if e == nil {
		return ""
	}
	return n.doc.raw[e.start:e.end]
}
func (n Node) Member(name string) (Node, bool) {
	e := n.data()
	if e == nil || e.kind != 'o' {
		return Node{}, false
	}
	if lookup := n.doc.lookups[n.id]; lookup != nil {
		idx, ok := lookup[name]
		if !ok {
			return Node{}, false
		}
		return Node{n.doc, n.doc.members[e.itemsStart+idx].node}, true
	}
	for _, m := range n.doc.members[e.itemsStart : e.itemsStart+e.itemsLen] {
		if m.name == name {
			return Node{n.doc, m.node}, true
		}
	}
	return Node{}, false
}
func (n Node) Index(i int) (Node, bool) {
	e := n.data()
	if e == nil || e.kind != 'a' || i < 0 || i >= e.itemsLen {
		return Node{}, false
	}
	return Node{n.doc, n.doc.children[e.itemsStart+i]}, true
}
func (n Node) Len() int {
	e := n.data()
	if e == nil {
		return 0
	}
	if e.kind == 'a' {
		return e.itemsLen
	}
	if e.kind == 'o' {
		return e.itemsLen
	}
	return 0
}
func (n Node) Keys() []string {
	e := n.data()
	if e == nil || e.kind != 'o' {
		return nil
	}
	out := make([]string, e.itemsLen)
	for i, m := range n.doc.members[e.itemsStart : e.itemsStart+e.itemsLen] {
		out[i] = strings.Clone(m.name)
	}
	return out
}
func (n Node) Text() (string, bool) {
	e := n.data()
	if e == nil || e.kind != 's' {
		return "", false
	}
	if e.itemsStart < 0 {
		return n.doc.raw[e.start+1 : e.end-1], true
	}
	return n.doc.strings[e.itemsStart], true
}
func (n Node) Number() (json.Number, bool) {
	e := n.data()
	if e == nil || e.kind != 'n' {
		return "", false
	}
	return json.Number(n.doc.raw[e.start:e.end]), true
}
func (n Node) Binary64() (float64, bool) {
	e := n.data()
	if e == nil || e.kind != 'n' || e.overflow {
		return 0, false
	}
	return e.number, true
}
func (n Node) Boolean() (bool, bool) {
	e := n.data()
	if e == nil || e.kind != 'b' {
		return false, false
	}
	return n.doc.raw[e.start] == 't', true
}

func (d *Document) Materialize(ctx context.Context, o Options) (any, error) {
	l, err := budget.Resolve(budget.DefaultLimits(), o.Limits)
	if err != nil {
		return nil, err
	}
	b, err := budget.New(ctx, l)
	if err != nil {
		return nil, err
	}
	defer b.Close()
	return MaterializeWithBudget(d, b)
}

// MaterializeWithBudget materializes using the caller's existing scope, so
// input conversion shares its cancellation, deadline, and work counters.
func MaterializeWithBudget(d *Document, b *budget.Budget) (any, error) {
	if b == nil {
		return nil, fmt.Errorf("%w: nil budget", jsonvalue.ErrInvalid)
	}
	if err := d.CheckLimits(b.Context(), b.Limits); err != nil {
		return nil, err
	}
	return d.Root().materialize(b)
}

// MaterializeNodeWithBudget converts only a selected subtree while retaining
// the whole document's input dimension checks. The caller supplies its current
// evaluation budget, so traversal shares work, depth and cancellation accounting.
func MaterializeNodeWithBudget(n Node, b *budget.Budget) (any, error) {
	if b == nil {
		return nil, fmt.Errorf("%w: nil budget", jsonvalue.ErrInvalid)
	}
	if err := n.doc.CheckLimits(b.Context(), b.Limits); err != nil {
		return nil, err
	}
	return n.materialize(b)
}

func (n Node) materialize(b *budget.Budget) (any, error) {
	if err := b.Enter(); err != nil {
		return nil, err
	}
	defer b.Leave()
	e := n.data()
	if e == nil {
		return nil, fmt.Errorf("%w: invalid node", jsonvalue.ErrInvalid)
	}
	switch e.kind {
	case 'z':
		return nil, nil
	case 'b':
		v, _ := n.Boolean()
		return v, nil
	case 'n':
		return json.Number(strings.Clone(n.JSON())), nil
	case 's':
		value, _ := n.Text()
		text := strings.Clone(value)
		if !utf8.ValidString(text) {
			return jsonvalue.UTF16String(text), nil
		}
		return text, nil
	case 'a':
		if err := b.Collection(e.itemsLen); err != nil {
			return nil, err
		}
		out := make([]any, e.itemsLen)
		for i, id := range n.doc.children[e.itemsStart : e.itemsStart+e.itemsLen] {
			v, err := (Node{n.doc, id}).materialize(b)
			if err != nil {
				return nil, err
			}
			out[i] = v
		}
		return out, nil
	case 'o':
		if err := b.Collection(e.itemsLen); err != nil {
			return nil, err
		}
		out := make(map[string]any, e.itemsLen)
		keys := make([]string, 0, e.itemsLen)
		needsUTF16 := false
		for _, m := range n.doc.members[e.itemsStart : e.itemsStart+e.itemsLen] {
			v, err := (Node{n.doc, m.node}).materialize(b)
			if err != nil {
				return nil, err
			}
			name := strings.Clone(m.name)
			if !utf8.ValidString(name) {
				needsUTF16 = true
			}
			out[name] = v
			keys = append(keys, name)
		}
		jsonvalue.RememberObject(b.Context(), out, keys)
		if needsUTF16 {
			return jsonvalue.UTF16Object(out), nil
		}
		return out, nil
	}
	return nil, fmt.Errorf("%w: invalid node kind", jsonvalue.ErrInvalid)
}
