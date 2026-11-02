package conformance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"reflect"
	"testing"

	"github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/jsonpath"
)

func equivalent(a, b any) bool {
	if x, ok := a.(json.Number); ok {
		y, ok := b.(json.Number)
		if !ok {
			return false
		}
		p, pok := new(big.Rat).SetString(string(x))
		q, qok := new(big.Rat).SetString(string(y))
		return pok && qok && p.Cmp(q) == 0
	}
	switch x := a.(type) {
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !equivalent(x[i], y[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for k, v := range x {
			w, ok := y[k]
			if !ok || !equivalent(v, w) {
				return false
			}
		}
		return true
	}
	return reflect.DeepEqual(a, b)
}
func TestJSONPathCTS(t *testing.T) {
	data, err := os.ReadFile("testdata/jsonpath/cts.json")
	if err != nil {
		t.Fatal(err)
	}
	var suite struct {
		Tests []struct {
			Name, Selector string
			Document       any
			Result         any
			Results        []any
			Invalid        bool `json:"invalid_selector"`
		}
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err = dec.Decode(&suite); err != nil {
		t.Fatal(err)
	}
	e, err := jsonpath.New()
	if err != nil {
		t.Fatal(err)
	}
	errata := map[int]struct {
		name, selector string
		original       []any
	}{
		516: {"functions, match, explicit caret", "$[?match(@, '^ab.*')]", []any{"abc", "ab"}},
		517: {"functions, match, explicit dollar", "$[?match(@, '.*bc$')]", []any{"abc"}},
	}
	corrected := 0
	for i, tc := range suite.Tests {
		if fix, ok := errata[i]; ok {
			if tc.Name != fix.name || tc.Selector != fix.selector || tc.Invalid || tc.Results != nil || !equivalent(tc.Result, fix.original) {
				t.Fatalf("CTS erratum %d no longer matches pinned case", i)
			}
			tc.Result = []any{}
			corrected++
			t.Logf("CTS case %d: applied RFC 9485 literal-anchor erratum", i)
		}
		t.Run(fmt.Sprintf("%03d_%s", i, tc.Name), func(t *testing.T) {
			p, err := e.Compile(context.Background(), tc.Selector)
			if tc.Invalid {
				if err == nil {
					t.Fatal("invalid selector compiled")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := p.Eval(context.Background(), tc.Document, jsontrans.EvalOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if !got.Defined {
				t.Fatal("undefined result")
			}
			expected := tc.Results
			if expected == nil {
				expected = []any{tc.Result}
			}
			for _, v := range expected {
				if equivalent(got.Value, v) {
					return
				}
			}
			t.Fatalf("got %#v; expected one of %#v", got.Value, expected)
		})
	}
	if corrected != len(errata) {
		t.Fatalf("applied %d of %d expected CTS errata", corrected, len(errata))
	}
}
