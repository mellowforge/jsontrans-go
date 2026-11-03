package javascript

import (
	"testing"

	jt "github.com/mellowforge/jsontrans-go"
)

func newTestEngine(t testing.TB, backend Backend) *Engine {
	t.Helper()
	e, err := New(backend, Config{Limits: jt.Limits{Timeout: semanticTestTimeout}})
	if err != nil {
		t.Fatalf("initialize %s fixture: %v", backend, err)
	}
	t.Cleanup(func() {
		if err := e.Close(); err != nil {
			t.Errorf("close %s fixture: %v", backend, err)
		}
	})
	return e
}
