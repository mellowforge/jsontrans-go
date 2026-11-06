package messages

import (
	"context"
	jt "github.com/mellowforge/jsontrans-go"
	"testing"
)

func TestFixtureOutputs(t *testing.T) {
	for _, engine := range []string{"jsonata", "jsonpath", "javascript"} {
		kinds := []string{"scalar", "object", "array", "fallback", "limits", "host", "output"}
		if engine == "jsonata" {
			kinds = append(kinds, "bindings")
		}
		if engine == "javascript" {
			kinds = []string{"object"}
		}
		for _, kind := range kinds {
			t.Run(engine+"/"+kind, func(t *testing.T) {
				f, err := NewFixture(engine, kind, 1024, 4)
				if err != nil {
					t.Fatal(err)
				}
				p, err := NewPlan(context.Background(), f.Config)
				if err != nil {
					t.Fatal(err)
				}
				defer p.Close()
				w := p.NewWorker()
				for _, s := range Strategies {
					for _, sample := range f.Samples {
						got, err := w.Eval(context.Background(), sample.Input, s)
						if err != nil {
							t.Fatal(s, err)
						}
						if err = CheckOutputs(got, sample.Want); err != nil {
							t.Fatal(s, err)
						}
					}
				}
			})
		}
	}
}

func TestOutputCheckRejectsCorruption(t *testing.T) {
	for _, pair := range [][2]string{{`"\ud800"`, `"\ud801"`}, {`1 true`, `1`}} {
		err := CheckOutputs([]jt.JSONResult{{Defined: true, Value: []byte(pair[0])}}, []jt.JSONResult{{Defined: true, Value: []byte(pair[1])}})
		if err == nil {
			t.Fatalf("accepted corrupted output %q == %q", pair[0], pair[1])
		}
	}
}

func TestFixtureDistinctObjectFanoutAndBounds(t *testing.T) {
	f, err := NewFixture("jsonpath", "object", 1024, 16)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, expr := range f.Config.Expressions {
		if seen[expr] {
			t.Fatalf("duplicate object transformation: %s", expr)
		}
		seen[expr] = true
	}
	if _, err = NewFixture("jsonata", "scalar", 1024, 17); err == nil {
		t.Fatal("accepted unsupported fanout")
	}
}
