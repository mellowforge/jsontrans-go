package javascript

import (
	"context"
	"errors"
	"testing"

	jt "github.com/mellowforge/jsontrans-go"
)

func TestOwnedProxyKeyListSafety(t *testing.T) {
	e, err := New(context.Background(), Config{GuestMemoryBytes: 8 << 20, Limits: jt.Limits{Timeout: semanticTestTimeout}})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	healthy, err := e.Compile(context.Background(), `()=>42`)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, source, json, phase string
		native                    bool
		conversion                bool
	}{
		{"overflow", `()=>Reflect.ownKeys(new Proxy({}, {ownKeys(){return {length:536870913,0:'x'}}}))`, "", jt.PhaseEvaluate, true, false},
		{"empty", `()=>Reflect.ownKeys(new Proxy({}, {ownKeys(){return {length:0}}}))`, `[]`, "", false, false},
		{"small", `()=>Reflect.ownKeys(new Proxy({}, {ownKeys(){return {length:1,0:'x'}}}))`, `["x"]`, "", false, false},
		{"hole", `()=>Reflect.ownKeys(new Proxy({}, {ownKeys(){return {length:1}}}))`, "", jt.PhaseEvaluate, true, false},
		{"duplicate", `()=>Reflect.ownKeys(new Proxy({}, {ownKeys(){return ['x','x']}}))`, "", jt.PhaseEvaluate, true, false},
		{"symbol", `()=>Reflect.ownKeys(new Proxy({}, {ownKeys(){return [Symbol('x')]}}))`, "", jt.PhaseOutput, false, true},
		{"getter", `()=>Reflect.ownKeys(new Proxy({}, {ownKeys(){return {length:1,get 0(){throw new URIError('key getter')}}}}))`, "", jt.PhaseEvaluate, true, false},
		{"proxy", `()=>Reflect.ownKeys(new Proxy({}, {ownKeys(){return new Proxy({}, {get(){throw new TypeError('key proxy')}})}}))`, "", jt.PhaseEvaluate, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := e.Compile(context.Background(), tc.source)
			if err != nil {
				t.Fatal(err)
			}
			result, err := jt.EvalJSON(context.Background(), p, []byte(`null`), jt.EvalOptions{})
			if tc.json != "" {
				if err != nil || !result.Defined || string(result.Value) != tc.json {
					t.Fatalf("result=%+v error=%v", result, err)
				}
			} else {
				var native *Exception
				var diagnostic *jt.Error
				if err == nil || result.Defined || result.Value != nil || !errors.As(err, &diagnostic) || diagnostic.Phase != tc.phase {
					t.Fatalf("result=%+v diagnostic=%+v error=%v", result, diagnostic, err)
				}
				if errors.As(err, &native) != tc.native || errors.Is(err, jt.ErrConversion) != tc.conversion || errors.Is(err, jt.ErrLimit) {
					t.Fatalf("incorrect error provenance: %v", err)
				}
			}
			if r, err := jt.EvalJSON(context.Background(), healthy, []byte(`null`), jt.EvalOptions{}); err != nil || string(r.Value) != "42" {
				t.Fatalf("recovery=%+v error=%v", r, err)
			}
		})
	}
}
