package quickjswasm

import (
	"context"
	"errors"
	jt "github.com/mellowforge/jsontrans-go"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestOwnedRuntimeFreshStateAndLifecycle(t *testing.T) {
	e, err := New(context.Background(), Config{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	codec, err := e.Compile(context.Background(), `(function(){return function(rule,input){return JSON.stringify(rule(JSON.parse(input)))}})()`)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := e.Compile(context.Background(), `input=>{globalThis.count=(globalThis.count||0)+1;return {n:globalThis.count,x:input.x+1}}`)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			result, err := e.Eval(context.Background(), rule, codec, `{"x":1}`, `{}`, Limits{Depth: 256, Items: 100000, Bytes: 1 << 20, Work: 1000000})
			if err != nil || result != `{"n":1,"x":2}` {
				t.Errorf("result %s error %v", result, err)
			}
		})
	}
	wg.Wait()
	if err = e.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = e.Compile(context.Background(), `1`); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed engine accepted call: %v", err)
	}
}
func TestOwnedRuntimeCancellationAndRecovery(t *testing.T) {
	e, err := New(context.Background(), Config{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	for _, source := range []string{`while(true){}`, `/^(a+)+$/.test("a".repeat(35)+"!")`, `throw {toString(){while(true){}}}`} {
		code, err := e.Compile(context.Background(), source)
		if err != nil {
			t.Fatal(err)
		}
		// Eval's bridge executes first; a script looping/throwing during rule load
		// must be canceled without calling guest free on the canceled module.
		bridge, err := e.Compile(context.Background(), `()=>"ok"`)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		_, err = e.Eval(ctx, code, bridge, `null`, `{}`, Limits{Depth: 256, Items: 100000, Bytes: 1 << 20, Work: 1000000})
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("%s: %v", source, err)
		}
		if _, err = e.Compile(context.Background(), `42`); err != nil {
			t.Fatalf("recovery: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = e.Compile(ctx, `42`); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled compile: %v", err)
	}
	large := strings.Repeat("void 0;", 8000)
	ctx, cancel = context.WithTimeout(context.Background(), time.Millisecond)
	_, err = e.Compile(ctx, large)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("source compilation: %v", err)
	}
}
func TestOwnedRuntimeCapAndIndependence(t *testing.T) {
	for _, pages := range []uint32{128, 1024} {
		e, err := New(context.Background(), Config{MemoryPages: pages, Timeout: 10 * time.Second})
		if err != nil {
			t.Fatal(err)
		}
		module, err := e.instantiate(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		maximum, has := module.Memory().Definition().Max()
		if !has || maximum > pages {
			t.Fatalf("guest cap missing: %d %v, requested %d", maximum, has, pages)
		}
		before := module.Memory().Size()
		if _, ok := module.Memory().Grow(pages); ok || module.Memory().Size() != before {
			t.Fatalf("guest grew beyond cap %d", pages)
		}
		module.Close(context.Background())
		e.Close()
	}
}

func TestCloseInterruptsActiveOwnedGuest(t *testing.T) {
	started := make(chan struct{})
	var entered sync.Once
	factory := experimental.FunctionListenerFactoryFunc(func(def api.FunctionDefinition) experimental.FunctionListener {
		for _, name := range def.ExportNames() {
			if name == "abi_eval" {
				return experimental.FunctionListenerFunc(func(context.Context, api.Module, api.FunctionDefinition, []uint64, experimental.StackIterator) {
					entered.Do(func() { close(started) })
				})
			}
		}
		return nil
	})
	compileContext := experimental.WithFunctionListenerFactory(context.Background(), factory)
	e, err := New(compileContext, Config{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	rule, err := e.Compile(context.Background(), `while(true){}`)
	if err != nil {
		t.Fatal(err)
	}
	codec, err := e.Compile(context.Background(), `()=>"ok"`)
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() {
		_, err := e.Eval(context.Background(), rule, codec, `null`, `{}`, Limits{Depth: 256, Items: 100000, Bytes: 1024, Work: 1000000})
		finished <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("guest ABI call did not start")
	}
	if err = e.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if !errors.Is(err, ErrClosed) {
			t.Fatalf("closed call: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close left active evaluation running")
	}
}

func TestSourceCompileCancellationInsideInitializedGuest(t *testing.T) {
	e, err := New(context.Background(), Config{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	m, err := e.instantiate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close(context.Background())
	source := []byte(strings.Repeat("void 0;", 8000))
	ptr, err := transfer(context.Background(), m, source, maxTransfer)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	_, err = call(ctx, m, "abi_compile", ptr, uint64(len(source)))
	if err == nil || !errors.Is(e.failure(ctx, err), context.DeadlineExceeded) {
		t.Fatalf("in-flight compile: %v", err)
	}
}

func TestConcurrentEnginesHaveIndependentHeapLimits(t *testing.T) {
	var wg sync.WaitGroup
	for _, pages := range []uint32{128, 1024} {
		wg.Go(func() {
			e, err := New(context.Background(), Config{MemoryPages: pages, Timeout: 10 * time.Second})
			if err != nil {
				t.Error(err)
				return
			}
			defer e.Close()
			rule, err := e.Compile(context.Background(), `()=>new ArrayBuffer(40*1024*1024).byteLength`)
			if err != nil {
				t.Error(err)
				return
			}
			codec, err := e.Compile(context.Background(), `rule=>JSON.stringify(rule())`)
			if err != nil {
				t.Error(err)
				return
			}
			value, err := e.Eval(context.Background(), rule, codec, `null`, `{}`, Limits{Depth: 256, Items: 100000, Bytes: 1024, Work: 1000000})
			if pages == 128 && err == nil {
				t.Error("8 MiB engine accepted 40 MiB buffer")
			}
			if pages == 1024 && (err != nil || value != "41943040") {
				t.Errorf("64 MiB engine: %s %v", value, err)
			}
		})
	}
	wg.Wait()
}

func TestOwnedRejectsUnsupportedStackQuota(t *testing.T) {
	e, err := New(context.Background(), Config{StackBytes: 16 << 10, Timeout: 10 * time.Second})
	if e != nil {
		e.Close()
	}
	if !errors.Is(err, jt.ErrUnsupported) {
		t.Fatalf("unenforced stack quota accepted: %v", err)
	}
}
