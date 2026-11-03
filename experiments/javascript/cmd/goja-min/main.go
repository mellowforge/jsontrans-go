package main

import (
	"fmt"
	"github.com/dop251/goja"
)

func main() { v, e := goja.New().RunString("1+1"); fmt.Println(v, e) }
