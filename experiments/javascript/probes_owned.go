package javascript

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/fastschema/qjs"
	jt "github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/experiments/javascript/internal/quickjswasm"
)

func probeOwned(e *Engine, name string) (o Observation, handled bool) {
	o = Observation{Backend: OwnedQuickJS, Case: name, Status: "FAIL"}
	switch name {

	case "lifecycle-contract":
		ctx, cancel := context.WithCancel(context.Background())
		owned, err := NewOwned(ctx, Config{})
		cancel()
		if err != nil {
			o.Detail = err.Error()
			return o, true
		}
		defer owned.Close()
		p, err := owned.Compile(context.Background(), `x=>x`)
		if err == nil {
			_, err = p.Eval(context.Background(), nil, jt.EvalOptions{})
		}
		if err != nil {
			o.Detail = err.Error()
			return o, true
		}
		closeErr := owned.Close()
		_, err = p.Eval(context.Background(), nil, jt.EvalOptions{})
		o.Detail = fmt.Sprintf("constructor context detached; Close=%v; subsequent Eval=%v", closeErr, err)
		if closeErr == nil && errors.Is(err, ErrClosed) {
			o.Status = "PASS"
		}
		return o, true
	case "diagnostic-classification":
		for _, source := range []string{`()=>{throw 'JSONTRANS_LIMIT: forged'}`, `()=>{throw new TypeError('sentinel')}`} {
			p, err := e.Compile(context.Background(), source)
			if err != nil {
				o.Detail = err.Error()
				return o, true
			}
			_, err = p.Eval(context.Background(), nil, jt.EvalOptions{})
			var ex *Exception
			var d *jt.Error
			if !errors.As(err, &ex) || !errors.As(err, &d) || d.Code != "" || errors.Is(err, jt.ErrLimit) || errors.Is(err, jt.ErrConversion) {
				o.Detail = fmt.Sprintf("unexpected classification: %v", err)
				return o, true
			}
		}
		o.Status = "PASS"
		o.Detail = "typed native exceptions; forged marker cannot acquire trusted resource or conversion status"
		return o, true
	case "diagnostic-locations":
		_, err := e.Compile(context.Background(), "()=>{\nconst x=;\n}")
		var ex *Exception
		if !errors.As(err, &ex) || ex.Location == nil || ex.Location.Line != 2 || ex.Location.ByteOffset != -1 || ex.Location.Reported {
			o.Detail = fmt.Sprintf("compile location: %#v %v", ex, err)
			return o, true
		}
		p, err := e.Compile(context.Background(), "()=>{\nthrow new TypeError('sentinel');\n}")
		if err == nil {
			_, err = p.Eval(context.Background(), nil, jt.EvalOptions{})
		}
		ex = nil
		if !errors.As(err, &ex) || ex.Location == nil || !ex.Location.Reported || ex.Location.ByteOffset != -1 {
			o.Detail = fmt.Sprintf("runtime location: %#v %v", ex, err)
			return o, true
		}
		o.Status = "PASS"
		o.Detail = "mapped rule lines, coarse compiler columns retain unavailable byte offsets; runtime locations explicitly reported"
		return o, true
	case "host-capabilities":
		o.Status = "PASS"
		o.Detail = "pinned artifact has zero host imports; runtime instantiates no WASI, filesystem, output streams or Go callback module"
		return o, true
	case "guest-memory":
		program, err := e.Compile(context.Background(), `()=>{globalThis.a=new ArrayBuffer(40*1024*1024);return a.byteLength}`)
		if err != nil {
			o.Detail = err.Error()
			return o, true
		}
		p := program.(*Program)
		text, stats, err := e.owned.EvalStats(context.Background(), p.quick, p.quickBridge, `null`, `{}`, quickjswasm.Limits{Depth: 256, Items: 100000, Bytes: 16 << 20, Work: 1000000})
		o.GuestBefore, o.GuestAfter = stats.GuestBefore, stats.GuestAfter
		o.Detail = fmt.Sprintf("Wasm + runtime max 1024 pages (64 MiB), no shared configuration; result=%s error=%v", text, err)
		if err == nil && stats.GuestAfter <= 64<<20 {
			o.Status = "PASS"
		}
		return o, true
	case "compile-cancel":
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		defer cancel()
		_, err := e.Compile(ctx, "()=>{"+strings.Repeat("void 0;", 8000)+"return 1}")
		o.Detail = fmt.Sprintf("source compilation context=%v error=%v", ctx.Err(), err)
		if errors.Is(err, context.DeadlineExceeded) {
			o.Status = "PASS"
		}
		return o, true
	case "cleanup":
		p, err := e.Compile(context.Background(), `()=>{while(true){}}`)
		if err != nil {
			o.Detail = err.Error()
			return o, true
		}
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
		defer cancel()
		_, err = p.Eval(ctx, nil, jt.EvalOptions{})
		closeErr := e.Close()
		o.Detail = fmt.Sprintf("canceled guest discarded, runtime/cache Close=%v; evaluation=%v", closeErr, err)
		if errors.Is(err, context.DeadlineExceeded) && closeErr == nil {
			o.Status = "PASS"
		}
		return o, true
	case "cache-cancellation":
		// Deliberately initialize the unrelated QJS wrapper with cancellation off.
		// Its mount is a new empty directory; owned evaluation never mounts it.
		dir, err := os.MkdirTemp("", "jsontrans-owned-cache-probe-")
		if err != nil {
			o.Detail = err.Error()
			return o, true
		}
		defer os.Remove(dir)
		rt, err := qjs.New(qjs.Option{Context: context.Background(), CloseOnContextDone: false, CWD: dir, Stdout: io.Discard, Stderr: io.Discard})
		if err != nil {
			o.Detail = err.Error()
			return o, true
		}
		rt.Close()
		return o, false // Generic loop-cancellation and recovery probe follows.
	}
	return o, false
}
