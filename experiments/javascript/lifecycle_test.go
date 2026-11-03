package javascript

import (
	"context"
	"errors"
	"sync"
	"testing"

	jt "github.com/mellowforge/jsontrans-go"
)

func TestOwnedConstructorContextLifetime(t *testing.T) {
	if _, err := NewOwned(nil, Config{}); err == nil {
		t.Fatal("nil context accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewOwned(ctx, Config{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled initialization: %v", err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	e, err := NewOwned(ctx, Config{Limits: jt.Limits{Timeout: semanticTestTimeout}})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	cancel()
	p, err := e.Compile(context.Background(), `x=>x`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Eval(context.Background(), nil, jt.EvalOptions{}); err != nil {
		t.Fatal(err)
	}
}
func TestOwnedCloseWaitsForCompleteCalls(t *testing.T) {
	life := newEngineLifetime()
	ctx, done, err := life.begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cleanup := errors.New("cleanup")
	finished := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() { finished <- life.close(func() error { return cleanup }) })
	}
	<-ctx.Done()
	if _, _, err = life.begin(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatalf("admission: %v", err)
	}
	select {
	case <-finished:
		t.Fatal("Close returned before host processing finished")
	default:
	}
	done()
	wg.Wait()
	for range 2 {
		if err = <-finished; !errors.Is(err, cleanup) {
			t.Fatal(err)
		}
	}
	if err = life.close(func() error { t.Fatal("repeated cleanup"); return nil }); !errors.Is(err, cleanup) {
		t.Fatal(err)
	}
}
func TestOwnedClosePreservesContextCauses(t *testing.T) {
	e := newTestEngine(t, OwnedQuickJS)
	p, err := e.Compile(context.Background(), `x=>x`)
	if err != nil {
		t.Fatal(err)
	}
	cyclic := map[string]any{}
	cyclic["self"] = cyclic
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = p.Eval(ctx, cyclic, jt.EvalOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	if err = e.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = p.Eval(ctx, cyclic, jt.EvalOptions{}); !errors.Is(err, ErrClosed) {
		t.Fatalf("closure: %v", err)
	}
	if _, err = e.Compile(ctx, `bad syntax`); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed compile: %v", err)
	}
}
