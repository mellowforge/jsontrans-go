package jsontrans_test

import (
	"context"
	jt "github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/jsonata"
	"testing"
)

func TestJSONataPreservesSerializedKeyOrder(t *testing.T) {
	engine, err := jsonata.New()
	if err != nil {
		t.Fatal(err)
	}
	transformer, err := jt.New(map[string]jt.Processor{"jsonata": engine})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ input, expression, want string }{
		{`{"books":[],"loans":[],"customers":[]}`, `$keys($)`, `["books","loans","customers"]`},
		{`{"2":0,"b":0,"1":0,"a":0}`, `$keys($)`, `["1","2","b","a"]`},
		{`null`, `$keys({"2":0,"b":0,"1":0,"a":0})`, `["1","2","b","a"]`},
	} {
		result, err := transformer.ProcessJSON(context.Background(), "jsonata", []byte(tc.input), tc.expression, jt.EvalOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if string(result.Value) != tc.want {
			t.Fatalf("got %s want %s", result.Value, tc.want)
		}
	}
}
