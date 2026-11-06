package messages

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	jt "github.com/mellowforge/jsontrans-go"
)

func TestByteStrategiesPreserveSemantics(t *testing.T) {
	for _, tc := range []struct {
		engine, expr, input, want string
		defined                   bool
	}{
		{"jsonata", "missing", `{"n":null}`, "", false},
		{"jsonata", "n", `{"n":null}`, "null", true},
		{"jsonpath", "$.missing", `{"n":null}`, "[]", true},
		{"jsonpath", "$.n", `{"n":null}`, "[null]", true},
		{"jsonpath", "$.x", `{"x":9007199254740993}`, "[9007199254740993]", true},
		{"jsonata", "x", `{"x":9007199254740993}`, "9007199254740992", true},
		{"jsonata", "x", `{"x":"\ud800"}`, `"\ud800"`, true},
		{"jsonpath", "$.x", `{"x":"\ud800"}`, `["\ud800"]`, true},
		{"jsonata", "x", `{"x":1,"x":2}`, "2", true},
	} {
		t.Run(tc.engine+tc.expr+tc.input, func(t *testing.T) {
			p, err := NewPlan(context.Background(), Config{Engine: tc.engine, Expressions: []string{tc.expr}})
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close()
			for _, strategy := range Strategies {
				raw := []byte(tc.input)
				copyOfRaw := append([]byte(nil), raw...)
				worker := p.NewWorker()
				got, err := worker.Eval(context.Background(), raw, strategy)
				if err != nil {
					t.Fatal(strategy, err)
				}
				if len(got) != 1 || got[0].Defined != tc.defined || string(got[0].Value) != tc.want {
					t.Fatalf("%s: %#v want %s defined=%v", strategy, got, tc.want, tc.defined)
				}
				if !reflect.DeepEqual(raw, copyOfRaw) {
					t.Fatal("mutated input")
				}
				retained := append([]byte(nil), got[0].Value...)
				for i := range raw {
					raw[i] = 'x'
				}
				if string(got[0].Value) != string(retained) {
					t.Fatal("result borrowed input")
				}
			}
		})
	}
}

func TestByteStrategiesRejectBadInputAndLimits(t *testing.T) {
	for _, strategy := range Strategies {
		p, err := NewPlan(context.Background(), Config{Engine: "jsonata", Expressions: []string{"x", "x"}, Options: jt.EvalOptions{Limits: &jt.Limits{MaxOutputBytes: 100}}})
		if err != nil {
			t.Fatal(err)
		}
		worker := p.NewWorker()
		for _, raw := range []string{`{`, `{} {}`, `{"x":1,}`} {
			if got, err := worker.Eval(context.Background(), []byte(raw), strategy); err == nil || got != nil {
				t.Fatalf("%s accepted %s", strategy, raw)
			}
		}
		input := []byte(`{"x":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`)
		if got, err := worker.Eval(context.Background(), input, strategy); !errors.Is(err, jt.ErrLimit) || got != nil {
			t.Fatalf("%s aggregate output: %v", strategy, err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := worker.Eval(ctx, []byte(`{"x":1}`), strategy); !errors.Is(err, context.Canceled) {
			t.Fatalf("%s cancellation: %v", strategy, err)
		}
		if _, err := worker.Eval(context.Background(), []byte(`{"x":1}`), strategy); err != nil {
			t.Fatalf("recovery: %v", err)
		}
		p.Close()
	}
}

func TestByteStrategiesPreserveNativeErrorAndBindings(t *testing.T) {
	p, err := NewPlan(context.Background(), Config{Engine: "jsonata", Expressions: []string{"$sum(x)"}})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	for _, strategy := range Strategies {
		_, err := p.NewWorker().Eval(context.Background(), []byte(`{"x":"bad"}`), strategy)
		var diagnostic *jt.Error
		if !errors.As(err, &diagnostic) || diagnostic.Code != "T0412" {
			t.Fatalf("%s: %v", strategy, err)
		}
	}
	q, err := NewPlan(context.Background(), Config{Engine: "jsonata", Expressions: []string{"$subject & ':' & x"}, Options: jt.EvalOptions{Bindings: map[string]any{"subject": "input.a"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for _, strategy := range Strategies {
				got, err := q.NewWorker().Eval(context.Background(), []byte(`{"x":"ok"}`), strategy)
				if err != nil || string(got[0].Value) != `"input.a:ok"` {
					t.Errorf("%s: %#v %v", strategy, got, err)
				}
			}
		})
	}
	wg.Wait()
}

func TestWorkerReleasesIntermediateTrees(t *testing.T) {
	p, err := NewPlan(context.Background(), Config{Engine: "jsonata", Expressions: []string{"$"}})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	worker := p.NewWorker()
	if _, err = worker.Eval(context.Background(), []byte(`{"x":[1,2]}`), Batched); err != nil {
		t.Fatal(err)
	}
	for _, v := range worker.values {
		if v.Defined || v.Value != nil {
			t.Fatal("worker retained intermediate Go tree after encoding")
		}
	}
}
