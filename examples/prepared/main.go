// Reuse one validated document for several independently compiled expressions.
package main

import (
	"context"
	"fmt"

	jt "github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/jsonata"
)

func main() {
	ctx := context.Background()
	engine, err := jsonata.New()
	if err != nil {
		panic(err)
	}
	d, err := jt.PrepareJSON(ctx, []byte(`{"name":"order","prices":[4,7]}`), jt.PrepareOptions{})
	if err != nil {
		panic(err)
	}
	for _, expression := range []string{`name`, `$sum(prices)`} {
		p, err := engine.Compile(ctx, expression)
		if err != nil {
			panic(err)
		}
		r, err := jt.EvalPrepared(ctx, p, d, jt.EvalOptions{})
		if err != nil {
			panic(err)
		}
		fmt.Printf("%s: %v (defined=%t)\n", expression, r.Value, r.Defined)
	}
}
