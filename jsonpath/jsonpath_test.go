package jsonpath

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mellowforge/jsontrans-go"
)

func TestSelectionAndIsolation(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), "$['a','a','missing','null']")
	if err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"a": []any{1}, "null": nil}
	r, err := p.Eval(context.Background(), input, jsontrans.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := []any{[]any{1}, []any{1}, nil}
	if !reflect.DeepEqual(r.Value, want) {
		t.Fatalf("got %#v", r.Value)
	}
	r.Value.([]any)[0].([]any)[0] = 42
	if input["a"].([]any)[0] != 1 {
		t.Fatal("result aliases input")
	}
}
func TestLimitsAndCancellation(t *testing.T) {
	e, _ := New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.Compile(ctx, "$"); !errors.Is(err, context.Canceled) {
		t.Fatalf("compile: %v", err)
	}
	p, err := e.Compile(context.Background(), "$..*")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Eval(ctx, nil, jsontrans.EvalOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("eval: %v", err)
	}
	if _, err = p.Eval(context.Background(), []any{1, 2, 3}, jsontrans.EvalOptions{Limits: &jsontrans.Limits{MaxSteps: 2}}); !errors.Is(err, jsontrans.ErrLimit) {
		t.Fatalf("steps: %v", err)
	}
	if _, err = p.Eval(context.Background(), nil, jsontrans.EvalOptions{Bindings: map[string]any{"x": 1}}); !errors.Is(err, jsontrans.ErrUnsupported) {
		t.Fatalf("bindings: %v", err)
	}
	cycle := map[string]any{}
	cycle["self"] = cycle
	if _, err = p.Eval(context.Background(), cycle, jsontrans.EvalOptions{}); !errors.Is(err, jsontrans.ErrInvalidInput) {
		t.Fatalf("cycle: %v", err)
	}
}
func TestHostFunctionAndConcurrentEvaluation(t *testing.T) {
	f := Function{Parameters: []Type{ValueType}, Result: LogicalType, Call: func(_ context.Context, a []any) (any, error) { return a[0] == "yes", nil }}
	e, err := New(WithFunction("accept", f))
	if err != nil {
		t.Fatal(err)
	}
	f.Parameters[0] = NodesType
	p, err := e.Compile(context.Background(), "$[?accept(@)]")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := p.Eval(context.Background(), []any{"yes", "no"}, jsontrans.EvalOptions{})
			if err != nil || !reflect.DeepEqual(r.Value, []any{"yes"}) {
				t.Errorf("got %#v, %v", r, err)
			}
		}()
	}
	wg.Wait()
}
func TestParseDepthLimit(t *testing.T) {
	e, _ := New(WithLimits(jsontrans.Limits{MaxDepth: 16}))
	s := "$[?"
	for i := 0; i < 1000; i++ {
		s += "("
	}
	s += "@"
	for i := 0; i < 1000; i++ {
		s += ")"
	}
	s += "]"
	if _, err := e.Compile(context.Background(), s); !errors.Is(err, jsontrans.ErrLimit) {
		t.Fatalf("got %v", err)
	}
}
func FuzzCompileAndEval(f *testing.F) {
	for _, s := range []string{"$", "$..*", "$[?@.x == null]", "$[::-1]", "$[?match(@, 'a.*')]"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		e, _ := New(WithLimits(jsontrans.Limits{MaxExpressionBytes: 4096, MaxSteps: 10000}))
		p, err := e.Compile(context.Background(), s)
		if err == nil {
			_, _ = p.Eval(context.Background(), map[string]any{"x": []any{nil, 1, "a"}}, jsontrans.EvalOptions{})
		}
	})
}

func TestInvalidHostResultDoesNotPanic(t *testing.T) {
	e, err := New(WithFunction("bad", Function{Result: NodesType, Call: func(context.Context, []any) (any, error) { return "not nodes", nil }}))
	if err != nil {
		t.Fatal(err)
	}
	p, err := e.Compile(context.Background(), "$[?count(bad()) == 0]")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Eval(context.Background(), []any{1}, jsontrans.EvalOptions{}); err == nil {
		t.Fatal("expected invalid host result error")
	}
}

func TestHugeNumberComparison(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), "$[?@ > 1e1000000000]")
	if err != nil {
		t.Fatal(err)
	}
	r, err := p.Eval(context.Background(), []any{json.Number("1e1000000001")}, jsontrans.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Value.([]any)) != 1 {
		t.Fatalf("got %#v", r.Value)
	}
}

func TestIRegexpRejectsNonInteroperableSyntax(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), "$[?search(@.s, @.p)]")
	if err != nil {
		t.Fatal(err)
	}
	input := []any{map[string]any{"s": "aaa", "p": `a+?`}, map[string]any{"s": "a", "p": `\x61`}, map[string]any{"s": "a", "p": `\p{Latin}`}, map[string]any{"s": "a", "p": `(?:a)`}}
	r, err := p.Eval(context.Background(), input, jsontrans.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Value.([]any)) != 0 {
		t.Fatalf("accepted invalid patterns: %#v", r.Value)
	}
}

func TestLogicalFunctionArgumentConvertsNodeList(t *testing.T) {
	e, _ := New(WithFunction("logical", Function{Parameters: []Type{LogicalType}, Result: LogicalType, Call: func(_ context.Context, a []any) (any, error) { v, ok := a[0].(bool); return ok && v, nil }}))
	p, err := e.Compile(context.Background(), "$[?logical(@.x)]")
	if err != nil {
		t.Fatal(err)
	}
	r, err := p.Eval(context.Background(), []any{map[string]any{"x": nil}, map[string]any{}}, jsontrans.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Value.([]any)) != 1 {
		t.Fatalf("got %#v", r.Value)
	}
}

func TestComparisonChargesForLongNumbers(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), "$[?@ == 1]")
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Eval(context.Background(), []any{json.Number("1e" + strings.Repeat("1", 5000))}, jsontrans.EvalOptions{Limits: &jsontrans.Limits{MaxSteps: 10}})
	if !errors.Is(err, jsontrans.ErrLimit) {
		t.Fatalf("got %v", err)
	}
}

func TestRejectInvalidFunctionRegistration(t *testing.T) {
	call := func(context.Context, []any) (any, error) { return nil, nil }
	for _, f := range []struct {
		name string
		fn   Function
	}{{"bad-name", Function{Result: ValueType, Call: call}}, {"count", Function{Result: ValueType, Call: call}}, {"f", Function{Result: Type(42), Call: call}}, {"f", Function{Parameters: []Type{Type(42)}, Call: call}}} {
		if _, err := New(WithFunction(f.name, f.fn)); err == nil {
			t.Errorf("accepted %#v", f)
		}
	}
}

func TestConfiguredLimitsExposed(t *testing.T) {
	e, _ := New(WithLimits(jsontrans.Limits{MaxSteps: 123}))
	if e.Limits().MaxSteps != 123 {
		t.Fatal(e.Limits())
	}
	p, err := e.Compile(context.Background(), "$")
	if err != nil {
		t.Fatal(err)
	}
	provider, ok := p.(interface{ Limits() jsontrans.Limits })
	if !ok || provider.Limits().MaxSteps != 123 {
		t.Fatalf("program limits: %#v", p)
	}
}

func TestIRegexpUnassignedCategory(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), "$[?match(@.s, @.p)]")
	if err != nil {
		t.Fatal(err)
	}
	input := []any{map[string]any{"s": "\u0378", "p": `\p{Cn}`}, map[string]any{"s": "A", "p": `\P{Cn}`}, map[string]any{"s": "A", "p": `[\p{Cn}A]`}}
	r, err := p.Eval(context.Background(), input, jsontrans.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Value.([]any)) != 3 {
		t.Fatalf("got %#v", r.Value)
	}
}

func TestIRegexpLargeRepeat(t *testing.T) {
	// Semantics only; see TestReviewIRegexp for why the limits are generous.
	e, _ := New(WithLimits(jsontrans.Limits{Timeout: 30 * time.Second, RegexTimeout: 10 * time.Second}))
	p, err := e.Compile(context.Background(), "$[?match(@.s, @.p)]")
	if err != nil {
		t.Fatal(err)
	}
	input := []any{map[string]any{"s": strings.Repeat("a", 1001), "p": `a{1001}`}, map[string]any{"s": strings.Repeat("ab", 1001), "p": `(ab){1001,1002}`}, map[string]any{"s": strings.Repeat("é", 1001), "p": `é{1001}`}}
	r, err := p.Eval(context.Background(), input, jsontrans.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Value.([]any)) != 3 {
		t.Fatalf("got %d matches", len(r.Value.([]any)))
	}
}

func FuzzIRegexp(f *testing.F) {
	for _, s := range []string{"a{1001}", "(ab){1001,1003}", "\\p{Cn}", "[a-z]", "a{4,2}", "a+?", "\\\\"} {
		f.Add(s)
	}
	e, _ := New()
	p, err := e.Compile(context.Background(), "$[?match(@.s,@.p)]")
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 4096 {
			return
		}
		_, _ = p.Eval(context.Background(), []any{map[string]any{"s": "aaa", "p": s}}, jsontrans.EvalOptions{Limits: &jsontrans.Limits{MaxSteps: 10000}})
	})
}

func TestInvalidUTF8ExpressionRejected(t *testing.T) {
	e, _ := New()
	if _, err := e.Compile(context.Background(), "$['\xff']"); err == nil {
		t.Fatal("accepted invalid UTF-8")
	}
}

func TestHostNodeResultValidated(t *testing.T) {
	e, _ := New(WithFunction("invalid_nodes", Function{Result: NodesType, Call: func(context.Context, []any) (any, error) { return Nodes{make(chan int)}, nil }}))
	p, err := e.Compile(context.Background(), "$[?count(invalid_nodes()) == 1]")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Eval(context.Background(), []any{1}, jsontrans.EvalOptions{}); !errors.Is(err, jsontrans.ErrInvalidInput) {
		t.Fatalf("got %v", err)
	}
}

func TestHostEmptyNodeResult(t *testing.T) {
	e, _ := New(WithFunction("empty", Function{Result: NodesType, Call: func(context.Context, []any) (any, error) { return Nodes(nil), nil }}))
	p, err := e.Compile(context.Background(), "$[?count(empty()) == 0]")
	if err != nil {
		t.Fatal(err)
	}
	r, err := p.Eval(context.Background(), []any{1}, jsontrans.EvalOptions{})
	if err != nil || len(r.Value.([]any)) != 1 {
		t.Fatalf("got %#v %v", r, err)
	}
}

func TestRegexTimeoutIsEnforced(t *testing.T) {
	e, _ := New(WithLimits(jsontrans.Limits{RegexTimeout: time.Nanosecond}))
	p, err := e.Compile(context.Background(), "$[?match(@, 'a')]")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Eval(context.Background(), []any{"a"}, jsontrans.EvalOptions{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
}

func TestIRegexpLargeRepeatHitsResourceLimit(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), "$[?match(@.s,@.p)]")
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Eval(context.Background(), []any{map[string]any{"s": "a", "p": "a{999999999999999999999999}"}}, jsontrans.EvalOptions{})
	if !errors.Is(err, jsontrans.ErrLimit) {
		t.Fatalf("got %v", err)
	}
}

func TestCompiledProgramFreezesFunctions(t *testing.T) {
	yes := Function{Result: LogicalType, Call: func(context.Context, []any) (any, error) { return true, nil }}
	no := Function{Result: LogicalType, Call: func(context.Context, []any) (any, error) { return false, nil }}
	e, _ := New(WithFunction("choice", yes))
	p, err := e.Compile(context.Background(), "$[?choice()]")
	if err != nil {
		t.Fatal(err)
	}
	e.functions["choice"] = no
	r, err := p.Eval(context.Background(), []any{1}, jsontrans.EvalOptions{})
	if err != nil || len(r.Value.([]any)) != 1 {
		t.Fatalf("got %#v %v", r, err)
	}
}

func TestReviewLogicalBudgets(t *testing.T) {
	e, _ := New(WithLimits(jsontrans.Limits{MaxDepth: 8, MaxSteps: 10}))
	_, err := e.Compile(context.Background(), "$[?"+strings.Repeat("@ || ", 5000)+"@]")
	if !errors.Is(err, jsontrans.ErrLimit) {
		t.Fatalf("logical chain: %v", err)
	}
	e, _ = New(WithLimits(jsontrans.Limits{MaxDepth: 8}))
	p, err := e.Compile(context.Background(), "$[?"+strings.Repeat("@ || ", 100)+"@]")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Eval(context.Background(), []any{1}, jsontrans.EvalOptions{Limits: &jsontrans.Limits{MaxSteps: 10000}}); !errors.Is(err, jsontrans.ErrLimit) {
		t.Fatalf("logical depth: %v", err)
	}
	e, _ = New(WithLimits(jsontrans.Limits{MaxSteps: 2}))
	_, err = e.Compile(context.Background(), "$["+strings.Repeat("0,", 10000)+"0]")
	if !errors.Is(err, jsontrans.ErrLimit) {
		t.Fatalf("selectors: %v", err)
	}
}
func TestReviewInvalidFilterSyntax(t *testing.T) {
	e, _ := New()
	for _, q := range []string{"$[?(1)==1]", "$[?(@)==1]", "$[?length(('abc'))==3]", "$[?!!@]"} {
		if _, err := e.Compile(context.Background(), q); err == nil {
			t.Errorf("accepted %s", q)
		}
	}
}
func TestReviewIRegexp(t *testing.T) {
	// This checks matching semantics; generous limits keep large inputs from
	// tripping the 50 ms default regex timeout on slow race-detector runners.
	e, _ := New(WithLimits(jsontrans.Limits{Timeout: 30 * time.Second, RegexTimeout: 10 * time.Second}))
	p, err := e.Compile(context.Background(), "$[?match(@.s,@.p)]")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		s, pattern string
		want       bool
	}{{strings.Repeat("a", 10000), "(a{100}){100}", true}, {strings.Repeat("a", 2002), "(a{1001}){2}", true}, {"a", "a{01}", true}, {"a", "[a-b-c]", false}, {"^abc", "^ab.*", true}, {"abc", "^ab.*", false}, {"abc$", ".*bc$", true}, {"abc", ".*bc$", false}} {
		r, err := p.Eval(context.Background(), []any{map[string]any{"s": tc.s, "p": tc.pattern}}, jsontrans.EvalOptions{})
		if err != nil {
			t.Errorf("%q: %v", tc.pattern, err)
			continue
		}
		if got := len(r.Value.([]any)) > 0; got != tc.want {
			t.Errorf("%q on %q: got %v want %v", tc.pattern, tc.s[:min(len(tc.s), 20)], got, tc.want)
		}
	}
}

func TestIRegexpEscapedHyphenBetweenRanges(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), "$[?match(@.s,@.p)]")
	if err != nil {
		t.Fatal(err)
	}
	r, err := p.Eval(context.Background(), []any{map[string]any{"s": "c", "p": `[a-b\-c-d]`}}, jsontrans.EvalOptions{})
	if err != nil || len(r.Value.([]any)) != 1 {
		t.Fatalf("got %#v %v", r, err)
	}
}

func TestIRegexpCharacterClassRangeGrammar(t *testing.T) {
	e, _ := New()
	p, err := e.Compile(context.Background(), "$[?match(@.s,@.p)]")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		pattern, input string
		want           bool
	}{
		{`[a-é-z]`, "a", false}, {`[\p{L}-a]`, "a", false}, {`[a-\p{L}]`, "a", false}, {`[--a]`, "a", false}, {`[a--]`, "a", false},
		{`[a-éx-z]`, "é", true}, {`[a-éx-z]`, "z", true}, {`[\p{L}0-9]`, "é", true},
		{`[\p{L}a-z]`, "z", true}, {`[^\p{L}]`, "3", true}, {`[a-\-]`, "a", false}, {`[a-b\-c-d]`, "-", true}, {`[-a]`, "-", true}, {`[a-]`, "-", true}, {`[é-ê]`, "ê", true},
	}
	for _, tc := range cases {
		t.Run(tc.pattern+"/"+tc.input, func(t *testing.T) {
			r, err := p.Eval(context.Background(), []any{map[string]any{"s": tc.input, "p": tc.pattern}}, jsontrans.EvalOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if got := len(r.Value.([]any)) > 0; got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

// Later options refine earlier ones instead of resetting omitted fields.
func TestWithLimitsAccumulates(t *testing.T) {
	e, err := New(WithLimits(jsontrans.Limits{MaxDepth: 8}), WithLimits(jsontrans.Limits{MaxSteps: 50}))
	if err != nil {
		t.Fatal(err)
	}
	if got := e.Limits(); got.MaxDepth != 8 || got.MaxSteps != 50 {
		t.Fatalf("got %+v", got)
	}
}
