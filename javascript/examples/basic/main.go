package main

import (
	"context"
	"fmt"
	jt "github.com/mellowforge/jsontrans-go"
	js "github.com/mellowforge/jsontrans-go/javascript"
	"log"
)

func main() {
	ctx := context.Background()
	engine, err := js.New(ctx, js.Config{})
	if err != nil {
		log.Fatal(err)
	}
	defer engine.Close()
	transformer, err := jt.New(map[string]jt.Processor{"js": engine})
	if err != nil {
		log.Fatal(err)
	}
	result, err := transformer.ProcessJSON(ctx, "js", []byte(`{"price":7}`), `(input,bindings)=>({total:input.price*bindings.quantity})`, jt.EvalOptions{Bindings: map[string]any{"quantity": 3}})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(result.Value))
}
