package jsontrans_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
	"testing"

	jt "github.com/mellowforge/jsontrans-go"
)

// An independent package can implement a processor without importing an engine.
type identity struct{}

func (identity) Compile(context.Context, string) (jt.Program, error) { return identityProgram{}, nil }

type identityProgram struct{}

func (identityProgram) Eval(_ context.Context, input any, _ jt.EvalOptions) (jt.Result, error) {
	return jt.Result{Value: input, Defined: true}, nil
}

func TestRegistrySnapshotAndJSONNumberPreservation(t *testing.T) {
	registry := map[string]jt.Processor{"identity": identity{}}
	x, err := jt.New(registry)
	if err != nil {
		t.Fatal(err)
	}
	delete(registry, "identity")
	r, err := x.ProcessJSON(context.Background(), "identity", []byte(`{"n":9007199254740993}`), "", jt.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Defined || string(r.Value) != `{"n":9007199254740993}` {
		t.Fatalf("%+v", r)
	}
	_, err = x.Compile(context.Background(), "missing", "")
	if !errors.Is(err, jt.ErrUnknownProcessor) {
		t.Fatalf("%v", err)
	}
}

func TestResultsDoNotAliasInput(t *testing.T) {
	x, _ := jt.New(map[string]jt.Processor{"identity": identity{}})
	input := map[string]any{"a": []any{json.Number("42")}}
	p, err := x.Compile(context.Background(), "identity", "")
	if err != nil {
		t.Fatal(err)
	}
	r, err := p.Eval(context.Background(), input, jt.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	r.Value.(map[string]any)["a"].([]any)[0] = 9
	if input["a"].([]any)[0] != json.Number("42") {
		t.Fatal("input mutated")
	}
}

func TestInvalidInputAndCancellation(t *testing.T) {
	x, _ := jt.New(map[string]jt.Processor{"identity": identity{}})
	p, _ := x.Compile(context.Background(), "identity", "")
	cycle := map[string]any{}
	cycle["self"] = cycle
	for _, input := range []any{cycle, struct{ A int }{1}, json.Number("oops")} {
		if _, err := p.Eval(context.Background(), input, jt.EvalOptions{}); !errors.Is(err, jt.ErrInvalidInput) {
			t.Fatalf("input %T: %v", input, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.Eval(ctx, nil, jt.EvalOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("%v", err)
	}
	if _, err := x.ProcessJSON(context.Background(), "identity", []byte(`{} {}`), "", jt.EvalOptions{}); !errors.Is(err, jt.ErrInvalidInput) {
		t.Fatalf("trailing JSON: %v", err)
	}
}

func TestConversionLimitsMeasureEncodedJSON(t *testing.T) {
	x, _ := jt.New(map[string]jt.Processor{"identity": identity{}})
	p, _ := x.Compile(context.Background(), "identity", "")
	for _, input := range []any{"\x00\x00", true, map[string]any{"abc": nil}, []any{nil, nil}} {
		_, err := p.Eval(context.Background(), input, jt.EvalOptions{Limits: &jt.Limits{MaxInputBytes: 3}})
		if !errors.Is(err, jt.ErrLimit) {
			t.Fatalf("%T: expected encoded size limit, got %v", input, err)
		}
	}
}

func TestCompiledProcessorRetainsConfiguredLimits(t *testing.T) {
	x, _ := jt.New(map[string]jt.Processor{"configured": configuredIdentity{}})
	p, err := x.Compile(context.Background(), "configured", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Eval(context.Background(), "four", jt.EvalOptions{})
	if !errors.Is(err, jt.ErrLimit) {
		t.Fatalf("processor limit lost: %v", err)
	}
}

type configuredIdentity struct{ identity }

func (configuredIdentity) Limits() jt.Limits { return jt.Limits{MaxInputBytes: 3} }

func TestJSONHelpersRejectNilProgram(t *testing.T) {
	if _, err := jt.EvalJSON(context.Background(), nil, []byte(`null`), jt.EvalOptions{}); err == nil {
		t.Fatal("nil program accepted")
	}
}

func TestResultPresence(t *testing.T) {
	for _, tc := range []struct {
		program jt.Program
		defined bool
		value   string
	}{
		{constantProgram{jt.Result{}}, false, ""},
		{constantProgram{jt.Result{Defined: true}}, true, "null"},
		{constantProgram{jt.Result{Defined: true, Value: []any{}}}, true, "[]"},
	} {
		r, err := jt.EvalJSON(context.Background(), tc.program, []byte(`null`), jt.EvalOptions{})
		if err != nil || r.Defined != tc.defined || string(r.Value) != tc.value {
			t.Fatalf("%+v %v", r, err)
		}
	}
}

type constantProgram struct{ result jt.Result }

func (p constantProgram) Eval(context.Context, any, jt.EvalOptions) (jt.Result, error) {
	return p.result, nil
}

func TestJSONDecodingStopsAtWorkBudget(t *testing.T) {
	// The syntax error after the budget must never be reached.
	_, err := jt.EvalJSON(context.Background(), identityProgram{}, []byte("["+strings.Repeat("0,", 1000)+"}"), jt.EvalOptions{Limits: &jt.Limits{MaxSteps: 4}})
	if !errors.Is(err, jt.ErrLimit) {
		t.Fatalf("decoder did not stop at work budget: %v", err)
	}
}

func TestAcyclicSubsliceIsAccepted(t *testing.T) {
	x, _ := jt.New(map[string]jt.Processor{"identity": identity{}})
	s := make([]any, 2)
	s[1] = s[:1]
	r, err := x.Process(context.Background(), "identity", s, "", jt.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(r.Value)
	if string(data) != `[null,[null]]` {
		t.Fatalf("%s", data)
	}
}

type cancelingProcessor struct{ cancel context.CancelFunc }

func (c cancelingProcessor) Compile(context.Context, string) (jt.Program, error) { return c, nil }
func (c cancelingProcessor) Eval(context.Context, any, jt.EvalOptions) (jt.Result, error) {
	c.cancel()
	return jt.Result{}, nil
}
func TestMissingResultsStillHonorCancellation(t *testing.T) {
	for _, raw := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		p := cancelingProcessor{cancel}
		var err error
		if raw {
			_, err = jt.EvalJSON(ctx, p, []byte(`null`), jt.EvalOptions{})
		} else {
			x, _ := jt.New(map[string]jt.Processor{"cancel": p})
			_, err = x.Process(ctx, "cancel", nil, "", jt.EvalOptions{})
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("raw=%v: cancellation lost: %v", raw, err)
		}
	}
}

type bindingMutator struct{}

func (bindingMutator) Eval(_ context.Context, _ any, o jt.EvalOptions) (jt.Result, error) {
	o.Bindings["x"].(map[string]any)["value"] = 2
	return jt.Result{Defined: true}, nil
}
func TestStandaloneJSONHelperDetachesBindings(t *testing.T) {
	bindings := map[string]any{"x": map[string]any{"value": 1}}
	_, err := jt.EvalJSON(context.Background(), bindingMutator{}, []byte(`null`), jt.EvalOptions{Bindings: bindings})
	if err != nil {
		t.Fatal(err)
	}
	if bindings["x"].(map[string]any)["value"] != 1 {
		t.Fatal("caller bindings mutated")
	}
}

func TestJSONInputErrorIdentifiesProcessor(t *testing.T) {
	x, _ := jt.New(map[string]jt.Processor{"identity": identity{}})
	_, err := x.ProcessJSON(context.Background(), "identity", []byte(`{`), "", jt.EvalOptions{})
	var diagnostic *jt.Error
	if !errors.As(err, &diagnostic) || diagnostic.Processor != "identity" || diagnostic.Phase != "input" || !errors.Is(err, jt.ErrInvalidInput) {
		t.Fatalf("%v", err)
	}
}

func FuzzJSONHelpers(f *testing.F) {
	for _, s := range []string{`null`, `[]`, `{"n":9007199254740993}`, `["\u0000",true]`, `{`, "[0,0,}"} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) > 4096 {
			return
		}
		r, err := jt.EvalJSON(context.Background(), identityProgram{}, input, jt.EvalOptions{Limits: &jt.Limits{MaxDepth: 32, MaxSteps: 10000, MaxCollectionItems: 1024, MaxInputBytes: 4096, MaxOutputBytes: 4096}})
		if err == nil && (!r.Defined || !json.Valid(r.Value)) {
			t.Fatalf("successful JSON result is invalid: %+v", r)
		}
	})
}

func TestUnpairedUTF16JSONRoundTrip(t *testing.T) {
	x, _ := jt.New(map[string]jt.Processor{"identity": identity{}})
	for _, input := range []string{`"\ud800"`, `["\udc00","😀"]`, `{"value":"\ud800x"}`, `{"\ud800":1}`} {
		r, err := x.ProcessJSON(context.Background(), "identity", []byte(input), "", jt.EvalOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if string(r.Value) != input {
			t.Fatalf("got %s want %s", r.Value, input)
		}
	}
}

func TestUTF16ObjectMarshalRejectsCycles(t *testing.T) {
	if os.Getenv("JSONTRANS_CYCLE_CHILD") == "1" {
		debug.SetMaxStack(64 << 10)
		value := jt.UTF16Object{}
		value["cycle"] = value
		if _, err := json.Marshal(value); err == nil {
			os.Exit(2)
		}
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestUTF16ObjectMarshalRejectsCycles$")
	cmd.Env = append(os.Environ(), "JSONTRANS_CYCLE_CHILD=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("MarshalJSON crashed instead of rejecting cycle: %v\n%.300s", err, output)
	}
}

func TestSurrogateBindingKeysAreDetached(t *testing.T) {
	x, _ := jt.New(map[string]jt.Processor{"identity": identity{}})
	options := jt.EvalOptions{Bindings: map[string]any{"\xed\xa0\x80": 1}}
	if _, err := x.Process(context.Background(), "identity", nil, "", options); err != nil {
		t.Fatal(err)
	}
	if _, err := x.ProcessJSON(context.Background(), "identity", []byte(`null`), "", options); err != nil {
		t.Fatal(err)
	}
}

func TestAdjacentSurrogatesCanonicalizeToUnicode(t *testing.T) {
	x, _ := jt.New(map[string]jt.Processor{"identity": identity{}})
	paired := "\xed\xa0\xbd\xed\xb8\x80"
	for _, input := range []any{paired, jt.UTF16String(paired)} {
		result, err := x.Process(context.Background(), "identity", input, "", jt.EvalOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if text, ok := result.Value.(string); !ok || text != "😀" {
			t.Fatalf("noncanonical result: %T %#v", result.Value, result.Value)
		}
	}
	result, err := x.Process(context.Background(), "identity", map[string]any{paired: 1}, "", jt.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Value.(map[string]any)["😀"] != 1 {
		t.Fatalf("noncanonical object key: %#v", result.Value)
	}
}
