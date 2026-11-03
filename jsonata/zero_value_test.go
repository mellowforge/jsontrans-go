package jsonata_test

import (
	"context"
	"testing"

	"github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/jsonata"
)

// Values not built by New, CompileRaw or EvalRaw fail with errors, not panics
// or silently missing built-ins.
func TestZeroValuesFailWithErrors(t *testing.T) {
	ctx := context.Background()
	var engine jsonata.Engine
	if _, err := engine.Compile(ctx, `$sum([1,2])`); err == nil {
		t.Error("zero Engine compiled")
	}
	var program jsonata.Program
	if _, err := program.Eval(ctx, nil, jsontrans.EvalOptions{}); err == nil {
		t.Error("zero Program evaluated")
	}
	if _, err := program.EvalRaw(ctx, nil, jsontrans.EvalOptions{}); err == nil {
		t.Error("zero Program evaluated raw")
	}
	if _, err := program.EvalPrepared(ctx, nil, jsontrans.EvalOptions{}); err == nil {
		t.Error("zero Program evaluated prepared")
	}
	e, err := jsonata.New()
	if err != nil {
		t.Fatal(err)
	}
	p, err := e.CompileRaw(ctx, `function($x){ $x + 1 }`)
	if err != nil {
		t.Fatal(err)
	}
	fn, err := p.EvalRaw(ctx, nil, jsontrans.EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fn.(jsonata.Callable).Call(&jsonata.CallContext{}, []jsonata.Value{float64(1)}); err == nil {
		t.Error("closure ran without an evaluation context")
	}
}
