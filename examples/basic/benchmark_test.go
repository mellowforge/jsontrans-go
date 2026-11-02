package main

import (
	"context"
	"testing"

	jt "github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/jsonata"
	"github.com/mellowforge/jsontrans-go/jsonpath"
)

func BenchmarkProcessors(b *testing.B) {
	ctx := context.Background()
	path, err := jsonpath.New()
	if err != nil {
		b.Fatal(err)
	}
	ata, err := jsonata.New()
	if err != nil {
		b.Fatal(err)
	}
	input := map[string]any{"items": []any{map[string]any{"price": 4}, map[string]any{"price": 7}}}
	for _, tc := range []struct {
		name, expression string
		engine           jt.Processor
	}{{"JSONPath", "$.items[*].price", path}, {"JSONata", "$sum(items.price)", ata}} {
		b.Run(tc.name+"/Compile", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := tc.engine.Compile(ctx, tc.expression); err != nil {
					b.Fatal(err)
				}
			}
		})
		p, err := tc.engine.Compile(ctx, tc.expression)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(tc.name+"/Eval", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := p.Eval(ctx, input, jt.EvalOptions{}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
