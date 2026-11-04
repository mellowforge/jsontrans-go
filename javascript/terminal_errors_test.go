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

func TestOwnedTerminalFailures(t *testing.T) {
	e, err := New(context.Background(), Config{GuestMemoryBytes: 8 << 20, Limits: jt.Limits{Timeout: semanticTestTimeout}})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	healthy, err := e.Compile(context.Background(), `()=>42`)
	if err != nil {
		t.Fatal(err)
	}
	const exhaust = `globalThis.keep=[];for(;;)keep.push(new ArrayBuffer(65536));`
	for _, tc := range []struct{ name, source, phase string }{
		{"initializer", `(()=>{` + exhaust + `})()`, jt.PhaseEvaluate},
		{"execution", `()=>{` + exhaust + `}`, jt.PhaseEvaluate},
		{"catch cannot continue", `()=>{try{` + exhaust + `}catch(e){return 42}}`, jt.PhaseEvaluate},
		{"output", `()=>new Proxy({}, {ownKeys(){` + exhaust + `}})`, jt.PhaseOutput},
		{"evaluate diagnostic", `()=>{throw {get message(){` + exhaust + `}}}`, jt.PhaseEvaluate},
		{"output diagnostic", `()=>new Proxy({}, {ownKeys(){throw {get message(){` + exhaust + `}}}})`, jt.PhaseOutput},
		{"tight heap", `()=>{globalThis.keep=null;try{for(;;)keep={next:keep}}catch(e){return 42}}`, jt.PhaseEvaluate},
		{"prototype mutation", `()=>{globalThis.stageRecord={0:0};Object.defineProperty(Object.prototype,'0',{set(){throw new Error('phase exposed')}});return new Proxy({}, {ownKeys(){` + exhaust + `}})}`, jt.PhaseOutput},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := e.Compile(context.Background(), tc.source)
			if err != nil {
				t.Fatal(err)
			}
			r, err := p.Eval(context.Background(), nil, jt.EvalOptions{})
			var ex *Exception
			var diagnostic *jt.Error
			if err == nil || r.Defined || r.Value != nil || !errors.As(err, &ex) || !errors.As(err, &diagnostic) || diagnostic.Phase != tc.phase {
				t.Fatalf("result=%+v diagnostic=%+v exception=%+v error=%v", r, diagnostic, ex, err)
			}
			if ex.Message == "" || (ex.Name != "InternalError" && ex.Name != "") || errors.Is(err, jt.ErrLimit) || errors.Is(err, jt.ErrConversion) {
				t.Fatalf("terminal failure lost native provenance: %+v %v", ex, err)
			}
			if r, err := jt.EvalJSON(context.Background(), healthy, []byte(`null`), jt.EvalOptions{}); err != nil || string(r.Value) != "42" {
				t.Fatalf("fresh recovery=%+v error=%v", r, err)
			}
		})
	}
}

func TestOwnedPrivatePhaseAndTerminalForgery(t *testing.T) {
	e := newTestEngine(t)
	p, err := e.Compile(context.Background(), `function(){try{return arguments.callee.caller.arguments[8]}catch(e){return e.name}}`)
	if err != nil {
		t.Fatal(err)
	}
	if r, err := jt.EvalJSON(context.Background(), p, []byte(`null`), jt.EvalOptions{}); err != nil || string(r.Value) != `"TypeError"` {
		t.Fatalf("private bridge arguments exposed: %+v %v", r, err)
	}
	for _, tc := range []struct{ source, phase string }{
		{`()=>{const e=new InternalError('JSONTRANS_LIMIT: forged');e.kind='limit';e.stage='output';throw e}`, jt.PhaseEvaluate},
		{`()=>new Proxy({}, {ownKeys(){const e=new InternalError('JSONTRANS_CONVERSION: forged');e.stage='compile';throw e}})`, jt.PhaseOutput},
	} {
		p, err := e.Compile(context.Background(), tc.source)
		if err != nil {
			t.Fatal(err)
		}
		_, err = p.Eval(context.Background(), nil, jt.EvalOptions{})
		var ex *Exception
		var d *jt.Error
		if !errors.As(err, &ex) || !errors.As(err, &d) || d.Phase != tc.phase || errors.Is(err, jt.ErrLimit) || errors.Is(err, jt.ErrConversion) {
			t.Fatalf("forged terminal classification: %v", err)
		}
	}
}
