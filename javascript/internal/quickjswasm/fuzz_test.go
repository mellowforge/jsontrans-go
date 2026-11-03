package quickjswasm

import (
	"context"
	"testing"
	"time"
)

func FuzzOwnedCompile(f *testing.F) {
	for _, s := range []string{`()=>1`, `function(input){return input}`, `(`, "\x00", `const x = /[a-z]+/;`} {
		f.Add(s)
	}
	e, err := New(context.Background(), Config{Timeout: semanticTestTimeout})
	if err != nil {
		f.Fatal(err)
	}
	defer e.Close()
	f.Fuzz(func(t *testing.T, source string) {
		if len(source) > 4096 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		code, err := e.Compile(ctx, source)
		// Syntax/resource errors are legitimate. A host panic/crash, or success
		// without bounded independent bytecode, violates the boundary contract.
		if err == nil && (len(code) == 0 || len(code) > maxBytecode) {
			t.Fatalf("invalid compiled artifact size %d", len(code))
		}
	})
}
