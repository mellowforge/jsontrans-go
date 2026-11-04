package javascript

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	jt "github.com/mellowforge/jsontrans-go"
)

// Exercise terminal failures through all trusted phase transitions using a
// bounded 8 MiB guest. Sources are fixed; mutation selects their call boundary
// rather than introducing arbitrary loops or uncapped allocation workloads.
func FuzzOwnedTerminalPhases(f *testing.F) {
	const exhaust = `globalThis.keep=null;try{for(;;)keep={next:keep}}catch(e){return 42}`
	cases := []struct{ source, phase string }{
		{`()=>{` + exhaust + `}`, jt.PhaseEvaluate},
		{`()=>new Proxy({}, {ownKeys(){` + exhaust + `}})`, jt.PhaseOutput},
		{`(()=>{` + exhaust + `})()`, jt.PhaseEvaluate},
		{`()=>{throw {get message(){` + exhaust + `}}}`, jt.PhaseEvaluate},
		{`()=>new Proxy({}, {ownKeys(){throw {get message(){` + exhaust + `}}}})`, jt.PhaseOutput},
		{`()=>{Error.prepareStackTrace=()=>{` + exhaust + `};try{null.x}catch(e){globalThis.keep=null;return 42}}`, jt.PhaseEvaluate},
		{`()=>{Error.prepareStackTrace=()=>{` + exhaust + `};return new Proxy({}, {ownKeys(){return null.x}})}`, jt.PhaseOutput},
		{`()=>{Error.prepareStackTrace=()=>{` + exhaust + `};try{new Error('x');return 42}catch(e){globalThis.keep=null;return 43}}`, jt.PhaseEvaluate},
	}
	e, err := New(context.Background(), Config{GuestMemoryBytes: 8 << 20, Limits: jt.Limits{Timeout: semanticTestTimeout}})
	if err != nil {
		f.Fatal(err)
	}
	defer e.Close()
	programs := make([]jt.Program, len(cases))
	for i, tc := range cases {
		f.Add(uint8(i))
		programs[i], err = e.Compile(context.Background(), tc.source)
		if err != nil {
			f.Fatal(err)
		}
	}
	healthy, err := e.Compile(context.Background(), `()=>42`)
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, choice uint8) {
		i := int(choice) % len(cases)
		r, err := programs[i].Eval(context.Background(), nil, jt.EvalOptions{})
		var ex *Exception
		var d *jt.Error
		if r.Defined || r.Value != nil || !errors.As(err, &ex) || !errors.As(err, &d) || d.Phase != cases[i].phase || errors.Is(err, jt.ErrLimit) || errors.Is(err, jt.ErrConversion) {
			t.Fatalf("terminal boundary %d: result=%+v error=%v", i, r, err)
		}
		if r, err := jt.EvalJSON(context.Background(), healthy, []byte(`null`), jt.EvalOptions{}); err != nil || string(r.Value) != "42" {
			t.Fatalf("fresh recovery: %+v %v", r, err)
		}
	})
}

// Exercise conversion syntax and escaped strings without executing arbitrary
// source or allocating unbounded guest collections in fuzz workers.
func FuzzOwnedStringConversion(f *testing.F) {
	for _, s := range []string{"plain", "😀", "\x00\n\"\\", "é", "\xff"} {
		f.Add(s)
	}
	e, err := New(context.Background(), Config{Limits: jt.Limits{Timeout: semanticTestTimeout}})
	if err != nil {
		f.Fatal(err)
	}
	defer e.Close()
	p, err := e.Compile(context.Background(), `input=>({value:input})`)
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 2048 {
			return
		}
		data, _ := json.Marshal(s)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		out, err := jt.EvalJSON(ctx, p, data, jt.EvalOptions{})
		if errors.Is(err, context.DeadlineExceeded) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		var want string
		_ = json.Unmarshal(data, &want)
		var got struct{ Value string }
		if err = json.Unmarshal(out.Value, &got); err != nil {
			t.Fatal(err)
		}
		if !out.Defined || got.Value != want {
			t.Fatalf("got %#v, want %q", out, want)
		}
	})
}

// Normalize through encoding/json's binary64 JSON model before comparison.
// This tests nested containers/keys as well as scalar strings; large-number
// decimal spelling and isolated-surrogate spelling are not comparison oracles.
func FuzzOwnedJSONConversion(f *testing.F) {
	for _, s := range []string{`null`, `[]`, `{"__proto__":{"x":1},"toJSON":"data"}`, `{"items":[1,true,null,"😀",{"é":"\ud800"}]}`, `9007199254740993`, `-0`, `{"x":[[[]]]}`} {
		f.Add(s)
	}
	e, err := New(context.Background(), Config{Limits: jt.Limits{Timeout: semanticTestTimeout}})
	if err != nil {
		f.Fatal(err)
	}
	defer e.Close()
	p, err := e.Compile(context.Background(), `input=>input`)
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, source string) {
		if len(source) > 2048 {
			return
		}
		var expected any
		if json.Unmarshal([]byte(source), &expected) != nil {
			return
		}
		var bounded func(any, int) bool
		count := 0
		bounded = func(v any, depth int) bool {
			count++
			if depth > 16 || count > 256 {
				return false
			}
			switch x := v.(type) {
			case []any:
				for _, item := range x {
					if !bounded(item, depth+1) {
						return false
					}
				}
			case map[string]any:
				for _, item := range x {
					if !bounded(item, depth+1) {
						return false
					}
				}
			}
			return true
		}
		if !bounded(expected, 0) {
			return
		}
		canonical, err := json.Marshal(expected)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		got, err := jt.EvalJSON(ctx, p, canonical, jt.EvalOptions{})
		if err != nil {
			t.Fatal(err)
		}
		var actual any
		if !got.Defined || json.Unmarshal(got.Value, &actual) != nil || !reflect.DeepEqual(actual, expected) {
			t.Fatalf("conversion: input %s, got %s", canonical, got.Value)
		}
	})
}
