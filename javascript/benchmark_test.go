package javascript_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	jt "github.com/mellowforge/jsontrans-go"
	js "github.com/mellowforge/jsontrans-go/javascript"
)

func BenchmarkJavaScript(b *testing.B) {
	ctx := context.Background()
	config := js.Config{Limits: jt.Limits{Timeout: 30 * time.Second}}
	b.Run("NewClose", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			e, err := js.New(ctx, config)
			if err != nil {
				b.Fatal(err)
			}
			if err = e.Close(); err != nil {
				b.Fatal(err)
			}
		}
	})
	e, err := js.New(ctx, config)
	if err != nil {
		b.Fatal(err)
	}
	defer e.Close()
	const source = `(input)=>({total:input.items.reduce((sum,item)=>sum+item.price,0)})`
	data := []byte(`{"items":[{"price":7},{"price":14}]}`)
	var input any
	if err := json.Unmarshal(data, &input); err != nil {
		b.Fatal(err)
	}
	doc, err := jt.PrepareJSON(ctx, data, jt.PrepareOptions{})
	if err != nil {
		b.Fatal(err)
	}
	p, err := e.Compile(ctx, source)
	if err != nil {
		b.Fatal(err)
	}
	programs := []jt.Program{p}
	for _, expression := range []string{`x=>x.items.length`, `x=>x.items.map(i=>i.price)`, `x=>x.items[0]`} {
		p, err := e.Compile(ctx, expression)
		if err != nil {
			b.Fatal(err)
		}
		programs = append(programs, p)
	}
	batch, err := jt.NewBatch(programs)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("Compile", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := e.Compile(ctx, source); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("Eval", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := p.Eval(ctx, input, jt.EvalOptions{}); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("EvalJSON", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := jt.EvalJSON(ctx, p, data, jt.EvalOptions{}); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("PreparedFallback", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := jt.EvalPrepared(ctx, p, doc, jt.EvalOptions{}); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("PreparedBatch4", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := batch.EvalPrepared(ctx, doc, nil, jt.EvalOptions{}); err != nil {
				b.Fatal(err)
			}
		}
	})
}
