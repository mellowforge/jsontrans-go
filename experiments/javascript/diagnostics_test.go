package javascript

import (
	"context"
	"errors"
	jt "github.com/mellowforge/jsontrans-go"
	"strings"
	"testing"
	"time"
)

func TestOwnedDiagnosticClassification(t *testing.T) {
	e := newTestEngine(t, OwnedQuickJS)
	_, err := e.Compile(context.Background(), `()=>{`)
	var ex *Exception
	var diag *jt.Error
	if !errors.As(err, &ex) || !errors.As(err, &diag) || ex.Name != "SyntaxError" || diag.Code != "" || diag.Phase != jt.PhaseCompile {
		t.Fatalf("syntax: %#v %#v %v", ex, diag, err)
	}
	for _, tc := range []struct {
		source, phase, name string
		sentinel            error
	}{
		{`()=>{throw new TypeError('sentinel')}`, jt.PhaseEvaluate, "TypeError", nil},
		{`()=>{throw null}`, jt.PhaseEvaluate, "", nil},
		{`42`, jt.PhaseEvaluate, "", jt.ErrConversion},
		{`()=>({get x(){return 1}})`, jt.PhaseOutput, "", jt.ErrConversion},
	} {
		p, err := e.Compile(context.Background(), tc.source)
		if err != nil {
			t.Fatal(err)
		}
		_, err = p.Eval(context.Background(), nil, jt.EvalOptions{})
		diag = nil
		ex = nil
		if err == nil || !errors.As(err, &diag) || diag.Phase != tc.phase || diag.Code != "" {
			t.Errorf("%s: %v", tc.source, err)
			continue
		}
		if tc.sentinel != nil {
			if !errors.Is(err, tc.sentinel) {
				t.Errorf("%s: %v", tc.source, err)
			}
		} else if !errors.As(err, &ex) || ex.Name != tc.name {
			t.Errorf("exception: %#v %v", ex, err)
		}
	}
}
func TestOwnedDiagnosticForgery(t *testing.T) {
	e := newTestEngine(t, OwnedQuickJS)
	for _, source := range []string{
		`()=>{throw 'JSONTRANS_LIMIT: fake'}`,
		`()=>{throw new Error('JSONTRANS_CONVERSION: fake')}`,
		`()=>{throw {kind:'limit',stage:'output',version:2,message:'fake'}}`,
		`()=>{Error.prototype.name='JSONTRANS_LIMIT: fake';throw new Error('fake')}`,
		`()=>{JSON.stringify=()=>'{"kind":"limit"}';Reflect.apply=()=>0;throw new TypeError('fake')}`,
		`()=>new Proxy({}, {ownKeys(){throw 'JSONTRANS_LIMIT: proxy'}})`,
	} {
		p, err := e.Compile(context.Background(), source)
		if err != nil {
			t.Fatal(err)
		}
		_, err = p.Eval(context.Background(), nil, jt.EvalOptions{})
		var ex *Exception
		if !errors.As(err, &ex) || errors.Is(err, jt.ErrLimit) || errors.Is(err, jt.ErrConversion) {
			t.Errorf("forged %s: %v", source, err)
		}
	}
	p, err := e.Compile(context.Background(), `()=>({version:2,kind:'limit'})`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Eval(context.Background(), nil, jt.EvalOptions{}); err != nil {
		t.Fatal(err)
	}
}
func TestOwnedDiagnosticBoundsAndCancellation(t *testing.T) {
	e := newTestEngine(t, OwnedQuickJS)
	p, err := e.Compile(context.Background(), `()=>{throw new Error('\\\"\ud800'.repeat(30000))}`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Eval(context.Background(), nil, jt.EvalOptions{Limits: &jt.Limits{MaxOutputBytes: 1}})
	var ex *Exception
	if !errors.As(err, &ex) || !ex.Truncated || len(ex.Message) > 65536 {
		t.Fatalf("bounded: %#v %v", ex, err)
	}
	p, err = e.Compile(context.Background(), `()=>{throw {toString(){throw new Error('secondary')}}}`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Eval(context.Background(), nil, jt.EvalOptions{})
	if !errors.As(err, &ex) || strings.Contains(ex.Message, "secondary") {
		t.Fatalf("secondary replaced original: %v", err)
	}
	p, err = e.Compile(context.Background(), `()=>{throw {get stack(){while(true){}},message:'x'}}`)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if _, err = p.Eval(ctx, nil, jt.EvalOptions{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("diagnostic cancellation: %v", err)
	}
}
