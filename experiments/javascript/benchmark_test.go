package javascript

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/dop251/goja"
	jt "github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/internal/budget"
)

var resultSink jt.Result
var bytesSink jt.JSONResult
var stringSink string

func workload() (map[string]any, []byte) {
	items := make([]any, 100)
	for i := range items {
		items[i] = map[string]any{"id": i, "price": i + 1, "name": "example"}
	}
	input := map[string]any{"items": items}
	data, _ := json.Marshal(input)
	return input, data
}
func BenchmarkLifecycle(b *testing.B) {
	for _, backend := range []Backend{QuickJS, Goja, OwnedQuickJS} {
		b.Run(string(backend), func(b *testing.B) {
			e, err := New(backend, Config{})
			if err != nil {
				b.Fatal(err)
			}
			defer e.Close()
			ctx := context.Background()
			source := `input=>({total:input.items.reduce((s,x)=>s+x.price,0)})`
			p, err := e.Compile(ctx, source)
			if err != nil {
				b.Fatal(err)
			}
			input, data := workload()
			doc, err := jt.PrepareJSON(ctx, data, jt.PrepareOptions{})
			if err != nil {
				b.Fatal(err)
			}
			got, err := p.Eval(ctx, input, jt.EvalOptions{})
			if err != nil || got.Value.(map[string]any)["total"] != json.Number("5050") {
				b.Fatalf("%#v %v", got, err)
			}
			b.Run("compile", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := e.Compile(ctx, source); err != nil {
						b.Fatal(err)
					}
				}
			})
			if backend != OwnedQuickJS {
				b.Run("runtime-init", func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						if backend == QuickJS {
							rt, err := e.newQuick(ctx)
							if err != nil {
								b.Fatal(err)
							}
							rt.Close()
						} else {
							vm := goja.New()
							if _, err := vm.RunString("1+1"); err != nil {
								b.Fatal(err)
							}
						}
					}
				})
			}

			b.Run("eval-go-value", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					resultSink, err = p.Eval(ctx, input, jt.EvalOptions{})
					if err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("eval-json", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					bytesSink, err = jt.EvalJSON(ctx, p, data, jt.EvalOptions{})
					if err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("prepared-fallback", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					resultSink, err = jt.EvalPrepared(ctx, p, doc, jt.EvalOptions{})
					if err != nil {
						b.Fatal(err)
					}
				}
			})
			identity, err := e.Compile(ctx, `input=>input`)
			if err != nil {
				b.Fatal(err)
			}
			b.Run("json-roundtrip", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					bytesSink, err = jt.EvalJSON(ctx, identity, data, jt.EvalOptions{})
					if err != nil {
						b.Fatal(err)
					}
				}
			})
			for _, n := range []int{4, 16} {
				programs := make([]jt.Program, n)
				for i := range programs {
					programs[i], err = e.Compile(ctx, fmt.Sprintf(`input=>input.items[%d].price`, i))
					if err != nil {
						b.Fatal(err)
					}
				}
				batch, err := jt.NewBatch(programs)
				if err != nil {
					b.Fatal(err)
				}
				b.Run(fmt.Sprintf("sequential-N%d", n), func(b *testing.B) {
					b.ReportAllocs()
					var dst []jt.Result
					for b.Loop() {
						dst, err = batch.EvalPrepared(ctx, doc, dst, jt.EvalOptions{})
						if err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}
func BenchmarkInputConversion(b *testing.B) {
	input, data := workload()
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		limits, err := budget.New(context.Background(), jt.DefaultLimits())
		if err != nil {
			b.Fatal(err)
		}
		stringSink, err = encodeInput(input, limits)
		limits.Close()
		if err != nil {
			b.Fatal(err)
		}
	}
}
