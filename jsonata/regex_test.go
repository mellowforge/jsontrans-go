package jsonata

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/internal/budget"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRegexMatcherCaptures(t *testing.T) {
	v, err := (RegexMatcher{Pattern: `(a)(b)`}).Call(&CallContext{}, []Value{"zab"})
	if err != nil {
		t.Fatal(err)
	}
	obj, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("got %#v", v)
	}
	if obj["match"] != "ab" || obj["start"] != float64(1) || obj["end"] != float64(3) || !reflect.DeepEqual(obj["groups"], []any{"a", "b"}) {
		t.Fatalf("got %#v", obj)
	}
}

func TestBuiltinMatchFindsAll(t *testing.T) {
	c := &CallContext{}
	c.Invoke = func(f Callable, a []Value) (Value, error) { return f.Call(c, a) }
	got, err := builtins()["match"].Call(c, []Value{"a1 b2", RegexMatcher{Pattern: `([a-z])(\d)`}})
	want := Sequence{map[string]any{"match": "a1", "index": float64(0), "groups": []any{"a", "1"}}, map[string]any{"match": "b2", "index": float64(3), "groups": []any{"b", "2"}}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestBuiltinReplaceRegexGroups(t *testing.T) {
	c := &CallContext{}
	c.Invoke = func(f Callable, a []Value) (Value, error) { return f.Call(c, a) }
	got, err := builtins()["replace"].Call(c, []Value{"a1 b2", RegexMatcher{Pattern: `([a-z])(\d)`}, "$2$1"})
	if err != nil || got != "1a 2b" {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestBuiltinSplitRegex(t *testing.T) {
	c := &CallContext{}
	c.Invoke = func(f Callable, a []Value) (Value, error) { return f.Call(c, a) }
	got, err := builtins()["split"].Call(c, []Value{"ababbxabbcc", RegexMatcher{Pattern: `b+`}})
	want := []any{"a", "a", "xa", "cc"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, %v", got, err)
	}
}
func TestBuiltinReplaceTwoDigitCaptures(t *testing.T) {
	c := &CallContext{}
	c.Invoke = func(f Callable, a []Value) (Value, error) { return f.Call(c, a) }
	got, err := builtins()["replace"].Call(c, []Value{"abcdefghijklmno", RegexMatcher{Pattern: `(a)(b)(c)(d)(e)(f)(g)(h)(i)(j)(k)(l)(m)`}, "$8$5$12$12$18$123"})
	if err != nil || got != "hella8l3no" {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestRegexMatcherAllowsInitialEmptyMatch(t *testing.T) {
	got, err := (RegexMatcher{Pattern: `a*`}).Call(&CallContext{}, []Value{"bbb"})
	if err != nil {
		t.Fatal(err)
	}
	obj, ok := got.(map[string]any)
	if !ok || obj["match"] != "" {
		t.Fatalf("got %#v", got)
	}
}

func TestRegexTimeoutIsResourceLimit(t *testing.T) {
	b, err := budget.New(context.Background(), budget.Limits{RegexTimeout: time.Millisecond, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	_, err = (RegexMatcher{Pattern: `(a+)+$`}).Call(&CallContext{Budget: b}, []Value{strings.Repeat("a", 5000) + "!"})
	if !errors.Is(err, budget.ErrLimit) {
		t.Fatalf("got %v", err)
	}
}

func TestBuiltinReplaceHonorsOutputLimit(t *testing.T) {
	b, err := budget.New(context.Background(), budget.Limits{MaxOutputBytes: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	c := &CallContext{Budget: b}
	c.Invoke = func(f Callable, a []Value) (Value, error) { return f.Call(c, a) }
	_, err = builtins()["replace"].Call(c, []Value{"aa", "a", "xyz"})
	if !errors.Is(err, budget.ErrLimit) {
		t.Fatalf("got %v", err)
	}
}

func TestRegexContinuationConcurrent(t *testing.T) {
	value, err := (RegexMatcher{Pattern: "a"}).Call(&CallContext{}, []Value{"aa"})
	if err != nil {
		t.Fatal(err)
	}
	next := value.(map[string]any)["next"].(Callable)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := next.Call(&CallContext{}, nil)
			if err != nil || got.(map[string]any)["start"] != float64(1) {
				t.Errorf("got %#v, %v", got, err)
			}
		}()
	}
	wg.Wait()
}

func TestContainsRejectsMalformedMatcher(t *testing.T) {
	c := &CallContext{}
	c.Invoke = func(f Callable, a []Value) (Value, error) { return f.Call(c, a) }
	fn := NativeFunction{N: 1, Fn: func(*CallContext, []Value) (Value, error) { return "not a match object", nil }}
	_, err := builtins()["contains"].Call(c, []Value{"text", fn})
	var native *jsontrans.Error
	if !errors.As(err, &native) || native.Code != "T1010" {
		t.Fatalf("got %v", err)
	}
}

func TestRegexUnicodeContinuation(t *testing.T) {
	c := &CallContext{}
	c.Invoke = func(f Callable, a []Value) (Value, error) { return f.Call(c, a) }
	r := RegexMatcher{Pattern: "a"}
	got, err := builtins()["match"].Call(c, []Value{"😀a😀a", r})
	want := Sequence{map[string]any{"match": "a", "index": float64(2), "groups": []any{}}, map[string]any{"match": "a", "index": float64(5), "groups": []any{}}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("match got %#v, %v", got, err)
	}
	got, err = builtins()["split"].Call(c, []Value{"éaéa", r})
	if err != nil || !reflect.DeepEqual(got, []any{"é", "é", ""}) {
		t.Fatalf("split got %#v, %v", got, err)
	}
	got, err = builtins()["replace"].Call(c, []Value{"😀a😀a", r, "!"})
	if err != nil || got != "😀!😀!" {
		t.Fatalf("replace got %#v, %v", got, err)
	}
}

func TestRegexMatchesUTF16CodeUnits(t *testing.T) {
	c := &CallContext{}
	c.Invoke = func(f Callable, a []Value) (Value, error) { return f.Call(c, a) }
	for _, tt := range []struct {
		pattern string
		want    bool
	}{{"^.$", false}, {"^..$", true}, {"^😀$", true}} {
		got, err := builtins()["contains"].Call(c, []Value{"😀", RegexMatcher{Pattern: tt.pattern}})
		if err != nil || got != tt.want {
			t.Errorf("%q got %#v, %v", tt.pattern, got, err)
		}
	}
}

func TestRegexUTF16SplitReplace(t *testing.T) {
	c := &CallContext{}
	c.Invoke = func(f Callable, a []Value) (Value, error) { return f.Call(c, a) }
	r := RegexMatcher{Pattern: "."}
	got, err := builtins()["split"].Call(c, []Value{"😀", r})
	if err != nil || !reflect.DeepEqual(got, []any{"", "", ""}) {
		t.Fatalf("split %#v %v", got, err)
	}
	got, err = builtins()["replace"].Call(c, []Value{"😀", r, "x"})
	if err != nil || got != "xx" {
		t.Fatalf("replace %#v %v", got, err)
	}
}
func TestRegexLoneSurrogateCapturePreservesWTF8(t *testing.T) {
	got, err := (RegexMatcher{Pattern: "(.)"}).Call(&CallContext{}, []Value{"😀"})
	if err != nil {
		t.Fatal(err)
	}
	obj := got.(map[string]any)
	if obj["match"] != string([]byte{0xed, 0xa0, 0xbd}) || obj["groups"].([]any)[0] != string([]byte{0xed, 0xa0, 0xbd}) {
		t.Fatalf("got %#v", obj)
	}
}

func TestRegexUTF16SplitPreservesUnmatchedSurrogate(t *testing.T) {
	c := &CallContext{}
	c.Invoke = func(f Callable, a []Value) (Value, error) { return f.Call(c, a) }
	got, err := builtins()["split"].Call(c, []Value{"😀", RegexMatcher{Pattern: "^."}})
	want := []any{"", string([]byte{0xed, 0xb8, 0x80})}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestStringSearchLoneSurrogate(t *testing.T) {
	high := string([]byte{0xed, 0xa0, 0xbd})
	low := string([]byte{0xed, 0xb8, 0x80})
	f := builtins()
	got, err := f["contains"].Call(&CallContext{}, []Value{"😀", high})
	if err != nil || got != true {
		t.Fatalf("contains %#v %v", got, err)
	}
	got, err = f["substringAfter"].Call(&CallContext{}, []Value{"😀", high})
	if err != nil || got != low {
		t.Fatalf("substringAfter %#v %v", got, err)
	}
}

func TestStringOperationsPreserveLoneSurrogate(t *testing.T) {
	s := "a" + string([]byte{0xed, 0xa0, 0xbd}) + "b"
	f := builtins()
	got, err := f["length"].Call(&CallContext{}, []Value{s})
	if err != nil || got != float64(3) {
		t.Fatalf("length %#v %v", got, err)
	}
	got, err = f["substring"].Call(&CallContext{}, []Value{s, float64(1), float64(1)})
	if err != nil || got != s[1:4] {
		t.Fatalf("substring %#v %v", got, err)
	}
}

func TestBase64EncodesLoneSurrogateAsReferenceByte(t *testing.T) {
	high := string([]byte{0xed, 0xa0, 0xbd})
	got, err := builtins()["base64encode"].Call(&CallContext{}, []Value{high})
	if err != nil || got != "PQ==" {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestUnicodeCasePreservesLoneSurrogate(t *testing.T) {
	high := string([]byte{0xed, 0xa0, 0xbd})
	got, err := builtins()["uppercase"].Call(&CallContext{}, []Value{"a" + high + "b"})
	if err != nil || got != "A"+high+"B" {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestPadPreservesLoneSurrogate(t *testing.T) {
	high := string([]byte{0xed, 0xa0, 0xbd})
	got, err := builtins()["pad"].Call(&CallContext{}, []Value{"x", float64(-3), high})
	if err != nil || got != high+high+"x" {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestRegexLoneSurrogatesRoundTripJSON(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatal(err)
	}
	p, err := e.CompileRaw(context.Background(), `$match("😀", /./).match`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := p.Eval(context.Background(), nil, jsontrans.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(result.Value)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `["\ud83d","\ude00"]` {
		t.Fatalf("got %s", b)
	}
}

func TestRegexRejectsInvalidNonWTF8(t *testing.T) {
	_, err := (RegexMatcher{Pattern: "."}).Call(&CallContext{}, []Value{string([]byte{0xff})})
	if !errors.Is(err, jsontrans.ErrInvalidInput) {
		t.Fatalf("got %v", err)
	}
}

func TestStringSplitUsesUTF16Units(t *testing.T) {
	got, err := builtins()["split"].Call(&CallContext{}, []Value{"😀", ""})
	want := []any{string([]byte{0xed, 0xa0, 0xbd}), string([]byte{0xed, 0xb8, 0x80})}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, %v", got, err)
	}
}

// A regex match may not outlive the evaluation deadline, even when the
// configured regex timeout is longer.
func TestRegexTimeoutCappedByEvaluationDeadline(t *testing.T) {
	b, err := budget.New(context.Background(), budget.Limits{Timeout: 50 * time.Millisecond, RegexTimeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	started := time.Now()
	_, err = (RegexMatcher{Pattern: `(a+)+$`}).Call(&CallContext{Budget: b}, []Value{strings.Repeat("a", 5000) + "!"})
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("match ran %v past a 50ms deadline", elapsed)
	}
	if !errors.Is(err, budget.ErrLimit) && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
}

func TestRegexLiteralContinuationState(t *testing.T) {
	for _, tt := range []struct {
		expression string
		want       any
	}{
		{`($m := /a/("😀a a"); [$m.start, $m.end, $m.next().start, $m.next().start])`, []any{float64(2), float64(3), float64(4)}},
		{`($r := /a/; $m := $r("aaa"); $r("aa", 1); $m.next().start)`, float64(2)},
		{`($r := /a/; $m := $r("aaa"); $r("bbb"); $m.next().start)`, float64(0)},
	} {
		t.Run(tt.expression, func(t *testing.T) {
			program := compileRegexTest(t, tt.expression)
			// Reusing the compiled program must create fresh matcher state.
			for range 4 {
				t.Run("evaluation", func(t *testing.T) {
					t.Parallel()
					result, err := program.Eval(context.Background(), nil, jsontrans.EvalOptions{})
					if err != nil || !result.Defined || !reflect.DeepEqual(result.Value, tt.want) {
						t.Fatalf("got %#v, %v, want %#v", result, err, tt.want)
					}
				})
			}
		})
	}
}

func TestRegexLiteralArgumentCoercion(t *testing.T) {
	for _, tt := range []struct {
		expression string
		input      any
		want       any
	}{
		{`($r := /a/; $m := $r("aba", "2"); $n := /1/(123); [$m.start, $n.match])`, nil, []any{float64(2), "1"}},
		{`($m := /undefined/(); $m.match)`, nil, "undefined"},
		{`($m := /null/(null); $m.match)`, nil, "null"},
		{`($m := /true/(true); $m.match)`, nil, "true"},
		{`($m := /1,,2,3/([1, null, [2, 3]]); $m.match)`, nil, "1,,2,3"},
		{`($m := /\[object Object\]/($); $m.match)`, map[string]any{}, "[object Object]"},
		{`($m := /async function/($sum); $m.match)`, nil, "async function"},
		{`($r := /fromIndex/; $f := /a/; $m := $r([$f]); $m.match)`, nil, "fromIndex"},
		{`($r := /re\.lastIndex/; $f := /a/("aa").next; $m := $r([$f]); $m.match)`, nil, "re.lastIndex"},
		{`($r := /async function/; $f := $r(?); $m := $f(function($x){$x}); $m.match)`, nil, "async function"},
		{`($r := /async function/; $f := function($x){$x} ~> $r; $m := $f($sum); $m.match)`, nil, "async function"},
		{`($f := function($x){[$x]}; $r := /async function/; $m := $r($f($sum)); $m.match)`, nil, "async function"},
		{`($r := /async function/; $f := function($x){$x ~> $r()}; $m := $f($sum); $m.match)`, nil, "async function"},
		{`($m := /a/("aba", true); $m.start)`, nil, float64(2)},
		{`($f := /./; $map(["ab","ab","ab"], $f).match)`, nil, []any{"a", "b"}},
		{`($m := /a/("aba", [2]); $m.start)`, nil, float64(2)},
		{`($m := /a/("aba", "0x2"); $m.start)`, nil, float64(2)},
		{`($m := /a/("aba", -1); $m.start)`, nil, float64(0)},
		{`$exists(/a/("aba", "Infinity"))`, nil, false},
		{`($m := /a/("aba", "1_2"); $m.start)`, nil, float64(0)},
		{`($m := /1/(123); [$m.next().start, $m.next().start])`, nil, []any{float64(0)}},
	} {
		t.Run(tt.expression, func(t *testing.T) {
			result, err := compileRegexTest(t, tt.expression).Eval(context.Background(), tt.input, jsontrans.EvalOptions{})
			if err != nil || !result.Defined || !reflect.DeepEqual(result.Value, tt.want) {
				t.Fatalf("got %#v, %v, want %#v", result, err, tt.want)
			}
		})
	}
}

func compileRegexTest(t *testing.T, expression string) *Program {
	t.Helper()
	engine, err := New()
	if err != nil {
		t.Fatal(err)
	}
	program, err := engine.CompileRaw(context.Background(), expression)
	if err != nil {
		t.Fatal(err)
	}
	return program
}

func TestRegexLiteralRejectsUnsupportedGroupSyntax(t *testing.T) {
	engine, err := New()
	if err != nil {
		t.Fatal(err)
	}
	for _, expression := range []string{
		`/(?i:a)/`,
		`/(?<x>a)|(?<x>b)/`,
		`/(?:(?<x>a)|(?<x>b))\k<x>/`,
		`/(?<\u0078>a)|(?<x>b)/`,
		`/(?<\u{78}>a)|(?<x>b)/`,
		`/a/ii`,
	} {
		_, err := engine.Compile(context.Background(), expression)
		var native *jsontrans.Error
		if !errors.As(err, &native) || native.Phase != jsontrans.PhaseCompile || native.Code != "" {
			t.Errorf("%s: got %v, want uncoded compile error", expression, err)
		}
	}
	for _, expression := range []string{
		`/\(\?i:a\)/`,
		`/[(?i:a)]/`,
		`/(?<\u0078>a)\k<x>/`,
		`/(?<=a)b/`,
		`/(?:a)(?=b)/`,
	} {
		if _, err := engine.Compile(context.Background(), expression); err != nil {
			t.Errorf("%s: %v", expression, err)
		}
	}
}

func TestRegexArgumentDetectionDoesNotWalkSharedComposition(t *testing.T) {
	expression := `($f := function($x){$x};` +
		strings.Repeat(`$f := $f ~> $f;`, 40) +
		`$f($sum))`
	program := compileRegexTest(t, expression)
	_, err := program.Eval(context.Background(), nil, jsontrans.EvalOptions{
		Limits: &jsontrans.Limits{MaxSteps: 500},
	})
	if !errors.Is(err, jsontrans.ErrLimit) {
		t.Fatalf("got %v, want step limit from evaluating the composition", err)
	}
}

func TestRegexCoercionTracksObjectPrototype(t *testing.T) {
	for _, subject := range []string{
		`{"x":1}`,
		`[1]{"x":$}`,
		`$merge([{"x":1}])`,
		`$spread({"x":1})`,
		`$sift({"x":1}, function(){true})`,
	} {
		t.Run(subject, func(t *testing.T) {
			program := compileRegexTest(t, `($m := /object/(`+subject+`); $m.match)`)
			_, err := program.Eval(context.Background(), nil, jsontrans.EvalOptions{})
			var native *jsontrans.Error
			if !errors.As(err, &native) || native.Phase != jsontrans.PhaseEvaluate || native.Code != "" ||
				!strings.Contains(err.Error(), "cannot convert object to primitive value") {
				t.Fatalf("got %v, want uncoded object coercion error", err)
			}
		})
	}
	program := compileRegexTest(t, `($m := /object/($clone({"x":1})); $m.match)`)
	result, err := program.Eval(context.Background(), nil, jsontrans.EvalOptions{})
	if err != nil || !result.Defined || result.Value != "object" {
		t.Fatalf("got %#v, %v, want cloned object to stringify", result, err)
	}
}
