package javascript_test

import (
	"context"
	"encoding/json"
	"errors"
	jt "github.com/mellowforge/jsontrans-go"
	js "github.com/mellowforge/jsontrans-go/javascript"
	"testing"
	"time"
)

func TestPublicProcessorRegistry(t *testing.T) {
	ctx := context.Background()
	e, err := js.New(ctx, js.Config{Limits: jt.Limits{Timeout: 10 * time.Second}})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	x, err := jt.New(map[string]jt.Processor{"js": e})
	if err != nil {
		t.Fatal(err)
	}
	p, err := x.Compile(ctx, "js", `(input,bindings)=>input.x+bindings.n`)
	if err != nil {
		t.Fatal(err)
	}
	o := jt.EvalOptions{Bindings: map[string]any{"n": 2}, Limits: &jt.Limits{Timeout: time.Second}}
	r, err := jt.EvalJSON(ctx, p, []byte(`{"x":3}`), o)
	if err != nil || string(r.Value) != "5" {
		t.Fatalf("JSON: %+v %v", r, err)
	}
	doc, err := jt.PrepareJSON(ctx, []byte(`{"x":3}`), jt.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := jt.NewBatch([]jt.Program{p})
	if err != nil {
		t.Fatal(err)
	}
	results, err := batch.EvalPrepared(ctx, doc, nil, o)
	if err != nil || results[0].Value != json.Number("5") {
		t.Fatalf("batch: %+v %v", results, err)
	}
	bad, err := e.Compile(ctx, `()=>{throw new TypeError('sentinel')}`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = bad.Eval(ctx, nil, jt.EvalOptions{})
	var ex *js.Exception
	if !errors.As(err, &ex) || ex.Name != "TypeError" {
		t.Fatal(err)
	}
	if _, err = p.Eval(ctx, nil, jt.EvalOptions{Limits: &jt.Limits{MaxSteps: 1}}); !errors.Is(err, jt.ErrUnsupported) {
		t.Fatal(err)
	}
	if err = e.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = p.Eval(ctx, nil, jt.EvalOptions{}); !errors.Is(err, js.ErrClosed) {
		t.Fatal(err)
	}
}
