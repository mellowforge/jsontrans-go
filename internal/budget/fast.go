package budget

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// NewFast uses monotonic deadline checks and creates a timer only when a
// consumer requests Done. Context callbacks and full-evaluator fallback still
// receive an ordinary usable deadline/cancellation context.
func NewFast(ctx context.Context, limits Limits) (*Budget, error) {
	if ctx == nil {
		return nil, errors.New("nil context")
	}
	l, err := Resolve(DefaultLimits(), &limits)
	if err != nil {
		return nil, err
	}
	b := new(Budget)
	if err := InitFastResolved(b, ctx, l); err != nil {
		return nil, err
	}
	return b, nil
}

// InitFastResolved initializes caller-owned evaluation state using already
// resolved limits. It never publishes the Budget pointer in a context.
// The Budget must not be shared or reused before Close.
func InitFastResolved(b *Budget, ctx context.Context, l Limits) error {
	return initFastResolved(b, ctx, l, true)
}

// InitFastOwnedResolved gives a callback-capable expression its own cancellation
// lifetime even when an enclosing batch has an earlier deadline.
func InitFastOwnedResolved(b *Budget, ctx context.Context, l Limits) error {
	return initFastResolved(b, ctx, l, false)
}

func initFastResolved(b *Budget, ctx context.Context, l Limits, borrowParent bool) error {
	if ctx == nil {
		return errors.New("nil context")
	}
	// An enclosing scope with an equal or shorter timeout was started earlier,
	// so its deadline already bounds this call without another clock read.
	if parent, ok := ctx.(*deadlineContext); borrowParent && ok && parent.timeout <= l.Timeout {
		*b = Budget{Limits: l, ctx: ctx, fast: true}
		return b.Check()
	}
	deadline := time.Now().Add(l.Timeout)
	if parent, ok := ctx.Deadline(); ok && !parent.After(deadline) {
		deadline = parent
	}
	lazy := &deadlineContext{parent: ctx, deadline: deadline, timeout: l.Timeout}
	*b = Budget{Limits: l, ctx: lazy, lazy: lazy, fast: true}
	if err := b.Check(); err != nil {
		b.Close()
		return err
	}
	return nil
}

type deadlineContext struct {
	parent   context.Context
	deadline time.Time
	timeout  time.Duration
	mu       sync.Mutex
	child    context.Context
	cancel   context.CancelFunc
	state    atomic.Int32 // 0 active, 1 canceled, 2 deadline exceeded, 3 finishing
}

func (c *deadlineContext) Deadline() (time.Time, bool) { return c.deadline, true }
func (c *deadlineContext) Value(key any) any           { return c.parent.Value(key) }
func (c *deadlineContext) Err() error {
	if state := c.state.Load(); state != 0 {
		return c.terminal(state)
	}
	if err := c.parent.Err(); err != nil {
		if err == context.DeadlineExceeded {
			return c.finish(2)
		}
		return c.finish(1)
	}
	// Until uses only the monotonic reading when present. Parent deadlines
	// without monotonic metadata retain time's wall-clock fallback.
	if time.Until(c.deadline) <= 0 {
		return c.finish(2)
	}
	return nil
}
func (c *deadlineContext) terminal(state int32) error {
	if state == 3 {
		// A Done consumer can awaken inside cancel(). Wait for publication of
		// the matching terminal reason before answering Err.
		c.mu.Lock()
		state = c.state.Load()
		c.mu.Unlock()
	}
	if state == 2 {
		return context.DeadlineExceeded
	}
	return context.Canceled
}
func (c *deadlineContext) finish(state int32) error {
	c.mu.Lock()
	if c.state.Load() == 0 {
		c.state.Store(3)
		if c.cancel != nil {
			c.cancel()
		}
		// Publish a terminal error only after an existing Done channel closes.
		c.state.Store(state)
	}
	c.mu.Unlock()
	return c.terminal(c.state.Load())
}
func (c *deadlineContext) Done() <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.child == nil {
		c.child, c.cancel = context.WithDeadline(c.parent, c.deadline)
		if c.state.Load() != 0 {
			c.cancel()
		}
	}
	return c.child.Done()
}
func (c *deadlineContext) close() {
	// Record an already observable deadline/parent cancellation before our own.
	_ = c.Err()
	_ = c.finish(1)
}
