package jsonata

import (
	"context"
	"errors"
	jt "github.com/mellowforge/jsontrans-go"
	"testing"
)

func TestSignatureCastsAndContext(t *testing.T) {
	s, err := parseSignature("<a<n>s-:n>")
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.validate([]Value{float64(3)}, "focus")
	if err != nil {
		t.Fatal(err)
	}
	if a, ok := got[0].([]any); !ok || len(a) != 1 || a[0] != float64(3) || got[1] != "focus" {
		t.Fatalf("%#v", got)
	}
	_, err = s.validate([]Value{[]any{float64(1), "bad"}}, "focus")
	var e *jt.Error
	if !errors.As(err, &e) || e.Code != "T0412" {
		t.Fatalf("%v", err)
	}
}
func TestSignatureContextBeforeRequired(t *testing.T) {
	s, err := parseSignature("<s-n:n>")
	if err != nil {
		t.Fatal(err)
	}
	args, err := s.validate([]Value{float64(2)}, "focus")
	if err != nil || len(args) != 2 || args[0] != "focus" || args[1] != float64(2) {
		t.Fatalf("%#v %v", args, err)
	}
	_, err = s.validate([]Value{"x", "bad"}, Undefined)
	var e *jt.Error
	if !errors.As(err, &e) || e.Code != "T0410" {
		t.Fatalf("%v", err)
	}
}
func TestSignatureSyntaxErrors(t *testing.T) {
	for _, tc := range []struct{ sig, code string }{{"<n<n>:n>", "S0401"}, {"<(sa<n>):n>", "S0402"}} {
		_, err := parseSignature(tc.sig)
		var e *jt.Error
		if !errors.As(err, &e) || e.Code != tc.code {
			t.Fatalf("%s: %v", tc.sig, err)
		}
	}
}

func TestMalformedSignatureCannotPanic(t *testing.T) {
	for _, signature := range []string{"<s++-:s>", "<s??-:s>", "<s-+-:s>", "<s*--:s>"} {
		_, err := parseSignature(signature)
		if err == nil {
			t.Fatalf("malformed signature accepted: %s", signature)
		}
	}
}

func TestNestedEvalClassifiesSignatureSyntaxErrors(t *testing.T) {
	engine, _ := New()
	p, err := engine.CompileRaw(context.Background(), `$eval("function($x)<n<n>:n>{$x}")`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.EvalRaw(context.Background(), nil, jt.EvalOptions{})
	var native *jt.Error
	if !errors.As(err, &native) || native.Code != "D3120" {
		t.Fatalf("%v", err)
	}
}
