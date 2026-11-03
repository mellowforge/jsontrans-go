package jsontrans_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	jt "github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/jsonata"
	"github.com/mellowforge/jsontrans-go/jsonpath"
)

// The recursive scanner must stop at the depth limit; without the check,
// hostile nesting would recurse without bound.
func TestPrepareRejectsNestingBeyondDepthLimit(t *testing.T) {
	deep := strings.Repeat("[", 100_000) + strings.Repeat("]", 100_000)
	_, err := jt.PrepareJSON(context.Background(), []byte(deep), jt.PrepareOptions{})
	if !errors.Is(err, jt.ErrLimit) {
		t.Fatalf("got %v, want ErrLimit", err)
	}
}

// Expression size limits apply to registry compilation, each engine, and
// expressions built at runtime by $eval.
func TestExpressionSizeLimit(t *testing.T) {
	ctx := context.Background()
	limits := jt.Limits{MaxExpressionBytes: 16}
	long := "1" + strings.Repeat("+1", 20)
	ata, err := jsonata.New(jsonata.WithLimits(limits))
	if err != nil {
		t.Fatal(err)
	}
	path, err := jsonpath.New(jsonpath.WithLimits(limits))
	if err != nil {
		t.Fatal(err)
	}
	x, err := jt.New(map[string]jt.Processor{"jsonata": ata})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := x.Compile(ctx, "jsonata", long); !errors.Is(err, jt.ErrLimit) {
		t.Errorf("registry: %v", err)
	}
	if _, err := ata.Compile(ctx, long); !errors.Is(err, jt.ErrLimit) {
		t.Errorf("jsonata: %v", err)
	}
	if _, err := path.Compile(ctx, "$"+strings.Repeat(".a", 20)); !errors.Is(err, jt.ErrLimit) {
		t.Errorf("jsonpath: %v", err)
	}
	program, err := ata.Compile(ctx, "$eval(s)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := program.Eval(ctx, map[string]any{"s": long}, jt.EvalOptions{}); !errors.Is(err, jt.ErrLimit) {
		t.Errorf("$eval: %v", err)
	}
}

// One compiled JSONata program serves concurrent evaluations; run with -race.
func TestJSONataProgramsEvaluateConcurrently(t *testing.T) {
	ctx := context.Background()
	engine, err := jsonata.New()
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"items":[{"n":"b","v":2,"t":["x"]},{"n":"a","v":1,"t":["y","x"]}],"s":"a-b-c"}`)
	expressions := []string{
		`items^(n).v`,
		`items{n: $sum(v)}`,
		`items.t[$ = "x"].%.n`,
		`$map(items, function($i){ $i.v * 2 })`,
		`$split(s, /-/)`,
		`($f := function($x){ $x + 1 }; $f(items[0].v))`,
		`items@$i.t@$t.{"n": $i.n, "t": $t}`,
		`$eval("items.v")`,
	}
	document, err := jt.PrepareJSON(ctx, input, jt.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, expression := range expressions {
		program, err := engine.Compile(ctx, expression)
		if err != nil {
			t.Fatal(err)
		}
		want, err := jt.EvalJSON(ctx, program, input, jt.EvalOptions{})
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		for i := range 8 {
			wg.Go(func() {
				var got jt.JSONResult
				var err error
				if i%2 == 0 {
					got, err = jt.EvalJSON(ctx, program, input, jt.EvalOptions{})
				} else {
					var r jt.Result
					r, err = jt.EvalPrepared(ctx, program, document, jt.EvalOptions{})
					got.Defined = r.Defined
					if err == nil && r.Defined {
						got.Value, err = jsonMarshal(r.Value)
					}
				}
				if err != nil || got.Defined != want.Defined || string(got.Value) != string(want.Value) {
					t.Errorf("%s: got %s, %v; want %s", expression, got.Value, err, want.Value)
				}
			})
		}
		wg.Wait()
	}
}

// Batch-local prefix memoization agrees with ordinary evaluation when shared
// parents are missing, null, scalar, arrays or duplicated.
func TestBatchMemoizationMatchesOrdinaryEvaluation(t *testing.T) {
	ctx := context.Background()
	engine, err := jsonata.New()
	if err != nil {
		t.Fatal(err)
	}
	expressions := []string{`a.b`, `a.c`, `a.b.d`, `a.b = 1`, `$exists(a.b)`, `a.c > 0`, `$count(a.b)`, `a.b.d = 2`}
	programs := make([]jt.Program, len(expressions))
	for i, expression := range expressions {
		if programs[i], err = engine.Compile(ctx, expression); err != nil {
			t.Fatal(err)
		}
	}
	batch, err := jt.NewBatch(programs)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{
		`{}`, `{"a":null}`, `{"a":5}`, `{"a":[{"b":1},{"b":2}]}`, `{"a":{"b":[1,2]}}`,
		`{"a":{"b":{"d":2}}}`, `{"a":{"b":1,"b":{"d":2}}}`, `{"a":{"b":null,"c":3}}`,
	} {
		document, err := jt.PrepareJSON(ctx, []byte(input), jt.PrepareOptions{})
		if err != nil {
			t.Fatal(err)
		}
		got, err := batch.EvalPrepared(ctx, document, nil, jt.EvalOptions{})
		if err != nil {
			t.Fatalf("%s: %v", input, err)
		}
		for i, program := range programs {
			want, err := jt.EvalJSON(ctx, program, []byte(input), jt.EvalOptions{})
			if err != nil {
				t.Fatal(err)
			}
			encoded := ""
			if got[i].Defined {
				b, err := jsonMarshal(got[i].Value)
				if err != nil {
					t.Fatal(err)
				}
				encoded = string(b)
			}
			if got[i].Defined != want.Defined || encoded != string(want.Value) {
				t.Errorf("%s on %s: batch %s (%v), ordinary %s (%v)", expressions[i], input, encoded, got[i].Defined, want.Value, want.Defined)
			}
		}
	}
}

func jsonMarshal(v any) ([]byte, error) {
	var b bytes.Buffer
	encoder := json.NewEncoder(&b)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}
