// Command hardening runs a bounded public-API failure/recovery soak. It is a
// development tool, not part of the processor's execution API.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	jt "github.com/mellowforge/jsontrans-go"
	js "github.com/mellowforge/jsontrans-go/javascript"
)

type settings struct{ Iterations, Workers, Cycles int }
type outcome struct {
	Calls   int   `json:"calls"`
	TotalNS int64 `json:"total_ns"`
	MaxNS   int64 `json:"max_ns"`
}
type memorySample struct {
	Stage      string `json:"stage,omitempty"`
	ElapsedNS  int64  `json:"elapsed_ns,omitempty"`
	Cycle      int    `json:"completed_cycles"`
	HeapAlloc  uint64 `json:"heap_alloc_bytes"`
	HeapInuse  uint64 `json:"heap_inuse_bytes"`
	Sys        uint64 `json:"go_sys_bytes"`
	Goroutines int    `json:"goroutines"`
}
type report struct {
	Batches        int                `json:"completed_batches,omitempty"`
	Campaign       *campaignSettings  `json:"campaign,omitempty"`
	Status         string             `json:"status,omitempty"`
	Complete       *bool              `json:"complete,omitempty"`
	Failure        string             `json:"failure,omitempty"`
	GoVersion      string             `json:"go_version"`
	Platform       string             `json:"platform"`
	Settings       settings           `json:"settings"`
	GuestCapBytes  int                `json:"guest_cap_bytes"`
	ElapsedNS      int64              `json:"elapsed_ns"`
	ConstructionNS []int64            `json:"construction_ns"`
	CompilationNS  []int64            `json:"compilation_ns"`
	FirstEvalNS    []int64            `json:"first_eval_ns"`
	CloseNS        []int64            `json:"close_ns"`
	Outcomes       map[string]outcome `json:"outcomes"`
	Recoveries     int                `json:"verified_recoveries"`
	Memory         []memorySample     `json:"post_gc_memory"`
}

func main() {
	s := settings{}
	c := campaignSettings{}
	var progressPath string
	flag.DurationVar(&c.Duration, "duration", 0, "duration mode, 1s..2h; unset keeps count mode")
	flag.DurationVar(&c.Recycle, "recycle", 0, "engine lifetime in duration mode; zero keeps one engine")
	flag.DurationVar(&c.SampleInterval, "sample-interval", 30*time.Second, "GC sampling interval in duration mode")
	flag.IntVar(&c.GuestMiB, "guest-mib", 8, "duration-mode guest cap: 2, 8 or 64 MiB")
	flag.StringVar(&progressPath, "progress", "", "exclusive-create JSONL campaign progress file")
	flag.IntVar(&s.Iterations, "iterations", 100, "iterations per worker per cycle (1..100000)")
	flag.IntVar(&s.Workers, "workers", 4, "concurrent evaluators (1..32)")
	flag.IntVar(&s.Cycles, "cycles", 3, "independent engine lifecycles (1..100)")
	flag.Parse()
	campaignMode := false
	countModeFlags := false
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "duration", "recycle", "sample-interval", "guest-mib", "progress":
			campaignMode = true
		case "iterations", "cycles":
			countModeFlags = true
		}
	})
	if campaignMode && countModeFlags {
		fmt.Fprintln(os.Stderr, "-iterations and -cycles cannot be combined with duration-mode options")
		os.Exit(2)
	}
	var r report
	var err error
	if campaignMode {
		c.Workers = s.Workers
		r, err = campaignCLI(c, progressPath)
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		r, err = run(ctx, s)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		if !campaignMode {
			os.Exit(1)
		}
	}
	if encodeErr := json.NewEncoder(os.Stdout).Encode(r); encodeErr != nil {
		fmt.Fprintln(os.Stderr, encodeErr)
		os.Exit(1)
	}
	if err != nil {
		os.Exit(1)
	}
}

func sample(cycle int) memorySample {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return memorySample{Cycle: cycle, HeapAlloc: m.HeapAlloc, HeapInuse: m.HeapInuse, Sys: m.Sys, Goroutines: runtime.NumGoroutine()}
}

func run(ctx context.Context, s settings) (report, error) {
	r := report{GoVersion: runtime.Version(), Platform: runtime.GOOS + "/" + runtime.GOARCH, Settings: s,
		GuestCapBytes: 8 << 20, Outcomes: make(map[string]outcome)}
	if s.Iterations < 1 || s.Iterations > 100000 || s.Workers < 1 || s.Workers > 32 || s.Cycles < 1 || s.Cycles > 100 || int64(s.Iterations)*int64(s.Workers)*int64(s.Cycles) > 1000000 {
		return r, errors.New("invalid soak settings: at most one million iterations, 100000 per worker, 32 workers, 100 cycles")
	}
	start := time.Now()
	r.Memory = append(r.Memory, sample(0))
	for cycle := range s.Cycles {
		if err := ctx.Err(); err != nil {
			return r, err
		}
		started := time.Now()
		e, err := js.New(ctx, js.Config{GuestMemoryBytes: r.GuestCapBytes, Limits: jt.Limits{Timeout: 100 * time.Second}})
		if err != nil {
			return r, err
		}
		r.ConstructionNS = append(r.ConstructionNS, time.Since(started).Nanoseconds())
		err = exercise(ctx, e, s, &r)
		started = time.Now()
		closeErr := e.Close()
		r.CloseNS = append(r.CloseNS, time.Since(started).Nanoseconds())
		if err = errors.Join(err, closeErr); err != nil {
			return r, err
		}
		if _, err = e.Compile(ctx, `()=>1`); !errors.Is(err, js.ErrClosed) {
			return r, fmt.Errorf("closed engine admitted compilation: %v", err)
		}
		r.Memory = append(r.Memory, sample(cycle+1))
	}
	r.ElapsedNS = time.Since(start).Nanoseconds()
	return r, nil
}

func exercise(ctx context.Context, e *js.Engine, s settings, r *report) error {
	sources := []string{
		`(input,b)=>{globalThis.n=(globalThis.n||0)+1;const sum=input.x+b.x;input.x=999;b.x=999;return {sum,n:globalThis.n}}`,
		`()=>{throw new TypeError('soak')}`,
		`()=>{const x={};x.self=x;return x}`,
		`()=>{while(true){}}`,
		fmt.Sprintf(`()=>new ArrayBuffer(%d).byteLength`, r.GuestCapBytes*5),
	}
	names := []string{"success", "exception", "conversion", "cancellation", "allocation"}
	programs := make([]jt.Program, len(sources))
	started := time.Now()
	for i, source := range sources {
		p, err := e.Compile(ctx, source)
		if err != nil {
			return err
		}
		programs[i] = p
	}
	healthy, err := e.Compile(ctx, `()=>42`)
	if err != nil {
		return err
	}
	// Record only compilation here, excluding the subsequent worker evaluations.
	r.CompilationNS = append(r.CompilationNS, time.Since(started).Nanoseconds())
	started = time.Now()
	first, err := healthy.Eval(ctx, nil, jt.EvalOptions{})
	r.FirstEvalNS = append(r.FirstEvalNS, time.Since(started).Nanoseconds())
	if err != nil || !first.Defined || first.Value != json.Number("42") {
		return fmt.Errorf("first evaluation: %#v, %v", first, err)
	}
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	type workerResult struct {
		outcomes   [5]outcome
		recoveries int
	}
	results := make([]workerResult, s.Workers)
	var wg sync.WaitGroup
	for worker := range s.Workers {
		wg.Go(func() {
			for iteration := range s.Iterations {
				if ctx.Err() != nil {
					return
				}
				kind := iteration % len(programs)
				input, bindings := map[string]any{"x": 2}, map[string]any{"x": 3}
				callCtx := ctx
				stop := func() {}
				if kind == 3 {
					callCtx, stop = context.WithTimeout(ctx, 20*time.Millisecond)
				}
				started := time.Now()
				got, err := programs[kind].Eval(callCtx, input, jt.EvalOptions{Bindings: bindings})
				duration := time.Since(started).Nanoseconds()
				stop()
				if failure := check(kind, got, err); failure != nil {
					cancel(fmt.Errorf("worker %d iteration %d %s: %w", worker, iteration, names[kind], failure))
					return
				}
				if input["x"] != 2 || bindings["x"] != 3 {
					cancel(errors.New("caller input/bindings mutated"))
					return
				}
				got, err = healthy.Eval(ctx, nil, jt.EvalOptions{})
				if err != nil || !got.Defined || got.Value != json.Number("42") {
					cancel(fmt.Errorf("failed recovery: %#v, %v", got, err))
					return
				}
				o := &results[worker].outcomes[kind]
				o.Calls++
				o.TotalNS += duration
				o.MaxNS = max(o.MaxNS, duration)
				results[worker].recoveries++
			}
		})
	}
	wg.Wait()
	if err = context.Cause(ctx); err != nil {
		return err
	}
	for _, w := range results {
		r.Recoveries += w.recoveries
		for i, o := range w.outcomes {
			total := r.Outcomes[names[i]]
			total.Calls += o.Calls
			total.TotalNS += o.TotalNS
			total.MaxNS = max(total.MaxNS, o.MaxNS)
			r.Outcomes[names[i]] = total
		}
	}
	return nil
}

func check(kind int, got jt.Result, err error) error {
	var ex *js.Exception
	var d *jt.Error
	if kind != 0 && (got.Defined || got.Value != nil) {
		return fmt.Errorf("failure returned a defined result: %#v", got)
	}
	if kind != 3 && (errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || errors.Is(err, js.ErrClosed)) {
		return fmt.Errorf("unexpected cancellation/closure: %w", err)
	}
	switch kind {
	case 0:
		v, ok := got.Value.(map[string]any)
		if err == nil && got.Defined && ok && v["sum"] == json.Number("5") && v["n"] == json.Number("1") {
			return nil
		}
	case 1:
		if errors.As(err, &ex) && ex.Name == "TypeError" && ex.Message == "soak" && !errors.Is(err, jt.ErrLimit) && !errors.Is(err, jt.ErrConversion) {
			return nil
		}
	case 2:
		if errors.Is(err, jt.ErrConversion) && !errors.Is(err, jt.ErrLimit) && errors.As(err, &d) && d.Phase == jt.PhaseOutput {
			return nil
		}
	case 3:
		if errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, js.ErrClosed) {
			return nil
		}
	case 4:
		// This exact literal allocation fixture must fail for guest OOM, not a
		// scheduling deadline. Text is asserted only here, never used by the adapter
		// to classify arbitrary user exceptions as trusted resource failures.
		if errors.As(err, &ex) && ex.Name == "InternalError" && ex.Message == "out of memory" && !errors.Is(err, jt.ErrLimit) && !errors.Is(err, jt.ErrConversion) && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
			return nil
		}
	}
	return fmt.Errorf("unexpected result %#v, error %v", got, err)
}
