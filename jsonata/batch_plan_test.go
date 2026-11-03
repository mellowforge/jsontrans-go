package jsonata

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/internal/budget"
	"github.com/mellowforge/jsontrans-go/internal/prepared"
)

func TestCompiledBatchUniqueRulesAndLocalState(t *testing.T) {
	ctx := context.Background()
	e, _ := New()
	handlers := make([]*prepared.Handler, 32)
	for i := range handlers {
		p, err := e.CompileRaw(ctx, fmt.Sprintf("Account.Count > %d", i))
		if err != nil {
			t.Fatal(err)
		}
		handlers[i] = p.NativePrepared()
	}
	plan := prepared.CompileBatch(ctx, handlers)
	if plan == nil {
		t.Fatal("no compiled native batch")
	}
	run := func(t *testing.T, input string, count int) {
		t.Helper()
		d, err := jsontrans.PrepareJSON(ctx, []byte(input), jsontrans.PrepareOptions{})
		if err != nil {
			t.Fatal(err)
		}
		state := plan.NewState(d)
		for i := range handlers {
			b := mustBudget(t)
			r, err := plan.Eval(i, prepared.Request{Document: d, Limits: b.Limits, Budget: b}, state)
			if err != nil {
				t.Fatal(err)
			}
			if !r.Defined || r.Value != (count > i) || r.Bytes != len(fmt.Sprint(count > i)) {
				t.Fatalf("rule %d: %#v", i, r)
			}
		}
	}
	run(t, `{"Account":{"Count":16}}`, 16)
	run(t, `{"Account":{"Count":3}}`, 3)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); run(t, `{"Account":{"Count":7}}`, 7) }()
	}
	wg.Wait()
}

func TestCompiledBatchFallbackAndBudgets(t *testing.T) {
	ctx := context.Background()
	e, _ := New()
	exprs := []string{`Account.Count > 2`, `$keys(Account)`, `Account.Order.Product[UnitPrice>50].SKU`, `$bound`}
	handlers := make([]*prepared.Handler, len(exprs))
	for i, expr := range exprs {
		p, err := e.CompileRaw(ctx, expr)
		if err != nil {
			t.Fatal(err)
		}
		handlers[i] = p.NativePrepared()
	}
	plan := prepared.CompileBatch(ctx, handlers)
	if plan == nil {
		t.Fatal("no compiled batch")
	}
	d, err := jsontrans.PrepareJSON(ctx, []byte(`{"Account":{"z":null,"Count":7,"a":true,"Order":{"Product":{"UnitPrice":"bad","SKU":"A"}}}}`), jsontrans.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	state := plan.NewState(d)
	for i := range handlers {
		b := mustBudget(t)
		req := prepared.Request{Document: d, Bindings: map[string]any{"bound": "yes"}, Customized: true, Limits: b.Limits, Budget: b}
		got, ge := plan.Eval(i, req, state)
		b2 := mustBudget(t)
		req.Budget = b2
		want, we := handlers[i].Eval(req)
		if fmt.Sprint(ge) != fmt.Sprint(we) || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: %#v/%v want %#v/%v", exprs[i], got, ge, want, we)
		}
	}
	b := mustBudget(t)
	b.Limits.MaxSteps = 1
	if _, err = plan.Eval(0, prepared.Request{Document: d, Limits: b.Limits, Budget: b}, state); !errors.Is(err, budget.ErrLimit) {
		t.Fatalf("step budget: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	var cb budget.Budget
	if err = budget.InitFastResolved(&cb, cancelled, budget.DefaultLimits()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err = plan.Eval(0, prepared.Request{Document: d, Limits: budget.DefaultLimits(), Budget: &cb}, state); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
}

func BenchmarkPreparedPrefixBatch(b *testing.B) {
	for _, fixture := range []struct{ name, path, input string }{
		{"shallow", "Account.Count", `{"Account":{"Count":16}}`},
		{"deep", "Account.Data.Summary.Stats.Count", `{"Account":{"Data":{"Summary":{"Stats":{"Count":16}}}}}`},
	} {
		b.Run(fixture.name, func(b *testing.B) {
			ctx := context.Background()
			e, _ := New()
			handlers := make([]*prepared.Handler, 32)
			for i := range handlers {
				p, err := e.CompileRaw(ctx, fmt.Sprintf("%s > %d", fixture.path, i))
				if err != nil {
					b.Fatal(err)
				}
				handlers[i] = p.NativePrepared()
			}
			plan := prepared.CompileBatch(ctx, handlers)
			if plan == nil {
				b.Fatal("no compiled batch")
			}
			d, err := jsontrans.PrepareJSON(ctx, []byte(fixture.input), jsontrans.PrepareOptions{})
			if err != nil {
				b.Fatal(err)
			}
			for _, compiled := range []bool{false, true} {
				b.Run(fmt.Sprint(compiled), func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						var state any
						if compiled {
							state = plan.NewState(d)
						}
						for i, h := range handlers {
							var bd budget.Budget
							if err := budget.InitFastResolved(&bd, ctx, budget.DefaultLimits()); err != nil {
								b.Fatal(err)
							}
							req := prepared.Request{Document: d, Limits: bd.Limits, Budget: &bd}
							var err error
							if compiled {
								_, err = plan.Eval(i, req, state)
							} else {
								_, err = h.Eval(req)
							}
							bd.Close()
							if err != nil {
								b.Fatal(err)
							}
						}
					}
				})
			}
		})
	}
}

func TestBatchCompilerRejectsForeignOwnersAndCancellation(t *testing.T) {
	ctx := context.Background()
	e, _ := New()
	p, err := e.CompileRaw(ctx, `Account.Count > 0`)
	if err != nil {
		t.Fatal(err)
	}
	foreign := prepared.New(new(int), func(prepared.Request) (prepared.Result, error) {
		return prepared.Result{Value: "foreign", Defined: true, Bytes: 9}, nil
	})
	if _, ok := compilePreparedBatch(ctx, []*prepared.Handler{p.NativePrepared(), foreign}); ok {
		t.Fatal("foreign owner admitted")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, ok := compilePreparedBatch(cancelled, []*prepared.Handler{p.NativePrepared()}); ok {
		t.Fatal("cancelled compilation admitted")
	}
	oversized := make([]*prepared.Handler, maxBatchCompileWork+1)
	for i := range oversized {
		oversized[i] = p.NativePrepared()
	}
	if _, ok := compilePreparedBatch(ctx, oversized); ok {
		t.Fatal("unbounded batch admitted")
	}
}
