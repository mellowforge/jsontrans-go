package jsontrans_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	jt "github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/jsonata"
)

// Preparation is excluded: this measures the public API for a document shared
// across expressions. Direct repeats validation; all paths include encoding.
func BenchmarkPreparedJSON(b *testing.B) {
	ctx := context.Background()
	input := []byte(`{"a":{"n":2},"padding":"` + strings.Repeat("x", 1024) + `"}`)
	d, err := jt.PrepareJSON(ctx, input, jt.PrepareOptions{})
	if err != nil {
		b.Fatal(err)
	}
	e, err := jsonata.New()
	if err != nil {
		b.Fatal(err)
	}
	for _, count := range []int{1, 16} {
		programs := make([]jt.Program, count)
		for i := range programs {
			programs[i], err = e.Compile(ctx, fmt.Sprintf(`{"n":a.n+%d}`, i))
			if err != nil {
				b.Fatal(err)
			}
		}
		batch, err := jt.NewBatch(programs)
		if err != nil {
			b.Fatal(err)
		}
		for _, mode := range []string{"direct", "prepared", "batch"} {
			b.Run(fmt.Sprintf("%s/%d", mode, count), func(b *testing.B) {
				b.ReportAllocs()
				var dst []jt.JSONResult
				for b.Loop() {
					if mode == "batch" {
						dst, err = batch.EvalPreparedJSON(ctx, d, dst, jt.EvalOptions{})
						if err != nil {
							b.Fatal(err)
						}
						continue
					}
					for _, p := range programs {
						var r jt.JSONResult
						if mode == "direct" {
							r, err = jt.EvalJSON(ctx, p, input, jt.EvalOptions{})
						} else {
							r, err = jt.EvalPreparedJSON(ctx, p, d, jt.EvalOptions{})
						}
						if err != nil || !r.Defined {
							b.Fatalf("%+v %v", r, err)
						}
					}
				}
			})
		}
	}
}
