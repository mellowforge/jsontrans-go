package javascript

import (
	"context"
	jt "github.com/mellowforge/jsontrans-go"
	"testing"
)

func newTestEngine(t testing.TB) *Engine {
	t.Helper()
	e, err := New(context.Background(), Config{Limits: jt.Limits{Timeout: semanticTestTimeout}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := e.Close(); err != nil {
			t.Error(err)
		}
	})
	return e
}
