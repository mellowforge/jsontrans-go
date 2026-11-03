package document

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/mellowforge/jsontrans-go/internal/budget"
	"github.com/mellowforge/jsontrans-go/internal/jsonvalue"
)

func TestScanChargesLongTokens(t *testing.T) {
	for name, input := range map[string]string{
		"space":   strings.Repeat(" ", 10000) + "null",
		"number":  "1" + strings.Repeat("0", 10000),
		"escaped": `"` + strings.Repeat(`\u0061`, 2000) + `"`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse(context.Background(), []byte(input), Options{Limits: &budget.Limits{MaxSteps: 2}})
			if !errors.Is(err, budget.ErrLimit) {
				t.Fatalf("long scan bypassed budget: %v", err)
			}
		})
	}
}

func TestMaterializeExactDepthAndOverflowSummary(t *testing.T) {
	l := budget.Limits{MaxDepth: 4}
	d, err := Parse(context.Background(), []byte(`[[[0]]]`), Options{Limits: &l})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = d.Materialize(context.Background(), Options{Limits: &l}); err != nil {
		t.Fatal(err)
	}
	d, err = Parse(context.Background(), []byte(`{"a":1e9999,"a":1}`), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if d.Stats().Binary64Overflow {
		t.Fatal("overwritten number incorrectly marked overflow")
	}
}

func FuzzPrepareAndMaterialize(f *testing.F) {
	for _, s := range []string{`null`, `{"a":1,"a":2}`, `[[],{},true,false,-1.23e20]`, `{"\ud800":"\ud800","x":"é"}`, `"\ud83d\ude00"`, `{"a":01}`, `1e999`} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 16384 {
			return
		}
		l := budget.DefaultLimits()
		l.MaxDepth = 64
		l.MaxSteps = 50000
		b, _ := budget.New(context.Background(), l)
		defer b.Close()
		want, oldErr := jsonvalue.Decode(raw, b)
		d, err := Parse(context.Background(), raw, Options{Limits: &l})
		// The two paths charge actual work differently; exhausted budgets are not
		// syntax differences.
		if errors.Is(oldErr, budget.ErrLimit) || errors.Is(err, budget.ErrLimit) {
			return
		}
		if (oldErr == nil) != (err == nil) {
			t.Fatalf("decode=%v prepare=%v input=%q", oldErr, err, raw)
		}
		if err != nil {
			return
		}
		got, err := d.Materialize(context.Background(), Options{Limits: &l})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v want %#v", got, want)
		}
	})
}
