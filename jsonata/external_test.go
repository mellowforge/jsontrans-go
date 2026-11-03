package jsonata_test

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/jsonata"
)

func TestInvokeReturnedFunction(t *testing.T) {
	e, err := jsonata.New()
	if err != nil {
		t.Fatal(err)
	}
	p, err := e.CompileRaw(context.Background(), `function($x){$x+1}`)
	if err != nil {
		t.Fatal(err)
	}
	v, err := p.EvalRaw(context.Background(), nil, jsontrans.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	fn, ok := v.(jsonata.Callable)
	if !ok {
		t.Fatalf("got %T", v)
	}
	got, err := jsonata.Invoke(context.Background(), fn, []jsonata.Value{float64(3)}, jsontrans.EvalOptions{})
	if err != nil || got != float64(4) {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestConcurrentInvokeReturnedFunction(t *testing.T) {
	e, _ := jsonata.New()
	p, err := e.CompileRaw(context.Background(), `($offset := 10; function($x){$x + $offset})`)
	if err != nil {
		t.Fatal(err)
	}
	v, err := p.EvalRaw(context.Background(), nil, jsontrans.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	fn := v.(jsonata.Callable)
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			got, err := jsonata.Invoke(context.Background(), fn, []jsonata.Value{float64(i)}, jsontrans.EvalOptions{})
			if err != nil {
				errs <- err
				return
			}
			if got != float64(i+10) {
				errs <- fmt.Errorf("got %v", got)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestInvokeReturnedTransform(t *testing.T) {
	e, _ := jsonata.New()
	p, err := e.CompileRaw(context.Background(), `|$|{"a":1}|`)
	if err != nil {
		t.Fatal(err)
	}
	v, err := p.EvalRaw(context.Background(), nil, jsontrans.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	fn := v.(jsonata.Callable)
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := jsonata.Invoke(context.Background(), fn, []jsonata.Value{map[string]any{"x": float64(2)}}, jsontrans.EvalOptions{})
			if err != nil {
				errs <- err
				return
			}
			if !reflect.DeepEqual(got, map[string]any{"a": float64(1), "x": float64(2)}) {
				errs <- fmt.Errorf("got %#v", got)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestConcurrentInvokeClosureAlias(t *testing.T) {
	e, _ := jsonata.New()
	p, err := e.CompileRaw(context.Background(), `($f:=function(){1};function(){($g:=$f;$g())})`)
	if err != nil {
		t.Fatal(err)
	}
	v, err := p.EvalRaw(context.Background(), nil, jsontrans.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	fn := v.(jsonata.Callable)
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				got, err := jsonata.Invoke(context.Background(), fn, nil, jsontrans.EvalOptions{})
				if err != nil || got != float64(1) {
					errs <- fmt.Errorf("got %v: %v", got, err)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestInvokePreservesCapturedObjectOrder(t *testing.T) {
	e, err := jsonata.New()
	if err != nil {
		t.Fatal(err)
	}
	p, err := e.CompileRaw(context.Background(), `($obj:={"b":1,"a":2};function(){$keys($obj)})`)
	if err != nil {
		t.Fatal(err)
	}
	v, err := p.EvalRaw(context.Background(), nil, jsontrans.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	fn := v.(jsonata.Callable)
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := jsonata.Invoke(context.Background(), fn, nil, jsontrans.EvalOptions{})
			if err != nil {
				errs <- err
				return
			}
			want := jsonata.Sequence{"b", "a"}
			if !reflect.DeepEqual(got, want) {
				errs <- fmt.Errorf("got %#v, want %#v", got, want)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestConcurrentInvokeNestedCallsWithCapturedOrderCache(t *testing.T) {
	cases := []struct {
		expr string
		args []jsonata.Value
		want any
	}{
		{`($f:=function(){1};$f();function(){($g:=function(){2};$g()+1)})`, nil, float64(3)},
		{`($f:=function(){1};$f();|$|{"v":(function(){2})()}|)`, []jsonata.Value{map[string]any{"x": float64(1)}}, map[string]any{"x": float64(1), "v": float64(2)}},
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			e, err := jsonata.New()
			if err != nil {
				t.Fatal(err)
			}
			p, err := e.CompileRaw(context.Background(), tc.expr)
			if err != nil {
				t.Fatal(err)
			}
			v, err := p.EvalRaw(context.Background(), nil, jsontrans.EvalOptions{})
			if err != nil {
				t.Fatal(err)
			}
			fn := v.(jsonata.Callable)
			var wg sync.WaitGroup
			errs := make(chan error, 4)
			for i := 0; i < 4; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for j := 0; j < 100; j++ {
						got, err := jsonata.Invoke(context.Background(), fn, tc.args, jsontrans.EvalOptions{})
						if err != nil || !reflect.DeepEqual(got, tc.want) {
							errs <- fmt.Errorf("got %#v, %v", got, err)
							return
						}
					}
				}()
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				t.Error(err)
			}
		})
	}
}

// Each Invoke result is owned by its caller: changing one result does not
// change captured data or later results.
func TestInvokeResultsAreDetached(t *testing.T) {
	for _, expression := range []string{
		`($o := {"a": 1}; function(){ $o })`,
		`function(){ $ }`,
		`($o := {"a": 1}; function(){ [$o] })`,
	} {
		t.Run(expression, func(t *testing.T) {
			e, err := jsonata.New()
			if err != nil {
				t.Fatal(err)
			}
			p, err := e.CompileRaw(context.Background(), expression)
			if err != nil {
				t.Fatal(err)
			}
			v, err := p.EvalRaw(context.Background(), map[string]any{"a": float64(1)}, jsontrans.EvalOptions{})
			if err != nil {
				t.Fatal(err)
			}
			fn := v.(jsonata.Callable)
			object := func(v jsonata.Value) map[string]any {
				if a, ok := v.([]any); ok {
					return a[0].(map[string]any)
				}
				return v.(map[string]any)
			}
			first, err := jsonata.Invoke(context.Background(), fn, nil, jsontrans.EvalOptions{})
			if err != nil {
				t.Fatal(err)
			}
			object(first)["a"] = float64(2)
			second, err := jsonata.Invoke(context.Background(), fn, nil, jsontrans.EvalOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if got := object(second)["a"]; got != float64(1) {
				t.Fatalf("second result sees caller change: %v", got)
			}
		})
	}
}
