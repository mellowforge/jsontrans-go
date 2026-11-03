package jsonpath

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mellowforge/jsontrans-go"
)

// Strings and object keys containing isolated surrogates arrive as lossless
// UTF16String/UTF16Object values. They remain ordinary JSON strings and objects
// to every selector and function, on ordinary, prepared and batch paths.
// Object members are visited in the engine's lexical key order; length counts
// each isolated surrogate as one character.
func TestIsolatedSurrogateValuesAreOrdinaryJSON(t *testing.T) {
	const object = `{"\ud800":1,"a":{"b":2}}`
	tests := []struct {
		query, input, want string
	}{
		{`$.a.b`, object, `[2]`},
		{`$.a`, object, `[{"b":2}]`},
		{`$.*`, object, `[{"b":2},1]`},
		{`$..b`, object, `[2]`},
		{`$[?@.b == 2]`, `[{"\ud800":0,"b":2}]`, `[{"b":2,"\ud800":0}]`},
		{`$[?length(@) == 1]`, `[{"\ud800":1},{}]`, `[{"\ud800":1}]`},
		{`$[?length(@) == 1]`, `["\ud800","ab"]`, `["\ud800"]`},
		{`$[?@ == $[0]]`, `["\ud800","\ud800","x"]`, `["\ud800","\ud800"]`},
		{`$[?@ == $[0]]`, `[{"\ud800":1},{"\ud800":1},{"a":1}]`, `[{"\ud800":1},{"\ud800":1}]`},
		{`$[?@ > 'a']`, `["\ud800","0"]`, `["\ud800"]`},
		{`$[?match(@, '.')]`, `["\ud800","ab"]`, `["\ud800"]`},
		{`$[?search(@, 'x')]`, `["x\ud800","y"]`, `["x\ud800"]`},
	}
	ctx := context.Background()
	engine, err := New()
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range tests {
		t.Run(tt.query+" "+tt.input, func(t *testing.T) {
			program, err := engine.Compile(ctx, tt.query)
			if err != nil {
				t.Fatal(err)
			}
			ordinary, err := jsontrans.EvalJSON(ctx, program, []byte(tt.input), jsontrans.EvalOptions{})
			if err != nil || string(ordinary.Value) != tt.want {
				t.Errorf("ordinary: got %s, %v; want %s", ordinary.Value, err, tt.want)
			}
			document, err := jsontrans.PrepareJSON(ctx, []byte(tt.input), jsontrans.PrepareOptions{})
			if err != nil {
				t.Fatal(err)
			}
			prepared, err := jsontrans.EvalPrepared(ctx, program, document, jsontrans.EvalOptions{})
			if got := marshalResult(t, prepared, err); got != tt.want {
				t.Errorf("prepared: got %s; want %s", got, tt.want)
			}
			batch, err := jsontrans.NewBatch([]jsontrans.Program{program})
			if err != nil {
				t.Fatal(err)
			}
			results, err := batch.EvalPrepared(ctx, document, nil, jsontrans.EvalOptions{})
			var batched jsontrans.Result
			if err == nil {
				batched = results[0]
			}
			if got := marshalResult(t, batched, err); got != tt.want {
				t.Errorf("batch: got %s; want %s", got, tt.want)
			}
		})
	}
}

func marshalResult(t *testing.T, r jsontrans.Result, err error) string {
	t.Helper()
	if err != nil {
		return "error: " + err.Error()
	}
	encoded, err := json.Marshal(r.Value)
	if err != nil {
		return "marshal error: " + err.Error()
	}
	return string(encoded)
}
