package jsontrans_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	jt "github.com/mellowforge/jsontrans-go"
)

func TestPreparedOwnsInputAndReadOnlyNodes(t *testing.T) {
	input := []byte(`{"a":1,"a":9007199254740993,"s":"\ud800","nested":[true,null]}`)
	doc, err := jt.PrepareJSON(context.Background(), input, jt.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	input[2] = 'z'
	if doc.Root().Kind() != "object" {
		t.Fatal("wrong root")
	}
	a, ok := doc.Root().Member("a")
	if !ok {
		t.Fatal("lost field")
	}
	n, ok := a.Number()
	if !ok || n != json.Number("9007199254740993") {
		t.Fatalf("%v", n)
	}
	keys := doc.Root().Keys()
	keys[0] = "changed"
	if doc.Root().Keys()[0] != "a" {
		t.Fatal("keys alias")
	}
	s, _ := doc.Root().Member("s")
	text, _ := s.Text()
	if text != "\xed\xa0\x80" {
		t.Fatalf("%q", text)
	}
	nested, _ := doc.Root().Member("nested")
	v, _ := nested.Index(0)
	b, ok := v.Boolean()
	if !ok || !b {
		t.Fatal("bool")
	}
	if _, ok := nested.Index(-1); ok {
		t.Fatal("negative index")
	}
	if _, ok := a.Member("x"); ok {
		t.Fatal("wrong kind accepted")
	}
	if a.Len() != 0 || (jt.Node{}).Kind() != "" {
		t.Fatal("zero/wrong kind")
	}
}
func TestPreparedValidationAndLimits(t *testing.T) {
	for _, s := range []string{`{}x`, `[1,]`, `{"a":01}`, `"\x"`, string([]byte{0xff}), `{"a":}`} {
		if _, err := jt.PrepareJSON(context.Background(), []byte(s), jt.PrepareOptions{}); !errors.Is(err, jt.ErrInvalidInput) {
			t.Fatalf("%q: %v", s, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := jt.PrepareJSON(ctx, []byte(`null`), jt.PrepareOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	doc, err := jt.PrepareJSON(context.Background(), []byte(`{"a":[1,2,3]}`), jt.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jt.EvalPrepared(context.Background(), identityProgram{}, doc, jt.EvalOptions{Limits: &jt.Limits{MaxCollectionItems: 2}}); !errors.Is(err, jt.ErrLimit) {
		t.Fatal(err)
	}
	if _, err := jt.EvalPrepared(context.Background(), nil, doc, jt.EvalOptions{}); err == nil {
		t.Fatal("nil program")
	}
	if _, err := jt.EvalPrepared(context.Background(), identityProgram{}, nil, jt.EvalOptions{}); err == nil {
		t.Fatal("nil document")
	}
}
func TestPreparedFallbackDetachedAndConcurrent(t *testing.T) {
	doc, err := jt.PrepareJSON(context.Background(), []byte(`{"a":[1,2]}`), jt.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := jt.EvalPrepared(context.Background(), identityProgram{}, doc, jt.EvalOptions{})
			if err != nil {
				t.Error(err)
				return
			}
			r.Value.(map[string]any)["a"].([]any)[0] = 99
		}()
	}
	wg.Wait()
	v, err := doc.Materialize(context.Background(), jt.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if v.(map[string]any)["a"].([]any)[0] != json.Number("1") {
		t.Fatal("document mutated")
	}
}

type nodeProcessor struct{}

func (nodeProcessor) Compile(context.Context, string) (jt.Program, error) { return nodeProgram{}, nil }

type nodeProgram struct{}

func (nodeProgram) Eval(context.Context, any, jt.EvalOptions) (jt.Result, error) {
	panic("prepared dispatch was lost")
}
func (nodeProgram) EvalPrepared(_ context.Context, d *jt.Document, _ jt.EvalOptions) (jt.Result, error) {
	n, ok := d.Root().Member("n")
	if !ok {
		return jt.Result{}, nil
	}
	v, _ := n.Number()
	return jt.Result{Value: v, Defined: true}, nil
}
func TestExternalPreparedCapabilitySurvivesWrapper(t *testing.T) {
	x, _ := jt.New(map[string]jt.Processor{"nodes": nodeProcessor{}})
	p, err := x.Compile(context.Background(), "nodes", "")
	if err != nil {
		t.Fatal(err)
	}
	d, err := jt.PrepareJSON(context.Background(), []byte(`{"n":42}`), jt.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	r, err := jt.EvalPrepared(context.Background(), p, d, jt.EvalOptions{})
	if err != nil || r.Value != json.Number("42") {
		t.Fatalf("%+v %v", r, err)
	}
}

type preparedBindingMutator struct{}

func (preparedBindingMutator) Eval(_ context.Context, _ any, o jt.EvalOptions) (jt.Result, error) {
	o.Bindings["x"].([]any)[0] = 2
	return jt.Result{Defined: true}, nil
}
func (b preparedBindingMutator) EvalPrepared(ctx context.Context, _ *jt.Document, o jt.EvalOptions) (jt.Result, error) {
	return b.Eval(ctx, nil, o)
}
func TestPreparedProtectsBindings(t *testing.T) {
	d, _ := jt.PrepareJSON(context.Background(), []byte(`null`), jt.PrepareOptions{})
	for _, p := range []jt.Program{preparedBindingMutator{}, legacyBindingMutator{}} {
		bindings := map[string]any{"x": []any{1}}
		if _, err := jt.EvalPrepared(context.Background(), p, d, jt.EvalOptions{Bindings: bindings}); err != nil {
			t.Fatal(err)
		}
		if bindings["x"].([]any)[0] != 1 {
			t.Fatal("caller binding mutated")
		}
	}
}

type legacyBindingMutator struct{}

func (legacyBindingMutator) Eval(ctx context.Context, v any, o jt.EvalOptions) (jt.Result, error) {
	return (preparedBindingMutator{}).Eval(ctx, v, o)
}

func TestPreparedCancelledMaterializationDoesNotPoisonDocument(t *testing.T) {
	d, _ := jt.PrepareJSON(context.Background(), []byte(`{"a":1}`), jt.PrepareOptions{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.Materialize(ctx, jt.PrepareOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := d.Materialize(context.Background(), jt.PrepareOptions{}); err != nil {
		t.Fatal(err)
	}
}

type deadlinePreparedProgram struct{ observed context.Context }

func (*deadlinePreparedProgram) Eval(context.Context, any, jt.EvalOptions) (jt.Result, error) {
	panic("wrong dispatch")
}
func (p *deadlinePreparedProgram) EvalPrepared(ctx context.Context, _ *jt.Document, _ jt.EvalOptions) (jt.Result, error) {
	p.observed = ctx
	<-ctx.Done()
	return jt.Result{Defined: true}, nil
}
func TestExternalPreparedReceivesWorkingDeadline(t *testing.T) {
	d, _ := jt.PrepareJSON(context.Background(), []byte(`null`), jt.PrepareOptions{})
	p := &deadlinePreparedProgram{}
	_, err := jt.EvalPrepared(context.Background(), p, d, jt.EvalOptions{Limits: &jt.Limits{Timeout: time.Millisecond}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if p.observed.Err() != context.DeadlineExceeded {
		t.Fatal(p.observed.Err())
	}
}
