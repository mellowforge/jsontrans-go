//go:build oracle

package jsonata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/mellowforge/jsontrans-go"
)

func TestNestedFastPlansReference(t *testing.T) {
	expressions := []string{`Account.Order.Product.SKU`, `Account.Order.Product[UnitPrice>50].SKU`, `$sum(Account.Order.Product.(UnitPrice * Quantity * (1 - Discount)))`}
	inputs := []string{
		`{"Account":{"Order":[{"Product":[{"SKU":"A","UnitPrice":10,"Quantity":2,"Discount":0.25},{"SKU":"B","UnitPrice":60,"Quantity":3,"Discount":0.5}]},{"Product":{"SKU":"C","UnitPrice":5,"Quantity":4,"Discount":0}}]}}`,
		`{"Account":{"Order":{"Product":[]}}}`,
		`{"Account":{"Order":{"Product":{"SKU":null,"UnitPrice":70,"Quantity":1,"Discount":0}}}}`,
		`{"Account":{"Order":[[{"Product":{"SKU":"nested","UnitPrice":51,"Quantity":1,"Discount":0}}]]}}`,
		`{"Account":{"Order":{"Product":{"SKU":["A","B"],"UnitPrice":"bad","Quantity":1,"Discount":0}}}}`,
		`{}`,
	}
	ctx := context.Background()
	e, _ := New()
	var requests bytes.Buffer
	for _, input := range inputs {
		for _, expr := range expressions {
			if err := json.NewEncoder(&requests).Encode(map[string]any{"expression": expr, "input": json.RawMessage(input)}); err != nil {
				t.Fatal(err)
			}
		}
	}
	cmd := exec.Command("node", "../conformance/oracle/evaluate.cjs")
	cmd.Stdin = &requests
	output, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) != len(inputs)*len(expressions) {
		t.Fatalf("oracle returned %d lines", len(lines))
	}
	for i, input := range inputs {
		d, err := jsontrans.PrepareJSON(ctx, []byte(input), jsontrans.PrepareOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for j, expr := range expressions {
			var want struct {
				Error   bool
				Value   any
				Defined bool
				Code    string
			}
			if err = json.Unmarshal([]byte(lines[i*len(expressions)+j]), &want); err != nil {
				t.Fatal(err)
			}
			p, err := e.CompileRaw(ctx, expr)
			if err != nil {
				t.Fatal(err)
			}
			got, err := p.EvalPrepared(ctx, d, jsontrans.EvalOptions{})
			if want.Error {
				var native *jsontrans.Error
				if err == nil || errors.As(err, &native) && native.Code != want.Code || !errors.As(err, &native) && want.Code != "" {
					t.Errorf("%s %s: error %v want code %q", expr, input, err, want.Code)
				}
				continue
			}
			if err != nil || !reflect.DeepEqual(got, jsontrans.Result{Value: want.Value, Defined: want.Defined}) {
				t.Errorf("%s %s: got %#v/%v want %#v", expr, input, got, err, want)
			}
		}
	}
}
