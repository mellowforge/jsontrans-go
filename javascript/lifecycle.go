package javascript

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

type engineLifetime struct {
	ctx     context.Context
	cancel  context.CancelCauseFunc
	mu      sync.Mutex
	active  sync.WaitGroup
	closing atomic.Bool
	once    sync.Once
	err     error
}

func newEngineLifetime() *engineLifetime {
	ctx, cancel := context.WithCancelCause(context.Background())
	return &engineLifetime{ctx: ctx, cancel: cancel}
}
func (l *engineLifetime) begin(ctx context.Context) (context.Context, func(), error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closing.Load() {
		return nil, nil, ErrClosed
	}
	if ctx == nil {
		return nil, nil, errors.New("nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	l.active.Add(1)
	ctx, cancel := context.WithCancelCause(ctx)
	stop := context.AfterFunc(l.ctx, func() { cancel(ErrClosed) })
	return ctx, func() { stop(); cancel(context.Canceled); l.active.Done() }, nil
}
func (l *engineLifetime) failure(ctx context.Context, err error) error {
	if l.closing.Load() {
		return errors.Join(ErrClosed, ctx.Err(), err)
	}
	if ctx.Err() != nil {
		return errors.Join(ctx.Err(), err)
	}
	return err
}
func (l *engineLifetime) close(cleanup func() error) error {
	l.once.Do(func() {
		l.mu.Lock()
		l.closing.Store(true)
		l.cancel(ErrClosed)
		l.mu.Unlock()
		l.active.Wait()
		l.err = cleanup()
	})
	return l.err
}
