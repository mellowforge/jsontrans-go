package javascript

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	jt "github.com/mellowforge/jsontrans-go"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
)

// Removing complete-call admission/draining must fail even when cancellation
// occurs while a guest call is blocked in host-side listener processing.
func TestPublicCloseDrainsAdmittedCalls(t *testing.T) {
	for _, operation := range []string{"abi_compile", "abi_eval"} {
		t.Run(operation, func(t *testing.T) {
			entered, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var first, unblock sync.Once
			factory := experimental.FunctionListenerFactoryFunc(func(def api.FunctionDefinition) experimental.FunctionListener {
				for _, name := range def.ExportNames() {
					if name == operation {
						return experimental.FunctionListenerFunc(func(ctx context.Context, _ api.Module, _ api.FunctionDefinition, _ []uint64, _ experimental.StackIterator) {
							first.Do(func() {
								close(entered)
								<-ctx.Done()
								close(canceled)
								<-release
							})
						})
					}
				}
				return nil
			})
			ctx := experimental.WithFunctionListenerFactory(context.Background(), factory)
			e, err := New(ctx, Config{Limits: jt.Limits{Timeout: semanticTestTimeout}})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { unblock.Do(func() { close(release) }); _ = e.Close() })
			var p jt.Program
			if operation == "abi_eval" {
				p, err = e.Compile(context.Background(), `()=>{while(true){}}`)
				if err != nil {
					t.Fatal(err)
				}
			}
			callDone := make(chan error, 1)
			go func() {
				if p == nil {
					_, err := e.Compile(context.Background(), `x=>x`)
					callDone <- err
				} else {
					_, err := p.Eval(context.Background(), nil, jt.EvalOptions{})
					callDone <- err
				}
			}()
			awaitSignal(t, entered)
			closed := make(chan error, 2)
			for range 2 {
				go func() { closed <- e.Close() }()
			}
			awaitSignal(t, canceled)
			select {
			case err := <-closed:
				t.Fatalf("Close returned before admitted call drained: %v", err)
			default:
			}
			if _, err := e.Compile(context.Background(), `x=>x`); !errors.Is(err, ErrClosed) {
				t.Fatalf("closing engine admitted compilation: %v", err)
			}
			unblock.Do(func() { close(release) })
			select {
			case err := <-callDone:
				if !errors.Is(err, ErrClosed) {
					t.Fatalf("active call lost closure cause: %v", err)
				}
			case <-time.After(semanticTestTimeout):
				t.Fatal("admitted call did not stop")
			}
			for range 2 {
				select {
				case err := <-closed:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(semanticTestTimeout):
					t.Fatal("Close did not finish")
				}
			}
		})
	}
}

func awaitSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(semanticTestTimeout):
		t.Fatal("guest lifecycle barrier was not reached")
	}
}

// A second internal timeout must not cap a raised per-call default; conversely,
// conversion/guest execution must not discard a lower override.
func TestPublicTimeoutOverrideReachesGuest(t *testing.T) {
	deadlines := make(chan time.Time, 1)
	factory := experimental.FunctionListenerFactoryFunc(func(def api.FunctionDefinition) experimental.FunctionListener {
		for _, name := range def.ExportNames() {
			if name == "abi_eval" {
				return experimental.FunctionListenerFunc(func(ctx context.Context, _ api.Module, _ api.FunctionDefinition, _ []uint64, _ experimental.StackIterator) {
					deadline, _ := ctx.Deadline()
					deadlines <- deadline
				})
			}
		}
		return nil
	})
	e, err := New(experimental.WithFunctionListenerFactory(context.Background(), factory), Config{Limits: jt.Limits{Timeout: semanticTestTimeout}})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	p, err := e.Compile(context.Background(), `()=>1`)
	if err != nil {
		t.Fatal(err)
	}
	for _, timeout := range []time.Duration{semanticTestTimeout / 2, semanticTestTimeout * 2} {
		before := time.Now()
		_, err := p.Eval(context.Background(), nil, jt.EvalOptions{Limits: &jt.Limits{Timeout: timeout}})
		after := time.Now()
		if err != nil {
			t.Fatal(err)
		}
		select {
		case got := <-deadlines:
			if got.Before(before.Add(timeout)) || got.After(after.Add(timeout)) {
				t.Fatalf("guest deadline %s outside override interval [%s, %s]", got, before.Add(timeout), after.Add(timeout))
			}
		default:
			t.Fatal("guest was not observed")
		}
	}
}

// Reusing guest state or passing caller maps directly would leak mutations or
// cause cross-call races. The hostile mutations also exercise captured intrinsics.
func TestPublicConcurrentDetachedState(t *testing.T) {
	e := newTestEngine(t)
	p, err := e.Compile(context.Background(), `(input, bindings)=>{
		globalThis.seen=(globalThis.seen||0)+1;
		const original=input.nested.value+bindings.nested.value;
		input.nested.value=999;bindings.nested.value=999;
		JSON.stringify=()=>"forged";Reflect.apply=()=>0;
		Object.prototype.toJSON=()=>"forged";Array.prototype.toJSON=()=>"forged";
		String.prototype.charCodeAt=()=>0;
		return {sum:original, seen:globalThis.seen, text:"😀", list:[1,null]};
	}`)
	if err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"nested": map[string]any{"value": 2}}
	bindings := map[string]any{"nested": map[string]any{"value": 3}}
	want := map[string]any{"sum": json.Number("5"), "seen": json.Number("1"), "text": "😀", "list": []any{json.Number("1"), nil}}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 8 {
				got, err := p.Eval(context.Background(), input, jt.EvalOptions{Bindings: bindings})
				if err != nil || !got.Defined || !reflect.DeepEqual(got.Value, want) {
					t.Errorf("isolated evaluation: %#v, %v", got, err)
					return
				}
			}
		})
	}
	wg.Wait()
	if input["nested"].(map[string]any)["value"] != 2 || bindings["nested"].(map[string]any)["value"] != 3 {
		t.Fatal("caller input or bindings mutated")
	}
}

func TestPublicHostileResultsAndRecovery(t *testing.T) {
	e := newTestEngine(t)
	healthy, err := e.Compile(context.Background(), `()=>42`)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{
		`()=>Promise.resolve(1)`, `()=>1n`, `()=>Symbol('x')`, `()=>NaN`, `()=>Infinity`,
		`()=>new Date()`, `()=>new Uint8Array(2)`, `()=>new Map()`, `()=>new Number(1)`,
		`()=>{const a=[];a[1]=1;return a}`, `()=>{const a=[1];a.extra=2;return a}`,
		`()=>({[Symbol('x')]:1})`, `()=>({get x(){throw new Error('must not run')}})`,
		`()=>{const x={};x.self=x;return x}`,
	} {
		t.Run(source, func(t *testing.T) {
			p, err := e.Compile(context.Background(), source)
			if err != nil {
				t.Fatal(err)
			}
			_, err = p.Eval(context.Background(), nil, jt.EvalOptions{})
			var d *jt.Error
			var ex *Exception
			if !errors.Is(err, jt.ErrConversion) || !errors.As(err, &d) || d.Phase != jt.PhaseOutput || errors.As(err, &ex) {
				t.Fatalf("strict conversion: %v", err)
			}
			if got, err := healthy.Eval(context.Background(), nil, jt.EvalOptions{}); err != nil || !got.Defined || got.Value != json.Number("42") {
				t.Fatalf("recovery: %#v %v", got, err)
			}
		})
	}
	for _, tc := range []struct {
		source string
		limits jt.Limits
	}{
		{`()=>[1,2,3]`, jt.Limits{MaxCollectionItems: 2}},
		{`()=>({a:{b:1}})`, jt.Limits{MaxDepth: 2}},
		{`()=>'abcdefgh'`, jt.Limits{MaxOutputBytes: 4}},
	} {
		p, err := e.Compile(context.Background(), tc.source)
		if err != nil {
			t.Fatal(err)
		}
		_, err = p.Eval(context.Background(), nil, jt.EvalOptions{Limits: &tc.limits})
		var d *jt.Error
		if !errors.Is(err, jt.ErrLimit) || !errors.As(err, &d) || d.Phase != jt.PhaseOutput {
			t.Fatalf("output budget %s: %v", tc.source, err)
		}
	}
}

func TestPublicPrototypeKeysRemainData(t *testing.T) {
	e := newTestEngine(t)
	p, err := e.Compile(context.Background(), `input=>{
		const result=Object.create(null);
		result.__proto__=input.__proto__;result.constructor=input.constructor;
		result.prototype=input.prototype;result.toJSON=input.toJSON;
		return result;
	}`)
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"__proto__":{"polluted":true},"constructor":2,"prototype":3,"toJSON":"ordinary data"}`)
	got, err := jt.EvalJSON(context.Background(), p, input, jt.EvalOptions{})
	var actual, expected any
	if err != nil || !got.Defined || json.Unmarshal(got.Value, &actual) != nil || json.Unmarshal(input, &expected) != nil || !reflect.DeepEqual(actual, expected) {
		t.Fatalf("prototype keys: %s %v", got.Value, err)
	}
}
