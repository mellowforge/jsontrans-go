package javascript

import (
	"context"
	"encoding/json"
	"errors"
	jt "github.com/mellowforge/jsontrans-go"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestProcessors(t *testing.T) {
	for _, backend := range []Backend{QuickJS, Goja} {
		t.Run(string(backend), func(t *testing.T) {
			e, err := New(backend, Config{Limits: jt.Limits{Timeout: 10 * time.Second}})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = e.Close() })
			p, err := e.Compile(context.Background(), `(input, bindings) => ({total: input.items.reduce((s,x)=>s+x.price,0), currency: bindings.currency})`)
			if err != nil {
				t.Fatal(err)
			}
			in := map[string]any{"items": []any{map[string]any{"price": 4}, map[string]any{"price": 7}}}
			out, err := p.Eval(context.Background(), in, jt.EvalOptions{Bindings: map[string]any{"currency": "USD"}})
			want := map[string]any{"total": json.Number("11"), "currency": "USD"}
			if err != nil || !out.Defined || !reflect.DeepEqual(out.Value, want) {
				t.Fatalf("got %#v %v", out, err)
			}
			if in["items"].([]any)[0].(map[string]any)["price"] != 4 {
				t.Fatal("input changed")
			}
		})
	}
}

func TestShapesAndIsolation(t *testing.T) {
	for _, backend := range []Backend{QuickJS, Goja} {
		t.Run(string(backend), func(t *testing.T) {
			e, _ := New(backend, Config{Limits: jt.Limits{Timeout: 10 * time.Second}})
			t.Cleanup(func() { _ = e.Close() })
			for _, tc := range []struct {
				source  string
				defined bool
				value   any
			}{
				{`()=>undefined`, false, nil}, {`()=>null`, true, nil}, {`()=>[]`, true, []any{}},
				{`input=>{input.x=9;globalThis.leaked=(globalThis.leaked||0)+1;return globalThis.leaked}`, true, json.Number("1")},
			} {
				p, err := e.Compile(context.Background(), tc.source)
				if err != nil {
					t.Fatal(err)
				}
				var wg sync.WaitGroup
				for range 3 {
					wg.Go(func() {
						in := map[string]any{"x": 1}
						r, err := p.Eval(context.Background(), in, jt.EvalOptions{})
						if err != nil || r.Defined != tc.defined || !reflect.DeepEqual(r.Value, tc.value) || in["x"] != 1 {
							t.Errorf("%s: %#v %v input%v", tc.source, r, err, in)
						}
					})
				}
				wg.Wait()
			}
		})
	}
}

func TestStrictResultsAndLimits(t *testing.T) {
	for _, backend := range []Backend{QuickJS, Goja} {
		t.Run(string(backend), func(t *testing.T) {
			e, _ := New(backend, Config{Limits: jt.Limits{Timeout: 10 * time.Second}})
			t.Cleanup(func() { _ = e.Close() })
			for _, source := range []string{`()=>NaN`, `()=>Infinity`, `()=>1n`, `()=>Symbol('x')`, `()=>function(){}`, `()=>Promise.resolve(1)`, `()=>{const x={};x.x=x;return x}`, `()=>({get x(){return 1}})`, `()=>new Map()`, `()=>[undefined]`} {
				p, err := e.Compile(context.Background(), source)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = p.Eval(context.Background(), nil, jt.EvalOptions{}); !errors.Is(err, jt.ErrConversion) {
					t.Errorf("%s: %v", source, err)
				}
			}
			p, _ := e.Compile(context.Background(), `()=>'abcdefghij'`)
			if _, err := p.Eval(context.Background(), nil, jt.EvalOptions{Limits: &jt.Limits{MaxOutputBytes: 4}}); !errors.Is(err, jt.ErrLimit) {
				t.Fatalf("output limit: %v", err)
			}
			if _, err := p.Eval(context.Background(), nil, jt.EvalOptions{Limits: &jt.Limits{MaxSteps: 1}}); !errors.Is(err, jt.ErrUnsupported) {
				t.Fatalf("unsupported instruction limit: %v", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if _, err := p.Eval(ctx, nil, jt.EvalOptions{}); !errors.Is(err, context.Canceled) {
				t.Fatalf("cancel: %v", err)
			}
		})
	}
}

func TestBridgeCannotBeReconfiguredBySource(t *testing.T) {
	for _, backend := range []Backend{QuickJS, Goja} {
		t.Run(string(backend), func(t *testing.T) {
			e, _ := New(backend, Config{Limits: jt.Limits{Timeout: 10 * time.Second}})
			t.Cleanup(func() { _ = e.Close() })

			p, err := e.Compile(context.Background(), `()=>typeof inputText`)
			if err != nil {
				t.Fatal(err)
			}
			r, err := p.Eval(context.Background(), nil, jt.EvalOptions{})
			if err != nil || r.Value != "undefined" {
				t.Fatalf("bridge lexical scope exposed: %#v %v", r, err)
			}
			p, err = e.Compile(context.Background(), `()=>{try { byteLimit=1000000; } catch {} return "abcdefghij";}`)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = p.Eval(context.Background(), nil, jt.EvalOptions{Limits: &jt.Limits{MaxOutputBytes: 4}}); !errors.Is(err, jt.ErrLimit) {
				t.Fatalf("bridge limit changed: %v", err)
			}
			p, err = e.Compile(context.Background(), `()=>{Object.defineProperty(Array.prototype,"0",{set(){throw Error("prototype setter")}, configurable:true});return {x:1}}`)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = p.Eval(context.Background(), nil, jt.EvalOptions{}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestOutputBudgetIndependentOfInput(t *testing.T) {
	for _, backend := range []Backend{QuickJS, Goja} {
		e, _ := New(backend, Config{Limits: jt.Limits{Timeout: 10 * time.Second}})
		t.Cleanup(func() { _ = e.Close() })

		p, err := e.Compile(context.Background(), `()=>"abcdefghij"`)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = p.Eval(context.Background(), nil, jt.EvalOptions{Limits: &jt.Limits{MaxInputBytes: 4, MaxOutputBytes: 32}}); err != nil {
			t.Errorf("%s: %v", backend, err)
		}
	}
}
func TestArrayPropertiesAreRejected(t *testing.T) {
	for _, backend := range []Backend{QuickJS, Goja} {
		e, _ := New(backend, Config{Limits: jt.Limits{Timeout: 10 * time.Second}})
		t.Cleanup(func() { _ = e.Close() })

		for _, source := range []string{`()=>{const a=[];a.x=1;return a}`, `()=>{const a=[];a[Symbol()]=1;return a}`} {
			p, err := e.Compile(context.Background(), source)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = p.Eval(context.Background(), nil, jt.EvalOptions{}); !errors.Is(err, jt.ErrConversion) {
				t.Errorf("%s %s: %v", backend, source, err)
			}
		}
	}
}

func TestUnsupportedConstructorControls(t *testing.T) {
	for _, backend := range []Backend{QuickJS, Goja} {
		for _, limits := range []jt.Limits{{MaxSteps: 1}, {RegexTimeout: time.Millisecond}} {
			e, err := New(backend, Config{Limits: limits})
			if e != nil {
				_ = e.Close()
			}
			if !errors.Is(err, jt.ErrUnsupported) {
				t.Errorf("%s explicit limits: %v", backend, err)
			}
		}
	}
	e, err := New(Goja, Config{GuestMemoryBytes: 64 << 20})
	if e != nil {
		_ = e.Close()
	}
	if !errors.Is(err, jt.ErrUnsupported) {
		t.Errorf("Goja heap quota accepted: %v", err)
	}
}

func TestInputBoundariesAndBindingOwnership(t *testing.T) {
	for _, backend := range []Backend{QuickJS, Goja} {
		e, err := New(backend, Config{Limits: jt.Limits{Timeout: 10 * time.Second}})
		if err != nil {
			t.Fatal(err)
		}
		defer e.Close()
		p, err := e.Compile(context.Background(), `(input,bindings)=>{bindings.value.x=7;input.x=9;return bindings.value.x}`)
		if err != nil {
			t.Fatal(err)
		}
		input := map[string]any{"x": 1}
		bindings := map[string]any{"value": map[string]any{"x": 2}}
		r, err := p.Eval(context.Background(), input, jt.EvalOptions{Bindings: bindings})
		if err != nil || r.Value != json.Number("7") || input["x"] != 1 || bindings["value"].(map[string]any)["x"] != 2 {
			t.Fatalf("%s ownership: %#v %v", backend, r, err)
		}
		cycle := map[string]any{}
		cycle["self"] = cycle
		for _, value := range []any{json.Number("1e400"), cycle, make(chan int)} {
			if _, err = p.Eval(context.Background(), value, jt.EvalOptions{}); !errors.Is(err, jt.ErrInvalidInput) {
				t.Errorf("%s invalid input %T: %v", backend, value, err)
			}
		}
	}
}

func TestSourceCannotReplaceResultBridge(t *testing.T) {
	for _, backend := range []Backend{QuickJS, Goja} {
		e, err := New(backend, Config{Limits: jt.Limits{Timeout: 10 * time.Second}})
		if err != nil {
			t.Fatal(err)
		}
		defer e.Close()
		source := `()=>null)]; [()=>'{"defined":true,"value":9007199254740993}', (()=>null`
		p, err := e.Compile(context.Background(), source)
		if err != nil {
			continue
		}
		result, err := p.Eval(context.Background(), nil, jt.EvalOptions{})
		if err == nil && result.Defined && result.Value == json.Number("9007199254740993") {
			t.Errorf("%s source replaced trusted JSON bridge", backend)
		}
	}
}
