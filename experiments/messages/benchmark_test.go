package messages

import (
	"context"
	"fmt"
	jt "github.com/mellowforge/jsontrans-go"
	"os"
	"runtime"
	"strconv"
	"testing"
)

var outputSink []jt.JSONResult

func BenchmarkBytes(b *testing.B) {
	for _, engine := range []string{"jsonata", "jsonpath"} {
		for _, kind := range []string{"scalar", "object", "array", "fallback", "bindings", "limits", "host", "output", "large-array"} {
			if engine == "jsonpath" && kind == "bindings" {
				continue
			} // Bindings are unsupported by the language API.
			sizes := []int{512, 1024, 8192, 65536}
			if kind == "large-array" {
				sizes = []int{1 << 20}
			}
			for _, size := range sizes {
				for _, n := range []int{1, 4, 16} {
					if kind == "large-array" && n > 1 {
						continue
					}
					b.Run(fmt.Sprintf("%s/%s/%d/N%d", engine, kind, size, n), func(b *testing.B) { benchmarkFixture(b, engine, kind, size, n, false) })
				}
			}
		}
	}
}
func BenchmarkParallelBytes(b *testing.B) {
	for _, engine := range []string{"jsonata", "jsonpath"} {
		for _, kind := range []string{"scalar", "object", "array", "bindings"} {
			if engine == "jsonpath" && kind == "bindings" {
				continue
			}
			for _, n := range []int{1, 4, 16} {
				b.Run(fmt.Sprintf("%s/%s/1024/N%d", engine, kind, n), func(b *testing.B) { benchmarkRoutedFixture(b, engine, kind, n) })
			}
		}
	}
}

func benchmarkRoutedFixture(b *testing.B, engine, kind string, n int) {
	type route struct {
		plan    *Plan
		fixture Fixture
		index   int
	}
	routes := map[string]route{}
	subjects := []string{}
	for i := range 8 {
		fixture, err := NewFixture(engine, kind, 1024, n)
		if err != nil {
			b.Fatal(err)
		}
		plan, err := NewPlan(context.Background(), fixture.Config)
		if err != nil {
			b.Fatal(err)
		}
		defer plan.Close()
		subject := fmt.Sprintf("input.%d", i)
		subjects = append(subjects, subject)
		routes[subject] = route{plan, fixture, i}
	}
	for _, strategy := range benchmarkOrder() {
		b.Run(string(strategy), func(b *testing.B) {
			for _, r := range routes {
				w := r.plan.NewWorker()
				for _, sample := range r.fixture.Samples {
					got, err := w.Eval(context.Background(), sample.Input, strategy)
					if err != nil {
						b.Fatal(err)
					}
					if err = CheckOutputs(got, sample.Want); err != nil {
						b.Fatal(err)
					}
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				workers := make([]*Worker, 8)
				for _, r := range routes {
					workers[r.index] = r.plan.NewWorker()
				}
				i := 0
				for pb.Next() {
					r := routes[subjects[i%len(subjects)]]
					sample := r.fixture.Samples[(i/len(subjects))%len(r.fixture.Samples)]
					out, err := workers[r.index].Eval(context.Background(), sample.Input, strategy)
					if err != nil {
						b.Error(err)
						return
					}
					runtime.KeepAlive(out)
					i++
				}
			})
		})
	}
}

func BenchmarkJavaScriptBytes(b *testing.B) {
	for _, n := range []int{1, 4} {
		b.Run(fmt.Sprintf("javascript/object/1024/N%d", n), func(b *testing.B) { benchmarkFixture(b, "javascript", "object", 1024, n, false) })
	}
}
func benchmarkFixture(b *testing.B, engine, kind string, size, n int, parallel bool) {
	f, err := NewFixture(engine, kind, size, n)
	if err != nil {
		b.Fatal(err)
	}
	p, err := NewPlan(context.Background(), f.Config)
	if err != nil {
		b.Fatal(err)
	}
	defer p.Close()
	for _, strategy := range benchmarkOrder() {
		b.Run(string(strategy), func(b *testing.B) {
			w := p.NewWorker()
			for _, sample := range f.Samples {
				got, err := w.Eval(context.Background(), sample.Input, strategy)
				if err != nil {
					b.Fatal(err)
				}
				if err = CheckOutputs(got, sample.Want); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportAllocs()
			if parallel {
				b.ResetTimer()
				b.RunParallel(func(pb *testing.PB) {
					worker := p.NewWorker()
					i := 0
					for pb.Next() {
						out, err := worker.Eval(context.Background(), f.Samples[i%len(f.Samples)].Input, strategy)
						if err != nil {
							b.Error(err)
							return
						}
						runtime.KeepAlive(out)
						i++
					}
				})
			} else {
				i := 0
				for b.Loop() {
					out, err := w.Eval(context.Background(), f.Samples[i%len(f.Samples)].Input, strategy)
					if err != nil {
						b.Fatal(err)
					}
					outputSink = out
					i++
				}
			}
		})
	}
}

func benchmarkOrder() []Strategy {
	n, _ := strconv.Atoi(os.Getenv("MESSAGE_STRATEGY_ORDER"))
	n = ((n % 3) + 3) % 3
	out := append([]Strategy(nil), Strategies[n:]...)
	return append(out, Strategies[:n]...)
}
