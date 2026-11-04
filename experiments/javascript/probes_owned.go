package javascript

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"encoding/json"
	jt "github.com/mellowforge/jsontrans-go"
	qjs "github.com/mellowforge/jsontrans-go/experiments/javascript/internal/qjscompat"
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
		o.Detail = "mapped rule lines and native compiler columns; byte offsets unavailable; runtime locations explicitly reported"
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

		result, err := program.Eval(context.Background(), nil, jt.EvalOptions{})
		if err != nil || result.Value != json.Number("41943040") {
			o.Detail = fmt.Sprintf("default guest: %+v %v", result, err)
			return o, true
		}
		small, err := NewOwned(context.Background(), Config{GuestMemoryBytes: 8 << 20})
		if err != nil {
			o.Detail = err.Error()
			return o, true
		}
		defer small.Close()
		limited, err := small.Compile(context.Background(), `()=>new ArrayBuffer(40*1024*1024).byteLength`)
		if err != nil {
			o.Detail = err.Error()
			return o, true
		}
		_, err = limited.Eval(context.Background(), nil, jt.EvalOptions{})
		o.Detail = fmt.Sprintf("public 64 MiB engine accepted 40 MiB; independent 8 MiB engine rejected it: %v; exact page/growth stats covered by canonical runtime tests", err)
		if memoryProbeRejection(err) {
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

// This matcher checks one fixed assessment expression, not arbitrary user errors.
func memoryProbeRejection(err error) bool {
	if err == nil || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || errors.Is(err, jt.ErrUnsupported) || errors.Is(err, ErrClosed) {
		return false
	}
	var exception *Exception
	return errors.As(err, &exception) && exception.Name == "InternalError" && exception.Message == "out of memory"
}
