//go:build jsontext_experiment && go1.27 && goexperiment.jsonv2

package jsonvalue

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/mellowforge/jsontrans-go/internal/budget"
)

func compareJSONDecoders(t testing.TB, raw []byte, limits budget.Limits) {
	t.Helper()
	var values [2]any
	var orders [2][]string
	var errs [2]error
	for i, decode := range []func([]byte, *budget.Budget) (any, error){decodeLegacy, decodeJSONText} {
		ctx := WithObjectOrder(context.Background())
		b, err := budget.New(ctx, limits)
		if err != nil {
			t.Fatal(err)
		}
		values[i], errs[i] = decode(raw, b)
		orders[i] = ObjectKeys(ctx, values[i])
		b.Close()
	}
	if (errs[0] == nil) != (errs[1] == nil) {
		t.Fatalf("legacy=%v jsontext=%v input=%q", errs[0], errs[1], raw)
	}
	if errs[0] != nil {
		for _, class := range []error{ErrInvalid, budget.ErrLimit, context.Canceled, context.DeadlineExceeded} {
			if errors.Is(errs[0], class) != errors.Is(errs[1], class) {
				t.Fatalf("error class %v: legacy=%v jsontext=%v input=%q", class, errs[0], errs[1], raw)
			}
		}
		return
	}
	if !reflect.DeepEqual(values[0], values[1]) || !reflect.DeepEqual(orders[0], orders[1]) {
		t.Fatalf("legacy=%#v (%v), jsontext=%#v (%v), input=%q", values[0], orders[0], values[1], orders[1], raw)
	}
}

func TestJSONTextCompatibility(t *testing.T) {
	for _, raw := range []string{
		`null`, `true`, `false`, `[]`, `{}`, `[[],{},null,true,false]`,
		`[-0,9007199254740993,1.2300e+45,1e9999]`,
		`{"b":1,"2":2,"1":3,"b":4,"a":{"z":0,"a":1}}`,
		`{"\ud800":"\udfff","pair":"\ud83d\ude00","unicode":"é世😀"}`,
		`{"\ud800":0,"\ud801":1,"\ud800":2}`, `"<&>\n\t\\\""`,
		" \n {\"x\": [1, 2]} \t", `"\ufffd"`,
		``, ` `, `[`, `{`, `[,]`, `[1,]`, `{"a":1,}`, `{"a" 1}`, `{"a":}`, `{"a":1 "b":2}`,
		`01`, `1.`, `1e`, `+1`, `NaN`, `true false`, `null x`, `"\uZZZZ"`, `"\ud800`,
		string([]byte{'"', 0xff, '"'}),
	} {
		t.Run(raw, func(t *testing.T) { compareJSONDecoders(t, []byte(raw), budget.Limits{}) })
	}
}

func TestJSONTextMalformedCloserBeforeLimits(t *testing.T) {
	for _, raw := range []string{`[}`, `{]`} {
		for _, limits := range []budget.Limits{{MaxSteps: 1}, {MaxDepth: 1}} {
			compareJSONDecoders(t, []byte(raw), limits)
		}
	}
}

func TestJSONTextLimits(t *testing.T) {
	for _, limits := range []budget.Limits{
		{MaxDepth: 1}, {MaxDepth: 2}, {MaxCollectionItems: 1}, {MaxInputBytes: 8},
	} {
		compareJSONDecoders(t, []byte(`{"x":[1,2],"y":3}`), limits)
	}
	// Exact existing step accounting includes the final validation/clone pass.
	for n := int64(1); n <= 40; n++ {
		compareJSONDecoders(t, []byte(`{"x":[1,2],"x":3}`), budget.Limits{MaxSteps: n})
	}
	compareJSONDecoders(t, []byte(`"`+strings.Repeat(`\u0001`, 500)+`"`), budget.Limits{MaxInputBytes: 4000})
	ctx, cancel := context.WithCancel(context.Background())
	b, err := budget.New(ctx, budget.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	cancel()
	if _, err = decodeJSONText([]byte(`{"x":1}`), b); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestJSONTextOwnsResultsAndConcurrentCalls(t *testing.T) {
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			raw := []byte(`{"key":["escaped\nvalue",1234567890123456789,"plain"]}`)
			b, err := budget.New(context.Background(), budget.Limits{})
			if err != nil {
				t.Error(err)
				return
			}
			defer b.Close()
			got, err := decodeJSONText(raw, b)
			if err != nil {
				t.Error(err)
				return
			}
			before, err := Encode(got, b)
			if err != nil {
				t.Error(err)
				return
			}
			for i := range raw {
				raw[i] = 'x'
			}
			after, err := Encode(got, b)
			if err != nil || string(before) != string(after) {
				t.Errorf("borrowed result changed: %s -> %s (%v)", before, after, err)
			}
		})
	}
	wg.Wait()
}

func FuzzJSONTextCompatibility(f *testing.F) {
	for _, raw := range []string{`null`, `{"x":1,"x":2}`, `[0,-0,1e999,"\ud800"]`, `{"\ud800":"\udfff"}`, `"é"`, `01`, `[1,]`} {
		f.Add([]byte(raw))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 16384 {
			return
		}
		compareJSONDecoders(t, raw, budget.Limits{MaxDepth: 64, MaxSteps: 100000})
	})
}
