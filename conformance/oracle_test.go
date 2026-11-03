//go:build oracle

package conformance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	jt "github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/jsonata"
)

func TestJSONataReference(t *testing.T) {
	expressions := []string{
		`$sum(items.price)`, `items[price > 5].name`, `{"names":items.name,"total":$sum(items.price)}`,
		`$map([1,2,3],function($x){$x*$x})`, `($f:=function($n){$n<2?1:$n*$f($n-1)};$f(5))`,
		`$formatInteger(123,"Ww")`, `$formatNumber(1234.5,"#,##0.00")`, `$fromMillis(0)`,
		`$match("abc123",/[0-9]+/)`, `$round(12.5)`, `null`, `missing`, `[]`, `[missing]`,
		`$sqrt(-1)`, `$sum(["bad"])`, `1 +`, `items^(>price).name`,
		`items.name`, `items[0].name`, `$count(items)`, `$sum(numbers)`, `$exists(items)`,
	}
	input := map[string]any{"items": []any{map[string]any{"name": "A", "price": 4}, map[string]any{"name": "B", "price": 7}}, "numbers": []any{1, 2, 3}}
	compareReference(t, expressions, input)
}

func TestJSONataReferenceOperators(t *testing.T) {
	values := []string{"0", "1", "-3", "1.5", `"a"`, `[1]`, `[]`, `null`, `missing`}
	operators := []string{"=", "!=", "<", ">", "and", "or", "&"}
	var expressions []string
	for _, left := range values {
		for _, right := range values {
			for _, operator := range operators {
				expressions = append(expressions, "("+left+") "+operator+" ("+right+")")
			}
		}
	}
	compareReference(t, expressions, nil)
}

func compareReference(t *testing.T, expressions []string, input any) {
	t.Helper()
	var requests bytes.Buffer
	for _, expression := range expressions {
		if err := json.NewEncoder(&requests).Encode(map[string]any{"expression": expression, "input": input}); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("node", "oracle/evaluate.cjs")
	cmd.Stdin = &requests
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("reference oracle unavailable (run npm ci --prefix conformance/oracle): %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) != len(expressions) {
		t.Fatalf("reference returned %d results", len(lines))
	}
	engine, err := jsonata.New()
	if err != nil {
		t.Fatal(err)
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	document, err := jt.PrepareJSON(context.Background(), inputJSON, jt.PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for i, expression := range expressions {
		t.Run(expression, func(t *testing.T) {
			var want struct {
				Defined bool
				Value   any
				Code    string
			}
			if err := json.Unmarshal([]byte(lines[i]), &want); err != nil {
				t.Fatal(err)
			}
			p, compileErr := engine.Compile(context.Background(), expression)
			check := func(t *testing.T, result jt.Result, err error) {
				t.Helper()
				if want.Code != "" {
					var native *jt.Error
					if !errors.As(err, &native) || native.Code != want.Code {
						t.Fatalf("reference code=%s; Go=%v", want.Code, err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				actual, err := json.Marshal(result.Value)
				if err != nil {
					t.Fatal(err)
				}
				var value any
				if err := json.Unmarshal(actual, &value); err != nil {
					t.Fatal(err)
				}
				if result.Defined != want.Defined || (want.Defined && !reflect.DeepEqual(value, want.Value)) {
					t.Fatalf("Go=%+v; reference=%+v", result, want)
				}
			}
			if compileErr != nil {
				check(t, jt.Result{}, compileErr)
				return
			}
			t.Run("ordinary", func(t *testing.T) {
				result, err := p.Eval(context.Background(), input, jt.EvalOptions{})
				check(t, result, err)
			})
			t.Run("prepared", func(t *testing.T) {
				result, err := jt.EvalPrepared(context.Background(), p, document, jt.EvalOptions{})
				check(t, result, err)
			})

		})
	}
}
