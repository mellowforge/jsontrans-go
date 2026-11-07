// Compile at startup, prepare each message once, and return owned JSON bytes.
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
	x, err := jt.New(map[string]jt.Processor{"jsonata": engine})
	if err != nil {
		panic(err)
	}
	p, err := x.Compile(ctx, "jsonata", `{"total": $sum(prices)}`)
	if err != nil {
		panic(err)
	}
	batch, err := jt.NewBatch([]jt.Program{p, p})
	if err != nil {
		panic(err)
	}

	// In a subscription handler, input would be the current message payload.
	input := []byte(`{"prices":[4,7]}`)
	d, err := jt.PrepareJSON(ctx, input, jt.PrepareOptions{})
	if err != nil {
		panic(err)
	}
	r, err := jt.EvalPreparedJSON(ctx, p, d, jt.EvalOptions{})
	if err != nil {
		panic(err)
	}
	if r.Defined {
		fmt.Printf("single: %s\n", r.Value)
	}
	results, err := batch.EvalPreparedJSON(ctx, d, nil, jt.EvalOptions{})
	if err != nil {
		panic(err)
	}
	for i, r := range results {
		if r.Defined {
			fmt.Printf("fan-out %d: %s\n", i, r.Value)
		}
	}
}
