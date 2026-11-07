package jsontrans_test

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	jt "github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/internal/budget"
	"github.com/mellowforge/jsontrans-go/internal/jsonvalue"
	"github.com/mellowforge/jsontrans-go/jsonata"
	"github.com/mellowforge/jsontrans-go/jsonpath"
)

var jsonAPISink any

// Fixed fixtures are generated before timing. Padding isolates token/string
// costs; the large-array fixture separately exercises many small values.
func jsonAPIFixture(size int, array bool) []byte {
	n := 8
	if array {
		n = size / 56
	}
	items := make([]any, n)
	for i := range items {
		items[i] = map[string]any{"price": i + 1, "label": "é-hello", "active": true}
	}
	raw, _ := json.Marshal(map[string]any{"items": items, "padding": ""})
	if len(raw) < size {
		raw, _ = json.Marshal(map[string]any{"items": items, "padding": strings.Repeat("x", size-len(raw))})
	}
	return raw
}

func BenchmarkJSONAPI(b *testing.B) {
	ctx := context.Background()
	a, err := jsonata.New()
	if err != nil {
		b.Fatal(err)
	}
	p, err := jsonpath.New()
	if err != nil {
		b.Fatal(err)
	}
	compile := func(engine jt.Processor, expr string) jt.Program {
		program, err := engine.Compile(ctx, expr)
		if err != nil {
			b.Fatal(err)
		}
		return program
	}
	selectA := compile(a, "items[0].price")
	selectP := compile(p, "$.items[0].price")
	identity := compile(a, "$")
	programs := []jt.Program{selectA, selectP, compile(a, "$sum(items.price)"), compile(p, "$.items[1].price")}
	fallback := compile(a, "$uppercase(items[0].label)")
	for _, fixture := range []struct {
		name  string
		size  int
		array bool
	}{
		{"1KiB", 1024, false}, {"8KiB", 8192, false}, {"64KiB", 65536, false}, {"array1MiB", 1 << 20, true},
	} {
		b.Run(fixture.name, func(b *testing.B) {
			raw := jsonAPIFixture(fixture.size, fixture.array)
			makeBudget := func() *budget.Budget {
				bb, err := budget.New(ctx, budget.Limits{})
				if err != nil {
					b.Fatal(err)
				}
				return bb
			}
			bb := makeBudget()
			value, err := jsonvalue.Decode(raw, bb)
			bb.Close()
			if err != nil {
				b.Fatal(err)
			}
			doc, err := jt.PrepareJSON(ctx, raw, jt.PrepareOptions{})
			if err != nil {
				b.Fatal(err)
			}
			b.Run("decode", func(b *testing.B) {
				b.SetBytes(int64(len(raw)))
				b.ReportAllocs()
				for b.Loop() {
					bb := makeBudget()
					v, err := jsonvalue.Decode(raw, bb)
					bb.Close()
					if err != nil {
						b.Fatal(err)
					}
					jsonAPISink = v
				}
			})
			b.Run("encode", func(b *testing.B) {
				bb := makeBudget()
				encoded, err := jsonvalue.Encode(value, bb)
				bb.Close()
				if err != nil {
					b.Fatal(err)
				}
				bb = makeBudget()
				roundtrip, err := jsonvalue.Decode(encoded, bb)
				bb.Close()
				if err != nil || !reflect.DeepEqual(value, roundtrip) {
					b.Fatalf("encoding changed value: %v", err)
				}
				b.SetBytes(int64(len(raw)))
				b.ReportAllocs()
				for b.Loop() {
					bb := makeBudget()
					v, err := jsonvalue.Encode(value, bb)
					bb.Close()
					if err != nil {
						b.Fatal(err)
					}
					jsonAPISink = v
				}
			})
			b.Run("prepare", func(b *testing.B) {
				b.SetBytes(int64(len(raw)))
				b.ReportAllocs()
				for b.Loop() {
					v, err := jt.PrepareJSON(ctx, raw, jt.PrepareOptions{})
					if err != nil {
						b.Fatal(err)
					}
					jsonAPISink = v
				}
			})
			for _, engine := range []struct {
				name string
				p    jt.Program
				want string
			}{{"jsonata", selectA, "1"}, {"jsonpath", selectP, "[1]"}} {
				b.Run("eval_json/"+engine.name, func(b *testing.B) {
					got, err := jt.EvalJSON(ctx, engine.p, raw, jt.EvalOptions{})
					if err != nil || !got.Defined || string(got.Value) != engine.want {
						b.Fatalf("incorrect result: %s %v", got.Value, err)
					}
					b.ReportAllocs()
					for b.Loop() {
						v, err := jt.EvalJSON(ctx, engine.p, raw, jt.EvalOptions{})
						if err != nil {
							b.Fatal(err)
						}
						jsonAPISink = v
					}
				})
			}
			b.Run("large_output", func(b *testing.B) {
				got, err := jt.EvalJSON(ctx, identity, raw, jt.EvalOptions{})
				if err != nil || !got.Defined {
					b.Fatal(err)
				}
				bb := makeBudget()
				roundtrip, err := jsonvalue.Decode(got.Value, bb)
				bb.Close()
				if err != nil || !reflect.DeepEqual(value, roundtrip) {
					b.Fatalf("identity changed value: %v", err)
				}
				b.ReportAllocs()
				for b.Loop() {
					v, err := jt.EvalJSON(ctx, identity, raw, jt.EvalOptions{})
					if err != nil {
						b.Fatal(err)
					}
					jsonAPISink = v
				}
			})
			// Large arrays are a boundary/conversion probe, not 64 expensive full scans.
			if fixture.array {
				return
			}
			for _, n := range []int{1, 16, 64} {
				rules := make([]jt.Program, n)
				for i := range rules {
					rules[i] = programs[i%len(programs)]
				}
				batch, err := jt.NewBatch(rules)
				if err != nil {
					b.Fatal(err)
				}
				want := make([]jt.Result, n)
				for i, p := range rules {
					want[i], err = p.Eval(ctx, value, jt.EvalOptions{})
					if err != nil {
						b.Fatal(err)
					}
				}
				for _, prepare := range []bool{false, true} {
					name := fmt.Sprintf("reuse/N%d", n)
					if prepare {
						name = fmt.Sprintf("prepare_batch/N%d", n)
					}
					b.Run(name, func(b *testing.B) {
						dst := make([]jt.Result, n)
						got, err := batch.EvalPrepared(ctx, doc, dst, jt.EvalOptions{})
						if err != nil || !reflect.DeepEqual(got, want) {
							b.Fatalf("batch changed results: %v", err)
						}
						b.ReportAllocs()
						for b.Loop() {
							d := doc
							if prepare {
								d, err = jt.PrepareJSON(ctx, raw, jt.PrepareOptions{})
								if err != nil {
									b.Fatal(err)
								}
							}
							v, err := batch.EvalPrepared(ctx, d, dst, jt.EvalOptions{})
							if err != nil {
								b.Fatal(err)
							}
							jsonAPISink = v
						}
					})
				}
			}
			b.Run("reuse_mixed/N16", func(b *testing.B) {
				rules := make([]jt.Program, 16)
				for i := range rules {
					rules[i] = programs[i%len(programs)]
				}
				rules[7] = fallback
				rules[15] = fallback
				batch, err := jt.NewBatch(rules)
				if err != nil {
					b.Fatal(err)
				}
				dst := make([]jt.Result, 16)
				got, err := batch.EvalPrepared(ctx, doc, dst, jt.EvalOptions{})
				if err != nil || got[7].Value != "É-HELLO" || got[15].Value != "É-HELLO" {
					b.Fatalf("fallback: %#v %v", got, err)
				}
				b.ReportAllocs()
				for b.Loop() {
					v, err := batch.EvalPrepared(ctx, doc, dst, jt.EvalOptions{})
					if err != nil {
						b.Fatal(err)
					}
					jsonAPISink = v
				}
			})
		})
	}
}
