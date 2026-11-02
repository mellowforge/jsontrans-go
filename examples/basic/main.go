package main

import (
	"context"
	"fmt"
	"log"

	"github.com/mellowforge/jsontrans-go"
	"github.com/mellowforge/jsontrans-go/jsonata"
	"github.com/mellowforge/jsontrans-go/jsonpath"
)

func main() {
	path, err := jsonpath.New()
	if err != nil {
		log.Fatal(err)
	}
	ata, err := jsonata.New()
	if err != nil {
		log.Fatal(err)
	}
	transformer, err := jsontrans.New(map[string]jsontrans.Processor{"jsonpath": path, "jsonata": ata})
	if err != nil {
		log.Fatal(err)
	}
	input := []byte(`{"items":[{"price":4},{"price":7}]}`)
	for _, query := range []struct{ engine, expression string }{{"jsonpath", "$.items[*].price"}, {"jsonata", "$sum(items.price)"}} {
		result, err := transformer.ProcessJSON(context.Background(), query.engine, input, query.expression, jsontrans.EvalOptions{})
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s: %s\n", query.engine, result.Value)
	}
}
