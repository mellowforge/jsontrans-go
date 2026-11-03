package javascript

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	jt "github.com/mellowforge/jsontrans-go"
)

// Exercise conversion syntax and escaped strings without executing arbitrary
// source or allocating unbounded guest collections in fuzz workers.
func FuzzStringConversion(f *testing.F)      { fuzzStringConversion(f, Goja) }
func FuzzOwnedStringConversion(f *testing.F) { fuzzStringConversion(f, OwnedQuickJS) }
func fuzzStringConversion(f *testing.F, backend Backend) {
	for _, s := range []string{"plain", "😀", "\x00\n\"\\", "é", "\xff"} {
		f.Add(s)
	}
	e, err := New(backend, Config{Limits: jt.Limits{Timeout: semanticTestTimeout}})
	if err != nil {
		f.Fatal(err)
	}
	defer e.Close()
	p, err := e.Compile(context.Background(), `input=>({value:input})`)
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 2048 {
			return
		}
		data, _ := json.Marshal(s)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		out, err := jt.EvalJSON(ctx, p, data, jt.EvalOptions{})
		if errors.Is(err, context.DeadlineExceeded) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		var want string
		_ = json.Unmarshal(data, &want)
		var got struct{ Value string }
		if err = json.Unmarshal(out.Value, &got); err != nil {
			t.Fatal(err)
		}
		if !out.Defined || got.Value != want {
			t.Fatalf("got %#v, want %q", out, want)
		}
	})
}
