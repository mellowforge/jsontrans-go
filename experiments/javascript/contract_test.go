package javascript

import (
	"context"
	"errors"
	jt "github.com/mellowforge/jsontrans-go"
	"strings"
	"testing"
	"time"
)

func TestOwnedCompileDoesNotRunInitialization(t *testing.T) {
	e := newTestEngine(t, OwnedQuickJS)
	p, err := e.Compile(context.Background(), `(()=>{throw new TypeError('init');})()`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Eval(context.Background(), nil, jt.EvalOptions{}); err == nil || !strings.Contains(err.Error(), "init") {
		t.Fatalf("initializer: %v", err)
	}
	p, err = e.Compile(context.Background(), `42`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Eval(context.Background(), nil, jt.EvalOptions{}); !errors.Is(err, jt.ErrConversion) {
		t.Fatalf("noncallable: %v", err)
	}
	p, err = e.Compile(context.Background(), `(()=>{while(true){}})()`)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err = p.Eval(ctx, nil, jt.EvalOptions{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("initializer cancellation: %v", err)
	}
}
