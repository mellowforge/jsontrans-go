package main

import (
	"fmt"
	"github.com/fastschema/qjs"
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
