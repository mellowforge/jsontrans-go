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
