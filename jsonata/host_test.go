package jsonata_test

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/jsonata"
)

func hostEngine(t *testing.T, name string, fn func() (jsonata.Value, error)) *jsonata.Engine {
	t.Helper()
	engine, err := jsonata.New(jsonata.WithFunction(name, jsonata.NativeFunction{Fn: func(*jsonata.CallContext, []jsonata.Value) (jsonata.Value, error) {
		return fn()
	}}))
	if err != nil {
		t.Fatal(err)
	}
	return engine
}

func evalHost(t *testing.T, engine *jsonata.Engine, expression string) (jsontrans.Result, error) {
	t.Helper()
	program, err := engine.Compile(context.Background(), expression)
	if err != nil {
		t.Fatal(err)
	}
	return program.Eval(context.Background(), map[string]any{}, jsontrans.EvalOptions{})
}

// Host results enter the language as JSONata values: Go numbers are numbers.
func TestHostResultNumbersAreJSONataNumbers(t *testing.T) {
	engine := hostEngine(t, "int", func() (jsonata.Value, error) { return 5, nil })
	for expression, want := range map[string]any{
		`$int() = 5`:     true,
		`$type($int())`:  "number",
		`$int() + 1`:     float64(6),
		`[$int(), 1][0]`: float64(5),
	} {
		got, err := evalHost(t, engine, expression)
		if err != nil || !got.Defined || got.Value != want {
			t.Errorf("%s: got %#v, %v; want %#v", expression, got, err, want)
		}
	}
}

// Unsupported host results are errors, never silently missing or unequal.
func TestHostResultRejectsUnsupportedValues(t *testing.T) {
	for name, value := range map[string]jsonata.Value{
		"channel":  make(chan int),
		"struct":   struct{ X int }{1},
		"nan":      math.NaN(),
		"infinity": math.Inf(1),
		"nested":   map[string]any{"x": []any{make(chan int)}},
	} {
		t.Run(name, func(t *testing.T) {
			engine := hostEngine(t, "bad", func() (jsonata.Value, error) { return value, nil })
			_, err := evalHost(t, engine, `$type($bad())`)
			if !errors.Is(err, jsontrans.ErrInvalidInput) {
				t.Fatalf("got %v, want ErrInvalidInput", err)
			}
		})
	}
}

// Host results are detached: later changes by the host do not reach a
// result that was already produced.
func TestHostResultIsDetached(t *testing.T) {
	shared := map[string]any{"x": float64(1)}
	engine := hostEngine(t, "shared", func() (jsonata.Value, error) { return shared, nil })
	program, err := engine.CompileRaw(context.Background(), `$shared()`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := program.EvalRaw(context.Background(), nil, jsontrans.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	shared["x"] = float64(2)
	if got.(map[string]any)["x"] != float64(1) {
		t.Fatalf("result follows host mutation: %#v", got)
	}
}

// Plain host errors name the failing function, as the reference does.
func TestHostErrorNamesFunction(t *testing.T) {
	failure := errors.New("boom")
	engine := hostEngine(t, "lookup", func() (jsonata.Value, error) { return nil, failure })
	_, err := evalHost(t, engine, `1 + $lookup()`)
	var native *jsontrans.Error
	if !errors.As(err, &native) || native.Token != "lookup" || native.Position != 12 || !errors.Is(err, failure) {
		t.Fatalf("got %v (%#v)", err, native)
	}
}
