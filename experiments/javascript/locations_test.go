package javascript

import (
	"context"
	"errors"
	jt "github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/jsonata"
	"strings"
	"testing"
)

func TestOwnedRuleLocations(t *testing.T) {
	e := newTestEngine(t, OwnedQuickJS)
	for _, source := range []string{`()=>{const x=;}`, "()=>{\nconst x=;\n}", "()=>{const α='😀';const x=;}", "()=>{const x='é';const y=;}", "()=>{\r\nconst x=;\r\n}"} {
		_, err := e.Compile(context.Background(), source)
		var ex *Exception
		var diag *jt.Error
		if !errors.As(err, &ex) || !errors.As(err, &diag) {
			t.Fatal(err)
		}
		want := strings.Count(source[:strings.LastIndex(source, ";")], "\n") + 1
		if ex.Location == nil || ex.Location.ByteOffset != -1 || ex.Location.Line != want || ex.Location.Column != 1 || ex.Location.Reported || diag.Position != 0 {
			t.Errorf("source %q: location %#v want %d stack %q", source, ex.Location, want, ex.Stack)
		}
	}
	p, err := e.Compile(context.Background(), "()=>{\nthrow new TypeError('x');\n}")
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Eval(context.Background(), nil, jt.EvalOptions{})
	var ex *Exception
	if !errors.As(err, &ex) || ex.Location == nil || !ex.Location.Reported || ex.Location.Line != 2 || ex.Location.ByteOffset != -1 {
		t.Fatalf("runtime location: %#v %v", ex, err)
	}
}
func TestOwnedUnmappableLocations(t *testing.T) {
	for _, tc := range []struct {
		source    string
		line, col int
		reported  bool
	}{{"bridge.js", 2, 1, false}, {"rule.js", 999, 1, false}, {"rule.js", 2, 999, false}, {"eval", 2, 1, true}, {"rule.js", 1, 1, false}} {
		if got := mapRuleLocation("()=>1", tc.source, tc.line, tc.col, tc.reported); got != nil {
			t.Errorf("invalid location %+v: %+v", tc, got)
		}
	}
	e := newTestEngine(t, OwnedQuickJS)
	for _, source := range []string{`()=>{throw {message:'x',stack:'    at fake (rule.js:2:1)'}}`, `()=>{throw {message:'x',stack:'    at eval (eval.js:2:1)'}}`, `()=>{const e=new Error('x');delete e.stack;throw e}`} {
		p, err := e.Compile(context.Background(), source)
		if err != nil {
			t.Fatal(err)
		}
		_, err = p.Eval(context.Background(), nil, jt.EvalOptions{})
		var ex *Exception
		var d *jt.Error
		if !errors.As(err, &ex) || !errors.As(err, &d) {
			t.Fatal(err)
		}
		if d.Position != 0 || ex.Location != nil && (!ex.Location.Reported || ex.Location.ByteOffset != -1) {
			t.Fatalf("forged location: %#v", ex.Location)
		}
	}
}
func TestOwnedDiagnosticsThroughWrappers(t *testing.T) {
	ctx := context.Background()
	e := newTestEngine(t, OwnedQuickJS)
	x, err := jt.New(map[string]jt.Processor{"custom-js": e})
	if err != nil {
		t.Fatal(err)
	}
	p, err := x.Compile(ctx, "custom-js", `()=>{throw new TypeError('sentinel')}`)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := jt.PrepareJSON(ctx, []byte(`null`), jt.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	check := func(err error) {
		t.Helper()
		var ex *Exception
		var d *jt.Error
		if !errors.As(err, &ex) || !errors.As(err, &d) || ex.Name != "TypeError" || ex.Message != "sentinel" || d.Processor != "javascript" || d.Phase != jt.PhaseEvaluate {
			t.Fatalf("wrapper: %#v %#v %v", ex, d, err)
		}
	}
	_, err = p.Eval(ctx, nil, jt.EvalOptions{})
	check(err)
	_, err = jt.EvalJSON(ctx, p, []byte(`null`), jt.EvalOptions{})
	check(err)
	_, err = x.ProcessJSON(ctx, "custom-js", []byte(`null`), `()=>{throw new TypeError('sentinel')}`, jt.EvalOptions{})
	check(err)
	_, err = jt.EvalPrepared(ctx, p, doc, jt.EvalOptions{})
	check(err)
	nativeEngine, err := jsonata.New()
	if err != nil {
		t.Fatal(err)
	}
	native, err := nativeEngine.Compile(ctx, "1")
	if err != nil {
		t.Fatal(err)
	}
	batch, err := jt.NewBatch([]jt.Program{native, p})
	if err != nil {
		t.Fatal(err)
	}
	_, err = batch.EvalPrepared(ctx, doc, nil, jt.EvalOptions{})
	check(err)
	var be *jt.BatchError
	if !errors.As(err, &be) || be.Index != 1 {
		t.Fatal(err)
	}
}

func FuzzRuleLocation(f *testing.F) {
	f.Add("()=>1", 2, 1, true)
	f.Add("()=>{\r\nconst x='😀';\n}", 3, 5, false)
	f.Fuzz(func(t *testing.T, source string, line, column int, reported bool) {
		if len(source) > 65536 {
			return
		}
		l := mapRuleLocation(source, "rule.js", line, column, reported)
		if l != nil && (l.ByteOffset != -1 || l.Line < 1 || l.Column < 1 || l.Reported != reported) {
			t.Fatalf("invented offset or invalid location: %+v", l)
		}
	})
}
