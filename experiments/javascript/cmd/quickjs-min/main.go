package main

import (
	"fmt"
	qjs "github.com/mellowforge/jsontrans-go/experiments/javascript/internal/qjscompat"
)

func main() {
	r, e := qjs.New()
	if e != nil {
		panic(e)
	}
	defer r.Close()
	v, e := r.Eval("one.js", qjs.Code("1+1"))
	if v != nil {
		defer v.Free()
	}
	fmt.Println(v, e)
}
