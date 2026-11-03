package javascript

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	jt "github.com/mellowforge/jsontrans-go"
)

func TestRegistryForwardsSupportedLimitsOnly(t *testing.T) {
	ctx := context.Background()
	for _, backend := range []Backend{QuickJS, Goja, OwnedQuickJS} {
		t.Run(string(backend), func(t *testing.T) {
			// The fixture configures a nondefault timeout, exercising synthesized
			// forwarding even when the per-call Limits pointer is nil.
			e := newTestEngine(t, backend)
			transformer, err := jt.New(map[string]jt.Processor{"js": e})
			if err != nil {
				t.Fatal(err)
			}
			source := `input=>input.x+1`
			p, err := transformer.Compile(ctx, "js", source)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := e.Compile(ctx, source)
			if err != nil {
				t.Fatal(err)
			}
			doc, err := jt.PrepareJSON(ctx, []byte(`{"x":1}`), jt.PrepareOptions{})
			if err != nil {
				t.Fatal(err)
			}
			batch, err := jt.NewBatch([]jt.Program{p, raw})
			if err != nil {
				t.Fatal(err)
			}
			for _, options := range []jt.EvalOptions{{}, {Limits: &jt.Limits{}}, {Limits: &jt.Limits{Timeout: time.Second}}} {
				check := func(r jt.Result, err error) {
					t.Helper()
					if err != nil || !r.Defined || r.Value != json.Number("2") {
						t.Errorf("got %#v error %v options %+v", r, err, options.Limits)
					}
				}
				check(p.Eval(ctx, map[string]any{"x": 1}, options))
				check(jt.EvalPrepared(ctx, p, doc, options))
				check(jt.EvalPrepared(ctx, raw, doc, options))
				r, err := transformer.ProcessJSON(ctx, "js", []byte(`{"x":1}`), source, options)
				if err != nil || !r.Defined || string(r.Value) != "2" {
					t.Errorf("JSON: %#v %v", r, err)
				}
				results, err := batch.EvalPrepared(ctx, doc, nil, options)
				if err != nil {
					t.Errorf("batch: %v", err)
				} else {
					for _, r := range results {
						check(r, nil)
					}
				}
			}
			for _, limits := range []jt.Limits{{MaxSteps: jt.DefaultLimits().MaxSteps}, {RegexTimeout: jt.DefaultLimits().RegexTimeout}} {
				options := jt.EvalOptions{Limits: &limits}
				if _, err := p.Eval(ctx, map[string]any{"x": 1}, options); !errors.Is(err, jt.ErrUnsupported) {
					t.Errorf("explicit limit ordinary: %v", err)
				}
				if _, err := jt.EvalPrepared(ctx, p, doc, options); !errors.Is(err, jt.ErrUnsupported) {
					t.Errorf("explicit limit prepared: %v", err)
				}
				if _, err := batch.EvalPrepared(ctx, doc, nil, options); !errors.Is(err, jt.ErrUnsupported) {
					t.Errorf("explicit limit batch: %v", err)
				}
			}

		})
	}
}
