package main

import (
	"context"
	"fmt"

	"github.com/mellowforge/jsontrans-go/experiments/javascript/internal/quickjswasm"
)

func main() {
	ctx := context.Background()
	e, err := quickjswasm.New(ctx, quickjswasm.Config{})
	if err != nil {
		panic(err)
	}
	defer e.Close()
	rule, err := e.Compile(ctx, `()=>1+1`)
	if err != nil {
		panic(err)
	}
	codec, err := e.Compile(ctx, `rule=>JSON.stringify(rule())`)
	if err != nil {
		panic(err)
	}
	result, err := e.Eval(ctx, rule, codec, `null`, `{}`, quickjswasm.Limits{Depth: 256, Items: 100000, Bytes: 1024, Work: 1000000})
	fmt.Println(result, err)
}
