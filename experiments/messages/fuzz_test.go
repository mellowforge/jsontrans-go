package messages

import (
	"context"
	"errors"
	jt "github.com/mellowforge/jsontrans-go"
	"testing"
)

func FuzzByteStrategies(f *testing.F) {
	for _, raw := range []string{`null`, `{"metrics":{"v0":1}}`, `{"metrics":{"v0":"\ud800"}}`, `{"metrics":{"v0":[null,1]}}`, `{"x":1e9999}`, `{} {}`, `{"metrics":{"v0":1,"v0":2}}`} {
		f.Add([]byte(raw))
	}
	plans := []*Plan{}
	for _, engine := range []string{"jsonata", "jsonpath"} {
		expr := "metrics.v0"
		if engine == "jsonpath" {
			expr = "$.metrics.v0"
		}
		p, err := NewPlan(context.Background(), Config{Engine: engine, Expressions: []string{expr}})
		if err != nil {
			f.Fatal(err)
		}
		plans = append(plans, p)
		f.Cleanup(func() { p.Close() })
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 8192 {
			return
		}
		for _, p := range plans {
			want, oldErr := p.NewWorker().Eval(context.Background(), raw, Direct)
			for _, strategy := range []Strategy{Prepared, Batched} {
				got, err := p.NewWorker().Eval(context.Background(), raw, strategy)
				// Paths charge actual work differently; counter exhaustion is not a
				// parsing or value mismatch and is covered by explicit bounded tests.
				if errors.Is(oldErr, jt.ErrLimit) || errors.Is(err, jt.ErrLimit) {
					continue
				}
				if (oldErr == nil) != (err == nil) {
					t.Fatalf("%s direct=%v prepared=%v input=%q", strategy, oldErr, err, raw)
				}
				if oldErr == nil {
					if err = CheckOutputs(got, want); err != nil {
						t.Fatal(strategy, err)
					}
				}
			}
		}
	})
}
