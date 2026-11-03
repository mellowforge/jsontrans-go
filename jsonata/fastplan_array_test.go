package jsonata

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/internal/jsonvalue"
)

func TestFastNestedProjection(t *testing.T) {
	ctx := context.Background()
	e, _ := New()
	for _, tc := range []struct {
		expr, input string
		want        jsontrans.Result
		fast        bool
	}{
		{`Account.Order.Product.SKU`, `{"Account":{"Order":[{"Product":[{"SKU":"A"},{"SKU":"B"}]},{"Product":{"SKU":"C"}}]}}`, jsontrans.Result{Value: []any{"A", "B", "C"}, Defined: true}, true},
		{`Account.Order.Product[UnitPrice > 50].SKU`, `{"Account":{"Order":[{"Product":[{"SKU":"A","UnitPrice":49},{"SKU":"B","UnitPrice":51}]},{"Product":{"SKU":"C","UnitPrice":75}}]}}`, jsontrans.Result{Value: []any{"B", "C"}, Defined: true}, true},
		{`Account.Order.Product.SKU`, `{"Account":{"Order":[[{"Product":{"SKU":"A"}}]]}}`, jsontrans.Result{Value: "A", Defined: true}, true},
		{`Account.Order.Product.SKU`, `{"Account":{"Order":{"Product":[{}, {"SKU":null}]}}}`, jsontrans.Result{Defined: true}, true},
		{`Account.Order.Product.SKU`, `{"Account":{"Order":[]}}`, jsontrans.Result{}, true},
		{`Account.Order.Product[UnitPrice > 50].SKU`, `{"Account":{"Order":{"Product":{"SKU":"B","UnitPrice":75}}}}`, jsontrans.Result{Value: "B", Defined: true}, true},
		{`Account.Order.Product.SKU`, `{"Account":{"Order":{"Product":{"SKU":"\ud800"}}}}`, jsontrans.Result{Value: jsonvalue.UTF16String("\xed\xa0\x80"), Defined: true}, true},
		{`Account.Order.Product.SKU`, `{"Account":{"Order":{"Product":{"SKU":["A","B"]}}}}`, jsontrans.Result{Value: []any{"A", "B"}, Defined: true}, false},
	} {
		t.Run(tc.expr+tc.input, func(t *testing.T) {
			p, err := e.CompileRaw(ctx, tc.expr)
			if err != nil {
				t.Fatal(err)
			}
			d, err := jsontrans.PrepareJSON(ctx, []byte(tc.input), jsontrans.PrepareOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if tc.fast {
				if p.fast == nil {
					t.Fatal("no fast plan")
				}
				got, handled, err := p.fast.eval(d.Root(), mustBudget(t))
				if err != nil || !handled {
					t.Fatalf("not directly handled: %v %v", handled, err)
				}
				if got.Defined {
					got.Value, err = jsonvalue.Clone(got.Value, mustBudget(t), 1<<20)
					if err != nil {
						t.Fatal(err)
					}
				}
				if !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("direct got %#v want %#v", got, tc.want)
				}
			}
			got, err := p.EvalPrepared(ctx, d, jsontrans.EvalOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("prepared got %#v want %#v", got, tc.want)
			}
			v, err := d.Materialize(ctx, jsontrans.PrepareOptions{})
			if err != nil {
				t.Fatal(err)
			}
			slow, err := p.Eval(ctx, v, jsontrans.EvalOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, slow) {
				t.Fatalf("got %#v interpreter %#v", got, slow)
			}
		})
	}
}

func TestFastArithmeticProjectionSum(t *testing.T) {
	ctx := context.Background()
	e, _ := New()
	expr := `$sum(Account.Order.Product.(UnitPrice * Quantity * (1 - Discount)))`
	p, err := e.CompileRaw(ctx, expr)
	if err != nil {
		t.Fatal(err)
	}
	if p.fast == nil {
		t.Fatal("arithmetic projection has no fast plan")
	}
	for _, tc := range []struct {
		input string
		want  jsontrans.Result
	}{
		{`{"Account":{"Order":[{"Product":[{"UnitPrice":10,"Quantity":2,"Discount":0.25},{"UnitPrice":20,"Quantity":3,"Discount":0.5}]},{"Product":{"UnitPrice":5,"Quantity":4,"Discount":0}}]}}`, jsontrans.Result{Value: float64(65), Defined: true}},
		{`{"Account":{"Order":{"Product":[]}}}`, jsontrans.Result{}},
	} {
		d, err := jsontrans.PrepareJSON(ctx, []byte(tc.input), jsontrans.PrepareOptions{})
		if err != nil {
			t.Fatal(err)
		}
		got, handled, err := p.fast.eval(d.Root(), mustBudget(t))
		if err != nil || !handled {
			t.Fatalf("not handled: %v %v", handled, err)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("got %#v want %#v", got, tc.want)
		}
		v, err := d.Materialize(ctx, jsontrans.PrepareOptions{})
		if err != nil {
			t.Fatal(err)
		}
		slow, err := p.Eval(ctx, v, jsontrans.EvalOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, slow) {
			t.Fatalf("got %#v interpreter %#v", got, slow)
		}
	}
}

// These fixtures catch accidental flattening of final array values and preserve
// native errors (including source positions) for unsupported operand shapes.
func TestFastNestedProjectionFallbackParity(t *testing.T) {
	ctx := context.Background()
	e, _ := New()
	expressions := []string{`Account.Order.Product[$$.Account.limit=50].SKU`, `Account.Order.Product.SKU`, `Account.Order.Product[UnitPrice>50].SKU`, `$sum(Account.Order.Product.(UnitPrice * Quantity * (1 - Discount)))`}
	inputs := []string{
		`{"Account":{"limit":50,"Order":{"Product":{"SKU":"root"}}}}`,
		`{}`, `{"Account":null}`, `{"Account":[]}`,
		`{"Account":[{"Order":{"Product":{"SKU":"one","UnitPrice":70,"Quantity":2,"Discount":0.25}}},{"Order":{"Product":{"SKU":null,"UnitPrice":1,"Quantity":1,"Discount":0}}}]}`,
		`{"Account":{"Order":{"Product":[[{"SKU":"nested","UnitPrice":60,"Quantity":1,"Discount":0}]]}}}`,
		`{"Account":{"Order":{"Product":[{"SKU":["a","b"],"UnitPrice":51,"Quantity":1,"Discount":0}]}}}`,
		`{"Account":{"Order":{"Product":[{"SKU":"a","UnitPrice":null,"Quantity":1,"Discount":0}]}}}`,
		`{"Account":{"Order":{"Product":[{"SKU":"a","UnitPrice":"bad","Quantity":1,"Discount":0}]}}}`,
		`{"Account":{"Order":{"Product":[{"SKU":"a","Quantity":1,"Discount":0}]}}}`,
		`{"Account":{"Order":{"Product":[{"SKU":"a","UnitPrice":1e308,"Quantity":2,"Discount":0}]}}}`,
		`{"Account":{"Order":{"Product":[{"SKU":"a","UnitPrice":[51],"Quantity":1,"Discount":0}]}}}`,
	}
	for _, input := range inputs {
		d, err := jsontrans.PrepareJSON(ctx, []byte(input), jsontrans.PrepareOptions{})
		if err != nil {
			t.Fatal(err)
		}
		v, err := d.Materialize(ctx, jsontrans.PrepareOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for _, expr := range expressions {
			p, err := e.CompileRaw(ctx, expr)
			if err != nil {
				t.Fatal(err)
			}
			got, ge := p.EvalPrepared(ctx, d, jsontrans.EvalOptions{})
			want, we := p.Eval(ctx, v, jsontrans.EvalOptions{})
			if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(ge, we) {
				t.Errorf("%s %s: got %#v/%#v want %#v/%#v", expr, input, got, ge, want, we)
			}
		}
	}
}

func FuzzPreparedNestedPlans(f *testing.F) {
	for _, input := range []string{
		`{"Account":{"Order":{"Product":[{"SKU":"a","UnitPrice":51,"Quantity":2,"Discount":0.25}]}}}`,
		`{"Account":{"Order":[[{"Product":{"SKU":null}}]]}}`,
		`{"Account":{"Order":{"Product":{"SKU":"\ud800","UnitPrice":"bad"}}}}`,
	} {
		f.Add(input)
	}
	ctx := context.Background()
	e, _ := New()
	var programs []*Program
	for _, expr := range []string{`Account.Order.Product.SKU`, `Account.Order.Product[UnitPrice>50].SKU`, `$sum(Account.Order.Product.(UnitPrice * Quantity * (1 - Discount)))`} {
		p, err := e.CompileRaw(ctx, expr)
		if err != nil {
			f.Fatal(err)
		}
		programs = append(programs, p)
	}
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 4096 {
			return
		}
		d, err := jsontrans.PrepareJSON(ctx, []byte(input), jsontrans.PrepareOptions{})
		if err != nil || d.Stats().Nodes > 256 || d.Stats().Depth > 20 {
			return
		}
		value, err := d.Materialize(ctx, jsontrans.PrepareOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range programs {
			got, ge := p.EvalPrepared(ctx, d, jsontrans.EvalOptions{})
			want, we := p.Eval(ctx, value, jsontrans.EvalOptions{})
			if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(ge, we) {
				t.Fatalf("input %s: got %#v/%#v want %#v/%#v", input, got, ge, want, we)
			}
		}
	})
}

// Numeric field comparisons are the unique-rule hot path: converting the field
// to an interface number would allocate once per rule before producing a bool.
func TestFastNumericComparisonAvoidsBoxing(t *testing.T) {
	ctx := context.Background()
	e, _ := New()
	p, err := e.CompileRaw(ctx, `Account.Count > 500`)
	if err != nil {
		t.Fatal(err)
	}
	d, err := jsontrans.PrepareJSON(ctx, []byte(`{"Account":{"Count":750}}`), jsontrans.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	b := mustBudget(t)
	allocs := testing.AllocsPerRun(100, func() {
		got, handled, err := p.fast.eval(d.Root(), b)
		if err != nil || !handled || got.Value != true || !got.Defined {
			panic("invalid numeric comparison")
		}
	})
	if allocs != 0 {
		t.Fatalf("numeric comparison boxes its operands: %g allocations", allocs)
	}
}

func TestFastNumericComparisonSemantics(t *testing.T) {
	ctx := context.Background()
	e, _ := New()
	for _, tc := range []struct {
		expr string
		want bool
	}{
		{`Account.Count = 500`, false}, {`Account.Count != 500`, true}, {`Account.Count > 500`, true}, {`Account.Count >= 500`, true}, {`Account.Count < 500`, false}, {`Account.Count <= 500`, false},
		{`500 = Account.Count`, false}, {`500 != Account.Count`, true}, {`500 > Account.Count`, false}, {`500 >= Account.Count`, false}, {`500 < Account.Count`, true}, {`500 <= Account.Count`, true},
		{"Account.`cøunt` > 500", true}, {`Account.Counts[0] > 500`, true}, {`500 < Account.Counts[0]`, true},
	} {
		p, err := e.CompileRaw(ctx, tc.expr)
		if err != nil {
			t.Fatal(err)
		}
		for _, input := range []string{`{"Account":{"Count":750,"cøunt":750,"Counts":[750]}}`, `{}`, `{"Account":{"Count":null}}`, `{"Account":{"Count":false}}`, `{"Account":{"Count":[750]}}`, `{"Account":{"Count":"\ud800"}}`, `{"Account":[{"Count":750},{"Count":1}]}`} {
			d, err := jsontrans.PrepareJSON(ctx, []byte(input), jsontrans.PrepareOptions{})
			if err != nil {
				t.Fatal(err)
			}
			got, ge := p.EvalPrepared(ctx, d, jsontrans.EvalOptions{})
			v, err := d.Materialize(ctx, jsontrans.PrepareOptions{})
			if err != nil {
				t.Fatal(err)
			}
			want, we := p.Eval(ctx, v, jsontrans.EvalOptions{})
			if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(ge, we) {
				t.Fatalf("%s %s got %#v/%#v want %#v/%#v", tc.expr, input, got, ge, want, we)
			}
			if input == `{"Account":{"Count":750,"cøunt":750,"Counts":[750]}}` && (!got.Defined || got.Value != tc.want) {
				t.Fatalf("%s: got %#v want %v", tc.expr, got, tc.want)
			}
		}
	}
}

func TestRawArithmeticProjectionUsesNativeFallback(t *testing.T) {
	ctx := context.Background()
	e, _ := New()
	for _, expression := range []struct {
		expr       string
		one, two   any
		arithmetic bool
	}{
		{`A.(x*y)`, float64(6), float64(20), true},
		{`A.(x+y)`, float64(5), float64(9), true},
		{`A.(x-y)`, float64(-1), float64(-1), true},
		{`A.(x/y)`, float64(2.0 / 3.0), float64(0.8), true},
		{`A.(42)`, float64(42), float64(42), false},
	} {
		t.Run(expression.expr, func(t *testing.T) {
			p, err := e.CompileRaw(ctx, expression.expr)
			if err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				input string
				want  jsontrans.Result
				bad   bool
			}{
				{`{"A":{"x":2,"y":3}}`, jsontrans.Result{Value: expression.one, Defined: true}, false},
				{`{"A":{"x":[],"y":3}}`, jsontrans.Result{Value: expression.one, Defined: true}, true},
				{`{"A":[{"x":2,"y":3},{"x":4,"y":5}]}`, jsontrans.Result{Value: []any{expression.one, expression.two}, Defined: true}, false},
			} {
				d, err := jsontrans.PrepareJSON(ctx, []byte(tc.input), jsontrans.PrepareOptions{})
				if err != nil {
					t.Fatal(err)
				}
				got, err := p.EvalPrepared(ctx, d, jsontrans.EvalOptions{})
				if tc.bad && expression.arithmetic {
					var native *jsontrans.Error
					if !errors.As(err, &native) || native.Code != "T2001" || native.Position != 5 {
						t.Errorf("%s: got %#v/%v want T2001 at 5", tc.input, got, err)
					}
					continue
				}
				if err != nil || !reflect.DeepEqual(got, tc.want) {
					t.Errorf("%s: got %#v/%v want %#v", tc.input, got, err, tc.want)
				}
			}
		})
	}
}
