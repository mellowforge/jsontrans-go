package main

import (
	"context"
	"fmt"
	jt "github.com/mellowforge/jsontrans-go"
	js "github.com/mellowforge/jsontrans-go/javascript"
)

func main() {
	ctx := context.Background()
	e, err := js.New(ctx, js.Config{})
	if err != nil {
		panic(err)
	}
	defer e.Close()
	p, err := e.Compile(ctx, `()=>1+1`)
	if err != nil {
		panic(err)
	}
	r, err := p.Eval(ctx, nil, jt.EvalOptions{})
	fmt.Println(r.Value, err)
}
