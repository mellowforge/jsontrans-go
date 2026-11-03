package jsontrans_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	jt "github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/jsonata"
	"github.com/mellowforge/jsontrans-go/jsonpath"
)

type failingCompiler struct{ err error }

func (p failingCompiler) Compile(context.Context, string) (jt.Program, error) { return nil, p.err }

func compileWith(t *testing.T, err error) error {
	t.Helper()
	x, e := jt.New(map[string]jt.Processor{"p": failingCompiler{err}})
	if e != nil {
		t.Fatal(e)
	}
	_, got := x.Compile(context.Background(), "p", "expression")
	if got == nil {
		t.Fatal("compile succeeded")
	}
	return got
}

// Wrappers around a native diagnostic stay reachable with errors.Is.
func TestDiagnosticKeepsWrappedChain(t *testing.T) {
	err := compileWith(t, errors.Join(jt.ErrLimit, &jt.Error{Code: "X1"}))
	var native *jt.Error
	if !errors.Is(err, jt.ErrLimit) || !errors.As(err, &native) || native.Processor != "p" || native.Code != "X1" {
		t.Fatalf("got %v (%#v)", err, native)
	}
}

// A processor's own diagnostic is completed, not wrapped a second time.
func TestDiagnosticCompletesOwnError(t *testing.T) {
	err := compileWith(t, &jt.Error{Phase: "compile", Code: "S0202", Position: 19, Cause: errors.New("unexpected token")})
	var native *jt.Error
	if !errors.As(err, &native) || native.Processor != "p" || native.Code != "S0202" || native.Position != 19 {
		t.Fatalf("got %v (%#v)", err, native)
	}
	if got := strings.Count(err.Error(), "[S0202]"); got != 1 {
		t.Fatalf("diagnostic repeated %d times: %v", got, err)
	}
}

// Another processor's diagnostic inside a host-function error is attributed to
// the evaluating processor at the call position, and stays reachable.
func TestDiagnosticAttributesForeignErrorToCaller(t *testing.T) {
	ctx := context.Background()
	path, err := jsonpath.New()
	if err != nil {
		t.Fatal(err)
	}
	_, inner := path.Compile(ctx, "$.")
	var pathError *jt.Error
	if !errors.As(inner, &pathError) || pathError.Processor != "jsonpath" {
		t.Fatalf("setup: %v", inner)
	}
	ata, err := jsonata.New(jsonata.WithFunction("lookup", jsonata.NativeFunction{Fn: func(*jsonata.CallContext, []jsonata.Value) (jsonata.Value, error) {
		return nil, fmt.Errorf("rule 7: %w", inner)
	}}))
	if err != nil {
		t.Fatal(err)
	}
	x, err := jt.New(map[string]jt.Processor{"jsonata": ata})
	if err != nil {
		t.Fatal(err)
	}
	_, err = x.Process(ctx, "jsonata", map[string]any{}, "1 + $lookup()", jt.EvalOptions{})
	var outer *jt.Error
	// JSONata 2.2.2 reports position 12, just after the call's "(".
	if !errors.As(err, &outer) || outer.Processor != "jsonata" || outer.Phase != "evaluate" || outer.Position != 12 {
		t.Fatalf("got %v (%#v)", err, outer)
	}
	if !strings.Contains(err.Error(), "rule 7") || !errors.Is(err, pathError) {
		t.Fatalf("lost host context or inner diagnostic: %v", err)
	}
}

// Compile errors keep the failing expression index.
func TestCompileBatchKeepsIndexWithNativeDiagnostic(t *testing.T) {
	ata, err := jsonata.New()
	if err != nil {
		t.Fatal(err)
	}
	x, err := jt.New(map[string]jt.Processor{"jsonata": ata})
	if err != nil {
		t.Fatal(err)
	}
	_, err = x.CompileBatch(context.Background(), "jsonata", []string{"a", "(", "b"})
	var batchError *jt.BatchError
	var native *jt.Error
	if !errors.As(err, &batchError) || batchError.Index != 1 || !errors.As(err, &native) || native.Code == "" {
		t.Fatalf("got %v", err)
	}
}

// Host errors are attributed the same way from direct and tail-position calls.
func TestHostErrorAttributionInTailCalls(t *testing.T) {
	ctx := context.Background()
	failure := errors.New("lookup failed")
	ata, err := jsonata.New(jsonata.WithFunction("lookup", jsonata.NativeFunction{Fn: func(*jsonata.CallContext, []jsonata.Value) (jsonata.Value, error) {
		return nil, failure
	}}))
	if err != nil {
		t.Fatal(err)
	}
	x, err := jt.New(map[string]jt.Processor{"jsonata": ata})
	if err != nil {
		t.Fatal(err)
	}
	for _, expression := range []string{`$lookup()`, `($g := function(){ $lookup() }; $g())`} {
		_, err := x.Process(ctx, "jsonata", map[string]any{}, expression, jt.EvalOptions{})
		var native *jt.Error
		if !errors.As(err, &native) || native.Processor != "jsonata" || native.Phase != "evaluate" || native.Position == 0 || !errors.Is(err, failure) {
			t.Errorf("%s: got %v (%#v)", expression, err, native)
		}
	}
}
