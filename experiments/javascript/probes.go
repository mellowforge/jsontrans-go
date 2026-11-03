package javascript

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"time"

	"github.com/fastschema/qjs"
	jt "github.com/mellowforge/jsontrans-go"
)

// Observation is assessment evidence, not a guarantee for arbitrary programs.
type Observation struct {
	Backend     Backend `json:"backend"`
	Case        string  `json:"case"`
	Status      string  `json:"status"`
	Detail      string  `json:"detail"`
	DurationNS  int64   `json:"duration_ns"`
	GuestBefore uint32  `json:"guest_before_bytes,omitempty"`
	GuestAfter  uint32  `json:"guest_after_bytes,omitempty"`
	GoHeap      uint64  `json:"go_heap_bytes"`
	PeakRSS     uint64  `json:"peak_rss_bytes"`
}

// Probe must be run in a subprocess with an external deadline. Some probes
// intentionally demonstrate that upstream cancellation is ineffective.
func Probe(backend Backend, name string) (o Observation) {
	o.Backend, o.Case, o.Status = backend, name, "FAIL"
	start := time.Now()
	defer func() {
		if v := recover(); v != nil {
			o.Detail = fmt.Sprintf("panic: %v", v)
		}
		o.DurationNS = time.Since(start).Nanoseconds()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		o.GoHeap = m.HeapAlloc
		o.PeakRSS = peakRSS()
	}()
	if name == "allocation-cap" && backend == Goja {
		o.Detail = "source audit: no per-runtime heap quota; uncapped allocation probe deliberately not executed"
		return
	}
	e, err := New(backend, Config{})
	if err != nil {
		o.Detail = err.Error()
		return
	}
	defer e.Close()
	if backend == OwnedQuickJS {
		if value, handled := probeOwned(e, name); handled {
			return value
		}
	}
	if name == "cache-cancellation" && backend != OwnedQuickJS {
		if backend == Goja {
			o.Status = "N/A"
			o.Detail = "no wrapper-wide cancellation config cache"
			return
		}
		rt, err := qjs.New(qjs.Option{Context: context.Background(), CloseOnContextDone: false, CWD: e.dir, Stdout: io.Discard, Stderr: io.Discard})
		if err != nil {
			o.Detail = err.Error()
			return
		}
		rt.Close()
	}
	if name == "guest-memory" {
		if backend == Goja {
			o.Detail = "no separately bounded guest linear memory"
			return
		}
		rt, err := e.newQuick(context.Background())
		if err != nil {
			o.Detail = err.Error()
			return
		}
		defer rt.Close()
		o.GuestBefore = rt.Mem().Size()
		v, err := rt.Eval("memory.js", qjs.Code(`globalThis.a=new ArrayBuffer(40*1024*1024); a.byteLength`))
		o.GuestAfter = rt.Mem().Size()
		if v != nil {
			defer v.Free()
		}
		o.Detail = fmt.Sprintf("64 MiB QuickJS allocator cap; 40 MiB ArrayBuffer; error=%v", err)
		o.Detail += "; source audit: no WithMemoryLimitPages cap; this sample does not prove a guest bound"
		return
	}
	if name == "host-capabilities" {
		if backend == QuickJS {
			o.Detail = "source audit: full WASI, writable directory mount, wall/monotonic clocks, nanosleep, stdout/stderr and env.jsFunctionProxy; empty mount and discarded output mitigate only part of the surface"
		} else {
			o.Status = "PASS"
			o.Detail = "bare Goja runtime: no Node/import/fs/network/host registrations"
		}
		return
	}
	if name == "compile-cancel" {
		expr := "()=>{" + strings.Repeat("void 0;", 8000) + "return 1}"
		// Warm engine compilation before measuring cancellation in source compilation.
		if _, err = e.Compile(context.Background(), `()=>1`); err != nil {
			o.Detail = err.Error()
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		begin := time.Now()
		if backend == QuickJS {
			err = func() (err error) {
				defer catchQuick(&err)
				rt, x := e.newQuick(ctx)
				if x != nil {
					return x
				}
				defer rt.Close()
				timer := time.AfterFunc(time.Millisecond, cancel)
				defer timer.Stop()
				_, err = rt.Compile("large.js", qjs.Code("("+expr+")"))
				return err
			}()
		} else {
			timer := time.AfterFunc(time.Millisecond, cancel)
			_, err = e.Compile(ctx, expr)
			timer.Stop()
		}
		o.Detail = fmt.Sprintf("source compilation + cleanup %s; context=%v; error=%v", time.Since(begin), ctx.Err(), err)
		if backend == QuickJS && ctx.Err() != nil && err != nil {
			o.Status = "PASS"
		}

		return
	}
	if name == "cleanup" {
		if backend == Goja {
			o.Status = "N/A"
			o.Detail = "runtime state is Go-managed"
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		rt, err := e.newQuick(ctx)
		if err != nil {
			o.Detail = err.Error()
			cancel()
			return
		}
		cancel()
		rt.Close()
		o.Status = "PASS"
		o.Detail = "cleanup succeeded after cancel"
		return
	}
	source := `()=>1`
	switch name {
	case "cancel-loop", "cache-cancellation":
		source = `()=>{while(true){}}`
	case "cancel-regex":
		source = `()=>/^(a+)+$/.test("a".repeat(35)+"!")`
	case "cancel-conversion":
		source = `()=>new Proxy({},{ownKeys(){while(true){}}})`
	case "recursion":
		source = `()=>{function f(){return 1+f()}return f()}`
	case "allocation-cap":
		source = `()=>new ArrayBuffer(128*1024*1024).byteLength`
	case "unicode":
		source = `input=>[input, "\ud800", "😀", "\udfff"]`
	case "numbers":
		source = `input=>input`
	case "exception-coercion":
		source = `()=>{throw {toString(){while(true){}}}}`
	case "exceptions":
		source = `()=>{throw new TypeError("sentinel")}`
	case "limits-forwarding", "prepared-fallback", "cold-start":
		source = `input=>input.x+1`
	case "conversion-limits":
		source = `()=>({a:[1,2,3]})`
	default:
		o.Detail = "unknown probe"
		return
	}
	p, err := e.Compile(context.Background(), source)
	if err != nil {
		o.Detail = err.Error()
		return
	}
	ctx := context.Background()
	cancel := func() {}
	if strings.HasPrefix(name, "cancel-") || (name == "cache-cancellation" || name == "exception-coercion") {
		ctx, cancel = context.WithTimeout(ctx, 75*time.Millisecond)
	}
	defer cancel()
	var r jt.Result
	switch name {
	case "unicode":
		jr, x := jt.EvalJSON(ctx, p, []byte(`"\ud800"`), jt.EvalOptions{})
		err = x
		o.Detail = string(jr.Value)
		if err == nil && strings.Count(o.Detail, `\ud800`) == 2 && strings.Contains(o.Detail, `\udfff`) {
			o.Status = "PASS"
		}
		return
	case "numbers":
		r, err = p.Eval(ctx, json.Number("9007199254740993"), jt.EvalOptions{})
		if err == nil && r.Value == json.Number("9007199254740992") {
			o.Status = "PASS"
		}
	case "limits-forwarding":
		t, _ := jt.New(map[string]jt.Processor{"js": e})
		_, err = t.Process(ctx, "js", map[string]any{"x": 1}, source, jt.EvalOptions{Limits: &jt.Limits{Timeout: time.Second}})
		if err == nil {
			o.Status = "PASS"
		}
	case "prepared-fallback":
		doc, x := jt.PrepareJSON(ctx, []byte(`{"x":1}`), jt.PrepareOptions{})
		if x != nil {
			o.Detail = x.Error()
			return
		}
		r, err = jt.EvalPrepared(ctx, p, doc, jt.EvalOptions{})
		if err == nil && r.Value == json.Number("2") {
			o.Status = "PASS"
		}
	case "cold-start":
		r, err = p.Eval(ctx, map[string]any{"x": 1}, jt.EvalOptions{})
		if err == nil && r.Value == json.Number("2") {
			o.Status = "PASS"
		}
	case "conversion-limits":
		_, a := p.Eval(ctx, nil, jt.EvalOptions{Limits: &jt.Limits{MaxDepth: 2}})
		_, b := p.Eval(ctx, nil, jt.EvalOptions{Limits: &jt.Limits{MaxCollectionItems: 2}})
		_, c := p.Eval(ctx, nil, jt.EvalOptions{Limits: &jt.Limits{MaxOutputBytes: 4}})
		if errors.Is(a, jt.ErrLimit) && errors.Is(b, jt.ErrLimit) && errors.Is(c, jt.ErrLimit) {
			o.Status = "PASS"
		}
		err = errors.Join(a, b, c)
	default:
		_, err = p.Eval(ctx, nil, jt.EvalOptions{})
		if strings.HasPrefix(name, "cancel-") || (name == "cache-cancellation" || name == "exception-coercion") {
			if errors.Is(err, context.DeadlineExceeded) {
				recovery, x := e.Compile(context.Background(), `()=>42`)
				if x == nil {
					r, x = recovery.Eval(context.Background(), nil, jt.EvalOptions{})
				}
				if x == nil && r.Value == json.Number("42") {
					o.Status = "PASS"
				} else {
					o.Detail = fmt.Sprintf("recovery: %v", x)
					return
				}
			}
		} else if name == "recursion" || name == "allocation-cap" || name == "exceptions" {
			if err != nil {
				o.Status = "PASS"
			}
		}
	}
	o.Detail = fmt.Sprintf("result=%#v; error=%v", r, err)
	return
}
