package comparison

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	jt "github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/jsonata"
	gnata "github.com/recolabs/gnata"
)

var ctx = context.Background()
var sink any

type rule struct {
	expr    string
	want    any
	defined bool
}

var simple = []rule{
	{"Account.Name", "Firefly", true},
	{"Account.Active", true, true},
	{"Account.Count", float64(4), true},
	{"Account.Count > 2", true, true},
	{"$exists(Account.Name)", true, true},
	{"Account.Nothing", nil, true},
	{"Account.Missing", nil, false},
	{"Account.Count = 4", true, true},
}

// These are the four expressions in gnata v0.6.1 bench_test.go, unchanged.
var published = []rule{
	{"Account.Name", "Firefly", true},
	{"Account.Order.Product.SKU", []any{"0406654608", "040657863", "0406654608", "0406654603"}, true},
	{"Account.Order.Product[UnitPrice > 50].SKU", []any{"0406654608", "040657863", "0406654608"}, true},
	{"$sum(Account.Order.Product.(UnitPrice * Quantity * (1 - Discount)))", float64(477.692), true},
}

const core = `{"Account":{"Name":"Firefly","Active":true,"Count":4,"Nothing":null,"Order":[{"OrderID":"order103","Product":[{"SKU":"0406654608","Description":"Bowler Hat","UnitPrice":68.45,"Quantity":2,"Discount":0.1},{"SKU":"040657863","Description":"Cloak","UnitPrice":107.99,"Quantity":1,"Discount":0.2}]},{"OrderID":"order104","Product":[{"SKU":"0406654608","Description":"Bowler Hat","UnitPrice":68.45,"Quantity":4,"Discount":0.1},{"SKU":"0406654603","Description":"Trilby","UnitPrice":21.67,"Quantity":1,"Discount":0.0}]}]},"padding":"%s"}`

func fixture(size int) []byte {
	base := fmt.Sprintf(core, "")
	return []byte(fmt.Sprintf(core, strings.Repeat("x", size-len(base))))
}

// Unique predicates share Account.Count but never repeat an expression. Expected
// values are derived from the independent fixture constant, not another engine.
func rules(kind string, n int) []rule {
	if kind == "unique" {
		out := make([]rule, n)
		for i := range out {
			out[i] = rule{fmt.Sprintf("Account.Count > %d", i), 4 > i, true}
		}
		return out
	}
	source := simple
	if kind == "mixed" {
		source = append(append([]rule{}, published...), simple...)
	}
	out := make([]rule, n)
	for i := range out {
		out[i] = source[i%len(source)]
	}
	return out
}
func normalized(v any) (any, error) {
	b, e := json.Marshal(v)
	if e != nil {
		return nil, e
	}
	var x any
	e = json.Unmarshal(b, &x)
	return x, e
}
func check(t testing.TB, label string, v any, defined bool, r rule) {
	t.Helper()
	if defined != r.defined {
		t.Fatalf("%s %q defined=%v, want %v (%#v)", label, r.expr, defined, r.defined, v)
	}
	if !defined {
		return
	}
	got, e := normalized(v)
	if e != nil {
		t.Fatal(e)
	}
	if a, ok := got.(float64); ok {
		if expected, ok := r.want.(float64); ok && math.Abs(a-expected) < 1e-9 {
			return
		}
	}
	if !reflect.DeepEqual(got, r.want) {
		t.Fatalf("%s %q got %#v want %#v", label, r.expr, got, r.want)
	}
}

type compiled struct {
	ours    []jt.Program
	theirs  []*gnata.Expression
	stream  *gnata.StreamEvaluator
	indices []int
	input   any
}

func setup(t testing.TB, raw []byte, rs []rule) compiled {
	t.Helper()
	var c compiled
	if err := json.Unmarshal(raw, &c.input); err != nil {
		t.Fatal(err)
	}
	e, err := jsonata.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rs {
		p, err := e.Compile(ctx, r.expr)
		if err != nil {
			t.Fatalf("ours compile %q: %v", r.expr, err)
		}
		c.ours = append(c.ours, p)
		q, err := gnata.Compile(r.expr)
		if err != nil {
			t.Fatalf("gnata compile %q: %v", r.expr, err)
		}
		c.theirs = append(c.theirs, q)
		c.indices = append(c.indices, len(c.indices))
	}
	c.stream = gnata.NewStreamEvaluator(c.theirs)
	result, err := c.stream.EvalMany(ctx, raw, "fixture", c.indices)
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range rs {
		a, err := c.ours[i].Eval(ctx, c.input, jt.EvalOptions{})
		if err != nil {
			t.Fatalf("ours %q: %v", r.expr, err)
		}
		check(t, "ours", a.Value, a.Defined, r)
		j, err := jt.EvalJSON(ctx, c.ours[i], raw, jt.EvalOptions{})
		if err != nil {
			t.Fatalf("ours EvalJSON %q: %v", r.expr, err)
		}
		var decoded any
		if j.Defined {
			if err := json.Unmarshal(j.Value, &decoded); err != nil {
				t.Fatal(err)
			}
		}
		check(t, "ours EvalJSON", decoded, j.Defined, r)
		b, err := c.theirs[i].Eval(ctx, c.input)
		if err != nil {
			t.Fatalf("gnata %q: %v", r.expr, err)
		}
		check(t, "gnata Eval", b, b != nil, r)
		b, err = c.theirs[i].EvalBytes(ctx, raw)
		if err != nil {
			t.Fatalf("gnata bytes %q: %v", r.expr, err)
		}
		check(t, "gnata EvalBytes", b, b != nil, r)
		check(t, "gnata EvalMany", result[i], result[i] != nil, r)
	}
	return c
}
func TestParity(t *testing.T) {
	for _, size := range []int{1024, 8192, 65536} {
		for _, kind := range []string{"simple", "mixed", "unique"} {
			for _, n := range []int{4, 16, 64} {
				t.Run(fmt.Sprintf("%d/%s/%d", size, kind, n), func(t *testing.T) { setup(t, fixture(size), rules(kind, n)) })
			}
		}
	}
	t.Run("published", func(t *testing.T) { setup(t, fixture(1024), published) })
}
func BenchmarkBatch(b *testing.B) {
	for _, size := range []int{1024, 8192, 65536} {
		for _, kind := range []string{"simple", "mixed", "unique"} {
			for _, n := range []int{4, 16, 64} {
				raw := fixture(size)
				rs := rules(kind, n)
				c := setup(b, raw, rs)
				name := fmt.Sprintf("%d/%s/N%d", size, kind, n)
				b.Run(name+"/ours_EvalJSON", func(b *testing.B) {
					b.ReportAllocs()
					for range b.N {
						for _, p := range c.ours {
							v, err := jt.EvalJSON(ctx, p, raw, jt.EvalOptions{})
							if err != nil {
								b.Fatal(err)
							}
							sink = v
						}
					}
				})
				b.Run(name+"/ours_Eval", func(b *testing.B) {
					b.ReportAllocs()
					for range b.N {
						for _, p := range c.ours {
							v, err := p.Eval(ctx, c.input, jt.EvalOptions{})
							if err != nil {
								b.Fatal(err)
							}
							sink = v
						}
					}
				})
				b.Run(name+"/gnata_EvalMany", func(b *testing.B) {
					b.ReportAllocs()
					for range b.N {
						v, err := c.stream.EvalMany(ctx, raw, "fixture", c.indices)
						if err != nil {
							b.Fatal(err)
						}
						sink = v
					}
				})
				b.Run(name+"/gnata_validate_EvalMany", func(b *testing.B) {
					b.ReportAllocs()
					for range b.N {
						if !json.Valid(raw) {
							b.Fatal("invalid JSON")
						}
						v, err := c.stream.EvalMany(ctx, raw, "fixture", c.indices)
						if err != nil {
							b.Fatal(err)
						}
						sink = v
					}
				})
			}
		}
	}
}

func BenchmarkSingle(b *testing.B) {
	raw := fixture(1024)
	r := simple[0]
	c := setup(b, raw, []rule{r})
	e, err := jsonata.New()
	if err != nil {
		b.Fatal(err)
	}
	b.Run("ours_compile", func(b *testing.B) {
		for range b.N {
			p, err := e.Compile(ctx, r.expr)
			if err != nil {
				b.Fatal(err)
			}
			sink = p
		}
	})
	b.Run("gnata_compile", func(b *testing.B) {
		for range b.N {
			p, err := gnata.Compile(r.expr)
			if err != nil {
				b.Fatal(err)
			}
			sink = p
		}
	})
	b.Run("ours_Eval", func(b *testing.B) {
		for range b.N {
			v, err := c.ours[0].Eval(ctx, c.input, jt.EvalOptions{})
			if err != nil {
				b.Fatal(err)
			}
			sink = v
		}
	})
	b.Run("ours_EvalJSON", func(b *testing.B) {
		for range b.N {
			v, err := jt.EvalJSON(ctx, c.ours[0], raw, jt.EvalOptions{})
			if err != nil {
				b.Fatal(err)
			}
			sink = v
		}
	})
	b.Run("gnata_Eval", func(b *testing.B) {
		for range b.N {
			v, err := c.theirs[0].Eval(ctx, c.input)
			if err != nil {
				b.Fatal(err)
			}
			sink = v
		}
	})
	b.Run("gnata_EvalBytes", func(b *testing.B) {
		for range b.N {
			v, err := c.theirs[0].EvalBytes(ctx, raw)
			if err != nil {
				b.Fatal(err)
			}
			sink = v
		}
	})
}
