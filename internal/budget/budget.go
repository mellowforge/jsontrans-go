// Package budget provides cooperative, evaluation-local execution accounting.
package budget

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var ErrLimit = errors.New("resource limit exceeded")

// Limits bound one compilation or evaluation. Engine options set defaults, and
// a per-evaluation override replaces each nonzero field, raising or lowering
// it. Zero fields keep the base value (library defaults when there is none);
// negative fields are invalid. Larger positive values permit trusted workloads.
type Limits struct {
	Timeout            time.Duration
	MaxExpressionBytes int
	MaxDepth           int
	MaxSteps           int64
	MaxCollectionItems int
	MaxInputBytes      int
	MaxOutputBytes     int
	RegexTimeout       time.Duration
}

func DefaultLimits() Limits {
	return Limits{time.Second, 64 << 10, 256, 1_000_000, 100_000, 16 << 20, 16 << 20, 50 * time.Millisecond}
}

// Resolve applies nonzero overrides to the base configuration.
func Resolve(base Limits, override *Limits) (Limits, error) {
	if base == (Limits{}) {
		base = DefaultLimits()
	}
	if override == nil {
		override = &Limits{}
	}
	v := *override
	if v.Timeout < 0 || v.MaxExpressionBytes < 0 || v.MaxDepth < 0 || v.MaxSteps < 0 || v.MaxCollectionItems < 0 || v.MaxInputBytes < 0 || v.MaxOutputBytes < 0 || v.RegexTimeout < 0 {
		return Limits{}, fmt.Errorf("%w: negative limit", ErrLimit)
	}
	if v.Timeout != 0 {
		base.Timeout = v.Timeout
	}
	if v.MaxExpressionBytes != 0 {
		base.MaxExpressionBytes = v.MaxExpressionBytes
	}
	if v.MaxDepth != 0 {
		base.MaxDepth = v.MaxDepth
	}
	if v.MaxSteps != 0 {
		base.MaxSteps = v.MaxSteps
	}
	if v.MaxCollectionItems != 0 {
		base.MaxCollectionItems = v.MaxCollectionItems
	}
	if v.MaxInputBytes != 0 {
		base.MaxInputBytes = v.MaxInputBytes
	}
	if v.MaxOutputBytes != 0 {
		base.MaxOutputBytes = v.MaxOutputBytes
	}
	if v.RegexTimeout != 0 {
		base.RegexTimeout = v.RegexTimeout
	}
	return base, nil
}

// Budget must not be shared by concurrent evaluations.
type Budget struct {
	Limits Limits
	ctx    context.Context
	cancel context.CancelFunc
	steps  int64
	depth  int
	lazy   *deadlineContext
	fast   bool
}

func New(ctx context.Context, limits Limits) (*Budget, error) {
	if ctx == nil {
		return nil, errors.New("nil context")
	}
	l, err := Resolve(DefaultLimits(), &limits)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, l.Timeout)
	b := &Budget{Limits: l, ctx: ctx, cancel: cancel}
	if err := b.Check(); err != nil {
		cancel()
		return nil, err
	}
	return b, nil
}

func (b *Budget) Context() context.Context { return b.ctx }
func (b *Budget) Close() {
	if b.lazy != nil {
		b.lazy.close()
	} else if b.cancel != nil {
		b.cancel()
	}
}
func (b *Budget) Check() error { return b.ctx.Err() }
func (b *Budget) Step() error {
	// Fast plans check context at entry, exit and at most every 32 work units.
	// Step/depth limits still apply to every operation. This amortizes clock
	// reads without allowing an unbounded loop to ignore cancellation.
	if !b.fast || (b.steps+1)&31 == 0 {
		if err := b.Check(); err != nil {
			return err
		}
	}
	b.steps++
	if b.steps > b.Limits.MaxSteps {
		return fmt.Errorf("%w: steps", ErrLimit)
	}
	return nil
}
func (b *Budget) Enter() error {
	if err := b.Step(); err != nil {
		return err
	}
	if b.depth >= b.Limits.MaxDepth {
		return fmt.Errorf("%w: depth", ErrLimit)
	}
	b.depth++
	return nil
}
func (b *Budget) Leave() { b.depth-- }
func (b *Budget) Collection(n int) error {
	if err := b.Check(); err != nil {
		return err
	}
	if n < 0 || n > b.Limits.MaxCollectionItems {
		return fmt.Errorf("%w: collection size", ErrLimit)
	}
	return nil
}
func (b *Budget) Expression(s string) error {
	if err := b.Check(); err != nil {
		return err
	}
	if len(s) > b.Limits.MaxExpressionBytes {
		return fmt.Errorf("%w: expression size", ErrLimit)
	}
	return nil
}
