package budget

import "context"

// SetContext attaches metadata to an existing evaluation without resetting its
// counters, limits, deadline, or cleanup ownership. ctx must be derived from
// Context() and must preserve its deadline and cancellation. Only trusted
// internal callers may replace a budget's context.
func (b *Budget) SetContext(ctx context.Context) {
	if ctx == nil {
		panic("nil budget context")
	}
	b.ctx = ctx
}
