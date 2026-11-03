package jsonata

import (
	"context"
	"fmt"

	"github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/internal/budget"
	"github.com/mellowforge/jsontrans-go/internal/document"
	"github.com/mellowforge/jsontrans-go/internal/jsonvalue"
	"github.com/mellowforge/jsontrans-go/internal/prepared"
)

// Compiled batches retain syntax and singular-prefix metadata only. Neither
// document nodes nor expression results survive a batch evaluation.
const maxBatchPrefixes = 4096
const maxBatchCompileWork = 100000

type batchPrefix struct {
	parent int
	name   string
}
type preparedBatchPlan struct {
	handlers []*prepared.Handler
	programs []*Program
	fast     []*fastPlan
	prefixes []batchPrefix
}
type batchNodeSlot struct {
	node                    jsontrans.Node
	ready, found, supported bool
}
type batchMemo struct {
	document *document.Document
	prefixes []batchPrefix
	slots    []batchNodeSlot
}

func compilePreparedBatch(ctx context.Context, handlers []*prepared.Handler) (prepared.BatchPlan, bool) {
	if ctx.Err() != nil || len(handlers) > maxBatchCompileWork {
		return nil, false
	}
	plan := &preparedBatchPlan{handlers: append([]*prepared.Handler(nil), handlers...), programs: make([]*Program, len(handlers)), fast: make([]*fastPlan, len(handlers)), prefixes: []batchPrefix{{}}}
	trie := make(map[batchPrefix]int)
	work := 0
	stopped := false
	tick := func() bool {
		work++
		if work > maxBatchCompileWork || work&31 == 0 && ctx.Err() != nil {
			stopped = true
			return false
		}
		return true
	}
	for i, h := range handlers {
		if !tick() {
			return nil, false
		}
		if h == nil {
			return nil, false
		}
		p, ok := h.Owner().(*Program)
		if !ok || p == nil || p.prepared != h {
			return nil, false
		}
		plan.programs[i] = p
		if p.fast == nil {
			continue
		}
		ids := make(map[*node]int)
		var primary *node
		var primaryID int
		var pathID func(*node) int
		pathID = func(n *node) int {
			if !tick() {
				return 0
			}
			if id, ok := ids[n]; ok {
				return id
			}
			var key batchPrefix
			switch n.op {
			case "name":
				key.name = n.val.(string)
			case ".":
				if n.children[1].op != "name" {
					return 0
				}
				key.parent = pathID(n.children[0])
				if key.parent == 0 {
					return 0
				}
				key.name = n.children[1].val.(string)
			default:
				return 0
			}
			id := trie[key]
			if id == 0 {
				if len(plan.prefixes) >= maxBatchPrefixes {
					return 0
				}
				id = len(plan.prefixes)
				trie[key] = id
				plan.prefixes = append(plan.prefixes, key)
			}
			ids[n] = id
			return id
		}
		var visit func(*node) bool
		visit = func(n *node) bool {
			if !tick() {
				return false
			}
			if id := pathID(n); id != 0 && primary == nil {
				primary = n
				primaryID = id
			}
			if stopped {
				return false
			}
			for _, child := range n.children {
				if !visit(child) {
					return false
				}
			}
			return true
		}
		if !visit(p.root) {
			return nil, false
		}
		copy := *p.fast
		copy.memoIDs = ids
		copy.memoPrimary = primary
		copy.memoPrimaryID = primaryID
		plan.fast[i] = &copy
	}
	if ctx.Err() != nil {
		return nil, false
	}
	return plan, true
}

func (p *preparedBatchPlan) NewState(d *document.Document) any {
	return &batchMemo{document: d, prefixes: p.prefixes, slots: make([]batchNodeSlot, len(p.prefixes))}
}

func (p *preparedBatchPlan) Eval(index int, req prepared.Request, state any) (result prepared.Result, err error) {
	program := p.programs[index]
	if err = req.CheckEntry(); err != nil {
		return result, program.wrapTimebox(err)
	}
	fast := p.fast[index]
	memo, ok := state.(*batchMemo)
	if !ok || memo.document != req.Document || fast == nil || program.engine.evaluationDepth != 0 || len(program.engine.functions) != 0 || req.Customized || req.Bindings != nil || req.Document.Stats().Nodes >= 10000 {
		return p.handlers[index].Eval(req)
	}
	if req.Document.Stats().Binary64Overflow {
		return result, fmt.Errorf("%w: number out of binary64 range", jsontrans.ErrInvalidInput)
	}
	r, handled, err := fast.evalWithMemo(req.Document.Root(), req.Budget, memo)
	if err != nil {
		return result, err
	}
	if !handled {
		// The attempted pure walk already consumed its accounting. Skip a second
		// fast walk while retaining the native materialization/evaluation fallback.
		req.Customized = true
		return p.handlers[index].Eval(req)
	}
	if !r.Defined {
		return result, req.CheckExit()
	}
	result.Value, result.Bytes, err = jsonvalue.CloneDetachedSize(r.Value, req.Budget, req.Limits.MaxOutputBytes)
	if err != nil {
		return prepared.Result{}, fmt.Errorf("%w: %w", jsontrans.ErrConversion, err)
	}
	result.Defined = true
	return result, req.CheckExit()
}

func (m *batchMemo) resolve(id int, b *budget.Budget) (jsontrans.Node, bool, bool, error) {
	if err := b.Enter(); err != nil {
		return jsontrans.Node{}, false, false, err
	}
	defer b.Leave()
	slot := &m.slots[id]
	// A cache hit is one bounded lookup. The eliminated parent traversal does
	// not consume work or depth again; every use still checks its local budget.
	if slot.ready {
		return slot.node, slot.found, slot.supported, nil
	}
	prefix := m.prefixes[id]
	focus := m.document.Root()
	if prefix.parent != 0 {
		var found, ok bool
		var err error
		focus, found, ok, err = m.resolve(prefix.parent, b)
		if err != nil || !found || !ok {
			return focus, found, ok, err
		}
		if err = b.Enter(); err != nil {
			return jsontrans.Node{}, false, false, err
		}
		defer b.Leave()
	}
	slot.ready = true
	slot.supported = focus.Kind() != "array"
	if slot.supported {
		slot.node, slot.found = focus.Member(prefix.name)
	}
	return slot.node, slot.found, slot.supported, nil
}
